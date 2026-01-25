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
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	kubernautnotif "github.com/jordigilh/kubernaut/pkg/notification"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationphase "github.com/jordigilh/kubernaut/pkg/notification/phase"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// NotificationRequestReconciler reconciles a NotificationRequest object
type NotificationRequestReconciler struct {
	client.Client
	APIReader client.Reader // DD-STATUS-001: Cache-bypassed reader for critical status checks
	Scheme    *runtime.Scheme

	// Delivery services
	ConsoleService *delivery.ConsoleDeliveryService
	SlackService   *delivery.SlackDeliveryService
	FileService    *delivery.FileDeliveryService // E2E testing only (DD-NOT-002)

	// ========================================
	// DELIVERY ORCHESTRATOR (Pattern 3 - P0)
	// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §3
	// ========================================
	//
	// DeliveryOrchestrator manages notification delivery across channels
	// Extracted from controller to improve testability and maintainability
	//
	// BENEFITS:
	// - ~217 lines extracted from controller
	// - Delivery logic isolated and testable
	// - Single responsibility principle
	//
	// WIRED IN: cmd/notification/main.go
	// USAGE: r.DeliveryOrchestrator.DeliverToChannels(...)
	//
	// ========================================
	// INTERFACE-BASED SERVICES PATTERN (P2)
	// ========================================
	// The orchestrator implements the Interface-Based Services pattern:
	//   - Interface: delivery.DeliveryService (pkg/notification/delivery/interface.go)
	//   - Registry: map[string]DeliveryService (orchestrator.channels)
	//   - Registration: orchestrator.RegisterChannel(name, service)
	//
	// All delivery channels (Slack, Console, File, Log, etc.) implement DeliveryService
	// and register dynamically via RegisterChannel() for pluggable architecture.
	//
	// See: docs/architecture/decisions/DD-NOT-007-DELIVERY-ORCHESTRATOR-REGISTRATION-PATTERN.md
	// ========================================
	DeliveryOrchestrator *delivery.Orchestrator

	// Data sanitization
	Sanitizer *sanitization.Sanitizer

	// v3.1: Circuit breaker for graceful degradation (Category B)
	// Migrated to github.com/sony/gobreaker via shared Manager wrapper
	// Provides per-channel isolation (Slack, console, webhooks)
	CircuitBreaker *circuitbreaker.Manager

	// v1.1: Audit integration for unified audit table (ADR-034)
	// BR-NOT-062: Unified Audit Table Integration
	// BR-NOT-063: Graceful Audit Degradation
	// See: DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md
	AuditStore   audit.AuditStore           // Buffered store for async audit writes (fire-and-forget)
	AuditManager *notificationaudit.Manager // Audit event manager (DD-AUDIT-002)

	// BR-NOT-065: Channel Routing Based on Labels
	// BR-NOT-067: Routing Configuration Hot-Reload
	// Thread-safe router with hot-reload support from ConfigMap
	// See: DD-WE-004 (skip-reason routing)
	Router *routing.Router

	// ========================================
	// METRICS RECORDER (DD-METRICS-001)
	// 📋 Design Decision: DD-METRICS-001 | ✅ Dependency Injection Pattern
	// See: docs/architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md
	// ========================================
	//
	// Metrics recorder for observability (DD-005 compliant)
	// Dependency-injected to enable testing and isolation
	//
	// MANDATORY: DD-METRICS-001 requires metrics to be dependency-injected
	// RATIONALE: Enables test isolation, prevents global state pollution
	//
	// WIRED IN: cmd/notification/main.go
	// USED IN: All reconciliation methods that emit metrics
	Metrics notificationmetrics.Recorder

	// ========================================
	// EVENT RECORDER (K8s Events for Debugging)
	// See: SERVICE_MATURITY_REQUIREMENTS.md v1.1.0 (P1 - Should Have)
	// See: docs/development/business-requirements/TESTING_GUIDELINES.md §1312-1357
	// ========================================
	//
	// EventRecorder for emitting Kubernetes Events
	// Used for operational debugging and troubleshooting
	//
	// WIRED IN: cmd/notification/main.go
	// EVENTS EMITTED: ReconcileStarted, PhaseTransition, ReconcileComplete, ReconcileFailed
	Recorder record.EventRecorder

	// ========================================
	// STATUS MANAGER (Pattern 2 - P1 Quick Win)
	// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §4
	// ========================================
	//
	// StatusManager handles all status updates with retry logic
	// Replaces controller's custom updateStatusWithRetry() method
	//
	// BENEFITS:
	// - Centralized status update logic (~100 lines saved)
	// - Consistent retry patterns across all status updates
	// - Better testability and separation of concerns
	//
	// WIRED IN: cmd/notification/main.go
	// USAGE: r.StatusManager.UpdatePhase(), r.StatusManager.RecordDeliveryAttempt()
	StatusManager *notificationstatus.Manager

	// NT-BUG-001 Fix: Idempotency tracking for audit events
	// Prevents duplicate audit event emission across multiple reconciles
	// Key: notification UID, Value: map[eventType]bool
	// Cleaned up on notification deletion
	emittedAuditEvents sync.Map
}

