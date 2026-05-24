package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/util"
)

// vcsProviderResponse is the public shape returned to API callers.
// token_encrypted is never included — callers see a masked token hint.
type vcsProviderResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Provider    string `json:"provider"`
	BaseURL     string `json:"base_url"`
	DisplayName string `json:"display_name,omitempty"`
	TokenHint   string `json:"token_hint"` // first 6 chars + "****"
	CreatedAt   string `json:"created_at"`
}

type createVCSProviderRequest struct {
	Provider    string `json:"provider"`    // "gitea" | "github"
	BaseURL     string `json:"base_url"`    // e.g. "http://localhost:3030"
	Token       string `json:"token"`       // plaintext PAT; stored encrypted
	DisplayName string `json:"display_name"` // optional human-readable label
}

// vcsEncryptionKey derives a 32-byte AES key from JWT_SECRET via SHA-256.
func vcsEncryptionKey() []byte {
	h := sha256.Sum256(auth.JWTSecret())
	return h[:]
}

func vcsEncrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func vcsDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, io.ErrUnexpectedEOF
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ct, nil)
}

func tokenHint(token string) string {
	if len(token) <= 6 {
		return "****"
	}
	return token[:6] + "****"
}

// ListVCSProviders returns all VCS provider bindings for a workspace.
// Requires workspace member role.
func (h *Handler) ListVCSProviders(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace id required")
		return
	}

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, workspace_id, provider, base_url, display_name, token_encrypted, created_at
		 FROM vcs_provider_binding WHERE workspace_id = $1 ORDER BY created_at`,
		util.MustParseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}
	defer rows.Close()

	key := vcsEncryptionKey()
	var result []vcsProviderResponse
	for rows.Next() {
		var (
			id, wsID, provider, baseURL string
			displayName                 *string
			tokenEnc                    []byte
			createdAt                   time.Time
		)
		if err := rows.Scan(&id, &wsID, &provider, &baseURL, &displayName, &tokenEnc, &createdAt); err != nil {
			continue
		}
		hint := "****"
		if plain, err := vcsDecrypt(tokenEnc, key); err == nil {
			hint = tokenHint(string(plain))
		}
		dn := ""
		if displayName != nil {
			dn = *displayName
		}
		result = append(result, vcsProviderResponse{
			ID:          id,
			WorkspaceID: wsID,
			Provider:    provider,
			BaseURL:     baseURL,
			DisplayName: dn,
			TokenHint:   hint,
			CreatedAt:   createdAt.Format(time.RFC3339),
		})
	}
	if result == nil {
		result = []vcsProviderResponse{}
	}
	writeJSON(w, http.StatusOK, result)
}

// CreateVCSProvider adds a VCS provider binding to a workspace.
// Requires workspace admin or owner role.
func (h *Handler) CreateVCSProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace id required")
		return
	}

	var req createVCSProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	req.Token = strings.TrimSpace(req.Token)

	if req.Provider != "gitea" && req.Provider != "github" {
		writeError(w, http.StatusBadRequest, "provider must be 'gitea' or 'github'")
		return
	}
	if req.BaseURL == "" {
		writeError(w, http.StatusBadRequest, "base_url is required")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	key := vcsEncryptionKey()
	tokenEnc, err := vcsEncrypt([]byte(req.Token), key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt token")
		return
	}

	var displayName *string
	if req.DisplayName != "" {
		displayName = &req.DisplayName
	}

	var (
		id, wsID, provider, baseURL string
		dn                          *string
		createdAt                   time.Time
	)
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO vcs_provider_binding (workspace_id, provider, base_url, token_encrypted, display_name)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, workspace_id, provider, base_url, display_name, created_at`,
		util.MustParseUUID(workspaceID), req.Provider, req.BaseURL, tokenEnc, displayName,
	).Scan(&id, &wsID, &provider, &baseURL, &dn, &createdAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create provider")
		return
	}

	dnStr := ""
	if dn != nil {
		dnStr = *dn
	}
	writeJSON(w, http.StatusCreated, vcsProviderResponse{
		ID:          id,
		WorkspaceID: wsID,
		Provider:    provider,
		BaseURL:     baseURL,
		DisplayName: dnStr,
		TokenHint:   tokenHint(req.Token),
		CreatedAt:   createdAt.Format(time.RFC3339),
	})
}

// DeleteVCSProvider removes a VCS provider binding.
// Requires workspace admin or owner role.
func (h *Handler) DeleteVCSProvider(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	providerID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "providerId"), "providerId")
	if !ok {
		return
	}

	tag, err := h.DB.Exec(r.Context(),
		`DELETE FROM vcs_provider_binding WHERE id = $1 AND workspace_id = $2`,
		providerID, util.MustParseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete provider")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
