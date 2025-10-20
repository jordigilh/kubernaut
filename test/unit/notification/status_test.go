package notification_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/status"
)

func TestStatus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NotificationRequest Status Suite")
}

var _ = Describe("BR-NOT-051: Status Tracking", func() {
	var (
		ctx           context.Context
		statusManager *status.Manager
		scheme        *runtime.Scheme
		fakeClient    client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = notificationv1alpha1.AddToScheme(scheme)

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&notificationv1alpha1.NotificationRequest{}).
			Build()

		statusManager = status.NewManager(fakeClient)
	})

	Context("DeliveryAttempts tracking", func() {
		It("should record all delivery attempts in order", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notification",
					Namespace: "kubernaut-notifications",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Test message",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			Expect(fakeClient.Create(ctx, notification)).To(Succeed())

			// Record first attempt (console success)
			err := statusManager.RecordDeliveryAttempt(ctx, notification, notificationv1alpha1.DeliveryAttempt{
				Channel:   "console",
				Timestamp: metav1.Now(),
				Status:    "success",
			})
			Expect(err).ToNot(HaveOccurred())

			// Record second attempt (Slack failure)
			err = statusManager.RecordDeliveryAttempt(ctx, notification, notificationv1alpha1.DeliveryAttempt{
				Channel:   "slack",
				Timestamp: metav1.Now(),
				Status:    "failed",
				Error:     "webhook returned 503",
			})
			Expect(err).ToNot(HaveOccurred())

			// Verify attempts recorded
			updated := &notificationv1alpha1.NotificationRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "test-notification",
				Namespace: "kubernaut-notifications",
			}, updated)
			Expect(err).ToNot(HaveOccurred())

			Expect(updated.Status.DeliveryAttempts).To(HaveLen(2))
			Expect(updated.Status.DeliveryAttempts[0].Channel).To(Equal("console"))
			Expect(updated.Status.DeliveryAttempts[0].Status).To(Equal("success"))
			Expect(updated.Status.DeliveryAttempts[1].Channel).To(Equal("slack"))
			Expect(updated.Status.DeliveryAttempts[1].Status).To(Equal("failed"))
			Expect(updated.Status.DeliveryAttempts[1].Error).To(Equal("webhook returned 503"))

			Expect(updated.Status.TotalAttempts).To(Equal(2))
			Expect(updated.Status.SuccessfulDeliveries).To(Equal(1))
			Expect(updated.Status.FailedDeliveries).To(Equal(1))
		})

		It("should track multiple retries for the same channel", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "retry-test",
					Namespace: "kubernaut-notifications",
				},
			}

			Expect(fakeClient.Create(ctx, notification)).To(Succeed())

			// Record 3 failed attempts for Slack
			for i := 1; i <= 3; i++ {
				err := statusManager.RecordDeliveryAttempt(ctx, notification, notificationv1alpha1.DeliveryAttempt{
					Channel:   "slack",
					Timestamp: metav1.Now(),
					Status:    "failed",
					Error:     "network timeout",
				})
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify all attempts recorded
			updated := &notificationv1alpha1.NotificationRequest{}
			err := fakeClient.Get(ctx, types.NamespacedName{
				Name:      "retry-test",
				Namespace: "kubernaut-notifications",
			}, updated)
			Expect(err).ToNot(HaveOccurred())

			Expect(updated.Status.DeliveryAttempts).To(HaveLen(3))
			Expect(updated.Status.TotalAttempts).To(Equal(3))
			Expect(updated.Status.FailedDeliveries).To(Equal(3))
		})
	})

	Context("Phase transitions", func() {
		// ⭐ TABLE-DRIVEN: Phase state machine validation
		DescribeTable("should update phase correctly",
			func(currentPhase, newPhase notificationv1alpha1.NotificationPhase, shouldSucceed bool) {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "phase-test",
						Namespace: "kubernaut-notifications",
					},
					Status: notificationv1alpha1.NotificationRequestStatus{
						Phase: currentPhase,
					},
				}

				Expect(fakeClient.Create(ctx, notification)).To(Succeed())

				err := statusManager.UpdatePhase(ctx, notification, newPhase, "TestReason", "Test message")

				if shouldSucceed {
					Expect(err).ToNot(HaveOccurred())

					updated := &notificationv1alpha1.NotificationRequest{}
					err = fakeClient.Get(ctx, types.NamespacedName{
						Name:      "phase-test",
						Namespace: "kubernaut-notifications",
					}, updated)
					Expect(err).ToNot(HaveOccurred())
					Expect(updated.Status.Phase).To(Equal(newPhase))
					Expect(updated.Status.Reason).To(Equal("TestReason"))
					Expect(updated.Status.Message).To(Equal("Test message"))
				} else {
					Expect(err).To(HaveOccurred())
				}
			},
			Entry("Pending → Sending (valid)", notificationv1alpha1.NotificationPhasePending, notificationv1alpha1.NotificationPhaseSending, true),
			Entry("Sending → Sent (valid)", notificationv1alpha1.NotificationPhaseSending, notificationv1alpha1.NotificationPhaseSent, true),
			Entry("Sending → Failed (valid)", notificationv1alpha1.NotificationPhaseSending, notificationv1alpha1.NotificationPhaseFailed, true),
			Entry("Sending → PartiallySent (valid)", notificationv1alpha1.NotificationPhaseSending, notificationv1alpha1.NotificationPhasePartiallySent, true),
			Entry("Sent → Pending (invalid)", notificationv1alpha1.NotificationPhaseSent, notificationv1alpha1.NotificationPhasePending, false),
			Entry("Failed → Sending (invalid)", notificationv1alpha1.NotificationPhaseFailed, notificationv1alpha1.NotificationPhaseSending, false),
		)

		It("should set completion time when reaching terminal phase", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "completion-test",
					Namespace: "kubernaut-notifications",
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseSending,
				},
			}

			Expect(fakeClient.Create(ctx, notification)).To(Succeed())

			// Update to terminal phase (Sent)
			err := statusManager.UpdatePhase(ctx, notification, notificationv1alpha1.NotificationPhaseSent, "AllDeliveriesSucceeded", "All channels delivered")
			Expect(err).ToNot(HaveOccurred())

			// Verify completion time set
			updated := &notificationv1alpha1.NotificationRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "completion-test",
				Namespace: "kubernaut-notifications",
			}, updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.CompletionTime).ToNot(BeNil())
			Expect(updated.Status.CompletionTime.Time).To(BeTemporally("~", time.Now(), 5*time.Second))
		})
	})

	Context("ObservedGeneration tracking", func() {
		It("should update ObservedGeneration to match Generation", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "generation-test",
					Namespace:  "kubernaut-notifications",
					Generation: 3, // Simulating spec update
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					ObservedGeneration: 1, // Out of sync
				},
			}

			Expect(fakeClient.Create(ctx, notification)).To(Succeed())

			err := statusManager.UpdateObservedGeneration(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			updated := &notificationv1alpha1.NotificationRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "generation-test",
				Namespace: "kubernaut-notifications",
			}, updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.ObservedGeneration).To(Equal(int64(3)))
		})
	})
})
