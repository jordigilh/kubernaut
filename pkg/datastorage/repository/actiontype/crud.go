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

package actiontype

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/txretry"
)

var (
	ErrActionTypeNotFound = errors.New("action type not found")
	ErrActionTypeDisabled = errors.New("action type is disabled")
)

// CreateResult describes the outcome of a Create operation.
type CreateResult struct {
	ActionType   *models.ActionTypeTaxonomy
	Status       string // "created", "exists", "reenabled"
	WasReenabled bool
}

// Create inserts a new action type or re-enables a disabled one (idempotent).
// BR-WORKFLOW-007.1: Idempotent CREATE — NOOP if active, re-enable if disabled, create if new.
func (r *Repository) Create(ctx context.Context, actionType string, description models.ActionTypeDescription, registeredBy string) (*CreateResult, error) {
	descJSON, err := json.Marshal(description)
	if err != nil {
		return nil, fmt.Errorf("marshal action type description: %w", err)
	}

	existing, err := r.GetByName(ctx, actionType)
	if err != nil {
		return nil, fmt.Errorf("check existing action type: %w", err)
	}

	if existing != nil {
		if existing.Status == "Active" {
			return &CreateResult{ActionType: existing, Status: "exists", WasReenabled: false}, nil
		}

		// Re-enable: set status to active, clear disabled_at/disabled_by
		_, err := r.db.ExecContext(ctx,
			`UPDATE action_type_taxonomy
			 SET status = 'Active', disabled_at = NULL, disabled_by = NULL, description = $2
			 WHERE action_type = $1`,
			actionType, descJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("re-enable action type %q: %w", actionType, err)
		}
		reenabled, err := r.GetByName(ctx, actionType)
		if err != nil {
			return nil, fmt.Errorf("fetch re-enabled action type: %w", err)
		}
		return &CreateResult{ActionType: reenabled, Status: "reenabled", WasReenabled: true}, nil
	}

	// Insert new
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO action_type_taxonomy (action_type, description, status)
		 VALUES ($1, $2, 'Active')`,
		actionType, descJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("create action type %q: %w", actionType, err)
	}

	created, err := r.GetByName(ctx, actionType)
	if err != nil {
		return nil, fmt.Errorf("fetch created action type: %w", err)
	}
	return &CreateResult{ActionType: created, Status: "created", WasReenabled: false}, nil
}

// GetByName returns the action type by its PascalCase name, or nil if not found.
func (r *Repository) GetByName(ctx context.Context, actionType string) (*models.ActionTypeTaxonomy, error) {
	var at models.ActionTypeTaxonomy
	err := r.db.QueryRowxContext(ctx,
		`SELECT action_type, description, status, disabled_at, disabled_by, created_at, updated_at
		 FROM action_type_taxonomy WHERE action_type = $1`,
		actionType,
	).StructScan(&at)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get action type %q: %w", actionType, err)
	}
	return &at, nil
}

// UpdateDescriptionResult captures old and new descriptions for audit trail.
type UpdateDescriptionResult struct {
	ActionType     *models.ActionTypeTaxonomy
	OldDescription models.ActionTypeDescription
	NewDescription models.ActionTypeDescription
	UpdatedFields  []string
}

// UpdateDescription updates the description JSONB for an active action type.
// BR-WORKFLOW-007.2: Only spec.description is mutable. Returns old+new for audit.
func (r *Repository) UpdateDescription(ctx context.Context, actionType string, newDesc models.ActionTypeDescription) (*UpdateDescriptionResult, error) {
	existing, err := r.GetByName(ctx, actionType)
	if err != nil {
		return nil, fmt.Errorf("fetch action type for update: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: %s", ErrActionTypeNotFound, actionType)
	}
	if existing.Status != "Active" {
		return nil, fmt.Errorf("%w: %s", ErrActionTypeDisabled, actionType)
	}

	var oldDesc models.ActionTypeDescription
	if err := json.Unmarshal(existing.Description, &oldDesc); err != nil {
		return nil, fmt.Errorf("unmarshal existing description: %w", err)
	}

	updatedFields := descriptionDiff(oldDesc, newDesc)
	if len(updatedFields) == 0 {
		return &UpdateDescriptionResult{
			ActionType:     existing,
			OldDescription: oldDesc,
			NewDescription: oldDesc,
			UpdatedFields:  nil,
		}, nil
	}

	newDescJSON, err := json.Marshal(newDesc)
	if err != nil {
		return nil, fmt.Errorf("marshal new description: %w", err)
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE action_type_taxonomy SET description = $2 WHERE action_type = $1`,
		actionType, newDescJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("update description for %q: %w", actionType, err)
	}

	updated, err := r.GetByName(ctx, actionType)
	if err != nil {
		return nil, fmt.Errorf("fetch updated action type: %w", err)
	}

	return &UpdateDescriptionResult{
		ActionType:     updated,
		OldDescription: oldDesc,
		NewDescription: newDesc,
		UpdatedFields:  updatedFields,
	}, nil
}

