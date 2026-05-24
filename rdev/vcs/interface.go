package vcs

import "context"

type RepoRef struct {
	ProviderID string
	Owner      string
	Repo       string
}

type Repo struct {
	Name          string
	FullName      string
	Description   string
	DefaultBranch string
	Private       bool
	CloneURL      string
}

type Branch struct {
	Name   string
	Commit string
}

type TreeEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime string
}

type PRParams struct {
	Title string
	Body  string
	Head  string
	Base  string
}

type PR struct {
	Number int
	URL    string
	Title  string
	State  string
}

type Provider interface {
	Name() string
	ListRepos(ctx context.Context, ownerOrOrg string) ([]Repo, error)
	GetRepo(ctx context.Context, ref RepoRef) (*Repo, error)
	ListBranches(ctx context.Context, ref RepoRef) ([]Branch, error)
	GetTree(ctx context.Context, ref RepoRef, branch, path string) ([]TreeEntry, error)
	GetFile(ctx context.Context, ref RepoRef, branch, path string, maxBytes int64) (content []byte, truncated bool, err error)
	CreatePR(ctx context.Context, ref RepoRef, params PRParams) (*PR, error)
}
