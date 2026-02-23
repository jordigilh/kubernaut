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
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// THREE-STEP WORKFLOW DISCOVERY REPOSITORY
// ========================================
// Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
// Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
// Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
//
// Step 1: ListActions -- list action types with active workflow counts
// Step 2: ListWorkflowsByActionType -- list workflows for an action type
// Step 3: GetWorkflowWithContextFilters -- get workflow with security gate
// ========================================

// ListActions returns action types from the taxonomy that have active workflows
// matching the provided signal context filters (Step 1 of discovery protocol).
// Returns action type entries with workflow counts, total count for pagination, and error.
func (r *Repository) ListActions(ctx context.Context, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.ActionTypeEntry, int, error) {
	// Build the context filter WHERE clause for the workflow join
	whereClause, args := buildContextFilterSQL(filters)

	// Always filter for active workflows only (GAP-WF-3: DD-WORKFLOW-016 - latest version only)
	activeFilter := "w.status = 'active' AND w.is_latest_version = true"
	if whereClause != "" {
		whereClause = activeFilter + " AND " + whereClause
	} else {
		whereClause = activeFilter
	}

	// Count query: total distinct action types with matching active workflows
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT t.action_type)
		FROM action_type_taxonomy t
		INNER JOIN remediation_workflow_catalog w ON w.action_type = t.action_type
		WHERE %s
	`, whereClause)

	var totalCount int
	err := r.db.GetContext(ctx, &totalCount, countQuery, args...)
	if err != nil {
		r.logger.Error(err, "failed to count action types")
		return nil, 0, fmt.Errorf("failed to count action types: %w", err)
	}

	// Main query: action types with workflow counts, paginated
	// Use positional parameters for offset and limit
	mainQuery := fmt.Sprintf(`
		SELECT
			t.action_type,
			t.description,
			COUNT(w.workflow_id) AS workflow_count
		FROM action_type_taxonomy t
		INNER JOIN remediation_workflow_catalog w ON w.action_type = t.action_type
		WHERE %s
		GROUP BY t.action_type, t.description
		ORDER BY t.action_type
		OFFSET $%d LIMIT $%d
	`, whereClause, len(args)+1, len(args)+2)

	args = append(args, offset, limit)

	type actionTypeRow struct {
		ActionType    string          `db:"action_type"`
		Description   json.RawMessage `db:"description"`
		WorkflowCount int             `db:"workflow_count"`
	}

	var rows []actionTypeRow
	err = r.db.SelectContext(ctx, &rows, mainQuery, args...)
	if err != nil {
		r.logger.Error(err, "failed to list action types")
		return nil, 0, fmt.Errorf("failed to list action types: %w", err)
	}

	// Convert to response entries
	entries := make([]models.ActionTypeEntry, 0, len(rows))
	for _, row := range rows {
		var desc models.ActionTypeDescription
		if err := json.Unmarshal(row.Description, &desc); err != nil {
			r.logger.Error(err, "failed to unmarshal action type description",
				"action_type", row.ActionType)
			// Use empty description rather than failing the entire request
			desc = models.ActionTypeDescription{}
		}
		entries = append(entries, models.ActionTypeEntry{
			ActionType:    row.ActionType,
			Description:   desc,
			WorkflowCount: row.WorkflowCount,
		})
	}

	return entries, totalCount, nil
}

// ListWorkflowsByActionType returns active workflows matching the specified action type
// and signal context filters (Step 2 of discovery protocol).
// Returns workflow list, total count for pagination, and error.
func (r *Repository) ListWorkflowsByActionType(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.RemediationWorkflow, int, error) {
	// Build context filter WHERE clause
	whereClause, args := buildContextFilterSQL(filters)

	// Always filter for active status, latest version, and specific action_type (GAP-WF-3: DD-WORKFLOW-016)
	baseFilter := fmt.Sprintf("action_type = $%d AND status = 'active' AND is_latest_version = true", len(args)+1)
	args = append(args, actionType)

	if whereClause != "" {
		whereClause = baseFilter + " AND " + whereClause
	} else {
		whereClause = baseFilter
	}

	// Count query
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM remediation_workflow_catalog
		WHERE %s
	`, whereClause)

	var totalCount int
	err := r.db.GetContext(ctx, &totalCount, countQuery, args...)
	if err != nil {
		r.logger.Error(err, "failed to count workflows by action type",
			"action_type", actionType)
		return nil, 0, fmt.Errorf("failed to count workflows by action type: %w", err)
	}

	// Main query with pagination
	mainQuery := fmt.Sprintf(`
		SELECT * FROM remediation_workflow_catalog
		WHERE %s
		ORDER BY actual_success_rate DESC NULLS LAST, created_at DESC
		OFFSET $%d LIMIT $%d
	`, whereClause, len(args)+1, len(args)+2)

	args = append(args, offset, limit)

	var workflows []models.RemediationWorkflow
	err = r.db.SelectContext(ctx, &workflows, mainQuery, args...)
	if err != nil {
		r.logger.Error(err, "failed to list workflows by action type",
			"action_type", actionType)
		return nil, 0, fmt.Errorf("failed to list workflows by action type: %w", err)
	}

	return workflows, totalCount, nil
}

