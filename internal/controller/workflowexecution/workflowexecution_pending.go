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

// Pending-phase reconciliation logic, split out of workflowexecution_controller.go
// per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520) to keep the file under
// the 700-line convention threshold. Pure structural move — no behavior change.
package workflowexecution

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
)

// resolveSchemaMetadata fetches all workflow catalog artifacts from DS in a
// single call (F6: consolidation of 3 DS round-trips into 1).
// Returns nil metadata (not an error) when no querier is configured or when
// the DS call fails — graceful degradation preserves execution without
// filtering or engine config.
//
// Issue #1481: dependency existence is no longer validated here (or anywhere
// pre-execution) — a schema-declared Secret/ConfigMap dependency is mounted
// as-is and Kubernetes validates its existence at runtime when the Job/
// PipelineRun attempts to mount the volume (BR-WORKFLOW-008 covers the
// resulting fail-fast/observability guarantees). The error return is kept
// for signature stability at the call site (resolvePendingSchemaAndEngine);
// it is currently always nil.
//
//nolint:unparam // see doc comment above -- error kept for call-site signature stability (Issue #1546 Tier 4)
func (r *WorkflowExecutionReconciler) resolveSchemaMetadata(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (*weclient.SchemaMetadata, weexecutor.CreateOptions, error) {
	opts := weexecutor.CreateOptions{}
	if r.WorkflowQuerier == nil {
		return nil, opts, nil
	}

	logger := log.FromContext(ctx)
	meta, err := r.WorkflowQuerier.GetWorkflowSchemaMetadata(ctx, wfe.Spec.WorkflowRef.WorkflowID)
	if err != nil {
		logger.Error(err, "Failed to fetch workflow schema metadata from DS (non-fatal, continuing without filtering)",
			"workflowID", wfe.Spec.WorkflowRef.WorkflowID)
		if r.Metrics != nil {
			r.Metrics.SchemaFetchFailures.Inc()
		}
		return nil, opts, nil
	}
	if meta == nil {
		return nil, opts, nil
	}

	opts.Dependencies = meta.Dependencies
	opts.DeclaredParameterNames = meta.DeclaredParameterNames
	return meta, opts, nil
}

// ========================================
// reconcilePending - Handle Pending phase
// V1.0: Pure execution logic, RO handles all routing (DD-RO-002)
// ========================================
func (r *WorkflowExecutionReconciler) reconcilePending(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Pending phase")

	if result, err, shouldReturn := r.refetchFreshPendingWFE(ctx, wfe, logger); shouldReturn {
		return result, err
	}

	// V1.0: No routing logic - RO makes ALL routing decisions before creating WFE
	// If WFE exists, execute it. RO already checked routing.

	if result, err, shouldReturn := r.validateAndAnnouncePendingSpec(ctx, wfe, logger); shouldReturn {
		return result, err
	}

	schemaMeta, createOpts, engine, schemaResult, schemaErr, shouldReturn := r.resolvePendingSchemaAndEngine(ctx, wfe, logger)
	if shouldReturn {
		return schemaResult, schemaErr
	}

	if cooldownResult, cooldownErr, blocked := r.checkPendingCooldownOrBlock(ctx, wfe, logger); blocked {
		return cooldownResult, cooldownErr
	}

	resourceName, auditResult, auditErr, shouldReturn := r.recordPendingSelectionAudit(ctx, wfe, schemaMeta, logger)
	if shouldReturn {
		return auditResult, auditErr
	}

	createResult, createResourceResult, createResourceErr, shouldReturn := r.createPendingExecutionResource(ctx, wfe, resourceName, createOpts, logger)
	if shouldReturn {
		return createResourceResult, createResourceErr
	}

	return r.finalizePendingToRunning(ctx, wfe, createResult, engine, logger)
}

// refetchFreshPendingWFE re-reads wfe from the API server via APIReader to
// bypass the informer cache (DD-STATUS-001), preventing race conditions where
// concurrent reconciles observe stale data (F1: stale ExecutionRef, F2: stale
// Phase causing duplicate audit events). On success, wfe is overwritten in
// place with the fresh data. shouldReturn is true when the caller must
// immediately return (result, err) — WFE deleted, read failure, or already
// progressed past Pending. Extracted from reconcilePending per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *WorkflowExecutionReconciler) refetchFreshPendingWFE(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, logger logr.Logger) (ctrl.Result, error, bool) {
	freshWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
	if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(wfe), freshWFE); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil, true // WFE was deleted
		}
		return ctrl.Result{}, fmt.Errorf("failed to re-read WFE from API server: %w", err), true
	}
	// If the fresh WFE has already progressed past Pending, requeue to let the
	// main Reconcile re-route based on the updated phase.
	if freshWFE.Status.Phase != "" && freshWFE.Status.Phase != workflowexecutionv1alpha1.PhasePending {
		logger.Info("WFE already progressed past Pending (informer cache was stale), requeueing",
			"freshPhase", freshWFE.Status.Phase)
		return ctrl.Result{Requeue: true}, nil, true
	}
	// Use fresh data for the remainder of this reconcile
	*wfe = *freshWFE
	return ctrl.Result{}, nil, false
}

