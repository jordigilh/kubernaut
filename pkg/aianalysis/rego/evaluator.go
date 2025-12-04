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

// Package rego provides OPA Rego policy evaluation for AIAnalysis approval decisions.
// Design Decision: DD-WORKFLOW-001 v2.2 - Approval policy evaluation
// Business Requirement: BR-AI-011, BR-AI-013, BR-AI-014
package rego

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"
)

const (
	// DefaultTimeout is the default timeout for Rego policy evaluation.
	DefaultTimeout = 5 * time.Second

	// DefaultPolicyQuery is the Rego query path for approval decisions.
	DefaultPolicyQuery = "data.aianalysis.approval"
)

// Config for Rego evaluator
type Config struct {
	// PolicyDir is the directory containing Rego policies
	PolicyDir string
	// Query is the Rego query path (default: "data.aianalysis.approval")
	Query string
	// Timeout for policy evaluation
	Timeout time.Duration
}

// PolicyInput represents input to Rego approval policy.
// DD-WORKFLOW-001 v2.2: Input schema for approval determination.
type PolicyInput struct {
	// Environment from SignalContext (e.g., "production", "staging")
	Environment string `json:"environment"`
	// TargetInOwnerChain indicates if RCA target was found in OwnerChain
	TargetInOwnerChain bool `json:"target_in_owner_chain"`
	// DetectedLabels are auto-detected cluster characteristics
	DetectedLabels map[string]interface{} `json:"detected_labels"`
	// CustomLabels are customer-defined labels from Rego policies
	CustomLabels map[string][]string `json:"custom_labels"`
	// FailedDetections lists fields where detection failed (RBAC, timeout, etc.)
	FailedDetections []string `json:"failed_detections"`
	// Warnings from HolmesGPT-API investigation
	Warnings []string `json:"warnings"`
}

// PolicyResult represents Rego policy evaluation result.
type PolicyResult struct {
	// ApprovalRequired indicates if manual approval is needed
	ApprovalRequired bool
	// Reason explains the approval decision
	Reason string
	// Degraded indicates if the result is from graceful degradation
	Degraded bool
}

// EvaluatorInterface defines the interface for Rego evaluation.
// This allows mocking in tests.
type EvaluatorInterface interface {
	Evaluate(ctx context.Context, input *PolicyInput) (*PolicyResult, error)
}

// Evaluator evaluates Rego policies for approval decisions.
// BR-AI-011: Policy evaluation
// BR-AI-014: Graceful degradation
type Evaluator struct {
	policyDir     string
	query         string
	timeout       time.Duration
	preparedQuery *rego.PreparedEvalQuery
	logger        logr.Logger
}

// NewEvaluator creates a new Rego evaluator.
func NewEvaluator(cfg Config, logger logr.Logger) *Evaluator {
	query := cfg.Query
	if query == "" {
		query = DefaultPolicyQuery
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &Evaluator{
		policyDir: cfg.PolicyDir,
		query:     query,
		timeout:   timeout,
		logger:    logger.WithName("rego-evaluator"),
	}
}

// Evaluate evaluates the approval policy.
// BR-AI-011: Policy evaluation
// BR-AI-014: Graceful degradation - returns safe default on errors
func (e *Evaluator) Evaluate(ctx context.Context, input *PolicyInput) (*PolicyResult, error) {
	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Try to prepare query if not already done
	if e.preparedQuery == nil {
		if err := e.prepareQuery(ctx); err != nil {
			// Graceful degradation: policy load failed
			e.logger.Error(err, "Failed to load Rego policy, defaulting to manual approval")
			return &PolicyResult{
				ApprovalRequired: true, // Safe default
				Reason:           fmt.Sprintf("Policy evaluation failed: %v - defaulting to manual approval", err),
				Degraded:         true,
			}, nil
		}
	}

	// Evaluate policy
	results, err := e.preparedQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		e.logger.Error(err, "Rego evaluation error, defaulting to manual approval")
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Policy evaluation error: %v", err),
			Degraded:         true,
		}, nil
	}

	// Parse results
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		e.logger.Info("No policy result, defaulting to manual approval")
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           "No policy result - defaulting to manual approval",
			Degraded:         true,
		}, nil
	}

	// Extract approval decision from result
	return e.parseResult(results[0].Expressions[0].Value)
}

// prepareQuery loads and prepares the Rego query.
func (e *Evaluator) prepareQuery(ctx context.Context) error {
	// Check if policy directory exists
	if _, err := os.Stat(e.policyDir); os.IsNotExist(err) {
		return fmt.Errorf("policy directory does not exist: %s", e.policyDir)
	}

	// Find all .rego files in directory
	policyFiles, err := filepath.Glob(filepath.Join(e.policyDir, "*.rego"))
	if err != nil {
		return fmt.Errorf("failed to find policy files: %w", err)
	}

	if len(policyFiles) == 0 {
		return fmt.Errorf("no policy files found in %s", e.policyDir)
	}

	// Prepare Rego query
	prepared, err := rego.New(
		rego.Query(e.query),
		rego.Load(policyFiles, nil),
	).PrepareForEval(ctx)

	if err != nil {
		return fmt.Errorf("failed to prepare Rego policy: %w", err)
	}

	e.preparedQuery = &prepared
	e.logger.Info("Rego policy loaded successfully",
		"query", e.query,
		"policy_files", len(policyFiles))

	return nil
}

// parseResult extracts the approval decision from Rego result.
func (e *Evaluator) parseResult(rawResult interface{}) (*PolicyResult, error) {
	// Expected format: map[string]interface{} with require_approval and reason
	resultMap, ok := rawResult.(map[string]interface{})
	if !ok {
		e.logger.Info("Invalid policy result format, defaulting to manual approval",
			"result_type", fmt.Sprintf("%T", rawResult))
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           "Invalid policy result format",
			Degraded:         true,
		}, nil
	}

	// Extract approval decision
	approvalRequired, _ := resultMap["require_approval"].(bool)
	reason, _ := resultMap["reason"].(string)

	e.logger.V(1).Info("Policy evaluation complete",
		"approval_required", approvalRequired,
		"reason", reason)

	return &PolicyResult{
		ApprovalRequired: approvalRequired,
		Reason:           reason,
		Degraded:         false,
	}, nil
}

