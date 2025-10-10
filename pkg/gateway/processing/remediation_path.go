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
	"sync"

	"github.com/open-policy-agent/opa/rego"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// RemediationPathDecider determines the remediation strategy based on environment and priority
//
// Remediation Paths:
// - **Aggressive**: Immediate automated remediation (kubectl apply, pod deletion, auto-rollback)
//   - Use case: P0 production outages, P0 development (fast feedback)
//   - Risk: Higher risk of unintended changes, but acceptable for critical issues
//
// - **Moderate**: Validation + automated execution (dry-run → validate → execute)
//   - Use case: P0 staging, P1 staging, P1 development
//   - Risk: Balanced risk/speed, validation catches issues before execution
//
// - **Conservative**: GitOps PR + manual approval (AI generates PR, human approves)
//   - Use case: P1/P2 production, unknown environments
//   - Risk: Lowest risk, human review prevents unintended changes
//
// - **Manual**: Analysis only, no automated remediation (AI provides recommendations)
//   - Use case: P2 staging/dev, invalid priorities, missing data
//   - Risk: Zero automation risk, requires operator action
//
// Business Value:
// - Prevents AI from being too aggressive in production (risk management)
// - Enables fast iteration in dev/staging (developer productivity)
// - Provides flexible rules via Rego policies (organizational customization)
type RemediationPathDecider struct {
	// regoEvaluator is the optional Rego policy evaluator
	// If nil, fallback table is always used
	regoEvaluator RegoEvaluator

	// fallbackTable maps (environment, priority) → path
	fallbackTable map[string]map[string]string

	// cache stores path decisions to avoid redundant Rego evaluations
	// Key: cacheKey(environment, priority)
	// Value: remediation path
	cache map[string]string
	mu    sync.RWMutex // Protects cache

	logger *logrus.Logger
}

// RegoEvaluator interface abstracts Rego policy evaluation
//
// This interface allows:
// - Testing with mock Rego evaluators
// - Future integration with real OPA Rego
// - Graceful fallback when Rego unavailable
type RegoEvaluator interface {
	// Evaluate evaluates Rego policy for remediation path
	// Returns: path ("aggressive", "moderate", "conservative", "manual"), error
	Evaluate(ctx context.Context, signalCtx *SignalContext) (string, error)
}

// MockRegoEvaluator is a test mock for RegoEvaluator
type MockRegoEvaluator struct {
	Result    string // Path to return
	Error     error  // Error to return
	CallCount int    // Tracks invocations
	Called    bool   // Tracks if Evaluate was called
}

// Evaluate implements RegoEvaluator for testing
func (m *MockRegoEvaluator) Evaluate(ctx context.Context, signalCtx *SignalContext) (string, error) {
	m.CallCount++
	m.Called = true
	return m.Result, m.Error
}

// NewRemediationPathDecider creates a new remediation path decider
func NewRemediationPathDecider(logger *logrus.Logger) *RemediationPathDecider {
	// Build fallback table (environment × priority → path)
	//
	// Matrix:
	// ┌─────────────┬─────────────┬──────────┬──────────────┬────────┐
	// │ Environment │     P0      │    P1    │      P2      │  P99+  │
	// ├─────────────┼─────────────┼──────────┼──────────────┼────────┤
	// │ production  │ aggressive  │ conserv  │ conservative │ manual │
	// │ staging     │  moderate   │ moderate │    manual    │ manual │
	// │ development │ aggressive  │ moderate │    manual    │ manual │
	// │ unknown     │ conservative│ conserv  │ conservative │ manual │
	// │ * (catch-all)│ moderate   │ moderate │ conservative │ manual │
	// └─────────────┴─────────────┴──────────┴──────────────┴────────┘
	//
	// Catch-all (*) handles custom environments (canary, qa-eu, blue, green, etc.):
	// - P0 + unknown env → moderate (act quickly, but validate first)
	// - P1 + unknown env → moderate (QA/test environments benefit from automation)
	// - P2 + unknown env → conservative (safe default for lower priority)
	fallbackTable := map[string]map[string]string{
		"production": {
			"P0": "aggressive",   // Critical prod → immediate action
			"P1": "conservative", // High prod → GitOps PR
			"P2": "conservative", // Normal prod → GitOps PR
		},
		"staging": {
			"P0": "moderate", // Critical staging → validate + execute
			"P1": "moderate", // High staging → validate + execute
			"P2": "manual",   // Normal staging → manual review
		},
		"development": {
			"P0": "aggressive", // Critical dev → fast feedback
			"P1": "moderate",   // High dev → validate + execute
			"P2": "manual",     // Normal dev → manual review
		},
		"unknown": {
			"P0": "conservative", // Unknown env → treat as prod (safe)
			"P1": "conservative",
			"P2": "conservative",
		},
		"*": {
			// Catch-all for custom environments (canary, qa-eu, blue, green, etc.)
			"P0": "moderate",     // P0 + custom env → moderate (act quickly with validation)
			"P1": "moderate",     // P1 + custom env → moderate (QA/test envs benefit from automation)
			"P2": "conservative", // P2 + custom env → conservative (safe default for lower priority)
		},
	}

	return &RemediationPathDecider{
		regoEvaluator: nil, // Rego not configured by default
		fallbackTable: fallbackTable,
		cache:         make(map[string]string),
		logger:        logger,
	}
}

