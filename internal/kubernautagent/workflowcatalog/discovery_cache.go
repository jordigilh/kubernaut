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

package workflowcatalog

import (
	"context"
	"fmt"
	"sort"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// CACHE-BACKED STEP 1/2/3 DISCOVERY (Issue #1677 Phase 2b)
// ========================================
// Authority: DD-WORKFLOW-019 (KA owns discovery directly). Ported from
// pkg/datastorage/repository/workflow/discovery_cache.go (Issue #1661
// Change 6): ListActions/ListWorkflowsByActionType/GetByID/
// GetWorkflowWithContextFilters read RemediationWorkflow/ActionType CRDs
// from the Phase 2a informer-backed Cache instead of issuing SQL.
// ========================================

// listActionsFromCache is ListActions' cache-backed implementation (Step 1).
// For every Active ActionType, it counts the CRD-cache workflows matching
// filters and includes the action type only if that count is > 0 -- mirrors
// the SQL INNER JOIN's implicit "at least one matching workflow" requirement.
func (c *Catalog) listActionsFromCache(ctx context.Context, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.ActionTypeEntry, int, error) {
	actionTypes, err := c.cache.ListActionTypes(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list action types from cache: %w", err)
	}

	entries := make([]models.ActionTypeEntry, 0, len(actionTypes))
	for i := range actionTypes {
		at := &actionTypes[i]
		if at.Status.CatalogStatus != sharedtypes.CatalogStatusActive {
			continue
		}

		workflows, err := c.cache.ListWorkflowsByActionType(ctx, at.Spec.Name)
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
func (c *Catalog) listWorkflowsByActionTypeFromCache(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.RemediationWorkflow, int, error) {
	workflows, err := c.cache.ListWorkflowsByActionType(ctx, actionType)
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

// getByIDFromCache is GetByID's cache-backed implementation: an unfiltered
// lookup by the content-hash workflow_id, with no security-gate check (matches
// GetByID's contract -- see discovery.go's Step 3 comment).
func (c *Catalog) getByIDFromCache(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error) {
	rw, err := c.cache.GetWorkflowByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow by ID from cache: %w", err)
	}
	if rw == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, workflowID)
	}
	wf, err := crdWorkflowToModel(rw)
	if err != nil {
		return nil, err
	}
	return &wf, nil
}

// getWorkflowWithContextFiltersFromCache is GetWorkflowWithContextFilters'
// cache-backed implementation (Step 3): looks the workflow up by workflow_id,
// then applies the same mandatory-label/detected-label security gate Step 1/2
// use (matchesMandatoryLabels, matchesDetectedLabelsFilter) -- no scoring, since
// this is a single-workflow lookup, not a ranked list. Returns (nil, nil) both
// when the workflow doesn't exist and when it exists but fails the gate,
// mirroring the SQL path's intentional non-disclosure (DD-WORKFLOW-016:
// prevent info leakage about a workflow's existence to an unauthorized context).
func (c *Catalog) getWorkflowWithContextFiltersFromCache(ctx context.Context, workflowID string, filters *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	rw, err := c.cache.GetWorkflowByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow by ID from cache: %w", err)
	}
	if rw == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, workflowID)
	}

	if !matchesMandatoryLabels(crdLabelsToMandatoryLabels(rw.Spec.Labels), filters) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, workflowID)
	}

	var dl *models.DetectedLabels
	if filters != nil {
		dl = filters.DetectedLabels
	}
	detectedLabels, err := crdDetectedLabelsToModel(rw.Spec.DetectedLabels)
	if err != nil {
		return nil, fmt.Errorf("workflow %s: %w", rw.Name, err)
	}
	if !matchesDetectedLabelsFilter(detectedLabels, dl) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, workflowID)
	}

	wf, err := crdWorkflowToModel(rw)
	if err != nil {
		return nil, err
	}
	return &wf, nil
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
