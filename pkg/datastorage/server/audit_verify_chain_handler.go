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
	"time"

	"github.com/google/uuid"
)

// ========================================
// AUDIT CHAIN VERIFICATION (SOC2 Gap #9)
// 📋 Design Decision: DD-IMMUDB-001 | ✅ Approved Design | Confidence: 85%
// See: docs/development/SOC2/GAP9_TAMPER_DETECTION_ANALYSIS_JAN06.md
// ========================================
//
// Immudb-based verification API for SOC2 compliance.
//
// WHY Immudb?
// - ✅ Built-in Merkle tree cryptographic verification
// - ✅ Automatic tamper detection on every read
// - ✅ Production-ready crypto (industry standard)
// - ✅ 70% time savings vs custom hash chain implementation
//
// This API provides auditor-facing verification capability and creates
// an audit trail of verification requests (SOC2 requirement).
//
// ⚠️ Enterprise Support Risk (85% confidence)
//    Mitigation: Fallback to custom PostgreSQL hash chain if needed (6 hours)
// ========================================

// VerifyChainRequest represents a request to verify audit event chain integrity
// POST /api/v1/audit/verify-chain
//
// SOC2 Requirement: Auditors must be able to verify tamper-evidence of audit logs
type VerifyChainRequest struct {
	CorrelationID string `json:"correlation_id" example:"rr-2025-001"`
}

// VerifyChainResponse represents the result of audit chain verification
//
// SOC2 Requirement: Verification results must include detailed status and metadata
type VerifyChainResponse struct {
	VerificationResult string                     `json:"verification_result"` // "valid" | "invalid"
	VerifiedAt         time.Time                  `json:"verified_at"`
	Details            *ChainVerificationDetails  `json:"details,omitempty"`
	Errors             []ChainVerificationError   `json:"errors,omitempty"`
}

// ChainVerificationDetails provides metadata about the verified chain
type ChainVerificationDetails struct {
	CorrelationID  string    `json:"correlation_id"`
	EventsVerified int       `json:"events_verified"`
	ChainStart     time.Time `json:"chain_start,omitempty"`
	ChainEnd       time.Time `json:"chain_end,omitempty"`
	FirstEventID   string    `json:"first_event_id,omitempty"`
	LastEventID    string    `json:"last_event_id,omitempty"`
}

// ChainVerificationError represents a verification failure
type ChainVerificationError struct {
	Code                  string     `json:"code"`
	Message               string     `json:"message"`
	TamperedEventID       string     `json:"tampered_event_id,omitempty"`
	TamperedEventTimestamp *time.Time `json:"tampered_event_timestamp,omitempty"`
}

