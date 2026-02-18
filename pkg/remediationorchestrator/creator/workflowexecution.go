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

// Package creator provides CRD creator components for the Remediation Orchestrator.
package creator

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	rrconditions "github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

// WorkflowExecutionCreator creates WorkflowExecution CRDs from RemediationRequests.
type WorkflowExecutionCreator struct {
	client  client.Client
	scheme  *runtime.Scheme
	metrics *metrics.Metrics // DD-METRICS-001: Dependency-injected metrics
}

// NewWorkflowExecutionCreator creates a new WorkflowExecutionCreator.
func NewWorkflowExecutionCreator(c client.Client, s *runtime.Scheme, m *metrics.Metrics) *WorkflowExecutionCreator {
	return &WorkflowExecutionCreator{
		client:  c,
		scheme:  s,
		metrics: m,
	}
}

// Create creates a WorkflowExecution CRD for the given RemediationRequest.
// It uses the selected workflow from the completed AIAnalysis CRD.
// It is idempotent - if the CRD already exists, it returns the existing name.
// Reference: BR-ORCH-025 (workflow data pass-through), BR-ORCH-031 (cascade deletion)
func (c *WorkflowExecutionCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
		"aiAnalysis", ai.Name,
	)

	// Validate preconditions (BR-ORCH-025: "Missing selectedWorkflow → RR marked as Failed")
	if ai.Status.SelectedWorkflow == nil {
		return "", fmt.Errorf("AIAnalysis has no selectedWorkflow")
	}
	if ai.Status.SelectedWorkflow.WorkflowID == "" {
		return "", fmt.Errorf("selectedWorkflow.workflowId is required")
	}
	if ai.Status.SelectedWorkflow.ContainerImage == "" {
		return "", fmt.Errorf("selectedWorkflow.containerImage is required")
	}

	// Generate deterministic name
	name := fmt.Sprintf("we-%s", rr.Name)

	// Check if already exists (idempotency)
	existing := &workflowexecutionv1.WorkflowExecution{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, existing)
	if err == nil {
		logger.Info("WorkflowExecution already exists, reusing", "name", name)
		return name, nil
	}
	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to check existing WorkflowExecution")
		return "", fmt.Errorf("failed to check existing WorkflowExecution: %w", err)
	}

	// Build WorkflowExecution CRD
	// BR-ORCH-025: Pass-through from AIAnalysis.Status.SelectedWorkflow
	we := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			// Issue #91: labels removed; parent tracked via spec.remediationRequestRef + ownerRef

		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			// Parent reference for audit trail (REQUIRED)
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},
			// WorkflowRef: Direct pass-through from AIAnalysis (BR-ORCH-025)
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				WorkflowID:      ai.Status.SelectedWorkflow.WorkflowID,
				Version:         ai.Status.SelectedWorkflow.Version,
				ContainerImage:  ai.Status.SelectedWorkflow.ContainerImage,
				ContainerDigest: ai.Status.SelectedWorkflow.ContainerDigest,
			},
			// TargetResource: String format "namespace/kind/name" (per API contract)
			// BR-HAPI-191: Prefer LLM-identified AffectedResource (e.g., Deployment)
			// over the RR's TargetResource (e.g., Pod) when available.
			// The LLM often identifies the correct higher-level resource to patch.
			TargetResource: resolveTargetResource(rr, ai),
			// Parameters: Direct pass-through from AIAnalysis
			Parameters: ai.Status.SelectedWorkflow.Parameters,
			// Audit fields from AIAnalysis
			Confidence: ai.Status.SelectedWorkflow.Confidence,
			Rationale:  ai.Status.SelectedWorkflow.Rationale,
			// BR-WE-014: Execution backend engine from AIAnalysis workflow recommendation.
			// Defaults to "tekton" for backwards compatibility when not specified by HAPI.
			ExecutionEngine: executionEngineWithDefault(ai.Status.SelectedWorkflow.ExecutionEngine),
			// ExecutionConfig: Optional timeout from RemediationRequest
			ExecutionConfig: c.buildExecutionConfig(rr),
		},
	}

	// Validate RemediationRequest has required metadata for owner reference (defensive programming)
	// Gap 2.1: Prevents orphaned child CRDs if RR not properly persisted
	if rr.UID == "" {
		logger.Error(nil, "RemediationRequest has empty UID, cannot set owner reference")
		return "", fmt.Errorf("failed to set owner reference: RemediationRequest UID is required but empty")
	}

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, we, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, we); err != nil {
		logger.Error(err, "Failed to create WorkflowExecution CRD")
		// DD-CRD-002-RR: Set WorkflowExecutionReady=False on creation failure
		rrconditions.SetWorkflowExecutionReady(rr, false, fmt.Sprintf("Failed to create WorkflowExecution: %v", err), c.metrics)
		return "", fmt.Errorf("failed to create WorkflowExecution: %w", err)
	}

	// DD-CRD-002-RR: Set WorkflowExecutionReady=True on successful creation
	// Note: Reconciler will handle Status().Update() after this call
	rrconditions.SetWorkflowExecutionReady(rr, true, fmt.Sprintf("WorkflowExecution CRD %s created successfully", name), c.metrics)

	logger.Info("Created WorkflowExecution CRD",
		"name", name,
		"workflowId", ai.Status.SelectedWorkflow.WorkflowID,
		"containerImage", ai.Status.SelectedWorkflow.ContainerImage,
		"targetResource", we.Spec.TargetResource,
	)
	return name, nil
}

