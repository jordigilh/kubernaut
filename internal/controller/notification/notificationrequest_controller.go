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
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/retry"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	"github.com/jordigilh/kubernaut/pkg/notification/sanitization"
)

// NotificationRequestReconciler reconciles a NotificationRequest object
type NotificationRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Delivery services
	ConsoleService *delivery.ConsoleDeliveryService
	SlackService   *delivery.SlackDeliveryService
	FileService    *delivery.FileDeliveryService // E2E testing only (DD-NOT-002)

	// Data sanitization
	Sanitizer *sanitization.Sanitizer

	// v3.1: Circuit breaker for graceful degradation (Category B)
	CircuitBreaker *retry.CircuitBreaker

	// v1.1: Audit integration for unified audit table (ADR-034)
	// BR-NOT-062: Unified Audit Table Integration
	// BR-NOT-063: Graceful Audit Degradation
	// See: DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md
	AuditStore   audit.AuditStore // Buffered store for async audit writes (fire-and-forget)
	AuditHelpers *AuditHelpers    // Helper functions for creating audit events

	// BR-NOT-065: Channel Routing Based on Labels
	// BR-NOT-067: Routing Configuration Hot-Reload
	// Thread-safe router with hot-reload support from ConfigMap
	// See: DD-WE-004 (skip-reason routing)
	Router *routing.Router

	// Legacy: Direct config (for backwards compatibility, use Router instead)
	routingConfig *routing.Config
	routingMu     sync.RWMutex
}

