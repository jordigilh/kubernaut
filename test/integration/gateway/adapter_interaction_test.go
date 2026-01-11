package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// K8sClientWrapper wraps controller-runtime client to implement k8s.ClientInterface
// This adapter allows integration tests to use the test client with CRDCreator
type K8sClientWrapper struct {
	Client client.Client
}

func (w *K8sClientWrapper) CreateRemediationRequest(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	return w.Client.Create(ctx, rr)
}

func (w *K8sClientWrapper) GetRemediationRequest(ctx context.Context, namespace, name string) (*remediationv1alpha1.RemediationRequest, error) {
	var rr remediationv1alpha1.RemediationRequest
	err := w.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &rr)
	if err != nil {
		return nil, err
	}
	return &rr, nil
}

// BR-001: Prometheus AlertManager webhook ingestion
// BR-002: Kubernetes Event API signal ingestion
//
// Business Outcome: All adapters integrate consistently with processing pipeline
//
// Test Strategy: Validate complete signal flow from adapter → dedup → CRD
// - Prometheus adapter → deduplication → CRD creation
// - K8s Event adapter → priority assignment → CRD creation
// - Adapter error handling → error propagation
//
// Defense-in-Depth: These integration tests complement unit tests
// - Unit: Test adapter validation logic (pure business logic)
// - Integration: Test adapter with real K8s infrastructure (DD-GATEWAY-012: Redis removed)
// - E2E: Test complete workflows across multiple services
//
// HTTP Anti-Pattern Phase 4: Refactored from HTTP calls to direct business logic calls

