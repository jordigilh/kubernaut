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
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// HandleGetActionTypeWorkflowCount handles GET /api/v1/action-types/{name}/workflow-count.
// BR-WORKFLOW-007: Returns the number of active workflows referencing this action type.
//
// #1661 Phase 55c: ported from actionTypeRepo.CountActiveWorkflows (a direct
// `SELECT ... FROM remediation_workflow_catalog WHERE action_type = $1 AND
// status = 'Active'` Postgres query) to the informer-backed workflowCache
// (DD-WORKFLOW-018). etcd/the RemediationWorkflow CRD is now the sole source
// of truth for workflow existence; Postgres no longer has a catalog table to
// query.
//
// #1661 Phase A3: this is the sole surviving handler in this file --
// HandleCreateActionType/HandleUpdateActionType/HandleDisableActionType (and
// their request/response types, audit helpers, and toDescPayload) were
// deleted alongside the createActionType/updateActionType/disableActionType
// OpenAPI operations (DD-WORKFLOW-018 -- AuthWebhook computes/patches
// ActionType CRD lifecycle entirely locally; there is no DS-side mutation
// path left to serve).
func (h *Handler) HandleGetActionTypeWorkflowCount(w http.ResponseWriter, r *http.Request) {
	if h.workflowCache == nil {
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "not-configured",
			"Service Not Configured", "workflow cache not initialized", h.logger)
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error",
			"Validation Error", "action type name is required in URL path", h.logger)
		return
	}

	workflows, err := h.workflowCache.ListWorkflowsByActionType(r.Context(), name)
	if err != nil {
		h.logger.Error(err, "Failed to count active workflows for action type", "name", name)
		response.WriteRFC7807InternalError(w, "cache-error", "Cache Error", err, h.logger)
		return
	}

	count := 0
	for _, wf := range workflows {
		if wf.Status.CatalogStatus == sharedtypes.CatalogStatusActive {
			count++
		}
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
