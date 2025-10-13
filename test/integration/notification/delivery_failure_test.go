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

var _ = Describe("Integration Test 2: Delivery Failure Recovery", func() {
	var notification *notificationv1alpha1.NotificationRequest
	var notificationName string
	var failureCount int

	BeforeEach(func() {
		resetSlackRequests()
		failureCount = 0
		notificationName = fmt.Sprintf("test-failure-%d", time.Now().Unix())

		By("Reconfiguring mock Slack server to fail first 2 attempts, then succeed")
		mockSlackServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			failureCount++

			if failureCount <= 2 {
				// Simulate 503 Service Unavailable (transient error)
				GinkgoWriter.Printf("🔴 Mock Slack webhook attempt %d failed (503 Service Unavailable)\n", failureCount)
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Service temporarily unavailable"))
				return
			}

			// Success on 3rd attempt
			body := make([]byte, r.ContentLength)
			r.Body.Read(body)
			slackRequests = append(slackRequests, SlackWebhookRequest{
				Timestamp: time.Now(),
				Body:      body,
				Headers:   r.Header.Clone(),
			})

			GinkgoWriter.Printf("✅ Mock Slack webhook attempt %d succeeded\n", failureCount)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
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

	It("should automatically retry failed Slack deliveries and eventually succeed (BR-NOT-052: Automatic Retry)", func() {
		By("Creating NotificationRequest with Slack channel only")
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      notificationName,
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Integration Test - Retry Logic",
				Body:     "Testing automatic retry on failure (exponential backoff)",
				Type:     notificationv1alpha1.NotificationTypeEscalation,
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelSlack,
				},
			},
		}

		err := crClient.Create(ctx, notification)
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("✅ Created NotificationRequest: %s\n", notificationName)

		By("Waiting for controller to retry and eventually succeed")
		// Expected timeline with exponential backoff:
		//   t=0s: Attempt 1 (fail) → requeue after 30s
		//   t=30s: Attempt 2 (fail) → requeue after 60s
		//   t=90s: Attempt 3 (success) → phase = Sent
		//
		// Total time: ~90-120 seconds with reconciliation overhead

		GinkgoWriter.Println("⏳ Waiting for retry logic (this will take ~2-3 minutes due to exponential backoff)...")
		GinkgoWriter.Println("   Attempt 1 (t=0s):  Expected to fail (503)")
		GinkgoWriter.Println("   Attempt 2 (t=30s): Expected to fail (503)")
		GinkgoWriter.Println("   Attempt 3 (t=90s): Expected to succeed (200)")

		Eventually(func() notificationv1alpha1.NotificationPhase {
			updated := &notificationv1alpha1.NotificationRequest{}
			err := crClient.Get(ctx, types.NamespacedName{
				Name:      notificationName,
				Namespace: "kubernaut-notifications",
			}, updated)
			if err != nil {
				return ""
			}

			GinkgoWriter.Printf("   Phase: %s, Attempts: %d, Successful: %d, Failed: %d\n",
				updated.Status.Phase,
				updated.Status.TotalAttempts,
				updated.Status.SuccessfulDeliveries,
				updated.Status.FailedDeliveries)

			return updated.Status.Phase
		}, 180*time.Second, 5*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		By("Retrieving final status")
		final := &notificationv1alpha1.NotificationRequest{}
		err = crClient.Get(ctx, types.NamespacedName{
			Name:      notificationName,
			Namespace: "kubernaut-notifications",
		}, final)
		Expect(err).ToNot(HaveOccurred())

		By("Verifying multiple delivery attempts recorded")
		Expect(final.Status.DeliveryAttempts).To(HaveLen(3), "Expected 3 attempts (2 failures + 1 success)")
		Expect(final.Status.TotalAttempts).To(Equal(3), "Total attempts should be 3")
		Expect(final.Status.SuccessfulDeliveries).To(Equal(1), "Should have 1 successful delivery")
		Expect(final.Status.FailedDeliveries).To(Equal(2), "Should have 2 failed deliveries")

		By("Verifying first attempt failed")
		Expect(final.Status.DeliveryAttempts[0].Channel).To(Equal("slack"))
		Expect(final.Status.DeliveryAttempts[0].Status).To(Equal("failed"))
		Expect(final.Status.DeliveryAttempts[0].Error).To(ContainSubstring("503"),
			"First attempt error should mention 503")

		By("Verifying second attempt failed")
		Expect(final.Status.DeliveryAttempts[1].Channel).To(Equal("slack"))
		Expect(final.Status.DeliveryAttempts[1].Status).To(Equal("failed"))
		Expect(final.Status.DeliveryAttempts[1].Error).To(ContainSubstring("503"),
			"Second attempt error should mention 503")

		By("Verifying third attempt succeeded")
		Expect(final.Status.DeliveryAttempts[2].Channel).To(Equal("slack"))
		Expect(final.Status.DeliveryAttempts[2].Status).To(Equal("success"))
		Expect(final.Status.DeliveryAttempts[2].Error).To(BeEmpty(),
			"Third attempt should have no error")

		By("Verifying final phase is Sent")
		Expect(final.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent))
		Expect(final.Status.Reason).To(Equal("AllDeliveriesSucceeded"))

		By("Verifying exponential backoff timestamps")
		// Verify time between attempts increases (exponential backoff)
		if len(final.Status.DeliveryAttempts) >= 3 {
			t1 := final.Status.DeliveryAttempts[0].Timestamp.Time
			t2 := final.Status.DeliveryAttempts[1].Timestamp.Time
			t3 := final.Status.DeliveryAttempts[2].Timestamp.Time

			// Time between attempt 1 and 2 should be ~30s
			delta12 := t2.Sub(t1)
			GinkgoWriter.Printf("   Time between attempt 1 and 2: %v (expected ~30s)\n", delta12)
			Expect(delta12).To(BeNumerically(">=", 25*time.Second),
				"Backoff 1→2 should be at least 25s")
			Expect(delta12).To(BeNumerically("<=", 45*time.Second),
				"Backoff 1→2 should be at most 45s")

			// Time between attempt 2 and 3 should be ~60s
			delta23 := t3.Sub(t2)
			GinkgoWriter.Printf("   Time between attempt 2 and 3: %v (expected ~60s)\n", delta23)
			Expect(delta23).To(BeNumerically(">=", 50*time.Second),
				"Backoff 2→3 should be at least 50s")
			Expect(delta23).To(BeNumerically("<=", 90*time.Second),
				"Backoff 2→3 should be at most 90s")
		}

		GinkgoWriter.Printf("✅ Automatic retry validated: 2 failures → 1 success\n")
		GinkgoWriter.Printf("   Total attempts: %d\n", final.Status.TotalAttempts)
		GinkgoWriter.Printf("   Mock Slack server calls: %d\n", failureCount)
	})

	It("should stop retrying after max attempts (5) and mark as Failed", func() {
		By("Reconfiguring mock Slack server to always fail")
		mockSlackServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			failureCount++
			GinkgoWriter.Printf("🔴 Mock Slack webhook attempt %d failed (always failing for this test)\n", failureCount)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Service unavailable"))
		})

		By("Creating NotificationRequest")
		notification = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("test-max-retry-%d", time.Now().Unix()),
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Integration Test - Max Retry",
				Body:     "Testing max retry limit (should fail after 5 attempts)",
				Type:     notificationv1alpha1.NotificationTypeEscalation,
				Priority: notificationv1alpha1.NotificationPriorityHigh,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelSlack,
				},
			},
		}

		err := crClient.Create(ctx, notification)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for max retries to be exhausted (this will take ~8+ minutes)")
		GinkgoWriter.Println("⏳ Waiting for max retries (5 attempts with exponential backoff)...")
		GinkgoWriter.Println("   This test will take ~8-10 minutes due to backoff: 30s, 60s, 120s, 240s, 480s")

		// Expected timeline: t=0, t=30s, t=90s, t=210s, t=450s, t=930s
		// Total: ~15-16 minutes, but we'll wait up to 20 minutes to be safe
		Eventually(func() notificationv1alpha1.NotificationPhase {
			updated := &notificationv1alpha1.NotificationRequest{}
			crClient.Get(ctx, types.NamespacedName{
				Name:      notification.Name,
				Namespace: "kubernaut-notifications",
			}, updated)

			if updated.Status.TotalAttempts > 0 {
				GinkgoWriter.Printf("   Phase: %s, Attempts: %d/%d\n",
					updated.Status.Phase,
					updated.Status.TotalAttempts,
					5)
			}

			return updated.Status.Phase
		}, 20*time.Minute, 10*time.Second).Should(Equal(notificationv1alpha1.NotificationPhaseFailed))

		By("Verifying max attempts reached")
		final := &notificationv1alpha1.NotificationRequest{}
		crClient.Get(ctx, types.NamespacedName{
			Name:      notification.Name,
			Namespace: "kubernaut-notifications",
		}, final)

		Expect(final.Status.TotalAttempts).To(Equal(5), "Should have exactly 5 attempts")
		Expect(final.Status.SuccessfulDeliveries).To(Equal(0), "No successful deliveries")
		Expect(final.Status.FailedDeliveries).To(Equal(5), "All 5 attempts should have failed")
		Expect(final.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseFailed))
		Expect(final.Status.Reason).To(Equal("MaxRetriesExceeded"))

		GinkgoWriter.Println("✅ Max retry limit validated: 5 attempts → Failed")
	})
})