//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
//
// BR-NOT-050: Data Loss Prevention (CRD persistence)
// BR-NOT-051: Complete Audit Trail (delivery attempts)
// BR-NOT-052: Automatic Retry (exponential backoff)
// BR-NOT-053: At-Least-Once Delivery (reconciliation loop)
// BR-NOT-056: CRD Lifecycle Management (phase state machine)
func (r *NotificationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch NotificationRequest CRD
	notification := &notificationv1alpha1.NotificationRequest{}
	err := r.Get(ctx, req.NamespacedName, notification)
	if err != nil {
		if errors.IsNotFound(err) {
			// Category A: NotificationRequest Not Found (normal cleanup)
			return r.handleNotFound(ctx, req.String())
		}
		log.Error(err, "Failed to fetch NotificationRequest")
		return ctrl.Result{}, err
	}

	// Initialize status if this is the first reconciliation
	if notification.Status.Phase == "" {
		notification.Status.Phase = notificationv1alpha1.NotificationPhasePending
		notification.Status.Reason = "Initialized"
		notification.Status.Message = "Notification request received"
		// DD-NOT-005: observedGeneration removed (spec is immutable, no change detection needed)
		notification.Status.DeliveryAttempts = []notificationv1alpha1.DeliveryAttempt{}
		notification.Status.TotalAttempts = 0
		notification.Status.SuccessfulDeliveries = 0
		notification.Status.FailedDeliveries = 0

		if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
			log.Error(err, "Failed to initialize status")
			return ctrl.Result{}, err
		}

		// Record metric for notification request creation (BR-NOT-054: Observability)
		UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhasePending), 1)

		log.Info("NotificationRequest status initialized", "name", notification.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// BR-NOT-053: Idempotent delivery - skip if already in terminal state
	// CRITICAL: Check phase BEFORE CompletionTime to prevent duplicate deliveries in parallel execution
	if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent {
		// Sent is a true terminal state - never retry successful deliveries
		log.Info("NotificationRequest already sent, skipping duplicate delivery", "phase", notification.Status.Phase)
		return ctrl.Result{}, nil
	}

	if notification.Status.Phase == notificationv1alpha1.NotificationPhaseFailed {
		// Failed state only terminal if CompletionTime set (max retries exhausted)
		if notification.Status.CompletionTime != nil {
			log.Info("NotificationRequest permanently failed (max retries exhausted), skipping", "phase", notification.Status.Phase)
			return ctrl.Result{}, nil
		}
		// Otherwise, allow retry
	}

	// Update phase to Sending
	if notification.Status.Phase == notificationv1alpha1.NotificationPhasePending {
		notification.Status.Phase = notificationv1alpha1.NotificationPhaseSending
		notification.Status.Reason = "ProcessingDeliveries"
		notification.Status.Message = "Processing delivery channels"

		if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
			log.Error(err, "Failed to update phase to Sending")
			return ctrl.Result{}, err
		}

		// Record metric for phase transition to Sending (BR-NOT-054: Observability)
		UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhaseSending), 1)
	}

	// BR-NOT-053: ALWAYS re-read before delivery to check if another reconcile completed
	// CRITICAL: Prevents duplicate delivery - must be OUTSIDE the Pending check
	// This catches both: concurrent reconciles AND queued reconciles after first one completes
	if err := r.Get(ctx, req.NamespacedName, notification); err != nil {
		log.Error(err, "Failed to refresh notification before delivery")
		return ctrl.Result{}, err
	}

	// Check if another reconcile completed while we were updating phase
	if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent {
		log.Info("NotificationRequest completed by concurrent reconcile, skipping duplicate delivery")
		return ctrl.Result{}, nil
	}

	// BR-NOT-053: CRITICAL - Re-read notification RIGHT BEFORE delivery loop
	// This prevents duplicate deliveries in parallel execution by ensuring we have
	// the absolute latest delivery attempts from any concurrent reconcile
	if err := r.Get(ctx, req.NamespacedName, notification); err != nil {
		log.Error(err, "Failed to refresh notification before channel delivery loop")
		return ctrl.Result{}, err
	}

	// Double-check phase after re-read (another reconcile might have just completed)
	if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent {
		log.Info("NotificationRequest just completed, skipping duplicate delivery after re-read")
		return ctrl.Result{}, nil
	}

	// Process deliveries for each channel
	deliveryResults := make(map[string]error)
	failureCount := 0

	// Get retry policy to check max attempts
	policy := r.getRetryPolicy(notification)

	// BR-NOT-065: Resolve channels from routing rules if spec.channels is empty
	channels := notification.Spec.Channels
	if len(channels) == 0 {
		channels = r.resolveChannelsFromRouting(ctx, notification)
		log.Info("Resolved channels from routing rules",
			"notification", notification.Name,
			"channels", channels,
			"labels", notification.Labels)
	}

	for _, channel := range channels {
		// Skip if channel already succeeded (idempotent delivery)
		if r.channelAlreadySucceeded(notification, string(channel)) {
			log.Info("Channel already delivered successfully, skipping", "channel", channel)
			continue
		}

		// BR-NOT-055: Check if channel has permanent error (skip retries for 4xx errors)
		if r.hasChannelPermanentError(notification, string(channel)) {
			log.Info("Channel has permanent error, skipping retries", "channel", channel)
			deliveryResults[string(channel)] = fmt.Errorf("permanent error - not retryable")
			failureCount++
			continue
		}

		// Check channel attempt count using policy max attempts
		attemptCount := r.getChannelAttemptCount(notification, string(channel))
		if attemptCount >= policy.MaxAttempts {
			log.Info("Max retry attempts reached for channel", "channel", channel, "attempts", attemptCount, "maxAttempts", policy.MaxAttempts)
			deliveryResults[string(channel)] = fmt.Errorf("max retry attempts exceeded")
			failureCount++
			continue
		}

		// Attempt delivery
		var deliveryErr error
		switch channel {
		case notificationv1alpha1.ChannelConsole:
			deliveryErr = r.deliverToConsole(ctx, notification)
		case notificationv1alpha1.ChannelSlack:
			deliveryErr = r.deliverToSlack(ctx, notification)
		default:
			deliveryErr = fmt.Errorf("unsupported channel: %s", channel)
		}

		// Record delivery attempt in status
		attempt := notificationv1alpha1.DeliveryAttempt{
			Channel:   string(channel),
			Timestamp: metav1.Now(),
		}

		if deliveryErr != nil {
			attempt.Status = "failed"
			attempt.Error = deliveryErr.Error()
			notification.Status.FailedDeliveries++
			deliveryResults[string(channel)] = deliveryErr
			failureCount++

			// BR-NOT-055: Permanent Error Classification
			// Check if error is retryable or permanent (4xx vs 5xx)
			isPermanent := !delivery.IsRetryableError(deliveryErr)
			if isPermanent {
				// Permanent error (4xx) - mark channel as permanently failed
				// Set attemptCount to max to prevent further retries
				log.Error(deliveryErr, "Delivery failed with permanent error (will NOT retry)", "channel", channel)
				attempt.Error = fmt.Sprintf("permanent failure: %s", deliveryErr.Error())
			} else {
				// Retryable error (5xx, network, etc.) - will retry with backoff
				log.Error(deliveryErr, "Delivery failed with retryable error", "channel", channel)
			}

			// AUDIT INTEGRATION POINT 1: Audit failed delivery (BR-NOT-062)
			r.auditMessageFailed(ctx, notification, string(channel), deliveryErr)

			// Record delivery failure metric (BR-NOT-054: Observability)
			RecordDeliveryAttempt(notification.Namespace, string(channel), "failure")
		} else {
			attempt.Status = "success"
			notification.Status.SuccessfulDeliveries++
			log.Info("Delivery successful", "channel", channel)

			// AUDIT INTEGRATION POINT 2: Audit successful delivery (BR-NOT-062)
			r.auditMessageSent(ctx, notification, string(channel))

			// Record delivery success metrics (BR-NOT-054: Observability)
			RecordDeliveryAttempt(notification.Namespace, string(channel), "success")
			RecordDeliveryDuration(notification.Namespace, string(channel), time.Since(notification.CreationTimestamp.Time).Seconds())

			// E2E FILE DELIVERY INTEGRATION (DD-NOT-002 V3.0)
			// FileService is E2E testing infrastructure only (non-blocking)
			// - Called AFTER successful production delivery
			// - Uses sanitized notification (matches production delivery)
			// - Errors are logged but NOT propagated (fire-and-forget)
			// - nil-safe: production deployments have FileService = nil
			if r.FileService != nil {
				// Sanitize notification before file delivery (matches production behavior)
				sanitizedNotification := r.sanitizeNotification(notification)
				if fileErr := r.FileService.Deliver(ctx, sanitizedNotification); fileErr != nil {
					log.Error(fileErr, "FileService delivery failed (E2E only, non-blocking)",
						"notification", notification.Name,
						"namespace", notification.Namespace,
						"channel", channel)
					// DO NOT propagate error - production delivery succeeded
				} else {
					log.V(1).Info("FileService delivery succeeded (E2E validation)",
						"notification", notification.Name,
						"namespace", notification.Namespace)
				}
			}
		}

		notification.Status.DeliveryAttempts = append(notification.Status.DeliveryAttempts, attempt)
		notification.Status.TotalAttempts++

		// BR-NOT-053: CRITICAL - Update status IMMEDIATELY after each channel delivery
		// This makes the attempt visible to concurrent reconciles BEFORE they try to deliver
		// to the same channel, preventing duplicate deliveries
		if err := r.Status().Update(ctx, notification); err != nil {
			log.Info("Failed to update status after channel delivery (non-fatal, will retry at end)", "channel", channel, "error", err)
			// Continue - worst case is we'll update at the end of the loop
		}
	}

	// Update final status based on delivery results
	// Check overall status, not just this reconciliation loop
	totalChannels := len(notification.Spec.Channels)
	totalSuccessful := notification.Status.SuccessfulDeliveries

	if totalSuccessful == totalChannels {
		// All channels delivered successfully
		notification.Status.Phase = notificationv1alpha1.NotificationPhaseSent
		now := metav1.Now()
		notification.Status.CompletionTime = &now
		notification.Status.Reason = "AllDeliveriesSucceeded"
		notification.Status.Message = fmt.Sprintf("Successfully delivered to %d channel(s)", notification.Status.SuccessfulDeliveries)

		if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
			log.Error(err, "Failed to update status to Sent")
			return ctrl.Result{}, err
		}

		// Record metric for successful completion (BR-NOT-054: Observability)
		UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhaseSent), 1)

		log.Info("All deliveries successful", "name", notification.Name)
		return ctrl.Result{}, nil // No requeue - done

	} else if totalSuccessful > 0 {
		// Partial success - some channels succeeded, some failed
		maxAttempt := r.getMaxAttemptCount(notification)

		// Check if max retries reached for failed channels
		if maxAttempt >= policy.MaxAttempts {
			// Max retries reached - terminal state, but keep PartiallySent since some succeeded
			notification.Status.Phase = notificationv1alpha1.NotificationPhasePartiallySent
			now := metav1.Now()
			notification.Status.CompletionTime = &now
			notification.Status.Reason = "PartialDeliveryFailure"
			notification.Status.Message = fmt.Sprintf("%d of %d deliveries succeeded, remaining failed after %d retries",
				notification.Status.SuccessfulDeliveries, len(notification.Spec.Channels), policy.MaxAttempts)

			if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
				log.Error(err, "Failed to update status to PartiallySent")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil // No requeue - terminal state
		}

		// Not yet at max retries - update status and requeue
		notification.Status.Phase = notificationv1alpha1.NotificationPhasePartiallySent
		notification.Status.Reason = "PartialDeliveryFailure"
		notification.Status.Message = fmt.Sprintf("%d of %d deliveries succeeded (attempt %d/%d)",
			notification.Status.SuccessfulDeliveries, len(notification.Spec.Channels), maxAttempt, policy.MaxAttempts)

		if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
			log.Error(err, "Failed to update status to PartiallySent")
			return ctrl.Result{}, err
		}

		// Requeue with exponential backoff using custom policy
		backoff := r.calculateBackoffWithPolicy(notification, maxAttempt)

		// v3.1: Record backoff duration metrics for Slack retries (Category B)
		for channel := range deliveryResults {
			if channel == string(notificationv1alpha1.ChannelSlack) {
				RecordSlackBackoff(notification.Namespace, backoff.Seconds())
				RecordSlackRetry(notification.Namespace, "rate_limiting")
			}
		}

		log.Info("Requeuing for retry", "after", backoff, "attempt", maxAttempt+1)
		return ctrl.Result{RequeueAfter: backoff}, nil

	} else {
		// All deliveries failed
		maxAttempt := r.getMaxAttemptCount(notification)

		// Check if max retries reached
		if maxAttempt >= policy.MaxAttempts {
			// Max retries reached - terminal state
			notification.Status.Phase = notificationv1alpha1.NotificationPhaseFailed
			notification.Status.Reason = "MaxRetriesExceeded"
			notification.Status.Message = fmt.Sprintf("Maximum retry attempts (%d) exceeded, all deliveries failed", policy.MaxAttempts)
			now := metav1.Now()
			notification.Status.CompletionTime = &now

			if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
				log.Error(err, "Failed to update status to Failed")
				return ctrl.Result{}, err
			}

			// Record metric for failed completion (BR-NOT-054: Observability)
			// DD-005: Using compliant metric names
			UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhaseFailed), 1)
			RecordDeliveryRetries(notification.Namespace, float64(maxAttempt))

			return ctrl.Result{}, nil // No requeue - terminal state
		}

		// Not yet at max retries - mark as failed but will retry
		notification.Status.Phase = notificationv1alpha1.NotificationPhaseFailed
		notification.Status.Reason = "AllDeliveriesFailed"
		notification.Status.Message = fmt.Sprintf("All %d deliveries failed (attempt %d/%d)", len(notification.Spec.Channels), maxAttempt, policy.MaxAttempts)

		// Update status and requeue with exponential backoff
		if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}

		backoff := r.calculateBackoffWithPolicy(notification, maxAttempt)

		// v3.1: Record backoff duration metrics for Slack retries (Category B)
		for _, channel := range notification.Spec.Channels {
			if channel == notificationv1alpha1.ChannelSlack {
				RecordSlackBackoff(notification.Namespace, backoff.Seconds())
				RecordSlackRetry(notification.Namespace, "delivery_failure")
			}
		}

		log.Info("All deliveries failed, requeuing", "after", backoff, "attempt", maxAttempt+1)
		return ctrl.Result{RequeueAfter: backoff}, nil
	}
}

