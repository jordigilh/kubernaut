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
	"errors"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/locking"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// errPhaseAlreadySet is returned when the phase was already initialized by a
// concurrent reconcile. Non-retryable: RetryOnConflict only retries k8s
// Conflict errors, so this propagates immediately to the caller for graceful handling.
var errPhaseAlreadySet = errors.New("phase already initialized by concurrent reconcile")

// Reconciler reconciles RemediationRequest objects.
type Reconciler struct {
	client              client.Client
	scheme              *runtime.Scheme
	statusAggregator    *aggregator.StatusAggregator
	aiAnalysisHandler   *handler.AIAnalysisHandler
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

	// BR-FLEET-054: Fleet reader factory for remote cluster reads.
	// nil = local-only mode (all reads go through apiReader).
	readerFactory fleet.ReaderFactory

	// DD-EM-004 v2.0, Issue #253: Config-driven async propagation delays.
	// Used by createEffectivenessAssessmentIfNeeded to compute HashComputeDelay
	// from gitOpsSyncDelay/operatorReconcileDelay instead of stabilization window.
	asyncPropagation roconfig.AsyncPropagationConfig

	// BR-ORCH-025: Distributed lock manager for WFE creation safety.
	// Prevents duplicate WFEs when concurrent reconciles target the same resource.
	// nil = locking disabled (single-replica deployments).
	lockManager *locking.DistributedLockManager

	// ADR-068: Fleet config for federated scope checking in default routing engine fallback.
	fleetConfig fleet.FleetConfig

	// #835: Guards hot-reloadable config fields. Setters acquire write lock,
	// getters acquire read lock. Per DD-INFRA-001 thread-safety requirement.
	configMu sync.RWMutex

	// #265: CRD retention TTL — how long terminal RRs persist before cleanup.
	// Default: 24h. Configurable via SetRetentionPeriod or YAML config.
	retentionPeriod time.Duration

	// #712, #736: Dry-run mode — pipeline stops after AI analysis.
	// No WFE or EA is created; RR completes with outcome "DryRun".
	dryRun bool
	// #712, #736: How long to suppress GW re-triggering after dry-run completion.
	// Sets NextAllowedExecution on the terminal RR (same pattern as NoActionRequired).
	dryRunHoldPeriod time.Duration

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

	// Issue #666: Phase Handler Registry for incremental handler extraction
	phaseRegistry *phase.Registry
}

// TimeoutConfig holds all timeout configuration for the controller.
// Provides defaults for all remediations, can be overridden per-RR via spec.timeoutConfig.
// Reference: BR-ORCH-027 (Global timeout), BR-ORCH-028 (Per-phase timeouts)
type TimeoutConfig struct {
	Global           time.Duration // Default: 1 hour
	Processing       time.Duration // Default: 5 minutes
	Analyzing        time.Duration // Default: 10 minutes
	Executing        time.Duration // Default: 30 minutes
	AwaitingApproval time.Duration // Default: 15 minutes (ADR-040)
	Verifying        time.Duration // Default: 30 minutes (#280: safety-net for Verifying phase)
	MaxAnalyzing     time.Duration // Default: 45 minutes (DD-INTERACTIVE-002: hard cap for interactive sessions)
}

// ReconcilerDeps groups the injected dependencies for NewReconciler,
// separate from the variadic optional eaCreator argument. Extracted per
// AGENTS.md's 8+-param Options-pattern rule.
type ReconcilerDeps struct {
	Client        client.Client
	APIReader     client.Reader
	Scheme        *runtime.Scheme
	AuditStore    audit.AuditStore
	Recorder      record.EventRecorder
	Metrics       *metrics.Metrics
	Timeouts      TimeoutConfig
	RoutingEngine routing.Engine
}

