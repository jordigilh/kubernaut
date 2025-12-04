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

// Package workflowexecution implements the WorkflowExecution CRD controller
// following TDD methodology - implementation driven by failing tests
package workflowexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"knative.dev/pkg/apis"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultServiceAccountName is the default SA for PipelineRuns
	DefaultServiceAccountName = "kubernaut-workflow-runner"
)

// WorkflowExecutionReconciler reconciles a WorkflowExecution object
// TDD: Interface defined by tests in workflowexecution_controller_test.go
type WorkflowExecutionReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ExecutionNamespace string        // DD-WE-002: Dedicated namespace for PipelineRuns
	CooldownPeriod     time.Duration // BR-WE-010: Default 5 minutes
}

// NewWorkflowExecutionReconciler creates a new reconciler
// TDD: Constructor defined by test requirements
func NewWorkflowExecutionReconciler(client client.Client, scheme *runtime.Scheme, executionNamespace string) *WorkflowExecutionReconciler {
	return &WorkflowExecutionReconciler{
		Client:             client,
		Scheme:             scheme,
		ExecutionNamespace: executionNamespace,
		CooldownPeriod:     5 * time.Minute, // Default per BR-WE-010
	}
}

// +kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
// +kubebuilder:rbac:groups=tekton.dev,resources=pipelineruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile implements the reconciliation loop for WorkflowExecution
func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the WorkflowExecution
	var wfe workflowexecutionv1.WorkflowExecution
	if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch WorkflowExecution")
			return ctrl.Result{}, err
		}
		// WFE was deleted
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if !wfe.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &wfe)
	}

	// Ensure finalizer is present
	if !containsFinalizer(wfe.Finalizers, FinalizerName) {
		wfe.Finalizers = append(wfe.Finalizers, FinalizerName)
		if err := r.Update(ctx, &wfe); err != nil {
			log.Error(err, "unable to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile based on phase
	switch wfe.Status.Phase {
	case "", workflowexecutionv1.PhasePending:
		return r.reconcilePending(ctx, &wfe)
	case workflowexecutionv1.PhaseRunning:
		return r.reconcileRunning(ctx, &wfe)
	case workflowexecutionv1.PhaseCompleted, workflowexecutionv1.PhaseFailed, workflowexecutionv1.PhaseSkipped:
		return r.reconcileTerminal(ctx, &wfe)
	default:
		log.Info("unknown phase", "phase", wfe.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// FinalizerName is the finalizer for WorkflowExecution resources
const FinalizerName = "workflowexecution.kubernaut.ai/finalizer"

// SetupWithManager sets up the controller with the Manager
func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workflowexecutionv1.WorkflowExecution{}).
		Owns(&tektonv1.PipelineRun{}). // Watch PipelineRuns we create
		Complete(r)
}

// containsFinalizer checks if a finalizer is present
func containsFinalizer(finalizers []string, finalizer string) bool {
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// removeFinalizer removes a finalizer from the list
func removeFinalizer(finalizers []string, finalizer string) []string {
	result := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f != finalizer {
			result = append(result, f)
		}
	}
	return result
}

// reconcilePending handles WFE in Pending phase - creates PipelineRun
func (r *WorkflowExecutionReconciler) reconcilePending(ctx context.Context, wfe *workflowexecutionv1.WorkflowExecution) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Check resource lock (BR-WE-009)
	blocked, reason, conflicting := r.CheckResourceLock(ctx, wfe.Spec.TargetResource, wfe.Spec.WorkflowRef.WorkflowID)
	if blocked {
		log.Info("resource is locked, skipping", "reason", reason)
		r.MarkSkipped(wfe, reason, conflicting, nil)
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check cooldown (BR-WE-010)
	blocked, reason, recent := r.CheckCooldown(ctx, wfe.Spec.TargetResource, wfe.Spec.WorkflowRef.WorkflowID)
	if blocked {
		log.Info("within cooldown period, skipping", "reason", reason)
		r.MarkSkipped(wfe, reason, nil, recent)
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Build and create PipelineRun
	pr := r.BuildPipelineRun(wfe)

	// Try to create PipelineRun - deterministic name handles race conditions (DD-WE-003)
	if err := r.Create(ctx, pr); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			// Another WFE won the race - skip this one
			log.Info("PipelineRun already exists (race condition), skipping")
			r.MarkSkipped(wfe, workflowexecutionv1.SkipReasonResourceBusy, &workflowexecutionv1.ConflictingWorkflowRef{
				Name:           pr.Name,
				TargetResource: wfe.Spec.TargetResource,
				StartedAt:      metav1.Now(), // Required field
			}, nil)
			if err := r.Status().Update(ctx, wfe); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to create PipelineRun")
		return ctrl.Result{}, err
	}

	log.Info("created PipelineRun", "name", pr.Name, "namespace", pr.Namespace)

	// Update WFE status to Running
	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1.PhaseRunning
	wfe.Status.StartTime = &now
	wfe.Status.PipelineRunRef = &corev1.LocalObjectReference{Name: pr.Name}

	if err := r.Status().Update(ctx, wfe); err != nil {
		log.Error(err, "unable to update status to Running")
		return ctrl.Result{}, err
	}

	// Requeue to check PipelineRun status
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// reconcileRunning handles WFE in Running phase - monitors PipelineRun
func (r *WorkflowExecutionReconciler) reconcileRunning(ctx context.Context, wfe *workflowexecutionv1.WorkflowExecution) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if wfe.Status.PipelineRunRef == nil {
		log.Error(nil, "Running WFE has no PipelineRunRef")
		r.HandleMissingPipelineRun(wfe)
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Fetch PipelineRun
	var pr tektonv1.PipelineRun
	prKey := client.ObjectKey{
		Name:      wfe.Status.PipelineRunRef.Name,
		Namespace: r.ExecutionNamespace,
	}
	if err := r.Get(ctx, prKey, &pr); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		// PipelineRun was deleted externally (BR-WE-007)
		log.Info("PipelineRun not found, marking as failed")
		r.HandleMissingPipelineRun(wfe)
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Map PipelineRun status to WFE
	phase, outcome := r.MapPipelineRunStatus(&pr)

	if phase == workflowexecutionv1.PhaseRunning {
		// Still running, requeue
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Terminal state reached
	log.Info("PipelineRun completed", "phase", phase, "outcome", outcome)

	r.UpdateWFEStatus(wfe, &pr)

	if phase == workflowexecutionv1.PhaseFailed {
		wfe.Status.FailureDetails = r.ExtractFailureDetails(&pr)
	}

	if err := r.Status().Update(ctx, wfe); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue to handle cooldown-based lock release
	return ctrl.Result{RequeueAfter: r.CooldownPeriod}, nil
}

// reconcileTerminal handles WFE in terminal phase - manages lock release
func (r *WorkflowExecutionReconciler) reconcileTerminal(ctx context.Context, wfe *workflowexecutionv1.WorkflowExecution) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Check if cooldown has expired
	if wfe.Status.CompletionTime == nil {
		return ctrl.Result{}, nil
	}

	elapsed := time.Since(wfe.Status.CompletionTime.Time)
	if elapsed < r.CooldownPeriod {
		// Still in cooldown, requeue
		remaining := r.CooldownPeriod - elapsed
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Cooldown expired - delete PipelineRun to release lock
	if wfe.Status.PipelineRunRef != nil && !wfe.Status.LockReleased {
		prKey := client.ObjectKey{
			Name:      wfe.Status.PipelineRunRef.Name,
			Namespace: r.ExecutionNamespace,
		}
		var pr tektonv1.PipelineRun
		if err := r.Get(ctx, prKey, &pr); err == nil {
			log.Info("deleting PipelineRun after cooldown", "name", pr.Name)
			if err := r.Delete(ctx, &pr); err != nil && !strings.Contains(err.Error(), "not found") {
				return ctrl.Result{}, err
			}
		}

		// Mark lock as released
		wfe.Status.LockReleased = true
		if err := r.Status().Update(ctx, wfe); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles WFE deletion - cleanup PipelineRun
func (r *WorkflowExecutionReconciler) reconcileDelete(ctx context.Context, wfe *workflowexecutionv1.WorkflowExecution) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Cleanup PipelineRun if exists
	if wfe.Status.PipelineRunRef != nil {
		prKey := client.ObjectKey{
			Name:      wfe.Status.PipelineRunRef.Name,
			Namespace: r.ExecutionNamespace,
		}
		var pr tektonv1.PipelineRun
		if err := r.Get(ctx, prKey, &pr); err == nil {
			log.Info("deleting PipelineRun during finalization", "name", pr.Name)
			if err := r.Delete(ctx, &pr); err != nil && !strings.Contains(err.Error(), "not found") {
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer
	wfe.Finalizers = removeFinalizer(wfe.Finalizers, FinalizerName)
	if err := r.Update(ctx, wfe); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("finalization complete")
	return ctrl.Result{}, nil
}

// =============================================================================
// TDD GREEN: Implementations to make tests pass
// =============================================================================

// BuildPipelineRun creates a Tekton PipelineRun from WorkflowExecution
// TDD GREEN: BR-WE-001, BR-WE-002, BR-WE-004, BR-WE-006
func (r *WorkflowExecutionReconciler) BuildPipelineRun(wfe *workflowexecutionv1.WorkflowExecution) *tektonv1.PipelineRun {
	// Get deterministic name based on target resource (DD-WE-003)
	prName := r.GetPipelineRunName(wfe.Spec.TargetResource)

	// Get ServiceAccount name (BR-WE-006)
	saName := wfe.Spec.ExecutionConfig.ServiceAccountName
	if saName == "" {
		saName = DefaultServiceAccountName
	}

	// Build parameters (BR-WE-002)
	params := r.convertParameters(wfe.Spec.Parameters)

	// Create PipelineRun with bundle resolver (ADR-044)
	pr := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prName,
			Namespace: r.ExecutionNamespace, // DD-WE-002: Dedicated namespace
			Labels: map[string]string{
				"kubernaut.ai/workflow-execution": wfe.Name,
				"kubernaut.ai/source-namespace":   wfe.Namespace,
				"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
			},
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: "bundles",
					Params: []tektonv1.Param{
						{
							Name:  "bundle",
							Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.WorkflowRef.ContainerImage},
						},
						{
							Name:  "name",
							Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "workflow"},
						},
						{
							Name:  "kind",
							Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "pipeline"},
						},
					},
				},
			},
			Params: params,
			TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
				ServiceAccountName: saName,
			},
		},
	}

	// Note: No OwnerReferences because PipelineRun is cross-namespace (BR-WE-004)
	// Labels are used for tracking instead

	return pr
}

// convertParameters converts map[string]string to []tektonv1.Param
// TDD GREEN: BR-WE-002
func (r *WorkflowExecutionReconciler) convertParameters(params map[string]string) []tektonv1.Param {
	if len(params) == 0 {
		return nil
	}

	result := make([]tektonv1.Param, 0, len(params))
	for name, value := range params {
		result = append(result, tektonv1.Param{
			Name:  name,
			Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: value},
		})
	}
	return result
}

