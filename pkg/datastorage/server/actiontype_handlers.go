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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	dsaudit "github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	actiontyperepo "github.com/jordigilh/kubernaut/pkg/datastorage/repository/actiontype"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// actionTypeCreateRequest is the request body for POST /api/v1/action-types.
type actionTypeCreateRequest struct {
	Name         string                       `json:"name"`
	Description  models.ActionTypeDescription `json:"description"`
	RegisteredBy string                       `json:"registeredBy"`
}

// actionTypeCreateResponse is the response for POST /api/v1/action-types.
type actionTypeCreateResponse struct {
	ActionType   string                       `json:"actionType"`
	Description  models.ActionTypeDescription `json:"description"`
	Status       string                       `json:"status"`
	WasReenabled bool                         `json:"wasReenabled"`
}

// actionTypeUpdateRequest is the request body for PATCH /api/v1/action-types/{name}.
type actionTypeUpdateRequest struct {
	Description models.ActionTypeDescription `json:"description"`
	UpdatedBy   string                       `json:"updatedBy"`
}

// actionTypeUpdateResponse is the response for PATCH /api/v1/action-types/{name}.
type actionTypeUpdateResponse struct {
	ActionType     string                       `json:"actionType"`
	OldDescription models.ActionTypeDescription `json:"oldDescription"`
	NewDescription models.ActionTypeDescription `json:"newDescription"`
	UpdatedFields  []string                     `json:"updatedFields"`
}

// actionTypeDisableRequest is the request body for PATCH /api/v1/action-types/{name}/disable.
type actionTypeDisableRequest struct {
	DisabledBy string `json:"disabledBy"`
	// Force enables orphan recovery (#512): disable only the named workflows
	// before attempting to disable the action type.
	Force              bool     `json:"force,omitempty"`
	OrphanedWorkflows  []string `json:"orphanedWorkflows,omitempty"`
}

// actionTypeDisableResponse is the response for PATCH /api/v1/action-types/{name}/disable.
type actionTypeDisableResponse struct {
	ActionType string `json:"actionType"`
	Status     string `json:"status"`
}

// actionTypeDisableDeniedResponse is the 409 response when disable is denied.
type actionTypeDisableDeniedResponse struct {
	ActionType             string   `json:"actionType"`
	DependentWorkflowCount int      `json:"dependentWorkflowCount"`
	DependentWorkflows     []string `json:"dependentWorkflows"`
}

// HandleCreateActionType handles POST /api/v1/action-types.
// BR-WORKFLOW-007.1: Idempotent CREATE — NOOP if active, re-enable if disabled, create if new.
func (h *Handler) HandleCreateActionType(w http.ResponseWriter, r *http.Request) {
	if h.actionTypeRepo == nil {
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "not-configured",
			"Service Not Configured", "ActionType repository not initialized", h.logger)
		return
	}

	var req actionTypeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error(err, "Failed to decode action type create request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request",
			"Bad Request", fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	if req.Name == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error",
			"Validation Error", "name is required", h.logger)
		return
	}
	if req.Description.What == "" || req.Description.WhenToUse == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error",
			"Validation Error", "description.what and description.whenToUse are required", h.logger)
		return
	}

	result, err := h.actionTypeRepo.Create(r.Context(), req.Name, req.Description, req.RegisteredBy)
	if err != nil {
		h.logger.Error(err, "Failed to create action type", "name", req.Name)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error",
			"Database Error", fmt.Sprintf("Failed to create action type: %v", err), h.logger)
		return
	}

	var descOut models.ActionTypeDescription
	if result.ActionType != nil {
		_ = json.Unmarshal(result.ActionType.Description, &descOut)
	}

	resp := actionTypeCreateResponse{
		ActionType:   req.Name,
		Description:  descOut,
		Status:       result.Status,
		WasReenabled: result.WasReenabled,
	}

	statusCode := http.StatusOK
	if result.Status == "created" {
		statusCode = http.StatusCreated
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error(err, "Failed to encode action type create response")
	}

	h.logger.Info("Action type create handled",
		"name", req.Name,
		"status", result.Status,
		"was_reenabled", result.WasReenabled,
	)

	// Emit audit (only for state changes: created or reenabled, NOT for NOOP)
	if h.auditStore != nil && result.Status != "exists" {
		desc := toDescPayload(req.Description)
		var auditEvent *api.AuditEventRequest
		var auditErr error
		if result.WasReenabled && result.ActionType != nil {
			disabledAt := time.Time{}
			disabledBy := ""
			if result.ActionType.DisabledAt != nil {
				disabledAt = *result.ActionType.DisabledAt
			}
			if result.ActionType.DisabledBy != nil {
				disabledBy = *result.ActionType.DisabledBy
			}
			auditEvent, auditErr = dsaudit.NewActionTypeReenabledAuditEvent(req.Name, req.RegisteredBy, disabledAt, disabledBy)
		} else {
			auditEvent, auditErr = dsaudit.NewActionTypeCreatedAuditEvent(req.Name, desc, req.RegisteredBy, false)
		}
		if auditErr == nil && auditEvent != nil {
			h.emitAuditEventsAsync([]*api.AuditEventRequest{auditEvent}, "action_type", req.Name)
		}
	}
}