// BuildTargetResourceString builds the target resource string for WorkflowExecution.
// Format: "namespace/kind/name" for namespaced resources
//
//	"kind/name" for cluster-scoped resources (e.g., Node)
//
// This format is used by WorkflowExecution for resource locking (DD-WE-001).
func BuildTargetResourceString(rr *remediationv1.RemediationRequest) string {
	tr := rr.Spec.TargetResource
	if tr.Namespace != "" {
		return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
	}
	return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}

// resolveTargetResource determines the target resource string for the WorkflowExecution.
// BR-HAPI-191: Prefers the LLM-identified AffectedResource from RootCauseAnalysis when
// available, falling back to the RemediationRequest's TargetResource.
// The LLM may identify a higher-level owner resource (e.g., Deployment) rather than the
// Pod that generated the signal, which is the correct target for patching operations.
func resolveTargetResource(rr *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) string {
	// Prefer AffectedResource from RCA if available
	if ai.Status.RootCauseAnalysis != nil && ai.Status.RootCauseAnalysis.AffectedResource != nil {
		ar := ai.Status.RootCauseAnalysis.AffectedResource
		if ar.Kind != "" && ar.Name != "" {
			if ar.Namespace != "" {
				return fmt.Sprintf("%s/%s/%s", ar.Namespace, ar.Kind, ar.Name)
			}
			return fmt.Sprintf("%s/%s", ar.Kind, ar.Name)
		}
	}
	// Fall back to RR's target resource
	return BuildTargetResourceString(rr)
}

// executionEngineWithDefault returns the execution engine, defaulting to "tekton"
// when the value is empty (backwards compatibility for HAPI responses without the field).
func executionEngineWithDefault(engine string) string {
	if engine == "" {
		return "tekton"
	}
	return engine
}

// buildExecutionConfig builds ExecutionConfig from RemediationRequest timeouts.
func (c *WorkflowExecutionCreator) buildExecutionConfig(rr *remediationv1.RemediationRequest) *workflowexecutionv1.ExecutionConfig {
	// Use custom timeout if specified in RemediationRequest (BR-ORCH-028)
	if rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Executing != nil && rr.Status.TimeoutConfig.Executing.Duration > 0 {
		return &workflowexecutionv1.ExecutionConfig{
			Timeout: rr.Status.TimeoutConfig.Executing,
		}
	}
	// Return nil to use WorkflowExecution controller defaults
	return nil
}
