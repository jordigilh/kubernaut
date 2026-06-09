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

package vertexanthropic_test

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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
)

var _ = Describe("Fix #1384 Bug B — vertexanthropic empty text block guard (SI-10)", func() {

	var (
		server    *httptest.Server
		tokenSrv  *httptest.Server
		client    *vertexanthropic.Client
		fakeCreds []byte
	)

	makeTestClient := func(handler http.HandlerFunc) {
		tokenSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fake-token","token_type":"Bearer","expires_in":3600}`))
		}))
		fakeCreds = generateFakeServiceAccountJSONWithTokenURL(tokenSrv.URL)

		server = httptest.NewServer(handler)
		var err error
		client, err = vertexanthropic.New(context.Background(),
			"claude-sonnet-4-6", fakeCreds, "my-project", "us-central1",
			vertexanthropic.WithSDKOptions(
				option.WithBaseURL(server.URL),
			),
		)
		Expect(err).NotTo(HaveOccurred())
	}

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if tokenSrv != nil {
			tokenSrv.Close()
		}
	})

	Describe("UT-KA-1384-B03: LLM client never sends empty text blocks to Vertex AI (SI-10 defense-in-depth)", func() {
		It("should skip or replace empty Content for assistant messages with no ToolCalls", func() {
			var receivedBody map[string]interface{}
			makeTestClient(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_b03",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "Final answer."}],
					"usage": {"input_tokens": 10, "output_tokens": 5}
				}`))
			})

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Start investigation"},
					{Role: "assistant", Content: ""},
					{Role: "user", Content: "Continue"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("Final answer."))

			messages, ok := receivedBody["messages"].([]interface{})
			Expect(ok).To(BeTrue())

			for i, rawMsg := range messages {
				msg := rawMsg.(map[string]interface{})
				if msg["role"] != "assistant" {
					continue
				}
				content, hasContent := msg["content"].([]interface{})
				if !hasContent {
					continue
				}
				for j, rawBlock := range content {
					block := rawBlock.(map[string]interface{})
					if block["type"] == "text" {
						text, _ := block["text"].(string)
						Expect(text).NotTo(BeEmpty(),
							"assistant message[%d].content[%d] has empty text block — violates SI-10", i, j)
					}
				}
			}
		})
	})

	Describe("UT-KA-1384-B04: LLM client correctly sends non-empty assistant text (no over-filtering)", func() {
		It("should include text block when Content is non-empty", func() {
			var receivedBody map[string]interface{}
			makeTestClient(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_b04",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "OK"}],
					"usage": {"input_tokens": 10, "output_tokens": 2}
				}`))
			})

			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Start"},
					{Role: "assistant", Content: "I found the issue."},
					{Role: "user", Content: "Tell me more"},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			messages := receivedBody["messages"].([]interface{})
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg["role"]).To(Equal("assistant"))

			content := assistantMsg["content"].([]interface{})
			Expect(content).To(HaveLen(1))
			textBlock := content[0].(map[string]interface{})
			Expect(textBlock["type"]).To(Equal("text"))
			Expect(textBlock["text"]).To(Equal("I found the issue."))
		})
	})
})
