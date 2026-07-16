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

	corev1 "k8s.io/api/core/v1"
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

// rrFieldMappers dispatches MapToRRFields by event type, replacing a large
// switch statement (registry pattern). Unknown event types have no entry and
// are silently ignored, keeping reconstruction tolerant of irrelevant events.
var rrFieldMappers = map[string]func(*ParsedAuditData, *ReconstructedRRFields) error{
	"gateway.signal.received":               mapGatewaySignalFields,
	"orchestrator.lifecycle.created":        mapOrchestratorCreatedFields,
	"aianalysis.analysis.completed":         mapAIAnalysisCompletedFields,
	"workflowexecution.selection.completed": mapWorkflowSelectionCompletedFields,
	"workflowexecution.execution.started":   mapWorkflowExecutionStartedFields,
	"orchestrator.lifecycle.completed":      mapOrchestratorTerminalFields,
	"orchestrator.lifecycle.failed":         mapOrchestratorTerminalFields,
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

	if mapper, ok := rrFieldMappers[parsedData.EventType]; ok {
		if err := mapper(parsedData, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// mapGatewaySignalFields maps Gateway audit data to RR Spec (Gaps #1-3).
func mapGatewaySignalFields(parsedData *ParsedAuditData, result *ReconstructedRRFields) error {
	if parsedData.SignalName == "" {
		return fmt.Errorf("alert name is required for gateway.signal.received event")
	}

	result.Spec.SignalName = parsedData.SignalName
	result.Spec.SignalType = parsedData.SignalType
	result.Spec.SignalFingerprint = parsedData.SignalFingerprint // BR-AUDIT-005: deduplication identity
	result.Spec.SignalLabels = parsedData.SignalLabels
	result.Spec.SignalAnnotations = parsedData.SignalAnnotations

	// Map OriginalPayload (string, issue #96)
	if len(parsedData.OriginalPayload) > 0 {
		result.Spec.OriginalPayload = parsedData.OriginalPayload
	}

	// DD-AUDIT-003 v2.2: Map cluster_name to Spec.ClusterID for fleet reconstruction (CC8.1)
	if parsedData.ClusterName != "" {
		result.Spec.ClusterID = parsedData.ClusterName
	}

	return nil
}

// mapOrchestratorCreatedFields maps Orchestrator audit data to RR Status (Gap #8).
func mapOrchestratorCreatedFields(parsedData *ParsedAuditData, result *ReconstructedRRFields) error {
	if parsedData.TimeoutConfig == nil {
		return nil
	}

	tc, err := buildTimeoutConfig(parsedData.TimeoutConfig)
	if err != nil {
		return err
	}
	result.Status.TimeoutConfig = tc
	return nil
}

// buildTimeoutConfig parses each duration string field of a TimeoutConfig
// into a *remediationv1.TimeoutConfig, leaving unset fields nil.
func buildTimeoutConfig(src *TimeoutConfigData) (*remediationv1.TimeoutConfig, error) {
	tc := &remediationv1.TimeoutConfig{}

	durations := []struct {
		name string
		src  string
		dst  **metav1.Duration
	}{
		{"Global", src.Global, &tc.Global},
		{"Processing", src.Processing, &tc.Processing},
		{"Analyzing", src.Analyzing, &tc.Analyzing},
		{"Executing", src.Executing, &tc.Executing},
	}

	for _, d := range durations {
		if d.src == "" {
			continue
		}
		parsed, err := parseDuration(d.src)
		if err != nil {
			return nil, fmt.Errorf("invalid %s duration: %w", d.name, err)
		}
		*d.dst = parsed
	}

	return tc, nil
}

// mapAIAnalysisCompletedFields maps AI Analysis audit data to RR Spec (Gap #4).
func mapAIAnalysisCompletedFields(parsedData *ParsedAuditData, result *ReconstructedRRFields) error {
	if len(parsedData.ProviderData) > 0 {
		result.Spec.ProviderData = parsedData.ProviderData
	}
	return nil
}

// mapWorkflowSelectionCompletedFields maps Workflow Selection audit data to RR Status (Gap #5).
func mapWorkflowSelectionCompletedFields(parsedData *ParsedAuditData, result *ReconstructedRRFields) error {
	if parsedData.SelectedWorkflowRef != nil {
		result.Status.SelectedWorkflowRef = &remediationv1.WorkflowReference{
			WorkflowID:            parsedData.SelectedWorkflowRef.WorkflowID,
			Version:               parsedData.SelectedWorkflowRef.Version,
			ExecutionBundle:       parsedData.SelectedWorkflowRef.ContainerImage,
			ExecutionBundleDigest: parsedData.SelectedWorkflowRef.ContainerDigest,
		}
	}
	return nil
}

// mapWorkflowExecutionStartedFields maps Workflow Execution audit data to RR Status (Gap #6).
func mapWorkflowExecutionStartedFields(parsedData *ParsedAuditData, result *ReconstructedRRFields) error {
	if parsedData.ExecutionRef != nil {
		result.Status.ExecutionRef = &corev1.ObjectReference{
			APIVersion: parsedData.ExecutionRef.APIVersion,
			Kind:       parsedData.ExecutionRef.Kind,
			Name:       parsedData.ExecutionRef.Name,
			Namespace:  parsedData.ExecutionRef.Namespace,
		}
	}
	return nil
}

// mapOrchestratorTerminalFields maps completion/failure data to RR Status (CC8.1).
func mapOrchestratorTerminalFields(parsedData *ParsedAuditData, result *ReconstructedRRFields) error {
	if parsedData.Outcome != "" {
		result.Status.Outcome = parsedData.Outcome
	}
	if parsedData.FailurePhase != "" {
		fp := remediationv1.FailurePhase(parsedData.FailurePhase)
		result.Status.FailurePhase = &fp
	}
	if parsedData.ErrorDetails != nil && parsedData.ErrorDetails.Message != "" {
		reason := parsedData.ErrorDetails.Message
		result.Status.FailureReason = &reason
	}
	return nil
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

		mergeSpecFields(result.Spec, eventFields.Spec)
		mergeStatusFields(result.Status, eventFields.Status)
	}

	return result, nil
}

// mergeSpecFields merges non-empty fields from src (gateway/AI-analysis
// events) into dst, keeping the last non-empty/non-nil value seen so later
// events in chronological order take precedence.
func mergeSpecFields(dst, src *remediationv1.RemediationRequestSpec) {
	if src == nil {
		return
	}

	mergeSpecScalarFields(dst, src)
	mergeSpecMapFields(dst, src)
}

// mergeSpecScalarFields merges the scalar (non-map) fields of src into dst.
func mergeSpecScalarFields(dst, src *remediationv1.RemediationRequestSpec) {
	if src.SignalName != "" {
		dst.SignalName = src.SignalName
	}
	if src.SignalType != "" {
		dst.SignalType = src.SignalType
	}
	if src.SignalFingerprint != "" {
		dst.SignalFingerprint = src.SignalFingerprint
	}
	// DD-AUDIT-003 v2.2: Merge ClusterID for fleet reconstruction (CC8.1)
	if src.ClusterID != "" {
		dst.ClusterID = src.ClusterID
	}
	if len(src.OriginalPayload) > 0 {
		dst.OriginalPayload = src.OriginalPayload
	}
	// Gap #4: Merge ProviderData from AI Analysis event
	if len(src.ProviderData) > 0 {
		dst.ProviderData = src.ProviderData
	}
}

// mergeSpecMapFields merges the map-valued fields (labels/annotations) of
// src into dst, initializing dst's maps lazily.
func mergeSpecMapFields(dst, src *remediationv1.RemediationRequestSpec) {
	if src.SignalLabels != nil {
		if dst.SignalLabels == nil {
			dst.SignalLabels = make(map[string]string)
		}
		for k, v := range src.SignalLabels {
			dst.SignalLabels[k] = v
		}
	}
	if src.SignalAnnotations != nil {
		if dst.SignalAnnotations == nil {
			dst.SignalAnnotations = make(map[string]string)
		}
		for k, v := range src.SignalAnnotations {
			dst.SignalAnnotations[k] = v
		}
	}
}

// mergeStatusFields merges non-nil/non-empty fields from src (orchestrator
// and workflow lifecycle events) into dst.
func mergeStatusFields(dst, src *remediationv1.RemediationRequestStatus) {
	if src == nil {
		return
	}

	if src.TimeoutConfig != nil {
		dst.TimeoutConfig = src.TimeoutConfig
	}
	// CC8.1: Merge outcome for completion/failure reconstruction
	if src.Outcome != "" {
		dst.Outcome = src.Outcome
	}
	if src.FailurePhase != nil {
		dst.FailurePhase = src.FailurePhase
	}
	if src.FailureReason != nil {
		dst.FailureReason = src.FailureReason
	}
	// Gap #5: Merge SelectedWorkflowRef from workflow selection event
	if src.SelectedWorkflowRef != nil {
		dst.SelectedWorkflowRef = src.SelectedWorkflowRef
	}
	// Gap #6: Merge ExecutionRef from workflow execution event
	if src.ExecutionRef != nil {
		dst.ExecutionRef = src.ExecutionRef
	}
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
