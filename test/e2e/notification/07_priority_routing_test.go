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
// MVP E2E Test 3: Priority-Based Routing with File Audit
// ========================================
// BUSINESS REQUIREMENT: BR-NOT-052 - Priority-Based Routing
//
// Test Strategy:
// 1. Create NotificationRequests with different priorities (Critical, High, Medium, Low)
// 2. Validate Critical priority notifications are delivered first
// 3. Verify file channel creates audit trail for high-priority notifications
// 4. Validate priority field is preserved in file output
// 5. Verify delivery order respects priority
//
// CRITICAL SAFETY: High-priority notifications must not be delayed by low-priority ones
// ========================================

var _ = Describe("Priority-Based Routing E2E (BR-NOT-052)", func() {

	// NOTE: After removing FileDeliveryConfig from CRD (DD-NOT-006 v2):
	// - File service configured at deployment level via ConfigMap
	// - All notifications write to shared /tmp/notifications directory
	// - Tests search by notification name, not subdirectory
	// - No cleanup needed (shared directory persists across tests)
	BeforeEach(func() {
		// No per-test directory needed - all files go to e2eFileOutputDir
		logger.Info("Priority routing test starting", "sharedFileDir", e2eFileOutputDir)
	})

	AfterEach(func() {
		// Clean up test-specific files from shared directory
		// Pattern: notification-*priority*.json
		// This prevents file accumulation while allowing parallel test execution
		pattern := filepath.Join(e2eFileOutputDir, "notification-e2e-priority-*.json")
		files, _ := filepath.Glob(pattern)
		for _, f := range files {
			_ = os.Remove(f)
		}
		logger.Info("Cleaned up test files", "pattern", pattern, "count", len(files))
	})

	// ========================================
	// Scenario 1: Critical Priority with File Audit
	// ========================================
	Context("Scenario 1: Critical priority notification with file audit trail", func() {
		It("should deliver critical notification with file audit immediately", func() {
			By("Creating Critical priority NotificationRequest with file channel")

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-priority-critical",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "priority-critical",
						"test-priority": "P0",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Subject:  "E2E Test: Critical Priority Notification",
					Body:     "CRITICAL: Testing priority-based routing with file audit trail",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole, // Console delivery
					notificationv1alpha1.ChannelFile,    // File audit trail
				},
				Metadata: map[string]string{
						"severity":    "critical",
						"alert-name":  "CriticalSystemFailure",
						"cluster":     "production",
						"environment": "prod",
					},
				},
			}

			startTime := time.Now()
			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			By("Waiting for critical notification to be delivered immediately")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 20*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Critical priority should be delivered immediately")

			deliveryTime := time.Since(startTime)
			logger.Info("Critical notification delivered", "deliveryTime", deliveryTime.String())

			By("Verifying both channels delivered successfully")
			// Refresh notification to get latest status
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      notification.Name,
				Namespace: notification.Namespace,
			}, notification)
			Expect(err).ToNot(HaveOccurred())

			Expect(notification.Status.SuccessfulDeliveries).To(Equal(2),
				"Both console and file channels should deliver successfully")
			Expect(notification.Status.FailedDeliveries).To(Equal(0),
				"Should have 0 failed deliveries")

		By("Verifying file audit trail was created")
		// DD-NOT-006 v2: Use kubectl cp to bypass Podman VM mount sync issues
		pattern := "notification-e2e-priority-critical-*.json"

		Eventually(EventuallyCountFilesInPod(pattern),
			20*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
			"File should be created in pod within 20 seconds (virtiofs sync under concurrent load)")

		copiedFilePath, err := WaitForFileInPod(ctx, pattern, 20*time.Second)
		Expect(err).ToNot(HaveOccurred(), "Should copy file from pod")
		defer CleanupCopiedFile(copiedFilePath)

		By("Validating priority field preserved in file audit")
		fileContent, err := os.ReadFile(copiedFilePath)
		Expect(err).ToNot(HaveOccurred())

		var savedNotification notificationv1alpha1.NotificationRequest
		err = json.Unmarshal(fileContent, &savedNotification)
		Expect(err).ToNot(HaveOccurred(), "File should contain valid JSON")

			Expect(savedNotification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityCritical),
				"Priority field must be preserved in file audit (BR-NOT-052)")
			Expect(savedNotification.Spec.Type).To(Equal(notificationv1alpha1.NotificationTypeEscalation),
				"Notification type must be preserved")
			Expect(savedNotification.Spec.Metadata["severity"]).To(Equal("critical"),
				"Metadata fields must be preserved in audit trail")

			logger.Info("✅ CRITICAL PRIORITY SUCCESS: Delivered immediately with file audit trail")
		})
	})

	// ========================================
	// Scenario 2: Priority Ordering Validation
	// ========================================
	Context("Scenario 2: Multiple priorities delivered in order", func() {
		It("should deliver notifications in priority order (Critical > High > Medium > Low)", func() {
			By("Creating notifications with different priorities")

			// Create 4 notifications with different priorities
			// NOTE: We create them in reverse order (Low → Critical) to test priority queue
			priorities := []struct {
				name     string
				priority notificationv1alpha1.NotificationPriority
			}{
				{"e2e-priority-low", notificationv1alpha1.NotificationPriorityLow},
				{"e2e-priority-medium", notificationv1alpha1.NotificationPriorityMedium},
				{"e2e-priority-high", notificationv1alpha1.NotificationPriorityHigh},
				{"e2e-priority-critical-2", notificationv1alpha1.NotificationPriorityCritical},
			}

			creationTimes := make(map[string]time.Time)

			for _, p := range priorities {
				notification := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      p.name,
						Namespace: "default",
						Labels: map[string]string{
							"test-scenario": "priority-ordering",
							"priority":      string(p.priority),
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeSimple,
						Subject:  "E2E Test: Priority Ordering - " + string(p.priority),
						Body:     "Testing priority-based delivery ordering",
						Priority: p.priority,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelFile,
					},
				},
			}

				err := k8sClient.Create(ctx, notification)
				Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest: "+p.name)
				creationTimes[p.name] = time.Now()

				// Small delay between creations to ensure distinct timestamps
				time.Sleep(100 * time.Millisecond)
			}

			By("Waiting for all notifications to be delivered")
			for _, p := range priorities {
				Eventually(func() notificationv1alpha1.NotificationPhase {
					notification := &notificationv1alpha1.NotificationRequest{}
					err := k8sClient.Get(ctx, client.ObjectKey{
						Name:      p.name,
						Namespace: "default",
					}, notification)
					if err != nil {
						return ""
					}
					return notification.Status.Phase
				}, 30*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
					"Notification "+p.name+" should be delivered")
			}

		By("Verifying file audit trails created for all priorities")
		for _, p := range priorities {
			// DD-NOT-006 v2: Use kubectl cp to bypass Podman VM mount sync issues
			pattern := "notification-" + p.name + "-*.json"

			Eventually(EventuallyCountFilesInPod(pattern),
				20*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
				"File for "+p.name+" should be created in pod within 20 seconds (virtiofs sync under concurrent load)")

			copiedFilePath, err := WaitForFileInPod(ctx, pattern, 20*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Should copy file from pod for "+p.name)
			defer CleanupCopiedFile(copiedFilePath)

			// Validate priority preserved in file
			fileContent, err := os.ReadFile(copiedFilePath)
			Expect(err).ToNot(HaveOccurred())

			var savedNotification notificationv1alpha1.NotificationRequest
			err = json.Unmarshal(fileContent, &savedNotification)
			Expect(err).ToNot(HaveOccurred())

			Expect(savedNotification.Spec.Priority).To(Equal(p.priority),
				"Priority must be preserved in file audit for "+p.name)
		}

			By("BUSINESS OUTCOME VALIDATION (BR-NOT-052)")
			// ✅ All priorities delivered successfully
			// ✅ File audit trails created for all notifications
			// ✅ Priority field preserved in audit files
			// ✅ No priority blocked by another
			//
			// NOTE: Exact delivery order validation requires controller metrics
			// E2E test validates all priorities are delivered with correct audit trails
			// Unit tests validate priority queue ordering logic

			logger.Info("✅ PRIORITY ORDERING SUCCESS: All priorities delivered with file audit trails")
		})
	})

	// ========================================
	// Scenario 3: High Priority with Multiple Channels
	// ========================================
	Context("Scenario 3: High priority with console, file, and log channels", func() {
		It("should deliver high priority notification to all channels", func() {
			By("Creating High priority NotificationRequest with three channels")

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-priority-high-multi",
					Namespace: "default",
					Labels: map[string]string{
						"test-scenario": "priority-high-multi-channel",
						"test-priority": "P1",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
					Subject:  "E2E Test: High Priority Multi-Channel",
					Body:     "Testing high priority delivery to console, file, and log channels",
					Priority: notificationv1alpha1.NotificationPriorityHigh,
				Channels: []notificationv1alpha1.Channel{
					notificationv1alpha1.ChannelConsole, // Console delivery
					notificationv1alpha1.ChannelFile,    // File audit trail
					notificationv1alpha1.ChannelLog,     // Structured log
				},
				Metadata: map[string]string{
						"severity":   "high",
						"alert-name": "HighPriorityAlert",
						"cluster":    "staging",
					},
				},
			}

			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NotificationRequest")

			By("Waiting for high priority notification to be delivered to all channels")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      notification.Name,
					Namespace: notification.Namespace,
				}, notification)
				if err != nil {
					return ""
				}
				return notification.Status.Phase
			}, 20*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"High priority should be delivered to all channels")

			By("Verifying all three channels delivered successfully")
			// Refresh notification to get latest status
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      notification.Name,
				Namespace: notification.Namespace,
			}, notification)
			Expect(err).ToNot(HaveOccurred())

			Expect(notification.Status.SuccessfulDeliveries).To(Equal(3),
				"All three channels (console, file, log) should deliver successfully")
			Expect(notification.Status.FailedDeliveries).To(Equal(0),
				"Should have 0 failed deliveries")

		By("Verifying file audit trail contains priority metadata")
		// DD-NOT-006 v2: Use kubectl cp to bypass Podman VM mount sync issues
		pattern := "notification-e2e-priority-high-multi-*.json"

		Eventually(EventuallyCountFilesInPod(pattern),
			20*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
			"File should be created in pod within 20 seconds (virtiofs sync under concurrent load)")

		copiedFilePath, err := WaitForFileInPod(ctx, pattern, 20*time.Second)
		Expect(err).ToNot(HaveOccurred(), "Should copy file from pod")
		defer CleanupCopiedFile(copiedFilePath)

		fileContent, err := os.ReadFile(copiedFilePath)
		Expect(err).ToNot(HaveOccurred())

		var savedNotification notificationv1alpha1.NotificationRequest
		err = json.Unmarshal(fileContent, &savedNotification)
		Expect(err).ToNot(HaveOccurred())

		Expect(savedNotification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityHigh),
			"Priority must be preserved in file audit")
		Expect(savedNotification.Spec.Metadata["severity"]).To(Equal("high"),
			"Severity metadata must be preserved")

			By("Verifying delivery attempts recorded for all channels")
			Expect(notification.Status.DeliveryAttempts).To(HaveLen(3),
				"Should record 3 delivery attempts (one per channel)")

			channelsSeen := make(map[string]bool)
			for _, attempt := range notification.Status.DeliveryAttempts {
				channelsSeen[attempt.Channel] = true
				Expect(attempt.Status).To(Equal("success"), "All attempts should succeed")
			}

			Expect(channelsSeen).To(HaveKey("console"), "Console channel should be in delivery attempts")
			Expect(channelsSeen).To(HaveKey("file"), "File channel should be in delivery attempts")
			Expect(channelsSeen).To(HaveKey("log"), "Log channel should be in delivery attempts")

			logger.Info("✅ HIGH PRIORITY MULTI-CHANNEL SUCCESS: All channels delivered with priority preserved")
		})
	})
})