//+kubebuilder:rbac:groups=kubernaut.ai,resources=notificationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=notificationrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=notificationrequests/finalizers,verbs=update
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

	// Emit ReconcileStarted event (P1: EventRecorder)
	r.Recorder.Event(notification, corev1.EventTypeNormal, "ReconcileStarted",
		fmt.Sprintf("Started reconciling notification %s", notification.Name))

	// DEBUG: Log reconcile start with current state
	log.Info("🔍 RECONCILE START DEBUG",
		"name", notification.Name,
		"generation", notification.Generation,
		"observedGeneration", notification.Status.ObservedGeneration,
		"phase", notification.Status.Phase,
		"successfulDeliveries", notification.Status.SuccessfulDeliveries,
		"failedDeliveries", notification.Status.FailedDeliveries,
		"totalAttempts", notification.Status.TotalAttempts,
		"deliveryAttemptCount", len(notification.Status.DeliveryAttempts))

	// NT-BUG-008: Prevent duplicate reconciliations from processing same generation twice
	// Bug: Status updates (Pending→Sending) trigger immediate reconciles that race with original reconcile
	// Symptom: 2x audit events per notification (discovered in E2E test 02_audit_correlation_test.go)
	// Fix: Skip reconcile if this generation was already processed (has delivery attempts) AND in terminal phase
	// CRITICAL: Must allow reconciliation for non-terminal phases (e.g., Sending → Failed transition)
	if notification.Generation == notification.Status.ObservedGeneration &&
		len(notification.Status.DeliveryAttempts) > 0 &&
		notificationphase.IsTerminal(notification.Status.Phase) {
		log.Info("✅ DUPLICATE RECONCILE PREVENTED: Generation already processed",
			"generation", notification.Generation,
			"observedGeneration", notification.Status.ObservedGeneration,
			"deliveryAttempts", len(notification.Status.DeliveryAttempts),
			"phase", notification.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Phase 1: Initialize status if first reconciliation
	initialized, err := r.handleInitialization(ctx, notification)
	if err != nil {
		return ctrl.Result{}, err
	}
	if initialized {
		return ctrl.Result{Requeue: true}, nil
	}

	// Phase 2: Check if already in terminal state
	// ========================================
	// TERMINAL STATE LOGIC (P1 PATTERN)
	// 📋 Refactoring: Controller Refactoring Pattern Library §2
	// Using: pkg/notification/phase.IsTerminal()
	// ========================================
	log.Info("🔍 TERMINAL CHECK #1 DEBUG",
		"phase", notification.Status.Phase,
		"isTerminal", notificationphase.IsTerminal(notification.Status.Phase),
		"successfulDeliveries", notification.Status.SuccessfulDeliveries,
		"failedDeliveries", notification.Status.FailedDeliveries)
	if notificationphase.IsTerminal(notification.Status.Phase) {
		log.Info("❌ EXITING: NotificationRequest in terminal state, skipping reconciliation",
			"phase", notification.Status.Phase)
		return ctrl.Result{}, nil
	}

	// NT-BUG-007: Backoff enforcement for Retrying phase
	// Problem: Status updates trigger immediate reconciles, bypassing RequeueAfter backoff
	// Solution: Check if enough time has elapsed since last attempt before retrying
	if notification.Status.Phase == notificationv1alpha1.NotificationPhaseRetrying &&
		len(notification.Status.DeliveryAttempts) > 0 {

		// Find the most recent failed delivery attempt
		var lastFailedAttempt *notificationv1alpha1.DeliveryAttempt
		for i := len(notification.Status.DeliveryAttempts) - 1; i >= 0; i-- {
			attempt := &notification.Status.DeliveryAttempts[i]
			if attempt.Status == "failed" {
				lastFailedAttempt = attempt
				break
			}
		}

		if lastFailedAttempt != nil {
			// Calculate expected next retry time
			attemptCount := lastFailedAttempt.Attempt
			nextBackoff := r.calculateBackoffWithPolicy(notification, attemptCount)
			nextRetryTime := lastFailedAttempt.Timestamp.Time.Add(nextBackoff)
			now := time.Now()

			if now.Before(nextRetryTime) {
				remainingBackoff := nextRetryTime.Sub(now)
				log.Info("⏸️ BACKOFF ENFORCEMENT: Too early to retry, requeueing",
					"attemptNumber", attemptCount,
					"lastAttemptTime", lastFailedAttempt.Timestamp.Time.Format(time.RFC3339),
					"nextRetryTime", nextRetryTime.Format(time.RFC3339),
					"remainingBackoff", remainingBackoff,
					"channel", lastFailedAttempt.Channel)
				return ctrl.Result{RequeueAfter: remainingBackoff}, nil
			}

			log.Info("✅ BACKOFF ELAPSED: Ready to retry",
				"attemptNumber", attemptCount,
				"lastAttemptTime", lastFailedAttempt.Timestamp.Time.Format(time.RFC3339),
				"elapsedSinceLastAttempt", now.Sub(lastFailedAttempt.Timestamp.Time),
				"expectedBackoff", nextBackoff)
		}
	}

	// Phase 3: Transition from Pending to Sending
	if err := r.handlePendingToSendingTransition(ctx, notification); err != nil {
		return ctrl.Result{}, err
	}

	// BR-NOT-053: ALWAYS re-read before delivery to check if another reconcile completed
	// CRITICAL: Prevents duplicate delivery - must be OUTSIDE the Pending check
	if err := r.Get(ctx, req.NamespacedName, notification); err != nil {
		log.Error(err, "Failed to refresh notification before delivery")
		return ctrl.Result{}, err
	}

	// Check if another reconcile completed while we were updating phase
	// Using phase.IsTerminal() - replaces duplicate terminal state check (P1 pattern)
	if notificationphase.IsTerminal(notification.Status.Phase) {
		log.Info("NotificationRequest completed by concurrent reconcile, skipping duplicate delivery",
			"phase", notification.Status.Phase)
		return ctrl.Result{}, nil
	}

	// BR-NOT-053: CRITICAL - Re-read notification RIGHT BEFORE delivery loop
	// DD-STATUS-001: Use APIReader (cache-bypassed) to prevent duplicate deliveries
	// during rapid reconciles from stale cached status reads
	if err := r.APIReader.Get(ctx, req.NamespacedName, notification); err != nil {
		log.Error(err, "Failed to refresh notification before channel delivery loop")
		return ctrl.Result{}, err
	}

	// Double-check phase after re-read
	// Using phase.IsTerminal() - replaces duplicate terminal state check (P1 pattern)
	if notificationphase.IsTerminal(notification.Status.Phase) {
		log.Info("NotificationRequest just completed, skipping duplicate delivery after re-read",
			"phase", notification.Status.Phase)
		return ctrl.Result{}, nil
	}

	// BR-NOT-065: Resolve channels from routing rules if spec.channels is empty
	// BR-NOT-069: Set RoutingResolved condition for visibility
	channels := notification.Spec.Channels
	if len(channels) == 0 {
		channels, routingMessage := r.resolveChannelsFromRoutingWithDetails(ctx, notification)
		log.Info("Resolved channels from routing rules",
			"notification", notification.Name,
			"channels", channels,
			"labels", notification.Labels)

		// BR-NOT-069: Set RoutingResolved condition after routing resolution
		kubernautnotif.SetRoutingResolved(
			notification,
			metav1.ConditionTrue,
			kubernautnotif.ReasonRoutingRuleMatched,
			routingMessage,
		)
	}

	// Phase 4: Process delivery loop
	result, err := r.handleDeliveryLoop(ctx, notification)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Phase 5: Determine phase transition based on delivery results
	// NOTE: Delivery attempts are recorded atomically during phase transitions
	// (DD-PERF-001: Atomic Status Updates - prevents double-counting bug)
	return r.determinePhaseTransition(ctx, notification, result)
}

// deliverToConsole delivers notification to console (stdout)
func (r *NotificationRequestReconciler) deliverToConsole(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error { //nolint:unused
	if r.ConsoleService == nil {
		return fmt.Errorf("console service not initialized")
	}

	// Sanitize notification content before delivery
	sanitizedNotification := r.sanitizeNotification(notification)
	return r.ConsoleService.Deliver(ctx, sanitizedNotification)
}

// deliverToSlack delivers notification to Slack webhook
func (r *NotificationRequestReconciler) deliverToSlack(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error { //nolint:unused
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
func (r *NotificationRequestReconciler) sanitizeNotification(notification *notificationv1alpha1.NotificationRequest) *notificationv1alpha1.NotificationRequest { //nolint:unused
	// Create a shallow copy to avoid mutating the original
	sanitized := notification.DeepCopy()

	// Sanitize subject and body if sanitizer is configured
	if r.Sanitizer != nil {
		sanitized.Spec.Subject = r.Sanitizer.Sanitize(sanitized.Spec.Subject)
		sanitized.Spec.Body = r.Sanitizer.Sanitize(sanitized.Spec.Body)
	}

	return sanitized
}

// handleNotFound handles Category A: NotificationRequest Not Found
// When: CRD deleted during reconciliation
// Action: Log deletion, remove from retry queue
// Recovery: Normal (no action needed)
func (r *NotificationRequestReconciler) handleNotFound(ctx context.Context, name string) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("NotificationRequest not found, likely deleted", "name", name)

	// NT-BUG-001 Fix: Cleanup audit event tracking to prevent memory leaks
	r.cleanupAuditEventTracking(name)
	log.V(1).Info("Cleaned up audit event tracking for deleted notification", "name", name)

	// Remove from retry queue if applicable (controller-runtime handles this automatically)
	return ctrl.Result{}, nil
}

// ========================================
// REMOVED: updateStatusWithRetry() (Pattern 2 Migration)
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §4
// ========================================
//
// This method has been REPLACED by pkg/notification/status/Manager
//
// BEFORE (custom controller method):
// - func (r *Reconciler) updateStatusWithRetry(...) error { ... }
// - 26 lines of retry logic
// - Scattered across 7 locations in controller
//
// AFTER (centralized Status Manager):
// - r.StatusManager.UpdatePhase(ctx, notification, phase, reason, message)
// - r.StatusManager.RecordDeliveryAttempt(ctx, notification, attempt)
// - r.StatusManager.UpdateObservedGeneration(ctx, notification)
//
// BENEFITS:
// - ~100 lines saved in controller
// - Consistent retry patterns
// - Better testability
// - Single source of truth for status updates
//
// See Pattern 2 commit for full migration details.
// ========================================

// ========================================
// AUDIT INTEGRATION HELPERS (v1.1)
// BR-NOT-062: Unified Audit Table Integration
// BR-NOT-063: Graceful Audit Degradation (fire-and-forget, non-blocking)
// See: DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md
// ========================================

// auditMessageSent audits successful message delivery
//
// BR-NOT-062: Unified audit table integration
// ADR-032 §1: Audit is MANDATORY - no graceful degradation allowed
//
// This method returns error per ADR-032 §1. Audit write failures (StoreAudit errors)
// are still fire-and-forget per BR-NOT-063, but nil store is a CRITICAL error.
func (r *NotificationRequestReconciler) auditMessageSent(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string) error {
	// ADR-032 §1: Audit is MANDATORY - no graceful degradation allowed
	// If audit store is nil, this indicates misconfiguration and MUST fail
	if r.AuditStore == nil || r.AuditManager == nil {
		err := fmt.Errorf("audit store or helpers nil - audit is MANDATORY per ADR-032 §1")
		log := log.FromContext(ctx)
		log.Error(err, "CRITICAL: Cannot record audit event", "event_type", "message.sent", "channel", channel)
		return err
	}

	log := log.FromContext(ctx)

	// NT-BUG-001 Fix: Check if this audit event was already emitted
	notificationKey := fmt.Sprintf("%s/%s", notification.Namespace, notification.Name)
	eventKey := fmt.Sprintf("message.sent:%s", channel)
	if !r.shouldEmitAuditEvent(notificationKey, eventKey) {
		log.V(1).Info("Audit event already emitted, skipping duplicate", "event_type", "message.sent", "channel", channel)
		return nil
	}

	// Create audit event
	event, err := r.AuditManager.CreateMessageSentEvent(notification, channel)
	if err != nil {
		log.Error(err, "Failed to create audit event - audit creation is MANDATORY per ADR-032 §1", "event_type", "message.sent", "channel", channel)
		return fmt.Errorf("failed to create audit event (ADR-032 §1): %w", err)
	}

	// Fire-and-forget: Audit write failures don't block reconciliation (BR-NOT-063)
	// ADR-032 §1: Store is available (checked above), write failure is acceptable (async buffered write)
	// This does NOT violate ADR-032 because store is initialized, just the write failed
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.sent", "channel", channel)
		// Continue reconciliation - audit failure is not critical to notification delivery (BR-NOT-063)
	} else {
		// NT-BUG-001 Fix: Mark event as emitted only if store succeeded
		r.markAuditEventEmitted(notificationKey, eventKey)
	}

	return nil
}

// auditMessageFailed audits failed message delivery
//
// BR-NOT-062: Unified audit table integration
// ADR-032 §1: Audit is MANDATORY - no graceful degradation allowed
//
// This method returns error per ADR-032 §1. Audit write failures (StoreAudit errors)
// are still fire-and-forget per BR-NOT-063, but nil store is a CRITICAL error.
func (r *NotificationRequestReconciler) auditMessageFailed(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string, deliveryErr error) error {
	// ADR-032 §1: Audit is MANDATORY - no graceful degradation allowed
	// If audit store is nil, this indicates misconfiguration and MUST fail
	if r.AuditStore == nil || r.AuditManager == nil {
		err := fmt.Errorf("audit store or helpers nil - audit is MANDATORY per ADR-032 §1")
		log := log.FromContext(ctx)
		log.Error(err, "CRITICAL: Cannot record audit event", "event_type", "message.failed", "channel", channel)
		return err
	}

	log := log.FromContext(ctx)

	// NT-BUG-001 Fix: Check if this audit event was already emitted
	notificationKey := fmt.Sprintf("%s/%s", notification.Namespace, notification.Name)
	// Note: We include attempt count in key to allow multiple failure events during retries
	eventKey := fmt.Sprintf("message.failed:%s:attempt%d", channel, notification.Status.TotalAttempts)
	if !r.shouldEmitAuditEvent(notificationKey, eventKey) {
		log.V(1).Info("Audit event already emitted, skipping duplicate", "event_type", "message.failed", "channel", channel)
		return nil
	}

	// Create audit event with error details
	event, err := r.AuditManager.CreateMessageFailedEvent(notification, channel, deliveryErr)
	if err != nil {
		log.Error(err, "Failed to create audit event - audit creation is MANDATORY per ADR-032 §1", "event_type", "message.failed", "channel", channel)
		return fmt.Errorf("failed to create audit event (ADR-032 §1): %w", err)
	}

	// Fire-and-forget: Audit write failures don't block reconciliation (BR-NOT-063)
	// ADR-032 §1: Store is available (checked above), write failure is acceptable (async buffered write)
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.failed", "channel", channel)
		// Continue reconciliation - audit failure is not critical to notification delivery (BR-NOT-063)
	} else {
		// NT-BUG-001 Fix: Mark event as emitted only if store succeeded
		r.markAuditEventEmitted(notificationKey, eventKey)
	}

	return nil
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
func (r *NotificationRequestReconciler) auditMessageAcknowledged(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	// ADR-032 §1: Audit is MANDATORY - no graceful degradation allowed
	// If audit store is nil, this indicates misconfiguration and MUST fail
	if r.AuditStore == nil || r.AuditManager == nil {
		err := fmt.Errorf("audit store or helpers nil - audit is MANDATORY per ADR-032 §1")
		log := log.FromContext(ctx)
		log.Error(err, "CRITICAL: Cannot record audit event", "event_type", "message.acknowledged")
		return err
	}

	log := log.FromContext(ctx)

	// NT-BUG-001 Fix: Check if this audit event was already emitted
	notificationKey := fmt.Sprintf("%s/%s", notification.Namespace, notification.Name)
	eventKey := "message.acknowledged"
	if !r.shouldEmitAuditEvent(notificationKey, eventKey) {
		log.V(1).Info("Audit event already emitted, skipping duplicate", "event_type", "message.acknowledged")
		return nil
	}

	// Create audit event
	event, err := r.AuditManager.CreateMessageAcknowledgedEvent(notification)
	if err != nil {
		log.Error(err, "Failed to create audit event - audit creation is MANDATORY per ADR-032 §1", "event_type", "message.acknowledged")
		return fmt.Errorf("failed to create audit event (ADR-032 §1): %w", err)
	}

	// Fire-and-forget: Audit write failures don't block reconciliation (BR-NOT-063)
	// ADR-032 §1: Store is available (checked above), write failure is acceptable (async buffered write)
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.acknowledged")
		// Continue reconciliation - audit failure is not critical (BR-NOT-063)
	} else {
		// NT-BUG-001 Fix: Mark event as emitted only if store succeeded
		r.markAuditEventEmitted(notificationKey, eventKey)
	}

	return nil
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
func (r *NotificationRequestReconciler) auditMessageEscalated(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	// ADR-032 §1: Audit is MANDATORY - no graceful degradation allowed
	// If audit store is nil, this indicates misconfiguration and MUST fail
	if r.AuditStore == nil || r.AuditManager == nil {
		err := fmt.Errorf("audit store or helpers nil - audit is MANDATORY per ADR-032 §1")
		log := log.FromContext(ctx)
		log.Error(err, "CRITICAL: Cannot record audit event", "event_type", "message.escalated")
		return err
	}

	log := log.FromContext(ctx)

	// NT-BUG-001 Fix: Check if this audit event was already emitted
	notificationKey := fmt.Sprintf("%s/%s", notification.Namespace, notification.Name)
	eventKey := "message.escalated"
	if !r.shouldEmitAuditEvent(notificationKey, eventKey) {
		log.V(1).Info("Audit event already emitted, skipping duplicate", "event_type", "message.escalated")
		return nil
	}

	// Create audit event
	event, err := r.AuditManager.CreateMessageEscalatedEvent(notification)
	if err != nil {
		log.Error(err, "Failed to create audit event - audit creation is MANDATORY per ADR-032 §1", "event_type", "message.escalated")
		return fmt.Errorf("failed to create audit event (ADR-032 §1): %w", err)
	}

	// Fire-and-forget: Audit write failures don't block reconciliation (BR-NOT-063)
	// ADR-032 §1: Store is available (checked above), write failure is acceptable (async buffered write)
	if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
		log.Error(err, "Failed to buffer audit event", "event_type", "message.escalated")
		// Continue reconciliation - audit failure is not critical (BR-NOT-063)
	} else {
		// NT-BUG-001 Fix: Mark event as emitted only if store succeeded
		r.markAuditEventEmitted(notificationKey, eventKey)
	}

	return nil
}

// =============================================================================
// Exported Methods for Testing (ADR-032 Compliance Tests)
// =============================================================================
// These methods expose private audit functions for unit testing ADR-032 §1 compliance.
// They should ONLY be used in test files (test/unit/notification/audit_adr032_compliance_test.go).

// ExportedAuditMessageSent exposes auditMessageSent for ADR-032 compliance testing
func (r *NotificationRequestReconciler) ExportedAuditMessageSent(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string) error {
	return r.auditMessageSent(ctx, notification, channel)
}

// ExportedAuditMessageFailed exposes auditMessageFailed for ADR-032 compliance testing
func (r *NotificationRequestReconciler) ExportedAuditMessageFailed(ctx context.Context, notification *notificationv1alpha1.NotificationRequest, channel string, deliveryErr error) error {
	return r.auditMessageFailed(ctx, notification, channel, deliveryErr)
}

// ExportedAuditMessageAcknowledged exposes auditMessageAcknowledged for ADR-032 compliance testing
func (r *NotificationRequestReconciler) ExportedAuditMessageAcknowledged(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	return r.auditMessageAcknowledged(ctx, notification)
}

// ExportedAuditMessageEscalated exposes auditMessageEscalated for ADR-032 compliance testing
func (r *NotificationRequestReconciler) ExportedAuditMessageEscalated(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	return r.auditMessageEscalated(ctx, notification)
}

// ========================================
// AUDIT EVENT IDEMPOTENCY (NT-BUG-001 Fix)
// ========================================
//
// Prevents duplicate audit event emission across multiple reconciles.
// Each notification can emit each event type exactly once.
//
// WHY IDEMPOTENCY?
// - ✅ Accurate audit trail (no 3x duplication)
// - ✅ Correct compliance reporting
// - ✅ Reduced database bloat
//
// NOTE: Uses namespace/name as key (not UID) since UID is unavailable after deletion.
// ========================================

// shouldEmitAuditEvent checks if audit event should be emitted for this notification.
// Returns true if event has NOT been emitted yet for this notification+eventType combination.
func (r *NotificationRequestReconciler) shouldEmitAuditEvent(notificationKey string, eventType string) bool {
	// Load existing events for this notification
	events, exists := r.emittedAuditEvents.Load(notificationKey)
	if !exists {
		return true // No events emitted yet
	}

	// Check if this specific event type was emitted
	if emittedMap, ok := events.(map[string]bool); ok {
		return !emittedMap[eventType]
	}

	return true // Default to allowing emission if type assertion fails
}

// markAuditEventEmitted records that audit event was emitted for this notification.
// Ensures subsequent reconciles won't emit the same event again.
func (r *NotificationRequestReconciler) markAuditEventEmitted(notificationKey string, eventType string) {
	// Load or create event map for this notification
	events, _ := r.emittedAuditEvents.LoadOrStore(notificationKey, make(map[string]bool))

	// Mark this event type as emitted
	if emittedMap, ok := events.(map[string]bool); ok {
		emittedMap[eventType] = true
		r.emittedAuditEvents.Store(notificationKey, emittedMap)
	}
}

// cleanupAuditEventTracking removes tracking for deleted notification.
// Called when notification is confirmed deleted to prevent memory leaks.
func (r *NotificationRequestReconciler) cleanupAuditEventTracking(notificationKey string) {
	r.emittedAuditEvents.Delete(notificationKey)
}

// countSuccessfulAttempts counts how many successful delivery attempts are in the list.
// Used for calculating accurate success counts when using atomic status updates.
func countSuccessfulAttempts(attempts []notificationv1alpha1.DeliveryAttempt) int {
	count := 0
	for _, attempt := range attempts {
		if attempt.Status == "success" {
			count++
		}
	}
	return count
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

// ========================================
// PHASE HANDLERS (P2 Refactoring - Complexity Reduction)
// ========================================
//
// These methods extract phase-specific logic from Reconcile to reduce
// cyclomatic complexity from 39 to ~10.
//
// Phase Transitions:
//   "" → Pending → Sending → (Sent | Failed)
//
// Each handler is responsible for a specific phase and returns
// the next action for the controller.
// ========================================

// handleInitialization initializes the NotificationRequest status if this is the first reconciliation.
// Returns true if initialization was performed (caller should requeue).
func (r *NotificationRequestReconciler) handleInitialization(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) (bool, error) {
	if notification.Status.Phase != "" {
		return false, nil // Already initialized
	}

	log := log.FromContext(ctx)

	// Initialize status fields
	notification.Status.DeliveryAttempts = []notificationv1alpha1.DeliveryAttempt{}
	notification.Status.TotalAttempts = 0
	notification.Status.SuccessfulDeliveries = 0
	notification.Status.FailedDeliveries = 0

	// Use Status Manager to update phase (Pattern 2)
	if err := r.StatusManager.UpdatePhase(
		ctx,
		notification,
		notificationv1alpha1.NotificationPhasePending,
		"Initialized",
		"Notification request received",
	); err != nil {
		log.Error(err, "Failed to initialize status")
		return false, err
	}

	// Record metric for notification request creation (BR-NOT-054: Observability)
	// DD-METRICS-001: Use injected metrics recorder
	r.Metrics.UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhasePending), 1)

	log.Info("NotificationRequest status initialized", "name", notification.Name)
	return true, nil // Requeue to process the initialized notification
}

