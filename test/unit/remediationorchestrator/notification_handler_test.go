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

package remediationorchestrator

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
)

// Business Requirement: BR-ORCH-029, BR-ORCH-030, BR-ORCH-031, BR-ORCH-034
// Purpose: Validates notification lifecycle tracking implementation mechanics

var _ = Describe("NotificationHandler", func() {
	var (
		handler    *controller.NotificationHandler
		fakeClient client.Client
		ctx        context.Context
		rr         *remediationv1.RemediationRequest
		notif      *notificationv1.NotificationRequest
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create scheme with all types
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		// Create fake client
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		// Create handler
		handler = controller.NewNotificationHandler(fakeClient, nil)

		// Create test RemediationRequest
		rr = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rr",
				Namespace: "default",
				UID:       "test-uid-123",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd",
				SignalName:        "HighMemoryUsage",
				Severity:          "critical",
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseAnalyzing,
				StartTime:    &metav1.Time{Time: time.Now()},
			},
		}

		// Create test NotificationRequest
		notif = &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-notif",
				Namespace: "default",
				UID:       "notif-uid-456",
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeApproval,
				Priority: notificationv1.NotificationPriorityHigh,
				Subject:  "Approval Required",
				Body:     "Test notification",
			},
			Status: notificationv1.NotificationRequestStatus{
				Phase: notificationv1.NotificationPhasePending,
			},
		}

		// Add notification reference to RR
		rr.Status.NotificationRequestRefs = []corev1.ObjectReference{
			{
				APIVersion: "notification.kubernaut.ai/v1alpha1",
				Kind:       "NotificationRequest",
				Name:       notif.Name,
				Namespace:  notif.Namespace,
				UID:        notif.UID,
			},
		}
	})

	Describe("HandleNotificationRequestDeletion", func() {
		Context("when NotificationRequest is deleted by user", func() {
			DescribeTable("should update notification status without changing phase",
				func(currentPhase remediationv1.RemediationPhase, expectedConditionReason string) {
					// Test: BR-ORCH-029 - User cancellation
					// CRITICAL: Verify overallPhase is UNCHANGED

					rr.Status.OverallPhase = currentPhase
					rr.DeletionTimestamp = nil // RR is NOT being deleted

					err := handler.HandleNotificationRequestDeletion(ctx, rr)

					Expect(err).ToNot(HaveOccurred())

					// Verify notification status updated
					Expect(rr.Status.NotificationStatus).To(Equal("Cancelled"))

					// CRITICAL: Verify phase UNCHANGED (remediation continues)
					Expect(rr.Status.OverallPhase).To(Equal(currentPhase))

					// Verify condition set
					cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(metav1.ConditionFalse))
					Expect(cond.Reason).To(Equal(expectedConditionReason))
				},
				Entry("BR-ORCH-029: During Analyzing phase", remediationv1.PhaseAnalyzing, "UserCancelled"),
				Entry("BR-ORCH-029: During Executing phase", remediationv1.PhaseExecuting, "UserCancelled"),
				Entry("BR-ORCH-029: During TimedOut phase", remediationv1.PhaseTimedOut, "UserCancelled"),
				Entry("BR-ORCH-029: During Failed phase", remediationv1.PhaseFailed, "UserCancelled"),
			)

			It("BR-ORCH-029: should set cancellation message", func() {
				rr.DeletionTimestamp = nil

				err := handler.HandleNotificationRequestDeletion(ctx, rr)

				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Status.Message).To(ContainSubstring("NotificationRequest deleted by user"))
			})
		})

		Context("when NotificationRequest is cascade deleted", func() {
			It("BR-ORCH-031: should not update notification status", func() {
				// Test: Cascade deletion (expected cleanup)
				rr.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				originalStatus := rr.Status.NotificationStatus
				originalPhase := rr.Status.OverallPhase

				err := handler.HandleNotificationRequestDeletion(ctx, rr)

				Expect(err).ToNot(HaveOccurred())

				// Status should not change
				Expect(rr.Status.NotificationStatus).To(Equal(originalStatus))
				Expect(rr.Status.OverallPhase).To(Equal(originalPhase))

				// No condition should be set
				cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
				Expect(cond).To(BeNil())
			})

			It("BR-ORCH-031: should handle gracefully without errors", func() {
				// Test: Cascade deletion should be silent (no errors, no logs)
				rr.DeletionTimestamp = &metav1.Time{Time: time.Now()}

				err := handler.HandleNotificationRequestDeletion(ctx, rr)

				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("UpdateNotificationStatus", func() {
		DescribeTable("should map NotificationRequest phase to RemediationRequest status",
			func(nrPhase notificationv1.NotificationPhase, expectedStatus string, expectedConditionStatus metav1.ConditionStatus, expectedConditionReason string) {
				// Test: BR-ORCH-030 - Status tracking
				notif.Status.Phase = nrPhase
				originalPhase := rr.Status.OverallPhase

				err := handler.UpdateNotificationStatus(ctx, rr, notif)

				Expect(err).ToNot(HaveOccurred())

				// Verify notification status updated
				Expect(rr.Status.NotificationStatus).To(Equal(expectedStatus))

				// CRITICAL: Verify phase UNCHANGED
				Expect(rr.Status.OverallPhase).To(Equal(originalPhase))

				// Verify condition (if expected)
				if expectedConditionReason != "" {
					cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
					Expect(cond).ToNot(BeNil())
					Expect(cond.Status).To(Equal(expectedConditionStatus))
					Expect(cond.Reason).To(Equal(expectedConditionReason))
				}
			},
			Entry("BR-ORCH-030: Pending phase",
				notificationv1.NotificationPhasePending,
				"Pending",
				metav1.ConditionUnknown,
				""),
			Entry("BR-ORCH-030: Sending phase",
				notificationv1.NotificationPhaseSending,
				"InProgress",
				metav1.ConditionUnknown,
				""),
			Entry("BR-ORCH-030: Sent phase",
				notificationv1.NotificationPhaseSent,
				"Sent",
				metav1.ConditionTrue,
				"DeliverySucceeded"),
			Entry("BR-ORCH-030: Failed phase",
				notificationv1.NotificationPhaseFailed,
				"Failed",
				metav1.ConditionFalse,
				"DeliveryFailed"),
		)

		Context("when notification delivery succeeds", func() {
			It("BR-ORCH-030: should set positive condition", func() {
				notif.Status.Phase = notificationv1.NotificationPhaseSent

				err := handler.UpdateNotificationStatus(ctx, rr, notif)

				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Status.NotificationStatus).To(Equal("Sent"))

				cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
				Expect(cond).ToNot(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
				Expect(cond.Message).To(ContainSubstring("successfully"))
			})
		})

		Context("when notification delivery fails", func() {
			It("BR-ORCH-030: should set failure condition with reason", func() {
				notif.Status.Phase = notificationv1.NotificationPhaseFailed
				notif.Status.Message = "SMTP connection timeout"

				err := handler.UpdateNotificationStatus(ctx, rr, notif)

				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Status.NotificationStatus).To(Equal("Failed"))

				cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
				Expect(cond).ToNot(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Message).To(ContainSubstring("SMTP connection timeout"))
			})
		})
	})

	Describe("Condition Management", func() {
		Context("when multiple condition updates occur", func() {
			It("BR-ORCH-029/030: should preserve condition history", func() {
				// First: Set delivered successfully
				notif.Status.Phase = notificationv1.NotificationPhaseSent
				err := handler.UpdateNotificationStatus(ctx, rr, notif)
				Expect(err).ToNot(HaveOccurred())

				firstTransitionTime := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered").LastTransitionTime

				// Wait a moment
				time.Sleep(10 * time.Millisecond)

				// Then: User cancels (simulated by deletion)
				rr.DeletionTimestamp = nil
				err = handler.HandleNotificationRequestDeletion(ctx, rr)
				Expect(err).ToNot(HaveOccurred())

				// Condition should be updated
				cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
				Expect(cond).ToNot(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				Expect(cond.Reason).To(Equal("UserCancelled"))

				// Transition time should change
				Expect(cond.LastTransitionTime).ToNot(Equal(firstTransitionTime))
			})
		})
	})

	Describe("Edge Cases", func() {
		Context("when RemediationRequest has no notification refs", func() {
			It("BR-ORCH-029: should handle gracefully", func() {
				rr.Status.NotificationRequestRefs = nil
				rr.DeletionTimestamp = nil

				err := handler.HandleNotificationRequestDeletion(ctx, rr)

				Expect(err).ToNot(HaveOccurred())
				// TDD REFACTOR (Day 2): With defensive programming, no status update when no refs
				// Status should remain unchanged (not set to "Cancelled")
				Expect(rr.Status.NotificationStatus).To(BeEmpty())
			})
		})

		Context("when NotificationRequest has no status message", func() {
			It("BR-ORCH-030: should handle empty failure message", func() {
				notif.Status.Phase = notificationv1.NotificationPhaseFailed
				notif.Status.Message = ""

				err := handler.UpdateNotificationStatus(ctx, rr, notif)

				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Status.NotificationStatus).To(Equal("Failed"))

				cond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
				Expect(cond).ToNot(BeNil())
				// Should still have a condition, even with empty message
			})
		})

		Context("when phase is unchanged but status updated", func() {
			It("BR-ORCH-030: should update condition timestamp", func() {
				// First update
				notif.Status.Phase = notificationv1.NotificationPhaseSent
				err := handler.UpdateNotificationStatus(ctx, rr, notif)
				Expect(err).ToNot(HaveOccurred())

				firstCond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
				Expect(firstCond).ToNot(BeNil())

				// Second update with same phase (should not change transition time)
				time.Sleep(10 * time.Millisecond)
				err = handler.UpdateNotificationStatus(ctx, rr, notif)
				Expect(err).ToNot(HaveOccurred())

				secondCond := meta.FindStatusCondition(rr.Status.Conditions, "NotificationDelivered")
				Expect(secondCond).ToNot(BeNil())

				// Transition time should be same (no status change)
				Expect(secondCond.LastTransitionTime).To(Equal(firstCond.LastTransitionTime))
			})
		})
	})
})
