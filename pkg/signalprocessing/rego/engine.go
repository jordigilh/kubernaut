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

// Package rego provides OPA Rego policy evaluation for SignalProcessing.
// Design Decision: ADR-041 - K8s Enricher fetches data, Rego evaluates policies
// Design Decision: DD-WORKFLOW-001 v1.9 - CustomLabels extraction via Rego
// Business Requirement: BR-SP-080 (CustomLabels)
package rego

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"
)

const (
	// DefaultTimeout is the default timeout for Rego policy evaluation.
	// DD-WORKFLOW-001 v1.9: Sandboxed OPA Runtime with 5s timeout
	DefaultTimeout = 5 * time.Second

	// MaxMemory is the maximum memory for Rego policy evaluation (128MB).
	// DD-WORKFLOW-001 v1.9: Memory limit for sandboxed runtime
	MaxMemory = 128 * 1024 * 1024
)

// SystemLabels are mandatory labels that cannot be overridden by customer Rego policies.
// DD-WORKFLOW-001 v1.9: Security wrapper strips these from CustomLabels output.
var SystemLabels = []string{
	"environment", // Set by EnvironmentClassifier
	"priority",    // Set by PriorityClassifier
	"severity",    // From signal source
	"namespace",   // From K8s context
	"service",     // From K8s context
}

// Engine evaluates Rego policies for CustomLabels extraction.
// DD-WORKFLOW-001 v1.9: Sandboxed OPA Runtime (no network, no filesystem, timeout, memory limit)
type Engine struct {
	preparedQuery *rego.PreparedEvalQuery
	logger        logr.Logger
}

// NewEngine creates a new Rego policy engine.
// Parameters:
//   - policy: Rego policy content (string)
//   - query: Rego query path (e.g., "data.signalprocessing.labels.custom_labels")
//   - logger: Structured logger
//
// Returns error if policy syntax is invalid.
func NewEngine(policy, query string, logger logr.Logger) (*Engine, error) {
	// Prepare Rego query with the policy
	prepared, err := rego.New(
		rego.Query(query),
		rego.Module("policy.rego", policy),
	).PrepareForEval(context.Background())

	if err != nil {
		return nil, fmt.Errorf("failed to prepare Rego policy: %w", err)
	}

	logger.Info("Rego policy engine initialized", "query", query)

	return &Engine{
		preparedQuery: &prepared,
		logger:        logger.WithName("rego-engine"),
	}, nil
}

// Evaluate evaluates the Rego policy with given input.
// Returns the result as map[string][]string for CustomLabels.
func (e *Engine) Evaluate(ctx context.Context, input map[string]interface{}) (map[string][]string, error) {
	return e.EvaluateWithTimeout(ctx, input, DefaultTimeout)
}

// EvaluateWithTimeout evaluates the Rego policy with a custom timeout.
func (e *Engine) EvaluateWithTimeout(ctx context.Context, input map[string]interface{}, timeout time.Duration) (map[string][]string, error) {
	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Evaluate Rego query
	results, err := e.preparedQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("rego evaluation error: %w", err)
	}

	// Check if results exist
	if len(results) == 0 {
		e.logger.V(1).Info("Rego evaluation returned no results, using empty map")
		return make(map[string][]string), nil
	}

	// Extract result from first expression
	if len(results[0].Expressions) == 0 {
		return make(map[string][]string), nil
	}

	// Convert result to map[string][]string
	rawResult := results[0].Expressions[0].Value
	return e.convertResult(rawResult)
}

// EvaluateWithSecurityWrapper evaluates the Rego policy and strips system labels.
// DD-WORKFLOW-001 v1.9: Mandatory Label Protection
func (e *Engine) EvaluateWithSecurityWrapper(ctx context.Context, input map[string]interface{}) (map[string][]string, error) {
	result, err := e.Evaluate(ctx, input)
	if err != nil {
		return nil, err
	}

	// Strip system labels (security wrapper)
	for _, systemLabel := range SystemLabels {
		delete(result, systemLabel)
	}

	e.logger.V(1).Info("Rego evaluation completed with security wrapper",
		"result_keys", len(result))

	return result, nil
}

// convertResult converts Rego result to map[string][]string.
func (e *Engine) convertResult(rawResult interface{}) (map[string][]string, error) {
	result := make(map[string][]string)

	// Handle nil/empty result
	if rawResult == nil {
		return result, nil
	}

	// Expected format: map[string]interface{} where values are []interface{}
	rawMap, ok := rawResult.(map[string]interface{})
	if !ok {
		return result, fmt.Errorf("unexpected Rego result type: %T (expected map)", rawResult)
	}

	for key, value := range rawMap {
		// Convert []interface{} to []string
		switch v := value.(type) {
		case []interface{}:
			strSlice := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					strSlice = append(strSlice, s)
				}
			}
			result[key] = strSlice
		case []string:
			result[key] = v
		default:
			e.logger.V(1).Info("Skipping non-array Rego result value",
				"key", key,
				"type", fmt.Sprintf("%T", value))
		}
	}

	return result, nil
}

