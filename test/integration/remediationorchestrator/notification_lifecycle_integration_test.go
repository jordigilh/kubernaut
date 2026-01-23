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

// Business Requirement: BR-ORCH-029, BR-ORCH-030, BR-ORCH-031
// Purpose: Validates notification lifecycle integration with real Kubernetes API

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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Notification Lifecycle Integration", Label("integration", "notification-lifecycle"), func() {
	var (
		testNamespace string
		testRR        *remediationv1.RemediationRequest
	)

	BeforeEach(func() {
		// Create unique namespace using standard helper (ensures uniqueness)
		testNamespace = createTestNamespace("ro-notif-lifecycle")

		// Create test RemediationRequest with all required fields
		// Note: These tests focus on notification tracking, not full RR lifecycle
		// The controller will manage phase naturally (starts in Pending)
		now := metav1.Now()
		testRR = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-rr-%d", time.Now().UnixNano()),
				Namespace: testNamespace,
			},
		Spec: remediationv1.RemediationRequestSpec{
			// Valid 64-char hex fingerprint (SHA256 format per CRD validation)
			// UNIQUE per test to avoid routing deduplication
			// Using SHA256(UUID) for guaranteed uniqueness in parallel execution
			SignalFingerprint: func() string {
				h := sha256.Sum256([]byte(uuid.New().String()))
				return hex.EncodeToString(h[:])
			}(),
				SignalName:        "NotificationLifecycleTest",
				Severity:          "warning",
				SignalType:        "prometheus",
				TargetType:        "kubernetes",
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
				SignalLabels: map[string]string{
					"test": "notification-lifecycle",
				},
			},
		}
		Expect(k8sClient.Create(ctx, testRR)).To(Succeed())

		// Wait for controller to initialize the RR
		Eventually(func() bool {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR)
			return err == nil && testRR.Status.OverallPhase != ""
		}, timeout, interval).Should(BeTrue())
	})

	AfterEach(func() {
		// Use standard helper for async namespace deletion
		deleteTestNamespace(testNamespace)
	})

	Describe("BR-ORCH-029: User-Initiated Cancellation", func() {
		It("should update status when user deletes NotificationRequest", func() {
			// Create NotificationRequest with owner reference
			notif := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-%d", time.Now().UnixNano()),
					Namespace: testNamespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeApproval, // Fixed: was "approval-required", should be "approval"
					Priority: notificationv1.NotificationPriorityMedium,
					Subject:  "Test Notification",
					Body:     "Test notification body for integration test",
					Metadata: map[string]string{
						"remediationRequest": testRR.Name,
					},
				},
			}

			// Set owner reference for cascade deletion
			Expect(controllerutil.SetControllerReference(testRR, notif, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, notif)).To(Succeed())

			// Update RemediationRequest status to reference notification
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return err
				}
				testRR.Status.NotificationRequestRefs = []corev1.ObjectReference{
					{
						APIVersion: notificationv1.GroupVersion.String(),
						Kind:       "NotificationRequest",
						Name:       notif.Name,
						Namespace:  notif.Namespace,
						UID:        notif.UID,
					},
				}
				return k8sClient.Status().Update(ctx, testRR)
			}, timeout, interval).Should(Succeed())

			// Capture phase before notification deletion
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR)).To(Succeed())
			phaseBeforeDeletion := testRR.Status.OverallPhase

			// User deletes NotificationRequest (simulates user cancellation)
			Expect(k8sClient.Delete(ctx, notif)).To(Succeed())

			// Wait for NotificationRequest to be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), notif)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Verify RemediationRequest status updated to "Cancelled"
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return ""
				}
				return testRR.Status.NotificationStatus
			}, timeout, interval).Should(Equal("Cancelled"))

			// CRITICAL: Verify overallPhase unchanged after notification deletion
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR)).To(Succeed())
			Expect(testRR.Status.OverallPhase).To(Equal(phaseBeforeDeletion), "Phase should not change when notification is cancelled")

			// Verify condition set
			cond := meta.FindStatusCondition(testRR.Status.Conditions, "NotificationDelivered")
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("UserCancelled"))
		})

		It("should handle multiple notification refs gracefully", func() {
			// Create first NotificationRequest
			notif1 := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-1-%d", time.Now().UnixNano()),
					Namespace: testNamespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeApproval,
					Priority: notificationv1.NotificationPriorityMedium,
					Subject:  "Test Notification 1",
					Body:     "Test notification body 1",
				},
			}
			Expect(controllerutil.SetControllerReference(testRR, notif1, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, notif1)).To(Succeed())

			// Create second NotificationRequest
			notif2 := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-2-%d", time.Now().UnixNano()),
					Namespace: testNamespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeEscalation,
					Priority: notificationv1.NotificationPriorityHigh,
					Subject:  "Test Notification 2",
					Body:     "Test notification body 2",
				},
			}
			Expect(controllerutil.SetControllerReference(testRR, notif2, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, notif2)).To(Succeed())

			// Update RemediationRequest status with both refs
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return err
				}
				testRR.Status.NotificationRequestRefs = []corev1.ObjectReference{
					{
						APIVersion: notificationv1.GroupVersion.String(),
						Kind:       "NotificationRequest",
						Name:       notif1.Name,
						Namespace:  notif1.Namespace,
						UID:        notif1.UID,
					},
					{
						APIVersion: notificationv1.GroupVersion.String(),
						Kind:       "NotificationRequest",
						Name:       notif2.Name,
						Namespace:  notif2.Namespace,
						UID:        notif2.UID,
					},
				}
				return k8sClient.Status().Update(ctx, testRR)
			}, timeout, interval).Should(Succeed())

			// Delete first notification (user cancellation)
			Expect(k8sClient.Delete(ctx, notif1)).To(Succeed())

			// Verify status updated (tracks first notification)
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return ""
				}
				return testRR.Status.NotificationStatus
			}, timeout, interval).Should(Equal("Cancelled"))

			// Verify second notification still exists
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(notif2), notif2)).To(Succeed())
		})
	})

	Describe("BR-ORCH-030: Status Tracking", func() {
		DescribeTable("should track NotificationRequest phase changes",
			func(nrPhase notificationv1.NotificationPhase, expectedStatus string, shouldSetCondition bool) {
				// Create NotificationRequest
				notif := &notificationv1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test-notif-%d", time.Now().UnixNano()),
						Namespace: testNamespace,
					},
					Spec: notificationv1.NotificationRequestSpec{
						Type:     notificationv1.NotificationTypeApproval,
						Priority: notificationv1.NotificationPriorityMedium,
						Subject:  "Test Notification",
						Body:     "Test notification body",
					},
				}
				Expect(controllerutil.SetControllerReference(testRR, notif, k8sClient.Scheme())).To(Succeed())
				Expect(k8sClient.Create(ctx, notif)).To(Succeed())

				// Update RemediationRequest status to reference notification
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
						return err
					}
					testRR.Status.NotificationRequestRefs = []corev1.ObjectReference{
						{
							APIVersion: notificationv1.GroupVersion.String(),
							Kind:       "NotificationRequest",
							Name:       notif.Name,
							Namespace:  notif.Namespace,
							UID:        notif.UID,
						},
					}
					return k8sClient.Status().Update(ctx, testRR)
				}, timeout, interval).Should(Succeed())

				// Update NotificationRequest phase
				Eventually(func() error {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), notif); err != nil {
						return err
					}
					notif.Status.Phase = nrPhase
					notif.Status.Message = fmt.Sprintf("Test message for %s", nrPhase)
					return k8sClient.Status().Update(ctx, notif)
				}, timeout, interval).Should(Succeed())

				// Verify RemediationRequest status updated
				Eventually(func() string {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
						return ""
					}
					return testRR.Status.NotificationStatus
				}, timeout, interval).Should(Equal(expectedStatus))

				// Verify condition if expected
				if shouldSetCondition {
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR)).To(Succeed())
					cond := meta.FindStatusCondition(testRR.Status.Conditions, "NotificationDelivered")
					Expect(cond).ToNot(BeNil())
				}
			},
			Entry("BR-ORCH-030: Pending phase", notificationv1.NotificationPhasePending, "Pending", false),
			Entry("BR-ORCH-030: Sending phase", notificationv1.NotificationPhaseSending, "InProgress", false),
			Entry("BR-ORCH-030: Sent phase", notificationv1.NotificationPhaseSent, "Sent", true),
			Entry("BR-ORCH-030: Failed phase", notificationv1.NotificationPhaseFailed, "Failed", true),
		)

		It("BR-ORCH-030: should set positive condition when notification delivery succeeds", func() {
			// Create NotificationRequest
			notif := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-%d", time.Now().UnixNano()),
					Namespace: testNamespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeApproval,
					Priority: notificationv1.NotificationPriorityMedium,
					Subject:  "Test Notification",
					Body:     "Test notification body",
				},
			}
			Expect(controllerutil.SetControllerReference(testRR, notif, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, notif)).To(Succeed())

			// Update RemediationRequest status to reference notification
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return err
				}
				testRR.Status.NotificationRequestRefs = []corev1.ObjectReference{
					{
						APIVersion: notificationv1.GroupVersion.String(),
						Kind:       "NotificationRequest",
						Name:       notif.Name,
						Namespace:  notif.Namespace,
						UID:        notif.UID,
					},
				}
				return k8sClient.Status().Update(ctx, testRR)
			}, timeout, interval).Should(Succeed())

			// Update NotificationRequest to Sent phase
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), notif); err != nil {
					return err
				}
				notif.Status.Phase = notificationv1.NotificationPhaseSent
				notif.Status.Message = "Notification delivered successfully"
				return k8sClient.Status().Update(ctx, notif)
			}, timeout, interval).Should(Succeed())

			// Verify condition
			Eventually(func() *metav1.Condition {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return nil
				}
				return meta.FindStatusCondition(testRR.Status.Conditions, "NotificationDelivered")
			}, timeout, interval).ShouldNot(BeNil())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR)).To(Succeed())
			cond := meta.FindStatusCondition(testRR.Status.Conditions, "NotificationDelivered")
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("DeliverySucceeded"))
			Expect(cond.Message).To(Equal("Notification delivered successfully"))
		})

		It("BR-ORCH-030: should set failure condition with reason when notification delivery fails", func() {
			// Create NotificationRequest
			notif := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-%d", time.Now().UnixNano()),
					Namespace: testNamespace,
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeApproval,
					Priority: notificationv1.NotificationPriorityMedium,
					Subject:  "Test Notification",
					Body:     "Test notification body",
				},
			}
			Expect(controllerutil.SetControllerReference(testRR, notif, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, notif)).To(Succeed())

			// Update RemediationRequest status to reference notification
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return err
				}
				testRR.Status.NotificationRequestRefs = []corev1.ObjectReference{
					{
						APIVersion: notificationv1.GroupVersion.String(),
						Kind:       "NotificationRequest",
						Name:       notif.Name,
						Namespace:  notif.Namespace,
						UID:        notif.UID,
					},
				}
				return k8sClient.Status().Update(ctx, testRR)
			}, timeout, interval).Should(Succeed())

			// Update NotificationRequest to Failed phase
			failureMessage := "SMTP server unreachable"
			Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), notif); err != nil {
					return err
				}
				notif.Status.Phase = notificationv1.NotificationPhaseFailed
				notif.Status.Message = failureMessage
				return k8sClient.Status().Update(ctx, notif)
			}, timeout, interval).Should(Succeed())

			// Verify condition
			Eventually(func() *metav1.Condition {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR); err != nil {
					return nil
				}
				return meta.FindStatusCondition(testRR.Status.Conditions, "NotificationDelivered")
			}, timeout, interval).ShouldNot(BeNil())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testRR), testRR)).To(Succeed())
			cond := meta.FindStatusCondition(testRR.Status.Conditions, "NotificationDelivered")
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("DeliveryFailed"))
			Expect(cond.Message).To(ContainSubstring(failureMessage))
		})
	})

	// BR-ORCH-031: Cascade Cleanup tests moved to E2E suite:
	// - test/e2e/remediationorchestrator/notification_cascade_e2e_test.go
})
