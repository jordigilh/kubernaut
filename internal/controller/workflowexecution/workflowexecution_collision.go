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

// Execution-resource-collision handling (AlreadyExists), split out of
// workflowexecution_controller.go per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 2
// (issue #1520) to keep the file under the 700-line convention threshold.
// Pure structural move — no behavior change.
package workflowexecution

import (
	"context"
	"fmt"
	"time"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
)

// handleJobAlreadyExists implements Issue #374 pre-execution cleanup for completed Jobs.
// When a Job creation fails with AlreadyExists, this method checks if the existing Job
// is in a terminal state. If so, it cleans up the stale Job and retries creation.
//
// Returns:
//
//	(createResult, true, false, "") — Job cleaned up and recreation succeeded.
//	(nil, false, true, "")          — Cleanup accepted but GC pending; caller should requeue.
//	(nil, false, false, name)       — Lock is valid (Job still running); 4th value is original WFE name from label (Issue #190).
//	(nil, false, false, "")         — Unrecoverable error or executor type mismatch.
func (r *WorkflowExecutionReconciler) handleJobAlreadyExists(
	ctx context.Context,
	exec weexecutor.Executor,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	resourceName string,
	createOpts weexecutor.CreateOptions,
) (*weexecutor.CreateResult, bool, bool, string) {
	logger := log.FromContext(ctx)

	jobExec, ok := exec.(*weexecutor.JobExecutor)
	if !ok {
		return nil, false, false, ""
	}

	completed, checkErr := jobExec.IsCompleted(ctx, wfe.Spec.ClusterID, wfe.Spec.TargetResource, r.ExecutionNamespace)
	if checkErr != nil {
		logger.V(1).Info("Could not check existing Job state (may have been deleted)", "error", checkErr)
		return nil, false, false, ""
	}
	if !completed {
		// Issue #190: Lock is valid (Job still running). Try to identify the
		// original WFE from the Job's label so the caller can classify this
		// as Deduplicated rather than Unknown.
		originalWFE := r.getOriginalWFEFromJob(ctx, resourceName)
		return nil, false, false, originalWFE
	}

	logger.Info("Stale completed Job detected, cleaning up before retry (Issue #374)",
		"resource", resourceName)

	cleanupWFE := &workflowexecutionv1alpha1.WorkflowExecution{
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			TargetResource: wfe.Spec.TargetResource,
		},
	}
	if cleanErr := jobExec.Cleanup(ctx, cleanupWFE, r.ExecutionNamespace); cleanErr != nil {
		logger.Error(cleanErr, "Failed to clean up completed Job", "resource", resourceName)
		return nil, false, false, ""
	}

	retryResult, retryErr := exec.Create(ctx, wfe, r.ExecutionNamespace, createOpts)
	if retryErr != nil {
		if apierrors.IsAlreadyExists(retryErr) {
			logger.Info("Job cleanup accepted but GC pending, will requeue (Issue #383)",
				"resource", resourceName)
			return nil, false, true, ""
		}
		logger.Error(retryErr, "Failed to create Job after stale cleanup", "resource", resourceName)
		return nil, false, false, ""
	}

	logger.Info("Successfully recreated Job after stale cleanup (Issue #374)",
		"resource", resourceName, "newJob", retryResult.ResourceName)
	return retryResult, true, false, ""
}

// getOriginalWFEFromJob fetches the existing Job by resource name and reads the
// kubernaut.ai/workflow-execution label. Returns empty string if the Job cannot
// be found or the label is absent (Issue #190).
func (r *WorkflowExecutionReconciler) getOriginalWFEFromJob(ctx context.Context, resourceName string) string {
	var job batchv1.Job
	if err := r.Get(ctx, client.ObjectKey{
		Name:      resourceName,
		Namespace: r.ExecutionNamespace,
	}, &job); err != nil {
		return ""
	}
	if job.Labels == nil {
		return ""
	}
	return job.Labels["kubernaut.ai/workflow-execution"]
}

// ========================================
// HandleAlreadyExists handles the race condition where PipelineRun already exists
// DD-WE-003: Layer 2 - Execution-time collision handling (not routing)
// V1.0: Fails WFE if race condition detected (RO should have prevented this)
// ========================================
func (r *WorkflowExecutionReconciler) HandleAlreadyExists(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, resourceName string, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	existingPR := &tektonv1.PipelineRun{}
	if getErr := r.Get(ctx, client.ObjectKey{
		Name:      resourceName,
		Namespace: r.ExecutionNamespace,
	}, existingPR); getErr != nil {
		logger.Error(getErr, "Failed to get existing PipelineRun", "name", resourceName)
		markErr := r.MarkFailedWithReason(ctx, wfe, "RaceConditionError", fmt.Sprintf("PipelineRun already exists but failed to verify ownership: %v", getErr))
		return ctrl.Result{}, markErr
	}

	if existingPR.Labels != nil &&
		existingPR.Labels["kubernaut.ai/workflow-execution"] == wfe.Name &&
		existingPR.Labels["kubernaut.ai/source-namespace"] == wfe.Namespace {
		logger.Info("PipelineRun already exists and is ours, continuing", "name", resourceName)

		now := metav1.Now()
		if err := r.StatusManager.AtomicStatusUpdate(ctx, wfe, func() error {
			if err := r.PhaseManager.TransitionTo(wfe, wephase.Running); err != nil {
				return fmt.Errorf("failed to transition to Running in HandleAlreadyExists: %w", err)
			}

			wfe.Status.StartTime = &now
			wfe.Status.ExecutionRef = &corev1.LocalObjectReference{
				Name: resourceName,
			}

			weconditions.SetExecutionCreated(wfe, true,
				weconditions.ReasonExecutionCreated,
				fmt.Sprintf("PipelineRun %s already exists (race condition)", resourceName))

			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status in HandleAlreadyExists: %w", err)
		}

		r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonPipelineRunCreated,
			fmt.Sprintf("PipelineRun %s/%s (already exists, ours)", r.ExecutionNamespace, resourceName))

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Issue #190: Another WFE created this PipelineRun — classify as Deduplicated
	// when the original WFE can be identified from labels.
	originalWFE := existingPR.Labels["kubernaut.ai/workflow-execution"]
	logger.Info("Execution-time collision: PipelineRun created by another WFE",
		"prName", resourceName,
		"existingWFE", originalWFE,
		"targetResource", wfe.Spec.TargetResource,
	)

	if originalWFE != "" {
		markErr := r.MarkFailedAsDeduplicated(ctx, wfe, originalWFE)
		return ctrl.Result{}, markErr
	}

	markErr := r.MarkFailedWithReason(ctx, wfe, "Unknown",
		fmt.Sprintf("PipelineRun '%s' already exists for target resource (owner label missing)", resourceName))
	return ctrl.Result{}, markErr
}
