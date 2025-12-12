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
//	    regoQuery        *rego.PreparedEvalQuery // Prepared query for performance
//	    k8sClient        client.Client           // For ConfigMap fallback (BR-SP-052)
//	    logger           logr.Logger             // DD-005 v2.0: logr.Logger
//	    configMapMu      sync.RWMutex            // Thread safety for ConfigMap cache
//	    configMapMapping map[string]string       // Namespace pattern → environment mapping
//	}
//
// Priority order: namespace labels → ConfigMap (BR-SP-052) → signal labels → default
// Graceful degradation returns "unknown" on policy errors.
package classifier

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

const (
	// ConfigMap names for BR-SP-052 fallback
	environmentConfigMapName      = "kubernaut-environment-config"
	environmentConfigMapNamespace = "kubernaut-system"
	environmentConfigMapKey       = "mapping"

	// Confidence levels per plan
	namespaceLabelsConfidence = 0.95
	configMapConfidence       = 0.75
	signalLabelsConfidence    = 0.80
	defaultConfidence         = 0.0
)

// EnvironmentClassifier determines environment using Rego policy.
// Per IMPLEMENTATION_PLAN_V1.22.md Day 4 specification.
type EnvironmentClassifier struct {
	regoQuery *rego.PreparedEvalQuery // Prepared query for performance
	k8sClient client.Client           // For ConfigMap fallback (BR-SP-052)
	logger    logr.Logger

	// ConfigMap cache
	configMapMu      sync.RWMutex
	configMapMapping map[string]string // pattern -> environment
}

// NewEnvironmentClassifier creates a new Rego-based environment classifier.
// Per plan: query is prepared once at construction time for performance.
//
// BR-SP-051: Primary detection from namespace labels
// BR-SP-052: ConfigMap fallback when labels absent
// BR-SP-053: Default to "unknown" when all methods fail
//
// DD-005 v2.0: Uses logr.Logger
func NewEnvironmentClassifier(ctx context.Context, policyPath string, k8sClient client.Client, logger logr.Logger) (*EnvironmentClassifier, error) {
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
		regoQuery:        &query,
		k8sClient:        k8sClient,
		logger:           log,
		configMapMapping: make(map[string]string),
	}

	// Load ConfigMap mapping (BR-SP-052)
	if err := classifier.loadConfigMapMapping(ctx); err != nil {
		// Log warning but don't fail - ConfigMap is optional fallback
		log.Info("ConfigMap mapping not loaded (will use defaults)",
			"configMap", environmentConfigMapName,
			"error", err)
	}

	return classifier, nil
}

// Classify determines environment using Rego policy and Go fallbacks.
// Per plan (line 1864): Priority order is namespace labels → ConfigMap → signal labels → default
//
// 1. BR-SP-051: Primary detection from namespace labels (confidence 0.95) - via Rego
// 2. BR-SP-052: ConfigMap fallback (confidence 0.75) - via Go
// 3. Signal labels fallback (confidence 0.80) - via Go
// 4. BR-SP-053: Default to "unknown" when all methods fail (confidence 0.0) - via Go
//
// Never fails - always returns a valid EnvironmentClassification (graceful degradation).
func (c *EnvironmentClassifier) Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	// Build input for Rego policy (namespace labels only)
	input := c.buildRegoInput(k8sCtx, signal)

	// Step 1: Try namespace labels via Rego (confidence 0.95)
	result, err := c.evaluateRego(ctx, input)
	if err != nil {
		c.logger.Info("Rego evaluation failed, trying fallbacks",
			"error", err)
	} else if result.Environment != "unknown" {
		// Rego found an environment from namespace labels
		c.logger.V(1).Info("Environment classified via namespace labels",
			"environment", result.Environment,
			"confidence", result.Confidence,
			"source", result.Source)
		return result, nil
	}

	// Step 2: Try ConfigMap fallback (confidence 0.75) - BR-SP-052
	if k8sCtx != nil && k8sCtx.Namespace != nil {
		if env := c.tryConfigMapFallback(k8sCtx.Namespace.Name); env != "" {
			result := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment:  strings.ToLower(env), // Case-insensitive per BR-SP-051
				Confidence:   configMapConfidence,
				Source:       "configmap",
				ClassifiedAt: metav1.Now(),
			}
			c.logger.V(1).Info("Environment classified via ConfigMap",
				"namespace", k8sCtx.Namespace.Name,
				"environment", result.Environment)
			return result, nil
		}
	}

	// Step 3: Try signal labels fallback (confidence 0.80)
	if env := c.trySignalLabelsFallback(signal); env != "" {
		result := &signalprocessingv1alpha1.EnvironmentClassification{
			Environment:  strings.ToLower(env), // Case-insensitive per BR-SP-051
			Confidence:   signalLabelsConfidence,
			Source:       "signal-labels",
			ClassifiedAt: metav1.Now(),
		}
		c.logger.V(1).Info("Environment classified via signal labels",
			"environment", result.Environment)
		return result, nil
	}

	// Step 4: Default fallback (confidence 0.0) - BR-SP-053
	c.logger.V(1).Info("No environment detected, returning default")
	return c.defaultResult(), nil
}

