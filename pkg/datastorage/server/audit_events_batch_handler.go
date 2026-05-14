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
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
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

// BatchAuditEventAcceptedResponse is returned when batch DB write fails
// but all events were successfully queued to DLQ for async retry (DD-009).
type BatchAuditEventAcceptedResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Count   int    `json:"count"`
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
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, s.logger)
			return
		}
		// Check if error is due to non-array payload
		errMsg := err.Error()
		if strings.Contains(errMsg, "cannot unmarshal object") {
			s.logger.Info("Batch endpoint received single object instead of array", "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_request", "Invalid Request", "request body must be a JSON array, not a single object", s.logger)
			return
		}

		s.logger.Info("Invalid JSON array in request body", "error", err)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_request", "Invalid Request",
			"The request body must be a valid JSON array of audit event objects", s.logger)
		return
	}

	// 2. Validate batch is not empty
	if len(requests) == 0 {
		s.logger.Info("Batch endpoint received empty array")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error", "batch cannot be empty", s.logger)
		return
	}

	// Issue #667 / BR-STORAGE-043: Enforce maximum batch size to prevent lock amplification
	if s.maxBatchSize > 0 && len(requests) > s.maxBatchSize {
		s.logger.Info("Batch exceeds maximum size",
			"count", len(requests), "max", s.maxBatchSize)
		writeValidationRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.ai/problems/batch-size-exceeded",
			Title:    "Batch Size Exceeded",
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("batch size %d exceeds maximum allowed batch size of %d", len(requests), s.maxBatchSize),
			Instance: r.URL.Path,
		}, s)
		return
	}

	s.logger.V(1).Info("Parsing batch of audit events", "count", len(requests))

	authenticatedActorID := r.Header.Get("X-Auth-Request-User")

	// 3. Validate and convert ALL events BEFORE persisting any (atomic batch)
	auditEvents := make([]*audit.AuditEvent, 0, len(requests))
	repositoryEvents := make([]*repository.AuditEvent, 0, len(requests))

	for i, req := range requests {
		// Validate business rules
		if err := helpers.ValidateAuditEventRequest(&req); err != nil {
			s.logger.Info("Batch validation failed", "index", i, "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error", fmt.Sprintf("event at index %d: %s", i, err.Error()), s.logger)
			return
		}

		// Convert to internal type
		internalEvent, err := helpers.ConvertAuditEventRequest(req, authenticatedActorID)
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
		s.logger.Error(err, "Batch database write failed, attempting per-item DLQ fallback",
			"count", len(auditEvents),
			"duration_seconds", duration)

		// DD-009: DLQ fallback on database errors (per-item, mirroring single-event path)
		dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dlqCancel()

		var dlqSuccessCount, dlqFailCount int
		for i, auditEvent := range auditEvents {
			if dlqErr := s.dlqClient.EnqueueAuditEvent(dlqCtx, auditEvent, err); dlqErr != nil {
				s.logger.Error(dlqErr, "DLQ fallback failed for batch item — data loss risk",
					"index", i,
					"event_type", auditEvent.EventType,
					"correlation_id", auditEvent.CorrelationID)
				dlqFailCount++
			} else {
				dlqSuccessCount++
			}
		}

		if dlqFailCount > 0 {
			s.logger.Error(err, "Batch DLQ fallback partially failed",
				"dlq_success", dlqSuccessCount,
				"dlq_failed", dlqFailCount,
				"total", len(auditEvents))
			writeValidationRFC7807Error(w, &validation.RFC7807Problem{
				Type:     "https://kubernaut.ai/problems/database-error",
				Title:    "Database Error",
				Status:   http.StatusInternalServerError,
				Detail:   fmt.Sprintf("Batch write failed; %d of %d events queued to DLQ, %d lost", dlqSuccessCount, len(auditEvents), dlqFailCount),
				Instance: r.URL.Path,
			}, s)
			return
		}

		s.logger.Info("Batch DLQ fallback succeeded — all events queued",
			"count", dlqSuccessCount)
		response.WriteJSON(w, http.StatusAccepted, BatchAuditEventAcceptedResponse{
			Status:  "accepted",
			Message: fmt.Sprintf("%d audit events queued for async processing via DLQ", dlqSuccessCount),
			Count:   dlqSuccessCount,
		}, s.logger)
		return
	}

	// 5. Build response with event_ids
	eventIDs := make([]string, len(createdEvents))
	for i, event := range createdEvents {
		eventIDs[i] = event.EventID.String()
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
