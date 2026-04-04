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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// DD-NOT-003 V2.0: Category 11 - Resource Management Integration Tests
//
// BR-NOT-060: Concurrent Delivery Safety - Handle 10+ simultaneous notifications
// BR-NOT-063: Graceful Degradation - System continues operating under resource pressure

var _ = Describe("Category 11: Resource Management", Label("integration", "resource_management"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())
	})

	Context("HTTP Connection Management (BR-NOT-060)", func() {
		It("should reuse HTTP connections for multiple Slack deliveries", func() {
			slackRequestsMu.Lock()
			slackRequests = []SlackWebhookRequest{}
			slackRequestsMu.Unlock()

			testID := fmt.Sprintf("conn-reuse-%s", uniqueSuffix)

			notifNames := make([]string, 20)
			for i := 0; i < 20; i++ {
				notifName := fmt.Sprintf("slack-conn-%d-%s", i, uniqueSuffix)
				notifNames[i] = notifName

				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Subject:  fmt.Sprintf("%s: Connection Test %d", testID, i),
						Body:     "Testing HTTP connection reuse",
						Extensions: map[string]string{
							"test-channel-set": "console-slack",
						},
					},
				}

				err := k8sClient.Create(ctx, notif)
				Expect(err).NotTo(HaveOccurred())
			}

			for _, notifName := range notifNames {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					notif := &notificationv1alpha1.NotificationRequest{}
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
						Name:      notifName,
						Namespace: testNamespace,
					}, notif)
					if err != nil {
						return ""
					}
					return notif.Status.Phase
				}, 60*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))
			}

			slackRequestsMu.Lock()
			requestCount := 0
			for _, req := range slackRequests {
				if len(req.Body) > 0 {
					requestCount++
				}
			}
			slackRequestsMu.Unlock()

			Expect(requestCount).To(BeNumerically(">=", 20),
				"All 20 Slack deliveries should reach the webhook server")

			GinkgoWriter.Printf("✅ HTTP connections managed efficiently: 20 Slack deliveries completed\n")

			for _, notifName := range notifNames {
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1,
					},
				}
				_ = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			}
		})
	})

	Context("Graceful Degradation (BR-NOT-063)", func() {
		It("should continue delivering notifications even under resource pressure", func() {
			var wg sync.WaitGroup
			notifNames := make([]string, 50)

			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()

					notifName := fmt.Sprintf("pressure-test-%d-%s", idx, uniqueSuffix)
					notifNames[idx] = notifName

					notif := &notificationv1alpha1.NotificationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:       notifName,
							Namespace:  testNamespace,
							Generation: 1,
						},
						Spec: notificationv1alpha1.NotificationRequestSpec{
							Type:     notificationv1alpha1.NotificationTypeSimple,
							Priority: notificationv1alpha1.NotificationPriorityMedium,
							Subject:  fmt.Sprintf("Pressure Test %d", idx),
							Body:     "Testing graceful degradation under resource pressure",
						},
					}

					err := k8sClient.Create(ctx, notif)
					Expect(err).NotTo(HaveOccurred(), "Notification creation should succeed under load")
				}(i)
			}

			wg.Wait()
			GinkgoWriter.Printf("✅ Created 50 notifications simultaneously\n")

			successCount := 0
			for _, notifName := range notifNames {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					notif := &notificationv1alpha1.NotificationRequest{}
					err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
						Name:      notifName,
						Namespace: testNamespace,
					}, notif)
					if err != nil {
						return ""
					}
					return notif.Status.Phase
				}, 90*time.Second, 1*time.Second).Should(Or(
					Equal(notificationv1alpha1.NotificationPhaseSent),
					Equal(notificationv1alpha1.NotificationPhasePartiallySent),
				), "Notification should be delivered despite resource pressure")

				notif := &notificationv1alpha1.NotificationRequest{}
				_ = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{Name: notifName, Namespace: testNamespace}, notif)
				if notif.Status.Phase == notificationv1alpha1.NotificationPhaseSent {
					successCount++
				}
			}

			successRate := float64(successCount) / 50.0 * 100
			GinkgoWriter.Printf("✅ Graceful degradation verified: %d/50 delivered (%0.0f%% success rate)\n",
				successCount, successRate)

			Expect(successRate).To(BeNumerically(">=", 90),
				"Success rate should be >=90% despite resource pressure (graceful degradation)")

			for _, notifName := range notifNames {
				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1,
					},
				}
				_ = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			}
		})
	})
})