// deliverToConsole delivers notification to console (stdout)
func (r *NotificationRequestReconciler) deliverToConsole(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	if r.ConsoleService == nil {
		return fmt.Errorf("console service not initialized")
	}

	// Sanitize notification content before delivery
	sanitizedNotification := r.sanitizeNotification(notification)
	return r.ConsoleService.Deliver(ctx, sanitizedNotification)
}

// deliverToSlack delivers notification to Slack webhook
func (r *NotificationRequestReconciler) deliverToSlack(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	if r.SlackService == nil {
		return fmt.Errorf("slack service not initialized")
	}

	// v3.1: Check circuit breaker (Category B - fail fast if Slack API is unhealthy)
	if r.isSlackCircuitBreakerOpen() {
		return fmt.Errorf("slack circuit breaker is open (too many failures, preventing cascading failures)")
	}

	// Sanitize notification content before delivery
	sanitizedNotification := r.sanitizeNotification(notification)
	err := r.SlackService.Deliver(ctx, sanitizedNotification)

	// v3.1: Record circuit breaker state (Category B)
	if r.CircuitBreaker != nil {
		if err != nil {
			r.CircuitBreaker.RecordFailure("slack")
		} else {
			r.CircuitBreaker.RecordSuccess("slack")
		}
	}

	return err
}

