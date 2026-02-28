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

package processing

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// ============================================================================
// INTEGRATION TESTS: ShouldDeduplicate with Real K8s Field Selectors
// ============================================================================
//
// PURPOSE: Validate ShouldDeduplicate function works correctly with real
//          Kubernetes field selectors (not testable with fake clients)
//
// BUSINESS VALUE:
// - BR-GATEWAY-185: Efficient deduplication queries via field selectors
// - DD-GATEWAY-011: Phase-based deduplication using RR status
// - Production behavior validation (field selectors require real K8s API)
//
// COVERAGE: This tests the PRIMARY code path (field selectors), not the
//           fallback path that unit tests cover
// ============================================================================

// Helper function to create test RemediationRequest with valid CRD fields
func createTestRR(name, namespace, fingerprintSeed, alertName, severity, phase string, kind, resourceName string) *remediationv1alpha1.RemediationRequest {
	now := metav1.Now()
	// Generate valid 64-char hex fingerprint (only 0-9a-f allowed)
	hexSeed := ""
	for _, c := range fingerprintSeed {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			hexSeed += string(c)
		}
	}
	fingerprint := hexSeed
	for len(fingerprint) < 64 {
		fingerprint += "0"
	}
	if len(fingerprint) > 64 {
		fingerprint = fingerprint[:64]
	}

	return &remediationv1alpha1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1, // Required for ObservedGeneration pattern
		},
		Spec: remediationv1alpha1.RemediationRequestSpec{
			SignalFingerprint: fingerprint,
			SignalName:        alertName,
			Severity:          severity,
			SignalType:        "alert", // ✅ ADAPTER-CONSTANT: All adapters normalize to SourceTypeAlert
			SignalSource:      "alertmanager",
			FiringTime:        now,
			ReceivedTime:      now,
			TargetType:        "kubernetes",
			TargetResource: remediationv1alpha1.ResourceIdentifier{
				Kind: kind,
				Name: resourceName,
			},
		},
		Status: remediationv1alpha1.RemediationRequestStatus{
			OverallPhase: remediationv1alpha1.RemediationPhase(phase),
		},
	}
}

