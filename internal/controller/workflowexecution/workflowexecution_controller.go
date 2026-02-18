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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"knative.dev/pkg/apis"
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
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
	"github.com/jordigilh/kubernaut/pkg/shared/k8serrors"
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

	// DefaultServiceAccountName is the default SA for PipelineRuns
	DefaultServiceAccountName = "kubernaut-workflow-runner"
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

	// ServiceAccountName for PipelineRuns
	// Default: "kubernaut-workflow-runner"
	ServiceAccountName string

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

// ========================================
// RBAC Markers
// ========================================

//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
//+kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=tekton.dev,resources=taskruns,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

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
	// OBSERVED GENERATION CHECK (DD-CONTROLLER-001)
	// ========================================
	// WFE must reconcile on PipelineRun status changes (external watch).
	// Only skip reconcile for annotation/label changes when:
	// 1. Generation unchanged
	// 2. Phase is Pending (not yet watching PipelineRun)
	//
	// IMPORTANT: Terminal phases (Completed/Failed) MUST continue reconciling
	// until cooldown expires and lock is released (ReconcileTerminal handles this).
	// Skipping terminal phases prevents cooldown processing and lock release.
	//
	// This allows reconciles for:
	// - PipelineRun status updates (Running phase)
	// - Cooldown processing (Completed/Failed phases)
	// - Condition updates
	// - Metrics recording
	if wfe.Status.ObservedGeneration == wfe.Generation &&
		wfe.Status.Phase == workflowexecutionv1alpha1.PhasePending {
		// Safe to skip: Pending phase not yet watching PipelineRun
		logger.V(1).Info("✅ DUPLICATE RECONCILE PREVENTED: Generation already processed (Pending phase)",
			"generation", wfe.Generation,
			"observedGeneration", wfe.Status.ObservedGeneration,
			"phase", wfe.Status.Phase)
		return ctrl.Result{}, nil
	}

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

	// BR-WE-014: Default to "tekton" if executionEngine is unset (backward compatibility)
	if wfe.Spec.ExecutionEngine == "" {
		wfe.Spec.ExecutionEngine = "tekton"
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
// reconcilePending - Handle Pending phase
// V1.0: Pure execution logic, RO handles all routing (DD-RO-002)
// ========================================
func (r *WorkflowExecutionReconciler) reconcilePending(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Pending phase")

	// ========================================
	// DD-STATUS-001: Re-read WFE from API server to bypass informer cache.
	// Prevents race conditions where concurrent reconciles read stale data:
	// - F1: ExecutionRef not yet cached → misses external deletion detection
	// - F2: Phase not yet cached → re-enters reconcilePending → duplicate audit events
	// ========================================
	freshWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
	if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(wfe), freshWFE); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // WFE was deleted
		}
		return ctrl.Result{}, fmt.Errorf("failed to re-read WFE from API server: %w", err)
	}
	// If the fresh WFE has already progressed past Pending, requeue to let the
	// main Reconcile re-route based on the updated phase.
	if freshWFE.Status.Phase != "" && freshWFE.Status.Phase != workflowexecutionv1alpha1.PhasePending {
		logger.Info("WFE already progressed past Pending (informer cache was stale), requeueing",
			"freshPhase", freshWFE.Status.Phase)
		return ctrl.Result{Requeue: true}, nil
	}
	// Use fresh data for the remainder of this reconcile
	*wfe = *freshWFE

	// V1.0: No routing logic - RO makes ALL routing decisions before creating WFE
	// If WFE exists, execute it. RO already checked routing.

	// ========================================
	// Step 1: Validate spec (prevent malformed PipelineRuns)
	// ========================================
	if err := r.ValidateSpec(wfe); err != nil {
		logger.Error(err, "Spec validation failed")
		// DD-EVENT-001 v1.1: Emit WorkflowValidationFailed event (P2: Decision Point)
		r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowValidationFailed,
			fmt.Sprintf("Spec validation failed: %v", err))
		// Mark as Failed with ConfigurationError reason
		// This is a pre-execution failure (wasExecutionFailure: false)
		if markErr := r.MarkFailedWithReason(ctx, wfe, "ConfigurationError", err.Error()); markErr != nil {
			return ctrl.Result{}, markErr
		}
		return ctrl.Result{}, nil
	}

	// DD-EVENT-001 v1.1: Emit WorkflowValidated event (P2: Decision Point)
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonWorkflowValidated,
		fmt.Sprintf("Workflow spec validated: %s (target: %s)", wfe.Spec.WorkflowRef.WorkflowID, wfe.Spec.TargetResource))

	// ========================================
	// Step 1.5: Check if cooldown is active for target resource (BR-WE-009)
	// BUGFIX: Was only tracked in terminal phase, not enforced during pending
	// ========================================
	currentWFEKey := fmt.Sprintf("%s/%s", wfe.Namespace, wfe.Name)
	if remaining, active := r.CheckCooldownActive(ctx, wfe.Spec.TargetResource, currentWFEKey); active {
		logger.Info("Blocking execution due to active cooldown",
			"targetResource", wfe.Spec.TargetResource,
			"remaining", remaining,
		)
		// Ensure phase is set to Pending if not already set (P0: Phase State Machine)
		if wfe.Status.Phase == "" || wfe.Status.Phase != workflowexecutionv1alpha1.PhasePending {
			if err := r.PhaseManager.TransitionTo(wfe, wephase.Pending); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to transition to Pending during cooldown: %w", err)
			}
			if err := r.Status().Update(ctx, wfe); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update phase to Pending during cooldown: %w", err)
			}
		}
		// DD-EVENT-001 v1.1: Emit CooldownActive event (P2: Decision Point)
		r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonCooldownActive,
			fmt.Sprintf("Execution deferred: cooldown active for %s (remaining: %s)", wfe.Spec.TargetResource, remaining.Round(time.Second)))

		// Stay in Pending, requeue after cooldown expires
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// ========================================
	// Gap #5: Record workflow selection audit event (BR-AUDIT-005)
	// Emitted AFTER validation, BEFORE execution resource creation
	// IDEMPOTENCY: Only emit once - skip if execution resource already exists
	// ========================================
	// DD-STATUS-001: Use APIReader to bypass informer cache for existence check.
	// Prevents duplicate audit events when concurrent reconciles don't yet see a recently-created resource in cache.
	resourceName := weexecutor.ExecutionResourceName(wfe.Spec.TargetResource)
	resourceExists := false
	var resourceGetErr error
	switch wfe.Spec.ExecutionEngine {
	case "job":
		existingJob := &batchv1.Job{}
		resourceGetErr = r.APIReader.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: r.ExecutionNamespace}, existingJob)
	default: // "tekton"
		existingPR := &tektonv1.PipelineRun{}
		resourceGetErr = r.APIReader.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: r.ExecutionNamespace}, existingPR)
	}
	if resourceGetErr == nil {
		resourceExists = true
	} else if apierrors.IsNotFound(resourceGetErr) && wfe.Status.ExecutionRef != nil {
		// INT-EXTERN-02: Execution resource was deleted externally after we created it
		logger.Error(resourceGetErr, "Execution resource not found - deleted externally during Pending phase",
			"engine", wfe.Spec.ExecutionEngine)
		return r.MarkFailed(ctx, wfe, nil)
	}

	if !resourceExists {
		if err := r.AuditManager.RecordWorkflowSelectionCompleted(ctx, wfe); err != nil {
			logger.V(1).Info("Failed to record workflow.selection.completed audit event", "error", err)
		}
	} else {
		logger.V(2).Info("Skipping workflow.selection.completed audit event - execution resource already exists",
			"resource", resourceName, "engine", wfe.Spec.ExecutionEngine)
	}

	// ========================================
	// Step 2: Create execution resource via executor dispatch (BR-WE-014)
	// ========================================
	exec, err := r.ExecutorRegistry.Get(wfe.Spec.ExecutionEngine)
	if err != nil {
		logger.Error(err, "Unsupported execution engine", "engine", wfe.Spec.ExecutionEngine)
		markErr := r.MarkFailedWithReason(ctx, wfe, "UnsupportedEngine", err.Error())
		return ctrl.Result{}, markErr
	}

	logger.Info("Creating execution resource",
		"engine", wfe.Spec.ExecutionEngine,
		"resource", resourceName,
		"namespace", r.ExecutionNamespace,
	)

	createdName, createErr := exec.Create(ctx, wfe, r.ExecutionNamespace)
	if createErr != nil {
		if apierrors.IsAlreadyExists(createErr) {
			// DD-WE-003 Layer 2: Execution-time collision handling
			if wfe.Spec.ExecutionEngine == "tekton" {
				pr := r.BuildPipelineRun(wfe)
				return r.HandleAlreadyExists(ctx, wfe, pr, createErr)
			}
			// For Job backend, AlreadyExists means deterministic name collision
			// Note: Uses "Unknown" reason (CRD enum constraint); specific cause in message
			markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown",
				fmt.Sprintf("Execution resource %s already exists (target resource locked)", resourceName))
			return ctrl.Result{}, markErr
		}
		logger.Error(createErr, "Failed to create execution resource", "engine", wfe.Spec.ExecutionEngine)
		markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown",
			fmt.Sprintf("Failed to create %s execution resource: %v", wfe.Spec.ExecutionEngine, createErr))
		return ctrl.Result{}, markErr
	}

	// Record execution creation metric (BR-WE-008)
	if r.Metrics != nil {
		r.Metrics.RecordExecutionCreation()
	}

	// ========================================
	// Gap #6: Record execution workflow started audit event (BR-AUDIT-005)
	// ========================================
	if err := r.AuditManager.RecordExecutionWorkflowStarted(ctx, wfe, createdName, r.ExecutionNamespace); err != nil {
		logger.V(1).Info("Failed to record execution.workflow.started audit event", "error", err)
		weconditions.SetAuditRecorded(wfe, false,
			weconditions.ReasonAuditFailed,
			fmt.Sprintf("Failed to record audit event: %v", err))
	} else {
		weconditions.SetAuditRecorded(wfe, true,
			weconditions.ReasonAuditSucceeded,
			"Audit event execution.workflow.started recorded to DataStorage")
	}

	// ========================================
	// BR-WE-006: Set ExecutionCreated condition
	// ========================================
	weconditions.SetExecutionCreated(wfe, true,
		weconditions.ReasonExecutionCreated,
		fmt.Sprintf("%s execution resource %s created in %s namespace",
			wfe.Spec.ExecutionEngine, createdName, r.ExecutionNamespace))

	// ========================================
	// Step 3: Prepare status update to Running (P0: Phase State Machine)
	// ========================================
	now := metav1.Now()
	if err := r.PhaseManager.TransitionTo(wfe, wephase.Running); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to transition to Running: %w", err)
	}
	wfe.Status.StartTime = &now
	wfe.Status.ExecutionRef = &corev1.LocalObjectReference{
		Name: createdName,
	}

	// Single atomic status update with all changes
	if err := r.updateStatus(ctx, wfe, "Running with conditions"); err != nil {
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonExecutionCreated,
		fmt.Sprintf("Created %s execution resource %s/%s", wfe.Spec.ExecutionEngine, r.ExecutionNamespace, createdName))

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for Pending → Running
	r.emitPhaseTransition(wfe, "Pending", "Running")

	// Requeue to check execution status
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// ========================================
// reconcileRunning - Handle Running phase
// Day 5: Status synchronization
// ========================================
func (r *WorkflowExecutionReconciler) reconcileRunning(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Running phase", "engine", wfe.Spec.ExecutionEngine)

	// ========================================
	// Step 1: Get execution status via executor dispatch (BR-WE-014)
	// ========================================
	exec, err := r.ExecutorRegistry.Get(wfe.Spec.ExecutionEngine)
	if err != nil {
		logger.Error(err, "Unsupported execution engine during Running phase")
		return r.MarkFailed(ctx, wfe, nil)
	}

	result, getErr := exec.GetStatus(ctx, wfe, r.ExecutionNamespace)
	if getErr != nil {
		if apierrors.IsNotFound(getErr) {
			logger.Error(getErr, "Execution resource not found - deleted externally",
				"engine", wfe.Spec.ExecutionEngine)
			return r.MarkFailed(ctx, wfe, nil)
		}
		return ctrl.Result{}, getErr
	}

	// ========================================
	// Step 2: Update ExecutionStatusSummary for visibility
	// ========================================
	if result.Summary != nil {
		wfe.Status.ExecutionStatus = result.Summary
	}

	// ========================================
	// Step 3: Map execution result to WFE phase
	// ========================================
	switch result.Phase {
	case workflowexecutionv1alpha1.PhaseCompleted:
		logger.Info("Execution succeeded", "engine", wfe.Spec.ExecutionEngine)
		if wfe.Spec.ExecutionEngine == "tekton" {
			// Tekton path: MarkCompleted needs the PipelineRun for detailed failure extraction
			var pr tektonv1.PipelineRun
			if err := r.Get(ctx, client.ObjectKey{
				Name:      wfe.Status.ExecutionRef.Name,
				Namespace: r.ExecutionNamespace,
			}, &pr); err != nil {
				return r.MarkCompleted(ctx, wfe, nil)
			}
			return r.MarkCompleted(ctx, wfe, &pr)
		}
		return r.MarkCompleted(ctx, wfe, nil)

	case workflowexecutionv1alpha1.PhaseFailed:
		logger.Info("Execution failed", "engine", wfe.Spec.ExecutionEngine, "reason", result.Reason)
		if wfe.Spec.ExecutionEngine == "tekton" {
			var pr tektonv1.PipelineRun
			if err := r.Get(ctx, client.ObjectKey{
				Name:      wfe.Status.ExecutionRef.Name,
				Namespace: r.ExecutionNamespace,
			}, &pr); err != nil {
				return r.MarkFailed(ctx, wfe, nil)
			}
			return r.MarkFailed(ctx, wfe, &pr)
		}
		return r.MarkFailed(ctx, wfe, nil)

	default:
		// Still running - update conditions and requeue
		logger.V(1).Info("Execution still running", "reason", result.Reason, "engine", wfe.Spec.ExecutionEngine)
		weconditions.SetExecutionRunning(wfe, true,
			weconditions.ReasonExecutionStarted,
			fmt.Sprintf("Execution running (%s: %s)", wfe.Spec.ExecutionEngine, result.Reason))
	}

	// ========================================
	// Step 4: Update status with current progress
	// ========================================
	if err := r.updateStatus(ctx, wfe, "current progress"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// ========================================
// ReconcileTerminal - Handle Completed/Failed phases
// Day 6: Cooldown enforcement and cleanup
// DD-WE-003: Lock Persistence (Deterministic Name)
// ========================================
func (r *WorkflowExecutionReconciler) ReconcileTerminal(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Terminal phase", "phase", wfe.Status.Phase)

	// Guard clause: no completion time means we can't calculate cooldown
	if wfe.Status.CompletionTime == nil {
		logger.V(1).Info("No completion time set, skipping cooldown check")
		return ctrl.Result{}, nil
	}

	// Get cooldown period (use default if not set)
	cooldown := r.CooldownPeriod
	if cooldown == 0 {
		cooldown = DefaultCooldownPeriod
	}

	// Calculate elapsed time since completion
	elapsed := time.Since(wfe.Status.CompletionTime.Time)

	// Wait for cooldown before releasing lock
	if elapsed < cooldown {
		remaining := cooldown - elapsed
		logger.V(1).Info("Waiting for cooldown",
			"remaining", remaining,
			"targetResource", wfe.Spec.TargetResource,
		)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Cooldown expired - delete execution resource to release lock
	// DD-WE-003: Use deterministic name for atomic locking
	if r.ExecutorRegistry != nil {
		exec, execErr := r.ExecutorRegistry.Get(wfe.Spec.ExecutionEngine)
		if execErr != nil {
			exec, _ = r.ExecutorRegistry.Get("tekton") // Fallback
		}
		if exec != nil {
			if err := exec.Cleanup(ctx, wfe, r.ExecutionNamespace); err != nil {
				logger.Error(err, "Failed to cleanup execution resource after cooldown",
					"engine", exec.Engine())
				// DD-EVENT-001 v1.1: Emit CleanupFailed event (P4: Error Path)
				r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonCleanupFailed,
					fmt.Sprintf("Cleanup failed after cooldown: %v", err))
				return ctrl.Result{}, err
			}
			logger.Info("Lock released after cooldown",
				"targetResource", wfe.Spec.TargetResource,
				"engine", exec.Engine(),
				"cooldownPeriod", cooldown,
			)
		}
	} else {
		// Fallback: inline Tekton cleanup when ExecutorRegistry is not configured
		prName := PipelineRunName(wfe.Spec.TargetResource)
		pr := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prName,
				Namespace: r.ExecutionNamespace,
			},
		}
		if err := r.Delete(ctx, pr); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to delete PipelineRun after cooldown")
				return ctrl.Result{}, err
			}
		}
		logger.Info("Lock released after cooldown",
			"targetResource", wfe.Spec.TargetResource,
			"cooldownPeriod", cooldown,
		)
	}

	// Emit LockReleased event
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonLockReleased,
		fmt.Sprintf("Lock released for %s after cooldown", wfe.Spec.TargetResource))

	return ctrl.Result{}, nil
}

