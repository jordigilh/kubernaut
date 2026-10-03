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

package openaicompat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

func issue2482Messages() []openaicompat.Message {
	toolCalls := make([]openaicompat.ToolCall, 6)
	for i := range toolCalls {
		toolCalls[i] = openaicompat.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      fmt.Sprintf("tool_%d", i),
			Arguments: "{}",
		}
	}

	messages := []openaicompat.Message{
		{Role: "system", Content: "investigate"},
		{Role: "user", Content: "find the cause"},
		{Role: "assistant", ToolCalls: toolCalls},
	}
	for i, toolCall := range toolCalls {
		content := fmt.Sprintf("result_%d", i)
		if i == 4 {
			content = ""
		}
		messages = append(messages, openaicompat.Message{
			Role:       "tool",
			Content:    content,
			ToolCallID: toolCall.ID,
		})
	}
	return messages
}

func hasIssue2482EmptyToolContent(body map[string]any) bool {
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) <= 7 {
		return false
	}
	toolMessage, ok := messages[7].(map[string]any)
	if !ok {
		return false
	}
	content, ok := toolMessage["content"].(string)
	return ok && content == ""
}

var _ = Describe("OpenAI-compatible empty tool-result content — #2482", func() {
	It("UT-KA-2482-001 [BR-KA-263, SI-10, ASVS V5.1]: sends an empty tool result as a JSON string", func() {
		var receivedBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				http.Error(w, "invalid request JSON", http.StatusBadRequest)
				return
			}
			if !hasIssue2482EmptyToolContent(receivedBody) {
				http.Error(w, `{"error":{"message":"Invalid value for 'content': expected a string, got null."}}`, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"continue"},"finish_reason":"stop"}]}`))
		}))
		defer server.Close()

		client := openaicompat.New("gpt-5.6-luna", server.URL, "test-key")
		_, err := client.Chat(context.Background(), openaicompat.Request{
			Messages: issue2482Messages(),
		})

		Expect(err).NotTo(HaveOccurred())
		toolMessage := receivedBody["messages"].([]any)[7].(map[string]any)
		Expect(toolMessage).To(HaveKey("content"))
		Expect(toolMessage["content"]).To(BeAssignableToTypeOf(""))
		Expect(toolMessage["content"]).To(Equal(""))
	})

	It("UT-KA-2482-002 [BR-KA-263, SI-10, ASVS V5.1]: preserves the same contract for streaming requests", func() {
		var receivedBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				http.Error(w, "invalid request JSON", http.StatusBadRequest)
				return
			}
			if !hasIssue2482EmptyToolContent(receivedBody) {
				http.Error(w, `{"error":{"message":"Invalid value for 'content': expected a string, got null."}}`, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"continue\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer server.Close()

		client := openaicompat.New("gpt-5.6-luna", server.URL, "test-key")
		err := client.StreamChat(context.Background(), openaicompat.Request{
			Messages: issue2482Messages(),
		}, func(openaicompat.StreamEvent) bool { return true })

		Expect(err).NotTo(HaveOccurred())
		toolMessage := receivedBody["messages"].([]any)[7].(map[string]any)
		Expect(toolMessage["content"]).To(Equal(""))
	})
})