// OPARegoEvaluator is a real OPA Rego policy evaluator
type OPARegoEvaluator struct {
	query  *rego.PreparedEvalQuery
	logger *logrus.Logger
}

// Evaluate implements RegoEvaluator for OPA
func (o *OPARegoEvaluator) Evaluate(ctx context.Context, signalCtx *SignalContext) (string, error) {
	// Prepare input for Rego policy
	input := map[string]interface{}{
		"priority":    signalCtx.Priority,
		"environment": signalCtx.Environment,
	}

	// Evaluate Rego query
	results, err := o.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return "", fmt.Errorf("rego evaluation error: %w", err)
	}

	// Check if results exist
	if len(results) == 0 {
		return "", fmt.Errorf("rego evaluation returned no results")
	}

	// Extract path from first result
	if len(results[0].Expressions) == 0 {
		return "", fmt.Errorf("rego evaluation returned no expressions")
	}

	path, ok := results[0].Expressions[0].Value.(string)
	if !ok {
		return "", fmt.Errorf("rego evaluation returned non-string path: %T", results[0].Expressions[0].Value)
	}

	// Validate path value
	validPaths := map[string]bool{
		"aggressive":   true,
		"moderate":     true,
		"conservative": true,
		"manual":       true,
	}

	if !validPaths[path] {
		return "", fmt.Errorf("rego evaluation returned invalid path: %s (expected aggressive/moderate/conservative/manual)", path)
	}

	return path, nil
}

// NewRemediationPathDeciderWithRego creates a new remediation path decider with Rego policy support
//
// The Rego policy is loaded from the specified file path and evaluated for each
// path decision. If Rego evaluation fails, the fallback table is used.
//
// Rego policy query: data.kubernaut.gateway.remediation.path
//
// Expected Rego input:
//
//	{
//	  "priority": "P0",
//	  "environment": "production"
//	}
//
// Expected Rego output: "aggressive", "moderate", "conservative", or "manual"
func NewRemediationPathDeciderWithRego(policyPath string, logger *logrus.Logger) (*RemediationPathDecider, error) {
	// Build fallback table (same as NewRemediationPathDecider) with catch-all for unknown environments
	fallbackTable := map[string]map[string]string{
		"production": {
			"P0": "aggressive",
			"P1": "conservative",
			"P2": "conservative",
		},
		"staging": {
			"P0": "moderate",
			"P1": "moderate",
			"P2": "manual",
		},
		"development": {
			"P0": "aggressive",
			"P1": "moderate",
			"P2": "manual",
		},
		"unknown": {
			"P0": "conservative",
			"P1": "conservative",
			"P2": "conservative",
		},
		"*": {
			// Catch-all for custom environments (canary, qa-eu, blue, green, etc.)
			"P0": "moderate",     // P0 + custom env → moderate (act quickly with validation)
			"P1": "moderate",     // P1 + custom env → moderate (QA/test envs benefit from automation)
			"P2": "conservative", // P2 + custom env → conservative (safe default for lower priority)
		},
	}

	// Load Rego policy from file
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Rego policy file: %w", err)
	}

	// Prepare Rego query
	query, err := rego.New(
		rego.Query("data.kubernaut.gateway.remediation.path"),
		rego.Module("remediation_path.rego", string(policyContent)),
	).PrepareForEval(context.Background())

	if err != nil {
		return nil, fmt.Errorf("failed to prepare Rego policy: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"policy_path": policyPath,
	}).Info("Rego policy loaded successfully for remediation path decision")

	// Create OPA Rego evaluator
	regoEvaluator := &OPARegoEvaluator{
		query:  &query,
		logger: logger,
	}

	return &RemediationPathDecider{
		regoEvaluator: regoEvaluator,
		fallbackTable: fallbackTable,
		cache:         make(map[string]string),
		logger:        logger,
	}, nil
}