// ========================================
// CheckCooldownActive checks if cooldown is active for a target resource
// BR-WE-009: Cooldown Period is Configurable
// Returns (remaining duration, is active)
// currentWFEName format: "namespace/name" to uniquely identify the current WFE
// ========================================
func (r *WorkflowExecutionReconciler) CheckCooldownActive(ctx context.Context, targetResource, currentWFEKey string) (time.Duration, bool) {
	logger := log.FromContext(ctx)

	// Get cooldown period (use default if not set)
	cooldown := r.CooldownPeriod
	if cooldown == 0 {
		cooldown = DefaultCooldownPeriod
	}

	// Query all WorkflowExecutions with the same targetResource
	wfeList := &workflowexecutionv1alpha1.WorkflowExecutionList{}
	if err := r.List(ctx, wfeList, client.MatchingFields{"spec.targetResource": targetResource}); err != nil {
		logger.Error(err, "Failed to list WorkflowExecutions for cooldown check",
			"targetResource", targetResource)
		// On error, don't block execution (fail open)
		return 0, false
	}

	// Find any completed/failed WFE still within cooldown period
	now := time.Now()
	for i := range wfeList.Items {
		otherWFE := &wfeList.Items[i]

		// Skip the current WFE (don't check cooldown against ourselves)
		otherKey := fmt.Sprintf("%s/%s", otherWFE.Namespace, otherWFE.Name)
		if otherKey == currentWFEKey {
			continue
		}

		// Only check terminal phases (Completed or Failed)
		if otherWFE.Status.Phase != workflowexecutionv1alpha1.PhaseCompleted &&
			otherWFE.Status.Phase != workflowexecutionv1alpha1.PhaseFailed {
			continue
		}

		// Check if completion time is set
		if otherWFE.Status.CompletionTime == nil {
			continue
		}

		// Calculate elapsed time since completion
		elapsed := now.Sub(otherWFE.Status.CompletionTime.Time)

		// Check if still within cooldown period
		if elapsed < cooldown {
			remaining := cooldown - elapsed
			logger.V(1).Info("Cooldown active for target resource",
				"targetResource", targetResource,
				"blockingWFE", otherKey,
				"remaining", remaining,
			)
			return remaining, true
		}
	}

	// No active cooldown found
	return 0, false
}

