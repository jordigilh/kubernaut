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

package kubernautagent_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	llmclient "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	mockllmutil "github.com/jordigilh/kubernaut/test/testutil/mockllm"
)

// oomChatRequestBody is a minimal chat-completion request that the Mock LLM's
// default scenario registry recognizes (OOMKilled signal), so these tests can
// assert on a genuine 200 response instead of a hand-rolled fixture reply.
const oomChatRequestBody = `{"model":"mock-model","messages":[{"role":"user","content":"- Signal Name: OOMKilled"}]}`

var _ = Describe("Auth Headers Mock LLM Verification — #417", func() {

	// IT-KA-417-006: End-to-end with the real Mock LLM Go service's
	// verification API (#570) -- both the header-injecting production
	// llmclient and the header-recording Mock LLM router are the real
	// implementations, not stand-ins for each other.
	Describe("IT-KA-417-006: Mock LLM records and exposes injected headers", func() {
		var (
			server *httptest.Server
			client *mockllmutil.Client
		)

		BeforeEach(func() {
			registry := scenarios.DefaultRegistry()
			router := handlers.NewFullRouter(registry, true, "", "Authorization", fault.NewInjector())
			server = httptest.NewServer(router)
			client = mockllmutil.NewClient(server.URL)
		})

		AfterEach(func() {
			server.Close()
		})

		It("should verify headers via Mock LLM verification API", func() {
			hdefs := []types.LLMHeaderDef{
				{Name: "Authorization", Value: "Bearer test-token-e2e"},
			}
			llm, err := llmclient.NewLLMClient(server.URL, hdefs)
			Expect(err).NotTo(HaveOccurred())

			resp, err := llm.Post(server.URL+"/v1/chat/completions",
				"application/json",
				strings.NewReader(oomChatRequestBody))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp.Body.Close()

			client.AssertHeaderReceived("Authorization", "Bearer test-token-e2e")
		})

		It("should process chat completion normally despite auth headers", func() {
			hdefs := []types.LLMHeaderDef{
				{Name: "Authorization", Value: "Bearer test-token-e2e"},
				{Name: "x-correlation-id", Value: "req-001"},
			}
			llm, err := llmclient.NewLLMClient(server.URL, hdefs)
			Expect(err).NotTo(HaveOccurred())

			resp, err := llm.Post(server.URL+"/v1/chat/completions",
				"application/json",
				strings.NewReader(oomChatRequestBody))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var result openai.ChatCompletionResponse
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result.Choices).NotTo(BeEmpty(),
				"Mock LLM should return a normal chat completion response")
		})
	})
})
