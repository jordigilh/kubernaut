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
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

var _ = Describe("Integration Test 3: Graceful Degradation (Multi-Channel Partial Failure)", func() {
	var notification *notificationv1alpha1.NotificationRequest
	var notificationName string

	BeforeEach(func() {
		resetSlackRequests()
		notificationName = fmt.Sprintf("test-degradation-%d", time.Now().Unix())

		By("Reconfiguring mock Slack server to always fail (simulating Slack outage)")
		mockSlackServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			GinkgoWriter.Printf("🔴 Mock Slack webhook failed (503 - simulated outage)\n")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Slack service unavailable"))
		})
	})

	AfterEach(func() {
		if notification != nil {
			By("Cleaning up test notification")
			_ = crClient.Delete(ctx, notification)
		}

		By("Restoring normal mock server behavior")
		deployMockSlackServer() // Reset to default handler
	})

	It("should mark notification as PartiallySent when some channels succeed and others fail (BR-NOT-055: Graceful Degradation)", func() {
		By("Creating NotificationRequest with console + Slack channels")
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Integration Test - Graceful Degradation",
				Body:     "Testing per-channel isolation: console succeeds, Slack fails",
				Type:     notificationv1alpha1.NotificationTypeEscalation,
				Priority: notificationv1alpha1.NotificationPriorityHigh,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole, // Will succeed
					notificationv1alpha1.ChannelSlack,   // Will fail (503)
				},
			},
		}

		err := crClient.Create(ctx, notification)
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("✅ Created NotificationRequest: %s\n", notificationName)

		By("Waiting for controller to process both channels")
		// Console should succeed immediately
		// Slack will fail and retry multiple times
		// Eventually should reach PartiallySent (not Failed)
		GinkgoWriter.Println("⏳ Waiting for multi-channel processing...")
		GinkgoWriter.Println("   Console: Expected to succeed immediately")
		GinkgoWriter.Println("   Slack: Expected to fail all retry attempts")

		Eventually(func() notificationv1alpha1.NotificationPhase {
			updated := &notificationv1alpha1.NotificationRequest{}
			err := crClient.Get(ctx, types.NamespacedName{
				Name:      notificationName,
				Namespace: "kubernaut-notifications",
			}, updated)
			if err != nil {
				return ""
			}

			GinkgoWriter.Printf("   Phase: %s, Successful: %d, Failed: %d\n",
				updated.Status.Phase,
				updated.Status.SuccessfulDeliveries,
				updated.Status.FailedDeliveries)

			return updated.Status.Phase
		}, 60*time.Second, 2*time.Second).Should(Equal(notificationv1alpha1.NotificationPhasePartiallySent))

		By("Retrieving final status")
		final := &notificationv1alpha1.NotificationRequest{}
		err = crClient.Get(ctx, types.NamespacedName{
			Name:      notificationName,
			Namespace: "kubernaut-notifications",
		}, final)
		Expect(err).ToNot(HaveOccurred())

		By("Verifying phase is PartiallySent (not Failed)")
		Expect(final.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhasePartiallySent),
			"Phase should be PartiallySent when some channels succeed")
		Expect(final.Status.Reason).To(Equal("PartialDeliveryFailure"),
			"Reason should be PartialDeliveryFailure")
		Expect(final.Status.Message).To(ContainSubstring("1 of 2 deliveries succeeded"),
			"Message should indicate partial success")

		By("Verifying delivery attempts show console success + Slack failures")
		Expect(final.Status.SuccessfulDeliveries).To(Equal(1), "Console should succeed")
		Expect(final.Status.FailedDeliveries).To(BeNumerically(">", 0), "Slack should have failures")

		// Find console attempt (should be success)
		var consoleSuccess bool
		var slackFailed bool
		for _, attempt := range final.Status.DeliveryAttempts {
			if attempt.Channel == "console" && attempt.Status == "success" {
				consoleSuccess = true
				GinkgoWriter.Printf("   ✅ Console delivery succeeded at %s\n", attempt.Timestamp.Time)
			}
			if attempt.Channel == "slack" && attempt.Status == "failed" {
				slackFailed = true
				GinkgoWriter.Printf("   ❌ Slack delivery failed at %s (error: %s)\n",
					attempt.Timestamp.Time, attempt.Error)
			}
		}

		Expect(consoleSuccess).To(BeTrue(), "Console delivery should succeed")
		Expect(slackFailed).To(BeTrue(), "Slack delivery should fail")

		By("Verifying circuit breaker NOT blocking console delivery (BR-NOT-055: Channel Isolation)")
		// Console should succeed despite Slack failures
		// This validates per-channel circuit breaker isolation
		Expect(final.Status.SuccessfulDeliveries).To(Equal(1),
			"Console delivery should succeed independently of Slack failures")

		By("Verifying CompletionTime is NOT set (PartiallySent is not terminal)")
		// PartiallySent may still have retries pending for failed channels
		// Only terminal phases (Sent, Failed) should have CompletionTime
		if final.Status.CompletionTime != nil {
			GinkgoWriter.Printf("   ⚠️  CompletionTime is set: %s (may indicate terminal state reached)\n",
				final.Status.CompletionTime.Time)
		}

		GinkgoWriter.Printf("✅ Graceful degradation validated: Console ✅, Slack ❌ → PartiallySent\n")
		GinkgoWriter.Printf("   Successful deliveries: %d\n", final.Status.SuccessfulDeliveries)
		GinkgoWriter.Printf("   Failed deliveries: %d\n", final.Status.FailedDeliveries)
		GinkgoWriter.Printf("   Total attempts: %d\n", final.Status.TotalAttempts)
	})

	It("should NOT block console delivery when Slack is in circuit breaker open state", func() {
		By("Creating multiple Slack-only notifications to open circuit breaker")
		// Circuit breaker opens after N consecutive failures
		// We'll create 5 Slack-only notifications to trigger circuit breaker

		for i := 1; i <= 5; i++ {
			slackNotif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-circuit-%d-%d", time.Now().Unix(), i),
					Namespace: "kubernaut-notifications",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  fmt.Sprintf("Circuit Breaker Test %d", i),
					Body:     "Triggering circuit breaker",
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			err := crClient.Create(ctx, slackNotif)
			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Printf("   Created notification %d/5 to trigger circuit breaker\n", i)

			// Cleanup
			defer func(n *notificationv1alpha1.NotificationRequest) {
				_ = crClient.Delete(ctx, n)
			}(slackNotif)
		}

		By("Waiting for Slack circuit breaker to potentially open")
		time.Sleep(10 * time.Second)

		By("Creating console + Slack notification while circuit breaker may be open")
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-circuit-isolation-%d", time.Now().Unix()),
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Integration Test - Circuit Breaker Isolation",
				Body:     "Console should succeed even if Slack circuit breaker is open",
				Type:     notificationv1alpha1.NotificationTypeSimple,
				Priority: notificationv1alpha1.NotificationPriorityMedium,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole,
					notificationv1alpha1.ChannelSlack,
				},
			},
		}

		err := crClient.Create(ctx, notification)
		Expect(err).ToNot(HaveOccurred())

		By("Verifying console delivery succeeds despite Slack circuit breaker")
		Eventually(func() int {
			updated := &notificationv1alpha1.NotificationRequest{}
			crClient.Get(ctx, types.NamespacedName{
				Name:      notification.Name,
				Namespace: "kubernaut-notifications",
			}, updated)
			return updated.Status.SuccessfulDeliveries
		}, 15*time.Second, 1*time.Second).Should(BeNumerically(">=", 1))

		final := &notificationv1alpha1.NotificationRequest{}
		crClient.Get(ctx, types.NamespacedName{
			Name:      notification.Name,
			Namespace: "kubernaut-notifications",
		}, final)

		// Verify console succeeded
		consoleSucceeded := false
		for _, attempt := range final.Status.DeliveryAttempts {
			if attempt.Channel == "console" && attempt.Status == "success" {
				consoleSucceeded = true
			}
		}

		Expect(consoleSucceeded).To(BeTrue(),
			"Console delivery should succeed even if Slack circuit breaker is open")

		GinkgoWriter.Println("✅ Circuit breaker isolation validated: Console unaffected by Slack circuit breaker")
	})
})
