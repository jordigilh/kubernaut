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

package datastorage

import (
	"encoding/json"
	"time"

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// TYPED AUDIT EVENT TEST HELPERS
// ========================================
//
// Purpose: Create audit events using strongly-typed ogenclient payloads
// to prevent unstructured map[string]interface{} anti-pattern.
//
// Anti-Pattern to Avoid:
//   EventData: map[string]interface{}{"field": "value"} // ❌ No compile-time validation
//
// Correct Pattern:
//   payload := ogenclient.GatewayAuditPayload{...}      // ✅ Compile-time validated
//   CreateGatewaySignalReceivedEvent(correlationID, payload)
//
// Benefits:
// - Compile-time type safety (missing required fields caught immediately)
// - IDE autocomplete for all payload fields
// - Schema compliance guaranteed by ogenclient types
// - No iterative "missing field" errors during test runs
//
// Reference: BR-AUDIT-005 v2.0 - RR Reconstruction Test Infrastructure
// ========================================

// CreateGatewaySignalReceivedEvent creates a typed gateway.signal.received audit event.
//
// Required fields in payload:
// - EventType (must be gateway.signal.received)
// - AlertName
// - Namespace
// - Fingerprint
func CreateGatewaySignalReceivedEvent(
	correlationID string,
	payload ogenclient.GatewayAuditPayload,
) (*repository.AuditEvent, error) {
	// Validate required discriminator
	if payload.EventType != ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived {
		payload.EventType = ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived
	}

	// Use ogen's encoder to properly handle Opt types
	encoder := &jx.Encoder{}
	payload.Encode(encoder)
	payloadJSON := encoder.Bytes()

	// Convert JSON to map for repository layer
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &eventDataMap); err != nil {
		return nil, err
	}

	return &repository.AuditEvent{
		EventID:        uuid.New(),
		Version:        "1.0",
		EventTimestamp: time.Now().UTC(),
		EventType:      "gateway.signal.received",
		EventCategory:  "gateway",
		EventAction:    "received",
		EventOutcome:   "success",
		CorrelationID:  correlationID,
		ResourceType:   "Signal",
		ResourceID:     payload.SignalName,
		EventData:      eventDataMap,
	}, nil
}

// CreateOrchestratorLifecycleCreatedEvent creates a typed orchestrator.lifecycle.created audit event.
//
// Required fields in payload:
// - EventType (must be orchestrator.lifecycle.created)
// - RRName
// - Namespace
func CreateOrchestratorLifecycleCreatedEvent(
	correlationID string,
	payload ogenclient.RemediationOrchestratorAuditPayload,
) (*repository.AuditEvent, error) {
	// Validate required discriminator
	if payload.EventType != ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated {
		payload.EventType = ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated
	}

	// Use ogen's encoder to properly handle Opt types
	encoder := &jx.Encoder{}
	payload.Encode(encoder)
	payloadJSON := encoder.Bytes()

	// Convert JSON to map for repository layer
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &eventDataMap); err != nil {
		return nil, err
	}

	return &repository.AuditEvent{
		EventID:        uuid.New(),
		Version:        "1.0",
		EventTimestamp: time.Now().UTC(),
		EventType:      "orchestrator.lifecycle.created",
		EventCategory:  "orchestrator",
		EventAction:    "created",
		EventOutcome:   "success",
		CorrelationID:  correlationID,
		ResourceType:   "RemediationRequest",
		ResourceID:     payload.RrName,
		EventData:      eventDataMap,
	}, nil
}

