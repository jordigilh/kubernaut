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
	"encoding/json"
	"fmt"
	"time"

	immuschema "github.com/codenotary/immudb/pkg/api/schema"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

// ========================================
// IMMUDB AUDIT EVENTS REPOSITORY
// 📋 SOC2 Gap #9: Tamper-Evident Audit Trail with Blockchain-Style Hash Chain
// Authority: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md - Day 7
// Phase: 5.1 - Minimal Repository (Create method only)
// ========================================
//
// This repository implements immutable, tamper-evident audit event storage using Immudb.
//
// KEY BENEFITS:
// - ✅ Automatic hash chain (Merkle tree) - NO custom hash logic needed
// - ✅ Cryptographic proof on every read (VerifiedGet)
// - ✅ Tamper detection built-in (any modification breaks chain)
// - ✅ Monotonic transaction IDs (perfect for audit trails)
//
// DESIGN DECISION: Key-Value API (not SQL)
// - Simpler implementation
// - Automatic hash chain maintenance
// - Sufficient for SOC2 Gap #9 requirements
//
// KEY FORMAT (Phase 5.1):
// - Simple: `audit_event:{event_id}`
// - Future (Phase 5.3): `audit_event:corr-{correlation_id}:{event_id}` (for prefix queries)
//
// SOC2 COMPLIANCE:
// - Immutable audit trail (write-once, never modified)
// - Cryptographic proof of integrity
// - Tamper-evident chain (Merkle tree)
// - Monotonic transaction IDs for ordering
//
// ========================================

// ImmudbClient defines the minimal Immudb client interface needed for audit storage
// This is a subset of github.com/codenotary/immudb/pkg/client.ImmuClient
// Only includes methods actually used by this repository and server
type ImmudbClient interface {
	// VerifiedSet inserts a key-value pair with cryptographic proof
	VerifiedSet(ctx context.Context, key []byte, value []byte) (*immuschema.TxHeader, error)

	// CurrentState returns the current database state (used for health checks)
	CurrentState(ctx context.Context) (*immuschema.ImmutableState, error)

	// HealthCheck verifies Immudb connectivity (Phase 5.2)
	HealthCheck(ctx context.Context) error

	// CloseSession closes the Immudb session (Phase 5.2)
	CloseSession(ctx context.Context) error

	// Login authenticates with Immudb (Phase 5.2)
	Login(ctx context.Context, user []byte, password []byte) (*immuschema.LoginResponse, error)

	// Future methods (Phase 5.3):
	// VerifiedGet(ctx, key) - For audit event reads
	// Scan(ctx, prefix) - For correlation_id queries
}

// ImmudbAuditEventsRepository handles Immudb operations for audit_events
// This provides tamper-evident, cryptographically-verified audit storage
type ImmudbAuditEventsRepository struct {
	client ImmudbClient
	logger logr.Logger
}

// NewImmudbAuditEventsRepository creates a new Immudb audit repository
// Connection must be established and authenticated before calling this
func NewImmudbAuditEventsRepository(client ImmudbClient, logger logr.Logger) *ImmudbAuditEventsRepository {
	return &ImmudbAuditEventsRepository{
		client: client,
		logger: logger,
	}
}

