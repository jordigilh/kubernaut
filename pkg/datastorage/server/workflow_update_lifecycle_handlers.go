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
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	workflowrepo "github.com/jordigilh/kubernaut/pkg/datastorage/repository/workflow"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// ========================================
// WORKFLOW CATALOG HANDLERS — UPDATE / LIFECYCLE
// ========================================
// - PATCH /api/v1/workflows/{workflowID} - Update mutable fields
// - PATCH /api/v1/workflows/{workflowID}/enable - Re-enable a workflow
// - PATCH /api/v1/workflows/{workflowID}/deprecate - Deprecate a workflow
// - PATCH /api/v1/workflows/{workflowID}/disable - Soft-delete workflow
//
// Split from workflow_handlers.go (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3,
// pure code motion, no behavior change).

// HandleUpdateWorkflow handles PATCH /api/v1/workflows/{workflowID}
// DD-WORKFLOW-012: Update ONLY mutable fields (status, metrics)
// Immutable fields (description, content, labels) require creating a new version
func (h *Handler) HandleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	updateReq, ok := h.decodeWorkflowUpdateRequest(w, r, workflowID)
	if !ok {
		return
	}

	workflow, err := h.workflowRepo.GetByID(r.Context(), workflowID)
	if err != nil {
		// Issue #1674: previously this branch always returned 404 for ANY
		// error, including real DB failures — masking outages as "not
		// found" and, worse, GetByID's old (nil, nil) "not found" contract
		// meant err was nil here, so this guard never even fired for the
		// not-found case: workflow stayed nil and reached
		// applyMutableWorkflowUpdate below, dereferencing workflow.Status
		// on a nil pointer whenever the request set "status" (panic,
		// recovered as an HTTP 500). Now that GetByID returns
		// workflowrepo.ErrNotFound, the two cases are distinguished
		// correctly.
		if errors.Is(err, workflowrepo.ErrNotFound) {
			response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
				fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
			return
		}
		h.logger.Error(err, "Failed to get workflow for update",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to get workflow", h.logger)
		return
	}

	if !h.applyMutableWorkflowUpdate(w, workflow, updateReq) {
		return
	}

	if err := h.workflowRepo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, workflow.Status, getStringValue(workflow.DisabledReason), getStringValue(workflow.DisabledBy)); err != nil {
		h.logger.Error(err, "Failed to update workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to update workflow", h.logger)
		return
	}

	// BR-STORAGE-183: Audit workflow update (business logic operation)
	h.auditWorkflowUpdateAsync(workflow, updateReq) //nolint:contextcheck // auditWorkflowUpdateAsync emits in a background goroutine by design, decoupled from the request lifecycle

	h.logger.Info("Workflow updated",
		"workflow_id", workflowID,
		"status", workflow.Status,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// decodeWorkflowUpdateRequest decodes the PATCH body and enforces
// DD-WORKFLOW-012: only mutable fields (status, disabledBy, disabledReason)
// may be updated; description/content/labels require a new version instead.
func (h *Handler) decodeWorkflowUpdateRequest(w http.ResponseWriter, r *http.Request, workflowID string) (models.WorkflowUpdateRequest, bool) {
	var updateReq models.WorkflowUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&updateReq); err != nil {
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, h.logger)
			return updateReq, false
		}
		h.logger.Error(err, "Failed to decode workflow update request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"request body is not valid JSON", h.logger)
		return updateReq, false
	}

	if updateReq.Description != nil || updateReq.Content != nil || updateReq.Labels != nil {
		h.logger.Error(fmt.Errorf("immutable fields cannot be updated"), "Attempted to update immutable fields",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "immutable-field-violation", "Bad Request",
			"Cannot update immutable fields (description, content, labels). Create a new version instead. See DD-WORKFLOW-012.", h.logger)
		return updateReq, false
	}

	return updateReq, true
}

