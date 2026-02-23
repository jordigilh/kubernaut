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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	"github.com/google/uuid"
)

// ========================================
// Phase 2 E2E Tests - Notification Cascade Cleanup
// ========================================
//
// PHASE 2 PATTERN: RO + Notification Controllers Running
// - RO controller creates NotificationRequest CRDs with owner references
// - Notification controller processes notifications independently
// - Kubernetes garbage collection cascade-deletes NotificationRequests
//
// PENDING: These tests await Notification controller deployment in E2E suite.
// See test/e2e/remediationorchestrator/suite_test.go lines 142-147 for TODO.
//
// Business Value:
// - BR-ORCH-031: Cascade cleanup prevents orphaned notifications
// - Multi-notification cleanup validation (approval + escalation)
// ========================================

var _ = Describe("BR-ORCH-031: Notification Cascade Cleanup E2E Tests", Label("e2e", "notification", "cascade", "pending"), func() {

	Describe("Single NotificationRequest Cascade Deletion", func() {
		It("should cascade delete NotificationRequest when RemediationRequest is deleted", func() {
			testNamespace := createTestNamespace("ro-notif-cascade-single")
			defer deleteTestNamespace(testNamespace)

			// Create RemediationRequest
			now := metav1.Now()
			testRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("rr-cascade-%s", uuid.New().String()[:8]),
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "test-signal",
					SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Severity:          "critical",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: testNamespace,
					},
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRR)).To(Succeed())

			// Create NotificationRequest with owner reference
			notif := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-%s", uuid.New().String()[:8]),
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

			// Verify NotificationRequest exists
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), notif)).To(Succeed())

			// Delete RemediationRequest
			Expect(k8sClient.Delete(ctx, testRR)).To(Succeed())

			// Verify NotificationRequest cascade deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), notif)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Describe("Multiple NotificationRequests Cascade Deletion", func() {
		It("should cascade delete multiple NotificationRequests when RemediationRequest is deleted", func() {
			testNamespace := createTestNamespace("ro-notif-cascade-multi")
			defer deleteTestNamespace(testNamespace)

			// Create RemediationRequest
			now := metav1.Now()
			testRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("rr-multi-%s", uuid.New().String()[:8]),
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "test-signal",
					SignalFingerprint: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
					Severity:          "critical",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					FiringTime:        now,
					ReceivedTime:      now,
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: testNamespace,
					},
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, testRR)).To(Succeed())

			// Create multiple NotificationRequests with owner references
			notif1 := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-1-%s", uuid.New().String()[:8]),
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

			notif2 := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-notif-2-%s", uuid.New().String()[:8]),
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

			// Verify both NotificationRequests exist
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(notif1), notif1)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(notif2), notif2)).To(Succeed())

			// Delete RemediationRequest
			Expect(k8sClient.Delete(ctx, testRR)).To(Succeed())

			// Verify both NotificationRequests cascade deleted
			Eventually(func() bool {
				err1 := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif1), notif1)
				err2 := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif2), notif2)
				return apierrors.IsNotFound(err1) && apierrors.IsNotFound(err2)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
