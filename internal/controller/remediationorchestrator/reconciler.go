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
	"strings"
	"time"


	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
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
	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/override"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/locking"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
	k8sutil "github.com/jordigilh/kubernaut/pkg/shared/k8s"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
)

// Sentinel errors for phase-conflict detection during status updates.
// These are non-retryable: RetryOnConflict only retries k8s Conflict errors,
// so these propagate immediately to the caller for graceful handling.
var (
	errPhaseAlreadySet = errors.New("phase already initialized by concurrent reconcile")
	errPhaseConflict   = errors.New("phase changed by concurrent reconcile")
)

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

	// #265: CRD retention TTL — how long terminal RRs persist before cleanup.
	// Default: 24h. Configurable via SetRetentionPeriod or YAML config.
	retentionPeriod time.Duration

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
	if timeouts.AwaitingApproval == 0 {
		timeouts.AwaitingApproval = 15 * time.Minute
	}
	if timeouts.Verifying == 0 {
		timeouts.Verifying = 30 * time.Minute
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
		approvalCreator:     creator.NewApprovalCreator(c, s, m, timeouts.AwaitingApproval),
		timeouts:            timeouts,
		auditStore:          auditStore,
		auditManager:        roaudit.NewManager(roaudit.ServiceName),
		notificationHandler: NewNotificationHandler(c, m),
		routingEngine:       routingEngine, // Use provided or default routing engine
		Metrics:             m,
		Recorder:            recorder,
		StatusManager:       statusManager,       // DD-PERF-001: Atomic status updates
		apiReader:           apiReader,            // DD-EM-002: Uncached reader for pre-remediation hash
		retentionPeriod:     24 * time.Hour,       // #265: Default CRD TTL
		phaseRegistry:       phase.NewRegistry(),  // Issue #666: Phase handler registry
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
	r.phaseRegistry.MustRegister(NewExecutingHandler(c, apiReader, r.statusAggregator, statusManager, m))
	r.phaseRegistry.MustRegister(NewBlockedHandler(m, BlockedCallbacks{
		RecheckResourceBusyBlock:      r.recheckResourceBusyBlock,
		RecheckDuplicateBlock:         r.recheckDuplicateBlock,
		HandleUnmanagedResourceExpiry: r.handleUnmanagedResourceExpiry,
		TransitionToFailedTerminal:    r.transitionToFailedTerminal,
	}))
	wfeCallbacks := WFECreationCallbacks{
		EmitWorkflowCreatedAudit: r.emitWorkflowCreatedAudit,
		CreateWFE:                func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, error) { return r.weCreator.Create(ctx, rr, ai) },
		ResolveWorkflowDisplay:   r.resolveWorkflowDisplay,
	}
	r.phaseRegistry.MustRegister(NewAnalyzingHandler(c, m, AnalyzingCallbacks{
		AtomicStatusUpdate: func(ctx context.Context, rr *remediationv1.RemediationRequest, fn func() error) error {
			return r.StatusManager.AtomicStatusUpdate(ctx, rr, fn)
		},
		IsWorkflowNotNeeded:            handler.IsWorkflowNotNeeded,
		HandleWorkflowNotNeeded:        r.aiAnalysisHandler.HandleAIAnalysisStatus,
		CreateApproval:                 func(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, error) { return r.approvalCreator.Create(ctx, rr, ai) },
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
		CapturePreRemediationHash: func(ctx context.Context, kind, name, namespace string) (string, string, error) {
			return CapturePreRemediationHash(ctx, r.apiReader, r.restMapper, kind, name, namespace)
		},
		ResolveDualTargets: func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult {
			dt := resolveDualTargets(rr, ai)
			return DualTargetResult{Remediation: TargetRef{Kind: dt.Remediation.Kind, Name: dt.Remediation.Name, Namespace: dt.Remediation.Namespace}}
		},
		PersistPreHash: func(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error {
			return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.PreRemediationSpecHash = preHash
				remediationrequest.SetPreRemediationHashCaptured(rr, true,
					fmt.Sprintf("Pre-remediation hash captured"), r.Metrics)
				return nil
			})
		},
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
		CapturePreRemediationHash: func(ctx context.Context, kind, name, namespace string) (string, string, error) {
			return CapturePreRemediationHash(ctx, r.apiReader, r.restMapper, kind, name, namespace)
		},
		ResolveDualTargets: func(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) DualTargetResult {
			dt := resolveDualTargets(rr, ai)
			return DualTargetResult{Remediation: TargetRef{Kind: dt.Remediation.Kind, Name: dt.Remediation.Name, Namespace: dt.Remediation.Namespace}}
		},
		PersistPreHash: func(ctx context.Context, rr *remediationv1.RemediationRequest, preHash string) error {
			return helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.PreRemediationSpecHash = preHash
				remediationrequest.SetPreRemediationHashCaptured(rr, true,
					fmt.Sprintf("Pre-remediation hash captured"), r.Metrics)
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
		WFECallbacks:       wfeCallbacks,
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
func (r *Reconciler) SetRetentionPeriod(d time.Duration) {
	if d > 0 {
		r.retentionPeriod = d
	}
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
				"retentionExpiryTime", rr.Status.RetentionExpiryTime.Time.Format(time.RFC3339))
			return ctrl.Result{}, nil
		}

		// Stamp expiry on first terminal reconcile (non-blocking — housekeeping continues)
		retentionRequeue := r.retentionPeriod
		if rr.Status.RetentionExpiryTime == nil {
			expiry := metav1.NewTime(time.Now().Add(r.retentionPeriod))
			if err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.RetentionExpiryTime = &expiry
				return nil
			}); err != nil {
				logger.Error(err, "Failed to set RetentionExpiryTime")
			} else {
				logger.Info("RetentionExpiryTime set (#265)", "expiry", expiry.Time.Format(time.RFC3339))
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

		// BR-ORCH-044: Track routing decision - no action needed
		r.Metrics.NoActionNeededTotal.WithLabelValues(rr.Namespace, string(rr.Status.OverallPhase)).Inc()

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

// transitionToInheritedCompleted transitions the RR to Completed with outcome "Remediated".
// Used when an original resource (WFE or RR) that caused deduplication completes successfully.
// The outcome is "Remediated" (not a separate "InheritedCompleted") because the CRD enum
// only allows Remediated|NoActionRequired|ManualReviewRequired|VerificationTimedOut, and
// the dedup lineage is already preserved via DeduplicatedByWE/DuplicateOf fields + K8s events.
// sourceRef identifies the original resource name; sourceKind is "WorkflowExecution" or "RemediationRequest".
func (r *Reconciler) transitionToInheritedCompleted(ctx context.Context, rr *remediationv1.RemediationRequest, sourceRef, sourceKind string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	oldPhase := rr.Status.OverallPhase
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		now := metav1.Now()
		rr.Status.OverallPhase = phase.Completed
		rr.Status.Outcome = "Remediated"
		rr.Status.CompletedAt = &now
		rr.Status.ObservedGeneration = rr.Generation
		if sourceKind == "RemediationRequest" {
			rr.Status.BlockReason = ""
			rr.Status.BlockMessage = ""
			rr.Status.DuplicateOf = ""
		}
		remediationrequest.SetReady(rr, true, remediationrequest.ReasonReady,
			fmt.Sprintf("Inherited completion from original %s", sourceKind), r.Metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to inherited Completed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to inherited Completed: %w", err)
	}

	if oldPhase != phase.Completed {
		if r.Metrics != nil {
			r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(phase.Completed), rr.Namespace).Inc()
		}
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonInheritedCompleted,
				fmt.Sprintf("Remediation inherited Completed from original %s %s", sourceKind, sourceRef))
		}

		if rr.Status.StartTime != nil {
			r.emitCompletionAudit(ctx, rr, "InheritedCompleted", time.Since(rr.Status.StartTime.Time).Milliseconds())
		}

		// F-3: Only create notifications for WFE-level inheritance — DuplicateInProgress
		// RRs never reached AIAnalysis phase, so ensureNotificationsCreated would fail.
		if sourceKind == "WorkflowExecution" {
			r.ensureNotificationsCreated(ctx, rr)
		}
	}

	logger.Info("RR inherited Completed",
		"inheritedFrom", sourceRef, "sourceKind", sourceKind, "outcome", "InheritedCompleted")
	return ctrl.Result{}, nil
}

