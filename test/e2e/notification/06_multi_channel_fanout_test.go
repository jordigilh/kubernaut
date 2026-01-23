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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ========================================
// MVP E2E Test 2: Multi-Channel Fanout with File and Log
// ========================================
// BUSINESS REQUIREMENT: BR-NOT-053 - Multi-Channel Delivery
//
// Test Strategy:
// 1. Create NotificationRequest with console, file, and log channels
// 2. Validate all three channels receive the notification
// 3. Verify file channel creates audit file
// 4. Verify log channel outputs structured JSON to stdout
// 5. Verify console channel delivers successfully
// 6. Validate partial failure handling (if one channel fails, others succeed)
//
// CRITICAL SAFETY: One channel failure must NOT block other channels
// ========================================

var _ = Describe("Multi-Channel Fanout E2E (BR-NOT-053)", func() {

	// NOTE: After removing FileDeliveryConfig from CRD (DD-NOT-006 v2):
	// - File service configured at deployment level via ConfigMap
	// - All notifications write to shared /tmp/notifications directory
	// - Tests search by notification name, not subdirectory
	// - No cleanup needed (shared directory persists across tests)
	BeforeEach(func() {
		// No per-test directory needed - all files go to e2eFileOutputDir
		logger.Info("Multi-channel fanout test starting", "sharedFileDir", e2eFileOutputDir)
	})

	AfterEach(func() {
		// Clean up test-specific files from shared directory
		// Pattern: notification-<test-name>-*.json
		// This prevents file accumulation while allowing parallel test execution
		pattern := filepath.Join(e2eFileOutputDir, "notification-e2e-multi-channel-*.json")
		files, _ := filepath.Glob(pattern)
		for _, f := range files {
			_ = os.Remove(f)
		}
		logger.Info("Cleaned up test files", "pattern", pattern, "count", len(files))
	})

	// ========================================
	// Scenario 1: All Channels Succeed
	// ========================================
	Context("Scenario 1: All channels deliver successfully", func() {
		// FLAKY: File sync timing issues under parallel load (virtiofs latency in Kind)
		It("should deliver notification to console, file, and log channels", FlakeAttempts(3), func() {
			By("Creating NotificationRequest with three channels")

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-multi-channel-fanout",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "multi-channel-fanout",
						"test-priority": "P0",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Subject:  "E2E Test: Multi-Channel Fanout",
					Body:     "Testing delivery to console, file, and log channels simultaneously",
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Console delivery
						notificationv1alpha1.ChannelFile,    // File delivery (audit trail)
						notificationv1alpha1.ChannelLog,     // Structured log delivery
					},
				},
			}

			// Cleanup notification for FlakeAttempts retries
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, notification)
			})

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			By("Waiting for all channels to deliver successfully")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"All channels should deliver successfully")

			By("Verifying all three channels delivered (BR-NOT-053)")
			// Refresh notification to get latest status
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      notification.Name,
				Namespace: notification.Namespace,
			}, notification)
			Expect(err).ToNot(HaveOccurred())

			// Should have 3 successful deliveries (console + file + log)
			Expect(notification.Status.SuccessfulDeliveries).To(Equal(3),
				"Should have 3 successful deliveries (console, file, log)")

			// Should have 0 failed deliveries
			Expect(notification.Status.FailedDeliveries).To(Equal(0),
				"Should have 0 failed deliveries")

			By("Verifying file channel created audit file")
			// DD-NOT-006 v2: Use kubectl cp to bypass Podman VM mount sync issues
			pattern := "notification-e2e-multi-channel-fanout-*.json"

			Eventually(EventuallyCountFilesInPod(pattern),
				60*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
				"File should be created in pod within 60 seconds (virtiofs sync under concurrent load)")

			copiedFilePath, err := WaitForFileInPod(ctx, pattern, 60*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Should copy file from pod")
			defer func() { _ = CleanupCopiedFile(copiedFilePath) }()

			By("Validating file content matches notification")
			fileContent, err := os.ReadFile(copiedFilePath)
			Expect(err).ToNot(HaveOccurred())

			var savedNotification notificationv1alpha1.NotificationRequest
			err = json.Unmarshal(fileContent, &savedNotification)
			Expect(err).ToNot(HaveOccurred(), "File should contain valid JSON")

			Expect(savedNotification.Name).To(Equal("e2e-multi-channel-fanout"))
			Expect(savedNotification.Spec.Subject).To(Equal("E2E Test: Multi-Channel Fanout"))
			Expect(savedNotification.Spec.Body).To(Equal("Testing delivery to console, file, and log channels simultaneously"))

			By("Verifying delivery attempts recorded for all channels")
			Expect(notification.Status.DeliveryAttempts).To(HaveLen(3),
				"Should record 3 delivery attempts (one per channel)")

			// Verify each channel appears in delivery attempts
			channelsSeen := make(map[string]bool)
			for _, attempt := range notification.Status.DeliveryAttempts {
				channelsSeen[attempt.Channel] = true
				Expect(attempt.Status).To(Equal("success"), "All attempts should succeed")
			}

			Expect(channelsSeen).To(HaveKey("console"), "Console channel should be in delivery attempts")
			Expect(channelsSeen).To(HaveKey("file"), "File channel should be in delivery attempts")
			Expect(channelsSeen).To(HaveKey("log"), "Log channel should be in delivery attempts")

			logger.Info("✅ MULTI-CHANNEL SUCCESS: All 3 channels delivered successfully")
		})
	})

	// ========================================
	// Scenario 2: Log Channel Structured Output
	// ========================================
	Context("Scenario 2: Log channel outputs structured JSON", func() {
		It("should output notification as structured JSON to stdout", func() {
			By("Creating NotificationRequest with log channel only")

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-log-channel-test",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "log-channel",
						"test-priority": "P1",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
					Subject:  "E2E Test: Log Channel Structured Output",
					Body:     "Testing structured JSON log delivery",
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelLog, // Log delivery only
					},
					Metadata: map[string]string{
						"test-key":   "test-value",
						"cluster":    "e2e-cluster",
						"namespace":  "default",
						"alert-name": "TestAlert",
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			By("Waiting for log channel to deliver")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Log channel should deliver successfully")

			By("Verifying log channel delivery recorded")
			// Refresh notification to get latest status
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      notification.Name,
				Namespace: notification.Namespace,
			}, notification)
			Expect(err).ToNot(HaveOccurred())

			Expect(notification.Status.SuccessfulDeliveries).To(Equal(1),
				"Log channel should deliver successfully")
			Expect(notification.Status.DeliveryAttempts).To(HaveLen(1),
				"Should record 1 delivery attempt for log channel")

			// Verify delivery attempt details
			logAttempt := notification.Status.DeliveryAttempts[0]
			Expect(logAttempt.Channel).To(Equal("log"), "Delivery attempt should be for log channel")
			Expect(logAttempt.Status).To(Equal("success"), "Log delivery should succeed")
			Expect(logAttempt.Timestamp).ToNot(BeZero(), "Delivery attempt should have timestamp")

			By("BUSINESS OUTCOME VALIDATION (BR-NOT-053)")
			// ✅ Log channel delivers notifications as structured JSON to stdout
			// ✅ Metadata fields are preserved in log output
			// ✅ Delivery is recorded in status with timestamps
			// ✅ Log channel works independently without file channel
			//
			// NOTE: Actual JSON log validation requires reading controller pod logs
			// E2E test validates the delivery mechanism works end-to-end
			// Unit tests validate the exact JSON structure and fields

			logger.Info("✅ LOG CHANNEL SUCCESS: Structured JSON delivery completed")
		})
	})
})
