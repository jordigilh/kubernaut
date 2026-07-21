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

// Package workflowexecution provides the WorkflowExecution CRD controller.
//
// Business Purpose (BR-WE-003):
// WorkflowExecution orchestrates Tekton PipelineRuns for workflow execution,
// providing resource locking, exponential backoff, and comprehensive failure reporting.
//
// Key Responsibilities:
// - BR-WE-003: Monitor execution status and sync with PipelineRun
// - BR-WE-005: Generate audit trail for execution lifecycle
// - BR-WE-006: Expose Kubernetes Conditions for status tracking
// - BR-WE-008: Emit Prometheus metrics for execution outcomes
// - BR-WE-012: Apply exponential backoff for failed executions
//
// Architecture:
// - Pure Executor: Only executes workflows (routing handled by RemediationOrchestrator)
// - Status Sync: Continuously syncs WFE status with PipelineRun status
// - Failure Analysis: Detects Tekton task failures and reports detailed reasons
//
// Design Decisions:
// - DD-WE-001: Resource locking safety (prevents concurrent execution on same target)
// - DD-WE-002: Dedicated execution namespace (isolates PipelineRuns)
// - DD-WE-003: Deterministic lock names (enables resource lock persistence)
// - DD-WE-004: Exponential backoff for pre-execution failures
//
// See: docs/services/crd-controllers/03-workflowexecution/ for detailed documentation
package workflowexecution

import (
	"context"
	"fmt"
	"strings"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/status"
)

// ========================================
// WorkflowExecution Controller
// ADR-044: Tekton PipelineRun Delegation
// DD-WE-001: Resource Locking Safety
// DD-WE-002: Dedicated Execution Namespace
// DD-WE-003: Lock Persistence (Deterministic Name)
// ========================================

const (
	// FinalizerName is the finalizer for WorkflowExecution cleanup
	// Per finalizers-lifecycle.md: domain/resource-cleanup pattern
	FinalizerName = "workflowexecution.kubernaut.ai/workflowexecution-cleanup"

	// DefaultCooldownPeriod is the default time between workflow executions on same target
	DefaultCooldownPeriod = 5 * time.Minute
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
type WorkflowExecutionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// DD-STATUS-001: APIReader bypasses informer cache for direct API server reads.
	// Used in reconcilePending to prevent race conditions from stale cache data:
	// - Prevents duplicate audit events (cache lag between concurrent reconciles)
	// - Ensures ExecutionRef is fresh for external deletion detection
	APIReader client.Reader

	// ========================================
	// V1.0 MATURITY REQUIREMENTS (SERVICE_MATURITY_REQUIREMENTS.md)
	// ========================================

	// Metrics for observability (DD-005, DD-METRICS-001)
	// Per DD-METRICS-001: Metrics MUST be dependency-injected, not global variables
	// Initialized in main.go and injected via SetupWithManager()
	Metrics *metrics.Metrics

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
	// - 50%+ API call reduction (2 updates → 1 atomic update)
	// - Eliminates race conditions from sequential updates
	// - Reduces etcd write load and watch events
	//
	// WIRED IN: cmd/workflowexecution/main.go
	// USAGE: r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() { ... })
	StatusManager *status.Manager

	// ========================================
	// WORKFLOW EXECUTION CONFIGURATION
	// ========================================

	// ExecutionNamespace is where PipelineRuns are created (DD-WE-002)
	// Default: "kubernaut-workflows"
	ExecutionNamespace string

	// CooldownPeriod prevents redundant sequential workflows (DD-WE-001)
	// Default: 5 minutes
	CooldownPeriod time.Duration

	// AuditStore for writing audit events (BR-WE-005, ADR-032)
	// Uses pkg/audit buffered store via Data Storage Service
	// Optional: nil disables audit (graceful degradation)
	AuditStore audit.AuditStore

	// ========================================
	// REFACTORING PATTERNS (CONTROLLER_REFACTORING_PATTERN_LIBRARY.md)
	// ========================================

	// PhaseManager manages phase state machine logic (P0: Phase State Machine)
	// Per CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §1
	// Provides validated phase transitions and terminal state checking
	PhaseManager *wephase.Manager

	// AuditManager manages audit event emission (P3: Audit Manager)
	// Per CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §7
	// Provides typed audit methods for better testability
	AuditManager *weaudit.Manager

	// ExecutorRegistry dispatches to the correct execution backend (BR-WE-014)
	// Maps execution engine names ("tekton", "job") to Executor implementations.
	// When nil, falls back to inline Tekton-only code path.
	ExecutorRegistry *weexecutor.Registry
}

