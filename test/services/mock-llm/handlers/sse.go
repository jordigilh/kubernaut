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
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
)

// sseChunk, sseChunkChoice, sseChunkDelta, sseChunkToolCall, and
// sseChunkToolFn mirror the private streaming wire types consumed by
// pkg/shared/llm/openaicompat's SSE parser (chatCompletionChunk et al. in
// response.go). Kept as a local mirror rather than an import since those
// types are unexported by design (openaicompat is the real client's wire
// contract, not a shared schema package).
type sseChunk struct {
	Choices []sseChunkChoice `json:"choices"`
}

type sseChunkChoice struct {
	Delta        sseChunkDelta `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

type sseChunkDelta struct {
	Role             string             `json:"role,omitempty"`
	Content          string             `json:"content,omitempty"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	ToolCalls        []sseChunkToolCall `json:"tool_calls,omitempty"`
}

type sseChunkToolCall struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Function sseChunkToolFn `json:"function"`
}

type sseChunkToolFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// writeChatCompletion writes resp as a plain-JSON body (the historical,
// still-default behavior) or as a Server-Sent Events stream when stream is
// true (#1637). Every mock-llm response builder (BuildTextResponse,
// BuildToolCallResponse, BuildMultiToolCallResponse, ...) constructs the
// same openai.ChatCompletionResponse regardless of transport; this is the
// single seam that picks how to put it on the wire.
func writeChatCompletion(w http.ResponseWriter, stream bool, resp openai.ChatCompletionResponse) {
	if !stream {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	writeSSEChatCompletion(w, resp)
}

// writeSSEChatCompletion streams resp as a single OpenAI-compatible Chat
// Completions chunk carrying the full delta (role, content,
// reasoning_content, tool_calls) plus finish_reason, followed by the
// terminal "[DONE]" sentinel. This is a compatibility floor, not
// token-by-token fragmentation: real providers emit many small deltas, but
// every field a spec-compliant SSE consumer reads is present here, which is
// sufficient for the mock's purpose (deterministic, fully-populated
// responses for E2E assertions) without reimplementing incremental
// tokenization.
func writeSSEChatCompletion(w http.ResponseWriter, resp openai.ChatCompletionResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	if len(resp.Choices) > 0 {
		writeSSEData(w, sseChunkFromChoice(resp.Choices[0]))
		if canFlush {
			flusher.Flush()
		}
	}

	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

// sseChunkFromChoice builds the single streamed chunk for a non-streaming
// Choice, preserving every field the compat client's parser inspects.
func sseChunkFromChoice(choice openai.Choice) sseChunk {
	// v1.5 does not have reasoning-content simulation (#1578, main-only);
	// the wire struct still carries the field for openaicompat parser
	// compatibility, it is just never populated here.
	delta := sseChunkDelta{
		Role: "assistant",
	}
	if choice.Message.Content != nil {
		delta.Content = *choice.Message.Content
	}
	for i, tc := range choice.Message.ToolCalls {
		delta.ToolCalls = append(delta.ToolCalls, sseChunkToolCall{
			Index: i,
			ID:    tc.ID,
			Function: sseChunkToolFn{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	finishReason := choice.FinishReason
	return sseChunk{Choices: []sseChunkChoice{{Delta: delta, FinishReason: &finishReason}}}
}

func writeSSEData(w http.ResponseWriter, chunk sseChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
