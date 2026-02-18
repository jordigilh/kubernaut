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

package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// DefaultServiceAccountName is the default SA for PipelineRuns
const DefaultServiceAccountName = "kubernaut-workflow-runner"

// TektonExecutor implements the Executor interface for Tekton PipelineRuns.
// Extracted from WorkflowExecutionReconciler (BR-WE-014).
type TektonExecutor struct {
	Client             client.Client
	ServiceAccountName string
}

// NewTektonExecutor creates a new TektonExecutor.
func NewTektonExecutor(c client.Client, serviceAccountName string) *TektonExecutor {
	if serviceAccountName == "" {
		serviceAccountName = DefaultServiceAccountName
	}
	return &TektonExecutor{
		Client:             c,
		ServiceAccountName: serviceAccountName,
	}
}

// Engine returns "tekton".
func (t *TektonExecutor) Engine() string {
	return "tekton"
}

// Create builds and creates a Tekton PipelineRun in the execution namespace.
// Returns the name of the created PipelineRun.
//
// DD-WE-002: PipelineRuns created in dedicated execution namespace
// DD-WE-003: Deterministic name for atomic locking
func (t *TektonExecutor) Create(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) (string, error) {
	pr := t.buildPipelineRun(wfe, namespace)

	if err := t.Client.Create(ctx, pr); err != nil {
		return "", err // Preserve original error for IsAlreadyExists checks
	}
	return pr.Name, nil
}

// GetStatus retrieves the current status of the PipelineRun and maps it to ExecutionResult.
// Returns nil result with nil error if the PipelineRun is not found.
func (t *TektonExecutor) GetStatus(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) (*ExecutionResult, error) {
	if wfe.Status.ExecutionRef == nil {
		return nil, fmt.Errorf("no execution ref set on WFE %s/%s", wfe.Namespace, wfe.Name)
	}

	var pr tektonv1.PipelineRun
	if err := t.Client.Get(ctx, client.ObjectKey{
		Name:      wfe.Status.ExecutionRef.Name,
		Namespace: namespace,
	}, &pr); err != nil {
		return nil, err
	}

	// Build status summary
	summary := t.buildStatusSummary(&pr)

	// Map Tekton condition to execution result
	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		switch {
		case succeededCond.IsTrue():
			return &ExecutionResult{
				Phase:   workflowexecutionv1alpha1.PhaseCompleted,
				Reason:  succeededCond.Reason,
				Message: succeededCond.Message,
				Summary: summary,
			}, nil
		case succeededCond.IsFalse():
			return &ExecutionResult{
				Phase:   workflowexecutionv1alpha1.PhaseFailed,
				Reason:  succeededCond.Reason,
				Message: succeededCond.Message,
				Summary: summary,
			}, nil
		default:
			return &ExecutionResult{
				Phase:   workflowexecutionv1alpha1.PhaseRunning,
				Reason:  succeededCond.Reason,
				Message: fmt.Sprintf("Pipeline executing (%s)", succeededCond.Reason),
				Summary: summary,
			}, nil
		}
	}

	// No condition yet - PipelineRun just created
	return &ExecutionResult{
		Phase:   workflowexecutionv1alpha1.PhaseRunning,
		Reason:  "Pending",
		Message: "Pipeline created, waiting for Tekton to start execution",
		Summary: summary,
	}, nil
}

// Cleanup deletes the PipelineRun in the execution namespace.
// Returns nil if the PipelineRun doesn't exist (idempotent).
func (t *TektonExecutor) Cleanup(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) error {
	prName := ExecutionResourceName(wfe.Spec.TargetResource)
	pr := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prName,
			Namespace: namespace,
		},
	}

	if err := t.Client.Delete(ctx, pr); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil // Already gone
		}
		return fmt.Errorf("failed to delete PipelineRun %s/%s: %w", namespace, prName, err)
	}
	return nil
}

// buildPipelineRun creates a PipelineRun with bundle resolver.
// Extracted from WorkflowExecutionReconciler.BuildPipelineRun.
func (t *TektonExecutor) buildPipelineRun(wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) *tektonv1.PipelineRun {
	params := convertParameters(wfe.Spec.Parameters)

	// Add TARGET_RESOURCE parameter (required by all pipelines)
	params = append(params, tektonv1.Param{
		Name:  "TARGET_RESOURCE",
		Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.TargetResource},
	})

	return &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ExecutionResourceName(wfe.Spec.TargetResource),
			Namespace: namespace,
			Labels: map[string]string{
				"kubernaut.ai/workflow-execution": wfe.Name,
				"kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
				"kubernaut.ai/target-resource":    sanitizeLabelValue(wfe.Spec.TargetResource),
				"kubernaut.ai/source-namespace":   wfe.Namespace,
			},
			Annotations: map[string]string{
				"kubernaut.ai/target-resource": wfe.Spec.TargetResource,
			},
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
				ServiceAccountName: t.ServiceAccountName,
			},
		},
	}
}

// buildStatusSummary creates a lightweight status summary from a PipelineRun.
func (t *TektonExecutor) buildStatusSummary(pr *tektonv1.PipelineRun) *workflowexecutionv1alpha1.ExecutionStatusSummary {
	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status: "Unknown",
	}

	summary.TotalTasks = len(pr.Status.ChildReferences)

	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		summary.Status = string(succeededCond.Status)
		summary.Reason = succeededCond.Reason
		summary.Message = succeededCond.Message
	}

	return summary
}

// ========================================
// Shared utility functions
// ========================================

// ExecutionResourceName generates a deterministic name from targetResource.
// DD-WE-003: Lock Persistence via Deterministic Name
// Format: wfe-<sha256(targetResource)[:16]>
func ExecutionResourceName(targetResource string) string {
	h := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}

// convertParameters converts map[string]string to Tekton params.
func convertParameters(params map[string]string) []tektonv1.Param {
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

// sanitizeLabelValue makes a string safe for use as a Kubernetes label value.
func sanitizeLabelValue(s string) string {
	result := strings.ReplaceAll(s, "/", "__")
	if len(result) > 63 {
		result = result[:63]
	}
	return result
}
