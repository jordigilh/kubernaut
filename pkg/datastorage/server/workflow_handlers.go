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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW CATALOG HANDLERS
// ========================================
// Business Requirements:
// - BR-STORAGE-013: Semantic search for remediation workflows
// - BR-STORAGE-014: Workflow catalog management
//
// API Endpoints:
// - POST /api/v1/workflows - Create a new workflow
// - POST /api/v1/workflows/search - Semantic search for workflows
// - GET /api/v1/workflows - List workflows with filters
// - GET /api/v1/workflows/{id}/{version} - Get specific workflow version
// - GET /api/v1/workflows/{id}/latest - Get latest workflow version

// HandleCreateWorkflow handles POST /api/v1/workflows
// BR-STORAGE-014: Workflow catalog management
// DD-WORKFLOW-005 v1.0: Direct REST API workflow registration
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var workflow models.RemediationWorkflow
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		h.logger.Error(err, "Failed to decode workflow create request")
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			fmt.Sprintf("Invalid request body: %v", err),
		)
		return
	}

	// Validate required fields
	if err := h.validateCreateWorkflowRequest(&workflow); err != nil {
		h.logger.Error(err, "Invalid workflow create request",
			"workflow_name", workflow.WorkflowName,
		)
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			err.Error(),
		)
		return
	}

	// V1.0: Embedding generation removed (label-only search)
	// Authority: CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md (92% confidence)

	// Set default status if not provided
	if workflow.Status == "" {
		workflow.Status = "active"
	}

	// DD-WORKFLOW-002 v3.0: New workflows are always the latest version
	// The repository will handle marking previous versions as not latest
	workflow.IsLatestVersion = true

	// Create workflow in repository
	if err := h.workflowRepo.Create(r.Context(), &workflow); err != nil {
		h.logger.Error(err, "Failed to create workflow",
			"workflow_name", workflow.WorkflowName,
			"version", workflow.Version,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to create workflow",
		)
		return
	}

	// Log success
	h.logger.Info("Workflow created successfully",
		"workflow_id", workflow.WorkflowID,
		"workflow_name", workflow.WorkflowName,
		"version", workflow.Version,
	)

	// Return created workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow create response")
	}
}

// validateCreateWorkflowRequest validates the workflow create request
func (h *Handler) validateCreateWorkflowRequest(workflow *models.RemediationWorkflow) error {
	if workflow.WorkflowName == "" {
		return fmt.Errorf("workflow_name is required")
	}
	if workflow.Version == "" {
		return fmt.Errorf("version is required")
	}
	if workflow.Name == "" {
		return fmt.Errorf("name is required")
	}
	if workflow.Description == "" {
		return fmt.Errorf("description is required")
	}
	if workflow.Content == "" {
		return fmt.Errorf("content is required")
	}
	if workflow.Labels == nil {
		return fmt.Errorf("labels is required")
	}
	return nil
}

