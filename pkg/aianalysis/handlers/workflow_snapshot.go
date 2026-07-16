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

package handlers

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// CRD-EMBEDDED EXECUTION SNAPSHOT EXTRACTION (Issue #1661 Change 11b)
// ========================================
// Authority: DD-WORKFLOW-018. KA already validated and placed Dependencies/
// Resources/DeclaredParameterNames on the wire selected_workflow map (Change
// 11a, Phase 37-39). These helpers decode that generic JSON into the typed
// AIAnalysis.Status.SelectedWorkflow fields so RO/WFE can trust the CRD
// snapshot instead of re-fetching the workflow from DataStorage.
// ========================================

// extractWorkflowDependencies decodes swMap["dependencies"] (a nested JSON
// object matching sharedtypes.WorkflowDependencies' shape) into a typed
// pointer. Returns nil if the key is absent or malformed (fail-closed: no
// unvalidated dependency data reaches the CRD).
func extractWorkflowDependencies(swMap map[string]interface{}) *sharedtypes.WorkflowDependencies {
	raw, ok := swMap["dependencies"]
	if !ok || raw == nil {
		return nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var deps sharedtypes.WorkflowDependencies
	if err := json.Unmarshal(bytes, &deps); err != nil {
		return nil
	}
	return &deps
}

// extractResourceRequirements decodes swMap["resources"] (a nested JSON
// object matching corev1.ResourceRequirements' shape) into a typed pointer.
// Returns nil if the key is absent or malformed.
func extractResourceRequirements(swMap map[string]interface{}) *corev1.ResourceRequirements {
	raw, ok := swMap["resources"]
	if !ok || raw == nil {
		return nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var res corev1.ResourceRequirements
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil
	}
	return &res
}

// extractDeclaredParameterNames decodes swMap["declared_parameter_names"]
// into a map[string]bool allowlist. Returns nil if the key is absent or
// malformed — distinct from an empty, non-nil map (schema declares zero
// parameters), matching WorkflowMeta.DeclaredParameterNames' nil-means-
// unavailable semantics (#243, Change 11a).
func extractDeclaredParameterNames(swMap map[string]interface{}) map[string]bool {
	raw, ok := swMap["declared_parameter_names"]
	if !ok || raw == nil {
		return nil
	}
	rawMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	names := make(map[string]bool, len(rawMap))
	for k, v := range rawMap {
		if b, ok := v.(bool); ok {
			names[k] = b
		}
	}
	return names
}
