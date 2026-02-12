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

// ========================================
// RETRY & CIRCUIT BREAKER HANDLER (Pattern 4: Controller Decomposition)
// 📋 Pattern: Pattern 4 - Controller Decomposition
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §5
// ========================================
//
// This file contains retry logic and circuit breaker helpers extracted from the main controller
// to improve maintainability and testability per Pattern 4.
//
// BENEFITS:
// - ~150 lines extracted from main controller
// - Retry/circuit breaker logic isolated
// - Clear separation of concerns
// - Easy to test retry policies independently
//
// RESPONSIBILITIES:
// - Retry policy management (BR-NOT-052)
// - Backoff calculation with jitter (BR-NOT-055)
// - Circuit breaker state checking (BR-NOT-055)
// - Channel delivery attempt tracking
//
// BR REFERENCES:
// - BR-NOT-052: Automatic Retry with custom retry policies
// - BR-NOT-055: Graceful Degradation (circuit breaker, anti-thundering herd)
// ========================================

package notification

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// PermanentFailureMarker is the string stored in delivery attempt errors
// to indicate a permanent (non-retryable) failure. Used by circuit breaker
// logic to skip retry for channels with permanent errors (4xx, auth failures).
const PermanentFailureMarker = "permanent failure"

// ========================================
// CHANNEL DELIVERY STATUS HELPERS
// ========================================

// channelAlreadySucceeded checks if channel delivery has already succeeded for this notification.
// Used to prevent duplicate successful deliveries and optimize retry logic.
// DD-NOT-008: Checks both persisted status and in-memory tracking to prevent duplicate deliveries
func (r *NotificationRequestReconciler) channelAlreadySucceeded(notification *notificationv1alpha1.NotificationRequest, channel string) bool {
	// Check persisted success in status
	persistedSuccess := false
	for _, attempt := range notification.Status.DeliveryAttempts {
		if attempt.Channel == channel && attempt.Status == "success" {
			persistedSuccess = true
			break
		}
	}
	
	// DD-NOT-008: Check both persisted and in-memory success
	// This prevents duplicate deliveries when status hasn't been persisted yet
	return r.DeliveryOrchestrator.HasChannelSucceeded(notification, channel, persistedSuccess)
}

// getChannelAttemptCount returns the number of delivery attempts for a specific channel.
// BR-NOT-052: Automatic Retry - tracks per-channel attempts for retry limit enforcement
// DD-NOT-008: Includes in-flight attempts to prevent "6 attempts instead of 5" bug
func (r *NotificationRequestReconciler) getChannelAttemptCount(notification *notificationv1alpha1.NotificationRequest, channel string) int {
	// Count persisted attempts from status
	persistedCount := 0
	for _, attempt := range notification.Status.DeliveryAttempts {
		if attempt.Channel == channel {
			persistedCount++
		}
	}
	
	// DD-NOT-008: Get total count (persisted + in-flight) from orchestrator
	// This prevents concurrent reconciliations from both thinking attemptCount < MaxAttempts
	return r.DeliveryOrchestrator.GetTotalAttemptCount(notification, channel, persistedCount)
}

// hasChannelPermanentError checks if channel has a permanent error that should not be retried.
// Permanent errors include: 4xx HTTP errors, authentication failures, invalid configurations.
// BR-NOT-052: Automatic Retry - distinguishes permanent from transient failures
func (r *NotificationRequestReconciler) hasChannelPermanentError(notification *notificationv1alpha1.NotificationRequest, channel string) bool {
	for _, attempt := range notification.Status.DeliveryAttempts {
		if attempt.Channel == channel && attempt.Status == "failed" {
			// Check if error message indicates permanent failure
			if strings.Contains(attempt.Error, PermanentFailureMarker) {
				return true
			}
		}
	}
	return false
}