// ReconcilerOptions groups the business-specific dependencies for the
// WorkflowExecution reconciler. Fields extracted from ctrl.Manager (Client,
// APIReader, Scheme, Recorder) are populated automatically by NewReconciler.
type ReconcilerOptions struct {
	ExecutionNamespace string
	CooldownPeriod     time.Duration
	Metrics            *metrics.Metrics
	StatusManager      *status.Manager
	AuditStore         audit.AuditStore
	PhaseManager       *wephase.Manager
	AuditManager       *weaudit.Manager
	ExecutorRegistry   *weexecutor.Registry
}

// NewReconciler creates a WorkflowExecutionReconciler, extracting
// infrastructure fields (Client, APIReader, Scheme, Recorder) from the manager
// and business fields from opts. This reduces the parameter count at the call
// site from ~13 fields to 2 (mgr + opts).
func NewReconciler(mgr ctrl.Manager, opts ReconcilerOptions) *WorkflowExecutionReconciler {
	return &WorkflowExecutionReconciler{
		Client:             mgr.GetClient(),
		APIReader:          mgr.GetAPIReader(),
		Scheme:             mgr.GetScheme(),
		Recorder:           mgr.GetEventRecorderFor("workflowexecution-controller"),
		Metrics:            opts.Metrics,
		StatusManager:      opts.StatusManager,
		ExecutionNamespace: opts.ExecutionNamespace,
		CooldownPeriod:     opts.CooldownPeriod,
		AuditStore:         opts.AuditStore,
		PhaseManager:       opts.PhaseManager,
		AuditManager:       opts.AuditManager,
		ExecutorRegistry:   opts.ExecutorRegistry,
	}
}

// ========================================
// RBAC Markers
// ========================================

//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=taskruns,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// BR-WORKFLOW-008 (Issue #1481): "watch" is required even though the
// reconciler only calls List() -- the manager's cached client establishes
// an Informer for any type it reads, and that Informer needs list+watch to
// sync, regardless of which verb application code exercises directly.
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch;get;list;watch

// Reconcile handles WorkflowExecution reconciliation
// Phase-based reconciliation per implementation plan
func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the WorkflowExecution instance
	var wfe workflowexecutionv1alpha1.WorkflowExecution
	if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
		// Ignore not-found errors (deleted before reconcile)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling WorkflowExecution",
		"name", wfe.Name,
		"namespace", wfe.Namespace,
		"phase", wfe.Status.Phase,
	)

	// ========================================
	// Handle Deletion
	// ========================================
	if !wfe.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(ctx, &wfe)
	}

	// ========================================
	// DD-CONTROLLER-001 v4.0: Pending-phase ObservedGeneration skip REMOVED.
	// Cooldown (BR-WE-009) uses RequeueAfter during Pending; the skip was
	// blocking those retries and permanently stalling WFEs (#374, #375).
	// GenerationChangedPredicate already filters status-only watch duplicates.
	// ========================================

	// ========================================
	// Add Finalizer (if not present)
	// ========================================
	if !controllerutil.ContainsFinalizer(&wfe, FinalizerName) {
		logger.Info("Adding finalizer", "finalizer", FinalizerName)
		controllerutil.AddFinalizer(&wfe, FinalizerName)
		if err := r.Update(ctx, &wfe); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Requeue after adding finalizer
		return ctrl.Result{Requeue: true}, nil
	}

	// ========================================
	// Phase-Based Reconciliation
	// Per Controller Refactoring Pattern Library:
	// - P1: Terminal State Logic (phase.IsTerminal)
	// - P0: Phase State Machine (phase.ValidTransitions)
	// ========================================

	// Terminal states are processed by ReconcileTerminal for cooldown tracking,
	// then no further reconciliation occurs
	switch wfe.Status.Phase {
	case "", workflowexecutionv1alpha1.PhasePending:
		return r.reconcilePending(ctx, &wfe)
	case workflowexecutionv1alpha1.PhaseRunning:
		return r.reconcileRunning(ctx, &wfe)
	case workflowexecutionv1alpha1.PhaseCompleted, workflowexecutionv1alpha1.PhaseFailed:
		// P1: Terminal State Logic - ReconcileTerminal handles cooldown, then returns
		// This prevents unnecessary reconciliation of terminal resources
		if wephase.IsTerminal(wephase.Phase(wfe.Status.Phase)) {
			return r.ReconcileTerminal(ctx, &wfe)
		}
	// V1.0: PhaseSkipped removed - RO handles routing (DD-RO-002)
	default:
		logger.Error(nil, "Unknown phase", "phase", wfe.Status.Phase)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Should never reach here
	return ctrl.Result{}, nil
}