// ========================================
// REMOVED: handleTerminalStateCheck() method (32 lines)
// 📋 Refactoring: Controller Refactoring Pattern Library §2 - Terminal State Logic (P1)
// Replaced with: pkg/notification/phase.IsTerminal()
// ========================================
//
// This method was removed as part of the Terminal State Logic refactoring pattern.
// All terminal state checks now use the centralized phase.IsTerminal() function.
//
// Benefits:
// - ✅ Single source of truth for terminal states
// - ✅ Consistent terminal state definition (Sent, PartiallySent, Failed)
// - ✅ Reduced code duplication (removed 4 duplicate checks)
// - ✅ Easier to maintain (add terminal phase once, applies everywhere)
//
// Migration:
// - Old: if r.handleTerminalStateCheck(ctx, notification) { ... }
// - New: if notificationphase.IsTerminal(notification.Status.Phase) { ... }
//
// See:
// - pkg/notification/phase/types.go - IsTerminal() implementation
// - test/unit/notification/phase/types_test.go - Unit tests
// - docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §2
// ========================================

// handlePendingToSendingTransition transitions notification from Pending to Sending phase.
func (r *NotificationRequestReconciler) handlePendingToSendingTransition(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) error {
	if notification.Status.Phase != notificationv1alpha1.NotificationPhasePending {
		return nil // Not in Pending phase
	}

	log := log.FromContext(ctx)

	// Use Status Manager to update phase to Sending (Pattern 2)
	if err := r.StatusManager.UpdatePhase(
		ctx,
		notification,
		notificationv1alpha1.NotificationPhaseSending,
		"ProcessingDeliveries",
		"Processing delivery channels",
	); err != nil {
		log.Error(err, "Failed to update phase to Sending")
		return err
	}

	// Emit PhaseTransition event (P1: EventRecorder)
	r.Recorder.Event(notification, corev1.EventTypeNormal, "PhaseTransition",
		fmt.Sprintf("Transitioned to %s phase", notificationv1alpha1.NotificationPhaseSending))

	// Record metric for phase transition to Sending (BR-NOT-054: Observability)
	// DD-METRICS-001: Use injected metrics recorder
	r.Metrics.UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhaseSending), 1)

	return nil
}

