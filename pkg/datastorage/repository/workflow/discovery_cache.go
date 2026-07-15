/*
Copyright 2026 Jordi Gil.

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
	"fmt"
	"sort"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// CACHE-BACKED STEP 1/2 DISCOVERY (Issue #1661 Change 6)
// ========================================
// Authority: DD-WORKFLOW-018 (etcd single source of truth). When r.cache is
// set (SetCache), ListActions/ListWorkflowsByActionType read RemediationWorkflow/
// ActionType CRDs from the Phase 28/29 informer-backed cache instead of issuing
// SQL against action_type_taxonomy/remediation_workflow_catalog. Step 3
// (GetWorkflowWithContextFilters/GetByID) is intentionally excluded -- it keeps
// using r.db unconditionally until a later phase removes WorkflowExecution's
// GetWorkflowByID Content dependency (see discovery.go's Step 3 comment).
// ========================================

// listActionsFromCache is ListActions' cache-backed implementation (Step 1).
// For every Active ActionType, it counts the CRD-cache workflows matching
// filters and includes the action type only if that count is > 0 -- mirrors
// the SQL INNER JOIN's implicit "at least one matching workflow" requirement.
func (r *Repository) listActionsFromCache(ctx context.Context, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.ActionTypeEntry, int, error) {
	actionTypes, err := r.cache.ListActionTypes(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list action types from cache: %w", err)
	}

	entries := make([]models.ActionTypeEntry, 0, len(actionTypes))
	for i := range actionTypes {
		at := &actionTypes[i]
		if at.Status.CatalogStatus != sharedtypes.CatalogStatusActive {
			continue
		}

		workflows, err := r.cache.ListWorkflowsByActionType(ctx, at.Spec.Name)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to list workflows for action type %s: %w", at.Spec.Name, err)
		}

		matched, err := filterAndScoreCachedWorkflows(workflows, filters)
		if err != nil {
			return nil, 0, fmt.Errorf("action type %s: %w", at.Spec.Name, err)
		}
		if len(matched) == 0 {
			continue
		}

		entries = append(entries, crdActionTypeToEntry(at, len(matched)))
	}

	sortActionTypeEntries(entries)
	totalCount := len(entries)
	return paginate(entries, offset, limit), totalCount, nil
}

// listWorkflowsByActionTypeFromCache is ListWorkflowsByActionType's
// cache-backed implementation (Step 2).
func (r *Repository) listWorkflowsByActionTypeFromCache(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.RemediationWorkflow, int, error) {
	workflows, err := r.cache.ListWorkflowsByActionType(ctx, actionType)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list workflows for action type %s: %w", actionType, err)
	}

	matched, err := filterAndScoreCachedWorkflows(workflows, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("action type %s: %w", actionType, err)
	}

	totalCount := len(matched)
	return paginate(matched, offset, limit), totalCount, nil
}

// scoredWorkflow pairs a converted models.RemediationWorkflow with its #220
// final_score, so filterAndScoreCachedWorkflows can sort before discarding
// the score (models.RemediationWorkflow itself carries no final_score field
// -- that value is transient, computed only for a specific query's filters).
type scoredWorkflow struct {
	workflow models.RemediationWorkflow
	score    float64
}

// filterAndScoreCachedWorkflows converts every CRD in workflows to
// models.RemediationWorkflow, keeps only those matching filters' hard-filter
// dimensions (mandatory labels + detected-labels filter), computes each
// match's #220 final_score, and returns the matches sorted by final_score
// DESC with workflow_id ASC as a deterministic tiebreaker -- mirrors
// selectScoredWorkflows' `ORDER BY final_score DESC, workflow_id ASC`.
//
// A converter error (e.g. malformed spec.detectedLabels JSON) aborts the
// whole call rather than silently dropping the offending workflow: an admin
// wrote invalid CRD content, which is exactly the kind of problem an error
// response (surfaced to a caller/alert) should not hide.
func filterAndScoreCachedWorkflows(workflows []rwv1alpha1.RemediationWorkflow, filters *models.WorkflowDiscoveryFilters) ([]models.RemediationWorkflow, error) {
	var dl *models.DetectedLabels
	var customLabels map[string][]string
	if filters != nil {
		dl = filters.DetectedLabels
		customLabels = filters.CustomLabels
	}

	scored := make([]scoredWorkflow, 0, len(workflows))
	for i := range workflows {
		rw := &workflows[i]
		if !matchesMandatoryLabels(crdLabelsToMandatoryLabels(rw.Spec.Labels), filters) {
			continue
		}

		detectedLabels, err := crdDetectedLabelsToModel(rw.Spec.DetectedLabels)
		if err != nil {
			return nil, fmt.Errorf("workflow %s: %w", rw.Name, err)
		}
		if !matchesDetectedLabelsFilter(detectedLabels, dl) {
			continue
		}

		wf, err := crdWorkflowToModel(rw)
		if err != nil {
			return nil, err
		}

		boost := detectedLabelsBoost(detectedLabels, dl)
		custom := customLabelsBoost(crdCustomLabelsToModel(rw.Spec.CustomLabels), customLabels)
		penalty := detectedLabelsPenalty(detectedLabels, dl)
		scored = append(scored, scoredWorkflow{workflow: wf, score: finalScore(boost, custom, penalty)})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].workflow.WorkflowID < scored[j].workflow.WorkflowID
	})

	result := make([]models.RemediationWorkflow, len(scored))
	for i, s := range scored {
		result[i] = s.workflow
	}
	return result, nil
}

// sortActionTypeEntries sorts entries alphabetically by ActionType -- mirrors
// ListActions' `ORDER BY t.action_type`.
func sortActionTypeEntries(entries []models.ActionTypeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ActionType < entries[j].ActionType
	})
}

// paginate returns items[offset:offset+limit], clamped to items' bounds.
// A negative or out-of-range offset/limit never panics -- it returns an
// empty slice instead, matching SQL's OFFSET/LIMIT semantics on an
// out-of-range window.
func paginate[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if limit < 0 || end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
