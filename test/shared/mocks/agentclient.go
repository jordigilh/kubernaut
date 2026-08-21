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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

// MockAgentClient is a mock implementation of handlers.AgentSessionGetOrCreator
// for unit tests (DD-AA-KA-001, BR-AA-KA-065.1). The retired ogen-generated
// agentclient.IncidentRequest/IncidentResponse types are gone -- GetOrCreate
// returns a plain agentsessionv1.AgentSession, and tests configure the
// Status.Phase/Result/Error the mock returns via the With* helpers below.
// The type name is kept (not renamed to e.g. MockAgentSessionGetOrCreator)
// to minimize churn across the ~14 existing call sites that already say
// `mocks.NewMockAgentClient()`.
type MockAgentClient struct {
	// GetOrCreateFunc allows tests to fully customize GetOrCreate behavior.
	GetOrCreateFunc func(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error)

	// AgentSession is returned by GetOrCreate when GetOrCreateFunc is nil and
	// Err is nil. Mutate its Status directly, or use the With* helpers.
	AgentSession *agentsessionv1.AgentSession

	// Err, when non-nil, is returned by GetOrCreate instead of AgentSession
	// (simulates a K8s API failure on Get/Create).
	Err error

	// CallCount tracks how many times GetOrCreate was called.
	CallCount int

	// LastRequest stores the last AIAnalysis passed to GetOrCreate.
	LastRequest *aianalysisv1.AIAnalysis

	// DeleteForRetryErr, when non-nil, is returned by DeleteForRetry.
	DeleteForRetryErr error

	// DeleteForRetryCallCount tracks how many times DeleteForRetry was called.
	DeleteForRetryCallCount int

	mu sync.Mutex
}

// NewMockAgentClient creates a new mock with a default Investigating-phase
// AgentSession (i.e. "still running, no result yet").
func NewMockAgentClient() *MockAgentClient {
	return &MockAgentClient{
		AgentSession: &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "as-mock-001",
				CreationTimestamp: metav1.Now(),
			},
			Status: agentsessionv1.AgentSessionStatus{
				Phase: agentsessionv1.AgentSessionPhaseInvestigating,
			},
		},
	}
}

// GetOrCreate implements handlers.AgentSessionGetOrCreator.
func (m *MockAgentClient) GetOrCreate(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error) {
	m.mu.Lock()
	m.CallCount++
	m.LastRequest = analysis
	m.mu.Unlock()

	if m.GetOrCreateFunc != nil {
		return m.GetOrCreateFunc(ctx, analysis)
	}
	if m.Err != nil {
		return nil, m.Err
	}
	return m.AgentSession, nil
}

// DeleteForRetry implements handlers.AgentSessionGetOrCreator (BR-AI-009,
// DD-AA-KA-001 amendment). The mock does not mutate AgentSession state --
// tests that exercise a full retry cycle configure the next GetOrCreate
// behavior separately via GetOrCreateFunc/WithPhase.
func (m *MockAgentClient) DeleteForRetry(ctx context.Context, as *agentsessionv1.AgentSession) error {
	m.mu.Lock()
	m.DeleteForRetryCallCount++
	m.mu.Unlock()
	return m.DeleteForRetryErr
}

// GetCallCount returns CallCount in a thread-safe manner.
func (m *MockAgentClient) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CallCount
}

// Reset clears call tracking state.
func (m *MockAgentClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallCount = 0
	m.LastRequest = nil
}

// AssertCalled returns an error if GetOrCreate was not called exactly expectedCount times.
func (m *MockAgentClient) AssertCalled(expectedCount int) error {
	if m.GetCallCount() != expectedCount {
		return fmt.Errorf("expected GetOrCreate to be called %d times, but was called %d times", expectedCount, m.GetCallCount())
	}
	return nil
}

// AssertNotCalled returns an error if GetOrCreate was called.
func (m *MockAgentClient) AssertNotCalled() error {
	if m.GetCallCount() > 0 {
		return fmt.Errorf("expected GetOrCreate to not be called, but was called %d times", m.GetCallCount())
	}
	return nil
}

