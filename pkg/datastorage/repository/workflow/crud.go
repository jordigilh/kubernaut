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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sqlbuilder "github.com/jordigilh/kubernaut/pkg/datastorage/repository/sql"
)

// ========================================
// CREATE OPERATIONS
// ========================================

// Create inserts a new workflow into the catalog
// BR-STORAGE-012: Workflow catalog persistence
// V1.0: Embedding generation removed (label-only search)
// DD-WORKFLOW-002 v3.0: Handles is_latest_version flag within a transaction
func (r *Repository) Create(ctx context.Context, workflow *models.RemediationWorkflow) error {
	// V1.0: Embeddings no longer generated (label-only search)
	// Authority: CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md (92% confidence)

	// DD-WORKFLOW-002 v3.0: Use transaction to ensure is_latest_version consistency
	// When creating a new version, mark all previous versions as not latest
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		r.logger.Error(err, "failed to begin transaction",
			"workflow_name", workflow.WorkflowName,
			"version", workflow.Version)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// DD-WORKFLOW-002 v3.0: Mark previous versions as not latest
	// This ensures only one version per workflow_name has is_latest_version=true
	if workflow.IsLatestVersion {
		if err = r.markPreviousVersionsNotLatest(ctx, tx, workflow); err != nil {
			return err
		}
	}

	// Issue #548: When WorkflowID is pre-set (deterministic UUID from content hash),
	// include it in the INSERT. Otherwise fall back to DB-generated UUID for safety.
	insertQuery, args := buildCreateInsert(workflow)

	var confirmedID string
	err = tx.QueryRowContext(ctx, insertQuery, args...).Scan(&confirmedID)
	if err != nil {
		r.logger.Error(err, "failed to create workflow",
			"workflow_name", workflow.WorkflowName,
			"version", workflow.Version,
			"error", err)
		return fmt.Errorf("failed to create workflow: %w", err)
	}
	workflow.WorkflowID = confirmedID

	// Commit transaction
	if err = tx.Commit(); err != nil {
		r.logger.Error(err, "failed to commit transaction",
			"workflow_name", workflow.WorkflowName,
			"version", workflow.Version)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	r.logger.Info("workflow created",
		"workflow_id", workflow.WorkflowID,
		"workflow_name", workflow.WorkflowName,
		"version", workflow.Version,
		"is_latest_version", workflow.IsLatestVersion)

	return nil
}

// markPreviousVersionsNotLatest clears is_latest_version on any prior row for
// workflow.WorkflowName within tx. Extracted from Create (Wave 6 6f GREEN:
// funlen remediation) — pure code motion, no behavior change.
func (r *Repository) markPreviousVersionsNotLatest(ctx context.Context, tx *sqlx.Tx, workflow *models.RemediationWorkflow) error {
	updateQuery := `
		UPDATE remediation_workflow_catalog
		SET is_latest_version = false, updated_at = NOW()
		WHERE workflow_name = $1 AND is_latest_version = true
	`
	result, err := tx.ExecContext(ctx, updateQuery, workflow.WorkflowName)
	if err != nil {
		r.logger.Error(err, "failed to update previous versions",
			"workflow_name", workflow.WorkflowName,
			"version", workflow.Version)
		return fmt.Errorf("failed to update previous versions: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		r.logger.Info("marked previous versions as not latest",
			"workflow_name", workflow.WorkflowName,
			"versions_updated", rowsAffected)
	}
	return nil
}

// buildCreateInsert builds the INSERT statement and positional args for Create.
// Extracted from Create (Wave 6 6f GREEN: funlen remediation) — pure code motion,
// no behavior change. Two shapes: with a pre-set WorkflowID (Issue #548,
// deterministic UUID from content hash) vs. DB-generated UUID.
func buildCreateInsert(workflow *models.RemediationWorkflow) (string, []interface{}) {
	if workflow.WorkflowID != "" {
		insertQuery := `
			INSERT INTO remediation_workflow_catalog (
				workflow_id,
				workflow_name, version, schema_version, name, description, owner, maintainer,
				content, content_hash, parameters, execution_engine, schema_image, schema_digest,
				execution_bundle, execution_bundle_digest, engine_config,
				labels, custom_labels, detected_labels, status,
				is_latest_version, previous_version, version_notes, change_summary,
				approved_by, approved_at, expected_success_rate, expected_duration_seconds,
				created_by, action_type, service_account_name
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8,
				$9, $10, $11, $12, $13, $14,
				$15, $16, $17,
				$18, $19, $20, $21,
				$22, $23, $24, $25,
				$26, $27, $28, $29,
				$30, $31, $32
			)
			RETURNING workflow_id
		`
		args := append([]interface{}{workflow.WorkflowID}, createInsertCommonArgs(workflow)...)
		return insertQuery, args
	}

	insertQuery := `
		INSERT INTO remediation_workflow_catalog (
			workflow_name, version, schema_version, name, description, owner, maintainer,
			content, content_hash, parameters, execution_engine, schema_image, schema_digest,
			execution_bundle, execution_bundle_digest, engine_config,
			labels, custom_labels, detected_labels, status,
			is_latest_version, previous_version, version_notes, change_summary,
			approved_by, approved_at, expected_success_rate, expected_duration_seconds,
			created_by, action_type, service_account_name
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16,
			$17, $18, $19, $20,
			$21, $22, $23, $24,
			$25, $26, $27, $28,
			$29, $30, $31
		)
		RETURNING workflow_id
	`
	return insertQuery, createInsertCommonArgs(workflow)
}

// createInsertCommonArgs returns the 31 positional args shared by both
// buildCreateInsert branches (with/without a leading pre-set WorkflowID).
// Extracted from buildCreateInsert (Wave 6 6f GREEN: funlen remediation) —
// pure code motion, no behavior change.
func createInsertCommonArgs(workflow *models.RemediationWorkflow) []interface{} {
	return []interface{}{
		workflow.WorkflowName, workflow.Version, workflow.SchemaVersion, workflow.Name, workflow.Description, workflow.Owner, workflow.Maintainer,
		workflow.Content, workflow.ContentHash, workflow.Parameters, workflow.ExecutionEngine, workflow.SchemaImage, workflow.SchemaDigest,
		workflow.ExecutionBundle, workflow.ExecutionBundleDigest, workflow.EngineConfig,
		workflow.Labels, workflow.CustomLabels, workflow.DetectedLabels, workflow.Status,
		workflow.IsLatestVersion, workflow.PreviousVersion, workflow.VersionNotes, workflow.ChangeSummary,
		workflow.ApprovedBy, workflow.ApprovedAt, workflow.ExpectedSuccessRate, workflow.ExpectedDurationSeconds,
		workflow.CreatedBy, workflow.ActionType, workflow.ServiceAccountName,
	}
}

// SupersedeAndCreate atomically marks the old workflow as Superseded and inserts
// the new one in a single transaction. This eliminates the visibility gap (#707)
// where the old workflow is already Superseded but the new one doesn't exist yet,
// causing ListWorkflowsByActionType to return zero results.
func (r *Repository) SupersedeAndCreate(ctx context.Context, oldID, oldVersion, reason string, newWorkflow *models.RemediationWorkflow) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = supersedeWorkflowVersion(ctx, tx, r.logger, oldID, oldVersion, reason); err != nil {
		return err
	}

	if newWorkflow.IsLatestVersion {
		if err = demoteExistingLatestVersion(ctx, tx, newWorkflow.WorkflowName); err != nil {
			return err
		}
	}

	insertQuery, args := buildSupersedeInsert(newWorkflow)

	var confirmedID string
	var reactivated bool
	confirmedID, reactivated, err = insertOrReactivateWorkflow(ctx, tx, newWorkflow, insertQuery, args)
	if err != nil {
		return err
	}
	newWorkflow.WorkflowID = confirmedID

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if reactivated {
		r.logger.Info("workflow re-activated from Superseded state (PK collision recovery)",
			"workflow_id", newWorkflow.WorkflowID,
			"workflow_name", newWorkflow.WorkflowName,
			"version", newWorkflow.Version)
	} else {
		r.logger.Info("workflow superseded and new version created",
			"old_workflow_id", oldID,
			"new_workflow_id", newWorkflow.WorkflowID,
			"workflow_name", newWorkflow.WorkflowName,
			"version", newWorkflow.Version)
	}

	return nil
}

