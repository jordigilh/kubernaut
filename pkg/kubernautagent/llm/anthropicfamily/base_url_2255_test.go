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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
)

// Issue #2255 / BR-AI-089: native Anthropic API-key auth mode must honor an
// operator-configured endpoint override (FedRAMP AC-4, information flow
// enforcement — see issue #1819's endpoint-keyed NetworkPolicy egress
// allowlist for the concrete downstream consumer of this behavior). Prior to
// this, the only way to redirect native Anthropic traffic was the
// internal/test-only WithSDKOptions(option.WithBaseURL(...)) escape hatch;
// WithBaseURL promotes this into a first-class, cfg-driven production Option.
var _ = Describe("anthropicfamily.WithBaseURL — #2255", func() {

	Describe("NewWithAPIKey — native auth honors WithBaseURL", func() {
		var server *httptest.Server

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
		})

		It("UT-KA-2255-001 [AC-4]: Chat() sends requests to the overridden base URL, not the SDK default", func() {
			var received bool
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received = true
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_2255_001",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "endpoint override works"}],
					"usage": {"input_tokens": 10, "output_tokens": 5}
				}`))
			}))

			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6",
				anthropicfamily.WithBaseURL(server.URL),
			)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(received).To(BeTrue(), "request must land on the operator-configured endpoint, not api.anthropic.com")
			Expect(resp.Message.Content).To(Equal("endpoint override works"))
		})

		It("UT-KA-2255-001b: request body is unaffected by the base URL override (no Vertex-specific fields leak in)", func() {
			var receivedBody map[string]interface{}
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := make([]byte, r.ContentLength)
				_, _ = r.Body.Read(body)
				_ = json.Unmarshal(body, &receivedBody)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{
					"id": "msg_2255_002",
					"type": "message",
					"role": "assistant",
					"model": "claude-sonnet-4-6",
					"stop_reason": "end_turn",
					"content": [{"type": "text", "text": "ok"}],
					"usage": {"input_tokens": 5, "output_tokens": 2}
				}`))
			}))

			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6",
				anthropicfamily.WithBaseURL(server.URL),
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hello"}},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedBody).To(HaveKeyWithValue("model", "claude-sonnet-4-6"))
		})

		It("UT-KA-2255-001c: empty WithBaseURL (unset) is a no-op — zero regression for existing deployments", func() {
			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6",
				anthropicfamily.WithBaseURL(""),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Describe("New (Vertex) — WithBaseURL is rejected, not silently ignored", func() {
		var origADC string

		BeforeEach(func() {
			origADC = os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
			adcPath := filepath.Join(GinkgoT().TempDir(), "adc.json")
			Expect(os.WriteFile(adcPath, generateFakeServiceAccountJSON(), 0600)).To(Succeed())
			Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", adcPath)).To(Succeed())
		})

		AfterEach(func() {
			if origADC != "" {
				Expect(os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", origADC)).To(Succeed())
			} else {
				Expect(os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")).To(Succeed())
			}
		})

		// UT-KA-2255-002 [AC-4]: New (Vertex-hosted auth) must not silently
		// accept WithBaseURL — Vertex's endpoint is derived from
		// vertexProject/vertexLocation via the SDK's own middleware.
		// Accepting-but-ignoring the option here would reintroduce the exact
		// silent-footgun class issue #2255 fixes, just scoped to Vertex
		// instead of native auth (BR-AI-089 AC4). The existing
		// WithSDKOptions(option.WithBaseURL(...)) escape hatch remains
		// available for Vertex tests that need to redirect to an httptest
		// server (see client_test.go's makeClient) — this guard is specific
		// to the new first-class WithBaseURL Option, not the raw SDK option.
		It("UT-KA-2255-002 [AC-4]: returns an explicit error rather than ignoring the option", func() {
			client, err := anthropicfamily.New(context.Background(),
				"claude-sonnet-4-6", nil, "my-project", "us-central1",
				anthropicfamily.WithBaseURL("https://gateway.example.com"),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("WithBaseURL"))
			Expect(client).To(BeNil())
		})

		It("UT-KA-2255-002b: New still succeeds when WithBaseURL is not supplied (no regression)", func() {
			client, err := anthropicfamily.New(context.Background(),
				"claude-sonnet-4-6", nil, "my-project", "us-central1")
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})
})
