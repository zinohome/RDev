package provider

import "context"

// LLMProvider abstracts downstream LLM backends (vLLM, Ollama).
// Both expose an OpenAI-compatible API, so the interface is thin.
type LLMProvider interface {
	ChatCompletions(ctx context.Context, req ChatRequest) (ChatResponse, error)
	ChatCompletionsStream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
}

// ChatRequest is the shared OpenAI-compatible request shape used by all drivers.
type ChatRequest struct {
	Model               string    `json:"model"`
	Messages            []Message `json:"messages"`
	MaxCompletionTokens int       `json:"max_completion_tokens,omitempty"`
	Tools               []Tool    `json:"tools,omitempty"`
	Stream              bool      `json:"stream,omitempty"`
	StreamOptions       *StreamOpts `json:"stream_options,omitempty"`
}

type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type ToolCall struct {
	Index    int          `json:"index,omitempty"` // streaming only: which tool call this delta belongs to
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type StreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

// ChatResponse is the OpenAI non-streaming response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
	Index        int     `json:"index"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// StreamChunk is an OpenAI streaming delta chunk.
type StreamChunk struct {
	ID      string          `json:"id"`
	Choices []StreamChoice  `json:"choices"`
	Usage   *Usage          `json:"usage,omitempty"`
}

type StreamChoice struct {
	Delta        *Delta `json:"delta,omitempty"`
	FinishReason string `json:"finish_reason"`
	Index        int    `json:"index"`
}

type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
