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
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/locking"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/override"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
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

	// Issue #643 v2: DS-backed workflow display resolver.
	// Resolves workflow UUID → human-readable WorkflowName + ActionType from DataStorage.
	// nil = graceful degradation (UUID shown as-is in printer columns).
	workflowResolver routing.WorkflowDisplayResolver

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
		routingEngine = routing.NewRoutingEngine(c, apiReader, routingNamespace, routingConfig, scopeChecker)
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
	r.phaseRegistry.MustRegister(NewPendingHandler(c, routingEngine, r.spCreator, statusManager, m))
	r.phaseRegistry.MustRegister(NewProcessingHandler(c, r.aiAnalysisCreator, statusManager, m))
	r.phaseRegistry.MustRegister(NewExecutingHandler(c, apiReader, r.statusAggregator, statusManager, m, nc, recorder))
	r.phaseRegistry.MustRegister(NewBlockedHandler(m, BlockedCallbacks{
		RecheckResourceBusyBlock:      r.recheckResourceBusyBlock,
		RecheckDuplicateBlock:         r.recheckDuplicateBlock,
		HandleUnmanagedResourceExpiry: r.handleUnmanagedResourceExpiry,
		TransitionToFailedTerminal:    r.transitionToFailedTerminal,
	}))
	wfeCallbacks := WFECreationCallbacks{
		EmitWorkflowCreatedAudit: r.emitWorkflowCreatedAudit,
		CreateWFE: func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, error) {
			return r.weCreator.Create(ctx, rr, ai)
		},
		ResolveWorkflowDisplay: r.resolveWorkflowDisplay,
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
		RecordEvent: func(rr *remediationv1.RemediationRequest, eventType, reason, message string) {
			if r.Recorder != nil {
				r.Recorder.Event(rr, eventType, reason, message)
			}
		},
		FetchFreshRR: func(ctx context.Context, key client.ObjectKey) (*remediationv1.RemediationRequest, error) {
			freshRR := &remediationv1.RemediationRequest{}
			err := r.apiReader.Get(ctx, key, freshRR)
			return freshRR, err
		},
		CheckPostAnalysisConditions: r.routingEngine.CheckPostAnalysisConditions,
		HandleBlocked:               r.handleBlocked,
		AcquireLock: func(ctx context.Context, target string) (bool, error) {
			if r.lockManager == nil {
				return true, nil
			}
			return r.lockManager.AcquireLock(ctx, target)
		},
		ReleaseLock: func(ctx context.Context, target string) error {
			if r.lockManager == nil {
				return nil
			}
			return r.lockManager.ReleaseLock(ctx, target)
		},
		CapturePreRemediationHash: func(ctx context.Context, kind, name, namespace, clusterID string) (string, string, error) {
			reader, err := r.readerForHash(ctx, clusterID)
			if err != nil {
				return "", "", fmt.Errorf("failed to get reader for cluster %s: %w", clusterID, err)
			}
			return CapturePreRemediationHash(ctx, reader, r.restMapper, kind, name, namespace)
		},
		ResolveDualTargets: func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult {
			dt := resolveDualTargets(rr, ai)
			return DualTargetResult{Remediation: TargetRef{Kind: dt.Remediation.Kind, Name: dt.Remediation.Name, Namespace: dt.Remediation.Namespace}}
		},
		PersistPreHash: func(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error {
			return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.PreRemediationSpecHash = preHash
				remediationrequest.SetPreRemediationHashCaptured(rr, true,
					"Pre-remediation hash captured", r.Metrics)
				return nil
			})
		},
		IsDryRun:     r.isDryRun,
		WFECallbacks: wfeCallbacks,
	}))
	r.phaseRegistry.MustRegister(NewAwaitingApprovalHandler(c, m, AwaitingApprovalCallbacks{
		RecordEvent: func(rr *remediationv1.RemediationRequest, eventType, reason, message string) {
			if r.Recorder != nil {
				r.Recorder.Event(rr, eventType, reason, message)
			}
		},
		UpdateRARConditions: func(ctx context.Context, _ *remediationv1.RemediationRequest, rar *remediationv1.RemediationApprovalRequest, decision string) error {
			return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
					return err
				}
				remediationapprovalrequest.SetApprovalPending(rar, false, "Decision received", r.Metrics)
				switch decision {
				case "approved":
					remediationapprovalrequest.SetApprovalDecided(rar, true,
						remediationapprovalrequest.ReasonApproved,
						fmt.Sprintf("Approved by %s", rar.Status.DecidedBy), r.Metrics)
				case "rejected":
					remediationapprovalrequest.SetApprovalDecided(rar, true,
						remediationapprovalrequest.ReasonRejected,
						fmt.Sprintf("Rejected by %s: %s", rar.Status.DecidedBy, rar.Status.DecisionMessage), r.Metrics)
				}
				remediationapprovalrequest.SetReady(rar, true, remediationapprovalrequest.ReasonReady, "Approval decided", r.Metrics)
				return r.client.Status().Update(ctx, rar)
			})
		},
		ResolveWorkflow: func(ctx context.Context, wo *remediationv1.WorkflowOverride, sw *aianalysisv1.SelectedWorkflow, ns string) (*aianalysisv1.SelectedWorkflow, bool, error) {
			return override.ResolveWorkflow(ctx, r.apiReader, wo, sw, ns)
		},
		CheckResourceBusy: r.routingEngine.CheckResourceBusy,
		HandleBlocked:     r.handleBlocked,
		AcquireLock: func(ctx context.Context, target string) (bool, error) {
			if r.lockManager == nil {
				return true, nil
			}
			return r.lockManager.AcquireLock(ctx, target)
		},
		ReleaseLock: func(ctx context.Context, target string) error {
			if r.lockManager == nil {
				return nil
			}
			return r.lockManager.ReleaseLock(ctx, target)
		},
		CapturePreRemediationHash: func(ctx context.Context, kind, name, namespace, clusterID string) (string, string, error) {
			reader, err := r.readerForHash(ctx, clusterID)
			if err != nil {
				return "", "", fmt.Errorf("failed to get reader for cluster %s: %w", clusterID, err)
			}
			return CapturePreRemediationHash(ctx, reader, r.restMapper, kind, name, namespace)
		},
		ResolveDualTargets: func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult {
			dt := resolveDualTargets(rr, ai)
			return DualTargetResult{Remediation: TargetRef{Kind: dt.Remediation.Kind, Name: dt.Remediation.Name, Namespace: dt.Remediation.Namespace}}
		},
		PersistPreHash: func(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error {
			return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.PreRemediationSpecHash = preHash
				remediationrequest.SetPreRemediationHashCaptured(rr, true,
					"Pre-remediation hash captured", r.Metrics)
				return nil
			})
		},
		TransitionToFailed: r.transitionToFailed,
		ExpireRAR: func(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error {
			return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
					return err
				}
				remediationapprovalrequest.SetApprovalPending(rar, false, "Expired without decision", r.Metrics)
				remediationapprovalrequest.SetApprovalExpired(rar, true,
					fmt.Sprintf("Expired after %v without decision",
						time.Since(rar.ObjectMeta.CreationTimestamp.Time).Round(time.Minute)), r.Metrics)
				remediationapprovalrequest.SetReady(rar, false, remediationapprovalrequest.ReasonNotReady, "Approval expired", r.Metrics)
				rar.Status.Decision = remediationv1.ApprovalDecisionExpired
				rar.Status.DecidedBy = "system"
				rar.Status.Expired = true
				rar.Status.TimeRemaining = remediationapprovalrequest.ComputeTimeRemaining(rar.Spec.RequiredBy.Time, time.Now())
				now := metav1.Now()
				rar.Status.DecidedAt = &now
				return r.client.Status().Update(ctx, rar)
			})
		},
		UpdateRARTimeRemaining: func(ctx context.Context, rar *remediationv1.RemediationApprovalRequest) error {
			return k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
				if err := r.client.Get(ctx, client.ObjectKeyFromObject(rar), rar); err != nil {
					return err
				}
				rar.Status.TimeRemaining = remediationapprovalrequest.ComputeTimeRemaining(rar.Spec.RequiredBy.Time, time.Now())
				return r.client.Status().Update(ctx, rar)
			})
		},
		WFECallbacks: wfeCallbacks,
	}))
	r.phaseRegistry.MustRegister(NewVerifyingHandler(c, m, timeouts, VerifyingCallbacks{
		EnsureNotificationsCreated:            r.ensureNotificationsCreated,
		CreateEffectivenessAssessmentIfNeeded: r.createEffectivenessAssessmentIfNeeded,
		TrackEffectivenessStatus:              r.trackEffectivenessStatus,
		EmitVerificationTimedOutAudit:         r.emitVerificationTimedOutAudit,
		EmitVerificationCompletedAudit:        r.emitVerificationCompletedAudit,
		EmitCompletionAudit:                   r.emitCompletionAudit,
	}))

	return r
}

