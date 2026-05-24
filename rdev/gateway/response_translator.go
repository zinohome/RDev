package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/zinohome/RDev/rdev/gateway/provider"
)

// AnthropicResponse is the Anthropic Messages API response shape.
type AnthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Model        string             `json:"model"`
	Content      []AnthropicContent `json:"content"`
	StopReason   string             `json:"stop_reason"`
	StopSequence *string            `json:"stop_sequence"`
	Usage        AnthropicUsage     `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func translateResponse(resp provider.ChatResponse, model string) AnthropicResponse {
	out := AnthropicResponse{
		ID:    "msg_" + resp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: model,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		msg := choice.Message

		if msg.Content != nil {
			var text string
			switch v := msg.Content.(type) {
			case string:
				text = v
			default:
				b, _ := json.Marshal(v)
				text = string(b)
			}
			if text != "" {
				out.Content = append(out.Content, AnthropicContent{Type: "text", Text: text})
			}
		}

		for _, tc := range msg.ToolCalls {
			var input interface{}
			json.Unmarshal([]byte(tc.Function.Arguments), &input)
			out.Content = append(out.Content, AnthropicContent{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}

		out.StopReason = mapFinishReason(choice.FinishReason)
	}

	if resp.Usage != nil {
		out.Usage = AnthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		}
	}

	return out
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return "end_turn"
	}
}

func translateStream(w http.ResponseWriter, flusher http.Flusher, events <-chan provider.StreamChunk, model string) {
	msgID := "msg_" + uuid.New().String()

	writeSSE(w, flusher, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	})
	writeSSE(w, flusher, "ping", map[string]string{"type": "ping"})

	type blockState struct {
		blockIndex int
	}
	openBlocks := map[int]blockState{}
	nextBlockIndex := 0
	textBlockIndex := -1

	var lastUsage *provider.Usage
	var finalFinishReason string

	for chunk := range events {
		if chunk.Usage != nil {
			lastUsage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finalFinishReason = choice.FinishReason
			}
			if choice.Delta == nil {
				continue
			}
			delta := choice.Delta

			if delta.Content != "" {
				if textBlockIndex < 0 {
					textBlockIndex = nextBlockIndex
					nextBlockIndex++
					writeSSE(w, flusher, "content_block_start", map[string]interface{}{
						"type":  "content_block_start",
						"index": textBlockIndex,
						"content_block": map[string]interface{}{
							"type": "text",
							"text": "",
						},
					})
				}
				writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": textBlockIndex,
					"delta": map[string]string{
						"type": "text_delta",
						"text": delta.Content,
					},
				})
			}

			for _, tc := range delta.ToolCalls {
				state, exists := openBlocks[tc.Index]
				if !exists {
					blockIdx := nextBlockIndex
					nextBlockIndex++
					writeSSE(w, flusher, "content_block_start", map[string]interface{}{
						"type":  "content_block_start",
						"index": blockIdx,
						"content_block": map[string]interface{}{
							"type":  "tool_use",
							"id":    tc.ID,
							"name":  tc.Function.Name,
							"input": map[string]interface{}{},
						},
					})
					state = blockState{blockIndex: blockIdx}
					openBlocks[tc.Index] = state
				}

				if tc.Function.Arguments != "" {
					writeSSE(w, flusher, "content_block_delta", map[string]interface{}{
						"type":  "content_block_delta",
						"index": state.blockIndex,
						"delta": map[string]string{
							"type":         "input_json_delta",
							"partial_json": tc.Function.Arguments,
						},
					})
				}
			}
		}
	}

	if textBlockIndex >= 0 {
		writeSSE(w, flusher, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": textBlockIndex,
		})
	}
	for _, state := range openBlocks {
		writeSSE(w, flusher, "content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": state.blockIndex,
		})
	}

	outTokens := 0
	if lastUsage != nil {
		outTokens = lastUsage.CompletionTokens
	}
	writeSSE(w, flusher, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   mapFinishReason(finalFinishReason),
			"stop_sequence": nil,
		},
		"usage": map[string]int{"output_tokens": outTokens},
	})
	writeSSE(w, flusher, "message_stop", map[string]string{"type": "message_stop"})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(b))
	flusher.Flush()
}
