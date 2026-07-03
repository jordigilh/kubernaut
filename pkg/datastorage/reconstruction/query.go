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

	rows, err := db.QueryContext(ctx, reconstructionEventsQuery, correlationID)
	if err != nil {
		logger.Error(err, "Failed to query audit events for RR reconstruction",
			"correlationID", correlationID)
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			logger.Error(err, "Failed to close database rows")
		}
	}()

	events, err := scanReconstructionRows(rows, logger, correlationID)
	if err != nil {
		return nil, err
	}

	logger.V(1).Info("Successfully queried audit events for RR reconstruction",
		"correlationID", correlationID,
		"eventCount", len(events))

	return events, nil
}

// reconstructionEventsQuery selects all reconstruction-relevant audit events
// for a correlation ID. Per ADR-034: use the unified audit_events table,
// ordered by timestamp ASC for chronological reconstruction.
const reconstructionEventsQuery = `
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
		  'orchestrator.lifecycle.created',
		  'orchestrator.lifecycle.completed',
		  'orchestrator.lifecycle.failed'
	  )
	ORDER BY event_timestamp ASC, event_id ASC
	LIMIT 1000
`

// scanReconstructionRows iterates over the query result set, scanning and
// decoding each row via scanReconstructionEvent. Extracted from
// QueryAuditEventsForReconstruction (Wave 6 6f GREEN: funlen remediation) —
// pure code motion, no behavior change.
func scanReconstructionRows(rows *sql.Rows, logger logr.Logger, correlationID string) ([]ogenclient.AuditEvent, error) {
	var events []ogenclient.AuditEvent
	for rows.Next() {
		event, include, err := scanReconstructionEvent(rows, logger, correlationID)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		logger.Error(err, "Error iterating audit event rows",
			"correlationID", correlationID)
		return nil, fmt.Errorf("failed to iterate audit events: %w", err)
	}

	return events, nil
}

// scanReconstructionEvent scans one row into an ogenclient.AuditEvent,
// converts nullable SQL columns to ogen Opt* fields, and decodes the
// discriminated-union event_data payload via eventDataDecoders.
//
// Return values:
//   - include=false, err=nil: the row's event_type has no registered decoder
//     (defense-in-depth skip; the query already filters to relevant types).
//   - err!=nil: scanning or decoding failed; the caller must abort the query.
func scanReconstructionEvent(rows *sql.Rows, logger logr.Logger, correlationID string) (ogenclient.AuditEvent, bool, error) {
	event, eventDataJSON, eventID, err := scanReconstructionRow(rows)
	if err != nil {
		logger.Error(err, "Failed to scan audit event row",
			"correlationID", correlationID)
		return event, false, fmt.Errorf("failed to scan audit event: %w", err)
	}

	if len(eventDataJSON) == 0 {
		return event, true, nil
	}

	// Manually construct discriminated union variant based on event_type.
	// The raw JSON in database doesn't have a discriminator, so the variant
	// is resolved via eventDataDecoders (registry keyed by event_type).
	// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3: replaces a 7-case wide switch
	// (cyclomatic 35 overall) with a map lookup; behavior is unchanged.
	decoder, ok := eventDataDecoders[event.EventType]
	if !ok {
		logger.V(1).Info("Skipping unsupported event type for reconstruction",
			"eventType", event.EventType,
			"correlationID", correlationID)
		// Skip unsupported event types - they're filtered by query but defense-in-depth
		return event, false, nil
	}

	decoded, decodeErr := decoder(eventDataJSON)
	if decodeErr != nil {
		logger.Error(decodeErr, "Failed to unmarshal event_data for reconstruction",
			"eventType", event.EventType,
			"correlationID", correlationID,
			"eventID", eventID)
		return event, false, fmt.Errorf("failed to unmarshal %s event_data: %w", event.EventType, decodeErr)
	}
	event.EventData = decoded
	return event, true, nil
}

