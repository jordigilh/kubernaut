/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package openaicompat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// chatCompletionResponse is the non-streaming Chat Completions wire shape.
type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   *chatCompletionUsage   `json:"usage,omitempty"`
}

type chatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      chatCompletionMessage `json:"message"`
	FinishReason *string               `json:"finish_reason"`
}

type chatCompletionMessage struct {
	Role             string         `json:"role"`
	Content          *string        `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function chatToolCallFn `json:"function"`
}

type chatToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// mapResponse translates the wire response into the shared Response shape.
func mapResponse(resp *chatCompletionResponse) *Response {
	out := &Response{Message: Message{Role: "assistant"}}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if choice.Message.Content != nil {
			out.Message.Content = *choice.Message.Content
		}
		out.Message.Reasoning = choice.Message.ReasoningContent
		for _, tc := range choice.Message.ToolCalls {
			out.Message.ToolCalls = append(out.Message.ToolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		if choice.FinishReason != nil {
			out.FinishReason = *choice.FinishReason
		}
	}

	if resp.Usage != nil {
		out.Usage = TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	return out
}

// Streaming wire types.
type chatCompletionChunk struct {
	Choices []chatCompletionChunkChoice `json:"choices"`
}

type chatCompletionChunkChoice struct {
	Delta        chatCompletionChunkDelta `json:"delta"`
	FinishReason *string                  `json:"finish_reason"`
}

type chatCompletionChunkDelta struct {
	Role             string              `json:"role,omitempty"`
	Content          string              `json:"content,omitempty"`
	ReasoningContent string              `json:"reasoning_content,omitempty"`
	ToolCalls        []chatChunkToolCall `json:"tool_calls,omitempty"`
}

type chatChunkToolCall struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Function chatChunkToolFn `json:"function"`
}

type chatChunkToolFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// toolCallAccumulator accumulates streaming tool-call fragments, which
// OpenAI-protocol servers deliver incrementally (id/name arrive once,
// arguments arrive character-by-character or in small chunks) across
// multiple SSE chunks, indexed by the tool call's position in the response.
type toolCallAccumulator struct {
	id   string
	name string
	args strings.Builder
}

// streamResponse reads the SSE stream, forwarding text deltas via yield and
// emitting a final accumulated Response (as StreamEvent.Final) once a
// choice reports a finish_reason.
func streamResponse(body io.Reader, yield func(StreamEvent) bool) error {
	accumulators := make(map[int]*toolCallAccumulator)
	var reasoning strings.Builder
	scanner := bufio.NewScanner(body)

	for scanner.Scan() {
		data, ok := sseDataLine(scanner.Text())
		if !ok {
			continue
		}
		if data == "[DONE]" {
			break
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		cont := processStreamChoice(chunk.Choices[0], accumulators, &reasoning, yield)
		if !cont {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("openaicompat: read SSE stream: %w", err)
	}
	return nil
}

// sseDataLine extracts the payload from an SSE "data: ..." line. ok is
// false for lines that are not data lines (comments, blank keep-alives).
func sseDataLine(line string) (data string, ok bool) {
	if !strings.HasPrefix(line, "data: ") {
		return "", false
	}
	return strings.TrimPrefix(line, "data: "), true
}

// processStreamChoice handles a single streamed choice: forwarding any
// content delta, accumulating tool-call/reasoning fragments, and (on
// FinishReason) emitting the final response. Returns false if yield
// requested the stream to stop.
func processStreamChoice(choice chatCompletionChunkChoice, accumulators map[int]*toolCallAccumulator, reasoning *strings.Builder, yield func(StreamEvent) bool) bool {
	if choice.Delta.ReasoningContent != "" {
		reasoning.WriteString(choice.Delta.ReasoningContent)
	}
	if choice.Delta.Content != "" {
		if !yield(StreamEvent{Delta: choice.Delta.Content}) {
			return false
		}
	}

	accumulateToolCallDeltas(choice.Delta.ToolCalls, accumulators)

	if choice.FinishReason != nil {
		final := buildFinishResponse(choice.FinishReason, accumulators, reasoning.String())
		return yield(StreamEvent{Done: true, Final: final})
	}
	return true
}

// accumulateToolCallDeltas merges streamed tool-call fragments into their
// per-index accumulator.
func accumulateToolCallDeltas(toolCalls []chatChunkToolCall, accumulators map[int]*toolCallAccumulator) {
	for _, tc := range toolCalls {
		acc, exists := accumulators[tc.Index]
		if !exists {
			acc = &toolCallAccumulator{}
			accumulators[tc.Index] = acc
		}
		if tc.ID != "" {
			acc.id = tc.ID
		}
		if tc.Function.Name != "" {
			acc.name = tc.Function.Name
		}
		acc.args.WriteString(tc.Function.Arguments)
	}
}

// buildFinishResponse builds the terminal Response once a choice reports a
// finish_reason, resolving all accumulated tool calls in ascending index
// order. Indices come from the provider's own per-tool-call "index" field
// and are not guaranteed to be contiguous from zero (a provider could, in
// principle, skip or reorder indices across chunks), so tool calls are
// collected by sorted key rather than assumed to span [0, len(accumulators)).
func buildFinishResponse(finishReason *string, accumulators map[int]*toolCallAccumulator, reasoning string) *Response {
	resp := &Response{
		Message: Message{Role: "assistant", Reasoning: reasoning},
	}
	if finishReason != nil {
		resp.FinishReason = *finishReason
	}

	indices := make([]int, 0, len(accumulators))
	for i := range accumulators {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	for _, i := range indices {
		acc := accumulators[i]
		resp.Message.ToolCalls = append(resp.Message.ToolCalls, ToolCall{
			ID:        acc.id,
			Name:      acc.name,
			Arguments: acc.args.String(),
		})
	}
	return resp
}
