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

package processing

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"
)

// PriorityEngine assigns priority based on severity and environment using Rego policies
//
// Priority assignment affects:
// - Notification routing (P0 → PagerDuty, P1 → Slack, P2 → email)
// - Remediation urgency (P0 → immediate, P1 → within 1 hour, P2 → best effort)
// - Resource allocation (P0 gets higher CPU/memory quotas for remediation jobs)
//
// Priority levels:
// - P0: Critical production issues (immediate response required)
// - P1: High priority issues (response within 1 hour)
// - P2: Normal priority issues (best effort)
// - P3: Low priority issues (background processing)
// //
// Design Rationale:
// - ✅ Policy-driven configuration (not code-driven)
// - ✅ Organizations define their own priority rules via Rego
// - ✅ ConfigMap-based policy updates (no redeployment)
// - ✅ Testable policies (unit tests for Rego)
type PriorityEngine struct {
	// regoQuery is the Rego policy evaluator (REQUIRED)
	regoQuery *rego.PreparedEvalQuery

	logger logr.Logger
}

// NewPriorityEngineWithRego creates a new priority engine with Rego policy
//
// The Rego policy is loaded from the specified file path and evaluated for each
// priority assignment. If Rego evaluation fails, returns default priority "P2".
//
// Architecture Decision: NO hardcoded priority tables in code.
// All priority rules MUST be defined in Rego policies.
//
// Rego policy query: data.kubernaut.gateway.priority.priority
//
// Expected Rego input:
//
//	{
//	  "severity": "critical",
//	  "environment": "production",
//	  "labels": {...}
//	}
//
// Expected Rego output: "P0", "P1", "P2", or "P3"
//
// Fallback behavior: If Rego evaluation fails, returns "P2" (safe default)
func NewPriorityEngineWithRego(policyPath string, logger logr.Logger) (*PriorityEngine, error) {
	// Load Rego policy from file
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Rego policy file: %w", err)
	}

	// Prepare Rego query
	query, err := rego.New(
		rego.Query("data.kubernaut.gateway.priority.priority"),
		rego.Module("priority.rego", string(policyContent)),
	).PrepareForEval(context.Background())

	if err != nil {
		return nil, fmt.Errorf("failed to prepare Rego policy: %w", err)
	}

	logger.Info("Rego policy loaded successfully for priority assignment",
		"policy_path", policyPath,
	)

	return &PriorityEngine{
		regoQuery: &query,
		logger:    logger,
	}, nil
}

// Assign determines priority based on severity and environment using Rego policy
//
// Architecture Decision: NO hardcoded priority logic in code.
// All priority rules MUST be defined in Rego policies.
//
// Assignment flow:
// 1. Evaluate Rego policy with severity + environment
// 2. On success: return Rego result (P0/P1/P2/P3)
// 3. On error: return "P2" (safe default)
//
// Examples (assuming standard Rego policy):
// - Assign(ctx, "critical", "production") → "P0"
// - Assign(ctx, "warning", "production") → "P1"
// - Assign(ctx, "warning", "staging") → "P2"
// - Assign(ctx, "info", "production") → "P2"
//
// Returns:
// - string: "P0", "P1", "P2", or "P3"
func (p *PriorityEngine) Assign(ctx context.Context, severity, environment string) string {
	// Evaluate Rego policy (REQUIRED - no fallback table)
	priority, err := p.evaluateRego(ctx, severity, environment)
	if err == nil {
		p.logger.V(1).Info("Priority assigned via Rego policy",
			"severity", severity,
			"environment", environment,
			"priority", priority,
			"source", "rego",
		)
		return priority
	}

	// Rego evaluation failed - return safe default
	p.logger.Error(err, "Rego policy evaluation failed, using safe default P2",
		"severity", severity,
		"environment", environment)

	return "P2"
}

// evaluateRego evaluates Rego policy for priority assignment
//
// This method is a placeholder for future Rego integration.
// When implemented, it will:
// 1. Prepare input data (severity, environment, other context)
// 2. Evaluate Rego policy
// 3. Extract priority from result
// 4. Validate priority value (P0/P1/P2)
//
// Example Rego policy:
//
//	package types.priority
//
//	# Critical production alerts get P0
//	priority := "P0" {
//	    input.severity == "critical"
//	    input.environment == "prod"
//	}
//
//	# Critical staging alerts get P1
//	priority := "P1" {
//	    input.severity == "critical"
//	    input.environment == "staging"
//	}
//
//	# Warning production alerts get P1
//	priority := "P1" {
//	    input.severity == "warning"
//	    input.environment == "prod"
//	}
//
//	# Default to P2
//	priority := "P2" { true }
//
// Returns:
// - string: Priority ("P0", "P1", "P2")
// - error: Rego evaluation errors (policy not found, invalid result, etc.)
func (p *PriorityEngine) evaluateRego(ctx context.Context, severity, environment string) (string, error) {
	// Prepare input for Rego policy
	input := map[string]interface{}{
		"severity":    severity,
		"environment": environment,
		"labels":      map[string]string{}, // Empty for now, can be extended
	}

	// Evaluate Rego query
	results, err := p.regoQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return "", fmt.Errorf("rego evaluation error: %w", err)
	}

	// Check if results exist
	if len(results) == 0 {
		return "", fmt.Errorf("rego evaluation returned no results")
	}

	// Extract priority from first result
	if len(results[0].Expressions) == 0 {
		return "", fmt.Errorf("rego evaluation returned no expressions")
	}

	priority, ok := results[0].Expressions[0].Value.(string)
	if !ok {
		return "", fmt.Errorf("rego evaluation returned non-string priority: %T", results[0].Expressions[0].Value)
	}

	// Validate priority value
	validPriorities := map[string]bool{
		"P0": true,
		"P1": true,
		"P2": true,
		"P3": true,
	}

	if !validPriorities[priority] {
		return "", fmt.Errorf("rego evaluation returned invalid priority: %s (expected P0/P1/P2/P3)", priority)
	}

	return priority, nil
}
