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
// Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification:
//
//	type EnvironmentClassifier struct {
//	    regoQuery *rego.PreparedEvalQuery
//	    logger    logr.Logger
//	}
//
// Classification is performed using Rego policies loaded from file.
// Graceful degradation returns "unknown" on policy errors.
package classifier

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// EnvironmentClassifier determines environment using Rego policy.
// Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification.
type EnvironmentClassifier struct {
	policyPath string
	logger     logr.Logger
}

// NewEnvironmentClassifier creates a new Rego-based environment classifier.
// DD-005 v2.0: Uses logr.Logger
func NewEnvironmentClassifier(policyPath string, logger logr.Logger) *EnvironmentClassifier {
	return &EnvironmentClassifier{
		policyPath: policyPath,
		logger:     logger.WithName("environment-classifier"),
	}
}

// Classify determines environment using Rego policy.
// Per plan: Priority order is namespace labels → signal labels → default
//
// BR-SP-051: Primary detection from namespace labels
// BR-SP-053: Default to "unknown" when all methods fail
//
// Never fails - always returns a valid EnvironmentClassification (graceful degradation).
func (c *EnvironmentClassifier) Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.EnvironmentClassification, error) {
	// Build input for Rego policy
	input := c.buildRegoInput(k8sCtx, signal)

	// Read policy file
	policyContent, err := os.ReadFile(c.policyPath)
	if err != nil {
		// Graceful degradation - policy file not found
		c.logger.Info("Policy file not found, returning default",
			"path", c.policyPath,
			"error", err)
		return c.defaultResult(), nil
	}

	// Prepare Rego query
	query, err := rego.New(
		rego.Query("data.signalprocessing.environment.result"),
		rego.Module("environment.rego", string(policyContent)),
	).PrepareForEval(ctx)

	if err != nil {
		// Graceful degradation - policy syntax error
		c.logger.Info("Policy compilation failed, returning default",
			"error", err)
		return c.defaultResult(), nil
	}

	// Evaluate policy
	results, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		// Graceful degradation - evaluation error
		c.logger.Info("Policy evaluation error, returning default",
			"error", err)
		return c.defaultResult(), nil
	}

	// Check for results
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		// Graceful degradation - no result
		c.logger.V(1).Info("No policy result, returning default")
		return c.defaultResult(), nil
	}

	// Extract result from Rego
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		// Graceful degradation - invalid result format
		c.logger.Info("Invalid policy result format, returning default",
			"value", fmt.Sprintf("%T", results[0].Expressions[0].Value))
		return c.defaultResult(), nil
	}

	// Extract fields from Rego result
	environment, _ := resultMap["environment"].(string)
	source, _ := resultMap["source"].(string)

	// Handle confidence - Rego returns json.Number, not float64
	var confidence float64
	switch v := resultMap["confidence"].(type) {
	case float64:
		confidence = v
	case int:
		confidence = float64(v)
	default:
		// Try to parse as json.Number (Rego's default for numbers)
		if num, ok := resultMap["confidence"].(interface{ Float64() (float64, error) }); ok {
			confidence, _ = num.Float64()
		}
	}

	if environment == "" {
		environment = "unknown"
	}
	if source == "" {
		source = "default"
	}

	c.logger.V(1).Info("Environment classified",
		"environment", environment,
		"confidence", confidence,
		"source", source)

	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment: environment,
		Confidence:  confidence,
		Source:      source,
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
			"labels": k8sCtx.Namespace.Labels,
		}
	} else {
		input["namespace"] = map[string]interface{}{
			"name":   "",
			"labels": map[string]string{},
		}
	}

	// Signal context
	if signal != nil {
		input["signal"] = map[string]interface{}{
			"labels": signal.Labels,
		}
	} else {
		input["signal"] = map[string]interface{}{
			"labels": map[string]string{},
		}
	}

	return input
}

// defaultResult returns the default environment classification.
// BR-SP-053: Default to "unknown" with 0.0 confidence.
func (c *EnvironmentClassifier) defaultResult() *signalprocessingv1alpha1.EnvironmentClassification {
	return &signalprocessingv1alpha1.EnvironmentClassification{
		Environment: "unknown",
		Confidence:  0.0,
		Source:      "default",
	}
}

