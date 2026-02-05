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

// Package rego provides CustomLabels extraction via sandboxed OPA Rego policies.
//
// # Business Requirements
//
// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-104: Mandatory Label Protection
//
// # Design Decisions
//
// DD-WORKFLOW-001 v1.9: Validation limits and sandbox requirements
//
// # Sandbox Configuration
//
//   - Evaluation timeout: 5 seconds
//   - Memory limit: 128 MB (enforced at runtime level)
//   - Network access: Disabled (OPA default)
//   - Filesystem access: Disabled (OPA default)
//
// # Validation Limits
//
//   - Max keys (subdomains): 10
//   - Max values per key: 5
//   - Max key length: 63 chars
//   - Max value length: 100 chars
package rego

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	rego "github.com/open-policy-agent/opa/v1/rego"

	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Sandbox configuration per DD-WORKFLOW-001 v1.9
const (
	evaluationTimeout = 5 * time.Second // Max Rego evaluation time
	maxKeys           = 10              // Max keys (subdomains)
	maxValuesPerKey   = 5               // Max values per key
	maxKeyLength      = 63              // K8s label key compatibility
	maxValueLength    = 100             // Prompt efficiency
)

// Reserved prefixes that must be stripped (BR-SP-104)
var reservedPrefixes = []string{"kubernaut.ai/", "system/"}

// Engine evaluates customer Rego policies for CustomLabels.
// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-104: Mandatory Label Protection (via security wrapper)
// DD-WORKFLOW-001 v1.9: Sandboxed OPA Runtime
// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify
type Engine struct {
	logger       logr.Logger
	policyPath   string
	policyModule string                 // Compiled policy with security wrapper
	fileWatcher  *hotreload.FileWatcher // Per DD-INFRA-001: fsnotify-based
	mu           sync.RWMutex
}

// NewEngine creates a new CustomLabels Rego engine.
// Per BR-SP-102: Extract customer labels via sandboxed OPA policies.
func NewEngine(logger logr.Logger, policyPath string) *Engine {
	return &Engine{
		logger:     logger.WithName("rego"),
		policyPath: policyPath,
	}
}

// RegoInput wraps shared types for Rego policy evaluation.
// Uses sharedtypes.KubernetesContext (authoritative source).
type RegoInput struct {
	Kubernetes     *sharedtypes.KubernetesContext `json:"kubernetes"`
	Signal         SignalContext                  `json:"signal"`
	DetectedLabels *sharedtypes.DetectedLabels    `json:"detected_labels,omitempty"`
}

// SignalContext contains signal-specific data for Rego policies.
type SignalContext struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
}

// LoadPolicy loads customer policy from string with validation.
// The policy is stored as-is; security filtering happens after evaluation.
// BR-SP-072: Validates Rego syntax before loading for graceful degradation.
func (e *Engine) LoadPolicy(policyContent string) error {
	// Validate Rego syntax before loading
	if err := e.validatePolicy(policyContent); err != nil {
		return fmt.Errorf("policy validation failed: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.policyModule = policyContent
	e.logger.Info("Rego policy loaded", "policySize", len(policyContent))
	return nil
}

// validatePolicy checks if the policy compiles successfully.
// Per BR-SP-072: Graceful degradation on invalid policy.
func (e *Engine) validatePolicy(policyContent string) error {
	// Try to compile the policy to check syntax
	_, err := rego.New(
		rego.Query("data.signalprocessing.labels.labels"),
		rego.Module("policy.rego", policyContent),
	).PrepareForEval(context.Background())

	return err
}

// EvaluatePolicy evaluates the policy and returns CustomLabels.
// Output format: map[string][]string (subdomain → list of values)
// DD-WORKFLOW-001 v1.9: 5s timeout, sandboxed execution
// BR-SP-104: Security filtering strips reserved prefixes after evaluation
func (e *Engine) EvaluatePolicy(ctx context.Context, input *RegoInput) (map[string][]string, error) {
	e.mu.RLock()
	policyModule := e.policyModule
	e.mu.RUnlock()

	if policyModule == "" {
		e.logger.V(1).Info("No policy loaded, returning empty labels")
		return make(map[string][]string), nil
	}

	// Sandboxed execution: 5s timeout per DD-WORKFLOW-001 v1.9
	evalCtx, cancel := context.WithTimeout(ctx, evaluationTimeout)
	defer cancel()

	// Check if context is already cancelled
	select {
	case <-evalCtx.Done():
		return nil, evalCtx.Err()
	default:
	}

	r := rego.New(
		rego.Query("data.signalprocessing.labels.labels"),
		rego.Module("policy.rego", policyModule),
		rego.Input(input),
		rego.StrictBuiltinErrors(true),    // Strict mode for safety
		rego.EnablePrintStatements(false), // Disable debugging in prod
	)

	rs, err := r.Eval(evalCtx)
	if err != nil {
		return nil, fmt.Errorf("rego evaluation failed: %w", err)
	}

	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return make(map[string][]string), nil
	}

	// Convert result to map[string][]string
	result, err := e.convertResult(rs[0].Expressions[0].Value)
	if err != nil {
		return nil, fmt.Errorf("invalid rego output type: %w", err)
	}

	// BR-SP-104: Security filtering - strip reserved prefixes
	// Validate and sanitize (DD-WORKFLOW-001 v1.9)
	result = e.validateAndSanitize(result)

	e.logger.Info("CustomLabels evaluated", "labelCount", len(result))
	return result, nil
}

// convertResult converts OPA output to map[string][]string.
func (e *Engine) convertResult(value interface{}) (map[string][]string, error) {
	result := make(map[string][]string)

	valueMap, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", value)
	}

	for key, val := range valueMap {
		switch v := val.(type) {
		case []interface{}:
			var strValues []string
			for _, item := range v {
				if strVal, ok := item.(string); ok {
					strValues = append(strValues, strVal)
				}
				// Skip non-string values silently
			}
			if len(strValues) > 0 {
				result[key] = strValues
			}
		case string:
			result[key] = []string{v}
			// Skip non-string/non-array values (e.g., numbers)
		}
	}

	return result, nil
}

