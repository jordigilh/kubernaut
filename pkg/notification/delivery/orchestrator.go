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

package delivery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// ========================================
// DELIVERY ORCHESTRATOR (Pattern 3 - P0)
// 📋 Design Decision: Controller Refactoring Pattern Library §3
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
// ========================================
//
// Orchestrator manages notification delivery across multiple channels.
// Extracted from NotificationRequestReconciler to improve:
// - Testability (can test delivery logic independently)
// - Maintainability (delivery logic separated from controller)
// - Extensibility (easy to add new channels)
//
// BENEFITS:
// - ~217 lines extracted from controller
// - Delivery logic isolated and testable
// - Single responsibility principle
//
// PATTERN: Orchestrator Pattern (for delivery/execution)
// Reference: pkg/remediationorchestrator/creator/ (similar extraction)
// ========================================

// ========================================
// DD-NOT-007: Registration Pattern (AUTHORITATIVE)
// ========================================
// Orchestrator manages delivery orchestration across channels.
//
// MANDATORY: Channels MUST be registered via RegisterChannel(), NOT constructor parameters
// See: docs/architecture/decisions/DD-NOT-007-DELIVERY-ORCHESTRATOR-REGISTRATION-PATTERN.md
type Orchestrator struct {
	// DD-NOT-007: Dynamic channel registration (sync.Map for thread-safe parallel test execution)
	// sync.Map is optimal for our access pattern: write-once per test, read-many during deliveries
	channels sync.Map

	// Dependencies
	sanitizer     *sanitization.Sanitizer
	metrics       notificationmetrics.Recorder
	statusManager *notificationstatus.Manager

	// Logger
	logger logr.Logger
}

// DeliveryResult represents the outcome of a delivery loop.
type DeliveryResult struct {
	DeliveryResults  map[string]error
	FailureCount     int
	DeliveryAttempts []notificationv1alpha1.DeliveryAttempt // Collected attempts for batch status update
}

// NewOrchestrator creates a new delivery orchestrator.
//
// DD-NOT-007: Registration Pattern - Constructor has NO channel parameters
// Channels MUST be registered after construction using RegisterChannel()
//
// Example:
//
//	orchestrator := delivery.NewOrchestrator(sanitizer, metrics, statusManager, logger)
//	orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
//	orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackService)
func NewOrchestrator(
	sanitizer *sanitization.Sanitizer,
	metrics notificationmetrics.Recorder,
	statusManager *notificationstatus.Manager,
	logger logr.Logger,
) *Orchestrator {
	return &Orchestrator{
		// channels: sync.Map requires no initialization
		sanitizer:     sanitizer,
		metrics:       metrics,
		statusManager: statusManager,
		logger:        logger,
	}
}

// RegisterChannel registers a delivery service for a specific channel.
//
// DD-NOT-007: MANDATORY pattern for all channels (production, integration, E2E)
//
// If service is nil, registration is skipped (allows conditional registration in tests).
//
// Example:
//
//	orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
//	orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackService)
//	orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), fileService)
//	orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), logService)
func (o *Orchestrator) RegisterChannel(channel string, service Service) {
	if service == nil {
		o.logger.Info("Skipping registration of nil service", "channel", channel)
		return
	}
	o.channels.Store(channel, service)
	o.logger.Info("Registered delivery channel", "channel", channel)
}

// UnregisterChannel removes a delivery service (useful for testing).
//
// DD-NOT-007: Test support for dynamic channel management
func (o *Orchestrator) UnregisterChannel(channel string) {
	o.channels.Delete(channel)
	o.logger.Info("Unregistered delivery channel", "channel", channel)
}

// HasChannel checks if a channel is registered.
//
// DD-NOT-007: Validation support
func (o *Orchestrator) HasChannel(channel string) bool {
	_, exists := o.channels.Load(channel)
	return exists
}