// GetPipelineRunName returns deterministic name based on target resource
// TDD GREEN: BR-WE-011, DD-WE-003
func (r *WorkflowExecutionReconciler) GetPipelineRunName(targetResource string) string {
	h := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}

// MapPipelineRunStatus maps Tekton status to WFE phase and outcome
// TDD GREEN: BR-WE-003
func (r *WorkflowExecutionReconciler) MapPipelineRunStatus(pr *tektonv1.PipelineRun) (phase string, outcome string) {
	cond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if cond == nil {
		return workflowexecutionv1.PhasePending, ""
	}

	switch cond.Status {
	case corev1.ConditionTrue:
		return workflowexecutionv1.PhaseCompleted, workflowexecutionv1.OutcomeSuccess
	case corev1.ConditionFalse:
		return workflowexecutionv1.PhaseFailed, workflowexecutionv1.OutcomeFailure
	default: // Unknown
		return workflowexecutionv1.PhaseRunning, ""
	}
}

// UpdateWFEStatus updates WorkflowExecution status from PipelineRun
// TDD GREEN: BR-WE-003
func (r *WorkflowExecutionReconciler) UpdateWFEStatus(wfe *workflowexecutionv1.WorkflowExecution, pr *tektonv1.PipelineRun) {
	phase, _ := r.MapPipelineRunStatus(pr)

	if phase == workflowexecutionv1.PhaseCompleted || phase == workflowexecutionv1.PhaseFailed {
		now := metav1.Now()
		wfe.Status.CompletionTime = &now

		// Calculate duration if StartTime is set
		if wfe.Status.StartTime != nil {
			duration := now.Sub(wfe.Status.StartTime.Time)
			wfe.Status.Duration = duration.Round(time.Second).String()
		}
	}

	wfe.Status.Phase = phase
}

