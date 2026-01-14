package reconstruction

import (
	"context"
	"database/sql"
	"encoding/json"
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
			resourceType  sql.NullString
			resourceID    sql.NullString
			actorType     sql.NullString
			actorID       sql.NullString
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
			&resourceType,
			&resourceID,
			&actorType,
			&actorID,
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
		if resourceType.Valid {
			event.ResourceType.SetTo(resourceType.String)
		}
		if resourceID.Valid {
			event.ResourceID.SetTo(resourceID.String)
		}
		if actorType.Valid {
			event.ActorType.SetTo(actorType.String)
		}
		if actorID.Valid {
			event.ActorID.SetTo(actorID.String)
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

		// Manually construct discriminated union variant based on event_type
		// The raw JSON in database doesn't have discriminator, so we determine variant from event_type
		if len(eventDataJSON) > 0 {
			switch event.EventType {
			case "gateway.signal.received":
				var payload ogenclient.GatewayAuditPayload
				if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
					logger.Error(err, "Failed to unmarshal gateway event_data",
						"correlationID", correlationID,
						"eventID", eventID)
					return nil, fmt.Errorf("failed to unmarshal gateway event_data: %w", err)
				}
				event.EventData.SetGatewayAuditPayload(
					ogenclient.AuditEventEventDataGatewaySignalReceivedAuditEventEventData,
					payload,
				)

		case "orchestrator.lifecycle.created":
			var payload ogenclient.RemediationOrchestratorAuditPayload
			if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
				logger.Error(err, "Failed to unmarshal orchestrator event_data",
					"correlationID", correlationID,
					"eventID", eventID)
				return nil, fmt.Errorf("failed to unmarshal orchestrator event_data: %w", err)
			}
			event.EventData.SetRemediationOrchestratorAuditPayload(
				ogenclient.AuditEventEventDataOrchestratorLifecycleCreatedAuditEventEventData,
				payload,
			)

		case "aianalysis.analysis.completed":
			var payload ogenclient.AIAnalysisAuditPayload
			if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
				logger.Error(err, "Failed to unmarshal aianalysis event_data",
					"correlationID", correlationID,
					"eventID", eventID)
				return nil, fmt.Errorf("failed to unmarshal aianalysis event_data: %w", err)
			}
			event.EventData.SetAIAnalysisAuditPayload(
				ogenclient.AuditEventEventDataAianalysisAnalysisCompletedAuditEventEventData,
				payload,
			)

		case "workflowexecution.selection.completed":
			var payload ogenclient.WorkflowExecutionAuditPayload
			if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
				logger.Error(err, "Failed to unmarshal workflowexecution selection event_data",
					"correlationID", correlationID,
					"eventID", eventID)
				return nil, fmt.Errorf("failed to unmarshal workflowexecution selection event_data: %w", err)
			}
			event.EventData.SetWorkflowExecutionAuditPayload(
				ogenclient.AuditEventEventDataWorkflowexecutionSelectionCompletedAuditEventEventData,
				payload,
			)

		case "workflowexecution.execution.started":
			var payload ogenclient.WorkflowExecutionAuditPayload
			if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
				logger.Error(err, "Failed to unmarshal workflowexecution execution event_data",
					"correlationID", correlationID,
					"eventID", eventID)
				return nil, fmt.Errorf("failed to unmarshal workflowexecution execution event_data: %w", err)
			}
			event.EventData.SetWorkflowExecutionAuditPayload(
				ogenclient.AuditEventEventDataWorkflowexecutionExecutionStartedAuditEventEventData,
				payload,
			)

		default:
			logger.V(1).Info("Skipping unsupported event type for reconstruction",
				"eventType", event.EventType,
				"correlationID", correlationID)
			// Skip unsupported event types - they're filtered by query but defense-in-depth
			continue
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
