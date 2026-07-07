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
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// Issue #1600 / BR-AI-086: Azure OpenAI support for the shared
// OpenAI-Chat-Completions client. Azure diverges from real OpenAI/OpenAI-
// compatible endpoints in exactly two ways this package must account for:
// a deployment-scoped URL (with a mandatory api-version query parameter)
// instead of a flat /chat/completions path, and an api-key header instead
// of Authorization: Bearer. This regression was introduced when #1598
// replaced langchaingo's permissive provider dispatch with an explicit
// allowlist — langchaingo's own Azure backend used exactly this URL/header
// shape, with the model name doubling as the Azure deployment ID (the
// common Azure convention, and the historical behavior being restored
// here, not a new design).
var _ = Describe("openaicompat.Client Azure OpenAI support — #1600", func() {
	var (
		server               *httptest.Server
		receivedPath         string
		receivedQuery        string
		receivedAPIKeyHeader string
		receivedAuthHeader   string
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newTestServer := func() {
		receivedPath, receivedQuery, receivedAPIKeyHeader, receivedAuthHeader = "", "", "", ""
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			receivedQuery = r.URL.RawQuery
			receivedAPIKeyHeader = r.Header.Get("api-key")
			receivedAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
	}

	It("UT-OAC-1600-101: WithAzureAPIVersion sends the deployment-scoped URL with api-version query param", func() {
		newTestServer()
		client := openaicompat.New("gpt-4o-deploy", server.URL, "test-key", openaicompat.WithAzureAPIVersion("2024-10-21"))
		_, err := client.Chat(context.Background(), openaicompat.Request{
			Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedPath).To(Equal("/openai/deployments/gpt-4o-deploy/chat/completions"))
		Expect(receivedQuery).To(Equal("api-version=2024-10-21"))
	})

	It("UT-OAC-1600-102: WithAzureAPIVersion sends api-key header instead of Authorization: Bearer", func() {
		newTestServer()
		client := openaicompat.New("gpt-4o-deploy", server.URL, "test-key", openaicompat.WithAzureAPIVersion("2024-10-21"))
		_, err := client.Chat(context.Background(), openaicompat.Request{
			Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedAPIKeyHeader).To(Equal("test-key"))
		Expect(receivedAuthHeader).To(BeEmpty())
	})

	It("UT-OAC-1600-103: without WithAzureAPIVersion, the flat /chat/completions path and Bearer auth are unchanged (no regression)", func() {
		newTestServer()
		client := openaicompat.New("gpt-4o", server.URL, "test-key")
		_, err := client.Chat(context.Background(), openaicompat.Request{
			Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedPath).To(Equal("/chat/completions"))
		Expect(receivedQuery).To(BeEmpty())
		Expect(receivedAPIKeyHeader).To(BeEmpty())
		Expect(receivedAuthHeader).To(Equal("Bearer test-key"))
	})

	It("UT-OAC-1600-104: an empty apiKey sends no api-key header even in Azure mode (leaves room for a transport-layer AAD Bearer token)", func() {
		newTestServer()
		client := openaicompat.New("gpt-4o-deploy", server.URL, "", openaicompat.WithAzureAPIVersion("2024-10-21"))
		_, err := client.Chat(context.Background(), openaicompat.Request{
			Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedAPIKeyHeader).To(BeEmpty())
		Expect(receivedAuthHeader).To(BeEmpty())
	})

	It("UT-OAC-1600-105: StreamChat honors the same Azure URL/header shape as Chat", func() {
		receivedPath, receivedQuery, receivedAPIKeyHeader = "", "", ""
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			receivedQuery = r.URL.RawQuery
			receivedAPIKeyHeader = r.Header.Get("api-key")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		client := openaicompat.New("gpt-4o-deploy", server.URL, "test-key", openaicompat.WithAzureAPIVersion("2024-10-21"))
		err := client.StreamChat(context.Background(), openaicompat.Request{
			Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
		}, func(openaicompat.StreamEvent) bool { return true })
		Expect(err).NotTo(HaveOccurred())
		Expect(receivedPath).To(Equal("/openai/deployments/gpt-4o-deploy/chat/completions"))
		Expect(receivedQuery).To(Equal("api-version=2024-10-21"))
		Expect(receivedAPIKeyHeader).To(Equal("test-key"))
	})
})
