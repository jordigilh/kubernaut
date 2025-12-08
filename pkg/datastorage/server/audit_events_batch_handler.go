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
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// AUDIT EVENTS BATCH WRITE HANDLER
// 📋 Design Decision: DD-AUDIT-002 | BR-AUDIT-001
// Authority: DD-AUDIT-002 "DataStorageClient.StoreBatch"
// ========================================
//
// This handler implements the batch write endpoint required by DD-AUDIT-002.
// The HTTPDataStorageClient.StoreBatch() sends arrays, so this endpoint
// MUST accept arrays.
//
// Endpoint: POST /api/v1/audit/events/batch
// Request Body: JSON array of audit events
// Response: 201 Created with array of event_ids
// Errors: 400 Bad Request, 500 Internal Server Error (RFC 7807)
//
// Defense-in-Depth Testing:
// - Integration tests: test/integration/datastorage/audit_events_batch_write_api_test.go
// - Unit tests: test/unit/datastorage/audit_events_batch_handler_test.go
//
// ========================================

// BatchAuditEventCreatedResponse represents the response for batch audit event creation
type BatchAuditEventCreatedResponse struct {
	EventIDs []string `json:"event_ids"`
	Message  string   `json:"message"`
}

// handleCreateAuditEventsBatch handles POST /api/v1/audit/events/batch
// DD-AUDIT-002: StoreBatch interface must accept arrays
// BR-AUDIT-001: Complete audit trail with no data loss
func (s *Server) handleCreateAuditEventsBatch(w http.ResponseWriter, r *http.Request) {
	s.logger.V(1).Info("handleCreateAuditEventsBatch called",
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// 1. Parse request body as JSON array
	var payloads []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payloads); err != nil {
		// Check if error is due to non-array payload
		errMsg := err.Error()
		if strings.Contains(errMsg, "cannot unmarshal object") {
			s.logger.Info("Batch endpoint received single object instead of array",
				"error", err,
				"remote_addr", r.RemoteAddr)

			if s.metrics != nil && s.metrics.ValidationFailures != nil {
				s.metrics.ValidationFailures.WithLabelValues("body", "not_array").Inc()
			}

			writeRFC7807Error(w, validation.NewValidationErrorProblem(
				"audit_events_batch",
				map[string]string{"body": "request body must be a JSON array, not a single object"},
			))
			return
		}

		s.logger.Info("Invalid JSON array in request body",
			"error", err,
			"remote_addr", r.RemoteAddr)

		if s.metrics != nil && s.metrics.ValidationFailures != nil {
			s.metrics.ValidationFailures.WithLabelValues("body", "invalid_json_array").Inc()
		}

		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"audit_events_batch",
			map[string]string{"body": "request body must be a JSON array: " + err.Error()},
		))
		return
	}

	// 2. Validate batch is not empty
	if len(payloads) == 0 {
		s.logger.Info("Batch endpoint received empty array",
			"remote_addr", r.RemoteAddr)

		if s.metrics != nil && s.metrics.ValidationFailures != nil {
			s.metrics.ValidationFailures.WithLabelValues("body", "empty_batch").Inc()
		}

		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"audit_events_batch",
			map[string]string{"body": "batch cannot be empty"},
		))
		return
	}

	s.logger.V(1).Info("Parsing batch of audit events",
		"count", len(payloads))

	// 3. Validate ALL events BEFORE persisting any (atomic batch)
	auditEvents := make([]*repository.AuditEvent, 0, len(payloads))
	for i, payload := range payloads {
		event, err := s.parseAndValidateBatchEvent(payload)
		if err != nil {
			s.logger.Info("Batch validation failed",
				"index", i,
				"error", err)

			if s.metrics != nil && s.metrics.ValidationFailures != nil {
				s.metrics.ValidationFailures.WithLabelValues("batch_event", "validation_failed").Inc()
			}

			writeRFC7807Error(w, validation.NewValidationErrorProblem(
				"audit_events_batch",
				map[string]string{
					"index": fmt.Sprintf("event at index %d: %s", i, err.Error()),
				},
			))
			return
		}
		auditEvents = append(auditEvents, event)
	}

	// 4. Persist batch atomically (transaction)
	s.logger.V(1).Info("Writing batch to database",
		"count", len(auditEvents))

	start := time.Now()
	createdEvents, err := s.auditEventsRepo.CreateBatch(ctx, auditEvents)
	duration := time.Since(start).Seconds()

	if s.metrics != nil && s.metrics.WriteDuration != nil {
		s.metrics.WriteDuration.WithLabelValues("audit_events_batch").Observe(duration)
	}

	if err != nil {
		s.logger.Error(err, "Batch database write failed",
			"count", len(auditEvents),
			"duration_seconds", duration)

		// Note: DLQ fallback would go here for 5xx errors (GAP-10)
		// Per DD-009: Only 5xx errors should trigger DLQ, not 4xx

		writeRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.io/problems/database-error",
			Title:    "Database Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to write audit events batch to database",
			Instance: r.URL.Path,
		})
		return
	}

	// 5. Build response with event_ids
	eventIDs := make([]string, len(createdEvents))
	for i, event := range createdEvents {
		eventIDs[i] = event.EventID.String()
	}

	// 6. Record metrics
	if s.metrics != nil && s.metrics.AuditTracesTotal != nil {
		for _, event := range createdEvents {
			s.metrics.AuditTracesTotal.WithLabelValues(event.EventCategory, "success").Inc()
		}
	}

	s.logger.Info("Batch audit events created successfully",
		"count", len(eventIDs),
		"duration_seconds", duration)

	// 7. Return 201 Created
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	response := BatchAuditEventCreatedResponse{
		EventIDs: eventIDs,
		Message:  fmt.Sprintf("%d audit events created successfully", len(eventIDs)),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "failed to encode batch response")
	}
}

