package main

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type authMiddleware struct {
	db *pgxpool.Pool
}

func newAuthMiddleware(db *pgxpool.Pool) *authMiddleware {
	return &authMiddleware{db: db}
}

func (a *authMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no DB configured — allow all (dev mode)
		if a.db == nil {
			next.ServeHTTP(w, r)
			return
		}

		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"missing api key"}}`, http.StatusUnauthorized)
			return
		}

		hash := sha256.Sum256([]byte(token))

		var count int
		err := a.db.QueryRow(
			context.Background(),
			`SELECT COUNT(*) FROM gateway_token WHERE token_hash = $1 AND revoked_at IS NULL`,
			hash[:],
		).Scan(&count)
		if err != nil || count == 0 {
			http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractBearerToken(r *http.Request) string {
	if v := r.Header.Get("x-api-key"); v != "" {
		return v
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
