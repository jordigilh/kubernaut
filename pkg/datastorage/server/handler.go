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
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	actiontyperepo "github.com/jordigilh/kubernaut/pkg/datastorage/repository/actiontype"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// WorkflowLifecycleRepository defines the data access interface for workflow
// lifecycle operations (enable, disable, deprecate). Used for testability.
// *repository.WorkflowRepository satisfies this interface.
//
// GAP-WF-1: DD-WORKFLOW-017 Phase 4.4 - PATCH /enable and PATCH /deprecate
type WorkflowLifecycleRepository interface {
	GetByID(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error)
	UpdateStatus(ctx context.Context, workflowID, version, status, reason, updatedBy string) error
}

// RemediationHistoryQuerier defines the data access interface for remediation
// history context queries. Used by HandleGetRemediationHistoryContext.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.4: Both tiers query by spec hash for causal chain integrity (#586).
type RemediationHistoryQuerier interface {
	QueryROEventsBySpecHash(ctx context.Context, specHash string, since, until time.Time) ([]repository.RawAuditRow, error)
	QueryEffectivenessEventsBatch(ctx context.Context, correlationIDs []string) (map[string][]*EffectivenessEvent, error)
}

// WorkflowContentIntegrityRepository defines the data access operations needed
// for content integrity checking during workflow registration. When a workflow
// with the same name+version already exists, these methods determine the correct
// action: idempotent return, supersede, or re-enable.
// BR-WORKFLOW-006: Content hash verification prevents spec tampering.
type WorkflowContentIntegrityRepository interface {
	Create(ctx context.Context, workflow *models.RemediationWorkflow) error
	GetActiveByNameAndVersion(ctx context.Context, workflowName, version string) (*models.RemediationWorkflow, error)
	GetActiveByWorkflowName(ctx context.Context, workflowName string) (*models.RemediationWorkflow, error)
	GetLatestDisabledByNameAndVersion(ctx context.Context, workflowName, version string) (*models.RemediationWorkflow, error)
	UpdateStatus(ctx context.Context, workflowID, version, status, reason, updatedBy string) error
	SupersedeAndCreate(ctx context.Context, oldID, oldVersion, reason string, newWorkflow *models.RemediationWorkflow) error
}

// ActionTypeValidator validates action types against the taxonomy before DB insertion.
// DD-WORKFLOW-016: Explicit validation for clean 400 errors instead of FK constraint 500.
type ActionTypeValidator interface {
	ActionTypeExists(ctx context.Context, actionType string) (bool, error)
}

// Handler handles REST API requests for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// REFACTOR: Enhanced with structured logging, request timing, and observability
// V1.0: Embedding service removed (label-only search per CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md)
type Handler struct {
	sqlDB                   *sql.DB                           // For reconstruction queries (BR-AUDIT-006)
	logger                  logr.Logger
	workflowRepo            *repository.WorkflowRepository    // BR-STORAGE-013: Workflow catalog (label-only search)
	workflowLifecycleRepo   WorkflowLifecycleRepository       // GAP-WF-1: Lifecycle ops (enable/disable/deprecate) - uses workflowRepo when nil
	workflowIntegrityRepo   WorkflowContentIntegrityRepository // BR-WORKFLOW-006: Content hash integrity checking
	actionTypeValidator     ActionTypeValidator                // GAP-4: DD-WORKFLOW-016 taxonomy validation
	auditStore              audit.AuditStore                  // BR-AUDIT-023: Workflow search audit
	schemaExtractor         *oci.SchemaExtractor              // DD-WE-006: OCI bundle validation (ValidateBundleExists)
	dependencyValidator     validation.DependencyValidator     // DD-WE-006: Schema-declared dependency validation
	executionNamespace      string                            // DD-WE-006: Namespace where dependencies are provisioned
	remediationHistoryRepo  RemediationHistoryQuerier         // BR-HAPI-016: Remediation history context (DD-HAPI-016 v1.1)
	actionTypeRepo          *actiontyperepo.Repository        // BR-WORKFLOW-007: ActionType CRD lifecycle
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

// WithWorkflowLifecycleRepository sets the workflow lifecycle repository for enable/disable/deprecate.
// When nil, lifecycle handlers use workflowRepo. Used for unit test mocking (GAP-WF-1).
func WithWorkflowLifecycleRepository(repo WorkflowLifecycleRepository) HandlerOption {
	return func(h *Handler) {
		h.workflowLifecycleRepo = repo
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

// WithSchemaExtractor sets the OCI schema extractor for bundle validation
// DD-WE-006: ValidateBundleExists uses OCI puller to verify execution bundles
func WithSchemaExtractor(extractor *oci.SchemaExtractor) HandlerOption {
	return func(h *Handler) {
		h.schemaExtractor = extractor
	}
}

// WithDependencyValidator sets the dependency validator for registration-time checks.
// DD-WE-006: Validates that declared Secrets/ConfigMaps exist with non-empty data
// in the execution namespace before storing the workflow in the catalog.
func WithDependencyValidator(v validation.DependencyValidator, executionNamespace string) HandlerOption {
	return func(h *Handler) {
		h.dependencyValidator = v
		h.executionNamespace = executionNamespace
	}
}

// WithWorkflowContentIntegrityRepository sets the content integrity repository
// for ContentHash-based duplicate detection during workflow registration.
// BR-WORKFLOW-006: Prevents spec tampering for same name+version workflows.
func WithWorkflowContentIntegrityRepository(repo WorkflowContentIntegrityRepository) HandlerOption {
	return func(h *Handler) {
		h.workflowIntegrityRepo = repo
	}
}

// WithRemediationHistoryQuerier sets the remediation history repository.
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
func WithRemediationHistoryQuerier(repo RemediationHistoryQuerier) HandlerOption {
	return func(h *Handler) {
		h.remediationHistoryRepo = repo
	}
}

// WithActionTypeRepository sets the action type taxonomy repository.
// BR-WORKFLOW-007: ActionType CRD lifecycle management.
func WithActionTypeRepository(repo *actiontyperepo.Repository) HandlerOption {
	return func(h *Handler) {
		h.actionTypeRepo = repo
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

