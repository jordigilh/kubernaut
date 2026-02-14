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

// Package controller provides the Kubernetes controller for RemediationRequest CRDs.
//
// Business Requirements:
// - BR-ORCH-025: Phase state transitions
// - BR-ORCH-026: Status aggregation from child CRDs
// - BR-ORCH-027: Global timeout handling
// - BR-ORCH-028: Per-phase timeout handling
// - BR-ORCH-001: Approval notification creation
// - BR-ORCH-036: Manual review notification creation
// - BR-ORCH-037: WorkflowNotNeeded handling
// - BR-ORCH-038: Preserve Gateway deduplication data
package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	rrconditions "github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
)

// Reconciler reconciles RemediationRequest objects.
type Reconciler struct {
	client              client.Client
	scheme              *runtime.Scheme
	statusAggregator    *aggregator.StatusAggregator
	aiAnalysisHandler   *handler.AIAnalysisHandler
	spHandler           *handler.SignalProcessingHandler     // Handler Consistency Refactoring (2026-01-22)
	weHandler           *handler.WorkflowExecutionHandler    // Handler Consistency Refactoring (2026-01-22)
	notificationCreator *creator.NotificationCreator
	spCreator           *creator.SignalProcessingCreator
	aiAnalysisCreator   *creator.AIAnalysisCreator
	weCreator           *creator.WorkflowExecutionCreator
	approvalCreator     *creator.ApprovalCreator
	// Audit integration (DD-AUDIT-003, BR-STORAGE-001)
	auditStore   audit.AuditStore
	auditManager *roaudit.Manager
	// Timeout configuration (BR-ORCH-027/028, Future-proof implementation)
	timeouts TimeoutConfig
	// Consecutive failure blocking (BR-ORCH-042)
	consecutiveBlock *ConsecutiveFailureBlocker
	// Notification lifecycle tracking (BR-ORCH-029/030)
	notificationHandler *NotificationHandler
	// Routing engine for centralized routing (DD-RO-002, V1.0)
	// Checks for blocking conditions before creating WorkflowExecution
	// Uses interface to allow mocking in unit tests
	routingEngine routing.Engine
	// V1.0 Maturity Requirements (per SERVICE_MATURITY_REQUIREMENTS.md)
	Metrics  *metrics.Metrics     // DD-METRICS-001: Dependency-injected metrics for testability
	Recorder record.EventRecorder // K8s best practice: EventRecorder for debugging

	// ADR-EM-001: EA creator for creating EffectivenessAssessment CRDs on terminal phases
	eaCreator *creator.EffectivenessAssessmentCreator

	// DD-EM-002: REST mapper for resolving Kind to GVK for pre-remediation hash capture
	restMapper meta.RESTMapper
	// DD-EM-002: Uncached API reader for pre-remediation hash capture
	apiReader client.Reader

	// ========================================
	// STATUS MANAGER (DD-PERF-001)
	// 📋 Design Decision: DD-PERF-001 | ✅ Atomic Status Updates Pattern
	// See: docs/architecture/decisions/DD-PERF-001-atomic-status-updates-mandate.md
	// ========================================
	//
	// StatusManager manages atomic status updates to reduce K8s API calls
	// Consolidates multiple status field updates into single atomic operations
	//
	// BENEFITS:
	// - 85-90% API call reduction (6-8+ updates → 1 atomic update per orchestration cycle)
	// - Eliminates race conditions from sequential condition updates
	// - Reduces etcd write load and watch events
	//
	// WIRED IN: cmd/remediationorchestrator/main.go
	// USAGE: r.StatusManager.AtomicStatusUpdate(ctx, rr, func() { ... })
	StatusManager *status.Manager
}

// TimeoutConfig holds all timeout configuration for the controller.
// Provides defaults for all remediations, can be overridden per-RR via spec.timeoutConfig.
// Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
type TimeoutConfig struct {
	Global     time.Duration // Default: 1 hour
	Processing time.Duration // Default: 5 minutes
	Analyzing  time.Duration // Default: 10 minutes
	Executing  time.Duration // Default: 30 minutes
}

// NewReconciler creates a new Reconciler with all dependencies.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// The auditStore parameter must be non-nil; the service will crash at startup
// (cmd/remediationorchestrator/main.go:128) if audit cannot be initialized.
// Tests must provide a non-nil audit store (use NoOpStore or mock).
// The timeouts parameter configures all timeout durations (global and per-phase).
// Zero values use defaults: Global=1h, Processing=5m, Analyzing=10m, Executing=30m.
// DD-STATUS-001: apiReader parameter added for cache-bypassed status refetch in atomic updates.
func NewReconciler(c client.Client, apiReader client.Reader, s *runtime.Scheme, auditStore audit.AuditStore, recorder record.EventRecorder, m *metrics.Metrics, timeouts TimeoutConfig, routingEngine routing.Engine, eaCreator ...*creator.EffectivenessAssessmentCreator) *Reconciler {
	// DD-METRICS-001: Metrics are REQUIRED (dependency injection pattern)
	// Metrics are initialized in main.go via rometrics.NewMetrics()
	// If nil is passed here, it's a programming error in main.go

	// Set default timeouts if not specified (BR-ORCH-027/028)
	if timeouts.Global == 0 {
		timeouts.Global = 1 * time.Hour
	}
	if timeouts.Processing == 0 {
		timeouts.Processing = 5 * time.Minute
	}
	if timeouts.Analyzing == 0 {
		timeouts.Analyzing = 10 * time.Minute
	}
	if timeouts.Executing == 0 {
		timeouts.Executing = 30 * time.Minute
	}

	nc := creator.NewNotificationCreator(c, s, m)

	// Initialize routing engine if not provided (DD-RO-002, DD-WE-004)
	// If routingEngine is nil, create default routing engine for production use
	// Unit tests can pass a mock routing engine to test orchestration logic in isolation
	if routingEngine == nil {
		routingConfig := routing.Config{
			ConsecutiveFailureThreshold: 3,                                    // BR-ORCH-042
			ConsecutiveFailureCooldown:  int64(1 * time.Hour / time.Second),   // 3600 seconds (1 hour)
			RecentlyRemediatedCooldown:  int64(5 * time.Minute / time.Second), // 300 seconds (5 minutes)
			// Exponential backoff (DD-WE-004, V1.0)
			ExponentialBackoffBase:        int64(1 * time.Minute / time.Second),  // 60 seconds (1 minute)
			ExponentialBackoffMax:         int64(10 * time.Minute / time.Second), // 600 seconds (10 minutes)
			ExponentialBackoffMaxExponent: 4,                                     // 2^4 = 16x multiplier
			// Scope validation backoff (ADR-053 Decision #4, BR-SCOPE-010)
			ScopeBackoffBase: 5,   // 5 seconds initial
			ScopeBackoffMax:  300, // 5 minutes max
		}
		// TODO: Get namespace from controller-runtime manager or environment variable
		// For now, using empty string which means all namespaces
		routingNamespace := ""
		// BR-SCOPE-010: Create scope manager using cached client for metadata-only informers (ADR-053)
		scopeMgr := scope.NewManager(c)
		// DD-STATUS-001: Pass apiReader for cache-bypassed routing queries
		routingEngine = routing.NewRoutingEngine(c, apiReader, routingNamespace, routingConfig, scopeMgr)
	}

	// ========================================
	// DD-PERF-001: Atomic Status Updates
	// Status Manager for reducing K8s API calls by 85-90%
	// Consolidates multiple status field updates into single atomic operations
	// ========================================
	statusManager := status.NewManager(c, apiReader)

	r := &Reconciler{
		client:              c,
		scheme:              s,
		statusAggregator:    aggregator.NewStatusAggregator(c),
		notificationCreator: nc,
		spCreator:           creator.NewSignalProcessingCreator(c, s, m),
		aiAnalysisCreator:   creator.NewAIAnalysisCreator(c, s, m),
		weCreator:           creator.NewWorkflowExecutionCreator(c, s, m),
		approvalCreator:     creator.NewApprovalCreator(c, s, m),
		timeouts:            timeouts,
		auditStore:          auditStore,
		auditManager:        roaudit.NewManager(roaudit.ServiceName),
		consecutiveBlock:    NewConsecutiveFailureBlocker(c, 3, 1*time.Hour, true),
		notificationHandler: NewNotificationHandler(c, m),
		routingEngine:       routingEngine, // Use provided or default routing engine
		Metrics:             m,
		Recorder:            recorder,
		StatusManager:       statusManager, // DD-PERF-001: Atomic status updates
		apiReader:           apiReader,     // DD-EM-002: Uncached reader for pre-remediation hash
	}

	// ADR-EM-001: Wire optional EA creator (variadic for backward compatibility)
	if len(eaCreator) > 0 && eaCreator[0] != nil {
		r.eaCreator = eaCreator[0]
	}

	// ========================================
	// HANDLER INITIALIZATION (Handler Consistency Refactoring 2026-01-22)
	// Initialize handlers with transition callbacks for audit emission (BR-AUDIT-005, DD-AUDIT-003)
	// ========================================

	// SignalProcessingHandler: delegates phase transitions
	r.spHandler = handler.NewSignalProcessingHandler(c, s, r.transitionPhase)

	// AIAnalysisHandler: delegates failure transitions
	r.aiAnalysisHandler = handler.NewAIAnalysisHandler(c, s, nc, m, r.transitionToFailed)

	// WorkflowExecutionHandler: delegates completion and failure transitions
	r.weHandler = handler.NewWorkflowExecutionHandler(c, s, m, r.transitionToFailed, r.transitionToCompleted)

	return r
}

// ========================================
// TIMEOUT CONFIGURATION INITIALIZATION
// BR-ORCH-027/028, BR-AUDIT-005 Gap #8
// ========================================

// populateTimeoutDefaults populates status.timeoutConfig with controller defaults.
// This is a pure function that modifies the RR in-place without performing status updates.
// Designed to be called from within AtomicStatusUpdate callbacks (DD-PERF-001).
//
// Per Gap #8 (BR-AUDIT-005): RO owns timeout initialization, operators can override later.
// This ensures:
// - Fresh RRs get controller defaults immediately
// - Operator modifications are preserved across reconciles
// - Audit trail can track initial configuration (Gap #8: orchestrator.lifecycle.created)
//
// Validation (REFACTOR enhancement):
// - Ensures all timeouts are positive (>0)
// - Logs warnings for unusual timeout values
// - Per-phase timeouts should not exceed global timeout
//
// Reference:
// - BR-ORCH-027 (Global timeout)
// - BR-ORCH-028 (Per-phase timeouts)
// - Gap #8: TimeoutConfig moved to status for operator mutability
// - DD-PERF-001: No status updates in helper methods
//
// Parameters:
// - ctx: Context for logging
// - rr: RemediationRequest to populate (modified in-place)
//
// Returns:
// - bool: true if TimeoutConfig was populated, false if already initialized
func (r *Reconciler) populateTimeoutDefaults(ctx context.Context, rr *remediationv1.RemediationRequest) bool {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "namespace", rr.Namespace)

	// Only initialize if status.timeoutConfig is nil (first reconcile)
	if rr.Status.TimeoutConfig != nil {
		logger.V(2).Info("TimeoutConfig already initialized, skipping",
			"global", rr.Status.TimeoutConfig.Global,
			"processing", rr.Status.TimeoutConfig.Processing,
			"analyzing", rr.Status.TimeoutConfig.Analyzing,
			"executing", rr.Status.TimeoutConfig.Executing)
		return false // Already initialized, preserve existing values
	}

	// REFACTOR: Validate controller timeouts before applying
	// This prevents configuration errors from propagating to RRs
	if err := r.validateControllerTimeouts(); err != nil {
		logger.Error(err, "Controller timeout configuration invalid, using safe defaults")
		// Fallback to safe defaults if controller config is invalid
		rr.Status.TimeoutConfig = r.getSafeDefaultTimeouts()
		return true
	}

	// Set defaults from controller config
	rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
		Global:     &metav1.Duration{Duration: r.timeouts.Global},
		Processing: &metav1.Duration{Duration: r.timeouts.Processing},
		Analyzing:  &metav1.Duration{Duration: r.timeouts.Analyzing},
		Executing:  &metav1.Duration{Duration: r.timeouts.Executing},
	}

	logger.Info("Populated timeout defaults in status.timeoutConfig",
		"global", r.timeouts.Global,
		"processing", r.timeouts.Processing,
		"analyzing", r.timeouts.Analyzing,
		"executing", r.timeouts.Executing)

	return true
}

// validateControllerTimeouts validates that controller-level timeout configuration is sane.
// REFACTOR enhancement: Prevents invalid configuration from affecting RRs.
//
// Validation rules:
// - All timeouts must be positive (>0)
// - Per-phase timeouts should not exceed global timeout
// - Global timeout should be at least 1 minute
//
// Returns:
// - error: Non-nil if validation fails
func (r *Reconciler) validateControllerTimeouts() error {
	if r.timeouts.Global <= 0 {
		return fmt.Errorf("global timeout must be positive, got %v", r.timeouts.Global)
	}
	if r.timeouts.Global < 1*time.Minute {
		return fmt.Errorf("global timeout too short (%v), must be at least 1 minute", r.timeouts.Global)
	}
	if r.timeouts.Processing <= 0 {
		return fmt.Errorf("processing timeout must be positive, got %v", r.timeouts.Processing)
	}
	if r.timeouts.Analyzing <= 0 {
		return fmt.Errorf("analyzing timeout must be positive, got %v", r.timeouts.Analyzing)
	}
	if r.timeouts.Executing <= 0 {
		return fmt.Errorf("executing timeout must be positive, got %v", r.timeouts.Executing)
	}

	// Warn if per-phase timeouts exceed global (not fatal, but suspicious)
	if r.timeouts.Processing > r.timeouts.Global {
		return fmt.Errorf("processing timeout (%v) exceeds global timeout (%v)", r.timeouts.Processing, r.timeouts.Global)
	}
	if r.timeouts.Analyzing > r.timeouts.Global {
		return fmt.Errorf("analyzing timeout (%v) exceeds global timeout (%v)", r.timeouts.Analyzing, r.timeouts.Global)
	}
	if r.timeouts.Executing > r.timeouts.Global {
		return fmt.Errorf("executing timeout (%v) exceeds global timeout (%v)", r.timeouts.Executing, r.timeouts.Global)
	}

	return nil
}