// deliveryLoopResult contains the results of the delivery loop.
type deliveryLoopResult struct {
	deliveryResults  map[string]error
	failureCount     int
	deliveryAttempts []notificationv1alpha1.DeliveryAttempt // Collected attempts for batch update
}

// handleDeliveryLoop processes delivery attempts for all channels.
// This is the core delivery logic extracted from Reconcile.
func (r *NotificationRequestReconciler) handleDeliveryLoop(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) (*deliveryLoopResult, error) {
	log := log.FromContext(ctx)

	// Get retry policy to check max attempts
	policy := r.getRetryPolicy(notification)

	// BR-NOT-065: Resolve channels from routing rules if spec.channels is empty
	// BR-NOT-069: Set RoutingResolved condition for visibility
	channels := notification.Spec.Channels
	if len(channels) == 0 {
		channels, routingMessage := r.resolveChannelsFromRoutingWithDetails(ctx, notification)
		log.Info("Resolved channels from routing rules",
			"notification", notification.Name,
			"channels", channels,
			"labels", notification.Labels)

		// BR-NOT-069: Set RoutingResolved condition after routing resolution
		kubernautnotif.SetRoutingResolved(
			notification,
			metav1.ConditionTrue,
			kubernautnotif.ReasonRoutingRuleMatched,
			routingMessage,
		)
	}

	// ========================================
	// DELEGATE TO ORCHESTRATOR (Pattern 3 - P0)
	// ========================================
	// Delivery orchestration extracted to pkg/notification/delivery/orchestrator.go
	// Controller provides callbacks for audit and helper methods
	orchestratorResult, err := r.DeliveryOrchestrator.DeliverToChannels(
		ctx,
		notification,
		channels,
		policy,
		// Callbacks for controller-specific logic
		r.channelAlreadySucceeded,
		r.hasChannelPermanentError,
		r.getChannelAttemptCount,
		r.auditMessageSent,
		r.auditMessageFailed,
	)
	if err != nil {
		return nil, err
	}

	// Convert orchestrator result to controller result format
	// 🔍 DEBUG: Track attempt count before status update
	log.Info("🔍 POST-DELIVERY DEBUG (handleDeliveryLoop)",
		"deliveryAttemptsFromOrchestrator", len(orchestratorResult.DeliveryAttempts),
		"statusDeliveryAttemptsBeforeUpdate", len(notification.Status.DeliveryAttempts),
		"channels", len(channels))

	return &deliveryLoopResult{
		deliveryResults:  orchestratorResult.DeliveryResults,
		failureCount:     orchestratorResult.FailureCount,
		deliveryAttempts: orchestratorResult.DeliveryAttempts, // Pass through for batch recording
	}, nil
}

