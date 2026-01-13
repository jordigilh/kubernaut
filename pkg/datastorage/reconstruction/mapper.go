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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ReconstructedRRFields represents the reconstructed fields of a RemediationRequest
// from audit trail data. This structure is designed to be merged from multiple
// audit events and then converted to a complete RR CRD.
type ReconstructedRRFields struct {
	Spec   *remediationv1.RemediationRequestSpec
	Status *remediationv1.RemediationRequestStatus
}

// MapToRRFields maps a single parsed audit event to RemediationRequest fields.
// TDD GREEN: Minimal implementation to pass current tests (Gaps #1-3, #8).
func MapToRRFields(parsedData *ParsedAuditData) (*ReconstructedRRFields, error) {
	if parsedData == nil {
		return nil, fmt.Errorf("parsedData cannot be nil")
	}

	result := &ReconstructedRRFields{
		Spec:   &remediationv1.RemediationRequestSpec{},
		Status: &remediationv1.RemediationRequestStatus{},
	}

	switch parsedData.EventType {
	case "gateway.signal.received":
		// Map Gateway audit data to RR Spec (Gaps #1-3)
		if parsedData.AlertName == "" {
			return nil, fmt.Errorf("alert name is required for gateway.signal.received event")
		}

		result.Spec.SignalName = parsedData.AlertName
		result.Spec.SignalType = string(parsedData.SignalType)
		result.Spec.SignalLabels = parsedData.SignalLabels
		result.Spec.SignalAnnotations = parsedData.SignalAnnotations

		// Convert jx.Raw to []byte for OriginalPayload
		if len(parsedData.OriginalPayload) > 0 {
			result.Spec.OriginalPayload = []byte(parsedData.OriginalPayload)
		}

	case "orchestrator.lifecycle.created":
		// Map Orchestrator audit data to RR Status (Gap #8)
		if parsedData.TimeoutConfig != nil {
			// Parse duration strings to metav1.Duration
			tc := &remediationv1.TimeoutConfig{}

			if parsedData.TimeoutConfig.Global != "" {
				globalDur, err := parseDuration(parsedData.TimeoutConfig.Global)
				if err != nil {
					return nil, fmt.Errorf("invalid Global duration: %w", err)
				}
				tc.Global = globalDur
			}

			if parsedData.TimeoutConfig.Processing != "" {
				processingDur, err := parseDuration(parsedData.TimeoutConfig.Processing)
				if err != nil {
					return nil, fmt.Errorf("invalid Processing duration: %w", err)
				}
				tc.Processing = processingDur
			}

			if parsedData.TimeoutConfig.Analyzing != "" {
				analyzingDur, err := parseDuration(parsedData.TimeoutConfig.Analyzing)
				if err != nil {
					return nil, fmt.Errorf("invalid Analyzing duration: %w", err)
				}
				tc.Analyzing = analyzingDur
			}

			if parsedData.TimeoutConfig.Executing != "" {
				executingDur, err := parseDuration(parsedData.TimeoutConfig.Executing)
				if err != nil {
					return nil, fmt.Errorf("invalid Executing duration: %w", err)
				}
				tc.Executing = executingDur
			}

			result.Status.TimeoutConfig = tc
		}

	// Add other event types as they become relevant
	// case "workflowexecution.selection.completed":
	// case "workflowexecution.execution.started":
	// case "orchestrator.lifecycle.completed":
	// case "webhook.remediationrequest.timeout_modified":

	default:
		// Unknown event types are silently ignored for now
		// This allows the reconstruction to be tolerant of irrelevant events
	}

	return result, nil
}

// MergeAuditData merges multiple parsed audit events into a single ReconstructedRRFields.
// The gateway.signal.received event is required as it contains the foundational RR spec.
// TDD GREEN: Minimal implementation to pass current merge tests.
func MergeAuditData(events []ParsedAuditData) (*ReconstructedRRFields, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no audit events provided for merging")
	}

	// Ensure gateway.signal.received event exists (mandatory for reconstruction)
	hasGatewayEvent := false
	for _, event := range events {
		if event.EventType == "gateway.signal.received" {
			hasGatewayEvent = true
			break
		}
	}
	if !hasGatewayEvent {
		return nil, fmt.Errorf("gateway.signal.received event is required for reconstruction")
	}

	// Initialize result with empty spec and status
	result := &ReconstructedRRFields{
		Spec:   &remediationv1.RemediationRequestSpec{},
		Status: &remediationv1.RemediationRequestStatus{},
	}

	// Map each event and merge into result
	for _, event := range events {
		eventFields, err := MapToRRFields(&event)
		if err != nil {
			return nil, fmt.Errorf("failed to map event %s: %w", event.EventType, err)
		}

		// Merge spec fields (gateway events populate spec)
		if eventFields.Spec != nil {
			if eventFields.Spec.SignalName != "" {
				result.Spec.SignalName = eventFields.Spec.SignalName
			}
			if eventFields.Spec.SignalType != "" {
				result.Spec.SignalType = eventFields.Spec.SignalType
			}
			if eventFields.Spec.SignalLabels != nil {
				if result.Spec.SignalLabels == nil {
					result.Spec.SignalLabels = make(map[string]string)
				}
				for k, v := range eventFields.Spec.SignalLabels {
					result.Spec.SignalLabels[k] = v
				}
			}
			if eventFields.Spec.SignalAnnotations != nil {
				if result.Spec.SignalAnnotations == nil {
					result.Spec.SignalAnnotations = make(map[string]string)
				}
				for k, v := range eventFields.Spec.SignalAnnotations {
					result.Spec.SignalAnnotations[k] = v
				}
			}
			if len(eventFields.Spec.OriginalPayload) > 0 {
				result.Spec.OriginalPayload = eventFields.Spec.OriginalPayload
			}
		}

		// Merge status fields (orchestrator events populate status)
		if eventFields.Status != nil {
			if eventFields.Status.TimeoutConfig != nil {
				result.Status.TimeoutConfig = eventFields.Status.TimeoutConfig
			}
		}
	}

	return result, nil
}

// parseDuration converts a duration string (e.g., "1h0m0s") to *metav1.Duration.
// Returns error if the duration string is invalid.
func parseDuration(durationStr string) (*metav1.Duration, error) {
	dur, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid duration '%s': %w", durationStr, err)
	}
	return &metav1.Duration{Duration: dur}, nil
}
