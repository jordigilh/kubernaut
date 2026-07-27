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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// CACHE-BACKED LIST (Issue #1677 Phase 2b)
// ========================================
// Authority: DD-WORKFLOW-019 (KA owns discovery directly). List is the
// generic, unfiltered-by-default catalog listing -- distinct from the
// discovery protocol's Steps 1/2/3 (discovery_cache.go). Ported from
// pkg/datastorage/repository/workflow/list_cache.go (Issue #1661 Change 6).
// ========================================

// listFromCache is List's cache-backed implementation: converts every
// cached RemediationWorkflow CRD, keeps those matching matchesSearchFilters,
// sorts by created_at DESC / workflow_id ASC (mirrors List's `ORDER BY
// created_at DESC, workflow_id ASC`), and paginates.
func (c *Catalog) listFromCache(ctx context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error) {
	workflows, err := c.cache.ListWorkflows(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list workflows from cache: %w", err)
	}

	matched := make([]models.RemediationWorkflow, 0, len(workflows))
	for i := range workflows {
		rw := &workflows[i]
		if !matchesSearchFilters(rw, filters) {
			continue
		}
		wf, err := crdWorkflowToModel(rw)
		if err != nil {
			return nil, 0, err
		}
		matched = append(matched, wf)
	}

	sortWorkflowsByCreatedAtDesc(matched)
	totalCount := len(matched)
	return paginate(matched, offset, limit), totalCount, nil
}

// sortWorkflowsByCreatedAtDesc sorts workflows by CreatedAt descending, with
// WorkflowID ascending as a deterministic tiebreaker -- mirrors List's SQL
// `ORDER BY created_at DESC, workflow_id ASC`.
func sortWorkflowsByCreatedAtDesc(workflows []models.RemediationWorkflow) {
	sort.SliceStable(workflows, func(i, j int) bool {
		if !workflows[i].CreatedAt.Equal(workflows[j].CreatedAt) {
			return workflows[i].CreatedAt.After(workflows[j].CreatedAt)
		}
		return workflows[i].WorkflowID < workflows[j].WorkflowID
	})
}