// transitionToInheritedFailed transitions the RR to Failed with FailurePhaseDeduplicated.
// Used when the original resource (WFE or RR) fails or is deleted (dangling reference).
// Does NOT increment ConsecutiveFailureCount — inherited failures are excluded from blocking.
// sourceRef identifies the original resource name; sourceKind is "WorkflowExecution" or "RemediationRequest".
func (r *Reconciler) transitionToInheritedFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failureErr error, sourceRef, sourceKind string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	oldPhase := rr.Status.OverallPhase
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		now := metav1.Now()
		failPhase := remediationv1.FailurePhaseDeduplicated
		rr.Status.OverallPhase = phase.Failed
		rr.Status.FailurePhase = &failPhase
		rr.Status.FailureReason = &failureReason
		rr.Status.CompletedAt = &now
		rr.Status.ObservedGeneration = rr.Generation
		if sourceKind == "RemediationRequest" {
			rr.Status.BlockReason = ""
			rr.Status.BlockMessage = ""
			rr.Status.DuplicateOf = ""
		}
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonNotReady,
			fmt.Sprintf("Inherited failure from original %s", sourceKind), r.Metrics)
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to inherited Failed")
		return ctrl.Result{}, fmt.Errorf("failed to transition to inherited Failed: %w", err)
	}

	if oldPhase != phase.Failed {
		if r.Metrics != nil {
			r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(phase.Failed), rr.Namespace).Inc()
		}
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonInheritedFailed,
				fmt.Sprintf("Remediation inherited Failed from original %s %s: %s", sourceKind, sourceRef, failureReason))
		}

		durationMs := time.Since(rr.CreationTimestamp.Time).Milliseconds()
		r.emitFailureAudit(ctx, rr, remediationv1.FailurePhaseDeduplicated, failureErr, durationMs)
	}

	logger.Info("RR inherited Failed",
		"inheritedFrom", sourceRef, "sourceKind", sourceKind, "reason", failureReason)
	return ctrl.Result{}, nil
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
		case remediationv1.BlockReasonIneffectiveChain:
			r.Recorder.Event(rr, corev1.EventTypeWarning, "IneffectiveChainDetected",
				fmt.Sprintf("Escalating to manual review: %s", blocked.Message))
		}
	}

	// Issue #803: Create ManualReview NotificationRequest for IneffectiveChain blocks (BR-ORCH-036).
	// Previously, this relied on a non-existent "notification controller watching for ManualReviewRequired".
	if remediationv1.BlockReason(blocked.Reason) == remediationv1.BlockReasonIneffectiveChain {
		logger.Info("Ineffective chain detected - escalating to manual review",
			"remediationRequest", rr.Name)

		nrName := fmt.Sprintf("nr-manual-review-%s", rr.Name)
		if !hasNotificationRef(rr, nrName) {
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceRoutingEngine,
				Reason:  "IneffectiveChain",
				Message: blocked.Message,
			}
			notifName, notifErr := r.notificationCreator.CreateManualReviewNotification(ctx, rr, reviewCtx)
			if notifErr != nil {
				logger.Error(notifErr, "Failed to create manual review notification for IneffectiveChain block")
			} else {
				logger.Info("Created manual review notification for IneffectiveChain block", "notification", notifName)
				ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
				if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
					rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
					return nil
				}); refErr != nil {
					logger.Error(refErr, "Failed to persist IneffectiveChain NR ref (non-critical)", "notification", notifName)
				}
				if r.Recorder != nil {
					r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
						fmt.Sprintf("Manual review notification created: %s", notifName))
				}
			}
		}
	}

	// GAP-6 / #810: Create block notification for non-IneffectiveChain block reasons (BR-ORCH-036, BR-ORCH-042.5).
	if remediationv1.BlockReason(blocked.Reason) != remediationv1.BlockReasonIneffectiveChain {
		blockNRName := fmt.Sprintf("nr-block-%s-%s", strings.ToLower(blocked.Reason), rr.Name)
		if !hasNotificationRef(rr, blockNRName) {
			blockCtx := &creator.BlockNotificationContext{
				BlockReason:  blocked.Reason,
				BlockMessage: blocked.Message,
			}
			notifName, notifErr := r.notificationCreator.CreateBlockNotification(ctx, rr, blockCtx)
			if notifErr != nil {
				logger.Error(notifErr, "Failed to create block notification (non-critical)", "blockReason", blocked.Reason)
			} else {
				logger.Info("Created block notification", "notification", notifName, "blockReason", blocked.Reason)
				ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
				if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
					rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
					return nil
				}); refErr != nil {
					logger.Error(refErr, "Failed to persist block NR ref (non-critical)", "notification", notifName)
				}
				if r.Recorder != nil {
					r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
						fmt.Sprintf("Block notification created: %s", notifName))
				}
			}
		}
	}

	// Update RR status to Blocked phase (REFACTOR-RO-001: using retry helper)
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseBlocked
		rr.Status.BlockReason = remediationv1.BlockReason(blocked.Reason)
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

		// Issue #214: Set ManualReviewRequired for IneffectiveChain blocks
		if remediationv1.BlockReason(blocked.Reason) == remediationv1.BlockReasonIneffectiveChain {
			rr.Status.Outcome = "ManualReviewRequired"
			rr.Status.RequiresManualReview = true
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

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Phase conflict guard: the reconcile loop selects a phase handler based on
		// the informer cache (potentially stale). This updateFn runs after
		// UpdateRemediationRequestStatus refetches the actual etcd state. If the
		// phase has diverged, another reconcile already changed it — abort to
		// avoid overwriting a legitimate state change (e.g., Blocked → Processing).
		if rr.Status.OverallPhase != oldPhase {
			return fmt.Errorf("%w: expected %s, got %s", errPhaseConflict,
				oldPhase, rr.Status.OverallPhase)
		}
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

		// Issue #636: Set Ready condition with phase-specific reason so that
		// `kubectl get rr` REASON column reflects the current pipeline stage.
		switch newPhase {
		case phase.Processing:
			remediationrequest.SetReady(rr, false, remediationrequest.ReasonProcessing, "Signal processing in progress", r.Metrics)
		case phase.Analyzing:
			remediationrequest.SetReady(rr, false, remediationrequest.ReasonAnalyzing, "AI analysis in progress", r.Metrics)
		case phase.AwaitingApproval:
			remediationrequest.SetReady(rr, false, remediationrequest.ReasonAwaitingApproval, "Waiting for human approval", r.Metrics)
		case phase.Executing:
			remediationrequest.SetReady(rr, false, remediationrequest.ReasonExecuting, "Workflow execution in progress", r.Metrics)
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, errPhaseConflict) {
			logger.Info("Phase conflict detected — requeueing for fresh state",
				"expectedPhase", oldPhase,
				"actualPhase", rr.Status.OverallPhase,
				"targetPhase", newPhase)
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
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

// transitionToVerifying transitions the RR to Verifying phase (#280).
// After WFE completes successfully, the RR enters Verifying (non-terminal) while the
// EffectivenessAssessment runs. The Gateway deduplicates signals during this window.
// RO transitions to Completed when EA reaches a terminal state (handleVerifyingPhase)
// or when VerificationDeadline expires.
// #281: Notification creation is delegated to ensureNotificationsCreated (idempotent).
// If creation fails here, handleVerifyingPhase retries on subsequent reconciles.
func (r *Reconciler) transitionToVerifying(ctx context.Context, rr *remediationv1.RemediationRequest, outcome string) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)
	// RO-AUDIT-IDEMPOTENCY: Refetch via apiReader (cache-bypassed) before the phase
	// check. Pattern: mirrors transitionToFailed and RAR audit deduplication.
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
		logger.Error(err, "Failed to refetch RemediationRequest via apiReader for idempotency check")
		return ctrl.Result{}, err
	}

	// #280: Idempotency — skip if already in Verifying or Completed
	if rr.Status.OverallPhase == phase.Verifying || rr.Status.OverallPhase == phase.Completed {
		logger.V(1).Info("Already in Verifying/Completed phase (confirmed via apiReader), skipping transition",
			"phase", rr.Status.OverallPhase)
		return ctrl.Result{}, nil
	}

	oldPhaseBeforeTransition := rr.Status.OverallPhase

	// #280: Transition to Verifying (not Completed). CompletedAt and Outcome are set later
	// when EA finishes or VerificationDeadline expires.
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Verifying
		rr.Status.ObservedGeneration = rr.Generation

		// BR-ORCH-043: Set Ready condition (remediation succeeded, verification pending)
		remediationrequest.SetReady(rr, true, remediationrequest.ReasonVerifying, "Remediation completed, verifying effectiveness", r.Metrics)

		// DD-WE-004 V1.0: Reset exponential backoff on success
		if rr.Status.NextAllowedExecution != nil {
			logger.Info("Clearing exponential backoff after successful remediation",
				"previousNextAllowed", rr.Status.NextAllowedExecution.Format(time.RFC3339),
				"previousConsecutiveFailures", rr.Status.ConsecutiveFailureCount)
			rr.Status.NextAllowedExecution = nil
		}
		rr.Status.ConsecutiveFailureCount = 0

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to Verifying")
		return ctrl.Result{}, fmt.Errorf("failed to transition to Verifying: %w", err)
	}

	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhaseBeforeTransition), string(phase.Verifying), rr.Namespace).Inc()
	}

	// Emit audit and K8s event for Verifying transition
	if oldPhaseBeforeTransition != phase.Verifying {
		r.emitVerifyingStartedAudit(ctx, rr)
		if r.Recorder != nil {
			r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonRemediationCompleted,
				"Remediation succeeded, entering verification phase (#280)")
		}
	}

	// #304: Notification creation deferred until after Outcome is set (completeVerificationIfNeeded).
	// ensureNotificationsCreated is called from handleVerifyingPhase after EA terminal transition
	// or timeout sets Outcome. Previously called here with empty Outcome (BR-ORCH-045 violation).

	// #280: Create EA — if this fails, handleVerifyingPhase will retry on next reconcile
	r.createEffectivenessAssessmentIfNeeded(ctx, rr)

	logger.Info("Remediation succeeded, entered Verifying phase", "outcome", outcome)
	return ctrl.Result{}, nil
}

