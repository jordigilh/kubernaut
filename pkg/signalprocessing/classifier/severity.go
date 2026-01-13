/*
Copyright 2026 Jordi Gil.

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

// Package classifier provides severity determination business logic.
//
// # Business Requirements
//
// BR-SP-105: Severity Determination via Rego Policy
//
// # Design Decisions
//
// DD-SEVERITY-001: Severity Determination Refactoring - Strategy B (Policy-Defined Fallback)
// See: docs/handoff/DD_SEVERITY_001_STRATEGY_B_DECISION_JAN13_2026.md
//
// # Strategy B Implementation
//
// Operators MUST define fallback behavior in Rego policy.
// System does NOT impose "unknown" fallback.
// Policy must return: "critical", "warning", or "info"
//
// Example Conservative Policy:
//
//	determine_severity := "critical" {
//	    input.signal.severity == "Sev1"
//	} else := "warning" {
//	    input.signal.severity == "Sev2"
//	} else := "critical" {  # Operator-defined fallback
//	    true
//	}
package classifier

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/rego"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

// SeverityClassifier determines normalized severity from external severity values.
// BR-SP-105: Severity Determination via Rego Policy
// DD-SEVERITY-001: Strategy B - Policy-defined fallback (operator control)
type SeverityClassifier struct {
	client       client.Client
	logger       logr.Logger
	policyModule string // Compiled Rego policy
	policyPath   string // Path to policy file for hot-reload
	fileWatcher  *hotreload.FileWatcher
	mu           sync.RWMutex
}

// SeverityResult contains the determined severity and source attribution.
type SeverityResult struct {
	// Severity is the normalized value: "critical", "warning", or "info"
	Severity string
	// Source indicates how severity was determined: "rego-policy"
	Source string
	// PolicyHash is the SHA256 hash of the Rego policy used (for audit trail)
	PolicyHash string
}

// NewSeverityClassifier creates a new severity classifier.
// BR-SP-105: Severity determination via operator-defined Rego policy.
func NewSeverityClassifier(client client.Client, logger logr.Logger) *SeverityClassifier {
	return &SeverityClassifier{
		client: client,
		logger: logger.WithName("severity"),
	}
}

// LoadRegoPolicy loads and validates a Rego policy for severity determination.
// DD-SEVERITY-001 Strategy B: Policy must define fallback behavior (no system default).
func (c *SeverityClassifier) LoadRegoPolicy(policyContent string) error {
	// Validate Rego syntax before loading
	if err := c.validatePolicy(policyContent); err != nil {
		return fmt.Errorf("policy validation failed: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.policyModule = policyContent
	c.logger.Info("Severity Rego policy loaded", "policySize", len(policyContent))
	return nil
}

// validatePolicy checks if the policy compiles successfully.
func (c *SeverityClassifier) validatePolicy(policyContent string) error {
	// Try to compile the policy to check syntax
	_, err := rego.New(
		rego.Query("data.signalprocessing.severity.determine_severity"),
		rego.Module("severity.rego", policyContent),
	).PrepareForEval(context.Background())

	return err
}

// ClassifySeverity determines normalized severity using Rego policy.
// DD-SEVERITY-001 Strategy B: Policy MUST return severity (no system fallback to "unknown").
func (c *SeverityClassifier) ClassifySeverity(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (*SeverityResult, error) {
	c.mu.RLock()
	policyModule := c.policyModule
	c.mu.RUnlock()

	// No policy loaded → error (policy is mandatory)
	if policyModule == "" {
		return nil, fmt.Errorf("no policy loaded - severity determination requires Rego policy")
	}

	// Build Rego input
	input := map[string]interface{}{
		"signal": map[string]interface{}{
			"severity": sp.Spec.Signal.Severity,
			"type":     sp.Spec.Signal.Type,
			"source":   sp.Spec.Signal.Source,
		},
	}

	// Evaluate policy
	r := rego.New(
		rego.Query("data.signalprocessing.severity.determine_severity"),
		rego.Module("severity.rego", policyModule),
		rego.Input(input),
		rego.StrictBuiltinErrors(true),
		rego.EnablePrintStatements(false),
	)

	rs, err := r.Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("rego evaluation failed: %w", err)
	}

	// Policy must return a result
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, fmt.Errorf("no severity determined by policy for input %q - add else clause to policy for unmapped values (e.g., } else := \"critical\" { true })", sp.Spec.Signal.Severity)
	}

	// Extract severity from result
	severityValue, ok := rs[0].Expressions[0].Value.(string)
	if !ok {
		return nil, fmt.Errorf("policy returned non-string severity: %T", rs[0].Expressions[0].Value)
	}

	// Validate severity is valid enum
	if !isValidSeverity(severityValue) {
		return nil, fmt.Errorf("policy returned invalid severity %q - must be critical/warning/info", severityValue)
	}

	return &SeverityResult{
		Severity:   severityValue,
		Source:     "rego-policy",
		PolicyHash: c.GetPolicyHash(),
	}, nil
}

// isValidSeverity checks if the severity is a valid enum value.
func isValidSeverity(severity string) bool {
	switch severity {
	case "critical", "warning", "info":
		return true
	default:
		return false
	}
}

// StartHotReload starts watching the policy file for changes.
// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify.
// Pattern follows existing environment/priority/customlabels classifiers.
func (c *SeverityClassifier) StartHotReload(ctx context.Context) error {
	if c.policyPath == "" {
		return fmt.Errorf("policy path not set - cannot start hot-reload")
	}

	var err error
	c.fileWatcher, err = hotreload.NewFileWatcher(
		c.policyPath,
		func(content string) error {
			// Validate and load new policy
			if err := c.LoadRegoPolicy(content); err != nil {
				return fmt.Errorf("policy validation failed: %w", err)
			}

			c.logger.Info("Severity policy hot-reloaded successfully",
				"hash", c.GetPolicyHash())
			return nil
		},
		c.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	return c.fileWatcher.Start(ctx)
}

// Stop gracefully stops the hot-reloader.
func (c *SeverityClassifier) Stop() {
	if c.fileWatcher != nil {
		c.fileWatcher.Stop()
	}
}

// GetPolicyHash returns the current policy hash (for audit/debugging).
func (c *SeverityClassifier) GetPolicyHash() string {
	if c.fileWatcher != nil {
		return c.fileWatcher.GetLastHash()
	}
	return ""
}

// SetPolicyPath sets the path to the policy file for hot-reload.
// Must be called before StartHotReload().
func (c *SeverityClassifier) SetPolicyPath(path string) {
	c.policyPath = path
}
