// Package files wsproto.go implements the WS request/response protocol for
// server↔daemon file operations using correlation IDs.
package files

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const wsRequestTimeout = 10 * time.Second

// fileRequest is sent from server to daemon.
type fileRequest struct {
	Kind      string          `json:"kind"`
	RequestID string          `json:"request_id"`
	Payload   json.RawMessage `json:"payload"`
}

// fileResponse is received from daemon to server.
type fileResponse struct {
	RequestID string          `json:"request_id"`
	Error     string          `json:"error,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type pendingRequest struct {
	ch chan fileResponse
}

var (
	pendingMu sync.Mutex
	pending   = map[string]*pendingRequest{}
)

// InitWS registers daemon→server response handlers for file operations.
// Must be called once with the server Hub before any file requests are made.
func InitWS(hub Hub) {
	hub.RegisterResponseHandler("rdev.file.tree.response", handleResponse)
	hub.RegisterResponseHandler("rdev.file.read.response", handleResponse)
	hub.RegisterResponseHandler("rdev.file.diff.response", handleResponse)
}

func handleResponse(_ string, payload []byte) {
	var resp fileResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return
	}
	pendingMu.Lock()
	pr, ok := pending[resp.RequestID]
	pendingMu.Unlock()
	if ok {
		select {
		case pr.ch <- resp:
		default:
		}
	}
}

func sendRequest(ctx context.Context, hub Hub, runtimeID, kind string, payloadObj any) (json.RawMessage, error) {
	payloadBytes, err := json.Marshal(payloadObj)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	reqID := uuid.New().String()
	frame, err := json.Marshal(fileRequest{
		Kind:      kind,
		RequestID: reqID,
		Payload:   payloadBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ch := make(chan fileResponse, 1)
	pendingMu.Lock()
	pending[reqID] = &pendingRequest{ch: ch}
	pendingMu.Unlock()
	defer func() {
		pendingMu.Lock()
		delete(pending, reqID)
		pendingMu.Unlock()
	}()

	if !hub.SendFrameToRuntime(runtimeID, frame) {
		return nil, fmt.Errorf("no daemon connected for runtime %q", runtimeID)
	}

	ctx, cancel := context.WithTimeout(ctx, wsRequestTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("daemon request timed out after %s", wsRequestTimeout)
	case resp := <-ch:
		if resp.Error != "" {
			return nil, fmt.Errorf("daemon error: %s", resp.Error)
		}
		return resp.Data, nil
	}
}

// fileTreeEntry matches the daemon's response format.
type fileTreeEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size,omitempty"`
	ModTime string `json:"mod_time,omitempty"`
}

// fileReadResult matches the daemon's response format.
type fileReadResult struct {
	Content   string `json:"content,omitempty"`
	Encoding  string `json:"encoding"`
	Truncated bool   `json:"truncated"`
}

// fileDiffResult matches the daemon's response format.
type fileDiffResult struct {
	Patch string `json:"patch"`
}

// RequestFileTree requests a directory listing from the daemon runtime.
func RequestFileTree(ctx context.Context, hub Hub, runtimeID, path string) ([]treeEntry, error) {
	data, err := sendRequest(ctx, hub, runtimeID, "rdev.file.tree", map[string]string{"path": path})
	if err != nil {
		return nil, err
	}
	var entries []fileTreeEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse tree response: %w", err)
	}
	out := make([]treeEntry, len(entries))
	for i, e := range entries {
		out[i] = treeEntry{Name: e.Name, Path: e.Path, IsDir: e.IsDir, Size: e.Size, ModTime: e.ModTime}
	}
	return out, nil
}

// RequestFileRead requests file contents from the daemon runtime.
func RequestFileRead(ctx context.Context, hub Hub, runtimeID, path string, maxBytes int64) (*readResponse, error) {
	data, err := sendRequest(ctx, hub, runtimeID, "rdev.file.read", map[string]any{
		"path":     path,
		"max_size": maxBytes,
	})
	if err != nil {
		return nil, err
	}
	var result fileReadResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse read response: %w", err)
	}
	return &readResponse{Content: result.Content, Encoding: result.Encoding, Truncated: result.Truncated}, nil
}

// RequestFileDiff requests a git diff from the daemon runtime.
func RequestFileDiff(ctx context.Context, hub Hub, runtimeID, path, base string) (*diffResponse, error) {
	data, err := sendRequest(ctx, hub, runtimeID, "rdev.file.diff", map[string]string{
		"path": path,
		"base": base,
	})
	if err != nil {
		return nil, err
	}
	var result fileDiffResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse diff response: %w", err)
	}
	return &diffResponse{Patch: result.Patch}, nil
}
