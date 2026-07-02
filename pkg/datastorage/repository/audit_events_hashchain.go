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

package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// ========================================
// GAP #9: HASH CHAIN IMPLEMENTATION (Tamper-Evidence)
// Authority: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md - Day 7
// SOC2 Requirement: Tamper-evident audit logs (SOC 2 Type II, NIST 800-53, Sarbanes-Oxley)
// ========================================
// Split from audit_events_repository.go (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 3, pure code motion, no behavior change).

// GAP-05 (Issue #1505): Hash chain algorithm identifiers, stored per-row in the
// hash_algorithm column so a mixed-algorithm chain is supported during HMAC key
// rollout (existing rows keep sha256-unkeyed; new writes use hmac-sha256 once a
// key is configured).
const (
	// HashAlgorithmSHA256Unkeyed is the legacy algorithm used before GAP-05: a
	// plain SHA256 of (previous_hash + event_json), computable by anyone without
	// a secret. A DB-privileged attacker can recompute this hash after tampering.
	HashAlgorithmSHA256Unkeyed = "sha256-unkeyed"

	// HashAlgorithmHMACSHA256 is the GAP-05 keyed algorithm: HMAC-SHA256 of
	// (previous_hash + event_json) using a key stored outside the database
	// (Kubernetes Secret). Forging a valid hash without the key is infeasible.
	HashAlgorithmHMACSHA256 = "hmac-sha256"
)

// PrepareEventForHashing returns a copy of the event with excluded fields zeroed out.
// This MUST be used by all hash calculation paths (write-time, export, verify-chain)
// to ensure consistent hashing across the entire audit pipeline.
//
// Excluded fields:
//  1. EventHash, PreviousEventHash — not yet calculated at write time
//  2. EventDate — derived from EventTimestamp (DB-generated)
//  3. LegalHold, LegalHoldReason, LegalHoldPlacedBy, LegalHoldPlacedAt — can change
//     after event creation (SOC2 Gap #8)
//  4. HashAlgorithm — GAP-05 metadata describing which algorithm produced EventHash,
//     not part of the hashed content. Excluding it keeps pre-GAP-05 hashes verifiable
//     (their original JSON payload never contained this field).
//
// Note: EventTimestamp IS included in hash (set before calculation during INSERT).
func PrepareEventForHashing(event *AuditEvent) AuditEvent {
	eventCopy := *event
	eventCopy.EventHash = ""
	eventCopy.PreviousEventHash = ""
	eventCopy.EventDate = DateOnly{}
	eventCopy.HashAlgorithm = ""

	// SOC2 Gap #8: Legal hold fields can change after event creation
	eventCopy.LegalHold = false
	eventCopy.LegalHoldReason = ""
	eventCopy.LegalHoldPlacedBy = ""
	eventCopy.LegalHoldPlacedAt = nil

	return eventCopy
}

// calculateEventHash computes SHA256 hash for blockchain-style chain
// Hash = SHA256(previous_event_hash + event_json)
// This creates an immutable chain where tampering with ANY event breaks the chain
//
// GAP-05 (Issue #1505): kept for the legacy HashAlgorithmSHA256Unkeyed algorithm.
// New writes prefer calculateEventHashHMAC when a key is configured (see
// AuditEventsRepository.hashEvent).
func calculateEventHash(previousHash string, event *AuditEvent) (string, error) {
	eventForHashing := PrepareEventForHashing(event)

	// Serialize event to JSON (canonical form for consistent hashing)
	eventJSON, err := json.Marshal(eventForHashing)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event for hashing: %w", err)
	}

	// Compute hash: SHA256(previous_hash + event_json)
	hasher := sha256.New()
	hasher.Write([]byte(previousHash))
	hasher.Write(eventJSON)
	hashBytes := hasher.Sum(nil)

	return hex.EncodeToString(hashBytes), nil
}

