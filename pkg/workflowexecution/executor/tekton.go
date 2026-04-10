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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// TektonExecutor implements the Executor interface for Tekton PipelineRuns.
// DD-WE-005 v2.0: SA is read from WFE spec at execution time, not from executor config.
type TektonExecutor struct {
	Client client.Client
}

// NewTektonExecutor creates a new TektonExecutor.
func NewTektonExecutor(c client.Client) *TektonExecutor {
	return &TektonExecutor{
		Client: c,
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
// DD-WE-006: opts.Dependencies are added as workspace bindings.
func (t *TektonExecutor) Create(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string, opts CreateOptions) (*CreateResult, error) {
	pr := t.BuildPipelineRun(ctx, wfe, namespace, opts)

	if err := t.Client.Create(ctx, pr); err != nil {
		return nil, err // Preserve original error for IsAlreadyExists checks
	}
	return &CreateResult{ResourceName: pr.Name}, nil
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
	summary := t.buildStatusSummary(ctx, &pr)

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
//
// Issue #383: Before deleting, verify the PipelineRun's
// kubernaut.ai/workflow-execution label matches this WFE's name to avoid
// destroying a newer WFE's execution resource that shares the deterministic name.
func (t *TektonExecutor) Cleanup(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string) error {
	prName := ExecutionResourceName(wfe.Spec.TargetResource)

	var existing tektonv1.PipelineRun
	if err := t.Client.Get(ctx, client.ObjectKey{Name: prName, Namespace: namespace}, &existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil // Already gone
		}
		return fmt.Errorf("failed to get PipelineRun %s/%s for ownership check: %w", namespace, prName, err)
	}

	if owner := existing.Labels["kubernaut.ai/workflow-execution"]; owner != wfe.Name {
		return nil // PipelineRun belongs to a different WFE; leave it alone
	}

	if err := t.Client.Delete(ctx, &existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		}
		return fmt.Errorf("failed to delete PipelineRun %s/%s: %w", namespace, prName, err)
	}
	return nil
}

// BuildPipelineRun creates a PipelineRun with bundle resolver.
// DD-WE-006: deps are added as workspace bindings when non-nil.
// #243: Parameters are filtered against DeclaredParameterNames before conversion.
func (t *TektonExecutor) BuildPipelineRun(ctx context.Context, wfe *workflowexecutionv1alpha1.WorkflowExecution, namespace string, opts CreateOptions) *tektonv1.PipelineRun {
	logger := log.FromContext(ctx).WithValues("wfe", wfe.Name, "workflowID", wfe.Spec.WorkflowRef.WorkflowID)
	filteredParams := FilterDeclaredParameters(wfe.Spec.Parameters, opts.DeclaredParameterNames, logger)
	params := convertParameters(filteredParams)

	params = append(params, tektonv1.Param{
		Name:  "TARGET_RESOURCE",
		Value: tektonv1.ParamValue{Type: tektonv1.ParamTypeString, StringVal: wfe.Spec.TargetResource},
	})

	workspaces := buildDependencyWorkspaces(opts.Dependencies)

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
			Params:     params,
			Workspaces: workspaces,
			TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
				ServiceAccountName: wfe.Status.ServiceAccountName,
			},
		},
	}
}

// buildDependencyWorkspaces creates Tekton workspace bindings for schema-declared
// dependencies (DD-WE-006). Workspace names are prefixed to avoid collisions.
func buildDependencyWorkspaces(deps *models.WorkflowDependencies) []tektonv1.WorkspaceBinding {
	if deps == nil {
		return nil
	}

	var workspaces []tektonv1.WorkspaceBinding

	for _, s := range deps.Secrets {
		workspaces = append(workspaces, tektonv1.WorkspaceBinding{
			Name:   "secret-" + s.Name,
			Secret: &corev1.SecretVolumeSource{SecretName: s.Name},
		})
	}

	for _, cm := range deps.ConfigMaps {
		workspaces = append(workspaces, tektonv1.WorkspaceBinding{
			Name: "configmap-" + cm.Name,
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cm.Name},
			},
		})
	}

	return workspaces
}

// buildStatusSummary creates a lightweight status summary from a PipelineRun.
func (t *TektonExecutor) buildStatusSummary(ctx context.Context, pr *tektonv1.PipelineRun) *workflowexecutionv1alpha1.ExecutionStatusSummary {
	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status: corev1.ConditionUnknown,
	}

	summary.TotalTasks = len(pr.Status.ChildReferences)

	// Count TaskRuns with ConditionSucceeded True (completed tasks)
	for _, ref := range pr.Status.ChildReferences {
		if ref.Kind != "TaskRun" {
			continue
		}
		var tr tektonv1.TaskRun
		if err := t.Client.Get(ctx, client.ObjectKey{
			Name:      ref.Name,
			Namespace: pr.Namespace,
		}, &tr); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			continue
		}
		cond := tr.Status.GetCondition(apis.ConditionSucceeded)
		if cond != nil && cond.IsTrue() {
			summary.CompletedTasks++
		}
	}

	succeededCond := pr.Status.GetCondition(apis.ConditionSucceeded)
	if succeededCond != nil {
		summary.Status = corev1.ConditionStatus(succeededCond.Status)
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
