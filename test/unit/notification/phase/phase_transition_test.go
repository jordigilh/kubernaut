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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationphase "github.com/jordigilh/kubernaut/pkg/notification/phase"
)

var _ = Describe("Phase Transition Logic - DetermineTransition", func() {

	// ========================================
	// NT-BUG-006: PartiallySent vs Retrying Phase Confusion
	// ========================================
	// This test reproduces the exact scenario from failing E2E tests:
	// - Console channel: Succeeds on first attempt
	// - File channel: Fails on first attempt (1/5 attempts used)
	// - Expected: Transition to Retrying (4 more attempts available)
	// - Actual (BUG): Transitions to PartiallySent (terminal phase)
	//
	// Root Cause: allChannelsExhausted logic incorrectly returns true
	// when hasSuccess=true for console channel, even though file channel
	// has 4 remaining retry attempts.
	Context("NT-BUG-006: Partial Success with Retries Remaining", func() {
		It("should transition to Retrying when one channel succeeds and another fails with retries remaining", func() {
			// ===== ARRANGE =====
			// Simulate E2E test scenario:
			// - 2 channels: Console (success), File (failed, 1 attempt)
			// - MaxAttempts: 5 (4 more retries available for file channel)
			//
			// IMPORTANT: notification.Status reflects state BEFORE the current delivery loop.
			// This is the first delivery attempt, so status is empty.
			// The delivery results are ONLY in the DeliveryResult.
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-retry-transition",
					Namespace: "default",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeSimple,
					Subject:  "Unit Test: Phase Transition Bug",
					Body:     "Testing Retrying vs PartiallySent transition",
					Priority: notificationv1.NotificationPriorityMedium,
					Channels: []notificationv1.Channel{
						notificationv1.ChannelConsole, // Succeeds
						notificationv1.ChannelFile,    // Fails, but has retries
					},
					RetryPolicy: &notificationv1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseSending,
					SuccessfulDeliveries: 0, // Not yet recorded (atomic update hasn't happened)
					FailedDeliveries:     0,
				},
			}

			// Current delivery loop results: console succeeded, file failed
			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"console": nil,                                                // Success
					"file":    fmt.Errorf("permission denied: read-only directory"), // Failure
				},
				FailureCount: 1,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{
						Channel:   "console",
						Timestamp: metav1.Now(),
						Status:    "success",
						Attempt:   1,
					},
					{
						Channel:   "file",
						Timestamp: metav1.Now(),
						Status:    "failed",
						Error:     "permission denied: read-only directory",
						Attempt:   1,
					},
				},
			}

			// Channel states reflect what the controller's helper methods would return:
			// - Console: 1 attempt, succeeded (from orchestrator in-memory tracking)
			// - File: 1 attempt, not succeeded, no permanent error
			channelStates := map[string]notificationphase.ChannelState{
				"console": {
					AlreadySucceeded:  true, // Console delivery succeeded
					AttemptCount:      1,
					HasPermanentError: false,
				},
				"file": {
					AlreadySucceeded:  false,
					AttemptCount:      1,    // Only 1 attempt, 4 more available
					HasPermanentError: false,
				},
			}

			// ===== ACT =====
			decision := notificationphase.DetermineTransition(
				notification, deliveryResult, channelStates, 5,
			)

			// ===== ASSERT =====
			// CRITICAL ASSERTION: Should transition to Retrying, NOT PartiallySent
			Expect(decision.NextPhase).To(Equal(notificationphase.Retrying),
				"Should transition to Retrying when partial success occurs with retries remaining.\n"+
					"Current bug: Transitions to PartiallySent (terminal) prematurely.\n"+
					"Expected: Retrying (non-terminal, allows 4 more retry attempts)\n"+
					"Console: SUCCESS (1 attempt)\n"+
					"File: FAILED (1/5 attempts, 4 remaining)")

			Expect(decision.Reason).To(Equal("PartialFailureRetrying"))
			Expect(decision.ShouldRequeue).To(BeTrue(),
				"Should schedule next retry")
			Expect(decision.IsTerminal).To(BeFalse(),
				"Retrying is NOT a terminal phase")
			Expect(decision.MaxFailedAttemptCount).To(Equal(1),
				"Max failed attempt count should be 1 (file channel's attempt count)")
		})

		It("should only transition to PartiallySent when retries are EXHAUSTED", func() {
			// ===== ARRANGE =====
			// Console succeeded on attempt 1. File failed ALL 5 attempts.
			// Status reflects persisted state AFTER previous delivery loops.
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-exhausted-transition",
					Namespace: "default",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Type:     notificationv1.NotificationTypeSimple,
					Subject:  "Unit Test: Exhausted Retries",
					Body:     "Testing PartiallySent when retries exhausted",
					Priority: notificationv1.NotificationPriorityMedium,
					Channels: []notificationv1.Channel{
						notificationv1.ChannelConsole, // Succeeded earlier
						notificationv1.ChannelFile,    // Failed all 5 attempts
					},
					RetryPolicy: &notificationv1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase: notificationv1.NotificationPhaseRetrying,
					// Console: 1 successful delivery (persisted)
					// File: 4 failed deliveries persisted (5th attempt in current loop)
					DeliveryAttempts: []notificationv1.DeliveryAttempt{
						{Channel: "console", Status: "success", Attempt: 1},
						{Channel: "file", Status: "failed", Error: "attempt 1: permission denied", Attempt: 1},
						{Channel: "file", Status: "failed", Error: "attempt 2: permission denied", Attempt: 2},
						{Channel: "file", Status: "failed", Error: "attempt 3: permission denied", Attempt: 3},
						{Channel: "file", Status: "failed", Error: "attempt 4: permission denied", Attempt: 4},
					},
					SuccessfulDeliveries: 1, // Console succeeded
					FailedDeliveries:     1, // File channel failed (unique channel count, not attempt count)
				},
			}

			// Current delivery loop: 5th and final attempt for file channel
			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"file": fmt.Errorf("attempt 5: permission denied"),
				},
				FailureCount: 1,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{
						Channel: "file",
						Status:  "failed",
						Error:   "attempt 5: permission denied",
						Attempt: 5,
					},
				},
			}

			// Channel states: console already succeeded, file exhausted (5/5 attempts)
			channelStates := map[string]notificationphase.ChannelState{
				"console": {
					AlreadySucceeded:  true,
					AttemptCount:      1,
					HasPermanentError: false,
				},
				"file": {
					AlreadySucceeded:  false,
					AttemptCount:      5, // All 5 attempts exhausted
					HasPermanentError: false,
				},
			}

			// ===== ACT =====
			decision := notificationphase.DetermineTransition(
				notification, deliveryResult, channelStates, 5,
			)

			// ===== ASSERT =====
			// NOW PartiallySent is correct (all retries exhausted)
			Expect(decision.NextPhase).To(Equal(notificationphase.PartiallySent),
				"Should transition to PartiallySent when retries are EXHAUSTED.\n"+
					"Console: SUCCESS (1 attempt)\n"+
					"File: FAILED (5/5 attempts, 0 remaining)")

			Expect(decision.IsTerminal).To(BeTrue(),
				"PartiallySent is a terminal phase")
			Expect(decision.ShouldRequeue).To(BeFalse(),
				"Should NOT requeue — terminal phase reached")
		})
	})

	Context("Edge Cases - Phase Transition Logic", func() {
		It("should stay in current phase with requeue when ALL channels fail but retries remain", func() {
			// ===== ARRANGE =====
			// Both channels fail on first attempt. Retries remain.
			// Current implementation: stays in current phase (Sending) and requeues.
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-all-failed-retries-remain",
					Namespace: "default",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Channels: []notificationv1.Channel{
						notificationv1.ChannelConsole,
						notificationv1.ChannelFile,
					},
					RetryPolicy: &notificationv1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseSending,
					SuccessfulDeliveries: 0,
					FailedDeliveries:     0,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"console": fmt.Errorf("network timeout"),
					"file":    fmt.Errorf("permission denied"),
				},
				FailureCount: 2,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{Channel: "console", Status: "failed", Error: "network timeout", Attempt: 1},
					{Channel: "file", Status: "failed", Error: "permission denied", Attempt: 1},
				},
			}

			channelStates := map[string]notificationphase.ChannelState{
				"console": {
					AlreadySucceeded:  false,
					AttemptCount:      1,
					HasPermanentError: false,
				},
				"file": {
					AlreadySucceeded:  false,
					AttemptCount:      1,
					HasPermanentError: false,
				},
			}

			// ===== ACT =====
			decision := notificationphase.DetermineTransition(
				notification, deliveryResult, channelStates, 5,
			)

			// ===== ASSERT =====
			// Phase stays unchanged (Sending) with requeue for retry
			Expect(decision.PhaseUnchanged).To(BeTrue(),
				"Phase should remain unchanged when all channels fail but retries remain")
			Expect(decision.NextPhase).To(Equal(notificationv1.NotificationPhaseSending),
				"NextPhase should be the current phase (Sending)")
			Expect(decision.ShouldRequeue).To(BeTrue(),
				"Should requeue with backoff for retry")
			Expect(decision.Reason).To(Equal("AllDeliveriesFailed"))
			Expect(decision.MaxFailedAttemptCount).To(Equal(1),
				"Max failed attempt count should be 1 (both channels at 1 attempt)")
		})

		It("should transition to Sent when all channels succeed", func() {
			// ===== ARRANGE =====
			// First delivery loop: both channels succeed.
			// Status is empty (first attempt, no persisted results yet).
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-all-succeeded",
					Namespace: "default",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Channels: []notificationv1.Channel{
						notificationv1.ChannelConsole,
						notificationv1.ChannelFile,
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseSending,
					SuccessfulDeliveries: 0,
					FailedDeliveries:     0,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"console": nil,
					"file":    nil,
				},
				FailureCount: 0,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{Channel: "console", Status: "success", Attempt: 1},
					{Channel: "file", Status: "success", Attempt: 1},
				},
			}

			channelStates := map[string]notificationphase.ChannelState{
				"console": {
					AlreadySucceeded:  true,
					AttemptCount:      1,
					HasPermanentError: false,
				},
				"file": {
					AlreadySucceeded:  true,
					AttemptCount:      1,
					HasPermanentError: false,
				},
			}

			// ===== ACT =====
			decision := notificationphase.DetermineTransition(
				notification, deliveryResult, channelStates, 5,
			)

			// ===== ASSERT =====
			Expect(decision.NextPhase).To(Equal(notificationphase.Sent),
				"Should transition to Sent when all channels succeed")
			Expect(decision.IsTerminal).To(BeTrue(),
				"Sent is a terminal phase")
			Expect(decision.ShouldRequeue).To(BeFalse(),
				"Should NOT requeue — terminal phase reached")
			Expect(decision.Reason).To(Equal("AllDeliveriesSucceeded"))
		})

		It("should transition to Failed (permanent) when all channels exhaust retries with no success", func() {
			// ===== ARRANGE =====
			// Both channels failed all 5 attempts. No successes.
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-all-exhausted-no-success",
					Namespace: "default",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Channels: []notificationv1.Channel{
						notificationv1.ChannelConsole,
						notificationv1.ChannelFile,
					},
					RetryPolicy: &notificationv1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 5,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     60,
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseRetrying,
					SuccessfulDeliveries: 0,
					FailedDeliveries:     2,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"console": fmt.Errorf("network timeout"),
					"file":    fmt.Errorf("permission denied"),
				},
				FailureCount: 2,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{Channel: "console", Status: "failed", Error: "network timeout", Attempt: 5},
					{Channel: "file", Status: "failed", Error: "permission denied", Attempt: 5},
				},
			}

			channelStates := map[string]notificationphase.ChannelState{
				"console": {
					AlreadySucceeded:  false,
					AttemptCount:      5, // Exhausted
					HasPermanentError: false,
				},
				"file": {
					AlreadySucceeded:  false,
					AttemptCount:      5, // Exhausted
					HasPermanentError: false,
				},
			}

			// ===== ACT =====
			decision := notificationphase.DetermineTransition(
				notification, deliveryResult, channelStates, 5,
			)

			// ===== ASSERT =====
			Expect(decision.NextPhase).To(Equal(notificationphase.Failed),
				"Should transition to Failed when all retries exhausted with no success")
			Expect(decision.IsTerminal).To(BeTrue(),
				"Failed is a terminal phase")
			Expect(decision.IsPermanentFailure).To(BeTrue(),
				"Should be a permanent failure")
			Expect(decision.Reason).To(Equal("MaxRetriesExhausted"))
		})

		It("should report AllDeliveriesFailed reason when all channels have permanent errors", func() {
			// ===== ARRANGE =====
			// Both channels have permanent (non-retryable) errors.
			notification := &notificationv1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-permanent-errors",
					Namespace: "default",
				},
				Spec: notificationv1.NotificationRequestSpec{
					Channels: []notificationv1.Channel{
						notificationv1.ChannelConsole,
						notificationv1.ChannelFile,
					},
					RetryPolicy: &notificationv1.RetryPolicy{
						MaxAttempts: 5,
					},
				},
				Status: notificationv1.NotificationRequestStatus{
					Phase:                notificationv1.NotificationPhaseSending,
					SuccessfulDeliveries: 0,
					FailedDeliveries:     2,
				},
			}

			deliveryResult := &notificationphase.DeliveryResult{
				ChannelResults: map[string]error{
					"console": fmt.Errorf("[PERMANENT] invalid config"),
					"file":    fmt.Errorf("[PERMANENT] path not found"),
				},
				FailureCount: 2,
				DeliveryAttempts: []notificationv1.DeliveryAttempt{
					{Channel: "console", Status: "failed", Error: "[PERMANENT] invalid config", Attempt: 1},
					{Channel: "file", Status: "failed", Error: "[PERMANENT] path not found", Attempt: 1},
				},
			}

			channelStates := map[string]notificationphase.ChannelState{
				"console": {
					AlreadySucceeded:  false,
					AttemptCount:      1,
					HasPermanentError: true, // Permanent error
				},
				"file": {
					AlreadySucceeded:  false,
					AttemptCount:      1,
					HasPermanentError: true, // Permanent error
				},
			}

			// ===== ACT =====
			decision := notificationphase.DetermineTransition(
				notification, deliveryResult, channelStates, 5,
			)

			// ===== ASSERT =====
			Expect(decision.NextPhase).To(Equal(notificationphase.Failed))
			Expect(decision.IsTerminal).To(BeTrue())
			Expect(decision.IsPermanentFailure).To(BeTrue())
			Expect(decision.Reason).To(Equal("AllDeliveriesFailed"),
				"Should use AllDeliveriesFailed reason when all channels have permanent errors")
		})
	})
})