// DeliverToChannels orchestrates delivery to all configured channels.
//
// This is the main entry point extracted from controller's handleDeliveryLoop().
// It handles:
// - Channel iteration
// - Idempotency checks (skip already-succeeded channels)
// - Retry limit enforcement
// - Delivery attempts
// - Result aggregation
//
// BR-NOT-055: Retry logic with permanent error classification
// BR-NOT-053: Idempotent delivery (skip already-succeeded channels)
func (o *Orchestrator) DeliverToChannels(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channels []notificationv1alpha1.Channel,
	policy *notificationv1alpha1.RetryPolicy,
	// Callback functions for controller-specific logic
	channelAlreadySucceeded func(*notificationv1alpha1.NotificationRequest, string) bool,
	hasChannelPermanentError func(*notificationv1alpha1.NotificationRequest, string) bool,
	getChannelAttemptCount func(*notificationv1alpha1.NotificationRequest, string) int,
	auditMessageSent func(context.Context, *notificationv1alpha1.NotificationRequest, string) error,
	auditMessageFailed func(context.Context, *notificationv1alpha1.NotificationRequest, string, error) error,
) (*DeliveryResult, error) {
	log := o.logger.WithValues("notification", notification.Name, "namespace", notification.Namespace)

	// Initialize result
	result := &DeliveryResult{
		DeliveryResults:  make(map[string]error),
		FailureCount:     0,
		DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{}, // Collect attempts for batch update
	}

	// Process each channel
	for _, channel := range channels {
		// Skip if channel already succeeded (idempotent delivery)
		if channelAlreadySucceeded(notification, string(channel)) {
			log.Info("Channel already delivered successfully, skipping", "channel", channel)
			// NT-BUG-004 Fix: Count already-successful channels as successes
			result.DeliveryResults[string(channel)] = nil // nil = success
			continue
		}

		// BR-NOT-055: Check if channel has permanent error (skip retries for 4xx errors)
		if hasChannelPermanentError(notification, string(channel)) {
			log.Info("Channel has permanent error, skipping retries", "channel", channel)
			result.DeliveryResults[string(channel)] = fmt.Errorf("permanent error - not retryable")
			result.FailureCount++
			continue
		}

		// Check channel attempt count using policy max attempts
		attemptCount := getChannelAttemptCount(notification, string(channel))
		if attemptCount >= policy.MaxAttempts {
			log.Info("Max retry attempts reached for channel", "channel", channel, "attempts", attemptCount, "maxAttempts", policy.MaxAttempts)
			result.DeliveryResults[string(channel)] = fmt.Errorf("max retry attempts exceeded")
			result.FailureCount++
			continue
		}

		// Attempt delivery
		deliveryErr := o.DeliverToChannel(ctx, notification, channel)

		// Create delivery attempt record (but DON'T write to status yet)
		// This prevents status updates from triggering immediate reconciles
		now := metav1.Now()
		// attemptCount already retrieved above (line 195)

		attempt := notificationv1alpha1.DeliveryAttempt{
			Channel:   string(channel),
			Attempt:   attemptCount + 1, // 1-based attempt number
			Timestamp: now,
		}

		if deliveryErr != nil {
			attempt.Status = "failed"
			attempt.Error = deliveryErr.Error()

			// BR-NOT-055: Permanent Error Classification
			isPermanent := !IsRetryableError(deliveryErr)
			if isPermanent {
				log.Error(deliveryErr, "Delivery failed with permanent error (will NOT retry)")
				attempt.Error = fmt.Sprintf("permanent failure: %s", deliveryErr.Error())
			} else {
				log.Error(deliveryErr, "Delivery failed with retryable error")
			}

			// AUDIT: Failed delivery (ADR-032 §1: MANDATORY)
			// Audit calls don't trigger reconciles, so they're safe to call immediately
			if auditErr := auditMessageFailed(ctx, notification, string(channel), deliveryErr); auditErr != nil {
				log.Error(auditErr, "CRITICAL: Failed to audit message.failed (ADR-032 §1)")
				return nil, fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
			}

			// Update metrics (DD-METRICS-001: Use injected metrics recorder)
			o.metrics.RecordDeliveryAttempt(notification.Namespace, string(channel), "failed")
			result.DeliveryResults[string(channel)] = deliveryErr
			result.FailureCount++
		} else {
			attempt.Status = "success"
			attempt.Error = ""

			log.Info("Delivery successful", "channel", channel)

			// AUDIT: Successful delivery (ADR-032 §1: MANDATORY)
			if auditErr := auditMessageSent(ctx, notification, string(channel)); auditErr != nil {
				log.Error(auditErr, "CRITICAL: Failed to audit message.sent (ADR-032 §1)")
				return nil, fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
			}

			// Update metrics (DD-METRICS-001: Use injected metrics recorder)
			o.metrics.RecordDeliveryAttempt(notification.Namespace, string(channel), "success")
			result.DeliveryResults[string(channel)] = nil // nil = success
		}

		// Add attempt to result (will be recorded in batch after loop completes)
		result.DeliveryAttempts = append(result.DeliveryAttempts, attempt)
	}

	return result, nil
}