// ========================================
// REMOVED: attemptChannelDelivery() (Pattern 3 Migration)
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §3
// ========================================
//
// This method has been REPLACED by pkg/notification/delivery/Orchestrator.DeliverToChannel()
//
// BEFORE (controller method):
// - func (r *Reconciler) attemptChannelDelivery(...) error { ... }
// - 14 lines switching on channel type
//
// AFTER (orchestrator method):
// - r.DeliveryOrchestrator.DeliverToChannel(ctx, notification, channel)
//
// See Pattern 3 commit for full migration details.
// ========================================

// ========================================
// REMOVED: recordDeliveryAttempt() (Pattern 3 Migration)
// See: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §3
// ========================================
//
// This method has been REPLACED by pkg/notification/delivery/Orchestrator.RecordDeliveryAttempt()
//
// BEFORE (controller method):
// - func (r *Reconciler) recordDeliveryAttempt(...) error { ... }
// - 124 lines of attempt recording, audit, metrics
//
// AFTER (orchestrator method):
// - Called internally by Orchestrator.DeliverToChannels()
//
// See Pattern 3 commit for full migration details.
// ========================================

// determinePhaseTransition determines the next phase based on delivery results.
func (r *NotificationRequestReconciler) determinePhaseTransition(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	result *deliveryLoopResult,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// NT-BUG-008 & NT-BUG-013: Handle race condition where phase is still Pending
	// If handlePendingToSendingTransition ran but the re-read returned a stale notification,
	// we need to persist the transition to Sending first before making terminal transitions.
	// This prevents invalid "Pending → Sent" transitions that violate the state machine.
	if notification.Status.Phase == notificationv1alpha1.NotificationPhasePending {
		log.Info("⚠️  RACE CONDITION DETECTED: Phase is still Pending after delivery loop",
			"expectedPhase", "Sending",
			"actualPhase", notification.Status.Phase,
			"fix", "Persisting transition to Sending before determining final state")

		// NT-BUG-013 Fix: Persist the Sending phase transition to K8s API
		// The local in-memory update is not enough - we must persist to K8s API
		// Otherwise AtomicStatusUpdate will try to transition from Pending (K8s state) to Sent (new state)
		if err := r.StatusManager.AtomicStatusUpdate(
			ctx,
			notification,
			notificationv1alpha1.NotificationPhaseSending,
			"ProcessingDeliveries",
			"Processing delivery channels",
			nil, // No delivery attempts yet
		); err != nil {
			log.Error(err, "Failed to persist Sending phase transition during race condition recovery")
			return ctrl.Result{}, err
		}

		log.Info("✅ Phase transition to Sending persisted successfully (race condition resolved)")

		// NT-BUG-014 Fix: Re-read notification from K8s to get latest resourceVersion
		// After persisting Sending phase, we must re-read to ensure we have the latest state
		// Otherwise, the next AtomicStatusUpdate will use stale resourceVersion and fail
		if err := r.Get(ctx, client.ObjectKeyFromObject(notification), notification); err != nil {
			log.Error(err, "Failed to re-read notification after Sending phase persistence")
			return ctrl.Result{}, err
		}

		log.Info("✅ Notification re-read after Sending phase persistence",
			"phase", notification.Status.Phase,
			"resourceVersion", notification.ResourceVersion)
	}

	// Calculate overall status
	totalChannels := len(notification.Spec.Channels)

	// Count successful deliveries from BOTH status and current attempts
	// This is critical because atomic updates haven't happened yet
	totalSuccessful := notification.Status.SuccessfulDeliveries
	for _, attempt := range result.deliveryAttempts {
		if attempt.Status == "success" {
			totalSuccessful++
		}
	}

	log.Info("🔍 PHASE TRANSITION LOGIC START",
		"currentPhase", notification.Status.Phase,
		"totalChannels", totalChannels,
		"totalSuccessful", totalSuccessful,
		"statusSuccessful", notification.Status.SuccessfulDeliveries,
		"attemptsSuccessful", len(result.deliveryAttempts),
		"failureCount", result.failureCount,
		"deliveryAttemptsRecorded", len(result.deliveryAttempts),
		"statusDeliveryAttempts", len(notification.Status.DeliveryAttempts))

	if totalSuccessful == totalChannels {
		// All channels delivered successfully → Sent
		log.Info("✅ ALL CHANNELS SUCCEEDED → transitioning to Sent")
		return r.transitionToSent(ctx, notification, result.deliveryAttempts)
	}

	// Check if all channels exhausted retries
	allChannelsExhausted := true
	policy := r.getRetryPolicy(notification)

	log.Info("🔍 STARTING EXHAUSTION CHECK",
		"allChannelsExhausted_initial", allChannelsExhausted,
		"maxAttempts", policy.MaxAttempts,
		"channels", notification.Spec.Channels)

	for _, channel := range notification.Spec.Channels {
		attemptCount := r.getChannelAttemptCount(notification, string(channel))
		hasSuccess := r.channelAlreadySucceeded(notification, string(channel))
		hasPermanentError := r.hasChannelPermanentError(notification, string(channel))

		log.Info("🔍 EXHAUSTION CHECK",
			"channel", channel,
			"attemptCount", attemptCount,
			"maxAttempts", policy.MaxAttempts,
			"hasSuccess", hasSuccess,
			"hasPermanentError", hasPermanentError,
			"isExhausted", hasSuccess || hasPermanentError || attemptCount >= policy.MaxAttempts)

		if !hasSuccess && !hasPermanentError && attemptCount < policy.MaxAttempts {
			allChannelsExhausted = false
			log.Info("✅ Channel NOT exhausted - retries available",
				"channel", channel,
				"attemptCount", attemptCount,
				"maxAttempts", policy.MaxAttempts)
			break
		}
	}

	log.Info("🔍 EXHAUSTION CHECK COMPLETE",
		"allChannelsExhausted_final", allChannelsExhausted,
		"willEnterExhaustedBlock", allChannelsExhausted)

	if allChannelsExhausted {
		log.Info("⚠️  ENTERING EXHAUSTED BLOCK",
			"totalSuccessful", totalSuccessful,
			"totalChannels", totalChannels)
		// NT-BUG-003 Fix: Check for partial success before marking as Failed
		if totalSuccessful > 0 && totalSuccessful < totalChannels {
			// Some channels succeeded, others failed → PartiallySent (terminal)
			log.Info("Partial delivery success with exhausted retries, transitioning to PartiallySent",
				"successful", totalSuccessful,
				"total", totalChannels)
			return r.transitionToPartiallySent(ctx, notification, result.deliveryAttempts)
		}

		// Determine failure reason: permanent errors vs retry exhaustion
		allPermanentErrors := true
		for _, channel := range notification.Spec.Channels {
			if !r.hasChannelPermanentError(notification, string(channel)) {
				allPermanentErrors = false
				break
			}
		}

		reason := "MaxRetriesExhausted"
		if allPermanentErrors {
			reason = "AllDeliveriesFailed"
		}

		// All retries exhausted with no successes → Failed (permanent)
		return r.transitionToFailed(ctx, notification, true, reason, result.deliveryAttempts)
	}

	// NT-BUG-005 Fix: Handle partial success correctly during retry loop
	// Don't transition to Failed if some channels succeeded - instead requeue with backoff
	log.Info("🔍 CHECKING FAILURE COUNT",
		"failureCount", result.failureCount,
		"totalSuccessful", totalSuccessful,
		"willCheckPartialSuccess", result.failureCount > 0)

	if result.failureCount > 0 {
		log.Info("🔍 INSIDE FAILURE COUNT BLOCK",
			"totalSuccessful", totalSuccessful,
			"willCheckPartialSuccessBranch", totalSuccessful > 0)

		if totalSuccessful > 0 {
			log.Info("🎯 PARTIAL SUCCESS BRANCH ENTERED - SHOULD TRANSITION TO RETRYING",
				"successful", totalSuccessful,
				"failed", result.failureCount,
				"total", totalChannels)
			// Partial success (some channels succeeded, some failed), retries remain
			// Stay in current phase and requeue with backoff to allow failed channels to retry

			// Calculate backoff based on max attempt count of failed channels
			maxAttemptCount := 0
			for _, channel := range notification.Spec.Channels {
				// Only consider failed channels for backoff calculation
				if !r.channelAlreadySucceeded(notification, string(channel)) {
					attemptCount := r.getChannelAttemptCount(notification, string(channel))
					if attemptCount > maxAttemptCount {
						maxAttemptCount = attemptCount
					}
				}
			}

			backoff := r.calculateBackoffWithPolicy(notification, maxAttemptCount)

			log.Info("⏰ PARTIAL SUCCESS WITH FAILURES → TRANSITIONING TO RETRYING",
				"successful", totalSuccessful,
				"failed", result.failureCount,
				"total", totalChannels,
				"backoff", backoff,
				"maxAttemptCount", maxAttemptCount,
				"currentPhase", notification.Status.Phase,
				"nextPhase", notificationv1alpha1.NotificationPhaseRetrying)

			return r.transitionToRetrying(ctx, notification, backoff, result.deliveryAttempts)
		}
		// All channels failed, retries remain → Failed (temporary, will retry)
		log.Info("🔍 ALL CHANNELS FAILED BRANCH",
			"totalSuccessful", totalSuccessful,
			"shouldTransitionToFailed", true)
		log.Info("All channels failed, retries remaining",
			"failed", result.failureCount,
			"total", totalChannels)
		return r.transitionToFailed(ctx, notification, false, "AllDeliveriesFailed", result.deliveryAttempts)
	}

	// Partial success with no failures (shouldn't reach here, but handle safely)
	log.Info("Partial delivery success, continuing",
		"successful", totalSuccessful,
		"total", totalChannels)
	return ctrl.Result{Requeue: true}, nil
}

