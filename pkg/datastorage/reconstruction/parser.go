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
	"fmt"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// ParsedAuditData contains structured data extracted from audit events for RR reconstruction.
// BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
type ParsedAuditData struct {
	// Gateway fields (from gateway.signal.received)
	SignalType        string
	AlertName         string
	SignalLabels      map[string]string
	SignalAnnotations map[string]string

	// Orchestrator fields (from orchestrator.lifecycle.created)
	TimeoutConfig *TimeoutConfigData
}

// TimeoutConfigData represents timeout configuration extracted from audit events.
type TimeoutConfigData struct {
	Global     string
	Processing string
	Analyzing  string
	Executing  string
}

// ParseAuditEvent extracts structured data from an audit event for RR reconstruction.
// TDD GREEN: Minimal implementation to pass current tests.
func ParseAuditEvent(event ogenclient.AuditEvent) (*ParsedAuditData, error) {
	switch event.EventType {
	case "gateway.signal.received":
		return parseGatewaySignalReceived(event)
	case "orchestrator.lifecycle.created":
		return parseOrchestratorLifecycleCreated(event)
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

	return data, nil
}

func parseOrchestratorLifecycleCreated(event ogenclient.AuditEvent) (*ParsedAuditData, error) {
	payload := event.EventData.RemediationOrchestratorAuditPayload

	data := &ParsedAuditData{}

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
