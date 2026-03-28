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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("Prometheus Metrics Endpoint", func() {

	var server *httptest.Server

	BeforeEach(func() {
		registry := scenarios.DefaultRegistry()
		router := handlers.NewMetricsRouter(registry, false)
		server = httptest.NewServer(router)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("IT-MOCK-568-001: GET /metrics returns Prometheus text format", func() {
		It("should return 200 with text/plain content type", func() {
			resp, err := http.Get(server.URL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(200))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))
		})
	})

	Describe("IT-MOCK-568-002: Request counter increments after OpenAI request", func() {
		It("should show mock_llm_requests_total in /metrics output", func() {
			body := chatRequest("- Signal Name: OOMKilled\n- Namespace: default", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			metricsBody := getMetricsBody(server.URL)
			Expect(metricsBody).To(ContainSubstring("mock_llm_requests_total"))
			Expect(metricsBody).To(ContainSubstring(`endpoint="/v1/chat/completions"`))
		})
	})

	Describe("IT-MOCK-568-003: Response duration histogram records latency", func() {
		It("should show mock_llm_response_duration_seconds in /metrics output", func() {
			body := chatRequest("- Signal Name: OOMKilled", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			metricsBody := getMetricsBody(server.URL)
			Expect(metricsBody).To(ContainSubstring("mock_llm_response_duration_seconds"))
		})
	})

	Describe("IT-MOCK-568-004: Scenario detection counter with labels", func() {
		It("should show mock_llm_scenario_detection_total with scenario label", func() {
			body := chatRequest("- Signal Name: OOMKilled\n- Namespace: default", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			metricsBody := getMetricsBody(server.URL)
			Expect(metricsBody).To(ContainSubstring("mock_llm_scenario_detection_total"))
			Expect(metricsBody).To(ContainSubstring(`scenario="oomkilled"`))
		})
	})

	Describe("IT-MOCK-568-005: DAG phase transition counter records traversal", func() {
		It("should show mock_llm_dag_phase_transitions_total after tool-based request", func() {
			body := chatRequest(
				"- Signal Name: OOMKilled\n- Namespace: default",
				[]string{openai.ToolSearchWorkflowCatalog},
			)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			metricsBody := getMetricsBody(server.URL)
			Expect(metricsBody).To(ContainSubstring("mock_llm_dag_phase_transitions_total"))
			Expect(metricsBody).To(ContainSubstring(`from_node="dispatch"`))
		})
	})

	Describe("IT-MOCK-568-006: Reset clears metric counters", func() {
		It("should not show any request counts after reset", func() {
			body := chatRequest("- Signal Name: OOMKilled", nil)
			resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/test/reset", nil)
			resetResp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resetResp.Body.Close()

			metricsBody := getMetricsBody(server.URL)
			Expect(metricsBody).NotTo(ContainSubstring(`mock_llm_requests_total{`))
		})
	})

	Describe("IT-MOCK-568-007: Ollama request increments counter", func() {
		It("should show /api/chat endpoint in request counter", func() {
			body := ollamaChatRequest("- Signal Name: CrashLoopBackOff")
			resp, err := http.Post(server.URL+"/api/chat", "application/json", body)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			metricsBody := getMetricsBody(server.URL)
			Expect(metricsBody).To(ContainSubstring(`endpoint="/api/chat"`))
		})
	})
})

func getMetricsBody(baseURL string) string {
	resp, err := http.Get(baseURL + "/metrics")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return strings.TrimSpace(string(body))
}