// HandleWorkflowSearch handles POST /api/v1/workflows/search
// BR-STORAGE-013: Label-based workflow search (V1.0 - embeddings removed)
// BR-AUDIT-023: Audit event generation for workflow search
// Authority: CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md (92% confidence)
func (h *Handler) HandleWorkflowSearch(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Parse request body
	var searchReq models.WorkflowSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		h.logger.Error(err, "Failed to decode workflow search request")
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			fmt.Sprintf("Invalid request body: %v", err),
		)
		return
	}

	// Validate request (filters are required for label-only search)
	if err := h.validateWorkflowSearchRequest(&searchReq); err != nil {
		h.logger.Error(err, "Invalid workflow search request",
			"filters", searchReq.Filters,
		)
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			err.Error(),
		)
		return
	}

	// Execute label-only search (NO embedding generation)
	// V1.0: Pure SQL label matching with wildcard weighting
	response, err := h.workflowRepo.SearchByLabels(r.Context(), &searchReq)
	if err != nil {
		h.logger.Error(err, "Failed to search workflows",
			"filters", searchReq.Filters,
			"top_k", searchReq.TopK,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to search workflows",
		)
		return
	}

	// Calculate search duration
	duration := time.Since(startTime)

	// BR-AUDIT-023: Generate and store audit event asynchronously (non-blocking per ADR-038)
	// Use background context because the request context is cancelled when the response is sent
	if h.auditStore != nil {
		go func() {
			auditEvent, err := dsaudit.NewWorkflowSearchAuditEvent(&searchReq, response, duration)
			if err != nil {
				h.logger.Error(err, "Failed to create workflow search audit event",
					"filters", searchReq.Filters,
				)
				return
			}

			// Use a background context with a timeout for async audit storage
			// The request context is cancelled when the response is sent
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
				h.logger.Error(err, "Failed to store workflow search audit event",
					"filters", searchReq.Filters,
				)
			}
		}()
	}

	// Log success
	h.logger.Info("Workflow search completed",
		"filters", searchReq.Filters,
		"results_count", len(response.Workflows),
		"top_k", searchReq.TopK,
		"duration_ms", duration.Milliseconds(),
	)

	// Return results
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error(err, "Failed to encode workflow search response")
	}
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

	// DD-WORKFLOW-001 v1.6: 5 mandatory labels (environment and priority are now mandatory)
	// Environment filter (mandatory)
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
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to list workflows",
		)
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
//
// Returns complete workflow object including:
// - spec.container_image: OCI container image reference (for HAPI validation)
// - spec.parameters[]: Parameter schema (for LLM parameter validation)
// - detected_labels: Signal type, severity labels (for workflow filtering)
//
// Cross-Service Integration:
// - HolmesGPT-API: Uses for validate_workflow_exists tool (Q17 in handoff doc)
// - AIAnalysis: May use for defense-in-depth validation
func (h *Handler) HandleGetWorkflowByID(w http.ResponseWriter, r *http.Request) {
	// Get workflow ID from URL path
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			"workflow_id is required",
		)
		return
	}

	// Get workflow from repository
	workflow, err := h.workflowRepo.GetByID(r.Context(), workflowID)
	if err != nil {
		// Check if workflow not found
		if err.Error() == fmt.Sprintf("workflow not found: %s", workflowID) {
			h.writeRFC7807Error(w, http.StatusNotFound,
				"https://kubernaut.dev/problems/not-found",
				"Not Found",
				fmt.Sprintf("Workflow not found: %s", workflowID),
			)
			return
		}

		h.logger.Error(err, "Failed to get workflow",
			"workflow_id", workflowID,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to get workflow",
		)
		return
	}

	// Log success
	h.logger.Info("Workflow retrieved",
		"workflow_id", workflowID,
		"workflow_name", workflow.WorkflowName,
		"version", workflow.Version,
	)

	// Return workflow
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// HandleListWorkflowVersions handles GET /api/v1/workflows/by-name/{workflowName}/versions
// BR-STORAGE-014: Workflow catalog management
// DD-WORKFLOW-002 v3.0: List all versions by workflow_name
func (h *Handler) HandleListWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	// Get workflow name from URL path
	workflowName := chi.URLParam(r, "workflowName")
	if workflowName == "" {
		h.logger.Error(fmt.Errorf("workflow_name is required"), "Missing workflow_name in request")
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			"workflow_name is required",
		)
		return
	}

	// Get all versions for this workflow
	workflows, err := h.workflowRepo.GetVersionsByName(r.Context(), workflowName)
	if err != nil {
		h.logger.Error(err, "Failed to list workflow versions",
			"workflow_name", workflowName,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to list workflow versions",
		)
		return
	}

	// Log success
	h.logger.Info("Workflow versions listed",
		"workflow_name", workflowName,
		"count", len(workflows),
	)

	// Return results
	response := models.WorkflowVersionsResponse{
		WorkflowName: workflowName,
		Versions:     workflows,
		Total:        len(workflows),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error(err, "Failed to encode workflow versions response")
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
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			"workflow_id is required",
		)
		return
	}

	// Parse request body
	var updateReq models.WorkflowUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&updateReq); err != nil {
		h.logger.Error(err, "Failed to decode workflow update request")
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			fmt.Sprintf("Invalid request body: %v", err),
		)
		return
	}

	// DD-WORKFLOW-012: Validate that ONLY mutable fields are being updated
	if updateReq.Description != nil || updateReq.Content != nil || updateReq.Labels != nil {
		h.logger.Error(fmt.Errorf("immutable fields cannot be updated"), "Attempted to update immutable fields",
			"workflow_id", workflowID,
		)
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/immutable-field-violation",
			"Bad Request",
			"Cannot update immutable fields (description, content, labels). Create a new version instead. See DD-WORKFLOW-012.",
		)
		return
	}

	// Get existing workflow
	workflow, err := h.workflowRepo.GetByID(r.Context(), workflowID)
	if err != nil {
		h.logger.Error(err, "Failed to get workflow for update",
			"workflow_id", workflowID,
		)
		h.writeRFC7807Error(w, http.StatusNotFound,
			"https://kubernaut.dev/problems/not-found",
			"Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID),
		)
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
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to update workflow",
		)
		return
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