// DisableResult captures the outcome of a disable operation.
type DisableResult struct {
	Disabled bool
	// When Disabled is false, these fields explain why
	DependentWorkflowCount int
	DependentWorkflows     []string
}

// Disable soft-disables an action type if no active workflows reference it.
// BR-WORKFLOW-007.3: Denied if active RemediationWorkflows reference the type.
//
// The operation runs inside a SERIALIZABLE transaction to prevent race conditions
// where concurrent requests read stale workflow counts. If the action type is
// already disabled, the operation is idempotent and returns Disabled: true (matching
// the RW disable pattern in HandleDisableWorkflow).
func (r *Repository) Disable(ctx context.Context, actionType string, disabledBy string) (*DisableResult, error) {
	const maxRetries = 3
	var result *DisableResult
	err := txretry.WithSerializableRetry(ctx, maxRetries, func() error {
		var txErr error
		result, txErr = r.disableOnce(ctx, actionType, disabledBy)
		return txErr
	})
	return result, err
}

// disableOnce runs a single attempt of the serializable disable transaction.
func (r *Repository) disableOnce(ctx context.Context, actionType string, disabledBy string) (*DisableResult, error) {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing models.ActionTypeTaxonomy
	err = tx.QueryRowxContext(ctx,
		`SELECT action_type, description, status, disabled_at, disabled_by, created_at, updated_at
		 FROM action_type_taxonomy WHERE action_type = $1`,
		actionType,
	).StructScan(&existing)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrActionTypeNotFound, actionType)
		}
		return nil, fmt.Errorf("check action type for disable: %w", err)
	}

	if existing.Status != "Active" {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit (already disabled): %w", err)
		}
		return &DisableResult{Disabled: true}, nil
	}

	rows, err := tx.QueryxContext(ctx,
		`SELECT workflow_name FROM remediation_workflow_catalog
		 WHERE action_type = $1 AND status = 'Active'`,
		actionType,
	)
	if err != nil {
		return nil, fmt.Errorf("count active workflows for %q: %w", actionType, err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan workflow name: %w", err)
		}
		names = append(names, name)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow rows: %w", err)
	}

	if len(names) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit (denied): %w", err)
		}
		return &DisableResult{
			Disabled:               false,
			DependentWorkflowCount: len(names),
			DependentWorkflows:     names,
		}, nil
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx,
		`UPDATE action_type_taxonomy
		 SET status = 'Disabled', disabled_at = $2, disabled_by = $3
		 WHERE action_type = $1 AND status = 'Active'`,
		actionType, now, disabledBy,
	)
	if err != nil {
		return nil, fmt.Errorf("disable action type %q: %w", actionType, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit disable: %w", err)
	}
	return &DisableResult{Disabled: true}, nil
}

// ForceDisable disables the specified orphaned workflows and then attempts to
// disable the action type, all within a single SERIALIZABLE transaction.
// Issue #512: Recovers from stale DS entries when K8s RW CRDs are deleted but
// the catalog-cleanup finalizer failed to disable the workflow in DS.
//
// Only the named workflows are disabled (scoped cleanup). If additional active
// workflows exist that are NOT in orphanedWorkflows, the action type remains
// active and DisableResult.Disabled is false.
func (r *Repository) ForceDisable(ctx context.Context, actionType string, disabledBy string, orphanedWorkflows []string) (*DisableResult, error) {
	const maxRetries = 3
	var result *DisableResult
	err := txretry.WithSerializableRetry(ctx, maxRetries, func() error {
		var txErr error
		result, txErr = r.forceDisableOnce(ctx, actionType, disabledBy, orphanedWorkflows)
		return txErr
	})
	return result, err
}