// sanitizeNotification creates a sanitized copy of the notification
func (r *NotificationRequestReconciler) sanitizeNotification(notification *notificationv1alpha1.NotificationRequest) *notificationv1alpha1.NotificationRequest {
	// Create a shallow copy to avoid mutating the original
	sanitized := notification.DeepCopy()

	// Sanitize subject and body if sanitizer is configured
	if r.Sanitizer != nil {
		sanitized.Spec.Subject = r.Sanitizer.Sanitize(sanitized.Spec.Subject)
		sanitized.Spec.Body = r.Sanitizer.Sanitize(sanitized.Spec.Body)
	}

	return sanitized
}

// channelAlreadySucceeded checks if a channel has already succeeded
// BR-NOT-053: Idempotent delivery - skip channels that already have successful delivery attempts
func (r *NotificationRequestReconciler) channelAlreadySucceeded(notification *notificationv1alpha1.NotificationRequest, channel string) bool {
	for _, attempt := range notification.Status.DeliveryAttempts {
		if attempt.Channel == channel && attempt.Status == "success" {
			return true
		}
	}
	return false
}

// getChannelAttemptCount returns the number of attempts for a specific channel
func (r *NotificationRequestReconciler) getChannelAttemptCount(notification *notificationv1alpha1.NotificationRequest, channel string) int {
	count := 0
	for _, attempt := range notification.Status.DeliveryAttempts {
		if attempt.Channel == channel {
			count++
		}
	}
	return count
}