// DeliverToChannel attempts delivery to a specific channel.
//
// DD-NOT-007: Map-based routing (NO switch statement)
// Routes to the appropriate delivery service via channel registration.
func (o *Orchestrator) DeliverToChannel(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channel notificationv1alpha1.Channel,
) error {
	// DD-NOT-007: Map lookup instead of switch statement (thread-safe)
	serviceVal, exists := o.channels.Load(string(channel))
	if !exists {
		return fmt.Errorf("channel not registered: %s (DD-NOT-007: use RegisterChannel() to register)", channel)
	}

	service, ok := serviceVal.(Service)
	if !ok {
		return fmt.Errorf("invalid service type for channel %s", channel)
	}

	// Sanitize before delivery
	sanitized := o.sanitizeNotification(notification)

	// Deliver via registered service
	return service.Deliver(ctx, sanitized)
}

// DD-NOT-007: Individual channel methods REMOVED
// All channels now use common DeliverToChannel() via registration pattern

// sanitizeNotification creates a sanitized copy of the notification.
func (o *Orchestrator) sanitizeNotification(
	notification *notificationv1alpha1.NotificationRequest,
) *notificationv1alpha1.NotificationRequest {
	if o.sanitizer == nil {
		return notification
	}

	sanitized := notification.DeepCopy()
	sanitized.Spec.Subject = o.sanitizer.Sanitize(notification.Spec.Subject)
	sanitized.Spec.Body = o.sanitizer.Sanitize(notification.Spec.Body)
	return sanitized
}

