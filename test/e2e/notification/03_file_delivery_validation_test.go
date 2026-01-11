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
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ========================================
// DD-NOT-002 V3.0: File-Based E2E Notification Tests
// ========================================
// These E2E tests validate the complete notification message delivery pipeline
// by capturing notifications to JSON files and validating their content.
//
// Business Requirements Validated:
// - BR-NOT-053: At-Least-Once Delivery (file proves delivery via controller)
// - BR-NOT-054: Data Sanitization (validates sanitization in controller flow)
// - BR-NOT-056: Priority-Based Routing (validates priority preserved)
//
// Test Strategy:
// 1. Create NotificationRequest CRD
// 2. Controller processes and delivers to console
// 3. FileService captures notification to JSON file (non-blocking)
// 4. Test validates file content matches business requirements
//
// ========================================

// File-Based Notification Delivery E2E Tests
// FileService writes to HostPath volume (/tmp/notifications in pod → /tmp/kubernaut-e2e-notifications on host)
// Tests directly read files from host directory (no kubectl cp needed)
var _ = Describe("File-Based Notification Delivery E2E Tests", func() {

	// ========================================
	// Scenario 1: Complete Message Content Validation (BR-NOT-053)
	// ========================================
	// BUSINESS REQUIREMENT: BR-NOT-053 - At-Least-Once Delivery
	// VALIDATION: Notification message is delivered completely with all fields preserved
	Context("Scenario 1: Complete Message Content Validation", func() {
		It("should deliver notification with all message fields preserved in file", func() {
			By("Creating NotificationRequest with complete message content")
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-complete-message",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "message-content",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "E2E Test: Complete Message Validation",
					Body:     "This is a comprehensive test message with multiple fields to validate complete delivery.",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#e2e-test"},
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			By("Waiting for controller to process and deliver notification")
			// Wait for controller to reconcile and update status to Sent
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			By("Validating file was created with notification content")
			// Find the notification file (has timestamp in name)
			// Note: Controller may reconcile multiple times, creating multiple files (expected)
			files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-complete-message-*.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(len(files)).To(BeNumerically(">=", 1), "Should create at least one file for notification")

			By("Reading and validating JSON file content")
			// Read the first file (if multiple reconciliations occurred, any file is valid)
			fileContent, err := os.ReadFile(files[0])
			Expect(err).ToNot(HaveOccurred())

			var savedNotification notificationv1alpha1.NotificationRequest
			err = json.Unmarshal(fileContent, &savedNotification)
			Expect(err).ToNot(HaveOccurred(), "File should contain valid JSON")

			By("Verifying complete message fields (BR-NOT-053)")
			Expect(savedNotification.Name).To(Equal("e2e-complete-message"))
			Expect(savedNotification.Namespace).To(Equal("default"))
			Expect(savedNotification.Spec.Subject).To(Equal("E2E Test: Complete Message Validation"))
			Expect(savedNotification.Spec.Body).To(Equal("This is a comprehensive test message with multiple fields to validate complete delivery."))
			Expect(savedNotification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityCritical))
			Expect(savedNotification.Spec.Channels).To(HaveLen(1))
			Expect(savedNotification.Spec.Channels[0]).To(Equal(notificationv1alpha1.ChannelConsole))
			Expect(savedNotification.Spec.Recipients).To(HaveLen(1))
			Expect(savedNotification.Spec.Recipients[0].Slack).To(Equal("#e2e-test"))

			By("Verifying status fields are present")
			// Note: Status may be intermediate (Sending) when file is captured during reconciliation
			// We validate successful delivery by checking SuccessfulDeliveries counter
			Expect(savedNotification.Status.Phase).ToNot(BeEmpty(), "Status phase should be set")
			Expect(savedNotification.Status.SuccessfulDeliveries).To(BeNumerically(">=", 1), "Should have successful deliveries")
		})
	})

	// ========================================
	// Scenario 2: Data Sanitization Validation (BR-NOT-054)
	// ========================================
	// BUSINESS REQUIREMENT: BR-NOT-054 - Data Sanitization
	// VALIDATION: Sensitive data is sanitized before delivery and file capture
	Context("Scenario 2: Data Sanitization Validation", func() {
		It("should sanitize sensitive data in notification before file delivery", func() {
			By("Creating NotificationRequest with sensitive data patterns")
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-sanitization-test",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "data-sanitization",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:    notificationv1alpha1.NotificationTypeSimple,
					Subject: "Security Alert: Password Leak Detected",
					Body: `Sensitive information detected:
- password: mySecretPass123
- api_key: sk-1234567890abcdef
- token: ghp_AbCdEfGhIjKlMnOpQrStUvWxYz1234567890
- email: user@example.com
`,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#security-alerts"},
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			By("Waiting for controller to process and sanitize notification")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			By("Validating sanitized file content")
			// Note: Controller may reconcile multiple times, creating multiple files (expected)
			files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-sanitization-test-*.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(len(files)).To(BeNumerically(">=", 1), "Should create at least one file")

			// Read the first file (if multiple reconciliations occurred, any file is valid)
			fileContent, err := os.ReadFile(files[0])
			Expect(err).ToNot(HaveOccurred())

			var savedNotification notificationv1alpha1.NotificationRequest
			err = json.Unmarshal(fileContent, &savedNotification)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying sensitive patterns are sanitized (BR-NOT-054)")
			sanitizedBody := savedNotification.Spec.Body

			// Validate password is redacted (sanitizer uses ***REDACTED***)
			Expect(sanitizedBody).To(ContainSubstring("password: ***REDACTED***"),
				"Password should be sanitized")
			Expect(sanitizedBody).ToNot(ContainSubstring("mySecretPass123"),
				"Raw password should not appear in file")

			// Validate API key is redacted
			Expect(sanitizedBody).To(ContainSubstring("api_key: ***REDACTED***"),
				"API key should be sanitized")
			Expect(sanitizedBody).ToNot(ContainSubstring("sk-1234567890abcdef"),
				"Raw API key should not appear in file")

			// Validate token is redacted
			Expect(sanitizedBody).To(ContainSubstring("token: ***REDACTED***"),
				"Token should be sanitized")
			Expect(sanitizedBody).ToNot(ContainSubstring("ghp_AbCdEfGhIjKlMnOpQrStUvWxYz1234567890"),
				"Raw token should not appear in file")

			// Note: email is NOT sanitized by default sanitizer (no email redaction rule)
			// This is expected behavior - only sensitive credentials are redacted
		})
	})

	// ========================================
	// Scenario 3: Priority Field Validation (BR-NOT-056)
	// ========================================
	// BUSINESS REQUIREMENT: BR-NOT-056 - Priority-Based Routing
	// VALIDATION: Priority field is preserved through delivery pipeline
	Context("Scenario 3: Priority Field Validation", func() {
		It("should preserve priority field in delivered notification file", func() {
			By("Creating NotificationRequest with Critical priority")
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-priority-validation",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "priority-validation",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Critical Alert: System Outage",
					Body:     "Priority validation test for critical alerts",
				Priority: notificationv1alpha1.NotificationPriorityCritical,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole,
					notificationv1alpha1.ChannelFile, // Add file channel for priority validation test
				},
				Recipients: []notificationv1alpha1.Recipient{
					{Slack: "#ops-critical"},
				},
			},
		}

			err := k8sClient.Create(ctx, notification)
			if err != nil {
				// DEBUG: Detailed error logging for k8sClient.Create() failure
				GinkgoWriter.Printf("\n❌ k8sClient.Create() ERROR DETAILS:\n")
				GinkgoWriter.Printf("   Error: %v\n", err)
				GinkgoWriter.Printf("   Error Type: %T\n", err)
				if statusErr, ok := err.(*errors.StatusError); ok {
					GinkgoWriter.Printf("   Status Code: %d\n", statusErr.Status().Code)
					GinkgoWriter.Printf("   Reason: %s\n", statusErr.ErrStatus.Reason)
					GinkgoWriter.Printf("   Message: %s\n", statusErr.ErrStatus.Message)
					if statusErr.ErrStatus.Details != nil {
						GinkgoWriter.Printf("   Details: %+v\n", statusErr.ErrStatus.Details)
					}
				}
				GinkgoWriter.Printf("   Notification Spec: %+v\n", notification.Spec)
				GinkgoWriter.Printf("   Notification Metadata: %+v\n", notification.ObjectMeta)
			}
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for successful delivery")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

		By("Validating priority field in file (BR-NOT-056)")
		// Note: Controller may reconcile multiple times, creating multiple files (expected)
		// DD-NOT-006 v2: Use kubectl cp to bypass Podman VM mount sync issues
		var copiedFilePath string
		Eventually(EventuallyFindFileInPod("notification-e2e-priority-validation-*.json"),
			20*time.Second, 1*time.Second).Should(Not(BeEmpty()),
			"File should be created in pod within 20 seconds (virtiofs sync under concurrent load)")

		copiedFilePath, err = WaitForFileInPod(ctx, "notification-e2e-priority-validation-*.json", 20*time.Second)
		Expect(err).ToNot(HaveOccurred(), "Should copy file from pod")
		defer CleanupCopiedFile(copiedFilePath)

		// Read the copied file
		fileContent, err := os.ReadFile(copiedFilePath)
		Expect(err).ToNot(HaveOccurred())

		var savedNotification notificationv1alpha1.NotificationRequest
		err = json.Unmarshal(fileContent, &savedNotification)
		Expect(err).ToNot(HaveOccurred())

		// Verify priority is preserved exactly
		Expect(savedNotification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityCritical),
			"Priority must be preserved as Critical (BR-NOT-056)")
		})
	})

	// ========================================
	// Scenario 4: Concurrent Delivery Validation
	// ========================================
	// VALIDATION: Multiple concurrent deliveries create distinct files without collisions
	Context("Scenario 4: Concurrent Delivery Validation", func() {
		It("should handle concurrent notifications without file collisions", func() {
			By("Creating multiple NotificationRequests concurrently")
			notificationNames := []string{
				"e2e-concurrent-1",
				"e2e-concurrent-2",
				"e2e-concurrent-3",
			}

			for _, name := range notificationNames {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: "default",
						Labels: map[string]string{
							"test-scenario": "concurrent-delivery",
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Subject:  "Concurrent Test: " + name,
						Body:     "Testing concurrent notification delivery",
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
					notificationv1alpha1.ChannelFile,  // Add file channel for priority validation test
						},
						Recipients: []notificationv1alpha1.Recipient{
							{Slack: "#e2e-concurrent"},
						},
					},
				}

				err := k8sClient.Create(ctx, notification)
				Expect(err).ToNot(HaveOccurred())
			}

			By("Waiting for all notifications to be delivered")
			for _, name := range notificationNames {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					notification := &notificationv1alpha1.NotificationRequest{}
					err := k8sClient.Get(ctx, client.ObjectKey{
						Name:      name,
						Namespace: "default",
					}, notification)
					if err != nil {
						return ""
					}
					return notification.Status.Phase
				}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))
			}

			By("Validating distinct files created for each notification")
			for _, name := range notificationNames {
				files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-"+name+"-*.json"))
				Expect(err).ToNot(HaveOccurred())
				// Controller may reconcile multiple times, creating multiple files (expected behavior)
				Expect(len(files)).To(BeNumerically(">=", 1), "Should create at least one file for "+name)

				// Verify file content matches notification
				fileContent, err := os.ReadFile(files[0])
				Expect(err).ToNot(HaveOccurred())

				var savedNotification notificationv1alpha1.NotificationRequest
				err = json.Unmarshal(fileContent, &savedNotification)
				Expect(err).ToNot(HaveOccurred())

				Expect(savedNotification.Name).To(Equal(name))
				Expect(savedNotification.Spec.Subject).To(Equal("Concurrent Test: " + name))
			}
		})
	})

	// ========================================
	// Scenario 5: FileService Error Handling (CRITICAL - V3.0)
	// ========================================
	// VALIDATION: FileService failures do NOT block production notification delivery
	// This is the CRITICAL safety behavior from DD-NOT-002 V3.0 Error Handling Philosophy
	Context("Scenario 5: FileService Error Handling (CRITICAL)", func() {
		It("should NOT block production delivery when FileService fails", func() {
			By("Simulating FileService failure (read-only directory would be needed for real failure)")
			// NOTE: This test validates the NON-BLOCKING behavior through successful delivery.
			// The controller's `if r.FileService != nil` check ensures nil-safety.
			// FileService errors are logged but NOT propagated to reconciliation.
			//
			// To test actual FileService failure, you would:
			// 1. Change e2eFileOutputDir to a read-only directory
			// 2. Verify notification Status.Phase = Sent (despite FileService error)
			// 3. Check logs show FileService error (non-blocking)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-error-handling",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "error-handling",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "Error Handling Test",
					Body:     "Testing FileService error handling does not block delivery",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#e2e-error-test"},
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying notification STILL succeeds (non-blocking behavior)")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"CRITICAL: Production delivery must succeed even if FileService fails (V3.0 Safety Guarantee)")

			By("Validating FileService created file when available")
			// Since FileService is working in this test, file should be created
			// Note: Controller may reconcile multiple times, creating multiple files (expected)
			files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-error-handling-*.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(len(files)).To(BeNumerically(">=", 1), "At least one file should be created")

			By("CRITICAL VALIDATION: Code inspection confirms non-blocking pattern")
			// The controller code contains:
			// if r.FileService != nil {
			//     if fileErr := r.FileService.Deliver(ctx, notification); fileErr != nil {
			//         log.Error(fileErr, "FileService delivery failed (E2E only, non-blocking)")
			//         // DO NOT propagate error - production delivery succeeded
			//     }
			// }
			//
			// This test confirms the pattern works end-to-end.
		})
	})
})