// NewReconciler creates a new Reconciler with all dependencies.
// Per ADR-032 §1: Audit is MANDATORY for RemediationOrchestrator (P0 service).
// The auditStore parameter must be non-nil; the service will crash at startup
// (cmd/remediationorchestrator/main.go:128) if audit cannot be initialized.
// Tests must provide a non-nil audit store (use NoOpStore or mock).
// The timeouts parameter configures all timeout durations (global and per-phase).
// Zero values use defaults: Global=1h, Processing=5m, Analyzing=10m, Executing=30m.
// DD-STATUS-001: apiReader parameter added for cache-bypassed status refetch in atomic updates.
func NewReconciler(deps ReconcilerDeps, eaCreator ...*creator.EffectivenessAssessmentCreator) *Reconciler {
	c, apiReader, s, auditStore, recorder, m, timeouts, routingEngine :=
		deps.Client, deps.APIReader, deps.Scheme, deps.AuditStore, deps.Recorder, deps.Metrics, deps.Timeouts, deps.RoutingEngine

	// DD-METRICS-001: Metrics are REQUIRED (dependency injection pattern)
	// Metrics are initialized in main.go via rometrics.NewMetrics()
	// If nil is passed here, it's a programming error in main.go

	// Set default timeouts if not specified (BR-ORCH-027/028)
	applyTimeoutDefaults(&timeouts)

	nc := creator.NewNotificationCreator(c, s, m)

	// Initialize routing engine if not provided (DD-RO-002, DD-WE-004)
	// Unit tests can pass a mock routing engine to test orchestration logic in isolation
	if routingEngine == nil {
		routingEngine = newDefaultRoutingEngine(c, apiReader)
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
		approvalCreator:     creator.NewApprovalCreator(c, s, m, timeouts.AwaitingApproval),
		timeouts:            timeouts,
		auditStore:          auditStore,
		auditManager:        roaudit.NewManager(roaudit.ServiceName),
		notificationHandler: NewNotificationHandler(c, m),
		routingEngine:       routingEngine, // Use provided or default routing engine
		Metrics:             m,
		Recorder:            recorder,
		StatusManager:       statusManager,       // DD-PERF-001: Atomic status updates
		apiReader:           apiReader,           // DD-EM-002: Uncached reader for pre-remediation hash
		retentionPeriod:     24 * time.Hour,      // #265: Default CRD TTL
		phaseRegistry:       phase.NewRegistry(), // Issue #666: Phase handler registry
	}

	// ADR-EM-001: Wire optional EA creator (variadic for backward compatibility)
	if len(eaCreator) > 0 && eaCreator[0] != nil {
		r.eaCreator = eaCreator[0]
	}

	// ========================================
	// HANDLER INITIALIZATION (Handler Consistency Refactoring 2026-01-22)
	// Initialize handlers with transition callbacks for audit emission (BR-AUDIT-005, DD-AUDIT-003)
	// ========================================

	// AIAnalysisHandler: delegates failure transitions
	noActionDelay := time.Duration(r.routingEngine.Config().NoActionRequiredDelayHours) * time.Hour
	r.aiAnalysisHandler = handler.NewAIAnalysisHandler(c, s, nc, m, r.transitionToFailed, noActionDelay)

	// Issue #666: Register phase handlers in the registry
	r.registerPhaseHandlers(phaseHandlerDeps{
		client:        c,
		apiReader:     apiReader,
		statusManager: statusManager,
		metrics:       m,
		notifCreator:  nc,
		recorder:      recorder,
		routingEngine: routingEngine,
		timeouts:      timeouts,
	})

	return r
}

// phaseHandlerDeps groups the dependencies needed to construct and register
// every phase handler, keeping registerPhaseHandlers under the 7-param
// argument-limit (AGENTS.md Go Anti-Pattern Checklist).
type phaseHandlerDeps struct {
	client        client.Client
	apiReader     client.Reader
	statusManager *status.Manager
	metrics       *metrics.Metrics
	notifCreator  *creator.NotificationCreator
	recorder      record.EventRecorder
	routingEngine routing.Engine
	timeouts      TimeoutConfig
}

// registerPhaseHandlers wires and registers every phase handler (Pending,
// Processing, Executing, Blocked, Analyzing, AwaitingApproval, Verifying)
// into r.phaseRegistry (Issue #666: Phase Handler Registry). Extracted from
// NewReconciler (Wave 6 6e-i GREEN: funlen remediation) — pure code motion,
// no behavior change.
func (r *Reconciler) registerPhaseHandlers(deps phaseHandlerDeps) {
	c, apiReader, statusManager, m, nc, recorder, routingEngine, timeouts :=
		deps.client, deps.apiReader, deps.statusManager, deps.metrics,
		deps.notifCreator, deps.recorder, deps.routingEngine, deps.timeouts

	r.phaseRegistry.MustRegister(NewPendingHandler(c, routingEngine, r.spCreator, statusManager, m))
	r.phaseRegistry.MustRegister(NewProcessingHandler(c, r.aiAnalysisCreator, statusManager, m))
	r.phaseRegistry.MustRegister(NewExecutingHandler(c, apiReader, r.statusAggregator, statusManager, m, nc, recorder))
	r.phaseRegistry.MustRegister(NewBlockedHandler(m, BlockedCallbacks{
		RecheckResourceBusyBlock:      r.recheckResourceBusyBlock,
		RecheckDuplicateBlock:         r.recheckDuplicateBlock,
		HandleUnmanagedResourceExpiry: r.handleUnmanagedResourceExpiry,
		TransitionToFailedTerminal:    r.transitionToFailedTerminal,
	}))
	r.registerWorkflowPhaseHandlers(c, m)
	r.phaseRegistry.MustRegister(NewVerifyingHandler(c, m, timeouts, VerifyingCallbacks{
		EnsureNotificationsCreated:            r.ensureNotificationsCreated,
		CreateEffectivenessAssessmentIfNeeded: r.createEffectivenessAssessmentIfNeeded,
		TrackEffectivenessStatus:              r.trackEffectivenessStatus,
		EmitVerificationTimedOutAudit:         r.emitVerificationTimedOutAudit,
		EmitVerificationCompletedAudit:        r.emitVerificationCompletedAudit,
		EmitCompletionAudit:                   r.emitCompletionAudit,
	}))
}

