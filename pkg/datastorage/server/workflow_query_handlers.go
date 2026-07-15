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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// ========================================
// WORKFLOW CATALOG HANDLERS — LIST / GET
// ========================================
// BR-STORAGE-014: Workflow catalog management
// BR-STORAGE-039: Workflow Catalog Retrieval API
//
// - GET /api/v1/workflows - List workflows with filters
// - GET /api/v1/workflows/{workflowID} - Get workflow by ID (with optional security gate)
//
// Split from workflow_handlers.go (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3,
// pure code motion, no behavior change).

// parseWorkflowSearchFilters extracts the HandleListWorkflows query filters:
// status, environment (DD-WORKFLOW-001 v2.5 single-value), priority,
// component, workflow_name (exact match, DD-API-001), and subdomain-based
// custom_labels[subdomain]=v1,v2 (DD-WORKFLOW-001 v1.5).
func parseWorkflowSearchFilters(r *http.Request) *models.WorkflowSearchFilters {
	filters := &models.WorkflowSearchFilters{}
	q := r.URL.Query()

	if status := q.Get("status"); status != "" {
		filters.Status = []string{status}
	}
	if env := q.Get("environment"); env != "" {
		filters.Environment = env
	}
	if priority := q.Get("priority"); priority != "" {
		filters.Priority = priority
	}
	if component := q.Get("component"); component != "" {
		filters.Component = component
	}
	// Authority: DD-API-001 (OpenAPI client mandatory - added in Jan 2026)
	// Used for test idempotency and workflow lookup by human-readable name
	if workflowName := q.Get("workflow_name"); workflowName != "" {
		filters.WorkflowName = workflowName
	}

	// Format: custom_labels[subdomain]=value1,value2
	// Example: custom_labels[constraint]=cost-constrained,stateful-safe
	for key, values := range q {
		if !strings.HasPrefix(key, "custom_labels[") || !strings.HasSuffix(key, "]") {
			continue
		}
		subdomain := strings.TrimSuffix(strings.TrimPrefix(key, "custom_labels["), "]")
		if subdomain == "" || len(values) == 0 {
			continue
		}
		if filters.CustomLabels == nil {
			filters.CustomLabels = make(map[string][]string)
		}
		for _, v := range values {
			filters.CustomLabels[subdomain] = append(filters.CustomLabels[subdomain], strings.Split(v, ",")...)
		}
	}

	return filters
}

// parseListPagination extracts limit/offset query parameters with defaults
// (limit=50, capped at 100) and invalid-input fallbacks.
func parseListPagination(r *http.Request) (limit, offset int) {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	offset, err = strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	return limit, offset
}

// HandleListWorkflows handles GET /api/v1/workflows
// BR-STORAGE-014: Workflow catalog management
func (h *Handler) HandleListWorkflows(w http.ResponseWriter, r *http.Request) {
	filters := parseWorkflowSearchFilters(r)
	limit, offset := parseListPagination(r)

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

	// Issue #1661 Change 7 (DD-WORKFLOW-018): total_executions/successful_executions/
	// actual_success_rate are computed on demand from audit_events, not stored catalog columns.
	h.overlaySuccessMetrics(r.Context(), workflowPtrs)

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
// - spec.schema_image: OCI container image reference (nil for inline registrations)
// - spec.parameters[]: Parameter schema (for LLM parameter validation)
// - detected_labels: Signal type, severity labels (for workflow filtering)
//
// Security Gate (when context filters are present):
// - Returns 404 if workflow exists but doesn't match context (DD-WORKFLOW-016)
// - Intentionally doesn't distinguish "not found" from "filtered out" to prevent info leakage
// - Emits workflow.catalog.workflow_retrieved audit event
//
// Cross-Service Integration:
// - KA: Uses for get_workflow tool (Step 3 of discovery protocol)
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
			"invalid discovery filter parameters", h.logger)
		return
	}

	wf, durationMs, ok := h.fetchWorkflowByIDWithGate(w, r, workflowID, filters)
	if !ok {
		return
	}

	// Issue #1661 Change 7 (DD-WORKFLOW-018): total_executions/successful_executions/
	// actual_success_rate are computed on demand from audit_events, not stored catalog columns.
	h.overlaySuccessMetrics(r.Context(), []*models.RemediationWorkflow{wf})

	// BR-AUDIT-023: Emit discovery audit events when context filters are present.
	h.emitWorkflowRetrievedAuditEvents(workflowID, filters, durationMs)

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

// fetchWorkflowByIDWithGate implements the query-and-error-handling portion
// of HandleGetWorkflowByID: run the context-filtered or plain GetByID query
// (DD-WORKFLOW-016 security gate) and normalize "not found" vs. "filtered
// out" vs. real errors into a single RFC 7807 404/500 (intentionally not
// distinguishing "not found" from "filtered out" to prevent info leakage).
// On any failure it writes the response itself and returns ok=false.
// Extracted from HandleGetWorkflowByID (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func (h *Handler) fetchWorkflowByIDWithGate(w http.ResponseWriter, r *http.Request, workflowID string, filters *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, int64, bool) {
	// DD-WORKFLOW-016: Use context-filtered query when filters are present (security gate)
	// GAP-WF-6: Measure query duration for audit payload (DD-WORKFLOW-014 v3.0)
	startGet := time.Now()
	var wf *models.RemediationWorkflow
	var err error
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
			return nil, 0, false
		}

		h.logger.Error(err, "Failed to get workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to get workflow", h.logger)
		return nil, 0, false
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
		return nil, 0, false
	}

	return wf, durationMs, true
}

// emitWorkflowRetrievedAuditEvents emits the workflow_retrieved and
// selection_validated audit events when context filters are present
// (DD-WORKFLOW-014 v3.0: context filters indicate KA is validating its
// selection, so both events are emitted together). No-op when there are no
// context filters or no audit store configured. Extracted from
// HandleGetWorkflowByID (Wave 6 6f GREEN: funlen/nestif remediation) — pure
// code motion, no behavior change.
func (h *Handler) emitWorkflowRetrievedAuditEvents(workflowID string, filters *models.WorkflowDiscoveryFilters, durationMs int64) {
	if !filters.HasContextFilters() || h.auditStore == nil {
		return
	}

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
