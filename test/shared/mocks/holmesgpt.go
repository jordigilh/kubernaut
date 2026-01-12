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

package mocks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-faster/jx"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// MockHolmesGPTClient is a mock implementation of HolmesGPTClientInterface for unit tests.
// Now uses generated types from HAPI OpenAPI spec for type-safe testing.
// BR-AI-006: Mock for API call testing
// BR-AI-082: Updated with InvestigateRecovery support
type MockHolmesGPTClient struct {
	// InvestigateFunc allows tests to customize the Investigate behavior
	InvestigateFunc func(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)

	// InvestigateRecoveryFunc allows tests to customize the InvestigateRecovery behavior
	InvestigateRecoveryFunc func(ctx context.Context, req *client.RecoveryRequest) (*client.RecoveryResponse, error)

	// CallCount tracks how many times Investigate was called
	CallCount int

	// RecoveryCallCount tracks how many times InvestigateRecovery was called
	RecoveryCallCount int

	// LastRequest stores the last request passed to Investigate
	LastRequest *client.IncidentRequest

	// LastRecoveryRequest stores the last request passed to InvestigateRecovery
	LastRecoveryRequest *client.RecoveryRequest

	// Response is the default response to return (if InvestigateFunc is nil)
	Response *client.IncidentResponse

	// RecoveryResponse is the default recovery response
	RecoveryResponse *client.RecoveryResponse

	// Err is the default error to return
	Err error

	// RecoveryErr is the default error for recovery calls (uses Err if nil)
	RecoveryErr error
}

// NewMockHolmesGPTClient creates a new mock HolmesGPT client with default success behavior.
func NewMockHolmesGPTClient() *MockHolmesGPTClient {
	// Build default SelectedWorkflow for recovery response
	swMap := make(map[string]jx.Raw)
	idBytes, _ := json.Marshal("mock-workflow-001")
	swMap["workflow_id"] = jx.Raw(idBytes)
	imgBytes, _ := json.Marshal("kubernaut.io/workflows/restart-pod:v1.0.0")
	swMap["container_image"] = jx.Raw(imgBytes)
	confBytes, _ := json.Marshal(0.8)
	swMap["confidence"] = jx.Raw(confBytes)

	recoveryResp := &client.RecoveryResponse{
		IncidentID:         "mock-recovery-001",
		CanRecover:         true,
		Strategies:         []client.RecoveryStrategy{},
		AnalysisConfidence: 0.8,
		Warnings:           []string{},
	}
	recoveryResp.SelectedWorkflow.SetTo(swMap)

	return &MockHolmesGPTClient{
		Response: &client.IncidentResponse{
			IncidentID: "mock-incident-001",
			Analysis:   "Mock analysis: No issues detected",
			Confidence: 0.8,
			Timestamp:  "2025-12-05T10:00:00Z",
			Warnings:   []string{},
		},
		// CRITICAL: Provide default RecoveryResponse to prevent nil pointer panics
		// when tests use IsRecoveryAttempt=true (BR-AI-083: Recovery endpoint routing)
		RecoveryResponse: recoveryResp,
	}
}

// Investigate implements HolmesGPTClientInterface.
func (m *MockHolmesGPTClient) Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error) {
	m.CallCount++
	m.LastRequest = req

	if m.InvestigateFunc != nil {
		return m.InvestigateFunc(ctx, req)
	}

	return m.Response, m.Err
}

// InvestigateRecovery implements HolmesGPTClientInterface for recovery attempts.
// BR-AI-082: Recovery endpoint implementation
func (m *MockHolmesGPTClient) InvestigateRecovery(ctx context.Context, req *client.RecoveryRequest) (*client.RecoveryResponse, error) {
	m.RecoveryCallCount++
	m.LastRecoveryRequest = req

	if m.InvestigateRecoveryFunc != nil {
		return m.InvestigateRecoveryFunc(ctx, req)
	}

	return m.RecoveryResponse, m.Err
}

// WithResponse configures the mock to return a specific response.
func (m *MockHolmesGPTClient) WithResponse(resp *client.IncidentResponse) *MockHolmesGPTClient {
	m.Response = resp
	m.Err = nil
	return m
}

// WithError configures the mock to return an error.
func (m *MockHolmesGPTClient) WithError(err error) *MockHolmesGPTClient {
	m.Response = nil
	m.Err = err
	return m
}