var _ = Describe("BR-GATEWAY-185: ShouldDeduplicate with Field Selectors", func() {
	var (
		ctx           context.Context
		phaseChecker  *processing.PhaseBasedDeduplicationChecker
		testNamespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = controllerNamespace // ADR-057: RRs live in controller namespace

		phaseChecker = processing.NewPhaseBasedDeduplicationChecker(k8sClient, 0)

		// Clean up existing test RRs in controller namespace
		rrList := &remediationv1alpha1.RemediationRequestList{}
		err := k8sClient.List(ctx, rrList, client.InNamespace(controllerNamespace))
		Expect(err).ToNot(HaveOccurred())

		for i := range rrList.Items {
			_ = k8sClient.Delete(ctx, &rrList.Items[i])
		}

		// Wait for deletions to propagate
		Eventually(func() int {
			list := &remediationv1alpha1.RemediationRequestList{}
			_ = k8sClient.List(ctx, list, client.InNamespace(controllerNamespace))
			return len(list.Items)
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(0))
	})

	Context("when no RemediationRequest exists for fingerprint", func() {
		It("returns false (create new RR)", func() {
			fingerprint := "0000000000000000000000000000000000000000000000000000000000000001"

			shouldDedup, existingRR, err := phaseChecker.ShouldDeduplicate(ctx, testNamespace, fingerprint)

			Expect(err).ToNot(HaveOccurred())
			Expect(shouldDedup).To(BeFalse())
			Expect(existingRR).To(BeNil())
		})
	})

	Context("when RemediationRequest exists in Pending phase", func() {
		It("returns true with existing RR (update dedup status)", func() {
			fingerprint := "abc123"

			rr := createTestRR("test-rr-pending", controllerNamespace, fingerprint, "TestAlert", "critical", "Pending", "Pod", "test-pod")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Update status subresource
			rr.Status.OverallPhase = "Pending"
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			// Wait for field selector to work (cache must index the object)
			var shouldDedup bool
			var existingRR *remediationv1alpha1.RemediationRequest
			var err error
			Eventually(func() bool {
				shouldDedup, existingRR, err = phaseChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
				return err == nil && shouldDedup && existingRR != nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Field selector should find Pending RR")

			Expect(existingRR.Name).To(Equal("test-rr-pending"))
			Expect(existingRR.Spec.SignalFingerprint).To(Equal(rr.Spec.SignalFingerprint))
		})
	})

	Context("when RemediationRequest exists in Processing phase", func() {
		It("returns true with existing RR (update dedup status)", func() {
			fingerprint := "def456"

			rr := createTestRR("test-rr-processing", controllerNamespace, fingerprint, "ActiveAlert", "warning", "Processing", "Deployment", "test-deploy")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			rr.Status.OverallPhase = "Processing"
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			var shouldDedup bool
			var existingRR *remediationv1alpha1.RemediationRequest
			Eventually(func() bool {
				var err error
				shouldDedup, existingRR, err = phaseChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
				return err == nil && shouldDedup && existingRR != nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			Expect(existingRR.Name).To(Equal("test-rr-processing"))
		})
	})

	Context("when RemediationRequest exists in Completed phase", func() {
		It("returns false (allow new RR for same problem)", func() {
			fingerprint := "abc789"

			rr := createTestRR("test-rr-completed", controllerNamespace, fingerprint, "RecurringAlert", "info", "Pending", "Pod", "recurring-pod")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			// Update status to Completed (terminal phase)
			rr.Status.OverallPhase = remediationv1alpha1.PhaseCompleted
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			// Wait for status update to propagate to cache, then check deduplication
			Eventually(func() bool {
				var fetchedRR remediationv1alpha1.RemediationRequest
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: rr.Name, Namespace: controllerNamespace}, &fetchedRR); err != nil {
					return false
				}
				return fetchedRR.Status.OverallPhase == remediationv1alpha1.PhaseCompleted
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "Status should be updated to Completed")

			// Now check deduplication - should return false for terminal phase
			var shouldDedup bool
			var existingRR *remediationv1alpha1.RemediationRequest
			Eventually(func() error {
				var err error
				shouldDedup, existingRR, err = phaseChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
				return err
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			Expect(shouldDedup).To(BeFalse(),
				"Completed RR is terminal - should allow new RR")
			Expect(existingRR).To(BeNil())
		})
	})

	Context("when RemediationRequest exists in Failed phase", func() {
		It("returns false (allow retry)", func() {
			fingerprint := "failed123"

			rr := createTestRR("test-rr-failed", controllerNamespace, fingerprint, "FailedAlert", "critical", "Failed", "Pod", "failed-pod")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			rr.Status.OverallPhase = "Failed"
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			// Wait for terminal phase to be recognized (shouldDedup = false)
			var shouldDedup bool
			var existingRR *remediationv1alpha1.RemediationRequest
			Eventually(func() bool {
				var err error
				shouldDedup, existingRR, err = phaseChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
				// Wait for: no error AND shouldDedup is false (terminal phase)
				return err == nil && !shouldDedup && existingRR == nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Failed phase should be terminal (shouldDedup=false)")
		})
	})

	Context("when RemediationRequest exists in Blocked phase", func() {
		It("returns true with existing RR (update dedup status during cooldown)", func() {
			fingerprint := "blocked456"

			rr := createTestRR("test-rr-blocked", controllerNamespace, fingerprint, "CooldownAlert", "warning", "Blocked", "Pod", "cooldown-pod")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			rr.Status.OverallPhase = "Blocked"
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			var shouldDedup bool
			var existingRR *remediationv1alpha1.RemediationRequest
			Eventually(func() bool {
				var err error
				shouldDedup, existingRR, err = phaseChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
				return err == nil && shouldDedup && existingRR != nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			Expect(existingRR.Name).To(Equal("test-rr-blocked"))
		})
	})

	Context("when multiple RemediationRequests exist with different fingerprints", func() {
		It("returns only RR matching the exact fingerprint", func() {
			targetFingerprint := "aaaa"
			otherFingerprint1 := "bbbb"
			otherFingerprint2 := "cccc"

			rr1 := createTestRR("test-rr-other-1", controllerNamespace, otherFingerprint1, "OtherAlert1", "info", "Pending", "Pod", "other-pod-1")
			Expect(k8sClient.Create(ctx, rr1)).To(Succeed())
			rr1.Status.OverallPhase = "Pending"
			Expect(k8sClient.Status().Update(ctx, rr1)).To(Succeed())

			rr2 := createTestRR("test-rr-target", controllerNamespace, targetFingerprint, "TargetAlert", "critical", "Processing", "Pod", "target-pod")
			Expect(k8sClient.Create(ctx, rr2)).To(Succeed())
			rr2.Status.OverallPhase = "Processing"
			Expect(k8sClient.Status().Update(ctx, rr2)).To(Succeed())

			rr3 := createTestRR("test-rr-other-2", controllerNamespace, otherFingerprint2, "OtherAlert2", "warning", "Pending", "Pod", "other-pod-2")
			Expect(k8sClient.Create(ctx, rr3)).To(Succeed())
			rr3.Status.OverallPhase = "Pending"
			Expect(k8sClient.Status().Update(ctx, rr3)).To(Succeed())

			// Wait for all RRs to be indexed
			Eventually(func() int {
				list := &remediationv1alpha1.RemediationRequestList{}
				_ = k8sClient.List(ctx, list, client.InNamespace(controllerNamespace))
				return len(list.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(3))

			// Query with field selector for target fingerprint only
			var shouldDedup bool
			var existingRR *remediationv1alpha1.RemediationRequest
			Eventually(func() bool {
				var err error
				shouldDedup, existingRR, err = phaseChecker.ShouldDeduplicate(ctx, testNamespace, rr2.Spec.SignalFingerprint)
				return err == nil && shouldDedup && existingRR != nil && existingRR.Name == "test-rr-target"
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Field selector should return only matching fingerprint")

			Expect(existingRR.Spec.SignalFingerprint).To(Equal(rr2.Spec.SignalFingerprint))
		})
	})

	// ========================================================================
	// Post-Completion Cooldown Tests (Issue #202)
	// Test Plan: docs/testing/COOLDOWN_GW_RO/TEST_PLAN.md
	//
	// BUSINESS VALUE:
	// - Validates cooldown with real K8s field selectors (primary code path)
	// - Proves CompletedAt timestamp comparison works against envtest API
	// ========================================================================

	Context("IT-GW-011-001: Completed RR within cooldown triggers dedup (#202)", func() {
		It("should deduplicate when Completed RR has CompletedAt within cooldown window", func() {
			// BUSINESS OUTCOME: Alert re-fires 2 minutes after a successful fix.
			// The Gateway must not create a new RR -- it would waste an LLM call
			// on stale signal data from before the remediation.
			fingerprint := "cd011001"
			cooldownChecker := processing.NewPhaseBasedDeduplicationChecker(k8sClient, 5*time.Minute)

			rr := createTestRR("test-rr-cooldown-active", controllerNamespace, fingerprint,
				"CooldownActiveAlert", "warning", "Pending", "Deployment", "api-frontend")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			completedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			rr.Status.OverallPhase = remediationv1alpha1.PhaseCompleted
			rr.Status.CompletedAt = &completedAt
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			Eventually(func() bool {
				shouldDedup, existingRR, err := cooldownChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
				return err == nil && shouldDedup && existingRR != nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Completed RR within 5-min cooldown must trigger dedup via real field selector")
		})
	})

	Context("IT-GW-011-002: Completed RR outside cooldown allows new RR (#202)", func() {
		It("should allow new RR when Completed RR has CompletedAt beyond cooldown window", func() {
			// BUSINESS OUTCOME: Alert re-fires 6 minutes after a successful fix.
			// Cooldown has expired, so this is likely a genuine recurrence.
			// Gateway allows a new RR so the pipeline processes fresh signal data.
			fingerprint := "cd011002"
			cooldownChecker := processing.NewPhaseBasedDeduplicationChecker(k8sClient, 5*time.Minute)

			rr := createTestRR("test-rr-cooldown-expired", controllerNamespace, fingerprint,
				"CooldownExpiredAlert", "warning", "Pending", "Deployment", "api-frontend-2")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			completedAt := metav1.NewTime(time.Now().Add(-6 * time.Minute))
			rr.Status.OverallPhase = remediationv1alpha1.PhaseCompleted
			rr.Status.CompletedAt = &completedAt
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			Eventually(func() bool {
				var fetchedRR remediationv1alpha1.RemediationRequest
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: rr.Name, Namespace: controllerNamespace}, &fetchedRR); err != nil {
					return false
				}
				return fetchedRR.Status.OverallPhase == remediationv1alpha1.PhaseCompleted
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			shouldDedup, existingRR, err := cooldownChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
			Expect(err).ToNot(HaveOccurred())
			Expect(shouldDedup).To(BeFalse(),
				"Completed RR outside cooldown must allow new RR creation")
			Expect(existingRR).To(BeNil())
		})
	})

	Context("when RemediationRequest exists in Cancelled phase", func() {
		It("returns false (allow retry after manual cancellation)", func() {
			fingerprint := "cancelled789"

			rr := createTestRR("test-rr-cancelled", controllerNamespace, fingerprint, "CancelledAlert", "info", "Cancelled", "Pod", "cancelled-pod")
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			rr.Status.OverallPhase = "Cancelled"
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			// Wait for terminal phase to be recognized (shouldDedup = false)
			var shouldDedup bool
			var existingRR *remediationv1alpha1.RemediationRequest
			Eventually(func() bool {
				var err error
				shouldDedup, existingRR, err = phaseChecker.ShouldDeduplicate(ctx, testNamespace, rr.Spec.SignalFingerprint)
				// Wait for: no error AND shouldDedup is false (terminal phase)
				return err == nil && !shouldDedup && existingRR == nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Cancelled phase should be terminal (shouldDedup=false)")
		})
	})
})
