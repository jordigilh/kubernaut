package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
// - Integration: Test adapter with real K8s infrastructure (DD-GATEWAY-012: Redis removed)
// - E2E: Test complete workflows across multiple services

var _ = Describe("BR-001, BR-002: Adapter Interaction Patterns - Integration Tests", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testServer    *httptest.Server
		gatewayServer *gateway.Server
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
		k8sClient = SetupK8sTestClient(ctx)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required")

		// Ensure test namespace exists
		EnsureTestNamespace(ctx, k8sClient, testNamespace)
		RegisterTestNamespace(testNamespace)

		// Start Gateway server
		// DD-GATEWAY-012: Redis removed, Gateway now K8s status-based
		// DD-TEST-001: Get Data Storage URL from suite's shared infrastructure
		dataStorageURL := os.Getenv("TEST_DATA_STORAGE_URL")
		if dataStorageURL == "" {
			dataStorageURL = "http://localhost:18090" // Fallback for manual testing
		}
		var startErr error
		gatewayServer, startErr = StartTestGateway(ctx, k8sClient, dataStorageURL)
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

			// STEP 2: Verify CRD created in correct namespace
			// DD-GATEWAY-011: Deduplication now tracked in RR status, not Redis
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
			Expect(crd.Spec.SignalType).To(Equal("prometheus-alert"), "Signal type - ✅ ADAPTER-CONSTANT: PrometheusAdapter uses SourceTypePrometheusAlert")
			// BR-GATEWAY-027: SignalSource is the monitoring system ("prometheus"), not adapter name
			// This enables LLM to select appropriate investigation tools (Prometheus queries)
			Expect(crd.Spec.SignalSource).To(Equal("prometheus"), "Signal source is monitoring system")
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

			// Use unique alert name per parallel process to prevent fingerprint collisions
			processID := GinkgoParallelProcess()
			uniqueAlertName := fmt.Sprintf("PodCrashLoop-p%d-%d", processID, time.Now().UnixNano())

			// STEP 1: Send first alert
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: uniqueAlertName,
				Namespace: testNamespace,
				Severity:  "warning",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "crash-pod",
				},
			})

			resp1 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
			Expect(resp1.StatusCode).To(Equal(201), "First alert should return 201 Created")

			// Wait for CRD creation AND deduplication state propagation to Redis
			// FIX: Use longer polling interval and wait for both CRD and Redis state
			Eventually(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, "30s", "500ms").Should(Equal(1), "Should have 1 CRD")

			// STEP 2: Send duplicate alert (same fingerprint)
			// FIX: Eventually handles both Redis propagation timing and Gateway readiness
			// No need for time.Sleep - Eventually will retry until deduplication state is ready
			Eventually(func() int {
				resp2 := SendWebhook(testServer.URL+"/api/v1/signals/prometheus", payload)
				GinkgoWriter.Printf("Duplicate alert status: %d (expecting 202)\n", resp2.StatusCode)
				return resp2.StatusCode
			}, "20s", "1s").Should(Equal(202), "Duplicate alert should return 202 Accepted")

			// BUSINESS VALIDATION: Still only 1 CRD (no duplicate CRD created)
			// The duplicate was detected (HTTP 202), so no new CRD should have been created.
			// We already verified 1 CRD exists at line 177-181, and duplicate returned 202,
			// so we just need to confirm the count is still 1.
			Eventually(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				if err != nil {
					GinkgoWriter.Printf("Error listing CRDs: %v\n", err)
					return -1
				}
				GinkgoWriter.Printf("CRD count in namespace %s: %d\n", testNamespace, len(crdList.Items))
				return len(crdList.Items)
			}, "10s", "500ms").Should(Equal(1), "Should still have only 1 CRD after duplicate detection")
		})
	})

	Context("BR-002: Kubernetes Event Adapter → Processing Pipeline", func() {
		It("should process Kubernetes Event through complete pipeline (adapter → priority → CRD)", func() {
			// BUSINESS OUTCOME: Kubernetes Events flow through entire pipeline
			// WHY: Validates K8s Event adapter integrates with priority assignment
			// EXPECTED: Event → Priority assignment → CRD with correct priority
			processID := GinkgoParallelProcess()

			// STEP 1: Send Kubernetes Event (using simple JSON payload)
			eventPayload := fmt.Sprintf(`{
				"metadata": {
					"name": "backoff-event-p%d",
					"namespace": "%s"
				},
				"involvedObject": {
					"kind": "Pod",
					"name": "failing-pod-p%d",
					"namespace": "%s"
				},
				"reason": "BackOff",
				"message": "Back-off restarting failed container",
				"type": "Warning",
				"firstTimestamp": "%s",
				"lastTimestamp": "%s"
			}`, processID, testNamespace, processID, testNamespace, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339))

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
			Expect(crd.Spec.SignalType).To(Equal("kubernetes-event"), "Signal type - ✅ ADAPTER-CONSTANT: KubernetesEventAdapter uses SourceTypeKubernetesEvent")
			Expect(crd.Spec.SignalSource).To(Equal("kubernetes-events"), "Signal source from adapter (monitoring system)")

			// Note: Priority validation removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001
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
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// BUSINESS VALIDATION: HTTP 415 Unsupported Media Type
			Expect(resp.StatusCode).To(Equal(415), "Wrong Content-Type should return 415")
		})
	})
})
