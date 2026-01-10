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
	"encoding/json"
	"fmt"
	"net/http"
	"time"


	dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// RESPONSE TYPES (Structured Data)
// ========================================

// AuditEventCreatedResponse represents the response when an audit event is successfully created
type AuditEventCreatedResponse struct {
	EventID        string `json:"event_id"`
	EventTimestamp string `json:"event_timestamp"`
	Message        string `json:"message"`
}

// AuditEventAcceptedResponse represents the response when an audit event is queued for async processing (DLQ)
type AuditEventAcceptedResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// AuditEventsQueryResponse represents the response for audit events query API
type AuditEventsQueryResponse struct {
	Data       []*repository.AuditEvent       `json:"data"`
	Pagination *repository.PaginationMetadata `json:"pagination"`
}

// ========================================
// AUDIT EVENTS WRITE HANDLER (TDD GREEN Phase)
// 📋 Tests Define Contract: test/integration/datastorage/audit_events_write_api_test.go
// Authority: DAY21_PHASE1_IMPLEMENTATION_PLAN.md Phase 3
// ========================================
//
// This file implements HTTP WRITE API handler for unified audit_events table.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (audit_events_write_api_test.go - 8 scenarios)
// - Handler implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-STORAGE-033: Generic audit write API
// - BR-STORAGE-032: Unified audit trail
//
// OpenAPI Compliance:
// - Endpoint: POST /api/v1/audit/events
// - Request Body: JSON with required fields (version, service, event_type, event_timestamp, correlation_id, outcome, operation, event_data)
// - Response: 201 Created with event_id (UUID) and created_at
// - Errors: 400 Bad Request, 500 Internal Server Error (RFC 7807)
//
// ========================================

