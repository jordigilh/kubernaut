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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	kubernautnotif "github.com/jordigilh/kubernaut/pkg/notification"
)

// =============================================================================
// BR-NOT-068: Multi-Channel Fanout Integration Tests — Issue #597
// =============================================================================
//
// These tests validate that the controller correctly resolves multiple receivers
// when `continue: true` routes are configured, and delivers to channels from
// ALL matched receivers.
//
// PARALLEL SAFETY: Fanout routes are pre-loaded in the suite routing config
// (suite_test.go). No runtime mutation of testRouter occurs in these tests.
// =============================================================================

var _ = Describe("Route Fanout Integration (BR-NOT-068, #597)", func() {

	var uniqueSuffix string

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())
	})

	// ========================================
	// IT-NOT-597-001: Controller multi-receiver fanout delivery
	// ========================================
	Context("Multi-receiver fanout via continue:true routing", func() {

		It("[IT-NOT-597-001] should deliver to channels from all matched receivers", func() {
			By("Creating NotificationRequest matching continue:true fanout routes")

			notifName := fmt.Sprintf("fanout-it-597-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notifName,
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "IT-NOT-597-001: Fanout Routing Test",
					Body:     "Testing multi-receiver fanout via continue:true routing",
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Extensions: map[string]string{
						"test-channel-set": "fanout-test",
					},
				},
			}

			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for notification to reach Sent phase")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				freshNotif := &notificationv1alpha1.NotificationRequest{}
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, freshNotif)
				if err != nil {
					return ""
				}
				return freshNotif.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			By("Verifying delivery to channels from BOTH matched receivers")
			freshNotif := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      notifName,
				Namespace: testNamespace,
			}, freshNotif)
			Expect(err).NotTo(HaveOccurred())

			// fanout-console (console) + fanout-file (file) = 2 delivery attempts
			Expect(freshNotif.Status.SuccessfulDeliveries).To(BeNumerically(">=", 2),
				"Should deliver to channels from both matched receivers (console + file)")

			channelsSeen := make(map[string]bool)
			for _, attempt := range freshNotif.Status.DeliveryAttempts {
				channelsSeen[string(attempt.Channel)] = true
			}
			Expect(channelsSeen).To(HaveKey("console"),
				"Console channel from fanout-console receiver should be in delivery attempts")
			Expect(channelsSeen).To(HaveKey("file"),
				"File channel from fanout-file receiver should be in delivery attempts")

			By("Verifying RoutingResolved condition lists both receiver names (BR-NOT-069)")
			routingCondition := kubernautnotif.GetRoutingResolved(freshNotif)
			Expect(routingCondition).ToNot(BeNil(), "RoutingResolved condition should be set")
			Expect(routingCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(routingCondition.Message).To(ContainSubstring("fanout-console"),
				"Condition message should mention fanout-console receiver")
			Expect(routingCondition.Message).To(ContainSubstring("fanout-file"),
				"Condition message should mention fanout-file receiver")

			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
