package files_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/zinohome/RDev/rdev/files"
	"github.com/zinohome/RDev/rdev/vcs"
)

// stubProvider implements vcs.Provider for testing.
type stubProvider struct {
	name    string
	entries []vcs.TreeEntry
	content []byte
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) ListRepos(_ context.Context, _ string) ([]vcs.Repo, error) {
	return nil, nil
}
func (s *stubProvider) GetRepo(_ context.Context, _ vcs.RepoRef) (*vcs.Repo, error) {
	return nil, nil
}
func (s *stubProvider) ListBranches(_ context.Context, _ vcs.RepoRef) ([]vcs.Branch, error) {
	return nil, nil
}
func (s *stubProvider) GetTree(_ context.Context, _ vcs.RepoRef, _, _ string) ([]vcs.TreeEntry, error) {
	return s.entries, nil
}
func (s *stubProvider) GetFile(_ context.Context, _ vcs.RepoRef, _, _ string, _ int64) ([]byte, bool, error) {
	return s.content, false, nil
}
func (s *stubProvider) CreatePR(_ context.Context, _ vcs.RepoRef, _ vcs.PRParams) (*vcs.PR, error) {
	return nil, nil
}

func newTestRouter(reg *vcs.ProviderRegistry) chi.Router {
	r := chi.NewRouter()
	files.Register(r, files.Config{VCSRegistry: reg})
	return r
}

func TestVCSTree(t *testing.T) {
	reg := vcs.NewRegistry()
	reg.Register(&stubProvider{
		name: "gitea",
		entries: []vcs.TreeEntry{
			{Name: "README.md", Path: "README.md", IsDir: false, Size: 100},
			{Name: "src", Path: "src", IsDir: true},
		},
	})
	r := newTestRouter(reg)

	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/files/tree?source=vcs:gitea:owner/repo:main&path=.", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var entries []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0]["name"] != "README.md" {
		t.Errorf("expected README.md, got %v", entries[0]["name"])
	}
}

func TestVCSRead(t *testing.T) {
	reg := vcs.NewRegistry()
	reg.Register(&stubProvider{
		name:    "gitea",
		content: []byte("hello world"),
	})
	r := newTestRouter(reg)

	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/files/read?source=vcs:gitea:owner/repo:main&path=README.md", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["content"] != "hello world" {
		t.Errorf("expected 'hello world', got %v", resp["content"])
	}
	if resp["encoding"] != "utf-8" {
		t.Errorf("expected encoding 'utf-8', got %v", resp["encoding"])
	}
}

func TestBinaryFileDetection(t *testing.T) {
	reg := vcs.NewRegistry()
	reg.Register(&stubProvider{
		name:    "gitea",
		content: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG header
	})
	r := newTestRouter(reg)

	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/files/read?source=vcs:gitea:owner/repo:main&path=image.png", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["encoding"] != "binary" {
		t.Errorf("expected encoding 'binary', got %v", resp["encoding"])
	}
	if resp["content"] != nil && resp["content"] != "" {
		t.Errorf("expected no content for binary, got %v", resp["content"])
	}
}

func TestMissingSource(t *testing.T) {
	r := newTestRouter(vcs.NewRegistry())
	req := httptest.NewRequest(http.MethodGet, "/api/rdev/files/tree", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	r := newTestRouter(vcs.NewRegistry())
	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/files/tree?source=vcs:gitea:owner/repo:main&path=../../etc/passwd", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for path traversal, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "traversal") {
		t.Errorf("expected traversal error message, got %s", rec.Body.String())
	}
}

func TestInvalidSourceFormat(t *testing.T) {
	r := newTestRouter(vcs.NewRegistry())
	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/files/tree?source=badformat&path=.", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUnregisteredVCSProvider(t *testing.T) {
	r := newTestRouter(vcs.NewRegistry()) // empty registry
	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/files/tree?source=vcs:unknown:owner/repo:main&path=.", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRuntimeSourceWithoutHub(t *testing.T) {
	r := newTestRouter(vcs.NewRegistry()) // no hub
	req := httptest.NewRequest(http.MethodGet,
		"/api/rdev/files/tree?source=runtime:rt123:task456&path=.", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for hub not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}
