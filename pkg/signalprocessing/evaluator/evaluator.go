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

package evaluator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	rego "github.com/open-policy-agent/opa/v1/rego"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

const (
	evaluationTimeout = 5 * time.Second
	regoEvalTimeout   = 100 * time.Millisecond

	maxKeys         = 10
	maxValuesPerKey = 5
	maxKeyLength    = 63
	maxValueLength  = 100

	queryEnvironment  = "data.signalprocessing.environment"
	querySeverity     = "data.signalprocessing.severity"
	queryPriority     = "data.signalprocessing.priority"
	queryCustomLabels = "data.signalprocessing.labels"
)

var (
	reservedPrefixes = []string{"kubernaut.ai/", "system/"}
	validPriorities  = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
)

// Evaluator provides unified OPA Rego evaluation for all SignalProcessing
// classification rules from a single policy.rego file.
//
// ADR-060: Replaces 5 separate classifiers (EnvironmentClassifier, PriorityEngine,
// SeverityClassifier, BusinessClassifier, RegoEngine) with a single evaluator that
// holds 4 prepared OPA queries over the same policy module.
type Evaluator struct {
	logger     logr.Logger
	policyPath string

	mu            sync.RWMutex
	policyModule  string
	envQuery      *rego.PreparedEvalQuery
	severityQuery *rego.PreparedEvalQuery
	priorityQuery *rego.PreparedEvalQuery
	labelsQuery   *rego.PreparedEvalQuery

	fileWatcher *hotreload.FileWatcher
}

// New creates a unified evaluator for a single policy.rego file.
func New(policyPath string, logger logr.Logger) *Evaluator {
	return &Evaluator{
		policyPath: policyPath,
		logger:     logger.WithName("evaluator"),
	}
}

// LoadPolicy loads and compiles a policy, preparing all 4 OPA queries.
// Returns error if the policy has syntax errors or any query fails to compile.
// On hot-reload, a failed load keeps the previous queries active.
func (e *Evaluator) LoadPolicy(policyContent string) error {
	envQ, err := rego.New(
		rego.Query(queryEnvironment),
		rego.Module("policy.rego", policyContent),
	).PrepareForEval(context.Background())
	if err != nil {
		return fmt.Errorf("environment query compilation failed: %w", err)
	}

	sevQ, err := rego.New(
		rego.Query(querySeverity),
		rego.Module("policy.rego", policyContent),
	).PrepareForEval(context.Background())
	if err != nil {
		return fmt.Errorf("severity query compilation failed: %w", err)
	}

	priQ, err := rego.New(
		rego.Query(queryPriority),
		rego.Module("policy.rego", policyContent),
	).PrepareForEval(context.Background())
	if err != nil {
		return fmt.Errorf("priority query compilation failed: %w", err)
	}

	lblQ, err := rego.New(
		rego.Query(queryCustomLabels),
		rego.Module("policy.rego", policyContent),
		rego.StrictBuiltinErrors(true),
		rego.EnablePrintStatements(false),
	).PrepareForEval(context.Background())
	if err != nil {
		return fmt.Errorf("labels query compilation failed: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.policyModule = policyContent
	e.envQuery = &envQ
	e.severityQuery = &sevQ
	e.priorityQuery = &priQ
	e.labelsQuery = &lblQ

	e.logger.Info("Policy loaded", "policySize", len(policyContent))
	return nil
}

// StartHotReload starts watching the policy file for changes.
// Per DD-INFRA-001: fsnotify-based hot-reload via shared FileWatcher.
func (e *Evaluator) StartHotReload(ctx context.Context) error {
	var err error
	e.fileWatcher, err = hotreload.NewFileWatcher(
		e.policyPath,
		func(content string) error {
			if err := e.LoadPolicy(content); err != nil {
				return fmt.Errorf("policy validation failed: %w", err)
			}
			e.logger.Info("Policy hot-reloaded", "hash", e.GetPolicyHash())
			return nil
		},
		e.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	return e.fileWatcher.Start(ctx)
}

// Stop gracefully stops the hot-reload watcher.
func (e *Evaluator) Stop() {
	if e.fileWatcher != nil {
		e.fileWatcher.Stop()
	}
}

// GetPolicyHash returns the SHA256 hash of the currently loaded policy.
func (e *Evaluator) GetPolicyHash() string {
	if e.fileWatcher != nil {
		return e.fileWatcher.GetLastHash()
	}
	return ""
}

// BuildInput constructs a PolicyInput from Kubernetes context and signal data.
// Handles nil fields gracefully with zero-value defaults.
func BuildInput(k8sCtx *sharedtypes.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) PolicyInput {
	input := PolicyInput{}

	if k8sCtx != nil && k8sCtx.Namespace != nil {
		input.Namespace = *k8sCtx.Namespace
	}
	if k8sCtx != nil && k8sCtx.Workload != nil {
		input.Workload = *k8sCtx.Workload
	}
	if signal != nil {
		input.Signal = SignalInput{
			Severity: signal.Severity,
			Type:     signal.Type,
			Source:   signal.Source,
			Labels:   signal.Labels,
		}
	}

	return input
}

// EvaluateEnvironment determines the environment classification.
// BR-SP-051: Primary detection from namespace labels via Rego.
// Returns error on evaluation failure (caller should treat as fatal).
func (e *Evaluator) EvaluateEnvironment(ctx context.Context, input PolicyInput) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	e.mu.RLock()
	query := e.envQuery
	e.mu.RUnlock()

	if query == nil {
		return nil, fmt.Errorf("environment query not loaded - policy not initialized")
	}

	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("rego evaluation failed: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return nil, fmt.Errorf("environment policy returned no results - add a `default` rule")
	}

	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("environment policy returned invalid output structure (expected map)")
	}

	environment, _ := resultMap["environment"].(string)
	source, _ := resultMap["source"].(string)
	if source == "" {
		source = "default"
	}

	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment:  environment,
		Source:       source,
		ClassifiedAt: metav1.Now(),
	}, nil
}

