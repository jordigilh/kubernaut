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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
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
	CorrelationID    string                 `json:"correlation_id"`
	IsValid          bool                   `json:"is_valid"`
	TotalEvents      int                    `json:"total_events"`
	VerifiedEvents   int                    `json:"verified_events"`
	TamperedEvents   []TamperedEvent        `json:"tampered_events,omitempty"`
	VerificationTime time.Time              `json:"verification_time"`
	Message          string                 `json:"message"`
}

// TamperedEvent contains details about a tampered event
type TamperedEvent struct {
	EventID          string    `json:"event_id"`
	EventTimestamp   time.Time `json:"event_timestamp"`
	ExpectedHash     string    `json:"expected_hash"`
	ActualHash       string    `json:"actual_hash"`
	PreviousHash     string    `json:"previous_hash"`
	Message          string    `json:"message"`
}

// HandleVerifyChain verifies the hash chain integrity for a correlation_id
func (s *Server) HandleVerifyChain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Parse request
	var req VerifyChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error(err, "Failed to decode verify chain request")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.CorrelationID == "" {
		http.Error(w, "correlation_id is required", http.StatusBadRequest)
		return
	}

	// Verify chain
	response, err := s.verifyHashChain(ctx, req.CorrelationID)
	if err != nil {
		s.logger.Error(err, "Failed to verify hash chain",
			"correlation_id", req.CorrelationID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if !response.IsValid {
		w.WriteHeader(http.StatusOK) // Still 200, but IsValid=false indicates tampering
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to encode verify chain response")
	}
}

// verifyHashChain performs the actual hash chain verification
func (s *Server) verifyHashChain(ctx context.Context, correlationID string) (*VerifyChainResponse, error) {
	// Query all events for this correlation_id, ordered by timestamp
	query := `
		SELECT
			event_id, event_timestamp, event_type,
			event_category, event_action, event_outcome,
			correlation_id, parent_event_id, parent_event_date,
			resource_type, resource_id, namespace, cluster_name,
			actor_id, actor_type,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data,
			event_hash, previous_event_hash,
			event_version
		FROM audit_events
		WHERE correlation_id = $1
		  AND event_hash IS NOT NULL
		ORDER BY event_timestamp ASC, event_id ASC
	`

	rows, err := s.db.QueryContext(ctx, query, correlationID)
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
		event := &repository.AuditEvent{}
		var eventDataJSON []byte

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
			&event.ResourceNamespace,
			&event.ClusterID,
			&event.ActorID,
			&event.ActorType,
			&event.Severity,
			&event.DurationMs,
			&event.ErrorCode,
			&event.ErrorMessage,
			&event.RetentionDays,
			&event.IsSensitive,
			&eventDataJSON,
			&event.EventHash,
			&event.PreviousEventHash,
			&event.Version,
		)
		if err != nil {
			return nil, err
		}

		// Unmarshal event_data
		if len(eventDataJSON) > 0 {
			if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
				return nil, err
			}
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Verify hash chain
	response := &VerifyChainResponse{
		CorrelationID:    correlationID,
		IsValid:          true,
		TotalEvents:      len(events),
		VerifiedEvents:   0,
		TamperedEvents:   []TamperedEvent{},
		VerificationTime: time.Now().UTC(),
	}

	if len(events) == 0 {
		response.Message = "No events found for correlation_id"
		return response, nil
	}

	// Verify each event's hash
	previousHash := ""
	for i, event := range events {
		// Calculate expected hash
		expectedHash, err := calculateExpectedHash(previousHash, event)
		if err != nil {
			return nil, err
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

		// First event should have empty previous_hash
		if i == 0 && previousHash != "" {
			response.IsValid = false
			response.TamperedEvents = append(response.TamperedEvents, TamperedEvent{
				EventID:        event.EventID.String(),
				EventTimestamp: event.EventTimestamp,
				ExpectedHash:   "",
				ActualHash:     "",
				PreviousHash:   previousHash,
				Message:        "First event in chain should have empty previous_event_hash",
			})
		}

		// Update previous hash for next iteration
		previousHash = event.EventHash
	}

	if response.IsValid {
		response.Message = "Hash chain verified successfully: no tampering detected"
	} else {
		response.Message = "Hash chain verification FAILED: tampering detected"
	}

	return response, nil
}

// calculateExpectedHash computes the expected SHA256 hash for verification
// Must match the calculateEventHash logic in repository
func calculateExpectedHash(previousHash string, event *repository.AuditEvent) (string, error) {
	// Serialize event to JSON (canonical form for consistent hashing)
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return "", err
	}

	// Compute hash: SHA256(previous_hash + event_json)
	hasher := sha256.New()
	hasher.Write([]byte(previousHash))
	hasher.Write(eventJSON)
	hashBytes := hasher.Sum(nil)

	return hex.EncodeToString(hashBytes), nil
}



