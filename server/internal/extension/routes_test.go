package extension_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/extension"
)

func TestRegisterExtensionRoutes(t *testing.T) {
	r := chi.NewRouter()
	extension.RegisterExtensionRoutes(func(r chi.Router) {
		r.Get("/api/rdev/ping", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pong"))
		})
	})
	extension.MountAll(r)

	req := httptest.NewRequest("GET", "/api/rdev/ping", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "pong" {
		t.Errorf("expected pong, got %s", rr.Body.String())
	}
}
