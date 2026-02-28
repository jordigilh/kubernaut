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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SEARCH OPERATIONS
// CONVENTION (#213, DD-WORKFLOW-016): All paginated queries on remediation_workflow_catalog
// must use workflow_id ASC as a deterministic tiebreaker in ORDER BY.
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

	// Mandatory Filter 1: signalName (exact match)
	if request.Filters.SignalName == "" {
		return nil, fmt.Errorf("filters.signalName is required")
	}
	whereClauses = append(whereClauses, fmt.Sprintf("labels->>'signalName' = $%d", argIndex))
	args = append(args, request.Filters.SignalName)
	argIndex++

	// Mandatory Filter 2: severity (JSONB array, use ? operator)
	// Workflow with severity=["critical","high"] matches if filter value is in array
	if request.Filters.Severity == "" {
		return nil, fmt.Errorf("filters.severity is required")
	}
	whereClauses = append(whereClauses, fmt.Sprintf("(labels->'severity' ? $%d OR labels->'severity' ? '*')", argIndex))
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
	// Delegates to shared scoring functions in scoring.go (#220, #215)
	detectedLabelBoostSQL := buildDetectedLabelsBoostSQL(&request.Filters.DetectedLabels)
	customLabelBoostSQL := buildCustomLabelsBoostSQL(request.Filters.CustomLabels)
	labelPenaltySQL := buildDetectedLabelsPenaltySQL(&request.Filters.DetectedLabels)

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
		ORDER BY final_score DESC, workflow_id ASC
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
		// V1.0: Extract signal_name from structured Labels
		// Authority: Zero unstructured data mandate
		signalName := result.Labels.SignalName

		// Handle optional pointer fields
		schemaImage := ""
		if result.SchemaImage != nil {
			schemaImage = *result.SchemaImage
		}
		executionBundle := ""
		if result.ExecutionBundle != nil {
			executionBundle = *result.ExecutionBundle
		}

		// DD-WORKFLOW-002 v3.0: Flat response structure
		// V1.0: Label-only scoring (no BaseSimilarity or SimilarityScore)
		searchResults[i] = models.WorkflowSearchResult{
			// Flat fields per DD-WORKFLOW-002 v3.0
			WorkflowID:      result.WorkflowID,
			Title:           result.Name, // DD-WORKFLOW-002 v3.0: "name" renamed to "title"
			Description:     result.Description,
			SignalName:      signalName,
			SchemaImage:     schemaImage,
			ExecutionBundle: executionBundle,

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
// SQL SANITIZATION HELPERS
// ========================================
// Used by scoring.go and search.go for safe SQL generation.
// Authority: DD-WORKFLOW-004 v1.5 (Fixed DetectedLabel Weights)

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

// sanitizeJSONBValue produces a safe SQL expression for a JSONB string comparison.
// It JSON-encodes the value (handling ", \, and control characters) and then
// SQL-escapes the result for embedding in a single-quoted SQL literal.
// Returns a string safe to embed as '...'::jsonb in SQL.
func sanitizeJSONBValue(value string) string {
	jsonBytes, _ := json.Marshal(value)
	jsonStr := string(jsonBytes)
	return strings.ReplaceAll(jsonStr, "'", "''")
}

// Legacy scoring methods removed -- replaced by shared standalone functions in scoring.go
// See: buildDetectedLabelsBoostSQL, buildDetectedLabelsPenaltySQL, buildCustomLabelsBoostSQL

// ========================================
// UPDATE OPERATIONS