// trySignalLabelsFallback checks signal labels for environment.
// Per plan: Signal labels are checked after ConfigMap, before default.
func (c *EnvironmentClassifier) trySignalLabelsFallback(signal *signalprocessingv1alpha1.SignalData) string {
	if signal == nil || signal.Labels == nil {
		return ""
	}
	return signal.Labels["kubernaut.ai/environment"]
}

// evaluateRego evaluates the prepared Rego query.
func (c *EnvironmentClassifier) evaluateRego(ctx context.Context, input map[string]interface{}) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	results, err := c.regoQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("rego evaluation failed: %w", err)
	}

	// Check for results
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return c.defaultResult(), nil
	}

	// Extract result from Rego
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		c.logger.Info("Invalid Rego result format",
			"value", fmt.Sprintf("%T", results[0].Expressions[0].Value))
		return c.defaultResult(), nil
	}

	// Extract fields from Rego result
	environment, _ := resultMap["environment"].(string)
	source, _ := resultMap["source"].(string)

	// Handle confidence - Rego returns json.Number, not float64
	confidence := c.extractConfidenceFromResult(resultMap["confidence"])

	if environment == "" {
		environment = "unknown"
	}
	if source == "" {
		source = "default"
	}

	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment:  environment,
		Confidence:   confidence,
		Source:       source,
		ClassifiedAt: metav1.Now(), // Set timestamp in Go, not Rego
	}, nil
}

// extractConfidenceFromResult handles the various types Rego can return for numbers.
// Wraps the shared extractConfidence function with default handling.
func (c *EnvironmentClassifier) extractConfidenceFromResult(v interface{}) float64 {
	conf := extractConfidence(v)
	if conf == 0.0 {
		return defaultConfidence
	}
	return conf
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

// loadConfigMapMapping loads the namespace→environment mapping from ConfigMap.
// BR-SP-052: ConfigMap-based environment mapping
func (c *EnvironmentClassifier) loadConfigMapMapping(ctx context.Context) error {
	if c.k8sClient == nil {
		return fmt.Errorf("k8s client is nil")
	}

	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      environmentConfigMapName,
		Namespace: environmentConfigMapNamespace,
	}

	if err := c.k8sClient.Get(ctx, key, configMap); err != nil {
		return fmt.Errorf("failed to get ConfigMap %s/%s: %w", environmentConfigMapNamespace, environmentConfigMapName, err)
	}

	mappingData, ok := configMap.Data[environmentConfigMapKey]
	if !ok {
		return fmt.Errorf("ConfigMap %s missing key %s", environmentConfigMapName, environmentConfigMapKey)
	}

	c.configMapMu.Lock()
	defer c.configMapMu.Unlock()

	// Parse simple YAML format: "pattern: environment"
	c.configMapMapping = make(map[string]string)
	for _, line := range strings.Split(mappingData, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		pattern := strings.TrimSpace(parts[0])
		env := strings.TrimSpace(parts[1])
		if pattern != "" && env != "" {
			c.configMapMapping[pattern] = env
		}
	}

	c.logger.V(1).Info("Loaded ConfigMap mapping",
		"patterns", len(c.configMapMapping))

	return nil
}

// tryConfigMapFallback attempts to match namespace name against ConfigMap patterns.
// BR-SP-052: Support namespace pattern → environment mapping
func (c *EnvironmentClassifier) tryConfigMapFallback(namespaceName string) string {
	c.configMapMu.RLock()
	defer c.configMapMu.RUnlock()

	for pattern, env := range c.configMapMapping {
		if matchPattern(pattern, namespaceName) {
			return env
		}
	}
	return ""
}

// matchPattern matches a namespace name against a pattern with * wildcard.
// Supports: "prod-*" matches "prod-payments", "prod-api", etc.
func matchPattern(pattern, name string) bool {
	// Simple glob matching with * wildcard
	if !strings.Contains(pattern, "*") {
		return pattern == name
	}

	// Split pattern by *
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		prefix := parts[0]
		suffix := parts[1]
		return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix)
	}

	// More complex patterns - fall back to exact match
	return pattern == name
}

// defaultResult returns the default environment classification.
// BR-SP-053: Default to "unknown" with 0.0 confidence.
func (c *EnvironmentClassifier) defaultResult() *signalprocessingv1alpha1.EnvironmentClassification {
	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment:  "unknown",
		Confidence:   defaultConfidence,
		Source:       "default",
		ClassifiedAt: metav1.Now(),
	}
}

// ReloadConfigMap reloads the ConfigMap mapping (for hot-reload support).
// BR-SP-052: Hot-reload mapping without restart
func (c *EnvironmentClassifier) ReloadConfigMap(ctx context.Context) error {
	return c.loadConfigMapMapping(ctx)
}