// supersedeWorkflowVersion marks the workflow at (oldID, oldVersion) as
// Superseded within tx. A zero-rows-affected result is non-fatal (logged
// only) since the target may already have been superseded by a concurrent
// request. Extracted from SupersedeAndCreate (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3) — pure code motion, no behavior change.
func supersedeWorkflowVersion(ctx context.Context, tx *sqlx.Tx, logger logr.Logger, oldID, oldVersion, reason string) error {
	supersedeQuery := `
		UPDATE remediation_workflow_catalog
		SET status = $1, status_reason = $2, updated_at = NOW()
		WHERE workflow_id = $3 AND version = $4
	`
	result, err := tx.ExecContext(ctx, supersedeQuery, "Superseded", reason, oldID, oldVersion)
	if err != nil {
		return fmt.Errorf("failed to supersede workflow %s: %w", oldID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		logger.Info("WARNING: supersede target not found",
			"workflow_id", oldID, "version", oldVersion)
	}
	return nil
}

// demoteExistingLatestVersion clears is_latest_version on any other workflow
// row sharing workflowName, so the new version can become the sole latest.
// Extracted from SupersedeAndCreate (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3)
// — pure code motion, no behavior change.
func demoteExistingLatestVersion(ctx context.Context, tx *sqlx.Tx, workflowName string) error {
	latestQuery := `
		UPDATE remediation_workflow_catalog
		SET is_latest_version = false, updated_at = NOW()
		WHERE workflow_name = $1 AND is_latest_version = true
	`
	if _, err := tx.ExecContext(ctx, latestQuery, workflowName); err != nil {
		return fmt.Errorf("failed to update previous versions: %w", err)
	}
	return nil
}

// buildSupersedeInsert builds the INSERT INTO remediation_workflow_catalog
// statement and its positional args for newWorkflow. Identical column/arg
// shape to Create's INSERT, so this delegates to buildCreateInsert (Wave 6 6f
// GREEN: deduped the two byte-for-byte-identical query builders instead of
// just shrinking each independently) — pure code motion, no behavior change.
func buildSupersedeInsert(newWorkflow *models.RemediationWorkflow) (string, []interface{}) {
	return buildCreateInsert(newWorkflow)
}

// insertOrReactivateWorkflow runs insertQuery within tx, using a SAVEPOINT to
// recover from a PK collision on the deterministic workflow_id: if the same
// content was previously registered and its row still exists in the
// Superseded state, it is re-activated in place instead of failing the
// request. Extracted from SupersedeAndCreate (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3) — pure code motion, no behavior change.
func insertOrReactivateWorkflow(ctx context.Context, tx *sqlx.Tx, newWorkflow *models.RemediationWorkflow, insertQuery string, args []interface{}) (string, bool, error) {
	// SAVEPOINT allows recovery from PK collision without aborting the whole
	// transaction. PostgreSQL marks a tx as aborted after any statement error;
	// ROLLBACK TO SAVEPOINT clears that state so subsequent statements can run.
	if _, err := tx.ExecContext(ctx, "SAVEPOINT before_insert"); err != nil {
		return "", false, fmt.Errorf("failed to set savepoint: %w", err)
	}

	var confirmedID string
	if err := tx.QueryRowContext(ctx, insertQuery, args...).Scan(&confirmedID); err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "remediation_workflow_catalog_pkey" {
			return "", false, fmt.Errorf("failed to create workflow: %w", err)
		}

		// Roll back to the savepoint to clear the aborted transaction state.
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT before_insert"); err != nil {
			return "", false, fmt.Errorf("failed to rollback to savepoint: %w", err)
		}

		// PK collision on deterministic UUID: the same content was previously registered
		// and its row still exists (Superseded). Re-activate it within this transaction.
		reactivateQuery := `
			UPDATE remediation_workflow_catalog
			SET status = 'Active', is_latest_version = $1, updated_at = NOW(),
			    status_reason = 'reactivated: re-registered via CRD'
			WHERE workflow_id = $2 AND status = 'Superseded'
		`
		result, err := tx.ExecContext(ctx, reactivateQuery, newWorkflow.IsLatestVersion, newWorkflow.WorkflowID)
		if err != nil {
			return "", false, fmt.Errorf("failed to re-activate workflow %s: %w", newWorkflow.WorkflowID, err)
		}
		rowsReactivated, _ := result.RowsAffected()
		if rowsReactivated == 0 {
			return "", false, fmt.Errorf("PK collision on workflow_id=%s but row is not Superseded — cannot re-activate", newWorkflow.WorkflowID)
		}

		return newWorkflow.WorkflowID, true, nil
	}

	return confirmedID, false, nil
}

