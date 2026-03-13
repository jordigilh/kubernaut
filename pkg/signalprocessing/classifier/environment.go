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

// Package classifier provides Rego-based environment and business classification.
//
// Per IMPLEMENTATION_PLAN_V1.22.md Day 4 specification:
//
//	type EnvironmentClassifier struct {
//	    regoQuery   *rego.PreparedEvalQuery // Prepared query for performance
//	    logger      logr.Logger             // DD-005 v2.0: logr.Logger
//	    policyPath  string                  // Path to mounted ConfigMap file
//	    fileWatcher *hotreload.FileWatcher  // Per DD-INFRA-001: fsnotify-based
//	    policyMu    sync.RWMutex            // Thread safety for policy access
//	}
//
// Environment classification is fully handled by Rego policies (BR-SP-051).
// Operators define their own defaults using the `default` keyword in Rego.
// BR-SP-052/BR-SP-053: ConfigMap fallback and Go defaults deprecated 2025-12-20.
package classifier

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

// EnvironmentClassifier determines environment using Rego policy.
// Per IMPLEMENTATION_PLAN_V1.22.md Day 4 specification.
// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify.
type EnvironmentClassifier struct {
	regoQuery   *rego.PreparedEvalQuery // Prepared query for performance
	logger      logr.Logger
	policyPath  string                 // Path to mounted ConfigMap file
	fileWatcher *hotreload.FileWatcher // Per DD-INFRA-001: fsnotify-based

	// Rego policy mutex
	policyMu sync.RWMutex
}

// NewEnvironmentClassifier creates a new Rego-based environment classifier.
// Per plan: query is prepared once at construction time for performance.
//
// BR-SP-051: Primary detection from namespace labels via Rego policy
// BR-SP-052: DEPRECATED (2025-12-20) - ConfigMap fallback removed
// BR-SP-053: DEPRECATED (2025-12-20) - Defaults now defined in Rego
//
// DD-005 v2.0: Uses logr.Logger
func NewEnvironmentClassifier(ctx context.Context, policyPath string, logger logr.Logger) (*EnvironmentClassifier, error) {
	log := logger.WithName("environment-classifier")

	// Read policy file
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file %s: %w", policyPath, err)
	}

	// Prepare Rego query once (per plan specification)
	query, err := rego.New(
		rego.Query("data.signalprocessing.environment.result"),
		rego.Module(filepath.Base(policyPath), string(policyContent)),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compile Rego policy: %w", err)
	}

	classifier := &EnvironmentClassifier{
		regoQuery:  &query,
		logger:     log,
		policyPath: policyPath,
	}

	return classifier, nil
}

// Classify determines environment using Rego policy and Go fallbacks.
// Per plan (line 1864): Priority order is namespace labels → ConfigMap → signal labels → default
//
// Classification is fully handled by Rego policy.
// Operators define their own defaults using the `default` keyword in Rego.
// Go code has NO hardcoded fallbacks - Rego is the single source of truth.
//
// BR-SP-051: Primary detection from namespace labels - via Rego
// BR-SP-052: ConfigMap fallback - DEPRECATED, define in Rego if needed
// BR-SP-053: Default value - DEPRECATED, operators define via Rego `default` keyword
//
// Returns error if Rego evaluation fails (policy is mandatory).
func (c *EnvironmentClassifier) Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	// Build input for Rego policy
	input := c.buildRegoInput(k8sCtx, signal)

	// Rego policy is the single source of truth for environment classification
	// Operators define their own defaults using `default result := {...}` in Rego
	result, err := c.evaluateRego(ctx, input)
	if err != nil {
		// BR-SP-053: DEPRECATED - Go fallback removed (2025-12-20)
		// Rego policy evaluation failures are ERRORS, not fallbacks.
		c.logger.Error(err, "Rego evaluation failed - policy is mandatory, no Go fallback",
			"hint", "Ensure your environment.rego policy has a `default` rule for catch-all cases")
		return nil, fmt.Errorf("environment policy evaluation failed: %w", err)
	}

	c.logger.V(1).Info("Environment classified via Rego policy",
		"environment", result.Environment,
		"source", result.Source)
	return result, nil
}

