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

		best := matched[0]
		entry := crdActionTypeToEntry(at, len(matched))
		entry.BestMatchScore = best.FinalScore
		entry.BestWorkflowID = best.Workflow.WorkflowID
		entry.MatchedDetectedLabels = best.matchedDetectedLabels
		entry.PreferenceReason = actionTypePreferenceReason(best.matchedDetectedLabels)
		entries = append(entries, entry)
	}

	sortActionTypeEntries(entries)
	totalCount := len(entries)
	return paginate(entries, offset, limit), totalCount, nil
}

// listWorkflowsByActionTypeFromCache is ListWorkflowsByActionType's
// cache-backed implementation (Step 2).
func (c *Catalog) listWorkflowsByActionTypeFromCache(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.RemediationWorkflow, int, error) {
	candidates, totalCount, err := c.listScoredWorkflowsByActionTypeFromCache(ctx, actionType, filters, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	workflows := make([]models.RemediationWorkflow, 0, len(candidates))
	for i := range candidates {
		workflows = append(workflows, candidates[i].Workflow)
	}
	return workflows, totalCount, nil
}

func (c *Catalog) listScoredWorkflowsByActionTypeFromCache(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]ScoredWorkflow, int, error) {
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

// ScoredWorkflow carries the cache-computed ranking score alongside its
// workflow. The score is query-specific and is retained for audit evidence;
// callers must not add it to the LLM-facing WorkflowDiscoveryEntry.
type ScoredWorkflow struct {
	Workflow              models.RemediationWorkflow
	FinalScore            float64
	matchedDetectedLabels *models.DetectedLabels
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
func filterAndScoreCachedWorkflows(workflows []rwv1alpha1.RemediationWorkflow, filters *models.WorkflowDiscoveryFilters) ([]ScoredWorkflow, error) {
	var dl *models.DetectedLabels
	var customLabels map[string][]string
	if filters != nil {
		dl = filters.DetectedLabels
		customLabels = filters.CustomLabels
	}

	scored := make([]ScoredWorkflow, 0, len(workflows))
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
		scored = append(scored, ScoredWorkflow{
			Workflow:              wf,
			FinalScore:            finalScore(boost, custom, penalty),
			matchedDetectedLabels: matchedDetectedLabelsEvidence(detectedLabels, dl),
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].FinalScore != scored[j].FinalScore {
			return scored[i].FinalScore > scored[j].FinalScore
		}
		return scored[i].Workflow.WorkflowID < scored[j].Workflow.WorkflowID
	})
	return scored, nil
}

// sortActionTypeEntries sorts Step 1 entries by the best matching workflow's
// score, descending, with action type ascending as a deterministic tiebreaker.
// Rank and Preferred are assigned after the complete result set is ordered and
// before pagination, so a later page retains its global position.
func sortActionTypeEntries(entries []models.ActionTypeEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].BestMatchScore != entries[j].BestMatchScore {
			return entries[i].BestMatchScore > entries[j].BestMatchScore
		}
		return entries[i].ActionType < entries[j].ActionType
	})

	for i := range entries {
		entries[i].Rank = i + 1
		entries[i].Preferred = i == 0
	}
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
