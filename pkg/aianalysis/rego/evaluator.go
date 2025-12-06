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

// Package rego provides Rego policy evaluation for AIAnalysis approval decisions.
// BR-AI-011: Policy evaluation
// BR-AI-014: Graceful degradation
// DD-AIANALYSIS-001: Follows Gateway pattern for Rego policy loading
package rego

import (
	"context"
	"fmt"
	"os"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Config for Rego evaluator
type Config struct {
	// PolicyPath is the path to the approval.rego policy file
	PolicyPath string
}

// PolicyInput represents input to Rego policy
// Per IMPLEMENTATION_PLAN_V1.0.md lines 1756-1785 (ApprovalInput schema)
// Fields align with AIAnalysis status fields captured by InvestigatingHandler
type PolicyInput struct {
	// Signal context (from SignalProcessing)
	SignalType       string `json:"signal_type"`
	Severity         string `json:"severity"`
	Environment      string `json:"environment"`
	BusinessPriority string `json:"business_priority"`

	// Target resource
	TargetResource TargetResourceInput `json:"target_resource"`

	// Detected labels (auto-detected by SignalProcessing)
	DetectedLabels map[string]interface{} `json:"detected_labels"`

	// Custom labels (customer-defined via Rego)
	CustomLabels map[string][]string `json:"custom_labels"`

	// HolmesGPT-API response data
	Confidence         float64  `json:"confidence"`
	TargetInOwnerChain bool     `json:"target_in_owner_chain"`
	Warnings           []string `json:"warnings,omitempty"`

	// FailedDetections (DD-WORKFLOW-001 v2.1)
	FailedDetections []string `json:"failed_detections,omitempty"`

	// Recovery context
	IsRecoveryAttempt     bool `json:"is_recovery_attempt"`
	RecoveryAttemptNumber int  `json:"recovery_attempt_number,omitempty"`
}

// TargetResourceInput contains target resource identification
type TargetResourceInput struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// PolicyResult represents Rego policy evaluation result
type PolicyResult struct {
	// ApprovalRequired indicates if human approval is needed
	ApprovalRequired bool
	// Reason explains why approval is/isn't required
	Reason string
	// Degraded indicates if evaluation used fallback due to policy errors
	Degraded bool
}

// EvaluatorInterface for dependency injection in tests
type EvaluatorInterface interface {
	Evaluate(ctx context.Context, input *PolicyInput) (*PolicyResult, error)
}

// Evaluator evaluates Rego policies for approval decisions
// BR-AI-011: Policy evaluation
type Evaluator struct {
	policyPath string
}

// NewEvaluator creates a new Rego evaluator
// DD-AIANALYSIS-001: Follows Gateway pattern
func NewEvaluator(cfg Config) *Evaluator {
	return &Evaluator{
		policyPath: cfg.PolicyPath,
	}
}

// Evaluate evaluates the approval policy
// BR-AI-011: Policy evaluation
// BR-AI-014: Graceful degradation on errors
func (e *Evaluator) Evaluate(ctx context.Context, input *PolicyInput) (*PolicyResult, error) {
	// Read policy file
	policyContent, err := os.ReadFile(e.policyPath)
	if err != nil {
		// BR-AI-014: Graceful degradation - policy file not found
		return &PolicyResult{
			ApprovalRequired: true, // Safe default
			Reason:           fmt.Sprintf("Policy file not found: %v - defaulting to manual approval", err),
			Degraded:         true,
		}, nil
	}

	// Prepare Rego query
	query, err := rego.New(
		rego.Query("data.aianalysis.approval"),
		rego.Module("approval.rego", string(policyContent)),
	).PrepareForEval(ctx)

	if err != nil {
		// BR-AI-014: Graceful degradation - policy syntax error
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Policy compilation failed: %v - defaulting to manual approval", err),
			Degraded:         true,
		}, nil
	}

	// Build input map for Rego (matches PolicyInput JSON tags)
	inputMap := map[string]interface{}{
		// Signal context
		"signal_type":       input.SignalType,
		"severity":          input.Severity,
		"environment":       input.Environment,
		"business_priority": input.BusinessPriority,
		// Target resource
		"target_resource": map[string]interface{}{
			"kind":      input.TargetResource.Kind,
			"name":      input.TargetResource.Name,
			"namespace": input.TargetResource.Namespace,
		},
		// Detected labels
		"detected_labels": input.DetectedLabels,
		"custom_labels":   input.CustomLabels,
		// HolmesGPT-API response data
		"confidence":            input.Confidence,
		"target_in_owner_chain": input.TargetInOwnerChain,
		"warnings":              input.Warnings,
		// FailedDetections
		"failed_detections": input.FailedDetections,
		// Recovery context
		"is_recovery_attempt":     input.IsRecoveryAttempt,
		"recovery_attempt_number": input.RecoveryAttemptNumber,
	}

	// Evaluate policy
	results, err := query.Eval(ctx, rego.EvalInput(inputMap))
	if err != nil {
		// BR-AI-014: Graceful degradation - evaluation error
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Policy evaluation error: %v - defaulting to manual approval", err),
			Degraded:         true,
		}, nil
	}

	// Check for results
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		// BR-AI-014: Graceful degradation - no result
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           "No policy result - defaulting to manual approval",
			Degraded:         true,
		}, nil
	}

	// Extract result map
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		// BR-AI-014: Graceful degradation - invalid result format
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Invalid policy result format (got %T) - defaulting to manual approval", results[0].Expressions[0].Value),
			Degraded:         true,
		}, nil
	}

	// Extract approval decision
	approvalRequired, foundApproval := resultMap["require_approval"].(bool)
	reason, foundReason := resultMap["reason"].(string)

	// If require_approval key doesn't exist, use safe default (true)
	if !foundApproval {
		approvalRequired = true
	}
	if !foundReason {
		if approvalRequired {
			reason = "Approval required by policy"
		} else {
			reason = "Auto-approved by policy"
		}
	}

	return &PolicyResult{
		ApprovalRequired: approvalRequired,
		Reason:           reason,
		Degraded:         false,
	}, nil
}
