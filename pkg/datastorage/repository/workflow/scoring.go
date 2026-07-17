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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// sqlZeroScore is the SQL numeric literal used as a no-op scoring expression
// (added to a SQL SELECT/ORDER BY clause) when there are no boost/penalty
// cases or label criteria to score against.
const sqlZeroScore = "0.0"

// sanitizeEnumValue validates that value is one of the allowedValues.
// Returns the value if valid, empty string otherwise.
func sanitizeEnumValue(value string, allowedValues []string) string {
	for _, allowed := range allowedValues {
		if value == allowed {
			return value
		}
	}
	return ""
}

// sanitizeJSONBKey removes characters that could cause SQL injection from JSONB keys.
func sanitizeJSONBKey(key string) string {
	return regexp.MustCompile(`[^a-zA-Z0-9_\-]`).ReplaceAllString(key, "")
}

// sanitizeJSONBValue produces a safe SQL expression for a JSONB string comparison.
// It JSON-encodes the value and SQL-escapes the result for embedding in a
// single-quoted SQL literal. Returns a string safe to embed as '...'::jsonb.
func sanitizeJSONBValue(value string) string {
	jsonBytes, _ := json.Marshal(value)
	jsonStr := string(jsonBytes)
	return strings.ReplaceAll(jsonStr, "'", "''")
}

// ========================================
// SHARED SCORING SQL BUILDERS
// ========================================
// Authority: DD-WORKFLOW-004 v1.5 (Label-Only Scoring with Wildcard Weighting)
// Authority: SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md
// Authority: DD-WORKFLOW-016 (final_score ordering)
//
// These standalone functions generate SQL scoring expressions usable from
// both the discovery flow (discovery.go) and search flow (search.go).
// Bugs: #220 (missing final_score), #215 (inverted wildcard), #212 (custom labels)

// Detected label weights from DD-WORKFLOW-004 v1.5
var detectedLabelWeights = map[string]float64{
	"git_ops_managed":  0.10,
	"git_ops_tool":     0.10,
	"pdb_protected":    0.05,
	"service_mesh":     0.05,
	"network_isolated": 0.03,
	"helm_managed":     0.02,
	"stateful":         0.02,
	"hpa_enabled":      0.02,
	"virtual_machine":  0.08,
	"live_migratable":  0.04,
	"cdi_managed":      0.03,
	"storage_backend":  0.05,
}

// appendBoolBoostCase appends a boost CASE expression for a simple boolean
// detected-label field (no wildcard semantics) when present is true.
// Extracted from buildDetectedLabelsBoostSQL (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3) — pure code motion, no behavior change.
func appendBoolBoostCase(cases []string, present bool, jsonKey string, weight float64) []string {
	if !present {
		return cases
	}
	return append(cases, fmt.Sprintf("CASE WHEN detected_labels->>'%s' = 'true' THEN %.2f ELSE 0.0 END", jsonKey, weight))
}

// appendWildcardStringBoostCase appends a bidirectional-wildcard boost CASE
// expression (#215 Gap 2 fix) for a string detected-label field: a literal
// "*" query value matches any non-empty workflow value at half boost;
// otherwise the query value (validated against allowedValues) matches
// exactly at full boost, or a workflow-side "*" at half boost. Extracted from
// buildDetectedLabelsBoostSQL (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure
// code motion, no behavior change.
func appendWildcardStringBoostCase(cases []string, value, jsonKey string, weight float64, allowedValues []string) []string {
	if value == "" {
		return cases
	}
	if value == "*" {
		// Query-side wildcard: match any workflow with a non-empty value → half boost
		return append(cases, fmt.Sprintf(
			"CASE WHEN detected_labels->>'%s' IS NOT NULL AND detected_labels->>'%s' != '' THEN %.2f ELSE 0.0 END",
			jsonKey, jsonKey, weight/2))
	}
	sanitized := sanitizeEnumValue(value, allowedValues)
	if sanitized == "" {
		return cases
	}
	return append(cases, fmt.Sprintf(
		"CASE WHEN detected_labels->>'%s' = '%s' THEN %.2f WHEN detected_labels->>'%s' = '*' THEN %.2f ELSE 0.0 END",
		jsonKey, sanitized, weight, jsonKey, weight/2))
}

