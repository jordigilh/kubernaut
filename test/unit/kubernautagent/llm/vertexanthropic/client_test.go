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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
)

// generateFakeServiceAccountJSON builds a GCP service account credential blob
// with a real RSA-2048 key so that the SDK's JWT-signing transport works in
// tests. The key is generated fresh each run and never leaves the process.
func generateFakeServiceAccountJSON() []byte {
	return generateFakeServiceAccountJSONWithTokenURL("https://oauth2.googleapis.com/token")
}

func generateFakeServiceAccountJSONWithTokenURL(tokenURL string) []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("generate RSA key for test credentials: %v", err))
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	creds := map[string]string{
		"type":           "service_account",
		"project_id":     "test-project",
		"private_key_id": "key123",
		"private_key":    string(keyPEM), // notsecret — generated at runtime via rsa.GenerateKey
		"client_email":   "test@test-project.iam.gserviceaccount.com",
		"client_id":      "123456789",
		"auth_uri":       "https://accounts.google.com/o/oauth2/auth",
		"token_uri":      tokenURL,
	}
	b, _ := json.Marshal(creds)
	return b
}

var _ = Describe("vertexanthropic.Client — #684 #686", func() {

	Describe("New() constructor validation", func() {
		var (
			fakeCreds []byte
			origADC   string
		)

		BeforeEach(func() {
			fakeCreds = generateFakeServiceAccountJSON()
			origADC = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
			adcPath := filepath.Join(GinkgoT().TempDir(), "adc.json")
			Expect(os.WriteFile(adcPath, fakeCreds, 0600)).To(Succeed())
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcPath)).To(Succeed())
		})

		AfterEach(func() {
			if origADC != "" {
				os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADC)
			} else {
				os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
			}
		})

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
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", fakeCreds, "my-project", "us-central1")
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

		It("UT-VA-686-008: rejects external_account credentials (SA1019 mitigation)", func() {
			externalAccountJSON := []byte(`{
				"type": "external_account",
				"audience": "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
				"token_url": "https://sts.googleapis.com/v1/token",
				"credential_source": {"url": "https://attacker.example.com/token"}
			}`)
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", externalAccountJSON, "my-project", "us-central1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported credential type"))
			Expect(err.Error()).To(ContainSubstring("external_account"))
			Expect(client).To(BeNil())
		})

		It("UT-VA-686-009: rejects credentials with unknown type field", func() {
			unknownJSON := []byte(`{"type": "weird_unknown_type", "token": "x"}`)
			client, err := vertexanthropic.New(context.Background(),
				"claude-sonnet-4-6", unknownJSON, "my-project", "us-central1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported credential type"))
			Expect(client).To(BeNil())
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
			server     *httptest.Server
			tokenSrv   *httptest.Server
			client     *vertexanthropic.Client
			fakeCreds  []byte
			makeClient func(http.HandlerFunc)
		)

		// makeClient spins up an httptest server for the Vertex AI endpoint and a
		// separate token server that satisfies the OAuth2 JWT exchange. The fake
		// service account credentials reference the token server's URL so the SDK
		// obtains a Bearer token without hitting real GCP.
		makeClient = func(handler http.HandlerFunc) {
			tokenSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		It("UT-VA-684-102: maps tool call response and populates Message.ToolCalls", func() {
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

			By("Message.ToolCalls must also be populated for conversation history")
			Expect(resp.Message.ToolCalls).To(HaveLen(1))
			Expect(resp.Message.ToolCalls[0].ID).To(Equal("toolu_001"))
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

		It("UT-VA-686-108: coalesces multiple tool results into a single user message", func() {
			var receivedBody map[string]interface{}
			makeClient(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_test_coalesce",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "Both pods are crashing."}],
					"usage": {"input_tokens": 120, "output_tokens": 20}
				}`))
			})

			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "user", Content: "Investigate crash"},
					{Role: "assistant", Content: "Let me check.", ToolCalls: []llm.ToolCall{
						{ID: "toolu_001", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"pod-a"}`},
						{ID: "toolu_002", Name: "kubectl_events", Arguments: `{"kind":"Pod","name":"pod-b"}`},
						{ID: "toolu_003", Name: "kubectl_logs", Arguments: `{"name":"pod-a"}`},
					}},
					{Role: "tool", Content: `{"status":"CrashLoopBackOff"}`, ToolCallID: "toolu_001", ToolName: "kubectl_describe"},
					{Role: "tool", Content: `{"events":["BackOff"]}`, ToolCallID: "toolu_002", ToolName: "kubectl_events"},
					{Role: "tool", Content: `OOMKilled`, ToolCallID: "toolu_003", ToolName: "kubectl_logs"},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			messages, ok := receivedBody["messages"].([]interface{})
			Expect(ok).To(BeTrue())

			By("expecting: user, assistant(tool_use x3), user(tool_result x3) = 3 messages")
			Expect(messages).To(HaveLen(3))

			By("message[1] should be assistant with 3+1 content blocks (text + 3 tool_use)")
			assistantMsg := messages[1].(map[string]interface{})
			Expect(assistantMsg["role"]).To(Equal("assistant"))
			assistantContent := assistantMsg["content"].([]interface{})
			Expect(assistantContent).To(HaveLen(4))

			By("message[2] should be a single user message with 3 tool_result blocks")
			toolResultMsg := messages[2].(map[string]interface{})
			Expect(toolResultMsg["role"]).To(Equal("user"))
			toolResultContent := toolResultMsg["content"].([]interface{})
			Expect(toolResultContent).To(HaveLen(3))

			for _, block := range toolResultContent {
				b := block.(map[string]interface{})
				Expect(b["type"]).To(Equal("tool_result"))
			}
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