// ========================================
// READ OPERATIONS
// ========================================

// GetByID retrieves a workflow by UUID (primary key)
// DD-WORKFLOW-002 v3.0: workflow_id is the sole UUID primary key
func (r *Repository) GetByID(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT ` + workflowCatalogColumns + ` FROM remediation_workflow_catalog
		WHERE workflow_id = $1
	`

	var workflow models.RemediationWorkflow
	err := r.db.GetContext(ctx, &workflow, query, workflowID)
	if errors.Is(err, sql.ErrNoRows) {
		// nolint:nilnil // intentional "not found" sentinel, not an error —
		// canonical repository idiom; all callers already guard with
		// `if x != nil` before use (Issue #1546 Tier 2).
		return nil, nil // Not found
	}
	if err != nil {
		r.logger.Error(err, "failed to get workflow by ID",
			"workflow_id", workflowID)
		return nil, fmt.Errorf("failed to get workflow by ID: %w", err)
	}

	return &workflow, nil
}

// GetByNameAndVersion retrieves a workflow by workflow_name and version
// DD-WORKFLOW-002 v3.0: workflow_name is the human-readable identifier
func (r *Repository) GetByNameAndVersion(ctx context.Context, workflowName, version string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT ` + workflowCatalogColumns + ` FROM remediation_workflow_catalog
		WHERE workflow_name = $1 AND version = $2
	`

	var workflow models.RemediationWorkflow
	err := r.db.GetContext(ctx, &workflow, query, workflowName, version)
	if errors.Is(err, sql.ErrNoRows) {
		// nolint:nilnil // intentional "not found" sentinel, not an error —
		// canonical repository idiom; all callers already guard with
		// `if x != nil` before use (Issue #1546 Tier 2).
		return nil, nil // Not found
	}
	if err != nil {
		r.logger.Error(err, "failed to get workflow by name and version",
			"workflow_name", workflowName,
			"version", version)
		return nil, fmt.Errorf("failed to get workflow by name and version: %w", err)
	}

	return &workflow, nil
}

