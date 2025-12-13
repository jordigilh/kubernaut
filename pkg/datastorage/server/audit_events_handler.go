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

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
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

	// 1. Parse request body (JSON payload with all fields)
	s.logger.V(1).Info("Parsing request body...")
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		s.logger.Info("Invalid JSON in request body",
			"error", err,
			"remote_addr", r.RemoteAddr)

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
	// ADR-034: Accept both old and new field names for backward compatibility
	requiredFields := []string{"version", "event_type", "event_timestamp", "correlation_id", "event_data"}
	requiredFieldsWithAliases := map[string][]string{
		"event_category": {"event_category", "service"}, // ADR-034 + legacy
		"event_action":   {"event_action", "operation"}, // ADR-034 + legacy
		"event_outcome":  {"event_outcome", "outcome"},  // ADR-034 + legacy
	}

	// Check simple required fields
	for _, field := range requiredFields {
		if _, ok := payload[field]; !ok {
			s.logger.Info("Missing required field in request body",
				"field", field)

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

	// Check fields with aliases (ADR-034 + backward compatibility)
	for canonical, aliases := range requiredFieldsWithAliases {
		found := false
		for _, alias := range aliases {
			if _, ok := payload[alias]; ok {
				found = true
				break
			}
		}
		if !found {
			s.logger.Info("Missing required field in request body",
				"field", canonical,
				"accepted_aliases", aliases)

			// Record validation failure metric (BR-STORAGE-019)
			if s.metrics != nil && s.metrics.ValidationFailures != nil {
				s.metrics.ValidationFailures.WithLabelValues(canonical, "missing_required_field").Inc()
			}

			writeRFC7807Error(w, validation.NewValidationErrorProblem(
				"audit_event",
				map[string]string{canonical: fmt.Sprintf("required field missing (accepted: %v)", aliases)},
			))
			return
		}
	}

	// 3. Extract and validate fields from JSON body (ADR-034 + backward compatibility)
	eventType, _ := payload["event_type"].(string)

	// event_category (ADR-034) or service (legacy)
	eventCategory, ok := payload["event_category"].(string)
	if !ok {
		eventCategory, _ = payload["service"].(string)
	}

	// event_action (ADR-034) or operation (legacy)
	eventAction, ok := payload["event_action"].(string)
	if !ok {
		eventAction, _ = payload["operation"].(string)
	}

	// event_outcome (ADR-034) or outcome (legacy)
	eventOutcome, ok := payload["event_outcome"].(string)
	if !ok {
		eventOutcome, _ = payload["outcome"].(string)
	}

	// Validate event_outcome enum (Gap 1.2: Malformed Event Rejection)
	// Valid values: success, failure, pending
	validOutcomes := map[string]bool{
		"success": true,
		"failure": true,
		"pending": true,
	}
	if !validOutcomes[eventOutcome] {
		s.logger.Info("Invalid event_outcome value",
			"event_outcome", eventOutcome,
			"valid_values", []string{"success", "failure", "pending"})

		// Record validation failure metric
		if s.metrics != nil && s.metrics.ValidationFailures != nil {
			s.metrics.ValidationFailures.WithLabelValues("event_outcome", "invalid_enum_value").Inc()
		}

		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"audit_event",
			map[string]string{"event_outcome": fmt.Sprintf("must be one of: success, failure, pending (got: %s)", eventOutcome)},
		))
		return
	}

	correlationID, _ := payload["correlation_id"].(string)
	eventTimestampStr, _ := payload["event_timestamp"].(string)

	// Parse event_timestamp
	eventTimestamp, err := time.Parse(time.RFC3339Nano, eventTimestampStr)
	if err != nil {
		// Try RFC3339 without nanoseconds
		eventTimestamp, err = time.Parse(time.RFC3339, eventTimestampStr)
		if err != nil {
			s.logger.Info("Invalid event_timestamp format",
				"error", err,
				"event_timestamp", eventTimestampStr)

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

	// Extract actor fields (ADR-034 REQUIRED fields)
	actorType, ok := payload["actor_type"].(string)
	if !ok || actorType == "" {
		// Default: derive from event_category
		actorType = "service" // Default to "service" for backward compatibility
	}

	actorID, ok := payload["actor_id"].(string)
	if !ok || actorID == "" {
		// Default: derive from event_category
		actorID = eventCategory + "-service" // e.g., "gateway-service"
	}

	// Extract resource fields (ADR-034 REQUIRED fields)
	resourceType, ok := payload["resource_type"].(string)
	if !ok || resourceType == "" {
		// Default: use event_category as resource_type for backward compatibility
		resourceType = eventCategory
	}

	resourceID, ok := payload["resource_id"].(string)
	if !ok || resourceID == "" {
		// Default: use correlation_id as resource_id for backward compatibility
		resourceID = correlationID
	}

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
			s.logger.Info("Invalid parent_event_id format",
				"error", err,
				"parent_event_id", parentEventIDStr)

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
			s.logger.Info("Parent event not found",
				"error", err,
				"parent_event_id", parentEventID.String())

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
		s.logger.Info("event_data must be a JSON object")
		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"audit_event",
			map[string]string{"event_data": "must be a JSON object"},
		))
		return
	}

	s.logger.V(1).Info("Request body parsed and validated successfully",
		"event_type", eventType,
		"event_category", eventCategory,
		"correlation_id", correlationID)

	// 4. Build AuditEvent domain model (ADR-034 schema)
	auditEvent := &repository.AuditEvent{
		EventTimestamp:    eventTimestamp,
		EventType:         eventType,
		EventCategory:     eventCategory, // ADR-034
		EventAction:       eventAction,   // ADR-034
		EventOutcome:      eventOutcome,  // ADR-034
		CorrelationID:     correlationID,
		ParentEventID:     parentEventID,   // Optional: for event causality chains
		ParentEventDate:   parentEventDate, // Auto-derived from parent_event_id
		ActorType:         actorType,       // ADR-034 REQUIRED
		ActorID:           actorID,         // ADR-034 REQUIRED
		ResourceType:      resourceType,    // ADR-034 REQUIRED
		ResourceID:        resourceID,      // ADR-034 REQUIRED
		ResourceNamespace: resourceNamespace,
		ClusterID:         clusterID,
		Severity:          severity,
		EventData:         eventData,
	}

	// 5. Persist to database via repository
	s.logger.V(1).Info("Writing audit event to database...")

	// Record write duration metric (BR-STORAGE-019)
	start := time.Now()
	created, err := s.auditEventsRepo.Create(ctx, auditEvent)
	duration := time.Since(start).Seconds()

	// Emit write_duration metric for observability
	if s.metrics != nil && s.metrics.WriteDuration != nil {
		s.metrics.WriteDuration.WithLabelValues("audit_events").Observe(duration)
	}

	if err != nil {
		s.logger.Error(err, "Database write failed",
			"event_type", eventType,
			"correlation_id", correlationID,
			"duration_seconds", duration)

		// BR-STORAGE-012: Audit Point 2 - Write failure (before DLQ fallback)
		// Note: No event_id yet since DB write failed
		s.auditWriteFailure(ctx, "", eventType, correlationID, err)

		// DD-009: DLQ fallback on database errors
		s.logger.Info("Attempting DLQ fallback for audit event",
			"event_type", eventType,
			"correlation_id", correlationID,
			"db_error", err.Error())

		// Create a FRESH context for DLQ write (not tied to original request timeout)
		// DD-009: DLQ fallback must succeed even if DB operation timed out
		dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer dlqCancel()

		// Convert repository.AuditEvent to audit.AuditEvent for DLQ
		// Serialize event_data to JSON
		eventDataJSON, marshalErr := json.Marshal(eventData)
		if marshalErr != nil {
			s.logger.Error(marshalErr, "Failed to marshal event_data for DLQ",
				"event_type", eventType)
			eventDataJSON = []byte("{}")
		}

		dlqAuditEvent := &audit.AuditEvent{
			EventID:        uuid.New(),
			EventVersion:   "1.0",
			EventTimestamp: eventTimestamp,
			EventType:      eventType,
			EventCategory:  eventCategory, // ADR-034
			EventAction:    eventAction,   // ADR-034
			EventOutcome:   eventOutcome,  // ADR-034
			ActorType:      "service",
			ActorID:        eventCategory, // Use event_category as actor_id
			ResourceType:   resourceType,
			ResourceID:     resourceID,
			CorrelationID:  correlationID,
			ParentEventID:  parentEventID,
			Namespace:      &resourceNamespace,
			ClusterName:    &clusterID,
			EventData:      eventDataJSON,
			Severity:       &severity,
		}

		// Attempt to enqueue to DLQ (use original database error, not marshalErr)
		if dlqErr := s.dlqClient.EnqueueAuditEvent(dlqCtx, dlqAuditEvent, err); dlqErr != nil {
			s.logger.Error(dlqErr, "DLQ fallback also failed - data loss risk",
				"event_type", eventType,
				"correlation_id", correlationID,
				"original_error", err.Error())

			// Both database and DLQ failed - return 500
			writeRFC7807Error(w, &validation.RFC7807Problem{
				Type:     "https://kubernaut.io/problems/database-error",
				Title:    "Database Error",
				Status:   http.StatusInternalServerError,
				Detail:   "Failed to write audit event to database and DLQ",
				Instance: r.URL.Path,
			})
			return
		}

		s.logger.Info("DLQ fallback succeeded",
			"event_type", eventType,
			"correlation_id", correlationID)

		// BR-STORAGE-012: Audit Point 3 - DLQ fallback success
		s.auditDLQFallback(ctx, dlqAuditEvent.EventID.String(), eventType, correlationID, eventCategory)

		// Record DLQ fallback metric
		if s.metrics != nil && s.metrics.AuditTracesTotal != nil {
			s.metrics.AuditTracesTotal.WithLabelValues(eventCategory, "dlq_fallback").Inc()
		}

		// DLQ success - return 202 Accepted (async processing)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		response := AuditEventAcceptedResponse{
			Status:  "accepted",
			Message: "audit event queued for async processing",
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.logger.Error(err, "failed to encode DLQ response")
		}
		return
	}

	// 6. Record metrics (BR-STORAGE-019: Logging and metrics)
	// BR-STORAGE-012: Audit Point 1 - Write success
	s.auditWriteSuccess(ctx, created.EventID.String(), eventType, correlationID, eventCategory)

	// Record successful audit write
	if s.metrics != nil && s.metrics.AuditTracesTotal != nil {
		s.metrics.AuditTracesTotal.WithLabelValues(eventCategory, "success").Inc()
	}

	// Record audit lag (time between event occurrence and write)
	if s.metrics != nil && s.metrics.AuditLagSeconds != nil {
		lag := time.Since(eventTimestamp).Seconds()
		s.metrics.AuditLagSeconds.WithLabelValues(eventCategory).Observe(lag)
	}

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
		writeRFC7807Error(w, validation.NewValidationErrorProblem("query parameters", map[string]string{
			"query": err.Error(),
		}))
		return
	}

	// 2. Build SQL query using AuditEventsQueryBuilder
	builder := s.buildQueryFromFilters(filters)
	querySQL, args, err := builder.Build()
	if err != nil {
		s.logger.Info("Failed to build query",
			"error", err)
		writeRFC7807Error(w, validation.NewValidationErrorProblem("query parameters", map[string]string{
			"pagination": err.Error(),
		}))
		return
	}

	// Build count query
	countSQL, _, err := builder.BuildCount()
	if err != nil {
		s.logger.Info("Failed to build count query",
			"error", err)
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
		s.logger.Error(err, "Failed to query audit events")
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
// SELF-AUDITING HELPER FUNCTIONS (DD-STORAGE-012)
// 📋 Design Decision: DD-STORAGE-012 | BR-STORAGE-012, BR-STORAGE-013, BR-STORAGE-014
// Authority: DD-STORAGE-012-AUDIT-INTEGRATION-PLAN.md
// ========================================
//
// These functions implement the three audit points for Data Storage Service:
// 1. datastorage.audit.written - Successful writes
// 2. datastorage.audit.failed - Write failures (before DLQ)
// 3. datastorage.dlq.fallback - DLQ fallback success
//
// BR-STORAGE-012: Data Storage Service must generate audit traces for its own operations
// BR-STORAGE-013: Audit traces must not create circular dependencies (uses InternalAuditClient)
// BR-STORAGE-014: Audit writes must not block business operations (async buffered)
//
// ========================================

// auditWriteSuccess audits successful audit event writes
// BR-STORAGE-012: Audit Point 1 - datastorage.audit.written
func (s *Server) auditWriteSuccess(ctx context.Context, eventID, eventType, correlationID, actorID string) {
	if s.auditStore == nil {
		return // Audit store not initialized (shouldn't happen)
	}

	// Create audit event
	auditEvent := audit.NewAuditEvent()
	auditEvent.EventType = "datastorage.audit.written"
	auditEvent.EventCategory = "storage"
	auditEvent.EventAction = "written"
	auditEvent.EventOutcome = "success"
	auditEvent.ActorType = "service"
	auditEvent.ActorID = "datastorage"
	auditEvent.ResourceType = "AuditEvent"
	auditEvent.ResourceID = eventID
	auditEvent.CorrelationID = correlationID

	// Create event_data (common envelope format)
	eventData := audit.NewEventData(
		"datastorage",
		"audit_written",
		"success",
		map[string]interface{}{
			"event_type": eventType,
			"actor_id":   actorID,
		},
	)
	eventDataJSON, _ := eventData.ToJSON()
	auditEvent.EventData = eventDataJSON

	// Non-blocking audit (async buffered)
	if err := s.auditStore.StoreAudit(ctx, auditEvent); err != nil {
		s.logger.Info("Failed to audit write success",
			"error", err,
			"event_id", eventID,
			"correlation_id", correlationID)

		// Record audit failure metric for visibility (Concern 3)
		if s.metrics != nil && s.metrics.AuditTracesTotal != nil {
			s.metrics.AuditTracesTotal.WithLabelValues("datastorage", "self_audit_failure").Inc()
		}
	}
}

// auditWriteFailure audits audit event write failures (before DLQ fallback)
// BR-STORAGE-012: Audit Point 2 - datastorage.audit.failed
func (s *Server) auditWriteFailure(ctx context.Context, eventID, eventType, correlationID string, writeErr error) {
	if s.auditStore == nil {
		return // Audit store not initialized (shouldn't happen)
	}

	// Use placeholder if eventID is empty (DB write failed before ID generation)
	if eventID == "" {
		eventID = "unknown"
	}

	// Create audit event
	auditEvent := audit.NewAuditEvent()
	auditEvent.EventType = "datastorage.audit.failed"
	auditEvent.EventCategory = "storage"
	auditEvent.EventAction = "write_failed"
	auditEvent.EventOutcome = "failure"
	auditEvent.ActorType = "service"
	auditEvent.ActorID = "datastorage"
	auditEvent.ResourceType = "AuditEvent"
	auditEvent.ResourceID = eventID
	auditEvent.CorrelationID = correlationID

	// Add error details
	errorMsg := writeErr.Error()
	auditEvent.ErrorMessage = &errorMsg

	// Create event_data (common envelope format)
	eventData := audit.NewEventData(
		"datastorage",
		"audit_write_failed",
		"failure",
		map[string]interface{}{
			"event_type": eventType,
			"error":      errorMsg,
		},
	)
	eventDataJSON, _ := eventData.ToJSON()
	auditEvent.EventData = eventDataJSON

	// Non-blocking audit (async buffered)
	if err := s.auditStore.StoreAudit(ctx, auditEvent); err != nil {
		s.logger.Info("Failed to audit write failure",
			"error", err,
			"event_id", eventID,
			"correlation_id", correlationID)

		// Record audit failure metric for visibility (Concern 3)
		if s.metrics != nil && s.metrics.AuditTracesTotal != nil {
			s.metrics.AuditTracesTotal.WithLabelValues("datastorage", "self_audit_failure").Inc()
		}
	}
}

// auditDLQFallback audits successful DLQ fallback after write failure
// BR-STORAGE-012: Audit Point 3 - datastorage.dlq.fallback
func (s *Server) auditDLQFallback(ctx context.Context, eventID, eventType, correlationID, actorID string) {
	if s.auditStore == nil {
		return // Audit store not initialized (shouldn't happen)
	}

	// Create audit event
	auditEvent := audit.NewAuditEvent()
	auditEvent.EventType = "datastorage.dlq.fallback"
	auditEvent.EventCategory = "storage"
	auditEvent.EventAction = "dlq_fallback"
	auditEvent.EventOutcome = "success"
	auditEvent.ActorType = "service"
	auditEvent.ActorID = actorID // Use provided actor_id (event_category from original event)
	auditEvent.ResourceType = "AuditEvent"
	auditEvent.ResourceID = eventID
	auditEvent.CorrelationID = correlationID

	// Create event_data (common envelope format)
	eventData := audit.NewEventData(
		"datastorage",
		"dlq_fallback",
		"success",
		map[string]interface{}{
			"event_type": eventType,
			"reason":     "postgresql_unavailable",
		},
	)
	eventDataJSON, _ := eventData.ToJSON()
	auditEvent.EventData = eventDataJSON

	// Non-blocking audit (async buffered)
	if err := s.auditStore.StoreAudit(ctx, auditEvent); err != nil {
		s.logger.Info("Failed to audit DLQ fallback",
			"error", err,
			"event_id", eventID,
			"correlation_id", correlationID)

		// Record audit failure metric for visibility (Concern 3)
		if s.metrics != nil && s.metrics.AuditTracesTotal != nil {
			s.metrics.AuditTracesTotal.WithLabelValues("datastorage", "self_audit_failure").Inc()
		}
	}
}
