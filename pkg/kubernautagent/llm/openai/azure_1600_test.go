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
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
)

// Issue #1600 / BR-AI-086: Azure OpenAI support wiring for KA's
// OpenAI-Chat-Completions-compatible wrapper. Mirrors
// pkg/shared/llm/openaicompat's WithAzureAPIVersion (azure_1600_test.go) —
// this test proves the option reaches the shared client unchanged.
var _ = Describe("kubernautagent/llm/openai.Client Azure OpenAI wiring — #1600", func() {
	var (
		server               *httptest.Server
		receivedPath         string
		receivedAPIKeyHeader string
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newTestServer := func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			receivedAPIKeyHeader = r.Header.Get("api-key")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
	}

	It("UT-KA-1600-401: WithAzureAPIVersion routes through the deployment-scoped URL with api-key auth", func() {
		newTestServer()
		client := kaopenai.New("gpt-4o-deploy", server.URL, "test-key", kaopenai.WithAzureAPIVersion("2024-10-21"))
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hi"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedPath).To(Equal("/openai/deployments/gpt-4o-deploy/chat/completions"))
		Expect(receivedAPIKeyHeader).To(Equal("test-key"))
	})

	It("UT-KA-1600-402: without WithAzureAPIVersion, the flat OpenAI path is unchanged (no regression)", func() {
		newTestServer()
		client := kaopenai.New("gpt-4o", server.URL, "test-key")
		_, err := client.Chat(context.Background(), llm.ChatRequest{
			Messages: []llm.Message{{Role: "user", Content: "hi"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedPath).To(Equal("/chat/completions"))
		Expect(receivedAPIKeyHeader).To(BeEmpty())
	})
})