// hasChannelPermanentError checks if a channel has a permanent error (4xx) that should not be retried
// BR-NOT-055: Permanent Error Classification
func (r *NotificationRequestReconciler) hasChannelPermanentError(notification *notificationv1alpha1.NotificationRequest, channel string) bool {
	for _, attempt := range notification.Status.DeliveryAttempts {
		if attempt.Channel == channel && attempt.Status == "failed" {
			// Check if error message indicates permanent failure
			if strings.Contains(attempt.Error, "permanent failure") {
				return true
			}
		}
	}
	return false
}

// getMaxAttemptCount returns the maximum attempt count across all channels
func (r *NotificationRequestReconciler) getMaxAttemptCount(notification *notificationv1alpha1.NotificationRequest) int {
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
func (r *NotificationRequestReconciler) calculateBackoffWithPolicy(notification *notificationv1alpha1.NotificationRequest, attemptCount int) time.Duration {
	policy := r.getRetryPolicy(notification)

	baseBackoff := time.Duration(policy.InitialBackoffSeconds) * time.Second
	maxBackoff := time.Duration(policy.MaxBackoffSeconds) * time.Second
	multiplier := policy.BackoffMultiplier

	// Calculate exponential backoff: baseBackoff * (multiplier ^ attemptCount)
	backoff := baseBackoff
	for i := 0; i < attemptCount; i++ {
		backoff = backoff * time.Duration(multiplier)
		// Cap at maxBackoff to prevent overflow
		if backoff > maxBackoff {
			backoff = maxBackoff
			break
		}
	}

	// Final cap at maxBackoff
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	// v3.1: Add jitter (±10%) to prevent thundering herd problem
	// This distributes retry attempts over time, reducing Slack API load spikes
	jitterRange := backoff / 10 // 10% of backoff
	if jitterRange > 0 {
		// Random jitter between -10% and +10%
		jitter := time.Duration(rand.Int63n(int64(jitterRange)*2)) - jitterRange
		backoff += jitter

		// Ensure backoff remains positive and doesn't exceed max
		if backoff < baseBackoff {
			backoff = baseBackoff
		}
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return backoff
}

// isSlackCircuitBreakerOpen checks if the Slack circuit breaker is open
// v3.1 Enhancement (Category B): Circuit breaker for graceful degradation
// BR-NOT-055: Graceful Degradation (prevent cascading failures)
func (r *NotificationRequestReconciler) isSlackCircuitBreakerOpen() bool {
	if r.CircuitBreaker == nil {
		return false // No circuit breaker configured, allow all requests
	}
	return !r.CircuitBreaker.AllowRequest("slack")
}

// CalculateBackoff calculates exponential backoff duration (legacy function for backward compatibility)
// New code should use calculateBackoffWithPolicy instead
// BR-NOT-052: Automatic Retry with exponential backoff
func CalculateBackoff(attemptCount int) time.Duration {
	baseBackoff := 30 * time.Second
	maxBackoff := 480 * time.Second

	// Calculate 2^attemptCount * baseBackoff
	backoff := baseBackoff * (1 << attemptCount)

	// Cap at maxBackoff
	if backoff > maxBackoff {
		return maxBackoff
	}

	return backoff
}

// ==============================================
// v3.1 Enhancement: Error Handling Categories
// ==============================================

// handleNotFound handles Category A: NotificationRequest Not Found
// When: CRD deleted during reconciliation
// Action: Log deletion, remove from retry queue
// Recovery: Normal (no action needed)
func (r *NotificationRequestReconciler) handleNotFound(ctx context.Context, name string) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("NotificationRequest not found, likely deleted", "name", name)
	// Remove from retry queue if applicable (controller-runtime handles this automatically)
	return ctrl.Result{}, nil
}

// updateStatusWithRetry updates the notification status with retry logic for conflicts
// Category D: Status Update Conflicts
// When: Multiple reconcile attempts updating status simultaneously
// Action: Retry with optimistic locking
// Recovery: Automatic (retry status update)
func (r *NotificationRequestReconciler) updateStatusWithRetry(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, maxRetries int) error {
	log := log.FromContext(ctx)

	for i := 0; i < maxRetries; i++ {
		err := r.Status().Update(ctx, notification)
		if err == nil {
			return nil
		}

		if !errors.IsConflict(err) {
			// Not a conflict error, return immediately
			return err
		}

		// Conflict error - refetch and retry
		log.Info("Status update conflict, retrying", "attempt", i+1, "maxRetries", maxRetries)

		// Refetch the latest version
		latest := &notificationv1alpha1.NotificationRequest{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(notification), latest); err != nil {
			return fmt.Errorf("failed to refetch notification after conflict: %w", err)
		}

		// Copy our status changes to the latest version
		latest.Status = notification.Status
		*notification = *latest
	}

	return fmt.Errorf("failed to update status after %d retries", maxRetries)
}

// ========================================
// AUDIT INTEGRATION HELPERS (v1.1)
// BR-NOT-062: Unified Audit Table Integration
// BR-NOT-063: Graceful Audit Degradation (fire-and-forget, non-blocking)
// See: DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md
// ========================================

// auditMessageSent audits successful message delivery (non-blocking)
//
// BR-NOT-062: Unified audit table integration
// BR-NOT-063: Graceful audit degradation - failures don't block reconciliation
//
// This method is fire-and-forget: audit write failures are logged but don't affect
// notification delivery success.
func (r *NotificationRequestReconciler) auditMessageSent(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string) {
	// Skip if audit store not initialized
	if r.AuditStore == nil || r.AuditHelpers == nil {
		return
	}

	log := log.FromContext(ctx)

	// Create audit event
	event, err := r.AuditHelpers.CreateMessageSentEvent(notification, channel)
	if err != nil {
		log.Error(err, "Failed to create audit event", "event_type", "message.sent", "channel", channel)
		return
	}

	// Fire-and-forget: Audit write failures don't block reconciliation (BR-NOT-063)
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.sent", "channel", channel)
		// Continue reconciliation - audit failure is not critical to notification delivery
	}
}