// buildDetectedLabelsBoostSQL generates SQL for detected label boost scoring.
// Checks the WORKFLOW side for exact and wildcard matches against the query values.
//
// Per DD-WORKFLOW-001 matching matrix and SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md:
//   - Exact match on workflow side: full boost
//   - Workflow declares "*": half boost (supports any value)
//   - No match: 0.0
func buildDetectedLabelsBoostSQL(dl *models.DetectedLabels) string {
	if dl == nil || dl.IsEmpty() {
		return sqlZeroScore
	}

	boostCases := []string{}
	boostCases = appendBoolBoostCase(boostCases, dl.GitOpsManaged, "gitOpsManaged", detectedLabelWeights["git_ops_managed"])
	boostCases = appendWildcardStringBoostCase(boostCases, dl.GitOpsTool, "gitOpsTool", detectedLabelWeights["git_ops_tool"], []string{"argocd", "flux"})
	boostCases = appendBoolBoostCase(boostCases, dl.PDBProtected, "pdbProtected", detectedLabelWeights["pdb_protected"])
	boostCases = appendWildcardStringBoostCase(boostCases, dl.ServiceMesh, "serviceMesh", detectedLabelWeights["service_mesh"], []string{"istio", "linkerd"})
	boostCases = appendBoolBoostCase(boostCases, dl.NetworkIsolated, "networkIsolated", detectedLabelWeights["network_isolated"])
	boostCases = appendBoolBoostCase(boostCases, dl.HelmManaged, "helmManaged", detectedLabelWeights["helm_managed"])
	boostCases = appendBoolBoostCase(boostCases, dl.Stateful, "stateful", detectedLabelWeights["stateful"])
	boostCases = appendBoolBoostCase(boostCases, dl.HPAEnabled, "hpaEnabled", detectedLabelWeights["hpa_enabled"])
	boostCases = appendBoolBoostCase(boostCases, dl.VirtualMachine, "virtualMachine", detectedLabelWeights["virtual_machine"])
	boostCases = appendBoolBoostCase(boostCases, dl.LiveMigratable, "liveMigratable", detectedLabelWeights["live_migratable"])
	boostCases = appendBoolBoostCase(boostCases, dl.CDIManaged, "cdiManaged", detectedLabelWeights["cdi_managed"])
	boostCases = appendWildcardStringBoostCase(boostCases, dl.StorageBackend, "storageBackend", detectedLabelWeights["storage_backend"], []string{"odf-ceph", "lvms", "local"})

	if len(boostCases) == 0 {
		return sqlZeroScore
	}

	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(boostCases, " + "))
}

// buildDetectedLabelsPenaltySQL generates SQL for detected label penalty scoring.
// Only high-impact fields apply penalties (gitOpsManaged, gitOpsTool).
//
// Per SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md:
//   - Query wants GitOps but workflow is NOT GitOps: penalty
//   - Query wants specific tool but workflow has different tool: penalty
func buildDetectedLabelsPenaltySQL(dl *models.DetectedLabels) string {
	if dl == nil || dl.IsEmpty() {
		return sqlZeroScore
	}

	penaltyCases := []string{}

	highImpactWeights := map[string]float64{
		"git_ops_managed": 0.10,
		"git_ops_tool":    0.10,
	}

	if dl.GitOpsManaged {
		weight := highImpactWeights["git_ops_managed"]
		penaltyCases = append(penaltyCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsManaged' IS NULL OR detected_labels->>'gitOpsManaged' = 'false' THEN %.2f ELSE 0.0 END", weight))
	}

	if dl.GitOpsTool != "" {
		weight := highImpactWeights["git_ops_tool"]
		tool := sanitizeEnumValue(dl.GitOpsTool, []string{"argocd", "flux"})
		if tool != "" {
			penaltyCases = append(penaltyCases,
				fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsTool' IS NULL OR (detected_labels->>'gitOpsTool' != '%s' AND detected_labels->>'gitOpsTool' != '*' AND detected_labels->>'gitOpsTool' != '') THEN %.2f ELSE 0.0 END", tool, weight))
		}
	}

	if len(penaltyCases) == 0 {
		return sqlZeroScore
	}

	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(penaltyCases, " + "))
}

// buildCustomLabelsBoostSQL generates SQL for custom label boost scoring.
//
// Per DD-WORKFLOW-004 v1.7:
//   - Exact match: 0.15 per key
//   - Wildcard match: 0.075 per key
//   - No match: 0.0
func buildCustomLabelsBoostSQL(customLabels map[string][]string) string {
	if len(customLabels) == 0 {
		return sqlZeroScore
	}

	boostCases := []string{}
	const customLabelWeight = 0.15

	for key, incidentValues := range customLabels {
		if len(incidentValues) == 0 {
			continue
		}

		for _, incidentValue := range incidentValues {
			safeKey := sanitizeJSONBKey(key)
			if safeKey == "" {
				continue
			}
			safeValue := sanitizeJSONBValue(incidentValue)

			boostCase := fmt.Sprintf(`
				CASE
					WHEN custom_labels->'%s' @> '%s'::jsonb THEN %.2f
					WHEN custom_labels->'%s' @> '"*"'::jsonb THEN %.2f
					ELSE 0.0
				END`,
				safeKey, safeValue, customLabelWeight,
				safeKey, customLabelWeight/2)

			boostCases = append(boostCases, boostCase)
		}
	}

	if len(boostCases) == 0 {
		return sqlZeroScore
	}

	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(boostCases, " + "))
}