// parseAndValidateBatchEvent parses and validates a single audit event from batch payload
// Reuses validation logic from single event handler with minor adaptations
func (s *Server) parseAndValidateBatchEvent(payload map[string]interface{}) (*repository.AuditEvent, error) {
	// Validate required fields
	requiredFields := []string{"version", "event_type", "event_timestamp", "correlation_id", "event_data"}
	for _, field := range requiredFields {
		if _, ok := payload[field]; !ok {
			return nil, fmt.Errorf("required field missing: %s", field)
		}
	}

	// Validate fields with aliases (ADR-034 + backward compatibility)
	requiredFieldsWithAliases := map[string][]string{
		"event_category": {"event_category", "service"},
		"event_action":   {"event_action", "operation"},
		"event_outcome":  {"event_outcome", "outcome"},
	}

	for canonical, aliases := range requiredFieldsWithAliases {
		found := false
		for _, alias := range aliases {
			if _, ok := payload[alias]; ok {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("required field missing: %s (accepted: %v)", canonical, aliases)
		}
	}

	// Extract and parse fields
	eventType, _ := payload["event_type"].(string)
	eventCategory, ok := payload["event_category"].(string)
	if !ok {
		eventCategory, _ = payload["service"].(string)
	}
	eventAction, ok := payload["event_action"].(string)
	if !ok {
		eventAction, _ = payload["operation"].(string)
	}
	eventOutcome, ok := payload["event_outcome"].(string)
	if !ok {
		eventOutcome, _ = payload["outcome"].(string)
	}
	correlationID, _ := payload["correlation_id"].(string)
	eventTimestampStr, _ := payload["event_timestamp"].(string)

	eventTimestamp, err := time.Parse(time.RFC3339Nano, eventTimestampStr)
	if err != nil {
		eventTimestamp, err = time.Parse(time.RFC3339, eventTimestampStr)
		if err != nil {
			return nil, fmt.Errorf("event_timestamp must be RFC3339 format: %w", err)
		}
	}

	// Extract optional fields with defaults
	actorType, _ := payload["actor_type"].(string)
	if actorType == "" {
		actorType = "service"
	}
	actorID, _ := payload["actor_id"].(string)
	if actorID == "" {
		actorID = eventCategory + "-service"
	}
	resourceType, _ := payload["resource_type"].(string)
	if resourceType == "" {
		resourceType = eventCategory
	}
	resourceID, _ := payload["resource_id"].(string)
	if resourceID == "" {
		resourceID = correlationID
	}
	severity, _ := payload["severity"].(string)
	if severity == "" {
		severity = "info"
	}
	resourceNamespace, _ := payload["resource_namespace"].(string)
	clusterName, _ := payload["cluster_name"].(string)

	// Extract event_data as map
	eventData, ok := payload["event_data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("event_data must be a JSON object")
	}

	return &repository.AuditEvent{
		EventID:           uuid.New(),
		EventTimestamp:    eventTimestamp,
		EventDate:         eventTimestamp.Truncate(24 * time.Hour),
		EventType:         eventType,
		EventCategory:     eventCategory,
		EventAction:       eventAction,
		EventOutcome:      eventOutcome,
		CorrelationID:     correlationID,
		ActorType:         actorType,
		ActorID:           actorID,
		ResourceType:      resourceType,
		ResourceID:        resourceID,
		ResourceNamespace: resourceNamespace,
		ClusterID:         clusterName, // ClusterID per ADR-034 schema
		Severity:          severity,
		EventData:         eventData,
	}, nil
}