// applyMutableWorkflowUpdate mutates workflow in place per updateReq.Status,
// validating the status transition first (workflowCatalogStatusTransitionForbidden).
// Returns false after writing the RFC 7807 conflict response if the
// transition is forbidden.
func (h *Handler) applyMutableWorkflowUpdate(w http.ResponseWriter, workflow *models.RemediationWorkflow, updateReq models.WorkflowUpdateRequest) bool {
	if updateReq.Status == nil {
		return true
	}

	if workflowCatalogStatusTransitionForbidden(workflow.Status, *updateReq.Status) {
		response.WriteRFC7807Error(w, http.StatusConflict, "workflow-status-conflict", "Conflict",
			fmt.Sprintf("invalid workflow status transition from %s to %s", workflow.Status, *updateReq.Status), h.logger)
		return false
	}

	workflow.Status = *updateReq.Status
	if *updateReq.Status == models.WorkflowStatusDisabled {
		now := time.Now()
		workflow.DisabledAt = &now
		workflow.DisabledBy = updateReq.DisabledBy
		workflow.DisabledReason = updateReq.DisabledReason
	}
	return true
}

// auditWorkflowUpdateAsync emits the workflow-updated audit event in a
// background goroutine (DD-AUDIT-002 V2.0.1: workflow state changes are
// business logic). Values are copied before the goroutine to avoid a data
// race on the shared workflow pointer (Issue #674 Bug 11).
func (h *Handler) auditWorkflowUpdateAsync(workflow *models.RemediationWorkflow, updateReq models.WorkflowUpdateRequest) {
	if h.auditStore == nil {
		return
	}

	wfStatus := workflow.Status
	wfID := workflow.WorkflowID
	wfDisabledBy := getStringValue(workflow.DisabledBy)
	wfDisabledReason := getStringValue(workflow.DisabledReason)
	isDisabling := updateReq.Status != nil && *updateReq.Status == models.WorkflowStatusDisabled

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		updatedFields := api.WorkflowCatalogUpdatedFields{}
		updatedFields.Status.SetTo(wfStatus)

		if isDisabling {
			if wfDisabledBy != "" {
				updatedFields.DisabledBy.SetTo(wfDisabledBy)
			}
			if wfDisabledReason != "" {
				updatedFields.DisabledReason.SetTo(wfDisabledReason)
			}
		}

		auditEvent, err := dsaudit.NewWorkflowUpdatedAuditEvent(wfID, updatedFields)
		if err != nil {
			h.logger.Error(err, "Failed to create workflow update audit event",
				"workflow_id", wfID,
			)
			return
		}

		if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
			h.logger.Error(err, "Failed to audit workflow update",
				"workflow_id", wfID,
			)
		}
	}()
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

// workflowCatalogStatusTransitionForbidden returns true when a catalog status PATCH should be rejected.
// Valid: Active→Disabled, Active→Deprecated, Active→Superseded, Disabled→Active.
// Terminal states (DD-WORKFLOW-017): Superseded and Deprecated cannot transition to any other status.
// Same-status transitions are no-ops and always allowed.
func workflowCatalogStatusTransitionForbidden(fromStatus, toStatus string) bool {
	if fromStatus == toStatus {
		return false
	}
	return fromStatus == "Superseded" || fromStatus == "Deprecated"
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
// parseWorkflowLifecycleRequest decodes a lifecycle PATCH request body
// (enable/deprecate/disable), enforcing the DD-WORKFLOW-017 Phase 4.4
// (GAP-WF-5) mandatory reason field. opName customizes error logging.
func (h *Handler) parseWorkflowLifecycleRequest(w http.ResponseWriter, r *http.Request, opName string) (models.WorkflowDisableRequest, bool) {
	var req models.WorkflowDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, h.logger)
			return req, false
		}
		h.logger.Error(err, fmt.Sprintf("Failed to decode workflow %s request", opName))
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"request body is not valid JSON", h.logger)
		return req, false
	}

	if req.Reason == nil || strings.TrimSpace(*req.Reason) == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-reason", "Missing Required Field",
			"reason is required for lifecycle operations", h.logger)
		return req, false
	}

	return req, true
}

