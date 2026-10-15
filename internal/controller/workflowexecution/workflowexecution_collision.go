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

	"github.com/go-logr/logr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
)

// getOriginalWFEFromJob fetches the existing Job by resource name and reads the
// kubernaut.ai/workflow-execution label. Returns empty string if the Job cannot
// be found or the label is absent. It is used only for collision classification;
// labels are never used to authorize deletion or adoption (DD-WE-010).
func (r *WorkflowExecutionReconciler) getOriginalWFEFromJob(ctx context.Context, clusterID, resourceName string) string {
	executionClient, err := r.executionClientFor(ctx, clusterID)
	if err != nil {
		return ""
	}
	var job batchv1.Job
	job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))
	if err := executionClient.Get(ctx, client.ObjectKey{
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

	executionClient, clientErr := r.executionClientFor(ctx, wfe.Spec.ClusterID)
	if clientErr != nil {
		logger.Error(clientErr, "Failed to get execution client for existing PipelineRun", "clusterID", wfe.Spec.ClusterID)
		markErr := r.MarkFailedWithReason(ctx, wfe, "RaceConditionError", fmt.Sprintf("PipelineRun already exists but execution client could not verify ownership: %v", clientErr))
		return ctrl.Result{}, markErr
	}
	existingPR := &tektonv1.PipelineRun{}
	existingPR.SetGroupVersionKind(tektonv1.SchemeGroupVersion.WithKind("PipelineRun"))
	if getErr := executionClient.Get(ctx, client.ObjectKey{
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
		if r.AuditManager == nil {
			return ctrl.Result{}, fmt.Errorf("audit manager is required before adopting existing PipelineRun %s", resourceName)
		}
		if auditErr := r.AuditManager.RecordExecutionWorkflowStarted(ctx, wfe, resourceName, r.ExecutionNamespace); auditErr != nil {
			return ctrl.Result{}, fmt.Errorf("record mandatory execution.workflow.started audit event for existing PipelineRun: %w", auditErr)
		}

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
			weconditions.SetAuditRecorded(wfe, true,
				weconditions.ReasonAuditSucceeded,
				"Audit event execution.workflow.started recorded to DataStorage")

			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update status in HandleAlreadyExists: %w", err)
		}

		r.Recorder.Event(wfe, corev1.EventTypeNormal, events.EventReasonPipelineRunCreated,
			fmt.Sprintf("PipelineRun %s/%s (already exists, ours)", r.ExecutionNamespace, resourceName))

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if r.RetainFailedExecutions {
		result, replaced, replaceErr := r.replaceFailedPipelineRun(ctx, wfe, resourceName, existingPR, logger)
		if replaceErr != nil {
			return ctrl.Result{}, replaceErr
		}
		if replaced {
			return result, nil
		}
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

func (r *WorkflowExecutionReconciler) replaceFailedPipelineRun(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, resourceName string, existingPR *tektonv1.PipelineRun, logger logr.Logger) (ctrl.Result, bool, error) {
	if !pipelineRunFailed(existingPR) {
		return ctrl.Result{}, false, nil
	}
	ownerName := existingPR.Labels["kubernaut.ai/workflow-execution"]
	ownerNamespace := existingPR.Labels["kubernaut.ai/source-namespace"]
	if ownerName == "" || ownerNamespace == "" {
		return ctrl.Result{}, false, nil
	}
	var ownerWFE workflowexecutionv1alpha1.WorkflowExecution
	if err := r.Get(ctx, client.ObjectKey{Name: ownerName, Namespace: ownerNamespace}, &ownerWFE); err != nil {
		return ctrl.Result{}, false, fmt.Errorf("get owner WorkflowExecution %s/%s: %w", ownerNamespace, ownerName, err)
	}
	if ownerWFE.Status.Phase != workflowexecutionv1alpha1.PhaseFailed {
		return ctrl.Result{}, false, nil
	}

	logger.Info("Replacing terminal retained PipelineRun", "name", resourceName, "owner", ownerNamespace+"/"+ownerName)
	executionClient, clientErr := r.executionClientFor(ctx, wfe.Spec.ClusterID)
	if clientErr != nil {
		return ctrl.Result{}, false, fmt.Errorf("get execution client for terminal PipelineRun replacement: %w", clientErr)
	}
	if deleteErr := executionClient.Delete(ctx, existingPR); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
		return ctrl.Result{}, false, fmt.Errorf("delete terminal retained PipelineRun: %w", deleteErr)
	}
	if r.ExecutorRegistry == nil {
		return ctrl.Result{}, false, fmt.Errorf("executor registry is required to replace terminal retained PipelineRun")
	}
	exec, execErr := r.ExecutorRegistry.Get(wfe.Spec.WorkflowRef.ExecutionEngine)
	if execErr != nil {
		return ctrl.Result{}, false, fmt.Errorf("get executor for terminal PipelineRun replacement: %w", execErr)
	}
	if _, createErr := exec.Create(ctx, wfe, r.ExecutionNamespace, weexecutor.CreateOptions{
		Dependencies:           convertWorkflowDependencies(wfe.Spec.WorkflowRef.Dependencies),
		DeclaredParameterNames: wfe.Spec.WorkflowRef.DeclaredParameterNames,
		RetainFailedExecutions: r.RetainFailedExecutions,
	}); createErr != nil {
		if apierrors.IsAlreadyExists(createErr) {
			return ctrl.Result{RequeueAfter: time.Second}, true, nil
		}
		return ctrl.Result{}, false, fmt.Errorf("create replacement PipelineRun: %w", createErr)
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, true, nil
}

func pipelineRunFailed(pr *tektonv1.PipelineRun) bool {
	for _, condition := range pr.Status.Conditions {
		if condition.Type == apis.ConditionSucceeded && condition.Status == corev1.ConditionFalse {
			return true
		}
	}
	return false
}
