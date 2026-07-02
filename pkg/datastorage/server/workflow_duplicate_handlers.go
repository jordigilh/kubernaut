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

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	deterministicuuid "github.com/jordigilh/kubernaut/pkg/datastorage/uuid"
)

// ========================================
// WORKFLOW CATALOG HANDLERS — DUPLICATE / CONTENT-INTEGRITY RESOLUTION
// ========================================
// BR-WORKFLOW-006: ContentHash-based duplicate detection prevents spec
// tampering. Split from workflow_handlers.go (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change); see workflow_create_handlers.go
// for the POST /api/v1/workflows entry point that calls into this file.

// duplicateResult holds the outcome of content integrity checking.
type duplicateResult struct {
	workflow   *models.RemediationWorkflow
	statusCode int
}

// handleDuplicateWorkflow implements ContentHash-based duplicate detection.
// BR-WORKFLOW-006: Determines the correct action when a workflow with the same
// name+version already exists: idempotent return, supersede, re-enable, or create new.
//
// Decision tree:
//
//	active  + same hash    → 200 (idempotent, no DB writes)
//	active  + diff hash    → 409 contentIntegrityError (must bump version — Issue #773)
//	disabled + same hash   → re-enable → 200
//	disabled + diff hash   → create new → 201
//	cross-version (any)    → supersede old → create new → 201
//	none                   → create new → 201
//
// Concurrency safety: All Create calls handle PostgreSQL 23505 (unique constraint
// violation on uq_workflow_name_version_active) by retrying the active-lookup.
// This handles the race where two concurrent requests both pass the initial lookup,
// one wins the INSERT, and the other must discover the winner's committed row.
func (h *Handler) handleDuplicateWorkflow(ctx context.Context, workflow *models.RemediationWorkflow) (*duplicateResult, error) {
	repo := h.workflowIntegrityRepo
	incomingHash := computeContentHash(workflow.Content)
	workflow.ContentHash = incomingHash
	// Issue #548: Derive workflow_id deterministically from content hash so that
	// re-registering the same CRD content after a PVC wipe recovers the original ID.
	workflow.WorkflowID = deterministicuuid.DeterministicUUID(incomingHash)

	active, err := repo.GetActiveByNameAndVersion(ctx, workflow.WorkflowName, workflow.Version)
	if err != nil {
		return nil, fmt.Errorf("lookup active workflow: %w", err)
	}
	if active != nil {
		return h.resolveActiveVersionDuplicate(active, incomingHash)
	}

	disabled, err := repo.GetLatestDisabledByNameAndVersion(ctx, workflow.WorkflowName, workflow.Version)
	if err != nil {
		return nil, fmt.Errorf("lookup disabled workflow: %w", err)
	}
	if disabled != nil {
		return h.resolveDisabledVersionDuplicate(ctx, disabled, workflow, incomingHash)
	}

	// Issue #371: Cross-version check — look for any active entry with the same
	// workflow_name (regardless of version). If found, the incoming request is a
	// version upgrade and the old entry must be superseded to enforce single-active
	// per (workflow_name, action_type).
	activeAnyVersion, err := repo.GetActiveByWorkflowName(ctx, workflow.WorkflowName)
	if err != nil {
		return nil, fmt.Errorf("lookup active workflow by name: %w", err)
	}
	if activeAnyVersion != nil {
		return h.supersedeCrossVersionWorkflow(ctx, activeAnyVersion, workflow, incomingHash)
	}

	if err := repo.Create(ctx, workflow); err != nil {
		if result, handled := h.retryOnUniqueViolation(ctx, err, workflow, incomingHash); handled {
			return result, nil
		}
		return nil, fmt.Errorf("create new workflow: %w", err)
	}
	return &duplicateResult{workflow: workflow, statusCode: http.StatusCreated}, nil
}

// resolveActiveVersionDuplicate handles the case where an Active workflow
// already exists for the same name+version: same content hash is an
// idempotent re-apply (200), differing hash is a content-integrity violation
// (409) requiring a version bump.
func (h *Handler) resolveActiveVersionDuplicate(active *models.RemediationWorkflow, incomingHash string) (*duplicateResult, error) {
	if active.ContentHash == incomingHash {
		h.logger.Info("Idempotent workflow re-apply (same content hash)",
			"workflow_id", active.WorkflowID,
			"workflow_name", active.WorkflowName,
			"content_hash", incomingHash,
		)
		return &duplicateResult{workflow: active, statusCode: http.StatusOK}, nil
	}

	return nil, &contentIntegrityError{
		WorkflowName: active.WorkflowName,
		Version:      active.Version,
		OldHash:      active.ContentHash,
		NewHash:      incomingHash,
	}
}