// WithSuccessResponse configures the mock to return a successful investigation response.
func (m *MockHolmesGPTClient) WithSuccessResponse(analysis string, confidence float64, warnings []string) *MockHolmesGPTClient {
	m.Response = &client.IncidentResponse{
		IncidentID: "mock-incident-001",
		Analysis:   analysis,
		Confidence: confidence,
		Timestamp:  "2025-12-05T10:00:00Z",
		Warnings:   warnings,
	}
	m.Err = nil
	return m
}

// WithFullResponse configures the mock to return a complete response including RCA and workflow.
func (m *MockHolmesGPTClient) WithFullResponse(
	analysis string,
	confidence float64,
	warnings []string,
	rcaSummary string,
	rcaSeverity string,
	workflowID string,
	containerImage string,
	workflowConfidence float64,
	targetInOwnerChain bool,
	workflowRationale string,
	includeAlternatives bool,
) *MockHolmesGPTClient {
	// Build RCA as map[string]jx.Raw
	rcaMap := make(map[string]jx.Raw)
	if rcaSummary != "" {
		summaryBytes, _ := json.Marshal(rcaSummary)
		rcaMap["summary"] = jx.Raw(summaryBytes)
		sevBytes, _ := json.Marshal(rcaSeverity)
		rcaMap["severity"] = jx.Raw(sevBytes)
	}

	// Build SelectedWorkflow as map[string]jx.Raw
	swMap := make(map[string]jx.Raw)
	if workflowID != "" {
		idBytes, _ := json.Marshal(workflowID)
		swMap["workflow_id"] = jx.Raw(idBytes)
		imgBytes, _ := json.Marshal(containerImage)
		swMap["container_image"] = jx.Raw(imgBytes)
		confBytes, _ := json.Marshal(workflowConfidence)
		swMap["confidence"] = jx.Raw(confBytes)
		if workflowRationale != "" {
			ratBytes, _ := json.Marshal(workflowRationale)
			swMap["rationale"] = jx.Raw(ratBytes)
		}
	}

	// Build AlternativeWorkflows as []client.AlternativeWorkflow
	var alternatives []client.AlternativeWorkflow
	if includeAlternatives && workflowID != "" {
		alt := client.AlternativeWorkflow{
			WorkflowID:     "wf-scale-deployment",
			Confidence:     0.75,
			Rationale:      "Consider scaling deployment for resource pressure",
			ContainerImage: client.NewOptNilString("kubernaut.io/workflows/scale:v1.0.0"),
		}
		alternatives = append(alternatives, alt)
	}

	m.Response = &client.IncidentResponse{
		IncidentID:           "mock-incident-001",
		Analysis:             analysis,
		RootCauseAnalysis:    rcaMap,
		Confidence:           confidence,
		Timestamp:            "2025-12-05T10:00:00Z",
		Warnings:             warnings,
		AlternativeWorkflows: alternatives,
	}

	if len(swMap) > 0 {
		m.Response.SelectedWorkflow.SetTo(swMap)
	}

	// Set TargetInOwnerChain from parameter
	m.Response.TargetInOwnerChain.SetTo(targetInOwnerChain)

	m.Err = nil
	return m
}

// Reset resets the mock's state (call count and last request).
func (m *MockHolmesGPTClient) Reset() {
	m.CallCount = 0
	m.RecoveryCallCount = 0
	m.LastRequest = nil
	m.LastRecoveryRequest = nil
}

// ========================================
// BR-AI-082: Recovery Test Helpers
// ========================================

// WithRecoveryResponse configures the mock to return a specific response for recovery calls.
func (m *MockHolmesGPTClient) WithRecoveryResponse(resp *client.RecoveryResponse) *MockHolmesGPTClient {
	m.RecoveryResponse = resp
	m.RecoveryErr = nil
	return m
}

// WithRecoveryError configures the mock to return an error for recovery calls.
func (m *MockHolmesGPTClient) WithRecoveryError(err error) *MockHolmesGPTClient {
	m.RecoveryResponse = nil
	m.RecoveryErr = err
	return m
}

