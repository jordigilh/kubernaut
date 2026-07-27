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

// IdentityInput carries the acting user's identity for Rego policy evaluation
// during interactive sessions (BR-INTERACTIVE-001, BR-AI-085). When nil, the
// flow is autonomous (alert-driven) and Rego policies should default to
// require_approval := true.
type IdentityInput struct {
	User   string   `json:"user"`
	Groups []string `json:"groups,omitempty"`
}

// SignalContextInput carries signal-source metadata (from SignalProcessing)
// used by Rego policies for environment/severity/priority-based approval
// rules. Grouped out of PolicyInput (Go Anti-Pattern Checklist "God struct"
// remediation, #247 follow-up) — these four fields are always populated
// together from AnalysisRequest.SignalContext.
type SignalContextInput struct {
	SignalType       string `json:"signal_type"`
	Severity         string `json:"severity"`
	Environment      string `json:"environment"`
	BusinessPriority string `json:"business_priority"`
}

// ClassificationInput carries label and business-classification metadata
// used by Rego policies for classification-based approval rules. Grouped
// out of PolicyInput (Go Anti-Pattern Checklist "God struct" remediation,
// #247 follow-up) — these fields are all sourced from EnrichmentResults/
// post-RCA context, distinct from the KA investigation response.
type ClassificationInput struct {
	// DetectedLabels (ADR-056: computed by KA post-RCA)
	DetectedLabels map[string]interface{} `json:"detected_labels"`

	// CustomLabels (customer-defined via Rego)
	CustomLabels map[string][]string `json:"custom_labels"`

	// BusinessClassification from SP categorization (BR-SP-002, BR-SP-080, BR-SP-081)
	BusinessClassification map[string]string `json:"business_classification,omitempty"`
}

// KAResponseInput carries fields populated from the Kubernaut Agent's (KA)
// investigation/RCA response. Grouped out of PolicyInput (Go Anti-Pattern
// Checklist "God struct" remediation, #247 follow-up).
type KAResponseInput struct {
	Confidence float64  `json:"confidence"`
	Warnings   []string `json:"warnings,omitempty"`

	// FailedDetections (DD-WORKFLOW-001 v2.1)
	FailedDetections []string `json:"failed_detections,omitempty"`
}

// PolicyInput represents input to Rego policy
// Per IMPLEMENTATION_PLAN_V1.0.md lines 1756-1785 (ApprovalInput schema)
// Fields align with AIAnalysis status fields captured by InvestigatingHandler
//
// Field groups (SignalContext/Classification/KAResponse) are Go-side
// organization only — buildRegoInputMap still flattens them to the same
// top-level Rego input keys (e.g. input.environment, input.confidence), so
// this restructuring does not change the Rego policy contract. Introduced
// to keep the top-level field count away from the "God struct" threshold
// (AGENTS.md Go Anti-Pattern Checklist) as the schema has grown across many
// BRs (#225, #774, #247).
type PolicyInput struct {
	// SignalContext carries signal-source metadata (from SignalProcessing).
	SignalContext SignalContextInput `json:"signal_context"`

	// TargetResource identifies the resource the signal was raised against.
	TargetResource TargetResourceInput `json:"target_resource"`

	// Classification carries label/business-classification metadata.
	Classification ClassificationInput `json:"classification"`

	// KAResponse carries fields from the Kubernaut Agent's investigation response.
	KAResponse KAResponseInput `json:"ka_response"`

	// ADR-055: Remediation target identified by LLM during RCA
	// Replaces target_in_owner_chain boolean with structured resource data
	RemediationTarget *RemediationTargetInput `json:"remediation_target,omitempty"`

	// #225: Operator-configurable confidence threshold for auto-approval.
	// When nil, the Rego policy uses its built-in default (0.8).
	// Stepping stone toward BR-HAPI-198 (V1.1 rule-based thresholds).
	ConfidenceThreshold *float64 `json:"confidence_threshold,omitempty"`

	// Identity carries the acting user's identity for interactive sessions.
	// nil for autonomous (alert-driven) flows. When set, Rego policies can
	// access input.identity.user and input.identity.groups for role-based
	// approval decisions (BR-AI-085, #774).
	Identity *IdentityInput `json:"identity,omitempty"`

	// ActionType is the catalog action type selected for remediation
	// (DD-WORKFLOW-016 taxonomy, e.g. ScaleReplicas, ProvisionNode).
	// Unlike RemediationTarget.Kind, this is catalog-authoritative (sourced
	// from the action_type_taxonomy table via KA's three-step discovery),
	// not LLM-inferred, enabling reliable gating on infrastructure-impacting
	// actions regardless of which resource the LLM reports as the
	// remediation target (BR-AI-085 FR-AI-085-006, #247).
	ActionType string `json:"action_type"`
}