// HandleDisableWorkflow handles PATCH /api/v1/workflows/{workflowID}/disable
// DD-WORKFLOW-012: Convenience endpoint for disabling workflows (soft delete)
func (h *Handler) HandleDisableWorkflow(w http.ResponseWriter, r *http.Request) {
	// Get workflow ID from URL path
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			"workflow_id is required",
		)
		return
	}

	// Parse request body
	var disableReq models.WorkflowDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&disableReq); err != nil {
		h.logger.Error(err, "Failed to decode workflow disable request")
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			fmt.Sprintf("Invalid request body: %v", err),
		)
		return
	}

	// Get existing workflow
	workflow, err := h.workflowRepo.GetByID(r.Context(), workflowID)
	if err != nil {
		h.logger.Error(err, "Failed to get workflow for disable",
			"workflow_id", workflowID,
		)
		h.writeRFC7807Error(w, http.StatusNotFound,
			"https://kubernaut.dev/problems/not-found",
			"Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID),
		)
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

	if err := h.workflowRepo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, "disabled", reason, updatedBy); err != nil {
		h.logger.Error(err, "Failed to disable workflow",
			"workflow_id", workflowID,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to disable workflow",
		)
		return
	}

	// Update workflow object for response
	workflow.Status = "disabled"
	now := time.Now()
	workflow.DisabledAt = &now
	workflow.DisabledBy = disableReq.UpdatedBy
	workflow.DisabledReason = disableReq.Reason

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

// validateWorkflowSearchRequest validates the workflow search request
// V1.0: Label-only search validation (filters required, no query/embedding)
func (h *Handler) validateWorkflowSearchRequest(req *models.WorkflowSearchRequest) error {
	// V1.0: Filters are required for label-only search
	if req.Filters == nil {
		return fmt.Errorf("filters are required for label-only search")
	}

	// Validate mandatory filter fields (5 required)
	if req.Filters.SignalType == "" {
		return fmt.Errorf("filters.signal_type is required")
	}
	if req.Filters.Severity == "" {
		return fmt.Errorf("filters.severity is required")
	}
	if req.Filters.Component == "" {
		return fmt.Errorf("filters.component is required")
	}
	if req.Filters.Environment == "" {
		return fmt.Errorf("filters.environment is required")
	}
	if req.Filters.Priority == "" {
		return fmt.Errorf("filters.priority is required")
	}

	// Validate TopK
	if req.TopK <= 0 {
		req.TopK = 10 // Default to 10 results
	}
	if req.TopK > 100 {
		req.TopK = 100 // Max 100 results
	}

	// Validate MinScore (replaces MinSimilarity)
	if req.MinScore < 0 || req.MinScore > 1 {
		return fmt.Errorf("min_score must be between 0 and 1")
	}

	return nil
}
