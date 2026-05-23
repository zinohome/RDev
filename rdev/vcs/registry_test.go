package vcs_test

import (
	"context"
	"testing"

	"github.com/zinohome/RDev/rdev/vcs"
)

type stubProvider struct{ name string }

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
	return nil, nil
}
func (s *stubProvider) GetFile(_ context.Context, _ vcs.RepoRef, _, _ string, _ int64) ([]byte, bool, error) {
	return nil, false, nil
}
func (s *stubProvider) CreatePR(_ context.Context, _ vcs.RepoRef, _ vcs.PRParams) (*vcs.PR, error) {
	return nil, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := vcs.NewRegistry()
	p := &stubProvider{name: "myprovider"}
	r.Register(p)

	got, err := r.Get("myprovider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name() != "myprovider" {
		t.Errorf("expected myprovider, got %s", got.Name())
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	r := vcs.NewRegistry()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRegistry_OverwriteOnDuplicate(t *testing.T) {
	r := vcs.NewRegistry()
	r.Register(&stubProvider{name: "foo"})
	r.Register(&stubProvider{name: "foo"}) // should overwrite, not panic

	_, err := r.Get("foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegistry_All(t *testing.T) {
	r := vcs.NewRegistry()
	r.Register(&stubProvider{name: "a"})
	r.Register(&stubProvider{name: "b"})
	r.Register(&stubProvider{name: "c"})

	all := r.All()
	if len(all) != 3 {
		t.Errorf("expected 3 providers, got %d", len(all))
	}
}
