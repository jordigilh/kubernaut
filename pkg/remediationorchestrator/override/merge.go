/*
Copyright 2026 Jordi Gil.

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

// Package override implements operator workflow/parameter override resolution
// for the Remediation Orchestrator (Issue #594).
//
// The RO is the single decision point: it resolves the final workflow spec
// from either the RAR override (if present) or the AIA (default), then
// passes the result to the WE creator (unchanged). The WE has no
// awareness of overrides.
package override

import (
	"context"
	"fmt"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// overrideNotFoundError wraps an error to indicate the referenced RW was not found.
// R10: enables the reconciler to distinguish permanent (NotFound) from transient errors.
type overrideNotFoundError struct {
	workflowName string
	namespace    string
	cause        error
}

func (e *overrideNotFoundError) Error() string {
	return fmt.Sprintf("override workflow %q not found in namespace %q: %v", e.workflowName, e.namespace, e.cause)
}

func (e *overrideNotFoundError) Unwrap() error {
	return e.cause
}

// IsOverrideNotFoundError returns true if the error is a permanent override-not-found error.
// R10: The reconciler uses this to decide between transitionToFailed (permanent) and requeue (transient).
func IsOverrideNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*overrideNotFoundError)
	return ok
}

// ResolveWorkflow resolves the final SelectedWorkflow spec from the RAR override
// (if present) or the AIA (default).
//
// Returns:
//   - resolved: the final SelectedWorkflow to use for WE creation
//   - overrideApplied: true if the override was non-nil (caller should emit OperatorOverride event)
//   - err: non-nil on failure. IsOverrideNotFoundError(err)==true means permanent (RW deleted).
//
// G1: 3-return signature for conditional event emission.
// G4: Uses aiWorkflow.DeepCopy() to avoid mutating the original.
func ResolveWorkflow(
	ctx context.Context,
	reader client.Reader,
	overrideSpec *remediationv1.WorkflowOverride,
	aiWorkflow *aianalysisv1.SelectedWorkflow,
	namespace string,
) (*aianalysisv1.SelectedWorkflow, bool, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues(
		"originalWorkflowID", aiWorkflow.WorkflowID,
	)

	if overrideSpec == nil {
		return aiWorkflow, false, nil
	}

	logger = logger.WithValues(
		"override.workflowName", overrideSpec.WorkflowName,
		"override.hasParams", overrideSpec.Parameters != nil,
	)

	resolved := aiWorkflow.DeepCopy()

	if overrideSpec.WorkflowName != "" {
		rw := &remediationworkflowv1.RemediationWorkflow{}
		err := reader.Get(ctx, client.ObjectKey{Name: overrideSpec.WorkflowName, Namespace: namespace}, rw)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, false, &overrideNotFoundError{
					workflowName: overrideSpec.WorkflowName,
					namespace:    namespace,
					cause:        err,
				}
			}
			return nil, false, fmt.Errorf("failed to lookup RemediationWorkflow %q in namespace %q: %w",
				overrideSpec.WorkflowName, namespace, err)
		}

		// G2: Complete 11-field mapping from RW to SelectedWorkflow
		resolved.WorkflowID = rw.Status.WorkflowID
		resolved.ActionType = rw.Spec.ActionType
		resolved.Version = rw.Spec.Version
		resolved.ExecutionBundle = rw.Spec.Execution.Bundle
		resolved.ExecutionBundleDigest = rw.Spec.Execution.BundleDigest
		resolved.ExecutionEngine = rw.Spec.Execution.Engine
		resolved.EngineConfig = rw.Spec.Execution.EngineConfig
		resolved.ServiceAccountName = rw.Spec.Execution.ServiceAccountName
		// Confidence preserved from aiWorkflow (AI's assessment, not overridden)
	}

	if overrideSpec.Rationale != "" {
		resolved.Rationale = overrideSpec.Rationale
	}

	if overrideSpec.Parameters != nil {
		resolved.Parameters = overrideSpec.Parameters
	}

	logger.Info("Workflow override resolved",
		"resolvedWorkflowID", resolved.WorkflowID,
	)

	return resolved, true, nil
}