// scanReconstructionRow performs the raw rows.Scan and converts sql.Null*
// intermediates to ogen Opt* fields on the returned AuditEvent. It also
// returns the raw event_data JSON bytes (decoded separately by the caller)
// and the scanned event_id (as sql.NullString, for error-log correlation).
func scanReconstructionRow(rows *sql.Rows) (ogenclient.AuditEvent, []byte, sql.NullString, error) {
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
		return event, nil, eventID, err
	}

	applyNullableReconstructionFields(&event, nullableReconstructionColumns{
		eventID:       eventID,
		parentEventID: parentEventID,
		namespace:     namespace,
		clusterName:   clusterName,
		severity:      severity,
		resourceType:  resourceType,
		resourceID:    resourceID,
		actorType:     actorType,
		actorID:       actorID,
		durationMs:    durationMs,
		eventDate:     eventDate,
	})

	return event, eventDataJSON, eventID, nil
}

// nullableReconstructionColumns groups the sql.Null* scan intermediates for
// columns that map to ogen Opt* fields on ogenclient.AuditEvent, so
// applyNullableReconstructionFields can take them as a single argument.
type nullableReconstructionColumns struct {
	eventID       sql.NullString
	parentEventID sql.NullString
	namespace     sql.NullString
	clusterName   sql.NullString
	severity      sql.NullString
	resourceType  sql.NullString
	resourceID    sql.NullString
	actorType     sql.NullString
	actorID       sql.NullString
	durationMs    sql.NullInt32
	eventDate     sql.NullTime
}

// applyNullableReconstructionFields converts sql.Null* scan intermediates to
// the corresponding ogen Opt* fields on event, leaving fields unset (their
// Opt zero value) when the source column was NULL.
func applyNullableReconstructionFields(event *ogenclient.AuditEvent, cols nullableReconstructionColumns) {
	if cols.eventID.Valid {
		if parsedUUID, parseErr := uuid.Parse(cols.eventID.String); parseErr == nil {
			event.EventID.SetTo(parsedUUID)
		}
	}
	if cols.parentEventID.Valid {
		if parsedParentUUID, parseErr := uuid.Parse(cols.parentEventID.String); parseErr == nil {
			event.ParentEventID.SetTo(parsedParentUUID)
		}
	}
	if cols.namespace.Valid {
		event.Namespace.SetTo(cols.namespace.String)
	}
	if cols.resourceType.Valid {
		event.ResourceType.SetTo(cols.resourceType.String)
	}
	if cols.resourceID.Valid {
		event.ResourceID.SetTo(cols.resourceID.String)
	}
	if cols.actorType.Valid {
		event.ActorType.SetTo(cols.actorType.String)
	}
	if cols.actorID.Valid {
		event.ActorID.SetTo(cols.actorID.String)
	}
	if cols.clusterName.Valid {
		event.ClusterName.SetTo(cols.clusterName.String)
	}
	if cols.severity.Valid {
		event.Severity.SetTo(cols.severity.String)
	}
	if cols.durationMs.Valid {
		event.DurationMs.SetTo(int(cols.durationMs.Int32))
	}
	if cols.eventDate.Valid {
		event.EventDate.SetTo(cols.eventDate.Time)
	}
}

// eventDataDecoder unmarshals a raw event_data JSON blob into the correctly
// typed discriminated-union variant of ogenclient.AuditEventEventData for one
// specific event_type value. One entry in eventDataDecoders corresponds to
// exactly one case of the reconstruction query's former wide switch
// statement (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3).
type eventDataDecoder func(eventDataJSON []byte) (ogenclient.AuditEventEventData, error)

