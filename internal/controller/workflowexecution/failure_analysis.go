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

// Package workflowexecution provides failure analysis for Tekton PipelineRun failures.
//
// This file implements BR-WE-012 (Exponential Backoff) by detecting and categorizing
// failures to determine appropriate retry strategies.
//
// Failure Analysis:
//   - Pre-Execution Failures: Configuration errors, permission issues, image pull failures
//     → Apply exponential backoff (DD-WE-004)
//   - Execution Failures: Task-level failures during PipelineRun execution
//     → Report to user, no automatic retry
//
// Failure Categories:
// - OOMKilled: Container out of memory
// - DeadlineExceeded: Timeout reached
// - Forbidden: Permission denied
// - ImagePullBackOff: Container image not available
// - ConfigurationError: Invalid workflow configuration
// - TaskFailed: Workflow task failed during execution
//
// See: docs/architecture/decisions/DD-WE-004-exponential-backoff.md
package workflowexecution

import (
	"context"
	"fmt"
	"strings"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// ========================================
// Day 7: TaskRun-Specific Failure Details
// Plan v3.4: Extract FailedTaskName, FailedTaskIndex, ExitCode
// ========================================

// FindFailedTaskRun finds the first failed TaskRun in a PipelineRun's ChildReferences
// Returns the TaskRun, its index in ChildReferences, and any error
// Returns (nil, -1, nil) if no failed TaskRun is found
func (r *WorkflowExecutionReconciler) FindFailedTaskRun(ctx context.Context, pr *tektonv1.PipelineRun) (*tektonv1.TaskRun, int, error) {
	logger := log.FromContext(ctx)

	for i, ref := range pr.Status.ChildReferences {
		// Skip non-TaskRun references (e.g., Run, CustomRun)
		if ref.Kind != "TaskRun" {
			continue
		}

		// Fetch the TaskRun
		var tr tektonv1.TaskRun
		if err := r.Get(ctx, client.ObjectKey{
			Name:      ref.Name,
			Namespace: pr.Namespace,
		}, &tr); err != nil {
			if apierrors.IsNotFound(err) {
				// TaskRun may have been garbage collected - skip
				logger.V(1).Info("TaskRun not found, may be deleted", "taskRun", ref.Name)
				continue
			}
			// Other errors - log and continue
			logger.Error(err, "Failed to get TaskRun", "taskRun", ref.Name)
			continue
		}

		// Check if this TaskRun failed
		cond := tr.Status.GetCondition(apis.ConditionSucceeded)
		if cond != nil && cond.IsFalse() {
			logger.V(1).Info("Found failed TaskRun",
				"taskRun", tr.Name,
				"index", i,
				"reason", cond.Reason,
			)
			return &tr, i, nil
		}
	}

	// No failed TaskRun found
	return nil, -1, nil
}

// ExtractFailureDetails extracts structured failure information from PipelineRun
// Day 7: Now includes TaskRun-specific fields (FailedTaskName, FailedTaskIndex, ExitCode)
// Day 6 Extension (BR-WE-012): Includes WasExecutionFailure for backoff decisions
// Maps Tekton failure reasons to our FailureReason enum
func (r *WorkflowExecutionReconciler) ExtractFailureDetails(ctx context.Context, pr *tektonv1.PipelineRun, startTime *metav1.Time) *workflowexecutionv1alpha1.FailureDetails {
	details := &workflowexecutionv1alpha1.FailureDetails{
		FailedAt:            metav1.Now(),
		Reason:              workflowexecutionv1alpha1.FailureReasonUnknown,
		WasExecutionFailure: false, // Default: pre-execution failure
	}

	// Calculate execution time before failure
	if startTime != nil {
		duration := time.Since(startTime.Time)
		details.ExecutionTimeBeforeFailure = duration.Round(time.Second).String()
	}

	// Handle nil PipelineRun (deleted externally)
	if pr == nil {
		details.Message = "PipelineRun was deleted externally"
		return details
	}

	// Get failed condition from PipelineRun
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		details.Message = succeededCond.Message

		// Map Tekton reasons to our enum
		details.Reason = r.mapTektonReasonToFailureReason(succeededCond.Reason, succeededCond.Message)
	}

	// ========================================
	// Day 7: Extract TaskRun-specific fields
	// ========================================
	failedTaskRun, index, err := r.FindFailedTaskRun(ctx, pr)
	if err == nil && failedTaskRun != nil {
		details.FailedTaskName = failedTaskRun.Name
		details.FailedTaskIndex = index

		// Extract exit code from container state (if available)
		details.ExitCode = r.extractExitCode(failedTaskRun)
	}

	// ========================================
	// Day 6 Extension (BR-WE-012): Determine WasExecutionFailure
	// DD-WE-004: Critical for backoff decisions
	//
	// WasExecutionFailure = true if:
	//   - PipelineRun has StartTime (execution began)
	//   - OR PipelineRun has any TaskRun (tasks were created)
	//   - OR failure reason indicates execution started (TaskFailed, OOMKilled, etc.)
	//
	// WasExecutionFailure = false if:
	//   - Failure is clearly pre-execution (ImagePullBackOff, ConfigurationError)
	//   - PipelineRun never started
	// ========================================
	details.WasExecutionFailure = r.determineWasExecutionFailure(pr, details.Reason)

	return details
}