// GetActiveByNameAndVersion retrieves an active workflow by name and version.
// BR-WORKFLOW-006: Used by content integrity check to detect idempotent re-apply vs supersede.
func (r *Repository) GetActiveByNameAndVersion(ctx context.Context, workflowName, version string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT ` + workflowCatalogColumns + ` FROM remediation_workflow_catalog
		WHERE workflow_name = $1 AND version = $2 AND status = 'Active'
	`

	var wf models.RemediationWorkflow
	err := r.db.GetContext(ctx, &wf, query, workflowName, version)
	if errors.Is(err, sql.ErrNoRows) {
		// nolint:nilnil // intentional "not found" sentinel, not an error —
		// canonical repository idiom; all callers already guard with
		// `if x != nil` before use (Issue #1546 Tier 2).
		return nil, nil
	}
	if err != nil {
		r.logger.Error(err, "failed to get active workflow by name and version",
			"workflow_name", workflowName, "version", version)
		return nil, fmt.Errorf("failed to get active workflow: %w", err)
	}
	return &wf, nil
}

// GetLatestDisabledByNameAndVersion retrieves the most recently disabled workflow
// by name and version. BR-WORKFLOW-006: Used by content integrity check to decide
// between re-enable (same hash) and create-new (different hash).
func (r *Repository) GetLatestDisabledByNameAndVersion(ctx context.Context, workflowName, version string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT ` + workflowCatalogColumns + ` FROM remediation_workflow_catalog
		WHERE workflow_name = $1 AND version = $2 AND status = 'Disabled'
		ORDER BY updated_at DESC
		LIMIT 1
	`

	var wf models.RemediationWorkflow
	err := r.db.GetContext(ctx, &wf, query, workflowName, version)
	if errors.Is(err, sql.ErrNoRows) {
		// nolint:nilnil // intentional "not found" sentinel, not an error —
		// canonical repository idiom; all callers already guard with
		// `if x != nil` before use (Issue #1546 Tier 2).
		return nil, nil
	}
	if err != nil {
		r.logger.Error(err, "failed to get disabled workflow by name and version",
			"workflow_name", workflowName, "version", version)
		return nil, fmt.Errorf("failed to get disabled workflow: %w", err)
	}
	return &wf, nil
}

