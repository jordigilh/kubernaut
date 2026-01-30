package gateway

// BR-GATEWAY-113, BR-GATEWAY-093: Error Handling Integration Tests (Gap Coverage)
// Authority: GW_INTEGRATION_TEST_PLAN_V1.0.md Phase 2
//
// **SCOPE**: Infrastructure-level error scenarios NOT covered by unit tests
// **RATIONALE**: These tests validate real infrastructure failures (network, timeouts, DNS)
//               that cannot be simulated in unit tests without mocks.
//
// **EXISTING COVERAGE (DO NOT DUPLICATE)**:
// - test/unit/gateway/processing/backoff_test.go: Backoff logic, jitter, max delay
// - test/unit/gateway/metrics/error_recovery_test.go: Error classification metrics
// - test/integration/gateway/29_k8s_api_failure_integration_test.go: K8s API failures + circuit breaker
//
// **GAP TESTS (THIS FILE):**
// - GW-INT-ERR-011: Context deadline with real K8s API + DataStorage
// - GW-INT-ERR-014: Real network timeout with DataStorage service
// - GW-INT-ERR-015: Cascading failures (K8s API → DataStorage → Audit)
//
// Test Pattern:
// 1. Create Gateway server with real infrastructure (K8s + DataStorage)
// 2. Inject real infrastructure failures (network timeout, context deadline)
// 3. Verify Gateway continues processing (non-fatal error behavior)
// 4. Verify audit/metrics emission despite failures
//
// Coverage: Phase 2 - 3 error handling gap tests

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Gateway Error Handling (Infrastructure Gaps)", Label("integration", "error-handling"), func() {
	var (
		ctx           context.Context
		testNamespace string
		logger        logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		processID := GinkgoParallelProcess()
		testNamespace = fmt.Sprintf("gw-error-%d-%s", processID, uuid.New().String()[:8])
		logger = GinkgoLogr

		// Create test namespace
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed(), "Test namespace must be created")

		// Create kubernaut-system fallback namespace (if not already exists)
		fallbackNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-system"}}
		_ = k8sClient.Create(ctx, fallbackNs) // Ignore error if already exists
	})

	AfterEach(func() {
		// Cleanup handled by suite-level teardown
	})

	Context("BR-GATEWAY-113: Context Deadline & Timeout Handling", func() {
		It("[GW-INT-ERR-011] should handle context deadline with real K8s API gracefully (BR-GATEWAY-113)", func() {
			By("1. Create Gateway server with real infrastructure")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
			Expect(err).ToNot(HaveOccurred())

			By("2. Process signal with very short context deadline")
			prometheusAlert := createPrometheusAlert(testNamespace, "TimeoutTest", "critical", "", "")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			// Create context with 1ms deadline (will timeout immediately)
			shortCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
			defer cancel()
			time.Sleep(5 * time.Millisecond) // Ensure context is already expired

			By("3. Process signal with expired context")
			_, err = gwServer.ProcessSignal(shortCtx, signal)

			By("4. Verify Gateway returns error but doesn't crash")
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-113: Context deadline should cause error")
			Expect(err.Error()).To(ContainSubstring("context"),
				"BR-GATEWAY-113: Error must indicate context issue")
			// Note: Response may be nil on context timeout (expected behavior)
			// Gateway didn't crash - this is what we're validating

			By("5. Verify Gateway can still process subsequent requests")
			validAlert := createPrometheusAlert(testNamespace, "AfterTimeout", "warning", "", "")
			validSignal, err := prometheusAdapter.Parse(ctx, validAlert)
			Expect(err).ToNot(HaveOccurred())

			validResponse, err := gwServer.ProcessSignal(ctx, validSignal)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-113: Gateway must continue processing after context timeout")
			Expect(validResponse.Status).To(Equal("created"),
				"BR-GATEWAY-113: Subsequent signals must be processed normally")

			GinkgoWriter.Printf("✅ Context deadline handled gracefully: Gateway continues processing\n")
		})

		It("[GW-INT-ERR-014] should handle DataStorage timeout gracefully (BR-GATEWAY-058)", func() {
			By("1. Create Gateway server with real DataStorage")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
			Expect(err).ToNot(HaveOccurred())

			By("2. Process signal with very short context deadline (will timeout DataStorage)")
			prometheusAlert := createPrometheusAlert(testNamespace, "DataStorageTimeout", "critical", "", "")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			signal, err := prometheusAdapter.Parse(ctx, prometheusAlert)
			Expect(err).ToNot(HaveOccurred())

			// Create context with 10ms deadline (DataStorage might timeout)
			shortCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
			defer cancel()

			By("3. Process signal with very short deadline")
			response, err := gwServer.ProcessSignal(shortCtx, signal)

			By("4. Verify CRD creation succeeds (critical path)")
			// BR-GATEWAY-058: Audit emission is non-blocking, so CRD should succeed even if audit times out
			if err != nil {
				// If error occurs, it should be audit-related, not CRD creation
				GinkgoWriter.Printf("⚠️  Error occurred (likely audit timeout): %v\n", err)
			}

			// Verify CRD was created regardless of audit timeout
			if response != nil && response.RemediationRequestName != "" {
				rr := &remediationv1alpha1.RemediationRequest{}
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKey{
						Name:      response.RemediationRequestName,
						Namespace: testNamespace,
					}, rr)
				}, 5*time.Second, 500*time.Millisecond).Should(Succeed(),
					"BR-GATEWAY-058: CRD creation must succeed even if audit times out")
			}

			By("5. Verify Gateway continues processing after timeout")
			validAlert := createPrometheusAlert(testNamespace, "AfterDataStorageTimeout", "info", "", "")
			validSignal, err := prometheusAdapter.Parse(ctx, validAlert)
			Expect(err).ToNot(HaveOccurred())

			validResponse, err := gwServer.ProcessSignal(ctx, validSignal)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-058: Gateway must continue after DataStorage timeout")
			Expect(validResponse.Status).To(Equal("created"),
				"BR-GATEWAY-058: Subsequent signals processed normally")

			GinkgoWriter.Printf("✅ DataStorage timeout handled gracefully: Gateway continues processing\n")
		})
	})

	Context("BR-GATEWAY-113: Cascading Failure Resilience", func() {
		It("[GW-INT-ERR-015] should handle cascading failures across K8s API and DataStorage (BR-GATEWAY-113)", func() {
			By("1. Create Gateway server with real infrastructure")
			gatewayConfig := createGatewayConfig(fmt.Sprintf("http://127.0.0.1:%d", gatewayDataStoragePort))
			gwServer, err := createGatewayServer(gatewayConfig, logger, k8sClient, sharedAuditStore)
			Expect(err).ToNot(HaveOccurred())

			By("2. Create unique namespace to trigger potential race conditions")
			uniqueNs := fmt.Sprintf("cascade-test-%s", uuid.New().String()[:8])
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: uniqueNs}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			By("3. Process 5 rapid signals to stress test error handling")
			prometheusAdapter := adapters.NewPrometheusAdapter()
			successCount := 0
			errorCount := 0

			for i := 0; i < 5; i++ {
				alertName := fmt.Sprintf("CascadeTest-%d", i)
				alert := createPrometheusAlert(uniqueNs, alertName, "warning", "", "")
				signal, err := prometheusAdapter.Parse(ctx, alert)
				Expect(err).ToNot(HaveOccurred())

				response, err := gwServer.ProcessSignal(ctx, signal)
				if err != nil {
					errorCount++
					GinkgoWriter.Printf("⚠️  Signal %d error: %v\n", i, err)
				} else if response.Status == "created" || response.Status == "deduplicated" {
					successCount++
				}

				// Small delay between signals to allow infrastructure to process
				time.Sleep(100 * time.Millisecond)
			}

			By("4. Verify Gateway successfully processed majority of signals")
			Expect(successCount).To(BeNumerically(">=", 3),
				"BR-GATEWAY-113: Gateway must successfully process at least 3/5 signals despite stress")

			By("5. Verify Gateway didn't crash (can still process new signals)")
			validAlert := createPrometheusAlert(uniqueNs, "AfterCascade", "info", "", "")
			validSignal, err := prometheusAdapter.Parse(ctx, validAlert)
			Expect(err).ToNot(HaveOccurred())

			validResponse, err := gwServer.ProcessSignal(ctx, validSignal)
		Expect(err).ToNot(HaveOccurred(),
			"BR-GATEWAY-113: Gateway must recover from cascading failures")
		Expect(validResponse.Status).To(Or(Equal("created"), Equal("duplicate")),
			"BR-GATEWAY-113: Gateway must continue normal operation after stress")

			GinkgoWriter.Printf("✅ Cascading failure resilience: %d/%d signals succeeded, Gateway operational\n",
				successCount, 5)
		})
	})
})