// determineWasExecutionFailure checks if failure occurred during execution or before
// DD-WE-004: Critical for determining retry behavior
func (r *WorkflowExecutionReconciler) determineWasExecutionFailure(pr *tektonv1.PipelineRun, failureReason string) bool {
	if pr == nil {
		return false // Can't determine, assume pre-execution
	}

	// If PipelineRun has StartTime, execution began
	if pr.Status.StartTime != nil {
		// But check for pre-execution failure reasons even after start
		switch failureReason {
		case workflowexecutionv1alpha1.FailureReasonImagePullBackOff,
			workflowexecutionv1alpha1.FailureReasonConfigurationError,
			workflowexecutionv1alpha1.FailureReasonResourceExhausted:
			// These are typically pre-execution even if PipelineRun started
			return false
		default:
			// PipelineRun started and failed with a non-pre-execution reason
			return true
		}
	}

	// If there are TaskRuns in ChildReferences, tasks were created
	if len(pr.Status.ChildReferences) > 0 {
		return true
	}

	// Check for specific execution failure reasons
	switch failureReason {
	case workflowexecutionv1alpha1.FailureReasonOOMKilled,
		workflowexecutionv1alpha1.FailureReasonDeadlineExceeded,
		workflowexecutionv1alpha1.FailureReasonForbidden,
		workflowexecutionv1alpha1.FailureReasonTaskFailed:
		// These indicate execution started
		return true
	}

	// Default: assume pre-execution failure
	return false
}

// extractExitCode extracts the exit code from a failed TaskRun's step states
func (r *WorkflowExecutionReconciler) extractExitCode(tr *tektonv1.TaskRun) *int32 {
	if tr == nil {
		return nil
	}

	// Check step states for terminated containers with exit codes
	for _, step := range tr.Status.Steps {
		if step.Terminated != nil && step.Terminated.ExitCode != 0 {
			exitCode := step.Terminated.ExitCode
			return &exitCode
		}
	}

	return nil
}