// GetWorkflowWithContextFilters retrieves a workflow by ID with an additional
// security gate that verifies the workflow matches the provided context filters.
// Returns nil if the workflow exists but doesn't match the context (security gate).
// This is Step 3 of the discovery protocol.
func (r *Repository) GetWorkflowWithContextFilters(ctx context.Context, workflowID string, filters *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	// If no context filters, fall back to simple GetByID
	if filters == nil || !filters.HasContextFilters() {
		return r.GetByID(ctx, workflowID)
	}

	// Build context filter WHERE clause
	whereClause, args := buildContextFilterSQL(filters)

	// Add workflow_id filter
	idFilter := fmt.Sprintf("workflow_id = $%d", len(args)+1)
	args = append(args, workflowID)

	fullWhere := idFilter
	if whereClause != "" {
		fullWhere = idFilter + " AND " + whereClause
	}

	query := fmt.Sprintf(`
		SELECT * FROM remediation_workflow_catalog
		WHERE %s
	`, fullWhere)

	var wf models.RemediationWorkflow
	err := r.db.GetContext(ctx, &wf, query, args...)
	if err == sql.ErrNoRows {
		// Security gate: workflow exists but doesn't match context, or doesn't exist
		// We intentionally don't distinguish these cases (DD-WORKFLOW-016: prevent info leakage)
		return nil, nil
	}
	if err != nil {
		r.logger.Error(err, "failed to get workflow with context filters",
			"workflow_id", workflowID)
		return nil, fmt.Errorf("failed to get workflow with context filters: %w", err)
	}

	return &wf, nil
}

// buildContextFilterSQL builds a WHERE clause from WorkflowDiscoveryFilters.
// Returns the SQL fragment and positional parameter args.
// The SQL uses positional parameters ($1, $2, ...) starting from $1.
//
// Shared across all three discovery methods (REFACTOR: extracted per TDD methodology).
//
// DD-WORKFLOW-016 v2.1: Label values in OCI workflow schemas can be either
// scalar strings (e.g., "high") or JSON arrays (e.g., ["low", "medium"]).
// The SQL must handle both types using CASE WHEN jsonb_typeof() checks.
// Component comparison is case-insensitive (Kubernetes Kind is PascalCase,
// but OCI labels store lowercase).
func buildContextFilterSQL(filters *models.WorkflowDiscoveryFilters) (string, []interface{}) {
	if filters == nil {
		return "", nil
	}

	var conditions []string
	var args []interface{}
	argIdx := 1

	// Mandatory label filters (JSONB queries on labels column)
	// Severity is always JSONB array (e.g. ["critical","high"]), use ? operator
	if filters.Severity != "" {
		conditions = append(conditions, fmt.Sprintf("labels->'severity' ? $%d", argIdx))
		args = append(args, filters.Severity)
		argIdx++
	}

	if filters.Component != "" {
		// DD-WORKFLOW-016 v2.1: Case-insensitive component matching.
		// Kubernetes resource Kind is PascalCase (e.g., "Deployment"),
		// but OCI workflow labels store lowercase (e.g., "deployment").
		conditions = append(conditions, fmt.Sprintf(
			"(LOWER(labels->>'component') = LOWER($%d) OR labels->>'component' = '*')", argIdx))
		args = append(args, filters.Component)
		argIdx++
	}

	if filters.Environment != "" {
		// DD-WORKFLOW-001 v2.5: environment is JSONB array, use ? operator; supports "*" wildcard per OpenAPI spec
		conditions = append(conditions, fmt.Sprintf("(labels->'environment' ? $%d OR labels->'environment' ? '*')", argIdx))
		args = append(args, filters.Environment)
		argIdx++
	}

	if filters.Priority != "" {
		// DD-WORKFLOW-016 v2.1: Handle both scalar and array JSONB values
		conditions = append(conditions, fmt.Sprintf(`(
			CASE WHEN jsonb_typeof(labels->'priority') = 'array'
				THEN labels->'priority' ? $%d
				ELSE labels->>'priority' = $%d OR labels->>'priority' = '*'
			END
		)`, argIdx, argIdx))
		args = append(args, filters.Priority)
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return strings.Join(conditions, " AND "), args
}

// ActionTypeExists checks whether the given action type is in the action_type_taxonomy table.
// DD-WORKFLOW-016 GAP-4: Explicit validation before DB FK constraint for clean 400 errors.
func (r *Repository) ActionTypeExists(ctx context.Context, actionType string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM action_type_taxonomy WHERE action_type = $1)",
		actionType,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("action type taxonomy lookup: %w", err)
	}
	return exists, nil
}
