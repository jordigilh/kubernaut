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

package executor

import (
	"fmt"

	"github.com/go-logr/logr"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// targetResourceParameterName is reserved for the execution target emitted
// from WorkflowExecution.Spec.TargetResource. A same-named typed parameter may
// be stale after a future RemediationApprovalRequest target override.
const targetResourceParameterName = "TARGET_RESOURCE"

// FilterDeclaredParameters strips undeclared parameters from the given map.
// #243: Defense-in-depth — even if KA-side validation is bypassed,
// undeclared parameters (including hallucinated credentials) are blocked.
//
// Semantics:
//   - declared == nil  → no schema available, pass all params through (backward compat)
//   - declared != nil  → keep only params whose key is in the declared set
//
// Stripped parameter names are logged at Info level (values are omitted to avoid
// leaking secrets).
func FilterDeclaredParameters(params map[string]string, declared map[string]bool, logger logr.Logger) map[string]string {
	if declared == nil {
		return params
	}
	filtered := make(map[string]string, len(declared))
	for k, v := range params {
		if declared[k] {
			filtered[k] = v
		} else {
			logger.Info("Stripped undeclared parameter", "parameter", k)
		}
	}
	return filtered
}

// FilterDeclaredTypedParameters strips undeclared values without converting the
// typed contract to strings. The returned map is an ephemeral execution view;
// the WFE's persisted typed values remain unchanged.
func FilterDeclaredTypedParameters(params sharedtypes.TypedParameterMap, declared map[string]bool, logger logr.Logger) sharedtypes.TypedParameterMap {
	if declared == nil {
		return params
	}
	filtered := make(sharedtypes.TypedParameterMap, len(declared))
	for name, value := range params {
		if declared[name] {
			filtered[name] = value
			continue
		}
		logger.Info("Stripped undeclared parameter", "parameter", name)
	}
	return filtered
}

// FilterDeclaredTypedParameterStrings creates the string-valued view required
// by Job and Tekton. Non-string values use compact JSON/text representations;
// no type is inferred from a string.
func FilterDeclaredTypedParameterStrings(params sharedtypes.TypedParameterMap, declared map[string]bool, logger logr.Logger) (map[string]string, error) {
	filtered := FilterDeclaredTypedParameters(params, declared, logger)
	values, err := sharedtypes.TypedParameterMapToEngineValues(filtered)
	if err != nil {
		return nil, fmt.Errorf("convert filtered workflow parameters: %w", err)
	}
	return values, nil
}
