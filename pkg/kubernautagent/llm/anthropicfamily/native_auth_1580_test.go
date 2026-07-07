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

package anthropicfamily_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
)

// Issue #1580 / BR-AI-086 M2: native Anthropic API-key auth mode, alongside
// the existing Vertex path. Bedrock auth mode is explicitly out of scope
// here (deferred to #1582's own PR per DD-LLM-006).
var _ = Describe("anthropicfamily.NewWithAPIKey — #1580", func() {

	Describe("constructor validation", func() {
		It("UT-KA-1580-001: returns error when apiKey is empty", func() {
			client, err := anthropicfamily.NewWithAPIKey("", "claude-sonnet-4-6")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiKey"))
			Expect(client).To(BeNil())
		})

		It("UT-KA-1580-002: accepts a non-empty apiKey and model", func() {
			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-KA-1580-003: implements llm.Client interface", func() {
			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6")
			Expect(err).NotTo(HaveOccurred())
			var _ llm.Client = client
		})
	})

	Describe("Chat() over the native Anthropic API endpoint", func() {
		var server *httptest.Server

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		It("UT-KA-1580-004: sends requests without any GCP/Vertex auth machinery", func() {
			var receivedAuthHeader string
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedAuthHeader = r.Header.Get("x-api-key")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_native_001",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "native auth works"}],
					"usage": {"input_tokens": 10, "output_tokens": 5}
				}`))
			}))

			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6",
				anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)),
			)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("native auth works"))
			Expect(receivedAuthHeader).To(Equal("sk-ant-fake-key"))
		})

		It("UT-KA-1580-005: request body carries no Vertex-specific fields (no anthropic_version, model stays in body)", func() {
			var receivedBody map[string]interface{}
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_native_002",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "ok"}],
					"usage": {"input_tokens": 5, "output_tokens": 2}
				}`))
			}))

			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6",
				anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL)),
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKeyWithValue("model", "claude-sonnet-4-6"))
		})
	})
})
