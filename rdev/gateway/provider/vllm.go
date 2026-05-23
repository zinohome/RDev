package provider

import (
	"bytes"
	"context"
	"net/http"
)

// VLLMProvider talks to a vLLM instance via its OpenAI-compatible API.
type VLLMProvider struct {
	cfg Config
}

func NewVLLM(cfg Config) *VLLMProvider {
	return &VLLMProvider{cfg: cfg}
}

func (p *VLLMProvider) Chat(ctx context.Context, _ string, body []byte, _ bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/chat/completions",
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}
