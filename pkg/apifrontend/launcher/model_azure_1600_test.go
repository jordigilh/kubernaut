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

package launcher_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #1600 / BR-AI-086: Azure OpenAI support wiring — end-to-end from
// types.LLMConfig.AzureAPIVersion to the outgoing HTTP request's URL and
// auth header, net-new for AF (it never had Azure support before).
var _ = Describe("NewModelFromConfig — Azure OpenAI wiring (#1600)", func() {
	var (
		server               *httptest.Server
		receivedPath         string
		receivedAPIKeyHeader string
	)

	BeforeEach(func() {
		receivedPath, receivedAPIKeyHeader = "", ""
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			receivedAPIKeyHeader = r.Header.Get("api-key")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		}))
	})

	AfterEach(func() {
		server.Close()
	})

	It("UT-AF-1600-701: cfg.AzureAPIVersion reaches the outgoing request as a deployment-scoped URL with api-key auth", func() {
		cfg := types.LLMConfig{
			Provider:        types.LLMProviderOpenAI,
			Model:           "gpt-4o-deploy",
			Endpoint:        server.URL,
			APIKey:          "azure-fake-test-key",
			AzureAPIVersion: "2024-10-21",
		}

		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())

		req := &model.LLMRequest{Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}}}
		for resp, err := range m.GenerateContent(context.Background(), req, false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}

		Expect(receivedPath).To(Equal("/openai/deployments/gpt-4o-deploy/chat/completions"),
			"types.LLMConfig.AzureAPIVersion must switch the outgoing request to Azure's deployment-scoped URL")
		Expect(receivedAPIKeyHeader).To(Equal("azure-fake-test-key"))
	})

	It("UT-AF-1600-702: without AzureAPIVersion, the flat OpenAI path is unchanged (no regression)", func() {
		cfg := types.LLMConfig{
			Provider: types.LLMProviderOpenAI,
			Model:    "gpt-4o",
			Endpoint: server.URL,
			APIKey:   "sk-fake-test-key",
		}

		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())

		req := &model.LLMRequest{Contents: []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: "hi"}}}}}
		for resp, err := range m.GenerateContent(context.Background(), req, false) {
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}

		Expect(receivedPath).To(Equal("/chat/completions"))
		Expect(receivedAPIKeyHeader).To(BeEmpty())
	})
})
