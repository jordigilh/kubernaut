// Package gateway contains integration tests for adapter interaction patterns
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// ADAPTER INTERACTION PATTERNS - INTEGRATION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Purpose: Validate all adapters integrate consistently with processing pipeline
// Coverage: BR-001 (Validation), BR-002 (Processing)
// Test Count: 3 tests
//
// Business Outcomes:
// - Prometheus adapter correctly processes alerts through full pipeline
// - K8s Event adapter correctly classifies priority and creates CRDs
// - Adapter errors are handled gracefully with proper HTTP responses
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("Adapter Interaction Patterns - Integration Tests", func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		testServer  *httptest.Server
		redisClient *RedisTestClient
		k8sClient   *K8sTestClient
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)

		// Setup test infrastructure
		redisClient = SetupRedisTestClient(ctx)
		k8sClient = SetupK8sTestClient(ctx)

		// Clean Redis state
		if redisClient != nil && redisClient.Client != nil {
			err := redisClient.Client.FlushDB(ctx).Err()
			Expect(err).ToNot(HaveOccurred())
		}

		// Create production namespace
		ns := &corev1.Namespace{}
		ns.Name = "production"
		_ = k8sClient.Client.Delete(ctx, ns)

		Eventually(func() error {
			checkNs := &corev1.Namespace{}
			return k8sClient.Client.Get(ctx, client.ObjectKey{Name: "production"}, checkNs)
		}, "10s", "100ms").Should(HaveOccurred(), "Namespace should be deleted")

		ns = &corev1.Namespace{}
		ns.Name = "production"
		ns.Labels = map[string]string{
			"environment": "production",
		}
		err := k8sClient.Client.Create(ctx, ns)
		Expect(err).ToNot(HaveOccurred())

		// Start Gateway server
		gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred())
		testServer = httptest.NewServer(gatewayServer.Handler())
	})

	AfterEach(func() {
		// Cleanup
		if testServer != nil {
			testServer.Close()
		}

		// Cleanup namespace
		if k8sClient != nil {
			ns := &corev1.Namespace{}
			ns.Name = "production"
			_ = k8sClient.Client.Delete(ctx, ns)
		}

		// Cleanup Redis
		if redisClient != nil && redisClient.Client != nil {
			_ = redisClient.Client.FlushDB(ctx)
		}

		if redisClient != nil {
			redisClient.Cleanup(ctx)
		}
		if k8sClient != nil {
			k8sClient.Cleanup(ctx)
		}
		if cancel != nil {
			cancel()
		}
	})

	Describe("BR-001: Prometheus Adapter → Deduplication → CRD Creation", func() {
		It("should process Prometheus alert through full pipeline", func() {
			// TDD GREEN: Test validates complete Prometheus adapter integration
			// Business Outcome: Prometheus alerts are processed end-to-end

			// Send Prometheus alert
			alertJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PrometheusAdapterTest",
						"severity": "critical",
						"namespace": "production",
						"pod": "test-pod-123"
					},
					"annotations": {
						"summary": "Test alert for Prometheus adapter integration",
						"description": "This validates the full pipeline"
					}
				}]
			}`

			// First request: Should create CRD
			resp1, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(alertJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp1.Body.Close()

			Expect(resp1.StatusCode).To(Equal(http.StatusCreated),
				"First request should create CRD")

			var response1 map[string]interface{}
			err = json.NewDecoder(resp1.Body).Decode(&response1)
			Expect(err).ToNot(HaveOccurred())

			Expect(response1).To(HaveKey("remediationRequestName"), "Response should include CRD name")
			Expect(response1).To(HaveKey("remediationRequestNamespace"), "Response should include namespace")
			Expect(response1["status"]).To(Equal("created"), "Status should be 'created'")

			// Second request: Should be deduplicated
			resp2, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(alertJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp2.Body.Close()

			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted),
				"Second request should be deduplicated")

			var response2 map[string]interface{}
			err = json.NewDecoder(resp2.Body).Decode(&response2)
			Expect(err).ToNot(HaveOccurred())

			Expect(response2["status"]).To(Equal("duplicate"), "Status should be 'duplicate'")
			Expect(response2).To(HaveKey("duplicate"), "Response should indicate duplicate")
		})
	})

	Describe("BR-002: K8s Event Adapter → Priority Classification → CRD Creation", func() {
		It("should process K8s Event through priority classification", func() {
			// TDD GREEN: Test validates K8s Event adapter integration with priority classification
			// Business Outcome: K8s Events are classified correctly and create appropriate CRDs

			// Send K8s Warning event
			eventJSON := `{
				"type": "Warning",
				"reason": "BackOff",
				"message": "Back-off restarting failed container",
				"involvedObject": {
					"kind": "Pod",
					"name": "failing-pod-456",
					"namespace": "production"
				},
				"metadata": {
					"name": "failing-pod-456.event123",
					"namespace": "production"
				}
			}`

			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/kubernetes-event", testServer.URL),
				"application/json",
				strings.NewReader(eventJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"K8s Event should create CRD")

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			Expect(response).To(HaveKey("remediationRequestName"), "Response should include CRD name")
			Expect(response).To(HaveKey("priority"), "Response should include priority")
			Expect(response["priority"]).To(Equal("P1"), "Production + Warning should be P1 (per priority.go:96)")
			Expect(response["status"]).To(Equal("created"), "Status should be 'created'")
		})
	})

	Describe("BR-001: Adapter Error Handling → HTTP Error Response", func() {
		It("should return appropriate HTTP error for adapter parse failures", func() {
			// TDD GREEN: Test validates adapter error handling
			// Business Outcome: Adapter errors are communicated clearly to operators

			// Send malformed Prometheus alert (invalid JSON)
			malformedJSON := `{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "MalformedTest"
						// Missing closing braces
			}`

			resp, err := http.Post(
				fmt.Sprintf("%s/api/v1/signals/prometheus", testServer.URL),
				"application/json",
				strings.NewReader(malformedJSON),
			)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
				"Malformed JSON should return 400 Bad Request")

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).ToNot(HaveOccurred())

			Expect(errorResponse).To(HaveKey("error"), "Error response should include error field")
			Expect(errorResponse["status"]).To(BeEquivalentTo(http.StatusBadRequest))

			errorMsg := errorResponse["error"].(string)
			Expect(errorMsg).To(Or(
				ContainSubstring("JSON"),
				ContainSubstring("parse"),
				ContainSubstring("invalid"),
			), "Error message should indicate JSON parsing issue")
		})
	})
})