// handleVerifyChain verifies the integrity of audit event chain using Immudb's cryptographic proofs
//
// SOC2 Gap #9: Tamper Detection API
// BR-AUDIT-005: Enterprise-Grade Audit Integrity
//
// This handler leverages Immudb's automatic Merkle tree verification.
// Every query to Immudb (via VerifiedGet/Scan) includes cryptographic proof validation.
// If data has been tampered with, Immudb will return an error, which we translate to
// a user-friendly verification failure response.
//
// Endpoint: POST /api/v1/audit/verify-chain
// Request Body: {"correlation_id": "rr-2025-001"}
// Response: 200 OK with verification status (valid/invalid)
//
// Example Valid Response:
//
//	{
//	  "verification_result": "valid",
//	  "verified_at": "2026-01-06T19:00:00Z",
//	  "details": {
//	    "correlation_id": "rr-2025-001",
//	    "events_verified": 42,
//	    "chain_start": "2026-01-01T10:00:00Z",
//	    "chain_end": "2026-01-06T18:00:00Z"
//	  }
//	}
//
// Example Invalid Response (Tampered):
//
//	{
//	  "verification_result": "invalid",
//	  "verified_at": "2026-01-06T19:00:00Z",
//	  "errors": [{
//	    "code": "IMMUDB_VERIFICATION_FAILED",
//	    "message": "Cryptographic verification failed - data may be tampered"
//	  }]
//	}
func (s *Server) handleVerifyChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Parse request
	var req VerifyChainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error(err, "Failed to parse verify chain request")
		s.writeErrorResponse(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Validate correlation_id
	if req.CorrelationID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "MISSING_CORRELATION_ID", "correlation_id is required")
		return
	}

	s.logger.V(1).Info("Verifying audit chain",
		"correlation_id", req.CorrelationID,
		"method", "Immudb Merkle tree verification")

	// 2. Query all events for correlation_id using Immudb
	//    Immudb automatically verifies cryptographic proofs during Scan/VerifiedGet
	//    If any data is tampered, Immudb will return an error
	//
	// NOTE: The Query method expects SQL strings for PostgreSQL compatibility.
	// For Immudb, the SQL is parsed to extract correlation_id for prefix scanning.
	querySQL := fmt.Sprintf("SELECT * FROM audit_events WHERE correlation_id = '%s' ORDER BY event_timestamp ASC LIMIT 10000", req.CorrelationID)
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM audit_events WHERE correlation_id = '%s'", req.CorrelationID)

	events, _, err := s.auditEventsRepo.Query(ctx, querySQL, countSQL, nil)

	if err != nil {
		// Immudb verification failed - this indicates tampered data
		s.logger.Error(err, "Immudb verification failed",
			"correlation_id", req.CorrelationID,
			"soc2_gap", "Gap #9 - Tamper Detection",
			"verification_result", "invalid")

		// Return invalid verification response
		response := &VerifyChainResponse{
			VerificationResult: "invalid",
			VerifiedAt:         time.Now().UTC(),
			Errors: []ChainVerificationError{
				{
					Code:    "IMMUDB_VERIFICATION_FAILED",
					Message: fmt.Sprintf("Cryptographic verification failed - data may be tampered: %v", err),
				},
			},
		}

		s.writeJSONResponse(w, http.StatusOK, response) // 200 OK with invalid result
		return
	}

	// 3. Handle empty results
	if len(events) == 0 {
		s.logger.V(1).Info("No events found for correlation_id",
			"correlation_id", req.CorrelationID)

		response := &VerifyChainResponse{
			VerificationResult: "valid",
			VerifiedAt:         time.Now().UTC(),
			Details: &ChainVerificationDetails{
				CorrelationID:  req.CorrelationID,
				EventsVerified: 0,
			},
		}

		s.writeJSONResponse(w, http.StatusOK, response)
		return
	}

	// 4. If we reach here, all events passed Immudb's Merkle tree verification
	//    Build success response with chain metadata
	firstEvent := events[0]
	lastEvent := events[len(events)-1]

	response := &VerifyChainResponse{
		VerificationResult: "valid",
		VerifiedAt:         time.Now().UTC(),
		Details: &ChainVerificationDetails{
			CorrelationID:  req.CorrelationID,
			EventsVerified: len(events),
			ChainStart:     firstEvent.EventTimestamp,
			ChainEnd:       lastEvent.EventTimestamp,
			FirstEventID:   firstEvent.EventID.String(),
			LastEventID:    lastEvent.EventID.String(),
		},
	}

	s.logger.Info("Audit chain verification successful",
		"correlation_id", req.CorrelationID,
		"events_verified", len(events),
		"verification_method", "Immudb Merkle tree",
		"soc2_gap", "Gap #9 - Tamper Detection",
		"verification_result", "valid")

	s.writeJSONResponse(w, http.StatusOK, response)
}

// writeJSONResponse is a helper to write JSON responses with proper headers
func (s *Server) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error(err, "Failed to encode JSON response")
	}
}

// writeErrorResponse is a helper to write RFC 7807 error responses
func (s *Server) writeErrorResponse(w http.ResponseWriter, statusCode int, errorCode string, message string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	response := map[string]interface{}{
		"type":   fmt.Sprintf("https://datastorage.svc/errors/%s", errorCode),
		"title":  errorCode,
		"status": statusCode,
		"detail": message,
		"instance": uuid.New().String(),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to encode error response")
	}
}

