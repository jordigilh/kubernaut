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

package remediationorchestrator

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ============================================================================
// Issue #265: RETENTION TTL INTEGRATION TESTS
// Validates the CRD retention lifecycle with real Kubernetes API (envtest).
// ============================================================================

var _ = Describe("Issue #265: CRD Retention TTL Enforcement", Label("integration", "retention", "265"), func() {

	Context("RetentionExpiryTime lifecycle", func() {
		var (
			namespace string
			rrName    string
		)

		BeforeEach(func() {
			namespace = createTestNamespace("ro-retention")
			rrName = fmt.Sprintf("rr-ret-%s", uuid.New().String()[:8])
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("IT-RO-265-001: terminal RR should have RetentionExpiryTime set by reconciler", func() {
			By("Creating a RemediationRequest")
			rr := createRemediationRequest(namespace, rrName)

			By("Waiting for RR to transition to Processing (SP created)")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			By("Finding the SP CRD created by RO")
			var spName string
			Eventually(func() bool {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, updated); err != nil {
					return false
				}
				if updated.Status.SignalProcessingRef != nil {
					spName = updated.Status.SignalProcessingRef.Name
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Completing SP to trigger transition to Analyzing")
			Expect(updateSPStatus(ROControllerNamespace, spName, signalprocessingv1.PhaseCompleted)).To(Succeed())

			By("Waiting for RR to reach Analyzing phase")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseAnalyzing))

			By("Waiting for global timeout to trigger terminal phase (TimedOut)")
			// The default global timeout is 1h in integration tests.
			// Instead of waiting, force the RR to terminal by directly updating status.
			// This simulates what happens when the reconciler detects the RR is terminal.
			updated := &remediationv1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, updated)).To(Succeed())

			// Force StartTime far in the past to trigger global timeout on next reconcile
			pastTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
			updated.Status.StartTime = &pastTime
			Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

			By("Waiting for RR to reach terminal phase (TimedOut)")
			Eventually(func() bool {
				rr := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr); err != nil {
					return false
				}
				return rr.Status.OverallPhase == remediationv1.PhaseTimedOut ||
					rr.Status.OverallPhase == remediationv1.PhaseFailed
			}, timeout, interval).Should(BeTrue())

			By("Verifying RetentionExpiryTime is set on terminal RR")
			Eventually(func() bool {
				rr := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr); err != nil {
					return false
				}
				return rr.Status.RetentionExpiryTime != nil
			}, timeout, interval).Should(BeTrue(),
				"Behavior: RetentionExpiryTime must be set on terminal RR within one reconcile cycle")

			By("Verifying CompletedAt is set on terminal RR (#265 F3)")
			rr = &remediationv1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, rr)).To(Succeed())
			Expect(rr.Status.CompletedAt).NotTo(BeNil(),
				"Behavior: CompletedAt must be set on terminal phase (F3 fix)")

			GinkgoWriter.Printf("✅ IT-RO-265-001: RetentionExpiryTime=%s, CompletedAt=%s\n",
				rr.Status.RetentionExpiryTime.Time.Format(time.RFC3339),
				rr.Status.CompletedAt.Time.Format(time.RFC3339))
		})

		It("IT-RO-265-002: RR with expired RetentionExpiryTime should be deleted", func() {
			By("Creating a RemediationRequest")
			rr := createRemediationRequest(namespace, rrName)

			By("Waiting for RR to be initialized (Processing phase)")
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing))

			By("Forcing RR to terminal phase with already-expired RetentionExpiryTime")
			updated := &remediationv1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, updated)).To(Succeed())

			now := metav1.Now()
			expired := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			failPhase := remediationv1.FailurePhaseConfiguration
			failReason := "test-forced-failure"
			updated.Status.OverallPhase = remediationv1.PhaseFailed
			updated.Status.CompletedAt = &now
			updated.Status.FailurePhase = &failPhase
			updated.Status.FailureReason = &failReason
			updated.Status.RetentionExpiryTime = &expired
			Expect(k8sClient.Status().Update(ctx, updated)).To(Succeed())

			By("Waiting for CRD to be deleted by reconciler")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: ROControllerNamespace}, &remediationv1.RemediationRequest{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(),
				"Behavior: expired CRD must be deleted from cluster by reconciler")

			GinkgoWriter.Printf("✅ IT-RO-265-002: Expired RR %s deleted\n", rrName)
		})
	})
})
