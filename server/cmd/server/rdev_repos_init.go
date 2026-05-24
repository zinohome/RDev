// rdev_repos_init.go wires REST-style file browser routes that the frontend expects.
// Routes: GET /api/rdev/repos/{providerId}/{owner}/{repo}/tree?ref=&path=
//          GET /api/rdev/repos/{providerId}/{owner}/{repo}/file?path=&ref=
// Provider credentials are loaded from the vcs_provider_binding table on each request.
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/extension"
	"github.com/zinohome/RDev/rdev/gitea"
	"github.com/zinohome/RDev/rdev/github"
	"github.com/zinohome/RDev/rdev/vcs"
)

var (
	reposPool     *pgxpool.Pool
	reposPoolOnce sync.Once
)

func getReposPool() *pgxpool.Pool {
	reposPoolOnce.Do(func() {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			dbURL = "postgres://multica:multica@localhost:5432/multica?sslmode=disable"
		}
		pool, err := pgxpool.New(context.Background(), dbURL)
		if err != nil {
			return
		}
		reposPool = pool
	})
	return reposPool
}

func reposVCSDecryptionKey() []byte {
	h := sha256.Sum256(auth.JWTSecret())
	return h[:]
}

func reposVCSDecrypt(ciphertext, key []byte) ([]byte, error) {
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

// loadVCSProvider loads a VCS provider from the database by UUID and returns a ready-to-use provider.
func loadVCSProvider(ctx context.Context, providerID string) (vcs.Provider, error) {
	pool := getReposPool()
	if pool == nil {
		return nil, io.ErrUnexpectedEOF
	}
	row := pool.QueryRow(ctx,
		`SELECT provider, base_url, token_encrypted FROM vcs_provider_binding WHERE id = $1`,
		providerID)

	var providerType, baseURL string
	var tokenEnc []byte
	if err := row.Scan(&providerType, &baseURL, &tokenEnc); err != nil {
		return nil, err
	}

	key := reposVCSDecryptionKey()
	tokenBytes, err := reposVCSDecrypt(tokenEnc, key)
	if err != nil {
		return nil, err
	}
	token := string(tokenBytes)

	switch providerType {
	case "github":
		return github.NewWithBase(baseURL, token), nil
	default:
		return gitea.New(baseURL, token), nil
	}
}

type repoTreeEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "blob" | "tree"
	Size int64  `json:"size,omitempty"`
}

func handleRepoTree(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "providerId")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := r.URL.Query().Get("ref")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	if ref == "" {
		ref = "main"
	}

	provider, err := loadVCSProvider(r.Context(), providerID)
	if err != nil {
		http.Error(w, `{"error":"provider not found"}`, http.StatusNotFound)
		return
	}

	entries, err := provider.GetTree(r.Context(), vcs.RepoRef{
		ProviderID: providerID, Owner: owner, Repo: repo,
	}, ref, path)
	if err != nil {
		http.Error(w, `{"error":"vcs tree error: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	out := make([]repoTreeEntry, len(entries))
	for i, e := range entries {
		t := "blob"
		if e.IsDir {
			t = "tree"
		}
		out[i] = repoTreeEntry{Name: e.Name, Path: e.Path, Type: t, Size: e.Size}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

type repoFileResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

func handleRepoFile(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "providerId")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	ref := r.URL.Query().Get("ref")
	path := r.URL.Query().Get("path")
	if ref == "" {
		ref = "main"
	}

	provider, err := loadVCSProvider(r.Context(), providerID)
	if err != nil {
		http.Error(w, `{"error":"provider not found"}`, http.StatusNotFound)
		return
	}

	const maxBytes = 5 * 1024 * 1024
	data, _, err := provider.GetFile(r.Context(), vcs.RepoRef{
		ProviderID: providerID, Owner: owner, Repo: repo,
	}, ref, path, maxBytes)
	if err != nil {
		http.Error(w, `{"error":"vcs file error: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repoFileResponse{
		Content:  string(data),
		Encoding: "utf-8",
	})
}

func init() {
	extension.RegisterExtensionRoutes(func(r chi.Router) {
		r.Get("/api/rdev/repos/{providerId}/{owner}/{repo}/tree", handleRepoTree)
		r.Get("/api/rdev/repos/{providerId}/{owner}/{repo}/file", handleRepoFile)
	})
}