// WithRecoverySuccessResponse configures the mock to return a successful recovery response.
func (m *MockHolmesGPTClient) WithRecoverySuccessResponse(
	confidence float64,
	workflowID string,
	containerImage string,
	workflowConfidence float64,
	includeRecoveryAnalysis ...bool, // Optional parameter - defaults to false
) *MockHolmesGPTClient {
	// Build SelectedWorkflow as map[string]jx.Raw
	swMap := make(map[string]jx.Raw)
	if workflowID != "" {
		idBytes, _ := json.Marshal(workflowID)
		swMap["workflow_id"] = jx.Raw(idBytes)
		imgBytes, _ := json.Marshal(containerImage)
		swMap["container_image"] = jx.Raw(imgBytes)
		confBytes, _ := json.Marshal(workflowConfidence)
		swMap["confidence"] = jx.Raw(confBytes)
	}

	m.RecoveryResponse = &client.RecoveryResponse{
		IncidentID:         "mock-recovery-001",
		CanRecover:         true,
		AnalysisConfidence: confidence,
		Strategies:         []client.RecoveryStrategy{},
		Warnings:           []string{},
	}

	if len(swMap) > 0 {
		m.RecoveryResponse.SelectedWorkflow.SetTo(swMap)
	}

	// Only include recovery_analysis if explicitly requested
	if len(includeRecoveryAnalysis) > 0 && includeRecoveryAnalysis[0] {
		// Build RecoveryAnalysis with default mock data
		recoveryAnalysisMap := make(map[string]jx.Raw)
		stateChangedBytes, _ := json.Marshal(false)
		recoveryAnalysisMap["state_changed"] = jx.Raw(stateChangedBytes)
		currentSignalBytes, _ := json.Marshal("OOMKilled")
		recoveryAnalysisMap["current_signal_type"] = jx.Raw(currentSignalBytes)

		// Build PreviousAttemptAssessment as a nested map (not marshaled)
		prevAttemptMap := make(map[string]interface{})
		prevAttemptMap["failure_understood"] = true
		prevAttemptMap["failure_reason_analysis"] = "Previous workflow did not resolve memory leak"
		prevAttemptBytes, _ := json.Marshal(prevAttemptMap)
		recoveryAnalysisMap["previous_attempt_assessment"] = jx.Raw(prevAttemptBytes)

		if len(recoveryAnalysisMap) > 0 {
			m.RecoveryResponse.RecoveryAnalysis.SetTo(recoveryAnalysisMap)
		}
	}

	m.RecoveryErr = nil
	return m
}

// AssertRecoveryCalled returns an error if InvestigateRecovery was not called the expected number of times.
func (m *MockHolmesGPTClient) AssertRecoveryCalled(expectedCount int) error {
	if m.RecoveryCallCount != expectedCount {
		return fmt.Errorf("expected InvestigateRecovery to be called %d times, but was called %d times", expectedCount, m.RecoveryCallCount)
	}
	return nil
}

// AssertCalled returns an error if Investigate was not called the expected number of times.
func (m *MockHolmesGPTClient) AssertCalled(expectedCount int) error {
	if m.CallCount != expectedCount {
		return fmt.Errorf("expected Investigate to be called %d times, but was called %d times", expectedCount, m.CallCount)
	}
	return nil
}

// AssertNotCalled returns an error if Investigate was called.
func (m *MockHolmesGPTClient) AssertNotCalled() error {
	if m.CallCount > 0 {
		return fmt.Errorf("expected Investigate to not be called, but was called %d times", m.CallCount)
	}
	return nil
}

// ========================================
// BR-HAPI-197: Human Review Required Test Helpers
// ========================================

// WithHumanReviewRequired configures the mock to return needs_human_review=true
func (m *MockHolmesGPTClient) WithHumanReviewRequired(warnings []string) *MockHolmesGPTClient {
	m.Response = &client.IncidentResponse{
		IncidentID: "mock-incident-001",
		Analysis:   "Mock analysis: Human review required",
		Confidence: 0.5,
		Timestamp:  "2025-12-06T10:00:00Z",
		Warnings:   warnings,
	}
	m.Response.NeedsHumanReview.SetTo(true)
	m.Err = nil
	return m
}