// SignalContext provides enriched context for remediation path decision
//
// NormalizedSignal doesn't contain Environment and Priority (those are enriched
// by EnvironmentClassifier and PriorityEngine). This struct wraps the signal
// with its enriched context.
type SignalContext struct {
	Signal      *types.NormalizedSignal
	Environment string
	Priority    string
}

// DeterminePath determines the remediation path for a signal context
//
// Decision flow:
// 1. Handle nil context (return "manual")
// 2. Normalize environment and priority
// 3. Check cache for existing decision
// 4. If Rego configured, try Rego evaluation
// 5. Fall back to table lookup
// 6. Return path and cache result
func (d *RemediationPathDecider) DeterminePath(ctx context.Context, signalCtx *SignalContext) string {
	// 1. Handle nil context
	if signalCtx == nil || signalCtx.Signal == nil {
		d.logger.Warn("Nil signal context provided to path decider, defaulting to manual")
		return "manual"
	}

	// 2. Normalize inputs (handle missing/invalid values)
	environment := d.normalizeEnvironment(signalCtx.Environment)
	priority := d.normalizePriority(signalCtx.Priority)

	// 3. Check cache
	cacheKey := d.cacheKey(environment, priority)
	if path, found := d.getFromCache(cacheKey); found {
		d.logger.WithFields(logrus.Fields{
			"environment": environment,
			"priority":    priority,
			"path":        path,
			"source":      "cache",
		}).Debug("Remediation path retrieved from cache")
		return path
	}

	// 4. Try Rego evaluation (if configured)
	if d.regoEvaluator != nil {
		if path, err := d.evaluateRego(ctx, signalCtx); err == nil {
			// Validate Rego output
			if d.isValidPath(path) {
				d.logger.WithFields(logrus.Fields{
					"environment": environment,
					"priority":    priority,
					"path":        path,
					"source":      "rego",
				}).Debug("Remediation path determined via Rego policy")

				d.setCache(cacheKey, path)
				return path
			}

			// Invalid Rego output, log and fall through
			d.logger.WithFields(logrus.Fields{
				"environment":  environment,
				"priority":     priority,
				"invalid_path": path,
			}).Warn("Rego policy returned invalid path, using fallback table")
		} else {
			// Rego evaluation failed, log and fall through
			d.logger.WithFields(logrus.Fields{
				"environment": environment,
				"priority":    priority,
				"error":       err,
			}).Warn("Rego policy evaluation failed, using fallback table")
		}
	}

	// 5. Use fallback table
	path := d.lookupFallbackTable(environment, priority)

	d.logger.WithFields(logrus.Fields{
		"environment": environment,
		"priority":    priority,
		"path":        path,
		"source":      "fallback_table",
	}).Debug("Remediation path determined via fallback table")

	// 6. Cache result
	d.setCache(cacheKey, path)

	return path
}

// normalizeEnvironment handles missing/invalid environment values
func (d *RemediationPathDecider) normalizeEnvironment(env string) string {
	// Empty environment → "unknown" (safe default)
	if env == "" {
		d.logger.Warn("Empty environment provided, defaulting to unknown")
		return "unknown"
	}

	// Accept ANY non-empty environment string for dynamic configuration
	// Organizations define their own environment taxonomy (canary, qa-eu, blue, green, etc.)
	// The fallback table's catch-all entry will handle unknown environments
	return env
}

// normalizePriority handles missing/invalid priority values
func (d *RemediationPathDecider) normalizePriority(priority string) string {
	// Empty priority → "P99" (triggers manual path)
	if priority == "" {
		return "P99"
	}

	// Valid priorities: P0, P1, P2
	validPriorities := map[string]bool{
		"P0": true,
		"P1": true,
		"P2": true,
	}

	if validPriorities[priority] {
		return priority
	}

	// Invalid priority → "P99" (manual path)
	d.logger.WithField("priority", priority).Warn("Invalid priority, defaulting to P99 (manual)")
	return "P99"
}