// handleVerifyingPhase is now handled by VerifyingHandler via the phase registry.
// See verifying_handler.go (Issue #666, TP-666-v1 §8.1).

// VerificationDeadlineBuffer is the grace period added to EA.Status.ValidityDeadline
// when computing VerificationDeadline. Allows for clock skew and final status propagation.
const VerificationDeadlineBuffer = 30 * time.Second

// transitionToFailed transitions the RR to Failed phase.
// BR-ORCH-042: Before transitioning to terminal Failed, checks if this failure
// triggers consecutive failure blocking (≥3 consecutive failures for same fingerprint).
// If blocking is triggered, transitions to non-terminal Blocked phase instead.
func (r *Reconciler) transitionToFailed(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// F-6: Derive string reason from error for status fields and logging
	failureReason := ""
	if failureErr != nil {
		failureReason = failureErr.Error()
	}

	// BR-ORCH-042: Log consecutive failures for observability
	// NOTE: This RR transitions to Failed (terminal state).
	// FUTURE RRs with same fingerprint will be blocked in Pending phase (routing check).
	if failurePhase != remediationv1.FailurePhaseBlocked {
		// Count consecutive failures BEFORE this one (current failure not yet recorded)
		consecutiveFailures := r.countConsecutiveFailures(ctx, rr.Spec.SignalFingerprint)

		// +1 for this failure (not yet in status)
		if consecutiveFailures+1 >= r.getConsecutiveFailureThreshold() {
			logger.Info("Consecutive failure threshold reached, future RRs will be blocked",
				"consecutiveFailures", consecutiveFailures+1,
				"threshold", r.getConsecutiveFailureThreshold(),
				"fingerprint", rr.Spec.SignalFingerprint,
			)
			// Do NOT transition this RR to Blocked - it failed and should go to Failed.
			// The routing engine will block FUTURE RRs for this fingerprint.
		}
	}

	// RO-AUDIT-IDEMPOTENCY: Refetch via apiReader (cache-bypassed) before the phase
	// check. The informer cache is eventually consistent — a second reconcile may
	// start with stale cache showing non-Failed phase after the first reconcile has
	// already transitioned, causing duplicate orchestrator.lifecycle.completed events.
	// Pattern: mirrors RAR audit deduplication (remediation_approval_request.go).
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
		logger.Error(err, "Failed to refetch RemediationRequest via apiReader for idempotency check")
		return ctrl.Result{}, err
	}

	if rr.Status.OverallPhase == phase.Failed {
		logger.V(1).Info("Already in Failed phase (confirmed via apiReader), skipping transition")
		return ctrl.Result{}, nil
	}

	// GAP-4 / #808: Create escalation NR for terminal failures (BR-ORCH-036).
	// Guard: skip if caller already created a ManualReview or Escalation NR.
	manualReviewNR := fmt.Sprintf("nr-manual-review-%s", rr.Name)
	escalationNR := fmt.Sprintf("nr-escalation-%s", rr.Name)
	if !hasNotificationRef(rr, manualReviewNR) && !hasNotificationRef(rr, escalationNR) {
		escCtx := &creator.EscalationContext{
			FailurePhase:  string(failurePhase),
			FailureReason: failureReason,
		}
		notifName, notifErr := r.notificationCreator.CreateEscalationNotification(ctx, rr, escCtx)
		if notifErr != nil {
			logger.Error(notifErr, "Failed to create escalation notification (non-critical)")
		} else {
			logger.Info("Created escalation notification for terminal failure", "notification", notifName)
			ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
			if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
				return nil
			}); refErr != nil {
				logger.Error(refErr, "Failed to persist escalation NR ref (non-critical)", "notification", notifName)
			}
			if r.Recorder != nil {
				r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
					fmt.Sprintf("Escalation notification created: %s", notifName))
			}
		}
	}

	// Capture old phase for metrics and audit
	oldPhaseBeforeTransition := rr.Status.OverallPhase
	startTime := rr.CreationTimestamp.Time

	// REFACTOR-RO-001: Using retry helper
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = phase.Failed
		rr.Status.ObservedGeneration = rr.Generation // DD-CONTROLLER-001: Track final generation
		rr.Status.FailurePhase = &failurePhase
		rr.Status.FailureReason = &failureReason
		now := metav1.Now()
		rr.Status.CompletedAt = &now // #265 F3: CompletedAt on all terminal transitions

		// BR-ORCH-043: Set Ready condition (terminal failure)
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonRemediationFailed, "Remediation failed", r.Metrics)

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

	// Issue #240: EA is NOT created on failure paths. EA should only be created
	// when WFE completes successfully (transitionToVerifying), because failed/timed-out
	// remediations may have partially applied or no changes, making EA unreliable.

	logger.Info("Remediation failed", "failurePhase", failurePhase, "reason", failureReason)
	return ctrl.Result{}, nil
}

