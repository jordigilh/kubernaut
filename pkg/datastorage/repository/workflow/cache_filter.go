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

package workflow

import (
	"strings"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// IN-MEMORY GO FILTER/SCORING PREDICATES (Issue #1661 Change 6)
// ========================================
// Authority: DD-WORKFLOW-018 (etcd single source of truth). Replaces the SQL
// WHERE-clause/scoring builders in discovery.go/scoring.go for the Phase
// 28/29 cache-backed Step 1/2 discovery path (ListActions,
// ListWorkflowsByActionType). Step 3 (GetWorkflowWithContextFilters/GetByID)
// intentionally keeps using buildContextFilterSQL against Postgres until a
// later phase removes WorkflowExecution's GetWorkflowByID dependency.
//
// Each function's doc comment names the exact SQL clause it mirrors so a
// future reader can diff behavior without re-deriving it from scratch.
// ========================================

// matchesArrayLabel reports whether workflowValues contains filterValue
// (case-insensitive) or the wildcard "*". An empty filterValue means the
// dimension is unconstrained (always matches). Mirrors
// appendMandatoryLabelConditions' shared EXISTS/jsonb_array_elements_text/
// LOWER pattern used for severity/component/environment/cluster.
//
// BR-FLEET-003 R6: when filterValue is non-empty and workflowValues is empty
// (no entries, no wildcard), the result is false -- exclusion, not a
// pass-through. This is the same behavior EXISTS() has over an empty JSONB
// array.
func matchesArrayLabel(workflowValues []string, filterValue string) bool {
	if filterValue == "" {
		return true
	}
	for _, v := range workflowValues {
		if v == "*" || strings.EqualFold(v, filterValue) {
			return true
		}
	}
	return false
}

// matchesPriority reports whether workflowPriority matches filterValue
// (case-insensitive) or is the wildcard "*". An empty filterValue means the
// dimension is unconstrained. Mirrors appendMandatoryLabelConditions'
// priority CASE WHEN scalar branch (priority is a single string on the
// workflow side, unlike the array fields).
func matchesPriority(workflowPriority, filterValue string) bool {
	if filterValue == "" {
		return true
	}
	return workflowPriority == "*" || strings.EqualFold(workflowPriority, filterValue)
}

// matchesMandatoryLabels reports whether labels satisfies every mandatory
// context-filter dimension in filters (severity, component, environment,
// priority, cluster), ANDed together. nil filters matches unconditionally.
// Mirrors buildContextFilterSQL's combination of
// appendMandatoryLabelConditions' per-dimension conditions.
func matchesMandatoryLabels(labels models.MandatoryLabels, filters *models.WorkflowDiscoveryFilters) bool {
	if filters == nil {
		return true
	}
	return matchesArrayLabel(labels.Severity, filters.Severity) &&
		matchesArrayLabel(labels.Component, filters.Component) &&
		matchesArrayLabel(labels.Environment, filters.Environment) &&
		matchesPriority(labels.Priority, filters.Priority) &&
		matchesArrayLabel(labels.Cluster, filters.Cluster)
}

// matchesDetectedLabelsFilter reports whether workflowDetected satisfies the
// hard-filter semantics of filterDetected. nil filterDetected matches
// unconditionally. Mirrors appendDetectedLabelConditions.
//
// Boolean fields (gitOpsManaged, pdbProtected, hpaEnabled, stateful,
// helmManaged, networkIsolated, virtualMachine, liveMigratable, cdiManaged)
// are deliberately NOT checked here: models.DetectedLabels' sparse JSON
// serialization (see MarshalJSON/SerializeLabels) never distinguishes an
// explicit `false` from an absent key, so the equivalent SQL condition
// "(x = 'true' OR x IS NULL)" is a tautology for a boolean field -- it can
// never exclude a workflow. This is existing SQL behavior, not a gap
// introduced by this port.
//
// String fields (gitOpsTool, serviceMesh, storageBackend) DO have real
// exclusion power because a non-empty declared value is distinguishable from
// absence: workflow must match the filter value exactly, declare "*", or
// declare nothing at all.
func matchesDetectedLabelsFilter(workflowDetected models.DetectedLabels, filterDetected *models.DetectedLabels) bool {
	if filterDetected == nil {
		return true
	}
	return matchesDetectedStringField(workflowDetected.GitOpsTool, filterDetected.GitOpsTool) &&
		matchesDetectedStringField(workflowDetected.ServiceMesh, filterDetected.ServiceMesh) &&
		matchesDetectedStringField(workflowDetected.StorageBackend, filterDetected.StorageBackend)
}

// matchesDetectedStringField applies a single string detected-label field's
// hard-filter condition: unconstrained when filterValue is empty; otherwise
// the workflow must match exactly, declare "*", or declare nothing.
func matchesDetectedStringField(workflowValue, filterValue string) bool {
	if filterValue == "" {
		return true
	}
	return workflowValue == "" || workflowValue == "*" || workflowValue == filterValue
}

// detectedBoolWeights are the DD-WORKFLOW-004 v1.5 boost weights for simple
// boolean detected-label fields (no wildcard semantics).
var detectedBoolWeights = map[string]float64{
	"gitOpsManaged":   detectedLabelWeights["git_ops_managed"],
	"pdbProtected":    detectedLabelWeights["pdb_protected"],
	"hpaEnabled":      detectedLabelWeights["hpa_enabled"],
	"stateful":        detectedLabelWeights["stateful"],
	"helmManaged":     detectedLabelWeights["helm_managed"],
	"networkIsolated": detectedLabelWeights["network_isolated"],
	"virtualMachine":  detectedLabelWeights["virtual_machine"],
	"liveMigratable":  detectedLabelWeights["live_migratable"],
	"cdiManaged":      detectedLabelWeights["cdi_managed"],
}

// detectedWildcardStringFields describes the three wildcard-capable string
// detected-label fields, their DD-WORKFLOW-004 weight, and the allowed enum
// values used to sanitize the query-side value (mirrors
// appendWildcardStringBoostCase's allowedValues parameter).
var detectedWildcardStringFields = []struct {
	get     func(models.DetectedLabels) string
	weight  float64
	allowed []string
}{
	{func(d models.DetectedLabels) string { return d.GitOpsTool }, detectedLabelWeights["git_ops_tool"], []string{"argocd", "flux"}},
	{func(d models.DetectedLabels) string { return d.ServiceMesh }, detectedLabelWeights["service_mesh"], []string{"istio", "linkerd"}},
	{func(d models.DetectedLabels) string { return d.StorageBackend }, detectedLabelWeights["storage_backend"], []string{"odf-ceph", "lvms", "local"}},
}

// detectedLabelsBoost computes the scoring boost contributed by
// workflowDetected against the query's filterDetected. Mirrors
// buildDetectedLabelsBoostSQL's per-field CASE WHEN weights.
func detectedLabelsBoost(workflowDetected models.DetectedLabels, filterDetected *models.DetectedLabels) float64 {
	if filterDetected == nil || filterDetected.IsEmpty() {
		return 0.0
	}

	var boost float64
	boost += boolFieldBoost(filterDetected.GitOpsManaged, workflowDetected.GitOpsManaged, detectedBoolWeights["gitOpsManaged"])
	boost += boolFieldBoost(filterDetected.PDBProtected, workflowDetected.PDBProtected, detectedBoolWeights["pdbProtected"])
	boost += boolFieldBoost(filterDetected.HPAEnabled, workflowDetected.HPAEnabled, detectedBoolWeights["hpaEnabled"])
	boost += boolFieldBoost(filterDetected.Stateful, workflowDetected.Stateful, detectedBoolWeights["stateful"])
	boost += boolFieldBoost(filterDetected.HelmManaged, workflowDetected.HelmManaged, detectedBoolWeights["helmManaged"])
	boost += boolFieldBoost(filterDetected.NetworkIsolated, workflowDetected.NetworkIsolated, detectedBoolWeights["networkIsolated"])
	boost += boolFieldBoost(filterDetected.VirtualMachine, workflowDetected.VirtualMachine, detectedBoolWeights["virtualMachine"])
	boost += boolFieldBoost(filterDetected.LiveMigratable, workflowDetected.LiveMigratable, detectedBoolWeights["liveMigratable"])
	boost += boolFieldBoost(filterDetected.CDIManaged, workflowDetected.CDIManaged, detectedBoolWeights["cdiManaged"])

	for _, f := range detectedWildcardStringFields {
		boost += wildcardStringFieldBoost(f.get(*filterDetected), f.get(workflowDetected), f.weight, f.allowed)
	}

	return boost
}

// boolFieldBoost mirrors appendBoolBoostCase: full weight when the query
// requests the field and the workflow declares it true, otherwise zero.
func boolFieldBoost(filterValue, workflowValue bool, weight float64) float64 {
	if filterValue && workflowValue {
		return weight
	}
	return 0.0
}

// wildcardStringFieldBoost mirrors appendWildcardStringBoostCase: a
// query-side "*" matches any non-empty workflow value at half boost;
// otherwise the (enum-sanitized) query value matches the workflow value
// exactly at full boost, or a workflow-side "*" at half boost.
func wildcardStringFieldBoost(filterValue, workflowValue string, weight float64, allowedValues []string) float64 {
	if filterValue == "" {
		return 0.0
	}
	if filterValue == "*" {
		if workflowValue != "" {
			return weight / 2
		}
		return 0.0
	}
	sanitized := sanitizeEnumValue(filterValue, allowedValues)
	if sanitized == "" {
		return 0.0
	}
	switch workflowValue {
	case sanitized:
		return weight
	case "*":
		return weight / 2
	default:
		return 0.0
	}
}

// highImpactPenaltyWeights are the DD-WORKFLOW-004 penalty weights -- only
// the two high-impact fields (gitOpsManaged, gitOpsTool) apply penalties.
var highImpactPenaltyWeights = map[string]float64{
	"gitOpsManaged": 0.10,
	"gitOpsTool":    0.10,
}

// detectedLabelsPenalty computes the scoring penalty contributed by
// workflowDetected against the query's filterDetected. Mirrors
// buildDetectedLabelsPenaltySQL.
func detectedLabelsPenalty(workflowDetected models.DetectedLabels, filterDetected *models.DetectedLabels) float64 {
	if filterDetected == nil || filterDetected.IsEmpty() {
		return 0.0
	}

	var penalty float64
	if filterDetected.GitOpsManaged && !workflowDetected.GitOpsManaged {
		penalty += highImpactPenaltyWeights["gitOpsManaged"]
	}

	if filterDetected.GitOpsTool != "" {
		tool := sanitizeEnumValue(filterDetected.GitOpsTool, []string{"argocd", "flux"})
		if tool != "" && workflowDetected.GitOpsTool != tool && workflowDetected.GitOpsTool != "*" {
			penalty += highImpactPenaltyWeights["gitOpsTool"]
		}
	}

	return penalty
}

// customLabelsWeight is the DD-WORKFLOW-004 v1.7 per-key custom-label boost.
const customLabelsWeight = 0.15

// customLabelsBoost computes the scoring boost contributed by workflowCustom
// against the query's filterCustom (map[subdomain][]incidentValues). Mirrors
// buildCustomLabelsBoostSQL's @> JSONB containment check: for every
// (key, incidentValue) pair in filterCustom, full boost if the workflow's
// values for that key contain incidentValue, half boost if they contain the
// wildcard "*", otherwise zero -- summed across all pairs.
func customLabelsBoost(workflowCustom models.CustomLabels, filterCustom map[string][]string) float64 {
	if len(filterCustom) == 0 {
		return 0.0
	}

	var boost float64
	for key, incidentValues := range filterCustom {
		workflowValues := workflowCustom[key]
		for _, incidentValue := range incidentValues {
			switch {
			case containsValue(workflowValues, incidentValue):
				boost += customLabelsWeight
			case containsValue(workflowValues, "*"):
				boost += customLabelsWeight / 2
			}
		}
	}
	return boost
}

// containsValue reports whether values contains target (exact match, same
// as the JSONB @> containment check on a single scalar element).
func containsValue(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// finalScore normalizes the raw boost/penalty contributions into the
// DD-WORKFLOW-016 0.0-1.0 confidence range. Mirrors selectScoredWorkflows'
// LEAST((5.0 + detectedBoost + customBoost - penalty) / 10.0, 1.0) formula.
func finalScore(detectedBoost, customBoost, penalty float64) float64 {
	score := (5.0 + detectedBoost + customBoost - penalty) / 10.0
	if score > 1.0 {
		return 1.0
	}
	return score
}
