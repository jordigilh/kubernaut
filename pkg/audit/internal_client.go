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
	"github.com/jordigilh/kubernaut/pkg/datastorage/eventdata"
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

// InternalAuditClientConfig holds configurable parameters for InternalAuditClient.
// DF-H2: RetentionDays is configurable instead of hardcoded.
type InternalAuditClientConfig struct {
	RetentionDays int // Per-event retention (default: 2555 = 7 years, SOC 2 / ISO 27001)
}

// DefaultInternalAuditClientConfig returns production defaults.
func DefaultInternalAuditClientConfig() InternalAuditClientConfig {
	return InternalAuditClientConfig{
		RetentionDays: 2555,
	}
}

// InternalAuditClient writes audit events directly to PostgreSQL
// Used by Data Storage Service to avoid circular dependency (cannot call its own REST API)
//
// BR-STORAGE-013: Must use direct PostgreSQL writes (not REST API)
type InternalAuditClient struct {
	db     *sql.DB
	config InternalAuditClientConfig
}

// NewInternalAuditClient creates a new internal audit client with default config.
func NewInternalAuditClient(db *sql.DB) DataStorageClient {
	return &InternalAuditClient{
		db:     db,
		config: DefaultInternalAuditClientConfig(),
	}
}

// NewInternalAuditClientWithConfig creates an internal audit client with explicit config.
// DF-H2: Allows configurable RetentionDays instead of hardcoded 90.
func NewInternalAuditClientWithConfig(db *sql.DB, config InternalAuditClientConfig) DataStorageClient {
	if config.RetentionDays <= 0 {
		config.RetentionDays = DefaultInternalAuditClientConfig().RetentionDays
	}
	return &InternalAuditClient{
		db:     db,
		config: config,
	}
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
		if err := c.insertAuditEvent(ctx, stmt, event); err != nil {
			return err
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// insertAuditEvent marshals, validates, and executes a single audit event
// insert against the prepared statement within StoreBatch's transaction.
func (c *InternalAuditClient) insertAuditEvent(ctx context.Context, stmt *sql.Stmt, event *ogenclient.AuditEventRequest) error {
	eventID := uuid.New().String()
	eventDate := event.EventTimestamp.Truncate(24 * time.Hour)

	eventDataJSON, err := json.Marshal(event.EventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event_data: %w", err)
	}

	// DF-M1: Validate EventData before insert (defense-in-depth)
	if err := eventdata.ValidateEventData(eventDataJSON); err != nil {
		return fmt.Errorf("event_data validation failed: %w", err)
	}

	const defaultIsSensitive = false

	_, err = stmt.ExecContext(ctx,
		eventID,
		event.Version,
		event.EventTimestamp,
		eventDate,
		event.EventType,
		event.EventCategory,
		event.EventAction,
		string(event.EventOutcome),
		optionalStringValue(event.ActorType),
		optionalStringValue(event.ActorID),
		optionalStringValue(event.ResourceType),
		optionalStringValue(event.ResourceID),
		event.CorrelationID,
		eventDataJSON,
		c.config.RetentionDays, // DF-H2: configurable, not hardcoded
		defaultIsSensitive,
	)
	if err != nil {
		return fmt.Errorf("failed to insert audit event: %w", err)
	}
	return nil
}

// optionalStringValue extracts the underlying value of an ogen OptString
// field, returning "" when unset.
func optionalStringValue(opt ogenclient.OptString) string {
	if !opt.IsSet() {
		return ""
	}
	return opt.Value
}
