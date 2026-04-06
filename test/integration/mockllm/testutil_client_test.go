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
package mockllm_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
	mockllmutil "github.com/jordigilh/kubernaut/test/testutil/mockllm"
)

var _ = Describe("Testutil Mock LLM Client", func() {

	var (
		server *httptest.Server
		client *mockllmutil.Client
	)

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewFullRouter(registry, false, "Authorization", fault.NewInjector())
		server = httptest.NewServer(router)
		client = mockllmutil.NewClient(server.URL)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-563-006: Test helper asserts tool call sequence", func() {
		It("should assert correct tool call sequence via helper", func() {
			body := chatRequest(
				"- Signal Name: OOMKilled\n- Namespace: default",
				[]string{openai.ToolSearchWorkflowCatalog},
			)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			_ = err
			resp.Body.Close()

			client.AssertToolCalled(openai.ToolSearchWorkflowCatalog)
			client.AssertToolSequence(openai.ToolSearchWorkflowCatalog)
			client.AssertScenarioMatched("oomkilled")
			client.AssertDAGPath("dispatch", openai.ToolSearchWorkflowCatalog)
			client.AssertRequestCount(1)
		})
	})

	Describe("IT-MOCK-570-003: Test helper asserts headers", func() {
		It("should assert header presence and absence via helper", func() {
			body := chatRequest("- Signal Name: OOMKilled", nil)
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", body)
			req.Header.Set("Authorization", "Bearer helper-test")
			req.Header.Set("Content-Type", "application/json")

			resp, _ := http.DefaultClient.Do(req)
			resp.Body.Close()

			client.AssertHeaderReceived("Authorization", "Bearer helper-test")
			client.AssertNoHeaderReceived("X-Not-Configured")
		})
	})

	Describe("IT-MOCK-563-007: Test helper reset works", func() {
		It("should reset state via helper and verify clean state", func() {
			body := chatRequest("- Signal Name: OOMKilled",
				[]string{openai.ToolSearchWorkflowCatalog})
			resp, _ := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			resp.Body.Close()

			client.AssertRequestCount(1)
			client.Reset()
			client.AssertRequestCount(0)
		})
	})
})
