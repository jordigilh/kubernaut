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
	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"

	"github.com/jordigilh/kubernaut/pkg/datastorage/workflowcache"
)

// ========================================
// WORKFLOW REPOSITORY
// ========================================
// Authority: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// Design Decision: DD-WORKFLOW-002 (MCP Workflow Catalog Architecture)
//
// V1.1 REFACTORING: Split from monolithic workflow_repository.go (1,173 lines)
// into focused modules for better maintainability
// ========================================

// Repository handles workflow catalog operations
// V1.0: Label-only search architecture (no embeddings)
//
// Issue #1661 Phase C: GetByID/List/ListActions/ListWorkflowsByActionType/
// GetWorkflowWithContextFilters are now unconditionally cache-backed (see
// crud.go/discovery.go) -- db is retained on the struct (and NewRepository's
// signature is unchanged) to avoid an unrelated, wide-reaching constructor
// signature change across every call site; it currently has no reader in
// this package outside _test.go files.
type Repository struct {
	db     *sqlx.DB
	logger logr.Logger

	// cache is the Issue #1661 Phase 28/29 informer-backed RemediationWorkflow/
	// ActionType CRD view (DD-WORKFLOW-018), unconditionally non-nil in
	// production (validateServerDeps requires ServerDeps.K8sRestConfig,
	// Phase 55) -- List, ListActions, ListWorkflowsByActionType, GetByID, and
	// GetWorkflowWithContextFilters all read from it exclusively (Phase C
	// deleted their Postgres SQL fallback). Tests that need a Repository
	// must call SetCache before exercising these methods.
	cache *workflowcache.Cache
}

// NewRepository creates a new workflow repository
// V1.0: Label-only search (embedding client removed)
func NewRepository(db *sqlx.DB, logger logr.Logger) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

// SetCache wires the Issue #1661 Phase 28/29 informer-backed CRD cache into
// the repository (DD-WORKFLOW-018) -- callers should only pass a non-nil
// cache once it has completed its initial sync (workflowcache.NewInformerCache
// blocks until then).
func (r *Repository) SetCache(cache *workflowcache.Cache) {
	r.cache = cache
}
