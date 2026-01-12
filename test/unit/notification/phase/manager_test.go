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

package phase

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationphase "github.com/jordigilh/kubernaut/pkg/notification/phase"
)

var _ = Describe("Notification Phase Manager", func() {
	var (
		manager      *notificationphase.Manager
		notification *notificationv1.NotificationRequest
	)

	BeforeEach(func() {
		manager = notificationphase.NewManager()
		notification = &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-notification",
				Namespace: "default",
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type:     notificationv1.NotificationTypeSimple,
				Priority: notificationv1.NotificationPriorityMedium,
			},
		}
	})

	Describe("CurrentPhase", func() {
		Context("When Status.Phase is empty (initial state)", func() {
			It("should return Pending", func() {
				notification.Status.Phase = ""
				Expect(manager.CurrentPhase(notification)).To(Equal(notificationphase.Pending))
			})
		})

		Context("When Status.Phase is set", func() {
			It("should return Sending phase", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSending
				Expect(manager.CurrentPhase(notification)).To(Equal(notificationphase.Sending))
			})

			It("should return Sent phase", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSent
				Expect(manager.CurrentPhase(notification)).To(Equal(notificationphase.Sent))
			})

			It("should return Retrying phase", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseRetrying
				Expect(manager.CurrentPhase(notification)).To(Equal(notificationphase.Retrying))
			})
		})
	})

	Describe("TransitionTo", func() {
		Context("Valid transitions", func() {
			It("should allow Pending → Sending", func() {
				notification.Status.Phase = notificationv1.NotificationPhasePending
				err := manager.TransitionTo(notification, notificationphase.Sending)
				Expect(err).ToNot(HaveOccurred())
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseSending))
			})

			It("should allow Sending → Sent", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSending
				err := manager.TransitionTo(notification, notificationphase.Sent)
				Expect(err).ToNot(HaveOccurred())
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseSent))
			})

			It("should allow Sending → Retrying", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSending
				err := manager.TransitionTo(notification, notificationphase.Retrying)
				Expect(err).ToNot(HaveOccurred())
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseRetrying))
			})

			It("should allow Sending → Failed", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSending
				err := manager.TransitionTo(notification, notificationphase.Failed)
				Expect(err).ToNot(HaveOccurred())
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseFailed))
			})

			It("should allow Retrying → Sent (retry success)", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseRetrying
				err := manager.TransitionTo(notification, notificationphase.Sent)
				Expect(err).ToNot(HaveOccurred())
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseSent))
			})

			It("should allow Retrying → PartiallySent (retry exhausted)", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseRetrying
				err := manager.TransitionTo(notification, notificationphase.PartiallySent)
				Expect(err).ToNot(HaveOccurred())
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhasePartiallySent))
			})

			It("should handle initial transition correctly", func() {
				// When Status.Phase is empty, CurrentPhase returns Pending (initial state)
				// The manager then validates Pending → Pending transition
				// This is a no-op but should be valid since we're already in Pending
				notification.Status.Phase = ""
				// Verify CurrentPhase returns Pending for empty phase
				Expect(manager.CurrentPhase(notification)).To(Equal(notificationphase.Pending))

				// Transition to Sending from initial state
				err := manager.TransitionTo(notification, notificationphase.Sending)
				Expect(err).ToNot(HaveOccurred())
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseSending))
			})
		})

		Context("Invalid transitions", func() {
			It("should reject Pending → Sent (skipping Sending)", func() {
				notification.Status.Phase = notificationv1.NotificationPhasePending
				err := manager.TransitionTo(notification, notificationphase.Sent)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid phase transition"))
				Expect(err.Error()).To(ContainSubstring("Pending"))
				Expect(err.Error()).To(ContainSubstring("Sent"))
				// Phase should remain unchanged after failed transition
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhasePending))
			})

			It("should reject Sent → Sending (terminal state)", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSent
				err := manager.TransitionTo(notification, notificationphase.Sending)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid phase transition"))
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseSent))
			})

			It("should reject PartiallySent → Retrying (terminal state)", func() {
				notification.Status.Phase = notificationv1.NotificationPhasePartiallySent
				err := manager.TransitionTo(notification, notificationphase.Retrying)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid phase transition"))
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhasePartiallySent))
			})

			It("should reject Failed → Pending (terminal state)", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseFailed
				err := manager.TransitionTo(notification, notificationphase.Pending)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid phase transition"))
				Expect(notification.Status.Phase).To(Equal(notificationv1.NotificationPhaseFailed))
			})
		})
	})

	Describe("IsInTerminalState", func() {
		Context("Terminal phases", func() {
			It("should return true for Sent", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSent
				Expect(manager.IsInTerminalState(notification)).To(BeTrue())
			})

			It("should return true for PartiallySent", func() {
				notification.Status.Phase = notificationv1.NotificationPhasePartiallySent
				Expect(manager.IsInTerminalState(notification)).To(BeTrue())
			})

			It("should return true for Failed", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseFailed
				Expect(manager.IsInTerminalState(notification)).To(BeTrue())
			})
		})

		Context("Non-terminal phases", func() {
			It("should return false for Pending", func() {
				notification.Status.Phase = notificationv1.NotificationPhasePending
				Expect(manager.IsInTerminalState(notification)).To(BeFalse())
			})

			It("should return false for Sending", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseSending
				Expect(manager.IsInTerminalState(notification)).To(BeFalse())
			})

			It("should return false for Retrying", func() {
				notification.Status.Phase = notificationv1.NotificationPhaseRetrying
				Expect(manager.IsInTerminalState(notification)).To(BeFalse())
			})

			It("should return false for empty phase (initial state)", func() {
				notification.Status.Phase = ""
				Expect(manager.IsInTerminalState(notification)).To(BeFalse())
			})
		})
	})

	Describe("Manager lifecycle", func() {
		It("should create a non-nil manager", func() {
			mgr := notificationphase.NewManager()
			Expect(mgr).ToNot(BeNil())
		})

		It("should be reusable across multiple notifications", func() {
			notification1 := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "notif-1", Namespace: "default"},
				Status:     notificationv1.NotificationRequestStatus{Phase: notificationv1.NotificationPhasePending},
			}
			notification2 := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "notif-2", Namespace: "default"},
				Status:     notificationv1.NotificationRequestStatus{Phase: notificationv1.NotificationPhaseSending},
			}

			// Use same manager for both
			Expect(manager.CurrentPhase(notification1)).To(Equal(notificationphase.Pending))
			Expect(manager.CurrentPhase(notification2)).To(Equal(notificationphase.Sending))

			// Transition both
			Expect(manager.TransitionTo(notification1, notificationphase.Sending)).To(Succeed())
			Expect(manager.TransitionTo(notification2, notificationphase.Sent)).To(Succeed())

			Expect(notification1.Status.Phase).To(Equal(notificationv1.NotificationPhaseSending))
			Expect(notification2.Status.Phase).To(Equal(notificationv1.NotificationPhaseSent))
		})
	})
})