// RecordDeliveryAttempt records a delivery attempt in the notification status.
//
// Extracted from controller's recordDeliveryAttempt() method (~124 lines).
// Handles:
// - Duplicate detection (NT-BUG-002)
// - Status updates
// - Audit event emission
// - Metrics recording
// - E2E file delivery (DD-NOT-002)
//
// BR-NOT-055: Permanent error classification
// BR-NOT-051: Complete audit trail
func (o *Orchestrator) RecordDeliveryAttempt(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channel notificationv1alpha1.Channel,
	deliveryErr error,
	// Callback functions
	getChannelAttemptCount func(*notificationv1alpha1.NotificationRequest, string) int,
	auditMessageSent func(context.Context, *notificationv1alpha1.NotificationRequest, string) error,
	auditMessageFailed func(context.Context, *notificationv1alpha1.NotificationRequest, string, error) error,
) error {
	log := o.logger.WithValues("notification", notification.Name, "channel", channel)

	// NT-BUG-002 Refinement: Prevent duplicate recording during rapid reconciliations
	now := metav1.Now()
	currentStatus := "success"
	currentError := ""
	if deliveryErr != nil {
		currentStatus = "failed"
		currentError = deliveryErr.Error()
	}

	// Get current attempt count for this channel (before adding new attempt)
	currentAttemptCount := getChannelAttemptCount(notification, string(channel))

	// Find the most recent attempt for this channel
	var mostRecentAttempt *notificationv1alpha1.DeliveryAttempt
	for i := len(notification.Status.DeliveryAttempts) - 1; i >= 0; i-- {
		if notification.Status.DeliveryAttempts[i].Channel == string(channel) {
			mostRecentAttempt = &notification.Status.DeliveryAttempts[i]
			break
		}
	}

	// Only skip if it's a true duplicate (rapid reconciliation of the SAME attempt)
	if mostRecentAttempt != nil && currentAttemptCount > 0 {
		timeSinceAttempt := now.Time.Sub(mostRecentAttempt.Timestamp.Time)
		if timeSinceAttempt < 500*time.Millisecond &&
			mostRecentAttempt.Status == currentStatus &&
			mostRecentAttempt.Error == currentError {
			log.V(1).Info("Delivery attempt already recorded (exact duplicate), skipping",
				"status", currentStatus,
				"timeSince", timeSinceAttempt,
				"attemptCount", currentAttemptCount)
			return nil
		}
	}

	// Create delivery attempt record
	// BR-NOT-051: Record attempt number for audit trail
	attempt := notificationv1alpha1.DeliveryAttempt{
		Channel:   string(channel),
		Attempt:   currentAttemptCount + 1, // 1-based attempt number
		Timestamp: now,
	}

	if deliveryErr != nil {
		attempt.Status = "failed"
		attempt.Error = deliveryErr.Error()
		notification.Status.FailedDeliveries++

		// BR-NOT-055: Permanent Error Classification
		isPermanent := !IsRetryableError(deliveryErr)
		if isPermanent {
			log.Error(deliveryErr, "Delivery failed with permanent error (will NOT retry)")
			attempt.Error = fmt.Sprintf("permanent failure: %s", deliveryErr.Error())
		} else {
			log.Error(deliveryErr, "Delivery failed with retryable error")
		}

		// AUDIT: Failed delivery (ADR-032 §1: MANDATORY)
		if auditErr := auditMessageFailed(ctx, notification, string(channel), deliveryErr); auditErr != nil {
			log.Error(auditErr, "CRITICAL: Failed to audit message.failed (ADR-032 §1)")
			return fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
		}

		// Metrics: Record failure
		o.metrics.RecordDeliveryAttempt(notification.Namespace, string(channel), "failure")
	} else {
		attempt.Status = "success"
		notification.Status.SuccessfulDeliveries++
		log.Info("Delivery successful")

		// AUDIT: Successful delivery (ADR-032 §1: MANDATORY)
		if auditErr := auditMessageSent(ctx, notification, string(channel)); auditErr != nil {
			log.Error(auditErr, "CRITICAL: Failed to audit message.sent (ADR-032 §1)")
			return fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
		}

		// Metrics: Record success
		o.metrics.RecordDeliveryAttempt(notification.Namespace, string(channel), "success")
		o.metrics.RecordDeliveryDuration(notification.Namespace, string(channel), time.Since(notification.CreationTimestamp.Time).Seconds())

		// E2E FILE DELIVERY (DD-NOT-002 V3.0) - Non-blocking
		// DD-NOT-007: Use registered file service if available (sync.Map for thread-safety)
		if fileServiceVal, exists := o.channels.Load(string(notificationv1alpha1.ChannelFile)); exists {
			if fileService, ok := fileServiceVal.(Service); ok {
				sanitizedNotification := o.sanitizeNotification(notification)
				if fileErr := fileService.Deliver(ctx, sanitizedNotification); fileErr != nil {
					log.Error(fileErr, "FileService delivery failed (E2E only, non-blocking)")
				}
			}
		}
	}

	// BR-NOT-053: Use Status Manager to record delivery attempt (Pattern 2)
	if err := o.statusManager.RecordDeliveryAttempt(ctx, notification, attempt); err != nil {
		log.Info("Failed to update status after channel delivery (non-fatal, will retry at end)", "error", err)
	}

	return nil
}
