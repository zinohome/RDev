package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- OpenAI response types ---

type OAIResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []OAIChoice `json:"choices"`
	Usage   *OAIUsage   `json:"usage,omitempty"`
}

type OAIChoice struct {
	Index        int        `json:"index"`
	Message      OAIMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type OAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// --- OpenAI streaming chunk types ---

type OAIChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []OAIDelta    `json:"choices"`
	Usage   *OAIUsage     `json:"usage,omitempty"`
}

type OAIDelta struct {
	Index        int           `json:"index"`
	Delta        OAIDeltaBody  `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

type OAIDeltaBody struct {
	Role      string           `json:"role,omitempty"`
	Content   *string          `json:"content,omitempty"`
	ToolCalls []OAIToolCallDelta `json:"tool_calls,omitempty"`
}

type OAIToolCallDelta struct {
	Index    int              `json:"index"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function OAIFunctionDelta `json:"function,omitempty"`
}

type OAIFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// --- Anthropic response types ---

type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        AnthropicUsage          `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// translateNonStreamResponse converts an OpenAI non-streaming response to Anthropic format.
func translateNonStreamResponse(body []byte, claudeModel string) ([]byte, error) {
	var oai OAIResponse
	if err := json.Unmarshal(body, &oai); err != nil {
		return nil, fmt.Errorf("parse OpenAI response: %w", err)
	}

	ar := AnthropicResponse{
		ID:    "msg_" + uuid.New().String(),
		Type:  "message",
		Role:  "assistant",
		Model: claudeModel,
	}

	if len(oai.Choices) > 0 {
		ch := oai.Choices[0]
		ar.StopReason = finishReasonToStopReason(ch.FinishReason)

		// text content
		var textContent string
		if ch.Message.Content != nil {
			_ = json.Unmarshal(ch.Message.Content, &textContent)
		}
		if textContent != "" {
			ar.Content = append(ar.Content, AnthropicContentBlock{
				Type: "text",
				Text: textContent,
			})
		}

		// tool_calls → tool_use blocks
		for _, tc := range ch.Message.ToolCalls {
			var inputRaw json.RawMessage
			if tc.Function.Arguments != "" {
				inputRaw = json.RawMessage(tc.Function.Arguments)
			} else {
				inputRaw = json.RawMessage("{}")
			}
			ar.Content = append(ar.Content, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: inputRaw,
			})
		}
	}

	if oai.Usage != nil {
		ar.Usage = AnthropicUsage{
			InputTokens:  oai.Usage.PromptTokens,
			OutputTokens: oai.Usage.CompletionTokens,
		}
	}

	return json.Marshal(ar)
}

// streamResponse reads OpenAI SSE from upstream and writes Anthropic SSE to w.
//
// Anthropic vs OpenAI key differences:
// 1. Anthropic allows multiple tool_use blocks; OpenAI uses tool_calls array+index.
// 2. Anthropic streaming: input_json_delta sends JSON fragments.
//    OpenAI streaming: function_arguments accumulates as a string.
// 3. Anthropic stop_reason: end_turn | tool_use | max_tokens.
//    OpenAI finish_reason: stop | tool_calls | length.
// 4. Anthropic usage: {input_tokens, output_tokens}.
//    OpenAI usage: {prompt_tokens, completion_tokens}.
func streamResponse(w http.ResponseWriter, upstream io.Reader, claudeModel string) error {
	msgID := "msg_" + uuid.New().String()
	flusher, _ := w.(http.Flusher)

	// Write initial message_start event.
	writeSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         claudeModel,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 0, "output_tokens": 1},
		},
	})
	writeSSE(w, "ping", map[string]any{"type": "ping"})
	if flusher != nil {
		flusher.Flush()
	}

	// State machine for SSE translation.
	//
	// nextBlockIndex: running Anthropic content_block index counter.
	// textBlockOpen: whether we have an open text content_block.
	// toolBlocks: OpenAI tool_call index → Anthropic content_block index.
	// toolNames: OpenAI tool_call index → function name (arrives in first delta).
	nextBlockIndex := 0
	textBlockOpen := false
	toolBlocks := map[int]int{}    // oaiIndex → anthIndex
	toolNames := map[int]string{}  // oaiIndex → name

	var finishReason string
	var usage *OAIUsage

	scanner := bufio.NewScanner(upstream)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk OAIChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		if chunk.Usage != nil {
			usage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]

		if ch.FinishReason != nil && *ch.FinishReason != "" {
			finishReason = *ch.FinishReason
		}

		delta := ch.Delta

		// Text content delta.
		if delta.Content != nil && *delta.Content != "" {
			if !textBlockOpen {
				writeSSE(w, "content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": nextBlockIndex,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				})
				textBlockOpen = true
				nextBlockIndex++
			}
			writeSSE(w, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": nextBlockIndex - 1,
				"delta": map[string]any{
					"type": "text_delta",
					"text": *delta.Content,
				},
			})
		}

		// Tool call deltas.
		for _, tc := range delta.ToolCalls {
			oaiIdx := tc.Index
			anthIdx, exists := toolBlocks[oaiIdx]

			if !exists {
				// Close open text block first if any.
				if textBlockOpen {
					writeSSE(w, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": nextBlockIndex - 1,
					})
					textBlockOpen = false
				}

				anthIdx = nextBlockIndex
				toolBlocks[oaiIdx] = anthIdx
				toolNames[oaiIdx] = tc.Function.Name
				nextBlockIndex++

				writeSSE(w, "content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": anthIdx,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Function.Name,
						"input": map[string]any{},
					},
				})
			}

			// Update name if it arrives in a later delta (shouldn't happen but defensive).
			if tc.Function.Name != "" && toolNames[oaiIdx] == "" {
				toolNames[oaiIdx] = tc.Function.Name
			}

			if tc.Function.Arguments != "" {
				writeSSE(w, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": anthIdx,
					"delta": map[string]any{
						"type":         "input_json_delta",
						"partial_json": tc.Function.Arguments,
					},
				})
			}
		}

		if flusher != nil {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Close all open blocks.
	if textBlockOpen {
		writeSSE(w, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": nextBlockIndex - 1,
		})
	}
	for oaiIdx, anthIdx := range toolBlocks {
		_ = toolNames[oaiIdx]
		writeSSE(w, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": anthIdx,
		})
	}

	// message_delta with stop_reason and usage.
	stopReason := finishReasonToStopReason(finishReason)
	msgDelta := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{"output_tokens": 0},
	}
	if usage != nil {
		msgDelta["usage"] = map[string]int{"output_tokens": usage.CompletionTokens}
	}
	writeSSE(w, "message_delta", msgDelta)
	writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})

	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func finishReasonToStopReason(reason string) string {
	// Anthropic vs OpenAI finish_reason mapping:
	// stop → end_turn, tool_calls → tool_use, length → max_tokens
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

func writeSSE(w io.Writer, event string, data any) {
	b, _ := json.Marshal(data)
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "event: %s\ndata: %s\n\n", event, string(b))
	_, _ = w.Write(buf.Bytes())
}

// newAnthropicMsgID generates a unique Anthropic-style message ID.
func newAnthropicMsgID() string {
	return "msg_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
}

// currentUnix returns the current Unix timestamp.
func currentUnix() int64 {
	return time.Now().Unix()
}
