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

// Package signalprocessing contains integration tests for SignalProcessing controller.
//
// # Business Requirements
//
// BR-SP-106: Predictive Signal Mode Classification
// BR-AUDIT-002: Comprehensive audit event emission
//
// # Design Decisions
//
// ADR-054: Predictive Signal Mode Classification and Prompt Strategy
// DD-TESTING-001: Audit Event Validation Standards
//
// # Test Infrastructure
//
// Uses envtest (real Kubernetes API server) per test plan requirements.
// SignalModeClassifier is wired into the reconciler via suite_test.go with a
// temp config file containing PredictedOOMKill→OOMKilled mappings.
package signalprocessing

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
)

var _ = Describe("Signal Mode Classification Integration Tests", Label("integration", "signalmode", "signalprocessing"), func() {
	var (
		namespace string
	)

	BeforeEach(func() {
		// ctx, k8sClient, dsClient, auditStore are package-level variables from suite_test.go

		// PARALLEL-SAFE: Unique namespace per test execution
		testID := uuid.New().String()[:8]
		namespace = fmt.Sprintf("sp-signalmode-%d-%s",
			GinkgoParallelProcess(), testID)

		// Create test namespace with environment label for classification
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
				Labels: map[string]string{
					"kubernaut.ai/environment": "production",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		// CLEANUP: Defer namespace deletion for parallel safety
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
		})
	})

	// ========================================
	// TEST SUITE 1: Predictive Signal Classification (CRD Status)
	// Business Context: Predictive signals must be classified and normalized
	// ========================================

	Context("IT-SP-106-001: Predictive Signal Mode Classification", func() {
		It("should classify PredictedOOMKill as predictive and normalize to OOMKilled", func() {
			// BUSINESS CONTEXT:
			// BR-SP-106: Predictive signals from Prometheus predict_linear() use
			// "Predicted" prefix. SP must classify these as predictive and normalize
			// the signal type for downstream workflow catalog matching.
			//
			// ADR-054: Separation of concerns — SP normalizes, HAPI adapts prompt.
			//
			// DATA FLOW: SP.Spec.Signal.Type="PredictedOOMKill"
			//   → Status.SignalMode="predictive"
			//   → Status.SignalType="OOMKilled" (normalized for workflow catalog)
			//   → Status.OriginalSignalType="PredictedOOMKill" (SOC2 audit trail)

			// GIVEN: SignalProcessing with predictive signal type
			sp := createSignalModeTestCRD(namespace, "test-predictive-oomkill")
			sp.Spec.Signal.Type = "PredictedOOMKill"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// WHEN: Controller reconciles and classifies signal mode
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())

				// THEN: Signal mode is predictive
				g.Expect(updated.Status.SignalMode).To(Equal("predictive"),
					"PredictedOOMKill should be classified as predictive signal mode")

				// THEN: Signal type is normalized for workflow catalog matching
				g.Expect(updated.Status.SignalType).To(Equal("OOMKilled"),
					"PredictedOOMKill should be normalized to OOMKilled for workflow catalog")

				// THEN: Original signal type is preserved for SOC2 audit trail
				g.Expect(updated.Status.OriginalSignalType).To(Equal("PredictedOOMKill"),
					"Original signal type must be preserved for SOC2 CC7.4 audit trail")

				// THEN: SP reaches Completed phase (full pipeline works with predictive signals)
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted),
					"SP should complete successfully with predictive signal")
			}, "30s", "1s").Should(Succeed())

			// BUSINESS OUTCOME VERIFIED:
			// - RO reads Status.SignalType="OOMKilled" → correct workflow catalog match
			// - RO reads Status.SignalMode="predictive" → passes to AA for prompt adaptation
			// - SOC2 auditors see OriginalSignalType="PredictedOOMKill" for traceability
		})

		It("should classify reactive signals with default mode and unchanged type", func() {
			// BUSINESS CONTEXT:
			// BR-SP-106: Signals not in the predictive mappings config default to
			// reactive mode. The signal type passes through unchanged.
			//
			// This is the backwards-compatible path — all existing signals work
			// exactly as before without any configuration.

			// GIVEN: SignalProcessing with standard reactive signal type
			sp := createSignalModeTestCRD(namespace, "test-reactive-oomkilled")
			sp.Spec.Signal.Type = "OOMKilled"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// WHEN: Controller reconciles
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())

				// THEN: Signal mode defaults to reactive
				g.Expect(updated.Status.SignalMode).To(Equal("reactive"),
					"OOMKilled should be classified as reactive (not in predictive mappings)")

				// THEN: Signal type is unchanged (no normalization needed)
				g.Expect(updated.Status.SignalType).To(Equal("OOMKilled"),
					"Reactive signal type should pass through unchanged")

				// THEN: Original signal type is empty (no normalization occurred)
				g.Expect(updated.Status.OriginalSignalType).To(BeEmpty(),
					"OriginalSignalType should be empty for reactive signals (no normalization)")

				// THEN: SP reaches Completed phase
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted),
					"SP should complete successfully with reactive signal")
			}, "30s", "1s").Should(Succeed())

			// BUSINESS OUTCOME: Existing reactive signals continue working unchanged
		})

		It("should classify PredictedCPUThrottling as predictive and normalize to CPUThrottling", func() {
			// BUSINESS CONTEXT:
			// Validates that multiple predictive signal mappings work, not just OOMKill.
			// Config has: PredictedCPUThrottling → CPUThrottling

			// GIVEN: SignalProcessing with a different predictive signal type
			sp := createSignalModeTestCRD(namespace, "test-predictive-cpu")
			sp.Spec.Signal.Type = "PredictedCPUThrottling"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// WHEN: Controller reconciles
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())

				// THEN: Classified as predictive with correct normalization
				g.Expect(updated.Status.SignalMode).To(Equal("predictive"),
					"PredictedCPUThrottling should be classified as predictive")
				g.Expect(updated.Status.SignalType).To(Equal("CPUThrottling"),
					"PredictedCPUThrottling should normalize to CPUThrottling")
				g.Expect(updated.Status.OriginalSignalType).To(Equal("PredictedCPUThrottling"),
					"Original type preserved for audit trail")
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			}, "30s", "1s").Should(Succeed())
		})

		It("should classify an unknown signal type as reactive", func() {
			// BUSINESS CONTEXT:
			// Unknown signal types that are not in the predictive mappings config
			// default to reactive. This is the safe default — new signal types
			// work out-of-the-box without needing config updates.

			// GIVEN: SignalProcessing with an unknown signal type not in any mapping
			sp := createSignalModeTestCRD(namespace, "test-unknown-type")
			sp.Spec.Signal.Type = "CustomAlertType"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// WHEN: Controller reconciles
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())

				// THEN: Defaults to reactive with type unchanged
				g.Expect(updated.Status.SignalMode).To(Equal("reactive"),
					"Unknown signal types should default to reactive")
				g.Expect(updated.Status.SignalType).To(Equal("CustomAlertType"),
					"Unknown signal type should pass through unchanged")
				g.Expect(updated.Status.OriginalSignalType).To(BeEmpty(),
					"No original type for reactive signals")
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			}, "30s", "1s").Should(Succeed())
		})
	})

	// ========================================
	// TEST SUITE 2: Audit Event Integration
	// Business Context: Signal mode decisions must be auditable
	// ========================================

	Context("IT-SP-106-001: Signal Mode Audit Events (DD-TESTING-001)", func() {
		It("should include signal_mode=predictive in classification.decision audit event", func() {
			// BUSINESS CONTEXT:
			// SOC2 CC7.4 requires complete audit trail for signal classification decisions.
			// Audit events must include signal_mode and original_signal_type for
			// predictive signals to demonstrate proper signal normalization.
			//
			// COMPLIANCE: Audit trail shows "PredictedOOMKill → predictive mode, normalized to OOMKilled"

			// GIVEN: SignalProcessing with predictive signal type
			sp := createSignalModeTestCRD(namespace, "test-audit-predictive")
			sp.Spec.Signal.Type = "PredictedOOMKill"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// Get unique correlation ID for audit event query
			correlationID := sp.Spec.RemediationRequestRef.Name

			// WHEN: Controller classifies signal mode and emits audit event
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted),
					"SP must complete before checking audit events")
			}, "30s", "1s").Should(Succeed())

			// THEN: Audit event contains signal_mode and original_signal_type
			flushAuditStoreAndWait()

			Eventually(func(g Gomega) {
				count := countAuditEvents(spaudit.EventTypeClassificationDecision, correlationID)
				GinkgoWriter.Printf("[%s] classification.decision audit events: %d (correlation_id: %s)\n",
					time.Now().Format("15:04:05.000"), count, correlationID)
				g.Expect(count).To(BeNumerically(">=", 1),
					"Should have at least 1 classification.decision event")

				event, err := getLatestAuditEvent(spaudit.EventTypeClassificationDecision, correlationID)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(event).To(Not(BeNil()))

				payload := event.EventData.SignalProcessingAuditPayload

				// Validate signal_mode is set to predictive
				g.Expect(payload.SignalMode.IsSet()).To(BeTrue(),
					"Audit event must include signal_mode for SOC2 CC7.4 compliance")
				g.Expect(string(payload.SignalMode.Value)).To(Equal("predictive"),
					"Audit event signal_mode should be 'predictive' for PredictedOOMKill")

				// Validate original_signal_type is preserved
				g.Expect(payload.OriginalSignalType.IsSet()).To(BeTrue(),
					"Audit event must include original_signal_type for SOC2 CC7.4")
				g.Expect(payload.OriginalSignalType.Value).To(Equal("PredictedOOMKill"),
					"Audit event should preserve original signal type before normalization")
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// BUSINESS OUTCOME VERIFIED:
			// - SOC2 auditor can trace: "PredictedOOMKill → predictive mode"
			// - Original signal type preserved for incident investigation
		})

		It("should include signal_mode=reactive in classification.decision audit event for reactive signals", func() {
			// BUSINESS CONTEXT:
			// All signals should have signal_mode in audit trail, not just predictive.
			// Reactive signals should explicitly record "reactive" for completeness.

			// GIVEN: SignalProcessing with reactive signal type
			sp := createSignalModeTestCRD(namespace, "test-audit-reactive")
			sp.Spec.Signal.Type = "OOMKilled"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			correlationID := sp.Spec.RemediationRequestRef.Name

			// WHEN: Controller completes processing
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			}, "30s", "1s").Should(Succeed())

			// THEN: Audit event has signal_mode=reactive, no original_signal_type
			flushAuditStoreAndWait()

			Eventually(func(g Gomega) {
				count := countAuditEvents(spaudit.EventTypeClassificationDecision, correlationID)
				g.Expect(count).To(BeNumerically(">=", 1))

				event, err := getLatestAuditEvent(spaudit.EventTypeClassificationDecision, correlationID)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(event).To(Not(BeNil()))

				payload := event.EventData.SignalProcessingAuditPayload

				// Validate signal_mode is reactive
				g.Expect(payload.SignalMode.IsSet()).To(BeTrue(),
					"Audit event should include signal_mode for all signals")
				g.Expect(string(payload.SignalMode.Value)).To(Equal("reactive"),
					"Audit event signal_mode should be 'reactive' for standard OOMKilled")

				// Original signal type should NOT be set for reactive signals
				if payload.OriginalSignalType.IsSet() {
					g.Expect(payload.OriginalSignalType.Value).To(BeEmpty(),
						"Reactive signals should not have original_signal_type set")
				}
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// BUSINESS OUTCOME: Complete audit trail for all signal modes
		})

		It("should include signal_mode in signal.processed audit event for predictive signals", func() {
			// BUSINESS CONTEXT:
			// The final signal.processed event should also contain signal_mode
			// for end-to-end audit trail completeness.

			// GIVEN: SignalProcessing with predictive signal
			sp := createSignalModeTestCRD(namespace, "test-audit-processed")
			sp.Spec.Signal.Type = "PredictedOOMKill"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			correlationID := sp.Spec.RemediationRequestRef.Name

			// WHEN: SP completes
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			}, "30s", "1s").Should(Succeed())

			// THEN: signal.processed event includes signal_mode
			flushAuditStoreAndWait()

			Eventually(func(g Gomega) {
				count := countAuditEvents(spaudit.EventTypeSignalProcessed, correlationID)
				GinkgoWriter.Printf("[%s] signal.processed audit events: %d (correlation_id: %s)\n",
					time.Now().Format("15:04:05.000"), count, correlationID)
				g.Expect(count).To(BeNumerically(">=", 1),
					"Should have at least 1 signal.processed event")

				event, err := getLatestAuditEvent(spaudit.EventTypeSignalProcessed, correlationID)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(event).To(Not(BeNil()))

				payload := event.EventData.SignalProcessingAuditPayload

				g.Expect(payload.SignalMode.IsSet()).To(BeTrue(),
					"signal.processed event should include signal_mode")
				g.Expect(string(payload.SignalMode.Value)).To(Equal("predictive"),
					"signal.processed event should have predictive signal_mode")

				g.Expect(payload.OriginalSignalType.IsSet()).To(BeTrue(),
					"signal.processed event should include original_signal_type")
				g.Expect(payload.OriginalSignalType.Value).To(Equal("PredictedOOMKill"),
					"signal.processed should preserve original signal type")

				g.Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess),
					"signal.processed event should have success outcome")
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// BUSINESS OUTCOME: End-to-end audit trail for predictive signal processing
		})
	})

	// ========================================
	// TEST SUITE 3: Classification Message Condition
	// Business Context: Kubernetes conditions reflect signal mode
	// ========================================

	Context("IT-SP-106-001: Classification Condition Message", func() {
		It("should include signalMode=predictive in ClassificationComplete condition message", func() {
			// BUSINESS CONTEXT:
			// BR-SP-110: Kubernetes conditions should reflect all classification outcomes
			// including signal mode for operator visibility via kubectl.

			// GIVEN: SignalProcessing with predictive signal
			sp := createSignalModeTestCRD(namespace, "test-condition-predictive")
			sp.Spec.Signal.Type = "PredictedOOMKill"
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())

			// WHEN: Controller classifies
			Eventually(func(g Gomega) {
				var updated signalprocessingv1alpha1.SignalProcessing
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      sp.Name,
					Namespace: sp.Namespace,
				}, &updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))

				// THEN: ClassificationComplete condition includes signalMode info
				var classificationCondition *metav1.Condition
				for i := range updated.Status.Conditions {
					if updated.Status.Conditions[i].Type == "ClassificationComplete" {
						classificationCondition = &updated.Status.Conditions[i]
						break
					}
				}
				g.Expect(classificationCondition).ToNot(BeNil(),
					"ClassificationComplete condition should exist")
				g.Expect(classificationCondition.Message).To(ContainSubstring("signalMode=predictive"),
					"ClassificationComplete message should mention signalMode=predictive")
				g.Expect(classificationCondition.Message).To(ContainSubstring("PredictedOOMKill"),
					"ClassificationComplete message should mention original type")
				g.Expect(classificationCondition.Message).To(ContainSubstring("OOMKilled"),
					"ClassificationComplete message should mention normalized type")
			}, "30s", "1s").Should(Succeed())
		})
	})
})

// ========================================
// TEST HELPERS
// ========================================

// createSignalModeTestCRD creates a SignalProcessing CRD for signal mode integration tests.
// Uses unique naming per test for parallel execution safety.
// Signal.Type should be overridden by each test case.
func createSignalModeTestCRD(namespace, name string) *signalprocessingv1alpha1.SignalProcessing {
	// Generate unique RR name with timestamp to avoid stale audit event collisions
	// Per DD-AUDIT-CORRELATION-001: RR name must be unique per remediation flow
	timestamp := time.Now().UnixNano()
	rrName := fmt.Sprintf("%s-rr-%d", name, timestamp)

	return &signalprocessingv1alpha1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: signalprocessingv1alpha1.SignalProcessingSpec{
			RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
				Name:      rrName,
				Namespace: namespace,
			},
			Signal: signalprocessingv1alpha1.SignalData{
				Fingerprint:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", // Valid 64-char hex fingerprint
				Name:         "TestPredictiveAlert",
				Severity:     "critical",
				Type:         "prometheus", // Overridden by each test case
				Source:       "test-source",
				TargetType:   "kubernetes",
				ReceivedTime: metav1.Now(),
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: namespace,
				},
			},
		},
	}
}
