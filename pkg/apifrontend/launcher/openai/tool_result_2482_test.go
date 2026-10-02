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

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
)

var _ = Describe("API Frontend empty tool-result replay — #2482", func() {
	It("IT-AF-2482-001 [BR-AI-086, SI-10]: forwards an empty tool result through the production AF adapter", func() {
		var receivedBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
				http.Error(w, "invalid request JSON", http.StatusBadRequest)
				return
			}
			messages, ok := receivedBody["messages"].([]any)
			if !ok || len(messages) != 1 {
				http.Error(w, "unexpected message history", http.StatusBadRequest)
				return
			}
			toolMessage, ok := messages[0].(map[string]any)
			content, contentIsString := toolMessage["content"].(string)
			if !ok || !contentIsString || content != "" {
				http.Error(w, `{"error":{"message":"Invalid value for 'content': expected a string, got null."}}`, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"continue"},"finish_reason":"stop"}]}`))
		}))
		defer server.Close()

		client := openaimodel.NewModel("gpt-5.6-luna", server.URL, "test-key")
		var response *model.LLMResponse
		for result, err := range client.GenerateContent(context.Background(), &model.LLMRequest{
			Contents: []*genai.Content{{
				Role:  "tool",
				Parts: []*genai.Part{{Text: ""}},
			}},
		}, false) {
			Expect(err).NotTo(HaveOccurred())
			response = result
		}

		Expect(response).NotTo(BeNil())
		toolMessage := receivedBody["messages"].([]any)[0].(map[string]any)
		Expect(toolMessage["content"]).To(Equal(""))
	})
})