// ========================================
// SetupWithManager sets up the controller with the Manager
// ========================================
func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := registerTargetResourceIndex(mgr); err != nil {
		return err
	}

	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&workflowexecutionv1alpha1.WorkflowExecution{}).
		// WE-BUG-001: Prevent duplicate reconciles from status-only updates
		// Use GenerationChangedPredicate to only reconcile on spec changes
		// Status updates (PipelineRunStatus) are informational and don't require reconciliation
		// Rationale: Controller only needs to act on spec changes, not status updates
		WithEventFilter(predicate.GenerationChangedPredicate{})

	// BR-WE-014: Only watch PipelineRuns if Tekton CRDs are installed.
	// This is a runtime optimization - if Tekton is not installed, the controller
	// still works for "job" engine workflows via polling (RequeueAfter).
	// If a workflow uses executionEngine: "tekton", the operator must ensure Tekton is installed.
	_, tektonDiscoveryErr := mgr.GetRESTMapper().RESTMapping(
		schema.GroupKind{Group: "tekton.dev", Kind: "PipelineRun"}, "v1",
	)
	if tektonDiscoveryErr == nil {
		// Watch PipelineRuns in execution namespace (cross-namespace via label)
		// Only watch PipelineRuns with our label to avoid unnecessary reconciles
		// Watch for status updates (not just metadata changes)
		ctrlBuilder = ctrlBuilder.Watches(
			&tektonv1.PipelineRun{},
			handler.EnqueueRequestsFromMapFunc(r.FindWFEForOwnedResource),
			builder.WithPredicates(workflowExecutionLabelPredicate()),
		)
	}

	// BR-WE-014: Watch Jobs for immediate completion detection.
	// Without this, the Job engine relies on RequeueAfter polling (10s),
	// causing slow completion detection and flaky integration tests.
	// Jobs use the same labeling convention as PipelineRuns, so the
	// same mapper function (FindWFEForOwnedResource) works for both.
	ctrlBuilder = ctrlBuilder.Watches(
		&batchv1.Job{},
		handler.EnqueueRequestsFromMapFunc(r.FindWFEForOwnedResource),
		builder.WithPredicates(jobLabelPredicate()),
	)

	return ctrlBuilder.Complete(r)
}

// registerTargetResourceIndex creates the index on spec.targetResource used
// for O(1) lock checks (DD-WE-003). Extracted from SetupWithManager (Wave 6
// 6e-ii GREEN: funlen remediation) — pure code motion, no behavior change.
func registerTargetResourceIndex(mgr ctrl.Manager) error {
	// NOTE: This index may already exist if RO controller was set up first.
	// Both controllers need this index for routing/locking, so if it exists, we're good.
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&workflowexecutionv1alpha1.WorkflowExecution{},
		"spec.targetResource",
		func(obj client.Object) []string {
			wfe := obj.(*workflowexecutionv1alpha1.WorkflowExecution)
			return []string{wfe.Spec.TargetResource}
		},
	); err != nil {
		// Ignore "indexer conflict" error - if RO controller created this index first, we're good
		// Both controllers need this index anyway (WE for locking, RO for routing)
		if !k8serrors.IsIndexerConflict(err) {
			return fmt.Errorf("failed to create field index on spec.targetResource: %w", err)
		}
		// Index already exists - safe to continue
	}
	return nil
}

// hasWorkflowExecutionLabel reports whether the given labels carry the
// kubernaut.ai/workflow-execution marker used by owned PipelineRuns/Jobs.
func hasWorkflowExecutionLabel(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	_, hasLabel := labels["kubernaut.ai/workflow-execution"]
	return hasLabel
}

// workflowExecutionLabelPredicate builds the predicate used to filter
// PipelineRun watch events down to those owned by a WorkflowExecution.
// Extracted from SetupWithManager (Wave 6 6e-ii GREEN: funlen remediation)
// — pure code motion, no behavior change.
func workflowExecutionLabelPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasWorkflowExecutionLabel(e.Object.GetLabels())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Watch for status updates on labeled PipelineRuns
			return hasWorkflowExecutionLabel(e.ObjectNew.GetLabels())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasWorkflowExecutionLabel(e.Object.GetLabels())
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return hasWorkflowExecutionLabel(e.Object.GetLabels())
		},
	}
}