// eventDataDecoders maps each reconstruction-relevant event_type (see
// IsReconstructionRelevant) to the decoder that builds its typed EventData
// variant. Keeping this as a package-level registry (rather than a switch
// inline in QueryAuditEventsForReconstruction) lets each event type's
// decode logic be extended independently without growing the query
// function's cyclomatic complexity.
var eventDataDecoders = map[string]eventDataDecoder{
	"gateway.signal.received": decodeGatewayAuditPayload,
	"orchestrator.lifecycle.created": decodeOrchestratorAuditPayload(
		ogenclient.AuditEventEventDataOrchestratorLifecycleCreatedAuditEventEventData),
	"orchestrator.lifecycle.completed": decodeOrchestratorAuditPayload(
		ogenclient.AuditEventEventDataOrchestratorLifecycleCompletedAuditEventEventData),
	"orchestrator.lifecycle.failed": decodeOrchestratorAuditPayload(
		ogenclient.AuditEventEventDataOrchestratorLifecycleFailedAuditEventEventData),
	"aianalysis.analysis.completed": decodeAIAnalysisAuditPayload,
	"workflowexecution.selection.completed": decodeWorkflowExecutionAuditPayload(
		ogenclient.AuditEventEventDataWorkflowexecutionSelectionCompletedAuditEventEventData),
	"workflowexecution.execution.started": decodeWorkflowExecutionAuditPayload(
		ogenclient.AuditEventEventDataWorkflowexecutionExecutionStartedAuditEventEventData),
}

func decodeGatewayAuditPayload(eventDataJSON []byte) (ogenclient.AuditEventEventData, error) {
	var payload ogenclient.GatewayAuditPayload
	if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
		return ogenclient.AuditEventEventData{}, err
	}
	var data ogenclient.AuditEventEventData
	data.SetGatewayAuditPayload(ogenclient.AuditEventEventDataGatewaySignalReceivedAuditEventEventData, payload)
	return data, nil
}

func decodeOrchestratorAuditPayload(discriminator ogenclient.AuditEventEventDataType) eventDataDecoder {
	return func(eventDataJSON []byte) (ogenclient.AuditEventEventData, error) {
		var payload ogenclient.RemediationOrchestratorAuditPayload
		if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
			return ogenclient.AuditEventEventData{}, err
		}
		var data ogenclient.AuditEventEventData
		data.SetRemediationOrchestratorAuditPayload(discriminator, payload)
		return data, nil
	}
}

func decodeAIAnalysisAuditPayload(eventDataJSON []byte) (ogenclient.AuditEventEventData, error) {
	var payload ogenclient.AIAnalysisAuditPayload
	if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
		return ogenclient.AuditEventEventData{}, err
	}
	var data ogenclient.AuditEventEventData
	data.SetAIAnalysisAuditPayload(ogenclient.AuditEventEventDataAianalysisAnalysisCompletedAuditEventEventData, payload)
	return data, nil
}

func decodeWorkflowExecutionAuditPayload(discriminator ogenclient.AuditEventEventDataType) eventDataDecoder {
	return func(eventDataJSON []byte) (ogenclient.AuditEventEventData, error) {
		var payload ogenclient.WorkflowExecutionAuditPayload
		if err := json.Unmarshal(eventDataJSON, &payload); err != nil {
			return ogenclient.AuditEventEventData{}, err
		}
		var data ogenclient.AuditEventEventData
		data.SetWorkflowExecutionAuditPayload(discriminator, payload)
		return data, nil
	}
}

// IsReconstructionRelevant checks if an event type is needed for RR reconstruction.
//
// This helper function validates event types before processing them for reconstruction.
func IsReconstructionRelevant(eventType string) bool {
	relevantTypes := map[string]bool{
		"gateway.signal.received":               true, // Gap #1, #3, #4
		"aianalysis.analysis.completed":         true, // Gap #2
		"workflowexecution.selection.completed": true, // Gap #5
		"workflowexecution.execution.started":   true, // Gap #6
		"orchestrator.lifecycle.created":        true, // Gap #8
		"orchestrator.lifecycle.completed":      true, // CC8.1: outcome/duration for RR status
		"orchestrator.lifecycle.failed":         true, // CC8.1: error_details for RR status
	}
	return relevantTypes[eventType]
}
