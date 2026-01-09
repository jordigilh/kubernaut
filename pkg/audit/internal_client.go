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

package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ========================================
// INTERNAL AUDIT CLIENT (DD-STORAGE-012)
// 📋 Design Decision: DD-STORAGE-012 | BR-STORAGE-013
// Authority: DD-STORAGE-012-AUDIT-INTEGRATION-PLAN.md
// ========================================
//
// InternalAuditClient writes audit events directly to PostgreSQL,
// bypassing the REST API to avoid circular dependency.
//
// WHY DD-STORAGE-012?
// - ✅ Avoids circular dependency (Data Storage cannot call itself)
// - ✅ Direct PostgreSQL writes (no HTTP overhead)
// - ✅ Transaction safety (batch inserts in single transaction)
//
// Business Requirements:
// - BR-STORAGE-012: Data Storage Service must generate audit traces
// - BR-STORAGE-013: Audit traces must not create circular dependencies
// - BR-STORAGE-014: Audit writes must not block business operations
//
// ========================================

// InternalAuditClient writes audit events directly to PostgreSQL
// Used by Data Storage Service to avoid circular dependency (cannot call its own REST API)
//
// BR-STORAGE-013: Must use direct PostgreSQL writes (not REST API)
type InternalAuditClient struct {
	db *sql.DB
}

// NewInternalAuditClient creates a new internal audit client
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - DataStorageClient: Client for writing audit events
//
// Usage:
//
//	internalClient := audit.NewInternalAuditClient(db)
//	auditStore := audit.NewBufferedStore(internalClient, audit.DefaultConfig(), logger)
func NewInternalAuditClient(db *sql.DB) DataStorageClient {
	return &InternalAuditClient{db: db}
}

// StoreBatch writes audit events directly to PostgreSQL (bypasses REST API)
//
// BR-STORAGE-013: Direct PostgreSQL writes avoid circular dependency
// BR-STORAGE-014: Batch inserts minimize performance impact
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - events: Audit events to write
//
// Returns:
//   - error: Write error (nil on success)
//
// Behavior:
//   - Empty batch: Returns immediately (no database call)
//   - Single transaction: All events inserted in one transaction
//   - Rollback on error: Transaction rolled back if any insert fails
//   - Context cancellation: Respects context cancellation
func (c *InternalAuditClient) StoreBatch(ctx context.Context, events []*ogenclient.AuditEventRequest) error {
	if len(events) == 0 {
		return nil
	}

	// Begin transaction
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Rollback if commit not reached

	// Prepare INSERT statement (batch insert for performance)
	// Uses ADR-034 schema column names
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO audit_events (
			event_id, event_version, event_timestamp, event_date, event_type,
			event_category, event_action, event_outcome,
			actor_type, actor_id, resource_type, resource_id,
			correlation_id, event_data, retention_days, is_sensitive
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	// Insert each event
	for _, event := range events {
		// Generate event_id
		eventID := uuid.New().String()

		// Calculate event_date from event_timestamp for partitioning
		eventDate := event.EventTimestamp.Truncate(24 * time.Hour)

		// Marshal event_data to JSON
		eventDataJSON, err := json.Marshal(event.EventData)
		if err != nil {
			return fmt.Errorf("failed to marshal event_data: %w", err)
		}

		// Extract optional fields (ogen uses OptString, not *string)
		actorType := ""
		if event.ActorType.IsSet() {
			actorType = event.ActorType.Value
		}
		actorID := ""
		if event.ActorID.IsSet() {
			actorID = event.ActorID.Value
		}
		resourceType := ""
		if event.ResourceType.IsSet() {
			resourceType = event.ResourceType.Value
		}
		resourceID := ""
		if event.ResourceID.IsSet() {
			resourceID = event.ResourceID.Value
		}

		// DD-AUDIT-002 V2.0: Hardcoded defaults (OpenAPI spec doesn't have these fields)
		// Option C (Hybrid): InternalAuditClient only, no spec/schema changes needed
		const defaultRetentionDays = 90
		const defaultIsSensitive = false

		_, err = stmt.ExecContext(ctx,
			eventID,                    // event_id (generated UUID)
			event.Version,              // version (OpenAPI field)
			event.EventTimestamp,       // event_timestamp
			eventDate,                  // event_date for partitioning
			event.EventType,            // event_type
			event.EventCategory,        // event_category (ADR-034)
			event.EventAction,          // event_action (ADR-034)
			string(event.EventOutcome), // event_outcome (ADR-034, enum)
			actorType,                  // actor_type (optional)
			actorID,                    // actor_id (optional)
			resourceType,               // resource_type (optional)
			resourceID,                 // resource_id (optional)
			event.CorrelationID,        // correlation_id (OpenAPI field)
			eventDataJSON,              // event_data (JSONB)
			defaultRetentionDays,       // retention_days (hardcoded default)
			defaultIsSensitive,         // is_sensitive (hardcoded default)
		)
		if err != nil {
			return fmt.Errorf("failed to insert audit event: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