// CheckResourceLock checks if resource is locked by another workflow
// TDD GREEN: BR-WE-009
func (r *WorkflowExecutionReconciler) CheckResourceLock(ctx context.Context, targetResource string, workflowID string) (blocked bool, reason string, conflicting *workflowexecutionv1.ConflictingWorkflowRef) {
	// List all WorkflowExecutions
	var wfeList workflowexecutionv1.WorkflowExecutionList
	if err := r.List(ctx, &wfeList); err != nil {
		return false, "", nil
	}

	// Check for Running or Pending WFEs on the same target
	for i := range wfeList.Items {
		wfe := &wfeList.Items[i]
		if wfe.Spec.TargetResource == targetResource {
			if wfe.Status.Phase == workflowexecutionv1.PhaseRunning ||
				wfe.Status.Phase == workflowexecutionv1.PhasePending {
				// Found blocking WFE
				return true, workflowexecutionv1.SkipReasonResourceBusy, &workflowexecutionv1.ConflictingWorkflowRef{
					Name:           wfe.Name,
					WorkflowID:     wfe.Spec.WorkflowRef.WorkflowID,
					StartedAt:      metav1.Now(), // Use current time if StartTime not set
					TargetResource: wfe.Spec.TargetResource,
				}
			}
		}
	}

	return false, "", nil
}

