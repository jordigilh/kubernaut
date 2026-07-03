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
	"math"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/sync/singleflight"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/enrichment"
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
// DD-NOT-008: Concurrent Delivery Deduplication (singleflight + optimistic locking)
// ========================================
// Orchestrator manages delivery orchestration across channels.
//
// MANDATORY: Channels MUST be registered via RegisterChannel(), NOT constructor parameters
// See: docs/architecture/decisions/DD-NOT-007-DELIVERY-ORCHESTRATOR-REGISTRATION-PATTERN.md
//
// DD-NOT-008: Production-Grade Concurrency Control
// See: docs/architecture/decisions/DD-NOT-008-CONCURRENT-DELIVERY-DEDUPLICATION.md
// - singleflight.Group: Deduplicates concurrent delivery attempts
// - Reserve-then-check: In-flight counter incremented BEFORE max-attempts gate (TOCTOU fix)
// - Optimistic locking: Detects stale reconciliations via resourceVersion checks
type Orchestrator struct {
	// DD-NOT-007: Dynamic channel registration (sync.Map for thread-safe parallel test execution)
	// sync.Map is optimal for our access pattern: write-once per test, read-many during deliveries
	channels sync.Map

	// DD-NOT-008: Concurrent delivery deduplication (prevents duplicate deliveries in multi-replica deployments)
	// Key format: "{notificationUID}:{channel}" ensures per-notification-channel deduplication
	deliveryGroup singleflight.Group

	// DD-NOT-008: In-flight attempt tracking (TOCTOU fix: reserve-then-check pattern)
	// Incremented BEFORE the max-attempts gate so concurrent callers see each other's reservations
	// Key format: "{notificationUID}:{channel}"
	// Value: int count of in-flight attempts for that notification+channel
	inFlightAttempts sync.Map

	// DD-NOT-008: Successful delivery tracking (prevents duplicate deliveries)
	// Tracks successful deliveries that haven't been persisted to status yet
	// Key format: "{notificationUID}:{channel}"
	// Value: bool (true if successfully delivered)
	successfulDeliveries sync.Map

	// DD-NOT-008 v2: Per-notification delivery mutex. Serializes the
	// reserve-check-deliver cycle for each notification, eliminating the
	// TOCTOU window between in-flight decrement and status persistence.
	// Key: notification UID, Value: *sync.Mutex
	deliveryMu sync.Map

	// Dependencies
	sanitizer     *sanitization.Sanitizer
	metrics       *notificationmetrics.Metrics
	statusManager *notificationstatus.Manager
	enricher      *enrichment.Enricher

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
	metrics *notificationmetrics.Metrics,
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

// SetEnricher sets the notification enricher for workflow name resolution (#553).
func (o *Orchestrator) SetEnricher(e *enrichment.Enricher) {
	o.enricher = e
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
// DeliveryCallbacks groups the controller-specific callback functions that
// DeliverToChannels uses to check delivery state and record outcomes.
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type DeliveryCallbacks struct {
	// ChannelAlreadySucceeded reports whether a channel already has a
	// successful delivery recorded (idempotent delivery).
	ChannelAlreadySucceeded func(*notificationv1alpha1.NotificationRequest, string) bool

	// HasChannelPermanentError reports whether a channel has a permanent
	// (non-retryable) error recorded. BR-NOT-055.
	HasChannelPermanentError func(*notificationv1alpha1.NotificationRequest, string) bool

	// GetChannelAttemptCount returns the total attempt count for a channel.
	GetChannelAttemptCount func(*notificationv1alpha1.NotificationRequest, string) int

	// AuditMessageSent records a successful delivery audit event.
	AuditMessageSent func(context.Context, *notificationv1alpha1.NotificationRequest, string) error

	// AuditMessageFailed records a failed delivery audit event.
	AuditMessageFailed func(context.Context, *notificationv1alpha1.NotificationRequest, string, error) error

	// CheckBeforeDelivery is an optional pre-delivery check (e.g. circuit
	// breaker). If it returns an error, delivery is skipped and treated as
	// a failure. DD-EVENT-001 v1.1.
	CheckBeforeDelivery func(*notificationv1alpha1.NotificationRequest, string) error
}

func (o *Orchestrator) DeliverToChannels(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channels []notificationv1alpha1.Channel,
	policy *notificationv1alpha1.RetryPolicy,
	callbacks DeliveryCallbacks,
) (*DeliveryResult, error) {
	log := o.logger.WithValues("notification", notification.Name, "namespace", notification.Namespace)

	// DD-NOT-008 v2: Serialize delivery attempts per notification. This eliminates
	// the TOCTOU window where concurrent reconciles both pass the max-attempts gate
	// because inFlight was decremented before status persistence.
	mu := o.getDeliveryMutex(string(notification.UID))
	mu.Lock()
	defer mu.Unlock()

	// Initialize result
	result := &DeliveryResult{
		DeliveryResults:  make(map[string]error),
		FailureCount:     0,
		DeliveryAttempts: []notificationv1alpha1.DeliveryAttempt{}, // Collect attempts for batch update
	}

	// #553: Enrich notification body (resolve workflow UUID → name) before delivery.
	// Operates on a DeepCopy so the original cached object is never mutated.
	if o.enricher != nil {
		notification = o.enricher.EnrichNotification(ctx, notification)
	}

	// Process each channel
	for _, channel := range channels {
		if err := o.deliverToOneChannel(ctx, notification, channel, policy, callbacks, result, log); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// deliverToOneChannel processes delivery for a single channel within
// DeliverToChannels: idempotency skip (already succeeded), BR-NOT-055
// permanent-error skip, DD-NOT-008 TOCTOU-safe attempt reservation/max-
// attempts gate, optional pre-delivery check (e.g. circuit breaker), and
// finally the delivery attempt itself with its audit/metrics recording.
// Mutates result in place; returns a non-nil error only for the ADR-032 §1
// mandatory-audit-failure case, which must abort the whole batch. Extracted
// from DeliverToChannels (Wave 6 6b GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (o *Orchestrator) deliverToOneChannel(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel notificationv1alpha1.Channel, policy *notificationv1alpha1.RetryPolicy, callbacks DeliveryCallbacks, result *DeliveryResult, log logr.Logger) error {
	// Skip if channel already succeeded (idempotent delivery)
	if callbacks.ChannelAlreadySucceeded(notification, string(channel)) {
		log.Info("Channel already delivered successfully, skipping", "channel", channel)
		// NT-BUG-004 Fix: Count already-successful channels as successes
		result.DeliveryResults[string(channel)] = nil // nil = success
		return nil
	}

	// BR-NOT-055: Check if channel has permanent error (skip retries for 4xx errors)
	if callbacks.HasChannelPermanentError(notification, string(channel)) {
		log.Info("Channel has permanent error, skipping retries", "channel", channel)
		result.DeliveryResults[string(channel)] = fmt.Errorf("permanent error - not retryable")
		result.FailureCount++
		return nil
	}

	// DD-NOT-008 TOCTOU fix: Reserve an in-flight slot BEFORE checking
	// the attempt count. This closes the race window where concurrent
	// reconciles both read attemptCount < MaxAttempts before either
	// increments. With reserve-then-check, each concurrent caller's
	// reservation is visible to all others via GetTotalAttemptCount.
	o.incrementInFlightAttempts(string(notification.UID), string(channel))

	attemptCount := callbacks.GetChannelAttemptCount(notification, string(channel))
	if attemptCount > policy.MaxAttempts {
		// Over-reserved: another concurrent reconcile already claimed
		// the last slot. Release our reservation and skip.
		// Note: use > (not >=) because attemptCount includes OUR reservation
		// from incrementInFlightAttempts above. When attemptCount == MaxAttempts,
		// we are the Nth allowed attempt and should proceed.
		o.decrementInFlightAttempts(string(notification.UID), string(channel))
		log.Info("Max retry attempts reached for channel", "channel", channel, "attempts", attemptCount, "maxAttempts", policy.MaxAttempts)
		result.DeliveryResults[string(channel)] = fmt.Errorf("max retry attempts exceeded")
		result.FailureCount++
		return nil
	}

	// DD-EVENT-001 v1.1: Optional pre-delivery check (e.g. circuit breaker)
	skip, err := o.runPreDeliveryCheck(ctx, notification, channel, attemptCount, callbacks, result, log)
	if err != nil || skip {
		return err
	}

	return o.deliverAndRecordAttempt(ctx, notification, channel, attemptCount, callbacks, result, log)
}

// runPreDeliveryCheck runs the optional CheckBeforeDelivery callback (e.g.
// circuit breaker) and, when it fails, records a failed DeliveryAttempt plus
// the mandatory message.failed audit event (ADR-032 §1) and releases the
// in-flight reservation. Returns a non-nil error only when the audit write
// itself fails. Extracted from DeliverToChannels (Wave 6 6b GREEN: funlen
// remediation) — pure code motion, no behavior change.
// Returns skip=true when the channel should NOT proceed to actual delivery
// (either no check configured and nothing to do, or the check failed and
// was already recorded).
func (o *Orchestrator) runPreDeliveryCheck(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel notificationv1alpha1.Channel, attemptCount int, callbacks DeliveryCallbacks, result *DeliveryResult, log logr.Logger) (bool, error) {
	if callbacks.CheckBeforeDelivery == nil {
		return false, nil
	}
	checkErr := callbacks.CheckBeforeDelivery(notification, string(channel))
	if checkErr == nil {
		return false, nil
	}

	o.decrementInFlightAttempts(string(notification.UID), string(channel))
	log.Info("Pre-delivery check failed, skipping channel", "channel", channel, "error", checkErr)
	attempt := notificationv1alpha1.DeliveryAttempt{
		Channel:         notificationv1alpha1.DeliveryChannelName(channel),
		Attempt:         attemptCount,
		Timestamp:       metav1.Now(),
		Status:          "failed",
		Error:           checkErr.Error(),
		DurationSeconds: 0,
	}
	if auditErr := callbacks.AuditMessageFailed(ctx, notification, string(channel), checkErr); auditErr != nil {
		log.Error(auditErr, "CRITICAL: Failed to audit message.failed (ADR-032 §1)")
		return true, fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
	}
	o.metrics.RecordDeliveryAttempt(notification.Namespace, string(channel), "failed")
	result.DeliveryResults[string(channel)] = checkErr
	result.FailureCount++
	result.DeliveryAttempts = append(result.DeliveryAttempts, attempt)
	return true, nil
}

// deliverAndRecordAttempt performs the actual channel delivery, builds the
// DeliveryAttempt record (status/error/duration), classifies permanent vs.
// retryable failures (BR-NOT-055), records the mandatory message.sent /
// message.failed audit event (ADR-032 §1), and updates delivery metrics.
// Extracted from DeliverToChannels (Wave 6 6b GREEN: funlen remediation) —
// pure code motion, no behavior change.
func (o *Orchestrator) deliverAndRecordAttempt(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel notificationv1alpha1.Channel, attemptCount int, callbacks DeliveryCallbacks, result *DeliveryResult, log logr.Logger) error {
	// Record duration for DeliveryAttempt.DurationSeconds
	start := time.Now()

	// Attempt delivery
	deliveryErr := o.DeliverToChannel(ctx, notification, channel)

	// Round to milliseconds - sub-ms precision is typically noise for observability
	durationSeconds := math.Round(time.Since(start).Seconds()*1000) / 1000

	// Decrement in-flight counter now that delivery is complete.
	// DD-NOT-008 v2: The per-notification mutex in DeliverToChannels
	// serializes concurrent reconciles, so the decrement-before-persist
	// gap is no longer exploitable.
	o.decrementInFlightAttempts(string(notification.UID), string(channel))

	// Create delivery attempt record (but DON'T write to status yet)
	// This prevents status updates from triggering immediate reconciles
	attempt := notificationv1alpha1.DeliveryAttempt{
		Channel:         notificationv1alpha1.DeliveryChannelName(channel),
		Attempt:         attemptCount, // Already includes our reservation from pre-check increment
		Timestamp:       metav1.Now(),
		DurationSeconds: durationSeconds,
	}

	var err error
	if deliveryErr != nil {
		err = o.recordFailedDeliveryOutcome(ctx, notification, deliveryErr, &attempt, callbacks.AuditMessageFailed, result, log)
	} else {
		err = o.recordSuccessfulDeliveryOutcome(ctx, notification, channel, &attempt, callbacks, result, log)
	}
	if err != nil {
		return err
	}

	// Add attempt to result (will be recorded in batch after loop completes)
	result.DeliveryAttempts = append(result.DeliveryAttempts, attempt)
	return nil
}

// recordFailedDeliveryOutcome finalizes a failed delivery attempt: BR-NOT-055
// permanent-vs-retryable error classification, the mandatory message.failed
// audit event (ADR-032 §1), and failure metrics. Extracted from
// deliverAndRecordAttempt (Wave 6 6b GREEN: nestif remediation) — pure code
// motion, no behavior change.
func (o *Orchestrator) recordFailedDeliveryOutcome(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, deliveryErr error, attempt *notificationv1alpha1.DeliveryAttempt, auditMessageFailed func(context.Context, *notificationv1alpha1.NotificationRequest, string, error) error, result *DeliveryResult, log logr.Logger) error {
	channel := string(attempt.Channel)
	attempt.Status = notificationv1alpha1.DeliveryAttemptStatusFailed
	attempt.Error = deliveryErr.Error()

	// BR-NOT-055: Permanent Error Classification
	if !IsRetryableError(deliveryErr) {
		log.Error(deliveryErr, "Delivery failed with permanent error (will NOT retry)")
		attempt.Error = fmt.Sprintf("permanent failure: %s", deliveryErr.Error())
	} else {
		log.Error(deliveryErr, "Delivery failed with retryable error")
	}

	// AUDIT: Failed delivery (ADR-032 §1: MANDATORY)
	// Audit calls don't trigger reconciles, so they're safe to call immediately
	if auditErr := auditMessageFailed(ctx, notification, channel, deliveryErr); auditErr != nil {
		log.Error(auditErr, "CRITICAL: Failed to audit message.failed (ADR-032 §1)")
		return fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
	}

	// Update metrics (DD-METRICS-001: Use injected metrics recorder)
	o.metrics.RecordDeliveryAttempt(notification.Namespace, channel, "failed")
	result.DeliveryResults[channel] = deliveryErr
	result.FailureCount++
	return nil
}

// recordSuccessfulDeliveryOutcome finalizes a successful delivery attempt:
// the mandatory message.sent audit event (ADR-032 §1) and success metrics.
// Extracted from deliverAndRecordAttempt (Wave 6 6b GREEN: nestif
// remediation) — pure code motion, no behavior change.
func (o *Orchestrator) recordSuccessfulDeliveryOutcome(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel notificationv1alpha1.Channel, attempt *notificationv1alpha1.DeliveryAttempt, callbacks DeliveryCallbacks, result *DeliveryResult, log logr.Logger) error {
	attempt.Status = notificationv1alpha1.DeliveryAttemptStatusSuccess
	attempt.Error = ""

	log.Info("Delivery successful", "channel", channel)

	// AUDIT: Successful delivery (ADR-032 §1: MANDATORY)
	if auditErr := callbacks.AuditMessageSent(ctx, notification, string(channel)); auditErr != nil {
		log.Error(auditErr, "CRITICAL: Failed to audit message.sent (ADR-032 §1)")
		return fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
	}

	// Update metrics (DD-METRICS-001: Use injected metrics recorder)
	o.metrics.RecordDeliveryAttempt(notification.Namespace, string(channel), "success")
	result.DeliveryResults[string(channel)] = nil // nil = success
	return nil
}

// DeliverToChannel attempts delivery to a specific channel.
//
// DD-NOT-007: Map-based routing (NO switch statement)
// DD-NOT-008: Concurrent delivery deduplication (singleflight)
//
// Production-grade concurrency control:
//   - singleflight prevents duplicate deliveries when multiple reconciliations
//     attempt delivery to the same notification+channel concurrently
//   - Key format: "{notificationUID}:{channel}" ensures per-notification-channel deduplication
//   - Only ONE goroutine executes delivery; others wait and receive same result
//
// This fixes the "6 attempts instead of 5" bug where stale cache caused
// concurrent reconciliations to all think attemptCount < MaxAttempts.
func (o *Orchestrator) DeliverToChannel(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channel notificationv1alpha1.Channel,
) error {
	// DD-NOT-008: Use singleflight to deduplicate concurrent delivery attempts
	// Key format ensures per-notification-channel deduplication
	key := fmt.Sprintf("%s:%s", notification.UID, channel)

	// singleflight.Do ensures only ONE delivery attempt executes
	// Concurrent calls with same key wait and receive the same result
	result, err, shared := o.deliveryGroup.Do(key, func() (interface{}, error) {
		// This function executes ONCE for all concurrent calls with same key
		return nil, o.doDelivery(ctx, notification, channel)
	})

	// Log if this was a deduplicated call (shared = true means we waited for another goroutine)
	if shared {
		o.logger.Info("DD-NOT-008: Concurrent delivery deduplicated (prevented duplicate attempt)",
			"notification", notification.Name,
			"channel", channel,
			"uid", notification.UID)
	}

	_ = result // result is always nil in our case
	return err
}

// doDelivery performs the actual delivery (called by singleflight)
// DD-NOT-008: Tracks successful deliveries to prevent duplicate deliveries
// In-flight counter is managed by caller (DeliverToChannels) via reserve-then-check pattern
func (o *Orchestrator) doDelivery(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channel notificationv1alpha1.Channel,
) error {
	// DD-NOT-008: Final guard against sequential duplicate delivery.
	// Singleflight deduplicates concurrent calls, but if a second call arrives
	// after the first completed (key removed from group), it starts a new execution.
	// This check catches that case by consulting the in-memory success tracker.
	key := fmt.Sprintf("%s:%s", notification.UID, channel)
	if _, already := o.successfulDeliveries.Load(key); already {
		o.logger.Info("DD-NOT-008: Delivery already succeeded, skipping sequential duplicate",
			"notification", notification.Name,
			"channel", string(channel))
		return nil
	}

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
	err := service.Deliver(ctx, sanitized)

	// DD-NOT-008: Track successful deliveries to prevent duplicate deliveries
	if err == nil {
		key := fmt.Sprintf("%s:%s", notification.UID, channel)
		o.successfulDeliveries.Store(key, true)
		o.logger.V(1).Info("DD-NOT-008: Marked channel as successfully delivered (in-memory)",
			"notification", notification.Name,
			"channel", channel)
	}

	return err
}

// DD-NOT-007: Individual channel methods REMOVED
// All channels now use common DeliverToChannel() via registration pattern

// DD-NOT-008: In-Flight Attempt Tracking Methods
// Reserve-then-check pattern: callers increment in-flight BEFORE the
// max-attempts gate, so concurrent reconciles see each other's reservations.

// GetTotalAttemptCount returns the total number of attempts for a channel,
// including both persisted attempts (in status) and in-flight attempts.
// This is the method controllers should use for exhaustion checks.
func (o *Orchestrator) GetTotalAttemptCount(
	notification *notificationv1alpha1.NotificationRequest,
	channel string,
	persistedCount int,
) int {
	key := fmt.Sprintf("%s:%s", notification.UID, channel)
	inFlightVal, exists := o.inFlightAttempts.Load(key)
	if !exists {
		return persistedCount
	}

	inFlight, ok := inFlightVal.(int)
	if !ok {
		return persistedCount
	}

	total := persistedCount + inFlight
	o.logger.V(1).Info("DD-NOT-008: Total attempt count calculated",
		"notification", notification.Name,
		"channel", channel,
		"persisted", persistedCount,
		"inFlight", inFlight,
		"total", total)

	return total
}

// HasChannelSucceeded checks if a channel has succeeded (either persisted or in-memory).
// DD-NOT-008: Checks both persisted status and in-memory tracking to prevent duplicate deliveries.
func (o *Orchestrator) HasChannelSucceeded(
	notification *notificationv1alpha1.NotificationRequest,
	channel string,
	persistedSuccess bool,
) bool {
	// If already persisted in status, return true
	if persistedSuccess {
		return true
	}

	// Check in-memory success tracking
	key := fmt.Sprintf("%s:%s", notification.UID, channel)
	_, exists := o.successfulDeliveries.Load(key)

	if exists {
		o.logger.V(1).Info("DD-NOT-008: Channel has in-memory success (not yet persisted)",
			"notification", notification.Name,
			"channel", channel)
	}

	return exists
}

// incrementInFlightAttempts increments the in-flight counter for a channel.
// Called BEFORE the max-attempts gate (reserve-then-check TOCTOU fix).
func (o *Orchestrator) incrementInFlightAttempts(uid string, channel string) {
	key := fmt.Sprintf("%s:%s", uid, channel)

	// Atomic increment using CompareAndSwap loop
	for {
		currentVal, _ := o.inFlightAttempts.LoadOrStore(key, 0)
		current := currentVal.(int)
		if o.inFlightAttempts.CompareAndSwap(key, current, current+1) {
			o.logger.V(1).Info("DD-NOT-008: Incremented in-flight counter",
				"key", key,
				"newCount", current+1)
			break
		}
	}
}

// decrementInFlightAttempts decrements the in-flight counter for a channel.
// Called when delivery attempt completes (after service.Deliver returns).
func (o *Orchestrator) decrementInFlightAttempts(uid string, channel string) {
	key := fmt.Sprintf("%s:%s", uid, channel)

	// Atomic decrement using CompareAndSwap loop
	for {
		currentVal, exists := o.inFlightAttempts.Load(key)
		if !exists {
			o.logger.Error(nil, "DD-NOT-008: Attempted to decrement non-existent in-flight counter", "key", key)
			return
		}

		current := currentVal.(int)
		if current <= 0 {
			o.logger.Error(nil, "DD-NOT-008: Attempted to decrement in-flight counter below 0", "key", key, "current", current)
			return
		}

		newCount := current - 1
		if o.inFlightAttempts.CompareAndSwap(key, current, newCount) {
			o.logger.V(1).Info("DD-NOT-008: Decremented in-flight counter",
				"key", key,
				"newCount", newCount)

			// Clean up if counter reaches 0
			if newCount == 0 {
				o.inFlightAttempts.Delete(key)
			}
			break
		}
	}
}

// getDeliveryMutex returns a per-notification mutex. Concurrent reconciles for
// the same notification are serialized, eliminating the TOCTOU window between
// in-flight decrement and status persistence (DD-NOT-008 v2).
func (o *Orchestrator) getDeliveryMutex(uid string) *sync.Mutex {
	mu, _ := o.deliveryMu.LoadOrStore(uid, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// ClearInMemoryState clears all in-memory tracking for a notification.
// DD-NOT-008: Called after status is persisted to clean up in-memory state.
// This is critical for test isolation in parallel execution.
func (o *Orchestrator) ClearInMemoryState(uid string) {
	// Clear in-flight attempts for all channels
	o.inFlightAttempts.Range(func(key, value interface{}) bool {
		keyStr := key.(string)
		if len(keyStr) > len(uid) && keyStr[:len(uid)] == uid && keyStr[len(uid)] == ':' {
			o.inFlightAttempts.Delete(key)
			o.logger.V(1).Info("DD-NOT-008: Cleared in-flight attempts",
				"key", keyStr)
		}
		return true
	})

	// Clear successful deliveries for all channels
	o.successfulDeliveries.Range(func(key, value interface{}) bool {
		keyStr := key.(string)
		if len(keyStr) > len(uid) && keyStr[:len(uid)] == uid && keyStr[len(uid)] == ':' {
			o.successfulDeliveries.Delete(key)
			o.logger.V(1).Info("DD-NOT-008: Cleared successful delivery tracking",
				"key", keyStr)
		}
		return true
	})

	// Clear per-notification delivery mutex
	o.deliveryMu.Delete(uid)
}

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

	// Get current attempt count for this channel (before adding new attempt)
	currentAttemptCount := getChannelAttemptCount(notification, string(channel))

	// NT-BUG-002 Refinement: Prevent duplicate recording during rapid reconciliations
	now := metav1.Now()
	if o.isDuplicateDeliveryAttempt(notification, channel, deliveryErr, currentAttemptCount, now, log) {
		return nil
	}

	// Create delivery attempt record
	// BR-NOT-051: Record attempt number for audit trail
	attempt := notificationv1alpha1.DeliveryAttempt{
		Channel:   notificationv1alpha1.DeliveryChannelName(channel),
		Attempt:   currentAttemptCount + 1, // 1-based attempt number
		Timestamp: now,
	}

	var err error
	if deliveryErr != nil {
		err = o.applyFailedAttemptOutcome(ctx, notification, channel, deliveryErr, &attempt, auditMessageFailed, log)
	} else {
		err = o.applySuccessfulAttemptOutcome(ctx, notification, channel, &attempt, auditMessageSent, log)
	}
	if err != nil {
		return err
	}

	// BR-NOT-053: Use Status Manager to record delivery attempt (Pattern 2)
	if err := o.statusManager.RecordDeliveryAttempt(ctx, notification, attempt); err != nil {
		log.Info("Failed to update status after channel delivery (non-fatal, will retry at end)", "error", err)
	}

	return nil
}

// isDuplicateDeliveryAttempt implements NT-BUG-002: it finds the most recent
// recorded attempt for this channel and reports true only when it is a true
// duplicate of the in-flight attempt (same status/error, recorded < 500ms
// ago), meaning this call should be skipped entirely. Extracted from
// RecordDeliveryAttempt (Wave 6 6b GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (o *Orchestrator) isDuplicateDeliveryAttempt(notification *notificationv1alpha1.NotificationRequest, channel notificationv1alpha1.Channel, deliveryErr error, currentAttemptCount int, now metav1.Time, log logr.Logger) bool {
	currentStatus := notificationv1alpha1.DeliveryAttemptStatusSuccess
	currentError := ""
	if deliveryErr != nil {
		currentStatus = notificationv1alpha1.DeliveryAttemptStatusFailed
		currentError = deliveryErr.Error()
	}

	var mostRecentAttempt *notificationv1alpha1.DeliveryAttempt
	for i := len(notification.Status.DeliveryAttempts) - 1; i >= 0; i-- {
		if notification.Status.DeliveryAttempts[i].Channel == notificationv1alpha1.DeliveryChannelName(channel) {
			mostRecentAttempt = &notification.Status.DeliveryAttempts[i]
			break
		}
	}
	if mostRecentAttempt == nil || currentAttemptCount == 0 {
		return false
	}

	timeSinceAttempt := now.Sub(mostRecentAttempt.Timestamp.Time)
	isDuplicate := timeSinceAttempt < 500*time.Millisecond &&
		mostRecentAttempt.Status == currentStatus &&
		mostRecentAttempt.Error == currentError
	if isDuplicate {
		log.V(1).Info("Delivery attempt already recorded (exact duplicate), skipping",
			"status", currentStatus,
			"timeSince", timeSinceAttempt,
			"attemptCount", currentAttemptCount)
	}
	return isDuplicate
}

// applyFailedAttemptOutcome finalizes a failed status-recorded delivery
// attempt: BR-NOT-055 permanent-vs-retryable classification, failure
// counters, the mandatory message.failed audit event (ADR-032 §1), and
// failure metrics. Extracted from RecordDeliveryAttempt (Wave 6 6b GREEN:
// funlen remediation) — pure code motion, no behavior change.
func (o *Orchestrator) applyFailedAttemptOutcome(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel notificationv1alpha1.Channel, deliveryErr error, attempt *notificationv1alpha1.DeliveryAttempt, auditMessageFailed func(context.Context, *notificationv1alpha1.NotificationRequest, string, error) error, log logr.Logger) error {
	attempt.Status = notificationv1alpha1.DeliveryAttemptStatusFailed
	attempt.Error = deliveryErr.Error()
	notification.Status.FailedDeliveries++

	// BR-NOT-055: Permanent Error Classification
	if !IsRetryableError(deliveryErr) {
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
	return nil
}

// applySuccessfulAttemptOutcome finalizes a successful status-recorded
// delivery attempt: success counters, the mandatory message.sent audit event
// (ADR-032 §1), success/duration metrics, and best-effort E2E file delivery
// (DD-NOT-002 V3.0). Extracted from RecordDeliveryAttempt (Wave 6 6b GREEN:
// funlen remediation) — pure code motion, no behavior change.
func (o *Orchestrator) applySuccessfulAttemptOutcome(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel notificationv1alpha1.Channel, attempt *notificationv1alpha1.DeliveryAttempt, auditMessageSent func(context.Context, *notificationv1alpha1.NotificationRequest, string) error, log logr.Logger) error {
	attempt.Status = notificationv1alpha1.DeliveryAttemptStatusSuccess
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
	return nil
}
