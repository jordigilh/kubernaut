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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

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
//         (see workflow_query_handlers.go for the Step 3 handler)
//
// Split from workflow_handlers.go (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3,
// pure code motion, no behavior change).

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
			"invalid discovery filter parameters", h.logger)
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
			"invalid discovery filter parameters", h.logger)
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
		if wf.ServiceAccountName != nil {
			entry.ServiceAccountName = *wf.ServiceAccountName
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
// DD-WORKFLOW-017: Content hash is derived from the raw YAML content (inline or OCI).
func computeContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
