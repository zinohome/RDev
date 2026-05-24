package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zinohome/RDev/rdev/gateway/provider"
)

// -- request translation tests --

func TestTranslateRequest_Basic(t *testing.T) {
	req := AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 512,
		System:    "You are helpful.",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	out := translateRequest(req, "Qwen/Qwen2.5-Coder-32B")
	if out.Model != "Qwen/Qwen2.5-Coder-32B" {
		t.Errorf("unexpected model: %s", out.Model)
	}
	if out.MaxCompletionTokens != 512 {
		t.Errorf("max_completion_tokens: got %d", out.MaxCompletionTokens)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(out.Messages))
	}
	if out.Messages[0].Role != "system" {
		t.Errorf("first message should be system, got %s", out.Messages[0].Role)
	}
	if out.Messages[1].Role != "user" {
		t.Errorf("second message should be user, got %s", out.Messages[1].Role)
	}
}

func TestTranslateRequest_Tools(t *testing.T) {
	req := AnthropicRequest{
		Model: "claude-3-haiku",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "What's the weather?"},
		},
		Tools: []AnthropicTool{
			{
				Name:        "get_weather",
				Description: "Get weather for a location",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]string{"type": "string"},
					},
				},
			},
		},
	}

	out := translateRequest(req, "qwen2.5-coder:7b")
	if len(out.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(out.Tools))
	}
	if out.Tools[0].Type != "function" {
		t.Errorf("tool type: got %s", out.Tools[0].Type)
	}
	if out.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tool name: got %s", out.Tools[0].Function.Name)
	}
}

func TestTranslateRequest_ImageContent(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"type": "image",
			"source": map[string]interface{}{
				"type":       "base64",
				"media_type": "image/png",
				"data":       "abc123",
			},
		},
		map[string]interface{}{
			"type": "text",
			"text": "Describe this image",
		},
	}

	req := AnthropicRequest{
		Model:    "claude-3-opus",
		Messages: []AnthropicMessage{{Role: "user", Content: raw}},
	}

	out := translateRequest(req, "target-model")
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	// content should be a slice of parts
	parts, ok := out.Messages[0].Content.([]interface{})
	if !ok {
		t.Fatalf("expected content parts slice, got %T", out.Messages[0].Content)
	}
	if len(parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(parts))
	}
}

func TestTranslateRequest_ToolResult(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"type":        "tool_result",
			"tool_use_id": "toolu_01",
			"content":     "sunny",
		},
	}

	req := AnthropicRequest{
		Model:    "claude-3-sonnet",
		Messages: []AnthropicMessage{{Role: "user", Content: raw}},
	}

	out := translateRequest(req, "target")
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	msg := out.Messages[0]
	if msg.Role != "tool" {
		t.Errorf("expected role=tool, got %s", msg.Role)
	}
	if msg.ToolCallID != "toolu_01" {
		t.Errorf("expected tool_call_id=toolu_01, got %s", msg.ToolCallID)
	}
}

// -- response translation tests --

