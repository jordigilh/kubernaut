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

package workflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SEARCH OPERATIONS
// ========================================
// V1.0: Label-only search with wildcard support
// Authority: DD-WORKFLOW-004 v1.5 (Label-Only Scoring)
// ========================================

func (r *Repository) SearchByLabels(ctx context.Context, request *models.WorkflowSearchRequest) (*models.WorkflowSearchResponse, error) {
	// Validate filters are provided (required for label-only search)
	if request.Filters == nil {
		return nil, fmt.Errorf("filters are required for label-only search")
	}

	// Build WHERE clause for hard filters
	whereClauses := []string{}
	args := []interface{}{}
	argIndex := 1

	// Default: only active workflows unless IncludeDisabled is true
	if !request.IncludeDisabled {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, "active")
		argIndex++
	}

	// Only latest versions
	whereClauses = append(whereClauses, "is_latest_version = true")

	// ========================================
	// MANDATORY LABEL FILTERING (5 labels)
	// ========================================
	// Authority: DD-WORKFLOW-001 v1.6 (5 mandatory labels)
	// These filters provide the base score of 5.0 (one point per exact match)

	// Mandatory Filter 1: signalType (exact match)
	if request.Filters.SignalType == "" {
		return nil, fmt.Errorf("filters.signalType is required")
	}
	whereClauses = append(whereClauses, fmt.Sprintf("labels->>'signalType' = $%d", argIndex))
	args = append(args, request.Filters.SignalType)
	argIndex++

	// Mandatory Filter 2: severity (JSONB array, use ? operator)
	// Workflow with severity=["critical","high"] matches if filter value is in array
	if request.Filters.Severity == "" {
		return nil, fmt.Errorf("filters.severity is required")
	}
	whereClauses = append(whereClauses, fmt.Sprintf("labels->'severity' ? $%d", argIndex))
	args = append(args, request.Filters.Severity)
	argIndex++

	// Mandatory Filter 3: component (supports wildcard matching)
	// Workflow with component='*' matches ANY search filter (wildcard)
	// Workflow with component='deployment' matches only 'deployment' (exact)
	if request.Filters.Component == "" {
		return nil, fmt.Errorf("filters.component is required")
	}
	whereClauses = append(whereClauses, fmt.Sprintf("(labels->>'component' = $%d OR labels->>'component' = '*')", argIndex))
	args = append(args, request.Filters.Component)
	argIndex++

	// Mandatory Filter 4: environment (JSONB array, use ? operator; supports "*" wildcard per OpenAPI spec)
	// DD-WORKFLOW-001 v2.5: Workflows store array, search with single value
	if request.Filters.Environment == "" {
		return nil, fmt.Errorf("filters.environment is required")
	}
	envWhereClause := fmt.Sprintf("(labels->'environment' ? $%d OR labels->'environment' ? '*')", argIndex)
	whereClauses = append(whereClauses, envWhereClause)
	args = append(args, request.Filters.Environment)
	argIndex++


	// Mandatory Filter 5: priority (supports wildcard matching)
	// Workflow with priority='*' matches ANY search filter (wildcard)
	// Workflow with priority='P0' matches only 'P0' (exact)
	// Wildcard support added because priority is derived from LLM severity assessment,
	// which is non-deterministic — a wildcard workflow matches regardless of priority mapping.
	if request.Filters.Priority == "" {
		return nil, fmt.Errorf("filters.priority is required")
	}
	whereClauses = append(whereClauses, fmt.Sprintf("(labels->>'priority' = $%d OR labels->>'priority' = '*')", argIndex))
	args = append(args, request.Filters.Priority)
	argIndex++

	// ========================================
	// CUSTOM LABELS REMOVED FROM HARD FILTERING
	// ========================================
	// V1.0: CustomLabels moved to scoring with wildcard support (no hard filtering)
	// Authority: DD-WORKFLOW-004 v1.5 + user confirmation 2025-12-11
	// Rationale: Wildcard matching allows workflows to specify "*" for any value,
	// enabling exact matches to rank higher than wildcard matches.

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Set default TopK
	topK := 10
	if request.TopK > 0 {
		topK = request.TopK
	}

	// ========================================
	// V1.0 SCORING: LABEL-ONLY WITH WILDCARD WEIGHTING
	// ========================================
	// Authority: DD-WORKFLOW-004 v1.5 (Label-Only Scoring)
	// Authority: SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md
	//
	// Scoring Components:
	//   - base_score: 5.0 (from 5 mandatory labels, hard-filtered in WHERE)
	//   - detected_label_boost: DetectedLabel boosts with wildcard support (0.0-0.39)
	//   - custom_label_boost: CustomLabel boosts with wildcard support (0.0-0.50) [V1.0 NEW]
	//   - label_penalty: High-impact conflicting DetectedLabels (0.0-0.20)
	//   - raw_score: base_score + detected_label_boost + custom_label_boost - label_penalty
	//   - final_score: raw_score / 10.0 (normalized to 0.0-1.0 range)
	//
	// Wildcard Logic (for ALL label types):
	//   - Exact match: Full boost (gitOpsTool='argocd' → +0.10)
	//   - Wildcard match: Half boost (gitOpsTool='*' → +0.05)
	//   - Conflicting match: Full penalty (gitOpsTool mismatch → -0.10)
	//   - No filter: No boost/penalty (gitOpsTool absent → 0.0)

	// Build scoring SQL components with wildcard support
	detectedLabelBoostSQL := r.buildDetectedLabelsBoostSQLWithWildcard(request)
	customLabelBoostSQL := r.buildCustomLabelsBoostSQLWithWildcard(request)
	labelPenaltySQL := r.buildDetectedLabelsPenaltySQL(request)

	query := fmt.Sprintf(`
		SELECT * FROM (
			SELECT
				*,
				%s AS detected_label_boost,
				%s AS custom_label_boost,
				%s AS label_penalty,
				(5.0 + (%s) + (%s) - (%s)) / 10.0 AS final_score
			FROM remediation_workflow_catalog
			%s
		) scored_workflows
		WHERE final_score >= $%d
		ORDER BY final_score DESC, created_at DESC
		LIMIT $%d
	`, detectedLabelBoostSQL, customLabelBoostSQL, labelPenaltySQL,
		detectedLabelBoostSQL, customLabelBoostSQL, labelPenaltySQL,
		whereClause,
		argIndex, argIndex+1)

	// Add MinScore and TopK to args
	args = append(args, request.MinScore) // $argIndex
	args = append(args, topK)             // $argIndex+1

	// Execute query with label-only scoring (V1.0: includes CustomLabels wildcard)
	type workflowWithScore struct {
		models.RemediationWorkflow
		DetectedLabelBoost float64 `db:"detected_label_boost"`
		CustomLabelBoost   float64 `db:"custom_label_boost"`
		LabelPenalty       float64 `db:"label_penalty"`
		FinalScore         float64 `db:"final_score"`
	}

	var results []workflowWithScore
	err := r.db.SelectContext(ctx, &results, query, args...)
	if err != nil {
		r.logger.Error(err, "failed to search workflows by labels",
			"filters", request.Filters)
		return nil, fmt.Errorf("failed to search workflows: %w", err)
	}

	// Build response with DD-WORKFLOW-002 v3.0 flat structure
	searchResults := make([]models.WorkflowSearchResult, len(results))
	for i, result := range results {
		// V1.0: Extract signal_type from structured Labels
		// Authority: Zero unstructured data mandate
		signalType := result.Labels.SignalType

		// Handle optional pointer fields
		containerImage := ""
		if result.ContainerImage != nil {
			containerImage = *result.ContainerImage
		}
		containerDigest := ""
		if result.ContainerDigest != nil {
			containerDigest = *result.ContainerDigest
		}

		// DD-WORKFLOW-002 v3.0: Flat response structure
		// V1.0: Label-only scoring (no BaseSimilarity or SimilarityScore)
		searchResults[i] = models.WorkflowSearchResult{
			// Flat fields per DD-WORKFLOW-002 v3.0
			WorkflowID:      result.WorkflowID,
			Title:           result.Name, // DD-WORKFLOW-002 v3.0: "name" renamed to "title"
			Description:     result.Description,
			SignalType:      signalType,
			ContainerImage:  containerImage,
			ContainerDigest: containerDigest,

			// V1.0: Label-only scoring fields (with CustomLabels wildcard support)
			Confidence:   result.FinalScore,                                   // DD-WORKFLOW-002 v3.0: "confidence" = final_score
			LabelBoost:   result.DetectedLabelBoost + result.CustomLabelBoost, // Combined boost
			LabelPenalty: result.LabelPenalty,
			FinalScore:   result.FinalScore,
			Rank:         i + 1,

			// DD-WORKFLOW-001 v1.6: Label columns in response
			CustomLabels:   result.CustomLabels,
			DetectedLabels: result.DetectedLabels,

			// BR-HAPI-191: Parameter schema for HAPI validation and LLM guidance
			Parameters: result.Parameters,

			// Internal: full workflow for audit/logging (not serialized)
			Workflow: result.RemediationWorkflow,
		}
	}

	response := &models.WorkflowSearchResponse{
		Workflows:    searchResults,
		TotalResults: len(searchResults),
		Filters:      request.Filters,
	}

	r.logger.Info("label-only search completed",
		"filters", request.Filters,
		"results", len(searchResults))

	return response, nil
}