// GetActiveByWorkflowName retrieves any active workflow entry for a given workflow_name,
// regardless of version. Issue #371, BR-WORKFLOW-006: Used by handleDuplicateWorkflow
// for cross-version supersession when a new version of an existing workflow is registered.
func (r *Repository) GetActiveByWorkflowName(ctx context.Context, workflowName string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT ` + workflowCatalogColumns + ` FROM remediation_workflow_catalog
		WHERE workflow_name = $1 AND status = 'Active'
		ORDER BY created_at DESC
		LIMIT 1
	`

	var wf models.RemediationWorkflow
	err := r.db.GetContext(ctx, &wf, query, workflowName)
	if errors.Is(err, sql.ErrNoRows) {
		// nolint:nilnil // intentional "not found" sentinel, not an error —
		// canonical repository idiom; all callers already guard with
		// `if x != nil` before use (Issue #1546 Tier 2).
		return nil, nil
	}
	if err != nil {
		r.logger.Error(err, "failed to get active workflow by name",
			"workflow_name", workflowName)
		return nil, fmt.Errorf("failed to get active workflow by name: %w", err)
	}
	return &wf, nil
}

// GetLatestVersion retrieves the latest version of a workflow by workflow_name
// DD-WORKFLOW-002 v3.0: Uses is_latest_version flag for efficient lookup
func (r *Repository) GetLatestVersion(ctx context.Context, workflowName string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT ` + workflowCatalogColumns + ` FROM remediation_workflow_catalog
		WHERE workflow_name = $1 AND is_latest_version = true
	`

	var workflow models.RemediationWorkflow
	err := r.db.GetContext(ctx, &workflow, query, workflowName)
	if errors.Is(err, sql.ErrNoRows) {
		// nolint:nilnil // intentional "not found" sentinel, not an error —
		// canonical repository idiom; all callers already guard with
		// `if x != nil` before use (Issue #1546 Tier 2).
		return nil, nil // Not found
	}
	if err != nil {
		r.logger.Error(err, "failed to get latest workflow version",
			"workflow_name", workflowName)
		return nil, fmt.Errorf("failed to get latest workflow version: %w", err)
	}

	return &workflow, nil
}

