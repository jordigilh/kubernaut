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

var _ = Describe("vertexanthropic.Client — #684 #686", func() {

	Describe("New() constructor validation", func() {

		It("UT-VA-686-001: returns error when project is empty", func() {
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", nil, "", "us-central1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("project"))
			Expect(client).To(BeNil())
		})

		It("UT-VA-686-002: defaults location to us-central1 when empty", func() {
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", nil, "my-project", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-VA-686-003: returns error for malformed credentials JSON", func() {
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", []byte(`not-json`), "my-project", "us-central1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid credentials JSON"))
			Expect(client).To(BeNil())
		})

		It("UT-VA-686-004: accepts valid service account credentials JSON", func() {
			creds := `{
				"type": "service_account",
				"project_id": "test-project",
				"private_key_id": "key123",
				"private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA2a2rwplBQXWOijvBPSFsFullGP0mMz2LSBEqkbwC3TVqAzjN\nEBRPNVGGFBOBNkN8eFNSGjmhkqOBEN7eY3aBkDiCJiS5YzFkJp3KLNjZFJnHCbW\nMSCMGzZ5dTBqDEPbRfidYnmy8s4RloGcBOjiMVFLpCn2VK0TJXNB5dJ8xBDCnqO\nNHY6OkOKiYarFBBGbIneWfJcJMOJJa7wC1DRCFPSNM4NjOLVfV9tMMR5pR3HYTQ6\nvQfhipT9PTG0IOAZBjWKFg3HnNpSjvNL4y3/2YN3GBIycT1Bo6fJaSTn3oUEfzMk\nqKsNf6MKbx49mCZbq1UGAsgiVSfbGS2Zpy0w1QIDAQABAoIBADqT1tTjJLp8qlBx\nmYiDAH1fACEaGkuHmIA4FDdYPKJk36pASph7BOjE/5KL6DBRLHP2bLcpBR4NClsM\nR9MUDv8v7RFf+JI2pCLtFa4M+YrGOPfr0v2M32o4FvYGKAnfVnfWxgx3g5m6MPDI\nz6MHxLAileZvy0zLNUWPF/5P/gEB7PdL0z1p1mNqBJiDydOsAW3DxjLj0j5T+HFO\niOYRLiGpRC+NNb9PPXK/0XNaiLIqPzYqcY2YIhnRKvU3hXPBFa7Gf/eMK3XLjJen\nhBs8SbVX2PjQ/Y4T7v7K6/4SJdMh1QW2JqWb/HfFRP3BhClmMp4I6cz+VcaGnGMq\ncMDKRoECgYEA7+THK/xHRPV7R0b+TQ6P5wSBfZcP8KGmjjM3jZmV7i+tnP/NGBM\nRAMSqimqpuyGk4mHFGocLxpJMIDWR/38/M7etsRU9LdI4+BOSVqy6pFM+g2ilt/1\nfJQIFMHJR7QIOHFYnQiNMNQafGV/WzRrp6l0kt6/kklEJ3PKsunMHSUCgYEA6J2W\nSCJzMAbMWP0maYcV/hIJDST6u4sFi2vBbjm+AcXGF7KcNPLqRB/05OgFnyK5FBwn\nzEGqYnkshFJOYQ/c9zBJpAy3UA0NqCVo/aGkZaFo9ql9qhqjkn3BZNkWmzq7zUL\nYDwRfT0mjS4YoNfPqsz5J/g7rl6LMqSP6EVKX0ECgYAPS44w+gJJWJTR9y/Cixbb\nSG8zVPOBZ6UGvZuj8DOmNmq+bgSJPK/a+EIRvwCWKOjf7eP7HBnQXqB88oqXEIF\nkGaJFTqiPkhqrTZ/CLqCB/MsLp7pNr8/QdqOkF3oIfFM3KJ/tIMLDl0uwR6SlCVs\nIQC5GBL/5XT0q3IqJkUfaQKBgG5RAZp+M/QbtVCPXMqFMl03JVE3PavtGP1dUB+j\ncg7h/cYaAYr+3G2cS5LAnGbnmfCWLv7IhmhVzXHRLgFX2uGxGBsOMwJde7z0xU3f\nZ8/rVxleXQRbqVB/GZE8JpfbPTFCw/r15bz3mfPOtv7GP9WLxmnEjVJOxSZ6Gx0f\nxLYBAoGBAMSqcnQeJ+fli/Q9Wx9Mi9F8RvUv/BoN/3iPC0LGK5TlOjfYFdl6QVhi\n6h0tXm7ikLl7Mi4UsPkQSkJLwSe3HKcOwL3qdEOz98gm/REAK+bqoA9fHDSwl0oH\nK1u8/b3Mu7j7FHQKQixWERr6E6OaYG/2RJVsF7j8AKePR3AvLnfM\n-----END RSA PRIVATE KEY-----\n",
				"client_email": "test@test-project.iam.gserviceaccount.com",
				"client_id": "123456789",
				"auth_uri": "https://accounts.google.com/o/oauth2/auth",
				"token_uri": "https://oauth2.googleapis.com/token"
			}`
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", []byte(creds), "my-project", "us-central1")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-VA-686-005: accepts empty credentials (ambient ADC)", func() {
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", nil, "my-project", "us-central1")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-VA-686-006: whitespace-only credentials treated as empty (ambient ADC)", func() {
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", []byte("   \n  "), "my-project", "us-central1")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-VA-686-007: implements llm.Client interface", func() {
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", nil, "my-project", "us-central1")
			Expect(err).NotTo(HaveOccurred())
			var _ llm.Client = client
		})
	})

	Describe("Chat() request/response mapping", func() {
		var (
			server *httptest.Server
			client *vertexanthropic.Client
		)

		makeClient := func(handler http.HandlerFunc) {
			server = httptest.NewServer(handler)
			var err error
			client, err = vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", nil, "my-project", "us-central1",
				vertexanthropic.WithSDKOptions(
					option.WithBaseURL(server.URL),
					option.WithAPIKey("test-key"),
				),
			)
			Expect(err).NotTo(HaveOccurred())
		}

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		It("UT-VA-684-101: maps simple user+system messages and parses text response", func() {
			var receivedBody map[string]interface{}
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_test_001",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "The pod is OOMKilled."}],
					"usage": {"input_tokens": 50, "output_tokens": 10}
				}`))
			})

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: "You are a Kubernetes investigator."},
					{Role: "user", Content: "Why is the pod crashing?"},
				},
				Options: llm.ChatOptions{MaxTokens: 200},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Role).To(Equal("assistant"))
			Expect(resp.Message.Content).To(Equal("The pod is OOMKilled."))
			Expect(resp.Usage.PromptTokens).To(Equal(50))
			Expect(resp.Usage.CompletionTokens).To(Equal(10))
			Expect(resp.Usage.TotalTokens).To(Equal(60))

			Expect(receivedBody).To(HaveKey("system"))
			Expect(receivedBody).To(HaveKeyWithValue("max_tokens", BeNumerically("==", 200)))
		})

		It("UT-VA-684-102: maps tool call response", func() {
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_test_tc",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "tool_use",
					"content": [
						{"type": "tool_use", "id": "toolu_001", "name": "kubectl_describe", "input": {"kind": "Pod", "name": "api-server"}}
					],
					"usage": {"input_tokens": 100, "output_tokens": 30}
				}`))
			})

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Describe the crashing pod"},
				},
				Tools: []llm.ToolDefinition{
					{
						Name:        "kubectl_describe",
						Description: "Describe a Kubernetes resource",
						Parameters:  json.RawMessage(`{"properties":{"kind":{"type":"string"}},"required":["kind"]}`),
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ToolCalls).To(HaveLen(1))
			Expect(resp.ToolCalls[0].ID).To(Equal("toolu_001"))
			Expect(resp.ToolCalls[0].Name).To(Equal("kubectl_describe"))
			Expect(resp.ToolCalls[0].Arguments).To(ContainSubstring("Pod"))
		})

		It("UT-VA-684-103: maps tool result messages correctly", func() {
			var receivedBody map[string]interface{}
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_test_tr",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "The pod has 5 restarts."}],
					"usage": {"input_tokens": 80, "output_tokens": 15}
				}`))
			})

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Investigate crash"},
					{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{
						{ID: "toolu_001", Name: "kubectl_describe", Arguments: `{"kind":"Pod"}`},
					}},
					{Role: "tool", Content: `{"restartCount":5}`, ToolCallID: "toolu_001", ToolName: "kubectl_describe"},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(ContainSubstring("5 restarts"))

			messages, ok := receivedBody["messages"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(len(messages)).To(BeNumerically(">=", 3))
		})

		It("UT-VA-684-104: uses default MaxTokens when not specified", func() {
			var receivedBody map[string]interface{}
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_test_def",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "OK"}],
					"usage": {"input_tokens": 10, "output_tokens": 2}
				}`))
			})

			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKeyWithValue("max_tokens", BeNumerically("==", 4096)))
		})

		It("UT-VA-684-105: applies temperature when specified", func() {
			var receivedBody map[string]interface{}
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_test_temp",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "OK"}],
					"usage": {"input_tokens": 10, "output_tokens": 2}
				}`))
			})

			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
				Options:  llm.ChatOptions{Temperature: 0.7},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKeyWithValue("temperature", BeNumerically("~", 0.7, 0.01)))
		})

		It("UT-VA-684-106: handles mixed text + tool_use content blocks", func() {
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_test_mixed",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "tool_use",
					"content": [
						{"type": "text", "text": "Let me check."},
						{"type": "tool_use", "id": "toolu_002", "name": "get_logs", "input": {"pod": "api"}}
					],
					"usage": {"input_tokens": 40, "output_tokens": 20}
				}`))
			})

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "Get pod logs"}},
				Tools: []llm.ToolDefinition{
					{Name: "get_logs", Description: "Get pod logs", Parameters: json.RawMessage(`{"properties":{"pod":{"type":"string"}}}`)},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(Equal("Let me check."))
			Expect(resp.ToolCalls).To(HaveLen(1))
			Expect(resp.ToolCalls[0].Name).To(Equal("get_logs"))
		})

		It("UT-VA-684-107: handles HTTP error from Vertex AI", func() {
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": {"message": "internal error"}}`))
			})

			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("vertexanthropic"))
		})
	})
})