// evaluateRego evaluates the prepared Rego query.
// Returns error if Rego fails or returns invalid output (no Go fallback).
func (c *EnvironmentClassifier) evaluateRego(ctx context.Context, input map[string]interface{}) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	c.policyMu.RLock()
	query := c.regoQuery
	c.policyMu.RUnlock()

	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("rego evaluation failed: %w", err)
	}

	// Check for results - empty means policy is missing `default` rule
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		c.logger.Error(nil, "Rego returned no results - policy missing `default` rule",
			"hint", "Add a `default result := {...}` rule to your environment.rego policy")
		return nil, fmt.Errorf("environment policy returned no results - add a `default` rule")
	}

	// Extract result from Rego
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		c.logger.Error(nil, "Invalid Rego result format - policy returns wrong structure",
			"value", fmt.Sprintf("%T", results[0].Expressions[0].Value),
			"hint", "Ensure environment.rego returns a map with 'environment' and 'source' fields")
		return nil, fmt.Errorf("environment policy returned invalid output structure (expected map)")
	}

	// Extract fields from Rego result
	// Empty string is valid - it means operators intentionally left environment unclassified
	environment, _ := resultMap["environment"].(string)
	source, _ := resultMap["source"].(string)
	if source == "" {
		source = "default"
	}

	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment:  environment,
		Source:       source,
		ClassifiedAt: metav1.Now(), // Set timestamp in Go, not Rego
	}, nil
}

// buildRegoInput constructs the input map for Rego policy evaluation.
// Per plan specification:
//
//	input := map[string]interface{}{
//	    "namespace": map[string]interface{}{
//	        "name":   k8sCtx.Namespace.Name,
//	        "labels": k8sCtx.Namespace.Labels,
//	    },
//	    "signal": map[string]interface{}{
//	        "labels": signal.Labels,
//	    },
//	}
func (c *EnvironmentClassifier) buildRegoInput(k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) map[string]interface{} {
	input := map[string]interface{}{}

	// Namespace context
	if k8sCtx != nil && k8sCtx.Namespace != nil {
		input["namespace"] = map[string]interface{}{
			"name":   k8sCtx.Namespace.Name,
			"labels": ensureLabelsMap(k8sCtx.Namespace.Labels),
		}
	} else {
		input["namespace"] = map[string]interface{}{
			"name":   "",
			"labels": map[string]interface{}{},
		}
	}

	// Signal context
	if signal != nil {
		input["signal"] = map[string]interface{}{
			"labels": ensureLabelsMap(signal.Labels),
		}
	} else {
		input["signal"] = map[string]interface{}{
			"labels": map[string]interface{}{},
		}
	}

	return input
}

// ensureLabelsMap converts map[string]string to map[string]interface{} for Rego.
func ensureLabelsMap(labels map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range labels {
		result[k] = v
	}
	return result
}

// BR-SP-052: tryConfigMapFallback - DEPRECATED (2025-12-20)
// ConfigMap fallback logic should be implemented in Rego policy if needed.
// Operators have full control over fallback behavior via Rego `default` keyword.

// BR-SP-053: defaultResult - DEPRECATED (2025-12-20)
// Default values should be defined by operators in their Rego policies.
// Go code has no hardcoded defaults - Rego is the single source of truth.

// ========================================
// HOT-RELOAD SUPPORT (BR-SP-072)
// ========================================
// Per DD-INFRA-001: ConfigMap Hot-Reload Pattern
// Uses shared FileWatcher component for fsnotify-based hot-reload

// StartHotReload starts watching the policy file for changes.
// Per BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify.
// Per DD-INFRA-001: Uses shared FileWatcher component.
func (c *EnvironmentClassifier) StartHotReload(ctx context.Context) error {
	var err error
	c.fileWatcher, err = hotreload.NewFileWatcher(
		c.policyPath, // e.g., "/etc/kubernaut/policies/environment.rego"
		func(content string) error {
			// Compile new Rego policy
			newQuery, err := rego.New(
				rego.Query("data.signalprocessing.environment.result"),
				rego.Module(filepath.Base(c.policyPath), content),
			).PrepareForEval(ctx)
			if err != nil {
				return fmt.Errorf("rego compilation failed: %w", err)
			}

			// Atomically swap policy
			c.policyMu.Lock()
			c.regoQuery = &newQuery
			c.policyMu.Unlock()

			c.logger.Info("Environment policy hot-reloaded successfully",
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
// Per BR-SP-072: Clean shutdown of FileWatcher.
func (c *EnvironmentClassifier) Stop() {
	if c.fileWatcher != nil {
		c.fileWatcher.Stop()
	}
}

// GetPolicyHash returns the current policy hash (for monitoring/debugging).
// Per DD-INFRA-001: SHA256 hash tracking for audit/debugging.
func (c *EnvironmentClassifier) GetPolicyHash() string {
	if c.fileWatcher != nil {
		return c.fileWatcher.GetLastHash()
	}
	return ""
}
