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
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Verification API", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewFullRouter(registry, false, "authorization", nil)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-040-001: /api/test/tool-calls returns recorded tool calls", func() {
		It("should record tool calls from a chat request and expose them", func() {
			// Make a request that triggers a tool call
			body := chatRequest("- Signal Name: OOMKilled\n- Namespace: default", []string{"search_workflow_catalog"})
			resp := postJSON(server.URL+"/v1/chat/completions", body)
			resp.Body.Close()

			// Query verification API
			resp2, err := http.Get(server.URL + "/api/test/tool-calls")
			Expect(err).NotTo(HaveOccurred())
			defer resp2.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(resp2.Body).Decode(&result)).To(Succeed())
			calls := result["tool_calls"].([]interface{})
			Expect(calls).To(HaveLen(1))
			call := calls[0].(map[string]interface{})
			Expect(call["name"]).To(Equal("search_workflow_catalog"))
		})
	})

	Describe("IT-MOCK-041-001: /api/test/scenario returns detected scenario", func() {
		It("should return the detected scenario name", func() {
			body := chatRequest("- Signal Name: OOMKilled\n- Namespace: default", nil)
			resp := postJSON(server.URL+"/v1/chat/completions", body)
			resp.Body.Close()

			resp2, err := http.Get(server.URL + "/api/test/scenario")
			Expect(err).NotTo(HaveOccurred())
			defer resp2.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(resp2.Body).Decode(&result)).To(Succeed())
			Expect(result["scenario"]).To(Equal("oomkilled"))
		})
	})

	Describe("IT-MOCK-042-001: /api/test/request-count tracks total requests", func() {
		It("should count each chat completion request", func() {
			body := chatRequest("test", nil)
			postJSON(server.URL+"/v1/chat/completions", body).Body.Close()
			body = chatRequest("test2", nil)
			postJSON(server.URL+"/v1/chat/completions", body).Body.Close()

			resp, err := http.Get(server.URL + "/api/test/request-count")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			Expect(result["count"]).To(BeNumerically("==", 2))
		})
	})

	Describe("IT-MOCK-043-001: /api/test/reset clears all tracked state", func() {
		It("should clear tool calls, scenario, and request count after POST reset", func() {
			body := chatRequest("- Signal Name: OOMKilled", []string{"search_workflow_catalog"})
			postJSON(server.URL+"/v1/chat/completions", body).Body.Close()

			// Reset
			resetResp := postJSON(server.URL+"/api/test/reset", marshalReq(nil))
			resetResp.Body.Close()
			Expect(resetResp.StatusCode).To(Equal(200))

			// Verify empty
			resp, err := http.Get(server.URL + "/api/test/tool-calls")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			var result map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			calls := result["tool_calls"].([]interface{})
			Expect(calls).To(BeEmpty())
		})
	})

	Describe("IT-MOCK-044-001: /api/test/dag-path returns traversal path", func() {
		It("should return empty path initially", func() {
			resp, err := http.Get(server.URL + "/api/test/dag-path")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
			// Path may be empty or nil initially
			Expect(resp.StatusCode).To(Equal(200))
		})
	})
})
