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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgconn"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// ========================================
// WORKFLOW CATALOG HANDLERS — CREATE
// ========================================
// Business Requirements:
// - BR-STORAGE-014: Workflow catalog management
// - BR-WORKFLOW-006: Inline workflow schema registration (ADR-058)
//
// This file covers POST /api/v1/workflows: request parsing, schema
// validation, external checks, and persistence. Duplicate/content-integrity
// resolution lives in workflow_duplicate_handlers.go; catalog read/update/
// lifecycle handlers live in workflow_query_handlers.go,
// workflow_update_lifecycle_handlers.go, and workflow_discovery_handlers.go
// (split from a single workflow_handlers.go — GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change).

// createWorkflowRequest is the POST /api/v1/workflows request body (ADR-058
// inline schema registration).
type createWorkflowRequest struct {
	Content      string `json:"content"`
	Source       string `json:"source"`
	RegisteredBy string `json:"registeredBy"`
	SchemaImage  string `json:"schemaImage"` // legacy field — reject if present
}

// HandleCreateWorkflow handles POST /api/v1/workflows
// ADR-058: Inline workflow schema registration (CRD-based)
// BR-WORKFLOW-006: Accept content (raw YAML), parse CRD envelope, validate, populate catalog
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	req, ok := h.parseCreateWorkflowRequest(w, r)
	if !ok {
		return
	}

	// Parse and validate the inline schema
	schemaParser := schema.NewParser()
	parsedSchema, err := schemaParser.ParseAndValidate(req.Content)
	if err != nil {
		h.logger.Error(err, "Inline schema validation failed")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Schema Validation Error",
			"workflow schema validation failed; check the 'content' field for YAML/structural errors", h.logger)
		return
	}

	// Build RemediationWorkflow from parsed schema (inline — no OCI image)
	workflow, err := h.buildWorkflowFromInlineSchema(schemaParser, parsedSchema, req.Content)
	if err != nil {
		h.logger.Error(err, "Failed to build workflow from inline schema")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to process inline schema", h.logger)
		return
	}

	// Validate external checks in parallel (Issue #1070)
	// Typed-result-slot pattern: each check writes to its own error slot,
	// then we check slots in priority order so callers always see the same
	// RFC 7807 error type regardless of goroutine completion order.
	if err := h.validateExternalChecks(r.Context(), schemaParser, parsedSchema, workflow); err != nil {
		err.writeTo(w, h.logger)
		return
	}

	// Create workflow in repository (with content integrity checking)
	// BR-WORKFLOW-006: ContentHash-based duplicate detection prevents spec tampering
	workflow, statusCode, ok := h.persistCreatedWorkflow(w, r, workflow)
	if !ok {
		return
	}

	// Audit workflow creation (synchronous per ADR-032)
	h.auditWorkflowCreation(r.Context(), workflow)

	h.logger.Info("Workflow registered successfully",
		"workflow_id", workflow.WorkflowID,
		"workflow_name", workflow.WorkflowName,
		"version", workflow.Version,
		"source", req.Source,
		"registered_by", req.RegisteredBy,
		"status_code", statusCode,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow create response")
	}
}

// parseCreateWorkflowRequest decodes and validates the request body: size
// limit (2 MiB), legacy schemaImage rejection (ADR-058), and content
// presence. Returns ok=false after writing the RFC 7807 error response.
func (h *Handler) parseCreateWorkflowRequest(w http.ResponseWriter, r *http.Request) (createWorkflowRequest, bool) {
	// Cap request body at 2 MiB to prevent memory exhaustion from oversized payloads.
	const maxBodySize = 2 << 20 // 2 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req createWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, h.logger)
			return req, false
		}
		h.logger.Error(err, "Failed to decode workflow create request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"request body is not valid JSON", h.logger)
		return req, false
	}

	// Reject legacy OCI format — guide to new inline format
	if req.SchemaImage != "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "legacy-format", "Legacy Format Rejected",
			"The 'schemaImage' field is no longer supported. Use 'content' with the raw YAML of the RemediationWorkflow CRD instead (ADR-058).",
			h.logger)
		return req, false
	}

	if req.Content == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"content is required", h.logger)
		return req, false
	}

	return req, true
}