// jobLabelPredicate builds the predicate used to filter Job watch events
// down to those owned by a WorkflowExecution. Extracted from
// SetupWithManager (Wave 6 6e-ii GREEN: funlen remediation) — pure code
// motion, no behavior change.
func jobLabelPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return hasWorkflowExecutionLabel(e.Object.GetLabels())
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return hasWorkflowExecutionLabel(e.ObjectNew.GetLabels())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return hasWorkflowExecutionLabel(e.Object.GetLabels())
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return hasWorkflowExecutionLabel(e.Object.GetLabels())
		},
	}
}

// ========================================
// FindWFEForOwnedResource maps owned resource events (PipelineRun, Job) to WorkflowExecution reconcile requests.
// Both executors label their resources with kubernaut.ai/workflow-execution and kubernaut.ai/source-namespace.
// ========================================
func (r *WorkflowExecutionReconciler) FindWFEForOwnedResource(ctx context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()
	if labels == nil {
		return nil
	}

	wfeName := labels["kubernaut.ai/workflow-execution"]
	sourceNS := labels["kubernaut.ai/source-namespace"]

	if wfeName == "" || sourceNS == "" {
		return nil
	}

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      wfeName,
			Namespace: sourceNS,
		},
	}}
}

// ========================================
// Day 8: Spec Validation
// Per controller-implementation.md
// ========================================

// ValidateSpec validates the WorkflowExecution spec
// Returns error if validation fails (ConfigurationError reason)
func (r *WorkflowExecutionReconciler) ValidateSpec(wfe *workflowexecutionv1alpha1.WorkflowExecution) error {
	// Validate container image is required
	if wfe.Spec.WorkflowRef.ExecutionBundle == "" {
		return fmt.Errorf("workflowRef.executionBundle is required")
	}

	// Validate target resource is required
	if wfe.Spec.TargetResource == "" {
		return fmt.Errorf("targetResource is required")
	}

	// Validate targetResource format per DD-WE-001:
	// - Namespaced resources: {namespace}/{kind}/{name} (3 parts)
	// - Cluster-scoped resources: {kind}/{name} (2 parts)
	// Examples:
	//   - "payment/deployment/payment-api" (namespaced)
	//   - "node/worker-node-1" (cluster-scoped)
	//   - "kube-system/configmap/coredns" (namespaced)
	parts := strings.Split(wfe.Spec.TargetResource, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("targetResource must be in format {namespace}/{kind}/{name} (namespaced) or {kind}/{name} (cluster-scoped), got %d parts", len(parts))
	}

	// Validate each part is non-empty
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("targetResource has empty part at position %d", i)
		}
	}

	return nil
}

// ========================================
// DD-EVENT-001 v1.1: K8s Event Emission Helpers
// BR-WE-095: K8s Event Observability for WorkflowExecution Controller
// ========================================

// emitPhaseTransition emits a PhaseTransition breadcrumb event for WFE phase changes.
func (r *WorkflowExecutionReconciler) emitPhaseTransition(wfe *workflowexecutionv1alpha1.WorkflowExecution, from, to string) {
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonPhaseTransition,
		fmt.Sprintf("Phase transition: %s → %s", from, to))
}

// engineGuidance returns a human-readable remediation hint for a missing engine (Issue #868).
func engineGuidance(engine string) string {
	switch engine {
	case workflowexecutionv1alpha1.ExecutionEngineTekton:
		return `install Tekton Pipelines CRDs or use executionEngine: "job"`
	case "ansible":
		return "configure ansible section in workflowexecution config"
	default:
		return "check controller configuration and logs"
	}
}

// mapExecutorReasonToCRDEnum maps engine-specific failure reasons (e.g., AWXJobFailed)
// to the CRD-validated FailureReason enum values.
func mapExecutorReasonToCRDEnum(reason string) string {
	switch reason {
	case "AWXJobFailed", "AWXJobError":
		return workflowexecutionv1alpha1.FailureReasonTaskFailed
	case "AWXJobCanceled":
		return workflowexecutionv1alpha1.FailureReasonTaskFailed
	case "JobFailed":
		return workflowexecutionv1alpha1.FailureReasonTaskFailed
	case "DeadlineExceeded":
		// BR-WORKFLOW-008: a Job's ActiveDeadlineSeconds elapsing (e.g. because
		// a Pod could never mount a missing Secret/ConfigMap dependency, #1481)
		// surfaces here as the Job condition reason.
		return workflowexecutionv1alpha1.FailureReasonDeadlineExceeded
	default:
		return workflowexecutionv1alpha1.FailureReasonUnknown
	}
}
