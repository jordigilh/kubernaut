/*
Copyright 2025 Kubernaut.

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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/retry"
	"github.com/jordigilh/kubernaut/pkg/notification/sanitization"
)

// NotificationRequestReconciler reconciles a NotificationRequest object
type NotificationRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Delivery services
	ConsoleService *delivery.ConsoleDeliveryService
	SlackService   *delivery.SlackDeliveryService

	// Data sanitization
	Sanitizer *sanitization.Sanitizer

	// v3.1: Circuit breaker for graceful degradation (Category B)
	CircuitBreaker *retry.CircuitBreaker
}

//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=notification.kubernaut.ai,resources=notificationrequests/finalizers,verbs=update

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
			return r.handleNotFound(ctx, req.NamespacedName.String())
		}
		log.Error(err, "Failed to fetch NotificationRequest")
		return ctrl.Result{}, err
	}

	// Initialize status if this is the first reconciliation
	if notification.Status.Phase == "" {
		notification.Status.Phase = notificationv1alpha1.NotificationPhasePending
		notification.Status.Reason = "Initialized"
		notification.Status.Message = "Notification request received"
		notification.Status.ObservedGeneration = notification.Generation
		notification.Status.DeliveryAttempts = []notificationv1alpha1.DeliveryAttempt{}
		notification.Status.TotalAttempts = 0
		notification.Status.SuccessfulDeliveries = 0
		notification.Status.FailedDeliveries = 0

		if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
			log.Error(err, "Failed to initialize status")
			return ctrl.Result{}, err
		}

		log.Info("NotificationRequest status initialized", "name", notification.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// Skip processing if already in terminal state
	if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent ||
		notification.Status.Phase == notificationv1alpha1.NotificationPhaseFailed {
		// Check if max retries reached
		if notification.Status.CompletionTime != nil {
			log.Info("NotificationRequest in terminal state, skipping", "phase", notification.Status.Phase)
			return ctrl.Result{}, nil
		}
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
	}

	// Process deliveries for each channel
	deliveryResults := make(map[string]error)
	failureCount := 0

	// Get retry policy to check max attempts
	policy := r.getRetryPolicy(notification)

	for _, channel := range notification.Spec.Channels {
		// Skip if channel already succeeded (idempotent delivery)
		if r.channelAlreadySucceeded(notification, string(channel)) {
			log.Info("Channel already delivered successfully, skipping", "channel", channel)
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
			log.Error(deliveryErr, "Delivery failed", "channel", channel)
		} else {
			attempt.Status = "success"
			notification.Status.SuccessfulDeliveries++
			log.Info("Delivery successful", "channel", channel)
		}

		notification.Status.DeliveryAttempts = append(notification.Status.DeliveryAttempts, attempt)
		notification.Status.TotalAttempts++
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

// markChannelFailed handles Category C: Invalid Slack Webhook (permanent failure)
// When: 401/403 auth errors, invalid webhook URL
// Action: Mark as failed immediately, create event
// Recovery: Manual (fix webhook configuration)
func (r *NotificationRequestReconciler) markChannelFailed(ctx context.Context, nr *notificationv1alpha1.NotificationRequest, channel string, err error) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Error(err, "Permanent failure for channel", "channel", channel)

	// Update status to Failed
	nr.Status.Phase = notificationv1alpha1.NotificationPhaseFailed
	nr.Status.Reason = "PermanentFailure"
	nr.Status.Message = fmt.Sprintf("Channel %s failed permanently: %v", channel, err)

	// Create Kubernetes event for visibility
	// Note: EventRecorder needs to be added to NotificationRequestReconciler struct
	// r.EventRecorder.Event(nr, "Warning", "DeliveryFailed", nr.Status.Message)

	return ctrl.Result{}, r.updateStatusWithRetry(ctx, nr, 3)
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

// SetupWithManager sets up the controller with the Manager.
func (r *NotificationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationv1alpha1.NotificationRequest{}).
		Complete(r)
}
