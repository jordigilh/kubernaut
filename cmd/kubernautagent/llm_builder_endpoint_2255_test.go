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

package main

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #2255 / BR-AI-089: native-auth-mode LLM client construction must
// honor an operator-configured cfg.Endpoint (FedRAMP AC-4, information flow
// enforcement). These IT tests exercise the real production dispatch path
// (buildLLMClientFromConfig -> buildAnthropicNativeClient/
// buildGeminiNativeClient), proving the wiring rather than just the
// package-local constructors (CHECKPOINT W).
var _ = Describe("buildLLMClientFromConfig — native endpoint override wiring (#2255, BR-AI-089)", func() {

	Describe("provider=anthropic honors cfg.Endpoint (IT-KA-2255-101)", func() {
		var server *httptest.Server

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		It("IT-KA-2255-101 [AC-4]: sends the outgoing request to the operator-configured endpoint", func() {
			var received bool
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received = true
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_it_2255_101",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "routed via gateway"}],
					"usage": {"input_tokens": 5, "output_tokens": 3}
				}`))
			}))

			cfg := types.LLMConfig{
				Provider: types.LLMProviderAnthropic,
				Model:    "claude-sonnet-4-6",
				APIKey:   "sk-ant-fake-test-key",
				Endpoint: server.URL,
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(received).To(BeTrue(),
				"operator's configured ai.llm.endpoint (e.g. an AI gateway or NetworkPolicy-allowlisted proxy, see issue #1819) must actually receive the request")
			Expect(resp.Message.Content).To(Equal("routed via gateway"))
		})
	})

	Describe("provider=gemini honors cfg.Endpoint (IT-KA-2255-102)", func() {
		var server *httptest.Server

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		It("IT-KA-2255-102 [AC-4]: sends the outgoing request to the operator-configured endpoint", func() {
			var received bool
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received = true
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"candidates": [{
						"content": {"role": "model", "parts": [{"text": "routed via gateway"}]},
						"finishReason": "STOP"
					}],
					"usageMetadata": {"promptTokenCount": 5, "candidatesTokenCount": 3, "totalTokenCount": 8}
				}`))
			}))

			cfg := types.LLMConfig{
				Provider: types.LLMProviderGemini,
				Model:    "gemini-2.5-pro",
				APIKey:   "fake-gemini-key",
				Endpoint: server.URL,
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(received).To(BeTrue(),
				"operator's configured ai.llm.endpoint (e.g. an AI gateway or NetworkPolicy-allowlisted proxy, see issue #1819) must actually receive the request")
			Expect(resp.Message.Content).To(Equal("routed via gateway"))
		})
	})

	Describe("zero-regression when cfg.Endpoint is empty (IT-KA-2255-103)", func() {
		// No behavior change for existing deployments that don't set
		// ai.llm.endpoint for these two providers: client construction must
		// keep succeeding exactly as before this fix.
		It("IT-KA-2255-103a: provider=anthropic still constructs successfully with no endpoint", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderAnthropic,
				Model:    "claude-sonnet-4-6",
				APIKey:   "sk-ant-fake-test-key",
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("IT-KA-2255-103b: provider=gemini still constructs successfully with no endpoint", func() {
			cfg := types.LLMConfig{
				Provider: types.LLMProviderGemini,
				Model:    "gemini-2.5-pro",
				APIKey:   "fake-gemini-key",
			}

			client, err := buildLLMClientFromConfig(context.Background(), cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})
})
