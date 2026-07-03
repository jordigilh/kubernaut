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
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// ========================================
// SOC2 Gap #9: Hash Chain Verification API
// Authority: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md - Day 7
// POST /api/v1/audit/verify-chain
// ========================================
//
// Verifies the integrity of audit event hash chains for a given correlation_id.
// Returns verification status, any broken links, and tampered events.
//
// SOC2 Compliance:
// - SOC 2 Type II: Tamper-evident audit logs (Trust Services Criteria CC8.1)
// - NIST 800-53: AU-9 (Protection of Audit Information)
// - Sarbanes-Oxley: Section 404 (Internal Controls)
//
// ========================================

// VerifyChainRequest contains the correlation_id to verify
type VerifyChainRequest struct {
	CorrelationID string `json:"correlation_id"`
}

// VerifyChainResponse contains the verification results
type VerifyChainResponse struct {
	CorrelationID    string          `json:"correlation_id"`
	IsValid          bool            `json:"is_valid"`
	TotalEvents      int             `json:"total_events"`
	VerifiedEvents   int             `json:"verified_events"`
	SkippedNullHash  int             `json:"skipped_null_hash,omitempty"`
	TamperedEvents   []TamperedEvent `json:"tampered_events,omitempty"`
	VerificationTime time.Time       `json:"verification_time"`
	Message          string          `json:"message"`
}

// TamperedEvent contains details about a tampered event
type TamperedEvent struct {
	EventID        string    `json:"event_id"`
	EventTimestamp time.Time `json:"event_timestamp"`
	ExpectedHash   string    `json:"expected_hash"`
	ActualHash     string    `json:"actual_hash"`
	PreviousHash   string    `json:"previous_hash"`
	Message        string    `json:"message"`
}

// HandleVerifyChain verifies the hash chain integrity for a correlation_id
func (s *Server) HandleVerifyChain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteRFC7807Error(w, http.StatusMethodNotAllowed,
			"method-not-allowed", "Method Not Allowed",
			"Only POST is accepted for verify-chain", s.logger)
		return
	}

	ctx := r.Context()

	// Parse request
	var req VerifyChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, s.logger)
			return
		}
		s.logger.Error(err, "Failed to decode verify chain request")
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"invalid-request-body", "Invalid Request Body",
			"The request body could not be parsed as JSON", s.logger)
		return
	}

	if req.CorrelationID == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"missing-correlation-id", "Missing Required Field",
			"Field 'correlation_id' is required", s.logger)
		return
	}

	const maxCorrelationIDLen = 256
	if len(req.CorrelationID) > maxCorrelationIDLen {
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"invalid-correlation-id", "Invalid Correlation ID",
			"Field 'correlation_id' exceeds maximum length of 256 characters", s.logger)
		return
	}

	// Verify chain
	resp, err := s.verifyHashChain(ctx, req.CorrelationID)
	if err != nil {
		response.WriteRFC7807InternalError(w,
			"verify-chain/internal-error", "Verification Failed",
			err, s.logger)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if !resp.IsValid {
		w.WriteHeader(http.StatusOK) // Still 200, but IsValid=false indicates tampering
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error(err, "Failed to encode verify chain response")
	}
}

// MaxVerifyChainEvents caps the number of events loaded for a single
// verify-chain request to bound memory and query time. A correlation_id
// with more events than this limit must be verified via export/offline tooling.
const MaxVerifyChainEvents = 10000

