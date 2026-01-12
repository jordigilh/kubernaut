package reconstruction

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// QueryAuditEventsForReconstruction retrieves all audit events needed for RR reconstruction.
//
// BR-AUDIT-005 v2.0: RR Reconstruction Support
// This function queries audit events by correlation ID and filters for reconstruction-relevant event types.
//
// Event Types Retrieved:
//   - gateway.signal.received: Captures RR.Spec fields (OriginalPayload, SignalLabels, SignalAnnotations)
//   - aianalysis.analysis.completed: Captures Provider data
//   - workflowexecution.selection.completed: Captures Workflow selection
//   - workflowexecution.execution.started: Captures Execution ref
//   - orchestrator.lifecycle.created: Captures TimeoutConfig
//
// Events are ordered by timestamp (oldest first) for chronological reconstruction.
func QueryAuditEventsForReconstruction(
	ctx context.Context,
	db *sql.DB,
	logger logr.Logger,
	correlationID string,
) ([]ogenclient.AuditEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	// Query all reconstruction-relevant audit events for this correlation ID
	// Per ADR-034: Use unified audit_events table
	// Order by timestamp ASC for chronological reconstruction
	query := `
		SELECT
			event_id, event_version, event_type, event_category, event_action,
			correlation_id, event_timestamp, event_outcome, severity,
			resource_type, resource_id, actor_type, actor_id, parent_event_id,
			event_data, event_date, namespace, cluster_name,
			duration_ms, error_code, error_message
		FROM audit_events
		WHERE correlation_id = $1
		  AND event_type IN (
			  'gateway.signal.received',
			  'aianalysis.analysis.completed',
			  'workflowexecution.selection.completed',
			  'workflowexecution.execution.started',
			  'orchestrator.lifecycle.created'
		  )
		ORDER BY event_timestamp ASC, event_id ASC
	`

	rows, err := db.QueryContext(ctx, query, correlationID)
	if err != nil {
		logger.Error(err, "Failed to query audit events for RR reconstruction",
			"correlationID", correlationID)
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	var events []ogenclient.AuditEvent
	for rows.Next() {
		var event ogenclient.AuditEvent
		var eventDataJSON []byte

		// Scan into intermediate sql.Null* variables for nullable fields
		var (
			eventID       sql.NullString // UUID as string
			parentEventID sql.NullString // UUID as string
			namespace     sql.NullString
			clusterName   sql.NullString
			severity      sql.NullString
			durationMs    sql.NullInt32
			errorCode     sql.NullString
			errorMessage  sql.NullString
			eventDate     sql.NullTime
		)

		// Scan row from database
		err := rows.Scan(
			&eventID,
			&event.Version,
			&event.EventType,
			&event.EventCategory,
			&event.EventAction,
			&event.CorrelationID,
			&event.EventTimestamp,
			&event.EventOutcome,
			&severity,
			&event.ResourceType,
			&event.ResourceID,
			&event.ActorType,
			&event.ActorID,
			&parentEventID,
			&eventDataJSON,
			&eventDate,
			&namespace,
			&clusterName,
			&durationMs,
			&errorCode,
			&errorMessage,
		)
		if err != nil {
			logger.Error(err, "Failed to scan audit event row",
				"correlationID", correlationID)
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		// Convert sql.Null* types to ogen Opt* types
		if eventID.Valid {
			parsedUUID, err := uuid.Parse(eventID.String)
			if err == nil {
				event.EventID.SetTo(parsedUUID)
			}
		}
		if parentEventID.Valid {
			parsedParentUUID, err := uuid.Parse(parentEventID.String)
			if err == nil {
				event.ParentEventID.SetTo(parsedParentUUID)
			}
		}
		if namespace.Valid {
			event.Namespace.SetTo(namespace.String)
		}
		if clusterName.Valid {
			event.ClusterName.SetTo(clusterName.String)
		}
		if severity.Valid {
			event.Severity.SetTo(severity.String)
		}
		if durationMs.Valid {
			event.DurationMs.SetTo(int(durationMs.Int32))
		}
		if eventDate.Valid {
			event.EventDate.SetTo(eventDate.Time)
		}

		// Unmarshal event_data JSONB to ogen-generated union type
		if len(eventDataJSON) > 0 {
			if err := event.EventData.UnmarshalJSON(eventDataJSON); err != nil {
				logger.Error(err, "Failed to unmarshal event_data",
					"correlationID", correlationID,
					"eventType", event.EventType,
					"eventID", eventID)
				return nil, fmt.Errorf("failed to unmarshal event_data: %w", err)
			}
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		logger.Error(err, "Error iterating audit event rows",
			"correlationID", correlationID)
		return nil, fmt.Errorf("failed to iterate audit events: %w", err)
	}

	logger.V(1).Info("Successfully queried audit events for RR reconstruction",
		"correlationID", correlationID,
		"eventCount", len(events))

	return events, nil
}

// IsReconstructionRelevant checks if an event type is needed for RR reconstruction.
//
// This helper function validates event types before processing them for reconstruction.
func IsReconstructionRelevant(eventType string) bool {
	relevantTypes := map[string]bool{
		"gateway.signal.received":                   true, // Gap #1, #3, #4
		"aianalysis.analysis.completed":             true, // Gap #2
		"workflowexecution.selection.completed":     true, // Gap #5
		"workflowexecution.execution.started":       true, // Gap #6
		"orchestrator.lifecycle.created":            true, // Gap #8
	}
	return relevantTypes[eventType]
}
