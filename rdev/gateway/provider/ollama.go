package provider

import (
	"bytes"
	"context"
	"net/http"
)

// OllamaProvider talks to an Ollama instance via its OpenAI-compatible API.
// Ollama exposes /v1/chat/completions when started with OLLAMA_HOST set.
type OllamaProvider struct {
	cfg Config
}

func NewOllama(cfg Config) *OllamaProvider {
	return &OllamaProvider{cfg: cfg}
}

func (p *OllamaProvider) Chat(ctx context.Context, _ string, body []byte, _ bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}
