package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/zinohome/RDev/rdev/vcs"
)

// Driver implements vcs.Provider for Gitea.
type Driver struct {
	BaseURL string
	Token   string
	client  *http.Client
}

func New(baseURL, token string) *Driver {
	return &Driver{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		client:  &http.Client{},
	}
}

func (d *Driver) Name() string { return "gitea" }

func (d *Driver) do(ctx context.Context, method, apiPath string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, d.BaseURL+apiPath, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+d.Token)
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
		return nil, fmt.Errorf("gitea: %s %s → %d: %s", method, apiPath, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (d *Driver) ListRepos(ctx context.Context, ownerOrOrg string) ([]vcs.Repo, error) {
	var out []vcs.Repo
	for page := 1; page <= 4; page++ {
		apiPath := fmt.Sprintf("/api/v1/repos/search?owner=%s&limit=50&page=%d", url.QueryEscape(ownerOrOrg), page)
		resp, err := d.do(ctx, http.MethodGet, apiPath, nil)
		if err != nil {
			return nil, err
		}
		var result struct {
			Data []struct {
				Name          string `json:"name"`
				FullName      string `json:"full_name"`
				Description   string `json:"description"`
				DefaultBranch string `json:"default_branch"`
				Private       bool   `json:"private"`
				CloneURL      string `json:"clone_url"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("gitea: decode repos: %w", err)
		}
		resp.Body.Close()
		for _, r := range result.Data {
			out = append(out, vcs.Repo{
				Name:          r.Name,
				FullName:      r.FullName,
				Description:   r.Description,
				DefaultBranch: r.DefaultBranch,
				Private:       r.Private,
				CloneURL:      r.CloneURL,
			})
		}
		if len(result.Data) < 50 {
			break
		}
	}
	return out, nil
}

func (d *Driver) GetRepo(ctx context.Context, ref vcs.RepoRef) (*vcs.Repo, error) {
	apiPath := fmt.Sprintf("/api/v1/repos/%s/%s", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo))
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
		return nil, fmt.Errorf("gitea: decode repo: %w", err)
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
	apiPath := fmt.Sprintf("/api/v1/repos/%s/%s/branches", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo))
	resp, err := d.do(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw []struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("gitea: decode branches: %w", err)
	}
	out := make([]vcs.Branch, len(raw))
	for i, b := range raw {
		out[i] = vcs.Branch{Name: b.Name, Commit: b.Commit.ID}
	}
	return out, nil
}

func (d *Driver) GetTree(ctx context.Context, ref vcs.RepoRef, branch, filePath string) ([]vcs.TreeEntry, error) {
	// resolve branch to sha
	branchPath := fmt.Sprintf("/api/v1/repos/%s/%s/branches/%s",
		url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), url.PathEscape(branch))
	resp, err := d.do(ctx, http.MethodGet, branchPath, nil)
	if err != nil {
		return nil, err
	}
	var branchInfo struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&branchInfo); err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("gitea: decode branch info: %w", err)
	}
	resp.Body.Close()

	sha := branchInfo.Commit.ID
	apiPath := fmt.Sprintf("/api/v1/repos/%s/%s/git/trees/%s?recursive=false",
		url.PathEscape(ref.Owner), url.PathEscape(ref.Repo), url.PathEscape(sha))
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
			URL  string `json:"url"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("gitea: decode tree: %w", err)
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
	apiPath := fmt.Sprintf("/api/v1/repos/%s/%s/raw/%s/%s",
		url.PathEscape(ref.Owner), url.PathEscape(ref.Repo),
		url.PathEscape(branch), filePath)
	resp, err := d.do(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	lr := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, false, fmt.Errorf("gitea: read file: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], true, nil
	}
	return data, false, nil
}

func (d *Driver) CreatePR(ctx context.Context, ref vcs.RepoRef, params vcs.PRParams) (*vcs.PR, error) {
	apiPath := fmt.Sprintf("/api/v1/repos/%s/%s/pulls", url.PathEscape(ref.Owner), url.PathEscape(ref.Repo))
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
		return nil, fmt.Errorf("gitea: decode pr: %w", err)
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
	cleaned := path.Clean("/" + p)
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return fmt.Errorf("vcs: path traversal not allowed: %q", p)
		}
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("vcs: path traversal not allowed: %q", p)
	}
	return nil
}
