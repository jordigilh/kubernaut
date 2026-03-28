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

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Verification API", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewFullRouter(registry, false, "Authorization,X-Custom", fault.NewInjector())
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-563-001: DAG path is recorded and queryable", func() {
		It("should return the DAG traversal path after a chat request", func() {
			body := chatRequest("- Signal Name: OOMKilled\n- Namespace: default",
				[]string{openai.ToolSearchWorkflowCatalog})
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))

			pathResp, err := http.Get(server.URL + "/api/test/dag-path")
			Expect(err).NotTo(HaveOccurred())
			defer pathResp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(pathResp.Body).Decode(&result)).To(Succeed())
			path := result["path"].([]interface{})
			Expect(path).To(HaveLen(2))
			Expect(path[0]).To(Equal("dispatch"))
		})
	})

	Describe("IT-MOCK-563-002: Tool calls are recorded with sequence", func() {
		It("should record tool calls in order", func() {
			body := chatRequest(
				"- Signal Name: OOMKilled\n- Namespace: default",
				[]string{openai.ToolSearchWorkflowCatalog},
			)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			tcResp, err := http.Get(server.URL + "/api/test/tool-calls")
			Expect(err).NotTo(HaveOccurred())
			defer tcResp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(tcResp.Body).Decode(&result)).To(Succeed())
			toolCalls := result["tool_calls"].([]interface{})
			Expect(toolCalls).To(HaveLen(1))
			first := toolCalls[0].(map[string]interface{})
			Expect(first["name"]).To(Equal(openai.ToolSearchWorkflowCatalog))
		})
	})

	Describe("IT-MOCK-563-003: Scenario detection is recorded", func() {
		It("should return the detected scenario name", func() {
			body := chatRequest("- Signal Name: CrashLoopBackOff\n- Namespace: staging", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			scResp, err := http.Get(server.URL + "/api/test/scenario")
			Expect(err).NotTo(HaveOccurred())
			defer scResp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(scResp.Body).Decode(&result)).To(Succeed())
			Expect(result["scenario"]).To(Equal("crashloop"))
		})
	})

	Describe("IT-MOCK-563-004: Reset clears all state", func() {
		It("should clear tool calls, scenario, DAG path after reset", func() {
			body := chatRequest("- Signal Name: OOMKilled",
				[]string{openai.ToolSearchWorkflowCatalog})
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/test/reset", nil)
			resetResp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resetResp.Body.Close()
			Expect(resetResp.StatusCode).To(Equal(200))

			// After reset, scenario should be empty
			scResp, err := http.Get(server.URL + "/api/test/scenario")
			Expect(err).NotTo(HaveOccurred())
			defer scResp.Body.Close()
			var result map[string]interface{}
			Expect(json.NewDecoder(scResp.Body).Decode(&result)).To(Succeed())
			Expect(result["scenario"]).To(Equal(""))

			// Request count should be zero
			rcResp, err := http.Get(server.URL + "/api/test/request-count")
			Expect(err).NotTo(HaveOccurred())
			defer rcResp.Body.Close()
			var rcResult map[string]interface{}
			Expect(json.NewDecoder(rcResp.Body).Decode(&rcResult)).To(Succeed())
			Expect(rcResult["count"]).To(BeNumerically("==", 0))
		})
	})

	Describe("IT-MOCK-570-001: Auth headers are recorded per request", func() {
		It("should record Authorization header from request", func() {
			body := chatRequest("- Signal Name: OOMKilled", nil)
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", body)
			req.Header.Set("Authorization", "Bearer test-token-123")
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			hdrResp, err := http.Get(server.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer hdrResp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(hdrResp.Body).Decode(&result)).To(Succeed())
			headers := result["headers"].(map[string]interface{})
			Expect(headers["Authorization"]).To(Equal("Bearer test-token-123"))
		})
	})

	Describe("IT-MOCK-570-002: Custom headers are recorded", func() {
		It("should record X-Custom header when configured", func() {
			body := chatRequest("- Signal Name: OOMKilled", nil)
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", body)
			req.Header.Set("X-Custom", "custom-value")
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			hdrResp, err := http.Get(server.URL + "/api/test/headers")
			Expect(err).NotTo(HaveOccurred())
			defer hdrResp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(hdrResp.Body).Decode(&result)).To(Succeed())
			headers := result["headers"].(map[string]interface{})
			Expect(headers["X-Custom"]).To(Equal("custom-value"))
		})
	})

	Describe("IT-MOCK-563-005: Request count is tracked", func() {
		It("should increment request count for each chat request", func() {
			for i := 0; i < 3; i++ {
				body := chatRequest("- Signal Name: OOMKilled", nil)
				resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
			}

			rcResp, err := http.Get(server.URL + "/api/test/request-count")
			Expect(err).NotTo(HaveOccurred())
			defer rcResp.Body.Close()

			var result map[string]interface{}
			Expect(json.NewDecoder(rcResp.Body).Decode(&result)).To(Succeed())
			Expect(result["count"]).To(BeNumerically("==", 3))
		})
	})
})
