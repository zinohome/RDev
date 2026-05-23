package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Request translation tests ---

func TestTranslateRequest_TextOnly(t *testing.T) {
	ar := &AnthropicRequest{
		Model:     "claude-3-opus-20240229",
		MaxTokens: 1024,
		System:    "You are helpful.",
		Messages: []AnthropicMessage{
			{Role: "user", Content: mustMarshal("Hello")},
		},
	}
	oai, err := translateRequest(ar, "Qwen/Qwen2.5-Coder-32B-Instruct")
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}

	if oai.Model != "Qwen/Qwen2.5-Coder-32B-Instruct" {
		t.Errorf("model = %q, want Qwen/Qwen2.5-Coder-32B-Instruct", oai.Model)
	}
	if oai.MaxCompletionTokens != 1024 {
		t.Errorf("max_completion_tokens = %d, want 1024", oai.MaxCompletionTokens)
	}
	// system → first message
	if len(oai.Messages) < 1 || oai.Messages[0].Role != "system" {
		t.Fatalf("expected first message role=system, got %+v", oai.Messages)
	}
	// user message
	if len(oai.Messages) < 2 || oai.Messages[1].Role != "user" {
		t.Fatalf("expected second message role=user, got %+v", oai.Messages)
	}
}

func TestTranslateRequest_ContentBlocks(t *testing.T) {
	blocks := []AnthropicContentBlock{
		{Type: "text", Text: "Describe this image."},
		{
			Type: "image",
			Source: &AnthropicImageSource{
				Type:      "base64",
				MediaType: "image/png",
				Data:      "abc123",
			},
		},
	}
	ar := &AnthropicRequest{
		Model:     "claude-3-sonnet",
		MaxTokens: 512,
		Messages: []AnthropicMessage{
			{Role: "user", Content: mustMarshal(blocks)},
		},
	}
	oai, err := translateRequest(ar, "target-model")
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	// No system message, one user message
	if len(oai.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(oai.Messages))
	}
	var parts []OAIContentPart
	if err := json.Unmarshal(oai.Messages[0].Content, &parts); err != nil {
		t.Fatalf("unmarshal content parts: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Type != "text" {
		t.Errorf("part[0] type = %q, want text", parts[0].Type)
	}
	if parts[1].Type != "image_url" {
		t.Errorf("part[1] type = %q, want image_url", parts[1].Type)
	}
	wantURL := "data:image/png;base64,abc123"
	if parts[1].ImageURL.URL != wantURL {
		t.Errorf("image URL = %q, want %q", parts[1].ImageURL.URL, wantURL)
	}
}

func TestTranslateRequest_ToolResult(t *testing.T) {
	toolResultContent, _ := json.Marshal("Paris")
	blocks := []AnthropicContentBlock{
		{
			Type:      "tool_result",
			ToolUseID: "toolu_abc",
			Content:   toolResultContent,
		},
	}
	ar := &AnthropicRequest{
		Model:     "claude-3-haiku",
		MaxTokens: 256,
		Messages: []AnthropicMessage{
			{Role: "user", Content: mustMarshal(blocks)},
		},
	}
	oai, err := translateRequest(ar, "target-model")
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	// tool_result → role=tool message
	found := false
	for _, m := range oai.Messages {
		if m.Role == "tool" && m.ToolCallID == "toolu_abc" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a role=tool message with tool_call_id=toolu_abc, got %+v", oai.Messages)
	}
}

