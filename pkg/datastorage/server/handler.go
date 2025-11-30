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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/embedding"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// DBInterface defines the database operations required by handlers
// This interface allows us to use both MockDB (tests) and real database (production)
type DBInterface interface {
	Query(filters map[string]string, limit, offset int) ([]map[string]interface{}, error)
	Get(id int) (map[string]interface{}, error)
	// CountTotal returns the total number of records matching the filters (for pagination metadata)
	// This is used to populate pagination.total with the actual database count, not page size
	CountTotal(filters map[string]string) (int64, error)

	// BR-STORAGE-030: Aggregation endpoints
	// AggregateSuccessRate calculates success rate for a workflow
	AggregateSuccessRate(workflowID string) (map[string]interface{}, error)
	// AggregateByNamespace groups incidents by namespace
	AggregateByNamespace() (map[string]interface{}, error)
	// AggregateBySeverity groups incidents by severity
	AggregateBySeverity() (map[string]interface{}, error)
	// AggregateIncidentTrend returns incident counts over time
	AggregateIncidentTrend(period string) (map[string]interface{}, error)
}

// Handler handles REST API requests for Data Storage Service
// BR-STORAGE-021: REST API read endpoints
// BR-STORAGE-024: RFC 7807 error responses
//
// REFACTOR: Enhanced with structured logging, request timing, and observability
type Handler struct {
	db                    DBInterface
	logger                logr.Logger
	actionTraceRepository *repository.ActionTraceRepository // ADR-033: Multi-dimensional success tracking
	workflowRepo          *repository.WorkflowRepository    // BR-STORAGE-013: Workflow catalog
	embeddingService      embedding.Service                 // BR-STORAGE-013: Embedding generation
}

// HandlerOption is a functional option for configuring the Handler
type HandlerOption func(*Handler)

// WithLogger sets a custom logger for the handler
// REFACTOR: Production deployments should provide a real logger
func WithLogger(logger logr.Logger) HandlerOption {
	return func(h *Handler) {
		if logger != nil {
			h.logger = logger
		}
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

// WithEmbeddingService sets the embedding service for semantic search
// BR-STORAGE-013: Embedding generation for semantic search
func WithEmbeddingService(service embedding.Service) HandlerOption {
	return func(h *Handler) {
		h.embeddingService = service
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
		h.writeRFC7807Error(w, http.StatusBadRequest, "invalid-limit", "Invalid Limit", err.Error())
		return
	}

	offset, err := parseOffset(query.Get("offset"))
	if err != nil {
		h.logger.Info("Invalid offset parameter",
			"request_id", requestID,
			"offset_value", query.Get("offset"),
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusBadRequest, "invalid-offset", "Invalid Offset", err.Error())
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
			h.writeRFC7807Error(w, http.StatusBadRequest, "invalid-severity", "Invalid Severity", fmt.Sprintf("severity must be one of: info, warning, critical, got: %s", sev))
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
		h.logger.Error("Database query failed",
			"request_id", requestID,
			"error", err,
			"limit", limit,
			"offset", offset,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error())
		return
	}

	// 🚨 FIX: Get actual total count from database (not len(incidents))
	// This fixes the critical pagination bug where total was page size instead of database count
	// See: docs/services/stateless/data-storage/implementation/DATA-STORAGE-INTEGRATION-TEST-TRIAGE.md
	totalCount, err := h.db.CountTotal(filters)
	if err != nil {
		h.logger.Error("Database count query failed",
			"request_id", requestID,
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Count Error", err.Error())
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
		h.logger.Error("Failed to encode JSON response",
			"request_id", requestID,
			"error", err,
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
		h.writeRFC7807Error(w, http.StatusBadRequest, "invalid-path", "Invalid Path", "incident ID not provided")
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
		h.writeRFC7807Error(w, http.StatusBadRequest, "invalid-id", "Invalid ID", fmt.Sprintf("incident ID must be a number, got: %s", idStr))
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
		h.logger.Error("Database query failed for GetIncident",
			"request_id", requestID,
			"incident_id", id,
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error())
		return
	}

	// BR-STORAGE-024: RFC 7807 error for not found
	if incident == nil {
		h.logger.Info("Incident not found",
			"request_id", requestID,
			"incident_id", id,
		)
		h.writeRFC7807Error(w, http.StatusNotFound, "not-found", "Incident Not Found", fmt.Sprintf("incident with ID %d not found", id))
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
		h.logger.Error("Failed to encode JSON response",
			"request_id", requestID,
			"incident_id", id,
			"error", err,
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

// writeRFC7807Error writes an RFC 7807 Problem Details error response
// BR-STORAGE-024: RFC 7807 error responses
func (h *Handler) writeRFC7807Error(w http.ResponseWriter, status int, errorType, title, detail string) {
	problemDetail := map[string]interface{}{
		"type":   fmt.Sprintf("https://api.kubernaut.io/problems/%s", errorType),
		"title":  title,
		"status": status,
		"detail": detail,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	// BR-STORAGE-024: Encode RFC 7807 error response - handle encoding errors properly
	if err := json.NewEncoder(w).Encode(problemDetail); err != nil {
		// Note: We've already written the status code, so we can't change the response
		// Log the encoding failure for observability
		h.logger.Error("Failed to encode RFC 7807 error response",
			"status", status,
			"error_type", errorType,
			"error", err,
		)
		// Still return - client gets the status code at least
	}
}

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
		h.writeRFC7807Error(w, http.StatusBadRequest, "missing-parameter", "Missing Parameter", "workflow_id parameter is required")
		return
	}

	// Query database for aggregation
	result, err := h.db.AggregateSuccessRate(workflowID)
	if err != nil {
		h.logger.Error("Database aggregation failed",
			"request_id", requestID,
			"workflow_id", workflowID,
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error())
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error("Failed to encode aggregation response",
			"request_id", requestID,
			"error", err,
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
		h.logger.Error("Database aggregation failed",
			"request_id", requestID,
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error())
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error("Failed to encode aggregation response",
			"request_id", requestID,
			"error", err,
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
		h.logger.Error("Database aggregation failed",
			"request_id", requestID,
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error())
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error("Failed to encode aggregation response",
			"request_id", requestID,
			"error", err,
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
		h.writeRFC7807Error(w, http.StatusBadRequest, "invalid-parameter", "Invalid Parameter", "period must be one of: 7d, 30d, 90d")
		return
	}

	// Query database for trend aggregation
	result, err := h.db.AggregateIncidentTrend(period)
	if err != nil {
		h.logger.Error("Database aggregation failed",
			"request_id", requestID,
			"period", period,
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error", err.Error())
		return
	}

	// Return aggregated results
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.logger.Error("Failed to encode aggregation response",
			"request_id", requestID,
			"error", err,
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