// handleCreateAuditEvent handles POST /api/v1/audit/events
// BR-STORAGE-033: Generic audit write API for unified audit table
//
// Request Body: JSON with required fields (version, service, event_type, event_timestamp, correlation_id, outcome, operation, event_data)
// Success Response: 201 Created with event_id (UUID) and created_at
// Error Responses: 400 Bad Request, 500 Internal Server Error (RFC 7807)
func (s *Server) handleCreateAuditEvent(w http.ResponseWriter, r *http.Request) {
	s.logger.V(1).Info("handleCreateAuditEvent called",
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr)

	// Create context with timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 1. Parse request body using OpenAPI type (type-safe, no manual parsing)
	s.logger.V(1).Info("Parsing request body with OpenAPI types...")
	var req dsclient.AuditEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Info("Invalid JSON in request body", "error", err, "remote_addr", r.RemoteAddr)

	// Record validation failure metric (BR-STORAGE-019)
	// Metrics are guaranteed non-nil by constructor
	s.metrics.ValidationFailures.WithLabelValues("body", "invalid_json").Inc()

		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_request", "Invalid Request", err.Error(), s.logger)
		return
	}

	// 2. Validate business rules (OpenAPI already validated required fields, types, and enums)
	// Gap 1.2 REFACTOR: Enhanced validation (timestamp bounds, field lengths)
	s.logger.V(1).Info("Validating business rules...")
	if err := helpers.ValidateAuditEventRequest(&req); err != nil {
		s.logger.Info("Business validation failed", "error", err)

	// Record validation failure metric (BR-STORAGE-019)
	// Metrics are guaranteed non-nil by constructor
	s.metrics.ValidationFailures.WithLabelValues("business_rules", "validation_failed").Inc()

		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error", err.Error(), s.logger)
		return
	}

	// 3. Convert OpenAPI request to internal audit event
	s.logger.V(1).Info("Converting OpenAPI request to internal type...")
	auditEvent, err := helpers.ConvertAuditEventRequest(req)
	if err != nil {
		s.logger.Error(err, "Failed to convert audit event request")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "conversion_error", "Conversion Error", err.Error(), s.logger)
		return
	}

	// 4. Handle parent_event_id FK constraint (query parent's event_date) - OGEN-MIGRATION: OptNilUUID
	if req.ParentEventID.IsSet() {
		parentEventID := req.ParentEventID.Value
		var parentDate time.Time
		err = s.db.QueryRowContext(ctx,
			"SELECT event_date FROM audit_events WHERE event_id = $1",
			&parentEventID).Scan(&parentDate)
		if err != nil {
			s.logger.Info("Parent event not found", "error", err, "parent_event_id", parentEventID.String())

		// Record validation failure metric (BR-STORAGE-019)
		// Metrics are guaranteed non-nil by constructor
		s.metrics.ValidationFailures.WithLabelValues("parent_event_id", "parent_not_found").Inc()

			response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error", "parent event does not exist", s.logger)
			return
		}
		auditEvent.ParentEventID = &parentEventID
	}

	// 5. Convert to repository type
	s.logger.V(1).Info("Converting to repository type...")
	repositoryEvent, err := helpers.ConvertToRepositoryAuditEvent(auditEvent)
	if err != nil {
		// Conversion errors are client-side validation errors (e.g., invalid event_data JSON)
		// Return 400 Bad Request, not 500 Internal Server Error
		s.logger.Info("Invalid event_data format", "error", err)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_event_data", "Invalid Event Data", err.Error(), s.logger)
		return
	}

	s.logger.V(1).Info("Request parsed and validated successfully",
		"event_type", req.EventType,
		"event_category", string(req.EventCategory),
		"correlation_id", req.CorrelationID)

	// 6. Persist to database via repository
	s.logger.V(1).Info("Writing audit event to database...")

	// Record write duration metric (BR-STORAGE-019)
	start := time.Now()
	created, err := s.auditEventsRepo.Create(ctx, repositoryEvent)
	duration := time.Since(start).Seconds()

	// Emit write_duration metric for observability
	// Metrics are guaranteed non-nil by constructor
	s.metrics.WriteDuration.WithLabelValues("audit_events").Observe(duration)

	if err != nil {
		s.logger.Error(err, "Database write failed",
			"event_type", req.EventType,
			"correlation_id", req.CorrelationID,
			"duration_seconds", duration)

		// DD-009: DLQ fallback on database errors
		s.logger.Info("Attempting DLQ fallback for audit event",
			"event_type", req.EventType,
			"correlation_id", req.CorrelationID,
			"db_error", err.Error())

		// Create a FRESH context for DLQ write (not tied to original request timeout)
		// DD-009: DLQ fallback must succeed even if DB operation timed out
		dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer dlqCancel()

		// Use the internal audit event we already created for DLQ
		// Note: auditEvent already has EventData as []byte from ConvertAuditEventRequest
		dlqAuditEvent := auditEvent

		// Attempt to enqueue to DLQ
		if dlqErr := s.dlqClient.EnqueueAuditEvent(dlqCtx, dlqAuditEvent, err); dlqErr != nil {
			s.logger.Error(dlqErr, "DLQ fallback also failed - data loss risk",
				"event_type", req.EventType,
				"correlation_id", req.CorrelationID,
				"original_error", err.Error())

			// Both database and DLQ failed - return 500
			response.WriteRFC7807ErrorWithRequestID(w, http.StatusInternalServerError, "database_error", "Database Error", "Failed to write audit event to database and DLQ", r.URL.Path, s.logger)
			return
		}

		s.logger.Info("DLQ fallback succeeded",
			"event_type", req.EventType,
			"correlation_id", req.CorrelationID)

	// Record DLQ fallback metric
	// Metrics are guaranteed non-nil by constructor
	s.metrics.AuditTracesTotal.WithLabelValues(string(req.EventCategory), "dlq_fallback").Inc()

		// DLQ success - return 202 Accepted (async processing)
		acceptedResp := AuditEventAcceptedResponse{
			Status:  "accepted",
			Message: "audit event queued for async processing",
		}
		response.WriteJSON(w, http.StatusAccepted, acceptedResp, s.logger)
		return
	}

	// 7. Record metrics (BR-STORAGE-019: Logging and metrics)
	// Record successful audit write
	// Metrics are guaranteed non-nil by constructor
	s.metrics.AuditTracesTotal.WithLabelValues(string(req.EventCategory), "success").Inc()

	// Record audit lag (time between event occurrence and write)
	lag := time.Since(req.EventTimestamp).Seconds()
	s.metrics.AuditLagSeconds.WithLabelValues(string(req.EventCategory)).Observe(lag)

	// 7. Success - return 201 Created with event_id and created_at
	s.logger.Info("Audit event created successfully",
		"event_id", created.EventID.String(),
		"event_type", created.EventType,
		"event_category", created.EventCategory,
		"correlation_id", created.CorrelationID,
		"duration_seconds", duration)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	response := AuditEventCreatedResponse{
		EventID:        created.EventID.String(),
		EventTimestamp: created.EventTimestamp.Format(time.RFC3339), // ADR-034
		Message:        "Audit event created successfully",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "failed to encode success response")
	}
}

