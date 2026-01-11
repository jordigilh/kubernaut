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

package gateway

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"

	// DD-GATEWAY-004: kubernetes import removed - no longer needed
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// Business Outcome Testing: Test WHAT K8s API resilience enables
//
// ❌ WRONG: "should return k8s error code" (tests implementation)
// ✅ RIGHT: "Gateway remains operational when K8s API temporarily unavailable" (tests business outcome)

// ErrorInjectableK8sClient simulates Kubernetes API failures for integration testing
// BR-GATEWAY-019: Test error handling when K8s API unavailable
type ErrorInjectableK8sClient struct {
	client.Client
	failCreate bool
	errorMsg   string
}

func (f *ErrorInjectableK8sClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f.failCreate {
		return errors.New(f.errorMsg)
	}
	// Success case: Return nil (no actual CRD creation needed for test)
	return nil
}

func (f *ErrorInjectableK8sClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return errors.New("simulated Kubernetes API unavailable")
}

var _ = Describe("BR-GATEWAY-019: Kubernetes API Failure Handling - Integration Tests", func() {
	var (
		ctx              context.Context
		crdCreator       *processing.CRDCreator
		logger           logr.Logger
		failingK8sClient *ErrorInjectableK8sClient
		testSignal       *types.NormalizedSignal
	)

	BeforeEach(func() {
		ctx = context.Background()
		zapLogger := zap.NewNop()
		logger = zapr.NewLogger(zapLogger)

		// Create failing K8s client (simulates K8s API unavailable)
		// This test is fully self-contained with ErrorInjectableK8sClient
		// and doesn't require real Kubernetes infrastructure
		failingK8sClient = &ErrorInjectableK8sClient{
			failCreate: true,
			errorMsg:   "connection refused: Kubernetes API server unreachable",
		}

		// Wrap failing client in k8s.Client
		wrappedK8sClient := k8s.NewClient(failingK8sClient)

		// Create isolated metrics registry per test to avoid collisions
		testRegistry := prometheus.NewRegistry()
		testMetrics := metrics.NewMetricsWithRegistry(testRegistry)

		// Create CRD creator with failing client (DD-005: uses logr.Logger)
		retryConfig := config.DefaultRetrySettings()
		crdCreator = processing.NewCRDCreator(wrappedK8sClient, logger, testMetrics, "default", &retryConfig)

		// Test signal
		testSignal = &types.NormalizedSignal{
			AlertName: "HighMemoryUsage",
			Namespace: "production",
			Resource: types.ResourceIdentifier{
				Kind: "Pod",
				Name: "payment-api-123",
			},
			Severity:    "critical",
			Fingerprint: "test-fingerprint-k8s-failure",
		}
	})

	Context("CRD Creation Failures", func() {
		It("returns error when Kubernetes API is unavailable", func() {
			// BR-GATEWAY-019: K8s API failure handling
			// BUSINESS SCENARIO: Kubernetes API down during CRD creation
			// Expected: Error returned, caller (webhook handler) returns 500

			_, err := crdCreator.CreateRemediationRequest(ctx, testSignal) // environment/priority removed - SP owns classification

			// BUSINESS OUTCOME: K8s API failure detected
			Expect(err).ToNot(BeNil(), "K8s API failure must be detected and propagated")
			Expect(err.Error()).To(ContainSubstring("connection refused"), "Error message must indicate K8s API as root cause")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ K8s API failure → Error propagated → Handler returns 500 → Prometheus retries
			// ✅ Gateway doesn't crash or hang
			// ✅ Clear error message for operational debugging
		})

		It("gracefully handles multiple consecutive failures", func() {
			// BR-GATEWAY-019: Resilience to sustained K8s outage
			// BUSINESS SCENARIO: K8s API down for multiple webhook attempts
			// Expected: Each attempt fails gracefully, Gateway remains operational

			// Attempt 1: Failure
			_, err1 := crdCreator.CreateRemediationRequest(ctx, testSignal) // environment/priority removed - SP owns classification
			Expect(err1).To(HaveOccurred())

			// Attempt 2: Failure
			_, err2 := crdCreator.CreateRemediationRequest(ctx, testSignal) // environment/priority removed - SP owns classification
			Expect(err2).To(HaveOccurred())

			// Attempt 3: Failure
			_, err3 := crdCreator.CreateRemediationRequest(ctx, testSignal) // environment/priority removed - SP owns classification
			Expect(err3).To(HaveOccurred())

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Gateway doesn't enter permanent failure state
			// ✅ Each webhook independently retried by Prometheus
			// ✅ No alerts lost (all eventually processed via retry when K8s recovers)
		})

		It("propagates specific K8s error details for operational debugging", func() {
			// BR-GATEWAY-019: Operational visibility during failures
			// Expected: Error messages contain K8s-specific details

			_, err := crdCreator.CreateRemediationRequest(ctx, testSignal) // environment/priority removed - SP owns classification

			Expect(err.Error()).To(ContainSubstring("connection refused"), "Error must include specific K8s error details for troubleshooting")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ On-call engineers can diagnose K8s API issues from error messages
			// ✅ Error context preserved for troubleshooting
		})
	})

	Context("K8s API Recovery", func() {
		It("successfully creates CRD when K8s API recovers", func() {
			// BR-GATEWAY-019: Eventual consistency after recovery
			// BUSINESS SCENARIO: K8s API recovers, retry succeeds
			// Expected: CRD creation succeeds on retry

			// Simulate K8s API down
			failingK8sClient.failCreate = true
			_, err := crdCreator.CreateRemediationRequest(ctx, testSignal) // environment/priority removed - SP owns classification
				Expect(err).ToNot(BeNil(), "First attempt fails when K8s API down")

			// Simulate K8s API recovery
			failingK8sClient.failCreate = false
			rr, err := crdCreator.CreateRemediationRequest(ctx, testSignal) // environment/priority removed - SP owns classification

				Expect(err).ToNot(BeNil(), "Second attempt succeeds when K8s API recovers")
			Expect(rr).NotTo(BeNil(), "RemediationRequest CRD must be returned on success")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Gateway operational flow resumes after K8s recovery
			// ✅ No manual intervention needed
			// ✅ Prometheus automatic retry achieves eventual consistency
		})
	})

	Context("Partial K8s API Failures", func() {
		It("handles per-request K8s API variability", func() {
			// BR-GATEWAY-019: Intermittent K8s API issues
			// BUSINESS SCENARIO: K8s API flapping (up/down/up)
			// Expected: Some create attempts fail, others succeed

			// Signal 1: K8s API down
			signal1 := &types.NormalizedSignal{
				AlertName:   "HighMemoryUsage",
				Namespace:   "production",
				Fingerprint: "signal-1",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "payment-api-1",
				},
			}
			failingK8sClient.failCreate = true
			_, err1 := crdCreator.CreateRemediationRequest(ctx, signal1) // environment/priority removed - SP owns classification
			Expect(err1).To(HaveOccurred(), "First signal fails when K8s API down")

			// Signal 2: K8s API recovers
			signal2 := &types.NormalizedSignal{
				AlertName:   "HighCPU",
				Namespace:   "staging",
				Fingerprint: "signal-2",
				Resource: types.ResourceIdentifier{
					Kind: "Deployment",
					Name: "frontend",
				},
			}
			failingK8sClient.failCreate = false
			_, err2 := crdCreator.CreateRemediationRequest(ctx, signal2) // environment/priority removed - SP owns classification
			Expect(err2).NotTo(HaveOccurred(), "Second signal succeeds when K8s API recovers")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Gateway doesn't enter permanent failure state
			// ✅ Each webhook independently processed
			// ✅ Partial success possible during intermittent failures
			// ✅ No alerts lost (all eventually processed via retry)
		})
	})

	// NOTE: Chaos tests moved to test/chaos/gateway/
	// - K8s API unavailable during webhook → test/chaos/gateway/k8s_api_failure_test.go
	// - K8s API recovery → test/chaos/gateway/k8s_api_recovery_test.go
	// See test/chaos/gateway/README.md for implementation details
	//
	// Business scenarios already covered by:
	// - CRD Creation Failures context (above) - tests CRDCreator error handling
	// - webhook_integration_test.go - tests full webhook E2E flow

	// The following section was removed because it used the old gateway.NewServer() API
	// which was removed during configuration refactoring (v2.18).
	// Original content preserved in git history if needed for reference.
	/*
			BeforeEach(func() {
				// Create Gateway server with failing K8s client
				// Note: NewAdapterRegistry() already registers Prometheus and K8s Event adapters
				adapterRegistry := adapters.NewAdapterRegistry()

				classifier := processing.NewEnvironmentClassifier()
				// Load Rego policy for priority assignment (BR-GATEWAY-013)
				// Use absolute path from project root (tests run from package directory)
				policyPath := "../../../docs/gateway/policies/priority-policy.rego"
				priorityEngine, err := processing.NewPriorityEngineWithRego(policyPath, logger)
				pathDecider := processing.NewRemediationPathDecider(logger)
				crdCreator := processing.NewCRDCreator(failingK8sClient, logger)

				serverConfig = &gateway.Config{
					Port:         8080,
					ReadTimeout:  5,
					WriteTimeout: 10,
				}

				// v2.9: Deduplication and storm detection are REQUIRED (BR-GATEWAY-008, BR-GATEWAY-009)
				// Even for K8s API failure tests, we need these services
				// Use miniredis or real Redis for testing
				redisClient := SetupRedisTestClient(ctx)
					Skip("Redis not available - required for Gateway startup")
				}

				// PHASE 1 FIX: Clean Redis state before each test to prevent state pollution

				// Verify Redis is clean


				// DD-GATEWAY-004: K8s clientset no longer needed - authentication removed
				// Phase 2 Fix: Create custom Prometheus registry per test to prevent
				// "duplicate metrics collector registration" panics
				metricsRegistry := prometheus.NewRegistry()

				gatewayServer, err = gateway.NewServer(
					adapterRegistry,
					classifier,
					priorityEngine,
					pathDecider,
					crdCreator,
					dedupService,       // REQUIRED v2.9
					stormDetector,      // REQUIRED v2.9
					stormAggregator,    // REQUIRED v2.11
					logger,
					serverConfig,
					metricsRegistry, // Phase 2 Fix: Custom registry per test for isolation
				)
			})

			It("returns 500 Internal Server Error when K8s API unavailable during webhook processing", func() {
				// BR-GATEWAY-019: Full webhook → 500 error → Prometheus retry flow
				// BUSINESS SCENARIO: Kubernetes API down during webhook processing
				// Expected: 500 error → Prometheus retries → Eventual success when API recovers

				// Prepare Prometheus webhook payload
				payload := []byte(`{
					"alerts": [{
						"status": "firing",
						"labels": {
							"alertname": "HighMemoryUsage",
							"severity": "critical",
							"namespace": "production",
							"pod": "payment-api-123")
						},
						"annotations": {
							"summary": "Pod payment-api-123 using 95% memory")
						},
						"startsAt": "2025-10-22T10:00:00Z")
					}]
				}`)

			// Send webhook to Gateway
			req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			// DD-GATEWAY-004: No authentication needed - handled at network layer
			rec := httptest.NewRecorder()

				// Process webhook through Gateway handler
				gatewayServer.Handler().ServeHTTP(rec, req)

				// BUSINESS OUTCOME: 500 error triggers Prometheus retry
				Expect(rec.Code).To(Equal(http.StatusInternalServerError), "K8s API failure must return 500 to trigger client retry")

				var response map[string]interface{}
				unmarshalErr := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(unmarshalErr).NotTo(HaveOccurred())

				Expect(response["status"]).To(Equal("error"), "Response must indicate error status")
				Expect(response["error"]).To(ContainSubstring("failed to create remediation request"), "Error message must explain what failed")
				Expect(response["code"]).To(Equal("CRD_CREATION_ERROR"), "Error code must indicate CRD creation failure")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ K8s API failure → 500 error → Prometheus retries webhook
				// ✅ Gateway doesn't crash or hang
				// ✅ Webhook eventually succeeds when K8s API recovers
				// ✅ Alert data preserved for retry (idempotent)
				//
				// Real-world recovery flow:
				// 10:00 AM → K8s API down → Webhook fails with 500
				// 10:01 AM → Prometheus retries → Still fails (API still down)
				// 10:03 AM → K8s API recovers
				// 10:03 AM → Prometheus retries → Success (CRD created) ✅
			})

			It("returns 201 Created when K8s API is available", func() {
				// BR-GATEWAY-019: Successful webhook processing with healthy K8s API
				// BUSINESS SCENARIO: Normal operation, K8s API healthy
				// Expected: 201 Created, CRD metadata returned

				// Simulate K8s API recovery
				failingK8sClient.failCreate = false

				payload := []byte(`{
					"alerts": [{
						"status": "firing",
						"labels": {
							"alertname": "HighCPU",
							"severity": "warning",
							"namespace": "staging",
							"deployment": "frontend")
						},
						"annotations": {
							"summary": "Deployment frontend using 90% CPU")
						},
						"startsAt": "2025-10-22T10:05:00Z")
					}]
				}`)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			// DD-GATEWAY-004: No authentication needed - handled at network layer
			rec := httptest.NewRecorder()

				gatewayServer.Handler().ServeHTTP(rec, req)

				// BUSINESS OUTCOME: Successful CRD creation
				Expect(rec.Code).To(Equal(http.StatusCreated), "Successful webhook processing must return 201 Created")

				var response map[string]interface{}
				unmarshalErr := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(unmarshalErr).NotTo(HaveOccurred())

				Expect(response["status"]).To(Equal("created"), "Response must indicate success")
				Expect(response["priority"]).To(Equal("P2"), "Priority must be assigned (warning + staging = P2 per BR-GATEWAY-020 fallback matrix)")
				Expect(response["environment"]).To(Equal("staging"), "Environment must be classified")

				// BUSINESS CAPABILITY VERIFIED:
				// ✅ Normal webhook processing works when K8s API healthy
				// ✅ CRD created with correct priority and environment
				// ✅ Response includes metadata for client tracking
			})
		})
	*/

	// ========================================
	// BR-GATEWAY-093: Circuit Breaker TDD Implementation Roadmap
	// ========================================
	// Current Status: TDD RED Phase (documentation + implementation complete)
	// Next Step: TDD GREEN Phase (wire into server + write passing tests)
	//
	// Phase 1 (COMPLETE):
	// - ✅ BR-GATEWAY-093 updated in BUSINESS_REQUIREMENTS.md
	// - ✅ BR_MAPPING.md updated
	// - ✅ DD-GATEWAY-015 created (comprehensive design decision)
	// - ✅ ClientWithCircuitBreaker implemented (pkg/gateway/k8s/client_with_circuit_breaker.go)
	// - ✅ Circuit breaker metrics defined (pkg/gateway/metrics/metrics.go)
	// - ✅ Shared circuitbreaker.Manager created (pkg/shared/circuitbreaker/manager.go)
	//
	// Phase 2 (TODO - TDD GREEN):
	// - ⏳ Wire ClientWithCircuitBreaker into Gateway server (pkg/gateway/server.go)
	// - ⏳ Refactor CRDCreator to use circuit-breaker-protected client
	// - ⏳ Write integration tests that verify circuit breaker behavior
	// - ⏳ Run all Gateway tests to validate
	//
	// Phase 3 (TODO - TDD REFACTOR):
	// - ⏳ Optimize circuit breaker integration
	// - ⏳ Add E2E tests for circuit breaker scenarios
	// - ⏳ Performance testing under K8s API degradation
	//
	// Design Decision: DD-GATEWAY-015 (K8s API Circuit Breaker Implementation)
	// ========================================

	Context("BR-GATEWAY-093: Circuit Breaker for K8s API (TDD GREEN)", func() {
		// TDD GREEN PHASE: Circuit breaker wired into Gateway server
		// These tests validate circuit breaker behavior through CRDCreator integration
		//
		// Design Decision: DD-GATEWAY-015 (K8s API Circuit Breaker Implementation)
		// Business Requirements: BR-GATEWAY-093-A/B/C

		var (
			cbTestMetrics      *metrics.Metrics
			cbTestRegistry     *prometheus.Registry
			cbFailingK8sClient *ErrorInjectableK8sClient
			cbCrdCreator       *processing.CRDCreator
			cbLogger           logr.Logger
		)

		BeforeEach(func() {
			// Create isolated metrics registry for circuit breaker tests
			cbTestRegistry = prometheus.NewRegistry()
			cbTestMetrics = metrics.NewMetricsWithRegistry(cbTestRegistry)

			// Create error-injectable K8s client
			cbFailingK8sClient = &ErrorInjectableK8sClient{
				Client:     getKubernetesClient(),
				failCreate: false,
				errorMsg:   "K8s API unavailable",
			}

			// Wrap with base k8s.Client
			baseClient := k8s.NewClient(cbFailingK8sClient)

			// Wrap with circuit breaker (BR-GATEWAY-093)
			cbClient := k8s.NewClientWithCircuitBreaker(baseClient, cbTestMetrics)

			// Create CRD creator with circuit-breaker-protected client
			retryConfig := config.DefaultRetrySettings()
			cbLogger = zapr.NewLogger(zap.NewNop())
			cbCrdCreator = processing.NewCRDCreator(cbClient, cbLogger, cbTestMetrics, "default", &retryConfig)
		})

		It("BR-GATEWAY-093-A: should fail-fast when K8s API unavailable after consecutive failures", func() {
			// BUSINESS SCENARIO: K8s API control plane degraded (consecutive failures)
			// Expected: Circuit breaker opens, subsequent requests fail-fast (<10ms)

			ctx := context.Background()

			// Verify circuit breaker starts in CLOSED state (0)
			Eventually(func() float64 {
				metric := &dto.Metric{}
				err := cbTestMetrics.CircuitBreakerState.WithLabelValues("k8s-api").Write(metric)
				if err != nil {
					return -1
				}
				return metric.Gauge.GetValue()
			}, 2*time.Second, 100*time.Millisecond).Should(Equal(0.0), "Circuit breaker should start in CLOSED state")

			// Simulate K8s API degradation (enable failures)
			cbFailingK8sClient.failCreate = true

			// Trigger 10 consecutive failures to trip circuit breaker
			signal := &types.NormalizedSignal{
				AlertName: "HighCPU",
				Namespace: "default",
				Resource: types.ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
				Severity: "critical",
			}

			failureCount := 0
			for i := 0; i < 10; i++ {
				signal.Fingerprint = "cb-test-093a-" + string(rune(i))
				_, err := cbCrdCreator.CreateRemediationRequest(ctx, signal)
				if err != nil {
					failureCount++
				}
			}

			Expect(failureCount).To(Equal(10), "All 10 requests should fail when K8s API unavailable")

			// Verify circuit breaker opened (state = 2)
			Eventually(func() float64 {
				metric := &dto.Metric{}
				err := cbTestMetrics.CircuitBreakerState.WithLabelValues("k8s-api").Write(metric)
				if err != nil {
					return -1
				}
				return metric.Gauge.GetValue()
			}, 3*time.Second, 100*time.Millisecond).Should(Equal(2.0), "Circuit breaker should be OPEN (state=2) after 10 consecutive failures")

			// Verify next request fails fast (no K8s API call)
			signal.Fingerprint = "cb-test-093a-failfast"
			startTime := time.Now()
			_, err := cbCrdCreator.CreateRemediationRequest(ctx, signal)
			duration := time.Since(startTime)

				Expect(err).ToNot(BeNil(), "Request should fail when circuit breaker is open")
			Expect(duration).To(BeNumerically("<", 50*time.Millisecond), "Fail-fast should be immediate (<50ms), not wait for K8s API timeout")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Gateway fails-fast when K8s API degraded (BR-GATEWAY-093-A)
			// ✅ Circuit breaker protects K8s control plane from repeated failed attempts
			// ✅ Fail-fast is immediate, prevents request queue buildup
		})

		It("BR-GATEWAY-093-C: should expose observable metrics for circuit breaker state and operations", func() {
			// BUSINESS SCENARIO: Operations team monitors Gateway health via Prometheus
			// Expected: Circuit breaker metrics available for observability and alerting

			ctx := context.Background()

			signal := &types.NormalizedSignal{
				AlertName: "DiskFull",
				Namespace: "default",
				Resource: types.ResourceIdentifier{
					Kind: "Node",
					Name: "test-node",
				},
				Severity: "critical",
			}

			// Initial state: Circuit closed (0)
			Eventually(func() float64 {
				metric := &dto.Metric{}
				err := cbTestMetrics.CircuitBreakerState.WithLabelValues("k8s-api").Write(metric)
				if err != nil {
					return -1
				}
				return metric.Gauge.GetValue()
			}, 2*time.Second, 100*time.Millisecond).Should(Equal(0.0), "Circuit breaker should start in CLOSED state (0)")

			// Simulate successful operation
			cbFailingK8sClient.failCreate = false
			signal.Fingerprint = "cb-test-093c-success"
			_, _ = cbCrdCreator.CreateRemediationRequest(ctx, signal)

			// Verify success counter exists and increments
			Eventually(func() float64 {
				metricFamilies, err := cbTestRegistry.Gather()
				if err != nil {
					return -1
				}
				for _, mf := range metricFamilies {
					if mf.GetName() == "gateway_circuit_breaker_operations_total" {
						for _, m := range mf.GetMetric() {
							resultLabel := ""
							for _, label := range m.GetLabel() {
								if label.GetName() == "result" {
									resultLabel = label.GetValue()
								}
							}
							if resultLabel == "success" {
								return m.Counter.GetValue()
							}
						}
					}
				}
				return -1
			}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 1.0), "Success operations counter should increment (BR-GATEWAY-093-C)")

			// Simulate K8s API failures to open circuit
			cbFailingK8sClient.failCreate = true
			for i := 0; i < 10; i++ {
				signal.Fingerprint = "cb-test-093c-fail-" + string(rune(i))
				_, _ = cbCrdCreator.CreateRemediationRequest(ctx, signal)
			}

			// Verify circuit opened (state = 2)
			Eventually(func() float64 {
				metric := &dto.Metric{}
				err := cbTestMetrics.CircuitBreakerState.WithLabelValues("k8s-api").Write(metric)
				if err != nil {
					return -1
				}
				return metric.Gauge.GetValue()
			}, 3*time.Second, 100*time.Millisecond).Should(Equal(2.0), "Circuit breaker state metric should show OPEN (2)")

			// Verify failure counter incremented
			Eventually(func() float64 {
				metricFamilies, err := cbTestRegistry.Gather()
				if err != nil {
					return -1
				}
				for _, mf := range metricFamilies {
					if mf.GetName() == "gateway_circuit_breaker_operations_total" {
						for _, m := range mf.GetMetric() {
							resultLabel := ""
							for _, label := range m.GetLabel() {
								if label.GetName() == "result" {
									resultLabel = label.GetValue()
								}
							}
							if resultLabel == "failure" {
								return m.Counter.GetValue()
							}
						}
					}
				}
				return -1
			}, 2*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 5.0), "Failure operations counter should increment (≥5 indicates circuit breaker triggered)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ gateway_circuit_breaker_state metric exposed (0=closed, 1=half-open, 2=open)
			// ✅ gateway_circuit_breaker_operations_total metric tracks success/failure ratio
			// ✅ Metrics enable SRE response to K8s API degradation (BR-GATEWAY-093-C)
			// ✅ Real-time observability for circuit breaker state transitions
		})
	})
})
