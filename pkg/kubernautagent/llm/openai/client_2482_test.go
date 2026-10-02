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

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
)

var _ = Describe("Kubernaut Agent empty tool-result replay — #2482", func() {
	It("IT-KA-2482-001 [BR-KA-263, CC7.2]: forwards an empty tool result through the production KA wrapper", func() {
		var receivedBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				http.Error(w, "invalid request JSON", http.StatusBadRequest)
				return
			}
			messages, ok := receivedBody["messages"].([]any)
			if !ok || len(messages) != 3 {
				http.Error(w, "unexpected message history", http.StatusBadRequest)
				return
			}
			toolMessage, ok := messages[2].(map[string]any)
			content, contentIsString := toolMessage["content"].(string)
			if !ok || !contentIsString || content != "" {
				http.Error(w, `{"error":{"message":"Invalid value for 'content': expected a string, got null."}}`, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"continue"},"finish_reason":"stop"}]}`))
		}))
		defer server.Close()

		client := kaopenai.New("gpt-5.6-luna", server.URL, "test-key")
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{
				{Role: "user", Content: "investigate"},
				{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_pods_top", Name: "pods_top", Arguments: "{}"}}},
				{Role: "tool", Content: "", ToolCallID: "call_pods_top", ToolName: "pods_top"},
			},
		})

		Expect(err).NotTo(HaveOccurred())
		toolMessage := receivedBody["messages"].([]any)[2].(map[string]any)
		Expect(toolMessage["content"]).To(Equal(""))
	})
})