// auditMessageFailed audits failed message delivery (non-blocking)
//
// BR-NOT-062: Unified audit table integration
// BR-NOT-063: Graceful audit degradation
func (r *NotificationRequestReconciler) auditMessageFailed(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string, deliveryErr error) {
	// Skip if audit store not initialized
	if r.AuditStore == nil || r.AuditHelpers == nil {
		return
	}

	log := log.FromContext(ctx)

	// Create audit event with error details
	event, err := r.AuditHelpers.CreateMessageFailedEvent(notification, channel, deliveryErr)
	if err != nil {
		log.Error(err, "Failed to create audit event", "event_type", "message.failed", "channel", channel)
		return
	}

	// Fire-and-forget
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.failed", "channel", channel)
	}
}

// auditMessageAcknowledged audits notification acknowledgment (non-blocking)
//
// V2.0 ROADMAP FEATURE: Operator acknowledgment tracking
//
// Planned for Notification Service v2.0 (not v1.x scope):
//   - Interactive Slack messages with [Acknowledge] button
//   - Webhook endpoint to receive acknowledgment events
//   - CRD fields: Status.AcknowledgedAt, Status.AcknowledgedBy
//   - Response time SLA tracking (time to acknowledge)
//   - Compliance audit trail (who acknowledged what, when)
//
// Current Implementation Status (v1.x):
//
//	✅ Audit method implemented and tested (110 unit tests)
//	✅ Ready for integration when v2.0 CRD schema is added
//	⏸️ NOT integrated (no CRD fields, no webhook endpoint)
//
// Integration Point (v2.0):
//
//	if notification.Status.AcknowledgedAt != nil && !notification.Status.AuditedAcknowledgment {
//	    r.auditMessageAcknowledged(ctx, notification)
//	}
//
// Business Requirement: v2.0 roadmap (operator accountability)
// Tests: test/unit/notification/audit_test.go (25+ test cases)
// BR-NOT-062: Unified audit table integration ✅
// BR-NOT-063: Graceful audit degradation ✅
//
//nolint:unused // v2.0 roadmap feature - prepared ahead of CRD schema changes
func (r *NotificationRequestReconciler) auditMessageAcknowledged(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) {
	// Skip if audit store not initialized
	if r.AuditStore == nil || r.AuditHelpers == nil {
		return
	}

	log := log.FromContext(ctx)

	// Create audit event
	event, err := r.AuditHelpers.CreateMessageAcknowledgedEvent(notification)
	if err != nil {
		log.Error(err, "Failed to create audit event", "event_type", "message.acknowledged")
		return
	}

	// Fire-and-forget
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.acknowledged")
	}
}

