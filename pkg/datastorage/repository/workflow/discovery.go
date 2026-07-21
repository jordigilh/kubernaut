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

package workflow

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// THREE-STEP WORKFLOW DISCOVERY REPOSITORY
// ========================================
// Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
// Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
// Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
//
// Step 1: ListActions -- list action types with active workflow counts
// Step 2: ListWorkflowsByActionType -- list workflows for an action type
// Step 3: GetWorkflowWithContextFilters -- get workflow with security gate
//
// Issue #1661 Phase C: all three methods' Postgres SQL fallback (and its
// helper query-builders: countActionTypes/selectActionTypeRows/
// actionTypeRow/actionTypeRowsToEntries/selectScoredWorkflows/
// workflowWithScore/buildContextFilterSQL/appendMandatoryLabelConditions/
// appendDetectedLabelConditions) were deleted -- Repository.cache is
// unconditionally non-nil in production (validateServerDeps requires
// ServerDeps.K8sRestConfig, Phase 55), so the `if r.cache != nil` guards
// were already dead in production, kept alive only by tests. The
// cache-backed implementations (discovery_cache.go) have their own
// dedicated coverage (discovery_cache_test.go, scoring_test.go,
// list_cache_test.go).
// ========================================

// ListActions returns action types from the taxonomy that have active workflows
// matching the provided signal context filters (Step 1 of discovery protocol).
// Returns action type entries with workflow counts, total count for pagination, and error.
func (r *Repository) ListActions(ctx context.Context, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.ActionTypeEntry, int, error) {
	return r.listActionsFromCache(ctx, filters, offset, limit)
}

// ListWorkflowsByActionType returns active workflows matching the specified action type
// and signal context filters (Step 2 of discovery protocol).
// #220: Results are scored and ordered by final_score DESC per DD-WORKFLOW-016.
// Returns workflow list, total count for pagination, and error.
func (r *Repository) ListWorkflowsByActionType(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.RemediationWorkflow, int, error) {
	return r.listWorkflowsByActionTypeFromCache(ctx, actionType, filters, offset, limit)
}

// GetWorkflowWithContextFilters retrieves a workflow by ID with an additional
// security gate that verifies the workflow matches the provided context filters.
// Returns ErrNotFound if the workflow doesn't exist OR exists but doesn't match
// the context (security gate) — DD-WORKFLOW-016: the two cases are
// deliberately not distinguished to prevent information leakage.
// This is Step 3 of the discovery protocol.
func (r *Repository) GetWorkflowWithContextFilters(ctx context.Context, workflowID string, filters *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	// If no context filters, fall back to simple GetByID
	if filters == nil || !filters.HasContextFilters() {
		return r.GetByID(ctx, workflowID)
	}

	return r.getWorkflowWithContextFiltersFromCache(ctx, workflowID, filters)
}
