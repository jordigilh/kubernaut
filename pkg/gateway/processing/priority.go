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

	"github.com/open-policy-agent/opa/v1/rego"
	"go.uber.org/zap"
)

// PriorityEngine assigns priority based on severity and environment
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
//
// Assignment methods:
// 1. Rego policy evaluation (optional, for complex rules)
// 2. Fallback table (always available, simple severity + environment mapping)
//
// Design Decision: Rego is fully supported in V1
// - Rego policy evaluation for flexible business rules
// - Fallback table ensures system works if Rego fails
// - ConfigMap-based policy updates (no code changes)
type PriorityEngine struct {
	// regoQuery is the optional Rego policy evaluator
	// If nil, fallback table is always used
	// If not nil, Rego is evaluated first, fallback table on error
	regoQuery *rego.PreparedEvalQuery

	// fallbackTable maps severity → environment → priority
	// This table is ALWAYS available as a fallback
	fallbackTable map[string]map[string]string

	logger *zap.Logger
}

// NewPriorityEngine creates a new priority engine with fallback table
//
// Fallback table (severity → environment → priority):
//
//	┌──────────┬────────────┬─────────┬─────────────┬──────────────┐
//	│ Severity │ production │ staging │ development │ unknown (*)  │
//	├──────────┼────────────┼─────────┼─────────────┼──────────────┤
//	│ critical │     P0     │   P1    │     P2      │     P1       │
//	│ warning  │     P1     │   P2    │     P2      │     P2       │
//	│ info     │     P2     │   P2    │     P2      │     P3       │
//	└──────────┴────────────┴─────────┴─────────────┴──────────────┘
//
// Rationale:
// - Critical + production: Immediate response (P0)
// - Critical + staging: High priority testing env (P1)
// - Critical + development: Normal priority for development (P2)
// - Critical + unknown: Treat as important but not production (P1)
// - Warning + production: High priority monitoring (P1)
// - Warning + unknown: Moderate priority (P2)
// - Info + unknown: Low priority (P3)
//
// Catch-all logic (*):
// - Enables dynamic environment configuration (canary, qa-eu, blue, green, etc.)
// - Organizations can use ANY environment taxonomy without code changes
// - Unknown environments get sensible priority based on severity
func NewPriorityEngine(logger *zap.Logger) *PriorityEngine {
	// Build fallback table with catch-all for unknown environments
	fallbackTable := map[string]map[string]string{
		"critical": {
			"production":  "P0",
			"staging":     "P1",
			"development": "P2",
			"*":           "P1", // Catch-all: critical in unknown environment → P1
		},
		"warning": {
			"production":  "P1",
			"staging":     "P2",
			"development": "P2",
			"*":           "P2", // Catch-all: warning in unknown environment → P2
		},
		"info": {
			"production":  "P2",
			"staging":     "P2",
			"development": "P2",
			"*":           "P3", // Catch-all: info in unknown environment → P3
		},
	}

	return &PriorityEngine{
		regoQuery:     nil, // No Rego policy, use fallback only
		fallbackTable: fallbackTable,
		logger:        logger,
	}
}

// NewPriorityEngineWithRego creates a new priority engine with Rego policy support
//
// The Rego policy is loaded from the specified file path and evaluated for each
// priority assignment. If Rego evaluation fails, the fallback table is used.
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
func NewPriorityEngineWithRego(policyPath string, logger *zap.Logger) (*PriorityEngine, error) {
	// Build fallback table (same as NewPriorityEngine) with catch-all for unknown environments
	fallbackTable := map[string]map[string]string{
		"critical": {
			"production":  "P0",
			"staging":     "P1",
			"development": "P2",
			"*":           "P1", // Catch-all: critical in unknown environment → P1
		},
		"warning": {
			"production":  "P1",
			"staging":     "P2",
			"development": "P2",
			"*":           "P2", // Catch-all: warning in unknown environment → P2
		},
		"info": {
			"production":  "P2",
			"staging":     "P2",
			"development": "P2",
			"*":           "P3", // Catch-all: info in unknown environment → P3
		},
	}

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
		zap.Any("policy_path", policyPath),
	)

	return &PriorityEngine{
		regoQuery:     &query,
		fallbackTable: fallbackTable,
		logger:        logger,
	}, nil
}

// Assign determines priority based on severity and environment
//
// Assignment flow:
// 1. If Rego evaluator configured:
//   - Try Rego policy evaluation
//   - On success: return Rego result
//   - On error: log warning, fall through to fallback table
//
// 2. Use fallback table lookup
// 3. If no mapping found: return "P2" (safe default)
//
// Examples:
// - Assign(ctx, "critical", "prod") → "P0"
// - Assign(ctx, "warning", "staging") → "P2"
// - Assign(ctx, "info", "dev") → "P2"
// - Assign(ctx, "unknown", "prod") → "P2" (default)
//
// Returns:
// - string: "P0", "P1", or "P2"
func (p *PriorityEngine) Assign(ctx context.Context, severity, environment string) string {
	// 1. Try Rego evaluation (if configured)
	if p.regoQuery != nil {
		if priority, err := p.evaluateRego(ctx, severity, environment); err == nil {
			p.logger.Debug("Priority assigned via Rego policy",
				zap.Any("severity", severity),
				zap.Any("environment", environment),
				zap.Any("priority", priority),
				zap.String("source", "rego"),
			)
			return priority
		} else {
			// Log Rego failure but continue to fallback
			p.logger.Warn("Rego policy evaluation failed, using fallback table",
				zap.Any("severity", severity),
				zap.Any("environment", environment),
				zap.Error(err),
			)
		}
	}

	// 2. Use fallback table
	if envMap, ok := p.fallbackTable[severity]; ok {
		// Try exact environment match first
		if priority, ok := envMap[environment]; ok {
			p.logger.Debug("Priority assigned via fallback table (exact match)",
				zap.Any("severity", severity),
				zap.Any("environment", environment),
				zap.Any("priority", priority),
				zap.String("source", "fallback_table_exact"),
			)
			return priority
		}

		// Try catch-all if exact match not found
		if priority, ok := envMap["*"]; ok {
			p.logger.Debug("Priority assigned via fallback table (catch-all for unknown environment)",
				zap.Any("severity", severity),
				zap.Any("environment", environment),
				zap.Any("priority", priority),
				zap.String("source", "fallback_table_catchall"),
			)
			return priority
		}
	}

	// 3. Final fallback (safe default - should rarely be reached)
	p.logger.Warn("No priority mapping found (unknown severity), defaulting to P2",
		zap.Any("severity", severity),
		zap.Any("environment", environment),
	)

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

// GetFallbackTable returns the fallback table (for testing/debugging)
//
// Returns:
// - map[string]map[string]string: severity → environment → priority mapping
func (p *PriorityEngine) GetFallbackTable() map[string]map[string]string {
	return p.fallbackTable
}
