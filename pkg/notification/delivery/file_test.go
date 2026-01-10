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

package delivery_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

var _ = Describe("FileDeliveryService", func() {
	var (
		ctx     context.Context
		service delivery.Service
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Directory Creation Error Handling (NT-BUG-006)", func() {
		It("should wrap directory creation errors as retryable", func() {
			By("Creating a read-only parent directory")
			tempDir := GinkgoT().TempDir()
			readOnlyDir := filepath.Join(tempDir, "readonly")
			Expect(os.Mkdir(readOnlyDir, 0555)).To(Succeed()) // Read-only parent

			// Attempt to create a subdirectory in read-only parent
			invalidDir := filepath.Join(readOnlyDir, "cannot-create-this")

			service = delivery.NewFileDeliveryService(invalidDir)

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

			service = delivery.NewFileDeliveryService(writableDir)

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

			service = delivery.NewFileDeliveryService(readOnlyFileDir)

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
