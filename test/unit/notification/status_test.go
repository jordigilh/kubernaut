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
	"context"
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

		statusManager = status.NewManager(fakeClient, fakeClient)
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
		// BR-NOT-051: FailedDeliveries tracks UNIQUE channels, not attempts
		// DD-E2E-003: 3 attempts for same channel = 1 failed channel
		Expect(updated.Status.FailedDeliveries).To(Equal(1))
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

				err := statusManager.UpdatePhase(ctx, notification, newPhase, "TestReason", "Test message", nil)

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
			err := statusManager.UpdatePhase(ctx, notification, notificationv1alpha1.NotificationPhaseSent, "AllDeliveriesSucceeded", "All channels delivered", nil)
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

	// Issue #79 Phase 1b: Conditions parameter in AtomicStatusUpdate and UpdatePhase
	Context("UT-NT-079-001: AtomicStatusUpdate conditions persistence", func() {
		It("should persist conditions passed through the conditions parameter", func() {
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cond-atomic-test",
					Namespace:  "kubernaut-notifications",
					Generation: 2,
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseSending,
				},
			}
			Expect(fakeClient.Create(ctx, nr)).To(Succeed())

			conditions := []metav1.Condition{
				{
					Type:               "RoutingResolved",
					Status:             metav1.ConditionTrue,
					Reason:             "RoutingRuleMatched",
					Message:            "Matched rule 'prod-critical'",
					ObservedGeneration: 2,
				},
			}

			err := statusManager.AtomicStatusUpdate(
				ctx, nr,
				notificationv1alpha1.NotificationPhaseSent,
				"Delivered", "All channels succeeded",
				nil, // no delivery attempts
				conditions,
			)
			Expect(err).ToNot(HaveOccurred())

			updated := &notificationv1alpha1.NotificationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      "cond-atomic-test",
				Namespace: "kubernaut-notifications",
			}, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent))
			Expect(updated.Status.Conditions).To(HaveLen(1))
			Expect(updated.Status.Conditions[0].Type).To(Equal("RoutingResolved"))
			Expect(updated.Status.Conditions[0].Reason).To(Equal("RoutingRuleMatched"))
		})

		It("should accept nil conditions without error", func() {
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cond-nil-test",
					Namespace:  "kubernaut-notifications",
					Generation: 1,
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: notificationv1alpha1.NotificationPhaseSending,
				},
			}
			Expect(fakeClient.Create(ctx, nr)).To(Succeed())

			err := statusManager.AtomicStatusUpdate(
				ctx, nr,
				notificationv1alpha1.NotificationPhaseSent,
				"Delivered", "OK",
				nil, nil,
			)
			Expect(err).ToNot(HaveOccurred())

			updated := &notificationv1alpha1.NotificationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      "cond-nil-test",
				Namespace: "kubernaut-notifications",
			}, updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(BeEmpty())
		})
	})

	Context("UT-NT-079-002: UpdatePhase conditions persistence", func() {
		It("should persist conditions passed through the conditions parameter", func() {
			nr := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cond-phase-test",
					Namespace:  "kubernaut-notifications",
					Generation: 1,
				},
				Status: notificationv1alpha1.NotificationRequestStatus{},
			}
			Expect(fakeClient.Create(ctx, nr)).To(Succeed())

			conditions := []metav1.Condition{
				{
					Type:               "RoutingResolved",
					Status:             metav1.ConditionTrue,
					Reason:             "RoutingFallback",
					Message:            "Using console fallback",
					ObservedGeneration: 1,
				},
			}

			err := statusManager.UpdatePhase(
				ctx, nr,
				notificationv1alpha1.NotificationPhasePending,
				"Initialized", "Notification request received",
				conditions,
			)
			Expect(err).ToNot(HaveOccurred())

			updated := &notificationv1alpha1.NotificationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      "cond-phase-test",
				Namespace: "kubernaut-notifications",
			}, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhasePending))
			Expect(updated.Status.Conditions).To(HaveLen(1))
			Expect(updated.Status.Conditions[0].Reason).To(Equal("RoutingFallback"))
		})
	})
})
