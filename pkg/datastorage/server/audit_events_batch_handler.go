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
	"github.com/lib/pq"

	"github.com/jordigilh/kubernaut/pkg/audit"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
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

	requests, ok := s.decodeAuditEventsBatchRequest(w, r)
	if !ok {
		return
	}

	authenticatedActorID := r.Header.Get("X-Auth-Request-User")
	auditEvents, repositoryEvents, ok := s.validateAndConvertBatch(w, requests, authenticatedActorID)
	if !ok {
		return
	}

	if !s.resolveBatchParentDates(ctx, w, requests, auditEvents, repositoryEvents) {
		return
	}

	s.warnOutOfOrderTimestamps(auditEvents)
	s.persistAuditEventsBatch(ctx, w, r, auditEvents, repositoryEvents)
}

// decodeAuditEventsBatchRequest parses and size-validates the batch request
// body. Returns ok=false after writing the appropriate RFC 7807 error
// response if the body is not a valid, non-empty, within-limits JSON array.
func (s *Server) decodeAuditEventsBatchRequest(w http.ResponseWriter, r *http.Request) ([]dsclient.AuditEventRequest, bool) {
	var requests []dsclient.AuditEventRequest
	if err := json.NewDecoder(r.Body).Decode(&requests); err != nil {
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, s.logger)
			return nil, false
		}
		if strings.Contains(err.Error(), "cannot unmarshal object") {
			s.logger.Info("Batch endpoint received single object instead of array", "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_request", "Invalid Request", "request body must be a JSON array, not a single object", s.logger)
			return nil, false
		}
		s.logger.Info("Invalid JSON array in request body", "error", err)
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_request", "Invalid Request",
			"The request body must be a valid JSON array of audit event objects", s.logger)
		return nil, false
	}

	if len(requests) == 0 {
		s.logger.Info("Batch endpoint received empty array")
		response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error", "batch cannot be empty", s.logger)
		return nil, false
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
		return nil, false
	}

	s.logger.V(1).Info("Parsing batch of audit events", "count", len(requests))
	return requests, true
}

// batchParentRef pairs a request's index in the batch with its declared
// parent_event_id, for the batched FK existence check in
// resolveBatchParentDates.
type batchParentRef struct {
	index    int
	parentID uuid.UUID
}

// validateAndConvertBatch validates and converts every request in the batch
// to internal/repository audit events BEFORE any persistence, preserving the
// atomic-batch guarantee (an invalid event at any position rejects the whole
// batch). Returns ok=false after writing the RFC 7807 error response for the
// first failing event.
func (s *Server) validateAndConvertBatch(w http.ResponseWriter, requests []dsclient.AuditEventRequest, authenticatedActorID string) ([]*audit.AuditEvent, []*repository.AuditEvent, bool) {
	auditEvents := make([]*audit.AuditEvent, 0, len(requests))
	repositoryEvents := make([]*repository.AuditEvent, 0, len(requests))

	for i, req := range requests {
		if err := helpers.ValidateAuditEventRequest(&req); err != nil {
			s.logger.Info("Batch validation failed", "index", i, "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error",
				fmt.Sprintf("event at index %d failed validation", i), s.logger)
			return nil, nil, false
		}

		internalEvent, err := helpers.ConvertAuditEventRequest(req, authenticatedActorID)
		if err != nil {
			s.logger.Info("Batch conversion failed", "index", i, "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "conversion_error", "Conversion Error",
				fmt.Sprintf("event at index %d could not be converted", i), s.logger)
			return nil, nil, false
		}

		if err := dlq.ValidateEventData(internalEvent.EventData); err != nil {
			s.logger.Info("Batch EventData validation failed", "index", i, "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error",
				fmt.Sprintf("event at index %d: event_data exceeds size or nesting depth limits", i), s.logger)
			return nil, nil, false
		}
		auditEvents = append(auditEvents, internalEvent)

		repoEvent, err := helpers.ConvertToRepositoryAuditEvent(internalEvent)
		if err != nil {
			s.logger.Info("Batch repository conversion failed - invalid event_data", "index", i, "error", err)
			response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid_event_data", "Invalid Event Data",
				fmt.Sprintf("event at index %d has invalid event_data", i), s.logger)
			return nil, nil, false
		}
		repositoryEvents = append(repositoryEvents, repoEvent)
	}

	return auditEvents, repositoryEvents, true
}

// resolveBatchParentDates performs the PERF-H1 batched FK existence check:
// a single query resolves event_date for every declared parent_event_id in
// the batch, instead of one query per row. Returns ok=false after writing
// the RFC 7807 error response if the lookup query fails or any referenced
// parent event does not exist.
func (s *Server) resolveBatchParentDates(ctx context.Context, w http.ResponseWriter, requests []dsclient.AuditEventRequest, auditEvents []*audit.AuditEvent, repositoryEvents []*repository.AuditEvent) bool {
	var parentRefs []batchParentRef
	for i, req := range requests {
		if req.ParentEventID.IsSet() {
			parentRefs = append(parentRefs, batchParentRef{index: i, parentID: req.ParentEventID.Value})
		}
	}
	if len(parentRefs) == 0 {
		return true
	}

	parentIDs := make([]uuid.UUID, 0, len(parentRefs))
	for _, pr := range parentRefs {
		parentIDs = append(parentIDs, pr.parentID)
	}

	foundParents, fkErr := s.batchLookupParentDates(ctx, parentIDs)
	if fkErr != nil {
		s.logger.Error(fkErr, "Batch FK lookup failed")
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"query-error", "Internal Server Error",
			"Failed to verify parent events", s.logger)
		return false
	}

	for _, pr := range parentRefs {
		parentDate, ok := foundParents[pr.parentID]
		if !ok {
			s.logger.Info("Batch parent event not found", "index", pr.index, "parent_event_id", pr.parentID.String())
			response.WriteRFC7807Error(w, http.StatusBadRequest, "validation-error", "Validation Error",
				fmt.Sprintf("event at index %d: parent event does not exist", pr.index), s.logger)
			return false
		}
		// DF-M2: Propagate resolved parent date into the internal event
		// so ConvertToRepositoryAuditEvent carries it to the DB layer.
		auditEvents[pr.index].ParentEventDate = &parentDate
		repositoryEvents[pr.index].ParentEventDate = &parentDate
	}
	return true
}

