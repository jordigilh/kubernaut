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

package workflowexecution

import (
	"context"
	"fmt"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ============================================================================
// PipelineRun Building (BR-WE-001, BR-WE-002)
// ============================================================================

// buildPipelineRun creates a Tekton PipelineRun from the WorkflowExecution spec
// Uses bundle resolver for OCI bundles per ADR-043
func (r *WorkflowExecutionReconciler) buildPipelineRun(wfe *workflowexecutionv1alpha1.WorkflowExecution) *tektonv1.PipelineRun {
	prName := pipelineRunName(wfe.Spec.TargetResource)

	// Determine ServiceAccount
	saName := r.ServiceAccountName
	if wfe.Spec.ExecutionConfig.ServiceAccountName != "" {
		saName = wfe.Spec.ExecutionConfig.ServiceAccountName
	}

	// Convert parameters to Tekton params (BR-WE-002)
	var params tektonv1.Params
	for k, v := range wfe.Spec.Parameters {
		params = append(params, tektonv1.Param{
			Name:  k,
			Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: v},
		})
	}

	// Build PipelineRun with bundle resolver
	pr := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prName,
			Namespace: r.ExecutionNamespace,
			Labels: map[string]string{
				labelWorkflowExecution: wfe.Name,
				labelSourceNamespace:   wfe.Namespace,
				labelWorkflowID:        wfe.Spec.WorkflowRef.WorkflowID,
				labelTargetResource:    truncateLabel(wfe.Spec.TargetResource, 63),
			},
			Annotations: map[string]string{
				"kubernaut.ai/remediation-request": wfe.Spec.RemediationRequestRef.Name,
				"kubernaut.ai/confidence":          fmt.Sprintf("%.2f", wfe.Spec.Confidence),
			},
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineRef: &tektonv1.PipelineRef{
				ResolverRef: tektonv1.ResolverRef{
					Resolver: "bundles",
					Params: tektonv1.Params{
						{Name: "bundle", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
						{Name: "name", Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: "workflow"}},
					},
				},
			},
			Params: params,
			TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
				ServiceAccountName: saName,
			},
		},
	}

	// Set timeout if specified
	if wfe.Spec.ExecutionConfig.Timeout != nil {
		pr.Spec.Timeouts = &tektonv1.TimeoutFields{
			Pipeline: wfe.Spec.ExecutionConfig.Timeout,
		}
	}

	return pr
}

// truncateLabel truncates a string to maxLen characters for K8s label compliance
func truncateLabel(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// ============================================================================
// Status Management
// ============================================================================

// markFailed transitions the WorkflowExecution to Failed phase
func (r *WorkflowExecutionReconciler) markFailed(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, reason, message string, wasExecutionFailure bool) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseFailed
	wfe.Status.CompletionTime = &now
	wfe.Status.FailureReason = reason

	// Calculate duration if we have start time
	if wfe.Status.StartTime != nil {
		duration := now.Sub(wfe.Status.StartTime.Time)
		wfe.Status.Duration = duration.String()
	}

	wfe.Status.FailureDetails = &workflowexecutionv1alpha1.FailureDetails{
		Reason:                     reason,
		Message:                    message,
		FailedAt:                   now,
		WasExecutionFailure:        wasExecutionFailure,
		NaturalLanguageSummary:     fmt.Sprintf("Workflow failed with %s: %s", reason, message),
		ExecutionTimeBeforeFailure: wfe.Status.Duration,
	}

	if err := r.Status().Update(ctx, wfe); err != nil {
		log.Error(err, "Failed to update status to Failed")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, "Warning", "Failed", message)
	RecordPhaseTransition(wfe.Namespace, workflowexecutionv1alpha1.PhaseFailed)

	// Record audit event (BR-WE-007)
	r.recordFailed(ctx, wfe)

	log.Info("WorkflowExecution failed", "reason", reason, "message", message)
	return ctrl.Result{RequeueAfter: r.CooldownPeriod}, nil
}

// markSkipped transitions the WorkflowExecution to Skipped phase due to resource lock
func (r *WorkflowExecutionReconciler) markSkipped(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, reason string, conflicting *workflowexecutionv1alpha1.ConflictingWorkflowRef) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
	wfe.Status.CompletionTime = &now

	wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
		Reason:              reason,
		Message:             fmt.Sprintf("Resource %s is busy with workflow %s", wfe.Spec.TargetResource, conflicting.Name),
		SkippedAt:           now,
		ConflictingWorkflow: conflicting,
	}

	if err := r.Status().Update(ctx, wfe); err != nil {
		log.Error(err, "Failed to update status to Skipped")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, "Normal", "Skipped",
		fmt.Sprintf("Skipped: %s (blocking workflow: %s)", reason, conflicting.Name))
	RecordSkip(wfe.Namespace, reason)

	// Record audit event (BR-WE-007)
	r.recordSkipped(ctx, wfe)

	log.Info("WorkflowExecution skipped", "reason", reason, "blockingWorkflow", conflicting.Name)
	return ctrl.Result{}, nil
}