// getSafeDefaultTimeouts returns safe fallback timeout values.
// Used when controller configuration is invalid.
// REFACTOR enhancement: Ensures system never operates with zero timeouts.
//
// Returns:
// - *remediationv1.TimeoutConfig: Safe default configuration
func (r *Reconciler) getSafeDefaultTimeouts() *remediationv1.TimeoutConfig {
	return &remediationv1.TimeoutConfig{
		Global:     &metav1.Duration{Duration: 1 * time.Hour},
		Processing: &metav1.Duration{Duration: 5 * time.Minute},
		Analyzing:  &metav1.Duration{Duration: 10 * time.Minute},
		Executing:  &metav1.Duration{Duration: 30 * time.Minute},
	}
}

// Reconcile implements the reconciliation loop for RemediationRequest.
// It handles phase transitions and delegates to appropriate handlers.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", req.NamespacedName)
	startTime := time.Now()

	// Fetch the RemediationRequest
	rr := &remediationv1.RemediationRequest{}
	if err := r.client.Get(ctx, req.NamespacedName, rr); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("RemediationRequest not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch RemediationRequest")
		return ctrl.Result{}, err
	}

	// Record reconcile duration on exit (only if metrics are available)
	defer func() {
		r.Metrics.ReconcileDurationSeconds.WithLabelValues(
			rr.Namespace,
			string(rr.Status.OverallPhase),
		).Observe(time.Since(startTime).Seconds())
		r.Metrics.ReconcileTotal.WithLabelValues(rr.Namespace, string(rr.Status.OverallPhase)).Inc()
	}()

	// BR-AUDIT-005 Gap #7: Validate timeout configuration (fail fast on invalid config)
	// Per test requirement: Negative timeouts should be rejected with ERR_INVALID_TIMEOUT_CONFIG
	if err := r.validateTimeoutConfig(ctx, rr); err != nil {
		logger.Error(err, "Invalid timeout configuration, transitioning to Failed")
		return r.transitionToFailed(ctx, rr, "configuration", err)
	}

	// ========================================
	// OBSERVED GENERATION CHECK (DD-CONTROLLER-001 Pattern B - Phase-Aware)
	// Per OBSERVED_GENERATION_DEEP_ANALYSIS_JAN_01_2026.md
	// ========================================
	// Phase-Aware Pattern: Parent Controllers with Active Orchestration
	// - Remove GenerationChangedPredicate (allow child status updates) ✅ Already done
	// - Add phase-aware ObservedGeneration check (balance idempotency with orchestration)
	//
	// The Challenge:
	// - Annotation changes: Generation unchanged, should skip (wasteful)
	// - Child status updates: Generation unchanged, MUST reconcile (critical!)
	// - Polling checks: Generation unchanged, MUST reconcile (critical!)
	// → Generation-based check CANNOT distinguish these events!
	//
	// The Solution: Phase-Aware Skip Logic
	// Skip reconcile ONLY when we're NOT actively orchestrating:
	// 1. Initial state (OverallPhase == "") → Allow (initialization)
	// 2. Pending phase → Skip (not yet orchestrating, wasteful)
	// 3. Processing/Analyzing/Executing → Allow (active orchestration of child CRDs)
	// 4. Terminal phases (Completed/Failed) → Allow (owned-resource housekeeping: Issue #88)
	//    Terminal RRs must still process NT delivery status and EA completion events.
	//    Guard2 (below) handles terminal phases with a dedicated housekeeping block.
	//
	// Tradeoff: Accepts extra reconciles during active and terminal phases
	// Benefit: Allows critical polling, child status updates, and terminal housekeeping
	if rr.Status.ObservedGeneration == rr.Generation &&
		rr.Status.OverallPhase == phase.Pending {
		logger.V(1).Info("⏭️  SKIPPED: No orchestration needed in Pending phase",
			"phase", rr.Status.OverallPhase,
			"generation", rr.Generation,
			"observedGeneration", rr.Status.ObservedGeneration,
			"reason", "ObservedGeneration matches and phase is Pending")
		return ctrl.Result{}, nil
	}

	// Log when we proceed during active orchestration (helps understand behavior)
	if rr.Status.ObservedGeneration == rr.Generation &&
		rr.Status.OverallPhase != "" &&
		!phase.IsTerminal(phase.Phase(rr.Status.OverallPhase)) {
		logger.V(1).Info("✅ PROCEEDING: Active orchestration phase",
			"phase", rr.Status.OverallPhase,
			"generation", rr.Generation,
			"reason", "Child orchestration requires reconciliation")
	}

	// Initialize phase if empty (new RemediationRequest from Gateway)
	// Per DD-GATEWAY-011: RO owns status.overallPhase, Gateway creates instances without status
	if rr.Status.OverallPhase == "" {
		logger.Info("Initializing new RemediationRequest", "name", rr.Name)

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Initialize phase + StartTime + TimeoutConfig in single API call
		// DD-CONTROLLER-001: ObservedGeneration NOT set here - only after processing phase
		// Gap #8: Initialize TimeoutConfig defaults on first reconcile
		// REFACTOR: Use extracted helper method for timeout initialization
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			rr.Status.OverallPhase = phase.Pending
			rr.Status.StartTime = &metav1.Time{Time: startTime}

		// Gap #8: Initialize TimeoutConfig with controller defaults
		// REFACTOR: Delegated to populateTimeoutDefaults() for validation + reusability
		r.populateTimeoutDefaults(ctx, rr)

		return nil
	}); err != nil {
		logger.Error(err, "Failed to initialize RemediationRequest status")
		return ctrl.Result{}, err
	}

	// RO-AUDIT-IDEMPOTENCY: Emit initialization audit events ONLY after winning the
	// AtomicStatusUpdate. Concurrent reconciles that both see OverallPhase == "" will
	// race on the status update; only the winner proceeds here, preventing duplicate
	// lifecycle.started/created events. Same pattern as RAR (ConditionAuditRecorded),
	// Notification (NT-BUG-001), and WFE (DD-STATUS-001).
	r.emitLifecycleStartedAudit(ctx, rr)

	// Gap #8: NO REFETCH NEEDED - AtomicStatusUpdate already updated rr in-memory
	// The rr object passed to AtomicStatusUpdate is updated with the persisted status
	// (including TimeoutConfig) by the Status().Update() call inside AtomicStatusUpdate.
	// Refetching here would risk getting a cached/stale version.

	// Gap #8: Emit orchestrator.lifecycle.created event with TimeoutConfig
	// Per BR-AUDIT-005 Gap #8: Capture initial TimeoutConfig for RR reconstruction
	// This happens AFTER status initialization to capture actual defaults
	r.emitRemediationCreatedAudit(ctx, rr)

	// DD-EVENT-001: Emit K8s event for new RR acceptance (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonRemediationCreated,
			fmt.Sprintf("RemediationRequest accepted for signal %s", rr.Spec.SignalName))
	}

	// Requeue after short delay to process the Pending phase
	// Using RequeueAfter instead of deprecated Requeue field
	return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	// Terminal-phase housekeeping (Issue #88: process owned-resource completion events)
	// Terminal RRs still need to track NotificationRequest delivery status and
	// (future) EffectivenessAssessment completion. Without this block, owned-resource
	// events arriving after RR reaches terminal state are silently dropped.
	if phase.IsTerminal(phase.Phase(rr.Status.OverallPhase)) {
		logger.V(1).Info("Terminal-phase housekeeping", "phase", rr.Status.OverallPhase)

		// BR-ORCH-029/030: Track notification delivery status for terminal RRs
		if err := r.trackNotificationStatus(ctx, rr); err != nil {
			logger.Error(err, "Failed to track notification status in terminal phase")
			// Non-fatal: continue to return
		}

		// BR-ORCH-044: Track routing decision - no action needed
		r.Metrics.NoActionNeededTotal.WithLabelValues(rr.Namespace, string(rr.Status.OverallPhase)).Inc()

		return ctrl.Result{}, nil
	}

	// Check for global timeout (BR-ORCH-027)
	// Supports per-RR override via spec.timeoutConfig.global (AC-027-4)
	// Business Value: Prevents stuck remediations from consuming resources indefinitely
	// Note: Uses status.StartTime (not CreationTimestamp) as StartTime is explicitly set by controller
	if rr.Status.StartTime != nil {
		globalTimeout := r.getEffectiveGlobalTimeout(rr)
		timeSinceStart := time.Since(rr.Status.StartTime.Time)
		if timeSinceStart > globalTimeout {
			logger.Info("RemediationRequest exceeded global timeout",
				"timeSinceStart", timeSinceStart,
				"globalTimeout", globalTimeout,
				"overridden", rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Global != nil,
				"startTime", rr.Status.StartTime.Time)
			return r.handleGlobalTimeout(ctx, rr)
		}
	}

	// Check for per-phase timeouts (BR-ORCH-028)
	// Enables faster detection of stuck phases without waiting for global timeout
	if err := r.checkPhaseTimeouts(ctx, rr); err != nil {
		return ctrl.Result{}, err
	}

	// Aggregate status from child CRDs
	aggregatedStatus, err := r.statusAggregator.AggregateStatus(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to aggregate status")
		return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil // REFACTOR-RO-003
	}

	// Track notification status (BR-ORCH-029/030)
	// Updates RR status based on NotificationRequest phase changes
	if err := r.trackNotificationStatus(ctx, rr); err != nil {
		logger.Error(err, "Failed to track notification status")
		// Non-fatal: Continue with reconciliation
	}

	// Handle based on current phase
	switch phase.Phase(rr.Status.OverallPhase) {
	case phase.Pending:
		return r.handlePendingPhase(ctx, rr)
	case phase.Processing:
		return r.handleProcessingPhase(ctx, rr, aggregatedStatus)
	case phase.Analyzing:
		return r.handleAnalyzingPhase(ctx, rr, aggregatedStatus)
	case phase.AwaitingApproval:
		return r.handleAwaitingApprovalPhase(ctx, rr)
	case phase.Executing:
		return r.handleExecutingPhase(ctx, rr, aggregatedStatus)
	case phase.Blocked:
		// BR-ORCH-042: Handle blocked phase (cooldown expiry check)
		return r.handleBlockedPhase(ctx, rr)
	default:
		logger.Info("Unknown phase", "phase", rr.Status.OverallPhase)
		return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil // REFACTOR-RO-003
	}
}

// handlePendingPhase handles the initial Pending phase.
// Creates SignalProcessing CRD and transitions to Processing.
// Per DD-AUDIT-003: Emits orchestrator.lifecycle.started (P1)
// Per BR-ORCH-025: Pass-through data to SignalProcessing CRD.
// Per BR-ORCH-031: Sets owner reference for cascade deletion.
func (r *Reconciler) handlePendingPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	logger.Info("Handling Pending phase - checking routing conditions")

	// Note: lifecycle.started audit event is emitted during phase initialization (line ~207)
	// This function handles the business logic of the Pending phase

	// V1.0: Check routing conditions BEFORE creating SignalProcessing (DD-RO-002)
	// This prevents duplicate RRs from flooding the system with duplicate SP/AI/WFE chains
	// Primary check: DuplicateInProgress (same fingerprint, active RR exists)
	// Note: Empty workflowID since we're in Pending phase (before AI selects workflow)
	blocked, err := r.routingEngine.CheckBlockingConditions(ctx, rr, "")
	if err != nil {
		logger.Error(err, "Failed to check routing conditions")
		return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
	}

	// If blocked, update status and requeue (DO NOT create SignalProcessing)
	if blocked != nil {
		logger.Info("Routing blocked - will not create SignalProcessing",
			"reason", blocked.Reason,
			"message", blocked.Message,
			"requeueAfter", blocked.RequeueAfter)
		return r.handleBlocked(ctx, rr, blocked, string(remediationv1.PhasePending), "")
	}

	// Routing checks passed - create SignalProcessing
	logger.Info("Routing checks passed, creating SignalProcessing")

	// Create SignalProcessing CRD (BR-ORCH-025, BR-ORCH-031)
	// DD-CRD-002-RR: Creator sets SignalProcessingReady condition in-memory
	spName, err := r.spCreator.Create(ctx, rr)
	if err != nil {
		// Check if namespace is terminating (async deletion in progress)
		// This is a benign race condition - namespace is being cleaned up
		if k8serrors.IsNamespaceTerminating(err) {
			logger.V(1).Info("Namespace is terminating, skipping reconciliation",
				"namespace", rr.Namespace, "reason", "async_cleanup")
			return ctrl.Result{}, nil // Don't requeue - namespace will be deleted
		}

		logger.Error(err, "Failed to create SignalProcessing CRD")
		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Persist SignalProcessingReady=False condition
		// Creator set condition in-memory, but refetch wipes it
		// Re-set condition AFTER refetch to preserve it
		// ========================================
		if updateErr := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			// Re-set condition after refetch (creator set it before, but refetch wiped it)
			remediationrequest.SetSignalProcessingReady(rr, false,
				fmt.Sprintf("Failed to create SignalProcessing: %v", err), r.Metrics)
			return nil
		}); updateErr != nil {
			logger.Error(updateErr, "Failed to update SignalProcessingReady condition")
		}
		return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
	}
	logger.Info("Created SignalProcessing CRD", "spName", spName)

	// BR-ORCH-044: Track child CRD creation
	r.Metrics.ChildCRDCreationsTotal.WithLabelValues("SignalProcessing", rr.Namespace).Inc()

	// Set SignalProcessingRef in status for aggregator (BR-ORCH-029)
	// REFACTOR-RO-001: Using retry helper
	// DD-CRD-002-RR: Also persists SignalProcessingReady=True condition set by creator
	err = helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.SignalProcessingRef = &corev1.ObjectReference{
			APIVersion: signalprocessingv1.GroupVersion.String(),
			Kind:       "SignalProcessing",
			Name:       spName,
			Namespace:  rr.Namespace,
		}
		// Preserve SignalProcessingReady condition from creator
		// (UpdateRemediationRequestStatus fetches fresh RR, so we need to re-set the condition)
		rrconditions.SetSignalProcessingReady(rr, true, fmt.Sprintf("SignalProcessing CRD %s created successfully", spName), r.Metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to set SignalProcessingRef in status")
		return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
	}
	logger.V(1).Info("Set SignalProcessingRef in status", "spName", spName)

	// Transition to Processing phase
	return r.transitionPhase(ctx, rr, phase.Processing)
}