// ========================================
// ReconcileDelete - Handle deletion with finalizer
// DD-WE-003: Use deterministic PipelineRun name
// finalizers-lifecycle.md: Event emission
// ========================================
func (r *WorkflowExecutionReconciler) ReconcileDelete(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Delete")

	// Check if finalizer is present
	if !controllerutil.ContainsFinalizer(wfe, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// ========================================
	// Cleanup: Delete execution resource via executor dispatch (BR-WE-014, DD-WE-003)
	// This ensures cleanup even if ExecutionRef was never set
	// ========================================
	if r.ExecutorRegistry != nil {
		exec, execErr := r.ExecutorRegistry.Get(wfe.Spec.ExecutionEngine)
		if execErr != nil {
			logger.Error(execErr, "Unknown engine during cleanup, falling back to Tekton cleanup")
			exec, _ = r.ExecutorRegistry.Get("tekton")
		}
		if exec != nil {
			logger.Info("Cleaning up execution resource",
				"engine", exec.Engine(),
				"namespace", r.ExecutionNamespace,
			)
			if err := exec.Cleanup(ctx, wfe, r.ExecutionNamespace); err != nil {
				logger.Error(err, "Failed to cleanup execution resource during finalization",
					"engine", exec.Engine())
				// DD-EVENT-001 v1.1: Emit CleanupFailed event (P4: Error Path)
				r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonCleanupFailed,
					fmt.Sprintf("Cleanup failed during finalization: %v", err))
				return ctrl.Result{}, err
			}
			logger.Info("Finalizer: cleaned up execution resource", "engine", exec.Engine())
		}
	} else {
		// Fallback: inline Tekton cleanup when ExecutorRegistry is not configured
		prName := PipelineRunName(wfe.Spec.TargetResource)
		pr := &tektonv1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prName,
				Namespace: r.ExecutionNamespace,
			},
		}
		if err := r.Delete(ctx, pr); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to delete PipelineRun during finalization")
				return ctrl.Result{}, err
			}
		} else {
			logger.Info("Finalizer: deleted associated PipelineRun", "pipelineRun", prName)
		}
	}

	// ========================================
	// Emit deletion event (finalizers-lifecycle.md)
	// ========================================
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonWorkflowExecutionDeleted,
		fmt.Sprintf("WorkflowExecution cleanup completed (phase: %s)", wfe.Status.Phase))

	// ========================================
	// Remove Finalizer
	// ========================================
	logger.Info("Removing finalizer", "finalizer", FinalizerName)
	controllerutil.RemoveFinalizer(wfe, FinalizerName)
	if err := r.Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("WorkflowExecution cleanup complete")
	return ctrl.Result{}, nil
}