// transitionToSent transitions notification to Sent (terminal success state).
func (r *NotificationRequestReconciler) transitionToSent(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	attempts []notificationv1alpha1.DeliveryAttempt,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// NT-BUG-009: Calculate correct successful count for message
	// The notification.Status.SuccessfulDeliveries hasn't been updated yet,
	// so we need to calculate from current batch + existing status
	totalSuccessful := notification.Status.SuccessfulDeliveries + countSuccessfulAttempts(attempts)

	// ATOMIC UPDATE: Record delivery attempts AND update phase to Sent in a single API call
	// DD-PERF-001: Atomic Status Updates
	if err := r.StatusManager.AtomicStatusUpdate(
		ctx,
		notification,
		notificationv1alpha1.NotificationPhaseSent,
		"AllDeliveriesSucceeded",
		fmt.Sprintf("Successfully delivered to %d channel(s)", totalSuccessful),
		attempts,
	); err != nil {
		log.Error(err, "Failed to atomically update status to Sent")
		return ctrl.Result{}, err
	}

	// DD-NOT-008: Clear in-memory tracking after successful status persistence
	// Critical for test isolation and to prevent stale state
	r.DeliveryOrchestrator.ClearInMemoryState(string(notification.UID))

	// Record metric
	// DD-METRICS-001: Use injected metrics recorder
	r.Metrics.UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhaseSent), 1)

	// AUDIT: Message acknowledged (ADR-032 §1: MANDATORY)
	if auditErr := r.auditMessageAcknowledged(ctx, notification); auditErr != nil {
		log.Error(auditErr, "CRITICAL: Failed to audit message.acknowledged (ADR-032 §1)")
		return ctrl.Result{}, fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
	}

	log.Info("NotificationRequest completed successfully (atomic update)",
		"name", notification.Name,
		"successfulDeliveries", notification.Status.SuccessfulDeliveries,
		"attemptsRecorded", len(attempts))

	return ctrl.Result{}, nil
}