// WithHumanReviewReasonEnum configures the mock to return needs_human_review=true with reason enum.
func (m *MockHolmesGPTClient) WithHumanReviewReasonEnum(reason string, warnings []string) *MockHolmesGPTClient {
	m.Response = &client.IncidentResponse{
		IncidentID: "mock-incident-001",
		Analysis:   "Mock analysis: Human review required",
		Confidence: 0.5,
		Timestamp:  "2025-12-06T10:00:00Z",
		Warnings:   warnings,
	}
	m.Response.NeedsHumanReview.SetTo(true)
	m.Response.HumanReviewReason.SetTo(client.HumanReviewReason(reason))
	m.Err = nil
	return m
}

// ========================================
// BR-HAPI-200: Problem Resolved Test Helpers
// ========================================

// WithProblemResolved configures the mock to return a "problem resolved" response.
// BR-HAPI-200 Outcome A: needs_human_review=false, selected_workflow=null, confidence >= 0.7
func (m *MockHolmesGPTClient) WithProblemResolved(confidence float64, warnings []string, analysis string) *MockHolmesGPTClient {
	m.Response = &client.IncidentResponse{
		IncidentID: "mock-incident-001",
		Analysis:   analysis,
		Confidence: confidence,
		Timestamp:  "2025-12-07T10:00:00Z",
		Warnings:   warnings,
	}
	m.Response.NeedsHumanReview.SetTo(false)
	// SelectedWorkflow left unset (null)
	m.Err = nil
	return m
}

// WithProblemResolvedAndRCA configures a "problem resolved" response with RCA context.
func (m *MockHolmesGPTClient) WithProblemResolvedAndRCA(confidence float64, warnings []string, analysis string, rcaSummary string, rcaSeverity string) *MockHolmesGPTClient {
	// Build RCA as map[string]jx.Raw
	rcaMap := make(map[string]jx.Raw)
	summaryBytes, _ := json.Marshal(rcaSummary)
	rcaMap["summary"] = jx.Raw(summaryBytes)
	sevBytes, _ := json.Marshal(rcaSeverity)
	rcaMap["severity"] = jx.Raw(sevBytes)
	// Add contributing factors for problem resolved with RCA
	contributingFactors := []string{"Temporary memory spike", "High traffic load"}
	cfBytes, _ := json.Marshal(contributingFactors)
	rcaMap["contributing_factors"] = jx.Raw(cfBytes)

	m.Response = &client.IncidentResponse{
		IncidentID:        "mock-incident-001",
		Analysis:          analysis,
		RootCauseAnalysis: rcaMap,
		Confidence:        confidence,
		Timestamp:         "2025-12-07T10:00:00Z",
		Warnings:          warnings,
	}
	m.Response.NeedsHumanReview.SetTo(false)
	m.Err = nil
	return m
}

// WithHumanReviewRequiredWithPartialResponse configures the mock to return
// needs_human_review=true with partial workflow/RCA data for operator context.
func (m *MockHolmesGPTClient) WithHumanReviewRequiredWithPartialResponse(
	reason string,
	warnings []string,
	workflowID string,
	containerImage string,
	rcaSummary string,
) *MockHolmesGPTClient {
	// Build RCA
	rcaMap := BuildMockRCA(rcaSummary, "medium", nil)

	// Build partial workflow
	swMap := BuildMockSelectedWorkflow(workflowID, containerImage, 0.5, "")

	m.Response = &client.IncidentResponse{
		IncidentID:        "mock-incident-001",
		Analysis:          "Mock analysis: Human review required",
		RootCauseAnalysis: rcaMap,
		Confidence:        0.5,
		Timestamp:         "2025-12-06T10:00:00Z",
		Warnings:          warnings,
	}
	m.Response.NeedsHumanReview.SetTo(true)
	m.Response.HumanReviewReason.SetTo(client.HumanReviewReason(reason))
	if len(swMap) > 0 {
		m.Response.SelectedWorkflow.SetTo(swMap)
	}
	m.Err = nil
	return m
}

