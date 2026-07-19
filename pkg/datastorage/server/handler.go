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
	"context"
	"database/sql"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/workflowcache"
)

// RemediationHistoryQuerier defines the data access interface for remediation
// history context queries. Used by HandleGetRemediationHistoryContext.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.4: Both tiers query by spec hash for causal chain integrity (#586).
type RemediationHistoryQuerier interface {
	QueryROEventsBySpecHash(ctx context.Context, specHash string, since, until time.Time) ([]repository.RawAuditRow, error)
	QueryEffectivenessEventsBatch(ctx context.Context, correlationIDs []string) (map[string][]*EffectivenessEvent, error)
}

// ActionTypeValidator validates action types against the taxonomy before DB insertion.
// DD-WORKFLOW-016: Explicit validation for clean 400 errors instead of FK constraint 500.
//
// #1661 Phase B: this interface's sole caller (HandleCreateWorkflow's
// validateActionType) was deleted alongside the RW mutation handlers.
// Deliberately left wired (dead-but-compiling) rather than removed here --
// mirrors Phase A3's precedent of deferring the actiontype repo/DI cleanup to
// its own gated step, now Phase C ("prune workflow/crud.go dead methods").
type ActionTypeValidator interface {
	ActionTypeExists(ctx context.Context, actionType string) (bool, error)
}

// SuccessMetricsQuerier computes on-demand workflow success-rate aggregates
// from audit_events. Issue #1661 Change 7 (DD-WORKFLOW-018): replaces the
// stored remediation_workflow_catalog total_executions/successful_executions/
// actual_success_rate columns (previously written by the now-deleted
// Repository.UpdateSuccessMetrics) with a query-time aggregation, since
// execution outcomes live only in the audit trail, not a mutable catalog row.
// *repository.AuditEventsRepository satisfies this interface.
type SuccessMetricsQuerier interface {
	GetSuccessMetrics(ctx context.Context, workflowIDs []string) (map[string]repository.WorkflowSuccessMetrics, error)
}

// Handler handles REST API requests for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// REFACTOR: Enhanced with structured logging, request timing, and observability
// V1.0: Embedding service removed (label-only search per CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md)
type Handler struct {
	sqlDB                  *sql.DB // For reconstruction queries (BR-AUDIT-006)
	logger                 logr.Logger
	workflowRepo           *repository.WorkflowRepository // BR-STORAGE-013: Workflow catalog (label-only search)
	actionTypeValidator    ActionTypeValidator             // GAP-4: DD-WORKFLOW-016 taxonomy validation; dead-but-compiling post-Phase-B, see ActionTypeValidator doc comment
	auditStore             audit.AuditStore                // BR-AUDIT-023: Workflow search audit
	schemaExtractor        *oci.SchemaExtractor            // DD-WORKFLOW-017: OCI image schema extraction; not currently invoked by any handler (Issue #1642 removed its last caller, ValidateBundleExists)
	remediationHistoryRepo RemediationHistoryQuerier       // BR-HAPI-016: Remediation history context (DD-HAPI-016 v1.1)
	workflowCache          *workflowcache.Cache            // Issue #1661 Phase 29: informer-backed RW/ActionType CRD view (DD-WORKFLOW-018); nil until Change 6 (Phase 31-33) rewires discovery to consume it
	successMetricsRepo     SuccessMetricsQuerier            // Issue #1661 Phase 35: on-demand audit_events success-rate aggregation (DD-WORKFLOW-018); nil is valid (metrics degrade to zero-value, logged) so tests without an audit DB keep working
}

// HandlerOption is a functional option for configuring the Handler
type HandlerOption func(*Handler)

// WithLogger sets a custom logger for the handler
// REFACTOR: Production deployments should provide a real logger
func WithLogger(logger logr.Logger) HandlerOption {
	return func(h *Handler) {
		h.logger = logger
	}
}

// WithWorkflowRepository sets the workflow repository for catalog operations
// BR-STORAGE-013: Workflow catalog semantic search
func WithWorkflowRepository(repo *repository.WorkflowRepository) HandlerOption {
	return func(h *Handler) {
		h.workflowRepo = repo
	}
}

// WithActionTypeValidator sets the action type taxonomy validator.
// DD-WORKFLOW-016 GAP-4: Validates action_type against taxonomy before DB insert
// for clean 400 errors instead of FK constraint 500.
func WithActionTypeValidator(v ActionTypeValidator) HandlerOption {
	return func(h *Handler) {
		h.actionTypeValidator = v
	}
}

// WithSQLDB sets the SQL database connection for reconstruction queries
// BR-AUDIT-006: RemediationRequest reconstruction from audit trail
func WithSQLDB(db *sql.DB) HandlerOption {
	return func(h *Handler) {
		h.sqlDB = db
	}
}

// WithAuditStore sets the audit store for workflow search audit events
// BR-AUDIT-023: Workflow search audit event generation
func WithAuditStore(store audit.AuditStore) HandlerOption {
	return func(h *Handler) {
		h.auditStore = store
	}
}

// WithSchemaExtractor sets the OCI schema extractor.
// DD-WORKFLOW-017: retained for OCI-based schema extraction; not currently
// invoked by any handler (Issue #1642 removed its last caller,
// ValidateBundleExists — the execution.bundle pre-flight existence check).
func WithSchemaExtractor(extractor *oci.SchemaExtractor) HandlerOption {
	return func(h *Handler) {
		h.schemaExtractor = extractor
	}
}

// WithRemediationHistoryQuerier sets the remediation history repository.
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
func WithRemediationHistoryQuerier(repo RemediationHistoryQuerier) HandlerOption {
	return func(h *Handler) {
		h.remediationHistoryRepo = repo
	}
}

// WithWorkflowCache sets the informer-backed RemediationWorkflow/ActionType
// CRD cache. Issue #1661 Phase 29 / DD-WORKFLOW-018. nil is valid (no
// K8sRestConfig supplied) and matches the pre-Phase-29 behavior.
func WithWorkflowCache(cache *workflowcache.Cache) HandlerOption {
	return func(h *Handler) {
		h.workflowCache = cache
	}
}

// WithSuccessMetricsRepository sets the on-demand success-metrics aggregator.
// Issue #1661 Phase 35 / DD-WORKFLOW-018. nil is valid (metrics degrade to
// zero-value TotalExecutions/nil ActualSuccessRate, logged) for tests and
// deployments without audit_events wired.
func WithSuccessMetricsRepository(repo SuccessMetricsQuerier) HandlerOption {
	return func(h *Handler) {
		h.successMetricsRepo = repo
	}
}

// NewHandler creates a new REST API handler
func NewHandler(opts ...HandlerOption) *Handler {
	h := &Handler{
		logger: logr.Discard(),
	}

	// Apply options
	for _, opt := range opts {
		opt(h)
	}

	return h
}
