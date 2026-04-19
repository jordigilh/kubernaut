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

package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
)

var _ = Describe("Vertex AI + Claude httptest round-trips — #684", func() {

	simpleRequest := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are a Kubernetes investigator."},
			{Role: "user", Content: "Why is the pod crashing?"},
		},
		Options: llm.ChatOptions{
			MaxTokens:   200,
			Temperature: 0.0,
		},
	}

	Describe("IT-KA-684-203: Azure adapter round-trip unaffected by #684 changes", func() {
		It("still works as before", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(openAIResponse("azure still works"))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("azure", server.URL, "gpt-4", "fake-key",
				langchaingo.WithAzureAPIVersion("2024-02-15-preview"))
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("azure still works"))
		})
	})

	Describe("IT-KA-684-204: Anthropic adapter round-trip unaffected by #684 changes", func() {
		It("still works as before", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(anthropicResponse("anthropic still works"))
			}))
			defer server.Close()

			adapter, err := langchaingo.New("anthropic", server.URL, "claude-sonnet-4-20250514", "fake-key")
			Expect(err).NotTo(HaveOccurred())

			resp, err := adapter.Chat(context.Background(), simpleRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("anthropic still works"))
		})
	})
})