// ========================================
// HYBRID SCORING HELPER FUNCTIONS
// ========================================
// Authority: DD-WORKFLOW-004 v1.5 (Fixed DetectedLabel Weights)
// Package: pkg/datastorage/scoring

// sanitizeEnumValue validates that a string value is in the allowed enum set.
// Returns the value if valid, empty string if invalid.
// This prevents SQL injection from unexpected enum values.
func sanitizeEnumValue(value string, allowedValues []string) string {
	for _, allowed := range allowedValues {
		if value == allowed {
			return value
		}
	}
	return "" // Invalid value, return empty string
}

// sanitizeJSONBKey sanitizes JSONB keys for SQL queries
// Removes characters that could cause SQL injection
func sanitizeJSONBKey(key string) string {
	// Allow alphanumeric, underscore, hyphen only
	return regexp.MustCompile(`[^a-zA-Z0-9_\-]`).ReplaceAllString(key, "")
}

// sanitizeSQLString sanitizes string values for SQL queries
// Escapes single quotes to prevent SQL injection
func sanitizeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

// buildDetectedLabelsPenaltySQL generates SQL to calculate the label penalty score.
// Returns SQL expression that sums penalties for conflicting high-impact DetectedLabels.
//
// Logic:
//   - Only high-impact labels apply penalties: gitOpsManaged, gitOpsTool
//   - If query has DetectedLabel=true but workflow has DetectedLabel=false (or missing) → penalty
//   - If query has DetectedLabel=toolA but workflow has DetectedLabel=toolB → penalty
//
// Example SQL output:
//
//	COALESCE(
//	  CASE WHEN detected_labels->>'git_ops_managed' IS NULL OR detected_labels->>'git_ops_managed' = 'false' THEN 0.10 ELSE 0.0 END,
//	  0.0
//	)
func (r *Repository) buildDetectedLabelsPenaltySQL(request *models.WorkflowSearchRequest) string {
	if request.Filters == nil || request.Filters.DetectedLabels.IsEmpty() {
		return "0.0" // No DetectedLabels in query = no penalty
	}

	dl := &request.Filters.DetectedLabels
	penaltyCases := []string{}

	// Only high-impact fields apply penalties (per scoring.ShouldApplyPenalty)
	highImpactWeights := map[string]float64{
		"git_ops_managed": 0.10,
		"git_ops_tool":    0.10,
	}

	// V1.0: DetectedLabels fields are plain types (not pointers)
	// GitOpsManaged penalty: Query wants GitOps but workflow is NOT GitOps
	if dl.GitOpsManaged {
		weight := highImpactWeights["git_ops_managed"]
		// Penalty if workflow is NOT GitOps-managed (null or false)
		penaltyCases = append(penaltyCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsManaged' IS NULL OR detected_labels->>'gitOpsManaged' = 'false' THEN %.2f ELSE 0.0 END", weight))
	}

	// GitOpsTool penalty: Query wants specific tool but workflow has different tool or no tool
	if dl.GitOpsTool != "" {
		weight := highImpactWeights["git_ops_tool"]
		tool := sanitizeEnumValue(dl.GitOpsTool, []string{"argocd", "flux"})
		if tool != "" {
			// Penalty if workflow has different tool or no tool
			penaltyCases = append(penaltyCases,
				fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsTool' IS NULL OR (detected_labels->>'gitOpsTool' != '%s' AND detected_labels->>'gitOpsTool' != '') THEN %.2f ELSE 0.0 END", tool, weight))
		}
	}

	if len(penaltyCases) == 0 {
		return "0.0" // No penalties = 0.0
	}

	// Sum all penalty cases
	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(penaltyCases, " + "))
}

// buildCustomLabelsBoostSQLWithWildcard generates SQL with wildcard weighting for CustomLabels.
// V1.0: CustomLabels wildcard support (2025-12-11 user approval)
// Authority: User confirmation 2025-12-11
//
// Wildcard Logic (for dynamic key-value pairs):
//   - Exact match: Full boost (incident has "cost-constrained", workflow has ["cost-constrained"] → +0.05)
//   - Wildcard match: Half boost (incident has "cost-constrained", workflow has ["*"] → +0.025)
//   - No match: No boost (incident has "cost-constrained", workflow has ["no-restart"] → 0.0)
//   - No filter: No boost (incident absent → 0.0)
//
// Weight: 0.05 per custom label key (up to 10 keys max = 0.50 total boost)
func (r *Repository) buildCustomLabelsBoostSQLWithWildcard(request *models.WorkflowSearchRequest) string {
	if request.Filters == nil || len(request.Filters.CustomLabels) == 0 {
		return "0.0" // No CustomLabels in query = no boost
	}

	boostCases := []string{}
	const customLabelWeight = 0.05 // Weight per custom label key

	// Iterate over incident custom labels (from SP Rego)
	for key, incidentValues := range request.Filters.CustomLabels {
		if len(incidentValues) == 0 {
			continue
		}

		// For each incident value, generate matching SQL
		for _, incidentValue := range incidentValues {
			// Sanitize key and value for SQL injection prevention
			safeKey := sanitizeJSONBKey(key)
			safeValue := sanitizeSQLString(incidentValue)

			// SQL pattern: Check workflow's custom_labels JSONB
			//   - Exact match: custom_labels->>'key' ? 'value' → Full boost
			//   - Wildcard match: custom_labels->>'key' ? '*' → Half boost
			boostCase := fmt.Sprintf(`
				CASE
					WHEN custom_labels->'%s' @> '"%s"'::jsonb THEN %.2f
					WHEN custom_labels->'%s' @> '"*"'::jsonb THEN %.2f
					ELSE 0.0
				END`,
				safeKey, safeValue, customLabelWeight, // Exact match
				safeKey, customLabelWeight/2) // Wildcard match

			boostCases = append(boostCases, boostCase)
		}
	}

	if len(boostCases) == 0 {
		return "0.0"
	}

	// Sum all boost cases
	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(boostCases, " + "))
}