// ========================================
// Phase/Result configuration helpers
// ========================================

// WithPhase sets the AgentSession's Status.Phase (e.g. Pending, Investigating).
func (m *MockAgentClient) WithPhase(phase agentsessionv1.AgentSessionPhase) *MockAgentClient {
	m.AgentSession.Status.Phase = phase
	return m
}

// WithInteractive marks the session interactive with the given acting user
// identity (DD-AA-KA-001 Amendment Gap 1: KA's dispatcher, not AA, decides
// this -- the mock just reports whatever Status the test configures).
func (m *MockAgentClient) WithInteractive(user string, groups []string) *MockAgentClient {
	m.AgentSession.Status.Interactive = true
	m.AgentSession.Status.ActingUser = user
	m.AgentSession.Status.ActingUserGroups = groups
	if m.AgentSession.Status.SessionID == "" {
		m.AgentSession.Status.SessionID = "mock-session-001"
	}
	return m
}

// WithResult marks the session Completed with the given result.
func (m *MockAgentClient) WithResult(res *agentsessionv1.AgentSessionResult) *MockAgentClient {
	m.AgentSession.Status.Phase = agentsessionv1.AgentSessionPhaseCompleted
	m.AgentSession.Status.Result = res
	return m
}

// WithFailed marks the session Failed with the given curated error message.
func (m *MockAgentClient) WithFailed(errMsg string) *MockAgentClient {
	m.AgentSession.Status.Phase = agentsessionv1.AgentSessionPhaseFailed
	m.AgentSession.Status.Error = errMsg
	return m
}

// WithFailedCapacityExceeded marks the session Failed with
// Status.Reason=AgentSessionReasonCapacityExceeded (BR-AI-009, DD-AA-KA-001
// amendment) -- KA's tag for a transient, self-resolving dispatch-capacity
// rejection, distinct from a genuine investigation failure.
func (m *MockAgentClient) WithFailedCapacityExceeded(errMsg string) *MockAgentClient {
	m.WithFailed(errMsg)
	m.AgentSession.Status.Reason = agentsessionv1.AgentSessionReasonCapacityExceeded
	return m
}

// WithCancelled marks the session Cancelled (interactive driver disconnected
// without a takeover, DD-AA-KA-001 Amendment).
func (m *MockAgentClient) WithCancelled() *MockAgentClient {
	m.AgentSession.Status.Phase = agentsessionv1.AgentSessionPhaseCancelled
	return m
}

// WithError configures GetOrCreate to return err instead of an AgentSession
// (simulates a K8s API failure, e.g. transient 5xx or a permanent 4xx).
func (m *MockAgentClient) WithError(err error) *MockAgentClient {
	m.Err = err
	return m
}

// WithCreatedAt overrides the AgentSession's CreationTimestamp, used by
// investigation-timeout tests to simulate an old, still-running session.
func (m *MockAgentClient) WithCreatedAt(t metav1.Time) *MockAgentClient {
	m.AgentSession.CreationTimestamp = t
	return m
}

// ========================================
// AgentSessionResult fixture builders
// BR-AI-008: full-fidelity fixtures for ResponseProcessor unit tests
// ========================================

// jsonRaw marshals v to *apiextensionsv1.JSON, panicking on failure (test-only helper).
func jsonRaw(v interface{}) *apiextensionsv1.JSON {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("jsonRaw: %v", err))
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

// BuildMockRCA creates a mock RootCauseAnalysis as raw JSON.
func BuildMockRCA(summary string, severity string, contributingFactors []string) *apiextensionsv1.JSON {
	m := map[string]interface{}{}
	if summary != "" {
		m["summary"] = summary
	}
	if severity != "" {
		m["severity"] = severity
	}
	if len(contributingFactors) > 0 {
		m["contributing_factors"] = contributingFactors
	}
	if len(m) == 0 {
		return nil
	}
	return jsonRaw(m)
}