// handleProcessingPhase handles the Processing phase.
// Waits for SignalProcessing to complete, then creates AIAnalysis.
func (r *Reconciler) handleProcessingPhase(ctx context.Context, rr *remediationv1.RemediationRequest, agg *aggregator.AggregatedStatus) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "spPhase", agg.SignalProcessingPhase)

	// First, check if we're in Processing but have no SP ref (corrupted state)
	if rr.Status.SignalProcessingRef == nil {
		logger.Error(nil, "Processing phase but no SignalProcessingRef - corrupted state")
		return r.transitionToFailed(ctx, rr, "signal_processing", fmt.Errorf("SignalProcessing not found"))
	}

	// Fetch SignalProcessing CRD
	sp := &signalprocessingv1.SignalProcessing{}
	err := r.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.SignalProcessingRef.Name,
		Namespace: rr.Status.SignalProcessingRef.Namespace,
	}, sp)
	if err != nil {
		logger.Error(err, "Failed to fetch SignalProcessing CRD")
		return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
	}

	// Special handling for Completed status: create AIAnalysis before transitioning
	// This is unique to SP because AIAnalysis creation requires SP enrichment data
	if sp.Status.Phase == signalprocessingv1.PhaseCompleted {
		logger.Info("SignalProcessing completed, creating AIAnalysis")

		// Create AIAnalysis CRD (BR-ORCH-025, BR-ORCH-031)
		// DD-CRD-002-RR: Creator sets AIAnalysisReady condition in-memory
		aiName, err := r.aiAnalysisCreator.Create(ctx, rr, sp)
		if err != nil {
			logger.Error(err, "Failed to create AIAnalysis CRD")
			// ========================================
			// DD-PERF-001: ATOMIC STATUS UPDATE
			// Persist AIAnalysisReady=False condition
			// ========================================
			if updateErr := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
				// Re-set condition after refetch (creator set it before, but refetch wiped it)
				remediationrequest.SetAIAnalysisReady(rr, false,
					fmt.Sprintf("Failed to create AIAnalysis: %v", err), r.Metrics)
				return nil
			}); updateErr != nil {
				logger.Error(updateErr, "Failed to update AIAnalysisReady condition")
			}
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.Info("Created AIAnalysis CRD", "aiName", aiName)

		// BR-ORCH-044: Track child CRD creation
		r.Metrics.ChildCRDCreationsTotal.WithLabelValues("AIAnalysis", rr.Namespace).Inc()

		// Set AIAnalysisRef in status for aggregator (BR-ORCH-029)
		// REFACTOR-RO-001: Using retry helper
		// DD-CRD-002-RR: Also persists AIAnalysisReady=True condition set by creator
		err = helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.AIAnalysisRef = &corev1.ObjectReference{
				APIVersion: aianalysisv1.GroupVersion.String(),
				Kind:       "AIAnalysis",
				Name:       aiName,
				Namespace:  rr.Namespace,
			}
			// Preserve AIAnalysisReady condition from creator
			rrconditions.SetAIAnalysisReady(rr, true, fmt.Sprintf("AIAnalysis CRD %s created successfully", aiName), r.Metrics)
			return nil
		})
		if err != nil {
			logger.Error(err, "Failed to set AIAnalysisRef in status")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.V(1).Info("Set AIAnalysisRef in status", "aiName", aiName)

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Set SignalProcessingComplete condition
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetSignalProcessingComplete(rr, true,
				remediationrequest.ReasonSignalProcessingSucceeded,
				"SignalProcessing completed successfully", r.Metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update SignalProcessingComplete condition")
			// Continue - condition update is best-effort
		}
	}

	// Handle SignalProcessing failure before delegating
	if sp.Status.Phase == signalprocessingv1.PhaseFailed {
		logger.Info("SignalProcessing failed, transitioning to Failed")
		// Set SignalProcessingComplete condition (false)
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetSignalProcessingComplete(rr, false,
				remediationrequest.ReasonSignalProcessingFailed,
				"SignalProcessing failed", r.Metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update SignalProcessingComplete condition")
			// Continue - condition update is best-effort
		}
		return r.transitionToFailed(ctx, rr, "signal_processing", fmt.Errorf("SignalProcessing failed"))
	}

	// Delegate to SignalProcessingHandler for status-based transitions
	// Handler Consistency Refactoring (2026-01-22): Extract status handling logic
	return r.spHandler.HandleStatus(ctx, rr, sp)
}

// handleAnalyzingPhase handles the Analyzing phase.
// Waits for AIAnalysis to complete, then handles the result.
// Reference: BR-ORCH-036 (manual review), BR-ORCH-037 (workflow not needed)
func (r *Reconciler) handleAnalyzingPhase(ctx context.Context, rr *remediationv1.RemediationRequest, agg *aggregator.AggregatedStatus) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "aiPhase", agg.AIAnalysisPhase)

	// Check if AIAnalysis exists
	if rr.Status.AIAnalysisRef == nil {
		logger.V(1).Info("AIAnalysis not created yet, waiting")
		return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
	}

	// Fetch the AIAnalysis CRD for detailed status
	ai := &aianalysisv1.AIAnalysis{}
	err := r.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.AIAnalysisRef.Name,
		Namespace: rr.Status.AIAnalysisRef.Namespace,
	}, ai)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AIAnalysis CRD not found, waiting for creation")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.Error(err, "Failed to fetch AIAnalysis CRD")
		return ctrl.Result{}, err
	}

	// Delegate to AIAnalysisHandler for Completed/Failed phases
	// This handles BR-ORCH-036 (manual review), BR-ORCH-037 (workflow not needed)
	// Phase values per api/aianalysis/v1alpha1: Pending|Investigating|Analyzing|Completed|Failed
	switch ai.Status.Phase {
	case "Completed":
		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Set AIAnalysisComplete condition
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetAIAnalysisComplete(rr, true,
				remediationrequest.ReasonAIAnalysisSucceeded,
				"AIAnalysis completed successfully", r.Metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update AIAnalysisComplete condition")
			// Continue - condition update is best-effort
		}

		// Check for WorkflowNotNeeded (BR-ORCH-037)
		if handler.IsWorkflowNotNeeded(ai) {
			logger.Info("AIAnalysis: WorkflowNotNeeded - delegating to handler")
			return r.aiAnalysisHandler.HandleAIAnalysisStatus(ctx, rr, ai)
		}

		// Check for approval required (BR-ORCH-026)
		if ai.Status.ApprovalRequired {
			logger.Info("AIAnalysis completed with approval required")

			// Create RemediationApprovalRequest (BR-ORCH-026)
			rarName, err := r.approvalCreator.Create(ctx, rr, ai)
			if err != nil {
				logger.Error(err, "Failed to create RemediationApprovalRequest")
				return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
			}
			logger.Info("Created RemediationApprovalRequest", "rarName", rarName)

			// DD-EVENT-001: Emit K8s event for approval required (BR-ORCH-095)
			if r.Recorder != nil {
				r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonApprovalRequired,
					fmt.Sprintf("Human approval required: confidence %.2f below threshold", ai.Status.SelectedWorkflow.Confidence))
			}

			// Create approval notification (BR-ORCH-001)
			result, err := r.aiAnalysisHandler.HandleAIAnalysisStatus(ctx, rr, ai)
			if err != nil {
				return result, err
			}

			// Per DD-AUDIT-003: Emit approval requested event
			// BUSINESS OUTCOME (BR-ORCH-001 AC-001-6): Exactly 1 audit event per approval request
			// Idempotency: Capture old phase, only emit if actually transitioning
			// This prevents duplicate emissions on reconcile retries
			oldPhaseBeforeTransition := rr.Status.OverallPhase

			// Transition to AwaitingApproval (RAR will be found by deterministic name)
			transitionResult, transitionErr := r.transitionPhase(ctx, rr, phase.AwaitingApproval)

			// Emit audit only if transition actually happened (wasn't already in AwaitingApproval)
			if transitionErr == nil && oldPhaseBeforeTransition != phase.AwaitingApproval {
				r.emitApprovalRequestedAudit(ctx, rr, ai.Status.SelectedWorkflow.Confidence, ai.Status.SelectedWorkflow.WorkflowID, ai.Status.ApprovalReason)
			}

			return transitionResult, transitionErr
		}

		// Normal completion - check routing conditions before creating WorkflowExecution
		logger.Info("AIAnalysis completed, checking routing conditions")

		// V1.0: Check routing conditions (DD-RO-002)
		// This checks for blocking conditions BEFORE creating WorkflowExecution:
		// - ConsecutiveFailures (BR-ORCH-042)
		// - DuplicateInProgress (DD-RO-002-ADDENDUM)
		// - ResourceBusy (DD-RO-002)
		// - RecentlyRemediated (DD-RO-002 Check 4) - workflow-specific cooldown
		// - ExponentialBackoff (DD-WE-004)
		//
		// DD-RO-002 Check 4: Pass workflow ID for workflow-specific cooldown.
		// Only blocks if SAME workflow was recently executed on same target.
		workflowID := ""
		if ai.Status.SelectedWorkflow != nil {
			workflowID = ai.Status.SelectedWorkflow.WorkflowID
		}
		blocked, err := r.routingEngine.CheckBlockingConditions(ctx, rr, workflowID)
		if err != nil {
			logger.Error(err, "Failed to check routing conditions")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
		}

		// If blocked, update status and requeue
		if blocked != nil {
			logger.Info("Routing blocked - will not create WorkflowExecution",
				"reason", blocked.Reason,
				"message", blocked.Message,
				"requeueAfter", blocked.RequeueAfter)
			return r.handleBlocked(ctx, rr, blocked, string(remediationv1.PhaseAnalyzing), workflowID)
		}

		// Routing checks passed - create WorkflowExecution
		logger.Info("Routing checks passed, creating WorkflowExecution")

		// ADR-EM-001, GAP-RO-1: Emit remediation.workflow_created BEFORE WFE creation
		// Captures pre-remediation spec hash for EM comparison (DD-EM-002)
		r.emitWorkflowCreatedAudit(ctx, rr, ai)

		// Create WorkflowExecution CRD (BR-ORCH-025, BR-ORCH-031)
		// DD-CRD-002-RR: Creator sets WorkflowExecutionReady condition in-memory
		weName, err := r.weCreator.Create(ctx, rr, ai)
		if err != nil {
			logger.Error(err, "Failed to create WorkflowExecution CRD")
			// ========================================
			// DD-PERF-001: ATOMIC STATUS UPDATE
			// Persist WorkflowExecutionReady=False condition
			// ========================================
			if updateErr := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
				// Re-set condition after refetch (creator set it before, but refetch wiped it)
				remediationrequest.SetWorkflowExecutionReady(rr, false,
					fmt.Sprintf("Failed to create WorkflowExecution: %v", err), r.Metrics)
				return nil
			}); updateErr != nil {
				logger.Error(updateErr, "Failed to update WorkflowExecutionReady condition")
			}
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.Info("Created WorkflowExecution CRD", "weName", weName)

		// BR-ORCH-044: Track child CRD creation
		r.Metrics.ChildCRDCreationsTotal.WithLabelValues("WorkflowExecution", rr.Namespace).Inc()

		// Set WorkflowExecutionRef in status for aggregator (BR-ORCH-029)
		// REFACTOR-RO-001: Using retry helper
		// DD-CRD-002-RR: Also persists WorkflowExecutionReady=True condition set by creator
		err = helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				APIVersion: workflowexecutionv1.GroupVersion.String(),
				Kind:       "WorkflowExecution",
				Name:       weName,
				Namespace:  rr.Namespace,
			}
			// Preserve WorkflowExecutionReady condition from creator
			rrconditions.SetWorkflowExecutionReady(rr, true, fmt.Sprintf("WorkflowExecution CRD %s created successfully", weName), r.Metrics)
			return nil
		})
		if err != nil {
			logger.Error(err, "Failed to set WorkflowExecutionRef in status")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.V(1).Info("Set WorkflowExecutionRef in status", "weName", weName)

		// Transition to Executing phase
		return r.transitionPhase(ctx, rr, phase.Executing)

	case "Failed":
		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Set AIAnalysisComplete condition
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetAIAnalysisComplete(rr, false,
				remediationrequest.ReasonAIAnalysisFailed,
				"AIAnalysis failed", r.Metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update AIAnalysisComplete condition")
			// Continue - condition update is best-effort
		}

		// DD-EVENT-001: Emit EscalatedToManualReview if AI needs human review (BR-ORCH-095)
		// Emitted from reconciler (handler doesn't have access to Recorder)
		if ai.Status.NeedsHumanReview && r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonEscalatedToManualReview,
				fmt.Sprintf("AI analysis requires manual review: %s", ai.Status.Message))
		}

		// Handle all failure scenarios (BR-ORCH-036: manual review)
		logger.Info("AIAnalysis failed - delegating to handler")
		return r.aiAnalysisHandler.HandleAIAnalysisStatus(ctx, rr, ai)

	case "Pending", "Investigating", "Analyzing":
		// Still in progress
		logger.V(1).Info("AIAnalysis in progress", "phase", ai.Status.Phase)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	default:
		logger.Info("Unknown AIAnalysis phase", "phase", ai.Status.Phase)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

