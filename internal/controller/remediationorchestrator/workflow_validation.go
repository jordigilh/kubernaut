package controller

import (
	"context"
	"errors"
	"fmt"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	"github.com/jordigilh/kubernaut/pkg/shared/workflowidentity"
)

type workflowVerificationError struct {
	err error
}

func (e *workflowVerificationError) Error() string {
	return e.err.Error()
}

func (e *workflowVerificationError) Unwrap() error {
	return e.err
}

func isWorkflowVerificationError(err error) bool {
	var target *workflowVerificationError
	return errors.As(err, &target)
}

// verifySelectedWorkflowForExecution is the RO live-catalog fence used before
// either approval or execution CRD creation. A missing namespace fails closed.
func (r *Reconciler) verifySelectedWorkflowForExecution(ctx context.Context, ai *aianalysisv1.AIAnalysis) error {
	if ai == nil {
		return fmt.Errorf("aianalysis is nil")
	}
	selected := ai.Status.GetRCAResult().SelectedWorkflow
	if selected == nil || selected.WorkflowID == "" {
		return fmt.Errorf("selected workflow identity is missing")
	}
	workflow, err := workflowidentity.GetVerified(ctx, r.apiReader, r.catalogNamespace, selected.WorkflowID)
	if err != nil {
		return &workflowVerificationError{err: fmt.Errorf("verify selected workflow %q: %w", selected.WorkflowID, err)}
	}
	if err := remediationapprovalrequest.ValidateTypedParametersAgainstWorkflow(workflow, selected.Parameters); err != nil {
		return &workflowVerificationError{err: fmt.Errorf("validate selected workflow parameters: %w", err)}
	}
	return nil
}
