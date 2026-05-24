package main

import (
	"encoding/json"

	"github.com/zinohome/RDev/rdev/gateway/provider"
)

// Anthropic vs OpenAI key differences:
// 1. Anthropic allows multiple tool_use blocks; OpenAI uses tool_calls array+index
// 2. Anthropic streaming: input_json_delta sends JSON fragments
//    OpenAI streaming: function_arguments accumulates a string directly
// 3. Anthropic stop_reason: end_turn | tool_use | max_tokens
//    OpenAI finish_reason: stop | tool_calls | length
// 4. Anthropic usage: {input_tokens, output_tokens}
//    OpenAI usage: {prompt_tokens, completion_tokens}

type AnthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens,omitempty"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []AnthropicContent
}

type AnthropicContent struct {
	Type      string           `json:"type"`
	Text      string           `json:"text,omitempty"`
	Source    *AnthropicSource `json:"source,omitempty"`
	ToolUseID string           `json:"tool_use_id,omitempty"`
	Content   interface{}      `json:"content,omitempty"`
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Input     interface{}      `json:"input,omitempty"`
}

type AnthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type AnthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema interface{} `json:"input_schema"`
}

func translateRequest(req AnthropicRequest, targetModel string) provider.ChatRequest {
	out := provider.ChatRequest{
		Model:               targetModel,
		MaxCompletionTokens: req.MaxTokens,
		Stream:              req.Stream,
	}

	if req.Stream {
		out.StreamOptions = &provider.StreamOpts{IncludeUsage: true}
	}

	if req.System != "" {
		out.Messages = append(out.Messages, provider.Message{
			Role:    "system",
			Content: req.System,
		})
	}

	for _, msg := range req.Messages {
		out.Messages = append(out.Messages, convertMessage(msg))
	}

	for _, t := range req.Tools {
		out.Tools = append(out.Tools, provider.Tool{
			Type: "function",
			Function: provider.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return out
}

func convertMessage(msg AnthropicMessage) provider.Message {
	switch v := msg.Content.(type) {
	case string:
		return provider.Message{Role: msg.Role, Content: v}
	case []interface{}:
		return convertContentArray(msg.Role, v)
	}
	return provider.Message{Role: msg.Role, Content: ""}
}

func convertContentArray(role string, contents []interface{}) provider.Message {
	var parts []interface{}
	var toolCalls []provider.ToolCall

	for _, raw := range contents {
		data, _ := json.Marshal(raw)
		var c AnthropicContent
		json.Unmarshal(data, &c)

		switch c.Type {
		case "text":
			parts = append(parts, map[string]interface{}{"type": "text", "text": c.Text})
		case "image":
			if c.Source != nil {
				var imgURL map[string]string
				if c.Source.Type == "base64" {
					imgURL = map[string]string{
						"url": "data:" + c.Source.MediaType + ";base64," + c.Source.Data,
					}
				} else {
					imgURL = map[string]string{"url": c.Source.URL}
				}
				parts = append(parts, map[string]interface{}{"type": "image_url", "image_url": imgURL})
			}
		case "tool_use":
			argsJSON, _ := json.Marshal(c.Input)
			toolCalls = append(toolCalls, provider.ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: provider.FunctionCall{
					Name:      c.Name,
					Arguments: string(argsJSON),
				},
			})
		case "tool_result":
			// tool_result becomes a standalone tool message; return immediately
			var resultContent string
			switch rv := c.Content.(type) {
			case string:
				resultContent = rv
			default:
				b, _ := json.Marshal(rv)
				resultContent = string(b)
			}
			return provider.Message{
				Role:       "tool",
				ToolCallID: c.ToolUseID,
				Content:    resultContent,
			}
		}
	}

	msg := provider.Message{Role: role}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	// single plain-text part → use string directly (simpler for non-vision models)
	if len(parts) == 1 {
		if p, ok := parts[0].(map[string]interface{}); ok && p["type"] == "text" {
			msg.Content = p["text"]
			return msg
		}
	}
	if len(parts) > 0 {
		msg.Content = parts
	}

	return msg
}