// handleAwaitingApprovalPhase handles the AwaitingApproval phase.
// Waits for human approval before proceeding.
// V1.0: Operator approves via `kubectl patch rar <name> --subresource=status -p '{"status":{"decision":"Approved"}}'`
// Audit trail: K8s audit log captures who made the patch.
// V1.1: Will add CEL validation requiring decidedBy when decision is set.
// Reference: ADR-040, BR-ORCH-026
func (r *Reconciler) handleAwaitingApprovalPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Check if RemediationApprovalRequest exists
	rarName := fmt.Sprintf("rar-%s", rr.Name)
	rar := &remediationv1.RemediationApprovalRequest{}
	err := r.client.Get(ctx, client.ObjectKey{Name: rarName, Namespace: rr.Namespace}, rar)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// RAR should have been created when transitioning to AwaitingApproval
			// This is unexpected - log warning and requeue
			logger.Info("RemediationApprovalRequest not found, will be created by approval handler")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.Error(err, "Failed to get RemediationApprovalRequest")
		return ctrl.Result{}, err
	}

	// Check the decision
	switch rar.Status.Decision {
	case remediationv1.ApprovalDecisionApproved:
		logger.Info("Approval granted via RemediationApprovalRequest",
			"decidedBy", rar.Status.DecidedBy,
			"message", rar.Status.DecisionMessage,
		)

		// Per DD-AUDIT-003: Emit approval decision event
		r.emitApprovalDecisionAudit(ctx, rr, "Approved", rar.Status.DecidedBy, rar.Status.DecisionMessage)

		// DD-EVENT-001: Emit K8s event for approval granted (BR-ORCH-095)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonApprovalGranted,
				fmt.Sprintf("Approval granted by %s", rar.Status.DecidedBy))
		}

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE (RAR)
		// Consolidate 2 conditions → 1 API call with retry
		// ========================================
		if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			// Refetch RAR for latest resourceVersion
			if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
				return err
			}
			// Set conditions after refetch
			remediationapprovalrequest.SetApprovalPending(rar, false, "Decision received", r.Metrics)
			remediationapprovalrequest.SetApprovalDecided(rar, true,
				remediationapprovalrequest.ReasonApproved,
				fmt.Sprintf("Approved by %s", rar.Status.DecidedBy), r.Metrics)
			return r.client.Status().Update(ctx, rar)
		}); err != nil {
			logger.Error(err, "Failed to update RAR conditions")
			// Continue - condition update is best-effort
		}

		// Fetch AIAnalysis CRD to get workflow details for WorkflowExecution
		ai := &aianalysisv1.AIAnalysis{}
		err := r.client.Get(ctx, client.ObjectKey{
			Name:      rr.Status.AIAnalysisRef.Name,
			Namespace: rr.Status.AIAnalysisRef.Namespace,
		}, ai)
		if err != nil {
			logger.Error(err, "Failed to fetch AIAnalysis CRD")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}

		// ADR-EM-001, GAP-RO-1: Emit remediation.workflow_created BEFORE WFE creation
		r.emitWorkflowCreatedAudit(ctx, rr, ai)

		// Create WorkflowExecution CRD (BR-ORCH-025, BR-ORCH-031)
		weName, err := r.weCreator.Create(ctx, rr, ai)
		if err != nil {
			logger.Error(err, "Failed to create WorkflowExecution CRD")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.Info("Created WorkflowExecution CRD after approval", "weName", weName)

		// BR-ORCH-044: Track child CRD creation
		r.Metrics.ChildCRDCreationsTotal.WithLabelValues("WorkflowExecution", rr.Namespace).Inc()

		// Set WorkflowExecutionRef in status for aggregator (BR-ORCH-029)
		// REFACTOR-RO-001: Using retry helper
		err = helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				APIVersion: workflowexecutionv1.GroupVersion.String(),
				Kind:       "WorkflowExecution",
				Name:       weName,
				Namespace:  rr.Namespace,
			}
			return nil
		})
		if err != nil {
			logger.Error(err, "Failed to set WorkflowExecutionRef in status")
			return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil // REFACTOR-RO-003
		}
		logger.V(1).Info("Set WorkflowExecutionRef in status after approval", "weName", weName)

		// Transition to Executing phase
		return r.transitionPhase(ctx, rr, phase.Executing)

	case remediationv1.ApprovalDecisionRejected:
		logger.Info("Approval rejected via RemediationApprovalRequest",
			"decidedBy", rar.Status.DecidedBy,
			"message", rar.Status.DecisionMessage,
		)

		// Per DD-AUDIT-003: Emit approval decision event
		r.emitApprovalDecisionAudit(ctx, rr, "Rejected", rar.Status.DecidedBy, rar.Status.DecisionMessage)

		// DD-EVENT-001: Emit K8s event for approval rejected (BR-ORCH-095)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonApprovalRejected,
				fmt.Sprintf("Approval rejected by %s: %s", rar.Status.DecidedBy, rar.Status.DecisionMessage))
		}

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE (RAR)
		// Consolidate 2 conditions → 1 API call with retry
		// ========================================
		if err := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
			// Refetch RAR for latest resourceVersion
			if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
				return err
			}
			// Set conditions after refetch
			remediationapprovalrequest.SetApprovalPending(rar, false, "Decision received", r.Metrics)
			remediationapprovalrequest.SetApprovalDecided(rar, true,
				remediationapprovalrequest.ReasonRejected,
				fmt.Sprintf("Rejected by %s: %s", rar.Status.DecidedBy, rar.Status.DecisionMessage), r.Metrics)
			return r.client.Status().Update(ctx, rar)
		}); err != nil {
			logger.Error(err, "Failed to update RAR conditions")
		}

		reason := "Rejected by operator"
		if rar.Status.DecisionMessage != "" {
			reason = rar.Status.DecisionMessage
		}
		return r.transitionToFailed(ctx, rr, "approval", fmt.Errorf("%s", reason))

	case remediationv1.ApprovalDecisionExpired:
		logger.Info("Approval expired (timeout)")

		// DD-EVENT-001: Emit K8s event for approval expired (BR-ORCH-095)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonApprovalExpired,
				"Approval request expired without a decision")
		}

		return r.transitionToFailed(ctx, rr, "approval", fmt.Errorf("Approval request expired (timeout)"))

	default:
		// Still pending - check if deadline passed (V1.0 timeout handling)
		if time.Now().After(rar.Spec.RequiredBy.Time) {
			logger.Info("Approval deadline passed, marking as expired")

			// DD-EVENT-001: Emit K8s event for approval expired (BR-ORCH-095)
			if r.Recorder != nil {
				r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonApprovalExpired,
					fmt.Sprintf("Approval deadline passed after %v without decision",
						time.Since(rar.ObjectMeta.CreationTimestamp.Time).Round(time.Minute)))
			}

			// ========================================
			// DD-PERF-001: ATOMIC STATUS UPDATE (RAR)
			// Consolidate: 2 conditions + Decision + DecidedBy + DecidedAt → 1 API call with retry
			// ========================================
			if updateErr := k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				// Refetch RAR for latest resourceVersion
				if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
					return err
				}
				// Set conditions and status fields after refetch
				remediationapprovalrequest.SetApprovalPending(rar, false, "Expired without decision", r.Metrics)
				remediationapprovalrequest.SetApprovalExpired(rar, true,
					fmt.Sprintf("Expired after %v without decision",
						time.Since(rar.ObjectMeta.CreationTimestamp.Time).Round(time.Minute)), r.Metrics)
				rar.Status.Decision = remediationv1.ApprovalDecisionExpired
				rar.Status.DecidedBy = "system"
				now := metav1.Now()
				rar.Status.DecidedAt = &now
				return r.client.Status().Update(ctx, rar)
			}); updateErr != nil {
				logger.Error(updateErr, "Failed to update RAR status to Expired")
			}
			return r.transitionToFailed(ctx, rr, "approval", fmt.Errorf("Approval request expired (timeout)"))
		}

		// Still waiting for approval
		logger.V(1).Info("Waiting for approval decision",
			"rarName", rarName,
			"requiredBy", rar.Spec.RequiredBy.Format(time.RFC3339),
		)
		return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil // REFACTOR-RO-003
	}
}

// handleExecutingPhase handles the Executing phase.
// Waits for WorkflowExecution to complete.
func (r *Reconciler) handleExecutingPhase(ctx context.Context, rr *remediationv1.RemediationRequest, agg *aggregator.AggregatedStatus) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "wePhase", agg.WorkflowExecutionPhase)

	// First, check if we're in Executing but have no WE ref (corrupted state)
	if rr.Status.WorkflowExecutionRef == nil {
		logger.Error(nil, "Executing phase but no WorkflowExecutionRef - corrupted state")
		return r.transitionToFailed(ctx, rr, "workflow_execution", fmt.Errorf("WorkflowExecution not found"))
	}

	// Check if child is missing (AllChildrenHealthy=false)
	if agg.WorkflowExecutionPhase == "" && !agg.AllChildrenHealthy {
		logger.Error(nil, "WorkflowExecution CRD not found",
			"workflowExecutionRef", rr.Status.WorkflowExecutionRef.Name)
		return r.transitionToFailed(ctx, rr, "workflow_execution", fmt.Errorf("WorkflowExecution not found"))
	}

	// Fetch WorkflowExecution CRD
	we := &workflowexecutionv1.WorkflowExecution{}
	err := r.client.Get(ctx, client.ObjectKey{
		Name:      rr.Status.WorkflowExecutionRef.Name,
		Namespace: rr.Status.WorkflowExecutionRef.Namespace,
	}, we)
	if err != nil {
		logger.Error(err, "Failed to fetch WorkflowExecution CRD")
		return r.transitionToFailed(ctx, rr, "workflow_execution", fmt.Errorf("WorkflowExecution not found: %w", err))
	}

	// Set WorkflowExecutionComplete condition before delegating to handler
	// DD-PERF-001: Atomic status updates with conditions
	if we.Status.Phase == workflowexecutionv1.PhaseCompleted {
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetWorkflowExecutionComplete(rr, true,
				remediationrequest.ReasonWorkflowSucceeded,
				"WorkflowExecution completed successfully", r.Metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update WorkflowExecutionComplete condition")
			// Continue - condition update is best-effort
		}
	} else if we.Status.Phase == workflowexecutionv1.PhaseFailed {
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			remediationrequest.SetWorkflowExecutionComplete(rr, false,
				remediationrequest.ReasonWorkflowFailed,
				"WorkflowExecution failed", r.Metrics)
			return nil
		}); err != nil {
			logger.Error(err, "Failed to update WorkflowExecutionComplete condition")
			// Continue - condition update is best-effort
		}
	}

	// Delegate to WorkflowExecutionHandler for status-based transitions
	// Handler Consistency Refactoring (2026-01-22): Extract status handling logic
	return r.weHandler.HandleStatus(ctx, rr, we)
}