// persistCreatedWorkflow persists a newly-built workflow, preferring the
// content-integrity-aware path (BR-WORKFLOW-006 ContentHash duplicate
// detection) when available, falling back to a plain Create with
// re-enable-on-conflict semantics otherwise. Returns ok=false after writing
// the RFC 7807 error response for any failure.
func (h *Handler) persistCreatedWorkflow(w http.ResponseWriter, r *http.Request, workflow *models.RemediationWorkflow) (*models.RemediationWorkflow, int, bool) {
	statusCode := http.StatusCreated

	if h.workflowIntegrityRepo != nil {
		result, integrityErr := h.handleDuplicateWorkflow(r.Context(), workflow)
		if integrityErr != nil {
			var cie *contentIntegrityError
			if errors.As(integrityErr, &cie) {
				h.logger.Info("Content integrity violation: same version with different content",
					"workflow_name", cie.WorkflowName,
					"version", cie.Version,
				)
				response.WriteRFC7807Error(w, http.StatusConflict, "content-integrity-violation",
					"Content Changed Without Version Bump", cie.Error(), h.logger)
				return nil, 0, false
			}
			h.logger.Error(integrityErr, "Content integrity check failed",
				"workflow_name", workflow.WorkflowName,
				"version", workflow.Version,
			)
			response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
				"Failed to process workflow registration", h.logger)
			return nil, 0, false
		}
		if result != nil {
			workflow = result.workflow
			statusCode = result.statusCode
		}
		return workflow, statusCode, true
	}

	if h.workflowRepo == nil {
		return workflow, statusCode, true
	}

	if err := h.workflowRepo.Create(r.Context(), workflow); err != nil {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			h.logger.Error(err, "Failed to create workflow",
				"workflow_name", workflow.WorkflowName,
				"version", workflow.Version,
			)
			response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
				"Failed to create workflow", h.logger)
			return nil, 0, false
		}

		reEnabled, reEnableErr := h.tryReEnableWorkflow(r.Context(), workflow)
		if reEnableErr != nil {
			h.logger.Error(reEnableErr, "Failed to re-enable workflow",
				"workflow_name", workflow.WorkflowName,
				"version", workflow.Version,
			)
			detail := fmt.Sprintf("Workflow '%s' version '%s' already exists in active state", workflow.WorkflowName, workflow.Version)
			response.WriteRFC7807Error(w, http.StatusConflict, "conflict",
				"Workflow Already Exists", detail, h.logger)
			return nil, 0, false
		}
		if reEnabled != nil {
			workflow = reEnabled
			statusCode = http.StatusOK
		}
	}

	return workflow, statusCode, true
}

// auditWorkflowCreation emits the workflow-created audit event (ADR-032
// synchronous audit). DS-H5: bounded 5s context prevents indefinite blocking
// if the audit store is slow.
func (h *Handler) auditWorkflowCreation(ctx context.Context, workflow *models.RemediationWorkflow) {
	if h.auditStore == nil {
		return
	}

	auditEvent, auditErr := dsaudit.NewWorkflowCreatedAuditEvent(workflow)
	if auditErr != nil {
		h.logger.Error(auditErr, "Failed to create workflow creation audit event",
			"workflow_id", workflow.WorkflowID,
			"workflow_name", workflow.WorkflowName,
		)
		return
	}

	auditCtx, auditCancel := context.WithTimeout(ctx, 5*time.Second)
	defer auditCancel()
	if storeErr := h.auditStore.StoreAudit(auditCtx, auditEvent); storeErr != nil {
		h.logger.Error(storeErr, "Failed to persist workflow creation audit",
			"workflow_id", workflow.WorkflowID,
			"workflow_name", workflow.WorkflowName,
		)
	}
}

// validationError carries all fields needed to produce an RFC 7807 response.
// Returned by validateExternalChecks so HandleCreateWorkflow can write the error
// without duplicating RFC 7807 construction logic.
type validationError struct {
	status    int
	errorType string
	title     string
	detail    string
}

// writeTo writes the validationError as an RFC 7807 Problem Details response.
func (ve *validationError) writeTo(w http.ResponseWriter, logger logr.Logger) {
	response.WriteRFC7807Error(w, ve.status, ve.errorType, ve.title, ve.detail, logger)
}

