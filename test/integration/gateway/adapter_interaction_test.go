package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	gateway "github.com/jordigilh/kubernaut/pkg/gateway"
)

// BR-001: Prometheus AlertManager webhook ingestion
// BR-002: Kubernetes Event API signal ingestion
//
// Business Outcome: All adapters integrate consistently with processing pipeline
//
// Test Strategy: Validate complete signal flow from adapter → dedup → CRD
// - Prometheus adapter → deduplication → CRD creation
// - K8s Event adapter → priority assignment → CRD creation
// - Adapter error handling → HTTP error responses
//
// Defense-in-Depth: These integration tests complement unit tests
// - Unit: Test adapter validation logic (pure business logic)
// - Integration: Test adapter with real Redis/K8s infrastructure
// - E2E: Test complete workflows across multiple services

var _ = Describe("BR-001, BR-002: Adapter Interaction Patterns - Integration Tests", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testServer    *httptest.Server
		gatewayServer *gateway.Server
		redisClient   *RedisTestClient
		k8sClient     *K8sTestClient
		testNamespace string
		testCounter   int
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)

		// Generate unique namespace for test isolation
		testCounter++
		testNamespace = fmt.Sprintf("test-adapter-%d-%d-%d",
			time.Now().UnixNano(),
			GinkgoRandomSeed(),
			testCounter)

		// Setup test infrastructure
		redisClient = SetupRedisTestClient(ctx)
		Expect(redisClient).ToNot(BeNil(), "Redis client required")
		Expect(redisClient.Client).ToNot(BeNil(), "Redis connection required")

		k8sClient = SetupK8sTestClient(ctx)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required")

		// Clean Redis state
		err := redisClient.Client.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Should flush Redis")

		// Ensure test namespace exists
		EnsureTestNamespace(ctx, k8sClient, testNamespace)
		RegisterTestNamespace(testNamespace)

		// Start Gateway server
		var startErr error
		gatewayServer, startErr = StartTestGateway(ctx, redisClient, k8sClient)
		Expect(startErr).ToNot(HaveOccurred(), "Gateway should start")
		Expect(gatewayServer).ToNot(BeNil(), "Gateway server should exist")

		// Create HTTP test server
		testServer = httptest.NewServer(gatewayServer.Handler())
		Expect(testServer).ToNot(BeNil(), "HTTP test server should exist")
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
		if cancel != nil {
			cancel()
		}
	})

	Context("BR-001: Prometheus Adapter → Processing Pipeline", func() {
		It("should process Prometheus alert through complete pipeline (adapter → dedup → CRD)", func() {
			// BUSINESS OUTCOME: Prometheus alerts flow through entire pipeline
			// WHY: Validates adapter integrates with deduplication and CRD creation
			// EXPECTED: Alert → Deduplication check → CRD created in correct namespace
			//
			// DEFENSE-IN-DEPTH: Complements unit tests
			// - Unit: Tests adapter validation logic
			// - Integration: Tests adapter with real Redis/K8s (THIS TEST)
			// - E2E: Tests complete alert-to-resolution workflow

			// STEP 1: Send Prometheus alert
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: testNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			})

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS VALIDATION 1: HTTP 201 Created (first alert, not duplicate)
			Expect(resp.StatusCode).To(Equal(201), "First alert should return 201 Created")

			// STEP 2: Verify deduplication state in Redis
			Eventually(func() int {
				return redisClient.CountFingerprints(ctx, testNamespace)
			}, "10s", "100ms").Should(Equal(1), "Should have 1 fingerprint in Redis")

			// STEP 3: Verify CRD created in correct namespace
			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				if err != nil {
					return err
				}
				if len(crdList.Items) == 0 {
					return fmt.Errorf("no CRDs found")
				}
				crd = crdList.Items[0]
				return nil
			}, "30s", "500ms").Should(Succeed(), "CRD should be created")

			// BUSINESS VALIDATION 2: CRD has correct metadata from adapter
			Expect(crd.Spec.SignalType).To(Equal("prometheus-alert"), "Signal type from adapter")
			Expect(crd.Spec.SignalSource).To(Equal("prometheus-adapter"), "Signal source from adapter")
			Expect(crd.Namespace).To(Equal(testNamespace), "CRD in correct namespace")

			// BUSINESS VALIDATION 3: CRD has correct business data
			Expect(crd.Spec.Severity).To(Equal("critical"), "Severity from alert")
			// Resource information is stored in ProviderData as JSON
			Expect(crd.Spec.ProviderData).ToNot(BeEmpty(), "Provider data should contain resource info")
		})

		It("should handle duplicate Prometheus alerts correctly (deduplication integration)", func() {
			// BUSINESS OUTCOME: Duplicate alerts don't create duplicate CRDs
			// WHY: Validates adapter integrates with deduplication service
			// EXPECTED: First alert → CRD, Second alert → HTTP 202 Accepted (duplicate)

			// STEP 1: Send first alert
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "PodCrashLoop",
				Namespace: testNamespace,
				Severity:  "warning",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "crash-pod",
				},
			})

			resp1 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp1.StatusCode).To(Equal(201), "First alert should return 201 Created")

			// Wait for CRD creation
			Eventually(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, "30s", "500ms").Should(Equal(1), "Should have 1 CRD")

			// STEP 2: Send duplicate alert (same fingerprint)
			resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)

			// BUSINESS VALIDATION: HTTP 202 Accepted (duplicate detected)
			Expect(resp2.StatusCode).To(Equal(202), "Duplicate alert should return 202 Accepted")

			// BUSINESS VALIDATION: Still only 1 CRD (no duplicate CRD created)
			Consistently(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, "5s", "500ms").Should(Equal(1), "Should still have only 1 CRD")
		})
	})

	Context("BR-002: Kubernetes Event Adapter → Processing Pipeline", func() {
		It("should process Kubernetes Event through complete pipeline (adapter → priority → CRD)", func() {
			// BUSINESS OUTCOME: Kubernetes Events flow through entire pipeline
			// WHY: Validates K8s Event adapter integrates with priority assignment
			// EXPECTED: Event → Priority assignment → CRD with correct priority

			// STEP 1: Send Kubernetes Event (using simple JSON payload)
			eventPayload := fmt.Sprintf(`{
				"metadata": {
					"name": "backoff-event",
					"namespace": "%s"
				},
				"involvedObject": {
					"kind": "Pod",
					"name": "failing-pod",
					"namespace": "%s"
				},
				"reason": "BackOff",
				"message": "Back-off restarting failed container",
				"type": "Warning",
				"firstTimestamp": "%s",
				"lastTimestamp": "%s"
			}`, testNamespace, testNamespace, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))

			resp := SendWebhook(testServer.URL+"/api/v1/signals/kubernetes-event", []byte(eventPayload))

			// BUSINESS VALIDATION 1: HTTP 201 Created
			Expect(resp.StatusCode).To(Equal(201), "Event should return 201 Created")

			// STEP 2: Verify CRD created with priority assignment
			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				if err != nil {
					return err
				}
				if len(crdList.Items) == 0 {
					return fmt.Errorf("no CRDs found")
				}
				crd = crdList.Items[0]
				return nil
			}, "30s", "500ms").Should(Succeed(), "CRD should be created")

			// BUSINESS VALIDATION 2: CRD has correct metadata from K8s Event adapter
			Expect(crd.Spec.SignalType).To(Equal("kubernetes-event"), "Signal type from adapter")
			Expect(crd.Spec.SignalSource).To(Equal("kubernetes-event"), "Signal source from adapter")

			// BUSINESS VALIDATION 3: CRD has priority assigned by processing pipeline
			Expect(crd.Spec.Priority).ToNot(BeEmpty(), "Priority should be assigned")
			// Priority is determined by Rego policy based on severity + environment
		})
	})

	Context("Adapter Error Handling", func() {
		It("should return HTTP 400 for invalid Prometheus alert payload", func() {
			// BUSINESS OUTCOME: Clear error messages for invalid payloads
			// WHY: Operators need actionable errors to fix misconfigured AlertManager
			// EXPECTED: HTTP 400 with RFC 7807 error details

			// Send invalid payload (malformed JSON)
			invalidPayload := []byte(`{"invalid": "json"`)

			resp := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", invalidPayload)

			// BUSINESS VALIDATION: HTTP 400 Bad Request
			Expect(resp.StatusCode).To(Equal(400), "Invalid payload should return 400")

			// BUSINESS VALIDATION: RFC 7807 error format
			Expect(resp.Headers.Get("Content-Type")).To(ContainSubstring("application/problem+json"),
				"Error should use RFC 7807 format")
		})

		It("should return HTTP 415 for invalid Content-Type", func() {
			// BUSINESS OUTCOME: Clear error for wrong Content-Type header
			// WHY: Prevents silent failures from misconfigured webhooks
			// EXPECTED: HTTP 415 Unsupported Media Type

			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "TestAlert",
				Namespace: testNamespace,
				Severity:  "info",
			})

			// Send with wrong Content-Type (using http.NewRequest directly)
			req, err := http.NewRequest("POST", testServer.URL+"/api/v1/signals/prometheus", bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "text/plain") // Wrong Content-Type

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// BUSINESS VALIDATION: HTTP 415 Unsupported Media Type
			Expect(resp.StatusCode).To(Equal(415), "Wrong Content-Type should return 415")
		})
	})
})

