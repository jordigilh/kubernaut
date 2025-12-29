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

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// MockRegoEvaluator is a mock implementation of rego.EvaluatorInterface for unit tests.
// It allows tests to control Rego policy evaluation behavior without requiring real policies.
// BR-AI-011: Mock for policy evaluation testing
type MockRegoEvaluator struct {
	// EvaluateFunc allows tests to customize the Evaluate behavior
	EvaluateFunc func(ctx context.Context, input *rego.PolicyInput) (*rego.PolicyResult, error)

	// CallCount tracks how many times Evaluate was called
	CallCount int

	// LastInput stores the last input passed to Evaluate
	LastInput *rego.PolicyInput

	// Result is the default result to return (if EvaluateFunc is nil)
	Result *rego.PolicyResult

	// Err is the default error to return (if EvaluateFunc is nil)
	Err error
}

// NewMockRegoEvaluator creates a new mock Rego evaluator with default behavior.
// Default: Returns approvalRequired=false (auto-approve)
func NewMockRegoEvaluator() *MockRegoEvaluator {
	return &MockRegoEvaluator{
		Result: &rego.PolicyResult{
			ApprovalRequired: false,
			Reason:           "Auto-approved by test policy",
			Degraded:         false,
		},
	}
}

// Evaluate implements rego.EvaluatorInterface.
func (m *MockRegoEvaluator) Evaluate(ctx context.Context, input *rego.PolicyInput) (*rego.PolicyResult, error) {
	m.CallCount++
	m.LastInput = input

	if m.EvaluateFunc != nil {
		return m.EvaluateFunc(ctx, input)
	}

	return m.Result, m.Err
}

// WithResult configures the mock to return a specific result.
func (m *MockRegoEvaluator) WithResult(result *rego.PolicyResult) *MockRegoEvaluator {
	m.Result = result
	m.Err = nil
	return m
}

// WithError configures the mock to return an error.
func (m *MockRegoEvaluator) WithError(err error) *MockRegoEvaluator {
	m.Result = nil
	m.Err = err
	return m
}

// WithApprovalRequired configures the mock to require approval.
// BR-AI-013: For testing approval-required scenarios
func (m *MockRegoEvaluator) WithApprovalRequired(reason string) *MockRegoEvaluator {
	m.Result = &rego.PolicyResult{
		ApprovalRequired: true,
		Reason:           reason,
		Degraded:         false,
	}
	m.Err = nil
	return m
}

// WithAutoApprove configures the mock to auto-approve.
func (m *MockRegoEvaluator) WithAutoApprove(reason string) *MockRegoEvaluator {
	m.Result = &rego.PolicyResult{
		ApprovalRequired: false,
		Reason:           reason,
		Degraded:         false,
	}
	m.Err = nil
	return m
}

// WithDegradedMode configures the mock to indicate degraded mode.
// BR-AI-014: For testing policy failure fallback
func (m *MockRegoEvaluator) WithDegradedMode(reason string) *MockRegoEvaluator {
	m.Result = &rego.PolicyResult{
		ApprovalRequired: true, // Safe default when degraded
		Reason:           reason,
		Degraded:         true,
	}
	m.Err = nil
	return m
}

// Reset resets the mock's state (call count and last input).
func (m *MockRegoEvaluator) Reset() {
	m.CallCount = 0
	m.LastInput = nil
}

// AssertCalled returns an error if Evaluate was not called the expected number of times.
func (m *MockRegoEvaluator) AssertCalled(expectedCount int) error {
	if m.CallCount != expectedCount {
		return fmt.Errorf("expected Evaluate to be called %d times, but was called %d times", expectedCount, m.CallCount)
	}
	return nil
}

// AssertNotCalled returns an error if Evaluate was called.
func (m *MockRegoEvaluator) AssertNotCalled() error {
	if m.CallCount > 0 {
		return fmt.Errorf("expected Evaluate to not be called, but was called %d times", m.CallCount)
	}
	return nil
}

// AssertInputEnvironment checks if the last input has the expected environment.
func (m *MockRegoEvaluator) AssertInputEnvironment(expectedEnv string) error {
	if m.LastInput == nil {
		return fmt.Errorf("no input was provided")
	}
	if m.LastInput.Environment != expectedEnv {
		return fmt.Errorf("expected environment %q, got %q", expectedEnv, m.LastInput.Environment)
	}
	return nil
}
