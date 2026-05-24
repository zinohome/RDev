// Package files implements the file browser HTTP API for RDev.
// Registers routes under /api/rdev/files/ supporting two source types:
//   - vcs:<providerID>:<owner>/<repo>:<branch>  — VCS provider (Gitea/GitHub)
//   - runtime:<runtimeID>:<taskID>              — daemon working directory via daemonws
package files

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/zinohome/RDev/rdev/vcs"
)

// Hub sends frames to a specific runtime's daemon and registers response handlers.
// Satisfied by an adapter wrapping *daemonws.Hub from the server.
type Hub interface {
	// SendFrameToRuntime delivers a JSON frame to all daemons connected under runtimeID.
	// Returns false when no daemon is connected.
	SendFrameToRuntime(runtimeID string, data []byte) bool
	// RegisterResponseHandler installs a handler for daemon→server messages of the given kind.
	// Must be called before any requests are made.
	RegisterResponseHandler(kind string, fn func(runtimeID string, payload []byte))
}

// Config holds dependencies injected at startup.
type Config struct {
	VCSRegistry *vcs.ProviderRegistry
	Hub         Hub // nil → runtime sources return 503
}

var defaultCfg Config

// Register mounts file browser routes onto r and stores hub/vcsReg for use by handlers.
// Must be called once, before any requests arrive.
func Register(r chi.Router, cfg Config) {
	defaultCfg = cfg
	if defaultCfg.VCSRegistry == nil {
		defaultCfg.VCSRegistry = vcs.NewRegistry()
	}
	r.Get("/api/rdev/files/tree", handleTree)
	r.Get("/api/rdev/files/read", handleRead)
	r.Get("/api/rdev/files/diff", handleDiff)
}

// source encoding:
//   vcs:<providerID>:<owner>/<repo>:<branch>
//   runtime:<runtimeID>:<taskID>
type sourceKind int

const (
	sourceVCS sourceKind = iota
	sourceRuntime
)

type parsedSource struct {
	kind       sourceKind
	providerID string
	owner      string
	repo       string
	branch     string
	runtimeID  string
	taskID     string
}

func parseSource(s string) (parsedSource, bool) {
	if strings.HasPrefix(s, "vcs:") {
		// vcs:<providerID>:<owner>/<repo>:<branch>
		rest := strings.TrimPrefix(s, "vcs:")
		parts := strings.SplitN(rest, ":", 3)
		if len(parts) != 3 {
			return parsedSource{}, false
		}
		slashIdx := strings.Index(parts[1], "/")
		if slashIdx < 0 {
			return parsedSource{}, false
		}
		return parsedSource{
			kind:       sourceVCS,
			providerID: parts[0],
			owner:      parts[1][:slashIdx],
			repo:       parts[1][slashIdx+1:],
			branch:     parts[2],
		}, true
	}
	if strings.HasPrefix(s, "runtime:") {
		// runtime:<runtimeID>:<taskID>
		rest := strings.TrimPrefix(s, "runtime:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return parsedSource{}, false
		}
		return parsedSource{
			kind:      sourceRuntime,
			runtimeID: parts[0],
			taskID:    parts[1],
		}, true
	}
	return parsedSource{}, false
}

func cleanPath(p string) (string, bool) {
	if p == "" {
		p = "."
	}
	cleaned := filepath.Clean(p)
	if strings.HasPrefix(cleaned, "..") {
		return "", false
	}
	return cleaned, true
}

type treeEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size,omitempty"`
	ModTime string `json:"mod_time,omitempty"`
}

type readResponse struct {
	Content   string `json:"content,omitempty"`
	Encoding  string `json:"encoding"`
	Truncated bool   `json:"truncated"`
}

type diffResponse struct {
	Patch string `json:"patch"`
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func handleTree(w http.ResponseWriter, r *http.Request) {
	src, err := getRequiredParam(r, "source")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "source parameter required")
		return
	}
	path := r.URL.Query().Get("path")
	cleanedPath, ok := cleanPath(path)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "path traversal not allowed")
		return
	}

	ps, ok := parseSource(src)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "invalid source format")
		return
	}

	switch ps.kind {
	case sourceVCS:
		handleVCSTree(w, r.Context(), ps, cleanedPath)
	case sourceRuntime:
		handleRuntimeTree(w, r.Context(), ps, cleanedPath)
	}
}

func handleRead(w http.ResponseWriter, r *http.Request) {
	src, err := getRequiredParam(r, "source")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "source parameter required")
		return
	}
	path := r.URL.Query().Get("path")
	cleanedPath, ok := cleanPath(path)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "path traversal not allowed")
		return
	}

	ps, ok := parseSource(src)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "invalid source format")
		return
	}

	switch ps.kind {
	case sourceVCS:
		handleVCSRead(w, r.Context(), ps, cleanedPath)
	case sourceRuntime:
		handleRuntimeRead(w, r.Context(), ps, cleanedPath)
	}
}

