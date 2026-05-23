package provider

import (
	"context"
	"io"
	"net/http"
)

// LLMProvider forwards OpenAI-compatible requests to a backend LLM service.
type LLMProvider interface {
	// Chat sends a chat completions request and returns the raw response body.
	// The caller is responsible for closing the returned body.
	Chat(ctx context.Context, model string, body []byte, stream bool) (*http.Response, error)
}

// Config holds provider connection settings.
type Config struct {
	BaseURL string
}

// doRequest is a shared helper for HTTP POST to an OpenAI-compatible endpoint.
func doRequest(ctx context.Context, baseURL string, body []byte, stream bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/chat/completions", io.NopCloser(bytesReader(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))

	return http.DefaultClient.Do(req)
}

func bytesReader(b []byte) *bytesReaderImpl {
	return &bytesReaderImpl{data: b}
}

type bytesReaderImpl struct {
	data []byte
	pos  int
}

func (r *bytesReaderImpl) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
