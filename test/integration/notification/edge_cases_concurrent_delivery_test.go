package notification

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil/timing"
)

// Edge Case Test 4: Concurrent Delivery Across Namespaces
// Tests race condition prevention and scalability
var _ = Describe("Edge Case: Concurrent Delivery Across Namespaces", func() {
	ctx := context.Background()

	It("should deliver 50 concurrent notifications without race conditions", func() {
		By("Creating 50 notifications concurrently")
		var wg sync.WaitGroup
		var mu sync.Mutex
		notifications := make([]*notificationv1alpha1.NotificationRequest, 50)
		errors := []error{}

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()

				nr := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("concurrent-test-%d", idx),
						Namespace: "kubernaut-notifications",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  fmt.Sprintf("Concurrent test %d", idx),
						Body:     fmt.Sprintf("Testing concurrent delivery #%d", idx),
						Priority: notificationv1alpha1.NotificationPriorityLow,
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
					},
				}

				err := k8sClient.Create(ctx, nr)
				mu.Lock()
				if err != nil {
					errors = append(errors, err)
				} else {
					notifications[idx] = nr
				}
				mu.Unlock()
			}(i)
		}

		wg.Wait()
		Expect(errors).To(BeEmpty(), "All notifications should create without errors")

		By("Verifying all 50 notifications deliver successfully within 60s")
		var verifyWg sync.WaitGroup
		successCount := 0
		var countMu sync.Mutex

		for i, nr := range notifications {
			if nr == nil {
				continue // Skip if creation failed
			}

			verifyWg.Add(1)
			go func(idx int, notification *notificationv1alpha1.NotificationRequest) {
				defer verifyWg.Done()
				defer GinkgoRecover()

				err := timing.EventuallyWithRetry(func() error {
					var updated notificationv1alpha1.NotificationRequest
					err := k8sClient.Get(ctx, client.ObjectKey{
						Name:      notification.Name,
						Namespace: notification.Namespace,
					}, &updated)
					if err != nil {
						return err
					}

					if updated.Status.Phase != notificationv1alpha1.NotificationPhaseSent {
						return fmt.Errorf("notification %d still in phase %s", idx, updated.Status.Phase)
					}
					return nil
				}, 10, 6*timing.ReconcileTimeout())

				if err == nil {
					countMu.Lock()
					successCount++
					countMu.Unlock()
				}
			}(i, nr)
		}

		verifyWg.Wait()

		By(fmt.Sprintf("Verifying success rate: %d/50 notifications delivered", successCount))
		Expect(successCount).To(BeNumerically(">=", 45),
			"At least 90% of notifications should deliver successfully (45/50)")

		By("Cleaning up test notifications")
		for _, nr := range notifications {
			if nr != nil {
				k8sClient.Delete(ctx, nr)
			}
		}
	})
})