// calculateEventHashHMAC computes a keyed HMAC-SHA256 hash chain link.
// Hash = HMAC-SHA256(key, previous_event_hash + event_json)
//
// GAP-05 (Issue #1505): unlike calculateEventHash (plain SHA256), forging a
// valid hash without the key is computationally infeasible even for an
// attacker with full read/write access to the database — closing the gap
// where a DB-privileged attacker could tamper with an event and recompute a
// self-consistent unkeyed SHA256 chain.
func calculateEventHashHMAC(key []byte, previousHash string, event *AuditEvent) (string, error) {
	eventForHashing := PrepareEventForHashing(event)

	eventJSON, err := json.Marshal(eventForHashing)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event for hashing: %w", err)
	}

	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(previousHash))
	mac.Write(eventJSON)
	hashBytes := mac.Sum(nil)

	return hex.EncodeToString(hashBytes), nil
}

// hashEvent computes the hash chain link for event using this repository's
// configured algorithm (HMAC-SHA256 when an HMAC key is set, else the legacy
// unkeyed SHA256) and stamps event.HashAlgorithm accordingly. Used by both
// Create and CreateBatch so the algorithm decision lives in exactly one place.
func (r *AuditEventsRepository) hashEvent(previousHash string, event *AuditEvent) (string, error) {
	if len(r.hmacKey) > 0 {
		event.HashAlgorithm = HashAlgorithmHMACSHA256
		return calculateEventHashHMAC(r.hmacKey, previousHash, event)
	}
	event.HashAlgorithm = HashAlgorithmSHA256Unkeyed
	return calculateEventHash(previousHash, event)
}

// CalculateHashForVerification computes the expected hash for verification,
// honoring the event's own HashAlgorithm (set at write time). GAP-05 (Issue
// #1505): supports both the legacy unkeyed SHA256 algorithm and the newer
// keyed HMAC-SHA256 algorithm within the same chain, since an HMAC key
// rollout does not retroactively re-hash pre-existing events.
//
// hmacKey may be nil when the caller has no HMAC key configured. Verifying a
// hmac-sha256 event without a key returns an error rather than silently
// falling back to the unkeyed formula, which would always mismatch and
// misleadingly report "tampered" for events that are actually intact.
// Shared by repository.Export and the /api/v1/audit/verify-chain handler so
// both verification paths apply identical logic.
func CalculateHashForVerification(hmacKey []byte, previousHash string, event *AuditEvent) (string, error) {
	switch event.HashAlgorithm {
	case HashAlgorithmHMACSHA256:
		if len(hmacKey) == 0 {
			return "", fmt.Errorf("event %s uses hash_algorithm=%s but no HMAC key is configured for verification", event.EventID, HashAlgorithmHMACSHA256)
		}
		return calculateEventHashHMAC(hmacKey, previousHash, event)
	case HashAlgorithmSHA256Unkeyed, "":
		// Empty HashAlgorithm covers rows read before the hash_algorithm column
		// existed (pre-GAP-05 code paths / tests using bare AuditEvent structs).
		return calculateEventHash(previousHash, event)
	default:
		return "", fmt.Errorf("event %s has unrecognized hash_algorithm %q", event.EventID, event.HashAlgorithm)
	}
}

// getPreviousEventHash retrieves the hash of the most recent event for a given correlation_id
// Returns empty string if no previous event exists (first event in chain)
// Uses advisory lock to prevent race conditions during concurrent inserts
func (r *AuditEventsRepository) getPreviousEventHash(ctx context.Context, tx *sql.Tx, correlationID string) (string, error) {
	// Step 1: Acquire advisory lock for this correlation_id (prevents race conditions)
	// Uses PostgreSQL function audit_event_lock_id() from migration 023
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(audit_event_lock_id($1))", correlationID)
	if err != nil {
		return "", fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	// Step 2: Query last event hash for this correlation_id
	var previousHash sql.NullString
	query := `
		SELECT event_hash
		FROM audit_events
		WHERE correlation_id = $1
		  AND event_hash IS NOT NULL
		ORDER BY event_timestamp DESC, event_id DESC
		LIMIT 1
	`

	err = tx.QueryRowContext(ctx, query, correlationID).Scan(&previousHash)
	if errors.Is(err, sql.ErrNoRows) {
		// First event in chain - no previous hash (return empty string)
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query previous event hash: %w", err)
	}

	return previousHash.String, nil
}

// ========================================
// END GAP #9 HASH CHAIN FUNCTIONS
// ========================================