// CheckCooldown checks if same workflow+target was recently executed
// TDD GREEN: BR-WE-010
func (r *WorkflowExecutionReconciler) CheckCooldown(ctx context.Context, targetResource string, workflowID string) (blocked bool, reason string, recent *workflowexecutionv1.RecentRemediationRef) {
	// List all WorkflowExecutions
	var wfeList workflowexecutionv1.WorkflowExecutionList
	if err := r.List(ctx, &wfeList); err != nil {
		return false, "", nil
	}

	now := time.Now()

	// Check for recently completed WFEs with same target+workflow
	for i := range wfeList.Items {
		wfe := &wfeList.Items[i]
		if wfe.Spec.TargetResource == targetResource &&
			wfe.Spec.WorkflowRef.WorkflowID == workflowID {
			if (wfe.Status.Phase == workflowexecutionv1.PhaseCompleted ||
				wfe.Status.Phase == workflowexecutionv1.PhaseFailed) &&
				wfe.Status.CompletionTime != nil {
				// Check if within cooldown period
				elapsed := now.Sub(wfe.Status.CompletionTime.Time)
				if elapsed < r.CooldownPeriod {
					remaining := r.CooldownPeriod - elapsed
					return true, workflowexecutionv1.SkipReasonRecentlyRemediated, &workflowexecutionv1.RecentRemediationRef{
						Name:              wfe.Name,
						WorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
						CompletedAt:       *wfe.Status.CompletionTime,
						Outcome:           wfe.Status.Phase, // Completed or Failed
						TargetResource:    wfe.Spec.TargetResource,
						CooldownRemaining: remaining.Round(time.Second).String(),
					}
				}
			}
		}
	}

	return false, "", nil
}

// MarkSkipped marks WorkflowExecution as Skipped with details
// TDD GREEN: BR-WE-009, BR-WE-010
func (r *WorkflowExecutionReconciler) MarkSkipped(wfe *workflowexecutionv1.WorkflowExecution, reason string, conflicting *workflowexecutionv1.ConflictingWorkflowRef, recent *workflowexecutionv1.RecentRemediationRef) {
	wfe.Status.Phase = workflowexecutionv1.PhaseSkipped
	wfe.Status.SkipDetails = &workflowexecutionv1.SkipDetails{
		Reason:    reason,
		Message:   r.generateSkipMessage(reason, conflicting, recent),
		SkippedAt: metav1.Now(),
	}

	if conflicting != nil {
		wfe.Status.SkipDetails.ConflictingWorkflow = conflicting
	}
	if recent != nil {
		wfe.Status.SkipDetails.RecentRemediation = recent
	}
}

// generateSkipMessage creates a human-readable skip message
func (r *WorkflowExecutionReconciler) generateSkipMessage(reason string, conflicting *workflowexecutionv1.ConflictingWorkflowRef, recent *workflowexecutionv1.RecentRemediationRef) string {
	switch reason {
	case workflowexecutionv1.SkipReasonResourceBusy:
		if conflicting != nil {
			return fmt.Sprintf("Resource is currently being remediated by workflow '%s' (workflowId: %s)",
				conflicting.Name, conflicting.WorkflowID)
		}
		return "Resource is currently being remediated by another workflow"
	case workflowexecutionv1.SkipReasonRecentlyRemediated:
		if recent != nil {
			return fmt.Sprintf("Same workflow '%s' was recently executed on this target (completed: %s, cooldown remaining: %s)",
				recent.WorkflowID, recent.CompletedAt.Format(time.RFC3339), recent.CooldownRemaining)
		}
		return "Same workflow was recently executed on this target"
	default:
		return "Execution was skipped"
	}
}