// EvaluateSeverity determines the normalized severity.
// BR-SP-105: Severity determination via Rego policy.
// Returns error on evaluation failure (caller should treat as permanent failure).
func (e *Evaluator) EvaluateSeverity(ctx context.Context, input PolicyInput) (*SeverityResult, error) {
	e.mu.RLock()
	query := e.severityQuery
	e.mu.RUnlock()

	if query == nil {
		return nil, fmt.Errorf("severity query not loaded - policy not initialized")
	}

	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("rego evaluation failed: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return nil, fmt.Errorf("no severity determined by policy for input %q - add else clause for unmapped values", input.Signal.Severity)
	}

	severityValue, ok := results[0].Expressions[0].Value.(string)
	if !ok {
		return nil, fmt.Errorf("policy returned non-string severity: %T", results[0].Expressions[0].Value)
	}

	if !isValidSeverity(severityValue) {
		return nil, fmt.Errorf("policy returned invalid severity %q - must be critical/high/medium/low/unknown", severityValue)
	}

	return &SeverityResult{
		Severity:   severityValue,
		Source:     "rego-policy",
		PolicyHash: e.GetPolicyHash(),
	}, nil
}

// EvaluatePriority determines the priority assignment.
// BR-SP-070: Rego-based priority assignment.
// Priority references the environment rule directly within Rego (no envClass parameter).
func (e *Evaluator) EvaluatePriority(ctx context.Context, input PolicyInput) (*signalprocessingv1alpha1.PriorityAssignment, error) {
	e.mu.RLock()
	query := e.priorityQuery
	e.mu.RUnlock()

	if query == nil {
		return nil, fmt.Errorf("priority query not loaded - policy not initialized")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, regoEvalTimeout)
	defer cancel()

	results, err := query.Eval(timeoutCtx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("priority policy evaluation failed: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return nil, fmt.Errorf("priority policy returned no results - add a `default` rule")
	}

	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("priority policy returned invalid output structure (expected map)")
	}

	priority, _ := resultMap["priority"].(string)
	policyName, _ := resultMap["policy_name"].(string)

	if !validPriorities[priority] {
		return nil, fmt.Errorf("invalid priority value: %s (must be P0, P1, P2, or P3)", priority)
	}

	return &signalprocessingv1alpha1.PriorityAssignment{
		Priority:   priority,
		Source:     "rego-policy",
		PolicyName: policyName,
		AssignedAt: metav1.Now(),
	}, nil
}

// EvaluateCustomLabels extracts custom labels from namespace/signal data.
// BR-SP-102: CustomLabels extraction via sandboxed OPA.
// BR-SP-104: Reserved prefix stripping applied after evaluation.
// Returns empty map (not error) on evaluation failure (non-fatal, fallback to namespace labels).
func (e *Evaluator) EvaluateCustomLabels(ctx context.Context, input PolicyInput) (map[string][]string, error) {
	e.mu.RLock()
	query := e.labelsQuery
	e.mu.RUnlock()

	if query == nil {
		e.logger.V(1).Info("Labels query not loaded, returning empty labels")
		return make(map[string][]string), nil
	}

	evalCtx, cancel := context.WithTimeout(ctx, evaluationTimeout)
	defer cancel()

	results, err := query.Eval(evalCtx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("rego evaluation failed: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return make(map[string][]string), nil
	}

	result, err := convertLabelResult(results[0].Expressions[0].Value)
	if err != nil {
		return nil, fmt.Errorf("invalid rego output type: %w", err)
	}

	result = validateAndSanitizeLabels(result, e.logger)

	e.logger.Info("CustomLabels evaluated", "labelCount", len(result))
	return result, nil
}

func isValidSeverity(severity string) bool {
	switch severity {
	case "critical", "high", "medium", "low", "unknown":
		return true
	default:
		return false
	}
}

func convertLabelResult(value interface{}) (map[string][]string, error) {
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
			}
			if len(strValues) > 0 {
				result[key] = strValues
			}
		case string:
			result[key] = []string{v}
		}
	}

	return result, nil
}

func validateAndSanitizeLabels(labels map[string][]string, logger logr.Logger) map[string][]string {
	result := make(map[string][]string)

	keyCount := 0
	for key, values := range labels {
		if keyCount >= maxKeys {
			logger.Info("CustomLabels key limit reached, truncating",
				"maxKeys", maxKeys, "totalKeys", len(labels))
			break
		}

		if hasReservedPrefix(key, reservedPrefixes) {
			logger.Info("CustomLabels reserved prefix stripped", "key", key)
			continue
		}

		truncatedKey := key
		if len(key) > maxKeyLength {
			logger.Info("CustomLabels key truncated", "key", key, "maxLength", maxKeyLength)
			truncatedKey = key[:maxKeyLength]
		}

		var validValues []string
		for i, value := range values {
			if i >= maxValuesPerKey {
				logger.Info("CustomLabels values limit reached",
					"key", truncatedKey, "maxValues", maxValuesPerKey)
				break
			}
			truncatedValue := value
			if len(value) > maxValueLength {
				logger.Info("CustomLabels value truncated",
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

func hasReservedPrefix(key string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