// validateAndAnnouncePendingSpec validates wfe's spec (BR-WE-014 guard
// against malformed PipelineRuns), emitting the WorkflowValidated/
// WorkflowValidationFailed events (DD-EVENT-001 v1.1) and marking the WFE
// Failed with ConfigurationError on validation failure (pre-execution
// failure, wasExecutionFailure: false). Extracted from reconcilePending per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
//
//nolint:unparam // ctrl.Result is always the zero value here; signature matches the shared (ctrl.Result, error, shouldReturn) contract of sibling reconcilePending step helpers, called uniformly as `if result, err, shouldReturn := r.xxx(...); shouldReturn { return result, err }` (Issue #1546 Tier 4)
func (r *WorkflowExecutionReconciler) validateAndAnnouncePendingSpec(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, logger logr.Logger) (ctrl.Result, error, bool) {
	if err := r.ValidateSpec(wfe); err != nil {
		logger.Error(err, "Spec validation failed")
		// DD-EVENT-001 v1.1: Emit WorkflowValidationFailed event (P2: Decision Point)
		r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowValidationFailed,
			fmt.Sprintf("Spec validation failed: %v", err))
		if markErr := r.MarkFailedWithReason(ctx, wfe, "ConfigurationError", err.Error()); markErr != nil {
			return ctrl.Result{}, markErr, true
		}
		return ctrl.Result{}, nil, true
	}

	// DD-EVENT-001 v1.1: Emit WorkflowValidated event (P2: Decision Point)
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonWorkflowValidated,
		fmt.Sprintf("Workflow spec validated: %s (target: %s)", wfe.Spec.WorkflowRef.WorkflowID, wfe.Spec.TargetResource))
	return ctrl.Result{}, nil, false
}

// resolvePendingSchemaAndEngine resolves the workflow's catalog metadata
// (schema, engineConfig, dependencies, execution bundle/SA) from DataStorage
// in a single call (F6), then determines the execution engine (Issue #518:
// catalog preferred, schema fallback). shouldReturn is true when the caller
// must immediately return (result, err) — schema resolution failure or no
// engine defined. Extracted from reconcilePending per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
//
//nolint:unparam // ctrl.Result is always the zero value here; signature matches the shared reconcilePending step-helper contract (see validateAndAnnouncePendingSpec) (Issue #1546 Tier 4)
func (r *WorkflowExecutionReconciler) resolvePendingSchemaAndEngine(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, logger logr.Logger) (*weclient.SchemaMetadata, weexecutor.CreateOptions, string, ctrl.Result, error, bool) {
	// ========================================
	// Step 1.2: Resolve all workflow catalog metadata from DS in a single call (F6)
	// Engine (#518), engineConfig (DD-WORKFLOW-017), dependencies (DD-WE-006),
	// and declared parameter names (#243) are fetched together.
	// ========================================
	schemaMeta, createOpts, schemaErr := r.resolveSchemaMetadata(ctx, wfe)
	if schemaErr != nil {
		r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowValidationFailed,
			fmt.Sprintf("Workflow dependency validation failed: %v", schemaErr))
		markErr := r.MarkFailedWithReason(ctx, wfe, "ConfigurationError", schemaErr.Error())
		return nil, weexecutor.CreateOptions{}, "", ctrl.Result{}, markErr, true
	}

	// DD-WORKFLOW-017: Resolve engineConfig from schema before catalog (schema takes precedence).
	if wfe.Spec.WorkflowRef.EngineConfig == nil && schemaMeta != nil && schemaMeta.EngineConfig != nil {
		logger.Info("Resolved engineConfig from DS schema metadata",
			"workflowID", wfe.Spec.WorkflowRef.WorkflowID)
		wfe.Spec.WorkflowRef.EngineConfig = &apiextensionsv1.JSON{Raw: schemaMeta.EngineConfig}
	}

	// Step 1.3: Resolve execution bundle, SA, and engine from DS catalog (Issue #650).
	// Called BEFORE setting engine from schema so the idempotency guard in
	// resolveWorkflowCatalog doesn't skip the SA and bundle resolution that
	// only the catalog provides.
	if _, catalogErr := r.resolveWorkflowCatalog(ctx, wfe); catalogErr != nil && !errors.Is(catalogErr, ErrAlreadyResolved) {
		logger.Error(catalogErr, "Failed to resolve workflow catalog from DS (non-fatal)")
	}

	// Resolve execution engine: prefer catalog (includes SA + bundle), schema fallback (Issue #518).
	if wfe.Status.ExecutionEngine == "" && schemaMeta != nil && schemaMeta.Engine != "" {
		wfe.Status.ExecutionEngine = schemaMeta.Engine
	}
	engine := wfe.Status.ExecutionEngine
	if engine == "" {
		engineErr := fmt.Errorf("no engine defined in remediation workflow %s", wfe.Spec.WorkflowRef.WorkflowID)
		if schemaMeta != nil && schemaMeta.WorkflowName != "" {
			engineErr = fmt.Errorf("no engine defined in remediation workflow %s - %s",
				schemaMeta.WorkflowName, wfe.Spec.WorkflowRef.WorkflowID)
		}
		logger.Error(engineErr, "Execution engine resolution failed")
		r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowValidationFailed,
			fmt.Sprintf("Workflow catalog resolution failed: %v", engineErr))
		if markErr := r.MarkFailedWithReason(ctx, wfe, "ConfigurationError", engineErr.Error()); markErr != nil {
			return schemaMeta, createOpts, "", ctrl.Result{}, markErr, true
		}
		return schemaMeta, createOpts, "", ctrl.Result{}, nil, true
	}
	logger.Info("Resolved execution engine from catalog", "engine", engine, "workflowID", wfe.Spec.WorkflowRef.WorkflowID)

	return schemaMeta, createOpts, engine, ctrl.Result{}, nil, false
}

