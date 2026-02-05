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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

var _ = Describe("Metadata Preservation Integration Test (Controller → Orchestrator → File)", func() {
	var (
		ctx         context.Context
		tempDir     string
		fileService *delivery.FileDeliveryService
		sanitizer   *sanitization.Sanitizer
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create temporary directory for file output
		tempDir = filepath.Join(os.TempDir(), fmt.Sprintf("metadata-integration-test-%s-process-%d",
			time.Now().Format("20060102-150405"),
			GinkgoParallelProcess()))
		Expect(os.MkdirAll(tempDir, 0755)).To(Succeed())

		fileService = delivery.NewFileDeliveryService(tempDir)
		sanitizer = sanitization.NewSanitizer()
	})

	AfterEach(func() {
		Eventually(func() error {
			return os.RemoveAll(tempDir)
		}, "5s", "100ms").Should(Succeed())
	})

	Context("when simulating full controller flow with Metadata", func() {
		It("should preserve Metadata through sanitization and file delivery (BR-NOT-064)", func() {
			// SCENARIO: E2E test creates NotificationRequest with Metadata for audit correlation
			// EXPECTATION: Metadata preserved through entire delivery pipeline

			// Step 1: Create NotificationRequest (what E2E test does)
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-metadata-e2e",
					Namespace: "notification-e2e",
					UID:       "test-uid-12345",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Subject:  "E2E Test: Critical Priority Notification",
					Body:     "CRITICAL: Testing priority-based routing with file audit trail",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelFile,
					},
					Metadata: map[string]string{
						"severity":               "critical",
						"alert-name":             "CriticalSystemFailure",
						"cluster":                "production",
						"environment":            "prod",
						"remediationRequestName": "rr-pod-crash-abc123",
					},
				},
			}

			// Step 2: Simulate orchestrator's sanitizeNotification() (what controller does)
			sanitized := notification.DeepCopy()
			sanitized.Spec.Subject = sanitizer.Sanitize(notification.Spec.Subject)
			sanitized.Spec.Body = sanitizer.Sanitize(notification.Spec.Body)

			// VALIDATION: Metadata preserved after sanitization
			Expect(sanitized.Spec.Metadata).ToNot(BeNil(), "Metadata must not be nil after sanitization")
			Expect(sanitized.Spec.Metadata).To(HaveLen(5), "All 5 metadata fields must be preserved after sanitization")
			Expect(sanitized.Spec.Metadata["severity"]).To(Equal("critical"), "severity field must be preserved")
			Expect(sanitized.Spec.Metadata["remediationRequestName"]).To(Equal("rr-pod-crash-abc123"), "remediationRequestName must be preserved for BR-NOT-064 correlation")

			// Step 3: Deliver to file (what orchestrator does)
			err := fileService.Deliver(ctx, sanitized)
			Expect(err).ToNot(HaveOccurred(), "File delivery should succeed")

			// Step 4: Read file back (what E2E test does)
			files, err := filepath.Glob(filepath.Join(tempDir, "notification-test-metadata-e2e-*.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveLen(1), "Should create exactly one file")

			data, err := os.ReadFile(files[0])
			Expect(err).ToNot(HaveOccurred())

			var savedNotification notificationv1alpha1.NotificationRequest
			err = json.Unmarshal(data, &savedNotification)
			Expect(err).ToNot(HaveOccurred(), "File should contain valid JSON")

			// FINAL VALIDATION: Metadata preserved in file (this is what E2E test line 169 checks)
			Expect(savedNotification.Spec.Metadata).ToNot(BeNil(), "Metadata must not be nil in saved file")
			Expect(savedNotification.Spec.Metadata).To(HaveLen(5), "All 5 metadata fields must be preserved in file")
			Expect(savedNotification.Spec.Metadata["severity"]).To(Equal("critical"),
				"E2E TEST LINE 169 EQUIVALENT: Metadata['severity'] must equal 'critical' (BR-NOT-064)")
			Expect(savedNotification.Spec.Metadata["remediationRequestName"]).To(Equal("rr-pod-crash-abc123"),
				"remediationRequestName must be preserved for audit correlation")
		})

		It("should handle empty Metadata gracefully (optional field)", func() {
			// SCENARIO: Standalone notification without remediation context
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-metadata",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "No Metadata Test",
					Body:     "Testing nil metadata handling",
					Metadata: nil, // Explicitly nil
				},
			}

			// Sanitize
			sanitized := notification.DeepCopy()
			sanitized.Spec.Subject = sanitizer.Sanitize(notification.Spec.Subject)
			sanitized.Spec.Body = sanitizer.Sanitize(notification.Spec.Body)

			// Deliver
			err := fileService.Deliver(ctx, sanitized)
			Expect(err).ToNot(HaveOccurred())

			// Read file
			files, _ := filepath.Glob(filepath.Join(tempDir, "notification-test-no-metadata-*.json"))
			Expect(files).To(HaveLen(1))
			data, _ := os.ReadFile(files[0])
			var saved notificationv1alpha1.NotificationRequest
			Expect(json.Unmarshal(data, &saved)).To(Succeed())

			// Optional field: omitempty means it may not appear in JSON, which is correct
		})

		It("should preserve Metadata with special characters and long values", func() {
			// EDGE CASE: Metadata values with special characters that might be sanitized
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-special-metadata",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Special Characters Test",
					Body:    "Testing metadata with special characters",
					Metadata: map[string]string{
						"severity":      "critical",
						"special-chars": "value-with-dashes_and_underscores",
						"long-value":    "this-is-a-very-long-value-that-should-not-be-truncated-by-any-processing",
						"unicode":       "✅ 🚨 ⚠️ emoji-test",
					},
				},
			}

			// Full flow
			sanitized := notification.DeepCopy()
			sanitized.Spec.Subject = sanitizer.Sanitize(notification.Spec.Subject)
			sanitized.Spec.Body = sanitizer.Sanitize(notification.Spec.Body)

			err := fileService.Deliver(ctx, sanitized)
			Expect(err).ToNot(HaveOccurred())

			// Validate
			files, _ := filepath.Glob(filepath.Join(tempDir, "notification-test-special-metadata-*.json"))
			data, _ := os.ReadFile(files[0])
			var saved notificationv1alpha1.NotificationRequest
			Expect(json.Unmarshal(data, &saved)).To(Succeed())

			// All special characters preserved
			Expect(saved.Spec.Metadata["special-chars"]).To(Equal("value-with-dashes_and_underscores"))
			Expect(saved.Spec.Metadata["long-value"]).To(Equal("this-is-a-very-long-value-that-should-not-be-truncated-by-any-processing"))
			Expect(saved.Spec.Metadata["unicode"]).To(Equal("✅ 🚨 ⚠️ emoji-test"))
		})
	})
})
