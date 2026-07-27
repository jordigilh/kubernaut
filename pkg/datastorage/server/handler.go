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

// Handler handles REST API requests for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// REFACTOR: Enhanced with structured logging, request timing, and observability
// V1.0: Embedding service removed (label-only search per CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md)
//
// #1677 Phase 2g (DD-WORKFLOW-019): workflowRepo/workflowCache/successMetricsRepo
// (and their WithWorkflowRepository/WithWorkflowCache/WithSuccessMetricsRepository
// options, and the SuccessMetricsQuerier interface) were removed -- workflow/
// action-type discovery is now owned directly by KubernautAgent.
type Handler struct {
	sqlDB                  *sql.DB // For reconstruction queries (BR-AUDIT-006)
	logger                 logr.Logger
	auditStore             audit.AuditStore           // BR-AUDIT-023: Workflow search audit
	schemaExtractor        *oci.SchemaExtractor       // DD-WORKFLOW-017: OCI image schema extraction; not currently invoked by any handler (Issue #1642 removed its last caller, ValidateBundleExists)
	remediationHistoryRepo RemediationHistoryQuerier // BR-HAPI-016: Remediation history context (DD-HAPI-016 v1.1)
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