// HandleUpdateActionType handles PATCH /api/v1/action-types/{name}.
// BR-WORKFLOW-007.2: Only spec.description is mutable.
func (h *Handler) HandleUpdateActionType(w http.ResponseWriter, r *http.Request) {
	if h.actionTypeRepo == nil {
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "not-configured",
			"Service Not Configured", "ActionType repository not initialized", h.logger)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error",
			"Validation Error", "action type name is required in URL path", h.logger)
		return
	}

	var req actionTypeUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error(err, "Failed to decode action type update request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request",
			"Bad Request", fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	if req.Description.What == "" || req.Description.WhenToUse == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error",
			"Validation Error", "description.what and description.whenToUse are required", h.logger)
		return
	}

	result, err := h.actionTypeRepo.UpdateDescription(r.Context(), name, req.Description)
	if err != nil {
		h.logger.Error(err, "Failed to update action type", "name", name)
		if errors.Is(err, actiontyperepo.ErrActionTypeNotFound) {
			response.WriteRFC7807Error(w, http.StatusNotFound, "not-found",
				"Not Found", fmt.Sprintf("Action type %q not found", name), h.logger)
			return
		}
		if errors.Is(err, actiontyperepo.ErrActionTypeDisabled) {
			response.WriteRFC7807Error(w, http.StatusConflict, "action-type-disabled",
				"Action Type Disabled", fmt.Sprintf("Action type %q is disabled and cannot be updated", name), h.logger)
			return
		}
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error",
			"Database Error", fmt.Sprintf("Failed to update action type: %v", err), h.logger)
		return
	}

	resp := actionTypeUpdateResponse{
		ActionType:     name,
		OldDescription: result.OldDescription,
		NewDescription: result.NewDescription,
		UpdatedFields:  result.UpdatedFields,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error(err, "Failed to encode action type update response")
	}

	h.logger.Info("Action type updated",
		"name", name,
		"updated_fields", result.UpdatedFields,
	)

	if h.auditStore != nil {
		auditEvent, auditErr := dsaudit.NewActionTypeUpdatedAuditEvent(
			name,
			toDescPayload(result.OldDescription),
			toDescPayload(result.NewDescription),
			req.UpdatedBy,
			result.UpdatedFields,
		)
		if auditErr == nil && auditEvent != nil {
			h.emitAuditEventsAsync([]*api.AuditEventRequest{auditEvent}, "action_type", name)
		}
	}
}