// handleGlobalTimeout transitions the RR to TimedOut phase when global timeout exceeded.
// BR-ORCH-027: Global Timeout Management
// Business Value: Prevents stuck remediations from consuming resources indefinitely
// Default timeout: 1 hour from CreationTimestamp
func (r *Reconciler) handleGlobalTimeout(ctx context.Context, rr *remediationv1.RemediationRequest) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	timeoutPhase := remediationv1.RemediationPhase(rr.Status.OverallPhase)
	oldPhase := rr.Status.OverallPhase

	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.OverallPhase = remediationv1.PhaseTimedOut
		now := metav1.Now()
		rr.Status.TimeoutTime = &now
		rr.Status.TimeoutPhase = &timeoutPhase
		rr.Status.CompletedAt = &now // #265 F3: CompletedAt on all terminal transitions

		// BR-ORCH-043: Set Ready condition (terminal timeout)
		remediationrequest.SetReady(rr, false, remediationrequest.ReasonRemediationTimedOut, "Remediation timed out", r.Metrics)

		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to transition to TimedOut")
		return ctrl.Result{}, fmt.Errorf("failed to transition to TimedOut: %w", err)
	}

	// Record metrics (BR-ORCH-044)
	if r.Metrics != nil {
		r.Metrics.PhaseTransitionsTotal.WithLabelValues(string(oldPhase), string(remediationv1.PhaseTimedOut), rr.Namespace).Inc()
		r.Metrics.TimeoutsTotal.WithLabelValues(rr.Namespace, string(timeoutPhase)).Inc()
	}

	// Per DD-AUDIT-003: Emit timeout event (lifecycle.completed with outcome=failure)
	if rr.Status.StartTime != nil {
		durationMs := time.Since(rr.Status.StartTime.Time).Milliseconds()
		r.emitTimeoutAudit(ctx, rr, "global", string(timeoutPhase), durationMs)
	}

	// DD-EVENT-001: Emit K8s event for global timeout (BR-ORCH-095)
	if r.Recorder != nil {
		r.Recorder.Event(rr, corev1.EventTypeWarning, events.EventReasonRemediationTimeout,
			fmt.Sprintf("Global timeout exceeded during %s phase", string(timeoutPhase)))
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
		},
		Spec: notificationv1.NotificationRequestSpec{
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: notificationv1.NotificationPriorityCritical,
			Severity: rr.Spec.Severity,
			Subject:  fmt.Sprintf("Remediation Timeout: %s", rr.Spec.SignalName),
			Body: r.notificationCreator.BuildGlobalTimeoutBody(
				rr.Spec.SignalName,
				rr.Name,
				string(timeoutPhase),
				r.getEffectiveGlobalTimeout(rr).String(),
				rr.Status.StartTime.Format(time.RFC3339),
				rr.Status.TimeoutTime.Format(time.RFC3339),
			),
			Context: buildTimeoutContext(rr.Name, string(timeoutPhase), "", rr.Spec.TargetResource),
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
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Timeout notification already exists (concurrent create), continuing", "notificationName", notificationName)
		} else {
			logger.Error(err, "Failed to create timeout notification",
				"notificationName", notificationName)
			return ctrl.Result{}, nil
		}
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
	err = helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
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

	// Issue #240: EA is NOT created on global timeout. See transitionToVerifying.

	return ctrl.Result{}, nil
}