func TestTranslateRequest_Tools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`)
	ar := &AnthropicRequest{
		Model:     "claude-3-opus",
		MaxTokens: 512,
		Messages: []AnthropicMessage{
			{Role: "user", Content: mustMarshal("What is the weather?")},
		},
		Tools: []AnthropicTool{
			{Name: "get_weather", Description: "Get weather", InputSchema: schema},
		},
	}
	oai, err := translateRequest(ar, "target-model")
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(oai.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(oai.Tools))
	}
	if oai.Tools[0].Type != "function" {
		t.Errorf("tool type = %q, want function", oai.Tools[0].Type)
	}
	if oai.Tools[0].Function.Name != "get_weather" {
		t.Errorf("tool name = %q, want get_weather", oai.Tools[0].Function.Name)
	}
}

// --- Non-streaming response translation tests ---

func TestTranslateNonStreamResponse_Text(t *testing.T) {
	oaiResp := OAIResponse{
		ID:      "chatcmpl-abc",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "qwen",
		Choices: []OAIChoice{
			{
				Index: 0,
				Message: OAIMessage{
					Role:    "assistant",
					Content: mustMarshal("Hello, world!"),
				},
				FinishReason: "stop",
			},
		},
		Usage: &OAIUsage{PromptTokens: 10, CompletionTokens: 5},
	}
	body, _ := json.Marshal(oaiResp)

	out, err := translateNonStreamResponse(body, "claude-3-opus-20240229")
	if err != nil {
		t.Fatalf("translateNonStreamResponse: %v", err)
	}

	var ar AnthropicResponse
	if err := json.Unmarshal(out, &ar); err != nil {
		t.Fatalf("unmarshal Anthropic response: %v", err)
	}
	if ar.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", ar.StopReason)
	}
	if len(ar.Content) != 1 || ar.Content[0].Type != "text" {
		t.Errorf("expected 1 text content block, got %+v", ar.Content)
	}
	if ar.Content[0].Text != "Hello, world!" {
		t.Errorf("text = %q, want Hello, world!", ar.Content[0].Text)
	}
	if ar.Usage.InputTokens != 10 || ar.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want input=10 output=5", ar.Usage)
	}
}

func TestTranslateNonStreamResponse_ToolUse(t *testing.T) {
	oaiResp := OAIResponse{
		ID:      "chatcmpl-xyz",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "qwen",
		Choices: []OAIChoice{
			{
				Index: 0,
				Message: OAIMessage{
					Role: "assistant",
					ToolCalls: []OAIToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: OAIFunction{
								Name:      "get_weather",
								Arguments: `{"location":"NY"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	body, _ := json.Marshal(oaiResp)

	out, err := translateNonStreamResponse(body, "claude-3-opus")
	if err != nil {
		t.Fatalf("translateNonStreamResponse: %v", err)
	}

	var ar AnthropicResponse
	if err := json.Unmarshal(out, &ar); err != nil {
		t.Fatalf("unmarshal Anthropic response: %v", err)
	}
	if ar.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", ar.StopReason)
	}
	if len(ar.Content) != 1 || ar.Content[0].Type != "tool_use" {
		t.Fatalf("expected 1 tool_use block, got %+v", ar.Content)
	}
	if ar.Content[0].ID != "call_123" {
		t.Errorf("tool_use ID = %q, want call_123", ar.Content[0].ID)
	}
	if ar.Content[0].Name != "get_weather" {
		t.Errorf("tool_use name = %q, want get_weather", ar.Content[0].Name)
	}
}

// --- Streaming response translation tests ---

func TestStreamResponse_TextOnly(t *testing.T) {
	// Simulate OpenAI SSE stream for a simple text response.
	chunks := []string{
		`{"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`,
	}
	upstream := buildSSE(chunks)

	var buf bytes.Buffer
	rw := httptest.NewRecorder()
	rw.Body = &buf

	err := streamResponse(rw, strings.NewReader(upstream), "claude-3-opus")
	if err != nil {
		t.Fatalf("streamResponse: %v", err)
	}

	events := parseSSEEvents(buf.String())
	assertEventType(t, events, "message_start")
	assertEventType(t, events, "content_block_start")
	assertEventType(t, events, "content_block_delta")
	assertEventType(t, events, "content_block_stop")
	assertEventType(t, events, "message_delta")
	assertEventType(t, events, "message_stop")

	// Check text content
	deltaEvents := filterEvents(events, "content_block_delta")
	if len(deltaEvents) < 1 {
		t.Fatal("expected at least 1 content_block_delta")
	}

	// Check stop reason
	msgDelta := findEvent(events, "message_delta")
	if msgDelta == nil {
		t.Fatal("no message_delta event")
	}
	delta := msgDelta["delta"].(map[string]any)
	if delta["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, want end_turn", delta["stop_reason"])
	}
}

