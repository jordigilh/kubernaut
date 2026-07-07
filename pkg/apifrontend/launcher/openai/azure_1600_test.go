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

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	openaimodel "github.com/jordigilh/kubernaut/pkg/apifrontend/launcher/openai"
)

// Issue #1600 / BR-AI-086: Azure OpenAI support for AF's ADK-facing
// OpenAI-Chat-Completions-compatible adapter — net-new for AF (it never had
// Azure support before), added at parity with KA's equivalent wiring, using
// the identical shared pkg/shared/llm/openaicompat Azure mode.
var _ = Describe("apifrontend/launcher/openai.Model Azure OpenAI wiring — #1600", func() {
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

	simpleRequest := func() *model.LLMRequest {
		return &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: "user", Parts: []*genai.Part{{Text: "hello"}}},
			},
		}
	}

	It("UT-AF-1600-601: WithAzureAPIVersion routes through the deployment-scoped URL with api-key auth", func() {
		newTestServer()
		m := openaimodel.NewModel("gpt-4o-deploy", server.URL, "test-key", openaimodel.WithAzureAPIVersion("2024-10-21"))
		for resp, err := range m.GenerateContent(context.Background(), simpleRequest(), false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(receivedPath).To(Equal("/openai/deployments/gpt-4o-deploy/chat/completions"))
		Expect(receivedAPIKeyHeader).To(Equal("test-key"))
	})

	It("UT-AF-1600-602: without WithAzureAPIVersion, the flat OpenAI path is unchanged (no regression)", func() {
		newTestServer()
		m := openaimodel.NewModel("gpt-4o", server.URL, "test-key")
		for resp, err := range m.GenerateContent(context.Background(), simpleRequest(), false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}
		Expect(receivedPath).To(Equal("/chat/completions"))
		Expect(receivedAPIKeyHeader).To(BeEmpty())
	})
})
