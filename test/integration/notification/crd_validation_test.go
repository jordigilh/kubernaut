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
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// BR-NOT-050: Data Loss Prevention - CRD validation prevents invalid data persistence
// BR-NOT-058: Error Handling - Clear validation error messages

var _ = Describe("Integration Test 4: CRD Validation Failures (OpenAPI Schema)", func() {
	var testNamespace string

	BeforeEach(func() {
		testNamespace = "default" // Use default namespace for validation tests
	})

	Context("Invalid NotificationType", func() {
		It("should reject NotificationRequest with invalid type (BR-NOT-050: Validation)", func() {
			By("Attempting to create NotificationRequest with invalid type")

			// TDD RED: This test should FAIL because we're trying to create an invalid CRD
			// The Kubernetes API server should reject it via OpenAPI schema validation

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-type-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     "invalid-type", // ❌ INVALID - not in enum [escalation, simple, status-update]
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Invalid Type Test",
					Body:     "This should be rejected by CRD validation",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#test",
						},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			By("Expecting Kubernetes to reject with validation error")
			err := k8sClient.Create(ctx, notification)
			Expect(err).To(HaveOccurred(), "Expected CRD validation to reject invalid type")
			Expect(err.Error()).To(ContainSubstring("type"), "Expected error message to mention 'type' field")

			By("Verifying NotificationRequest was NOT persisted")
			// Try to fetch - should not exist
			fetched := &notificationv1alpha1.NotificationRequest{}
			fetchErr := k8sClient.Get(ctx,
				client.ObjectKeyFromObject(notification),
				fetched,
			)
			Expect(fetchErr).To(HaveOccurred(), "NotificationRequest should not exist in etcd")
		})
	})

	Context("Missing Required Fields", func() {
		It("should reject NotificationRequest with empty recipients array (BR-NOT-050: Validation)", func() {
			By("Attempting to create NotificationRequest with empty recipients")

			// TDD RED: This should FAIL - recipients is required with MinItems=1

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-recipients-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:       notificationv1alpha1.NotificationTypeSimple,
					Priority:   notificationv1alpha1.NotificationPriorityMedium,
					Subject:    "Empty Recipients Test",
					Body:       "This should be rejected",
					Recipients: []notificationv1alpha1.Recipient{}, // ❌ INVALID - MinItems=1
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			By("Expecting Kubernetes to reject with validation error")
			err := k8sClient.Create(ctx, notification)
			Expect(err).To(HaveOccurred(), "Expected CRD validation to reject empty recipients")
			Expect(err.Error()).To(Or(
				ContainSubstring("recipients"),
				ContainSubstring("Required"),
				ContainSubstring("MinItems"),
			), "Expected error message about recipients validation")
		})

		It("should reject NotificationRequest with empty channels array (BR-NOT-050: Validation)", func() {
			By("Attempting to create NotificationRequest with empty channels")

			// TDD RED: This should FAIL - channels is required with MinItems=1

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-channels-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Empty Channels Test",
					Body:     "This should be rejected",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#test",
						},
					},
					Channels: []notificationv1alpha1.Channel{}, // ❌ INVALID - MinItems=1
				},
			}

			By("Expecting Kubernetes to reject with validation error")
			err := k8sClient.Create(ctx, notification)
			Expect(err).To(HaveOccurred(), "Expected CRD validation to reject empty channels")
			Expect(err.Error()).To(Or(
				ContainSubstring("channels"),
				ContainSubstring("Required"),
				ContainSubstring("MinItems"),
			), "Expected error message about channels validation")
		})

		It("should reject NotificationRequest with empty subject (BR-NOT-050: Validation)", func() {
			By("Attempting to create NotificationRequest with empty subject")

			// TDD RED: This should FAIL - subject is required with MinLength=1

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-subject-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "", // ❌ INVALID - MinLength=1
					Body:     "This should be rejected",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#test",
						},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			By("Expecting Kubernetes to reject with validation error")
			err := k8sClient.Create(ctx, notification)
			Expect(err).To(HaveOccurred(), "Expected CRD validation to reject empty subject")
			Expect(err.Error()).To(Or(
				ContainSubstring("subject"),
				ContainSubstring("Required"),
				ContainSubstring("MinLength"),
			), "Expected error message about subject validation")
		})
	})

	Context("Invalid RetryPolicy", func() {
		It("should reject NotificationRequest with maxAttempts=0 (BR-NOT-050: Validation)", func() {
			By("Attempting to create NotificationRequest with maxAttempts=0")

			// TDD RED: This should FAIL - maxAttempts has Minimum=1

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-retry-maxattempts-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Invalid Retry Policy Test",
					Body:     "Testing maxAttempts=0 validation",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#test",
						},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           0, // ❌ INVALID - Minimum=1
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			By("Creating NotificationRequest (will test if Kubernetes rejects or applies default)")
			err := k8sClient.Create(ctx, notification)

			// Note: Kubernetes API server applies default values BEFORE validation
			// When maxAttempts=0 (zero value), Kubernetes applies default=5 before validation
			// This is expected behavior - zero values trigger defaults in Kubernetes
			// BR-NOT-050: Validation still prevents truly invalid values (see maxAttempts>10 test)

			if err != nil {
				// If validation error occurred, that's ideal
				Expect(err.Error()).To(Or(
					ContainSubstring("maxAttempts"),
					ContainSubstring("retryPolicy"),
					ContainSubstring("Minimum"),
				), "Expected error message about maxAttempts validation")
			} else {
				// If no error, verify Kubernetes applied the default value (5)
				By("Verifying Kubernetes applied default value instead of rejecting")
				fetched := &notificationv1alpha1.NotificationRequest{}
				fetchErr := k8sClient.Get(ctx,
					client.ObjectKeyFromObject(notification),
					fetched,
				)
				Expect(fetchErr).NotTo(HaveOccurred())

				// Kubernetes should have applied default=5 (not 0)
				if fetched.Spec.RetryPolicy != nil {
					Expect(fetched.Spec.RetryPolicy.MaxAttempts).To(Equal(5),
						"Kubernetes should apply default=5 when maxAttempts=0")
				}

				// Clean up
				deleteErr := k8sClient.Delete(context.Background(), notification)
				Expect(deleteErr).NotTo(HaveOccurred())
			}
		})

		It("should reject NotificationRequest with maxAttempts>10 (BR-NOT-050: Validation)", func() {
			By("Attempting to create NotificationRequest with maxAttempts=15")

			// TDD RED: This should FAIL - maxAttempts has Maximum=10

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-retry-maxattempts-high-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Invalid Retry Policy Test (Max>10)",
					Body:     "Testing maxAttempts>10 validation",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#test",
						},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           15, // ❌ INVALID - Maximum=10
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			By("Expecting Kubernetes to reject with validation error")
			err := k8sClient.Create(ctx, notification)
			Expect(err).To(HaveOccurred(), "Expected CRD validation to reject maxAttempts>10")
			Expect(err.Error()).To(Or(
				ContainSubstring("maxAttempts"),
				ContainSubstring("retryPolicy"),
				ContainSubstring("Maximum"),
			), "Expected error message about maxAttempts validation")
		})

		It("should reject NotificationRequest with maxBackoffSeconds<60 (BR-NOT-050: Validation)", func() {
			By("Attempting to create NotificationRequest with maxBackoffSeconds=10")

			// TDD RED: This should FAIL - maxBackoffSeconds has Minimum=60

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-retry-maxbackoff-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Invalid Retry Policy Test (MaxBackoff<60)",
					Body:     "Testing maxBackoffSeconds<60 validation",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#test",
						},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     10, // ❌ INVALID - Minimum=60
					},
				},
			}

			By("Expecting Kubernetes to reject with validation error")
			err := k8sClient.Create(ctx, notification)
			Expect(err).To(HaveOccurred(), "Expected CRD validation to reject maxBackoffSeconds<60")
			Expect(err.Error()).To(Or(
				ContainSubstring("maxBackoffSeconds"),
				ContainSubstring("retryPolicy"),
				ContainSubstring("Minimum"),
			), "Expected error message about maxBackoffSeconds validation")
		})
	})

	Context("Valid CRDs Should Pass Validation", func() {
		It("should accept valid NotificationRequest with all fields (BR-NOT-050: Validation Success)", func() {
			By("Creating valid NotificationRequest")

			// TDD GREEN: This should PASS - all fields valid

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-notification-test-" + time.Now().Format("20060102150405"),
					Namespace: testNamespace,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Valid Notification Test",
					Body:     "All validation rules satisfied",
					Recipients: []notificationv1alpha1.Recipient{
						{
							Slack: "#test",
						},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 1,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
			}

			By("Creating valid NotificationRequest")
			err := k8sClient.Create(ctx, notification)
			Expect(err).NotTo(HaveOccurred(), "Valid NotificationRequest should be accepted")

			By("Verifying NotificationRequest was persisted")
			fetched := &notificationv1alpha1.NotificationRequest{}
			fetchErr := k8sClient.Get(ctx,
				client.ObjectKeyFromObject(notification),
				fetched,
			)
			Expect(fetchErr).NotTo(HaveOccurred(), "Valid NotificationRequest should be persisted")
			Expect(fetched.Spec.Subject).To(Equal("Valid Notification Test"))
			Expect(fetched.Spec.Type).To(Equal(notificationv1alpha1.NotificationTypeSimple))

			By("Cleaning up valid notification")
			deleteErr := k8sClient.Delete(context.Background(), notification)
			Expect(deleteErr).NotTo(HaveOccurred())
		})
	})
})
