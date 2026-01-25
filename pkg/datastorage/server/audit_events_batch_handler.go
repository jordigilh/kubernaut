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

	"github.com/jordigilh/kubernaut/pkg/audit"
	dsclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/helpers"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
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

	// 1. Parse request body as JSON array using OpenAPI type (type-safe)
	var requests []dsclient.AuditEventRequest
	if err := json.NewDecoder(r.Body).Decode(&requests); err != nil {
		// Check if error is due to non-array payload
		errMsg := err.Error()
		if strings.Contains(errMsg, "cannot unmarshal object") {
			s.logger.Info("Batch endpoint received single object instead of array", "error", err)

		// Metrics are guaranteed non-nil by constructor
		s.metrics.ValidationFailures.WithLabelValues("body", "not_array").Inc()

			response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_request", "Invalid Request", "request body must be a JSON array, not a single object", s.logger)
			return
		}

		s.logger.Info("Invalid JSON array in request body", "error", err)

	// Metrics are guaranteed non-nil by constructor
	s.metrics.ValidationFailures.WithLabelValues("body", "invalid_json_array").Inc()

		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_request", "Invalid Request", "request body must be a JSON array: "+err.Error(), s.logger)
		return
	}

	// 2. Validate batch is not empty
	if len(requests) == 0 {
		s.logger.Info("Batch endpoint received empty array")

	// Metrics are guaranteed non-nil by constructor
	s.metrics.ValidationFailures.WithLabelValues("body", "empty_batch").Inc()

		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error", "batch cannot be empty", s.logger)
		return
	}

	s.logger.V(1).Info("Parsing batch of audit events", "count", len(requests))

	// 3. Validate and convert ALL events BEFORE persisting any (atomic batch)
	auditEvents := make([]*audit.AuditEvent, 0, len(requests))
	repositoryEvents := make([]*repository.AuditEvent, 0, len(requests))

	for i, req := range requests {
		// Validate business rules
		if err := helpers.ValidateAuditEventRequest(&req); err != nil {
			s.logger.Info("Batch validation failed", "index", i, "error", err)

		// Metrics are guaranteed non-nil by constructor
		s.metrics.ValidationFailures.WithLabelValues("batch_event", "validation_failed").Inc()

			response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error", fmt.Sprintf("event at index %d: %s", i, err.Error()), s.logger)
			return
		}

		// Convert to internal type
		internalEvent, err := helpers.ConvertAuditEventRequest(req)
		if err != nil {
			s.logger.Info("Batch conversion failed", "index", i, "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "conversion_error", "Conversion Error", fmt.Sprintf("event at index %d: %s", i, err.Error()), s.logger)
			return
		}
		auditEvents = append(auditEvents, internalEvent)

		// Convert to repository type
		repoEvent, err := helpers.ConvertToRepositoryAuditEvent(internalEvent)
		if err != nil {
			// Conversion errors are client-side validation errors (e.g., invalid event_data JSON)
			// Return 400 Bad Request, not 500 Internal Server Error
			s.logger.Info("Batch repository conversion failed - invalid event_data", "index", i, "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_event_data", "Invalid Event Data", fmt.Sprintf("event at index %d: %s", i, err.Error()), s.logger)
			return
		}
		repositoryEvents = append(repositoryEvents, repoEvent)
	}

	// 4. Persist batch atomically (transaction)
	s.logger.V(1).Info("Writing batch to database", "count", len(repositoryEvents))

	start := time.Now()
	createdEvents, err := s.auditEventsRepo.CreateBatch(ctx, repositoryEvents)
	duration := time.Since(start).Seconds()

	// Metrics are guaranteed non-nil by constructor
	s.metrics.WriteDuration.WithLabelValues("audit_events_batch").Observe(duration)

	if err != nil {
		s.logger.Error(err, "Batch database write failed",
			"count", len(auditEvents),
			"duration_seconds", duration)

		// Note: DLQ fallback would go here for 5xx errors (GAP-10)
		// Per DD-009: Only 5xx errors should trigger DLQ, not 4xx

		writeValidationRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.ai/problems/database-error",
			Title:    "Database Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to write audit events batch to database",
			Instance: r.URL.Path,
		}, s)
		return
	}

	// 5. Build response with event_ids
	eventIDs := make([]string, len(createdEvents))
	for i, event := range createdEvents {
		eventIDs[i] = event.EventID.String()
	}

	// 6. Record metrics
	// Metrics are guaranteed non-nil by constructor
	for _, event := range createdEvents {
		s.metrics.AuditTracesTotal.WithLabelValues(string(event.EventCategory), "success").Inc()
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
