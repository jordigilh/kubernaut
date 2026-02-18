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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// ========================================
// WORKFLOW CATALOG HANDLERS
// ========================================
// Business Requirements:
// - BR-STORAGE-014: Workflow catalog management
// - BR-STORAGE-039: Workflow Catalog Retrieval API
//
// API Endpoints:
// - POST /api/v1/workflows - Create a new workflow
// - GET /api/v1/workflows - List workflows with filters
// - GET /api/v1/workflows/{workflowID} - Get workflow by ID (with optional security gate)
// - PATCH /api/v1/workflows/{workflowID} - Update mutable fields
// - PATCH /api/v1/workflows/{workflowID}/disable - Soft-delete workflow
//
// Three-Step Discovery Protocol (DD-HAPI-017):
// - GET /api/v1/workflows/actions - Step 1: List available action types
// - GET /api/v1/workflows/actions/{action_type} - Step 2: List workflows by action type
// - GET /api/v1/workflows/{workflowID} - Step 3: Get workflow with security gate

// HandleCreateWorkflow handles POST /api/v1/workflows
// DD-WORKFLOW-017: OCI-based workflow registration (pullspec-only)
// BR-WORKFLOW-017-001: Accept only schemaImage, pull OCI image, extract schema, populate catalog
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	// Step 1: Parse request body — expect only {"schemaImage": "..."}
	var req struct {
		SchemaImage string `json:"schemaImage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error(err, "Failed to decode workflow create request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	// Step 2: Validate schemaImage is present
	if req.SchemaImage == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"schemaImage is required", h.logger)
		return
	}

	// Step 3: Validate schema extractor is configured
	if h.schemaExtractor == nil {
		h.logger.Error(fmt.Errorf("schemaExtractor not configured"), "Handler misconfiguration")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"OCI schema extraction not configured", h.logger)
		return
	}

	// Step 4: Pull OCI image and extract /workflow-schema.yaml (DD-WORKFLOW-017)
	result, err := h.schemaExtractor.ExtractFromImage(r.Context(), req.SchemaImage)
	if err != nil {
		h.classifyExtractionError(w, err, req.SchemaImage)
		return
	}

	// Step 5: Build RemediationWorkflow from extracted schema
	schemaParser := schema.NewParser()
	workflow, err := h.buildWorkflowFromSchema(schemaParser, result, req.SchemaImage)
	if err != nil {
		h.logger.Error(err, "Failed to build workflow from extracted schema",
			"schema_image", req.SchemaImage,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to process extracted schema", h.logger)
		return
	}

	// Step 5b: Validate action_type against taxonomy (GAP-4, DD-WORKFLOW-016)
	// Explicit validation for clean 400 instead of FK constraint 500.
	if h.actionTypeValidator != nil {
		exists, err := h.actionTypeValidator.ActionTypeExists(r.Context(), workflow.ActionType)
		if err != nil {
			h.logger.Error(err, "Failed to validate action_type against taxonomy",
				"action_type", workflow.ActionType,
			)
			response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
				"Failed to validate action type", h.logger)
			return
		}
		if !exists {
			detail := fmt.Sprintf("action_type '%s' is not in the action type taxonomy (DD-WORKFLOW-016)", workflow.ActionType)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error",
				detail, h.logger)
			return
		}
	}

	// Step 6: Create workflow in repository
	if h.workflowRepo != nil {
		if err := h.workflowRepo.Create(r.Context(), workflow); err != nil {
			// DS-BUG-001: Check for PostgreSQL unique constraint violation (duplicate workflow)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				h.logger.Info("Workflow creation skipped - already exists",
					"workflow_name", workflow.WorkflowName,
					"version", workflow.Version,
				)
				detail := fmt.Sprintf("Workflow '%s' version '%s' already exists", workflow.WorkflowName, workflow.Version)
				response.WriteRFC7807Error(w, http.StatusConflict, "conflict",
					"Workflow Already Exists", detail, h.logger)
				return
			}

			h.logger.Error(err, "Failed to create workflow",
				"workflow_name", workflow.WorkflowName,
				"version", workflow.Version,
			)
			response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
				"Failed to create workflow", h.logger)
			return
		}
	}

	// Step 7: Audit workflow creation (async, best-effort)
	if h.auditStore != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			auditEvent, err := dsaudit.NewWorkflowCreatedAuditEvent(workflow)
			if err != nil {
				h.logger.Error(err, "Failed to create workflow creation audit event",
					"workflow_id", workflow.WorkflowID,
					"workflow_name", workflow.WorkflowName,
				)
				return
			}

			if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
				h.logger.Error(err, "Failed to audit workflow creation",
					"workflow_id", workflow.WorkflowID,
					"workflow_name", workflow.WorkflowName,
				)
			}
		}()
	}

	// Log success
	h.logger.Info("Workflow created successfully from OCI image",
		"workflow_id", workflow.WorkflowID,
		"workflow_name", workflow.WorkflowName,
		"version", workflow.Version,
		"schema_image", req.SchemaImage,
		"schema_digest", workflow.SchemaDigest,
	)

	// Return created workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow create response")
	}
}

// classifyExtractionError maps OCI extraction errors to appropriate HTTP status codes.
// DD-WORKFLOW-017 validation order:
// - Image pull failure → 502 Bad Gateway
// - /workflow-schema.yaml not found → 422 Unprocessable Entity
// - Schema validation failure → 400 Bad Request
func (h *Handler) classifyExtractionError(w http.ResponseWriter, err error, containerImage string) {
	errMsg := err.Error()

	// Image pull failure: errors from the puller contain "pull image"
	if strings.Contains(errMsg, "pull image") {
		h.logger.Error(err, "OCI image pull failed",
			"schema_image", containerImage,
		)
		response.WriteRFC7807Error(w, http.StatusBadGateway, "image-pull-failed", "Image Pull Failed",
			fmt.Sprintf("Failed to pull OCI image %q: %v", containerImage, err), h.logger)
		return
	}

	// Schema not found: the extractor returns "not found in image layers"
	if strings.Contains(errMsg, "not found in image layers") {
		h.logger.Error(err, "workflow-schema.yaml not found in OCI image",
			"schema_image", containerImage,
		)
		response.WriteRFC7807Error(w, http.StatusUnprocessableEntity, "schema-not-found", "Schema Not Found",
			fmt.Sprintf("/workflow-schema.yaml not found in OCI image %q", containerImage), h.logger)
		return
	}

	// Schema validation failure: errors from ParseAndValidate contain "validate workflow-schema.yaml"
	if strings.Contains(errMsg, "validate workflow-schema.yaml") {
		h.logger.Error(err, "Invalid workflow-schema.yaml in OCI image",
			"schema_image", containerImage,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Schema Validation Error",
			fmt.Sprintf("Invalid workflow-schema.yaml in image %q: %v", containerImage, err), h.logger)
		return
	}

	// Fallback: unknown extraction error
	h.logger.Error(err, "Unexpected error during OCI schema extraction",
		"schema_image", containerImage,
	)
	response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
		fmt.Sprintf("Schema extraction failed: %v", err), h.logger)
}

// buildWorkflowFromSchema populates a RemediationWorkflow from the extracted OCI schema.
// DD-WORKFLOW-017: All catalog fields are derived from the schema; nothing from the API request.
func (h *Handler) buildWorkflowFromSchema(
	schemaParser *schema.Parser,
	result *oci.ExtractionResult,
	containerImage string,
) (*models.RemediationWorkflow, error) {
	parsedSchema := result.Schema

	// Extract parameters and wrap for HAPI validator compatibility
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

	// Extract labels as JSONB
	labelsJSON, err := schemaParser.ExtractLabels(parsedSchema)
	if err != nil {
		return nil, fmt.Errorf("extract labels: %w", err)
	}

	// Convert WorkflowDescription (schema) to StructuredDescription (DB model)
	desc := models.StructuredDescription{
		What:          parsedSchema.Metadata.Description.What,
		WhenToUse:     parsedSchema.Metadata.Description.WhenToUse,
		WhenNotToUse:  parsedSchema.Metadata.Description.WhenNotToUse,
		Preconditions: parsedSchema.Metadata.Description.Preconditions,
	}

	// Convert execution engine string to ExecutionEngine type
	execEngine := models.ExecutionEngine(schemaParser.ExtractExecutionEngine(parsedSchema))

	// SchemaImage is the OCI image used for registration, SchemaDigest is its sha256
	imgRef := containerImage
	digest := result.Digest

	// Build the workflow from schema fields
	workflow := &models.RemediationWorkflow{
		WorkflowName:    parsedSchema.Metadata.WorkflowID,
		Version:         parsedSchema.Metadata.Version,
		Name:            parsedSchema.Metadata.WorkflowID, // Use workflowId as display name
		Description:     desc,
		Content:         result.RawContent,
		Parameters:      &rawParams,
		ExecutionEngine: execEngine,
		SchemaImage:     &imgRef,
		SchemaDigest:    &digest,
		ActionType:      parsedSchema.ActionType,
		Status:          "active",
		IsLatestVersion: true,
	}

	// Extract execution bundle from schema (Issue #89)
	if bundle := schemaParser.ExtractExecutionBundle(parsedSchema); bundle != nil {
		workflow.ExecutionBundle = bundle
		if _, digest, err := schema.ParseBundleDigest(*bundle); err == nil {
			workflow.ExecutionBundleDigest = &digest
		}
	}

	// Set labels from extracted JSONB
	if err := json.Unmarshal(labelsJSON, &workflow.Labels); err != nil {
		return nil, fmt.Errorf("unmarshal labels: %w", err)
	}

	// Compute content hash (SHA-256)
	workflow.ContentHash = computeContentHash(result.RawContent)

	return workflow, nil
}

// HandleListWorkflows handles GET /api/v1/workflows
// BR-STORAGE-014: Workflow catalog management
func (h *Handler) HandleListWorkflows(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	filters := &models.WorkflowSearchFilters{}

	// Status filter
	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = []string{status}
	}

	// DD-WORKFLOW-001 v2.4: Multi-environment support (accepts multiple values)
	// Parse environment filter from query parameters
	// DD-WORKFLOW-001 v2.5: Single environment value (workflows store arrays, searches use single value)
	if env := r.URL.Query().Get("environment"); env != "" {
		filters.Environment = env
	}

	// Priority filter (mandatory)
	if priority := r.URL.Query().Get("priority"); priority != "" {
		filters.Priority = priority
	}

	// Component filter (mandatory)
	if component := r.URL.Query().Get("component"); component != "" {
		filters.Component = component
	}

	// Workflow name filter (exact match for metadata lookup)
	// Authority: DD-API-001 (OpenAPI client mandatory - added in Jan 2026)
	// Used for test idempotency and workflow lookup by human-readable name
	if workflowName := r.URL.Query().Get("workflow_name"); workflowName != "" {
		filters.WorkflowName = workflowName
	}

	// DD-WORKFLOW-001 v1.5: Custom labels (subdomain-based)
	// Format: custom_labels[subdomain]=value1,value2
	// Example: custom_labels[constraint]=cost-constrained,stateful-safe
	for key, values := range r.URL.Query() {
		if strings.HasPrefix(key, "custom_labels[") && strings.HasSuffix(key, "]") {
			subdomain := strings.TrimSuffix(strings.TrimPrefix(key, "custom_labels["), "]")
			if subdomain != "" && len(values) > 0 {
				if filters.CustomLabels == nil {
					filters.CustomLabels = make(map[string][]string)
				}
				// Split comma-separated values
				for _, v := range values {
					filters.CustomLabels[subdomain] = append(filters.CustomLabels[subdomain], strings.Split(v, ",")...)
				}
			}
		}
	}

	// Pagination
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	// Execute list query
	workflows, total, err := h.workflowRepo.List(r.Context(), filters, limit, offset)
	if err != nil {
		h.logger.Error(err, "Failed to list workflows",
			"filters", filters,
			"limit", limit,
			"offset", offset,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to list workflows", h.logger)
		return
	}

	// Log success
	h.logger.Info("Workflows listed",
		"count", len(workflows),
		"filters", filters,
		"limit", limit,
		"offset", offset,
	)

	// Convert to pointer slice for response
	workflowPtrs := make([]*models.RemediationWorkflow, len(workflows))
	for i := range workflows {
		workflowPtrs[i] = &workflows[i]
	}

	// Return results
	response := models.WorkflowListResponse{
		Workflows: workflowPtrs,
		Limit:     limit,
		Offset:    offset,
		Total:     total,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error(err, "Failed to encode workflow list response")
	}
}

// HandleGetWorkflowByID handles GET /api/v1/workflows/{workflowID}
// BR-STORAGE-039: Workflow Catalog Retrieval API
// DD-WORKFLOW-002 v3.0: UUID primary key for workflow retrieval
// DD-WORKFLOW-016, DD-HAPI-017: Security gate via optional context filters (Step 3)
//
// Returns complete workflow object including:
// - spec.schema_image: OCI container image reference (for HAPI validation)
// - spec.parameters[]: Parameter schema (for LLM parameter validation)
// - detected_labels: Signal type, severity labels (for workflow filtering)
//
// Security Gate (when context filters are present):
// - Returns 404 if workflow exists but doesn't match context (DD-WORKFLOW-016)
// - Intentionally doesn't distinguish "not found" from "filtered out" to prevent info leakage
// - Emits workflow.catalog.workflow_retrieved audit event
//
// Cross-Service Integration:
// - HolmesGPT-API: Uses for get_workflow tool (Step 3 of discovery protocol)
// - AIAnalysis: May use for defense-in-depth validation
func (h *Handler) HandleGetWorkflowByID(w http.ResponseWriter, r *http.Request) {
	// Get workflow ID from URL path
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	// DD-WORKFLOW-016: Parse optional context filters for security gate
	filters, err := ParseDiscoveryFilters(r)
	if err != nil {
		h.logger.Error(err, "Invalid discovery filter parameters")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			err.Error(), h.logger)
		return
	}

	var wf *models.RemediationWorkflow

	// DD-WORKFLOW-016: Use context-filtered query when filters are present (security gate)
	// GAP-WF-6: Measure query duration for audit payload (DD-WORKFLOW-014 v3.0)
	startGet := time.Now()
	if filters.HasContextFilters() {
		wf, err = h.workflowRepo.GetWorkflowWithContextFilters(r.Context(), workflowID, filters)
	} else {
		wf, err = h.workflowRepo.GetByID(r.Context(), workflowID)
	}
	durationMs := time.Since(startGet).Milliseconds()

	if err != nil {
		// Check if workflow not found
		if err.Error() == fmt.Sprintf("workflow not found: %s", workflowID) {
			response.WriteRFC7807Error(w, http.StatusNotFound, "workflow-not-found", "Not Found",
				fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
			return
		}

		h.logger.Error(err, "Failed to get workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to get workflow", h.logger)
		return
	}

	// Check for nil workflow (not found or security gate filtered out)
	if wf == nil {
		h.logger.Info("Workflow not found or filtered by security gate",
			"workflow_id", workflowID,
			"has_context_filters", filters.HasContextFilters(),
		)
		// DD-WORKFLOW-016: Return same 404 for "not found" and "filtered out" (prevent info leakage)
		response.WriteRFC7807Error(w, http.StatusNotFound, "workflow-not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}

	// BR-AUDIT-023: Emit discovery audit events when context filters are present.
	// DD-WORKFLOW-014 v3.0: Context filters indicate HAPI is validating its selection,
	// so we emit both workflow_retrieved and selection_validated.
	if filters.HasContextFilters() && h.auditStore != nil {
		retrievedEvent, err := dsaudit.NewWorkflowRetrievedAuditEvent(workflowID, filters, durationMs)
		if err != nil {
			h.logger.Error(err, "Failed to create workflow_retrieved audit event", "workflow_id", workflowID)
		}
		validatedEvent, err := dsaudit.NewSelectionValidatedAuditEvent(workflowID, filters, true, durationMs)
		if err != nil {
			h.logger.Error(err, "Failed to create selection_validated audit event", "workflow_id", workflowID)
		}

		var events []*api.AuditEventRequest
		if retrievedEvent != nil {
			events = append(events, retrievedEvent)
		}
		if validatedEvent != nil {
			events = append(events, validatedEvent)
		}
		if len(events) > 0 {
			h.emitAuditEventsAsync(events, "workflow_id", workflowID)
		}
	}

	// Log success
	h.logger.Info("Workflow retrieved",
		"workflow_id", workflowID,
		"workflow_name", wf.WorkflowName,
		"version", wf.Version,
		"has_context_filters", filters.HasContextFilters(),
	)

	// Return workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(wf); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// HandleUpdateWorkflow handles PATCH /api/v1/workflows/{workflowID}
// DD-WORKFLOW-012: Update ONLY mutable fields (status, metrics)
// Immutable fields (description, content, labels) require creating a new version
func (h *Handler) HandleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	// Get workflow ID from URL path
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	// Parse request body
	var updateReq models.WorkflowUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&updateReq); err != nil {
		h.logger.Error(err, "Failed to decode workflow update request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	// DD-WORKFLOW-012: Validate that ONLY mutable fields are being updated
	if updateReq.Description != nil || updateReq.Content != nil || updateReq.Labels != nil {
		h.logger.Error(fmt.Errorf("immutable fields cannot be updated"), "Attempted to update immutable fields",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "immutable-field-violation", "Bad Request",
			"Cannot update immutable fields (description, content, labels). Create a new version instead. See DD-WORKFLOW-012.", h.logger)
		return
	}

	// Get existing workflow
	workflow, err := h.workflowRepo.GetByID(r.Context(), workflowID)
	if err != nil {
		h.logger.Error(err, "Failed to get workflow for update",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}

	// Apply mutable field updates
	if updateReq.Status != nil {
		workflow.Status = *updateReq.Status
		if *updateReq.Status == "disabled" {
			now := time.Now()
			workflow.DisabledAt = &now
			workflow.DisabledBy = updateReq.DisabledBy
			workflow.DisabledReason = updateReq.DisabledReason
		}
	}

	// Update the workflow
	if err := h.workflowRepo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, workflow.Status, getStringValue(workflow.DisabledReason), getStringValue(workflow.DisabledBy)); err != nil {
		h.logger.Error(err, "Failed to update workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to update workflow", h.logger)
		return
	}

	// BR-STORAGE-183: Audit workflow update (business logic operation)
	// DD-AUDIT-002 V2.0.1: Workflow state changes are business logic
	if h.auditStore != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Build structured updated fields (ogen type - no more map[string]interface{}!)
			updatedFields := api.WorkflowCatalogUpdatedFields{}
			updatedFields.Status.SetTo(workflow.Status)

			if updateReq.Status != nil && *updateReq.Status == "disabled" {
				if disabledBy := getStringValue(workflow.DisabledBy); disabledBy != "" {
					updatedFields.DisabledBy.SetTo(disabledBy)
				}
				if disabledReason := getStringValue(workflow.DisabledReason); disabledReason != "" {
					updatedFields.DisabledReason.SetTo(disabledReason)
				}
			}

			auditEvent, err := dsaudit.NewWorkflowUpdatedAuditEvent(workflow.WorkflowID, updatedFields)
			if err != nil {
				h.logger.Error(err, "Failed to create workflow update audit event",
					"workflow_id", workflowID,
				)
				return
			}

			if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
				h.logger.Error(err, "Failed to audit workflow update",
					"workflow_id", workflowID,
				)
			}
		}()
	}

	// Log success
	h.logger.Info("Workflow updated",
		"workflow_id", workflowID,
		"status", workflow.Status,
	)

	// Return updated workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// getStringValue safely dereferences a string pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// emitAuditEventsAsync stores one or more audit events asynchronously in a background goroutine.
// This is the standard pattern for non-blocking audit event emission (BR-AUDIT-024).
// Events are stored sequentially within a single goroutine to share one context/timeout.
// If event creation fails for any event, remaining events are still attempted.
func (h *Handler) emitAuditEventsAsync(events []*api.AuditEventRequest, kvs ...interface{}) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for _, event := range events {
			if err := h.auditStore.StoreAudit(ctx, event); err != nil {
				args := append([]interface{}{"Failed to store audit event", "event_type", event.EventType}, kvs...)
				h.logger.Error(err, args[0].(string), args[1:]...)
			}
		}
	}()
}

// getWorkflowLifecycleRepo returns the workflow lifecycle repository for enable/disable/deprecate.
// Uses workflowLifecycleRepo when set (tests), otherwise workflowRepo.
func (h *Handler) getWorkflowLifecycleRepo() WorkflowLifecycleRepository {
	if h.workflowLifecycleRepo != nil {
		return h.workflowLifecycleRepo
	}
	return h.workflowRepo
}

// HandleEnableWorkflow handles PATCH /api/v1/workflows/{workflowID}/enable
// DD-WORKFLOW-017 Phase 4.4 (GAP-WF-1): Convenience endpoint for re-enabling workflows
func (h *Handler) HandleEnableWorkflow(w http.ResponseWriter, r *http.Request) {
	// Get workflow ID from URL path
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	// Parse request body
	var req models.WorkflowDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error(err, "Failed to decode workflow enable request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	// DD-WORKFLOW-017 Phase 4.4 (GAP-WF-5): reason is mandatory for lifecycle operations
	if req.Reason == nil || strings.TrimSpace(*req.Reason) == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-reason", "Missing Required Field",
			"reason is required for lifecycle operations", h.logger)
		return
	}

	// Get existing workflow
	repo := h.getWorkflowLifecycleRepo()
	if repo == nil {
		h.logger.Error(fmt.Errorf("workflow repository not configured"), "Handler misconfiguration")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Workflow repository not configured", h.logger)
		return
	}

	workflow, err := repo.GetByID(r.Context(), workflowID)
	if err != nil {
		h.logger.Error(err, "Failed to get workflow for enable",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}
	if workflow == nil {
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}

	// Update the workflow status to active
	reason := ""
	if req.Reason != nil {
		reason = *req.Reason
	}
	updatedBy := ""
	if req.UpdatedBy != nil {
		updatedBy = *req.UpdatedBy
	}

	if err := repo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, "active", reason, updatedBy); err != nil {
		h.logger.Error(err, "Failed to enable workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to enable workflow", h.logger)
		return
	}

	// Update workflow object for response
	workflow.Status = "active"
	workflow.DisabledAt = nil
	workflow.DisabledBy = nil
	workflow.DisabledReason = nil

	// BR-STORAGE-183: Audit workflow enable (business logic operation)
	if h.auditStore != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			updatedFields := api.WorkflowCatalogUpdatedFields{}
			updatedFields.Status.SetTo("active")

			auditEvent, err := dsaudit.NewWorkflowUpdatedAuditEvent(workflow.WorkflowID, updatedFields)
			if err != nil {
				h.logger.Error(err, "Failed to create workflow enable audit event",
					"workflow_id", workflowID,
				)
				return
			}

			if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
				h.logger.Error(err, "Failed to audit workflow enable",
					"workflow_id", workflowID,
				)
			}
		}()
	}

	// Log success
	h.logger.Info("Workflow enabled",
		"workflow_id", workflowID,
		"reason", reason,
		"updated_by", updatedBy,
	)

	// Return updated workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// HandleDeprecateWorkflow handles PATCH /api/v1/workflows/{workflowID}/deprecate
// DD-WORKFLOW-017 Phase 4.4 (GAP-WF-1): Convenience endpoint for deprecating workflows
func (h *Handler) HandleDeprecateWorkflow(w http.ResponseWriter, r *http.Request) {
	// Get workflow ID from URL path
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	// Parse request body
	var req models.WorkflowDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error(err, "Failed to decode workflow deprecate request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	// DD-WORKFLOW-017 Phase 4.4 (GAP-WF-5): reason is mandatory for lifecycle operations
	if req.Reason == nil || strings.TrimSpace(*req.Reason) == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-reason", "Missing Required Field",
			"reason is required for lifecycle operations", h.logger)
		return
	}

	// Get existing workflow
	repo := h.getWorkflowLifecycleRepo()
	if repo == nil {
		h.logger.Error(fmt.Errorf("workflow repository not configured"), "Handler misconfiguration")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Workflow repository not configured", h.logger)
		return
	}

	workflow, err := repo.GetByID(r.Context(), workflowID)
	if err != nil {
		h.logger.Error(err, "Failed to get workflow for deprecate",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}
	if workflow == nil {
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}

	// Update the workflow status to deprecated
	reason := ""
	if req.Reason != nil {
		reason = *req.Reason
	}
	updatedBy := ""
	if req.UpdatedBy != nil {
		updatedBy = *req.UpdatedBy
	}

	if err := repo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, "deprecated", reason, updatedBy); err != nil {
		h.logger.Error(err, "Failed to deprecate workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to deprecate workflow", h.logger)
		return
	}

	// Update workflow object for response
	workflow.Status = "deprecated"

	// BR-STORAGE-183: Audit workflow deprecate (business logic operation)
	if h.auditStore != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			updatedFields := api.WorkflowCatalogUpdatedFields{}
			updatedFields.Status.SetTo("deprecated")

			auditEvent, err := dsaudit.NewWorkflowUpdatedAuditEvent(workflow.WorkflowID, updatedFields)
			if err != nil {
				h.logger.Error(err, "Failed to create workflow deprecate audit event",
					"workflow_id", workflowID,
				)
				return
			}

			if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
				h.logger.Error(err, "Failed to audit workflow deprecate",
					"workflow_id", workflowID,
				)
			}
		}()
	}

	// Log success
	h.logger.Info("Workflow deprecated",
		"workflow_id", workflowID,
		"reason", reason,
		"updated_by", updatedBy,
	)

	// Return updated workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// HandleDisableWorkflow handles PATCH /api/v1/workflows/{workflowID}/disable
// DD-WORKFLOW-012: Convenience endpoint for disabling workflows (soft delete)
func (h *Handler) HandleDisableWorkflow(w http.ResponseWriter, r *http.Request) {
	// Get workflow ID from URL path
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	// Parse request body
	var disableReq models.WorkflowDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&disableReq); err != nil {
		h.logger.Error(err, "Failed to decode workflow disable request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	// DD-WORKFLOW-017 Phase 4.4 (GAP-WF-5): reason is mandatory for lifecycle operations
	if disableReq.Reason == nil || strings.TrimSpace(*disableReq.Reason) == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-reason", "Missing Required Field",
			"reason is required for lifecycle operations", h.logger)
		return
	}

	// Get existing workflow
	repo := h.getWorkflowLifecycleRepo()
	if repo == nil {
		h.logger.Error(fmt.Errorf("workflow repository not configured"), "Handler misconfiguration")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Workflow repository not configured", h.logger)
		return
	}

	workflow, err := repo.GetByID(r.Context(), workflowID)
	if err != nil {
		h.logger.Error(err, "Failed to get workflow for disable",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}
	if workflow == nil {
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return
	}

	// Update the workflow status to disabled
	reason := ""
	if disableReq.Reason != nil {
		reason = *disableReq.Reason
	}
	updatedBy := ""
	if disableReq.UpdatedBy != nil {
		updatedBy = *disableReq.UpdatedBy
	}

	if err := repo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, "disabled", reason, updatedBy); err != nil {
		h.logger.Error(err, "Failed to disable workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to disable workflow", h.logger)
		return
	}

	// Update workflow object for response
	workflow.Status = "disabled"
	now := time.Now()
	workflow.DisabledAt = &now
	workflow.DisabledBy = disableReq.UpdatedBy
	workflow.DisabledReason = disableReq.Reason

	// BR-STORAGE-183: Audit workflow disable (business logic operation)
	// DD-AUDIT-002 V2.0.1: Workflow disable is a status update (captured via workflow.updated)
	if h.auditStore != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Build structured updated fields (OGEN-MIGRATION)
			updatedFields := api.WorkflowCatalogUpdatedFields{}
			updatedFields.Status.SetTo("disabled")
			updatedFields.DisabledBy.SetTo(updatedBy)
			updatedFields.DisabledReason.SetTo(reason)

			auditEvent, err := dsaudit.NewWorkflowUpdatedAuditEvent(workflow.WorkflowID, updatedFields)
			if err != nil {
				h.logger.Error(err, "Failed to create workflow disable audit event",
					"workflow_id", workflowID,
				)
				return
			}

			if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
				h.logger.Error(err, "Failed to audit workflow disable",
					"workflow_id", workflowID,
				)
			}
		}()
	}

	// Log success
	h.logger.Info("Workflow disabled",
		"workflow_id", workflowID,
		"reason", reason,
		"disabled_by", updatedBy,
	)

	// Return updated workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// ========================================
// THREE-STEP WORKFLOW DISCOVERY HANDLERS
// ========================================
// Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
// Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
// Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
//
// Step 1: GET /api/v1/workflows/actions - List available action types
// Step 2: GET /api/v1/workflows/actions/{action_type} - List workflows for action type
// Step 3: GET /api/v1/workflows/{workflowID} (modified) - Get workflow with security gate

// HandleListAvailableActions handles GET /api/v1/workflows/actions
// Step 1: Returns action types from taxonomy that have active workflows
// matching the provided signal context filters.
// Emits workflow.catalog.actions_listed audit event (DD-WORKFLOW-014 v3.0)
func (h *Handler) HandleListAvailableActions(w http.ResponseWriter, r *http.Request) {
	// Parse discovery filters from query parameters
	filters, err := ParseDiscoveryFilters(r)
	if err != nil {
		h.logger.Error(err, "Invalid discovery filter parameters")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			err.Error(), h.logger)
		return
	}

	// Parse pagination
	offset, limit := ParsePagination(r)

	// Execute query (GAP-WF-6: measure duration for audit payload)
	startList := time.Now()
	entries, totalCount, err := h.workflowRepo.ListActions(r.Context(), filters, offset, limit)
	durationMs := time.Since(startList).Milliseconds()
	if err != nil {
		h.logger.Error(err, "Failed to list available actions",
			"filters", filters)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to list available actions", h.logger)
		return
	}

	// Build response
	resp := models.ActionTypeListResponse{
		ActionTypes: entries,
		Pagination: models.PaginationMetadata{
			TotalCount: totalCount,
			Offset:     offset,
			Limit:      limit,
			HasMore:    offset+limit < totalCount,
		},
	}

	// BR-AUDIT-023: Emit workflow.catalog.actions_listed audit event
	if h.auditStore != nil {
		if event, err := dsaudit.NewActionsListedAuditEvent(filters, totalCount, durationMs); err != nil {
			h.logger.Error(err, "Failed to create actions_listed audit event")
		} else {
			h.emitAuditEventsAsync([]*api.AuditEventRequest{event})
		}
	}

	h.logger.Info("Available actions listed",
		"total_count", totalCount,
		"returned_count", len(entries),
		"offset", offset,
		"limit", limit,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error(err, "Failed to encode actions list response")
	}
}

// HandleListWorkflowsByActionType handles GET /api/v1/workflows/actions/{action_type}
// Step 2: Returns active workflows matching the specified action type and context filters.
// Emits workflow.catalog.workflows_listed audit event (DD-WORKFLOW-014 v3.0)
func (h *Handler) HandleListWorkflowsByActionType(w http.ResponseWriter, r *http.Request) {
	// Get action_type from URL path
	actionType := chi.URLParam(r, "action_type")
	if actionType == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"action_type path parameter is required", h.logger)
		return
	}

	// Parse discovery filters from query parameters
	filters, err := ParseDiscoveryFilters(r)
	if err != nil {
		h.logger.Error(err, "Invalid discovery filter parameters")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			err.Error(), h.logger)
		return
	}

	// Parse pagination
	offset, limit := ParsePagination(r)

	// Execute query (GAP-WF-6: measure duration for audit payload)
	startList := time.Now()
	workflows, totalCount, err := h.workflowRepo.ListWorkflowsByActionType(r.Context(), actionType, filters, offset, limit)
	durationMs := time.Since(startList).Milliseconds()
	if err != nil {
		h.logger.Error(err, "Failed to list workflows by action type",
			"action_type", actionType)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to list workflows by action type", h.logger)
		return
	}

	// Convert to discovery entries
	// DD-HAPI-017 v1.1: ActualSuccessRate and TotalExecutions excluded from
	// LLM-facing response — global aggregates are misleading for per-incident selection.
	discoveryEntries := make([]models.WorkflowDiscoveryEntry, 0, len(workflows))
	for _, wf := range workflows {
		entry := models.WorkflowDiscoveryEntry{
			WorkflowID:      wf.WorkflowID,
			WorkflowName:    wf.WorkflowName,
			Name:            wf.Name,
			Description:     wf.Description,
			Version:         wf.Version,
			ExecutionEngine: string(wf.ExecutionEngine),
		}
		if wf.SchemaImage != nil {
			entry.SchemaImage = *wf.SchemaImage
		}
		if wf.ExecutionBundle != nil {
			entry.ExecutionBundle = *wf.ExecutionBundle
		}
		discoveryEntries = append(discoveryEntries, entry)
	}

	// Build response
	resp := models.WorkflowDiscoveryResponse{
		ActionType: actionType,
		Workflows:  discoveryEntries,
		Pagination: models.PaginationMetadata{
			TotalCount: totalCount,
			Offset:     offset,
			Limit:      limit,
			HasMore:    offset+limit < totalCount,
		},
	}

	// BR-AUDIT-023: Emit workflow.catalog.workflows_listed audit event
	if h.auditStore != nil {
		if event, err := dsaudit.NewWorkflowsListedAuditEvent(actionType, filters, totalCount, durationMs); err != nil {
			h.logger.Error(err, "Failed to create workflows_listed audit event", "action_type", actionType)
		} else {
			h.emitAuditEventsAsync([]*api.AuditEventRequest{event}, "action_type", actionType)
		}
	}

	h.logger.Info("Workflows listed by action type",
		"action_type", actionType,
		"total_count", totalCount,
		"returned_count", len(discoveryEntries),
		"offset", offset,
		"limit", limit,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error(err, "Failed to encode workflows list response")
	}
}

// ParseDiscoveryFilters extracts WorkflowDiscoveryFilters from query parameters.
// Used by all three discovery endpoints.
// Exported for unit testing (UT-DS-017-001-003).
func ParseDiscoveryFilters(r *http.Request) (*models.WorkflowDiscoveryFilters, error) {
	filters := &models.WorkflowDiscoveryFilters{
		Severity:      r.URL.Query().Get("severity"),
		Component:     r.URL.Query().Get("component"),
		Environment:   r.URL.Query().Get("environment"),
		Priority:      r.URL.Query().Get("priority"),
		RemediationID: r.URL.Query().Get("remediation_id"),
	}

	// Parse optional JSON-encoded custom_labels
	if customLabelsStr := r.URL.Query().Get("custom_labels"); customLabelsStr != "" {
		var customLabels map[string][]string
		if err := json.Unmarshal([]byte(customLabelsStr), &customLabels); err != nil {
			return nil, fmt.Errorf("invalid custom_labels JSON: %w", err)
		}
		filters.CustomLabels = customLabels
	}

	// Parse optional JSON-encoded detected_labels
	if detectedLabelsStr := r.URL.Query().Get("detected_labels"); detectedLabelsStr != "" {
		var detectedLabels models.DetectedLabels
		if err := json.Unmarshal([]byte(detectedLabelsStr), &detectedLabels); err != nil {
			return nil, fmt.Errorf("invalid detected_labels JSON: %w", err)
		}
		filters.DetectedLabels = &detectedLabels
	}

	return filters, nil
}

// ParsePagination extracts offset and limit from query parameters.
// Defaults: offset=0, limit=10 (DD-WORKFLOW-016 default page size)
// Exported for unit testing (UT-DS-017-001-002).
func ParsePagination(r *http.Request) (int, int) {
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		limit = models.DefaultPaginationLimit
	}
	if limit > models.MaxPaginationLimit {
		limit = models.MaxPaginationLimit
	}

	return offset, limit
}

// computeContentHash computes a SHA-256 hash of the workflow content.
// DD-WORKFLOW-017: Content hash is derived from the raw YAML extracted from the OCI image.
func computeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
