package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type authMiddleware struct {
	db *pgxpool.Pool
}

func newAuthMiddleware(db *pgxpool.Pool) *authMiddleware {
	return &authMiddleware{db: db}
}

func (a *authMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			writeAuthError(w, "missing API key")
			return
		}

		if !a.validateToken(r.Context(), token) {
			writeAuthError(w, "invalid or revoked API key")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	// Authorization: Bearer <token>
	if auth := r.Header.Get("Authorization"); auth != "" {
		if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return strings.TrimSpace(after)
		}
	}
	// x-api-key: <token>
	return strings.TrimSpace(r.Header.Get("x-api-key"))
}

func (a *authMiddleware) validateToken(ctx context.Context, token string) bool {
	if os.Getenv("RDEV_GATEWAY_NO_AUTH") == "1" {
		return true
	}
	if a.db == nil {
		return false
	}
	hash := sha256.Sum256([]byte(token))

	var exists bool
	err := a.db.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM gateway_token
			WHERE token_hash = $1 AND revoked_at IS NULL
		)`,
		hash[:],
	).Scan(&exists)
	return err == nil && exists
}

func writeAuthError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    "authentication_error",
			"message": msg,
		},
	})
}