// BuildMockSelectedWorkflow creates a mock SelectedWorkflow as raw JSON.
func BuildMockSelectedWorkflow(workflowID string, containerImage string, confidence float64, rationale string) *apiextensionsv1.JSON {
	m := map[string]interface{}{}
	if workflowID != "" {
		m["workflow_id"] = workflowID
	}
	if containerImage != "" {
		m["execution_bundle"] = containerImage
	}
	if confidence > 0 {
		m["confidence"] = confidence
	}
	if rationale != "" {
		m["rationale"] = rationale
	}
	if len(m) == 0 {
		return nil
	}
	return jsonRaw(m)
}

// NewMockAgentSessionResult builds a minimal successful AgentSessionResult.
func NewMockAgentSessionResult() *agentsessionv1.AgentSessionResult {
	return &agentsessionv1.AgentSessionResult{
		IncidentID: "mock-incident-001",
		Analysis:   "Mock analysis: No issues detected",
		Confidence: 0.8,
		Timestamp:  "2025-12-05T10:00:00Z",
		Warnings:   []string{},
	}
}

// WithSuccessResponse configures the mock to return a successful investigation result.
func (m *MockAgentClient) WithSuccessResponse(analysis string, confidence float64, warnings []string) *MockAgentClient {
	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID: "mock-incident-001",
		Analysis:   analysis,
		Confidence: confidence,
		Timestamp:  "2025-12-05T10:00:00Z",
		Warnings:   warnings,
	})
}

// WithFullResponse configures the mock to return a complete result including RCA and workflow.
// ADR-055: targetInOwnerChain parameter removed - remediationTarget is now in RCA output.
func (m *MockAgentClient) WithFullResponse(
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
) *MockAgentClient {
	res := &agentsessionv1.AgentSessionResult{
		IncidentID:        "mock-incident-001",
		Analysis:          analysis,
		RootCauseAnalysis: BuildMockRCA(rcaSummary, rcaSeverity, nil),
		Confidence:        confidence,
		Timestamp:         "2025-12-05T10:00:00Z",
		Warnings:          warnings,
		SelectedWorkflow:  BuildMockSelectedWorkflow(workflowID, containerImage, workflowConfidence, workflowRationale),
	}
	if includeAlternatives && workflowID != "" {
		res.AlternativeWorkflows = []agentsessionv1.AgentSessionAlternativeWorkflow{{
			WorkflowID:      "wf-scale-deployment",
			Confidence:      0.75,
			Rationale:       "Consider scaling deployment for resource pressure",
			ExecutionBundle: "kubernaut.io/workflows/scale:v1.0.0",
		}}
	}
	return m.WithResult(res)
}

// WithHumanReviewRequired configures the mock to return needs_human_review=true.
func (m *MockAgentClient) WithHumanReviewRequired(warnings []string) *MockAgentClient {
	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID:       "mock-incident-001",
		Analysis:         "Mock analysis: Human review required",
		Confidence:       0.5,
		Timestamp:        "2025-12-06T10:00:00Z",
		Warnings:         warnings,
		NeedsHumanReview: true,
	})
}

// WithHumanReviewReasonEnum configures the mock to return needs_human_review=true with reason enum.
func (m *MockAgentClient) WithHumanReviewReasonEnum(reason string, warnings []string) *MockAgentClient {
	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID:        "mock-incident-001",
		Analysis:          "Mock analysis: Human review required",
		Confidence:        0.5,
		Timestamp:         "2025-12-06T10:00:00Z",
		Warnings:          warnings,
		NeedsHumanReview:  true,
		HumanReviewReason: reason,
	})
}

// WithProblemResolved configures the mock to return a "problem resolved" result.
// BR-KA-200 Outcome A: needs_human_review=false, selected_workflow=null, confidence >= 0.7
func (m *MockAgentClient) WithProblemResolved(confidence float64, warnings []string, analysis string) *MockAgentClient {
	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID:       "mock-incident-001",
		Analysis:         analysis,
		Confidence:       confidence,
		Timestamp:        "2025-12-07T10:00:00Z",
		Warnings:         warnings,
		NeedsHumanReview: false,
	})
}