// createEffectivenessAssessmentIfNeeded creates an EA CRD if the eaCreator is wired.
// ADR-EM-001: EA creation is ALWAYS non-fatal. The terminal phase transition must succeed
// even if EA creation fails. Errors are logged but not propagated.
// BR-HAPI-191: Resolves the target from AIAnalysis.RemediationTarget when available,
// so the EA assesses the resource the workflow actually modified (not the signal Pod).
// Batch 3: After creating the EA, persists the EffectivenessAssessmentRef on the RR status
// so that trackEffectivenessStatus can find the EA for condition tracking.
func (r *Reconciler) createEffectivenessAssessmentIfNeeded(ctx context.Context, rr *remediationv1.RemediationRequest) {
	if r.eaCreator == nil {
		return
	}

	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	// DD-EM-003: Resolve dual targets for the EA.
	// Signal target: from RR (always available).
	// Remediation target: from AIAnalysis RemediationTarget (when available), else RR fallback.
	var dualTarget *creator.DualTarget
	var isGitOpsManaged bool
	var ai *aianalysisv1.AIAnalysis
	if rr.Status.AIAnalysisRef != nil {
		ai = &aianalysisv1.AIAnalysis{}
		if err := r.client.Get(ctx, client.ObjectKey{
			Name:      rr.Status.AIAnalysisRef.Name,
			Namespace: rr.Status.AIAnalysisRef.Namespace,
		}, ai); err != nil {
			logger.V(1).Info("Could not fetch AIAnalysis for target resolution (non-fatal), using RR target",
				"error", err)
			ai = nil
		} else {
			dualTarget = resolveDualTargets(rr, ai)
			// DD-EM-004, BR-RO-103.2: Read GitOps detection from RCA pipeline.
			if ai.Status.PostRCAContext != nil &&
				ai.Status.PostRCAContext.DetectedLabels != nil &&
				ai.Status.PostRCAContext.DetectedLabels.GitOpsManaged {
				isGitOpsManaged = true
			}
		}
	}

	// DD-EM-004 v2.0, BR-RO-103, Issue #253, #277: Detect async-managed targets.
	// Compute Duration-based hashComputeDelay for the EA Config.
	var hashComputeDelay *metav1.Duration
	remediationKind := rr.Spec.TargetResource.Kind
	if dualTarget != nil {
		remediationKind = dualTarget.Remediation.Kind
	}

	isCRD := false
	gvk, err := k8sutil.ResolveGVKForKind(r.restMapper, remediationKind)
	if err != nil {
		logger.V(1).Info("Cannot resolve GVK for kind, treating as sync target for hash timing",
			"kind", remediationKind, "error", err)
	} else if !creator.IsBuiltInGroup(gvk.Group) {
		isCRD = true
	}

	propagationDelay := r.asyncPropagation.ComputePropagationDelay(isGitOpsManaged, isCRD)
	if propagationDelay > 0 {
		hashComputeDelay = &metav1.Duration{Duration: propagationDelay}
		logger.Info("Async-managed target detected, setting hash check delay",
			"kind", remediationKind,
			"gitOps", isGitOpsManaged,
			"isCRD", isCRD,
			"hashComputeDelay", propagationDelay)
	}

	// #277: Detect proactive signals via AIAnalysis.Spec.AnalysisRequest.SignalContext.SignalMode.
	// Proactive alerts (e.g. predict_linear) need extra time to resolve.
	var alertCheckDelay *metav1.Duration
	if ai != nil && ai.Spec.AnalysisRequest.SignalContext.SignalMode == "proactive" {
		if r.asyncPropagation.ProactiveAlertDelay > 0 {
			alertCheckDelay = &metav1.Duration{Duration: r.asyncPropagation.ProactiveAlertDelay}
			logger.Info("Proactive signal detected, setting alert check delay",
				"signalMode", ai.Spec.AnalysisRequest.SignalContext.SignalMode,
				"alertCheckDelay", r.asyncPropagation.ProactiveAlertDelay)
		}
	}

	name, err := r.eaCreator.CreateEffectivenessAssessment(ctx, rr, dualTarget, hashComputeDelay, alertCheckDelay)
	if err != nil {
		logger.Error(err, "Failed to create EffectivenessAssessment (non-fatal per ADR-EM-001)")
		return
	}
	logger.Info("EffectivenessAssessment created", "eaName", name, "rrPhase", rr.Status.OverallPhase)

	// #277: Emit orchestrator.ea.created audit event with propagation delay breakdown.
	r.emitEACreatedAudit(ctx, rr, name, hashComputeDelay, alertCheckDelay, isGitOpsManaged, isCRD)

	// ADR-EM-001, Batch 3: Persist EA ref on RR status for trackEffectivenessStatus.
	// Uses helpers.UpdateRemediationRequestStatus for atomic persistence (same pattern
	// as NT ref tracking in handleGlobalTimeout).
	// GAP-2 (ADR-EM-001 Section 9.4.15): Also set initial EffectivenessAssessed=False /
	// AssessmentInProgress so operators can distinguish "no EA yet" from "EA in progress."
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       name,
			Namespace:  rr.Namespace,
			APIVersion: eav1.GroupVersion.String(),
		}
		meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
			Type:    ConditionEffectivenessAssessed,
			Status:  metav1.ConditionFalse,
			Reason:  "AssessmentInProgress",
			Message: fmt.Sprintf("EffectivenessAssessment %s created, assessment in progress", name),
		})
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist EA ref on RR status (non-critical)", "eaName", name)
	}
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
// before WorkflowExecution creation. Includes the pre-remediation spec hash
// and selected workflow metadata for the audit trail.
// ADR-EM-001 Section 9.1, GAP-RO-1, DD-EM-002.
// Non-blocking — failures are logged but don't affect business logic.
//
// The preHash parameter is the hash already captured by the caller (sites 1/2)
// via CapturePreRemediationHash. This avoids a redundant uncached API read of
// the same target resource that was just hashed moments before.
func (r *Reconciler) emitWorkflowCreatedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis, preHash string) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		logger.Error(fmt.Errorf("auditStore is nil"),
			"CRITICAL: Cannot record workflow_created audit event - violates ADR-032 §1",
			"remediationRequest", rr.Name)
		return
	}

	correlationID := rr.Name
	// DD-EM-003: Use remediation target for audit trail consistency
	remTarget := resolveDualTargets(rr, ai).Remediation
	targetResource := fmt.Sprintf("%s/%s/%s", remTarget.Namespace, remTarget.Kind, remTarget.Name)

	// Extract workflow metadata from AIAnalysis status
	var workflowID, workflowVersion, actionType string
	if ai.Status.SelectedWorkflow != nil {
		workflowID = ai.Status.SelectedWorkflow.WorkflowID
		workflowVersion = ai.Status.SelectedWorkflow.Version
		actionType = ai.Status.SelectedWorkflow.ActionType
	}

	event, err := r.auditManager.BuildRemediationWorkflowCreatedEvent(
		correlationID, rr.Namespace, rr.Name,
		roaudit.RemediationWorkflowCreatedData{
			PreRemediationSpecHash: preHash,
			TargetResource:         targetResource,
			WorkflowID:             workflowID,
			WorkflowVersion:        workflowVersion,
			ActionType:             actionType,
			SignalType:             rr.Spec.SignalType,
			SignalFingerprint:      rr.Spec.SignalFingerprint,
		},
	)
	if err != nil {
		logger.Error(err, "Failed to build workflow_created audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store workflow_created audit event")
	}
}

// emitEACreatedAudit emits the orchestrator.ea.created audit event with propagation
// delay breakdown. Issue #277: The RO is the source of truth for these delays.
// Non-blocking — failures are logged but don't affect business logic.
func (r *Reconciler) emitEACreatedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, eaName string, hashComputeDelay, alertCheckDelay *metav1.Duration, isGitOpsManaged, isCRD bool) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		return
	}

	data := roaudit.EACreatedData{
		EAName:          eaName,
		IsGitOpsManaged: isGitOpsManaged,
		IsCRD:           isCRD,
	}
	if hashComputeDelay != nil {
		data.HashComputeDelay = hashComputeDelay.Duration
	}
	if alertCheckDelay != nil {
		data.AlertCheckDelay = alertCheckDelay.Duration
	}
	if isGitOpsManaged {
		data.GitOpsSyncDelay = r.asyncPropagation.GitOpsSyncDelay
	}
	if isCRD {
		data.OperatorReconcileDelay = r.asyncPropagation.OperatorReconcileDelay
	}

	event, err := r.auditManager.BuildEACreatedEvent(rr.Name, rr.Namespace, rr.Name, data)
	if err != nil {
		logger.Error(err, "Failed to build EA created audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store EA created audit event")
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

// emitVerifyingStartedAudit emits an audit event when RR enters the Verifying phase (#280).
// Non-blocking — failures are logged but don't affect business logic.
func (r *Reconciler) emitVerifyingStartedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)
	if r.auditStore == nil {
		return
	}

	event, err := r.auditManager.BuildLifecycleVerifyingStartedEvent(rr.Name, rr.Namespace, rr.Name)
	if err != nil {
		logger.Error(err, "Failed to build verifying_started audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store verifying_started audit event")
	}
}

// emitVerificationCompletedAudit emits an audit event when Verifying -> Completed (#280).
func (r *Reconciler) emitVerificationCompletedAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)
	if r.auditStore == nil {
		return
	}

	eaName := ""
	if rr.Status.EffectivenessAssessmentRef != nil {
		eaName = rr.Status.EffectivenessAssessmentRef.Name
	}
	durationMs := time.Since(rr.CreationTimestamp.Time).Milliseconds()
	event, err := r.auditManager.BuildLifecycleVerificationCompletedEvent(
		rr.Name, rr.Namespace, rr.Name, eaName, rr.Status.Outcome, durationMs)
	if err != nil {
		logger.Error(err, "Failed to build verification_completed audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store verification_completed audit event")
	}
}