// lookupFallbackTable looks up path from fallback table
func (d *RemediationPathDecider) lookupFallbackTable(environment, priority string) string {
	// 1. Try exact environment match first
	if envMap, ok := d.fallbackTable[environment]; ok {
		if path, ok := envMap[priority]; ok {
			return path
		}
	}

	// 2. Try catch-all environment ("*") if exact match not found
	if catchAllMap, ok := d.fallbackTable["*"]; ok {
		if path, ok := catchAllMap[priority]; ok {
			d.logger.WithFields(logrus.Fields{
				"environment": environment,
				"priority":    priority,
				"path":        path,
			}).Debug("Using catch-all fallback for custom environment")
			return path
		}
	}

	// 3. Final fallback → manual (safest, should rarely be reached)
	d.logger.WithFields(logrus.Fields{
		"environment": environment,
		"priority":    priority,
	}).Warn("No fallback mapping found for environment/priority combination, defaulting to manual")
	return "manual"
}

// evaluateRego evaluates Rego policy for remediation path
func (d *RemediationPathDecider) evaluateRego(ctx context.Context, signalCtx *SignalContext) (string, error) {
	return d.regoEvaluator.Evaluate(ctx, signalCtx)
}

// isValidPath checks if path is one of the four valid paths
func (d *RemediationPathDecider) isValidPath(path string) bool {
	validPaths := map[string]bool{
		"aggressive":   true,
		"moderate":     true,
		"conservative": true,
		"manual":       true,
	}
	return validPaths[path]
}

// cacheKey generates cache key from environment and priority
func (d *RemediationPathDecider) cacheKey(environment, priority string) string {
	return fmt.Sprintf("%s:%s", environment, priority)
}

// getFromCache retrieves path from cache (thread-safe)
func (d *RemediationPathDecider) getFromCache(key string) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	path, found := d.cache[key]
	return path, found
}

// setCache stores path in cache (thread-safe)
func (d *RemediationPathDecider) setCache(key, path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache[key] = path
}

// SetRegoEvaluator configures the Rego policy evaluator
func (d *RemediationPathDecider) SetRegoEvaluator(evaluator RegoEvaluator) {
	d.regoEvaluator = evaluator
	d.logger.Info("Rego evaluator configured for remediation path decision")
}

// GetCRDMetadata returns metadata to include in RemediationRequest CRD
//
// This metadata guides AI remediation strategy selection:
// - remediationPath: The determined path (aggressive/moderate/conservative/manual)
// - environment: Environment classification (production/staging/development/unknown)
// - priority: Priority level (P0/P1/P2)
func (d *RemediationPathDecider) GetCRDMetadata(signalCtx *SignalContext, path string) map[string]string {
	return map[string]string{
		"remediationPath": path,
		"environment":     signalCtx.Environment,
		"priority":        signalCtx.Priority,
	}
}

// GetPathExplanation returns human-readable explanation for path decision
//
// Explanation format: "Path: {path} (Environment: {env}, Priority: {priority})"
//
// Used for:
// - Audit logs (compliance)
// - Troubleshooting (why did AI choose this path?)
// - Metrics labels (observability)
func (d *RemediationPathDecider) GetPathExplanation(signalCtx *SignalContext, path string) string {
	return fmt.Sprintf("Path: %s (Environment: %s, Priority: %s, Reason: %s)",
		path, signalCtx.Environment, signalCtx.Priority, d.getPathReason(signalCtx, path))
}

// getPathReason provides reasoning for path decision
func (d *RemediationPathDecider) getPathReason(signalCtx *SignalContext, path string) string {
	// Path reasoning matrix
	reasons := map[string]map[string]string{
		"production": {
			"P0": "Critical production outage requires immediate automated remediation",
			"P1": "High priority production issue requires GitOps PR approval",
			"P2": "Normal production priority requires conservative approach",
		},
		"staging": {
			"P0": "Critical staging failure needs validation before automated execution",
			"P1": "High priority staging allows moderate risk with validation",
			"P2": "Low priority staging requires manual review",
		},
		"development": {
			"P0": "Critical dev failure needs immediate fix for developer productivity",
			"P1": "High priority dev allows faster remediation with validation",
			"P2": "Low priority dev requires manual investigation",
		},
		"unknown": {
			"P0": "Unknown environment treated as production for safety",
			"P1": "Unknown environment treated as production for safety",
			"P2": "Unknown environment treated as production for safety",
		},
	}

	if envMap, ok := reasons[signalCtx.Environment]; ok {
		if reason, ok := envMap[signalCtx.Priority]; ok {
			return reason
		}
	}

	return "Default fallback path for undefined scenario"
}