// resolveDisabledVersionDuplicate handles the case where the latest entry for
// this name+version is Disabled: same content hash re-enables it in place
// (200), differing hash creates a new active entry alongside it (201).
func (h *Handler) resolveDisabledVersionDuplicate(ctx context.Context, disabled, workflow *models.RemediationWorkflow, incomingHash string) (*duplicateResult, error) {
	repo := h.workflowIntegrityRepo

	if disabled.ContentHash == incomingHash {
		if err := repo.UpdateStatus(ctx, disabled.WorkflowID, disabled.Version, "Active", "re-enabled via CRD re-creation", ""); err != nil {
			return nil, fmt.Errorf("re-enable workflow %s: %w", disabled.WorkflowID, err)
		}
		disabled.Status = "Active"
		disabled.DisabledAt = nil
		disabled.DisabledBy = nil
		disabled.DisabledReason = nil

		h.logger.Info("Disabled workflow re-enabled (same content hash)",
			"workflow_id", disabled.WorkflowID,
			"workflow_name", disabled.WorkflowName,
			"content_hash", incomingHash,
		)
		return &duplicateResult{workflow: disabled, statusCode: http.StatusOK}, nil
	}

	if err := repo.Create(ctx, workflow); err != nil {
		if result, handled := h.retryOnUniqueViolation(ctx, err, workflow, incomingHash); handled {
			return result, nil
		}
		return nil, fmt.Errorf("create new workflow (disabled had different content): %w", err)
	}

	h.logger.Info("New workflow created (disabled had different content hash)",
		"workflow_id", workflow.WorkflowID,
		"workflow_name", workflow.WorkflowName,
		"disabled_id", disabled.WorkflowID,
	)
	return &duplicateResult{workflow: workflow, statusCode: http.StatusCreated}, nil
}

// supersedeCrossVersionWorkflow handles Issue #371: an Active entry exists
// for the same workflow_name under a different version, so the incoming
// request is a version upgrade — the old entry is superseded and the new
// version created in a single transaction.
func (h *Handler) supersedeCrossVersionWorkflow(ctx context.Context, activeAnyVersion, workflow *models.RemediationWorkflow, incomingHash string) (*duplicateResult, error) {
	repo := h.workflowIntegrityRepo
	reason := fmt.Sprintf("superseded: new version %s registered (was %s)", workflow.Version, activeAnyVersion.Version)
	if err := repo.SupersedeAndCreate(ctx, activeAnyVersion.WorkflowID, activeAnyVersion.Version, reason, workflow); err != nil {
		if result, handled := h.retryOnUniqueViolation(ctx, err, workflow, incomingHash); handled {
			return result, nil
		}
		return nil, fmt.Errorf("supersede and create after cross-version update: %w", err)
	}

	h.logger.Info("Workflow superseded due to cross-version update (Issue #371)",
		"old_workflow_id", activeAnyVersion.WorkflowID,
		"old_version", activeAnyVersion.Version,
		"workflow_name", activeAnyVersion.WorkflowName,
		"new_version", workflow.Version,
	)
	return &duplicateResult{workflow: workflow, statusCode: http.StatusCreated}, nil
}

// retryOnUniqueViolation handles PostgreSQL 23505 errors from concurrent Create
// calls by re-querying for the active workflow that won the INSERT race.
// Returns (result, true) if the 23505 was successfully handled, or (nil, false)
// if the error is not a 23505 and should be propagated by the caller.
func (h *Handler) retryOnUniqueViolation(ctx context.Context, createErr error, workflow *models.RemediationWorkflow, incomingHash string) (*duplicateResult, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(createErr, &pgErr) || pgErr.Code != "23505" {
		return nil, false
	}

	h.logger.Info("Concurrent INSERT race detected (23505), retrying active-lookup",
		"workflow_name", workflow.WorkflowName,
		"version", workflow.Version,
		"constraint", pgErr.ConstraintName,
	)

	active, err := h.workflowIntegrityRepo.GetActiveByNameAndVersion(ctx, workflow.WorkflowName, workflow.Version)
	if err != nil {
		h.logger.Error(err, "Retry lookup failed after 23505",
			"workflow_name", workflow.WorkflowName, "version", workflow.Version)
		return nil, false
	}
	if active == nil {
		h.logger.Error(nil, "No active workflow found on retry after 23505 — constraint violation from unexpected source",
			"workflow_name", workflow.WorkflowName, "version", workflow.Version)
		return nil, false
	}

	if active.ContentHash == incomingHash {
		h.logger.Info("Concurrent race resolved: idempotent (same content hash)",
			"workflow_id", active.WorkflowID,
			"workflow_name", active.WorkflowName,
			"content_hash", incomingHash,
		)
		return &duplicateResult{workflow: active, statusCode: http.StatusOK}, true
	}

	h.logger.Info("Concurrent race resolved: different content already active",
		"active_workflow_id", active.WorkflowID,
		"active_hash", active.ContentHash,
		"incoming_hash", incomingHash,
	)
	return &duplicateResult{workflow: active, statusCode: http.StatusConflict}, true
}