// HandleDisableActionType handles PATCH /api/v1/action-types/{name}/disable.
// BR-WORKFLOW-007.3: Soft-disable with dependency guard.
func (h *Handler) HandleDisableActionType(w http.ResponseWriter, r *http.Request) {
	if h.actionTypeRepo == nil {
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "not-configured",
			"Service Not Configured", "ActionType repository not initialized", h.logger)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error",
			"Validation Error", "action type name is required in URL path", h.logger)
		return
	}

	var req actionTypeDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error(err, "Failed to decode action type disable request")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "bad-request",
			"Bad Request", fmt.Sprintf("Invalid request body: %v", err), h.logger)
		return
	}

	var (
		result *actiontyperepo.DisableResult
		disableErr error
	)
	if req.Force && len(req.OrphanedWorkflows) > 0 {
		result, disableErr = h.actionTypeRepo.ForceDisable(r.Context(), name, req.DisabledBy, req.OrphanedWorkflows)
	} else {
		result, disableErr = h.actionTypeRepo.Disable(r.Context(), name, req.DisabledBy)
	}
	if disableErr != nil {
		h.logger.Error(disableErr, "Failed to disable action type", "name", name, "force", req.Force)
		if errors.Is(disableErr, actiontyperepo.ErrActionTypeNotFound) {
			response.WriteRFC7807Error(w, http.StatusNotFound, "not-found",
				"Not Found", fmt.Sprintf("Action type %q not found", name), h.logger)
			return
		}
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error",
			"Database Error", fmt.Sprintf("Failed to disable action type: %v", disableErr), h.logger)
		return
	}

	if !result.Disabled {
		resp := actionTypeDisableDeniedResponse{
			ActionType:             name,
			DependentWorkflowCount: result.DependentWorkflowCount,
			DependentWorkflows:     result.DependentWorkflows,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			h.logger.Error(err, "Failed to encode disable denied response")
		}
		h.logger.Info("Action type disable denied — active workflows exist",
			"name", name,
			"dependent_count", result.DependentWorkflowCount,
			"dependent_workflows", result.DependentWorkflows,
		)

		if h.auditStore != nil {
			auditEvent, auditErr := dsaudit.NewActionTypeDisableDeniedAuditEvent(
				name, req.DisabledBy, result.DependentWorkflowCount, result.DependentWorkflows,
			)
			if auditErr == nil && auditEvent != nil {
				h.emitAuditEventsAsync([]*api.AuditEventRequest{auditEvent}, "action_type", name)
			}
		}
		return
	}

	resp := actionTypeDisableResponse{
		ActionType: name,
		Status:     "disabled",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error(err, "Failed to encode disable response")
	}

	h.logger.Info("Action type disabled",
		"name", name,
		"disabled_by", req.DisabledBy,
	)

	if h.auditStore != nil {
		auditEvent, auditErr := dsaudit.NewActionTypeDisabledAuditEvent(name, req.DisabledBy, time.Now())
		if auditErr == nil && auditEvent != nil {
			h.emitAuditEventsAsync([]*api.AuditEventRequest{auditEvent}, "action_type", name)
		}
	}
}

// HandleGetActionTypeWorkflowCount handles GET /api/v1/action-types/{name}/workflow-count.
// BR-WORKFLOW-007: Returns the number of active workflows referencing this action type.
func (h *Handler) HandleGetActionTypeWorkflowCount(w http.ResponseWriter, r *http.Request) {
	if h.actionTypeRepo == nil {
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "not-configured",
			"Service Not Configured", "ActionType repository not initialized", h.logger)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error",
			"Validation Error", "action type name is required in URL path", h.logger)
		return
	}

	count, _, err := h.actionTypeRepo.CountActiveWorkflows(r.Context(), name)
	if err != nil {
		h.logger.Error(err, "Failed to count active workflows", "name", name)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error",
			"Database Error", fmt.Sprintf("Failed to count active workflows: %v", err), h.logger)
		return
	}

	type countResponse struct {
		Count int `json:"count"`
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(countResponse{Count: count}); err != nil {
		h.logger.Error(err, "Failed to encode workflow count response")
	}
}

// toDescPayload converts an internal ActionTypeDescription to the ogen audit payload struct.
func toDescPayload(d models.ActionTypeDescription) api.ActionTypeDescriptionPayload {
	p := api.ActionTypeDescriptionPayload{
		What:      d.What,
		WhenToUse: d.WhenToUse,
	}
	if d.WhenNotToUse != "" {
		p.WhenNotToUse.SetTo(d.WhenNotToUse)
	}
	if d.Preconditions != "" {
		p.Preconditions.SetTo(d.Preconditions)
	}
	return p
}
