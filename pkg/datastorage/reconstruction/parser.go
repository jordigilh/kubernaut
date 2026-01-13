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

package reconstruction

import (
	"encoding/json"
	"fmt"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ParsedAuditData contains structured data extracted from audit events for RR reconstruction.
// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
type ParsedAuditData struct {
	// Metadata
	EventType     string
	CorrelationID string

	// Gateway fields (from gateway.signal.received)
	SignalType        string
	AlertName         string
	SignalLabels      map[string]string
	SignalAnnotations map[string]string
	OriginalPayload   string

	// Orchestrator fields (from orchestrator.lifecycle.created)
	TimeoutConfig *TimeoutConfigData

	// Workflow fields (Gap #5-6)
	SelectedWorkflowRef *WorkflowRefData     // from workflowexecution.selection.completed
	ExecutionRef        *ExecutionRefData    // from workflowexecution.execution.started
}

// TimeoutConfigData represents timeout configuration extracted from audit events.
type TimeoutConfigData struct {
	Global     string
	Processing string
	Analyzing  string
	Executing  string
}

// WorkflowRefData represents workflow reference extracted from workflowexecution.selection.completed event (Gap #5).
type WorkflowRefData struct {
	WorkflowID      string
	Version         string
	ContainerImage  string
	ContainerDigest string
}

// ExecutionRefData represents execution reference extracted from workflowexecution.execution.started event (Gap #6).
type ExecutionRefData struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
}

// ParseAuditEvent extracts structured data from an audit event for RR reconstruction.
// TDD GREEN: Minimal implementation to pass current tests.
func ParseAuditEvent(event ogenclient.AuditEvent) (*ParsedAuditData, error) {
	switch event.EventType {
	case "gateway.signal.received":
		return parseGatewaySignalReceived(event)
	case "orchestrator.lifecycle.created":
		return parseOrchestratorLifecycleCreated(event)
	case "workflowexecution.selection.completed":
		return parseWorkflowSelectionCompleted(event)
	case "workflowexecution.execution.started":
		return parseExecutionWorkflowStarted(event)
	default:
		return nil, fmt.Errorf("unsupported event type: %s", event.EventType)
	}
}

func parseGatewaySignalReceived(event ogenclient.AuditEvent) (*ParsedAuditData, error) {
	payload := event.EventData.GatewayAuditPayload

	// Validate required fields
	if payload.AlertName == "" {
		return nil, fmt.Errorf("missing alert_name in gateway.signal.received event")
	}

	data := &ParsedAuditData{
		EventType:         event.EventType,
		CorrelationID:     event.CorrelationID,
		SignalType:        string(payload.SignalType),
		AlertName:         payload.AlertName,
		SignalLabels:      make(map[string]string),
		SignalAnnotations: make(map[string]string),
	}

	// Extract optional labels
	if payload.SignalLabels.IsSet() {
		data.SignalLabels = payload.SignalLabels.Value
	}

	// Extract optional annotations
	if payload.SignalAnnotations.IsSet() {
		data.SignalAnnotations = payload.SignalAnnotations.Value
	}

	// Extract optional original payload
	if payload.OriginalPayload.IsSet() {
		originalPayloadBytes, err := json.Marshal(payload.OriginalPayload.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal original_payload: %w", err)
		}
		data.OriginalPayload = string(originalPayloadBytes)
	}

	return data, nil
}

func parseOrchestratorLifecycleCreated(event ogenclient.AuditEvent) (*ParsedAuditData, error) {
	payload := event.EventData.RemediationOrchestratorAuditPayload

	data := &ParsedAuditData{
		EventType:     event.EventType,
		CorrelationID: event.CorrelationID,
	}

	// Extract TimeoutConfig if present
	if payload.TimeoutConfig.IsSet() {
		tc := payload.TimeoutConfig.Value
		data.TimeoutConfig = &TimeoutConfigData{
			Global:     getOptString(tc.Global),
			Processing: getOptString(tc.Processing),
			Analyzing:  getOptString(tc.Analyzing),
			Executing:  getOptString(tc.Executing),
		}
	}

	return data, nil
}

// getOptString extracts the value from an OptString, returning empty string if not set.
func getOptString(opt ogenclient.OptString) string {
	if opt.IsSet() {
		return opt.Value
	}
	return ""
}

// parseWorkflowSelectionCompleted extracts workflow reference from workflowexecution.selection.completed event (Gap #5).
func parseWorkflowSelectionCompleted(event ogenclient.AuditEvent) (*ParsedAuditData, error) {
	payload := event.EventData.WorkflowExecutionAuditPayload

	data := &ParsedAuditData{
		EventType:     event.EventType,
		CorrelationID: event.CorrelationID,
	}

	// Extract workflow reference (Gap #5)
	data.SelectedWorkflowRef = &WorkflowRefData{
		WorkflowID:     payload.WorkflowID,
		Version:        payload.WorkflowVersion,
		ContainerImage: payload.ContainerImage,
	}

	return data, nil
}

// parseExecutionWorkflowStarted extracts execution reference from workflowexecution.execution.started event (Gap #6).
func parseExecutionWorkflowStarted(event ogenclient.AuditEvent) (*ParsedAuditData, error) {
	payload := event.EventData.WorkflowExecutionAuditPayload

	data := &ParsedAuditData{
		EventType:     event.EventType,
		CorrelationID: event.CorrelationID,
	}

	// Extract execution reference (Gap #6)
	if payload.PipelinerunName.IsSet() {
		// ExecutionRef points to the WorkflowExecution CRD, not the PipelineRun
		// Per BR-AUDIT-005: Link RR to WFE CRD for complete lifecycle tracking
		data.ExecutionRef = &ExecutionRefData{
			APIVersion: "workflowexecution.kubernaut.ai/v1alpha1",
			Kind:       "WorkflowExecution",
			Name:       payload.ExecutionName, // WFE CRD name
			Namespace:  event.Namespace.Value,
		}
	}

	return data, nil
}