// persistAuditEventsBatch writes the batch atomically via CreateBatch. On
// success, it responds 201 with the created event_ids. On DB failure, it
// falls back to per-item DLQ enqueue (DD-009) and responds 202 if every item
// was queued, or 500 if any item was lost.
func (s *Server) persistAuditEventsBatch(ctx context.Context, w http.ResponseWriter, r *http.Request, auditEvents []*audit.AuditEvent, repositoryEvents []*repository.AuditEvent) {
	s.logger.V(1).Info("Writing batch to database", "count", len(repositoryEvents))

	start := time.Now()
	createdEvents, err := s.auditEventsRepo.CreateBatch(ctx, repositoryEvents)
	duration := time.Since(start).Seconds()

	// Metrics are guaranteed non-nil by constructor
	s.metrics.WriteDuration.WithLabelValues("audit_events_batch").Observe(duration)

	if err != nil {
		s.handleBatchWriteFailure(w, r, auditEvents, err, duration) //nolint:contextcheck // DD-009 per-item DLQ fallback is a last-resort integrity path; must not risk losing events to the same request context that saw the primary write fail
		return
	}

	eventIDs := make([]string, len(createdEvents))
	for i, event := range createdEvents {
		eventIDs[i] = event.EventID.String()
	}

	s.logger.Info("Batch audit events created successfully",
		"count", len(eventIDs),
		"duration_seconds", duration)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	resp := BatchAuditEventCreatedResponse{
		EventIDs: eventIDs,
		Message:  fmt.Sprintf("%d audit events created successfully", len(eventIDs)),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error(err, "failed to encode batch response")
	}
}

// handleBatchWriteFailure implements the DD-009 per-item DLQ fallback when
// the atomic batch write to the database fails: every event is individually
// enqueued to the DLQ for async retry so no event is lost on a transient DB
// outage. Responds 202 if all events were queued, 500 if any were lost.
func (s *Server) handleBatchWriteFailure(w http.ResponseWriter, r *http.Request, auditEvents []*audit.AuditEvent, writeErr error, duration float64) {
	s.logger.Error(writeErr, "Batch database write failed, attempting per-item DLQ fallback",
		"count", len(auditEvents),
		"duration_seconds", duration)

	dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dlqCancel()

	var dlqSuccessCount, dlqFailCount int
	for i, auditEvent := range auditEvents {
		if dlqErr := s.dlqClient.EnqueueAuditEvent(dlqCtx, auditEvent, writeErr); dlqErr != nil {
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
		s.logger.Error(writeErr, "Batch DLQ fallback partially failed",
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
}

// warnOutOfOrderTimestamps logs when events in the same batch share a correlation_id
// but appear in strictly decreasing timestamp order (soft check for clock skew; batch is still accepted).
func (s *Server) warnOutOfOrderTimestamps(events []*audit.AuditEvent) {
	byCorrelation := make(map[string][]time.Time)
	for _, e := range events {
		if e == nil {
			continue
		}
		byCorrelation[e.CorrelationID] = append(byCorrelation[e.CorrelationID], e.EventTimestamp)
	}
	for corrID, timestamps := range byCorrelation {
		for i := 1; i < len(timestamps); i++ {
			if timestamps[i].Before(timestamps[i-1]) {
				s.logger.Info("out-of-order event timestamps in batch",
					"correlation_id", corrID,
					"event_index", i,
					"current_ts", timestamps[i],
					"previous_ts", timestamps[i-1])
			}
		}
	}
}

// batchLookupParentDates resolves parent event dates for a set of event IDs
// in a single query. PERF-H1: Replaces N per-row queries with one batch query.
func (s *Server) batchLookupParentDates(ctx context.Context, parentIDs []uuid.UUID) (map[uuid.UUID]time.Time, error) {
	query := `SELECT event_id, event_date FROM audit_events WHERE event_id = ANY($1)`
	rows, err := s.db.QueryContext(ctx, query, pq.Array(parentIDs))
	if err != nil {
		return nil, fmt.Errorf("batch parent lookup: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			s.logger.Error(cerr, "failed to close batch parent lookup rows")
		}
	}()

	result := make(map[uuid.UUID]time.Time, len(parentIDs))
	for rows.Next() {
		var id uuid.UUID
		var date time.Time
		if err := rows.Scan(&id, &date); err != nil {
			return nil, fmt.Errorf("batch parent lookup scan: %w", err)
		}
		result[id] = date
	}
	return result, rows.Err()
}