// verifyHashChain performs the actual hash chain verification
func (s *Server) verifyHashChain(ctx context.Context, correlationID string) (*VerifyChainResponse, error) {
	var skippedNullHash int
	countQuery := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1 AND event_hash IS NULL`
	if err := s.db.QueryRowContext(ctx, countQuery, correlationID).Scan(&skippedNullHash); err != nil {
		return nil, fmt.Errorf("failed to count NULL-hash events: %w", err)
	}

	events, err := s.queryVerifyChainEvents(ctx, correlationID)
	if err != nil {
		return nil, err
	}

	if len(events) > MaxVerifyChainEvents {
		return nil, fmt.Errorf("correlation_id has more than %d hashed events; use export/offline verification", MaxVerifyChainEvents)
	}

	response := &VerifyChainResponse{
		CorrelationID:    correlationID,
		IsValid:          true,
		TotalEvents:      len(events),
		VerifiedEvents:   0,
		SkippedNullHash:  skippedNullHash,
		TamperedEvents:   []TamperedEvent{},
		VerificationTime: time.Now().UTC(),
	}

	if len(events) == 0 {
		response.Message = "No events found for correlation_id"
		return response, nil
	}

	// Verify each event's hash (GAP-05: algorithm-aware, honors each event's own hash_algorithm)
	if err := verifyEventChain(response, events, s.auditEventsRepo.HMACKey()); err != nil {
		return nil, err
	}

	if response.IsValid {
		response.Message = "Hash chain verified successfully: no tampering detected"
	} else {
		response.Message = "Hash chain verification FAILED: tampering detected"
	}
	if skippedNullHash > 0 {
		response.Message += fmt.Sprintf(" (%d events without hashes were excluded from verification)", skippedNullHash)
	}

	return response, nil
}

// queryVerifyChainEvents queries the (up to MaxVerifyChainEvents+1) hashed
// audit events for correlationID, ordered oldest-first, and scans each row
// into a repository.AuditEvent. Extracted from verifyHashChain
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code motion, no behavior
// change.
func (s *Server) queryVerifyChainEvents(ctx context.Context, correlationID string) ([]*repository.AuditEvent, error) {
	query := `
		SELECT
			event_id, event_timestamp, event_type,
			event_category, event_action, event_outcome,
			correlation_id, parent_event_id, parent_event_date,
			resource_type, resource_id, namespace, cluster_name,
			actor_id, actor_type, actor_ip,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data,
			event_hash, previous_event_hash, hash_algorithm,
			event_version
		FROM audit_events
		WHERE correlation_id = $1
		  AND event_hash IS NOT NULL
		ORDER BY event_timestamp ASC, event_id ASC
		LIMIT $2
	`

	rows, err := s.db.QueryContext(ctx, query, correlationID, MaxVerifyChainEvents+1)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Error(err, "Failed to close database rows")
		}
	}()

	var events []*repository.AuditEvent
	for rows.Next() {
		event, err := scanVerifyChainEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// scanVerifyChainEvent scans a single audit_events row into a
// repository.AuditEvent, applying the sql.Null* -> Go type conversions and
// forcing the timestamp to UTC for hash-consistency (write-time JSON was
// computed against a UTC timestamp). Extracted from verifyHashChain
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code motion, no behavior
// change.
func scanVerifyChainEvent(rows *sql.Rows) (*repository.AuditEvent, error) {
	event := &repository.AuditEvent{}
	var eventDataJSON []byte

	// Nullable columns must use sql.Null* types; ToNullStringValue("")
	// stores empty strings as SQL NULL, so scanning into a bare
	// Go string/int would fail with "converting NULL to <type>".
	var (
		namespace    sql.NullString
		clusterName  sql.NullString
		actorIP      sql.NullString
		severity     sql.NullString
		durationMs   sql.NullInt32
		errorCode    sql.NullString
		errorMessage sql.NullString
	)

	err := rows.Scan(
		&event.EventID,
		&event.EventTimestamp,
		&event.EventType,
		&event.EventCategory,
		&event.EventAction,
		&event.EventOutcome,
		&event.CorrelationID,
		&event.ParentEventID,
		&event.ParentEventDate,
		&event.ResourceType,
		&event.ResourceID,
		&namespace,
		&clusterName,
		&event.ActorID,
		&event.ActorType,
		&actorIP,
		&severity,
		&durationMs,
		&errorCode,
		&errorMessage,
		&event.RetentionDays,
		&event.IsSensitive,
		&eventDataJSON,
		&event.EventHash,
		&event.PreviousEventHash,
		&event.HashAlgorithm,
		&event.Version,
	)
	if err != nil {
		return nil, err
	}

	assignVerifyChainNullableFields(event, namespace, clusterName, actorIP, severity, durationMs, errorCode, errorMessage)

	// CRITICAL: Force timestamp to UTC for hash consistency
	// PostgreSQL timestamptz stores in UTC but Go reads them with local timezone.
	// Must match the write-time JSON representation for correct hash verification.
	event.EventTimestamp = event.EventTimestamp.UTC()

	// Unmarshal event_data
	if len(eventDataJSON) > 0 {
		if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
			return nil, err
		}
	}

	return event, nil
}

// assignVerifyChainNullableFields copies the sql.Null* columns scanned by
// scanVerifyChainEvent onto event's plain Go-typed fields (NULL -> zero
// value). Extracted from scanVerifyChainEvent (Wave 6 6f GREEN: funlen
// remediation) — pure code motion, no behavior change.
func assignVerifyChainNullableFields(
	event *repository.AuditEvent,
	namespace, clusterName, actorIP, severity sql.NullString,
	durationMs sql.NullInt32,
	errorCode, errorMessage sql.NullString,
) {
	event.ResourceNamespace = namespace.String
	event.ClusterID = clusterName.String
	event.ActorIP = actorIP.String
	event.Severity = severity.String
	event.DurationMs = int(durationMs.Int32)
	event.ErrorCode = errorCode.String
	event.ErrorMessage = errorMessage.String
}

// verifyEventChain walks events in order, recomputing each event's expected
// hash (GAP-05: algorithm-aware, honors each event's own hash_algorithm) and
// comparing it against the stored hash and previous_event_hash link,
// appending any mismatches to response.TamperedEvents and updating
// response.IsValid/VerifiedEvents in place. Extracted from verifyHashChain
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code motion, no behavior
// change.
func verifyEventChain(response *VerifyChainResponse, events []*repository.AuditEvent, hmacKey []byte) error {
	previousHash := ""
	for _, event := range events {
		expectedHash, err := repository.CalculateHashForVerification(hmacKey, previousHash, event)
		if err != nil {
			return err
		}

		// Verify previous_event_hash matches
		if event.PreviousEventHash != previousHash {
			response.IsValid = false
			response.TamperedEvents = append(response.TamperedEvents, TamperedEvent{
				EventID:        event.EventID.String(),
				EventTimestamp: event.EventTimestamp,
				ExpectedHash:   "",
				ActualHash:     "",
				PreviousHash:   previousHash,
				Message:        "Previous hash mismatch: event claims different previous_event_hash",
			})
		}

		// Verify event_hash matches calculated hash
		if event.EventHash != expectedHash {
			response.IsValid = false
			response.TamperedEvents = append(response.TamperedEvents, TamperedEvent{
				EventID:        event.EventID.String(),
				EventTimestamp: event.EventTimestamp,
				ExpectedHash:   expectedHash,
				ActualHash:     event.EventHash,
				PreviousHash:   previousHash,
				Message:        "Event hash mismatch: event data has been tampered",
			})
		} else {
			response.VerifiedEvents++
		}

		// Update previous hash for next iteration
		previousHash = event.EventHash
	}
	return nil
}
