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

// workflowCatalogColumns is the explicit column list for remediation_workflow_catalog,
// derived from RemediationWorkflow struct db: tags. #1088 Phase 6.1: replaces SELECT *
// to protect against schema drift and avoid fetching deprecated columns (e.g., embedding).
//
// Issue #1661 Change 7 (DD-WORKFLOW-018): actual_success_rate/total_executions/
// successful_executions are deliberately excluded -- migration 015 dropped
// their backing columns. They are computed on demand from audit_events by
// Handler.overlaySuccessMetrics (pkg/datastorage/server/workflow_success_metrics.go),
// not scanned from this table.
const workflowCatalogColumns = "workflow_id, workflow_name, version, schema_version, " +
	"name, description, owner, maintainer, " +
	"content, content_hash, " +
	"action_type, " +
	"parameters, execution_engine, " +
	"schema_image, schema_digest, " +
	"execution_bundle, execution_bundle_digest, " +
	"engine_config, service_account_name, " +
	"labels, custom_labels, detected_labels, " +
	"status, status_reason, " +
	"disabled_at, disabled_by, disabled_reason, " +
	"is_latest_version, previous_version, deprecation_notice, " +
	"version_notes, change_summary, approved_by, approved_at, " +
	"expected_success_rate, expected_duration_seconds, " +
	"created_at, updated_at, created_by, updated_by"

// Repository handles workflow catalog operations
// V1.0: Label-only search architecture (no embeddings)
type Repository struct {
	db     *sqlx.DB
	logger logr.Logger

	// cache is the Issue #1661 Phase 28/29 informer-backed RemediationWorkflow/
	// ActionType CRD view (DD-WORKFLOW-018). When set (via SetCache), ListActions
	// and ListWorkflowsByActionType (Step 1/2 of the discovery protocol) read from
	// it instead of issuing SQL against remediation_workflow_catalog/
	// action_type_taxonomy. nil means "no cache wired" -- callers (tests,
	// server_construction.go without K8sRestConfig) keep the existing SQL path
	// unconditionally. GetWorkflowWithContextFilters/GetByID (Step 3) never
	// consult the cache -- deferred per Phase 31 scope decision.
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
// the repository, switching ListActions/ListWorkflowsByActionType from SQL
// to in-memory reads (DD-WORKFLOW-018). A nil cache is a no-op (keeps the
// SQL path) -- callers should only pass a non-nil cache once it has
// completed its initial sync (workflowcache.NewInformerCache blocks until
// then).
func (r *Repository) SetCache(cache *workflowcache.Cache) {
	r.cache = cache
}
