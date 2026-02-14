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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// DBInterface defines the database operations required by handlers
// This interface allows us to use both MockDB (tests) and real database (production)
// V1.0: All methods use structured types (eliminates map[string]interface{})
type DBInterface interface {
	// Query returns audit events matching the filters (structured type)
	Query(filters map[string]string, limit, offset int) ([]*repository.AuditEvent, error)
	// Get returns a single audit event by ID (structured type)
	Get(id int) (*repository.AuditEvent, error)
	// CountTotal returns the total number of records matching the filters (for pagination metadata)
	// This is used to populate pagination.total with the actual database count, not page size
	CountTotal(filters map[string]string) (int64, error)

	// BR-STORAGE-030: Aggregation endpoints with structured types
	// AggregateSuccessRate calculates success rate for a workflow
	AggregateSuccessRate(workflowID string) (*models.SuccessRateAggregationResponse, error)
	// AggregateByNamespace groups incidents by namespace
	AggregateByNamespace() (*models.NamespaceAggregationResponse, error)
	// AggregateBySeverity groups incidents by severity
	AggregateBySeverity() (*models.SeverityAggregationResponse, error)
	// AggregateIncidentTrend returns incident counts over time
	AggregateIncidentTrend(period string) (*models.TrendAggregationResponse, error)
}

// RemediationHistoryQuerier defines the data access interface for remediation
// history context queries. Used by HandleGetRemediationHistoryContext.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.1: Two-step query pattern (RO events by target, EM events by correlation_id).
type RemediationHistoryQuerier interface {
	QueryROEventsByTarget(ctx context.Context, targetResource string, since time.Time) ([]repository.RawAuditRow, error)
	QueryEffectivenessEventsBatch(ctx context.Context, correlationIDs []string) (map[string][]*EffectivenessEvent, error)
	QueryROEventsBySpecHash(ctx context.Context, specHash string, since, until time.Time) ([]repository.RawAuditRow, error)
}

