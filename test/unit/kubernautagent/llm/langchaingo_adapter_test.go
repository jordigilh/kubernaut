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
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	openaitypes "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
)

var _ = Describe("LangChainGo Adapter — #433", func() {

	Describe("New() provider selection", func() {
		It("should create an adapter for the openai provider", func() {
			adapter, err := langchaingo.New("openai", "http://localhost:9999", "test-model", "sk-test")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})

		It("should create an adapter for the ollama provider", func() {
			adapter, err := langchaingo.New("ollama", "http://localhost:11434", "llama3", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})

		It("should return an error for an unknown provider", func() {
			adapter, err := langchaingo.New("unknown-provider", "http://localhost:9999", "m", "k")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported"))
			Expect(adapter).To(BeNil())
		})

		It("UT-KA-433-200: should create an adapter for the azure provider", func() {
			adapter, err := langchaingo.New("azure", "https://my-resource.openai.azure.com", "gpt-4", "az-key", // pre-commit:allow-sensitive (test fixture)
				langchaingo.WithAzureAPIVersion("2024-02-15-preview"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})

		It("UT-KA-433-201: azure provider without API version should fail", func() {
			adapter, err := langchaingo.New("azure", "https://my-resource.openai.azure.com", "gpt-4", "az-key") // pre-commit:allow-sensitive (test fixture)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("api_version"))
			Expect(adapter).To(BeNil())
		})

		It("UT-KA-433-202: should create an adapter for the vertex provider", func() {
			_, thisFile, _, _ := runtime.Caller(0)
			fixturesDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "fixtures")
			credPath := filepath.Join(fixturesDir, "gcp-mock-credentials.json")
			Expect(credPath).To(BeAnExistingFile(), "GCP mock credentials fixture must exist")

			origCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credPath)).To(Succeed())
			DeferCleanup(func() {
				if origCreds == "" {
					Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
				} else {
					Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origCreds)).To(Succeed())
				}
			})

			adapter, err := langchaingo.New("vertex", "", "gemini-1.5-pro", "",
				langchaingo.WithVertexProject("my-project"),
				langchaingo.WithVertexLocation("us-central1"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})

		It("UT-KA-433-203: vertex provider without project should fail", func() {
			adapter, err := langchaingo.New("vertex", "", "gemini-1.5-pro", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("project"))
			Expect(adapter).To(BeNil())
		})

		It("UT-KA-433-209: should create an adapter for the anthropic provider", func() {
			adapter, err := langchaingo.New("anthropic", "", "claude-sonnet-4-20250514", "sk-ant-test")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
			var _ llm.Client = adapter
		})

		It("UT-KA-433-210: should create an adapter for the bedrock provider", func() {
			adapter, err := langchaingo.New("bedrock", "", "anthropic.claude-3-sonnet-20240229-v1:0", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
			var _ llm.Client = adapter
		})

		It("UT-KA-433-211: should create an adapter for the huggingface provider", func() {
			adapter, err := langchaingo.New("huggingface", "", "HuggingFaceH4/zephyr-7b-beta", "hf-test-token")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
			var _ llm.Client = adapter
		})

		It("UT-KA-433-212: should create an adapter for the mistral provider", func() {
			adapter, err := langchaingo.New("mistral", "", "mistral-large-latest", "mistral-test-key")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
			var _ llm.Client = adapter
		})

		It("UT-KA-433-213: bedrock adapter uses explicit region when WithBedrockRegion is provided", func() {
			adapter, err := langchaingo.New("bedrock", "", "anthropic.claude-3-sonnet-20240229-v1:0", "",
				langchaingo.WithBedrockRegion("eu-west-1"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})
	})

	Describe("Chat() text-only response", func() {
		var (
			server  *httptest.Server
			adapter llm.Client
		)

		BeforeEach(func() {
			content := "Root cause: OOMKilled due to memory limit exceeded."
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := openaitypes.ChatCompletionResponse{
					ID:      "chatcmpl-test001",
					Object:  openaitypes.ObjectChatCompletion,
					Created: openaitypes.FixedCreatedTime,
					Model:   "mock-model",
					Choices: []openaitypes.Choice{
						{
							Index: 0,
							Message: openaitypes.Message{
								Role:    "assistant",
								Content: &content,
							},
							FinishReason: "stop",
						},
					},
					Usage: openaitypes.Usage{
						PromptTokens:     100,
						CompletionTokens: 50,
						TotalTokens:      150,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(w).Encode(resp)).To(Succeed())
			}))

			var err error
			adapter, err = langchaingo.New("openai", server.URL, "mock-model", "sk-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			server.Close()
		})

		It("should return assistant content from a text-only response", func() {
			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: "You are a K8s investigator."},
					{Role: "user", Content: "Investigate OOMKilled pod"},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Role).To(Equal("assistant"))
			Expect(resp.Message.Content).To(ContainSubstring("OOMKilled"))
			Expect(resp.ToolCalls).To(BeEmpty())
		})

		It("should populate token usage from the response", func() {
			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "test"},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Usage.PromptTokens).To(Equal(100))
			Expect(resp.Usage.CompletionTokens).To(Equal(50))
			Expect(resp.Usage.TotalTokens).To(Equal(150))
		})
	})

	Describe("Chat() tool call response", func() {
		var (
			server  *httptest.Server
			adapter llm.Client
		)

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := openaitypes.ChatCompletionResponse{
					ID:      "chatcmpl-test002",
					Object:  openaitypes.ObjectChatCompletion,
					Created: openaitypes.FixedCreatedTime,
					Model:   "mock-model",
					Choices: []openaitypes.Choice{
						{
							Index: 0,
							Message: openaitypes.Message{
								Role: "assistant",
								ToolCalls: []openaitypes.ToolCall{
									{
										ID:   "call_abc123",
										Type: "function",
										Function: openaitypes.FunctionCall{
											Name:      "kubectl_describe",
											Arguments: `{"kind":"Pod","name":"api-server","namespace":"prod"}`,
										},
									},
									{
										ID:   "call_def456",
										Type: "function",
										Function: openaitypes.FunctionCall{
											Name:      "list_workflows",
											Arguments: `{"action_type":"IncreaseMemory"}`,
										},
									},
								},
							},
							FinishReason: "tool_calls",
						},
					},
					Usage: openaitypes.Usage{
						PromptTokens:     500,
						CompletionTokens: 50,
						TotalTokens:      550,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(w).Encode(resp)).To(Succeed())
			}))

			var err error
			adapter, err = langchaingo.New("openai", server.URL, "mock-model", "sk-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			server.Close()
		})

		It("should map tool calls with correct ID, Name, and Arguments", func() {
			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: "You are a K8s investigator."},
					{Role: "user", Content: "Investigate pod crash"},
				},
				Tools: []llm.ToolDefinition{
					{
						Name:        "kubectl_describe",
						Description: "Describe a Kubernetes resource",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}}}`),
					},
					{
						Name:        "list_workflows",
						Description: "List available workflows",
						Parameters:  json.RawMessage(`{"type":"object","properties":{"action_type":{"type":"string"}}}`),
					},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ToolCalls).To(HaveLen(2))

			Expect(resp.ToolCalls[0].ID).To(Equal("call_abc123"))
			Expect(resp.ToolCalls[0].Name).To(Equal("kubectl_describe"))
			Expect(resp.ToolCalls[0].Arguments).To(ContainSubstring("api-server"))

			Expect(resp.ToolCalls[1].ID).To(Equal("call_def456"))
			Expect(resp.ToolCalls[1].Name).To(Equal("list_workflows"))
			Expect(resp.ToolCalls[1].Arguments).To(ContainSubstring("IncreaseMemory"))
		})
	})

	Describe("Chat() tool response message construction", func() {
		var (
			server       *httptest.Server
			adapter      llm.Client
			receivedBody []byte
		)

		BeforeEach(func() {
			content := "I found the issue."
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var reqBody json.RawMessage
				Expect(json.NewDecoder(r.Body).Decode(&reqBody)).To(Succeed())
				receivedBody = reqBody

				resp := openaitypes.ChatCompletionResponse{
					ID:      "chatcmpl-test003",
					Object:  openaitypes.ObjectChatCompletion,
					Created: openaitypes.FixedCreatedTime,
					Model:   "mock-model",
					Choices: []openaitypes.Choice{
						{
							Index: 0,
							Message: openaitypes.Message{
								Role:    "assistant",
								Content: &content,
							},
							FinishReason: "stop",
						},
					},
					Usage: openaitypes.Usage{PromptTokens: 200, CompletionTokens: 30, TotalTokens: 230},
				}
				w.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(w).Encode(resp)).To(Succeed())
			}))

			var err error
			adapter, err = langchaingo.New("openai", server.URL, "mock-model", "sk-test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			server.Close()
		})

		It("should send tool result messages to the LLM endpoint", func() {
			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: "You are a K8s investigator."},
					{Role: "user", Content: "Investigate crash"},
					{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{
						{ID: "call_tc1", Name: "kubectl_describe", Arguments: `{"kind":"Pod"}`},
					}},
					{Role: "tool", Content: `{"status":"Running","restartCount":5}`, ToolCallID: "call_tc1"},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Content).To(ContainSubstring("found the issue"))

			Expect(receivedBody).NotTo(BeEmpty(), "expected the adapter to make an HTTP request to the mock server")
			var sent openaitypes.ChatCompletionRequest
			Expect(json.Unmarshal(receivedBody, &sent)).To(Succeed())
			Expect(sent.Messages).To(HaveLen(4))
			Expect(sent.Messages[3].Role).To(Equal("tool"))
		})
	})

	Describe("UT-KA-433-051: Anthropic adapter Chat() via mock HTTP server", func() {
		var (
			server  *httptest.Server
			adapter llm.Client
		)

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/messages"))
				Expect(r.Header.Get("x-api-key")).To(Equal("sk-ant-test"))
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

				resp := map[string]interface{}{
					"id":    "msg_test_001",
					"type":  "message",
					"role":  "assistant",
					"model": "claude-sonnet-4-20250514",
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": "The pod is OOMKilled due to memory limits.",
						},
					},
					"stop_reason": "end_turn",
					"usage": map[string]interface{}{
						"input_tokens":  120,
						"output_tokens": 45,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(w).Encode(resp)).To(Succeed())
			}))

			var err error
			adapter, err = langchaingo.New("anthropic", server.URL, "claude-sonnet-4-20250514", "sk-ant-test")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})

		AfterEach(func() {
			server.Close()
		})

		It("should route Chat() to the configured Anthropic endpoint and return content", func() {
			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Why is the pod crashing?"},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Role).To(Equal("assistant"))
			Expect(resp.Message.Content).To(ContainSubstring("OOMKilled"))
		})
	})

	Describe("UT-KA-433-052: Mistral adapter Chat() via mock HTTP server", func() {
		var (
			server  *httptest.Server
			adapter llm.Client
		)

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/v1/chat/completions"))
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer mistral-test-key"))
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

				resp := map[string]interface{}{
					"id":      "chatcmpl-mistral-001",
					"object":  "chat.completion",
					"created": 1700000000,
					"model":   "mistral-large-latest",
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"message": map[string]interface{}{
								"role":    "assistant",
								"content": "The deployment has insufficient replicas for the load.",
							},
							"finish_reason": "stop",
						},
					},
					"usage": map[string]interface{}{
						"prompt_tokens":     80,
						"completion_tokens": 35,
						"total_tokens":      115,
					},
				}
				w.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(w).Encode(resp)).To(Succeed())
			}))

			var err error
			adapter, err = langchaingo.New("mistral", server.URL, "mistral-large-latest", "mistral-test-key")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter).NotTo(BeNil())
		})

		AfterEach(func() {
			server.Close()
		})

		It("should route Chat() to the configured Mistral endpoint and return content", func() {
			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Why is the deployment degraded?"},
				},
			}

			resp, err := adapter.Chat(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Message.Role).To(Equal("assistant"))
			Expect(resp.Message.Content).To(ContainSubstring("insufficient replicas"))
		})
	})

	Describe("Chat() error handling", func() {
		It("should return an error when the LLM returns HTTP 500", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				Expect(json.NewEncoder(w).Encode(openaitypes.ErrorResponse{
					Error: openaitypes.ErrorDetail{
						Message: "internal server error",
						Type:    "server_error",
						Code:    "internal_error",
					},
				})).To(Succeed())
			}))
			defer server.Close()

			adapter, err := langchaingo.New("openai", server.URL, "mock-model", "sk-test")
			Expect(err).NotTo(HaveOccurred())

			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "test"},
				},
			}

			_, err = adapter.Chat(context.Background(), req)
			Expect(err).To(HaveOccurred())
		})

		It("should return an error when the LLM returns malformed JSON", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{invalid json`))
				Expect(err).NotTo(HaveOccurred())
			}))
			defer server.Close()

			adapter, err := langchaingo.New("openai", server.URL, "mock-model", "sk-test")
			Expect(err).NotTo(HaveOccurred())

			req := llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "test"},
				},
			}

			_, err = adapter.Chat(context.Background(), req)
			Expect(err).To(HaveOccurred())
		})
	})
})
