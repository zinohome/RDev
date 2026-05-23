package main

import (
	"encoding/json"
	"fmt"
)

// --- Anthropic request types ---

type AnthropicRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	System        string             `json:"system,omitempty"`
	Messages      []AnthropicMessage `json:"messages"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
	Stream        bool               `json:"stream"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
}

type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []AnthropicContentBlock
}

type AnthropicContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// image
	Source *AnthropicImageSource `json:"source,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or []block
	IsError   bool            `json:"is_error,omitempty"`
}

type AnthropicImageSource struct {
	Type      string `json:"type"`       // base64 | url
	MediaType string `json:"media_type"` // image/jpeg etc.
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// --- OpenAI request types ---

type OAIRequest struct {
	Model               string         `json:"model"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	Messages            []OAIMessage   `json:"messages"`
	Tools               []OAITool      `json:"tools,omitempty"`
	Stream              bool           `json:"stream"`
	StreamOptions       *StreamOptions `json:"stream_options,omitempty"`
	Temperature         *float64       `json:"temperature,omitempty"`
	TopP                *float64       `json:"top_p,omitempty"`
	Stop                []string       `json:"stop,omitempty"`
}

type OAIMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"` // string or []OAIContentPart
	ToolCalls  []OAIToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type OAIContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *OAIImageURL  `json:"image_url,omitempty"`
}

type OAIImageURL struct {
	URL string `json:"url"`
}

type OAIToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"` // "function"
	Function OAIFunction `json:"function"`
}

type OAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OAITool struct {
	Type     string       `json:"type"` // "function"
	Function OAIFuncDecl  `json:"function"`
}

type OAIFuncDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// translateRequest converts an Anthropic Messages request to OpenAI Chat Completions format.
func translateRequest(ar *AnthropicRequest, targetModel string) (*OAIRequest, error) {
	oai := &OAIRequest{
		Model:               targetModel,
		MaxCompletionTokens: ar.MaxTokens,
		Stream:              ar.Stream,
		Temperature:         ar.Temperature,
		TopP:                ar.TopP,
		Stop:                ar.StopSequences,
	}

	if ar.Stream {
		oai.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

	// system prompt → first system message
	if ar.System != "" {
		content, _ := json.Marshal(ar.System)
		oai.Messages = append(oai.Messages, OAIMessage{
			Role:    "system",
			Content: content,
		})
	}

	for _, am := range ar.Messages {
		msgs, err := translateMessage(am)
		if err != nil {
			return nil, err
		}
		oai.Messages = append(oai.Messages, msgs...)
	}

	// tools
	for _, t := range ar.Tools {
		oai.Tools = append(oai.Tools, OAITool{
			Type: "function",
			Function: OAIFuncDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return oai, nil
}

// translateMessage converts one Anthropic message into one or more OAI messages.
// A user message with tool_result blocks splits into separate tool-role messages.
func translateMessage(am AnthropicMessage) ([]OAIMessage, error) {
	// content can be a plain string
	var text string
	if err := json.Unmarshal(am.Content, &text); err == nil {
		content, _ := json.Marshal(text)
		return []OAIMessage{{Role: am.Role, Content: content}}, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(am.Content, &blocks); err != nil {
		return nil, fmt.Errorf("unrecognised content format: %w", err)
	}

	switch am.Role {
	case "user":
		return translateUserBlocks(blocks)
	case "assistant":
		return translateAssistantBlocks(blocks)
	default:
		content, _ := json.Marshal(am.Content)
		return []OAIMessage{{Role: am.Role, Content: content}}, nil
	}
}

func translateUserBlocks(blocks []AnthropicContentBlock) ([]OAIMessage, error) {
	var msgs []OAIMessage
	var parts []OAIContentPart

	flush := func() {
		if len(parts) == 0 {
			return
		}
		content, _ := json.Marshal(parts)
		msgs = append(msgs, OAIMessage{Role: "user", Content: content})
		parts = nil
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, OAIContentPart{Type: "text", Text: b.Text})
		case "image":
			imgURL, err := convertImage(b.Source)
			if err != nil {
				return nil, err
			}
			parts = append(parts, OAIContentPart{Type: "image_url", ImageURL: &OAIImageURL{URL: imgURL}})
		case "tool_result":
			// flush buffered text/image parts as a user message first
			flush()
			toolMsg, err := convertToolResult(b)
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, toolMsg)
		}
	}
	flush()
	return msgs, nil
}

func translateAssistantBlocks(blocks []AnthropicContentBlock) ([]OAIMessage, error) {
	msg := OAIMessage{Role: "assistant"}
	var textParts []string
	var toolCalls []OAIToolCall

	for _, b := range blocks {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "tool_use":
			args, err := marshalToolInput(b.Input)
			if err != nil {
				return nil, err
			}
			toolCalls = append(toolCalls, OAIToolCall{
				ID:   b.ID,
				Type: "function",
				Function: OAIFunction{
					Name:      b.Name,
					Arguments: args,
				},
			})
		}
	}

	if len(textParts) > 0 {
		combined := ""
		for _, t := range textParts {
			combined += t
		}
		content, _ := json.Marshal(combined)
		msg.Content = content
	}
	msg.ToolCalls = toolCalls
	return []OAIMessage{msg}, nil
}

func convertImage(src *AnthropicImageSource) (string, error) {
	if src == nil {
		return "", fmt.Errorf("image block missing source")
	}
	switch src.Type {
	case "base64":
		return "data:" + src.MediaType + ";base64," + src.Data, nil
	case "url":
		return src.URL, nil
	default:
		return "", fmt.Errorf("unsupported image source type: %s", src.Type)
	}
}

func convertToolResult(b AnthropicContentBlock) (OAIMessage, error) {
	msg := OAIMessage{
		Role:       "tool",
		ToolCallID: b.ToolUseID,
	}

	// content can be a string or []block
	var text string
	if err := json.Unmarshal(b.Content, &text); err == nil {
		content, _ := json.Marshal(text)
		msg.Content = content
		return msg, nil
	}

	var subBlocks []AnthropicContentBlock
	if err := json.Unmarshal(b.Content, &subBlocks); err == nil {
		combined := ""
		for _, sb := range subBlocks {
			if sb.Type == "text" {
				combined += sb.Text
			}
		}
		content, _ := json.Marshal(combined)
		msg.Content = content
		return msg, nil
	}

	msg.Content = b.Content
	return msg, nil
}

func marshalToolInput(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "{}", nil
	}
	return string(raw), nil
}