// ========================================
// SetupWithManager sets up the controller with the Manager
// ========================================
func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create index on targetResource for O(1) lock check (DD-WE-003)
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
		ctrlBuilder = ctrlBuilder.
			// Watch PipelineRuns in execution namespace (cross-namespace via label)
			// Only watch PipelineRuns with our label to avoid unnecessary reconciles
			// Watch for status updates (not just metadata changes)
			Watches(
				&tektonv1.PipelineRun{},
				handler.EnqueueRequestsFromMapFunc(r.FindWFEForPipelineRun),
				builder.WithPredicates(predicate.Funcs{
					CreateFunc: func(e event.CreateEvent) bool {
						labels := e.Object.GetLabels()
						if labels == nil {
							return false
						}
						_, hasLabel := labels["kubernaut.ai/workflow-execution"]
						return hasLabel
					},
					UpdateFunc: func(e event.UpdateEvent) bool {
						// Watch for status updates on labeled PipelineRuns
						labels := e.ObjectNew.GetLabels()
						if labels == nil {
							return false
						}
						_, hasLabel := labels["kubernaut.ai/workflow-execution"]
						return hasLabel
					},
					DeleteFunc: func(e event.DeleteEvent) bool {
						labels := e.Object.GetLabels()
						if labels == nil {
							return false
						}
						_, hasLabel := labels["kubernaut.ai/workflow-execution"]
						return hasLabel
					},
					GenericFunc: func(e event.GenericEvent) bool {
						labels := e.Object.GetLabels()
						if labels == nil {
							return false
						}
						_, hasLabel := labels["kubernaut.ai/workflow-execution"]
						return hasLabel
					},
				}),
			)
	}

	return ctrlBuilder.Complete(r)
}

