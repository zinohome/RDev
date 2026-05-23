package github_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinohome/RDev/rdev/github"
	"github.com/zinohome/RDev/rdev/vcs"
)

// patchedDriver wraps Driver but overrides the base URL for testing.
// Since the real GitHub driver hardcodes api.github.com, we use a helper
// that creates a driver with a custom HTTP client pointing to our test server.

func newTestDriver(srv *httptest.Server) *github.Driver {
	d := github.NewWithBase(srv.URL, "test-token")
	return d
}

func TestGitHubListRepos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/orgs/myorg/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"name":           "myrepo",
				"full_name":      "myorg/myrepo",
				"description":    "test",
				"default_branch": "main",
				"private":        false,
				"clone_url":      "https://github.com/myorg/myrepo.git",
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	repos, err := d.ListRepos(context.Background(), "myorg")
	if err != nil {
		t.Fatalf("ListRepos error: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "myrepo" {
		t.Errorf("unexpected repos: %+v", repos)
	}
}

func TestGitHubGetRepo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/myrepo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":           "myrepo",
			"full_name":      "owner/myrepo",
			"description":    "desc",
			"default_branch": "main",
			"private":        false,
			"clone_url":      "https://github.com/owner/myrepo.git",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	repo, err := d.GetRepo(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetRepo error: %v", err)
	}
	if repo.FullName != "owner/myrepo" {
		t.Errorf("unexpected full_name: %s", repo.FullName)
	}
}

func TestGitHubListBranches(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/myrepo/branches", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"name": "main", "commit": map[string]string{"sha": "abc123"}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	branches, err := d.ListBranches(context.Background(), ref)
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}
	if len(branches) != 1 || branches[0].Name != "main" {
		t.Errorf("unexpected branches: %+v", branches)
	}
}

func TestGitHubGetTree(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/myrepo/branches/main", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"commit": map[string]interface{}{
				"sha": "commitsha",
				"commit": map[string]interface{}{
					"tree": map[string]string{"sha": "treesha"},
				},
			},
		})
	})
	mux.HandleFunc("/repos/owner/myrepo/git/trees/treesha", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tree": []map[string]interface{}{
				{"path": "README.md", "type": "blob", "size": 50},
				{"path": "src", "type": "tree", "size": 0},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	entries, err := d.GetTree(context.Background(), ref, "main", "")
	if err != nil {
		t.Fatalf("GetTree error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestGitHubGetFile(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("hello github"))
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/myrepo/contents/README.md", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content":  content,
			"encoding": "base64",
			"size":     12,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	data, truncated, err := d.GetFile(context.Background(), ref, "main", "README.md", 1024)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	if truncated {
		t.Error("expected not truncated")
	}
	if string(data) != "hello github" {
		t.Errorf("unexpected content: %q", data)
	}
}

func TestGitHubGetFileTruncated(t *testing.T) {
	bigContent := make([]byte, 200)
	for i := range bigContent {
		bigContent[i] = 'x'
	}
	encoded := base64.StdEncoding.EncodeToString(bigContent)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/myrepo/contents/big.txt", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content":  encoded,
			"encoding": "base64",
			"size":     200,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	data, truncated, err := d.GetFile(context.Background(), ref, "main", "big.txt", 50)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	if !truncated {
		t.Error("expected truncated=true")
	}
	if int64(len(data)) != 50 {
		t.Errorf("expected 50 bytes, got %d", len(data))
	}
}

func TestGitHubGetFilePathTraversal(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	_, _, err := d.GetFile(context.Background(), ref, "main", "../../etc/passwd", 1024)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestGitHubCreatePR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/myrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   7,
			"html_url": "https://github.com/owner/myrepo/pull/7",
			"title":    "Feature PR",
			"state":    "open",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := newTestDriver(srv)
	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	pr, err := d.CreatePR(context.Background(), ref, vcs.PRParams{
		Title: "Feature PR",
		Body:  "desc",
		Head:  "feature",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePR error: %v", err)
	}
	if pr.Number != 7 {
		t.Errorf("expected PR number 7, got %d", pr.Number)
	}
	_ = fmt.Sprintf("pr url: %s", pr.URL)
}