// handleBlocked updates RR status when routing is blocked and requeues appropriately.
// This function is called when CheckBlockingConditions() finds a blocking condition.
//
// Reference: DD-RO-002 (Centralized Routing), DD-RO-002-ADDENDUM (Blocked Phase Semantics)
func (r *Reconciler) handleBlocked(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	blocked *routing.BlockingCondition,
	fromPhase string,
	workflowID string,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"blockReason", blocked.Reason,
		"blockMessage", blocked.Message,
	)

	// Emit routing blocked audit event (DD-RO-002, ADR-032 §1)
	r.emitRoutingBlockedAudit(ctx, rr, fromPhase, blocked, workflowID)

	// DD-EVENT-001: Emit K8s event based on blocking reason (BR-ORCH-095)
	if r.Recorder != nil {
		switch remediationv1.BlockReason(blocked.Reason) {
		case remediationv1.BlockReasonRecentlyRemediated, remediationv1.BlockReasonExponentialBackoff:
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonCooldownActive,
				fmt.Sprintf("Remediation deferred: %s", blocked.Message))
		case remediationv1.BlockReasonConsecutiveFailures:
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonConsecutiveFailureBlocked,
				fmt.Sprintf("Target blocked: %s", blocked.Message))
		}
	}

	// Update RR status to Blocked phase (REFACTOR-RO-001: using retry helper)
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseBlocked
		rr.Status.BlockReason = blocked.Reason
		rr.Status.BlockMessage = blocked.Message

		// Set time-based block fields
		if blocked.BlockedUntil != nil {
			rr.Status.BlockedUntil = &metav1.Time{Time: *blocked.BlockedUntil}
		} else {
			rr.Status.BlockedUntil = nil // Clear if not set
		}

		// Set WFE-based block fields
		if blocked.BlockingWorkflowExecution != "" {
			rr.Status.BlockingWorkflowExecution = blocked.BlockingWorkflowExecution
		} else {
			rr.Status.BlockingWorkflowExecution = "" // Clear if not set
		}

		// Set duplicate tracking
		if blocked.DuplicateOf != "" {
			rr.Status.DuplicateOf = blocked.DuplicateOf
		} else {
			rr.Status.DuplicateOf = "" // Clear if not set
		}

		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to update blocked status")
		return ctrl.Result{}, fmt.Errorf("failed to update blocked status: %w", err)
	}

	// BR-ORCH-042: Track blocking metrics (DD-METRICS-001)
	// Metric expects: []string{"namespace", "reason"}
	r.Metrics.BlockedTotal.WithLabelValues(rr.Namespace, blocked.Reason).Inc()

	// BR-ORCH-044: Track duplicate skips specifically
	if blocked.DuplicateOf != "" {
		r.Metrics.DuplicatesSkippedTotal.WithLabelValues(rr.Namespace, rr.Spec.SignalFingerprint).Inc()
	}

	// Emit metric (using existing metrics package)
	// V1.0: Basic counter, future enhancement: add duration histogram
	r.Metrics.PhaseTransitionsTotal.WithLabelValues(
		string(rr.Status.OverallPhase),     // from_phase
		string(remediationv1.PhaseBlocked), // to_phase
		rr.Namespace,
	).Inc()

	logger.Info("RemediationRequest blocked",
		"reason", blocked.Reason,
		"requeueAfter", blocked.RequeueAfter)

	// Requeue after specified duration
	return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
}

// transitionPhase transitions the RR to a new phase.
// REFACTOR-RO-001: Using retry helper for status updates (BR-ORCH-038).
func (r *Reconciler) transitionPhase(ctx context.Context, rr *remediationv1.RemediationRequest, newPhase phase.Phase) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name, "newPhase", newPhase)

	oldPhase := rr.Status.OverallPhase

	// ========================================
	// IDEMPOTENCY CHECK (Prevents Duplicate Audit Events)
	// Per RO_AUDIT_DUPLICATION_RISK_ANALYSIS_JAN_01_2026.md - Option C
	// ========================================
	// Without GenerationChangedPredicate, controller reconciles on annotation/label changes.
	// This check prevents duplicate audit emissions when phase hasn't actually changed.
	// ADR-032 §1: Audit integrity requires exactly-once emission per state change.
	if oldPhase == newPhase {
		logger.V(1).Info("Phase transition skipped - already in target phase",
			"currentPhase", oldPhase,
			"requestedPhase", newPhase)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Update only RO-owned fields (preserves Gateway fields per DD-GATEWAY-011)
		rr.Status.OverallPhase = newPhase
		rr.Status.ObservedGeneration = rr.Generation // DD-CONTROLLER-001: Track processed generation
		now := metav1.Now()

		// Set phase start times
		switch newPhase {
		case phase.Processing:
			rr.Status.ProcessingStartTime = &now
		case phase.Analyzing:
			rr.Status.AnalyzingStartTime = &now
		case phase.Executing:
			rr.Status.ExecutingStartTime = &now
		}

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition phase")
		return ctrl.Result{}, fmt.Errorf("failed to transition phase: %w", err)
	}

	// Record metric
	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(newPhase), rr.Namespace).Inc()
	}

	// Emit audit event (DD-AUDIT-003)
	r.emitPhaseTransitionAudit(ctx, rr, string(oldPhase), string(newPhase))

	// DD-EVENT-001: Emit K8s event for phase transition (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
			fmt.Sprintf("Phase transition: %s → %s", oldPhase, newPhase))
	}

	logger.Info("Phase transition successful", "from", oldPhase, "to", newPhase)

	// Requeue with delay to check progress of child CRDs
	// Different phases have different check intervals
	var requeueAfter time.Duration
	switch newPhase {
	case phase.Processing, phase.Analyzing, phase.Executing:
		// Check child CRD progress every 5 seconds
		requeueAfter = 5 * time.Second
	default:
		// Quick requeue for other phases (using RequeueAfter instead of deprecated Requeue)
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// transitionToCompleted transitions the RR to Completed phase.
func (r *Reconciler) transitionToCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	// IDEMPOTENCY: Check if already in Completed phase before attempting transition
	// This prevents duplicate transitions and audit emissions when multiple reconciles happen simultaneously
	if rr.Status.OverallPhase == phase.Completed {
		logger.V(1).Info("Already in Completed phase, skipping transition")
		return ctrl.Result{}, nil
	}

	// Capture old phase for metrics and audit
	oldPhaseBeforeTransition := rr.Status.OverallPhase
	startTime := rr.CreationTimestamp.Time

	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Completed
		rr.Status.Outcome = outcome
		rr.Status.ObservedGeneration = rr.Generation // DD-CONTROLLER-001: Track final generation
		now := metav1.Now()
		rr.Status.CompletedAt = &now

		// DD-WE-004 V1.0: Reset exponential backoff on success
		if rr.Status.NextAllowedExecution != nil {
			logger.Info("Clearing exponential backoff after successful completion",
				"previousNextAllowed", rr.Status.NextAllowedExecution.Format(time.RFC3339),
				"previousConsecutiveFailures", rr.Status.ConsecutiveFailureCount)
			rr.Status.NextAllowedExecution = nil
		}

		// Reset consecutive failure count (fresh start after success)
		rr.Status.ConsecutiveFailureCount = 0

		// BR-ORCH-043: Set RecoveryComplete condition (terminal state)
		remediationrequest.SetRecoveryComplete(rr, true,
			remediationrequest.ReasonRecoverySucceeded,
			fmt.Sprintf("Remediation completed successfully with outcome: %s", outcome), r.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Completed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Completed: %w", err)
	}

	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhaseBeforeTransition), string(phase.Completed), rr.Namespace).Inc()
	}

	// Emit audit event (DD-AUDIT-003)
	// IDEMPOTENCY: Only emit if phase actually changed (prevents duplicate events on reconcile retries)
	// Race condition protection: oldPhaseBeforeTransition captured before status update ensures
	// only the reconcile that successfully transitioned will emit the audit event
	if oldPhaseBeforeTransition != phase.Completed {
		durationMs := time.Since(startTime).Milliseconds()
		r.emitCompletionAudit(ctx, rr, outcome, durationMs)

		// DD-EVENT-001: Emit K8s event for successful completion (BR-ORCH-095)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonRemediationCompleted,
				fmt.Sprintf("Remediation completed successfully: %s", outcome))
		}
	}

	// ========================================
	// BR-ORCH-045: COMPLETION NOTIFICATION
	// Notify operators of successful remediation
	// ========================================
	if oldPhaseBeforeTransition != phase.Completed {
		// Fetch AIAnalysis for RCA and workflow context
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: aiName, Namespace: rr.Namespace}, ai); err != nil {
			// Non-fatal: Log and continue - completion succeeded even if notification fails
			logger.Error(err, "Failed to fetch AIAnalysis for completion notification", "aiAnalysis", aiName)
		} else {
			notifName, notifErr := r.notificationCreator.CreateCompletionNotification(ctx, rr, ai)
			if notifErr != nil {
				// Non-fatal: Log and continue - completion succeeded even if notification fails
				logger.Error(notifErr, "Failed to create completion notification")
			} else {
				logger.Info("Created completion notification", "notification", notifName)
				// Issue #88, BR-ORCH-035 AC-2: Append completion NT to NotificationRequestRefs
				// Without this, trackNotificationStatus cannot find the NT for delivery tracking.
				rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, corev1.ObjectReference{
					Name:      notifName,
					Namespace: rr.Namespace,
				})
				// DD-EVENT-001: Emit K8s event for notification creation (BR-ORCH-095)
				if r.Recorder != nil {
					r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
						fmt.Sprintf("Completion notification created: %s", notifName))
				}
			}
		}

		// BR-ORCH-034: Create bulk duplicate notification if duplicates exist
		if rr.Status.DuplicateCount > 0 {
			bulkName, bulkErr := r.notificationCreator.CreateBulkDuplicateNotification(ctx, rr)
			if bulkErr != nil {
				// Non-fatal: Log and continue
				logger.Error(bulkErr, "Failed to create bulk duplicate notification")
			} else {
				logger.Info("Created bulk duplicate notification", "notification", bulkName)
			}
		}
	}

	// ADR-EM-001: Create EffectivenessAssessment CRD (non-fatal)
	r.createEffectivenessAssessmentIfNeeded(ctx, rr)

	logger.Info("Remediation completed successfully", "outcome", outcome)
	return ctrl.Result{}, nil
}

// transitionToFailed transitions the RR to Failed phase.
// BR-ORCH-042: Before transitioning to terminal Failed, checks if this failure
// triggers consecutive failure blocking (≥3 consecutive failures for same fingerprint).
// If blocking is triggered, transitions to non-terminal Blocked phase instead.
func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase string, failureErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// F-6: Derive string reason from error for status fields and logging
	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	// BR-ORCH-042: Log consecutive failures for observability
	// NOTE: This RR transitions to Failed (terminal state).
	// FUTURE RRs with same fingerprint will be blocked in Pending phase (routing check).
	if failurePhase != "blocked" {
		// Count consecutive failures BEFORE this one (current failure not yet recorded)
		consecutiveFailures := r.countConsecutiveFailures(ctx, rr.Spec.SignalFingerprint)

		// +1 for this failure (not yet in status)
		if consecutiveFailures+1 >= DefaultBlockThreshold {
			logger.Info("Consecutive failure threshold reached, future RRs will be blocked",
				"consecutiveFailures", consecutiveFailures+1,
				"threshold", DefaultBlockThreshold,
				"fingerprint", rr.Spec.SignalFingerprint,
			)
			// Do NOT transition this RR to Blocked - it failed and should go to Failed.
			// The routing engine will block FUTURE RRs for this fingerprint.
		}
	}

	// Normal terminal Failed transition
	// IDEMPOTENCY: Check if already in Failed phase before attempting transition
	// This prevents duplicate transitions and audit emissions when multiple reconciles happen simultaneously
	if rr.Status.OverallPhase == phase.Failed {
		logger.V(1).Info("Already in Failed phase, skipping transition")
		return ctrl.Result{}, nil
	}

	// Capture old phase for metrics and audit
	oldPhaseBeforeTransition := rr.Status.OverallPhase
	startTime := rr.CreationTimestamp.Time

	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Failed
		rr.Status.ObservedGeneration = rr.Generation // DD-CONTROLLER-001: Track final generation
		rr.Status.FailurePhase = &failurePhase
		rr.Status.FailureReason = &failureReason

		// DD-WE-004 V1.0: Set exponential backoff for pre-execution failures
		// Only applies when BELOW consecutive failure threshold (at threshold → 1-hour fixed block)
		// Increment consecutive failures (this happens for all failures, not just pre-execution)
		rr.Status.ConsecutiveFailureCount++

		// Calculate and set exponential backoff if below threshold
		// (At threshold, routing engine's CheckConsecutiveFailures will block with fixed cooldown)
		if rr.Status.ConsecutiveFailureCount < int32(r.routingEngine.Config().ConsecutiveFailureThreshold) {
			// Calculate backoff: 1min → 2min → 4min → 8min → 10min (capped)
			backoff := r.routingEngine.CalculateExponentialBackoff(rr.Status.ConsecutiveFailureCount)
			if backoff > 0 {
				nextAllowed := metav1.NewTime(time.Now().Add(backoff))
				rr.Status.NextAllowedExecution = &nextAllowed
				logger.Info("Set exponential backoff for failure",
					"consecutiveFailures", rr.Status.ConsecutiveFailureCount,
					"backoff", backoff.Round(time.Second),
					"nextAllowedExecution", nextAllowed.Format(time.RFC3339))
			}
		}

		// BR-ORCH-043: Set RecoveryComplete condition (terminal failure state)
		remediationrequest.SetRecoveryComplete(rr, false,
			remediationrequest.ReasonRecoveryFailed,
			fmt.Sprintf("Remediation failed during %s: %s", failurePhase, failureReason), r.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Failed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Failed: %w", err)
	}

	// Labels order: from_phase, to_phase, namespace (per prometheus.go definition)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhaseBeforeTransition), string(phase.Failed), rr.Namespace).Inc()
	}

	// Emit audit event (DD-AUDIT-003)
	// IDEMPOTENCY: Only emit if phase actually changed (prevents duplicate events on reconcile retries)
	// Race condition protection: oldPhaseBeforeTransition captured before status update ensures
	// only the reconcile that successfully transitioned will emit the audit event
	if oldPhaseBeforeTransition != phase.Failed {
		durationMs := time.Since(startTime).Milliseconds()
		r.emitFailureAudit(ctx, rr, failurePhase, failureErr, durationMs)

		// DD-EVENT-001: Emit K8s event for failure (BR-ORCH-095)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationFailed,
				fmt.Sprintf("Remediation failed during %s: %s", failurePhase, failureReason))
		}
	}

	// ADR-EM-001: Create EffectivenessAssessment CRD (non-fatal)
	r.createEffectivenessAssessmentIfNeeded(ctx, rr)

	logger.Info("Remediation failed", "failurePhase", failurePhase, "reason", failureReason)
	return ctrl.Result{}, nil
}