// ========================================
// PipelineRunName generates deterministic name from targetResource
// DD-WE-003: Lock Persistence via Deterministic Name
// Format: wfe-<sha256(targetResource)[:16]>
// ========================================
func PipelineRunName(targetResource string) string {
	h := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}

// sanitizeLabelValue makes a string safe for use as a Kubernetes label value
// Label values must consist of alphanumeric characters, '-', '_' or '.'
// and must start and end with an alphanumeric character
func sanitizeLabelValue(s string) string {
	// Replace forward slashes with double underscores
	result := strings.ReplaceAll(s, "/", "__")
	// Truncate to max 63 characters (K8s label value limit)
	if len(result) > 63 {
		result = result[:63]
	}
	return result
}

// ========================================
// HandleAlreadyExists handles the race condition where PipelineRun already exists
// DD-WE-003: Layer 2 - Execution-time collision handling (not routing)
// V1.0: Fails WFE if race condition detected (RO should have prevented this)
// ========================================
func (r *WorkflowExecutionReconciler) HandleAlreadyExists(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// PipelineRun already exists - check if it's ours
	prName := pr.Name
	existingPR := &tektonv1.PipelineRun{}
	if getErr := r.Get(ctx, client.ObjectKey{
		Name:      prName,
		Namespace: r.ExecutionNamespace,
	}, existingPR); getErr != nil {
		logger.Error(getErr, "Failed to get existing PipelineRun", "name", prName)
		markErr := r.MarkFailedWithReason(ctx, wfe, "RaceConditionError", fmt.Sprintf("PipelineRun already exists but failed to verify ownership: %v", getErr))
		return ctrl.Result{}, markErr
	}

	// Check if the existing PipelineRun was created by this WFE
	if existingPR.Labels != nil &&
		existingPR.Labels["kubernaut.ai/workflow-execution"] == wfe.Name &&
		existingPR.Labels["kubernaut.ai/source-namespace"] == wfe.Namespace {
		// It's ours - we must have lost a race with ourselves (unlikely but safe)
		// Continue with normal flow
		logger.Info("PipelineRun already exists and is ours, continuing", "name", prName)

		// ========================================
		// P1: ATOMIC STATUS UPDATE with retry logic
		// Consolidates phase transition + conditions into single API call
		// ========================================
		now := metav1.Now()
		if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
			// Set phase (P0: Phase State Machine)
			if err := r.PhaseManager.TransitionTo(wfe, wephase.Running); err != nil {
				return fmt.Errorf("failed to transition to Running in HandleAlreadyExists: %w", err)
			}

			// Set start time and PipelineRun reference
			wfe.Status.StartTime = &now
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{
				Name: pr.Name,
			}

			// Set ExecutionCreated condition (consistency with main flow)
			weconditions.SetExecutionCreated(wfe, true,
				weconditions.ReasonExecutionCreated,
				fmt.Sprintf("PipelineRun %s already exists (race condition)", prName))

			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status in HandleAlreadyExists: %w", err)
		}

		r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonPipelineRunCreated,
			fmt.Sprintf("PipelineRun %s/%s (already exists, ours)", pr.Namespace, pr.Name))

		// Requeue to check PipelineRun status
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// V1.0: Another WFE created this PipelineRun - execution-time race condition
	// This should be rare (RO handles routing), but handle gracefully
	logger.Error(err, "Race condition at execution time: PipelineRun created by another WFE",
		"prName", prName,
		"existingWFE", existingPR.Labels["kubernaut.ai/workflow-execution"],
		"targetResource", wfe.Spec.TargetResource,
	)

	// Note: Using "Unknown" reason as "ExecutionRaceCondition" is not in CRD enum
	markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown",
		fmt.Sprintf("Race condition: PipelineRun '%s' already exists for target resource (created by %s). This indicates RO routing may have failed.",
			prName, existingPR.Labels["kubernaut.ai/workflow-execution"]))
	return ctrl.Result{}, markErr
}