// transitionToRetrying transitions notification to Retrying (non-terminal retry state).
// Used when some channels succeeded, some failed, but retries remain available.
// This is a non-terminal phase that allows the controller to continue retrying failed channels.
func (r *NotificationRequestReconciler) transitionToRetrying(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	backoff time.Duration,
	attempts []notificationv1alpha1.DeliveryAttempt,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// ATOMIC UPDATE: Record delivery attempts AND update phase to Retrying in a single API call
	// DD-PERF-001: Atomic Status Updates
	// 🔍 DEBUG: Track attempts before atomic update
	log.Info("🔍 BEFORE ATOMIC UPDATE (transitionToRetrying)",
		"newAttempts", len(attempts),
		"existingAttempts", len(notification.Status.DeliveryAttempts),
		"totalAttemptsShouldBe", len(attempts)+len(notification.Status.DeliveryAttempts))

	if err := r.StatusManager.AtomicStatusUpdate(
		ctx,
		notification,
		notificationv1alpha1.NotificationPhaseRetrying,
		"PartialFailureRetrying",
		fmt.Sprintf("Delivered to %d/%d channel(s), retrying failed channels with backoff %v",
			notification.Status.SuccessfulDeliveries,
			len(notification.Spec.Channels),
			backoff),
		attempts,
	); err != nil {
		log.Error(err, "Failed to atomically update status to Retrying")
		return ctrl.Result{}, err
	}

	// DD-NOT-008: Clear in-memory tracking after successful status persistence
	// Critical for test isolation and to prevent stale state
	r.DeliveryOrchestrator.ClearInMemoryState(string(notification.UID))

	// Record metric
	// DD-METRICS-001: Use injected metrics recorder
	r.Metrics.UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhaseRetrying), 1)

	log.Info("NotificationRequest entering retry phase (atomic update)",
		"name", notification.Name,
		"successfulDeliveries", notification.Status.SuccessfulDeliveries,
		"failedDeliveries", notification.Status.FailedDeliveries,
		"backoff", backoff,
		"attemptsRecorded", len(attempts))

	// Schedule next retry with exponential backoff
	return ctrl.Result{RequeueAfter: backoff}, nil
}