// buildDetectedLabelsBoostSQLWithWildcard generates SQL with wildcard weighting support.
// V1.0: Label-only scoring with wildcard differentiation
// Authority: SQL_WILDCARD_WEIGHTING_IMPLEMENTATION.md
//
// Wildcard Logic (for string fields like gitOpsTool, serviceMesh):
//   - Exact match: Full boost (gitOpsTool='argocd' + workflow='argocd' → +0.10)
//   - Wildcard match: Half boost (gitOpsTool='*' + workflow='argocd' → +0.05)
//   - No match: No boost (gitOpsTool='argocd' + workflow='flux' → 0.0)
//   - No filter: No boost (gitOpsTool absent → 0.0)
//
// Example SQL output:
//
//	COALESCE(
//	  CASE WHEN detected_labels->>'git_ops_managed' = 'true' THEN 0.10 ELSE 0.0 END +
//	  CASE
//	    WHEN detected_labels->>'git_ops_tool' = 'argocd' THEN 0.10  -- exact match
//	    WHEN detected_labels->>'git_ops_tool' IS NOT NULL THEN 0.05  -- wildcard match
//	    ELSE 0.0
//	  END,
//	  0.0
//	)
func (r *Repository) buildDetectedLabelsBoostSQLWithWildcard(request *models.WorkflowSearchRequest) string {
	if request.Filters == nil || request.Filters.DetectedLabels.IsEmpty() {
		return "0.0" // No DetectedLabels in query = no boost
	}

	dl := &request.Filters.DetectedLabels
	boostCases := []string{}

	// Weights from scoring package (inline to avoid circular deps)
	weights := map[string]float64{
		"git_ops_managed":  0.10,
		"git_ops_tool":     0.10,
		"pdb_protected":    0.05,
		"service_mesh":     0.05,
		"network_isolated": 0.03,
		"helm_managed":     0.02,
		"stateful":         0.02,
		"hpa_enabled":      0.02,
	}

	// V1.0: DetectedLabels fields are plain types (not pointers)
	// Booleans: check if true, Strings: check if non-empty

	// GitOpsManaged (boolean - no wildcard)
	if dl.GitOpsManaged {
		weight := weights["git_ops_managed"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsManaged' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// GitOpsTool (string - wildcard support)
	if dl.GitOpsTool != "" {
		weight := weights["git_ops_tool"]
		if dl.GitOpsTool == "*" {
			// Wildcard: Half boost if workflow has ANY git_ops_tool
			boostCases = append(boostCases,
				fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsTool' IS NOT NULL AND detected_labels->>'gitOpsTool' != '' THEN %.2f ELSE 0.0 END", weight/2))
		} else {
			// Exact match: Full boost
			tool := sanitizeEnumValue(dl.GitOpsTool, []string{"argocd", "flux"})
			if tool != "" {
				boostCases = append(boostCases,
					fmt.Sprintf("CASE WHEN detected_labels->>'gitOpsTool' = '%s' THEN %.2f ELSE 0.0 END", tool, weight))
			}
		}
	}

	// PDBProtected (boolean - no wildcard)
	if dl.PDBProtected {
		weight := weights["pdb_protected"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'pdbProtected' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// ServiceMesh (string - wildcard support)
	if dl.ServiceMesh != "" {
		weight := weights["service_mesh"]
		if dl.ServiceMesh == "*" {
			// Wildcard: Half boost if workflow has ANY service_mesh
			boostCases = append(boostCases,
				fmt.Sprintf("CASE WHEN detected_labels->>'serviceMesh' IS NOT NULL AND detected_labels->>'serviceMesh' != '' THEN %.2f ELSE 0.0 END", weight/2))
		} else {
			// Exact match: Full boost
			mesh := sanitizeEnumValue(dl.ServiceMesh, []string{"istio", "linkerd"})
			if mesh != "" {
				boostCases = append(boostCases,
					fmt.Sprintf("CASE WHEN detected_labels->>'serviceMesh' = '%s' THEN %.2f ELSE 0.0 END", mesh, weight))
			}
		}
	}

	// NetworkIsolated (boolean - no wildcard)
	if dl.NetworkIsolated {
		weight := weights["network_isolated"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'networkIsolated' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// HelmManaged (boolean - no wildcard)
	if dl.HelmManaged {
		weight := weights["helm_managed"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'helmManaged' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// Stateful (boolean - no wildcard)
	if dl.Stateful {
		weight := weights["stateful"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'stateful' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	// HPAEnabled (boolean - no wildcard)
	if dl.HPAEnabled {
		weight := weights["hpa_enabled"]
		boostCases = append(boostCases,
			fmt.Sprintf("CASE WHEN detected_labels->>'hpaEnabled' = 'true' THEN %.2f ELSE 0.0 END", weight))
	}

	if len(boostCases) == 0 {
		return "0.0" // No matching labels = no boost
	}

	// Sum all boost cases
	return fmt.Sprintf("COALESCE((%s), 0.0)", strings.Join(boostCases, " + "))
}

// ========================================
// UPDATE OPERATIONS