// ========================================
// BuildPipelineRun creates a PipelineRun with bundle resolver
// DD-WE-002: PipelineRuns created in dedicated execution namespace
// DD-WE-003: Deterministic name for atomic locking
// ========================================
func (r *WorkflowExecutionReconciler) BuildPipelineRun(wfe *workflowexecutionv1alpha1.WorkflowExecution) *tektonv1.PipelineRun {
	// Convert parameters to Tekton format
	params := r.ConvertParameters(wfe.Spec.Parameters)

	// Add TARGET_RESOURCE parameter (required by all pipelines)
	params = append(params, tektonv1.Param{
		Name:  "TARGET_RESOURCE",
		Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.TargetResource},
	})

	// Get service account name (use default if not set)
	saName := r.ServiceAccountName
	if saName == "" {
		saName = DefaultServiceAccountName
	}

	return &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			// CRITICAL: Deterministic name = atomic lock (DD-WE-003)
			Name:      PipelineRunName(wfe.Spec.TargetResource),
			Namespace: r.ExecutionNamespace, // Always "kubernaut-workflows" (DD-WE-002)
			Labels: map[string]string{
				"kubernaut.ai/workflow-execution": wfe.Name,
				"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
				// Sanitize for label (slashes not allowed, replace with __)
				"kubernaut.ai/target-resource": sanitizeLabelValue(wfe.Spec.TargetResource),
				// Source tracking for cross-namespace lookup
				"kubernaut.ai/source-namespace": wfe.Namespace,
			},
			Annotations: map[string]string{
				// Store original target resource value (with slashes) in annotation
				"kubernaut.ai/target-resource": wfe.Spec.TargetResource,
			},
			// NOTE: No OwnerReference - cross-namespace not supported
			// Cleanup handled via finalizer in ReconcileDelete()
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: "bundles",
					Params: []tektonv1.Param{
						{Name: "bundle", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.WorkflowRef.ExecutionBundle}},
						{Name: "name", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "workflow"}},
						{Name: "kind", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "pipeline"}},
					},
				},
			},
			Params: params,
			TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
				ServiceAccountName: saName,
			},
		},
	}
}

// ========================================
// ConvertParameters converts map[string]string to Tekton params
// ========================================
func (r *WorkflowExecutionReconciler) ConvertParameters(params map[string]string) []tektonv1.Param {
	if len(params) == 0 {
		return []tektonv1.Param{}
	}

	tektonParams := make([]tektonv1.Param, 0, len(params))
	for key, value := range params {
		tektonParams = append(tektonParams, tektonv1.Param{
			Name:  key,
			Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
		})
	}
	return tektonParams
}

// ========================================
// FindWFEForPipelineRun maps PipelineRun events to WorkflowExecution reconcile requests
// Used for cross-namespace watch
// ========================================
func (r *WorkflowExecutionReconciler) FindWFEForPipelineRun(ctx context.Context, obj client.Object) []reconcile.Request {
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
// Day 5: Status Synchronization Methods
// ========================================

// BuildPipelineRunStatusSummary creates a lightweight status summary from PipelineRun
// Provides visibility into task progress during execution (v3.2)
func (r *WorkflowExecutionReconciler) BuildPipelineRunStatusSummary(pr *tektonv1.PipelineRun) *workflowexecutionv1alpha1.ExecutionStatusSummary {
	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status: "Unknown",
	}

	// Extract task counts from ChildReferences
	summary.TotalTasks = len(pr.Status.ChildReferences)

	// Get Succeeded condition
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		summary.Status = string(succeededCond.Status)
		summary.Reason = succeededCond.Reason
		summary.Message = succeededCond.Message
	}

	return summary
}