// Create inserts a new audit event into Immudb with automatic hash chain
// Returns the created event with event_id and timestamp populated
//
// IMMUDB BENEFITS:
// - Automatic hash chain (Merkle tree maintained by Immudb)
// - Cryptographic proof on every write (VerifiedSet)
// - Tamper detection built-in (any modification breaks chain)
//
// KEY FORMAT: audit_event:{event_id}
// FUTURE: Will use audit_event:corr-{correlation_id}:{event_id} for efficient queries
func (r *ImmudbAuditEventsRepository) Create(ctx context.Context, event *AuditEvent) (*AuditEvent, error) {
	// Generate UUID if not provided
	if event.EventID == uuid.Nil {
		event.EventID = uuid.New()
	}

	// Set event_timestamp if not provided
	if event.EventTimestamp.IsZero() {
		event.EventTimestamp = time.Now().UTC()
	}

	// Set event_date from event_timestamp (for consistency with PostgreSQL)
	event.EventDate = DateOnly(time.Date(
		event.EventTimestamp.Year(),
		event.EventTimestamp.Month(),
		event.EventTimestamp.Day(),
		0, 0, 0, 0, time.UTC,
	))

	// Set default version if not specified (ADR-034: current version is "1.0")
	if event.Version == "" {
		event.Version = "1.0"
	}

	// Set default retention days if not specified (ADR-034: 7 years = 2555 days)
	if event.RetentionDays == 0 {
		event.RetentionDays = 2555
	}

	// Serialize entire event to JSON
	// Immudb stores this as immutable bytes with automatic hash chain
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// Create Immudb key: audit_event:{event_id}
	// Phase 5.1: Simple key format
	// Phase 5.3: Will add correlation_id prefix for efficient queries
	key := []byte(fmt.Sprintf("audit_event:%s", event.EventID.String()))

	// VerifiedSet provides:
	// - Automatic hash chain (Merkle tree)
	// - Cryptographic proof of write
	// - Tamper detection (any modification breaks chain)
	// - Monotonic transaction ID
	tx, err := r.client.VerifiedSet(ctx, key, eventJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to insert audit event into Immudb: %w", err)
	}

	r.logger.V(1).Info("Audit event created in Immudb",
		"event_id", event.EventID.String(),
		"tx_id", tx.Id, // Monotonic transaction ID (perfect for audit trails)
		"correlation_id", event.CorrelationID,
		"event_type", event.EventType,
		"event_category", event.EventCategory,
	)

	return event, nil
}

// HealthCheck verifies Immudb connectivity
func (r *ImmudbAuditEventsRepository) HealthCheck(ctx context.Context) error {
	// Immudb health check: attempt to get database info
	// This validates connection and authentication
	_, err := r.client.CurrentState(ctx)
	if err != nil {
		return fmt.Errorf("Immudb health check failed: %w", err)
	}
	return nil
}

// ========================================
// PHASE 5.3 STUBS: For Compilation Only
// ========================================

// Query retrieves audit events by SQL query (Phase 5.3)
// Stub implementation for compilation - will be fully implemented in Phase 5.3
// Note: Immudb doesn't use SQL, so this will be refactored to use Scan/prefix queries
func (r *ImmudbAuditEventsRepository) Query(ctx context.Context, querySQL string, countSQL string, args []interface{}) ([]*AuditEvent, *PaginationMetadata, error) {
	r.logger.Info("Query called (Phase 5.3 stub - not implemented yet)", "querySQL", querySQL)
	// TODO Phase 5.3: Implement Immudb Scan with prefix queries (no SQL)
	return []*AuditEvent{}, &PaginationMetadata{}, fmt.Errorf("Query not implemented yet (Phase 5.3)")
}

// CreateBatch inserts multiple audit events in a single transaction (Phase 5.3)
// Stub implementation for compilation - will be fully implemented in Phase 5.3
func (r *ImmudbAuditEventsRepository) CreateBatch(ctx context.Context, events []*AuditEvent) ([]*AuditEvent, error) {
	r.logger.Info("CreateBatch called (Phase 5.3 stub - not implemented yet)", "event_count", len(events))
	// TODO Phase 5.3: Implement Immudb batch writes
	return nil, fmt.Errorf("CreateBatch not implemented yet (Phase 5.3)")
}

// ========================================
// PHASE 5.2-5.4: Future Methods (NOT IMPLEMENTED YET)
// ========================================
//
// These will be implemented in subsequent phases:
//
// Phase 5.2: Test with DataStorage integration ✅ (in progress)
// Phase 5.3: Implement Query() and CreateBatch() (stubs above)
// Phase 5.4: Full integration with all 7 services
//
// ========================================