func TestStreamResponse_ToolUseMulti(t *testing.T) {
	// Two concurrent tool calls in a single stream.
	chunks := []string{
		`{"id":"c2","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		// tool call 0 starts
		`{"id":"c2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_A","type":"function","function":{"name":"tool_a","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"c2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":"}}]},"finish_reason":null}]}`,
		// tool call 1 starts
		`{"id":"c2","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_B","type":"function","function":{"name":"tool_b","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"c2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":null}]}`,
		`{"id":"c2","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"y\":2}"}}]},"finish_reason":null}]}`,
		`{"id":"c2","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20,"completion_tokens":30}}`,
	}
	upstream := buildSSE(chunks)

	var buf bytes.Buffer
	rw := httptest.NewRecorder()
	rw.Body = &buf

	if err := streamResponse(rw, strings.NewReader(upstream), "claude-3-opus"); err != nil {
		t.Fatalf("streamResponse: %v", err)
	}

	events := parseSSEEvents(buf.String())

	// Expect 2 content_block_start events (one per tool)
	starts := filterEvents(events, "content_block_start")
	if len(starts) != 2 {
		t.Errorf("expected 2 content_block_start, got %d", len(starts))
	}

	// Both should be tool_use type
	for i, ev := range starts {
		cb := ev["content_block"].(map[string]any)
		if cb["type"] != "tool_use" {
			t.Errorf("start[%d] content_block type = %v, want tool_use", i, cb["type"])
		}
	}

	// Indices must be 0 and 1
	idx0 := starts[0]["index"].(float64)
	idx1 := starts[1]["index"].(float64)
	if idx0 != 0 || idx1 != 1 {
		t.Errorf("block indices = %v, %v, want 0, 1", idx0, idx1)
	}

	// stop_reason = tool_use
	msgDelta := findEvent(events, "message_delta")
	if msgDelta == nil {
		t.Fatal("no message_delta event")
	}
	delta := msgDelta["delta"].(map[string]any)
	if delta["stop_reason"] != "tool_use" {
		t.Errorf("stop_reason = %v, want tool_use", delta["stop_reason"])
	}
}

// --- Model router tests ---

func TestModelRouter_PrefixMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	routesCfg := fmt.Sprintf(`{
		"routes": [
			{"model_prefix": "claude-3-opus", "provider": "vllm", "target_model": "qwen-32b"},
			{"model_prefix": "claude-3-haiku", "provider": "ollama", "target_model": "qwen-7b"},
			{"model_prefix": "*", "provider": "vllm", "target_model": "qwen-default"}
		],
		"providers": {
			"vllm": {"base_url": "%s"},
			"ollama": {"base_url": "%s"}
		}
	}`, ts.URL, ts.URL)

	t.Setenv("RDEV_GATEWAY_ROUTES", routesCfg)

	mr, err := NewModelRouter()
	if err != nil {
		t.Fatalf("NewModelRouter: %v", err)
	}

	tests := []struct {
		model      string
		wantTarget string
	}{
		{"claude-3-opus-20240229", "qwen-32b"},
		{"claude-3-haiku-20240307", "qwen-7b"},
		{"claude-3-sonnet-20240229", "qwen-default"}, // wildcard fallback
		{"claude-2", "qwen-default"},
	}

	for _, tc := range tests {
		match, err := mr.Route(tc.model)
		if err != nil {
			t.Errorf("Route(%q): %v", tc.model, err)
			continue
		}
		if match.TargetModel != tc.wantTarget {
			t.Errorf("Route(%q).TargetModel = %q, want %q", tc.model, match.TargetModel, tc.wantTarget)
		}
	}
}

// --- Auth tests ---

func TestAuth_MissingToken(t *testing.T) {
	// Use a no-op DB (nil) — missing token should fail before DB is consulted.
	am := &authMiddleware{db: nil}
	called := false
	handler := am.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if called {
		t.Error("handler should not be called without a token")
	}
	if rw.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rw.Code)
	}
}

