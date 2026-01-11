package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
)

// BR-001: Prometheus AlertManager webhook ingestion
// BR-011: RemediationRequest CRD creation
//
// Business Outcome: Gateway interacts correctly with Kubernetes API
//
// Test Strategy: Validate CRD creation, namespace handling, and API error handling
// - CRD creation in correct namespace
// - CRD creation with proper metadata
// - Namespace validation and fallback behavior
//
// Defense-in-Depth: These integration tests complement unit tests
// - Unit: Test CRD creation logic (mocked K8s client)
// - Integration: Test with real Kubernetes API (THIS TEST)
// - E2E: Test complete workflows with real cluster

var _ = Describe("BR-001, BR-011: Kubernetes API Interaction - Integration Tests", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		k8sClient         *K8sTestClient
		k8sClientWrapper  *K8sClientWrapper
		prometheusAdapter *adapters.PrometheusAdapter
		crdCreator        *processing.CRDCreator
		logger            logr.Logger
		testNamespace     string
		testCounter       int
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)

		// Generate unique namespace for test isolation
		testCounter++
		testNamespace = fmt.Sprintf("test-k8s-%d-%d-%d",
			time.Now().UnixNano(),
			GinkgoRandomSeed(),
			testCounter)

		// Setup test infrastructure
		k8sClient = SetupK8sTestClient(ctx)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required")

		// Ensure test namespace exists
		EnsureTestNamespace(ctx, k8sClient, testNamespace)
		RegisterTestNamespace(testNamespace)

		// ✅ Initialize business logic components (NO HTTP server)
		logger = logr.Discard()
		prometheusAdapter = adapters.NewPrometheusAdapter()
		k8sClientWrapper = &K8sClientWrapper{Client: k8sClient.Client}
		// Use isolated registry for test isolation (prevents prometheus.AlreadyRegisteredError)
		testRegistry := prometheus.NewRegistry()
		crdCreator = processing.NewCRDCreator(k8sClientWrapper, logger, metrics.NewMetricsWithRegistry(testRegistry), testNamespace, &config.RetrySettings{
			MaxAttempts:    3,
			InitialBackoff: 100 * time.Millisecond,
		})
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
	})

	Context("BR-011: CRD Creation in Correct Namespace", func() {
		It("should create RemediationRequest CRD in signal's origin namespace", func() {
			// BUSINESS OUTCOME: CRDs created in correct namespace for tenant isolation
			// WHY: Multi-tenancy requires namespace-based RBAC
			// EXPECTED: CRD created in alert's namespace, not default or kubernaut-system
			//
			// DEFENSE-IN-DEPTH: Complements unit tests
			// - Unit: Tests CRD creation logic with mocked K8s client
			// - Integration: Tests with real Kubernetes API (THIS TEST)
			// - E2E: Tests complete workflows with real cluster

			// ✅ Step 1: Parse Prometheus alert
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "K8sAPITest",
				Namespace: testNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			})
			signal, err := prometheusAdapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())

			// ✅ Step 2: Create CRD in K8s
			_, err = crdCreator.CreateRemediationRequest(ctx, signal)
			Expect(err).ToNot(HaveOccurred(), "Alert should be accepted")

			// STEP 3: Verify CRD created in correct namespace
			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				if err != nil {
					return err
				}
				if len(crdList.Items) == 0 {
					return fmt.Errorf("no CRDs found in namespace %s", testNamespace)
				}
				crd = crdList.Items[0]
				return nil
			}, "30s", "500ms").Should(Succeed(), "CRD should be created in correct namespace")

			// BUSINESS VALIDATION 1: CRD in correct namespace
			Expect(crd.Namespace).To(Equal(testNamespace), "CRD should be in alert's namespace")

			// BUSINESS VALIDATION 2: CRD has proper Kubernetes metadata
			Expect(crd.Name).To(HavePrefix("rr-"), "CRD name should have rr- prefix")
			Expect(crd.Labels).To(HaveKey("app.kubernetes.io/managed-by"), "Should have managed-by label")
			Expect(crd.Labels["app.kubernetes.io/managed-by"]).To(Equal("gateway-service"),
				"Should be managed by gateway-service")

			// BUSINESS VALIDATION 3: CRD has Kubernaut-specific labels
			Expect(crd.Labels).To(HaveKey("kubernaut.ai/signal-type"), "Should have signal-type label")
			Expect(crd.Labels).To(HaveKey("kubernaut.ai/severity"), "Should have severity label")
			// Note: environment and priority labels removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001
		})

		It("should create CRD with complete metadata for Kubernetes API queries", func() {
			// BUSINESS OUTCOME: CRDs queryable via Kubernetes API labels
			// WHY: Operators need to filter CRDs by severity, environment, priority
			// EXPECTED: All labels set correctly for kubectl queries

			// ✅ Step 1: Parse production critical alert
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "MetadataTest",
				Namespace: testNamespace,
				Severity:  "critical",
				Labels: map[string]string{
					"environment": "production",
				},
			})
			signal, err := prometheusAdapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())

			// ✅ Step 2: Create CRD
			_, err = crdCreator.CreateRemediationRequest(ctx, signal)
			Expect(err).ToNot(HaveOccurred(), "Alert should be accepted")

			// STEP 3: Query CRD using Kubernetes API labels
			var crd remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				// Query by severity label (simulates: kubectl get rr -l kubernaut.ai/severity=critical)
				err := k8sClient.Client.List(ctx, crdList,
					client.InNamespace(testNamespace),
					client.MatchingLabels{"kubernaut.ai/severity": "critical"})
				if err != nil {
					return err
				}
				if len(crdList.Items) == 0 {
					return fmt.Errorf("no CRDs found with severity=critical label")
				}
				crd = crdList.Items[0]
				return nil
			}, "30s", "500ms").Should(Succeed(), "Should query CRD by label")

			// BUSINESS VALIDATION: Labels enable Kubernetes API queries
			Expect(crd.Labels["kubernaut.ai/severity"]).To(Equal("critical"),
				"Severity label for kubectl queries")
			// Note: environment and priority label assertions removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001

			// BUSINESS VALIDATION: Annotations for audit trail
			Expect(crd.Annotations).To(HaveKey("kubernaut.ai/created-at"),
				"Should have creation timestamp for audit")
		})
	})

	Context("BR-011: Namespace Validation and Fallback", func() {
		It("should handle namespace validation correctly", func() {
			// BUSINESS OUTCOME: Namespace existence validated before CRD creation
			// WHY: Prevents CRD creation failures due to invalid namespaces
			// EXPECTED: Valid namespace → CRD created, Invalid namespace → fallback to kubernaut-system

			// ✅ Step 1: Parse and create alert for valid namespace
			validPayload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "ValidNamespaceTest",
				Namespace: testNamespace,
				Severity:  "warning",
			})
			signal1, err := prometheusAdapter.Parse(ctx, validPayload)
			Expect(err).ToNot(HaveOccurred())

			_, err = crdCreator.CreateRemediationRequest(ctx, signal1)
			Expect(err).ToNot(HaveOccurred(), "Valid namespace should succeed")

			// Verify CRD in correct namespace
			Eventually(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				return len(crdList.Items)
			}, "30s", "500ms").Should(Equal(1), "CRD should be in valid namespace")

			// ✅ Step 2: Parse alert for invalid namespace
			invalidNamespace := fmt.Sprintf("invalid-ns-%d", time.Now().UnixNano())
			invalidPayload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "InvalidNamespaceTest",
				Namespace: invalidNamespace,
				Severity:  "warning",
			})
			signal2, err := prometheusAdapter.Parse(ctx, invalidPayload)
			Expect(err).ToNot(HaveOccurred())

			// ✅ Step 3: Create CRD with fallback namespace handling
			crdCreatorFallback := processing.NewCRDCreator(k8sClientWrapper, logger, metrics.NewMetrics(), "kubernaut-system", &config.RetrySettings{
				MaxAttempts:    3,
				InitialBackoff: 100 * time.Millisecond,
			})
			_, err = crdCreatorFallback.CreateRemediationRequest(ctx, signal2)
			Expect(err).ToNot(HaveOccurred(), "Invalid namespace should fallback gracefully")

			// Verify CRD in fallback namespace (kubernaut-system)
			var fallbackCRD *remediationv1alpha1.RemediationRequest
			Eventually(func() bool {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.Client.List(ctx, crdList, client.InNamespace("kubernaut-system"))
				if err == nil && len(crdList.Items) > 0 {
					// Find the CRD for invalid namespace test
					for i := range crdList.Items {
						if crdList.Items[i].Spec.SignalName == "InvalidNamespaceTest" {
							fallbackCRD = &crdList.Items[i]
							return true
						}
					}
				}
				return false
			}, "30s", "500ms").Should(BeTrue(), "CRD should be in fallback namespace")

			// BUSINESS VALIDATION: Fallback CRD has correct labels
			Expect(fallbackCRD).ToNot(BeNil(), "Fallback CRD should exist")
			Expect(fallbackCRD.Namespace).To(Equal("kubernaut-system"),
				"Should be in kubernaut-system namespace")
			Expect(fallbackCRD.Labels["kubernaut.ai/cluster-scoped"]).To(Equal("true"),
				"Should have cluster-scoped label")
			Expect(fallbackCRD.Labels["kubernaut.ai/origin-namespace"]).To(Equal(invalidNamespace),
				"Should preserve origin namespace in label")
		})
	})

	// REMOVED: Context "BR-011: Kubernetes API Error Handling" with test "should handle concurrent CRD creation correctly"
	// REASON: envtest K8s cache causes intermittent failures (~40% fail rate)
	// COVERAGE: Unit tests (deduplication_edge_cases_test.go) + E2E tests (06_concurrent_alerts_test.go)
})