// checkPendingCooldownOrBlock enforces the target-resource cooldown
// (BR-WE-009) during the Pending phase (previously only enforced in the
// terminal phase). If active, ensures the phase remains Pending, emits the
// CooldownActive event (DD-EVENT-001 v1.1), and requeues after the
// remaining cooldown. shouldReturn is true when the caller must immediately
// return (result, err). Extracted from reconcilePending per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *WorkflowExecutionReconciler) checkPendingCooldownOrBlock(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, logger logr.Logger) (ctrl.Result, error, bool) {
	currentWFEKey := fmt.Sprintf("%s/%s", wfe.Namespace, wfe.Name)
	remaining, active := r.CheckCooldownActive(ctx, wfe.Spec.TargetResource, currentWFEKey)
	if !active {
		return ctrl.Result{}, nil, false
	}

	logger.Info("Blocking execution due to active cooldown",
		"targetResource", wfe.Spec.TargetResource,
		"remaining", remaining,
	)
	// Ensure phase is set to Pending if not already set (P0: Phase State Machine)
	if wfe.Status.Phase == "" || wfe.Status.Phase != workflowexecutionv1alpha1.PhasePending {
		if err := r.PhaseManager.TransitionTo(wfe, wephase.Pending); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to transition to Pending during cooldown: %w", err), true
		}
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update phase to Pending during cooldown: %w", err), true
		}
	}
	// DD-EVENT-001 v1.1: Emit CooldownActive event (P2: Decision Point)
	r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonCooldownActive,
		fmt.Sprintf("Execution deferred: cooldown active for %s (remaining: %s)", wfe.Spec.TargetResource, remaining.Round(time.Second)))

	// Stay in Pending, requeue after cooldown expires
	return ctrl.Result{RequeueAfter: remaining}, nil, true
}

