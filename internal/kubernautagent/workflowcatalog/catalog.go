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
	"errors"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ErrNotFound is returned by GetByID/GetWorkflowWithContextFilters when no
// workflow matches. Ported from
// pkg/datastorage/repository/workflow/crud.go (Issue #1674): callers should
// use errors.Is(err, ErrNotFound).
//
// DD-WORKFLOW-016: GetWorkflowWithContextFilters (discovery.go) also returns
// this sentinel for its security gate -- it deliberately does not distinguish
// "workflow doesn't exist" from "workflow exists but doesn't match context
// filters" to avoid leaking which case occurred to the caller.
var ErrNotFound = errors.New("workflow not found")

// Catalog serves KubernautAgent's workflow/action-type discovery protocol
// (DD-WORKFLOW-019, Issue #1677) directly from the informer-backed Cache --
// no DataStorage round-trip. Ported from
// pkg/datastorage/repository/workflow.Repository (Issue #1661 Change 6),
// dropping the dead *sqlx.DB field/constructor param: Repository.db had zero
// production readers even in DataStorage post-Phase-C, since GetByID/List/
// ListActions/ListWorkflowsByActionType/GetWorkflowWithContextFilters were
// already unconditionally cache-backed there.
type Catalog struct {
	cache  *Cache
	logger logr.Logger
}

// NewCatalog constructs a Catalog backed by cache. Callers should only pass
// a cache that has completed its initial sync (NewInformerCache blocks until
// then).
func NewCatalog(cache *Cache, logger logr.Logger) *Catalog {
	return &Catalog{cache: cache, logger: logger}
}

// GetByID retrieves a workflow by UUID (primary key) from the Cache.
// DD-WORKFLOW-002 v3.0: workflow_id is the sole UUID primary key. Returns
// ErrNotFound (wrapped with the queried ID) if no workflow exists.
func (c *Catalog) GetByID(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error) {
	return c.getByIDFromCache(ctx, workflowID)
}

// List retrieves workflows with optional filtering and pagination from the
// Cache. BR-STORAGE-012: Workflow catalog listing.
func (c *Catalog) List(ctx context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error) {
	return c.listFromCache(ctx, filters, limit, offset)
}
