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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ========================================
// E2E Test: Multi-Receiver Fanout Routing (BR-NOT-068, #597)
// ========================================
//
// BUSINESS REQUIREMENT: BR-NOT-068 - Multi-Channel Fanout
//
// Test Strategy:
// 1. Create NotificationRequest matching pre-deployed continue:true routes
// 2. Validate delivery to channels from ALL matched receivers
// 3. Verify per-channel delivery status tracking
//
// PARALLEL SAFETY: Fanout routes are pre-deployed in the initial routing
// ConfigMap (test/infrastructure/notification_e2e.go). No ConfigMap mutation
// occurs at test time.
// ========================================

var _ = Describe("Fanout Routing E2E (BR-NOT-068, #597)", func() {

	// ========================================
	// E2E-NOT-597-001: Multi-receiver fanout delivery
	// ========================================
	Context("Multi-receiver fanout via continue:true routing", func() {

		It("[E2E-NOT-597-001] should deliver to channels from all matched receivers", FlakeAttempts(3), func() {
			By("Creating NotificationRequest matching continue:true fanout routes")

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-fanout-routing-597",
					Namespace: controllerNamespace,
					Labels: map[string]string{
						"test-scenario": "fanout-routing",
						"test-priority": "P0",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "E2E-NOT-597-001: Fanout Routing Test",
					Body:     "Testing multi-receiver fanout via continue:true routing in Kind cluster",
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Extensions: map[string]string{
						"test-channel-set": "e2e-fanout",
					},
				},
			}

			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, notification)
			})

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			By("Waiting for notification to reach Sent phase")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"All channels from matched receivers should deliver successfully")

			By("Verifying delivery to channels from all matched receivers")
			err = apiReader.Get(ctx, client.ObjectKey{
				Name:      notification.Name,
				Namespace: notification.Namespace,
			}, notification)
			Expect(err).ToNot(HaveOccurred())

			// fanout-console (console) + fanout-file-log (file + log) = 3 delivery attempts
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(3),
				"Should have 3 successful deliveries (console from fanout-console, file+log from fanout-file-log)")
			Expect(notification.Status.FailedDeliveries).To(Equal(0),
				"Should have 0 failed deliveries")

			By("Verifying per-channel delivery attempts (BR-NOT-068)")
			Expect(notification.Status.DeliveryAttempts).To(HaveLen(3),
				"Should record 3 delivery attempts (one per channel)")

			channelsSeen := make(map[string]bool)
			for _, attempt := range notification.Status.DeliveryAttempts {
				channelsSeen[string(attempt.Channel)] = true
				Expect(attempt.Status).To(Equal(notificationv1alpha1.DeliveryAttemptStatusSuccess),
					"All delivery attempts should succeed")
			}

			Expect(channelsSeen).To(HaveKey("console"),
				"Console channel from fanout-console receiver should be in delivery attempts")
			Expect(channelsSeen).To(HaveKey("file"),
				"File channel from fanout-file-log receiver should be in delivery attempts")
			Expect(channelsSeen).To(HaveKey("log"),
				"Log channel from fanout-file-log receiver should be in delivery attempts")

			logger.Info("FANOUT ROUTING SUCCESS: All 3 channels from 2 receivers delivered successfully")
		})
	})
})
