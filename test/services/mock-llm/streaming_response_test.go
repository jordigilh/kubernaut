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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
// correct scenario was detected server-side. This was masked because prior
// tests only asserted reasoning-event *type* presence, never populated
// content. See E2E-AF-1637-001 / #1637 investigation.
var _ = Describe("Streaming Chat Completions (issue #1637)", func() {

	Describe("UT-MOCK-1637-001: stream=true text response carries content and reasoning_content", func() {
		It("should emit an SSE chunk with the full delta plus a terminal [DONE] sentinel", func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewRouter(registry, false, config.ModeInteractive)
			ts := httptest.NewServer(router)
			defer ts.Close()

			reqBody := openai.ChatCompletionRequest{
				Model:  "mock-model",
				Stream: true,
				Messages: []openai.Message{
					{Role: "user", Content: strPtr("investigate mock_reasoning_capture")},
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

			var sawReasoning bool
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
				reasoningContent, _ := delta["reasoning_content"].(string)
				if reasoningContent != "" {
					sawReasoning = true
					Expect(reasoningContent).To(ContainSubstring("memory"))
				}
				content, _ := delta["content"].(string)
				Expect(content).To(ContainSubstring("root_cause_analysis"))
			}
			Expect(sawReasoning).To(BeTrue(), "streamed delta must carry the scenario's reasoning_content")
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
					{Role: "user", Content: strPtr("investigate mock_reasoning_capture")},
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
			Expect(out.Choices[0].Message.ReasoningContent).To(ContainSubstring("memory"))
		})
	})
})