// markSkippedRecentlyRemediated transitions to Skipped due to recent execution
func (r *WorkflowExecutionReconciler) markSkippedRecentlyRemediated(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, recent *workflowexecutionv1alpha1.WorkflowExecution, cooldownRemaining time.Duration) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseSkipped
	wfe.Status.CompletionTime = &now

	wfe.Status.SkipDetails = &workflowexecutionv1alpha1.SkipDetails{
		Reason:    workflowexecutionv1alpha1.SkipReasonRecentlyRemediated,
		Message:   fmt.Sprintf("Same workflow ran recently on target %s, cooldown remaining: %s", wfe.Spec.TargetResource, cooldownRemaining.String()),
		SkippedAt: now,
		RecentRemediation: &workflowexecutionv1alpha1.RecentRemediationRef{
			Name:              recent.Name,
			WorkflowID:        recent.Spec.WorkflowRef.WorkflowID,
			CompletedAt:       *recent.Status.CompletionTime,
			Outcome:           recent.Status.Phase,
			TargetResource:    recent.Spec.TargetResource,
			CooldownRemaining: cooldownRemaining.String(),
		},
	}

	if err := r.Status().Update(ctx, wfe); err != nil {
		log.Error(err, "Failed to update status to Skipped (recently remediated)")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, "Normal", "Skipped",
		fmt.Sprintf("Skipped: recently remediated by %s", recent.Name))
	RecordSkip(wfe.Namespace, workflowexecutionv1alpha1.SkipReasonRecentlyRemediated)

	// Record audit event (BR-WE-007)
	r.recordSkipped(ctx, wfe)

	log.Info("WorkflowExecution skipped (recently remediated)",
		"recentWorkflow", recent.Name, "cooldownRemaining", cooldownRemaining.String())
	return ctrl.Result{}, nil
}

// markCompleted transitions the WorkflowExecution to Completed phase
func (r *WorkflowExecutionReconciler) markCompleted(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseCompleted
	wfe.Status.CompletionTime = &now

	// Calculate duration
	if wfe.Status.StartTime != nil {
		duration := now.Sub(wfe.Status.StartTime.Time)
		wfe.Status.Duration = duration.String()
	}

	if err := r.Status().Update(ctx, wfe); err != nil {
		log.Error(err, "Failed to update status to Completed")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, "Normal", "Completed",
		fmt.Sprintf("Workflow completed successfully in %s", wfe.Status.Duration))
	RecordPhaseTransition(wfe.Namespace, workflowexecutionv1alpha1.PhaseCompleted)
	RecordDuration(wfe.Namespace, wfe.Spec.WorkflowRef.WorkflowID, workflowexecutionv1alpha1.PhaseCompleted, wfe.Status.StartTime.Time)

	// Record audit event (BR-WE-007)
	r.recordCompleted(ctx, wfe)

	log.Info("WorkflowExecution completed", "duration", wfe.Status.Duration)
	return ctrl.Result{RequeueAfter: r.CooldownPeriod}, nil
}

// ============================================================================
// PipelineRun Status Synchronization (BR-WE-003)
// ============================================================================

