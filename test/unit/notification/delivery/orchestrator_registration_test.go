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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Mock delivery service for testing
type mockDeliveryService struct {
	deliverFunc func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error
}

func (m *mockDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	if m.deliverFunc != nil {
		return m.deliverFunc(ctx, notification)
	}
	return nil
}

var _ = Describe("Orchestrator Channel Registration (DD-NOT-007)", func() {
	var (
		orchestrator  *delivery.Orchestrator
		mockService   *mockDeliveryService
		sanitizer     *sanitization.Sanitizer
		metrics       notificationmetrics.Recorder
		statusManager *notificationstatus.Manager
		logger        = ctrl.Log.WithName("test-orchestrator")
		ctx           = context.Background()
	)

	BeforeEach(func() {
		// Create orchestrator WITHOUT channel parameters (DD-NOT-007)
		orchestrator = delivery.NewOrchestrator(
			sanitizer,
			metrics,
			statusManager,
			logger,
		)

		// Create mock service
		mockService = &mockDeliveryService{
			deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
				return nil // Success by default
			},
		}
	})

	Describe("RegisterChannel", func() {
		It("should register a new channel successfully", func() {
			// Register mock service
			orchestrator.RegisterChannel("test-channel", mockService)

			// Verify channel is registered
			Expect(orchestrator.HasChannel("test-channel")).To(BeTrue())
		})

		It("should skip registration if service is nil", func() {
			// Attempt to register nil service
			orchestrator.RegisterChannel("nil-channel", nil)

			// Verify channel is NOT registered
			Expect(orchestrator.HasChannel("nil-channel")).To(BeFalse())
		})

		It("should allow overwriting existing channel", func() {
			// Register first service
			firstService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return errors.New("first service")
				},
			}
			orchestrator.RegisterChannel("overwrite-channel", firstService)

			// Register second service (overwrite)
			secondService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return errors.New("second service")
				},
			}
			orchestrator.RegisterChannel("overwrite-channel", secondService)

			// Verify second service is used
			notification := helpers.NewNotificationRequest("test", "default")
			err := orchestrator.DeliverToChannel(ctx, notification, "overwrite-channel")
			Expect(err).To(MatchError("second service"))
		})
	})

	Describe("UnregisterChannel", func() {
		It("should remove a registered channel", func() {
			// Register channel
			orchestrator.RegisterChannel("remove-me", mockService)
			Expect(orchestrator.HasChannel("remove-me")).To(BeTrue())

			// Unregister channel
			orchestrator.UnregisterChannel("remove-me")
			Expect(orchestrator.HasChannel("remove-me")).To(BeFalse())
		})

		It("should be safe to unregister non-existent channel", func() {
			// Unregister non-existent channel (should not panic)
			Expect(func() {
				orchestrator.UnregisterChannel("non-existent")
			}).NotTo(Panic())
		})
	})

	Describe("HasChannel", func() {
		It("should return true for registered channel", func() {
			orchestrator.RegisterChannel("exists", mockService)
			Expect(orchestrator.HasChannel("exists")).To(BeTrue())
		})

		It("should return false for unregistered channel", func() {
			Expect(orchestrator.HasChannel("does-not-exist")).To(BeFalse())
		})
	})

	Describe("DeliverToChannel with Registration (DD-NOT-007)", func() {
		It("should deliver to registered channel successfully", func() {
			// Register channel
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockService)

			// Attempt delivery
			notification := helpers.NewNotificationRequest("test", "default")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelConsole)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error if channel not registered", func() {
			// Do NOT register channel

			// Attempt delivery to unregistered channel
			notification := helpers.NewNotificationRequest("test", "default")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelConsole)

			// Verify descriptive error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("channel not registered"))
			Expect(err.Error()).To(ContainSubstring("console"))
		})

		It("should delegate to registered service", func() {
			// Track if service was called
			called := false
			trackedService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					called = true
					return nil
				},
			}

			// Register tracked service
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), trackedService)

			// Deliver
			notification := helpers.NewNotificationRequest("test", "default")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelSlack)

			// Verify service was called
			Expect(err).ToNot(HaveOccurred())
			Expect(called).To(BeTrue())
		})

		It("should propagate service errors", func() {
			// Service that returns error
			failingService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return errors.New("delivery failed")
				},
			}

			// Register failing service
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), failingService)

			// Deliver
			notification := helpers.NewNotificationRequest("test", "default")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelFile)

			// Verify error propagated
			Expect(err).To(MatchError("delivery failed"))
		})
	})

	Describe("Registration Flexibility for Tests", func() {
		It("should allow registering only needed channels", func() {
			// Test scenario: Only need console for this test
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockService)

			// Should succeed for registered channel
			notification := helpers.NewNotificationRequest("test", "default")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelConsole)
			Expect(err).ToNot(HaveOccurred())

			// Should fail for unregistered channels
			err = orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelSlack)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("channel not registered"))
		})
	})

	Describe("DD-NOT-007 Compliance", func() {
		It("should have no hardcoded channel fields in struct", func() {
			// This test verifies the refactoring removed hardcoded fields
			// If struct still has consoleService, slackService fields, this documents the issue

			// Create orchestrator
			o := delivery.NewOrchestrator(sanitizer, metrics, statusManager, logger)

			// Verify channels must be registered
			Expect(o.HasChannel("console")).To(BeFalse(), "console should not be hardcoded")
			Expect(o.HasChannel("slack")).To(BeFalse(), "slack should not be hardcoded")
			Expect(o.HasChannel("file")).To(BeFalse(), "file should not be hardcoded")
			Expect(o.HasChannel("log")).To(BeFalse(), "log should not be hardcoded")
		})
	})
})