func TestTranslateResponse_Text(t *testing.T) {
	resp := provider.ChatResponse{
		ID: "chatcmpl-abc",
		Choices: []provider.Choice{
			{
				Message:      provider.Message{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: &provider.Usage{PromptTokens: 10, CompletionTokens: 5},
	}

	out := translateResponse(resp, "claude-3-sonnet")
	if out.Type != "message" {
		t.Errorf("type: %s", out.Type)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" {
		t.Fatalf("expected 1 text content block")
	}
	if out.Content[0].Text != "Hello!" {
		t.Errorf("text: %s", out.Content[0].Text)
	}
	if out.StopReason != "end_turn" {
		t.Errorf("stop_reason: %s", out.StopReason)
	}
	if out.Usage.InputTokens != 10 || out.Usage.OutputTokens != 5 {
		t.Errorf("usage: %+v", out.Usage)
	}
}

func TestTranslateResponse_ToolUse(t *testing.T) {
	resp := provider.ChatResponse{
		ID: "chatcmpl-xyz",
		Choices: []provider.Choice{
			{
				Message: provider.Message{
					Role: "assistant",
					ToolCalls: []provider.ToolCall{
						{
							ID:   "call_01",
							Type: "function",
							Function: provider.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location":"Tokyo"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	out := translateResponse(resp, "claude-3-sonnet")
	if len(out.Content) != 1 || out.Content[0].Type != "tool_use" {
		t.Fatalf("expected 1 tool_use block, got %+v", out.Content)
	}
	if out.Content[0].Name != "get_weather" {
		t.Errorf("tool name: %s", out.Content[0].Name)
	}
	if out.StopReason != "tool_use" {
		t.Errorf("stop_reason: %s", out.StopReason)
	}
	input, ok := out.Content[0].Input.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map input, got %T", out.Content[0].Input)
	}
	if input["location"] != "Tokyo" {
		t.Errorf("input.location: %v", input["location"])
	}
}

// -- streaming translation tests --

type fakeProvider struct {
	chunks []provider.StreamChunk
	resp   provider.ChatResponse
}

func (f *fakeProvider) ChatCompletions(_ context.Context, _ provider.ChatRequest) (provider.ChatResponse, error) {
	return f.resp, nil
}

func (f *fakeProvider) ChatCompletionsStream(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamChunk, error) {
	ch := make(chan provider.StreamChunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func TestTranslateStream_TextDelta(t *testing.T) {
	chunks := []provider.StreamChunk{
		{ID: "c1", Choices: []provider.StreamChoice{{Delta: &provider.Delta{Content: "Hello"}, Index: 0}}},
		{ID: "c2", Choices: []provider.StreamChoice{{Delta: &provider.Delta{Content: " world"}, Index: 0}}},
		{ID: "c3", Choices: []provider.StreamChoice{{FinishReason: "stop", Index: 0}},
			Usage: &provider.Usage{PromptTokens: 5, CompletionTokens: 3}},
	}

	ch := make(chan provider.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)

	rr := httptest.NewRecorder()
	translateStream(rr, rr, ch, "claude-3-sonnet")

	body := rr.Body.String()
	if !strings.Contains(body, "message_start") {
		t.Error("missing message_start event")
	}
	if !strings.Contains(body, "content_block_start") {
		t.Error("missing content_block_start event")
	}
	if !strings.Contains(body, "text_delta") {
		t.Error("missing text_delta event")
	}
	if !strings.Contains(body, "Hello") {
		t.Error("missing text content Hello")
	}
	if !strings.Contains(body, "message_stop") {
		t.Error("missing message_stop event")
	}
}

func TestTranslateStream_ToolUseMulti(t *testing.T) {
	chunks := []provider.StreamChunk{
		{Choices: []provider.StreamChoice{{Delta: &provider.Delta{
			ToolCalls: []provider.ToolCall{
				{Index: 0, ID: "call_01", Type: "function", Function: provider.FunctionCall{Name: "fn_a", Arguments: `{"a`}},
			},
		}}}},
		{Choices: []provider.StreamChoice{{Delta: &provider.Delta{
			ToolCalls: []provider.ToolCall{
				{Index: 0, Function: provider.FunctionCall{Arguments: `":1}`}},
				{Index: 1, ID: "call_02", Type: "function", Function: provider.FunctionCall{Name: "fn_b", Arguments: `{"b":2}`}},
			},
		}}}},
		{Choices: []provider.StreamChoice{{FinishReason: "tool_calls"}}},
	}

	ch := make(chan provider.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)

	rr := httptest.NewRecorder()
	translateStream(rr, rr, ch, "claude-3-sonnet")

	body := rr.Body.String()
	if strings.Count(body, "content_block_start") < 2 {
		t.Error("expected at least 2 content_block_start events for 2 tools")
	}
	if !strings.Contains(body, "input_json_delta") {
		t.Error("missing input_json_delta event")
	}
	if !strings.Contains(body, "fn_a") || !strings.Contains(body, "fn_b") {
		t.Error("missing tool names in stream")
	}
}

// -- auth tests --

func TestAuth_MissingToken(t *testing.T) {
	// no DB → allow all; test the extraction
	token := extractBearerToken(httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestAuth_BearerExtraction(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", "Bearer sk-test-123")
	token := extractBearerToken(r)
	if token != "sk-test-123" {
		t.Errorf("expected sk-test-123, got %q", token)
	}
}

func TestAuth_XAPIKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("x-api-key", "mytoken")
	token := extractBearerToken(r)
	if token != "mytoken" {
		t.Errorf("expected mytoken, got %q", token)
	}
}

// -- model router tests --

func TestModelRouter_PrefixMatch(t *testing.T) {
	rt, err := newModelRouter(`{
		"routes": [
			{"model_prefix": "claude-3-opus", "provider": "vllm", "target_model": "big-model"},
			{"model_prefix": "claude-3-haiku", "provider": "ollama", "target_model": "small-model"},
			{"model_prefix": "*", "provider": "vllm", "target_model": "default-model"}
		],
		"providers": {
			"vllm": {"base_url": "http://vllm:8000"},
			"ollama": {"base_url": "http://ollama:11434"}
		}
	}`)
	if err != nil {
		t.Fatalf("newModelRouter: %v", err)
	}

	cases := []struct {
		model    string
		wantProv string
		wantMdl  string
	}{
		{"claude-3-opus-20240229", "vllm", "big-model"},
		{"claude-3-haiku-20240307", "ollama", "small-model"},
		{"claude-3-sonnet-anything", "vllm", "default-model"},
		{"unknown-model", "vllm", "default-model"},
	}

	for _, tc := range cases {
		match, err := rt.Route(tc.model)
		if err != nil {
			t.Errorf("Route(%q) error: %v", tc.model, err)
			continue
		}
		if match.Provider != tc.wantProv {
			t.Errorf("Route(%q) provider: got %s, want %s", tc.model, match.Provider, tc.wantProv)
		}
		if match.TargetModel != tc.wantMdl {
			t.Errorf("Route(%q) model: got %s, want %s", tc.model, match.TargetModel, tc.wantMdl)
		}
	}
}

// -- integration: full request through server (no DB) --

func TestServer_NonStreaming(t *testing.T) {
	// Use default routes (no DB, allow all)
	handler, err := newServer("", "")
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	// We can't hit a real vLLM, so just verify routing and auth pass-through.
	body, _ := json.Marshal(AnthropicRequest{
		Model:     "claude-3-opus",
		MaxTokens: 10,
		Messages:  []AnthropicMessage{{Role: "user", Content: "hi"}},
	})

	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("x-api-key", "test-token")

	handler.ServeHTTP(rr, r)

	// We expect 502 because there's no real vLLM, but NOT 401 or 400
	if rr.Code == http.StatusUnauthorized {
		t.Errorf("got 401, auth should pass with no DB configured")
	}
	if rr.Code == http.StatusBadRequest {
		t.Errorf("got 400, request was valid")
	}
}

func TestServer_HealthCheck(t *testing.T) {
	handler, err := newServer("", "")
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}

	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		t.Errorf("health check: got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Errorf("health decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("health status: %s", resp["status"])
	}
}
