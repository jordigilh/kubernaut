package notification

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil/timing"
)

// Edge Case Test 3: Large Payload Handling
// Tests graceful degradation for oversized notification payloads
var _ = Describe("Edge Case: Large Payload Handling", func() {
	ctx := context.Background()

	It("should deliver 10KB payload successfully", func() {
		By("Creating notification with 10KB body")
		// 10KB = 10,240 characters
		largeBody := strings.Repeat("A", 10240)

		nr := &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "large-payload-10kb",
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Large payload test (10KB)",
				Body:     largeBody,
				Priority: notificationv1alpha1.NotificationPriorityLow,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole, // Use console to avoid Slack size limits
				},
			},
		}

		Expect(k8sClient.Create(ctx, nr)).To(Succeed())

		By("Verifying notification delivers successfully")
		err := timing.EventuallyWithRetry(func() error {
			var updated notificationv1alpha1.NotificationRequest
			err := k8sClient.Get(ctx, client.ObjectKey{
				Name:      nr.Name,
				Namespace: nr.Namespace,
			}, &updated)
			if err != nil {
				return err
			}

			if updated.Status.Phase != notificationv1alpha1.NotificationPhaseSent {
				return fmt.Errorf("notification still in phase %s", updated.Status.Phase)
			}
			return nil
		}, 5, 6*timing.ReconcileTimeout())
		Expect(err).ToNot(HaveOccurred(), "10KB payload should deliver successfully")

		By("Cleaning up test notification")
		k8sClient.Delete(ctx, nr)
	})

	It("should handle 50KB payload with graceful degradation", func() {
		Skip("Requires payload truncation logic - deferred to V1.1")

		By("Creating notification with 50KB body")
		// 50KB = 51,200 characters
		veryLargeBody := strings.Repeat("B", 51200)

		nr := &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "large-payload-50kb",
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Very large payload test (50KB)",
				Body:     veryLargeBody,
				Priority: notificationv1alpha1.NotificationPriorityLow,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelSlack,
				},
			},
		}

		Expect(k8sClient.Create(ctx, nr)).To(Succeed())

		// TODO V1.1: Implement truncation logic
		// 1. Verify notification delivers (possibly truncated)
		// 2. Check for [TRUNCATED] suffix in delivered content
		// 3. Verify no delivery failures due to oversized payload
		// 4. Verify audit trail records truncation event

		By("Cleaning up test notification")
		k8sClient.Delete(ctx, nr)
	})
})