// validateExternalChecks runs Steps 5a–5c (action-type, bundle-exists,
// dependency validation) in parallel and returns the highest-priority
// error, preserving the original sequential error contract.
//
// A 10-second timeout budget bounds the total wall-clock time for all
// external calls, preventing a degraded backend from consuming the
// entire server WriteTimeout.
//
// Issue #1070: Typed-result-slot pattern — each goroutine writes to its
// own slot; after all goroutines complete we check slots in priority order.
func (h *Handler) validateExternalChecks(
	ctx context.Context,
	schemaParser *schema.Parser,
	parsedSchema *models.WorkflowSchema,
	workflow *models.RemediationWorkflow,
) *validationError {
	const (
		slotActionType = iota
		slotBundle
		slotDependency
		slotCount

		validationTimeout = 10 * time.Second
	)

	ctx, cancel := context.WithTimeout(ctx, validationTimeout)
	defer cancel()

	var (
		slots [slotCount]*validationError
		mu    sync.Mutex
		wg    sync.WaitGroup
	)

	totalStart := time.Now()

	setSlot := func(idx int, ve *validationError) {
		mu.Lock()
		slots[idx] = ve
		mu.Unlock()
	}

	// 5a: Validate action_type against taxonomy (GAP-4, DD-WORKFLOW-016)
	if h.actionTypeValidator != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			exists, err := h.actionTypeValidator.ActionTypeExists(ctx, workflow.ActionType)
			if err != nil {
				dsmetrics.WorkflowValidationDuration.WithLabelValues("action_type", "error").Observe(time.Since(start).Seconds())
				h.logger.Error(err, "Failed to validate action_type against taxonomy",
					"action_type", workflow.ActionType,
				)
				setSlot(slotActionType, &validationError{
					status:    http.StatusInternalServerError,
					errorType: "internal-error",
					title:     "Internal Server Error",
					detail:    "Failed to validate action type",
				})
				return
			}
			if !exists {
				dsmetrics.WorkflowValidationDuration.WithLabelValues("action_type", "error").Observe(time.Since(start).Seconds())
				setSlot(slotActionType, &validationError{
					status:    http.StatusBadRequest,
					errorType: "validation-error",
					title:     "Validation Error",
					detail:    fmt.Sprintf("action_type '%s' is not in the action type taxonomy (DD-WORKFLOW-016)", workflow.ActionType),
				})
				return
			}
			dsmetrics.WorkflowValidationDuration.WithLabelValues("action_type", "ok").Observe(time.Since(start).Seconds())
		}()
	}

	// 5b: Validate execution bundle image exists in the registry (skip for ansible — Git repo)
	if workflow.ExecutionBundle != nil && *workflow.ExecutionBundle != "" &&
		workflow.ExecutionEngine != models.ExecutionEngineAnsible && h.schemaExtractor != nil {
		bundleRef := *workflow.ExecutionBundle
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			if err := h.schemaExtractor.ValidateBundleExists(ctx, bundleRef); err != nil {
				dsmetrics.WorkflowValidationDuration.WithLabelValues("bundle_exists", "error").Observe(time.Since(start).Seconds())
				h.logger.Error(err, "Execution bundle image not found in registry",
					"execution_bundle", bundleRef,
				)
				setSlot(slotBundle, &validationError{
					status:    http.StatusBadRequest,
					errorType: "bundle-not-found",
					title:     "Execution Bundle Not Found",
					detail:    "execution.bundle image could not be resolved; verify the image reference is correct",
				})
				return
			}
			dsmetrics.WorkflowValidationDuration.WithLabelValues("bundle_exists", "ok").Observe(time.Since(start).Seconds())
		}()
	}

	// 5c: Validate schema-declared dependencies exist in execution namespace (DD-WE-006)
	if h.dependencyValidator != nil && parsedSchema.Dependencies != nil {
		deps := schemaParser.ExtractDependencies(parsedSchema)
		if deps != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				start := time.Now()
				if err := h.dependencyValidator.ValidateDependencies(ctx, h.executionNamespace, deps); err != nil {
					dsmetrics.WorkflowValidationDuration.WithLabelValues("dependency", "error").Observe(time.Since(start).Seconds())
					h.logger.Error(err, "Dependency validation failed",
						"execution_namespace", h.executionNamespace,
					)
					setSlot(slotDependency, &validationError{
						status:    http.StatusBadRequest,
						errorType: "dependency-validation-error",
						title:     "Dependency Validation Error",
						detail: fmt.Sprintf("Schema-declared dependency not satisfied in namespace %q; "+
							"ensure all dependencies are provisioned before registering the workflow (DD-WE-006)", h.executionNamespace),
					})
					return
				}
				dsmetrics.WorkflowValidationDuration.WithLabelValues("dependency", "ok").Observe(time.Since(start).Seconds())
			}()
		}
	}

	wg.Wait()

	// Return the highest-priority error (lowest slot index).
	for _, slot := range slots {
		if slot != nil {
			dsmetrics.WorkflowValidationDuration.WithLabelValues("total", "error").Observe(time.Since(totalStart).Seconds())
			return slot
		}
	}
	dsmetrics.WorkflowValidationDuration.WithLabelValues("total", "ok").Observe(time.Since(totalStart).Seconds())
	return nil
}

// contentIntegrityError is returned when an active workflow with the same
// (name, version) already exists but has a different content hash. The caller
// must bump the version to register new content. This enforces version-locked
// content immutability per Issue #773.
type contentIntegrityError struct {
	WorkflowName string
	Version      string
	OldHash      string
	NewHash      string
}

func (e *contentIntegrityError) Error() string {
	oldPrefix, newPrefix := e.OldHash, e.NewHash
	if len(oldPrefix) > 12 {
		oldPrefix = oldPrefix[:12]
	}
	if len(newPrefix) > 12 {
		newPrefix = newPrefix[:12]
	}
	return fmt.Sprintf(
		"active workflow %q version %q already has different content (hash %s→%s); bump the version to register new content",
		e.WorkflowName, e.Version, oldPrefix, newPrefix,
	)
}