// handleGlobalTimeout transitions the RR to TimedOut phase when global timeout exceeded.
// BR-ORCH-027: Global Timeout Management
// Business Value: Prevents stuck remediations from consuming resources indefinitely
// Default timeout: 1 hour from CreationTimestamp
func (r *Reconciler) handleGlobalTimeout(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// Record which phase timed out for troubleshooting
	timeoutPhase := string(rr.Status.OverallPhase)
	oldPhase := rr.Status.OverallPhase

	// Update status to TimedOut (BR-ORCH-027)
	// REFACTOR-RO-001: Using retry helper for optimistic concurrency
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseTimedOut
		now := metav1.Now()
		rr.Status.TimeoutTime = &now
		rr.Status.TimeoutPhase = &timeoutPhase
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to TimedOut")
		return ctrl.Result{}, fmt.Errorf("failed to transition to TimedOut: %w", err)
	}

	// Record metrics (BR-ORCH-044)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(remediationv1.PhaseTimedOut), rr.Namespace).Inc()
		r.Metrics.TimeoutsTotal.WithLabelValues(rr.Namespace, timeoutPhase).Inc() // BR-ORCH-044: Track timeout occurrences
	}

	// Per DD-AUDIT-003: Emit timeout event (lifecycle.completed with outcome=failure)
	if rr.Status.StartTime != nil {
		durationMs := time.Since(rr.Status.StartTime.Time).Milliseconds()
		r.emitTimeoutAudit(ctx, rr, "global", timeoutPhase, durationMs)
	}

	// DD-EVENT-001: Emit K8s event for global timeout (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationTimeout,
			fmt.Sprintf("Global timeout exceeded during %s phase", timeoutPhase))
	}

	logger.Info("Remediation timed out (global timeout exceeded)",
		"timeoutPhase", timeoutPhase,
		"creationTimestamp", rr.CreationTimestamp)

	// ========================================
	// CREATE TIMEOUT NOTIFICATION (BR-ORCH-027)
	// Business Value: Operators notified for manual intervention
	// ========================================

	// Create notification for timeout escalation
	notificationName := fmt.Sprintf("timeout-%s", rr.Name)
	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      notificationName,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "timeout",
				"kubernaut.ai/severity":            rr.Spec.Severity,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: notificationv1.NotificationPriorityCritical,
			Subject:  fmt.Sprintf("Remediation Timeout: %s", rr.Spec.SignalName),
			Body: fmt.Sprintf(`Remediation request has exceeded the global timeout and requires manual intervention.

**Signal**: %s
**Timeout Phase**: %s
**Timeout Duration**: %s
**Started**: %v
**Timed Out**: %v

The remediation was in %s phase when it timed out. Please investigate why the remediation did not complete within the expected timeframe.`,
				rr.Spec.SignalName,
				timeoutPhase,
				r.getEffectiveGlobalTimeout(rr).String(),
				rr.Status.StartTime.Format(time.RFC3339),
				rr.Status.TimeoutTime.Format(time.RFC3339),
				timeoutPhase,
			),
			Channels: []notificationv1.Channel{
				notificationv1.ChannelSlack,
				notificationv1.ChannelEmail,
			},
			Metadata: map[string]string{
				"remediationRequest": rr.Name,
				"timeoutPhase":       timeoutPhase,
				"severity":           rr.Spec.Severity,
				"targetResource":     fmt.Sprintf("%s/%s", rr.Spec.TargetResource.Kind, rr.Spec.TargetResource.Name),
			},
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference on timeout notification")
		// Continue without notification - timeout transition is primary goal
		return ctrl.Result{}, nil
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, nr, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on timeout notification")
		// Log error but don't fail timeout transition - timeout is primary goal
		return ctrl.Result{}, nil
	}

	// Create notification (non-blocking - timeout transition is primary goal)
	if err := r.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create timeout notification",
			"notificationName", notificationName)
		// Don't return error - timeout transition succeeded, notification is best-effort
		return ctrl.Result{}, nil
	}

	logger.Info("Created timeout notification",
		"notificationName", notificationName,
		"priority", nr.Spec.Priority,
		"timeoutPhase", timeoutPhase)

	// DD-EVENT-001: Emit K8s event for notification creation (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
			fmt.Sprintf("Timeout notification created: %s", notificationName))
	}

	// Track notification in status (Recommendation #2, BR-ORCH-035)
	// REFACTOR-RO-001: Using retry helper
	err = helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Add notification to tracking list (BR-ORCH-035)
		notifRef := corev1.ObjectReference{
			Kind:       "NotificationRequest",
			Namespace:  nr.Namespace,
			Name:       nr.Name,
			UID:        nr.UID,
			APIVersion: "notification.kubernaut.ai/v1alpha1",
		}
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, notifRef)
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to track notification in status (non-critical)",
			"notificationName", notificationName)
		// Don't fail - notification was created successfully, tracking is best-effort
	} else {
		logger.V(1).Info("Tracked notification in status",
			"notificationName", notificationName,
			"totalNotifications", len(rr.Status.NotificationRequestRefs)+1)
	}

	// ADR-EM-001: Create EffectivenessAssessment CRD (non-fatal)
	r.createEffectivenessAssessmentIfNeeded(ctx, rr)

	return ctrl.Result{}, nil
}

// createEffectivenessAssessmentIfNeeded creates an EA CRD if the eaCreator is wired.
// ADR-EM-001: EA creation is ALWAYS non-fatal. The terminal phase transition must succeed
// even if EA creation fails. Errors are logged but not propagated.
func (r *Reconciler) createEffectivenessAssessmentIfNeeded(ctx context.Context, rr *remediationv1.RemediationRequest) {
	if r.eaCreator == nil {
		return
	}

	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	name, err := r.eaCreator.CreateEffectivenessAssessment(ctx, rr)
	if err != nil {
		logger.Error(err, "Failed to create EffectivenessAssessment (non-fatal per ADR-EM-001)")
		return
	}
	logger.Info("EffectivenessAssessment created", "eaName", name, "rrPhase", rr.Status.OverallPhase)
}

// ========================================
// AUDIT EVENT EMISSION (DD-AUDIT-003)
// ========================================

// emitRemediationCreatedAudit emits an audit event for RemediationRequest creation with TimeoutConfig.
// Per BR-AUDIT-005 Gap #8: Captures initial TimeoutConfig for RR reconstruction.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per ADR-034: orchestrator.lifecycle.created event
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitRemediationCreatedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "orchestrator.lifecycle.created")
		// Note: In production, this never happens due to main.go:128 crash check.
		// If we reach here, it's a programming error (e.g., test misconfiguration).
		return
	}

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Convert TimeoutConfig for audit event (Gap #8)
	// Direct pointer assignment - roaudit.TimeoutConfig uses same *metav1.Duration type
	var auditTimeoutConfig *roaudit.TimeoutConfig
	if rr.Status.TimeoutConfig != nil {
		auditTimeoutConfig = &roaudit.TimeoutConfig{
			Global:     rr.Status.TimeoutConfig.Global,
			Processing: rr.Status.TimeoutConfig.Processing,
			Analyzing:  rr.Status.TimeoutConfig.Analyzing,
			Executing:  rr.Status.TimeoutConfig.Executing,
		}
	}

	event, err := r.auditManager.BuildRemediationCreatedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		auditTimeoutConfig,
	)
	if err != nil {
		logger.Error(err, "Failed to build remediation created audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store remediation created audit event")
	}
}

// emitWorkflowCreatedAudit emits the remediation.workflow_created audit event
// before WorkflowExecution creation. Captures the pre-remediation spec hash
// and selected workflow metadata for the EM to compare post-remediation.
// ADR-EM-001 Section 9.1, GAP-RO-1, DD-EM-002.
// Non-blocking — failures are logged but don't affect business logic.
func (r *Reconciler) emitWorkflowCreatedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record workflow_created audit event - violates ADR-032 §1",
			"remediationRequest", rr.Name)
		return
	}

	correlationID := rr.Name
	targetResource := fmt.Sprintf("%s/%s/%s",
		rr.Spec.TargetResource.Namespace,
		rr.Spec.TargetResource.Kind,
		rr.Spec.TargetResource.Name)

	// DD-EM-002: Capture pre-remediation spec hash via uncached reader
	preHash, err := CapturePreRemediationHash(
		ctx, r.apiReader, r.restMapper,
		rr.Spec.TargetResource.Kind,
		rr.Spec.TargetResource.Name,
		rr.Spec.TargetResource.Namespace,
	)
	if err != nil {
		logger.Error(err, "Failed to capture pre-remediation hash (non-fatal)")
	}

	// Extract workflow metadata from AIAnalysis status
	var workflowID, workflowVersion, workflowType string
	if ai.Status.SelectedWorkflow != nil {
		workflowID = ai.Status.SelectedWorkflow.WorkflowID
		workflowVersion = ai.Status.SelectedWorkflow.Version
		workflowType = ai.Status.SelectedWorkflow.ActionType
	}

	event, err := r.auditManager.BuildRemediationWorkflowCreatedEvent(
		correlationID, rr.Namespace, rr.Name,
		preHash, targetResource,
		workflowID, workflowVersion, workflowType,
	)
	if err != nil {
		logger.Error(err, "Failed to build workflow_created audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store workflow_created audit event")
	}
}

// emitLifecycleStartedAudit emits an audit event for remediation lifecycle started.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.lifecycle.started (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitLifecycleStartedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.started")
		// Note: In production, this never happens due to main.go:128 crash check.
		// If we reach here, it's a programming error (e.g., test misconfiguration).
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildLifecycleStartedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
	)
	if err != nil {
		logger.Error(err, "Failed to build lifecycle started audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store lifecycle started audit event")
	}
}

// emitPhaseTransitionAudit emits an audit event for phase transitions.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.phase.transitioned (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitPhaseTransitionAudit(ctx context.Context, rr *remediationv1.RemediationRequest, fromPhase, toPhase string) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "phase.transitioned",
			"fromPhase", fromPhase,
			"toPhase", toPhase)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildPhaseTransitionEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		fromPhase,
		toPhase,
	)
	if err != nil {
		logger.Error(err, "Failed to build phase transition audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store phase transition audit event")
	}
}

// emitCompletionAudit emits an audit event for remediation completion.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.lifecycle.completed (P1)
func (r *Reconciler) emitCompletionAudit(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string, durationMs int64) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.completed",
			"outcome", outcome,
			"durationMs", durationMs)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildCompletionEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		outcome,
		durationMs,
	)
	if err != nil {
		logger.Error(err, "Failed to build completion audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store completion audit event")
	}
}

// emitFailureAudit emits an audit event for remediation failure.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// This function assumes auditStore is non-nil, enforced by cmd/remediationorchestrator/main.go:128.
// Per DD-AUDIT-003: orchestrator.lifecycle.failed (P1)
func (r *Reconciler) emitFailureAudit(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase string, failureErr error, durationMs int64) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.failed",
			"failurePhase", failurePhase,
			"failureErr", failureErr,
			"durationMs", durationMs)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	event, err := r.auditManager.BuildFailureEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		failurePhase,
		failureErr,
		durationMs,
	)
	if err != nil {
		logger.Error(err, "Failed to build failure audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store failure audit event")
	}
}

// emitRoutingBlockedAudit emits an audit event for routing blocked decisions.
// Per DD-RO-002: Centralized Routing Engine blocking conditions.
// Per ADR-032 §1: All phase transitions must be audited (Pending/Analyzing → Blocked).
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitRoutingBlockedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, fromPhase string, blocked *routing.BlockingCondition, workflowID string) {
	logger := log.FromContext(ctx)

	// Per ADR-032 §2: Audit is MANDATORY - controller crashes at startup if nil.
	// This check should never trigger in production (defensive programming only).
	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "routing.blocked",
			"blockReason", blocked.Reason)
		// Note: In production, this never happens due to main.go:128 crash check.
		return
	}
	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Build routing blocked data
	requeueSeconds := int(blocked.RequeueAfter.Seconds())
	var blockedUntilStr *string
	if blocked.BlockedUntil != nil {
		str := blocked.BlockedUntil.Format(time.RFC3339)
		blockedUntilStr = &str
	}

	blockData := &roaudit.RoutingBlockedData{
		BlockReason:         blocked.Reason,
		BlockMessage:        blocked.Message,
		FromPhase:           fromPhase,
		ToPhase:             string(remediationv1.PhaseBlocked),
		WorkflowID:          workflowID,
		TargetResource:      rr.Spec.TargetResource.String(),
		RequeueAfterSeconds: requeueSeconds,
		BlockedUntil:        blockedUntilStr,
		BlockingWFE:         blocked.BlockingWorkflowExecution,
		DuplicateOf:         blocked.DuplicateOf,
		ConsecutiveFailures: rr.Status.ConsecutiveFailureCount,
	}

	// Calculate backoff seconds if NextAllowedExecution is set
	if rr.Status.NextAllowedExecution != nil {
		backoff := time.Until(rr.Status.NextAllowedExecution.Time)
		if backoff > 0 {
			blockData.BackoffSeconds = int(backoff.Seconds())
		}
	}

	event, err := r.auditManager.BuildRoutingBlockedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		fromPhase,
		blockData,
	)
	if err != nil {
		logger.Error(err, "Failed to build routing blocked audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store routing blocked audit event")
	}
}

