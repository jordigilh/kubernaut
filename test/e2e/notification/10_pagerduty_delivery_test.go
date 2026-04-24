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
// E2E Test: PagerDuty Delivery Channel (#60)
// ========================================
//
// Business Requirements:
// - BR-NOT-060: PagerDuty Events API v2 delivery
// - BR-NOT-104: Per-receiver credential resolution
// - BR-NOT-055: Graceful degradation (circuit breaker)
//
// Test Strategy:
// 1. Success path: Route to pagerduty-success receiver → mock-webhook /pagerduty → 202
// 2. Failure path: Route to pagerduty-failure receiver → mock-webhook /pagerduty/fail → 503
//
// Infrastructure: mock-webhook nginx service provides /pagerduty and /pagerduty/fail endpoints.
// PagerDuty routing configs use url override to point at mock-webhook instead of real PD API.
// Credential files contain mock routing keys resolved via projected volume.
// ========================================

var _ = Describe("E2E Test: PagerDuty Delivery Channel (#60)", Label("e2e", "pagerduty"), func() {
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

	It("should deliver successfully to PagerDuty mock endpoint and reach Sent phase", func() {
		testCtx, testCancel = context.WithTimeout(ctx, 2*time.Minute)

		testID := time.Now().Format("20060102-150405")
		notificationName := "e2e-pd-success-" + testID

		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: controllerNamespace,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeEscalation,
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Subject:  "E2E PagerDuty Success Test",
				Body:     "Testing PagerDuty delivery to mock endpoint.",
				Context: &notificationv1alpha1.NotificationContext{
					Analysis: &notificationv1alpha1.AnalysisContext{
						RootCause: "OOMKill in production pod",
					},
				},
				Extensions: map[string]string{
					"test-channel-set": "pagerduty-success",
				},
			},
		}

		By("Creating NotificationRequest routed to pagerduty-success receiver")
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

		By("Validating delivery attempts include pagerduty channel")
		var nr notificationv1alpha1.NotificationRequest
		Expect(apiReader.Get(testCtx, types.NamespacedName{
			Name:      notificationName,
			Namespace: controllerNamespace,
		}, &nr)).To(Succeed())

		Expect(nr.Status.SuccessfulDeliveries).To(BeNumerically(">=", 1))
		foundPD := false
		for _, attempt := range nr.Status.DeliveryAttempts {
			if attempt.Status == notificationv1alpha1.DeliveryAttemptStatusSuccess {
				channelStr := string(attempt.Channel)
				if len(channelStr) >= 9 && channelStr[:9] == "pagerduty" {
					foundPD = true
					break
				}
			}
		}
		Expect(foundPD).To(BeTrue(), "Should have a successful pagerduty delivery attempt")
	})

	It("should fail delivery to PagerDuty and reach Failed phase when endpoint returns 503", func() {
		testCtx, testCancel = context.WithTimeout(ctx, 2*time.Minute)

		testID := time.Now().Format("20060102-150405")
		notificationName := "e2e-pd-failure-" + testID

		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: controllerNamespace,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityHigh,
				Subject:  "E2E PagerDuty Failure Test",
				Body:     "Testing PagerDuty failure path via mock endpoint returning 503.",
				RetryPolicy: &notificationv1alpha1.RetryPolicy{
					MaxAttempts:           1,
					InitialBackoffSeconds: 1,
					BackoffMultiplier:     2,
					MaxBackoffSeconds:     60,
				},
				Extensions: map[string]string{
					"test-channel-set": "pagerduty-failure",
				},
			},
		}

		By("Creating NotificationRequest routed to pagerduty-failure receiver")
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
