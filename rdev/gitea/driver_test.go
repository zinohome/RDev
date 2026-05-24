package gitea_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinohome/RDev/rdev/gitea"
	"github.com/zinohome/RDev/rdev/vcs"
)

func newTestServer(mux *http.ServeMux) (*httptest.Server, *gitea.Driver) {
	srv := httptest.NewServer(mux)
	d := gitea.New(srv.URL, "test-token")
	return srv, d
}

func TestGiteaListRepos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		result := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"name":           "myrepo",
					"full_name":      "owner/myrepo",
					"description":    "test repo",
					"default_branch": "main",
					"private":        false,
					"clone_url":      "https://gitea.example.com/owner/myrepo.git",
				},
			},
		}
		json.NewEncoder(w).Encode(result)
	})
	srv, d := newTestServer(mux)
	defer srv.Close()

	repos, err := d.ListRepos(context.Background(), "owner")
	if err != nil {
		t.Fatalf("ListRepos error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != "myrepo" {
		t.Errorf("expected myrepo, got %s", repos[0].Name)
	}
}

func TestGiteaGetRepo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/owner/myrepo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":           "myrepo",
			"full_name":      "owner/myrepo",
			"description":    "test",
			"default_branch": "main",
			"private":        true,
			"clone_url":      "https://gitea.example.com/owner/myrepo.git",
		})
	})
	srv, d := newTestServer(mux)
	defer srv.Close()

	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	repo, err := d.GetRepo(context.Background(), ref)
	if err != nil {
		t.Fatalf("GetRepo error: %v", err)
	}
	if repo.Name != "myrepo" {
		t.Errorf("expected myrepo, got %s", repo.Name)
	}
	if !repo.Private {
		t.Error("expected Private=true")
	}
}

func TestGiteaListBranches(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/owner/myrepo/branches", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"name": "main", "commit": map[string]string{"id": "abc123"}},
			{"name": "dev", "commit": map[string]string{"id": "def456"}},
		})
	})
	srv, d := newTestServer(mux)
	defer srv.Close()

	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	branches, err := d.ListBranches(context.Background(), ref)
	if err != nil {
		t.Fatalf("ListBranches error: %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}
	if branches[0].Name != "main" || branches[0].Commit != "abc123" {
		t.Errorf("unexpected branch[0]: %+v", branches[0])
	}
}

func TestGiteaGetTree(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/owner/myrepo/branches/main", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"commit": map[string]interface{}{
				"id": "sha1234",
			},
		})
	})
	mux.HandleFunc("/api/v1/repos/owner/myrepo/git/trees/sha1234", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tree": []map[string]interface{}{
				{"path": "README.md", "type": "blob", "size": 100},
				{"path": "src", "type": "tree", "size": 0},
			},
		})
	})
	srv, d := newTestServer(mux)
	defer srv.Close()

	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	entries, err := d.GetTree(context.Background(), ref, "main", "")
	if err != nil {
		t.Fatalf("GetTree error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestGiteaGetFile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/owner/myrepo/raw/main/README.md", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello world")
	})
	srv, d := newTestServer(mux)
	defer srv.Close()

	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	content, truncated, err := d.GetFile(context.Background(), ref, "main", "README.md", 1024)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	if truncated {
		t.Error("expected not truncated")
	}
	if string(content) != "hello world" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestGiteaGetFileTruncated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/owner/myrepo/raw/main/big.txt", func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 100; i++ {
			fmt.Fprint(w, "0123456789")
		}
	})
	srv, d := newTestServer(mux)
	defer srv.Close()

	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	content, truncated, err := d.GetFile(context.Background(), ref, "main", "big.txt", 50)
	if err != nil {
		t.Fatalf("GetFile error: %v", err)
	}
	if !truncated {
		t.Error("expected truncated=true")
	}
	if int64(len(content)) != 50 {
		t.Errorf("expected 50 bytes, got %d", len(content))
	}
}

func TestGiteaGetFilePathTraversal(t *testing.T) {
	mux := http.NewServeMux()
	srv, d := newTestServer(mux)
	defer srv.Close()

	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	_, _, err := d.GetFile(context.Background(), ref, "main", "../../etc/passwd", 1024)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestGiteaCreatePR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/repos/owner/myrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"number":   42,
			"html_url": "https://gitea.example.com/owner/myrepo/pulls/42",
			"title":    "My PR",
			"state":    "open",
		})
	})
	srv, d := newTestServer(mux)
	defer srv.Close()

	ref := vcs.RepoRef{Owner: "owner", Repo: "myrepo"}
	pr, err := d.CreatePR(context.Background(), ref, vcs.PRParams{
		Title: "My PR",
		Body:  "body",
		Head:  "feature",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePR error: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected PR number 42, got %d", pr.Number)
	}
}