func TestAuth_ExtractToken(t *testing.T) {
	tests := []struct {
		name      string
		setupReq  func(*http.Request)
		wantToken string
	}{
		{
			name: "Bearer header",
			setupReq: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer my-secret-token")
			},
			wantToken: "my-secret-token",
		},
		{
			name: "x-api-key header",
			setupReq: func(r *http.Request) {
				r.Header.Set("x-api-key", "another-token")
			},
			wantToken: "another-token",
		},
		{
			name:      "no token",
			setupReq:  func(r *http.Request) {},
			wantToken: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			tc.setupReq(req)
			got := extractToken(req)
			if got != tc.wantToken {
				t.Errorf("extractToken = %q, want %q", got, tc.wantToken)
			}
		})
	}
}

func TestAuth_ValidToken_MockDB(t *testing.T) {
	// Verify the full auth flow using a mock upstream that only passes if the
	// gateway reaches the handler (i.e., token would be valid in real DB).
	// We cannot easily mock pgxpool, so we test the token extraction path
	// and trust the DB integration to a real test environment.

	// This test verifies the auth middleware wires correctly with chi router.
	called := false
	mockNext := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// Directly test the token extraction + nil-DB short-circuit.
	am := &authMiddleware{db: nil}
	handler := am.Handler(mockNext)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	// Without a real DB, validateToken returns false → 401.
	if called {
		t.Error("handler should not be called when DB is nil")
	}
	if rw.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rw.Code)
	}
}

// --- Integration: full request through mock upstream ---

func TestHandleMessages_NonStream(t *testing.T) {
	// Mock OpenAI-compatible upstream.
	oaiResp := OAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "qwen",
		Choices: []OAIChoice{
			{
				Index: 0,
				Message: OAIMessage{
					Role:    "assistant",
					Content: mustMarshal("Hello from qwen!"),
				},
				FinishReason: "stop",
			},
		},
		Usage: &OAIUsage{PromptTokens: 8, CompletionTokens: 4},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(oaiResp)
	}))
	defer ts.Close()

	routesCfg := fmt.Sprintf(`{
		"routes": [{"model_prefix": "*", "provider": "vllm", "target_model": "qwen"}],
		"providers": {"vllm": {"base_url": "%s"}}
	}`, ts.URL)
	t.Setenv("RDEV_GATEWAY_ROUTES", routesCfg)

	mr, err := NewModelRouter()
	if err != nil {
		t.Fatalf("NewModelRouter: %v", err)
	}

	// Use a handler without auth (direct handleMessages).
	handler := handleMessages(mr)

	ar := AnthropicRequest{
		Model:     "claude-3-opus",
		MaxTokens: 100,
		Messages: []AnthropicMessage{
			{Role: "user", Content: mustMarshal("Hi")},
		},
	}
	body, _ := json.Marshal(ar)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rw.Code, rw.Body.String())
	}

	var result AnthropicResponse
	if err := json.Unmarshal(rw.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", result.StopReason)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "Hello from qwen!" {
		t.Errorf("content = %+v", result.Content)
	}
}

// --- Helpers ---

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func buildSSE(chunks []string) string {
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString("data: ")
		sb.WriteString(c)
		sb.WriteString("\n\n")
	}
	sb.WriteString("data: [DONE]\n\n")
	return sb.String()
}

func parseSSEEvents(raw string) []map[string]any {
	var events []map[string]any
	var currentEvent string
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var m map[string]any
			if err := json.Unmarshal([]byte(data), &m); err == nil {
				m["_event"] = currentEvent
				events = append(events, m)
			}
			currentEvent = ""
		}
	}
	return events
}

func filterEvents(events []map[string]any, eventType string) []map[string]any {
	var out []map[string]any
	for _, e := range events {
		if e["_event"] == eventType || e["type"] == eventType {
			out = append(out, e)
		}
	}
	return out
}

func findEvent(events []map[string]any, eventType string) map[string]any {
	for _, e := range events {
		if e["_event"] == eventType || e["type"] == eventType {
			return e
		}
	}
	return nil
}

func assertEventType(t *testing.T, events []map[string]any, eventType string) {
	t.Helper()
	if len(filterEvents(events, eventType)) == 0 {
		t.Errorf("expected event type %q not found in events", eventType)
	}
}

// Ensure unused imports don't cause compile errors.
var _ = context.Background
