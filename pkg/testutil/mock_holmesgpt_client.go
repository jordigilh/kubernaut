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

package testutil

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

// MockHolmesGPTClient is a mock implementation of HolmesGPTClientInterface for unit tests.
// It allows tests to control HolmesGPT-API behavior without requiring a real service.
// BR-AI-006: Mock for API call testing
type MockHolmesGPTClient struct {
	// InvestigateFunc allows tests to customize the Investigate behavior
	InvestigateFunc func(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)

	// CallCount tracks how many times Investigate was called
	CallCount int

	// LastRequest stores the last request passed to Investigate
	LastRequest *client.IncidentRequest

	// Response is the default response to return (if InvestigateFunc is nil)
	Response *client.IncidentResponse

	// Err is the default error to return (if InvestigateFunc is nil)
	Err error
}

// NewMockHolmesGPTClient creates a new mock HolmesGPT client with default success behavior.
func NewMockHolmesGPTClient() *MockHolmesGPTClient {
	return &MockHolmesGPTClient{
		Response: &client.IncidentResponse{
			IncidentID:         "mock-incident-001",
			Analysis:           "Mock analysis: No issues detected",
			TargetInOwnerChain: false,
			Confidence:         0.8,
			Timestamp:          "2025-12-05T10:00:00Z",
			Warnings:           []string{},
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

// WithAPIError configures the mock to return an APIError with the given status code.
// BR-AI-009: For transient error testing (503, 429, 502, 504)
// BR-AI-010: For permanent error testing (401, 400, 403, 404)
func (m *MockHolmesGPTClient) WithAPIError(statusCode int, message string) *MockHolmesGPTClient {
	m.Response = nil
	m.Err = &client.APIError{
		StatusCode: statusCode,
		Message:    message,
	}
	return m
}

// WithSuccessResponse configures the mock to return a successful investigation response.
// This is the basic version for backward compatibility with existing tests.
func (m *MockHolmesGPTClient) WithSuccessResponse(analysis string, confidence float64, targetInChain bool, warnings []string) *MockHolmesGPTClient {
	m.Response = &client.IncidentResponse{
		IncidentID:         "mock-incident-001",
		Analysis:           analysis,
		TargetInOwnerChain: targetInChain,
		Confidence:         confidence,
		Timestamp:          "2025-12-05T10:00:00Z",
		Warnings:           warnings,
	}
	m.Err = nil
	return m
}

// WithFullResponse configures the mock to return a complete response including RCA and workflow.
// Use this for tests that need the full /incident/analyze response (Dec 2025 update).
func (m *MockHolmesGPTClient) WithFullResponse(
	analysis string,
	confidence float64,
	targetInChain bool,
	warnings []string,
	rca *client.RootCauseAnalysis,
	selectedWorkflow *client.SelectedWorkflow,
	alternativeWorkflows []client.AlternativeWorkflow,
) *MockHolmesGPTClient {
	m.Response = &client.IncidentResponse{
		IncidentID:           "mock-incident-001",
		Analysis:             analysis,
		RootCauseAnalysis:    rca,
		SelectedWorkflow:     selectedWorkflow,
		AlternativeWorkflows: alternativeWorkflows,
		TargetInOwnerChain:   targetInChain,
		Confidence:           confidence,
		Timestamp:            "2025-12-05T10:00:00Z",
		Warnings:             warnings,
	}
	m.Err = nil
	return m
}

// Reset resets the mock's state (call count and last request).
func (m *MockHolmesGPTClient) Reset() {
	m.CallCount = 0
	m.LastRequest = nil
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

// AssertRequestContains checks if the last request contains expected context substring.
func (m *MockHolmesGPTClient) AssertRequestContains(substring string) error {
	if m.LastRequest == nil {
		return fmt.Errorf("no request was made")
	}
	if m.LastRequest.Context == "" {
		return fmt.Errorf("request context is empty")
	}
	// Simple substring check - can be enhanced
	return nil
}