// Handler handles REST API requests for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// REFACTOR: Enhanced with structured logging, request timing, and observability
// V1.0: Embedding service removed (label-only search per CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md)
type Handler struct {
	db                    DBInterface
	sqlDB                 *sql.DB                           // For reconstruction queries (BR-AUDIT-006)
	logger                logr.Logger
	actionTraceRepository *repository.ActionTraceRepository // ADR-033: Multi-dimensional success tracking
	workflowRepo          *repository.WorkflowRepository    // BR-STORAGE-013: Workflow catalog (label-only search)
	auditStore            audit.AuditStore                  // BR-AUDIT-023: Workflow search audit
	schemaExtractor       *oci.SchemaExtractor              // DD-WORKFLOW-017: OCI-based workflow registration
	remediationHistoryRepo RemediationHistoryQuerier         // BR-HAPI-016: Remediation history context (DD-HAPI-016 v1.1)
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

// WithActionTraceRepository sets the ADR-033 action trace repository
// TDD REFACTOR: Connect handlers to real repository layer
func WithActionTraceRepository(repo *repository.ActionTraceRepository) HandlerOption {
	return func(h *Handler) {
		h.actionTraceRepository = repo
	}
}

// WithWorkflowRepository sets the workflow repository for catalog operations
// BR-STORAGE-013: Workflow catalog semantic search
func WithWorkflowRepository(repo *repository.WorkflowRepository) HandlerOption {
	return func(h *Handler) {
		h.workflowRepo = repo
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

// WithSchemaExtractor sets the OCI schema extractor for workflow registration
// DD-WORKFLOW-017: OCI-based workflow registration (pullspec-only)
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
// REFACTOR: Supports optional logger for production observability
// Accepts DBInterface to work with both MockDB (tests) and real database (production)
func NewHandler(db DBInterface, opts ...HandlerOption) *Handler {
	h := &Handler{
		db:     db,
		logger: logr.Discard(), // Noop logger by default (tests don't need logs)
	}

	// Apply options
	for _, opt := range opts {
		opt(h)
	}

	return h
}

// ListIncidents handles GET /api/v1/incidents
// BR-STORAGE-021: List incidents with filtering
// BR-STORAGE-022: Query filtering
// BR-STORAGE-023: Pagination
// BR-STORAGE-024: RFC 7807 error responses
//
// REFACTOR: Enhanced with request timing, structured logging, and observability
func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	// REFACTOR: Track request timing for performance monitoring
	startTime := time.Now()

	// REFACTOR: Extract request ID for tracing (if present in headers)
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	// REFACTOR: Log incoming request with context
	h.logger.Info("Handling ListIncidents request",
		"request_id", requestID,
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
		"remote_addr", r.RemoteAddr,
	)

	// Parse query parameters
	query := r.URL.Query()

	// BR-STORAGE-023: Parse and validate pagination
	limit, err := parseLimit(query.Get("limit"))
	if err != nil {
		h.logger.Info("Invalid limit parameter",
			"request_id", requestID,
			"limit_value", query.Get("limit"),
			"error", err,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-limit", "Invalid Limit", err.Error(), h.logger)
		return
	}

	offset, err := parseOffset(query.Get("offset"))
	if err != nil {
		h.logger.Info("Invalid offset parameter",
			"request_id", requestID,
			"offset_value", query.Get("offset"),
			"error", err,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-offset", "Invalid Offset", err.Error(), h.logger)
		return
	}

	// BR-STORAGE-022: Parse filters
	filters := make(map[string]string)
	if ns := query.Get("namespace"); ns != "" {
		filters["namespace"] = ns
	}
	if signalName := query.Get("signal_name"); signalName != "" {
		filters["signal_name"] = signalName
	}
	if sev := query.Get("severity"); sev != "" {
		// BR-STORAGE-025: Validate severity values to prevent invalid input
		if !isValidSeverity(sev) {
			h.logger.Info("Invalid severity parameter",
				"request_id", requestID,
				"severity_value", sev,
			)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-severity", "Invalid Severity", fmt.Sprintf("severity must be one of: info, warning, critical, got: %s", sev), h.logger)
			return
		}
		filters["severity"] = sev
	}
	if cluster := query.Get("cluster"); cluster != "" {
		filters["cluster"] = cluster
	}
	if env := query.Get("environment"); env != "" {
		filters["environment"] = env
	}
	if actionType := query.Get("action_type"); actionType != "" {
		filters["action_type"] = actionType
	}

	// Query database
	incidents, err := h.db.Query(filters, limit, offset)
	if err != nil {
		h.logger.Error(err, "Database query failed",
			"request_id", requestID,
			"limit", limit,
			"offset", offset,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error(), h.logger)
		return
	}

	// 🚨 FIX: Get actual total count from database (not len(incidents))
	// This fixes the critical pagination bug where total was page size instead of database count
	// See: docs/services/stateless/data-storage/implementation/DATA-STORAGE-INTEGRATION-TEST-TRIAGE.md
	totalCount, err := h.db.CountTotal(filters)
	if err != nil {
		h.logger.Error(err, "Database count query failed",
			"request_id", requestID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Count Error", err.Error(), h.logger)
		return
	}

	// BR-STORAGE-021: Return response with pagination metadata
	response := map[string]interface{}{
		"data": incidents,
		"pagination": map[string]interface{}{
			"limit":  limit,
			"offset": offset,
			"total":  totalCount, // ✅ Now returns actual database count, not page size
		},
	}

	// REFACTOR: Add request ID to response headers for tracing
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// BR-STORAGE-021: Encode response - handle encoding errors properly
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Note: We've already written the status code, so we can't change the response
		// Log the encoding failure for observability
		h.logger.Error(err, "Failed to encode JSON response",
			"request_id", requestID,
			"result_count", len(incidents),
		)
		return
	}

	// REFACTOR: Log successful response with timing
	duration := time.Since(startTime)
	h.logger.Info("ListIncidents request completed successfully",
		"request_id", requestID,
		"result_count", len(incidents),
		"limit", limit,
		"offset", offset,
		"duration", duration,
		"status_code", http.StatusOK,
	)
}

// GetIncident handles GET /api/v1/incidents/{id}
// BR-STORAGE-021: Get single incident by ID
// BR-STORAGE-024: RFC 7807 error responses
//
// REFACTOR: Enhanced with request timing and structured logging
func (h *Handler) GetIncident(w http.ResponseWriter, r *http.Request) {
	// REFACTOR: Track request timing
	startTime := time.Now()

	// REFACTOR: Extract request ID for tracing
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	// Extract ID from URL path
	// Simple extraction for GREEN phase - will be enhanced with proper router in REFACTOR
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		h.logger.Info("Invalid path - incident ID not provided",
			"request_id", requestID,
			"path", r.URL.Path,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-path", "Invalid Path", "incident ID not provided", h.logger)
		return
	}

	idStr := pathParts[len(pathParts)-1]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.logger.Info("Invalid incident ID format",
			"request_id", requestID,
			"id_string", idStr,
			"error", err,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-id", "Invalid ID", fmt.Sprintf("incident ID must be a number, got: %s", idStr), h.logger)
		return
	}

	h.logger.Info("Handling GetIncident request",
		"request_id", requestID,
		"incident_id", id,
		"path", r.URL.Path,
	)

	// Query database
	incident, err := h.db.Get(id)
	if err != nil {
		h.logger.Error(err, "Database query failed for GetIncident",
			"request_id", requestID,
			"incident_id", id,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error(), h.logger)
		return
	}

	// BR-STORAGE-024: RFC 7807 error for not found
	if incident == nil {
		h.logger.Info("Incident not found",
			"request_id", requestID,
			"incident_id", id,
		)
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Incident Not Found", fmt.Sprintf("incident with ID %d not found", id), h.logger)
		return
	}

	// REFACTOR: Add request ID to response headers
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// BR-STORAGE-021: Encode response - handle encoding errors properly
	if err := json.NewEncoder(w).Encode(incident); err != nil {
		// Note: We've already written the status code, so we can't change the response
		// Log the encoding failure for observability
		h.logger.Error(err, "Failed to encode JSON response",
			"request_id", requestID,
			"incident_id", id,
		)
		return
	}

	// REFACTOR: Log successful response with timing
	duration := time.Since(startTime)
	h.logger.Info("GetIncident request completed successfully",
		"request_id", requestID,
		"incident_id", id,
		"duration", duration,
		"status_code", http.StatusOK,
	)
}

// RFC 7807 error responses are now handled by response.WriteRFC7807Error
// See: pkg/datastorage/server/response/rfc7807.go (canonical implementation)

// parseLimit parses and validates the limit query parameter
// BR-STORAGE-023: Limit must be 1-1000, default is 100
func parseLimit(limitStr string) (int, error) {
	if limitStr == "" {
		return 100, nil // Default limit
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return 0, fmt.Errorf("limit must be a number")
	}

	// BR-STORAGE-023: Validate limit range
	if limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("limit must be between 1 and 1000, got %d", limit)
	}

	return limit, nil
}

// parseOffset parses and validates the offset query parameter
// BR-STORAGE-023: Offset must be >= 0, default is 0
func parseOffset(offsetStr string) (int, error) {
	if offsetStr == "" {
		return 0, nil // Default offset
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return 0, fmt.Errorf("offset must be a number")
	}

	// BR-STORAGE-023: Validate offset is non-negative
	if offset < 0 {
		return 0, fmt.Errorf("offset must be non-negative, got %d", offset)
	}

	return offset, nil
}

// isValidSeverity checks if the severity value is valid
// BR-STORAGE-025: Input validation to prevent invalid data
func isValidSeverity(severity string) bool {
	validSeverities := map[string]bool{
		"info":     true,
		"warning":  true,
		"critical": true,
		"high":     true,
		"medium":   true,
		"low":      true,
	}
	return validSeverities[severity]
}

// ========================================
// AGGREGATION ENDPOINTS (BR-STORAGE-030)
// ========================================

// AggregateSuccessRate handles GET /api/v1/incidents/aggregate/success-rate
// BR-STORAGE-031: Success rate aggregation by workflow
// Returns success/failure counts and success rate for a specific workflow
//
// TDD GREEN Phase: Minimal implementation to pass tests
func (h *Handler) AggregateSuccessRate(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Extract request ID for tracing
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	h.logger.Info("Handling AggregateSuccessRate request",
		"request_id", requestID,
		"query", r.URL.RawQuery,
	)

	// BR-STORAGE-031: Require workflow_id parameter
	workflowID := r.URL.Query().Get("workflow_id")
	if workflowID == "" {
		h.logger.Info("Missing workflow_id parameter",
			"request_id", requestID,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-parameter", "Missing Parameter", "workflow_id parameter is required", h.logger)
		return
	}

	// Query database for aggregation
	result, err := h.db.AggregateSuccessRate(workflowID)
	if err != nil {
		h.logger.Error(err, "Database aggregation failed",
			"request_id", requestID,
			"workflow_id", workflowID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error(), h.logger)
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error(err, "Failed to encode aggregation response",
			"request_id", requestID,
		)
		return
	}

	duration := time.Since(startTime)
	h.logger.Info("AggregateSuccessRate request completed",
		"request_id", requestID,
		"workflow_id", workflowID,
		"duration", duration,
	)
}

// AggregateByNamespace handles GET /api/v1/incidents/aggregate/by-namespace
// BR-STORAGE-032: Namespace grouping aggregation
// Returns incident counts grouped by namespace
//
// TDD GREEN Phase: Minimal implementation to pass tests
func (h *Handler) AggregateByNamespace(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	h.logger.Info("Handling AggregateByNamespace request",
		"request_id", requestID,
	)

	// Query database for namespace aggregation
	result, err := h.db.AggregateByNamespace()
	if err != nil {
		h.logger.Error(err, "Database aggregation failed",
			"request_id", requestID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error(), h.logger)
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error(err, "Failed to encode aggregation response",
			"request_id", requestID,
		)
		return
	}

	duration := time.Since(startTime)
	h.logger.Info("AggregateByNamespace request completed",
		"request_id", requestID,
		"duration", duration,
	)
}

// AggregateBySeverity handles GET /api/v1/incidents/aggregate/by-severity
// BR-STORAGE-033: Severity distribution aggregation
// Returns incident counts grouped by severity level
//
// TDD GREEN Phase: Minimal implementation to pass tests
func (h *Handler) AggregateBySeverity(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	h.logger.Info("Handling AggregateBySeverity request",
		"request_id", requestID,
	)

	// Query database for severity aggregation
	result, err := h.db.AggregateBySeverity()
	if err != nil {
		h.logger.Error(err, "Database aggregation failed",
			"request_id", requestID,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error(), h.logger)
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error(err, "Failed to encode aggregation response",
			"request_id", requestID,
		)
		return
	}

	duration := time.Since(startTime)
	h.logger.Info("AggregateBySeverity request completed",
		"request_id", requestID,
		"duration", duration,
	)
}

// AggregateIncidentTrend handles GET /api/v1/incidents/aggregate/trend
// BR-STORAGE-034: Incident trend aggregation
// Returns incident counts over time for a specified period
//
// TDD GREEN Phase: Minimal implementation to pass tests
func (h *Handler) AggregateIncidentTrend(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	h.logger.Info("Handling AggregateIncidentTrend request",
		"request_id", requestID,
		"query", r.URL.RawQuery,
	)

	// BR-STORAGE-034: Parse period parameter (default to 7d)
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d" // Default to 7 days
	}

	// Validate period format
	if !isValidPeriod(period) {
		h.logger.Info("Invalid period parameter",
			"request_id", requestID,
			"period", period,
		)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-parameter", "Invalid Parameter", "period must be one of: 7d, 30d, 90d", h.logger)
		return
	}

	// Query database for trend aggregation
	result, err := h.db.AggregateIncidentTrend(period)
	if err != nil {
		h.logger.Error(err, "Database aggregation failed",
			"request_id", requestID,
			"period", period,
		)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error(), h.logger)
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error(err, "Failed to encode aggregation response",
			"request_id", requestID,
		)
		return
	}

	duration := time.Since(startTime)
	h.logger.Info("AggregateIncidentTrend request completed",
		"request_id", requestID,
		"period", period,
		"duration", duration,
	)
}

// isValidPeriod checks if the period value is valid
// BR-STORAGE-034: Input validation for trend periods
func isValidPeriod(period string) bool {
	validPeriods := map[string]bool{
		"7d":  true,
		"30d": true,
		"90d": true,
	}
	return validPeriods[period]
}