// SetWorkflowResolver wires the DS-backed workflow display resolver.
// Must be called after NewReconciler in production (cmd/remediationorchestrator/main.go).
// nil is safe — resolveWorkflowDisplay falls back to the raw UUID.
func (r *Reconciler) SetWorkflowResolver(resolver routing.WorkflowDisplayResolver) {
	r.workflowResolver = resolver
}

// SetRetentionPeriod configures how long terminal RemediationRequest CRDs persist
// before automatic cleanup. Default: 24h. Issue #265.
// Thread-safe: acquires configMu write lock (#835, DD-INFRA-001).
func (r *Reconciler) SetRetentionPeriod(d time.Duration) {
	if d > 0 {
		r.configMu.Lock()
		r.retentionPeriod = d
		r.configMu.Unlock()
	}
}

// getRetentionPeriod returns the current retention TTL for terminal RRs.
// Thread-safe: acquires configMu read lock (#835).
func (r *Reconciler) getRetentionPeriod() time.Duration {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.retentionPeriod
}

// SetDSClient wires the DataStorage history querier into the routing engine.
// Must be called after NewReconciler if the default routing engine is used in production.
// Issue #214: Enables CheckIneffectiveRemediationChain to query real DS data.
func (r *Reconciler) SetDSClient(dsClient routing.RemediationHistoryQuerier) {
	if re, ok := r.routingEngine.(*routing.RoutingEngine); ok {
		re.SetDSClient(dsClient)
	} else {
		log.Log.Info("SetDSClient skipped: routing engine is not *routing.RoutingEngine (mock or custom implementation)")
	}
}

