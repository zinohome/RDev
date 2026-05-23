package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/zinohome/RDev/rdev/vcs"
)

const defaultAPIBase = "https://api.github.com"

// Driver implements vcs.Provider for GitHub.
type Driver struct {
	apiBase string
	Token   string
	client  *http.Client
}

func New(token string) *Driver {
	return &Driver{
		apiBase: defaultAPIBase,
		Token:   token,
		client:  &http.Client{},
	}
}

// NewWithBase creates a Driver with a custom API base URL (useful for testing).
func NewWithBase(baseURL, token string) *Driver {
	return &Driver{
		apiBase: strings.TrimRight(baseURL, "/"),
		Token:   token,
		client:  &http.Client{},
	}
}

func (d *Driver) Name() string { return "github" }

func (d *Driver) do(ctx context.Context, method, apiPath string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, d.apiBase+apiPath, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+d.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("github: %s %s → %d: %s", method, apiPath, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (d *Driver) ListRepos(ctx context.Context, ownerOrOrg string) ([]vcs.Repo, error) {
	// try org first, fall back to user repos
	var out []vcs.Repo
	for page := 1; ; page++ {
		apiPath := fmt.Sprintf("/orgs/%s/repos?per_page=50&page=%d", ownerOrOrg, page)
		resp, err := d.do(ctx, http.MethodGet, apiPath, nil)
		if err != nil {
			// fall back to user repos
			apiPath = fmt.Sprintf("/users/%s/repos?per_page=50&page=%d", ownerOrOrg, page)
			resp, err = d.do(ctx, http.MethodGet, apiPath, nil)
			if err != nil {
				return nil, err
			}
		}
		var raw []struct {
			Name          string `json:"name"`
			FullName      string `json:"full_name"`
			Description   string `json:"description"`
			DefaultBranch string `json:"default_branch"`
			Private       bool   `json:"private"`
			CloneURL      string `json:"clone_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("github: decode repos: %w", err)
		}
		resp.Body.Close()
		for _, r := range raw {
			out = append(out, vcs.Repo{
				Name:          r.Name,
				FullName:      r.FullName,
				Description:   r.Description,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Private,
				CloneURL:      r.CloneURL,
			})
		}
		if len(raw) < 50 {
			break
		}
	}
	return out, nil
}

func (d *Driver) GetRepo(ctx context.Context, ref vcs.RepoRef) (*vcs.Repo, error) {
	apiPath := fmt.Sprintf("/repos/%s/%s", ref.Owner, ref.Repo)
	resp, err := d.do(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Name          string `json:"name"`
		FullName      string `json:"full_name"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		CloneURL      string `json:"clone_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("github: decode repo: %w", err)
	}
	return &vcs.Repo{
		Name:          r.Name,
		FullName:      r.FullName,
		Description:   r.Description,
		DefaultBranch: r.DefaultBranch,
		Private:       r.Private,
		CloneURL:      r.CloneURL,
	}, nil
}

func (d *Driver) ListBranches(ctx context.Context, ref vcs.RepoRef) ([]vcs.Branch, error) {
	apiPath := fmt.Sprintf("/repos/%s/%s/branches?per_page=100", ref.Owner, ref.Repo)
	resp, err := d.do(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("github: decode branches: %w", err)
	}
	out := make([]vcs.Branch, len(raw))
	for i, b := range raw {
		out[i] = vcs.Branch{Name: b.Name, Commit: b.Commit.SHA}
	}
	return out, nil
}

func (d *Driver) GetTree(ctx context.Context, ref vcs.RepoRef, branch, filePath string) ([]vcs.TreeEntry, error) {
	// get branch info to resolve root tree SHA
	branchPath := fmt.Sprintf("/repos/%s/%s/branches/%s", ref.Owner, ref.Repo, branch)
	resp, err := d.do(ctx, http.MethodGet, branchPath, nil)
	if err != nil {
		return nil, err
	}
	var branchInfo struct {
		Commit struct {
			Commit struct {
				Tree struct {
					SHA string `json:"sha"`
				} `json:"tree"`
			} `json:"commit"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&branchInfo); err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("github: decode branch: %w", err)
	}
	resp.Body.Close()

	treeSHA := branchInfo.Commit.Commit.Tree.SHA

	// if a sub-path is requested, resolve its tree SHA via the Contents API
	if filePath != "" {
		contentsPath := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", ref.Owner, ref.Repo, filePath, branch)
		cr, err := d.do(ctx, http.MethodGet, contentsPath, nil)
		if err != nil {
			return nil, err
		}
		var entry struct {
			SHA string `json:"sha"`
		}
		if err := json.NewDecoder(cr.Body).Decode(&entry); err != nil {
			cr.Body.Close()
			return nil, fmt.Errorf("github: decode contents entry: %w", err)
		}
		cr.Body.Close()
		treeSHA = entry.SHA
	}

	apiPath := fmt.Sprintf("/repos/%s/%s/git/trees/%s", ref.Owner, ref.Repo, treeSHA)
	resp2, err := d.do(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("github: decode tree: %w", err)
	}
	out := make([]vcs.TreeEntry, 0, len(tree.Tree))
	for _, e := range tree.Tree {
		name := path.Base(e.Path)
		out = append(out, vcs.TreeEntry{
			Name:  name,
			Path:  e.Path,
			IsDir: e.Type == "tree",
			Size:  e.Size,
		})
	}
	return out, nil
}

func (d *Driver) GetFile(ctx context.Context, ref vcs.RepoRef, branch, filePath string, maxBytes int64) ([]byte, bool, error) {
	if err := validatePath(filePath); err != nil {
		return nil, false, err
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", ref.Owner, ref.Repo, filePath, branch)
	resp, err := d.do(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	var content struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		Size     int64  `json:"size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return nil, false, fmt.Errorf("github: decode file content: %w", err)
	}

	var data []byte
	if content.Encoding == "base64" {
		// GitHub wraps base64 in newlines
		cleaned := strings.ReplaceAll(content.Content, "\n", "")
		data, err = base64.StdEncoding.DecodeString(cleaned)
		if err != nil {
			return nil, false, fmt.Errorf("github: base64 decode: %w", err)
		}
	} else {
		data = []byte(content.Content)
	}

	if int64(len(data)) > maxBytes {
		return data[:maxBytes], true, nil
	}
	return data, false, nil
}

func (d *Driver) CreatePR(ctx context.Context, ref vcs.RepoRef, params vcs.PRParams) (*vcs.PR, error) {
	apiPath := fmt.Sprintf("/repos/%s/%s/pulls", ref.Owner, ref.Repo)
	body := map[string]string{
		"title": params.Title,
		"body":  params.Body,
		"head":  params.Head,
		"base":  params.Base,
	}
	b, _ := json.Marshal(body)
	resp, err := d.do(ctx, http.MethodPost, apiPath, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var pr struct {
		Number int    `json:"number"`
		URL    string `json:"html_url"`
		Title  string `json:"title"`
		State  string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("github: decode pr: %w", err)
	}
	return &vcs.PR{
		Number: pr.Number,
		URL:    pr.URL,
		Title:  pr.Title,
		State:  pr.State,
	}, nil
}

// validatePath guards against path traversal attacks.
func validatePath(p string) error {
	if strings.Contains(p, "..") {
		return fmt.Errorf("vcs: path traversal not allowed: %q", p)
	}
	return nil
}
