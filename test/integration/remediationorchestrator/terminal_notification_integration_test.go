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

// Business Requirement: BR-ORCH-029, BR-ORCH-030, BR-ORCH-035
// Purpose: Validates Issue #88 fix — notification tracking in terminal-phase RRs
// with real Kubernetes API (envtest)

package remediationorchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Issue #88: NotificationRequest completion events lost for terminal-phase RemediationRequests
var _ = Describe("Issue #88: Terminal-Phase Notification Tracking Integration", Label("integration", "issue-88"), func() {
	var (
		testNamespace string
	)

	BeforeEach(func() {
		testNamespace = createTestNamespace("ro-terminal-notif")
	})

	AfterEach(func() {
		deleteTestNamespace(testNamespace)
	})

	// IT-RO-088-001: Terminal (Completed) RR should track NT delivery via real K8s reconcile
	It("IT-RO-088-001: should set NotificationDelivered=True when NT reaches Sent on a Completed RR", func() {
		rrName := fmt.Sprintf("rr-term-sent-%d", time.Now().UnixNano())
		notifName := fmt.Sprintf("nr-completion-%s", rrName)

		By("Creating a RemediationRequest")
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: testNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: func() string {
					h := sha256.Sum256([]byte(uuid.New().String()))
					return hex.EncodeToString(h[:])
				}(),
				SignalName: "TerminalNotifTest",
				Severity:   "warning",
				SignalType: "prometheus",
				TargetType: "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: testNamespace,
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		By("Waiting for controller to fully process the RR (reach Processing phase)")
		// RC-11 FIX: Wait for Processing phase with ObservedGeneration matching Generation.
		// Previously we waited only for OverallPhase != "" (catches Pending), but the
		// controller was still mid-reconciliation (handlePendingPhase). When we then
		// manually set Completed, the controller's requeued handlePendingPhase would
		// overwrite it back to Processing, causing a phase regression.
		Eventually(func() bool {
			err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			if err != nil {
				return false
			}
			return rr.Status.OverallPhase == remediationv1.PhaseProcessing &&
				rr.Status.ObservedGeneration == rr.Generation
		}, timeout, interval).Should(BeTrue(),
			"RR should reach Processing phase with ObservedGeneration matching Generation")

		By("Manually transitioning RR to Completed phase (simulating end of lifecycle)")
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
				return err
			}
			rr.Status.OverallPhase = remediationv1.PhaseCompleted
			rr.Status.ObservedGeneration = rr.Generation
			return k8sClient.Status().Update(ctx, rr)
		}, timeout, interval).Should(Succeed())

		By("Creating a NotificationRequest owned by the terminal RR")
		notif := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notifName,
				Namespace: testNamespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeCompletion,
				Priority: notificationv1.NotificationPriorityLow,
				Subject:  "Remediation Completed",
				Body:     "Test completion notification for terminal tracking",
				Metadata: map[string]string{
					"remediationRequest": rrName,
				},
			},
		}
		// Refresh RR before setting owner reference (need current resourceVersion)
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		Expect(controllerutil.SetControllerReference(rr, notif, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, notif)).To(Succeed())

		By("Adding NT ref to RR status (BR-ORCH-035: NotificationRequestRefs)")
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
				return err
			}
			rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, corev1.ObjectReference{
				APIVersion: notificationv1.GroupVersion.String(),
				Kind:       "NotificationRequest",
				Name:       notif.Name,
				Namespace:  notif.Namespace,
			})
			return k8sClient.Status().Update(ctx, rr)
		}, timeout, interval).Should(Succeed())

		By("Updating NotificationRequest to Sent phase (delivery succeeded)")
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(notif), notif); err != nil {
				return err
			}
			notif.Status.Phase = notificationv1.NotificationPhaseSent
			notif.Status.Message = "Notification delivered successfully"
			return k8sClient.Status().Update(ctx, notif)
		}, timeout, interval).Should(Succeed())

		By("Verifying NotificationDelivered condition is set on terminal RR")
		// This is the key assertion: before Issue #88 fix, Guard1 would skip
		// the reconcile and NotificationDelivered would never be set.
		Eventually(func() *metav1.Condition {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
				return nil
			}
			return meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
		}, timeout, interval).ShouldNot(BeNil())

		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal("DeliverySucceeded"))

		By("Verifying RR remains in Completed phase (no phase regression)")
		Consistently(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, 2*time.Second, 250*time.Millisecond).Should(Equal(remediationv1.PhaseCompleted),
			"Terminal phase must remain stable after notification tracking")
	})

	// IT-RO-088-002: Terminal (Failed) RR should track NT delivery failure
	It("IT-RO-088-002: should set NotificationDelivered=False when NT reaches Failed on a Failed RR", func() {
		rrName := fmt.Sprintf("rr-term-fail-%d", time.Now().UnixNano())
		notifName := fmt.Sprintf("timeout-%s", rrName)

		By("Creating a RemediationRequest")
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rrName,
				Namespace: testNamespace,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: func() string {
					h := sha256.Sum256([]byte(uuid.New().String()))
					return hex.EncodeToString(h[:])
				}(),
				SignalName: "TerminalNotifFailTest",
				Severity:   "critical",
				SignalType: "prometheus",
				TargetType: "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: testNamespace,
				},
				FiringTime:   now,
				ReceivedTime: now,
				Deduplication: sharedtypes.DeduplicationInfo{
					FirstOccurrence: now,
					LastOccurrence:  now,
					OccurrenceCount: 1,
				},
			},
		}
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		By("Waiting for controller to fully process the RR (reach Processing phase)")
		// RC-11 FIX: Same as IT-RO-088-001 - wait for Processing phase to avoid race condition
		Eventually(func() bool {
			err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			if err != nil {
				return false
			}
			return rr.Status.OverallPhase == remediationv1.PhaseProcessing &&
				rr.Status.ObservedGeneration == rr.Generation
		}, timeout, interval).Should(BeTrue(),
			"RR should reach Processing phase with ObservedGeneration matching Generation")

		By("Manually transitioning RR to Failed phase")
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
				return err
			}
			rr.Status.OverallPhase = remediationv1.PhaseFailed
			rr.Status.ObservedGeneration = rr.Generation
			return k8sClient.Status().Update(ctx, rr)
		}, timeout, interval).Should(Succeed())

		By("Creating a NotificationRequest owned by the terminal RR")
		notif := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notifName,
				Namespace: testNamespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeEscalation,
				Priority: notificationv1.NotificationPriorityHigh,
				Subject:  "Remediation Timed Out",
				Body:     "Test timeout notification",
				Metadata: map[string]string{
					"remediationRequest": rrName,
				},
			},
		}
		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		Expect(controllerutil.SetControllerReference(rr, notif, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, notif)).To(Succeed())

		By("Adding NT ref to RR status")
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
				return err
			}
			rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, corev1.ObjectReference{
				APIVersion: notificationv1.GroupVersion.String(),
				Kind:       "NotificationRequest",
				Name:       notif.Name,
				Namespace:  notif.Namespace,
			})
			return k8sClient.Status().Update(ctx, rr)
		}, timeout, interval).Should(Succeed())

		By("Updating NotificationRequest to Failed phase (delivery failed)")
		failureMessage := "SMTP connection refused"
		Eventually(func() error {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(notif), notif); err != nil {
				return err
			}
			notif.Status.Phase = notificationv1.NotificationPhaseFailed
			notif.Status.Message = failureMessage
			return k8sClient.Status().Update(ctx, notif)
		}, timeout, interval).Should(Succeed())

		By("Verifying NotificationDelivered=False condition is set on terminal RR")
		Eventually(func() *metav1.Condition {
			if err := k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
				return nil
			}
			return meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
		}, timeout, interval).ShouldNot(BeNil())

		Expect(k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("DeliveryFailed"))
		Expect(cond.Message).To(ContainSubstring(failureMessage))

		By("Verifying RR remains in Failed phase (no phase regression)")
		Consistently(func() remediationv1.RemediationPhase {
			_ = k8sManager.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(rr), rr)
			return rr.Status.OverallPhase
		}, 2*time.Second, 250*time.Millisecond).Should(Equal(remediationv1.PhaseFailed))
	})
})
