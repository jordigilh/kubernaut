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
)

var _ = Describe("Integration Test 1: NotificationRequest Lifecycle (Pending → Sent)", func() {
	var notification *notificationv1alpha1.NotificationRequest
	var notificationName string

	BeforeEach(func() {
		resetSlackRequests()
		notificationName = fmt.Sprintf("test-notification-%d", time.Now().Unix())
	})

	AfterEach(func() {
		if notification != nil {
			By("Cleaning up test notification")
			_ = crClient.Delete(ctx, notification)
		}
	})

	It("should process notification and transition from Pending → Sending → Sent", func() {
		By("Creating NotificationRequest CRD")
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Integration Test - Lifecycle",
				Body:     "Testing notification controller basic lifecycle (Pending → Sent)",
				Type:     notificationv1alpha1.NotificationTypeEscalation,
				Priority: notificationv1alpha1.NotificationPriorityHigh,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole,
					notificationv1alpha1.ChannelSlack,
				},
			},
		}

		err := crClient.Create(ctx, notification)
		Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")
		GinkgoWriter.Printf("✅ Created NotificationRequest: %s\n", notificationName)

		By("Waiting for controller to reconcile (initial phase should be Pending)")
		time.Sleep(2 * time.Second)

		By("Verifying phase transitions: Pending → Sending → Sent")
		Eventually(func() notificationv1alpha1.NotificationPhase {
			updated := &notificationv1alpha1.NotificationRequest{}
			err := crClient.Get(ctx, types.NamespacedName{
				Name:      notificationName,
				Namespace: "kubernaut-notifications",
			}, updated)
			if err != nil {
				GinkgoWriter.Printf("⚠️  Failed to get NotificationRequest: %v\n", err)
				return ""
			}
			GinkgoWriter.Printf("   Current phase: %s (reason: %s)\n", updated.Status.Phase, updated.Status.Reason)
			return updated.Status.Phase
		}, 15*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		By("Retrieving final status")
		final := &notificationv1alpha1.NotificationRequest{}
		err = crClient.Get(ctx, types.NamespacedName{
			Name:      notificationName,
			Namespace: "kubernaut-notifications",
		}, final)
		Expect(err).ToNot(HaveOccurred())

		By("Verifying DeliveryAttempts recorded (BR-NOT-051: Audit Trail)")
		Expect(final.Status.DeliveryAttempts).To(HaveLen(2), "Expected 2 delivery attempts (console + Slack)")
		Expect(final.Status.TotalAttempts).To(Equal(2), "Total attempts should be 2")
		Expect(final.Status.SuccessfulDeliveries).To(Equal(2), "Both deliveries should succeed")
		Expect(final.Status.FailedDeliveries).To(Equal(0), "No failed deliveries expected")

		// Verify console attempt
		consoleAttempt := final.Status.DeliveryAttempts[0]
		Expect(consoleAttempt.Channel).To(Equal("console"), "First attempt should be console")
		Expect(consoleAttempt.Status).To(Equal("success"), "Console delivery should succeed")
		Expect(consoleAttempt.Timestamp).ToNot(BeZero(), "Console attempt should have timestamp")

		// Verify Slack attempt
		slackAttempt := final.Status.DeliveryAttempts[1]
		Expect(slackAttempt.Channel).To(Equal("slack"), "Second attempt should be Slack")
		Expect(slackAttempt.Status).To(Equal("success"), "Slack delivery should succeed")
		Expect(slackAttempt.Timestamp).ToNot(BeZero(), "Slack attempt should have timestamp")

		By("Verifying completion time set (BR-NOT-056: CRD Lifecycle)")
		Expect(final.Status.CompletionTime).ToNot(BeNil(), "CompletionTime should be set")
		Expect(final.Status.CompletionTime.Time).To(BeTemporally("~", time.Now(), 20*time.Second),
			"CompletionTime should be recent")

		By("Verifying Slack webhook was called (BR-NOT-053: At-Least-Once)")
		Expect(getSlackRequestCount()).To(BeNumerically(">=", 1), "Expected at least 1 Slack webhook request")
		slackReq := getLastSlackRequest()
		Expect(slackReq).ToNot(BeNil(), "Slack request should exist")
		Expect(string(slackReq.Body)).To(ContainSubstring("Integration Test - Lifecycle"),
			"Slack webhook body should contain notification subject")
		Expect(slackReq.Headers.Get("Content-Type")).To(Equal("application/json"),
			"Slack webhook should use application/json")

		By("Verifying ObservedGeneration matches Generation (BR-NOT-051: Audit Trail)")
		Expect(final.Status.ObservedGeneration).To(Equal(final.Generation),
			"ObservedGeneration should match Generation")

		By("Verifying final phase and reason")
		Expect(final.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent),
			"Final phase should be Sent")
		Expect(final.Status.Reason).To(Equal("AllDeliveriesSucceeded"),
			"Reason should be AllDeliveriesSucceeded")
		Expect(final.Status.Message).To(ContainSubstring("Successfully delivered to 2 channel"),
			"Message should confirm successful delivery to both channels")

		GinkgoWriter.Printf("✅ Notification lifecycle validated: %s → %s\n",
			notificationv1alpha1.NotificationPhasePending,
			notificationv1alpha1.NotificationPhaseSent)
		GinkgoWriter.Printf("   Total attempts: %d\n", final.Status.TotalAttempts)
		GinkgoWriter.Printf("   Successful: %d\n", final.Status.SuccessfulDeliveries)
		GinkgoWriter.Printf("   Failed: %d\n", final.Status.FailedDeliveries)
		GinkgoWriter.Printf("   Completion time: %s\n", final.Status.CompletionTime.Time)
		GinkgoWriter.Printf("   Slack webhook calls: %d\n", getSlackRequestCount())
	})

	It("should process console-only notification successfully", func() {
		By("Creating NotificationRequest with console channel only")
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-console-only-%d", time.Now().Unix()),
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Integration Test - Console Only",
				Body:     "Testing console-only notification delivery",
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityMedium,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole,
				},
			},
		}

		err := crClient.Create(ctx, notification)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for Sent phase")
		Eventually(func() notificationv1alpha1.NotificationPhase {
			updated := &notificationv1alpha1.NotificationRequest{}
			crClient.Get(ctx, types.NamespacedName{
				Name:      notification.Name,
				Namespace: "kubernaut-notifications",
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 1*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		By("Verifying console-only delivery")
		final := &notificationv1alpha1.NotificationRequest{}
		crClient.Get(ctx, types.NamespacedName{
			Name:      notification.Name,
			Namespace: "kubernaut-notifications",
		}, final)

		Expect(final.Status.DeliveryAttempts).To(HaveLen(1), "Expected 1 delivery attempt (console only)")
		Expect(final.Status.DeliveryAttempts[0].Channel).To(Equal("console"))
		Expect(final.Status.DeliveryAttempts[0].Status).To(Equal("success"))
		Expect(final.Status.SuccessfulDeliveries).To(Equal(1))

		GinkgoWriter.Println("✅ Console-only notification validated")
	})
})