// TargetResourceInput contains target resource identification
type TargetResourceInput struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// RemediationTargetInput is the LLM-identified target resource for Rego policy evaluation.
// ADR-055: Replaces target_in_owner_chain boolean with structured resource data,
// enabling granular per-kind approval policies.
type RemediationTargetInput struct {
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
	query, degradedResult := e.resolveCompiledQuery(ctx)
	if degradedResult != nil {
		return degradedResult, nil
	}

	inputMap := buildRegoInputMap(input)

	results, err := query.Eval(ctx, rego.EvalInput(inputMap))
	if err != nil {
		// BR-AI-014: Graceful degradation - evaluation error
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Policy evaluation error: %v - defaulting to manual approval", err),
			Degraded:         true,
		}, nil
	}

	return extractPolicyResult(results), nil
}

// resolveCompiledQuery returns the cached compiled policy (per ADR-050),
// eliminating 2-5ms of file I/O + compilation overhead per call. If no
// policy is cached (StartHotReload() was not called, e.g. legacy tests),
// it falls back to reading and compiling the policy file inline. On
// failure, it returns a non-nil degraded PolicyResult per BR-AI-014
// graceful degradation. Extracted from Evaluate (Wave 6 6c GREEN: funlen
// remediation) — pure code motion, no behavior change.
func (e *Evaluator) resolveCompiledQuery(ctx context.Context) (rego.PreparedEvalQuery, *PolicyResult) {
	e.mu.RLock()
	query := e.compiledQuery
	e.mu.RUnlock()

	// BR-AI-014: Graceful degradation if no policy loaded
	// This can only happen if StartHotReload() was not called
	// (e.g., in legacy tests that don't use hot-reload)
	if query != (rego.PreparedEvalQuery{}) {
		return query, nil
	}

	// Fallback: Read policy file (legacy behavior for backward compatibility)
	policyContent, err := os.ReadFile(e.policyPath)
	if err != nil {
		return query, &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Policy file not found: %v - defaulting to manual approval", err),
			Degraded:         true,
		}
	}

	// Compile policy (legacy path - only for backward compatibility)
	query, err = rego.New(
		rego.Query("data.aianalysis.approval"),
		rego.Module("approval.rego", string(policyContent)),
	).PrepareForEval(ctx)
	if err != nil {
		return query, &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Policy compilation failed: %v - defaulting to manual approval", err),
			Degraded:         true,
		}
	}

	return query, nil
}

// buildRegoInputMap builds the Rego evaluation input map (matches
// PolicyInput JSON tags). Extracted from Evaluate (Wave 6 6c GREEN: funlen
// remediation) — pure code motion, no behavior change.
func buildRegoInputMap(input *PolicyInput) map[string]interface{} {
	inputMap := map[string]interface{}{
		// Signal context
		"signal_type":       input.SignalContext.SignalType,
		"severity":          input.SignalContext.Severity,
		"environment":       input.SignalContext.Environment,
		"business_priority": input.SignalContext.BusinessPriority,
		// Target resource
		"target_resource": map[string]interface{}{
			"kind":      input.TargetResource.Kind,
			"name":      input.TargetResource.Name,
			"namespace": input.TargetResource.Namespace,
		},
		// Detected labels
		"detected_labels": input.Classification.DetectedLabels,
		"custom_labels":   input.Classification.CustomLabels,
		// Business classification (BR-SP-002)
		"business_classification": input.Classification.BusinessClassification,
		// KA response data
		"confidence": input.KAResponse.Confidence,
		"warnings":   input.KAResponse.Warnings,
		// ADR-055: Remediation target for granular per-kind policies
		"remediation_target": input.RemediationTarget,
		// FailedDetections
		"failed_detections": input.KAResponse.FailedDetections,
		// #247: Catalog-authoritative action type for infrastructure-action gating
		"action_type": input.ActionType,
	}

	// #225: Only include confidence_threshold when explicitly configured.
	// Omitting it lets the Rego policy's default (0.8) apply.
	if input.ConfidenceThreshold != nil {
		inputMap["confidence_threshold"] = *input.ConfidenceThreshold
	}

	// #774: Include identity when present (interactive sessions).
	// Absent for autonomous (alert-driven) flows so Rego policies can
	// distinguish with `default require_approval := true`.
	if input.Identity != nil {
		inputMap["identity"] = map[string]interface{}{
			"user":   input.Identity.User,
			"groups": input.Identity.Groups,
		}
	}

	return inputMap
}

// extractPolicyResult extracts the approval decision from the raw Rego
// evaluation results, applying BR-AI-014 graceful-degradation defaults
// when the result shape is missing or unexpected. Extracted from Evaluate
// (Wave 6 6c GREEN: funlen remediation) — pure code motion, no behavior
// change.
func extractPolicyResult(results rego.ResultSet) *PolicyResult {
	// Check for results
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		// BR-AI-014: Graceful degradation - no result
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           "No policy result - defaulting to manual approval",
			Degraded:         true,
		}
	}

	// Extract result map
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		// BR-AI-014: Graceful degradation - invalid result format
		return &PolicyResult{
			ApprovalRequired: true,
			Reason:           fmt.Sprintf("Invalid policy result format (got %T) - defaulting to manual approval", results[0].Expressions[0].Value),
			Degraded:         true,
		}
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
	}
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
		func(content string) error { //nolint:contextcheck // policy hot-reload callback fires asynchronously on file-change events, independent of any request
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
