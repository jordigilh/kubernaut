package notification

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

func TestCancellationNotificationController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification cancellation delivery suite")
}

var _ = Describe("Cancellation NotificationRequest delivery policy", func() {
	It("UT-NT-2588-001: uses the explicit retry policy for cancellation delivery", func() {
		reconciler := &NotificationRequestReconciler{}
		notification := &notificationv1.NotificationRequest{
			Spec: notificationv1.NotificationRequestSpec{
				Type: notificationv1.NotificationTypeCancellation,
				RetryPolicy: &notificationv1.RetryPolicy{
					MaxAttempts:           3,
					InitialBackoffSeconds: 2,
					BackoffMultiplier:     2,
					MaxBackoffSeconds:     8,
				},
			},
		}

		policy := reconciler.getRetryPolicy(notification)
		Expect(policy).To(Equal(notification.Spec.RetryPolicy))
		Expect(reconciler.calculateBackoffWithPolicy(notification, 1)).To(BeNumerically(">=", time.Second))
	})

	It("UT-NT-2588-002: keeps cancellation delivery retryable until backoff expires", func() {
		reconciler := &NotificationRequestReconciler{}
		notification := &notificationv1.NotificationRequest{
			Spec: notificationv1.NotificationRequestSpec{
				Type: notificationv1.NotificationTypeCancellation,
				RetryPolicy: &notificationv1.RetryPolicy{
					MaxAttempts:           3,
					InitialBackoffSeconds: 30,
					BackoffMultiplier:     2,
					MaxBackoffSeconds:     120,
				},
			},
			Status: notificationv1.NotificationRequestStatus{
				Phase: notificationv1.NotificationPhaseRetrying,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{{
					Channel:   notificationv1.DeliveryChannelName("console"),
					Attempt:   1,
					Timestamp: metav1.NewTime(time.Now()),
					Status:    notificationv1.DeliveryAttemptStatusFailed,
					Error:     "temporary delivery failure",
				}},
			},
		}

		shouldRequeue, remaining := reconciler.checkRetryBackoff(notification, GinkgoLogr)
		Expect(shouldRequeue).To(BeTrue())
		Expect(remaining).To(BeNumerically(">", 0))

		notification.Status.DeliveryAttempts[0].Timestamp = metav1.NewTime(time.Now().Add(-2 * time.Minute))
		shouldRequeue, remaining = reconciler.checkRetryBackoff(notification, GinkgoLogr)
		Expect(shouldRequeue).To(BeFalse())
		Expect(remaining).To(Equal(time.Duration(0)))
	})
})