// tryReEnableWorkflow attempts to re-enable a previously disabled workflow on duplicate insert.
// Returns the re-enabled workflow on success, or an error if the workflow is active.
func (h *Handler) tryReEnableWorkflow(ctx context.Context, workflow *models.RemediationWorkflow) (*models.RemediationWorkflow, error) {
	repo := h.getWorkflowLifecycleRepo()
	if repo == nil {
		return nil, fmt.Errorf("workflow repository not configured for re-enable")
	}

	existing, err := h.workflowRepo.GetByNameAndVersion(ctx, workflow.WorkflowName, workflow.Version)
	if err != nil {
		return nil, fmt.Errorf("lookup existing workflow: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("workflow not found after conflict")
	}

	if existing.Status != "Disabled" {
		return nil, fmt.Errorf("workflow is %s, not disabled", existing.Status)
	}

	reason := "re-enabled via CRD re-creation"
	if err := repo.UpdateStatus(ctx, existing.WorkflowID, existing.Version, "Active", reason, ""); err != nil {
		return nil, fmt.Errorf("update status to active: %w", err)
	}

	existing.Status = "Active"
	existing.DisabledAt = nil
	existing.DisabledBy = nil
	existing.DisabledReason = nil
	return existing, nil
}

// buildWorkflowFromInlineSchema populates a RemediationWorkflow from inline CRD YAML.
// ADR-058: SchemaImage and SchemaDigest are nil for inline registration.
func (h *Handler) buildWorkflowFromInlineSchema(
	schemaParser *schema.Parser,
	parsedSchema *models.WorkflowSchema,
	rawContent string,
) (*models.RemediationWorkflow, error) {
	return h.buildWorkflowCommon(schemaParser, parsedSchema, rawContent)
}

// buildWorkflowCommon is the shared workflow builder for both OCI and inline registration.
func (h *Handler) buildWorkflowCommon(
	schemaParser *schema.Parser,
	parsedSchema *models.WorkflowSchema,
	rawContent string,
) (*models.RemediationWorkflow, error) {
	extractedParams, err := schemaParser.ExtractParameters(parsedSchema)
	if err != nil {
		return nil, fmt.Errorf("extract parameters: %w", err)
	}
	wrappedParams := map[string]interface{}{
		"schema": map[string]json.RawMessage{
			"parameters": extractedParams,
		},
	}
	wrappedJSON, err := json.Marshal(wrappedParams)
	if err != nil {
		return nil, fmt.Errorf("marshal parameters: %w", err)
	}
	rawParams := json.RawMessage(wrappedJSON)

	labelsJSON, err := schemaParser.ExtractLabels(parsedSchema)
	if err != nil {
		return nil, fmt.Errorf("extract labels: %w", err)
	}

	desc := models.StructuredDescription{
		What:          parsedSchema.Description.What,
		WhenToUse:     parsedSchema.Description.WhenToUse,
		WhenNotToUse:  parsedSchema.Description.WhenNotToUse,
		Preconditions: parsedSchema.Description.Preconditions,
	}

	execEngine := models.ExecutionEngine(schemaParser.ExtractExecutionEngine(parsedSchema))

	workflow := &models.RemediationWorkflow{
		WorkflowName:    parsedSchema.WorkflowName,
		Version:         parsedSchema.Version,
		SchemaVersion:   parsedSchema.SchemaVersion,
		Name:            parsedSchema.WorkflowName,
		Description:     desc,
		Content:         rawContent,
		Parameters:      &rawParams,
		ExecutionEngine: execEngine,
		ActionType:      parsedSchema.ActionType,
		Status:          "Active",
		IsLatestVersion: true,
	}

	if bundle := schemaParser.ExtractExecutionBundle(parsedSchema); bundle != nil {
		workflow.ExecutionBundle = bundle
		if _, digest, err := schema.ParseBundleDigest(*bundle); err == nil {
			workflow.ExecutionBundleDigest = &digest
		}
	}

	workflow.EngineConfig = schemaParser.ExtractEngineConfig(parsedSchema)
	workflow.ServiceAccountName = schemaParser.ExtractServiceAccountName(parsedSchema)

	if err := json.Unmarshal(labelsJSON, &workflow.Labels); err != nil {
		return nil, fmt.Errorf("unmarshal labels: %w", err)
	}

	detectedLabels, err := schemaParser.ExtractDetectedLabels(parsedSchema)
	if err != nil {
		return nil, fmt.Errorf("extract detected labels: %w", err)
	}
	workflow.DetectedLabels = *detectedLabels

	workflow.CustomLabels = schemaParser.ExtractCustomLabels(parsedSchema)
	workflow.ContentHash = computeContentHash(rawContent)

	return workflow, nil
}
