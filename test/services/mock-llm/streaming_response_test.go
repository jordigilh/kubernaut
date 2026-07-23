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
package mockllm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kallm "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// UT-MOCK-1637: mock-llm previously ignored the OpenAI "stream" request
// field entirely and always wrote a single plain-JSON response body. Any
// caller using the streaming Chat Completions protocol (KA's
// pkg/shared/llm/openaicompat client, used whenever an event sink is
// attached for live SSE relay) would parse that plain-JSON body as an SSE
// stream, find no "data: " lines, and silently receive a zero-value
// response -- no content, no reasoning, no tool calls -- even though the
// correct scenario was detected server-side. This is also the root cause
// of the #1640 backport's E2E-KA-DISC-001 regression on release/v1.5: once
// discover_workflows' HTTP-session enrichment correctly attaches a
// LazySink for fallback sessions, chatOrStream switches to streaming, and
// this mock previously returned a zero-value ChatResponse (no tool calls)
// for every streamed request regardless of scenario.
//
// v1.5 does not carry main's reasoning_content simulation (#1578), so
// unlike upstream's version of this test, these specs assert on the
// scenario's root_cause_analysis text content rather than
// reasoning_content. See E2E-AF-1637-001 / #1637 investigation upstream.
var _ = Describe("Streaming Chat Completions (issue #1637)", func() {

	Describe("UT-MOCK-1637-001: stream=true text response carries the full content delta", func() {
		It("should emit an SSE chunk with the full delta plus a terminal [DONE] sentinel", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, config.ModeInteractive)
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := openai.ChatCompletionRequest{
				Model:  "mock-model",
				Stream: true,
				Messages: []openai.Message{
					{Role: "user", Content: strPtr("investigate unmatched_streaming_probe")},
				},
			}
			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

			raw, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			text := string(raw)

			Expect(text).To(ContainSubstring("data: "))
			Expect(text).To(HaveSuffix("data: [DONE]\n\n"))

			var sawContent bool
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" || data == "" {
					continue
				}
				var chunk map[string]any
				Expect(json.Unmarshal([]byte(data), &chunk)).To(Succeed())
				choices, _ := chunk["choices"].([]any)
				Expect(choices).NotTo(BeEmpty())
				choice, _ := choices[0].(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				Expect(delta).NotTo(BeNil())
				content, _ := delta["content"].(string)
				if content != "" {
					sawContent = true
					Expect(content).To(ContainSubstring("root_cause_analysis"))
				}
			}
			Expect(sawContent).To(BeTrue(), "streamed delta must carry the scenario's analysis content")
		})
	})

	Describe("UT-MOCK-1637-002: stream=false is unaffected (backward compatibility)", func() {
		It("should still return a plain-JSON non-streaming response", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, config.ModeInteractive)
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := openai.ChatCompletionRequest{
				Model: "mock-model",
				Messages: []openai.Message{
					{Role: "user", Content: strPtr("investigate unmatched_streaming_probe")},
				},
			}
			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

			var out openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
			Expect(out.Choices).To(HaveLen(1))
			Expect(*out.Choices[0].Message.Content).To(ContainSubstring("root_cause_analysis"))
		})
	})

	Describe("UT-MOCK-1637-003: stream=true tool-call response carries the full tool_calls delta", func() {
		It("should emit an SSE chunk whose delta.tool_calls matches the non-streaming response (#1640 E2E regression)", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, config.ModeAutonomous)
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := openai.ChatCompletionRequest{
				Model:  "mock-model",
				Stream: true,
				Messages: []openai.Message{
					{Role: "user", Content: strPtr("investigate OOMKilled")},
				},
				Tools: []openai.Tool{
					{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolSubmitResult}},
				},
			}
			body, err := json.Marshal(reqBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := http.Post(ts.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

			raw, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			text := string(raw)

			var sawToolCall bool
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" || data == "" {
					continue
				}
				var chunk map[string]any
				Expect(json.Unmarshal([]byte(data), &chunk)).To(Succeed())
				choices, _ := chunk["choices"].([]any)
				Expect(choices).NotTo(BeEmpty())
				choice, _ := choices[0].(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				Expect(delta).NotTo(BeNil())
				toolCalls, _ := delta["tool_calls"].([]any)
				if len(toolCalls) > 0 {
					sawToolCall = true
					tc, _ := toolCalls[0].(map[string]any)
					Expect(tc["type"]).To(Equal("function"), "delta tool_call must carry a non-empty type — langchaingo's updateToolCalls treats an empty type as an arguments-only continuation fragment, not a new tool call (#1640 root cause)")
					fn, _ := tc["function"].(map[string]any)
					Expect(fn["name"]).To(Equal("submit_result"))
					Expect(fn["arguments"]).NotTo(BeEmpty())
				}
			}
			Expect(sawToolCall).To(BeTrue(), "streamed delta must carry the scenario's tool call — regression guard for #1640 E2E-KA-DISC failures caused by empty streamed tool_calls")
		})
	})

	Describe("UT-MOCK-1637-004: real langchaingo client reconstructs streamed tool_calls end-to-end", func() {
		It("should populate ChatResponse.ToolCalls via KA's actual StreamChat path, not just raw SSE JSON", func() {
			// v1.5's KA still uses the third-party tmc/langchaingo client
			// (pkg/kubernautagent/llm/langchaingo). This drives the mock
			// through that exact client — the same one chatOrStream uses
			// in production — rather than only inspecting the raw wire
			// format, so it fails if langchaingo's own SSE tool-call
			// accumulator (openaiclient.updateToolCalls) ever regresses
			// against this mock's chunk shape again (#1640 E2E-KA-DISC
			// regression root cause).
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, config.ModeAutonomous)
			ts := httptest.NewServer(router)
			defer ts.Close()

			adapter, err := langchaingo.New("openai", ts.URL, "mock-model", "mock-api-key")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = adapter.Close() }()

			req := kallm.ChatRequest{
				Messages: []kallm.Message{
					{Role: "user", Content: "investigate OOMKilled"},
				},
				Tools: []kallm.ToolDefinition{
					{Name: openai.ToolSubmitResult, Description: "submit the investigation result", Parameters: json.RawMessage(`{"type":"object"}`)},
				},
			}

			resp, err := adapter.StreamChat(context.Background(), req, func(kallm.ChatStreamEvent) error { return nil })
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ToolCalls).NotTo(BeEmpty(), "StreamChat must reconstruct at least one tool call from the mock's SSE stream")
			Expect(resp.ToolCalls[0].Name).To(Equal(openai.ToolSubmitResult))
			Expect(resp.ToolCalls[0].Arguments).NotTo(BeEmpty())
		})
	})
})
