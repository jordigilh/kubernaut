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

package phase

import (
	"fmt"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// ========================================
// PHASE TRANSITION DECISION LOGIC
// 📋 Extracted from internal/controller/notification (determinePhaseTransition)
// ========================================
//
// This file contains the pure business logic for determining phase transitions
// based on delivery results and channel states. It is independent of K8s
// persistence, metrics, or audit concerns.
//
// The controller delegates to DetermineTransition() for the decision, then
// handles the K8s persistence (AtomicStatusUpdate, metrics, audit) itself.
//
// Business Requirements:
// - BR-NOT-053: At-Least-Once Delivery
// - BR-NOT-054: Delivery Retry with Exponential Backoff
// - NT-BUG-003: Check for partial success before marking as Failed
// - NT-BUG-005: Handle partial success correctly during retry loop
// - NT-BUG-006: PartiallySent vs Retrying Phase Confusion
// ========================================

// DeliveryResult contains the results of a delivery loop for phase transition determination.
type DeliveryResult struct {
	// ChannelResults maps channel name to delivery error (nil = success)
	ChannelResults map[string]error

	// FailureCount is the number of channels that failed in this delivery loop
	FailureCount int

	// DeliveryAttempts from this delivery loop (used to count new successes)
	DeliveryAttempts []notificationv1.DeliveryAttempt
}

// ChannelState provides pre-computed state information about a delivery channel.
// The controller builds this from its helper methods before calling DetermineTransition.
type ChannelState struct {
	// AlreadySucceeded indicates the channel has already delivered successfully
	// (from persisted status + in-memory tracking)
	AlreadySucceeded bool

	// AttemptCount is the total number of delivery attempts for this channel
	// (persisted + in-flight)
	AttemptCount int

	// HasPermanentError indicates the channel has a non-retryable error
	HasPermanentError bool
}

// TransitionDecision contains the result of phase transition determination.
type TransitionDecision struct {
	// NextPhase is the determined next phase for the notification.
	// When PhaseUnchanged is true, this holds the current phase (no transition).
	NextPhase Phase

	// Reason is the machine-readable reason for the transition
	Reason string

	// Message is a human-readable description of the transition
	Message string

	// IsTerminal indicates if the next phase is a terminal state (Sent, PartiallySent, Failed)
	IsTerminal bool

	// ShouldRequeue indicates if the notification should be requeued for retry
	ShouldRequeue bool

	// PhaseUnchanged indicates no phase transition occurs (stay in current phase)
	PhaseUnchanged bool

	// IsPermanentFailure indicates this is a permanent failure (all retries exhausted)
	// Used by the controller to distinguish permanent vs temporary failure handling
	IsPermanentFailure bool

	// MaxFailedAttemptCount is the maximum attempt count among failed channels.
	// Used by the caller to calculate exponential backoff duration.
	MaxFailedAttemptCount int
}

// DetermineTransition determines the next phase based on delivery results and channel states.
//
// This function encodes the pure business logic for phase transition decisions,
// independent of K8s persistence, metrics, or audit concerns.
//
// Parameters:
//   - notification: The notification being processed (read-only, not modified)
//   - result: The delivery loop results
//   - channelStates: Map of channel name to pre-computed ChannelState
//   - maxAttempts: Maximum retry attempts from the retry policy
//
// The caller is responsible for:
//   - Building channelStates from its helper methods (channelAlreadySucceeded, etc.)
//   - Executing the K8s persistence based on the returned decision
//   - Calculating backoff duration using MaxFailedAttemptCount
func DetermineTransition(
	notification *notificationv1.NotificationRequest,
	result *DeliveryResult,
	channelStates map[string]ChannelState,
	maxAttempts int,
) *TransitionDecision {
	totalChannels := len(notification.Spec.Channels)

	// Count successful deliveries from BOTH status and current delivery loop attempts.
	// Status.SuccessfulDeliveries reflects persisted state; result.DeliveryAttempts
	// contains NEW attempts from the current loop that haven't been persisted yet.
	totalSuccessful := notification.Status.SuccessfulDeliveries
	for _, attempt := range result.DeliveryAttempts {
		if attempt.Status == "success" {
			totalSuccessful++
		}
	}

	// Case 1: All channels delivered successfully → Sent (terminal)
	if totalSuccessful == totalChannels {
		return &TransitionDecision{
			NextPhase:  Sent,
			Reason:     "AllDeliveriesSucceeded",
			Message:    fmt.Sprintf("Successfully delivered to %d channel(s)", totalSuccessful),
			IsTerminal: true,
		}
	}

	// Check if all channels have exhausted their retries (or succeeded/permanent-errored)
	allChannelsExhausted := true
	for _, channel := range notification.Spec.Channels {
		state := channelStates[string(channel)]
		if !state.AlreadySucceeded && !state.HasPermanentError && state.AttemptCount < maxAttempts {
			allChannelsExhausted = false
			break
		}
	}

	if allChannelsExhausted {
		// NT-BUG-003: Check for partial success before marking as Failed
		if totalSuccessful > 0 && totalSuccessful < totalChannels {
			// Case 2: Partial success, all retries exhausted → PartiallySent (terminal)
			return &TransitionDecision{
				NextPhase:  PartiallySent,
				Reason:     "PartialDeliverySuccess",
				Message:    fmt.Sprintf("Delivered to %d/%d channel(s), others failed", totalSuccessful, totalChannels),
				IsTerminal: true,
			}
		}

		// Determine failure reason: permanent errors vs retry exhaustion
		allPermanentErrors := true
		for _, channel := range notification.Spec.Channels {
			state := channelStates[string(channel)]
			if !state.HasPermanentError {
				allPermanentErrors = false
				break
			}
		}

		reason := "MaxRetriesExhausted"
		if allPermanentErrors {
			reason = "AllDeliveriesFailed"
		}

		// Case 3: All retries exhausted with no successes → Failed (terminal, permanent)
		return &TransitionDecision{
			NextPhase:          Failed,
			Reason:             reason,
			Message:            "All delivery attempts failed or exhausted retries",
			IsTerminal:         true,
			IsPermanentFailure: true,
		}
	}

	// Not all channels exhausted — check for failures with retries remaining
	if result.FailureCount > 0 {
		// Calculate max attempt count for failed channels (for backoff calculation)
		maxFailedAttempts := 0
		for _, channel := range notification.Spec.Channels {
			state := channelStates[string(channel)]
			if !state.AlreadySucceeded && state.AttemptCount > maxFailedAttempts {
				maxFailedAttempts = state.AttemptCount
			}
		}

		if totalSuccessful > 0 {
			// Case 4: NT-BUG-005/006: Partial success with retries remaining → Retrying
			return &TransitionDecision{
				NextPhase: Retrying,
				Reason:    "PartialFailureRetrying",
				Message: fmt.Sprintf("Delivered to %d/%d channel(s), retrying failed channels",
					totalSuccessful, totalChannels),
				ShouldRequeue:         true,
				MaxFailedAttemptCount: maxFailedAttempts,
			}
		}

		// Case 5: All channels failed, retries remain → stay in current phase, requeue
		return &TransitionDecision{
			NextPhase:             notification.Status.Phase,
			Reason:                "AllDeliveriesFailed",
			Message:               "Delivery failed, will retry with backoff",
			ShouldRequeue:         true,
			PhaseUnchanged:        true,
			MaxFailedAttemptCount: maxFailedAttempts,
		}
	}

	// Case 6: No failures (partial success with no failures — shouldn't normally reach here)
	return &TransitionDecision{
		NextPhase:      notification.Status.Phase,
		ShouldRequeue:  true,
		PhaseUnchanged: true,
	}
}