func (r *Repository) forceDisableOnce(ctx context.Context, actionType string, disabledBy string, orphanedWorkflows []string) (*DisableResult, error) {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing models.ActionTypeTaxonomy
	err = tx.QueryRowxContext(ctx,
		`SELECT action_type, description, status, disabled_at, disabled_by, created_at, updated_at
		 FROM action_type_taxonomy WHERE action_type = $1`,
		actionType,
	).StructScan(&existing)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrActionTypeNotFound, actionType)
		}
		return nil, fmt.Errorf("check action type for force-disable: %w", err)
	}

	if existing.Status != "Active" {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit (already disabled): %w", err)
		}
		return &DisableResult{Disabled: true}, nil
	}

	// Disable only the named orphaned workflows (scoped cleanup).
	if len(orphanedWorkflows) > 0 {
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx,
			`UPDATE remediation_workflow_catalog
			 SET status = 'Disabled', disabled_at = $2, disabled_by = $3,
			     disabled_reason = 'orphan cleanup (#512)', status_reason = 'orphan cleanup (#512)'
			 WHERE action_type = $1 AND status = 'Active'
			   AND workflow_name = ANY($4)`,
			actionType, now, disabledBy, orphanedWorkflows,
		)
		if err != nil {
			return nil, fmt.Errorf("disable orphaned workflows for %q: %w", actionType, err)
		}
	}

	// Check for remaining active workflows after orphan cleanup.
	rows, err := tx.QueryxContext(ctx,
		`SELECT workflow_name FROM remediation_workflow_catalog
		 WHERE action_type = $1 AND status = 'Active'`,
		actionType,
	)
	if err != nil {
		return nil, fmt.Errorf("count remaining active workflows for %q: %w", actionType, err)
	}
	var remaining []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan workflow name: %w", err)
		}
		remaining = append(remaining, name)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate remaining workflow rows: %w", err)
	}

	if len(remaining) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit (denied, non-orphaned workflows remain): %w", err)
		}
		return &DisableResult{
			Disabled:               false,
			DependentWorkflowCount: len(remaining),
			DependentWorkflows:     remaining,
		}, nil
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx,
		`UPDATE action_type_taxonomy
		 SET status = 'Disabled', disabled_at = $2, disabled_by = $3
		 WHERE action_type = $1 AND status = 'Active'`,
		actionType, now, disabledBy,
	)
	if err != nil {
		return nil, fmt.Errorf("disable action type %q: %w", actionType, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit force-disable: %w", err)
	}
	return &DisableResult{Disabled: true}, nil
}

// CountActiveWorkflows returns the count and names of active workflows referencing this action type.
func (r *Repository) CountActiveWorkflows(ctx context.Context, actionType string) (int, []string, error) {
	rows, err := r.db.QueryxContext(ctx,
		`SELECT workflow_name FROM remediation_workflow_catalog
		 WHERE action_type = $1 AND status = 'Active'`,
		actionType,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("count active workflows for %q: %w", actionType, err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, nil, fmt.Errorf("scan workflow name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("iterate workflow rows: %w", err)
	}

	return len(names), names, nil
}

// ListActive returns all action types with status='Active'.
// BR-WORKFLOW-007.5: Discovery filtering excludes disabled action types.
func (r *Repository) ListActive(ctx context.Context) ([]models.ActionTypeTaxonomy, error) {
	var types []models.ActionTypeTaxonomy
	err := r.db.SelectContext(ctx, &types,
		`SELECT action_type, description, status, disabled_at, disabled_by, created_at, updated_at
		 FROM action_type_taxonomy WHERE status = 'Active'
		 ORDER BY action_type`)
	if err != nil {
		return nil, fmt.Errorf("list active action types: %w", err)
	}
	return types, nil
}

// ActionTypeExists checks whether the given action type is active in the action_type_taxonomy table.
// DD-WORKFLOW-016 GAP-4: Explicit validation before DB FK constraint for clean 400 errors.
// BR-WORKFLOW-007: Disabled action types are not considered to exist for new workflow references.
func (r *Repository) ActionTypeExists(ctx context.Context, actionType string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM action_type_taxonomy WHERE action_type = $1 AND status = 'Active')",
		actionType,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("action type taxonomy lookup: %w", err)
	}
	return exists, nil
}

// descriptionDiff returns the list of field names that differ between two descriptions.
func descriptionDiff(old, new models.ActionTypeDescription) []string {
	var changed []string
	if old.What != new.What {
		changed = append(changed, "what")
	}
	if old.WhenToUse != new.WhenToUse {
		changed = append(changed, "whenToUse")
	}
	if old.WhenNotToUse != new.WhenNotToUse {
		changed = append(changed, "whenNotToUse")
	}
	if old.Preconditions != new.Preconditions {
		changed = append(changed, "preconditions")
	}
	return changed
}