// registerWorkflowPhaseHandlers registers the Analyzing and AwaitingApproval
// phase handlers, which share a single WFECreationCallbacks instance for
// workflow-execution-CRD creation. Extracted from registerPhaseHandlers
// (Wave 6 6e-i GREEN: funlen remediation) — pure code motion, no behavior
// change.
func (r *Reconciler) registerWorkflowPhaseHandlers(c client.Client, m *metrics.Metrics) {
	wfeCallbacks := WFECreationCallbacks{
		EmitWorkflowCreatedAudit: r.emitWorkflowCreatedAudit,
		CreateWFE: func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, error) {
			return r.weCreator.Create(ctx, rr, ai)
		},
	}
	r.phaseRegistry.MustRegister(NewAnalyzingHandler(c, m, AnalyzingCallbacks{
		AtomicStatusUpdate: func(ctx context.Context, rr *remediationv1.RemediationRequest, fn func() error) error {
			return r.StatusManager.AtomicStatusUpdate(ctx, rr, fn)
		},
		IsWorkflowNotNeeded:     handler.IsWorkflowNotNeeded,
		HandleWorkflowNotNeeded: r.aiAnalysisHandler.HandleAIAnalysisStatus,
		CreateApproval: func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, error) {
			return r.approvalCreator.Create(ctx, rr, ai)
		},
		HandleAIAnalysisStatus:         r.aiAnalysisHandler.HandleAIAnalysisStatus,
		HandleRemediationTargetMissing: r.aiAnalysisHandler.HandleRemediationTargetMissing,
		EmitApprovalRequestedAudit:     r.emitApprovalRequestedAudit,
		RecordEvent:                    r.recordEvent,
		FetchFreshRR:                   r.fetchFreshRR,
		CheckPostAnalysisConditions:    r.routingEngine.CheckPostAnalysisConditions,
		HandleBlocked:                  r.handleBlocked,
		AcquireLock:                    r.acquireLock,
		ReleaseLock:                    r.releaseLock,
		CapturePreRemediationHash:      r.capturePreRemediationHashForCallback,
		ResolveDualTargets:             resolveDualTargetsForCallback,
		PersistPreHash:                 r.persistPreRemediationHash,
		IsDryRun:                       r.isDryRun,
		WFECallbacks:                   wfeCallbacks,
	}))
	r.phaseRegistry.MustRegister(NewAwaitingApprovalHandler(c, m, AwaitingApprovalCallbacks{
		RecordEvent:               r.recordEvent,
		UpdateRARConditions:       r.updateRARConditionsOnDecision,
		ResolveWorkflow:           r.resolveWorkflowOverride,
		CheckResourceBusy:         r.routingEngine.CheckResourceBusy,
		HandleBlocked:             r.handleBlocked,
		AcquireLock:               r.acquireLock,
		ReleaseLock:               r.releaseLock,
		CapturePreRemediationHash: r.capturePreRemediationHashForCallback,
		ResolveDualTargets:        resolveDualTargetsForCallback,
		PersistPreHash:            r.persistPreRemediationHash,
		TransitionToFailed:        r.transitionToFailed,
		ExpireRAR:                 r.expireRARWithoutDecision,
		UpdateRARTimeRemaining:    r.updateRARTimeRemaining,
		WFECallbacks:              wfeCallbacks,
	}))
}

// applyTimeoutDefaults populates zero-valued timeout fields with controller
// defaults (BR-ORCH-027/028). Extracted from NewReconciler per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func applyTimeoutDefaults(timeouts *TimeoutConfig) {
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
	if timeouts.AwaitingApproval == 0 {
		timeouts.AwaitingApproval = 15 * time.Minute
	}
	if timeouts.Verifying == 0 {
		timeouts.Verifying = 30 * time.Minute
	}
	if timeouts.MaxAnalyzing == 0 {
		timeouts.MaxAnalyzing = 45 * time.Minute
	}
}

// newDefaultRoutingEngine builds the production routing engine used when
// ReconcilerDeps.RoutingEngine is not supplied (DD-RO-002, DD-WE-004).
// Extracted from NewReconciler per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520). Unit tests pass a mock routing engine instead, bypassing
// this constructor entirely.
func newDefaultRoutingEngine(c client.Client, apiReader client.Reader) routing.Engine {
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
		// NoActionRequired suppression (Issue #314)
		NoActionRequiredDelayHours: 24, // 24 hours
	}
	// ADR-057: All CRDs live in the controller namespace; empty string is correct.
	routingNamespace := ""
	// BR-SCOPE-010: Create scope manager using cached client for metadata-only informers (ADR-053)
	scopeMgr := scope.NewManager(c)
	// ADR-068: Wrap local scope manager with fleet-aware checker if fleet is configured.
	scopeChecker, scopeErr := fleet.NewScopeChecker(scopeMgr, fleet.FleetConfig{}, log.Log)
	if scopeErr != nil {
		scopeChecker = scopeMgr
	}
	// DD-STATUS-001: Pass apiReader for cache-bypassed routing queries
	return routing.NewRoutingEngine(c, apiReader, routingNamespace, routingConfig, scopeChecker)
}