// getMaxAttemptCount returns the maximum number of attempts across all channels.
// Used for global retry limit enforcement and notification-level backoff calculation.
func (r *NotificationRequestReconciler) getMaxAttemptCount(notification *notificationv1alpha1.NotificationRequest) int { //nolint:unused
	maxAttempt := 0
	attemptCounts := make(map[string]int)

	for _, attempt := range notification.Status.DeliveryAttempts {
		attemptCounts[attempt.Channel]++
		if attemptCounts[attempt.Channel] > maxAttempt {
			maxAttempt = attemptCounts[attempt.Channel]
		}
	}

	return maxAttempt
}

// ========================================
// RETRY POLICY MANAGEMENT
// ========================================

// getRetryPolicy returns the retry policy from the notification spec, or default if not specified
// BR-NOT-052: Automatic Retry with custom retry policies
func (r *NotificationRequestReconciler) getRetryPolicy(notification *notificationv1alpha1.NotificationRequest) *notificationv1alpha1.RetryPolicy {
	if notification.Spec.RetryPolicy != nil {
		return notification.Spec.RetryPolicy
	}

	// Return default policy
	return &notificationv1alpha1.RetryPolicy{
		MaxAttempts:           5,
		InitialBackoffSeconds: 30,
		BackoffMultiplier:     2,
		MaxBackoffSeconds:     480,
	}
}

// calculateBackoffWithPolicy calculates exponential backoff duration using the notification's retry policy
// v3.1 Enhancement (Category B): Added jitter (±10%) to prevent thundering herd
// BR-NOT-052: Automatic Retry with exponential backoff
//
// Implementation: Uses shared backoff utility (pkg/shared/backoff) - DD-SHARED-001
// Extracted from NT's production-proven implementation (v3.1)
func (r *NotificationRequestReconciler) calculateBackoffWithPolicy(notification *notificationv1alpha1.NotificationRequest, attemptCount int) time.Duration {
	policy := r.getRetryPolicy(notification)

	// Use shared backoff utility (extracted from NT's implementation)
	// Configuration maps directly from RetryPolicy to backoff.Config
	config := backoff.Config{
		BasePeriod:    time.Duration(policy.InitialBackoffSeconds) * time.Second,
		MaxPeriod:     time.Duration(policy.MaxBackoffSeconds) * time.Second,
		Multiplier:    float64(policy.BackoffMultiplier),
		JitterPercent: 10, // v3.1: Anti-thundering herd (BR-NOT-055)
	}

	return config.Calculate(int32(attemptCount))
}

// ========================================
// CIRCUIT BREAKER HELPERS
// ========================================

// isSlackCircuitBreakerOpen checks if the Slack circuit breaker is open
// v3.1 Enhancement (Category B): Circuit breaker for graceful degradation
// BR-NOT-055: Graceful Degradation (prevent cascading failures)
func (r *NotificationRequestReconciler) isSlackCircuitBreakerOpen() bool {
	if r.CircuitBreaker == nil {
		return false // No circuit breaker configured, allow all requests
	}
	return !r.CircuitBreaker.AllowRequest("slack")
}

// checkBeforeDelivery is called by the orchestrator before each channel delivery.
// DD-EVENT-001 v1.1: For Slack, checks circuit breaker; if open, emits CircuitBreakerOpen and returns error.
// Returns nil if delivery should proceed.
func (r *NotificationRequestReconciler) checkBeforeDelivery(notification *notificationv1alpha1.NotificationRequest, channel string) error {
	if channel != "slack" {
		return nil
	}
	if !r.isSlackCircuitBreakerOpen() {
		return nil
	}
	err := fmt.Errorf("slack circuit breaker is open (too many failures, preventing cascading failures)")
	if r.Recorder != nil {
		r.Recorder.Event(notification, corev1.EventTypeWarning, events.EventReasonCircuitBreakerOpen,
			fmt.Sprintf("Slack channel circuit breaker is open: %s", err.Error()))
	}
	return err
}