func handleDiff(w http.ResponseWriter, r *http.Request) {
	src, err := getRequiredParam(r, "source")
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "source parameter required")
		return
	}
	path := r.URL.Query().Get("path")
	cleanedPath, ok := cleanPath(path)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "path traversal not allowed")
		return
	}

	ps, ok := parseSource(src)
	if !ok {
		jsonErr(w, http.StatusBadRequest, "invalid source format")
		return
	}

	switch ps.kind {
	case sourceVCS:
		jsonErr(w, http.StatusNotImplemented, "diff not supported for vcs sources")
	case sourceRuntime:
		handleRuntimeDiff(w, r.Context(), ps, cleanedPath)
	}
}

func handleVCSTree(w http.ResponseWriter, ctx context.Context, ps parsedSource, path string) {
	provider, err := defaultCfg.VCSRegistry.Get(ps.providerID)
	if err != nil {
		jsonErr(w, http.StatusServiceUnavailable, "vcs provider not registered: "+ps.providerID)
		return
	}
	ref := vcs.RepoRef{ProviderID: ps.providerID, Owner: ps.owner, Repo: ps.repo}
	entries, err := provider.GetTree(ctx, ref, ps.branch, path)
	if err != nil {
		jsonErr(w, http.StatusBadGateway, "vcs tree error: "+err.Error())
		return
	}
	out := make([]treeEntry, len(entries))
	for i, e := range entries {
		out[i] = treeEntry{
			Name:    e.Name,
			Path:    e.Path,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func handleVCSRead(w http.ResponseWriter, ctx context.Context, ps parsedSource, path string) {
	provider, err := defaultCfg.VCSRegistry.Get(ps.providerID)
	if err != nil {
		jsonErr(w, http.StatusServiceUnavailable, "vcs provider not registered: "+ps.providerID)
		return
	}
	const maxBytes = 5 * 1024 * 1024 // 5 MB
	ref := vcs.RepoRef{ProviderID: ps.providerID, Owner: ps.owner, Repo: ps.repo}
	data, truncated, err := provider.GetFile(ctx, ref, ps.branch, path, maxBytes)
	if err != nil {
		jsonErr(w, http.StatusBadGateway, "vcs read error: "+err.Error())
		return
	}
	resp := readResponse{Truncated: truncated}
	if isUTF8(data) {
		resp.Content = string(data)
		resp.Encoding = "utf-8"
	} else {
		resp.Encoding = "binary"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleRuntimeTree(w http.ResponseWriter, ctx context.Context, ps parsedSource, path string) {
	if defaultCfg.Hub == nil {
		jsonErr(w, http.StatusServiceUnavailable, "runtime file access not configured")
		return
	}
	entries, err := RequestFileTree(ctx, defaultCfg.Hub, ps.runtimeID, path)
	if err != nil {
		jsonErr(w, http.StatusBadGateway, "runtime tree error: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func handleRuntimeRead(w http.ResponseWriter, ctx context.Context, ps parsedSource, path string) {
	if defaultCfg.Hub == nil {
		jsonErr(w, http.StatusServiceUnavailable, "runtime file access not configured")
		return
	}
	const maxBytes = 5 * 1024 * 1024
	resp, err := RequestFileRead(ctx, defaultCfg.Hub, ps.runtimeID, path, maxBytes)
	if err != nil {
		jsonErr(w, http.StatusBadGateway, "runtime read error: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleRuntimeDiff(w http.ResponseWriter, ctx context.Context, ps parsedSource, path string) {
	if defaultCfg.Hub == nil {
		jsonErr(w, http.StatusServiceUnavailable, "runtime file access not configured")
		return
	}
	base := "HEAD"
	resp, err := RequestFileDiff(ctx, defaultCfg.Hub, ps.runtimeID, path, base)
	if err != nil {
		jsonErr(w, http.StatusBadGateway, "runtime diff error: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func getRequiredParam(r *http.Request, key string) (string, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return "", &missingParamError{key}
	}
	return v, nil
}

type missingParamError struct{ param string }

func (e *missingParamError) Error() string { return "missing parameter: " + e.param }

// isUTF8 returns true when data is valid UTF-8.
func isUTF8(data []byte) bool {
	// strings.ToValidUTF8 approach: just check the whole slice.
	// unicode/utf8.Valid is the standard stdlib function.
	return utf8Valid(data)
}