// getWorkflowForLifecycleTransition resolves the workflow lifecycle
// repository, loads the target workflow, and validates the requested status
// transition (workflowCatalogStatusTransitionForbidden). opName customizes
// error logging; newStatus is the transition's target status.
func (h *Handler) getWorkflowForLifecycleTransition(w http.ResponseWriter, r *http.Request, workflowID, opName, newStatus string) (WorkflowLifecycleRepository, *models.RemediationWorkflow, bool) {
	repo := h.getWorkflowLifecycleRepo()
	if repo == nil {
		h.logger.Error(fmt.Errorf("workflow repository not configured"), "Handler misconfiguration")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Workflow repository not configured", h.logger)
		return nil, nil, false
	}

	workflow, err := repo.GetByID(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, workflowrepo.ErrNotFound) {
			response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
				fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
			return nil, nil, false
		}
		h.logger.Error(err, fmt.Sprintf("Failed to get workflow for %s", opName),
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to get workflow", h.logger)
		return nil, nil, false
	}
	// Defensive: repo is the WorkflowLifecycleRepository interface, and test
	// doubles (see workflow_lifecycle_handler_test.go) may still return
	// (nil, nil) directly rather than the workflowrepo.ErrNotFound sentinel.
	if workflow == nil {
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("Workflow not found: %s", workflowID), h.logger)
		return nil, nil, false
	}

	if workflowCatalogStatusTransitionForbidden(workflow.Status, newStatus) {
		response.WriteRFC7807Error(w, http.StatusConflict, "workflow-status-conflict", "Conflict",
			fmt.Sprintf("invalid workflow status transition from %s to %s", workflow.Status, newStatus), h.logger)
		return nil, nil, false
	}

	return repo, workflow, true
}

// auditWorkflowLifecycleChange emits a workflow.updated audit event
// (BR-STORAGE-183) in a background goroutine for an enable/deprecate/disable
// transition. configureFields lets each caller populate operation-specific
// fields (e.g. DisabledBy/DisabledReason) beyond the always-set Status field.
func (h *Handler) auditWorkflowLifecycleChange(workflowID, newStatus string, workflow *models.RemediationWorkflow, opName string, configureFields func(*api.WorkflowCatalogUpdatedFields)) {
	if h.auditStore == nil {
		return
	}
	wfID := workflow.WorkflowID

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		updatedFields := api.WorkflowCatalogUpdatedFields{}
		updatedFields.Status.SetTo(newStatus)
		if configureFields != nil {
			configureFields(&updatedFields)
		}

		auditEvent, err := dsaudit.NewWorkflowUpdatedAuditEvent(wfID, updatedFields)
		if err != nil {
			h.logger.Error(err, fmt.Sprintf("Failed to create workflow %s audit event", opName),
				"workflow_id", workflowID,
			)
			return
		}

		if err := h.auditStore.StoreAudit(ctx, auditEvent); err != nil {
			h.logger.Error(err, fmt.Sprintf("Failed to audit workflow %s", opName),
				"workflow_id", workflowID,
			)
		}
	}()
}

