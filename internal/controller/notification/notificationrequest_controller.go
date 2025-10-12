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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
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
			log.Info("NotificationRequest not found, likely deleted")
			return ctrl.Result{}, nil
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

		if err := r.Status().Update(ctx, notification); err != nil {
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

		if err := r.Status().Update(ctx, notification); err != nil {
			log.Error(err, "Failed to update phase to Sending")
			return ctrl.Result{}, err
		}
	}

	// Process deliveries for each channel
	deliveryResults := make(map[string]error)
	failureCount := 0

	for _, channel := range notification.Spec.Channels {
		// Skip if channel already succeeded (idempotent delivery)
		if r.channelAlreadySucceeded(notification, string(channel)) {
			log.Info("Channel already delivered successfully, skipping", "channel", channel)
			continue
		}

		// Check channel attempt count (max 5 attempts per channel)
		attemptCount := r.getChannelAttemptCount(notification, string(channel))
		if attemptCount >= 5 {
			log.Info("Max retry attempts reached for channel", "channel", channel, "attempts", attemptCount)
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
	if failureCount == 0 {
		// All deliveries succeeded
		notification.Status.Phase = notificationv1alpha1.NotificationPhaseSent
		now := metav1.Now()
		notification.Status.CompletionTime = &now
		notification.Status.Reason = "AllDeliveriesSucceeded"
		notification.Status.Message = fmt.Sprintf("Successfully delivered to %d channel(s)", len(deliveryResults))

		if err := r.Status().Update(ctx, notification); err != nil {
			log.Error(err, "Failed to update status to Sent")
			return ctrl.Result{}, err
		}

		log.Info("All deliveries successful", "name", notification.Name)
		return ctrl.Result{}, nil // No requeue - done

	} else if failureCount < len(deliveryResults) {
		// Partial success
		notification.Status.Phase = notificationv1alpha1.NotificationPhasePartiallySent
		notification.Status.Reason = "PartialDeliveryFailure"
		notification.Status.Message = fmt.Sprintf("%d of %d deliveries succeeded", len(deliveryResults)-failureCount, len(deliveryResults))

		if err := r.Status().Update(ctx, notification); err != nil {
			log.Error(err, "Failed to update status to PartiallySent")
			return ctrl.Result{}, err
		}

		// Requeue failed channels with exponential backoff
		maxAttempt := r.getMaxAttemptCount(notification)
		if maxAttempt >= 5 {
			// Max retries reached
			notification.Status.Phase = notificationv1alpha1.NotificationPhaseFailed
			now := metav1.Now()
			notification.Status.CompletionTime = &now
			notification.Status.Reason = "MaxRetriesExceeded"
			notification.Status.Message = "Maximum retry attempts exceeded"

			if err := r.Status().Update(ctx, notification); err != nil {
				log.Error(err, "Failed to update status to Failed")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil // No requeue - terminal state
		}

		// Requeue with exponential backoff
		backoff := CalculateBackoff(maxAttempt)
		log.Info("Requeuing for retry", "after", backoff, "attempt", maxAttempt+1)
		return ctrl.Result{RequeueAfter: backoff}, nil

	} else {
		// All deliveries failed
		notification.Status.Phase = notificationv1alpha1.NotificationPhaseFailed
		notification.Status.Reason = "AllDeliveriesFailed"
		notification.Status.Message = fmt.Sprintf("All %d deliveries failed", len(deliveryResults))

		// Check if max retries reached
		maxAttempt := r.getMaxAttemptCount(notification)
		if maxAttempt >= 5 {
			now := metav1.Now()
			notification.Status.CompletionTime = &now

			if err := r.Status().Update(ctx, notification); err != nil {
				log.Error(err, "Failed to update status to Failed")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil // No requeue - terminal state
		}

		// Update status and requeue with exponential backoff
		if err := r.Status().Update(ctx, notification); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}

		backoff := CalculateBackoff(maxAttempt)
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

	// Sanitize notification content before delivery
	sanitizedNotification := r.sanitizeNotification(notification)
	return r.SlackService.Deliver(ctx, sanitizedNotification)
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

// CalculateBackoff calculates exponential backoff duration
// Backoff progression: 30s, 60s, 120s, 240s, 480s (capped)
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

// SetupWithManager sets up the controller with the Manager.
func (r *NotificationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationv1alpha1.NotificationRequest{}).
		Complete(r)
}