// CreateAIAnalysisCompletedEvent creates a typed aianalysis.analysis.completed audit event.
//
// Required fields in payload:
// - EventType (must be aianalysis.analysis.completed)
// - AnalysisName
// - Namespace
// - Phase
// - ApprovalRequired
// - DegradedMode
// - WarningsCount
func CreateAIAnalysisCompletedEvent(
	correlationID string,
	payload ogenclient.AIAnalysisAuditPayload,
) (*repository.AuditEvent, error) {
	// Validate required discriminator
	if payload.EventType != ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted {
		payload.EventType = ogenclient.AIAnalysisAuditPayloadEventTypeAianalysisAnalysisCompleted
	}

	// Use ogen's encoder to properly handle Opt types
	encoder := &jx.Encoder{}
	payload.Encode(encoder)
	payloadJSON := encoder.Bytes()

	// Convert JSON to map for repository layer
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &eventDataMap); err != nil {
		return nil, err
	}

	return &repository.AuditEvent{
		EventID:        uuid.New(),
		Version:        "1.0",
		EventTimestamp: time.Now().UTC(),
		EventType:      "aianalysis.analysis.completed",
		EventCategory:  "analysis",
		EventAction:    "completed",
		EventOutcome:   "success",
		CorrelationID:  correlationID,
		ResourceType:   "AIAnalysis",
		ResourceID:     payload.AnalysisName,
		EventData:      eventDataMap,
	}, nil
}

// CreateWorkflowSelectionCompletedEvent creates a typed workflowexecution.selection.completed audit event.
//
// Required fields in payload:
// - EventType (must be workflowexecution.selection.completed)
// - ExecutionName
// - Namespace
// - Phase
// - WorkflowID
// - WorkflowVersion
// - TargetResource
// - ContainerImage
func CreateWorkflowSelectionCompletedEvent(
	correlationID string,
	payload ogenclient.WorkflowExecutionAuditPayload,
) (*repository.AuditEvent, error) {
	// Validate required discriminator
	if payload.EventType != ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionSelectionCompleted {
		payload.EventType = ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionSelectionCompleted
	}

	// Use ogen's encoder to properly handle Opt types
	encoder := &jx.Encoder{}
	payload.Encode(encoder)
	payloadJSON := encoder.Bytes()

	// Convert JSON to map for repository layer
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &eventDataMap); err != nil {
		return nil, err
	}

	return &repository.AuditEvent{
		EventID:        uuid.New(),
		Version:        "1.0",
		EventTimestamp: time.Now().UTC(),
		EventType:      "workflowexecution.selection.completed",
		EventCategory:  "workflowexecution",
		EventAction:    "selection_completed",
		EventOutcome:   "success",
		CorrelationID:  correlationID,
		ResourceType:   "WorkflowExecution",
		ResourceID:     payload.ExecutionName,
		EventData:      eventDataMap,
	}, nil
}

// CreateWorkflowExecutionStartedEvent creates a typed workflowexecution.execution.started audit event.
//
// Required fields in payload:
// - EventType (must be workflowexecution.execution.started)
// - ExecutionName
// - Namespace
// - Phase
// - WorkflowID
// - WorkflowVersion
// - TargetResource
// - ContainerImage
func CreateWorkflowExecutionStartedEvent(
	correlationID string,
	payload ogenclient.WorkflowExecutionAuditPayload,
) (*repository.AuditEvent, error) {
	// Validate required discriminator
	if payload.EventType != ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted {
		payload.EventType = ogenclient.WorkflowExecutionAuditPayloadEventTypeWorkflowexecutionExecutionStarted
	}

	// Use ogen's encoder to properly handle Opt types
	encoder := &jx.Encoder{}
	payload.Encode(encoder)
	payloadJSON := encoder.Bytes()

	// Convert JSON to map for repository layer
	var eventDataMap map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &eventDataMap); err != nil {
		return nil, err
	}

	return &repository.AuditEvent{
		EventID:        uuid.New(),
		Version:        "1.0",
		EventTimestamp: time.Now().UTC(),
		EventType:      "workflowexecution.execution.started",
		EventCategory:  "workflowexecution",
		EventAction:    "execution_started",
		EventOutcome:   "success",
		CorrelationID:  correlationID,
		ResourceType:   "WorkflowExecution",
		ResourceID:     payload.ExecutionName,
		EventData:      eventDataMap,
	}, nil
}