func (h *Handler) HandleEnableWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	req, ok := h.parseWorkflowLifecycleRequest(w, r, "enable")
	if !ok {
		return
	}

	repo, workflow, ok := h.getWorkflowForLifecycleTransition(w, r, workflowID, "enable", "Active")
	if !ok {
		return
	}

	reason := getStringValue(req.Reason)
	updatedBy := getStringValue(req.UpdatedBy)

	if err := repo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, "Active", reason, updatedBy); err != nil {
		h.logger.Error(err, "Failed to enable workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to enable workflow", h.logger)
		return
	}

	workflow.Status = "Active"
	workflow.DisabledAt = nil
	workflow.DisabledBy = nil
	workflow.DisabledReason = nil

	h.auditWorkflowLifecycleChange(workflowID, "Active", workflow, "enable", nil) //nolint:contextcheck // auditWorkflowLifecycleChange emits in a background goroutine (BR-STORAGE-183); see doc comment

	h.logger.Info("Workflow enabled",
		"workflow_id", workflowID,
		"reason", reason,
		"updated_by", updatedBy,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// HandleDeprecateWorkflow handles PATCH /api/v1/workflows/{workflowID}/deprecate
// DD-WORKFLOW-017 Phase 4.4 (GAP-WF-1): Convenience endpoint for deprecating workflows
func (h *Handler) HandleDeprecateWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	req, ok := h.parseWorkflowLifecycleRequest(w, r, "deprecate")
	if !ok {
		return
	}

	// DD-WORKFLOW-017: Superseded and Deprecated are terminal states
	repo, workflow, ok := h.getWorkflowForLifecycleTransition(w, r, workflowID, "deprecate", "Deprecated")
	if !ok {
		return
	}

	reason := getStringValue(req.Reason)
	updatedBy := getStringValue(req.UpdatedBy)

	if err := repo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, "Deprecated", reason, updatedBy); err != nil {
		h.logger.Error(err, "Failed to deprecate workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to deprecate workflow", h.logger)
		return
	}

	workflow.Status = "Deprecated"

	h.auditWorkflowLifecycleChange(workflowID, "Deprecated", workflow, "deprecate", nil) //nolint:contextcheck // auditWorkflowLifecycleChange emits in a background goroutine (BR-STORAGE-183); see doc comment

	h.logger.Info("Workflow deprecated",
		"workflow_id", workflowID,
		"reason", reason,
		"updated_by", updatedBy,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}

// HandleDisableWorkflow handles PATCH /api/v1/workflows/{workflowID}/disable
// DD-WORKFLOW-012: Convenience endpoint for disabling workflows (soft delete)
func (h *Handler) HandleDisableWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "workflowID")
	if workflowID == "" {
		h.logger.Error(fmt.Errorf("workflow_id is required"), "Missing workflow_id in request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request", "Bad Request",
			"workflow_id is required", h.logger)
		return
	}

	disableReq, ok := h.parseWorkflowLifecycleRequest(w, r, "disable")
	if !ok {
		return
	}

	repo, workflow, ok := h.getWorkflowForLifecycleTransition(w, r, workflowID, "disable", models.WorkflowStatusDisabled)
	if !ok {
		return
	}

	reason := getStringValue(disableReq.Reason)
	updatedBy := getStringValue(disableReq.UpdatedBy)

	if err := repo.UpdateStatus(r.Context(), workflow.WorkflowID, workflow.Version, models.WorkflowStatusDisabled, reason, updatedBy); err != nil {
		h.logger.Error(err, "Failed to disable workflow",
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "internal-error", "Internal Server Error",
			"Failed to disable workflow", h.logger)
		return
	}

	workflow.Status = models.WorkflowStatusDisabled
	now := time.Now()
	workflow.DisabledAt = &now
	workflow.DisabledBy = disableReq.UpdatedBy
	workflow.DisabledReason = disableReq.Reason

	// DD-AUDIT-002 V2.0.1: Workflow disable is a status update (captured via workflow.updated)
	h.auditWorkflowLifecycleChange(workflowID, models.WorkflowStatusDisabled, workflow, "disable", func(fields *api.WorkflowCatalogUpdatedFields) { //nolint:contextcheck // auditWorkflowLifecycleChange emits in a background goroutine (BR-STORAGE-183); see doc comment
		fields.DisabledBy.SetTo(updatedBy)
		fields.DisabledReason.SetTo(reason)
	})

	h.logger.Info("Workflow disabled",
		"workflow_id", workflowID,
		"reason", reason,
		"disabled_by", updatedBy,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(workflow); err != nil {
		h.logger.Error(err, "Failed to encode workflow response")
	}
}
