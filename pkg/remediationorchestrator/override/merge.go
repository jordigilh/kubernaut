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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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

// NewOverrideNotFoundError creates a permanent override-not-found error for testing.
func NewOverrideNotFoundError(workflowName, namespace string, cause error) error {
	return &overrideNotFoundError{workflowName: workflowName, namespace: namespace, cause: cause}
}

// IsOverrideNotFoundError returns true if the error is a permanent override-not-found error.
// R10: The reconciler uses this to decide between transitionToFailed (permanent) and requeue (transient).
func IsOverrideNotFoundError(err error) bool {
	var target *overrideNotFoundError
	return errors.As(err, &target)
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
		return nil, false, fmt.Errorf("workflow switching is not permitted")
	}

	if overrideSpec.Rationale != "" {
		resolved.Rationale = overrideSpec.Rationale
	}
	if len(overrideSpec.Parameters) > 0 {
		evidence, err := mergeEvidence(resolved.Parameters, resolved.ParameterEvidence, overrideSpec.Parameters, overrideSpec.ParameterEvidence)
		if err != nil {
			return nil, false, fmt.Errorf("merge parameter evidence: %w", err)
		}
		resolved.ParameterEvidence = evidence
	}

	for name, value := range overrideSpec.Parameters {
		original, exists := resolved.Parameters[name]
		if !exists {
			return nil, false, fmt.Errorf("override parameter %q is absent from original parameters", name)
		}
		if strings.HasPrefix(name, "TARGET_RESOURCE_") && !reflect.DeepEqual(value, original) {
			return nil, false, fmt.Errorf("override parameter %q is locked", name)
		}
		resolved.Parameters[name] = value
	}

	logger.Info("Workflow override resolved",
		"resolvedWorkflowID", resolved.WorkflowID,
	)

	return resolved, true, nil
}

// mergeEvidence checks both submissions against the typed values before
// overlaying their raw tokens. JSON RawMessage retains numeric lexemes.
func mergeEvidence(original sharedtypes.TypedParameterMap, originalEvidence string, changes sharedtypes.TypedParameterMap, changesEvidence string) (string, error) {
	if strings.TrimSpace(originalEvidence) == "" {
		var err error
		originalEvidence, err = evidenceFromTypedParameters(original)
		if err != nil {
			return "", fmt.Errorf("derive original evidence: %w", err)
		}
	}
	if strings.TrimSpace(changesEvidence) == "" {
		var err error
		changesEvidence, err = evidenceFromTypedParameters(changes)
		if err != nil {
			return "", fmt.Errorf("derive override evidence: %w", err)
		}
	}
	originalValues, _, err := sharedtypes.DecodeTypedParameterMap([]byte(originalEvidence))
	if err != nil || !reflect.DeepEqual(originalValues, original) {
		return "", fmt.Errorf("original evidence does not match selected parameters")
	}
	changeValues, _, err := sharedtypes.DecodeTypedParameterMap([]byte(changesEvidence))
	if err != nil || !reflect.DeepEqual(changeValues, changes) {
		return "", fmt.Errorf("override evidence does not match submitted parameters")
	}
	var originalTokens, changeTokens map[string]json.RawMessage
	if err := json.Unmarshal([]byte(originalEvidence), &originalTokens); err != nil {
		return "", fmt.Errorf("decode original evidence: %w", err)
	}
	if err := json.Unmarshal([]byte(changesEvidence), &changeTokens); err != nil {
		return "", fmt.Errorf("decode override evidence: %w", err)
	}
	for name, value := range changeTokens {
		originalTokens[name] = value
	}
	encoded, err := json.Marshal(originalTokens)
	if err != nil {
		return "", fmt.Errorf("encode effective evidence: %w", err)
	}
	return string(encoded), nil
}

func evidenceFromTypedParameters(values sharedtypes.TypedParameterMap) (string, error) {
	jsonValues, err := sharedtypes.TypedParameterMapToJSONValues(values)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(jsonValues)
	if err != nil {
		return "", fmt.Errorf("encode typed parameters: %w", err)
	}
	return string(encoded), nil
}