// HandleMissingPipelineRun handles the case when PipelineRun is deleted externally
// TDD GREEN: BR-WE-007
func (r *WorkflowExecutionReconciler) HandleMissingPipelineRun(wfe *workflowexecutionv1.WorkflowExecution) {
	wfe.Status.Phase = workflowexecutionv1.PhaseFailed
	wfe.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
		Reason:                     workflowexecutionv1.FailureReasonUnknown,
		Message:                    "PipelineRun was deleted externally before completion",
		NaturalLanguageSummary:     "The workflow execution failed because the underlying PipelineRun was deleted before it could complete. This may have been done by an operator or automated cleanup process.",
		FailedAt:                   metav1.Now(),
		WasExecutionFailure:        false, // Pre-execution failure - safe to retry
		ExecutionTimeBeforeFailure: "0s",
	}
}

// ExtractFailureDetails extracts structured failure information from PipelineRun
// TDD GREEN: BR-WE-005 (supports audit and notifications)
func (r *WorkflowExecutionReconciler) ExtractFailureDetails(pr *tektonv1.PipelineRun) *workflowexecutionv1.FailureDetails {
	cond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if cond == nil {
		return nil
	}

	// Map message to Kubernetes-style reason code
	reason := r.mapTektonMessageToReason(cond.Message)

	// Generate natural language summary
	nlSummary := r.generateNaturalLanguageSummary(reason, cond.Message)

	return &workflowexecutionv1.FailureDetails{
		Reason:                     reason,
		Message:                    cond.Message,
		NaturalLanguageSummary:     nlSummary,
		FailedAt:                   metav1.Now(),
		WasExecutionFailure:        true, // Tekton ran, so execution failure
		ExecutionTimeBeforeFailure: "unknown",
	}
}

// mapTektonMessageToReason maps Tekton message to Kubernetes-style reason code
func (r *WorkflowExecutionReconciler) mapTektonMessageToReason(message string) string {
	msg := strings.ToLower(message)

	switch {
	case strings.Contains(msg, "oomkilled") || strings.Contains(msg, "oom"):
		return workflowexecutionv1.FailureReasonOOMKilled
	case strings.Contains(msg, "deadline") || strings.Contains(msg, "timeout"):
		return workflowexecutionv1.FailureReasonDeadlineExceeded
	case strings.Contains(msg, "forbidden") || strings.Contains(msg, "permission"):
		return workflowexecutionv1.FailureReasonForbidden
	case strings.Contains(msg, "imagepullbackoff") || strings.Contains(msg, "errimagepull"):
		return workflowexecutionv1.FailureReasonImagePullBackOff
	case strings.Contains(msg, "resourceexhausted") || strings.Contains(msg, "quota"):
		return workflowexecutionv1.FailureReasonResourceExhausted
	case strings.Contains(msg, "configuration") || strings.Contains(msg, "invalid"):
		return workflowexecutionv1.FailureReasonConfigurationError
	default:
		return workflowexecutionv1.FailureReasonUnknown
	}
}

// generateNaturalLanguageSummary creates a human-readable failure summary
func (r *WorkflowExecutionReconciler) generateNaturalLanguageSummary(reason, message string) string {
	switch reason {
	case workflowexecutionv1.FailureReasonOOMKilled:
		return fmt.Sprintf("The workflow task was terminated because it exceeded its memory limit. Consider increasing the memory allocation or optimizing the workflow. Original error: %s", message)
	case workflowexecutionv1.FailureReasonDeadlineExceeded:
		return fmt.Sprintf("The workflow exceeded its timeout limit. Consider increasing the timeout or investigating why the workflow takes longer than expected. Original error: %s", message)
	case workflowexecutionv1.FailureReasonForbidden:
		return fmt.Sprintf("The workflow failed due to insufficient permissions. Verify that the ServiceAccount has the required RBAC permissions. Original error: %s", message)
	case workflowexecutionv1.FailureReasonImagePullBackOff:
		return fmt.Sprintf("The workflow container image could not be pulled. Verify the image reference and registry credentials. Original error: %s", message)
	case workflowexecutionv1.FailureReasonResourceExhausted:
		return fmt.Sprintf("The workflow failed due to cluster resource limits (e.g., ResourceQuota). Consider requesting more resources or freeing up existing resources. Original error: %s", message)
	case workflowexecutionv1.FailureReasonConfigurationError:
		return fmt.Sprintf("The workflow failed due to invalid configuration or parameters. Review the workflow parameters and configuration. Original error: %s", message)
	default:
		return fmt.Sprintf("The workflow failed with an unclassified error. Review the detailed error message for more information: %s", message)
	}
}
