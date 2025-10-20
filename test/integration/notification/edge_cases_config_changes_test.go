package notification

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/testutil/timing"
)

// Edge Case Test 2: Config Changes During Delivery
// Tests graceful handling of configuration changes mid-flight
var _ = Describe("Edge Case: Config Changes During Delivery", func() {
	ctx := context.Background()

	It("should handle SLACK_WEBHOOK_URL changes without delivery failures", func() {
		Skip("Requires mock Slack server with configurable webhook URL - deferred to V1.1")

		By("Creating initial notification with original webhook URL")
		nr1 := &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config-change-before",
				Namespace: "kubernaut-notifications",
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Subject:  "Before config change",
				Body:     "This notification uses the original webhook URL",
				Priority: notificationv1alpha1.NotificationPriorityLow,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelSlack,
				},
			},
		}

		Expect(k8sClient.Create(ctx, nr1)).To(Succeed())

		By("Waiting for first notification to start delivering")
		err := timing.EventuallyWithRetry(func() error {
			var updated notificationv1alpha1.NotificationRequest
			err := k8sClient.Get(ctx, client.ObjectKey{
				Name:      nr1.Name,
				Namespace: nr1.Namespace,
			}, &updated)
			if err != nil {
				return err
			}

			if updated.Status.Phase != notificationv1alpha1.NotificationPhaseSending &&
				updated.Status.Phase != notificationv1alpha1.NotificationPhaseSent {
				return fmt.Errorf("notification still in phase %s", updated.Status.Phase)
			}
			return nil
		}, 5, 6*timing.ReconcileTimeout())
		Expect(err).ToNot(HaveOccurred())

		// TODO V1.1: Implement webhook URL rotation test
		// 1. Update SlackService config with new webhook URL
		// 2. Create second notification mid-transition
		// 3. Verify both notifications deliver successfully (or retry gracefully)
		// 4. Verify no data loss during config transition

		By("Cleaning up test notifications")
		k8sClient.Delete(ctx, nr1)
	})
})

