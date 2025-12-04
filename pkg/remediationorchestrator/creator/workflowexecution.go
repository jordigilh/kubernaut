// Package creator provides child CRD creation logic for the Remediation Orchestrator.
//
// Business Requirements:
// - BR-ORCH-025: Workflow data pass-through to child CRDs
// - BR-ORCH-031: Cascade deletion via owner references
// - BR-ORCH-032: Resource lock deduplication support
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
)

// WorkflowExecutionCreator creates WorkflowExecution CRDs from RemediationRequests.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-032 (resource lock support)
type WorkflowExecutionCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewWorkflowExecutionCreator creates a new WorkflowExecutionCreator.
func NewWorkflowExecutionCreator(c client.Client, s *runtime.Scheme) *WorkflowExecutionCreator {
	return &WorkflowExecutionCreator{
		client: c,
		scheme: s,
	}
}

// Create creates a WorkflowExecution CRD for the given RemediationRequest.
// It uses the selected workflow from the completed AIAnalysis CRD.
// It is idempotent - if the CRD already exists, it returns the existing name.
// Reference: BR-ORCH-025 (data pass-through), BR-ORCH-031 (cascade deletion)
func (c *WorkflowExecutionCreator) Create(
	ctx context.Context,
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
) (string, error) {
	logger := log.FromContext(ctx).WithValues(
		"remediationRequest", rr.Name,
		"namespace", rr.Namespace,
	)

	// Validate AIAnalysis has selected workflow
	if ai.Status.SelectedWorkflow == nil {
		return "", fmt.Errorf("AIAnalysis %s has no selected workflow", ai.Name)
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
		return "", fmt.Errorf("failed to check existing WorkflowExecution: %w", err)
	}

	// Build WorkflowExecution from RemediationRequest and AIAnalysis
	we := c.buildWorkflowExecution(rr, ai, name)

	// Set owner reference for cascade deletion (BR-ORCH-031)
	if err := controllerutil.SetControllerReference(rr, we, c.scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the CRD
	if err := c.client.Create(ctx, we); err != nil {
		logger.Error(err, "Failed to create WorkflowExecution CRD")
		return "", fmt.Errorf("failed to create WorkflowExecution: %w", err)
	}

	logger.Info("Created WorkflowExecution CRD", "name", name,
		"workflowId", ai.Status.SelectedWorkflow.WorkflowID,
		"confidence", ai.Status.SelectedWorkflow.Confidence)
	return name, nil
}

// buildWorkflowExecution constructs the WorkflowExecution CRD from RemediationRequest and AIAnalysis.
func (c *WorkflowExecutionCreator) buildWorkflowExecution(
	rr *remediationv1.RemediationRequest,
	ai *aianalysisv1.AIAnalysis,
	name string,
) *workflowexecutionv1.WorkflowExecution {
	sw := ai.Status.SelectedWorkflow

	return &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rr.Namespace,
			Labels: map[string]string{
				"kubernaut.ai/remediation-request": rr.Name,
				"kubernaut.ai/component":           "workflow-execution",
				"kubernaut.ai/workflow-id":         sw.WorkflowID,
			},
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			// Parent reference for audit trail
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       rr.Name,
				Namespace:  rr.Namespace,
				UID:        rr.UID,
			},

			// Workflow reference from AIAnalysis (DD-CONTRACT-001)
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				WorkflowID:      sw.WorkflowID,
				Version:         sw.Version,
				ContainerImage:  sw.ContainerImage,
				ContainerDigest: sw.ContainerDigest,
			},

			// Target resource for resource locking (DD-WE-001)
			// Format: "namespace/kind/name" for namespaced, "kind/name" for cluster-scoped
			TargetResource: c.buildTargetResourceString(rr),

			// Parameters from LLM selection (DD-WORKFLOW-003)
			Parameters: sw.Parameters,

			// Confidence and rationale for audit trail
			Confidence: sw.Confidence,
			Rationale:  sw.Rationale,

			// Execution configuration (default values)
			ExecutionConfig: workflowexecutionv1.ExecutionConfig{
				ServiceAccountName: "kubernaut-workflow-runner",
			},
		},
	}
}

// buildTargetResourceString creates the target resource identifier string.
// Format: "namespace/kind/name" for namespaced resources
// Format: "kind/name" for cluster-scoped resources
func (c *WorkflowExecutionCreator) buildTargetResourceString(rr *remediationv1.RemediationRequest) string {
	tr := rr.Spec.TargetResource

	// Handle cluster-scoped resources (empty namespace)
	if tr.Namespace == "" {
		return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
	}

	// Namespaced resources
	return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
}
