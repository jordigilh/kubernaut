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
	"sync"

	"github.com/go-logr/logr"
	rego "github.com/open-policy-agent/opa/v1/rego"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
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

	// Detected labels (ADR-056: computed by HAPI post-RCA)
	DetectedLabels map[string]interface{} `json:"detected_labels"`

	// Custom labels (customer-defined via Rego)
	CustomLabels map[string][]string `json:"custom_labels"`

	// Business classification from SP categorization (BR-SP-002, BR-SP-080, BR-SP-081)
	BusinessClassification map[string]string `json:"business_classification,omitempty"`

	// HolmesGPT-API response data
	Confidence float64  `json:"confidence"`
	Warnings   []string `json:"warnings,omitempty"`

	// ADR-055: Affected resource identified by LLM during RCA
	// Replaces target_in_owner_chain boolean with structured resource data
	AffectedResource *AffectedResourceInput `json:"affected_resource,omitempty"`

	// FailedDetections (DD-WORKFLOW-001 v2.1)
	FailedDetections []string `json:"failed_detections,omitempty"`
}

// TargetResourceInput contains target resource identification
type TargetResourceInput struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// AffectedResourceInput is the LLM-identified target resource for Rego policy evaluation.
// ADR-055: Replaces target_in_owner_chain boolean with structured resource data,
// enabling granular per-kind approval policies.
type AffectedResourceInput struct {
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
// ADR-050: Configuration Validation Strategy
// DD-AIANALYSIS-002: Rego Policy Startup Validation
type Evaluator struct {
	policyPath    string
	logger        logr.Logger
	fileWatcher   *hotreload.FileWatcher
	compiledQuery rego.PreparedEvalQuery // ✅ Cached compiled policy (per ADR-050)
	mu            sync.RWMutex
}

// NewEvaluator creates a new Rego evaluator
// DD-AIANALYSIS-001: Follows Gateway pattern
// ADR-050: Configuration Validation Strategy
func NewEvaluator(cfg Config, logger logr.Logger) *Evaluator {
	return &Evaluator{
		policyPath: cfg.PolicyPath,
		logger:     logger.WithName("rego"),
	}
}

// Evaluate evaluates the approval policy
// BR-AI-011: Policy evaluation
// BR-AI-014: Graceful degradation on errors
// ADR-050: Uses cached compiled policy (no file I/O or compilation overhead)
func (e *Evaluator) Evaluate(ctx context.Context, input *PolicyInput) (*PolicyResult, error) {
	// ✅ REFACTOR: Use cached compiled policy (per ADR-050)
	// Eliminates 2-5ms overhead (file I/O + compilation) per call
	e.mu.RLock()
	query := e.compiledQuery
	e.mu.RUnlock()

	// BR-AI-014: Graceful degradation if no policy loaded
	// This can only happen if StartHotReload() was not called
	// (e.g., in legacy tests that don't use hot-reload)
	if query == (rego.PreparedEvalQuery{}) {
		// Fallback: Read policy file (legacy behavior for backward compatibility)
		policyContent, err := os.ReadFile(e.policyPath)
		if err != nil {
			return &PolicyResult{
				ApprovalRequired: true,
				Reason:           fmt.Sprintf("Policy file not found: %v - defaulting to manual approval", err),
				Degraded:         true,
			}, nil
		}

		// Compile policy (legacy path - only for backward compatibility)
		query, err = rego.New(
			rego.Query("data.aianalysis.approval"),
			rego.Module("approval.rego", string(policyContent)),
		).PrepareForEval(ctx)

		if err != nil {
			return &PolicyResult{
				ApprovalRequired: true,
				Reason:           fmt.Sprintf("Policy compilation failed: %v - defaulting to manual approval", err),
				Degraded:         true,
			}, nil
		}
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
		// Business classification (BR-SP-002)
		"business_classification": input.BusinessClassification,
		// HolmesGPT-API response data
		"confidence": input.Confidence,
		"warnings":   input.Warnings,
		// ADR-055: Affected resource for granular per-kind policies
		"affected_resource": input.AffectedResource,
		// FailedDetections
		"failed_detections": input.FailedDetections,
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

// ========================================
// STARTUP VALIDATION + HOT-RELOAD (ADR-050)
// ========================================
// Per ADR-050: Fail-fast on startup, gracefully degrade at runtime
// Per DD-AIANALYSIS-002: Rego Policy Startup Validation

// StartHotReload starts watching the policy file for changes.
// Per ADR-050: Invalid policy at startup causes service to fail (exit 1).
// Per DD-INFRA-001: Uses shared FileWatcher component.
func (e *Evaluator) StartHotReload(ctx context.Context) error {
	var err error
	e.fileWatcher, err = hotreload.NewFileWatcher(
		e.policyPath,
		func(content string) error {
			// Validate and load new policy
			// Per ADR-050: Graceful degradation on runtime validation failure
			if err := e.LoadPolicy(content); err != nil {
				return fmt.Errorf("policy validation failed: %w", err)
			}

			e.logger.Info("Approval policy hot-reloaded successfully",
				"hash", e.GetPolicyHash())
			return nil
		},
		e.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Start() loads initial policy and validates
	// Per ADR-050: Fails fast if initial policy is invalid
	return e.fileWatcher.Start(ctx)
}

// LoadPolicy validates and caches compiled policy.
// Per ADR-050: Validates Rego syntax before loading for graceful degradation.
func (e *Evaluator) LoadPolicy(policyContent string) error {
	// Validate Rego syntax by compiling
	query, err := rego.New(
		rego.Query("data.aianalysis.approval"),
		rego.Module("approval.rego", policyContent),
	).PrepareForEval(context.Background())

	if err != nil {
		return fmt.Errorf("policy compilation failed: %w", err)
	}

	// Cache compiled policy
	e.mu.Lock()
	e.compiledQuery = query
	e.mu.Unlock()

	e.logger.V(1).Info("Rego policy loaded", "policySize", len(policyContent))
	return nil
}

// Stop gracefully stops the hot-reloader.
// Per ADR-050: Clean shutdown of FileWatcher.
func (e *Evaluator) Stop() {
	if e.fileWatcher != nil {
		e.fileWatcher.Stop()
	}
}

// GetPolicyHash returns the current policy hash (for monitoring/debugging).
// Per DD-INFRA-001: SHA256 hash tracking for audit/debugging.
func (e *Evaluator) GetPolicyHash() string {
	if e.fileWatcher != nil {
		return e.fileWatcher.GetLastHash()
	}
	return ""
}