var _ = Describe("BR-001, BR-002: Adapter Interaction Patterns - Integration Tests", func() {
	var (
		ctx                context.Context
		cancel             context.CancelFunc
		k8sClient          *K8sTestClient
		testNamespace      string
		testCounter        int
		prometheusAdapter  adapters.SignalAdapter
		k8sEventAdapter    adapters.SignalAdapter
		crdCreator         *processing.CRDCreator
		dedupChecker       *processing.PhaseBasedDeduplicationChecker
		logger             logr.Logger
		metricsInstance    *metrics.Metrics
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

		// HTTP Anti-Pattern Phase 4: Initialize business logic components directly
		// No HTTP server needed - testing business logic coordination

		// Initialize logger
		logger = logr.Discard()

		// Initialize metrics (required by CRDCreator)
		metricsInstance = metrics.NewMetrics()

		// Initialize adapters
		prometheusAdapter = adapters.NewPrometheusAdapter()
		k8sEventAdapter = adapters.NewKubernetesEventAdapter()

		// Initialize deduplication checker
		dedupChecker = processing.NewPhaseBasedDeduplicationChecker(k8sClient.Client)

		// Initialize CRD creator
		retryConfig := &config.RetrySettings{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond * 100,
		}
		// Note: CRDCreator expects k8s.ClientInterface which wraps controller-runtime client
		// For integration tests, we create a wrapper that implements the interface
		k8sClientWrapper := &K8sClientWrapper{Client: k8sClient.Client}
		crdCreator = processing.NewCRDCreator(
			k8sClientWrapper,
			logger,
			metricsInstance,
			testNamespace,
			retryConfig,
		)
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
		// HTTP Anti-Pattern Phase 4: No HTTP server cleanup needed
	})

	Context("BR-001: Prometheus Adapter → Processing Pipeline", func() {
		It("should process Prometheus alert through complete pipeline (adapter → dedup → CRD)", func() {
			// BUSINESS OUTCOME: Prometheus alerts flow through entire pipeline
			// WHY: Validates adapter integrates with deduplication and CRD creation
			// EXPECTED: Alert → Parse → Validate → Deduplication check → CRD created
			//
			// HTTP Anti-Pattern Phase 4: Refactored from HTTP calls to direct business logic

			// STEP 1: Generate Prometheus alert payload
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: "HighMemoryUsage",
				Namespace: testNamespace,
				Severity:  "critical",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "test-pod",
				},
			})

			// STEP 2: Parse payload using Prometheus adapter
			signal, err := prometheusAdapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred(), "Prometheus adapter should parse valid alert")
			Expect(signal).ToNot(BeNil())
			Expect(signal.Fingerprint).ToNot(BeEmpty(), "Signal must have fingerprint for deduplication")

			// STEP 3: Validate signal
			err = prometheusAdapter.Validate(signal)
			Expect(err).ToNot(HaveOccurred(), "Signal should pass validation")

			// STEP 4: Check deduplication (first alert, should NOT be duplicate)
			shouldDedup, existingRR, err := dedupChecker.ShouldDeduplicate(ctx, testNamespace, signal.Fingerprint)
			Expect(err).ToNot(HaveOccurred(), "Deduplication check should succeed")
			Expect(shouldDedup).To(BeFalse(), "First alert should NOT be duplicate")
			Expect(existingRR).To(BeNil(), "No existing RR for first alert")

			// STEP 5: Create CRD
			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)
			Expect(err).ToNot(HaveOccurred(), "CRD creation should succeed")
			Expect(rr).ToNot(BeNil())
			Expect(rr.Name).ToNot(BeEmpty())

			// STEP 6: Verify CRD created in Kubernetes
			Eventually(func() bool {
				var created remediationv1alpha1.RemediationRequest
				err := k8sClient.Client.Get(ctx, client.ObjectKey{
					Name:      rr.Name,
					Namespace: rr.Namespace,
				}, &created)
				return err == nil
			}, "30s", "500ms").Should(BeTrue(), "CRD should be created in K8s")

			// BUSINESS VALIDATION: Verify CRD has correct metadata from adapter
			var finalRR remediationv1alpha1.RemediationRequest
			err = k8sClient.Client.Get(ctx, client.ObjectKey{Name: rr.Name, Namespace: rr.Namespace}, &finalRR)
			Expect(err).ToNot(HaveOccurred())

			Expect(finalRR.Spec.SignalType).To(Equal("prometheus-alert"), "Signal type from PrometheusAdapter")
			Expect(finalRR.Spec.SignalSource).To(Equal("prometheus"), "Signal source is monitoring system (BR-GATEWAY-027)")
			Expect(finalRR.Namespace).To(Equal(testNamespace), "CRD in correct namespace")
			Expect(finalRR.Spec.Severity).To(Equal("critical"), "Severity from alert")
			Expect(finalRR.Spec.ProviderData).ToNot(BeEmpty(), "Provider data should contain resource info")
		})

		It("should handle duplicate Prometheus alerts correctly (deduplication integration)", func() {
			// BUSINESS OUTCOME: Duplicate alerts don't create duplicate CRDs
			// WHY: Validates adapter integrates with deduplication service
			// EXPECTED: First alert → CRD created, Second alert → detected as duplicate, no new CRD
			//
			// HTTP Anti-Pattern Phase 4: Refactored from HTTP calls to direct business logic

			// Use unique alert name per parallel process to prevent fingerprint collisions
			processID := GinkgoParallelProcess()
			uniqueAlertName := fmt.Sprintf("PodCrashLoop-p%d-%d", processID, time.Now().UnixNano())

			// STEP 1: Generate first alert payload
			payload := GeneratePrometheusAlert(PrometheusAlertOptions{
				AlertName: uniqueAlertName,
				Namespace: testNamespace,
				Severity:  "warning",
				Resource: ResourceIdentifier{
					Kind: "Pod",
					Name: "crash-pod",
				},
			})

			// STEP 2: Process first alert
			signal1, err := prometheusAdapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			err = prometheusAdapter.Validate(signal1)
			Expect(err).ToNot(HaveOccurred())

			// Check deduplication (first alert, should NOT be duplicate)
			shouldDedup1, _, err := dedupChecker.ShouldDeduplicate(ctx, testNamespace, signal1.Fingerprint)
			Expect(err).ToNot(HaveOccurred())
			Expect(shouldDedup1).To(BeFalse(), "First alert should NOT be duplicate")

			// Create CRD for first alert
			rr1, err := crdCreator.CreateRemediationRequest(ctx, signal1)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr1).ToNot(BeNil())

			// Wait for CRD creation in K8s
			Eventually(func() bool {
				var created remediationv1alpha1.RemediationRequest
				err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: rr1.Name, Namespace: rr1.Namespace}, &created)
				return err == nil
			}, "30s", "500ms").Should(BeTrue(), "First CRD should be created")

			// STEP 3: Process duplicate alert (same payload → same fingerprint)
			// Wait for K8s to index the new RR (deduplication queries by fingerprint)
			time.Sleep(1 * time.Second)

			signal2, err := prometheusAdapter.Parse(ctx, payload)
			Expect(err).ToNot(HaveOccurred())
			err = prometheusAdapter.Validate(signal2)
			Expect(err).ToNot(HaveOccurred())

			// Check deduplication (should detect duplicate)
			Eventually(func() bool {
				shouldDedup2, existingRR, err := dedupChecker.ShouldDeduplicate(ctx, testNamespace, signal2.Fingerprint)
				if err != nil {
					GinkgoWriter.Printf("Deduplication check error: %v\n", err)
					return false
				}
				if !shouldDedup2 {
					GinkgoWriter.Printf("Duplicate not detected yet, retrying...\n")
					return false
				}
				if existingRR == nil {
					GinkgoWriter.Printf("Duplicate detected but existingRR is nil\n")
					return false
				}
				GinkgoWriter.Printf("✅ Duplicate detected: existing RR = %s\n", existingRR.Name)
				Expect(existingRR.Name).To(Equal(rr1.Name), "Existing RR should match first RR")
				return true
			}, "20s", "1s").Should(BeTrue(), "Duplicate alert should be detected")

			// BUSINESS VALIDATION: Still only 1 CRD (no duplicate CRD created)
			Eventually(func() int {
				crdList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(crdList.Items)
			}, "10s", "500ms").Should(Equal(1), "Should still have only 1 CRD after duplicate detection")
		})
	})

	Context("BR-002: Kubernetes Event Adapter → Processing Pipeline", func() {
		It("should process Kubernetes Event through complete pipeline (adapter → priority → CRD)", func() {
			// BUSINESS OUTCOME: Kubernetes Events flow through entire pipeline
			// WHY: Validates K8s Event adapter integrates with pipeline
			// EXPECTED: Event → Parse → Validate → Dedup check → CRD created
			//
			// HTTP Anti-Pattern Phase 4: Refactored from HTTP calls to direct business logic
			processID := GinkgoParallelProcess()

			// STEP 1: Generate Kubernetes Event payload
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

			// STEP 2: Parse payload using Kubernetes Event adapter
			signal, err := k8sEventAdapter.Parse(ctx, []byte(eventPayload))
			Expect(err).ToNot(HaveOccurred(), "K8s Event adapter should parse valid event")
			Expect(signal).ToNot(BeNil())
			Expect(signal.Fingerprint).ToNot(BeEmpty())

			// STEP 3: Validate signal
			err = k8sEventAdapter.Validate(signal)
			Expect(err).ToNot(HaveOccurred())

			// STEP 4: Check deduplication
			shouldDedup, existingRR, err := dedupChecker.ShouldDeduplicate(ctx, testNamespace, signal.Fingerprint)
			Expect(err).ToNot(HaveOccurred())
			Expect(shouldDedup).To(BeFalse(), "First event should NOT be duplicate")
			Expect(existingRR).To(BeNil())

			// STEP 5: Create CRD
			rr, err := crdCreator.CreateRemediationRequest(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			Expect(rr).ToNot(BeNil())

			// STEP 6: Verify CRD created in Kubernetes
			Eventually(func() bool {
				var created remediationv1alpha1.RemediationRequest
				err := k8sClient.Client.Get(ctx, client.ObjectKey{Name: rr.Name, Namespace: rr.Namespace}, &created)
				return err == nil
			}, "30s", "500ms").Should(BeTrue())

			// BUSINESS VALIDATION: Verify CRD metadata from K8s Event adapter
			var finalRR remediationv1alpha1.RemediationRequest
			err = k8sClient.Client.Get(ctx, client.ObjectKey{Name: rr.Name, Namespace: rr.Namespace}, &finalRR)
			Expect(err).ToNot(HaveOccurred())

			Expect(finalRR.Spec.SignalType).To(Equal("kubernetes-event"), "Signal type from KubernetesEventAdapter")
			Expect(finalRR.Spec.SignalSource).To(Equal("kubernetes-events"), "Signal source is monitoring system")

			// Note: Priority validation removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001
		})
	})

	Context("Adapter Error Handling", func() {
		It("should reject invalid Prometheus alert payload", func() {
			// BUSINESS OUTCOME: Adapters validate payloads and return clear errors
			// WHY: Operators need actionable errors to fix misconfigured AlertManager
			// EXPECTED: Parse() returns error for malformed JSON
			//
			// HTTP Anti-Pattern Phase 4: Refactored from HTTP 400 check to Parse() error check

			// Create invalid payload (malformed JSON)
			invalidPayload := []byte(`{"invalid": "json"`)

			// BUSINESS VALIDATION: Parse should fail with error
			signal, err := prometheusAdapter.Parse(ctx, invalidPayload)
			Expect(err).To(HaveOccurred(), "Invalid payload should fail parsing")
			Expect(signal).To(BeNil(), "Signal should be nil for invalid payload")

			// No CRD should be created
			time.Sleep(2 * time.Second) // Allow time for any potential CRD creation
			crdList := &remediationv1alpha1.RemediationRequestList{}
			err = k8sClient.Client.List(ctx, crdList, client.InNamespace(testNamespace))
			Expect(err).ToNot(HaveOccurred())
			Expect(len(crdList.Items)).To(Equal(0), "No CRD should be created for invalid payload")
		})
	})
})
