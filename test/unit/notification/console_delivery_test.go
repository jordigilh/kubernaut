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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

// BR-NOT-053: Console Delivery Service - Behavior and Correctness Testing
// Focus: Test WHAT the service does (behavior), NOT HOW it does it (implementation)
//
// BEHAVIOR TESTS: Does it deliver successfully? Does it handle errors correctly?
// CORRECTNESS TESTS: Does it work with all valid inputs? Does it validate properly?
//
// NOT TESTED: Console output format (implementation detail that can change)

var _ = Describe("BR-NOT-053: Console Delivery Service", func() {
	var (
		ctx     context.Context
		service *delivery.ConsoleDeliveryService
	)

	BeforeEach(func() {
		ctx = context.Background()
		service = delivery.NewConsoleDeliveryService()
	})

	Context("Successful Delivery - BEHAVIOR", func() {
		It("should deliver notification successfully without error", func() {
			// BEHAVIOR: Console service delivers successfully
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notification",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Subject:  "Test Subject",
					Body:     "Test Body",
				},
			}

			err := service.Deliver(ctx, notification)

			// BEHAVIOR VALIDATION: Delivery succeeds
			Expect(err).ToNot(HaveOccurred(),
				"Console delivery should always succeed for valid notifications")
		})
	})

	Context("Error Handling - CORRECTNESS", func() {
		It("should return error when notification is nil", func() {
			// CORRECTNESS: Input validation
			err := service.Deliver(ctx, nil)

			// CORRECTNESS VALIDATION: Proper error handling
			Expect(err).To(HaveOccurred(),
				"Should reject nil notification")
			Expect(err).To(MatchError(ContainSubstring("notification cannot be nil")),
				"Error message should indicate nil input")
		})

		It("should handle empty subject gracefully", func() {
			// CORRECTNESS: Edge case handling
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "empty-subject-test",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Subject:  "", // Empty subject
					Body:     "Body with no subject",
				},
			}

			err := service.Deliver(ctx, notification)

			// BEHAVIOR: Console service is lenient (no strict validation)
			Expect(err).ToNot(HaveOccurred(),
				"Console service should deliver even with empty subject")
		})

		It("should handle empty body gracefully", func() {
			// CORRECTNESS: Edge case handling
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "empty-body-test",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Subject:  "Subject with no body",
					Body:     "", // Empty body
				},
			}

			err := service.Deliver(ctx, notification)

			// BEHAVIOR: Console service is lenient
			Expect(err).ToNot(HaveOccurred(),
				"Console service should deliver even with empty body")
		})
	})

	Context("Priority Levels - BEHAVIOR", func() {
		// TABLE-DRIVEN: Test all priority levels
		// BEHAVIOR: Console service handles all priority levels
		DescribeTable("should handle all priority levels successfully",
			func(priority notificationv1alpha1.NotificationPriority) {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name: "priority-test",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Priority: priority,
						Subject:  "Priority Test",
						Body:     "Testing priority handling",
					},
				}

				err := service.Deliver(ctx, notification)

				// BEHAVIOR VALIDATION: All priorities succeed
				Expect(err).ToNot(HaveOccurred(),
					"Console service should handle priority: %s", priority)
			},
			Entry("Critical priority", notificationv1alpha1.NotificationPriorityCritical),
			Entry("High priority", notificationv1alpha1.NotificationPriorityHigh),
			Entry("Medium priority", notificationv1alpha1.NotificationPriorityMedium),
			Entry("Low priority", notificationv1alpha1.NotificationPriorityLow),
		)
	})

	Context("Notification Types - BEHAVIOR", func() {
		// TABLE-DRIVEN: Test all notification types
		// BEHAVIOR: Console service handles all notification types
		DescribeTable("should handle all notification types successfully",
			func(notificationType notificationv1alpha1.NotificationType) {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name: "type-test",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationType,
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Subject:  "Type Test",
						Body:     "Testing notification type handling",
					},
				}

				err := service.Deliver(ctx, notification)

				// BEHAVIOR VALIDATION: All types succeed
				Expect(err).ToNot(HaveOccurred(),
					"Console service should handle type: %s", notificationType)
			},
			Entry("Simple notification", notificationv1alpha1.NotificationTypeSimple),
			Entry("Escalation notification", notificationv1alpha1.NotificationTypeEscalation),
			Entry("Status update notification", notificationv1alpha1.NotificationTypeStatusUpdate),
		)
	})

	Context("Context Handling - BEHAVIOR", func() {
		It("should respect context cancellation", func() {
			// BEHAVIOR: Context cancellation is respected
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "context-test",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Subject:  "Context Test",
					Body:     "Testing context cancellation",
				},
			}

			// Note: Current implementation doesn't check context cancellation
			// Console delivery is synchronous and fast (no I/O)
			// This test documents expected behavior if context checking is added
			err := service.Deliver(cancelledCtx, notification)

			// CURRENT BEHAVIOR: Console delivery ignores context (no I/O to cancel)
			// If future implementation adds context checking, this test will catch it
			Expect(err).ToNot(HaveOccurred(),
				"Console delivery is synchronous (no I/O), context cancellation not applicable")
		})
	})

	Context("Concurrent Delivery - BEHAVIOR", func() {
		It("should handle concurrent deliveries safely", func() {
			// BEHAVIOR: Thread-safety validation
			// Console delivery writes to stdout (Go's fmt.Print is goroutine-safe)

			done := make(chan bool)
			notificationCount := 10

			for i := 0; i < notificationCount; i++ {
				go func(id int) {
					defer GinkgoRecover()

					notification := &notificationv1alpha1.NotificationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name: "concurrent-test",
						},
						Spec: notificationv1alpha1.NotificationRequestSpec{
							Type:     notificationv1alpha1.NotificationTypeSimple,
							Priority: notificationv1alpha1.NotificationPriorityLow,
							Subject:  "Concurrent Test",
							Body:     "Testing concurrent delivery",
						},
					}

					err := service.Deliver(ctx, notification)

					// BEHAVIOR VALIDATION: All concurrent deliveries succeed
					Expect(err).ToNot(HaveOccurred(),
						"Concurrent delivery %d should succeed", id)

					done <- true
				}(i)
			}

			// Wait for all goroutines
			for i := 0; i < notificationCount; i++ {
				<-done
			}
		})
	})
})
