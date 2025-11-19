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

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

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
	s.logger.Debug("handleCreateAuditEvent called",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("remote_addr", r.RemoteAddr))

	// Create context with timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 1. Parse request body (JSON payload with all fields)
	s.logger.Debug("Parsing request body...")
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.logger.Warn("Invalid JSON in request body",
			zap.Error(err),
			zap.String("remote_addr", r.RemoteAddr))

		// Record validation failure metric (BR-STORAGE-019)
		if s.metrics != nil && s.metrics.ValidationFailures != nil {
			s.metrics.ValidationFailures.WithLabelValues("body", "invalid_json").Inc()
		}

		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"audit_event",
			map[string]string{"body": "invalid JSON: " + err.Error()},
		))
		return
	}

	// 2. Validate required fields in JSON body
	requiredFields := []string{"version", "service", "event_type", "event_timestamp", "correlation_id", "outcome", "operation", "event_data"}
	for _, field := range requiredFields {
		if _, ok := payload[field]; !ok {
			s.logger.Warn("Missing required field in request body",
				zap.String("field", field))

			// Record validation failure metric (BR-STORAGE-019)
			if s.metrics != nil && s.metrics.ValidationFailures != nil {
				s.metrics.ValidationFailures.WithLabelValues(field, "missing_required_field").Inc()
			}

			writeRFC7807Error(w, validation.NewValidationErrorProblem(
				"audit_event",
				map[string]string{field: "required field missing"},
			))
			return
		}
	}

	// 3. Extract and validate fields from JSON body
	eventType, _ := payload["event_type"].(string)
	service, _ := payload["service"].(string)
	correlationID, _ := payload["correlation_id"].(string)
	outcome, _ := payload["outcome"].(string)
	operation, _ := payload["operation"].(string)
	eventTimestampStr, _ := payload["event_timestamp"].(string)

	// Parse event_timestamp
	eventTimestamp, err := time.Parse(time.RFC3339Nano, eventTimestampStr)
	if err != nil {
		// Try RFC3339 without nanoseconds
		eventTimestamp, err = time.Parse(time.RFC3339, eventTimestampStr)
		if err != nil {
			s.logger.Warn("Invalid event_timestamp format",
				zap.Error(err),
				zap.String("event_timestamp", eventTimestampStr))

			// Record validation failure metric (BR-STORAGE-019)
			if s.metrics != nil && s.metrics.ValidationFailures != nil {
				s.metrics.ValidationFailures.WithLabelValues("event_timestamp", "invalid_time_format").Inc()
			}

			writeRFC7807Error(w, validation.NewValidationErrorProblem(
				"audit_event",
				map[string]string{"event_timestamp": "must be RFC3339 format"},
			))
			return
		}
	}

	// Extract optional fields
	resourceType, _ := payload["resource_type"].(string)
	resourceID, _ := payload["resource_id"].(string)
	resourceNamespace, _ := payload["resource_namespace"].(string)
	clusterID, _ := payload["cluster_id"].(string)
	severity, _ := payload["severity"].(string)
	if severity == "" {
		severity = "info" // Default severity
	}

	// Extract parent_event_id and automatically derive parent_event_date (FK constraint requirement)
	var parentEventID *uuid.UUID
	var parentEventDate *time.Time
	if parentEventIDStr, ok := payload["parent_event_id"].(string); ok && parentEventIDStr != "" {
		parsedParentID, err := uuid.Parse(parentEventIDStr)
		if err != nil {
			s.logger.Warn("Invalid parent_event_id format",
				zap.Error(err),
				zap.String("parent_event_id", parentEventIDStr))

			// Record validation failure metric (BR-STORAGE-019)
			if s.metrics != nil && s.metrics.ValidationFailures != nil {
				s.metrics.ValidationFailures.WithLabelValues("parent_event_id", "invalid_uuid_format").Inc()
			}

			writeRFC7807Error(w, validation.NewValidationErrorProblem(
				"audit_event",
				map[string]string{"parent_event_id": "must be a valid UUID"},
			))
			return
		}
		parentEventID = &parsedParentID

		// Query database to get parent's event_date (required for FK constraint)
		var parentDate time.Time
		err = s.db.QueryRowContext(ctx,
			"SELECT event_date FROM audit_events WHERE event_id = $1",
			parentEventID).Scan(&parentDate)
		if err != nil {
			s.logger.Warn("Parent event not found",
				zap.Error(err),
				zap.String("parent_event_id", parentEventID.String()))

			// Record validation failure metric (BR-STORAGE-019)
			if s.metrics != nil && s.metrics.ValidationFailures != nil {
				s.metrics.ValidationFailures.WithLabelValues("parent_event_id", "parent_not_found").Inc()
			}

			writeRFC7807Error(w, validation.NewValidationErrorProblem(
				"audit_event",
				map[string]string{"parent_event_id": "parent event does not exist"},
			))
			return
		}
		parentEventDate = &parentDate
	}

	// Extract event_data (nested JSONB)
	eventData, ok := payload["event_data"].(map[string]interface{})
	if !ok {
		s.logger.Warn("event_data must be a JSON object")
		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"audit_event",
			map[string]string{"event_data": "must be a JSON object"},
		))
		return
	}

	s.logger.Debug("Request body parsed and validated successfully",
		zap.String("event_type", eventType),
		zap.String("service", service),
		zap.String("correlation_id", correlationID))

	// 4. Build AuditEvent domain model
	auditEvent := &repository.AuditEvent{
		EventTimestamp:    eventTimestamp,
		EventType:         eventType,
		Service:           service,
		CorrelationID:     correlationID,
		ParentEventID:     parentEventID,   // Optional: for event causality chains
		ParentEventDate:   parentEventDate, // Auto-derived from parent_event_id
		ResourceType:      resourceType,
		ResourceID:        resourceID,
		ResourceNamespace: resourceNamespace,
		ClusterID:         clusterID,
		Operation:         operation,
		Outcome:           outcome,
		Severity:          severity,
		EventData:         eventData,
	}

	// 5. Persist to database via repository
	s.logger.Debug("Writing audit event to database...")

	// Record write duration metric (BR-STORAGE-019)
	start := time.Now()
	created, err := s.auditEventsRepo.Create(ctx, auditEvent)
	duration := time.Since(start).Seconds()

	// Emit write_duration metric for observability
	if s.metrics != nil && s.metrics.WriteDuration != nil {
		s.metrics.WriteDuration.WithLabelValues("audit_events").Observe(duration)
	}

	if err != nil {
		s.logger.Error("Database write failed",
			zap.Error(err),
			zap.String("event_type", eventType),
			zap.String("correlation_id", correlationID),
			zap.Float64("duration_seconds", duration))

		// Return 500 Internal Server Error with RFC 7807
		writeRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.io/problems/database-error",
			Title:    "Database Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to write audit event to database",
			Instance: r.URL.Path,
		})
		return
	}

	// 6. Record metrics (BR-STORAGE-019: Logging and metrics)
	// Record successful audit write
	if s.metrics != nil && s.metrics.AuditTracesTotal != nil {
		s.metrics.AuditTracesTotal.WithLabelValues(service, "success").Inc()
	}

	// Record audit lag (time between event occurrence and write)
	if s.metrics != nil && s.metrics.AuditLagSeconds != nil {
		lag := time.Since(eventTimestamp).Seconds()
		s.metrics.AuditLagSeconds.WithLabelValues(service).Observe(lag)
	}

	// 7. Success - return 201 Created with event_id and created_at
	s.logger.Info("Audit event created successfully",
		zap.String("event_id", created.EventID.String()),
		zap.String("event_type", created.EventType),
		zap.String("service", created.Service),
		zap.String("correlation_id", created.CorrelationID),
		zap.Float64("duration_seconds", duration))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"event_id":   created.EventID.String(),
		"created_at": created.CreatedAt.Format(time.RFC3339),
		"message":    "Audit event created successfully",
	})
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
	s.logger.Debug("handleQueryAuditEvents called",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("query", r.URL.RawQuery),
		zap.String("remote_addr", r.RemoteAddr))

	// Create context with timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1. Parse and validate query parameters
	filters, err := s.parseQueryFilters(r)
	if err != nil {
		s.logger.Warn("Invalid query parameters",
			zap.Error(err),
			zap.String("query", r.URL.RawQuery))
		writeRFC7807Error(w, validation.NewValidationErrorProblem("query parameters", map[string]string{
			"query": err.Error(),
		}))
		return
	}

	// 2. Build SQL query using AuditEventsQueryBuilder
	builder := s.buildQueryFromFilters(filters)
	querySQL, args, err := builder.Build()
	if err != nil {
		s.logger.Warn("Failed to build query",
			zap.Error(err))
		writeRFC7807Error(w, validation.NewValidationErrorProblem("query parameters", map[string]string{
			"pagination": err.Error(),
		}))
		return
	}

	// Build count query
	countSQL, _, err := builder.BuildCount()
	if err != nil {
		s.logger.Warn("Failed to build count query",
			zap.Error(err))
		writeRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.io/errors/internal-error",
			Title:    "Internal Server Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to build count query",
			Instance: r.URL.Path,
		})
		return
	}

	// 3. Execute query via repository
	events, pagination, err := s.auditEventsRepo.Query(ctx, querySQL, countSQL, args)
	if err != nil {
		s.logger.Error("Failed to query audit events",
			zap.Error(err))
		writeRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.io/errors/database-error",
			Title:    "Database Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to query audit events from database",
			Instance: r.URL.Path,
		})
		return
	}

	// 4. Success - return 200 OK with data and pagination metadata
	s.logger.Info("Audit events queried successfully",
		zap.Int("count", len(events)),
		zap.Int("total", pagination.Total),
		zap.Int("limit", pagination.Limit),
		zap.Int("offset", pagination.Offset))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":       events,
		"pagination": pagination,
	})
}

// parseQueryFilters extracts and validates query parameters from HTTP request
func (s *Server) parseQueryFilters(r *http.Request) (*queryFilters, error) {
	query := r.URL.Query()

	filters := &queryFilters{
		correlationID: query.Get("correlation_id"),
		eventType:     query.Get("event_type"),
		service:       query.Get("service"),
		outcome:       query.Get("outcome"),
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
type queryFilters struct {
	correlationID string
	eventType     string
	service       string
	outcome       string
	severity      string
	since         *time.Time
	until         *time.Time
	limit         int
	offset        int
}