// GetVersionsByName retrieves all versions of a workflow by workflow_name
// DD-WORKFLOW-002 v3.0: Returns all versions ordered by created_at DESC
func (r *Repository) GetVersionsByName(ctx context.Context, workflowName string) ([]models.RemediationWorkflow, error) {
	query := `
		SELECT ` + workflowCatalogColumns + ` FROM remediation_workflow_catalog
		WHERE workflow_name = $1
		ORDER BY created_at DESC, workflow_id ASC
	`

	var workflows []models.RemediationWorkflow
	err := r.db.SelectContext(ctx, &workflows, query, workflowName)
	if err != nil {
		r.logger.Error(err, "failed to get workflow versions",
			"workflow_name", workflowName)
		return nil, fmt.Errorf("failed to get workflow versions: %w", err)
	}

	return workflows, nil
}

// List retrieves workflows with optional filtering and pagination
// BR-STORAGE-012: Workflow catalog listing
// V1.0 REFACTOR: Uses SQL builder for type-safe query construction
func (r *Repository) List(ctx context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error) {
	builder := sqlbuilder.NewBuilder().
		Select(workflowCatalogColumns).
		From("remediation_workflow_catalog")

	// Apply filters if provided
	applyListFilters(builder, filters)

	// Get total count first (without pagination)
	countQuery, countArgs := builder.BuildCount()
	var totalCount int
	err := r.db.GetContext(ctx, &totalCount, countQuery, countArgs...)
	if err != nil {
		r.logger.Error(err, "failed to count workflows")
		return nil, 0, fmt.Errorf("failed to count workflows: %w", err)
	}

	// Add pagination and ordering
	builder.OrderBy("created_at", sqlbuilder.DESC).
		OrderBy("workflow_id", sqlbuilder.ASC).
		Limit(limit).
		Offset(offset)

	// Execute main query
	query, args := builder.Build()
	var workflows []models.RemediationWorkflow
	err = r.db.SelectContext(ctx, &workflows, query, args...)
	if err != nil {
		r.logger.Error(err, "failed to list workflows")
		return nil, 0, fmt.Errorf("failed to list workflows: %w", err)
	}

	return workflows, totalCount, nil
}

// applyListFilters applies List's optional metadata/label filters to builder.
// Extracted from List (Wave 6 6f GREEN: nestif remediation) — pure code
// motion, no behavior change; each filter remains an independent guard clause
// rather than nested inside a shared `if filters != nil` block.
func applyListFilters(builder *sqlbuilder.Builder, filters *models.WorkflowSearchFilters) {
	if filters == nil {
		return
	}

	// Metadata filters (exact match on workflow columns)
	// Authority: DD-API-001 (OpenAPI client mandatory - workflow_name filter added Jan 2026)
	if filters.WorkflowName != "" {
		builder.Where("workflow_name = ?", filters.WorkflowName)
	}

	// Label filters (JSONB queries)
	// Issue #522: Wildcard parity with discovery path (buildContextFilterSQL).
	// Stored labels may use "*" to match any query value; the SQL must include
	// wildcard fallback conditions identical to the discovery path.
	if filters.Severity != "" {
		// DD-WORKFLOW-001 v2.8: severity supports "*" wildcard (like environment/priority)
		builder.WhereRaw(fmt.Sprintf("(labels->'severity' ? $%d OR labels->'severity' ? '*')", builder.CurrentArgIndex()), filters.Severity)
	}
	if filters.Component != "" {
		// DD-WORKFLOW-016 v2.1: Case-insensitive + wildcard "*"
		// Issue #790: component is now a JSONB array (like severity/environment).
		// Guard with jsonb_typeof to handle legacy scalar values that would crash
		// jsonb_array_elements_text (ERROR: cannot extract elements from a scalar).
		idx := builder.CurrentArgIndex()
		builder.WhereRaw(fmt.Sprintf(`(CASE WHEN jsonb_typeof(labels->'component') = 'array'
			THEN EXISTS (SELECT 1 FROM jsonb_array_elements_text(labels->'component') elem WHERE LOWER(elem) = LOWER($%d)) OR labels->'component' ? '*'
			ELSE LOWER(labels->>'component') = LOWER($%d) OR labels->>'component' = '*'
		END)`, idx, idx), filters.Component)
	}
	// DD-WORKFLOW-001 v2.5: environment is JSONB array, use ? operator; supports "*" wildcard per OpenAPI spec
	if filters.Environment != "" {
		builder.WhereRaw(fmt.Sprintf("(labels->'environment' ? $%d OR labels->'environment' ? '*')", builder.CurrentArgIndex()), filters.Environment)
	}
	if filters.Priority != "" {
		// DD-WORKFLOW-016 v2.1: Handle both scalar and array JSONB values + wildcard
		idx := builder.CurrentArgIndex()
		builder.WhereRaw(fmt.Sprintf(`(CASE WHEN jsonb_typeof(labels->'priority') = 'array'
			THEN labels->'priority' ? $%d OR labels->'priority' ? '*'
			ELSE labels->>'priority' = $%d OR labels->>'priority' = '*'
		END)`, idx, idx), filters.Priority)
	}
	if len(filters.Status) > 0 {
		builder.WhereRaw(fmt.Sprintf("status = ANY($%d)", builder.CurrentArgIndex()), filters.Status)
	}
}