// recordPendingSelectionAudit checks (via the non-cached APIReader,
// DD-STATUS-001) whether the execution resource already exists, and records
// the workflow.selection.completed audit event (Gap #5, BR-AUDIT-005)
// exactly once — skipping it when the resource already exists to preserve
// idempotency across reconciles. shouldReturn is true when the execution
// resource was deleted externally during Pending and the WFE was marked
// Failed. Extracted from reconcilePending per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *WorkflowExecutionReconciler) recordPendingSelectionAudit(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, schemaMeta *weclient.SchemaMetadata, logger logr.Logger) (string, ctrl.Result, error, bool) {
	resourceName := weexecutor.ExecutionResourceName(wfe.Spec.TargetResource)
	resourceExists := false
	switch wfe.Status.ExecutionEngine {
	case "job":
		existingJob := &batchv1.Job{}
		err := r.APIReader.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: r.ExecutionNamespace}, existingJob)
		if err == nil {
			resourceExists = true
		} else if apierrors.IsNotFound(err) && wfe.Status.ExecutionRef != nil {
			logger.Error(err, "Execution resource not found - deleted externally during Pending phase",
				"engine", wfe.Status.ExecutionEngine)
			result, markErr := r.MarkFailed(ctx, wfe, nil)
			return resourceName, result, markErr, true
		}
	case workflowexecutionv1alpha1.ExecutionEngineTekton:
		existingPR := &tektonv1.PipelineRun{}
		err := r.APIReader.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: r.ExecutionNamespace}, existingPR)
		if err == nil {
			resourceExists = true
		} else if apierrors.IsNotFound(err) && wfe.Status.ExecutionRef != nil {
			logger.Error(err, "Execution resource not found - deleted externally during Pending phase",
				"engine", wfe.Status.ExecutionEngine)
			result, markErr := r.MarkFailed(ctx, wfe, nil)
			return resourceName, result, markErr, true
		}
	case "ansible":
		// AWX manages jobs externally — no K8s resource to check.
		// Use ExecutionRef as the idempotency signal.
		resourceExists = wfe.Status.ExecutionRef != nil
	default:
		logger.Info("Unknown engine for existence check, skipping audit guard",
			"engine", wfe.Status.ExecutionEngine)
	}

	if !resourceExists {
		workflowName := ""
		if schemaMeta != nil {
			workflowName = schemaMeta.WorkflowName
		}
		if err := r.AuditManager.RecordWorkflowSelectionCompleted(ctx, wfe, workflowName); err != nil {
			logger.V(1).Info("Failed to record workflow.selection.completed audit event", "error", err)
		}
	} else {
		logger.V(2).Info("Skipping workflow.selection.completed audit event - execution resource already exists",
			"resource", resourceName, "engine", wfe.Status.ExecutionEngine)
	}

	return resourceName, ctrl.Result{}, nil, false
}

// createPendingExecutionResource dispatches to the engine-specific executor
// (BR-WE-014) to create the execution resource, handling the
// already-exists collision path (DD-WE-003 Layer 2: Tekton delegates to
// HandleAlreadyExists; Job attempts terminal-state cleanup + retry via
// handleJobAlreadyExists, Issue #374/#383/#190). shouldReturn is true when
// the caller must immediately return (result, err) — unsupported engine,
// unresolvable collision, or hard creation failure. Reads the engine from
// wfe.Status.ExecutionEngine (set by resolvePendingSchemaAndEngine before
// this is called, so it is always identical to the caller's local `engine`
// variable). Extracted from reconcilePending per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *WorkflowExecutionReconciler) createPendingExecutionResource(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, resourceName string, createOpts weexecutor.CreateOptions, logger logr.Logger) (*weexecutor.CreateResult, ctrl.Result, error, bool) {
	exec, err := r.ExecutorRegistry.Get(wfe.Status.ExecutionEngine)
	if err != nil {
		// Issue #868: Provide actionable guidance for unavailable engines
		guidance := engineGuidance(wfe.Status.ExecutionEngine)
		msg := fmt.Sprintf("execution engine %q is not available -- %s", wfe.Status.ExecutionEngine, guidance)
		logger.Error(err, "Unsupported execution engine", "engine", wfe.Status.ExecutionEngine)
		r.Recorder.Event(wfe, corev1.EventTypeWarning, events.EventReasonWorkflowValidationFailed, msg)
		markErr := r.MarkFailedWithReason(ctx, wfe, "UnsupportedEngine", msg)
		return &weexecutor.CreateResult{}, ctrl.Result{}, markErr, true
	}

	logger.Info("Creating execution resource",
		"engine", wfe.Status.ExecutionEngine,
		"resource", resourceName,
		"namespace", r.ExecutionNamespace,
	)

	// DD-WE-006 / F6: Dependencies, parameter names, and dependency validation
	// already resolved by resolveSchemaMetadata above (createOpts is populated).

	createResult, createErr := exec.Create(ctx, wfe, r.ExecutionNamespace, createOpts)
	if createErr != nil {
		if !apierrors.IsAlreadyExists(createErr) {
			logger.Error(createErr, "Failed to create execution resource", "engine", wfe.Status.ExecutionEngine)
			markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown",
				fmt.Sprintf("Failed to create %s execution resource: %v", wfe.Status.ExecutionEngine, createErr))
			return &weexecutor.CreateResult{}, ctrl.Result{}, markErr, true
		}

		// DD-WE-003 Layer 2: Execution-time collision handling
		result, res, err, shouldReturn := r.handleCreateAlreadyExists(ctx, wfe, resourceName, createOpts, exec, createErr, logger)
		if shouldReturn {
			return result, res, err, true
		}
		createResult = result
	}

	return createResult, ctrl.Result{}, nil, false
}

