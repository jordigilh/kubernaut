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

package notification

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ========================================
// E2E Test: Microsoft Teams Delivery Channel (#593)
// ========================================
//
// Business Requirements:
// - BR-NOT-593: Microsoft Teams Adaptive Card delivery via Power Automate Workflows
// - BR-NOT-104: Per-receiver credential resolution
// - BR-NOT-055: Graceful degradation (circuit breaker)
//
// Test Strategy:
// 1. Success path: Route to teams-success receiver → mock-webhook /teams → 200
// 2. Failure path: Route to teams-failure receiver → mock-webhook /teams/fail → 503
//
// Infrastructure: mock-webhook nginx service provides /teams and /teams/fail endpoints.
// Credential files contain mock webhook URLs resolved via projected volume.
// ========================================

var _ = Describe("E2E Test: Microsoft Teams Delivery Channel (#593)", Label("e2e", "teams"), func() {
	var (
		testCtx      context.Context
		testCancel   context.CancelFunc
		notification *notificationv1alpha1.NotificationRequest
	)

	AfterEach(func() {
		if testCancel != nil {
			testCancel()
		}
		if notification != nil {
			_ = k8sClient.Delete(ctx, notification)
		}
	})

	It("should deliver successfully to Teams mock endpoint and reach Sent phase", func() {
		testCtx, testCancel = context.WithTimeout(ctx, 2*time.Minute)

		testID := time.Now().Format("20060102-150405")
		notificationName := "e2e-teams-success-" + testID

		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: controllerNamespace,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeApproval,
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Subject:  "E2E Teams Success Test",
				Body:     "Testing Teams Adaptive Card delivery to mock endpoint.",
				Context: &notificationv1alpha1.NotificationContext{
					Analysis: &notificationv1alpha1.AnalysisContext{
						RootCause: "Memory leak detected in request handler",
					},
					Lineage: &notificationv1alpha1.LineageContext{
						RemediationRequest: "e2e-teams-rr",
					},
				},
				Extensions: map[string]string{
					"test-channel-set": "teams-success",
				},
			},
		}

		By("Creating NotificationRequest routed to teams-success receiver")
		Expect(k8sClient.Create(testCtx, notification)).To(Succeed())

		By("Waiting for notification to reach Sent phase")
		Eventually(func() notificationv1alpha1.NotificationPhase {
			var n notificationv1alpha1.NotificationRequest
			if err := apiReader.Get(testCtx, types.NamespacedName{
				Name:      notificationName,
				Namespace: controllerNamespace,
			}, &n); err != nil {
				return ""
			}
			return n.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		By("Validating delivery attempts include teams channel")
		var nr notificationv1alpha1.NotificationRequest
		Expect(apiReader.Get(testCtx, types.NamespacedName{
			Name:      notificationName,
			Namespace: controllerNamespace,
		}, &nr)).To(Succeed())

		Expect(nr.Status.SuccessfulDeliveries).To(BeNumerically(">=", 1))
		foundTeams := false
		for _, attempt := range nr.Status.DeliveryAttempts {
			if attempt.Status == notificationv1alpha1.DeliveryAttemptStatusSuccess {
				channelStr := string(attempt.Channel)
				if len(channelStr) >= 5 && channelStr[:5] == "teams" {
					foundTeams = true
					break
				}
			}
		}
		Expect(foundTeams).To(BeTrue(), "Should have a successful teams delivery attempt")
	})

	It("should fail delivery to Teams and reach Failed phase when endpoint returns 503", func() {
		testCtx, testCancel = context.WithTimeout(ctx, 2*time.Minute)

		testID := time.Now().Format("20060102-150405")
		notificationName := "e2e-teams-failure-" + testID

		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: controllerNamespace,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityMedium,
				Subject:  "E2E Teams Failure Test",
				Body:     "Testing Teams failure path via mock endpoint returning 503.",
				RetryPolicy: &notificationv1alpha1.RetryPolicy{
					MaxAttempts:           1,
					InitialBackoffSeconds: 1,
					BackoffMultiplier:     2,
					MaxBackoffSeconds:     60,
				},
				Extensions: map[string]string{
					"test-channel-set": "teams-failure",
				},
			},
		}

		By("Creating NotificationRequest routed to teams-failure receiver")
		Expect(k8sClient.Create(testCtx, notification)).To(Succeed())

		By("Waiting for notification to reach Failed phase")
		Eventually(func() notificationv1alpha1.NotificationPhase {
			var n notificationv1alpha1.NotificationRequest
			if err := apiReader.Get(testCtx, types.NamespacedName{
				Name:      notificationName,
				Namespace: controllerNamespace,
			}, &n); err != nil {
				return ""
			}
			return n.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseFailed))

		By("Validating failure reason is MaxRetriesExhausted")
		var nr notificationv1alpha1.NotificationRequest
		Expect(apiReader.Get(testCtx, types.NamespacedName{
			Name:      notificationName,
			Namespace: controllerNamespace,
		}, &nr)).To(Succeed())
		Expect(nr.Status.Reason).To(Equal(notificationv1alpha1.StatusReasonMaxRetriesExhausted))
	})
})
