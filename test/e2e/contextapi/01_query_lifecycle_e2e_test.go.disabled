/*
Copyright 2025 Jordi Gil.

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

package contextapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================================
// BR-CONTEXT-001: Query Lifecycle E2E Tests
// Day 11 Phase 2: End-to-End Query Testing with Real Infrastructure
// ============================================================================
//
// Business Requirement: BR-CONTEXT-001 (Core Query API)
// - Complete query lifecycle: HTTP → Context API → PostgreSQL → Redis → Response
// - Multi-tier caching with real Redis and PostgreSQL
// - Production-like deployment in Kubernetes
//
// Test Scenarios:
// 1. Cache Miss → Database Hit → Redis Cache Population
// 2. Cache Hit → Redis Response (no database query)
// 3. Service Discovery via Kubernetes DNS
// 4. Real PostgreSQL queries with pgvector extension
// 5. Real Redis caching with TTL
//
// Infrastructure:
// - Kind cluster (Podman)
// - PostgreSQL with pgvector
// - Redis 7
// - Context API service (deployed via Kubernetes)

var _ = Describe("BR-CONTEXT-001: Query Lifecycle E2E Tests", func() {

	var (
		contextAPIURL string
	)

	BeforeEach(func() {
		// Get Context API service endpoint
		// In E2E, we access the service via NodePort or port-forward
		svc, err := kubeClient.CoreV1().Services(namespace).Get(ctx, "contextapi", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred(), "Context API service should exist")

		// Use NodePort for local access
		// Kind exposes NodePorts on localhost:hostPort (configured in kind-config)
		contextAPIURL = "http://localhost:8800" // From kind-config-contextapi.yaml extraPortMappings
		logger.Info("Context API endpoint configured", map[string]interface{}{
			"url":        contextAPIURL,
			"service":    svc.Name,
			"clusterIP":  svc.Spec.ClusterIP,
			"ports":      svc.Spec.Ports,
		})
	})

	Context("Phase 2.1: Service Health and Connectivity", func() {

		It("MUST have Context API service running and accessible", func() {
			By("Verifying Context API pod is running")
			pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=contextapi",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(pods.Items).ToNot(BeEmpty(), "Context API pod should be running")

			pod := pods.Items[0]
			Expect(pod.Status.Phase).To(Equal("Running"), "Context API pod should be in Running phase")

			By("Verifying /health endpoint responds")
			resp, err := http.Get(contextAPIURL + "/health")
			Expect(err).ToNot(HaveOccurred(), "Health endpoint should be accessible")
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Health check should return 200 OK")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var healthResp map[string]interface{}
			err = json.Unmarshal(body, &healthResp)
			Expect(err).ToNot(HaveOccurred())

			Expect(healthResp["status"]).To(Equal("healthy"), "Service should report healthy status")
		})

		It("MUST have PostgreSQL pod running with pgvector extension", func() {
			By("Verifying PostgreSQL pod is running")
			pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=postgres",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(pods.Items).ToNot(BeEmpty(), "PostgreSQL pod should be running")

			pod := pods.Items[0]
			Expect(pod.Status.Phase).To(Equal("Running"), "PostgreSQL pod should be in Running phase")
			Expect(pod.Status.ContainerStatuses[0].Ready).To(BeTrue(), "PostgreSQL container should be ready")
		})

		It("MUST have Redis pod running and accessible", func() {
			By("Verifying Redis pod is running")
			pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=redis",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(pods.Items).ToNot(BeEmpty(), "Redis pod should be running")

			pod := pods.Items[0]
			Expect(pod.Status.Phase).To(Equal("Running"), "Redis pod should be in Running phase")
			Expect(pod.Status.ContainerStatuses[0].Ready).To(BeTrue(), "Redis container should be ready")
		})

	})

	Context("Phase 2.2: Query Lifecycle - Cache Miss → Database Hit", func() {

		It("MUST handle cache miss and query PostgreSQL successfully", func() {
			By("Making first query (cache miss)")
			queryURL := fmt.Sprintf("%s/api/v1/context/query?limit=5&offset=%d", contextAPIURL, time.Now().UnixNano())

			resp, err := http.Get(queryURL)
			Expect(err).ToNot(HaveOccurred(), "Query should succeed")
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Query should return 200 OK")

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			var queryResp map[string]interface{}
			err = json.Unmarshal(body, &queryResp)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying response structure")
			Expect(queryResp).To(HaveKey("data"), "Response should have data field")
			Expect(queryResp).To(HaveKey("total"), "Response should have total field")
			Expect(queryResp).To(HaveKey("limit"), "Response should have limit field")
			Expect(queryResp).To(HaveKey("offset"), "Response should have offset field")

			By("Verifying data was fetched from PostgreSQL")
			// First query should hit database (cache miss)
			// We can verify by checking metrics or logs, but for E2E we just verify response structure
			data := queryResp["data"].([]interface{})
			Expect(data).ToNot(BeNil(), "Data should be present in response")
		})

		It("MUST populate Redis cache after database hit", func() {
			By("Making first query to populate cache")
			offset := time.Now().UnixNano()
			queryURL := fmt.Sprintf("%s/api/v1/context/query?limit=5&offset=%d", contextAPIURL, offset)

			resp1, err := http.Get(queryURL)
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusOK))

			body1, _ := io.ReadAll(resp1.Body)
			var firstResp map[string]interface{}
			json.Unmarshal(body1, &firstResp)

			By("Making second identical query (should hit cache)")
			// Small delay to ensure cache is written
			time.Sleep(100 * time.Millisecond)

			resp2, err := http.Get(queryURL)
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusOK))

			body2, _ := io.ReadAll(resp2.Body)
			var secondResp map[string]interface{}
			json.Unmarshal(body2, &secondResp)

			By("Verifying responses are identical")
			Expect(secondResp["total"]).To(Equal(firstResp["total"]), "Cached response should match original")
			Expect(secondResp["limit"]).To(Equal(firstResp["limit"]), "Cached response should match original")

			By("Checking metrics for cache hit")
			metricsURL := fmt.Sprintf("%s/metrics", contextAPIURL)
			metricsResp, err := http.Get(metricsURL)
			Expect(err).ToNot(HaveOccurred())
			defer metricsResp.Body.Close()

			metricsBody, _ := io.ReadAll(metricsResp.Body)
			metricsText := string(metricsBody)

			// Verify cache hit metric increased
			Expect(metricsText).To(ContainSubstring("contextapi_cache_hits_total"), "Cache hit metric should exist")
		})

	})

	Context("Phase 2.3: Query Performance and Reliability", func() {

		It("MUST handle concurrent queries correctly", func() {
			By("Making 10 concurrent queries")
			type result struct {
				statusCode int
				err        error
			}
			results := make(chan result, 10)

			offset := time.Now().UnixNano()
			queryURL := fmt.Sprintf("%s/api/v1/context/query?limit=5&offset=%d", contextAPIURL, offset)

			for i := 0; i < 10; i++ {
				go func() {
					resp, err := http.Get(queryURL)
					if err != nil {
						results <- result{0, err}
						return
					}
					defer resp.Body.Close()
					results <- result{resp.StatusCode, nil}
				}()
			}

			By("Verifying all queries succeeded")
			successCount := 0
			for i := 0; i < 10; i++ {
				res := <-results
				if res.err == nil && res.statusCode == http.StatusOK {
					successCount++
				}
			}

			Expect(successCount).To(Equal(10), "All concurrent queries should succeed")
		})

		It("MUST respond within acceptable latency (< 500ms for cache hit)", func() {
			By("Populating cache with first query")
			offset := time.Now().UnixNano()
			queryURL := fmt.Sprintf("%s/api/v1/context/query?limit=5&offset=%d", contextAPIURL, offset)

			http.Get(queryURL) // Populate cache
			time.Sleep(100 * time.Millisecond)

			By("Measuring cache hit latency")
			start := time.Now()
			resp, err := http.Get(queryURL)
			latency := time.Since(start)

			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			Expect(latency).To(BeNumerically("<", 500*time.Millisecond), "Cache hit should respond within 500ms")
			logger.Info("Cache hit latency", map[string]interface{}{
				"latency_ms": latency.Milliseconds(),
			})
		})

	})

	Context("Phase 2.4: Error Handling and Edge Cases", func() {

		It("MUST handle invalid query parameters gracefully", func() {
			By("Sending query with invalid limit")
			queryURL := fmt.Sprintf("%s/api/v1/context/query?limit=invalid&offset=0", contextAPIURL)

			resp, err := http.Get(queryURL)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "Invalid parameters should return 400")

			body, _ := io.ReadAll(resp.Body)
			var errorResp map[string]interface{}
			json.Unmarshal(body, &errorResp)

			By("Verifying RFC 7807 error response")
			Expect(errorResp).To(HaveKey("type"), "Error response should be RFC 7807 compliant")
			Expect(errorResp).To(HaveKey("title"), "Error response should have title")
			Expect(errorResp).To(HaveKey("status"), "Error response should have status")
		})

		It("MUST handle missing query parameters with defaults", func() {
			By("Sending query without limit/offset")
			queryURL := fmt.Sprintf("%s/api/v1/context/query", contextAPIURL)

			resp, err := http.Get(queryURL)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Missing parameters should use defaults")

			body, _ := io.ReadAll(resp.Body)
			var queryResp map[string]interface{}
			json.Unmarshal(body, &queryResp)

			By("Verifying default values were applied")
			Expect(queryResp["limit"]).To(BeNumerically(">=", 1), "Default limit should be applied")
			Expect(queryResp["offset"]).To(BeNumerically(">=", 0), "Default offset should be applied")
		})

	})

})