// emitVerificationTimedOutAudit emits an audit event when Verifying times out (#280).
func (r *Reconciler) emitVerificationTimedOutAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)
	if r.auditStore == nil {
		return
	}

	eaName := ""
	if rr.Status.EffectivenessAssessmentRef != nil {
		eaName = rr.Status.EffectivenessAssessmentRef.Name
	}
	durationMs := time.Since(rr.CreationTimestamp.Time).Milliseconds()
	event, err := r.auditManager.BuildLifecycleVerificationTimedOutEvent(
		rr.Name, rr.Namespace, rr.Name, eaName, durationMs)
	if err != nil {
		logger.Error(err, "Failed to build verification_timed_out audit event")
		return
	}

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store verification_timed_out audit event")
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
func (r *Reconciler) emitFailureAudit(ctx context.Context, rr *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, failureErr error, durationMs int64) {
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
		string(failurePhase),
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
func (r *Reconciler) emitApprovalRequestedAudit(ctx context.Context, rr *remediationv1.RemediationRequest, confidence float64, workflowID string) {
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

// resolveWorkflowDisplay resolves a workflow UUID to human-readable display
// fields (WorkflowName + ActionType) by querying DataStorage.
// Returns (actionType, workflowName) for use with FormatWorkflowDisplay.
// Falls back to ("", workflowID) if the resolver is nil or DS lookup fails.
// Issue #643 v2: Replaced CRD-based resolution with authoritative DS lookup.
func (r *Reconciler) resolveWorkflowDisplay(ctx context.Context, workflowID string) (string, string) {
	if r.workflowResolver != nil {
		if info := r.workflowResolver.ResolveWorkflowDisplay(ctx, workflowID); info != nil {
			return info.ActionType, info.WorkflowName
		}
	}
	return "", workflowID
}

// emitRetentionCleanupAudit emits an audit event before deleting an expired RR (#265).
// Ensures the audit trail is complete before CRD removal — PostgreSQL is the long-term store.
// Non-blocking: failures are logged but do not prevent deletion.
func (r *Reconciler) emitRetentionCleanupAudit(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx)

	if r.auditStore == nil {
		return
	}

	correlationID := rr.Name
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, roaudit.EventTypeLifecycleCompleted)
	audit.SetEventCategory(event, roaudit.CategoryOrchestration)
	audit.SetEventAction(event, "retention_cleanup")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", roaudit.ServiceName)
	audit.SetResource(event, "RemediationRequest", rr.Name)
	audit.SetCorrelationID(event, correlationID)
	audit.SetNamespace(event, rr.Namespace)

	payload := api.RemediationOrchestratorAuditPayload{
		EventType: api.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCompleted,
		RrName:    rr.Name,
		Namespace: rr.Namespace,
	}
	if rr.Status.RetentionExpiryTime != nil {
		payload.DurationMs.SetTo(time.Since(rr.Status.RetentionExpiryTime.Time).Milliseconds())
	}

	event.EventData = api.NewAuditEventRequestEventDataOrchestratorLifecycleCompletedAuditEventRequestEventData(payload)

	if err := r.auditStore.StoreAudit(ctx, event); err != nil {
		logger.Error(err, "Failed to store retention cleanup audit event")
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
	err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		// Transition to TimedOut phase
		rr.Status.OverallPhase = remediationv1.PhaseTimedOut
		rr.Status.Message = fmt.Sprintf("Phase %s exceeded timeout of %s", phase, timeout)
		rr.Status.TimeoutTime = &metav1.Time{Time: time.Now()}
		rr.Status.TimeoutPhase = &phase
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

	// Issue #240: EA is NOT created on phase timeout. See transitionToVerifying.

	return nil
}

// hasNotificationRef returns true if a NotificationRequest with the given name
// is already tracked in rr.Status.NotificationRequestRefs.
func hasNotificationRef(rr *remediationv1.RemediationRequest, name string) bool {
	for i := range rr.Status.NotificationRequestRefs {
		if rr.Status.NotificationRequestRefs[i].Name == name {
			return true
		}
	}
	return false
}

// ensureNotificationsCreated creates completion and bulk-duplicate notifications
// if they are not yet tracked in NotificationRequestRefs.
// Idempotent: deterministic names + ref check prevent duplicates across reconciles.
// Non-blocking: errors are logged but never propagated.
// #304: Called ONLY after Outcome is set (completeVerificationIfNeeded or timeout transitions).
// Previously called from transitionToVerifying before Outcome was populated (BR-ORCH-045 violation).
// Reference: BR-ORCH-045 (completion), BR-ORCH-034 (bulk duplicate), #281 (retry).
func (r *Reconciler) ensureNotificationsCreated(ctx context.Context, rr *remediationv1.RemediationRequest) {
	logger := log.FromContext(ctx).WithValues("remediationRequest", rr.Name)

	completionName := fmt.Sprintf("nr-completion-%s", rr.Name)
	bulkName := fmt.Sprintf("nr-bulk-%s", rr.Name)

	if !hasNotificationRef(rr, completionName) {
		aiName := fmt.Sprintf("ai-%s", rr.Name)
		ai := &aianalysisv1.AIAnalysis{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: aiName, Namespace: rr.Namespace}, ai); err != nil {
			logger.Error(err, "Failed to fetch AIAnalysis for completion notification, will retry", "aiAnalysis", aiName)
		} else {
			// Issue #518: Read executionEngine from WFE status (resolved at runtime by WE controller).
			executionEngine := ""
			if rr.Status.WorkflowExecutionRef != nil {
				we := &workflowexecutionv1.WorkflowExecution{}
				weKey := client.ObjectKey{Name: rr.Status.WorkflowExecutionRef.Name, Namespace: rr.Status.WorkflowExecutionRef.Namespace}
				if weErr := r.client.Get(ctx, weKey, we); weErr != nil {
					logger.V(1).Info("Could not fetch WFE for executionEngine (best-effort)", "error", weErr)
				} else {
					executionEngine = we.Status.ExecutionEngine
				}
			}
			// #318: Fetch EA for verification summary (graceful degradation: nil if unavailable)
			var ea *eav1.EffectivenessAssessment
			if rr.Status.EffectivenessAssessmentRef != nil {
				eaObj := &eav1.EffectivenessAssessment{}
				eaKey := client.ObjectKey{
					Name:      rr.Status.EffectivenessAssessmentRef.Name,
					Namespace: rr.Namespace,
				}
				if err := r.client.Get(ctx, eaKey, eaObj); err != nil {
					if apierrors.IsNotFound(err) {
						logger.Info("EA not found for verification summary, notification will show 'not available'",
							"ea", eaKey.Name)
					} else {
						logger.Error(err, "Failed to fetch EA for verification summary, notification will show 'not available'",
							"ea", eaKey.Name)
					}
				} else {
					ea = eaObj
				}
			}
			notifName, notifErr := r.notificationCreator.CreateCompletionNotification(ctx, rr, ai, executionEngine, ea)
			if notifErr != nil {
				logger.Error(notifErr, "Failed to create completion notification, will retry")
			} else {
				logger.Info("Created completion notification", "notification", notifName)
				ref := r.buildNotificationRef(ctx, notifName, rr.Namespace)
				if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
					rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
					return nil
				}); refErr != nil {
					logger.Error(refErr, "Failed to persist completion NT ref (non-critical)", "notification", notifName)
				}
				if r.Recorder != nil {
					r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
						fmt.Sprintf("Completion notification created: %s", notifName))
				}
			}
		}
	}

	if rr.Status.DuplicateCount > 0 && !hasNotificationRef(rr, bulkName) {
		name, bulkErr := r.notificationCreator.CreateBulkDuplicateNotification(ctx, rr)
		if bulkErr != nil {
			logger.Error(bulkErr, "Failed to create bulk duplicate notification, will retry")
		} else {
			logger.Info("Created bulk duplicate notification", "notification", name)
			ref := r.buildNotificationRef(ctx, name, rr.Namespace)
			if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
				rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
				return nil
			}); refErr != nil {
				logger.Error(refErr, "Failed to persist bulk NT ref (non-critical)", "notification", name)
			}
			if r.Recorder != nil {
				r.Recorder.Event(rr, corev1.EventTypeNormal, events.EventReasonNotificationCreated,
					fmt.Sprintf("Bulk duplicate notification created: %s", name))
			}
		}
	}
}

