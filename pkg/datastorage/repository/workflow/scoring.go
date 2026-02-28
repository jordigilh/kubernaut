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
	"fmt"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

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
		return "0.0"
	}

	boostCases := []string{}

	// GitOpsManaged (boolean -- no wildcard for booleans)
	if dl.GitOpsManaged {
		weight := detectedLabelWeights["git_ops_managed"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsManaged' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// GitOpsTool (string -- workflow-side wildcard: #215 Gap 2 fix)
	if dl.GitOpsTool != "" {
		weight := detectedLabelWeights["git_ops_tool"]
		tool := sanitizeEnumValue(dl.GitOpsTool, []string{"argocd", "flux"})
		if tool != "" {
			boostCases = append(boostCases,
				fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsTool' = '%s' THEN %.2f WHEN detected_labels->>'gitOpsTool' = '*' THEN %.2f ELSE 0.0 END",
					tool, weight, weight/2))
		}
	}

	// PDBProtected (boolean)
	if dl.PDBProtected {
		weight := detectedLabelWeights["pdb_protected"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'pdbProtected' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// ServiceMesh (string -- workflow-side wildcard: #215 Gap 2 fix)
	if dl.ServiceMesh != "" {
		weight := detectedLabelWeights["service_mesh"]
		mesh := sanitizeEnumValue(dl.ServiceMesh, []string{"istio", "linkerd"})
		if mesh != "" {
			boostCases = append(boostCases,
				fmt.Sprintf("CASE WHEN detected_labels->>'serviceMesh' = '%s' THEN %.2f WHEN detected_labels->>'serviceMesh' = '*' THEN %.2f ELSE 0.0 END",
					mesh, weight, weight/2))
		}
	}

	// NetworkIsolated (boolean)
	if dl.NetworkIsolated {
		weight := detectedLabelWeights["network_isolated"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'networkIsolated' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// HelmManaged (boolean)
	if dl.HelmManaged {
		weight := detectedLabelWeights["helm_managed"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'helmManaged' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// Stateful (boolean)
	if dl.Stateful {
		weight := detectedLabelWeights["stateful"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'stateful' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// HPAEnabled (boolean)
	if dl.HPAEnabled {
		weight := detectedLabelWeights["hpa_enabled"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'hpaEnabled' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	if len(boostCases) == 0 {
		return "0.0"
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
		return "0.0"
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
		return "0.0"
	}

	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(penaltyCases, " + "))
}

// buildCustomLabelsBoostSQL generates SQL for custom label boost scoring.
//
// Per DD-WORKFLOW-004 v1.5:
//   - Exact match: 0.05 per key
//   - Wildcard match: 0.025 per key
//   - No match: 0.0
func buildCustomLabelsBoostSQL(customLabels map[string][]string) string {
	if len(customLabels) == 0 {
		return "0.0"
	}

	boostCases := []string{}
	const customLabelWeight = 0.05

	for key, incidentValues := range customLabels {
		if len(incidentValues) == 0 {
			continue
		}

		for _, incidentValue := range incidentValues {
			safeKey := sanitizeJSONBKey(key)
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
		return "0.0"
	}

	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(boostCases, " + "))
}