// WithHumanReviewAndHistory configures a complete needs_human_review=true response
// with reason enum and validation attempts history (DD-HAPI-002 v1.4 compliant).
func (m *MockHolmesGPTClient) WithHumanReviewAndHistory(
	reason string,
	warnings []string,
	validationAttempts []map[string]interface{},
) *MockHolmesGPTClient {
	// Convert validation attempts to client.ValidationAttempt structs
	var history []client.ValidationAttempt
	for _, attempt := range validationAttempts {
		va := client.ValidationAttempt{
			Attempt:   int(attempt["attempt"].(int)),
			IsValid:   attempt["is_valid"].(bool),
			Timestamp: attempt["timestamp"].(string),
		}

		// Handle optional workflow_id
		if wfID, ok := attempt["workflow_id"].(string); ok && wfID != "" {
			va.WorkflowID = client.NewOptNilString(wfID)
		}

		// Handle errors array
		if errs, ok := attempt["errors"].([]string); ok {
			va.Errors = errs
		}

		history = append(history, va)
	}

	m.Response = &client.IncidentResponse{
		IncidentID:                "mock-incident-001",
		Analysis:                  "Mock analysis: Human review required after LLM self-correction",
		Confidence:                0.5,
		Timestamp:                 "2025-12-06T10:00:00Z",
		Warnings:                  warnings,
		ValidationAttemptsHistory: history,
	}
	m.Response.NeedsHumanReview.SetTo(true)
	m.Response.HumanReviewReason.SetTo(client.HumanReviewReason(reason))

	m.Err = nil
	return m
}

// ========================================
// Helper Functions for Building Generated Types
// ========================================

// BuildMockRCA creates a mock RootCauseAnalysis as map[string]jx.Raw
func BuildMockRCA(summary string, severity string, contributingFactors []string) map[string]jx.Raw {
	rcaMap := make(map[string]jx.Raw)

	if summary != "" {
		bytes, _ := json.Marshal(summary)
		rcaMap["summary"] = jx.Raw(bytes)
	}
	if severity != "" {
		bytes, _ := json.Marshal(severity)
		rcaMap["severity"] = jx.Raw(bytes)
	}
	if len(contributingFactors) > 0 {
		bytes, _ := json.Marshal(contributingFactors)
		rcaMap["contributing_factors"] = jx.Raw(bytes)
	}

	return rcaMap
}

// BuildMockSelectedWorkflow creates a mock SelectedWorkflow as map[string]jx.Raw
func BuildMockSelectedWorkflow(workflowID string, containerImage string, confidence float64, rationale string) map[string]jx.Raw {
	swMap := make(map[string]jx.Raw)

	if workflowID != "" {
		bytes, _ := json.Marshal(workflowID)
		swMap["workflow_id"] = jx.Raw(bytes)
	}
	if containerImage != "" {
		bytes, _ := json.Marshal(containerImage)
		swMap["container_image"] = jx.Raw(bytes)
	}
	if confidence > 0 {
		bytes, _ := json.Marshal(confidence)
		swMap["confidence"] = jx.Raw(bytes)
	}
	if rationale != "" {
		bytes, _ := json.Marshal(rationale)
		swMap["rationale"] = jx.Raw(bytes)
	}

	return swMap
}

// BuildMockRecoveryAnalysis creates a mock RecoveryAnalysis as map[string]jx.Raw
func BuildMockRecoveryAnalysis(failureUnderstood bool, failureReason string, stateChanged bool) map[string]jx.Raw {
	raMap := make(map[string]jx.Raw)

	// Build previous_attempt_assessment
	prevMap := map[string]interface{}{
		"failure_understood":      failureUnderstood,
		"failure_reason_analysis": failureReason,
	}
	prevBytes, _ := json.Marshal(prevMap)
	raMap["previous_attempt_assessment"] = jx.Raw(prevBytes)

	// Add state_changed if needed
	if stateChanged {
		bytes, _ := json.Marshal(stateChanged)
		raMap["state_changed"] = jx.Raw(bytes)
	}

	return raMap
}

// NewMockValidationAttempts creates mock validation attempts for testing.
// Each attempt represents a failed LLM self-correction iteration.
// Returns []map[string]interface{} for use with WithHumanReviewAndHistory
func NewMockValidationAttempts(failureScenarios []string) []map[string]interface{} {
	attempts := make([]map[string]interface{}, 0, len(failureScenarios))
	for i, scenario := range failureScenarios {
		attempts = append(attempts, map[string]interface{}{
			"attempt":     i + 1,
			"workflow_id": fmt.Sprintf("mock-workflow-attempt-%d", i+1),
			"is_valid":    false,
			"errors":      []string{scenario},
			"timestamp":   fmt.Sprintf("2025-12-06T10:00:%02dZ", i*5),
		})
	}
	return attempts
}