// buildNotificationRef fetches the NotificationRequest by name to obtain its UID
// and returns a fully populated ObjectReference (BR-ORCH-035 AC-6).
// If the fetch fails, UID is omitted (best-effort; Name+Namespace still sufficient for lookup).
func (r *Reconciler) buildNotificationRef(ctx context.Context, name, namespace string) corev1.ObjectReference {
	ref := corev1.ObjectReference{
		Kind:       "NotificationRequest",
		Name:       name,
		Namespace:  namespace,
		APIVersion: "notification.kubernaut.ai/v1alpha1",
	}
	nr := &notificationv1.NotificationRequest{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, nr); err == nil {
		ref.UID = nr.UID
	}
	return ref
}

// createPhaseTimeoutNotification creates a notification for phase timeout.
// Non-blocking - logs errors but doesn't fail reconciliation.
// Reference: BR-ORCH-028 (Per-phase timeout escalation)
// buildTimeoutContext constructs the typed notification context for timeout notifications.
// Shared by both global timeout and per-phase timeout notification creation.
func buildTimeoutContext(rrName, timeoutPhase, phaseTimeout string, target remediationv1.ResourceIdentifier) *notificationv1.NotificationContext {
	ctx := &notificationv1.NotificationContext{
		Lineage: &notificationv1.LineageContext{
			RemediationRequest: rrName,
		},
		Execution: &notificationv1.ExecutionContext{
			TimeoutPhase: timeoutPhase,
		},
		Target: &notificationv1.TargetContext{
			TargetResource: fmt.Sprintf("%s/%s", target.Kind, target.Name),
		},
	}
	if phaseTimeout != "" {
		ctx.Execution.PhaseTimeout = phaseTimeout
	}
	return ctx
}

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
		},
		Spec: notificationv1.NotificationRequestSpec{
			RemediationRequestRef: &corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			Type:     notificationv1.NotificationTypeEscalation,
			Priority: notificationv1.NotificationPriorityHigh,
			Severity: rr.Spec.Severity,
			Phase:    string(phase),
			Subject:  fmt.Sprintf("Phase Timeout: %s - %s", phase, rr.Spec.SignalName),
			Body: r.notificationCreator.BuildPhaseTimeoutBody(
				rr.Spec.SignalName,
				rr.Name,
				string(phase),
				timeout.String(),
				safeFormatTime(rr.Status.StartTime),
				safeFormatTime(rr.Status.TimeoutTime),
			),
			Context: buildTimeoutContext(rr.Name, string(phase), timeout.String(), rr.Spec.TargetResource),
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(rr, nr, r.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on phase timeout notification")
		return
	}

	// Create notification (non-blocking)
	if err := r.client.Create(ctx, nr); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Phase timeout notification already exists (concurrent create), continuing",
				"notificationName", notificationName, "phase", phase)
		} else {
			logger.Error(err, "Failed to create phase timeout notification",
				"notificationName", notificationName,
				"phase", phase)
			return
		}
	}

	logger.Info("Created phase timeout notification",
		"notificationName", notificationName,
		"phase", phase,
		"timeout", timeout)

	// BR-ORCH-035 AC-4: Track timeout notification ref (non-blocking)
	ref := r.buildNotificationRef(ctx, notificationName, rr.Namespace)
	if refErr := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
		rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, ref)
		return nil
	}); refErr != nil {
		logger.Error(refErr, "Failed to persist timeout NT ref (non-critical)", "notification", notificationName)
	}
}

// IsTerminalPhase checks if a phase is terminal (no further processing).
// BR-ORCH-042.2: Blocked is NON-terminal (active)
// #280: Verifying is NON-terminal (EA assessment in progress)
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

// getConsecutiveFailureThreshold returns the configured threshold from the routing engine.
func (r *Reconciler) getConsecutiveFailureThreshold() int {
	return r.routingEngine.Config().ConsecutiveFailureThreshold
}

