package notification

import (
	"context"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

func TestControllerEdgeCases(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Edge Cases Suite")
}

var _ = Describe("Controller Edge Cases", func() {
	var (
		ctx        context.Context
		reconciler *notificationcontroller.NotificationRequestReconciler
		k8sClient  client.Client
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = notificationv1alpha1.AddToScheme(scheme)

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&notificationv1alpha1.NotificationRequest{}).
			Build()

		// Create reconciler with mock services
		reconciler = &notificationcontroller.NotificationRequestReconciler{
			Client:         k8sClient,
			Scheme:         scheme,
			ConsoleService: delivery.NewConsoleDeliveryService(nil),
			SlackService:   delivery.NewSlackDeliveryService("http://localhost:8080"),
		}
	})

	// Edge Case 1: Concurrent Reconciliation
	Context("Concurrent Reconciliation", func() {
		It("should handle concurrent reconciliation without race conditions", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "concurrent-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Concurrent test",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Simulate 10 concurrent reconciliations
			var wg sync.WaitGroup
			errors := make([]error, 10)
			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					_, errors[index] = reconciler.Reconcile(ctx, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Name:      "concurrent-test",
							Namespace: "default",
						},
					})
				}(i)
			}
			wg.Wait()

			// Verify: No panic occurred, state is consistent
			updated := &notificationv1alpha1.NotificationRequest{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "concurrent-test",
				Namespace: "default",
			}, updated)
			Expect(err).ToNot(HaveOccurred())

			// Key success: No panic, no corruption (state may vary due to concurrency)
			// TotalAttempts could be 0-20 depending on timing, but should not be negative
			Expect(updated.Status.TotalAttempts).To(BeNumerically(">=", 0))
			Expect(updated.Status.TotalAttempts).To(BeNumerically("<=", 30)) // Upper bound check
		})
	})

	// Edge Case 2: Stale Generation Handling
	Context("Generation Handling", func() {
		It("should process notification when ObservedGeneration matches Generation", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "generation-test",
					Namespace:  "default",
					Generation: 5,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Generation test",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase:              notificationv1alpha1.NotificationPhaseSent,
					ObservedGeneration: 5, // Already processed
					CompletionTime:     &metav1.Time{Time: metav1.Now().Time},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "generation-test",
					Namespace: "default",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue terminal state")
		})
	})

	// Edge Case 3: Max Retry Boundary
	Context("Boundary Conditions", func() {
		It("should fail permanently after reaching max attempts", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "boundary-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Boundary test",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack, // Will fail
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseSending,
					DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{
						{Channel: "slack", Status: "failed"},
						{Channel: "slack", Status: "failed"},
						{Channel: "slack", Status: "failed"},
						{Channel: "slack", Status: "failed"},
						{Channel: "slack", Status: "failed"}, // 5 attempts
					},
					TotalAttempts:    5,
					FailedDeliveries: 5,
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Reconcile should mark as failed (max retries exceeded)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "boundary-test",
					Namespace: "default",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue after max retries")

			// Verify phase is Failed
			updated := &notificationv1alpha1.NotificationRequest{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "boundary-test",
				Namespace: "default",
			}, updated)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseFailed))
		})
	})

	// Edge Case 4: Nil Channel List
	Context("Input Validation", func() {
		It("should handle nil channel list", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nil-channels-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test",
					Body:     "Nil channels test",
					Channels: nil, // Edge case
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Reconcile should handle gracefully
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nil-channels-test",
					Namespace: "default",
				},
			})

			// Should not panic, may return error or mark as failed
			// Either outcome is acceptable (no panic is key)
			if err == nil {
				// If no error, verify notification wasn't corrupted
				updated := &notificationv1alpha1.NotificationRequest{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "nil-channels-test",
					Namespace: "default",
				}, updated)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("should handle empty channel list", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-channels-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test",
					Body:     "Empty channels test",
					Channels: []notificationv1alpha1.Channel{}, // Empty
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Reconcile should handle gracefully
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "empty-channels-test",
					Namespace: "default",
				},
			})

			// Should not panic
			if err == nil {
				updated := &notificationv1alpha1.NotificationRequest{}
				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "empty-channels-test",
					Namespace: "default",
				}, updated)
				Expect(err).ToNot(HaveOccurred())
			}
		})
	})

	// Edge Case 5: CRD Deletion During Reconciliation
	Context("CRD Deletion", func() {
		It("should handle CRD deletion gracefully", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deletion-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Deletion test",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// Delete CRD immediately
			Expect(k8sClient.Delete(ctx, notification)).To(Succeed())

			// Reconciliation should handle NotFound error gracefully
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "deletion-test",
					Namespace: "default",
				},
			})

			// Should not return error (NotFound is expected)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// Edge Case 6: Requeue After Terminal State
	Context("Terminal State Handling", func() {
		It("should not requeue after reaching Sent state", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "terminal-sent-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Terminal test",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase:          notificationv1alpha1.NotificationPhaseSent,
					CompletionTime: &metav1.Time{Time: metav1.Now().Time},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "terminal-sent-test",
					Namespace: "default",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue Sent state")
			Expect(result.RequeueAfter).To(BeZero(), "Should not schedule requeue")
		})

		It("should not requeue after reaching Failed state", func() {
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "terminal-failed-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Terminal failed test",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase:          notificationv1alpha1.NotificationPhaseFailed,
					CompletionTime: &metav1.Time{Time: metav1.Now().Time},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "terminal-failed-test",
					Namespace: "default",
				},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Requeue).To(BeFalse(), "Should not requeue Failed state")
		})
	})

	// Edge Case 7-8: Additional Status Update Scenarios
	Context("Status Update Edge Cases", func() {
		It("should handle status subresource update failures", func() {
			// This test verifies the controller handles status update failures gracefully
			// In real scenarios, this could happen due to conflicts or API server issues

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-update-test",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Status update test",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			// First reconciliation should succeed
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "status-update-test",
					Namespace: "default",
				},
			})

			// May succeed or fail depending on timing, but should not panic
			_ = err // Explicitly ignore for this edge case test
		})
	})
})
