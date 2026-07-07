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

package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
)

// Issue #1581 / BR-AI-086: compatibility-floor oracle, written BEFORE the
// #1581 decomposition of adapter.go into a thin wrapper over
// pkg/shared/llm/openaicompat starts (DD-LLM-005's "fail-closed by
// construction" tenet).
//
// Unlike adapter_test.go's other specs (which exercise real OpenAI's fully
// -compliant behavior), this fixture simulates the bare-bones floor: a
// minimal OpenAI-compatible server that echoes no reasoning field, has no
// strict/structured-output support, and only understands the basic
// non-streaming and streaming chat-completions shapes (self-hosted vLLM /
// Ollama / LlamaStack / TGI deployments commonly look like this).
//
// This must pass unchanged, byte-for-byte, both BEFORE and AFTER the #1581
// extraction — it is the regression gate proving the shared
// pkg/shared/llm/openaicompat core does not silently start assuming
// capabilities (reasoning echo, strict mode) that a minimal server doesn't
// have, which would break every self-hosted/local deployment (BR-AI-086
// req 3, "unsupported/unknown models skip the parameter, never error").
var _ = Describe("Compatibility floor — minimal OpenAI-compatible server (#1581)", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("UT-AF-1581-101: a minimal non-streaming server round-trips a plain text reply with no extra request fields", func() {
		var receivedBody map[string]any
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			// Bare-bones response: no "reasoning_content", no "logprobs", no
			// vendor extensions — just the fields the OpenAI protocol requires.
			_, _ = w.Write([]byte(`{
				"id": "cmpl-min-1", "object": "chat.completion", "model": "local-model",
				"choices": [{"index": 0, "message": {"role": "assistant", "content": "ack"}, "finish_reason": "stop"}]
			}`))
		}))

		m := openaimodel.NewModel("local-model", server.URL, "")
		var resp *model.LLMResponse
		for r, err := range m.GenerateContent(context.Background(), &model.LLMRequest{
			Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "ping"}}}},
		}, false) {
			Expect(err).NotTo(HaveOccurred())
			resp = r
		}
		Expect(resp).NotTo(BeNil())
		Expect(resp.Content.Parts[0].Text).To(Equal("ack"))

		// Compatibility floor: no reasoning/thinking/strict-mode field is
		// ever sent unless explicitly requested — none of those concepts
		// exist for a minimal server, and a request carrying an unrecognized
		// field is precisely the failure mode this fixture guards against.
		Expect(receivedBody).NotTo(HaveKey("reasoning_content"))
		Expect(receivedBody).NotTo(HaveKey("response_format"))
		Expect(receivedBody).NotTo(HaveKey("thinking"))
	})

	It("UT-AF-1581-102: a minimal streaming server round-trips text deltas and one basic tool call", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			chunks := []string{
				`{"choices":[{"index":0,"delta":{"role":"assistant","content":"go"}}]}`,
				`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_min","function":{"name":"noop","arguments":"{}"}}]}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			}
			for _, c := range chunks {
				_, _ = w.Write([]byte("data: " + c + "\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))

		m := openaimodel.NewModel("local-model", server.URL, "")
		var texts []string
		var sawToolCall bool
		for r, err := range m.GenerateContent(context.Background(), &model.LLMRequest{
			Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "run"}}}},
		}, true) {
			Expect(err).NotTo(HaveOccurred())
			for _, p := range r.Content.Parts {
				if p.Text != "" {
					texts = append(texts, p.Text)
				}
				if p.FunctionCall != nil {
					sawToolCall = true
				}
			}
		}
		Expect(texts).To(Equal([]string{"go"}))
		Expect(sawToolCall).To(BeTrue())
	})
})
