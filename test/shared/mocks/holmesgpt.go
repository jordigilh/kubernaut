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
	"sync"

	"github.com/go-faster/jx"
	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// MockHolmesGPTClient is a mock implementation of HolmesGPTClientInterface for unit tests.
// Now uses generated types from HAPI OpenAPI spec for type-safe testing.
// BR-AI-006: Mock for API call testing
// BR-AA-HAPI-064: Extended with async session methods
type MockHolmesGPTClient struct {
	// InvestigateFunc allows tests to customize the Investigate behavior
	InvestigateFunc func(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)

	// CallCount tracks how many times Investigate was called
	CallCount int

	// LastRequest stores the last request passed to Investigate
	LastRequest *client.IncidentRequest

	// Response is the default response to return (if InvestigateFunc is nil)
	Response *client.IncidentResponse

	// Err is the default error to return
	Err error

	// ========================================
	// Async session fields (BR-AA-HAPI-064)
	// ========================================

	// SubmitInvestigationFunc allows tests to customize SubmitInvestigation behavior
	SubmitInvestigationFunc func(ctx context.Context, req *client.IncidentRequest) (string, error)

	// PollSessionFunc allows tests to customize PollSession behavior
	PollSessionFunc func(ctx context.Context, sessionID string) (*client.SessionStatus, error)

	// GetSessionResultFunc allows tests to customize GetSessionResult behavior
	GetSessionResultFunc func(ctx context.Context, sessionID string) (*client.IncidentResponse, error)

	// SubmitCallCount tracks how many times SubmitInvestigation was called
	SubmitCallCount int

	// PollCallCount tracks how many times PollSession was called
	PollCallCount int

	// GetResultCallCount tracks how many times GetSessionResult was called
	GetResultCallCount int

	// DefaultSessionID is returned by SubmitInvestigation when no func is set
	DefaultSessionID string

	// DefaultSessionStatus is returned by PollSession when no func is set
	DefaultSessionStatus *client.SessionStatus

	// SubmitErr is the error returned by SubmitInvestigation
	SubmitErr error

	// PollErr is the error returned by PollSession
	PollErr error

	// GetResultErr is the error returned by GetSessionResult
	GetResultErr error

	// mu protects concurrent access to call counts
	mu sync.Mutex
}