// auditMessageEscalated audits notification escalation (non-blocking)
//
// V2.0 ROADMAP FEATURE: Automatic notification escalation
//
// Planned for Notification Service v2.0 (not v1.x scope):
//   - Auto-escalation policy (escalate if unacknowledged after N minutes)
//   - RemediationOrchestrator watches for unacknowledged notifications
//   - CRD fields: Status.EscalatedAt, Status.EscalatedTo, Status.EscalationReason
//   - Escalation metrics (frequency by team, escalation patterns)
//   - SLA compliance tracking (time to acknowledge vs. escalation threshold)
//
// Current Implementation Status (v1.x):
//
//	✅ Audit method implemented and tested (110 unit tests)
//	✅ Ready for integration when v2.0 CRD schema is added
//	⏸️ NOT integrated (no CRD fields, no escalation policy)
//
// Integration Point (v2.0):
//
//	if notification.Status.EscalatedAt != nil && !notification.Status.AuditedEscalation {
//	    r.auditMessageEscalated(ctx, notification)
//	}
//
// Business Requirement: v2.0 roadmap (auto-escalation for unacknowledged alerts)
// Tests: test/unit/notification/audit_test.go (25+ test cases)
// BR-NOT-062: Unified audit table integration ✅
// BR-NOT-063: Graceful audit degradation ✅
//
//nolint:unused // v2.0 roadmap feature - prepared ahead of CRD schema changes
func (r *NotificationRequestReconciler) auditMessageEscalated(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) {
	// Skip if audit store not initialized
	if r.AuditStore == nil || r.AuditHelpers == nil {
		return
	}

	log := log.FromContext(ctx)

	// Create audit event
	event, err := r.AuditHelpers.CreateMessageEscalatedEvent(notification)
	if err != nil {
		log.Error(err, "Failed to create audit event", "event_type", "message.escalated")
		return
	}

	// Fire-and-forget
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.escalated")
	}
}

// =============================================================================
// BR-NOT-065: Channel Routing Based on Labels
// =============================================================================
// Routing configuration management for label-based channel selection.
// See: DD-WE-004 (skip-reason routing), NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md
// =============================================================================

// GetRoutingConfig returns the current routing configuration (thread-safe).
func (r *NotificationRequestReconciler) GetRoutingConfig() *routing.Config {
	r.routingMu.RLock()
	defer r.routingMu.RUnlock()
	return r.routingConfig
}

// SetRoutingConfig updates the routing configuration (thread-safe).
// Called by hot-reload when ConfigMap changes (BR-NOT-067).
func (r *NotificationRequestReconciler) SetRoutingConfig(config *routing.Config) {
	r.routingMu.Lock()
	defer r.routingMu.Unlock()
	r.routingConfig = config
}

// resolveChannelsFromRouting resolves delivery channels from routing rules when spec.channels is empty.
// BR-NOT-065: Use routing rules to determine channels based on CRD labels.
//
// Routing Priority (DD-WE-004):
//   - PreviousExecutionFailed → CRITICAL (PagerDuty)
//   - ExhaustedRetries → HIGH (Slack)
//   - ResourceBusy/RecentlyRemediated → LOW (Console bulk)
func (r *NotificationRequestReconciler) resolveChannelsFromRouting(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) []notificationv1alpha1.Channel {
	logger := log.FromContext(ctx)

	// BR-NOT-067: Use Router for thread-safe routing with hot-reload support
	if r.Router == nil {
		logger.Info("No routing router initialized, using default console channel",
			"notification", notification.Name)
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}
	}

	// Find receiver based on notification labels
	labels := notification.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	// Log routing labels for debugging
	routingLabels := make(map[string]string)
	for k, v := range labels {
		if strings.HasPrefix(k, "kubernaut.ai/") {
			routingLabels[k] = v
		}
	}
	logger.V(1).Info("Routing labels", "labels", routingLabels)

	// BR-NOT-067: Find matching receiver using thread-safe Router
	receiver := r.Router.FindReceiver(labels)

	// Convert receiver to channels
	channels := r.receiverToChannels(receiver)

	logger.Info("Resolved channels from routing",
		"notification", notification.Name,
		"receiver", receiver.Name,
		"channels", channels)

	return channels
}