// syncPipelineRunStatus maps Tekton PipelineRun status to WorkflowExecution status
func (r *WorkflowExecutionReconciler) syncPipelineRunStatus(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, pr *tektonv1.PipelineRun) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the Succeeded condition
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond == nil {
		// No condition yet, requeue
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Update PipelineRun status summary
	wfe.Status.PipelineRunStatus = &workflowexecutionv1alpha1.PipelineRunStatusSummary{
		Status:  string(succeededCond.Status),
		Reason:  succeededCond.Reason,
		Message: succeededCond.Message,
	}

	// Map status
	switch succeededCond.Status {
	case corev1.ConditionTrue:
		// PipelineRun succeeded
		return r.markCompleted(ctx, wfe)

	case corev1.ConditionFalse:
		// PipelineRun failed - extract failure details
		failureDetails := r.extractFailureDetails(pr)
		return r.markFailedWithDetails(ctx, wfe, failureDetails)

	case corev1.ConditionUnknown:
		// Still running - update status and requeue
		if err := r.Status().Update(ctx, wfe); err != nil {
			log.Error(err, "Failed to update PipelineRun status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// extractFailureDetails extracts failure information from a failed PipelineRun
// BR-WE-004: Extract Failure Details
func (r *WorkflowExecutionReconciler) extractFailureDetails(pr *tektonv1.PipelineRun) *workflowexecutionv1alpha1.FailureDetails {
	now := metav1.Now()

	// Find the failed task
	var failedTaskName string
	var failedTaskIndex int
	var failedStepName string
	var exitCode *int32

	// Look through child statuses to find failed task
	for i, taskRunStatus := range pr.Status.ChildReferences {
		if taskRunStatus.PipelineTaskName != "" {
			// Check if this task failed (simplified check)
			failedTaskName = taskRunStatus.PipelineTaskName
			failedTaskIndex = i
			break
		}
	}

	// Map Tekton reason to K8s-style reason
	reason := mapTektonReasonToK8s(pr.Status.GetCondition(apis.ConditionSucceeded).Reason)

	// Calculate execution time before failure
	var executionTime string
	if pr.Status.StartTime != nil {
		duration := now.Sub(pr.Status.StartTime.Time)
		executionTime = duration.String()
	}

	// Generate natural language summary
	summary := generateNaturalLanguageSummary(failedTaskName, failedTaskIndex, reason,
		pr.Status.GetCondition(apis.ConditionSucceeded).Message, executionTime)

	return &workflowexecutionv1alpha1.FailureDetails{
		FailedTaskIndex:            failedTaskIndex,
		FailedTaskName:             failedTaskName,
		FailedStepName:             failedStepName,
		Reason:                     reason,
		Message:                    pr.Status.GetCondition(apis.ConditionSucceeded).Message,
		ExitCode:                   exitCode,
		FailedAt:                   now,
		ExecutionTimeBeforeFailure: executionTime,
		NaturalLanguageSummary:     summary,
		WasExecutionFailure:        true,
	}
}

// markFailedWithDetails transitions to Failed with detailed failure info
func (r *WorkflowExecutionReconciler) markFailedWithDetails(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, details *workflowexecutionv1alpha1.FailureDetails) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	now := metav1.Now()
	wfe.Status.Phase = workflowexecutionv1alpha1.PhaseFailed
	wfe.Status.CompletionTime = &now
	wfe.Status.FailureReason = details.Reason
	wfe.Status.FailureDetails = details

	// Calculate duration
	if wfe.Status.StartTime != nil {
		duration := now.Sub(wfe.Status.StartTime.Time)
		wfe.Status.Duration = duration.String()
	}

	if err := r.Status().Update(ctx, wfe); err != nil {
		log.Error(err, "Failed to update status to Failed with details")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(wfe, "Warning", "Failed",
		fmt.Sprintf("Workflow failed at task '%s': %s", details.FailedTaskName, details.Reason))
	RecordPhaseTransition(wfe.Namespace, workflowexecutionv1alpha1.PhaseFailed)

	// Record audit event (BR-WE-007)
	r.recordFailed(ctx, wfe)

	log.Info("WorkflowExecution failed with details",
		"reason", details.Reason,
		"failedTask", details.FailedTaskName,
		"message", details.Message)
	return ctrl.Result{RequeueAfter: r.CooldownPeriod}, nil
}

// mapTektonReasonToK8s maps Tekton failure reasons to K8s-style reason codes
func mapTektonReasonToK8s(tektonReason string) string {
	switch tektonReason {
	case "TaskRunTimeout", "PipelineRunTimeout":
		return workflowexecutionv1alpha1.FailureReasonDeadlineExceeded
	case "TaskRunImagePullFailed":
		return workflowexecutionv1alpha1.FailureReasonImagePullBackOff
	case "InvalidTaskResultType", "InvalidParamValue":
		return workflowexecutionv1alpha1.FailureReasonConfigurationError
	case "CouldntGetTask", "CouldntGetPipeline":
		return workflowexecutionv1alpha1.FailureReasonConfigurationError
	default:
		return workflowexecutionv1alpha1.FailureReasonUnknown
	}
}

// generateNaturalLanguageSummary creates a human/LLM-readable failure description
func generateNaturalLanguageSummary(taskName string, taskIndex int, reason, message, executionTime string) string {
	if taskName == "" {
		taskName = "unknown"
	}

	return fmt.Sprintf(
		"Task '%s' (step %d) failed after %s with %s. %s. "+
			"This failure occurred during workflow execution.",
		taskName, taskIndex+1, executionTime, reason, message,
	)
}