// ========================================
// AUDIT EVENTS QUERY HANDLER (TDD GREEN Phase)
// 📋 Tests Define Contract: test/integration/datastorage/audit_events_query_api_test.go
// Authority: DD-STORAGE-010 Query API Pagination Strategy
// ========================================
//
// This file implements HTTP QUERY API handler for unified audit_events table.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (audit_events_query_api_test.go - 10 scenarios)
// - Handler implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-STORAGE-021: REST API Read Endpoints
// - BR-STORAGE-022: Query Filtering
// - BR-STORAGE-023: Pagination Validation
//
// DD-STORAGE-010 Compliance:
// - V1.0: Offset-based pagination (limit/offset)
// - Query Parameters: correlation_id, event_type, service, outcome, severity, since, until, limit, offset
// - Response: JSON with data array and pagination metadata
// - Errors: 400 Bad Request (RFC 7807)
//
// ========================================

// handleQueryAuditEvents handles GET /api/v1/audit/events
// BR-STORAGE-021: REST API Read Endpoints
// BR-STORAGE-022: Query Filtering
// BR-STORAGE-023: Pagination Validation
// DD-STORAGE-010: Offset-based pagination
func (s *Server) handleQueryAuditEvents(w http.ResponseWriter, r *http.Request) {
	s.logger.V(1).Info("handleQueryAuditEvents called",
		"method", r.Method,
		"path", r.URL.Path,
		"query", r.URL.RawQuery,
		"remote_addr", r.RemoteAddr)

	// Create context with timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1. Parse and validate query parameters
	filters, err := s.parseQueryFilters(r)
	if err != nil {
		s.logger.Info("Invalid query parameters",
			"error", err,
			"query", r.URL.RawQuery)
		writeValidationRFC7807Error(w, validation.NewValidationErrorProblem("query parameters", map[string]string{
			"query": err.Error(),
		}), s)
		return
	}

	// 2. Build SQL query using AuditEventsQueryBuilder
	builder := s.buildQueryFromFilters(filters)
	querySQL, args, err := builder.Build()
	if err != nil {
		s.logger.Info("Failed to build query",
			"error", err)
		writeValidationRFC7807Error(w, validation.NewValidationErrorProblem("query parameters", map[string]string{
			"pagination": err.Error(),
		}), s)
		return
	}

	// Build count query
	countSQL, _, err := builder.BuildCount()
	if err != nil {
		s.logger.Info("Failed to build count query",
			"error", err)
		writeValidationRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.ai/problems/internal-error",
			Title:    "Internal Server Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to build count query",
			Instance: r.URL.Path,
		}, s)
		return
	}

	// 3. Execute query via repository
	events, pagination, err := s.auditEventsRepo.Query(ctx, querySQL, countSQL, args)
	if err != nil {
		s.logger.Error(err, "Failed to query audit events")
		writeValidationRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.ai/problems/database-error",
			Title:    "Database Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to query audit events from database",
			Instance: r.URL.Path,
		}, s)
		return
	}

	// 4. Success - return 200 OK with data and pagination metadata
	s.logger.Info("Audit events queried successfully",
		"count", len(events),
		"total", pagination.Total,
		"limit", pagination.Limit,
		"offset", pagination.Offset)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := AuditEventsQueryResponse{
		Data:       events,
		Pagination: pagination,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "failed to encode query response")
	}
}

