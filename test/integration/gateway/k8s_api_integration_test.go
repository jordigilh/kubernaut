// Package gateway contains integration tests for Gateway Service Kubernetes API integration
package gateway

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// DAY 8 PHASE 3: KUBERNETES API INTEGRATION TESTS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Purpose: Validate Gateway behavior with Kubernetes API operations
// Coverage: BR-GATEWAY-015 (CRD creation), BR-GATEWAY-018 (K8s resilience)
// Test Count: 11 tests (6 original + 5 edge cases)
//
// Business Outcomes Validated:
// 1. CRD creation succeeds under normal conditions
// 2. K8s API rate limiting handled gracefully
// 3. CRD name collisions resolved correctly
// 4. K8s API failures don't corrupt state
// 5. Watch connection interruptions handled
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("DAY 8 PHASE 3: Kubernetes API Integration Tests", func() {
	var (
		ctx                context.Context
		cancel             context.CancelFunc
		testServer         *httptest.Server
		redisClient        *RedisTestClient
		k8sClient          *K8sTestClient
		testNamespaceProd  string
		testNamespaceStage string
		testNamespaceDev   string
		testCounter        int
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)

		// Generate unique namespaces for test isolation
		testCounter++
		baseTimestamp := time.Now().UnixNano()
		baseSeed := GinkgoRandomSeed()
		testNamespaceProd = fmt.Sprintf("test-k8s-prod-p%d-%d-%d-%d", GinkgoParallelProcess(), baseTimestamp, baseSeed, testCounter)
		testNamespaceStage = fmt.Sprintf("test-k8s-stage-%d-%d-%d", baseTimestamp, baseSeed, testCounter)
		testNamespaceDev = fmt.Sprintf("test-k8s-dev-%d-%d-%d", baseTimestamp, baseSeed, testCounter)

		// Setup test infrastructure
		redisClient = SetupRedisTestClient(ctx)
		k8sClient = SetupK8sTestClient(ctx)

		// Clean Redis state (safe - each process uses different Redis DB)
		if redisClient != nil && redisClient.Client != nil {
			err := redisClient.Client.FlushDB(ctx).Err()
			Expect(err).ToNot(HaveOccurred(), "Should clean Redis before test")
		}

		// Create test namespaces with environment labels for classification
		// This is required for environment-based priority assignment
		testNamespaces := []struct {
			name  string
			label string
		}{
			{testNamespaceProd, "production"},
			{testNamespaceStage, "staging"},
			{testNamespaceDev, "development"},
		}

		for _, ns := range testNamespaces {
			// Use helper to ensure namespace exists with proper labels
			namespace := &corev1.Namespace{}
			namespace.Name = ns.name

			// Wait for deletion to complete (namespace deletion is asynchronous)
			Eventually(func() error {
				checkNs := &corev1.Namespace{}
				return k8sClient.Client.Get(ctx, client.ObjectKey{Name: ns.name}, checkNs)
			}, "10s", "100ms").Should(HaveOccurred(), fmt.Sprintf("%s namespace should be deleted", ns.name))

			// Recreate with correct label
			namespace = &corev1.Namespace{}
			namespace.Name = ns.name
			namespace.Labels = map[string]string{
				"environment": ns.label, // Required for EnvironmentClassifier
			}
			err := k8sClient.Client.Create(ctx, namespace)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Should create %s namespace with environment label", ns.name))
		}

		// Start Gateway server
		gatewayServer, err := StartTestGateway(ctx, redisClient, k8sClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Gateway server")
		testServer = httptest.NewServer(gatewayServer.Handler())
	})

	AfterEach(func() {
		// Reset Redis config to prevent OOM cascade failures
		if redisClient != nil && redisClient.Client != nil {
			redisClient.Client.ConfigSet(ctx, "maxmemory", "2147483648")
			redisClient.Client.ConfigSet(ctx, "maxmemory-policy", "allkeys-lru")
		}

		// CRITICAL FIX: Don't delete namespaces during parallel test execution
		// Let Kind cluster deletion handle cleanup at the end of the test suite
		// Previous code (REMOVED):
		// testNamespaces := []string{testNamespaceProd, testNamespaceStage, testNamespaceDev}
		// for _, nsName := range testNamespaces {
		//     ns := &corev1.Namespace{}
		//     ns.Name = nsName
		//     _ = k8sClient.Client.Delete(ctx, ns)
		// }

		// Cleanup
		if testServer != nil {
			testServer.Close()
		}
		redisClient.Cleanup(ctx)
		k8sClient.Cleanup(ctx)
		cancel()
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// ORIGINAL TESTS (1-6): Basic K8s API Integration
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Basic K8s API Integration", func() {
		It("should create RemediationRequest CRD successfully", func() {
			// BR-GATEWAY-015: CRD creation
			// BUSINESS OUTCOME: Alert converted to CRD

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "CRDCreationTest",
				Namespace: testNamespaceProd,
			})

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp.StatusCode).To(Equal(201))

			// BUSINESS OUTCOME: CRD exists in Kubernetes
			crds := ListRemediationRequests(ctx, k8sClient, testNamespaceProd)
			Expect(crds).To(HaveLen(1))
			Expect(crds[0].Spec.SignalName).To(Equal("CRDCreationTest"))
		})

		It("should populate CRD with correct metadata", func() {
			// BR-GATEWAY-015: CRD metadata correctness
			// BUSINESS OUTCOME: CRD contains all required fields

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "MetadataTest",
				Namespace: testNamespaceProd,
				Severity:  "critical",
			})

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp.StatusCode).To(Equal(201))

			// Verify CRD metadata
			crds := ListRemediationRequests(ctx, k8sClient, testNamespaceProd)
			Expect(crds).To(HaveLen(1))

			crd := crds[0]
			Expect(crd.Spec.SignalName).To(Equal("MetadataTest"))
			Expect(crd.Spec.Severity).To(Equal("critical"))
			Expect(crd.Spec.SignalType).To(Equal("prometheus-alert")) // Prometheus adapter sets SourceType to "prometheus-alert"
			Expect(crd.Spec.TargetType).To(Equal("kubernetes"))
			Expect(crd.Spec.SignalFingerprint).NotTo(BeEmpty())
		})

		// REMOVED: "should handle K8s API rate limiting"
		// Reason: Load test (100 concurrent requests), not integration test
		// See: test/integration/gateway/PENDING_TESTS_ANALYSIS.md
		// If needed, implement in test/load/gateway/k8s_api_load_test.go

		It("should handle CRD name collisions", func() {
			// BR-GATEWAY-015: CRD name uniqueness
			// BUSINESS OUTCOME: Each alert gets unique CRD name

			// Send 2 alerts with same name but different namespaces
			payload1 := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "CollisionTest",
				Namespace: testNamespaceProd,
			})
			payload2 := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "CollisionTest",
				Namespace: testNamespaceStage,
			})

			resp1 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload1)
			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload2)

			Expect(resp1.StatusCode).To(Equal(201))
			Expect(resp2.StatusCode).To(Equal(201))

			// BUSINESS OUTCOME: Both CRDs created with unique names
			prodCRDs := ListRemediationRequests(ctx, k8sClient, testNamespaceProd)
			stagingCRDs := ListRemediationRequests(ctx, k8sClient, testNamespaceStage)

			Expect(prodCRDs).To(HaveLen(1))
			Expect(stagingCRDs).To(HaveLen(1))
			Expect(prodCRDs[0].Name).NotTo(Equal(stagingCRDs[0].Name))
		})

		It("should validate CRD schema before creation", func() {
			// BR-GATEWAY-015: Schema validation
			// BUSINESS OUTCOME: Invalid CRDs rejected before API call

			// Create invalid payload (missing required field)
			invalidPayload := []byte(`{
				"version": "4",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"namespace": testNamespaceProd
					}
				}]
			}`)

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", invalidPayload)

			// BUSINESS OUTCOME: Invalid payload rejected
			Expect(resp.StatusCode).To(Equal(400))

			// Verify: No CRD created
			crds := ListRemediationRequests(ctx, k8sClient, testNamespaceProd)
			Expect(crds).To(HaveLen(0))
		})

		It("should handle K8s API temporary failures with retry", func() {
			// BR-GATEWAY-018: K8s API resilience with retry
			// BUSINESS OUTCOME: Transient failures don't lose alerts

			// Simulate K8s API unavailability
			k8sClient.SimulateTemporaryFailure(ctx, 3*time.Second)

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "RetryTest",
				Namespace: testNamespaceProd,
			})

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS OUTCOME: Request may fail initially but retries succeed
			// Either succeeds immediately (201) or fails and retries (500 → 201)
			if resp.StatusCode == 500 {
				// Wait for retry (2s is sufficient for test retry logic)
				time.Sleep(2 * time.Second)
			}

			// Eventually, CRD should be created
			Eventually(func() int {
				return len(ListRemediationRequests(ctx, k8sClient, testNamespaceProd))
			}, "10s", "1s").Should(Equal(1))
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// EDGE CASE TESTS (7-11): Advanced K8s API Scenarios
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("Edge Cases: Advanced K8s API Scenarios", func() {
		It("should handle K8s API quota exceeded gracefully", func() {
			// EDGE CASE: K8s API quota exhausted
			// BUSINESS OUTCOME: Requests queued and processed when quota available
			// Production Risk: Quota limits in multi-tenant clusters
			// Integration Test: Validates quota behavior with realistic load (10 requests)
			// Note: Full load test (50+ requests) belongs in test/load/gateway/

			// Create realistic production load (parallelized for speed)
			// Production simulation: 10 alerts arriving within seconds
			var wg sync.WaitGroup
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					defer GinkgoRecover()

					payload := GeneratePrometheusAlert(PrometheusAlertOptions{
						AlertName: fmt.Sprintf("QuotaTest-%d", index),
						Namespace: testNamespaceProd,
					})

					SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
				}(i)
			}

			// Wait for all requests to complete
			wg.Wait()

			// BUSINESS OUTCOME: Requests processed successfully
			// Note: Some may be deduplicated or storm-aggregated (expected behavior)
			Eventually(func() int {
				count := len(ListRemediationRequests(ctx, k8sClient, testNamespaceProd))
				GinkgoWriter.Printf("Found %d CRDs in namespace %s (waiting for >= 5)\n", count, testNamespaceProd)
				return count
			}, "120s", "2s").Should(BeNumerically(">=", 5),
				"At least 5 CRDs should be created (deduplication/storm aggregation may reduce count) - 120s timeout for parallel execution")
		})

		// REMOVED: "should handle CRD name length limit (253 chars)"
		// Reason: Converted to unit test for name generation logic
		// See: test/unit/gateway/crd_name_generation_test.go (to be created)
		// See: test/integration/gateway/PENDING_TESTS_ANALYSIS.md

		It("should handle watch connection interruption", func() {
			// EDGE CASE: K8s watch connection drops mid-operation
			// BUSINESS OUTCOME: Watch reconnects automatically
			// Production Risk: Network issues, API server restart

			// Send alert
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "WatchTest",
				Namespace: testNamespaceProd,
			})

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp.StatusCode).To(Equal(201))

			// Simulate watch connection interruption
			k8sClient.InterruptWatchConnection(ctx)

			// Send another alert after interruption
			payload2 := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "WatchTest2",
				Namespace: testNamespaceProd,
			})

			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload2)

			// BUSINESS OUTCOME: Second alert processed after watch reconnect
			Expect(resp2.StatusCode).To(Equal(201))

			// Verify: Both CRDs exist
			Eventually(func() int {
				return len(ListRemediationRequests(ctx, k8sClient, testNamespaceProd))
			}, "10s", "1s").Should(Equal(2))
		})

		// REMOVED: "should handle K8s API slow responses without timeout"
		// Reason: Requires chaos engineering infrastructure (infrastructure simulation)
		// See: test/integration/gateway/PENDING_TESTS_ANALYSIS.md
		// If needed, implement in E2E chaos testing tier

		// REMOVED: "should handle concurrent CRD creates to same namespace"
		// Reason: Load test (50 concurrent requests), not integration test
		// See: test/integration/gateway/PENDING_TESTS_ANALYSIS.md
		// If needed, implement in test/load/gateway/k8s_api_load_test.go
	})
})