// receiverToChannels converts a routing.Receiver to a list of notification channels.
func (r *NotificationRequestReconciler) receiverToChannels(receiver *routing.Receiver) []notificationv1alpha1.Channel {
	var channels []notificationv1alpha1.Channel

	// Map receiver configs to CRD channel types
	if len(receiver.SlackConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelSlack)
	}
	if len(receiver.PagerDutyConfigs) > 0 {
		// PagerDuty uses webhook channel type
		channels = append(channels, notificationv1alpha1.ChannelWebhook)
	}
	if len(receiver.EmailConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelEmail)
	}
	if len(receiver.WebhookConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelWebhook)
	}
	if len(receiver.ConsoleConfigs) > 0 {
		channels = append(channels, notificationv1alpha1.ChannelConsole)
	}

	// Default to console if no channels configured
	if len(channels) == 0 {
		channels = append(channels, notificationv1alpha1.ChannelConsole)
	}

	return channels
}

// SetupWithManager sets up the controller with the Manager.
// BR-NOT-067: Watches ConfigMap for routing configuration hot-reload
func (r *NotificationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize router with default config if not provided
	if r.Router == nil {
		r.Router = routing.NewRouter(ctrl.Log.WithName("routing"))
	}

	// Load initial routing config from ConfigMap (if exists)
	if err := r.loadRoutingConfigFromCluster(context.Background()); err != nil {
		// Non-fatal: use default config if ConfigMap doesn't exist
		ctrl.Log.Info("Using default routing config", "error", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationv1alpha1.NotificationRequest{}).
		// BR-NOT-067: Watch ConfigMaps for routing configuration hot-reload
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.handleConfigMapChange),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
				// Only watch the routing ConfigMap
				return routing.IsRoutingConfigMap(obj.GetName(), obj.GetNamespace())
			})),
		).
		Complete(r)
}

// =============================================================================
// BR-NOT-067: ConfigMap Hot-Reload Handler
// =============================================================================

// handleConfigMapChange handles changes to the routing ConfigMap.
// It reloads the routing configuration when the ConfigMap is created, updated, or deleted.
func (r *NotificationRequestReconciler) handleConfigMapChange(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	// Verify this is the routing ConfigMap
	if !routing.IsRoutingConfigMap(obj.GetName(), obj.GetNamespace()) {
		return nil
	}

	logger.Info("Routing ConfigMap changed, reloading configuration",
		"name", obj.GetName(),
		"namespace", obj.GetNamespace(),
	)

	// Reload routing configuration
	if err := r.loadRoutingConfigFromCluster(ctx); err != nil {
		logger.Error(err, "Failed to reload routing configuration, keeping previous config")
	}

	// Return empty - we don't need to reconcile any specific NotificationRequest
	// The new config will be used for future notifications
	return nil
}

// loadRoutingConfigFromCluster loads routing configuration from the cluster ConfigMap.
// BR-NOT-067: ConfigMap changes detected within 30 seconds
func (r *NotificationRequestReconciler) loadRoutingConfigFromCluster(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// Fetch the routing ConfigMap
	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      routing.DefaultConfigMapName,
		Namespace: routing.DefaultConfigMapNamespace,
	}

	if err := r.Get(ctx, key, configMap); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Routing ConfigMap not found, using default configuration",
				"name", key.Name,
				"namespace", key.Namespace,
			)
			// Use default config (already set in NewRouter)
			return nil
		}
		return fmt.Errorf("failed to get routing ConfigMap: %w", err)
	}

	// Extract routing YAML from ConfigMap
	yamlData, ok := routing.ExtractRoutingConfig(configMap.Data)
	if !ok {
		logger.Info("Routing ConfigMap found but missing routing.yaml key, using default configuration",
			"name", key.Name,
			"namespace", key.Namespace,
		)
		return nil
	}

	// Load the new configuration
	// BR-NOT-067: Routing table updated without restart
	if err := r.Router.LoadConfig(yamlData); err != nil {
		return fmt.Errorf("failed to load routing configuration: %w", err)
	}

	logger.Info("Routing configuration loaded successfully from ConfigMap",
		"name", key.Name,
		"namespace", key.Namespace,
		"summary", r.Router.GetConfigSummary(),
	)

	return nil
}