// SetDryRun configures dry-run mode for the RO reconciler.
// When enabled, the pipeline stops after AI analysis without creating WFE or EA.
// holdPeriod controls how long the Gateway suppresses re-triggering for the same fingerprint.
// #712, #736: Called from cmd/remediationorchestrator/main.go.
// Thread-safe: acquires configMu write lock (#835, DD-INFRA-001).
func (r *Reconciler) SetDryRun(enabled bool, holdPeriod time.Duration) {
	r.configMu.Lock()
	r.dryRun = enabled
	r.dryRunHoldPeriod = holdPeriod
	r.configMu.Unlock()
}

// isDryRun returns whether dry-run mode is enabled.
// Thread-safe: acquires configMu read lock (#835).
func (r *Reconciler) isDryRun() bool {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.dryRun
}

// getDryRunHoldPeriod returns the current dry-run suppression window.
// Thread-safe: acquires configMu read lock (#835).
func (r *Reconciler) getDryRunHoldPeriod() time.Duration {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.dryRunHoldPeriod
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
	}()

	// BR-AUDIT-005 Gap #7: Validate timeout configuration (fail fast on invalid config)
	// Per test requirement: Negative timeouts should be rejected with ERR_INVALID_TIMEOUT_CONFIG
	if err := r.validateTimeoutConfig(rr); err != nil {
		logger.Error(err, "Invalid timeout configuration, transitioning to Failed")
		return r.transitionToFailed(ctx, rr, remediationv1.FailurePhaseConfiguration, err)
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
		rr.Status.OverallPhase == phase.Pending &&
		rr.Status.SignalProcessingRef != nil {
		logger.V(1).Info("⏭️  SKIPPED: No orchestration needed in Pending phase",
			"phase", rr.Status.OverallPhase,
			"generation", rr.Generation,
			"observedGeneration", rr.Status.ObservedGeneration,
			"reason", "ObservedGeneration matches, phase is Pending, and SP already created")
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
		// RO-AUDIT-IDEMPOTENCY: Refetch via apiReader (cache-bypassed) to confirm the
		// phase is genuinely empty. The informer cache is eventually consistent — a second
		// reconcile may start with stale cache showing OverallPhase=="" even after a
		// previous reconcile already set it to Pending in etcd. Without this check, both
		// reconciles enter the initialization block and emit duplicate audit events.
		if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			logger.Error(err, "Failed to refetch RemediationRequest via apiReader")
			return ctrl.Result{}, err
		}
		if rr.Status.OverallPhase != "" {
			// Cache was stale — another reconcile already initialized this RR.
			// Requeue to proceed with the now-initialized phase.
			logger.V(1).Info("Skipped duplicate initialization (stale cache)",
				"name", rr.Name, "phase", rr.Status.OverallPhase)
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}

		logger.Info("Initializing new RemediationRequest", "name", rr.Name)

		// ========================================
		// DD-PERF-001: ATOMIC STATUS UPDATE
		// Initialize phase + StartTime + TimeoutConfig in single API call
		// DD-CONTROLLER-001: ObservedGeneration NOT set here - only after processing phase
		// Gap #8: Initialize TimeoutConfig defaults on first reconcile
		// REFACTOR: Use extracted helper method for timeout initialization
		// ========================================
		if err := r.StatusManager.AtomicStatusUpdate(ctx, rr, func() error {
			// TOCTOU guard: AtomicStatusUpdate refetches the RR internally.
			// Between the apiReader check above and this refetch, a concurrent
			// reconcile or status update may have already set the phase.
			if rr.Status.OverallPhase != "" {
				return errPhaseAlreadySet
			}
			rr.Status.OverallPhase = phase.Pending
			rr.Status.StartTime = &metav1.Time{Time: startTime}

			// Gap #8: Initialize TimeoutConfig with controller defaults
			// REFACTOR: Delegated to populateTimeoutDefaults() for validation + reusability
			r.populateTimeoutDefaults(ctx, rr)

			return nil
		}); err != nil {
			if errors.Is(err, errPhaseAlreadySet) {
				logger.V(1).Info("Initialization aborted (phase set during TOCTOU window)",
					"name", rr.Name, "phase", rr.Status.OverallPhase)
				return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
			}
			logger.Error(err, "Failed to initialize RemediationRequest status")
			return ctrl.Result{}, err
		}

		// RO-AUDIT-IDEMPOTENCY: Safe to emit — the apiReader refetch above confirmed this
		// reconcile is the sole initializer. AtomicStatusUpdate handles optimistic locking
		// conflicts via RetryOnConflict, but the phase transition is guaranteed to have
		// been performed by this reconcile (not a stale-cache duplicate).
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

	// Terminal-phase housekeeping (Issue #88, #265)
	// Handles: TTL enforcement (stamp RetentionExpiryTime, delete expired CRDs),
	// Ready safety net, notification tracking, effectiveness assessment tracking.
	if phase.IsTerminal(phase.Phase(rr.Status.OverallPhase)) {
		logger.V(1).Info("Terminal-phase housekeeping", "phase", rr.Status.OverallPhase)

		// #265: TTL enforcement — delete expired CRDs or stamp expiry for future cleanup.
		// Expired CRDs are deleted immediately (before housekeeping).
		// Non-expired terminal RRs proceed through housekeeping, then requeue at expiry.
		if rr.Status.RetentionExpiryTime != nil && time.Now().After(rr.Status.RetentionExpiryTime.Time) {
			r.emitRetentionCleanupAudit(ctx, rr)
			if err := r.client.Delete(ctx, rr); err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Failed to delete expired RemediationRequest")
					return ctrl.Result{}, err
				}
			}
			logger.Info("Deleted expired RemediationRequest (#265)",
				"retentionExpiryTime", rr.Status.RetentionExpiryTime.Format(time.RFC3339))
			return ctrl.Result{}, nil
		}

		// Stamp expiry on first terminal reconcile (non-blocking — housekeeping continues)
		retention := r.getRetentionPeriod()
		retentionRequeue := retention
		if rr.Status.RetentionExpiryTime == nil {
			expiry := metav1.NewTime(time.Now().Add(retention))
			if err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.RetentionExpiryTime = &expiry
				return nil
			}); err != nil {
				logger.Error(err, "Failed to set RetentionExpiryTime")
			} else {
				logger.Info("RetentionExpiryTime set (#265)", "expiry", expiry.Format(time.RFC3339))
			}
		} else {
			retentionRequeue = time.Until(rr.Status.RetentionExpiryTime.Time)
		}

		// Issue #79 Phase 7c: Safety net for terminal RRs without Ready condition (e.g., externally cancelled)
		readyCondition := remediationrequest.GetCondition(rr, remediationrequest.ConditionReady)
		if readyCondition == nil {
			isSuccess := rr.Status.OverallPhase == remediationv1.PhaseCompleted || rr.Status.OverallPhase == remediationv1.PhaseSkipped
			if isSuccess {
				if updateErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
					remediationrequest.SetReady(rr, true, remediationrequest.ReasonReady, "Terminal phase: "+string(rr.Status.OverallPhase), r.Metrics)
					return nil
				}); updateErr != nil {
					logger.Error(updateErr, "Failed to set Ready safety net")
				}
			} else {
				if updateErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
					remediationrequest.SetReady(rr, false, remediationrequest.ReasonNotReady, "Terminal phase: "+string(rr.Status.OverallPhase), r.Metrics)
					return nil
				}); updateErr != nil {
					logger.Error(updateErr, "Failed to set Ready safety net")
				}
			}
		}

		// BR-ORCH-029/030: Track notification delivery status for terminal RRs
		if err := r.trackNotificationStatus(ctx, rr); err != nil {
			logger.Error(err, "Failed to track notification status in terminal phase")
			// Non-fatal: continue to return
		}

		// ADR-EM-001, GAP-RO-2: Track effectiveness assessment status for terminal RRs
		if err := r.trackEffectivenessStatus(ctx, rr); err != nil {
			logger.Error(err, "Failed to track effectiveness assessment status in terminal phase")
			// Non-fatal: continue to return
		}

		// #1421: Cascade terminal phase to non-terminal child CRDs.
		// Kubernetes-native parent-manages-children: RO is responsible for
		// transitioning children to a terminal state when the parent RR terminates.
		if err := r.cascadeTerminalToChildren(ctx, rr); err != nil {
			logger.Error(err, "Failed to cascade terminal phase to children")
			// Non-fatal: continue to return
		}

		// BR-ORCH-044: Track routing decision - no action needed
		r.Metrics.NoActionNeededTotal.WithLabelValues(string(rr.Status.OverallPhase), rr.Namespace).Inc()

		// #265: Requeue for TTL cleanup at expiry time
		return ctrl.Result{RequeueAfter: retentionRequeue}, nil
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

	// Track notification status (BR-ORCH-029/030)
	// Updates RR status based on NotificationRequest phase changes
	if err := r.trackNotificationStatus(ctx, rr); err != nil {
		logger.Error(err, "Failed to track notification status")
		// Non-fatal: Continue with reconciliation
	}

	// Issue #666: Check phase handler registry first
	currentPhase := phase.Phase(rr.Status.OverallPhase)
	if h, ok := r.phaseRegistry.Lookup(currentPhase); ok {
		intent, err := h.Handle(ctx, rr)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("phase handler %s: %w", currentPhase, err)
		}
		return r.ApplyTransition(ctx, rr, intent)
	}

	// All phase handlers are now registered in the registry. If we reach
	// here, the phase is genuinely unknown.
	logger.Info("Unknown phase", "phase", rr.Status.OverallPhase)
	return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil // REFACTOR-RO-003
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

	// Issue #91: Register field indexes on child CRDs for spec.remediationRequestRef.name
	// Enables MatchingFields queries and kubectl --field-selector for child lookups by parent RR
	childCRDIndexes := []struct {
		obj       client.Object
		extractor func(client.Object) []string
	}{
		{
			obj: &aianalysisv1.AIAnalysis{},
			extractor: func(obj client.Object) []string {
				aa := obj.(*aianalysisv1.AIAnalysis)
				if aa.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{aa.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &notificationv1.NotificationRequest{},
			extractor: func(obj client.Object) []string {
				nr := obj.(*notificationv1.NotificationRequest)
				if nr.Spec.RemediationRequestRef == nil || nr.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{nr.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &signalprocessingv1.SignalProcessing{},
			extractor: func(obj client.Object) []string {
				sp := obj.(*signalprocessingv1.SignalProcessing)
				if sp.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{sp.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &remediationv1.RemediationApprovalRequest{},
			extractor: func(obj client.Object) []string {
				rar := obj.(*remediationv1.RemediationApprovalRequest)
				if rar.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{rar.Spec.RemediationRequestRef.Name}
			},
		},
		{
			obj: &workflowexecutionv1.WorkflowExecution{},
			extractor: func(obj client.Object) []string {
				wfe := obj.(*workflowexecutionv1.WorkflowExecution)
				if wfe.Spec.RemediationRequestRef.Name == "" {
					return nil
				}
				return []string{wfe.Spec.RemediationRequestRef.Name}
			},
		},
	}

	for _, idx := range childCRDIndexes {
		if err := mgr.GetFieldIndexer().IndexField(
			context.Background(),
			idx.obj,
			RemediationRequestRefNameIndex,
			idx.extractor,
		); err != nil {
			if !k8serrors.IsIndexerConflict(err) {
				return fmt.Errorf("failed to create field index %s on %T: %w", RemediationRequestRefNameIndex, idx.obj, err)
			}
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&remediationv1.RemediationRequest{}).
		Owns(&signalprocessingv1.SignalProcessing{}).
		Owns(&aianalysisv1.AIAnalysis{}).
		Owns(&workflowexecutionv1.WorkflowExecution{}).
		Owns(&remediationv1.RemediationApprovalRequest{}).
		Owns(&notificationv1.NotificationRequest{}). // BR-ORCH-029/030: Watch notification lifecycle
		Owns(&eav1.EffectivenessAssessment{}).       // ADR-EM-001, GAP-RO-3: Watch EA for EffectivenessAssessed condition
		// V1.0 P1 FIX: GenerationChangedPredicate removed to allow child CRD status changes
		// Previous optimization filtered status updates, breaking integration tests (RO_INTEGRATION_CRITICAL_BUG_JAN_01_2026.md)
		// Rationale: Correctness > Performance for P0 orchestration service
		// WithEventFilter(predicate.GenerationChangedPredicate{}). // ❌ REMOVED - breaks integration tests
		Complete(r)
}