// NewMockHolmesGPTClient creates a new mock HolmesGPT client with default success behavior.
func NewMockHolmesGPTClient() *MockHolmesGPTClient {
	return &MockHolmesGPTClient{
		Response: &client.IncidentResponse{
			IncidentID: "mock-incident-001",
			Analysis:   "Mock analysis: No issues detected",
			Confidence: 0.8,
			Timestamp:  "2025-12-05T10:00:00Z",
			Warnings:   []string{},
		},
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
// ADR-055: targetInOwnerChain parameter removed - affectedResource is now in RCA output.
func (m *MockHolmesGPTClient) WithFullResponse(
	analysis string,
	confidence float64,
	warnings []string,
	rcaSummary string,
	rcaSeverity string,
	workflowID string,
	containerImage string,
	workflowConfidence float64,
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
		swMap["execution_bundle"] = jx.Raw(imgBytes)
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
			ExecutionBundle: client.NewOptNilString("kubernaut.io/workflows/scale:v1.0.0"),
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

	m.Err = nil
	return m
}

// ========================================
// Async Session Methods (BR-AA-HAPI-064)
// ========================================

// SubmitInvestigation implements HolmesGPTClientInterface for async submit.
func (m *MockHolmesGPTClient) SubmitInvestigation(ctx context.Context, req *client.IncidentRequest) (string, error) {
	m.mu.Lock()
	m.SubmitCallCount++
	m.LastRequest = req
	m.mu.Unlock()

	if m.SubmitInvestigationFunc != nil {
		return m.SubmitInvestigationFunc(ctx, req)
	}

	if m.SubmitErr != nil {
		return "", m.SubmitErr
	}

	sessionID := m.DefaultSessionID
	if sessionID == "" {
		sessionID = "mock-session-001"
	}
	return sessionID, nil
}

// PollSession implements HolmesGPTClientInterface for session polling.
func (m *MockHolmesGPTClient) PollSession(ctx context.Context, sessionID string) (*client.SessionStatus, error) {
	m.mu.Lock()
	m.PollCallCount++
	m.mu.Unlock()

	if m.PollSessionFunc != nil {
		return m.PollSessionFunc(ctx, sessionID)
	}

	if m.PollErr != nil {
		return nil, m.PollErr
	}

	if m.DefaultSessionStatus != nil {
		return m.DefaultSessionStatus, nil
	}

	return &client.SessionStatus{Status: "completed"}, nil
}

// GetSessionResult implements HolmesGPTClientInterface for result retrieval.
func (m *MockHolmesGPTClient) GetSessionResult(ctx context.Context, sessionID string) (*client.IncidentResponse, error) {
	m.mu.Lock()
	m.GetResultCallCount++
	m.mu.Unlock()

	if m.GetSessionResultFunc != nil {
		return m.GetSessionResultFunc(ctx, sessionID)
	}

	if m.GetResultErr != nil {
		return nil, m.GetResultErr
	}

	return m.Response, m.Err
}

// ========================================
// Async Session Test Helpers (BR-AA-HAPI-064)
// ========================================

// WithSessionSubmitResponse configures the mock to return a specific session ID on submit.
func (m *MockHolmesGPTClient) WithSessionSubmitResponse(sessionID string) *MockHolmesGPTClient {
	m.DefaultSessionID = sessionID
	m.SubmitErr = nil
	return m
}

// WithSessionSubmitError configures the mock to return an error on submit.
func (m *MockHolmesGPTClient) WithSessionSubmitError(err error) *MockHolmesGPTClient {
	m.SubmitErr = err
	return m
}

// WithSessionPollStatus configures the mock to return a specific session status on poll.
func (m *MockHolmesGPTClient) WithSessionPollStatus(status string) *MockHolmesGPTClient {
	m.DefaultSessionStatus = &client.SessionStatus{Status: status}
	m.PollErr = nil
	return m
}

// WithSessionPollError configures the mock to return an error on poll (e.g., 404 for session lost).
func (m *MockHolmesGPTClient) WithSessionPollError(err error) *MockHolmesGPTClient {
	m.PollErr = err
	return m
}

// WithSessionResultError configures the mock to return an error on result retrieval.
func (m *MockHolmesGPTClient) WithSessionResultError(err error) *MockHolmesGPTClient {
	m.GetResultErr = err
	return m
}

// WithSessionPollSequence configures the mock to return different statuses on consecutive polls.
// Useful for testing the poll flow: pending -> investigating -> completed.
func (m *MockHolmesGPTClient) WithSessionPollSequence(statuses []string) *MockHolmesGPTClient {
	callIndex := 0
	mu := &sync.Mutex{}
	m.PollSessionFunc = func(ctx context.Context, sessionID string) (*client.SessionStatus, error) {
		mu.Lock()
		defer mu.Unlock()
		if callIndex >= len(statuses) {
			return &client.SessionStatus{Status: statuses[len(statuses)-1]}, nil
		}
		status := statuses[callIndex]
		callIndex++
		return &client.SessionStatus{Status: status}, nil
	}
	return m
}

// WithSessionPollFailThenRecover configures the mock to return 404 for the first N polls,
// then succeed on subsequent polls. Used for testing session regeneration (IT-AA-064-003).
func (m *MockHolmesGPTClient) WithSessionPollFailThenRecover(failCount int, sessionLostErr error) *MockHolmesGPTClient {
	callIndex := 0
	mu := &sync.Mutex{}
	m.PollSessionFunc = func(ctx context.Context, sessionID string) (*client.SessionStatus, error) {
		mu.Lock()
		defer mu.Unlock()
		callIndex++
		if callIndex <= failCount {
			return nil, sessionLostErr
		}
		return &client.SessionStatus{Status: "completed"}, nil
	}
	return m
}

// Reset resets the mock's state (call count and last request).
func (m *MockHolmesGPTClient) Reset() {
	m.CallCount = 0
	m.LastRequest = nil
	m.SubmitCallCount = 0
	m.PollCallCount = 0
	m.GetResultCallCount = 0
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
		swMap["execution_bundle"] = jx.Raw(bytes)
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
