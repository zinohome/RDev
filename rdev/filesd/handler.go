// Package filesd implements the daemon-side file browser message handlers for RDev.
// The daemon process imports this package and calls Init() to register WS message
// handlers for rdev.file.tree, rdev.file.read, and rdev.file.diff.
package filesd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// MessageHandlerFunc processes a daemon WS message and returns a response.
type MessageHandlerFunc func(ctx context.Context, payload []byte) ([]byte, error)

// DaemonHub registers inbound message handlers for the daemon WS connection.
// Satisfied by the daemon's WS client.
type DaemonHub interface {
	RegisterMessageHandler(kind string, fn MessageHandlerFunc)
}

// workDirs is the sandbox whitelist, populated from RDEV_WORK_DIRS at Init time.
var workDirs []string

// Init registers file message handlers with the daemon hub.
// workDirsEnv is the value of RDEV_WORK_DIRS (colon-separated paths).
// If empty, os.Getenv("RDEV_WORK_DIRS") is used.
func Init(hub DaemonHub, workDirsEnv string) {
	if workDirsEnv == "" {
		workDirsEnv = os.Getenv("RDEV_WORK_DIRS")
	}
	workDirs = parseWorkDirs(workDirsEnv)

	hub.RegisterMessageHandler("rdev.file.tree", handleTree)
	hub.RegisterMessageHandler("rdev.file.read", handleRead)
	hub.RegisterMessageHandler("rdev.file.diff", handleDiff)
}

func parseWorkDirs(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ":") {
		p = strings.TrimSpace(p)
		if p != "" {
			abs, err := filepath.Abs(p)
			if err == nil {
				out = append(out, abs)
			}
		}
	}
	return out
}

// inSandbox checks that absPath is under one of the allowed work dirs.
func inSandbox(absPath string) bool {
	if len(workDirs) == 0 {
		return false
	}
	for _, dir := range workDirs {
		rel, err := filepath.Rel(dir, absPath)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

type fileRequest struct {
	Kind      string          `json:"kind"`
	RequestID string          `json:"request_id"`
	Payload   json.RawMessage `json:"payload"`
}

type fileResponse struct {
	RequestID string          `json:"request_id"`
	Error     string          `json:"error,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

func respond(requestID string, data any, errMsg string) ([]byte, error) {
	resp := fileResponse{RequestID: requestID}
	if errMsg != "" {
		resp.Error = errMsg
	} else {
		b, err := json.Marshal(data)
		if err != nil {
			resp.Error = "marshal error: " + err.Error()
		} else {
			resp.Data = b
		}
	}
	return json.Marshal(resp)
}

type treePayload struct {
	Path string `json:"path"`
}

type treeEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size,omitempty"`
	ModTime string `json:"mod_time,omitempty"`
}

func handleTree(_ context.Context, raw []byte) ([]byte, error) {
	var req fileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	var p treePayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return respond(req.RequestID, nil, "invalid payload: "+err.Error())
	}
	absPath, err := resolveAndValidate(p.Path)
	if err != nil {
		return respond(req.RequestID, nil, err.Error())
	}

	entries, err := listDir(absPath)
	if err != nil {
		return respond(req.RequestID, nil, fmt.Sprintf("read dir: %v", err))
	}
	return respond(req.RequestID, entries, "")
}

func listDir(absPath string) ([]treeEntry, error) {
	fis, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	out := make([]treeEntry, 0, len(fis))
	for _, fi := range fis {
		info, _ := fi.Info()
		var size int64
		var modTime string
		if info != nil {
			size = info.Size()
			modTime = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
		}
		out = append(out, treeEntry{
			Name:    fi.Name(),
			Path:    filepath.Join(absPath, fi.Name()),
			IsDir:   fi.IsDir(),
			Size:    size,
			ModTime: modTime,
		})
	}
	return out, nil
}

type readPayload struct {
	Path    string `json:"path"`
	MaxSize int64  `json:"max_size"`
}

type readResult struct {
	Content   string `json:"content,omitempty"`
	Encoding  string `json:"encoding"`
	Truncated bool   `json:"truncated"`
}

const defaultMaxSize = 5 * 1024 * 1024

func handleRead(_ context.Context, raw []byte) ([]byte, error) {
	var req fileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	var p readPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return respond(req.RequestID, nil, "invalid payload: "+err.Error())
	}
	absPath, err := resolveAndValidate(p.Path)
	if err != nil {
		return respond(req.RequestID, nil, err.Error())
	}

	maxBytes := p.MaxSize
	if maxBytes <= 0 {
		maxBytes = defaultMaxSize
	}

	f, err := os.Open(absPath)
	if err != nil {
		return respond(req.RequestID, nil, fmt.Sprintf("open file: %v", err))
	}
	defer f.Close()

	// Read at most maxBytes+1 to detect truncation.
	buf := make([]byte, maxBytes+1)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return respond(req.RequestID, nil, fmt.Sprintf("read file: %v", err))
	}
	data := buf[:n]
	truncated := int64(n) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}

	result := readResult{Truncated: truncated}
	if utf8.Valid(data) {
		result.Content = string(data)
		result.Encoding = "utf-8"
	} else {
		result.Encoding = "binary"
	}
	return respond(req.RequestID, result, "")
}

type diffPayload struct {
	Path string `json:"path"`
	Base string `json:"base"`
}

type diffResult struct {
	Patch string `json:"patch"`
}

func handleDiff(_ context.Context, raw []byte) ([]byte, error) {
	var req fileRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	var p diffPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return respond(req.RequestID, nil, "invalid payload: "+err.Error())
	}
	absPath, err := resolveAndValidate(p.Path)
	if err != nil {
		return respond(req.RequestID, nil, err.Error())
	}

	base := p.Base
	if base == "" {
		base = "HEAD"
	}

	// Run git diff in the directory containing the file.
	dir := filepath.Dir(absPath)
	out, err := exec.Command("git", "-C", dir, "diff", base, "--", absPath).Output()
	if err != nil {
		return respond(req.RequestID, nil, fmt.Sprintf("git diff: %v", err))
	}
	return respond(req.RequestID, diffResult{Patch: string(out)}, "")
}

// resolveAndValidate cleans the path and checks it's inside the sandbox.
func resolveAndValidate(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	// filepath.Clean removes .. traversals; Abs makes it absolute.
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if !inSandbox(abs) {
		return "", fmt.Errorf("path %q is outside allowed work directories (sandbox violation)", path)
	}
	return abs, nil
}