// validateTimeoutConfig validates the timeout configuration in RemediationRequest.Status.TimeoutConfig.
// BR-AUDIT-005 Gap #7: Validates that all timeouts are non-negative.
// Gap #8: TimeoutConfig moved from Spec to Status for operator mutability.
// Returns error with ERR_INVALID_TIMEOUT_CONFIG code if validation fails.
func (r *Reconciler) validateTimeoutConfig(rr *remediationv1.RemediationRequest) error {
	if rr.Status.TimeoutConfig == nil {
		return nil // No custom timeout config, use defaults
	}

	// Validate Global timeout
	if rr.Status.TimeoutConfig.Global != nil && rr.Status.TimeoutConfig.Global.Duration < 0 {
		return fmt.Errorf("global timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Global.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Processing timeout
	if rr.Status.TimeoutConfig.Processing != nil && rr.Status.TimeoutConfig.Processing.Duration < 0 {
		return fmt.Errorf("processing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Processing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Analyzing timeout
	if rr.Status.TimeoutConfig.Analyzing != nil && rr.Status.TimeoutConfig.Analyzing.Duration < 0 {
		return fmt.Errorf("analyzing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Analyzing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	// Validate Executing timeout
	if rr.Status.TimeoutConfig.Executing != nil && rr.Status.TimeoutConfig.Executing.Duration < 0 {
		return fmt.Errorf("executing timeout cannot be negative (got: %v): %w", rr.Status.TimeoutConfig.Executing.Duration, roaudit.ErrInvalidTimeoutConfig)
	}

	return nil
}

// SetRESTMapper sets the REST mapper used by CapturePreRemediationHash to
// resolve Kind strings to GroupVersionKind for the unstructured client.
// DD-EM-002: Called from cmd/remediationorchestrator/main.go after manager setup.
func (r *Reconciler) SetRESTMapper(mapper meta.RESTMapper) {
	r.restMapper = mapper
}

// SetAsyncPropagation configures propagation delays for async-managed targets.
// DD-EM-004 v2.0, Issue #253: Called from cmd/remediationorchestrator/main.go.
func (r *Reconciler) SetAsyncPropagation(cfg roconfig.AsyncPropagationConfig) {
	r.asyncPropagation = cfg
}

// SetNotifySelfResolved enables or disables the self-resolved status-update notification.
// BR-ORCH-037 AC-037-08, Issue #590: Called from cmd/remediationorchestrator/main.go.
func (r *Reconciler) SetNotifySelfResolved(enabled bool) {
	r.aiAnalysisHandler.SetNotifySelfResolved(enabled)
}

// SetClusterIdentity configures the cluster name and UUID for inclusion in notification bodies.
// Issue #615: Called from cmd/remediationorchestrator/main.go after DiscoverIdentity.
func (r *Reconciler) SetClusterIdentity(name, uuid string) {
	r.notificationCreator.SetClusterIdentity(name, uuid)
}

// SetLockManager configures the distributed lock manager for WFE creation safety.
// BR-ORCH-025: Called from cmd/remediationorchestrator/main.go.
// nil = locking disabled (single-replica deployments).
func (r *Reconciler) SetLockManager(lm *locking.DistributedLockManager) {
	r.lockManager = lm
}

// resolveDualTargets resolves both signal and remediation targets for the EA (DD-EM-003).
//
// Signal target: Always from RR.Spec.TargetResource (the resource that triggered the alert).
// Remediation target: Prefers the LLM-identified RemediationTarget from the AIAnalysis
// RootCauseAnalysis. Falls back to RR.Spec.TargetResource when AI analysis is unavailable
// or did not identify a specific resource.
func resolveDualTargets(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) *creator.DualTarget {
	signal := eav1.TargetResource{
		Kind:      rr.Spec.TargetResource.Kind,
		Name:      rr.Spec.TargetResource.Name,
		Namespace: rr.Spec.TargetResource.Namespace,
	}

	remediation := signal
	if ai != nil && ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.RemediationTarget != nil {
		ar := ai.Status.RootCauseAnalysis.RemediationTarget
		if ar.Kind != "" && ar.Name != "" {
			remediation = eav1.TargetResource{
				Kind:      ar.Kind,
				Name:      ar.Name,
				Namespace: ar.Namespace,
			}
		}
	}

	return &creator.DualTarget{Signal: signal, Remediation: remediation}
}

// formatRemediationTargetString builds a "namespace/kind/name" or "kind/name"
// string from an AIAnalysis RemediationTarget. Returns "" if the target is nil.
func formatRemediationTargetString(ai *aianalysisv1.AIAnalysis) string {
	if ai == nil || ai.Status.RootCauseAnalysis == nil || ai.Status.RootCauseAnalysis.RemediationTarget == nil {
		return ""
	}
	ar := ai.Status.RootCauseAnalysis.RemediationTarget
	if ar.Kind == "" || ar.Name == "" {
		return ""
	}
	if ar.Namespace != "" {
		return ar.Namespace + "/" + ar.Kind + "/" + ar.Name
	}
	return ar.Kind + "/" + ar.Name
}

// CapturePreRemediationHash fetches the target resource via an uncached reader
// and computes the canonical resource fingerprint (DD-EM-002 v2.0, #765).
//
// Returns (hash, degradedReason, err) where:
//   - ("sha256:...", "", nil) on success
//   - ("", "", nil) when legitimately no hash: NotFound, unknown GVK
//   - ("", "reason", nil) when degraded: Forbidden, transient API errors (Issue #545 defense-in-depth)
//   - ("", "", err) on hard errors: fingerprint computation failures
//
// This is exported for testability from the test package.
func CapturePreRemediationHash(
	ctx context.Context,
	reader client.Reader,
	restMapper meta.RESTMapper,
	targetKind string,
	targetName string,
	targetNamespace string,
) (string, string, error) {
	logger := log.FromContext(ctx)

	gvk, err := k8sutil.ResolveGVKForKind(restMapper, targetKind)
	if err != nil {
		logger.V(1).Info("Cannot resolve GVK for kind, skipping pre-remediation hash",
			"kind", targetKind, "error", err)
		return "", "", nil
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	key := client.ObjectKey{Name: targetName, Namespace: targetNamespace}
	if err := reader.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Target resource not found, skipping pre-remediation hash",
				"kind", targetKind, "name", targetName, "namespace", targetNamespace)
			return "", "", nil
		}
		reason := fmt.Sprintf("failed to fetch target resource %s/%s: %v", targetKind, targetName, err)
		logger.Info("Pre-remediation hash capture degraded (soft-fail)",
			"kind", targetKind, "name", targetName, "namespace", targetNamespace, "reason", reason)
		return "", reason, nil
	}

	fingerprint, err := canonicalhash.CanonicalResourceFingerprint(obj.Object)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute resource fingerprint for %s/%s: %w", targetKind, targetName, err)
	}

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	configMapHashes := resolveConfigMapHashes(ctx, reader, spec, targetKind, targetNamespace)

	compositeHash, err := canonicalhash.CompositeResourceFingerprint(fingerprint, configMapHashes)
	if err != nil {
		return "", "", fmt.Errorf("failed to compute composite fingerprint for %s/%s: %w", targetKind, targetName, err)
	}

	if len(configMapHashes) > 0 {
		logger.V(1).Info("Pre-remediation composite fingerprint computed",
			"kind", targetKind, "name", targetName,
			"fingerprint", fingerprint, "configMapCount", len(configMapHashes),
			"compositeHash", compositeHash)
	}

	return compositeHash, "", nil
}

// resolveConfigMapHashes extracts ConfigMap references from the resource spec,
// fetches each ConfigMap's data, and returns a map of name -> content hash.
// Missing/forbidden ConfigMaps produce a deterministic sentinel hash.
// Transient errors are logged and the ConfigMap is skipped (non-fatal).
func resolveConfigMapHashes(
	ctx context.Context,
	reader client.Reader,
	spec map[string]interface{},
	kind string,
	namespace string,
) map[string]string {
	refs := canonicalhash.ExtractConfigMapRefs(spec, kind)
	if len(refs) == 0 {
		return nil
	}

	logger := log.FromContext(ctx)
	configMapHashes := make(map[string]string, len(refs))

	for _, cmName := range refs {
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Name: cmName, Namespace: namespace}
		if err := reader.Get(ctx, key, cm); err != nil {
			// All fetch errors (404, 403, transient) use sentinel to ensure deterministic
			// hash computation: the same set of ConfigMap names always contributes to the
			// composite hash, preventing false-positive drift from intermittent failures.
			sentinelData := map[string]string{"__sentinel__": fmt.Sprintf("__absent:%s__", cmName)}
			sentinelHash, hashErr := canonicalhash.ConfigMapDataHash(sentinelData, nil)
			if hashErr != nil {
				logger.Error(hashErr, "Failed to compute sentinel hash for ConfigMap", "configMap", cmName)
				continue
			}
			configMapHashes[cmName] = sentinelHash
			if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
				logger.V(1).Info("ConfigMap not accessible, using sentinel hash",
					"configMap", cmName, "namespace", namespace, "reason", err.Error())
			} else {
				logger.Error(err, "Transient ConfigMap fetch error, using sentinel hash",
					"configMap", cmName, "namespace", namespace)
			}
			continue
		}

		cmHash, err := canonicalhash.ConfigMapDataHash(cm.Data, cm.BinaryData)
		if err != nil {
			logger.Error(err, "Failed to compute hash for ConfigMap, skipping", "configMap", cmName)
			continue
		}
		configMapHashes[cmName] = cmHash
	}

	return configMapHashes
}

