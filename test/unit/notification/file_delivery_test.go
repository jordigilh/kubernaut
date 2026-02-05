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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

// File delivery service tests are now part of the notification test suite
// No need for separate test registration - handled by suite_test.go

var _ = Describe("FileDeliveryService Unit Tests", func() {
	var (
		ctx         context.Context
		fileService *delivery.FileDeliveryService
		tempDir     string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create temporary directory for tests
		// Include GinkgoParallelProcess() to ensure unique directories across parallel tests
		tempDir = filepath.Join(os.TempDir(), fmt.Sprintf("file-delivery-unit-test-%s-process-%d",
			time.Now().Format("20060102-150405"),
			GinkgoParallelProcess()))
		Expect(os.MkdirAll(tempDir, 0755)).To(Succeed())

		fileService = delivery.NewFileDeliveryService(tempDir)
	})

	AfterEach(func() {
		// Clean up temporary directory
		// Use Eventually to handle filesystem delays (especially on macOS)
		Eventually(func() error {
			return os.RemoveAll(tempDir)
		}, "5s", "100ms").Should(Succeed())
	})

	Context("when delivering notification to file", func() {
		It("should create file with complete notification content (BR-NOT-053)", func() {
			// BUSINESS SCENARIO: Deliver notification to file for E2E validation
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notification",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test Subject",
					Body:     "Test Body",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
				},
			}

			// BEHAVIOR: FileDeliveryService writes notification to JSON file
			err := fileService.Deliver(ctx, notification)

			// CORRECTNESS: File created with correct content
			Expect(err).ToNot(HaveOccurred(), "Delivery should succeed")

			// Find created file (has timestamp in name)
			files, err := filepath.Glob(filepath.Join(tempDir, "notification-test-notification-*.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveLen(1), "Should create exactly one file")

			// Read and validate content
			data, err := os.ReadFile(files[0])
			Expect(err).ToNot(HaveOccurred())

			var savedNotification notificationv1alpha1.NotificationRequest
			err = json.Unmarshal(data, &savedNotification)
			Expect(err).ToNot(HaveOccurred())

			// Validate complete message content (BR-NOT-053: At-Least-Once Delivery)
			Expect(savedNotification.Name).To(Equal("test-notification"))
			Expect(savedNotification.Spec.Subject).To(Equal("Test Subject"))
			Expect(savedNotification.Spec.Body).To(Equal("Test Body"))
			Expect(savedNotification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityCritical))
			Expect(savedNotification.Spec.Channels).To(HaveLen(1))
			Expect(savedNotification.Spec.Recipients).To(HaveLen(1))
		})

		It("should create unique files for concurrent deliveries (thread safety)", func() {
			// BUSINESS SCENARIO: High-throughput alert processing
			notifications := make([]*notificationv1alpha1.NotificationRequest, 3)
			for i := 0; i < 3; i++ {
				notifications[i] = &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("concurrent-test-%d", i),
						Namespace: "default",
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject: fmt.Sprintf("Concurrent Test %d", i),
						Body:    "Testing concurrent delivery",
					},
				}
			}

			// BEHAVIOR: Concurrent deliveries should create distinct files
			var wg sync.WaitGroup
			errChan := make(chan error, 3)

			for _, notification := range notifications {
				// Launch concurrent deliveries
				// Note: Filename uniqueness is ensured by notification name + timestamp in filename
				wg.Add(1)
				go func(n *notificationv1alpha1.NotificationRequest) {
					defer wg.Done()
					if err := fileService.Deliver(ctx, n); err != nil {
						errChan <- err
					}
				}(notification)
			}

			wg.Wait()
			close(errChan)

			// CORRECTNESS: No errors
			for err := range errChan {
				Expect(err).ToNot(HaveOccurred(), "Concurrent deliveries should succeed")
			}

			// CORRECTNESS: 3 distinct files created (unique names ensure no collisions)
			files, err := filepath.Glob(filepath.Join(tempDir, "notification-concurrent-test-*-*.json"))
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveLen(3), "Should create 3 distinct files (thread-safe)")
		})

		It("should create output directory if it doesn't exist", func() {
			// BUSINESS SCENARIO: First run in E2E environment
			newDir := filepath.Join(tempDir, "nested", "directory")
			service := delivery.NewFileDeliveryService(newDir)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dir-creation",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Directory Creation Test",
				},
			}

			// BEHAVIOR: Service creates directory structure
			err := service.Deliver(ctx, notification)

			// CORRECTNESS: Directory created and file written
			Expect(err).ToNot(HaveOccurred(), "Should create nested directories")

			// Verify directory exists
			info, err := os.Stat(newDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())

			// Verify file created
			files, _ := filepath.Glob(filepath.Join(newDir, "*.json"))
			Expect(files).To(HaveLen(1))
		})
	})

	Context("when encountering errors", func() {
		It("should return error for invalid directory permissions", func() {
			// BUSINESS SCENARIO: E2E environment misconfiguration
			// Create read-only directory
			readOnlyDir := filepath.Join(tempDir, "readonly")
			Expect(os.MkdirAll(readOnlyDir, 0444)).To(Succeed()) // Read-only
			defer func() {
				_ = os.Chmod(readOnlyDir, 0755) // Ignore error in cleanup
			}()

			service := delivery.NewFileDeliveryService(filepath.Join(readOnlyDir, "subdir"))

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-permissions",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Permission Test",
				},
			}

			// BEHAVIOR: Service returns error for permission issues
			err := service.Deliver(ctx, notification)

			// CORRECTNESS: Error returned (not panic)
			Expect(err).To(HaveOccurred(), "Should return error for permission issues")
			Expect(err.Error()).To(ContainSubstring("failed to create output directory"))
		})

		// NOTE: "Repeated deliveries" test removed (was flaky due to timing dependencies)
		// This scenario is comprehensively covered in E2E tests:
		// See: test/e2e/notification/03_file_delivery_validation_test.go
		// - Scenario 1: Complete message content (controller reconciliation creates multiple files)
		// - Scenario 4: Concurrent file delivery (validates distinct files without collisions)
		// Rationale: Unit tests should not depend on filesystem timing or wall-clock time.
		// E2E tests provide better validation of retry behavior with real controller reconciliation.
		// For triage details, see: docs/services/crd-controllers/06-notification/FLAKY-UNIT-TEST-TRIAGE.md
	})

	Context("when validating message content", func() {
		It("should preserve priority field in delivered message (BR-NOT-056)", func() {
			// BUSINESS SCENARIO: Critical alert prioritization
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-priority",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Priority Test",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
				},
			}

			// BEHAVIOR: Deliver notification with priority
			err := fileService.Deliver(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// Read file
			files, _ := filepath.Glob(filepath.Join(tempDir, "notification-test-priority-*.json"))
			Expect(files).To(HaveLen(1))

			data, _ := os.ReadFile(files[0])
			var saved notificationv1alpha1.NotificationRequest
			Expect(json.Unmarshal(data, &saved)).To(Succeed())

			// CORRECTNESS: Priority preserved (BR-NOT-056)
			Expect(saved.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityCritical))
		})

		It("should preserve recipients in delivered message", func() {
			// BUSINESS SCENARIO: Multi-channel notification routing
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-recipients",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Recipients Test",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#channel1"},
						{Slack: "#channel2"},
						{Slack: "#channel3"},
					},
				},
			}

			// BEHAVIOR: Deliver multi-recipient notification
			err := fileService.Deliver(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// Read file
			files, _ := filepath.Glob(filepath.Join(tempDir, "notification-test-recipients-*.json"))
			data, _ := os.ReadFile(files[0])
			var saved notificationv1alpha1.NotificationRequest
			Expect(json.Unmarshal(data, &saved)).To(Succeed())

			// CORRECTNESS: All recipients preserved
			Expect(saved.Spec.Recipients).To(HaveLen(3))
			Expect(saved.Spec.Recipients[0].Slack).To(Equal("#channel1"))
			Expect(saved.Spec.Recipients[1].Slack).To(Equal("#channel2"))
			Expect(saved.Spec.Recipients[2].Slack).To(Equal("#channel3"))
		})

		It("should preserve metadata fields in delivered message (BR-NOT-064)", func() {
			// BUSINESS SCENARIO: Audit correlation with RemediationRequest context
			// BR-NOT-064: Audit event correlation requires metadata preservation
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-metadata",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Metadata Preservation Test",
					Body:    "Testing metadata field preservation for audit correlation",
					Metadata: map[string]string{
						"severity":               "critical",
						"remediationRequestName": "rr-pod-crash-abc123",
						"cluster":                "production",
						"environment":            "prod",
					},
				},
			}

			// BEHAVIOR: Deliver notification with metadata
			err := fileService.Deliver(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// Read file
			files, _ := filepath.Glob(filepath.Join(tempDir, "notification-test-metadata-*.json"))
			Expect(files).To(HaveLen(1), "Should create exactly one file")
			data, _ := os.ReadFile(files[0])
			var saved notificationv1alpha1.NotificationRequest
			Expect(json.Unmarshal(data, &saved)).To(Succeed())

			// CORRECTNESS: Metadata map preserved (BR-NOT-064)
			Expect(saved.Spec.Metadata).ToNot(BeNil(), "Metadata map must not be nil when explicitly set")
			Expect(saved.Spec.Metadata).To(HaveLen(4), "All metadata fields must be preserved")
			Expect(saved.Spec.Metadata["severity"]).To(Equal("critical"), "severity field must be preserved")
			Expect(saved.Spec.Metadata["remediationRequestName"]).To(Equal("rr-pod-crash-abc123"), "remediationRequestName field must be preserved for audit correlation")
			Expect(saved.Spec.Metadata["cluster"]).To(Equal("production"), "cluster field must be preserved")
			Expect(saved.Spec.Metadata["environment"]).To(Equal("prod"), "environment field must be preserved")
		})

		It("should handle nil metadata gracefully (optional field)", func() {
			// BUSINESS SCENARIO: Standalone notifications without RemediationRequest context
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-metadata",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "No Metadata Test",
					Body:     "Testing nil metadata handling",
					Metadata: nil, // Explicitly nil (optional field)
				},
			}

			// BEHAVIOR: Deliver notification without metadata
			err := fileService.Deliver(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			// Read file
			files, _ := filepath.Glob(filepath.Join(tempDir, "notification-test-no-metadata-*.json"))
			Expect(files).To(HaveLen(1))
			data, _ := os.ReadFile(files[0])
			var saved notificationv1alpha1.NotificationRequest
			Expect(json.Unmarshal(data, &saved)).To(Succeed())

			// CORRECTNESS: Nil metadata is acceptable (optional field per CRD definition)
			// Note: omitempty means it may be omitted from JSON, but that's correct behavior
		})
	})

	// ========================================
	// Error Handling Tests (NT-BUG-006)
	// Merged from pkg/notification/delivery/file_test.go
	// ========================================
	Context("Directory Creation Error Handling (NT-BUG-006)", func() {
		It("should wrap directory creation errors as retryable", func() {
			By("Creating a read-only parent directory")
			tempDir := GinkgoT().TempDir()
			readOnlyDir := filepath.Join(tempDir, "readonly")
			Expect(os.Mkdir(readOnlyDir, 0555)).To(Succeed()) // Read-only parent

			// Attempt to create a subdirectory in read-only parent
			invalidDir := filepath.Join(readOnlyDir, "cannot-create-this")

			service := delivery.NewFileDeliveryService(invalidDir)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notification",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test Directory Permission Error",
					Body:    "Testing NT-BUG-006: Directory creation errors should be retryable",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelFile,
					},
				},
			}

			By("Attempting delivery with permission denied error")
			err := service.Deliver(ctx, notification)
			Expect(err).To(HaveOccurred(), "Delivery should fail with permission denied")

			By("Verifying error is wrapped as RetryableError")
			var retryableErr *delivery.RetryableError
			Expect(err).To(BeAssignableToTypeOf(retryableErr),
				"Directory creation error should be wrapped as RetryableError (NT-BUG-006)")

			By("Verifying error message contains directory creation failure")
			Expect(err.Error()).To(ContainSubstring("failed to create output directory"),
				"Error message should indicate directory creation failure")
		})

		It("should succeed when directory is writable", func() {
			By("Creating a writable directory")
			tempDir := GinkgoT().TempDir()
			writableDir := filepath.Join(tempDir, "writable")

			service := delivery.NewFileDeliveryService(writableDir)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notification-success",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test Successful Delivery",
					Body:    "Testing that delivery succeeds with writable directory",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelFile,
					},
				},
			}

			By("Attempting delivery with writable directory")
			err := service.Deliver(ctx, notification)
			Expect(err).ToNot(HaveOccurred(), "Delivery should succeed with writable directory")

			By("Verifying file was created")
			files, err := os.ReadDir(writableDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).To(HaveLen(1), "Exactly one notification file should be created")
		})
	})

	Context("File Write Error Handling (NT-BUG-006)", func() {
		It("should wrap file write errors as retryable", func() {
			By("Creating a directory and making it read-only after creation")
			tempDir := GinkgoT().TempDir()
			readOnlyFileDir := filepath.Join(tempDir, "readonly-files")
			Expect(os.Mkdir(readOnlyFileDir, 0755)).To(Succeed())

			// Make directory read-only (no write permission)
			Expect(os.Chmod(readOnlyFileDir, 0555)).To(Succeed())

			service := delivery.NewFileDeliveryService(readOnlyFileDir)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notification-file-write",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test File Write Error",
					Body:    "Testing NT-BUG-006: File write errors should be retryable",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelFile,
					},
				},
			}

			By("Attempting delivery with write permission denied")
			err := service.Deliver(ctx, notification)
			Expect(err).To(HaveOccurred(), "Delivery should fail with write permission denied")

			By("Verifying error is wrapped as RetryableError")
			var retryableErr *delivery.RetryableError
			Expect(err).To(BeAssignableToTypeOf(retryableErr),
				"File write error should be wrapped as RetryableError (NT-BUG-006)")

			By("Verifying error message contains file write failure")
			Expect(err.Error()).To(ContainSubstring("failed to write temporary file"),
				"Error message should indicate file write failure")
		})
	})
})