// handleCreateAlreadyExists resolves a DD-WE-003 Layer 2 execution-time
// collision (the execution resource already exists): Tekton delegates to
// HandleAlreadyExists, Job attempts terminal-state cleanup + retry via
// handleJobAlreadyExists (Issue #374/#383/#190), and any other outcome
// (including non-tekton/non-job engines) fails the WFE with a
// resource-locked message. shouldReturn is true when the caller must
// immediately return (result, res, err); when false, createResult replaces
// the caller's create result (Job retry succeeded) and processing continues.
// Extracted from createPendingExecutionResource (Wave 6 6e-ii GREEN: nestif
// remediation) — pure code motion, no behavior change.
func (r *WorkflowExecutionReconciler) handleCreateAlreadyExists(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, resourceName string, createOpts weexecutor.CreateOptions, exec weexecutor.Executor, createErr error, logger logr.Logger) (*weexecutor.CreateResult, ctrl.Result, error, bool) {
	if wfe.Status.ExecutionEngine == workflowexecutionv1alpha1.ExecutionEngineTekton {
		result, handleErr := r.HandleAlreadyExists(ctx, wfe, resourceName, createErr)
		return &weexecutor.CreateResult{}, result, handleErr, true
	}

	// Issue #374 / DD-WE-003: Pre-execution cleanup of completed Jobs.
	// If the existing Job is in a terminal state (Succeeded/Failed), clean it up
	// and retry creation. If still running, the lock is valid -- fail the WFE.
	if wfe.Status.ExecutionEngine == "job" {
		retryResult, handled, requeueForGC, originalWFE := r.handleJobAlreadyExists(ctx, exec, wfe, resourceName, createOpts)
		switch {
		case handled:
			return retryResult, ctrl.Result{}, nil, false
		case requeueForGC:
			logger.Info("Requeuing for Job GC completion (Issue #383)", "resource", resourceName)
			return &weexecutor.CreateResult{}, ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil, true
		case originalWFE != "":
			// Issue #190: Valid lock owned by another WFE — classify as Deduplicated.
			markErr := r.MarkFailedAsDeduplicated(ctx, wfe, originalWFE)
			return &weexecutor.CreateResult{}, ctrl.Result{}, markErr, true
		}
	}

	markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown",
		fmt.Sprintf("Execution resource %s already exists (target resource locked)", resourceName))
	return &weexecutor.CreateResult{}, ctrl.Result{}, markErr, true
}

// finalizePendingToRunning persists the created execution resource's
// warnings (Issue #501) and observability trail (execution.workflow.started
// audit — Gap #6/BR-AUDIT-005, ExecutionCreated condition — BR-WE-006),
// transitions the WFE to Running (P0: Phase State Machine), and requeues to
// poll execution status. Extracted from reconcilePending per
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2 (issue #1520).
func (r *WorkflowExecutionReconciler) finalizePendingToRunning(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, createResult *weexecutor.CreateResult, engine string, logger logr.Logger) (ctrl.Result, error) {
	createdName := createResult.ResourceName

	// Restore in-memory status fields that may have been lost if Create() performed
	// an internal status update (e.g., storeCredentialIDs). These fields were set by
	// resolveWorkflowCatalog but not yet persisted to the API server.
	wfe.Status.ExecutionEngine = engine

	// Issue #501: Process warnings from CreateResult (e.g., TokenTTL issues)
	for _, w := range createResult.Warnings {
		meta.SetStatusCondition(&wfe.Status.Conditions, metav1.Condition{
			Type:               w.Type,
			Status:             metav1.ConditionTrue,
			Reason:             w.Reason,
			Message:            w.Message,
			ObservedGeneration: wfe.Generation,
		})
		r.Recorder.Event(wfe, corev1.EventTypeWarning, w.Reason, w.Message)
		logger.Info("CreateResult warning applied", "type", w.Type, "reason", w.Reason)
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
			wfe.Status.ExecutionEngine, createdName, r.ExecutionNamespace))

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
		fmt.Sprintf("Created %s execution resource %s/%s", wfe.Status.ExecutionEngine, r.ExecutionNamespace, createdName))

	// DD-EVENT-001 v1.1: PhaseTransition breadcrumb for Pending → Running
	r.emitPhaseTransition(wfe, "Pending", "Running")

	// Requeue to check execution status
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
