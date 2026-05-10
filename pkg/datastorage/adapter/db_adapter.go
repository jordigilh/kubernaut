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

package adapter

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// DBAdapter adapts sql.DB to work with our Handler
// Day 3: Real database implementation using query builder
type DBAdapter struct {
	db     *sql.DB
	logger logr.Logger
}

// NewDBAdapter creates a new database adapter
func NewDBAdapter(db *sql.DB, logger logr.Logger) *DBAdapter {
	return &DBAdapter{
		db:     db,
		logger: logger,
	}
}

// Get retrieves a single incident by ID
// BR-STORAGE-021: Get incident by ID
// Get retrieves a single audit event by event_id
// V1.0: Returns structured type (*repository.AuditEvent) for type safety
func (d *DBAdapter) Get(id int) (*repository.AuditEvent, error) {
	d.logger.V(1).Info("DBAdapter.Get called",
		"id", id,
	)

	// V1.0: Query unified audit_events table (not legacy resource_action_traces)
	// ADR-034: Unified audit table schema
	sqlQuery := `
		SELECT
			event_id, event_version, event_timestamp, event_date, event_type,
			event_category, event_action, event_outcome,
			correlation_id, parent_event_id, parent_event_date,
			resource_type, resource_id, namespace, cluster_name,
			actor_id, actor_type,
			severity, duration_ms, error_code, error_message,
			retention_days, is_sensitive, event_data
		FROM audit_events
		WHERE id = $1
		LIMIT 1
	`

	row := d.db.QueryRow(sqlQuery, id)

	event := &repository.AuditEvent{}

	// V1.0: Direct struct scanning (type-safe, faster than map conversion)
	var (
		parentEventID   sql.NullString // UUID stored as string, convert to *uuid.UUID
		parentEventDate sql.NullTime
		namespace       sql.NullString
		clusterName     sql.NullString
		severity        sql.NullString
		durationMs      sql.NullInt32
		errorCode       sql.NullString
		errorMessage    sql.NullString
		eventDataJSON   []byte // JSONB stored as bytes
	)

	err := row.Scan(
		&event.EventID,
		&event.Version,
		&event.EventTimestamp,
		&event.EventDate,
		&event.EventType,
		&event.EventCategory,
		&event.EventAction,
		&event.EventOutcome,
		&event.CorrelationID,
		&parentEventID,
		&parentEventDate,
		&event.ResourceType,
		&event.ResourceID,
		&namespace,
		&clusterName,
		&event.ActorID,
		&event.ActorType,
		&severity,
		&durationMs,
		&errorCode,
		&errorMessage,
		&event.RetentionDays,
		&event.IsSensitive,
		&eventDataJSON,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			d.logger.V(1).Info("No audit event found with ID",
				"id", id,
			)
			return nil, nil // Not found
		}
		d.logger.Error(err, "Failed to scan audit event",
			"id", id,
		)
		return nil, fmt.Errorf("row scan error: %w", err)
	}

	// Convert sql.Null* types to Go types
	if parentEventID.Valid {
		parentUUID, err := uuid.Parse(parentEventID.String)
		if err == nil {
			event.ParentEventID = &parentUUID
		}
	}
	if parentEventDate.Valid {
		event.ParentEventDate = &parentEventDate.Time
	}
	if namespace.Valid {
		event.ResourceNamespace = namespace.String
	}
	if clusterName.Valid {
		event.ClusterID = clusterName.String
	}
	if severity.Valid {
		event.Severity = severity.String
	}
	if durationMs.Valid {
		event.DurationMs = int(durationMs.Int32)
	}
	if errorCode.Valid {
		event.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		event.ErrorMessage = errorMessage.String
	}

	// Unmarshal event_data JSONB
	if len(eventDataJSON) > 0 {
		if err := json.Unmarshal(eventDataJSON, &event.EventData); err != nil {
		d.logger.Error(err, "Failed to unmarshal event_data",
			"event_id", event.EventID,
		)
		// Continue with nil EventData rather than failing
		event.EventData = nil
	}
} else {
	event.EventData = nil
}

	d.logger.Info("Audit event retrieved successfully",
		"id", id,
		"event_id", event.EventID,
	)

	return event, nil
}