// parseQueryFilters extracts and validates query parameters from HTTP request
func (s *Server) parseQueryFilters(r *http.Request) (*queryFilters, error) {
	query := r.URL.Query()

	filters := &queryFilters{
		correlationID: query.Get("correlation_id"),
		eventType:     query.Get("event_type"),
		service:       query.Get("event_category"), // ADR-034: Use event_category parameter
		outcome:       query.Get("event_outcome"),  // ADR-034: Use event_outcome parameter
		severity:      query.Get("severity"),
		limit:         100, // Default limit
		offset:        0,   // Default offset
	}

	// Parse time parameters
	if sinceParam := query.Get("since"); sinceParam != "" {
		since, err := s.parseTimeParam(sinceParam)
		if err != nil {
			return nil, err
		}
		filters.since = &since
	}

	if untilParam := query.Get("until"); untilParam != "" {
		until, err := s.parseTimeParam(untilParam)
		if err != nil {
			return nil, err
		}
		filters.until = &until
	}

	// Parse pagination parameters
	if limitParam := query.Get("limit"); limitParam != "" {
		var limit int
		if _, err := fmt.Sscanf(limitParam, "%d", &limit); err != nil {
			return nil, fmt.Errorf("invalid limit parameter: must be an integer")
		}
		filters.limit = limit
	}

	if offsetParam := query.Get("offset"); offsetParam != "" {
		var offset int
		if _, err := fmt.Sscanf(offsetParam, "%d", &offset); err != nil {
			return nil, fmt.Errorf("invalid offset parameter: must be an integer")
		}
		filters.offset = offset
	}

	return filters, nil
}

// parseTimeParam parses time parameters (relative or absolute)
// DD-STORAGE-010: Time parsing for query API
func (s *Server) parseTimeParam(param string) (time.Time, error) {
	// Import the time parser from query package
	return query.ParseTimeParam(param)
}

// buildQueryFromFilters creates an AuditEventsQueryBuilder from parsed filters
func (s *Server) buildQueryFromFilters(filters *queryFilters) *query.AuditEventsQueryBuilder {
	builder := query.NewAuditEventsQueryBuilder(query.WithAuditEventsLogger(s.logger))

	if filters.correlationID != "" {
		builder = builder.WithCorrelationID(filters.correlationID)
	}
	if filters.eventType != "" {
		builder = builder.WithEventType(filters.eventType)
	}
	if filters.service != "" {
		builder = builder.WithService(filters.service)
	}
	if filters.outcome != "" {
		builder = builder.WithOutcome(filters.outcome)
	}
	if filters.severity != "" {
		builder = builder.WithSeverity(filters.severity)
	}
	if filters.since != nil {
		builder = builder.WithSince(*filters.since)
	}
	if filters.until != nil {
		builder = builder.WithUntil(*filters.until)
	}

	builder = builder.WithLimit(filters.limit).WithOffset(filters.offset)

	return builder
}

// queryFilters holds parsed query parameters
// Note: 'service' and 'outcome' kept for API backward compatibility,
// but map to ADR-034 event_category and event_outcome in database
type queryFilters struct {
	correlationID string
	eventType     string
	service       string // Maps to event_category (ADR-034)
	outcome       string // Maps to event_outcome (ADR-034)
	severity      string
	since         *time.Time
	until         *time.Time
	limit         int
	offset        int
}

// ========================================
// NOTE: Meta-auditing removed per DD-AUDIT-002 V2.0.1
// ========================================
//
// The following self-audit events were removed as redundant:
// - datastorage.audit.written (event in DB IS proof of success)
// - datastorage.audit.failed (DLQ already captures failures)
// - datastorage.dlq.fallback (DLQ record IS proof of fallback)
//
// Operational visibility maintained through:
// - ✅ Prometheus metrics (audit_writes_total)
// - ✅ Structured logs (all operations logged)
// - ✅ DLQ records (failed writes captured)
//
// See: docs/handoff/DS_AUDIT_ARCHITECTURE_CHANGES_NOTIFICATION.md
// ========================================