// transitionToPartiallySent transitions notification to PartiallySent (terminal partial success state).
// NT-BUG-003 Fix: When some channels succeed but others permanently fail (max retries exhausted).
func (r *NotificationRequestReconciler) transitionToPartiallySent(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	attempts []notificationv1alpha1.DeliveryAttempt,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// NT-BUG-009: Calculate correct successful count for message
	// The notification.Status.SuccessfulDeliveries hasn't been updated yet,
	// so we need to calculate from current batch + existing status
	totalSuccessful := notification.Status.SuccessfulDeliveries + countSuccessfulAttempts(attempts)

	// ATOMIC UPDATE: Record delivery attempts AND update phase to PartiallySent in a single API call
	// DD-PERF-001: Atomic Status Updates
	// 🔍 DEBUG: Track attempts before atomic update
	log.Info("🔍 BEFORE ATOMIC UPDATE (transitionToPartiallySent)",
		"newAttempts", len(attempts),
		"existingAttempts", len(notification.Status.DeliveryAttempts),
		"totalAttemptsShouldBe", len(attempts)+len(notification.Status.DeliveryAttempts))

	if err := r.StatusManager.AtomicStatusUpdate(
		ctx,
		notification,
		notificationv1alpha1.NotificationPhasePartiallySent,
		"PartialDeliverySuccess",
		fmt.Sprintf("Delivered to %d/%d channel(s), others failed",
			totalSuccessful,
			len(notification.Spec.Channels)),
		attempts,
	); err != nil {
		log.Error(err, "Failed to atomically update status to PartiallySent")
		return ctrl.Result{}, err
	}

	// DD-NOT-008: Clear in-memory tracking after successful status persistence
	r.DeliveryOrchestrator.ClearInMemoryState(string(notification.UID))

	// Record metric
	// DD-METRICS-001: Use injected metrics recorder
	r.Metrics.UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhasePartiallySent), 1)

	log.Info("NotificationRequest partially completed (atomic update)",
		"name", notification.Name,
		"successfulDeliveries", notification.Status.SuccessfulDeliveries,
		"failedDeliveries", notification.Status.FailedDeliveries,
		"attemptsRecorded", len(attempts))

	return ctrl.Result{}, nil
}

// transitionToFailed transitions notification to Failed state.
// If permanent is true, sets CompletionTime (terminal state).
func (r *NotificationRequestReconciler) transitionToFailed(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	permanent bool,
	reason string,
	attempts []notificationv1alpha1.DeliveryAttempt,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if permanent {
		// ATOMIC UPDATE: Record delivery attempts AND update phase to Failed in a single API call
		if err := r.StatusManager.AtomicStatusUpdate(
			ctx,
			notification,
			notificationv1alpha1.NotificationPhaseFailed,
			reason,
			"All delivery attempts failed or exhausted retries",
			attempts,
		); err != nil {
			log.Error(err, "Failed to atomically update status to Failed (permanent)")
			return ctrl.Result{}, err
		}

		// DD-NOT-008: Clear in-memory tracking after successful status persistence
		r.DeliveryOrchestrator.ClearInMemoryState(string(notification.UID))

		// Record metric
		// DD-METRICS-001: Use injected metrics recorder
		r.Metrics.UpdatePhaseCount(notification.Namespace, string(notificationv1alpha1.NotificationPhaseFailed), 1)

		// AUDIT: Message escalated (ADR-032 §1: MANDATORY)
		if auditErr := r.auditMessageEscalated(ctx, notification); auditErr != nil {
			log.Error(auditErr, "CRITICAL: Failed to audit message.escalated (ADR-032 §1)")
			return ctrl.Result{}, fmt.Errorf("audit failure (ADR-032 §1): %w", auditErr)
		}

		log.Info("NotificationRequest permanently failed (atomic update)",
			"name", notification.Name,
			"failedDeliveries", notification.Status.FailedDeliveries,
			"attemptsRecorded", len(attempts))

		return ctrl.Result{}, nil
	}

	// Temporary failure - will retry with backoff
	// For temporary failures, we still need to record attempts but stay in current phase
	// Use atomic update with current phase (no phase change)
	if len(attempts) > 0 {
		// Record attempts without changing phase (atomic operation)
		if err := r.StatusManager.AtomicStatusUpdate(
			ctx,
			notification,
			notification.Status.Phase, // Stay in current phase
			reason,
			"Delivery failed, will retry with backoff",
			attempts,
		); err != nil {
			log.Error(err, "Failed to atomically record attempts for temporary failure")
			return ctrl.Result{}, err
		}
	}

	// Calculate backoff for retry
	maxAttemptCount := 0
	for _, channel := range notification.Spec.Channels {
		attemptCount := r.getChannelAttemptCount(notification, string(channel))
		if attemptCount > maxAttemptCount {
			maxAttemptCount = attemptCount
		}
	}

	backoff := r.calculateBackoffWithPolicy(notification, maxAttemptCount)

	log.Info("NotificationRequest failed, will retry with backoff (atomic update)",
		"name", notification.Name,
		"backoff", backoff,
		"attemptCount", maxAttemptCount,
		"attemptsRecorded", len(attempts))

	return ctrl.Result{RequeueAfter: backoff}, nil
}
