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
	"errors"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// READ OPERATIONS
// ========================================
// Issue #1661 Phase C: Create/SupersedeAndCreate/UpdateStatus and the
// name+version read variants (GetByNameAndVersion/GetActiveByNameAndVersion/
// GetLatestDisabledByNameAndVersion/GetActiveByWorkflowName/GetLatestVersion/
// GetVersionsByName) were deleted from this file -- all had zero production
// callers post-Phase-B (AuthWebhook owns the RemediationWorkflow CRD
// lifecycle entirely locally, DD-WORKFLOW-018) and their Postgres
// composite-PK (workflow_name, version) semantics are obsolete now that
// metadata.name is the workflow's sole identity. GetByID/List's `if r.cache
// != nil` SQL-fallback guards were also removed here (Repository.cache is
// unconditionally non-nil in production -- validateServerDeps requires
// ServerDeps.K8sRestConfig, Phase 55 -- so the fallback branch was already
// dead in production, kept alive only by tests that constructed a
// Repository without a cache; those tests were migrated to the cache-backed
// suites in this package, see discovery_cache_test.go/list_cache_test.go).

// ErrNotFound is returned by GetByID/GetWorkflowWithContextFilters when no
// workflow matches. Issue #1674: replaces the ambiguous (nil, nil) return
// that forced callers to distinguish "not found" from "real error" by
// nil-checking the result instead of checking err. Callers should use
// errors.Is(err, ErrNotFound).
//
// DD-WORKFLOW-016: GetWorkflowWithContextFilters (discovery.go) also returns
// this sentinel for its security gate -- it deliberately does not distinguish
// "workflow doesn't exist" from "workflow exists but doesn't match context
// filters" to avoid leaking which case occurred to the caller.
var ErrNotFound = errors.New("workflow not found")

// GetByID retrieves a workflow by UUID (primary key) from the Issue #1661
// Phase 28/29 informer-backed CRD cache (DD-WORKFLOW-018).
// DD-WORKFLOW-002 v3.0: workflow_id is the sole UUID primary key.
// Returns ErrNotFound (wrapped with the queried ID) if no workflow exists.
func (r *Repository) GetByID(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error) {
	return r.getByIDFromCache(ctx, workflowID)
}

// List retrieves workflows with optional filtering and pagination from the
// Issue #1661 Phase 28/29 informer-backed CRD cache (DD-WORKFLOW-018).
// BR-STORAGE-012: Workflow catalog listing.
func (r *Repository) List(ctx context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error) {
	return r.listFromCache(ctx, filters, limit, offset)
}