// WithNotActionable configures the mock to return a "not actionable" result.
// #388 Outcome D: actionable=false, needs_human_review=false, selected_workflow=null, confidence >= 0.7
func (m *MockAgentClient) WithNotActionable(confidence float64, rcaSummary string, rcaSeverity string, contributingFactors []string) *MockAgentClient {
	notActionable := false
	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID:        "mock-incident-001",
		Analysis:          rcaSummary,
		RootCauseAnalysis: BuildMockRCA(rcaSummary, rcaSeverity, contributingFactors),
		Confidence:        confidence,
		Timestamp:         "2025-12-07T10:00:00Z",
		Warnings:          []string{"Alert not actionable \u2014 no remediation warranted"},
		NeedsHumanReview:  false,
		IsActionable:      &notActionable,
	})
}

// WithProblemResolvedAndRCA configures a "problem resolved" result with RCA context.
func (m *MockAgentClient) WithProblemResolvedAndRCA(confidence float64, warnings []string, analysis string, rcaSummary string, rcaSeverity string) *MockAgentClient {
	contributingFactors := []string{"Temporary memory spike", "High traffic load"}
	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID:        "mock-incident-001",
		Analysis:          analysis,
		RootCauseAnalysis: BuildMockRCA(rcaSummary, rcaSeverity, contributingFactors),
		Confidence:        confidence,
		Timestamp:         "2025-12-07T10:00:00Z",
		Warnings:          warnings,
		NeedsHumanReview:  false,
	})
}

// WithHumanReviewRequiredWithPartialResponse configures the mock to return
// needs_human_review=true with partial workflow/RCA data for operator context.
func (m *MockAgentClient) WithHumanReviewRequiredWithPartialResponse(
	reason string,
	warnings []string,
	workflowID string,
	containerImage string,
	rcaSummary string,
) *MockAgentClient {
	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID:        "mock-incident-001",
		Analysis:          "Mock analysis: Human review required",
		RootCauseAnalysis: BuildMockRCA(rcaSummary, "medium", nil),
		Confidence:        0.5,
		Timestamp:         "2025-12-06T10:00:00Z",
		Warnings:          warnings,
		NeedsHumanReview:  true,
		HumanReviewReason: reason,
		SelectedWorkflow:  BuildMockSelectedWorkflow(workflowID, containerImage, 0.5, ""),
	})
}

// WithHumanReviewAndHistory configures a complete needs_human_review=true result
// with reason enum and validation attempts history (DD-KA-001 v1.4 compliant).
func (m *MockAgentClient) WithHumanReviewAndHistory(
	reason string,
	warnings []string,
	validationAttempts []map[string]interface{},
) *MockAgentClient {
	history := make([]agentsessionv1.AgentSessionValidationAttempt, 0, len(validationAttempts))
	for _, attempt := range validationAttempts {
		va := agentsessionv1.AgentSessionValidationAttempt{
			Attempt:   attempt["attempt"].(int),
			IsValid:   attempt["is_valid"].(bool),
			Timestamp: attempt["timestamp"].(string),
		}
		if wfID, ok := attempt["workflow_id"].(string); ok {
			va.WorkflowID = wfID
		}
		if errs, ok := attempt["errors"].([]string); ok {
			va.Errors = errs
		}
		history = append(history, va)
	}

	return m.WithResult(&agentsessionv1.AgentSessionResult{
		IncidentID:                "mock-incident-001",
		Analysis:                  "Mock analysis: Human review required after LLM self-correction",
		Confidence:                0.5,
		Timestamp:                 "2025-12-06T10:00:00Z",
		Warnings:                  warnings,
		NeedsHumanReview:          true,
		HumanReviewReason:         reason,
		ValidationAttemptsHistory: history,
	})
}

// ========================================
// Helper Functions
// ========================================

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