// MarkCompleted transitions WFE to Completed phase
// Calculates Duration from StartTime to CompletionTime (v3.2)
// Day 6 Extension (BR-WE-012): Resets ConsecutiveFailures counter
// Records metrics per BR-WE-008 (Day 7)
func (r *WorkflowExecutionReconciler) MarkCompleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Completed")

	// ========================================
	// DD-RO-002 Phase 3: Counter Reset Removed (Dec 19, 2025)
	// RO resets RR.Status.ConsecutiveFailureCount on successful remediation
	// WE no longer tracks routing state
	// ========================================

	// Calculate duration for use in atomic update
	now := metav1.Now()
	var completionTime *metav1.Time
	if pr != nil && pr.Status.CompletionTime != nil {
		completionTime = pr.Status.CompletionTime
	} else {
		completionTime = &now
	}

	var durationStr string
	var durationSeconds float64
	if wfe.Status.StartTime != nil && completionTime != nil {
		duration := completionTime.Sub(wfe.Status.StartTime.Time)
		durationStr = duration.Round(time.Second).String()
		durationSeconds = duration.Seconds()
	}

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidates phase transition + conditions into single API call
	// BEFORE: 2 API calls (phase update + conditions update)
	// AFTER: 1 atomic API call (50% reduction)
	// ========================================
	if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
		// Set phase (P0: Phase State Machine)
		if err := r.PhaseManager.TransitionTo(wfe, wephase.Completed); err != nil {
			return fmt.Errorf("failed to transition to Completed: %w", err)
		}

		// Set completion time
		wfe.Status.CompletionTime = completionTime

		// Set duration
		wfe.Status.Duration = durationStr

		// BR-WE-006: Set TektonPipelineComplete condition
		weconditions.SetExecutionComplete(wfe, true,
			weconditions.ReasonExecutionSucceeded,
			fmt.Sprintf("All tasks completed successfully in %s", wfe.Status.Duration))

		// Day 8: Record audit event for workflow completion (BR-WE-005, ADR-032)
		// Uses Audit Manager (P3: Audit Manager pattern)
		if err := r.AuditManager.RecordWorkflowCompleted(ctx, wfe); err != nil {
			logger.V(1).Info("Failed to record workflow.completed audit event", "error", err)
			weconditions.SetAuditRecorded(wfe, false,
				weconditions.ReasonAuditFailed,
				fmt.Sprintf("Failed to record audit event: %v", err))
		} else {
			weconditions.SetAuditRecorded(wfe, true,
				weconditions.ReasonAuditSucceeded,
				"Audit event workflow.completed recorded to DataStorage")
		}

		return nil
	}); err != nil {
		logger.Error(err, "Failed to atomically update status to Completed")
		return ctrl.Result{}, err
	}

	// Day 7: Record metrics (BR-WE-008)
	// DD-METRICS-001: Use injected metrics instead of global function
	if r.Metrics != nil {
		r.Metrics.RecordWorkflowCompletion(durationSeconds)
	}

	// V1.0: Consecutive failures gauge removed - RO handles routing (DD-RO-002)

	// Emit event
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonWorkflowCompleted,
		fmt.Sprintf("Workflow %s completed successfully in %s", wfe.Spec.WorkflowRef.WorkflowID, wfe.Status.Duration))

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for Running → Completed
	r.emitPhaseTransition(wfe, "Running", "Completed")

	logger.Info("WorkflowExecution completed (atomic status update)")

	return ctrl.Result{}, nil
}

// MarkFailed transitions WFE to Failed phase with FailureDetails
// Extracts failure information from PipelineRun (v3.2)
// Day 6 Extension (BR-WE-012): Handles exponential backoff for pre-execution failures
// Records metrics per BR-WE-008 (Day 7)
func (r *WorkflowExecutionReconciler) MarkFailed(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Failed")

	// Calculate values for atomic update
	now := metav1.Now()
	var durationStr string
	var durationSeconds float64
	if wfe.Status.StartTime != nil {
		duration := now.Sub(wfe.Status.StartTime.Time)
		durationStr = duration.Round(time.Second).String()
		durationSeconds = duration.Seconds()
	}

	// Extract failure details (Day 7: includes TaskRun-specific fields, Day 6 Extension: WasExecutionFailure)
	failureDetails := r.ExtractFailureDetails(ctx, pr, wfe.Status.StartTime)

	// Generate natural language summary
	if failureDetails != nil {
		failureDetails.NaturalLanguageSummary = r.GenerateNaturalLanguageSummary(wfe, failureDetails)
	}

	// Determine condition values
	failureReason := weconditions.ReasonExecutionFailed
	failureMessage := "Pipeline execution failed"
	if failureDetails != nil {
		// Map WE failure reasons to condition reasons
		switch failureDetails.Reason {
		case "TaskFailed":
			failureReason = weconditions.ReasonTaskFailed
			failureMessage = failureDetails.Message
		case "DeadlineExceeded":
			failureReason = weconditions.ReasonDeadlineExceeded
			failureMessage = "Pipeline exceeded timeout deadline"
		case "OOMKilled":
			failureReason = weconditions.ReasonOOMKilled
			failureMessage = "Pipeline task killed due to out of memory"
		default:
			failureMessage = failureDetails.Message
		}
	}

	// ========================================
	// DD-RO-002 Phase 3: Routing Logic Removed (Dec 19, 2025)
	// WE is now a pure executor - no routing decisions
	// RO tracks ConsecutiveFailureCount and NextAllowedExecution in RR.Status
	// RO makes ALL routing decisions BEFORE creating WFE
	// ========================================
	logger.V(1).Info("Workflow execution failed - routing handled by RO",
		"wasExecutionFailure", failureDetails != nil && failureDetails.WasExecutionFailure)

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidates phase transition + conditions into single API call
	// ========================================
	if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
		// Set phase (P0: Phase State Machine)
		if err := r.PhaseManager.TransitionTo(wfe, wephase.Failed); err != nil {
			return fmt.Errorf("failed to transition to Failed: %w", err)
		}

		// Set completion time
		wfe.Status.CompletionTime = &now

		// Set duration
		wfe.Status.Duration = durationStr

		// Set failure details
		wfe.Status.FailureDetails = failureDetails

		// BR-WE-006: Set TektonPipelineComplete condition to False
		weconditions.SetExecutionComplete(wfe, false,
			failureReason,
			failureMessage)

		// Day 8: Record audit event for workflow failure (BR-WE-005, ADR-032)
		// Uses Audit Manager (P3: Audit Manager pattern)
		if err := r.AuditManager.RecordWorkflowFailed(ctx, wfe); err != nil {
			logger.V(1).Info("Failed to record workflow.failed audit event", "error", err)
			weconditions.SetAuditRecorded(wfe, false,
				weconditions.ReasonAuditFailed,
				fmt.Sprintf("Failed to record audit event: %v", err))
		} else {
			weconditions.SetAuditRecorded(wfe, true,
				weconditions.ReasonAuditSucceeded,
				"Audit event workflow.failed recorded to DataStorage")
		}

		return nil
	}); err != nil {
		logger.Error(err, "Failed to atomically update status to Failed")
		return ctrl.Result{}, err
	}

	// Day 7: Record metrics (BR-WE-008)
	// DD-METRICS-001: Use injected metrics instead of global function
	if r.Metrics != nil {
		r.Metrics.RecordWorkflowFailure(durationSeconds)
	}

	// V1.0: Consecutive failures gauge removed - RO handles routing (DD-RO-002)

	// Emit event
	reason := "Unknown"
	if wfe.Status.FailureDetails != nil {
		reason = wfe.Status.FailureDetails.Reason
	}
	r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowFailed,
		fmt.Sprintf("Workflow %s failed: %s", wfe.Spec.WorkflowRef.WorkflowID, reason))

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for Running → Failed
	r.emitPhaseTransition(wfe, "Running", "Failed")

	return ctrl.Result{}, nil
}