// emitApprovalRequestedAudit emits an audit event for approval requested.
// Per DD-AUDIT-003: orchestrator.approval.requested (P2)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitApprovalRequestedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, confidence float64, workflowID, approvalReason string) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "approval.requested")
		return
	}

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Calculate RAR name using deterministic naming pattern
	rarName := fmt.Sprintf("rar-%s", rr.Name)

	// Calculate requiredBy timestamp (7 days from now per ADR-040)
	requiredBy := metav1.Now().Add(7 * 24 * time.Hour)

	// Build event using audit manager's build method (refactored per TODO comment)
	event, err := r.auditManager.BuildApprovalRequestedEvent(
		correlationID,
		rr.Namespace,
		rr.Name,
		rarName,
		workflowID,
		fmt.Sprintf("%.2f", confidence),
		requiredBy,
	)
	if err != nil {
		logger.Error(err, "Failed to build approval requested audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store approval requested audit event")
	}
}

// emitApprovalDecisionAudit emits an audit event for approval decision.
// Per DD-AUDIT-003: orchestrator.approval.approved or orchestrator.approval.rejected (P2)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitApprovalDecisionAudit(ctx context.Context, rr *remediationv1.RemediationRequest, decision, decidedBy, decisionMessage string) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", fmt.Sprintf("approval.%s", strings.ToLower(decision)))
		return
	}

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Determine event type and outcome based on decision
	var eventType string
	var outcome api.AuditEventRequestEventOutcome
	var action string

	switch decision {
	case "Approved":
		eventType = roaudit.EventTypeApprovalApproved
		outcome = audit.OutcomeSuccess
		action = roaudit.ActionApproved
	case "Rejected":
		eventType = roaudit.EventTypeApprovalRejected
		outcome = audit.OutcomeFailure
		action = roaudit.ActionRejected
	default:
		logger.Info("Unknown approval decision", "decision", decision)
		return
	}

	// Use audit helper to create event with proper timestamp (DD-AUDIT-002 V2.0)
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, eventType)
	audit.SetEventCategory(event, roaudit.CategoryOrchestration)
	audit.SetEventAction(event, action)
	audit.SetEventOutcome(event, outcome)
	audit.SetActor(event, "user", decidedBy)
	audit.SetResource(event, "RemediationRequest", rr.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, rr.Namespace)

	// Build payload using ogen types
	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventType(eventType),
		RrName:    rr.Name,
		Namespace: rr.Namespace,
		Decision:  roaudit.ToOptDecision(decision), // ToOptDecision returns Opt type, assign directly
	}
	if decision == "Approved" {
		payload.ApprovedBy.SetTo(decidedBy)
	} else {
		payload.RejectedBy.SetTo(decidedBy)
		payload.RejectionReason.SetTo(decisionMessage)
	}

	// Use the correct union constructor based on decision
	if decision == "Approved" {
		event.EventData = api.NewAuditEventRequestEventDataOrchestratorApprovalApprovedAuditEventRequestEventData(payload)
	} else {
		event.EventData = api.NewAuditEventRequestEventDataOrchestratorApprovalRejectedAuditEventRequestEventData(payload)
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store approval decision audit event", "decision", decision)
	}
}

// emitTimeoutAudit emits an audit event for global or phase timeout.
// Per DD-AUDIT-003: orchestrator.lifecycle.completed with outcome=failure (P1)
// Non-blocking - failures are logged but don't affect business logic.
func (r *Reconciler) emitTimeoutAudit(ctx context.Context, rr *remediationv1.RemediationRequest, timeoutType, timeoutPhase string, durationMs int64) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record audit event - violates ADR-032 §1 mandatory requirement",
			"remediationRequest", rr.Name,
			"namespace", rr.Namespace,
			"event", "lifecycle.completed.timeout")
		return
	}

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) as correlation ID
	// Per universal standard: All services use RemediationRequest.Name for audit correlation
	correlationID := rr.Name

	// Use audit helper to create event with proper timestamp (DD-AUDIT-002 V2.0)
	// Reuse lifecycle.completed event type with outcome=failure for timeouts
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, roaudit.EventTypeLifecycleCompleted)
	audit.SetEventCategory(event, roaudit.CategoryOrchestration)
	audit.SetEventAction(event, roaudit.ActionCompleted)
	audit.SetEventOutcome(event, audit.OutcomeFailure)
	audit.SetActor(event, "service", roaudit.ServiceName)
	audit.SetResource(event, "RemediationRequest", rr.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, rr.Namespace)
	audit.SetDuration(event, int(durationMs))

	// Build payload using ogen types (timeout is represented as failure)
	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted,
		RrName:    rr.Name,
		Namespace: rr.Namespace,
	}
	payload.FailurePhase = roaudit.ToOptFailurePhase(timeoutPhase)
	payload.FailureReason = roaudit.ToOptFailureReason(fmt.Sprintf("%s timeout", timeoutType))
	payload.DurationMs.SetTo(durationMs)

	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData(payload)

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store timeout audit event", "timeoutType", timeoutType)
	}
}

// SetupWithManager sets up the controller with the Manager.
// Creates field index on spec.signalFingerprint for O(1) consecutive failure lookups.
// Reference: BR-ORCH-042, BR-GATEWAY-185 v1.1
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// BR-ORCH-042, BR-GATEWAY-185 v1.1: Create field index on spec.signalFingerprint
	// Uses immutable spec field (64 chars) instead of mutable labels (63 chars max)
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&remediationv1.RemediationRequest{},
		FingerprintFieldIndex, // "spec.signalFingerprint"
		func(obj client.Object) []string {
			rr := obj.(*remediationv1.RemediationRequest)
			if rr.Spec.SignalFingerprint == "" {
				return nil
			}
			return []string{rr.Spec.SignalFingerprint}
		},
	); err != nil {
		return fmt.Errorf("failed to create field index on spec.signalFingerprint: %w", err)
	}

	// ========================================
	// V1.0: FIELD INDEX FOR CENTRALIZED ROUTING (DD-RO-002)
	// Index WorkflowExecution by spec.targetResource for efficient routing queries
	// Reference: DD-RO-002, V1.0 Implementation Plan Day 1
	// ========================================
	// Create field index on WorkflowExecution.Spec.TargetResource for O(1) routing lookups
	// Used by RO routing logic to find recent/active WFEs for same target
	// Enables efficient queries: "Find all WFEs targeting deployment/my-app"
	// Pattern from WE controller (lines 508-518)
	//
	// NOTE: This index may already exist if WE controller was set up first.
	// If the index already exists, we can safely ignore the "indexer conflict" error
	// since both controllers need the same index for the same purpose.
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&workflowexecutionv1.WorkflowExecution{},
		"spec.targetResource", // Field to index
		func(obj client.Object) []string {
			wfe := obj.(*workflowexecutionv1.WorkflowExecution)
			if wfe.Spec.TargetResource == "" {
				return nil
			}
			return []string{wfe.Spec.TargetResource}
		},
	); err != nil {
		// Ignore "indexer conflict" error - means WE controller already created this index
		// Both controllers need this index, so if it exists, we're good
		if !k8serrors.IsIndexerConflict(err) {
			return fmt.Errorf("failed to create field index on WorkflowExecution.spec.targetResource: %w", err)
		}
		// Index already exists - safe to continue
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationRequest{}).
		Owns(&signalprocessingv1.SignalProcessing{}).
		Owns(&aianalysisv1.AIAnalysis{}).
		Owns(&workflowexecutionv1.WorkflowExecution{}).
		Owns(&remediationv1.RemediationApprovalRequest{}).
		Owns(&notificationv1.NotificationRequest{}). // BR-ORCH-029/030: Watch notification lifecycle
		// V1.0 P1 FIX: GenerationChangedPredicate removed to allow child CRD status changes
		// Previous optimization filtered status updates, breaking integration tests (RO_INTEGRATION_CRITICAL_BUG_JAN_01_2026.md)
		// Rationale: Correctness > Performance for P0 orchestration service
		// WithEventFilter(predicate.GenerationChangedPredicate{}). // ❌ REMOVED - breaks integration tests
		Complete(r)
}

// ========================================
// TIMEOUT HELPER METHODS (BR-ORCH-027/028)
// ========================================

// safeFormatTime safely formats a time, returning "N/A" if nil.
func safeFormatTime(t *metav1.Time) string {
	if t == nil {
		return "N/A"
	}
	return t.Format(time.RFC3339)
}

// getEffectiveGlobalTimeout returns the effective global timeout for a remediation.
// Checks for per-RR override in spec.timeoutConfig.global (AC-027-4).
// Falls back to controller-level default if not overridden.
func (r *Reconciler) getEffectiveGlobalTimeout(rr *remediationv1.RemediationRequest) time.Duration {
	if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Global != nil {
		return rr.Status.TimeoutConfig.Global.Duration
	}
	return r.timeouts.Global
}

// getEffectivePhaseTimeout returns the effective timeout for a specific phase.
// Checks for per-RR override in spec.timeoutConfig (AC-028-5).
// Falls back to controller-level default if not overridden.
func (r *Reconciler) getEffectivePhaseTimeout(rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase) time.Duration {
	if rr.Status.TimeoutConfig != nil {
		switch phase {
		case remediationv1.PhaseProcessing:
			if rr.Status.TimeoutConfig.Processing != nil {
				return rr.Status.TimeoutConfig.Processing.Duration
			}
		case remediationv1.PhaseAnalyzing:
			if rr.Status.TimeoutConfig.Analyzing != nil {
				return rr.Status.TimeoutConfig.Analyzing.Duration
			}
		case remediationv1.PhaseExecuting:
			if rr.Status.TimeoutConfig.Executing != nil {
				return rr.Status.TimeoutConfig.Executing.Duration
			}
		}
	}

	// Fall back to controller-level defaults
	switch phase {
	case remediationv1.PhaseProcessing:
		return r.timeouts.Processing
	case remediationv1.PhaseAnalyzing:
		return r.timeouts.Analyzing
	case remediationv1.PhaseExecuting:
		return r.timeouts.Executing
	default:
		// For phases without specific timeouts, return 0 (no timeout)
		return 0
	}
}

// checkPhaseTimeouts checks if the current phase has exceeded its timeout.
// Returns error if phase timeout detected and transition to TimedOut phase succeeds.
// Reference: BR-ORCH-028 (Per-phase timeouts), AC-028-2
func (r *Reconciler) checkPhaseTimeouts(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	logger := log.FromContext(ctx)

	currentPhase := rr.Status.OverallPhase
	var phaseStartTime *metav1.Time

	// Get phase start time based on current phase
	switch currentPhase {
	case remediationv1.PhaseProcessing:
		phaseStartTime = rr.Status.ProcessingStartTime
	case remediationv1.PhaseAnalyzing:
		phaseStartTime = rr.Status.AnalyzingStartTime
	case remediationv1.PhaseExecuting:
		phaseStartTime = rr.Status.ExecutingStartTime
	default:
		// Phase doesn't have specific timeout
		return nil
	}

	// No phase start time set yet, skip check
	if phaseStartTime == nil {
		return nil
	}

	// Get effective timeout for this phase
	phaseTimeout := r.getEffectivePhaseTimeout(rr, currentPhase)
	if phaseTimeout == 0 {
		// No timeout configured for this phase
		return nil
	}

	// Check if phase has exceeded timeout
	timeSincePhaseStart := time.Since(phaseStartTime.Time)
	if timeSincePhaseStart > phaseTimeout {
		logger.Info("RemediationRequest exceeded per-phase timeout",
			"phase", currentPhase,
			"timeSincePhaseStart", timeSincePhaseStart,
			"phaseTimeout", phaseTimeout,
			"phaseStartTime", phaseStartTime.Time,
			"overridden", rr.Status.TimeoutConfig != nil)
		return r.handlePhaseTimeout(ctx, rr, currentPhase, phaseTimeout)
	}

	return nil
}

// handlePhaseTimeout handles phase timeout by transitioning to TimedOut phase.
// Creates notification for phase-specific escalation.
// Reference: BR-ORCH-028 (Per-phase timeouts), AC-028-4
func (r *Reconciler) handlePhaseTimeout(ctx context.Context, rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase, timeout time.Duration) error {
	logger := log.FromContext(ctx)

	// Update status to TimedOut with phase-specific metadata
	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, r.Metrics, rr, func(rr *remediationv1.RemediationRequest) error {
		// Transition to TimedOut phase
		rr.Status.OverallPhase = remediationv1.PhaseTimedOut
		rr.Status.Message = fmt.Sprintf("Phase %s exceeded timeout of %s", phase, timeout)
		rr.Status.TimeoutTime = &metav1.Time{Time: time.Now()}
		phaseStr := string(phase)
		rr.Status.TimeoutPhase = &phaseStr
		rr.Status.CompletedAt = &metav1.Time{Time: time.Now()}
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to update status on phase timeout")
		return err
	}

	// Record timeout metric (BR-ORCH-044)
	if r.Metrics != nil {
		r.Metrics.TimeoutsTotal.WithLabelValues(rr.Namespace, string(phase)).Inc()
	}

	// Per DD-AUDIT-003: Emit timeout event (lifecycle.completed with outcome=failure)
	if rr.Status.StartTime != nil {
		durationMs := time.Since(rr.Status.StartTime.Time).Milliseconds()
		r.emitTimeoutAudit(ctx, rr, "phase", string(phase), durationMs)
	}

	// DD-EVENT-001: Emit K8s event for phase timeout (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationTimeout,
			fmt.Sprintf("Phase %s exceeded timeout of %s", phase, timeout))
	}

	logger.Info("RemediationRequest transitioned to TimedOut due to phase timeout",
		"phase", phase,
		"timeout", timeout)

	// Create phase-specific timeout notification (non-blocking)
	r.createPhaseTimeoutNotification(ctx, rr, phase, timeout)

	return nil
}