// mapTektonReasonToFailureReason converts Tekton/K8s reasons to our FailureReason enum
func (r *WorkflowExecutionReconciler) mapTektonReasonToFailureReason(reason, message string) string {
	messageLower := strings.ToLower(message)
	reasonLower := strings.ToLower(reason)

	switch {
	// IMPORTANT: Check specific failure types BEFORE generic TaskFailed
	// Otherwise "task failed due to oom" would match TaskFailed instead of OOMKilled

	// OOMKilled - check before TaskFailed to avoid false matches
	case strings.Contains(messageLower, "oomkilled") || strings.Contains(messageLower, "oom"):
		return workflowexecutionv1alpha1.FailureReasonOOMKilled

	// Timeout/Deadline - check before TaskFailed
	case strings.Contains(reasonLower, "timeout") || strings.Contains(messageLower, "timeout") ||
		strings.Contains(messageLower, "deadline"):
		return workflowexecutionv1alpha1.FailureReasonDeadlineExceeded

	// RBAC/Permission errors - check before TaskFailed
	case strings.Contains(messageLower, "forbidden") || strings.Contains(messageLower, "rbac") ||
		strings.Contains(messageLower, "permission denied"):
		return workflowexecutionv1alpha1.FailureReasonForbidden

	// Resource exhaustion - check before TaskFailed
	case strings.Contains(messageLower, "quota") || strings.Contains(messageLower, "resource exhausted"):
		return workflowexecutionv1alpha1.FailureReasonResourceExhausted

	// Image pull failures - check before TaskFailed
	case strings.Contains(messageLower, "imagepullbackoff") || strings.Contains(messageLower, "image pull"):
		return workflowexecutionv1alpha1.FailureReasonImagePullBackOff

	// Configuration errors - check before TaskFailed
	case strings.Contains(messageLower, "invalid") || strings.Contains(messageLower, "configuration"):
		return workflowexecutionv1alpha1.FailureReasonConfigurationError

	// Task failure - only if message explicitly mentions task failure
	// Note: Don't match on reason "TaskRunFailed" alone, as it's too generic
	// "TaskRunFailed" with no specific message indicators should fall through to Unknown
	case strings.Contains(reasonLower, "taskfailed") ||
		(strings.Contains(messageLower, "task") && strings.Contains(messageLower, "failed")):
		return workflowexecutionv1alpha1.FailureReasonTaskFailed

	// Unknown - no patterns matched (includes generic "TaskRunFailed" without specific message)
	default:
		return workflowexecutionv1alpha1.FailureReasonUnknown
	}
}

// GenerateNaturalLanguageSummary creates a human/LLM-readable failure description
// For failure reporting and user notifications
// Day 9 (v3.5): Handles nil FailureDetails gracefully per Q4 decision
func (r *WorkflowExecutionReconciler) GenerateNaturalLanguageSummary(wfe *workflowexecutionv1alpha1.WorkflowExecution, details *workflowexecutionv1alpha1.FailureDetails) string {
	var sb strings.Builder

	// Workflow identification
	sb.WriteString(fmt.Sprintf("Workflow '%s' failed on target '%s'.\n",
		wfe.Spec.WorkflowRef.WorkflowID,
		wfe.Spec.TargetResource))

	// Handle nil FailureDetails gracefully (Day 9 edge case)
	if details == nil {
		sb.WriteString("Reason: Unknown - No failure details available.\n")
		sb.WriteString("Recommendation: Check PipelineRun logs for detailed failure information.\n")
		return sb.String()
	}

	// Failure reason
	sb.WriteString(fmt.Sprintf("Reason: %s\n", details.Reason))

	// Error message
	if details.Message != "" {
		sb.WriteString(fmt.Sprintf("Error: %s\n", details.Message))
	}

	// Execution time
	if details.ExecutionTimeBeforeFailure != "" {
		sb.WriteString(fmt.Sprintf("Failed after: %s\n", details.ExecutionTimeBeforeFailure))
	}

	// Reason-specific recommendations
	switch details.Reason {
	case workflowexecutionv1alpha1.FailureReasonOOMKilled:
		sb.WriteString("Recommendation: The workflow task ran out of memory. Consider increasing task resource limits.\n")
	case workflowexecutionv1alpha1.FailureReasonForbidden:
		sb.WriteString("Recommendation: The service account lacks required RBAC permissions. Grant appropriate permissions or use an alternative workflow.\n")
	case workflowexecutionv1alpha1.FailureReasonDeadlineExceeded:
		sb.WriteString("Recommendation: The workflow exceeded its timeout. Consider increasing the timeout or using a faster workflow variant.\n")
	case workflowexecutionv1alpha1.FailureReasonImagePullBackOff:
		sb.WriteString("Recommendation: Unable to pull the workflow container image. Verify image exists and credentials are configured.\n")
	}

	return sb.String()
}