// ========================================
// MarkFailedWithReason - Handle pre-execution failures
// Used for validation errors, configuration errors before PipelineRun creation
// ========================================
func (r *WorkflowExecutionReconciler) MarkFailedWithReason(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, reason, message string) error {
	logger := log.FromContext(ctx)
	logger.Info("Marking WorkflowExecution as Failed (pre-execution)",
		"reason", reason,
		"message", message,
	)

	// Calculate values for atomic update
	now := metav1.Now()

	// Create failure details for pre-execution failure
	failureDetails := &workflowexecutionv1alpha1.FailureDetails{
		Reason:              reason,
		Message:             message,
		FailedAt:            now,
		WasExecutionFailure: false, // Pre-execution failure
	}

	// Generate natural language summary
	failureDetails.NaturalLanguageSummary = r.GenerateNaturalLanguageSummary(wfe, failureDetails)

	// Determine condition values
	conditionReason := weconditions.ReasonExecutionCreationFailed
	switch reason {
	case "QuotaExceeded":
		conditionReason = weconditions.ReasonQuotaExceeded
	case "PermissionDenied", "RBACDenied":
		conditionReason = weconditions.ReasonRBACDenied
	case "ImagePullFailed":
		conditionReason = weconditions.ReasonImagePullFailed
	}

	// ========================================
	// DD-PERF-001: ATOMIC STATUS UPDATE
	// Consolidates phase transition + conditions into single API call
	// BEFORE: 2 API calls (phase update + conditions update)
	// AFTER: 1 atomic API call (50% reduction)
	// ========================================
	if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
		// Set phase (P0: Phase State Machine)
		if err := r.PhaseManager.TransitionTo(wfe, wephase.Failed); err != nil {
			return fmt.Errorf("failed to transition to Failed: %w", err)
		}

		// Set completion time (no start time for pre-execution failures)
		wfe.Status.CompletionTime = &now

		// Set failure details
		wfe.Status.FailureDetails = failureDetails

		// BR-WE-006: Set ExecutionCreated condition to False for pre-execution failures
		weconditions.SetExecutionCreated(wfe, false,
			conditionReason,
			fmt.Sprintf("Failed to create PipelineRun: %s", message))

		// Issue #99: Deprecated backoff block removed (DD-RO-002 Phase 3)
		// RO handles all routing/backoff decisions via RR.Status

		// Day 8: Record audit event for workflow failure (BR-WE-005)
		// Uses Audit Manager (P3: Audit Manager pattern)
		if err := r.AuditManager.RecordWorkflowFailed(ctx, wfe); err != nil {
			logger.V(1).Info("Failed to record workflow.failed audit event", "error", err)
			weconditions.SetAuditRecorded(wfe, false,
				weconditions.ReasonAuditFailed,
				fmt.Sprintf("Failed to record audit event: %v", err))
		} else {
			weconditions.SetAuditRecorded(wfe, true,
				weconditions.ReasonAuditSucceeded,
				"Audit event workflow.failed recorded to DataStorage")
		}

		return nil
	}); err != nil {
		logger.Error(err, "Failed to atomically update status to Failed")
		return err
	}

	// Day 7: Record metrics (BR-WE-008) - 0 duration for pre-execution failures
	// DD-METRICS-001: Use injected metrics instead of global function
	if r.Metrics != nil {
		r.Metrics.RecordWorkflowFailure(0)
	}

	// V1.0: Consecutive failures gauge removed - RO handles routing (DD-RO-002)

	// Emit event
	r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowFailed,
		fmt.Sprintf("Pre-execution failure: %s - %s", reason, message))

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for Pending → Failed
	r.emitPhaseTransition(wfe, "Pending", "Failed")

	logger.Info("WorkflowExecution failed with reason (atomic status update)", "reason", reason)

	return nil
}

// ========================================
// Helper Functions for Status Updates
// ========================================

// updateStatus is a helper that updates the WFE status with consistent error handling
func (r *WorkflowExecutionReconciler) updateStatus(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	operation string,
) error {
	logger := log.FromContext(ctx)

	if err := r.Status().Update(ctx, wfe); err != nil {
		logger.Error(err, "Failed to update status", "operation", operation)
		return err
	}
	return nil
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