// createPhaseTimeoutNotification creates a notification for phase timeout.
// Non-blocking - logs errors but doesn't fail reconciliation.
// Reference: BR-ORCH-028 (Per-phase timeout escalation)
func (r *Reconciler) createPhaseTimeoutNotification(ctx context.Context, rr *remediationv1.RemediationRequest, phase remediationv1.RemediationPhase, timeout time.Duration) {
	logger := log.FromContext(ctx)

	// Defensive: Refresh RR to get latest status (including TimeoutTime)
	latest := &remediationv1.RemediationRequest{}
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(rr), latest); err != nil {
		logger.Error(err, "Failed to refresh RR for phase timeout notification")
		return
	}
	rr = latest // Use refreshed version

	// Kubernetes names must be lowercase RFC 1123
	phaseLower := strings.ToLower(string(phase))
	notificationName := fmt.Sprintf("phase-timeout-%s-%s", phaseLower, rr.Name)

	nr := &notificationv1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      notificationName,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/notification-type":   "phase-timeout",
				"kubernaut.ai/phase":               string(phase),
				"kubernaut.ai/severity":            rr.Spec.Severity,
				"kubernaut.ai/component":           "remediation-orchestrator",
			},
		},
		Spec: notificationv1.NotificationRequestSpec{
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: notificationv1.NotificationPriorityHigh,
			Subject:  fmt.Sprintf("Phase Timeout: %s - %s", phase, rr.Spec.SignalName),
			Body: fmt.Sprintf(`Remediation phase has exceeded timeout and requires investigation.

**Signal**: %s
**Phase**: %s
**Phase Timeout**: %s
**Started**: %v
**Timed Out**: %v

The %s phase did not complete within the expected timeframe. Please investigate why this phase is taking longer than expected.`,
				rr.Spec.SignalName,
				phase,
				timeout.String(),
				safeFormatTime(rr.Status.StartTime),
				safeFormatTime(rr.Status.TimeoutTime),
				phase,
			),
			Channels: []notificationv1.Channel{
				notificationv1.ChannelSlack,
				notificationv1.ChannelEmail,
			},
			Metadata: map[string]string{
				"remediationRequest": rr.Name,
				"timeoutPhase":       string(phase),
				"phaseTimeout":       timeout.String(),
				"severity":           rr.Spec.Severity,
				"targetResource":     fmt.Sprintf("%s/%s", rr.Spec.TargetResource.Kind, rr.Spec.TargetResource.Name),
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(rr, nr, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on phase timeout notification")
		return
	}

	// Create notification (non-blocking)
	if err := r.client.Create(ctx, nr); err != nil {
		logger.Error(err, "Failed to create phase timeout notification",
			"notificationName", notificationName,
			"phase", phase)
		return
	}

	logger.Info("Created phase timeout notification",
		"notificationName", notificationName,
		"phase", phase,
		"timeout", timeout)
}

// ════════════════════════════════════════════════════════════════════════════
// BR-ORCH-042: Consecutive Failure Blocking Integration
// ════════════════════════════════════════════════════════════════════════════

// SetConsecutiveFailureBlocker sets the consecutive failure blocker for testing.
// Production code should create blocker in NewReconciler or via controller config.
func (r *Reconciler) SetConsecutiveFailureBlocker(blocker *ConsecutiveFailureBlocker) {
	r.consecutiveBlock = blocker
}

// HandleBlockedPhase handles RemediationRequests in Blocked phase.
// Checks if cooldown has expired and transitions to Failed if so.
//
// AC-042-3-2: RO transitions to Failed when cooldown expires
// AC-042-3-3: RO requeues at exact expiry time
func (r *Reconciler) HandleBlockedPhase(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithName("handle-blocked-phase")

	// Check if this is a manual block (no BlockedUntil set)
	if rr.Status.BlockedUntil == nil {
		// Manual block - no auto-expiry
		logger.Info("RemediationRequest manually blocked - no auto-expiry",
			"name", rr.Name,
			"blockReason", rr.Status.BlockReason)
		return ctrl.Result{}, nil
	}

	// Check if cooldown has expired
	now := time.Now()
	if now.After(rr.Status.BlockedUntil.Time) {
		// BR-SCOPE-010: UnmanagedResource blocks re-validate scope instead of failing
		if rr.Status.BlockReason == string(remediationv1.BlockReasonUnmanagedResource) {
			return r.handleUnmanagedResourceExpiry(ctx, rr, logger)
		}

		// Cooldown expired - transition to Failed
		logger.Info("Blocked cooldown expired - transitioning to Failed",
			"name", rr.Name,
			"blockedUntil", rr.Status.BlockedUntil.Time,
			"blockReason", rr.Status.BlockReason)

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Consolidate: OverallPhase + Outcome + Message → 1 API call
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			rr.Status.OverallPhase = remediationv1.PhaseFailed
			rr.Status.Outcome = "Blocked"
			rr.Status.Message = fmt.Sprintf("Cooldown expired after %d consecutive failures",
				r.getConsecutiveFailureThreshold())
			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status after cooldown expiry: %w", err)
		}

		return ctrl.Result{}, nil
	}

	// Cooldown still active - requeue at expiry time
	requeueAfter := time.Until(rr.Status.BlockedUntil.Time)
	logger.Info("RemediationRequest blocked - requeuing at cooldown expiry",
		"name", rr.Name,
		"blockedUntil", rr.Status.BlockedUntil.Time,
		"requeueAfter", requeueAfter)

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// handleUnmanagedResourceExpiry re-validates scope when an UnmanagedResource block expires.
// If still unmanaged: re-block with increased backoff.
// If now managed: transition back to Pending for re-processing.
//
// Reference: BR-SCOPE-010, ADR-053 (Resource Scope Management)
func (r *Reconciler) handleUnmanagedResourceExpiry(ctx context.Context, rr *remediationv1.RemediationRequest, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("UnmanagedResource block expired - re-validating scope",
		"name", rr.Name)

	// Re-validate scope using the routing engine
	blocked := r.routingEngine.CheckUnmanagedResource(ctx, rr)

	if blocked != nil {
		// Still unmanaged — re-block with increased backoff
		logger.Info("Resource still unmanaged - re-blocking with increased backoff",
			"name", rr.Name,
			"newBackoff", blocked.RequeueAfter)

		blockedUntilMeta := metav1.NewTime(*blocked.BlockedUntil)
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			rr.Status.BlockReason = blocked.Reason
			rr.Status.Message = blocked.Message
			rr.Status.BlockedUntil = &blockedUntilMeta
			rr.Status.ConsecutiveFailureCount++ // increment to increase next backoff
			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status for scope re-block: %w", err)
		}

		return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
	}

	// Now managed — transition back to Pending for re-processing
	logger.Info("Resource is now managed - unblocking and transitioning to Pending",
		"name", rr.Name)

	if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
		rr.Status.OverallPhase = remediationv1.PhasePending
		rr.Status.BlockReason = ""
		rr.Status.BlockedUntil = nil
		rr.Status.Message = "Resource is now managed by Kubernaut - re-processing"
		return nil
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status after scope unblock: %w", err)
	}

	// Requeue immediately for re-processing
	return ctrl.Result{Requeue: true}, nil
}

// IsTerminalPhase checks if a phase is terminal (no further processing).
// BR-ORCH-042.2: Blocked is NON-terminal (active)
//
// AC-042-2-1: IsTerminal(Blocked) returns false
func IsTerminalPhase(phase remediationv1.RemediationPhase) bool {
	terminalPhases := []remediationv1.RemediationPhase{
		remediationv1.PhaseCompleted,
		remediationv1.PhaseFailed,
		remediationv1.PhaseTimedOut,
		remediationv1.PhaseCancelled,
		remediationv1.PhaseSkipped,
	}

	for _, terminal := range terminalPhases {
		if phase == terminal {
			return true
		}
	}
	return false
}

// getConsecutiveFailureThreshold returns the configured threshold or default (3).
func (r *Reconciler) getConsecutiveFailureThreshold() int {
	if r.consecutiveBlock != nil {
		return r.consecutiveBlock.threshold
	}
	return 3 // default
}

// validateTimeoutConfig validates the timeout configuration in RemediationRequest.Status.TimeoutConfig.
// BR-AUDIT-005 Gap #7: Validates that all timeouts are non-negative.
// Gap #8: TimeoutConfig moved from Spec to Status for operator mutability.
// Returns error with ERR_INVALID_TIMEOUT_CONFIG code if validation fails.
func (r *Reconciler) validateTimeoutConfig(ctx context.Context, rr *remediationv1.RemediationRequest) error {
	if rr.Status.TimeoutConfig == nil {
		return nil // No custom timeout config, use defaults
	}

	// Validate Global timeout
	if rr.Status.TimeoutConfig.Global != nil && rr.Status.TimeoutConfig.Global.Duration < 0 {
		return fmt.Errorf("Global timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Global.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Processing timeout
	if rr.Status.TimeoutConfig.Processing != nil && rr.Status.TimeoutConfig.Processing.Duration < 0 {
		return fmt.Errorf("Processing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Processing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Analyzing timeout
	if rr.Status.TimeoutConfig.Analyzing != nil && rr.Status.TimeoutConfig.Analyzing.Duration < 0 {
		return fmt.Errorf("Analyzing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Analyzing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Executing timeout
	if rr.Status.TimeoutConfig.Executing != nil && rr.Status.TimeoutConfig.Executing.Duration < 0 {
		return fmt.Errorf("Executing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Executing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	return nil
}

// SetRESTMapper sets the REST mapper used by CapturePreRemediationHash to
// resolve Kind strings to GroupVersionKind for the unstructured client.
// DD-EM-002: Called from cmd/remediationorchestrator/main.go after manager setup.
func (r *Reconciler) SetRESTMapper(mapper meta.RESTMapper) {
	r.restMapper = mapper
}

// CapturePreRemediationHash fetches the target resource via an uncached reader,
// extracts its .spec, and computes the canonical SHA-256 hash (DD-EM-002).
//
// Returns empty string (no error) when:
//   - The resource is not found (non-fatal: RO logs and continues)
//   - The resource has no .spec field
//   - The REST mapper cannot resolve the Kind
//
// This is exported for testability from the test package.
func CapturePreRemediationHash(
	ctx context.Context,
	reader client.Reader,
	restMapper meta.RESTMapper,
	targetKind string,
	targetName string,
	targetNamespace string,
) (string, error) {
	logger := log.FromContext(ctx)

	// Resolve Kind to GVK via REST mapper
	gvk, err := resolveGVKForKind(restMapper, targetKind)
	if err != nil {
		logger.V(1).Info("Cannot resolve GVK for kind, skipping pre-remediation hash",
			"kind", targetKind, "error", err)
		return "", nil
	}

	// Fetch the resource using unstructured client
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{Name: targetName, Namespace: targetNamespace}
	if err := reader.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Target resource not found, skipping pre-remediation hash",
				"kind", targetKind, "name", targetName, "namespace", targetNamespace)
			return "", nil
		}
		return "", fmt.Errorf("failed to fetch target resource %s/%s: %w", targetKind, targetName, err)
	}

	// Extract .spec from unstructured
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return "", fmt.Errorf("failed to extract .spec from %s/%s: %w", targetKind, targetName, err)
	}
	if !found || spec == nil {
		logger.V(1).Info("Target resource has no .spec, skipping pre-remediation hash",
			"kind", targetKind, "name", targetName)
		return "", nil
	}

	// Compute canonical hash
	hash, err := canonicalhash.CanonicalSpecHash(spec)
	if err != nil {
		return "", fmt.Errorf("failed to compute canonical hash for %s/%s: %w", targetKind, targetName, err)
	}

	return hash, nil
}

// resolveGVKForKind resolves a Kind string to its GroupVersionKind using the
// REST mapper. Falls back to common Kubernetes kinds if the mapper fails.
func resolveGVKForKind(mapper meta.RESTMapper, kind string) (schema.GroupVersionKind, error) {
	// Try well-known kinds first for reliability
	switch kind {
	case "Deployment":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, nil
	case "StatefulSet":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}, nil
	case "DaemonSet":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}, nil
	case "Pod":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, nil
	case "Service":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, nil
	case "ConfigMap":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, nil
	}

	// Fall back to REST mapper for custom resources
	if mapper != nil {
		gvk, err := mapper.RESTMapping(schema.GroupKind{Kind: kind})
		if err == nil {
			return gvk.GroupVersionKind, nil
		}
	}

	return schema.GroupVersionKind{}, fmt.Errorf("cannot resolve GVK for kind %q", kind)
}