// validateAndSanitize enforces validation limits per DD-WORKFLOW-001 v1.9.
// Strips reserved prefixes (BR-SP-104) and enforces size limits.
func (e *Engine) validateAndSanitize(labels map[string][]string) map[string][]string {
	result := make(map[string][]string)

	keyCount := 0
	for key, values := range labels {
		// Check key count limit
		if keyCount >= maxKeys {
			e.logger.Info("CustomLabels key limit reached, truncating",
				"maxKeys", maxKeys, "totalKeys", len(labels))
			break
		}

		// Skip reserved prefixes (BR-SP-104: Mandatory Label Protection)
		if hasReservedPrefix(key, reservedPrefixes) {
			e.logger.Info("CustomLabels reserved prefix stripped", "key", key)
			continue
		}

		// Truncate key if too long
		truncatedKey := key
		if len(key) > maxKeyLength {
			e.logger.Info("CustomLabels key truncated",
				"key", key, "maxLength", maxKeyLength)
			truncatedKey = key[:maxKeyLength]
		}

		// Validate and truncate values
		var validValues []string
		for i, value := range values {
			if i >= maxValuesPerKey {
				e.logger.Info("CustomLabels values limit reached",
					"key", truncatedKey, "maxValues", maxValuesPerKey)
				break
			}
			truncatedValue := value
			if len(value) > maxValueLength {
				e.logger.Info("CustomLabels value truncated",
					"key", truncatedKey, "maxLength", maxValueLength)
				truncatedValue = value[:maxValueLength]
			}
			validValues = append(validValues, truncatedValue)
		}

		if len(validValues) > 0 {
			result[truncatedKey] = validValues
			keyCount++
		}
	}

	return result
}

// hasReservedPrefix checks if key starts with any reserved prefix.
func hasReservedPrefix(key string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// ========================================
// HOT-RELOAD SUPPORT (BR-SP-072)
// ========================================
// Per DD-INFRA-001: ConfigMap Hot-Reload Pattern
// Uses shared FileWatcher component for fsnotify-based hot-reload

// StartHotReload starts watching the policy file for changes.
// Per BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify.
// Per DD-INFRA-001: Uses shared FileWatcher component.
func (e *Engine) StartHotReload(ctx context.Context) error {
	var err error
	e.fileWatcher, err = hotreload.NewFileWatcher(
		e.policyPath, // e.g., "/etc/kubernaut/policies/labels.rego"
		func(content string) error {
			// Validate and load new policy
			// Per BR-SP-072: Graceful degradation on validation failure
			if err := e.LoadPolicy(content); err != nil {
				return fmt.Errorf("policy validation failed: %w", err)
			}

			e.logger.Info("CustomLabels policy hot-reloaded successfully",
				"hash", e.GetPolicyHash())
			return nil
		},
		e.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	return e.fileWatcher.Start(ctx)
}

// Stop gracefully stops the hot-reloader.
// Per BR-SP-072: Clean shutdown of FileWatcher.
func (e *Engine) Stop() {
	if e.fileWatcher != nil {
		e.fileWatcher.Stop()
	}
}

// GetPolicyHash returns the current policy hash (for monitoring/debugging).
// Per DD-INFRA-001: SHA256 hash tracking for audit/debugging.
func (e *Engine) GetPolicyHash() string {
	if e.fileWatcher != nil {
		return e.fileWatcher.GetLastHash()
	}
	return ""
}