// ========================================
// UPDATE OPERATIONS
// ========================================

// UpdateSuccessMetrics updates workflow success metrics
// BR-STORAGE-015: Track workflow success rate
func (r *Repository) UpdateSuccessMetrics(ctx context.Context, workflowID, version string, totalExecutions, successfulExecutions int) error {
	query := `
		UPDATE remediation_workflow_catalog
		SET
			total_executions = $1,
			successful_executions = $2,
			actual_success_rate = CASE
				WHEN $1 > 0 THEN CAST($2 AS FLOAT) / $1
				ELSE 0
			END,
			last_executed_at = NOW(),
			updated_at = NOW()
		WHERE workflow_id = $3 AND version = $4
	`

	result, err := r.db.ExecContext(ctx, query, totalExecutions, successfulExecutions, workflowID, version)
	if err != nil {
		r.logger.Error(err, "failed to update workflow success metrics",
			"workflow_id", workflowID,
			"version", version)
		return fmt.Errorf("failed to update workflow success metrics: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: workflow_id=%s version=%s", workflowID, version)
	}

	r.logger.Info("workflow success metrics updated",
		"workflow_id", workflowID,
		"version", version,
		"total_executions", totalExecutions,
		"successful_executions", successfulExecutions)

	return nil
}

// UpdateStatus updates workflow status
// BR-STORAGE-016: Workflow status management
func (r *Repository) UpdateStatus(ctx context.Context, workflowID, version, status, reason, updatedBy string) error {
	// Simpler approach: use different queries for different status transitions
	var query string
	var args []interface{}

	if status == "Disabled" {
		// When transitioning to disabled, set all lifecycle fields
		query = `
			UPDATE remediation_workflow_catalog
			SET
				status = $1,
				status_reason = $2,
				updated_by = $3,
				updated_at = NOW(),
				disabled_at = NOW(),
				disabled_by = $3,
				disabled_reason = $2
			WHERE workflow_id = $4 AND version = $5
		`
		args = []interface{}{status, reason, updatedBy, workflowID, version}
	} else {
		// For other status transitions, just update status and metadata
		query = `
			UPDATE remediation_workflow_catalog
			SET
				status = $1,
				status_reason = $2,
				updated_by = $3,
				updated_at = NOW()
			WHERE workflow_id = $4 AND version = $5
		`
		args = []interface{}{status, reason, updatedBy, workflowID, version}
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		r.logger.Error(err, "failed to update workflow status",
			"workflow_id", workflowID,
			"version", version,
			"status", status)
		return fmt.Errorf("failed to update workflow status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: workflow_id=%s version=%s", workflowID, version)
	}

	r.logger.Info("workflow status updated",
		"workflow_id", workflowID,
		"version", version,
		"status", status,
		"reason", reason)

	return nil
}
