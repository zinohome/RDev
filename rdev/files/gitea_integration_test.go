package files_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/zinohome/RDev/rdev/files"
	"github.com/zinohome/RDev/rdev/gitea"
	"github.com/zinohome/RDev/rdev/vcs"
)

// TestGiteaIntegration tests the file browser API against a real Gitea instance.
// Requires RDEV_GITEA_URL and RDEV_GITEA_TOKEN to be set; skips otherwise.
func TestGiteaIntegration(t *testing.T) {
	url := os.Getenv("RDEV_GITEA_URL")
	token := os.Getenv("RDEV_GITEA_TOKEN")
	owner := os.Getenv("RDEV_GITEA_USER")
	repo := os.Getenv("RDEV_GITEA_REPO")
	if url == "" || token == "" {
		t.Skip("RDEV_GITEA_URL/RDEV_GITEA_TOKEN not set; skipping integration test")
	}
	if owner == "" {
		owner = "rdev-admin"
	}
	if repo == "" {
		repo = "rdev-test"
	}

	reg := vcs.NewRegistry()
	reg.Register(gitea.New(url, token))
	r := chi.NewRouter()
	files.Register(r, files.Config{VCSRegistry: reg})

	t.Run("tree", func(t *testing.T) {
		src := "vcs:gitea:" + owner + "/" + repo + ":main"
		req := httptest.NewRequest(http.MethodGet,
			"/api/rdev/files/tree?source="+src+"&path=.", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("tree: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var entries []map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
			t.Fatalf("tree: decode: %v", err)
		}
		if len(entries) == 0 {
			t.Error("tree: expected at least one entry")
		}
		t.Logf("tree returned %d entries", len(entries))
	})

	t.Run("read", func(t *testing.T) {
		src := "vcs:gitea:" + owner + "/" + repo + ":main"
		req := httptest.NewRequest(http.MethodGet,
			"/api/rdev/files/read?source="+src+"&path=README.md", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("read: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var result map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
			t.Fatalf("read: decode: %v", err)
		}
		if result["encoding"] != "utf-8" {
			t.Errorf("read: expected utf-8 encoding, got %v", result["encoding"])
		}
		t.Logf("read: encoding=%v truncated=%v content_len=%d",
			result["encoding"], result["truncated"],
			len(result["content"].(string)))
	})
}
