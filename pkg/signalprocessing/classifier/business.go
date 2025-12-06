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

// Package classifier provides Rego-based classification for Signal Processing.
// Business Classifier: BR-SP-002, BR-SP-080, BR-SP-081
package classifier

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

const (
	// Per BR-SP-080: Default fallback confidence
	defaultFallbackConfidence = 0.4
	// Per BR-SP-080: Rego inference confidence
	regoInferenceConfidence = 0.6
	// Per BR-SP-080: Pattern match confidence
	patternMatchConfidence = 0.8
	// Per BR-SP-080: Explicit label match confidence
	explicitLabelConfidence = 1.0

	// Rego evaluation timeout (aligned with Priority Engine pattern)
	businessRegoEvalTimeout = 200 * time.Millisecond
)

// validCriticality defines valid criticality levels per BR-SP-081
var validCriticality = map[string]bool{
	"critical": true, "high": true, "medium": true, "low": true, "unknown": true,
}

// validSLATier defines valid SLA tiers per BR-SP-081
var validSLATier = map[string]bool{
	"platinum": true, "gold": true, "silver": true, "bronze": true, "unknown": true,
}

// BusinessClassifier determines business context using Rego policy.
// Per IMPLEMENTATION_PLAN_V1.25.md Day 6 specification.
//
// BR-SP-002: Multi-dimensional business categorization
// BR-SP-080: Confidence scoring (labels 1.0, pattern 0.8, Rego 0.6, default 0.4)
// BR-SP-081: businessUnit, serviceOwner, criticality, sla validation
type BusinessClassifier struct {
	regoQuery *rego.PreparedEvalQuery
	logger    logr.Logger // DD-005 v2.0: logr.Logger (not *zap.Logger)
	mu        sync.RWMutex
}

// NewBusinessClassifier creates a new Rego-based business classifier.
// Per BR-SP-002, BR-SP-080, BR-SP-081 specifications.
//
// DD-005 v2.0: Uses logr.Logger
func NewBusinessClassifier(ctx context.Context, policyPath string, logger logr.Logger) (*BusinessClassifier, error) {
	log := logger.WithName("business-classifier")

	// Read and compile initial policy
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file %s: %w", policyPath, err)
	}

	query, err := rego.New(
		rego.Query("data.signalprocessing.business.result"),
		rego.Module(filepath.Base(policyPath), string(policyContent)),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compile Rego policy: %w", err)
	}

	return &BusinessClassifier{
		regoQuery: &query,
		logger:    log,
	}, nil
}

// Classify performs multi-dimensional business categorization.
// BR-SP-002: businessUnit, serviceOwner, criticality, sla
// BR-SP-080: Confidence scoring (labels 1.0, pattern 0.8, Rego 0.6, default 0.4)
// BR-SP-081: Multi-dimensional categorization
//
// Priority order: Explicit Labels (1.0) → Pattern Match (0.8) → Rego (0.6) → Default (0.4)
func (b *BusinessClassifier) Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification) (*signalprocessingv1alpha1.BusinessClassification, error) {
	// Validate input - nil k8sCtx is an error
	if k8sCtx == nil {
		return nil, fmt.Errorf("kubernetes context is required")
	}

	// Initialize default result
	result := &signalprocessingv1alpha1.BusinessClassification{
		OverallConfidence: defaultFallbackConfidence,
		BusinessUnit:      "unknown",
		ServiceOwner:      "unknown",
		Criticality:       "unknown",
		SLARequirement:    "unknown",
	}

	// 1. Explicit Label Match (Confidence 1.0) - BR-SP-080
	if explicitResult := b.tryExplicitLabelMatch(k8sCtx); explicitResult != nil {
		return explicitResult, nil
	}

	// 2. Pattern Match (Confidence 0.8) - BR-SP-080
	if patternResult := b.tryPatternMatch(k8sCtx); patternResult != nil {
		return patternResult, nil
	}

	// 3. Rego Policy Evaluation (Confidence 0.6) - BR-SP-080
	regoResult, err := b.evaluateRego(ctx, k8sCtx, envClass)
	if err == nil && regoResult != nil && regoResult.BusinessUnit != "" && regoResult.BusinessUnit != "unknown" {
		return regoResult, nil
	}
	if err != nil {
		b.logger.V(1).Info("Rego evaluation failed, using default fallback", "error", err)
	}

	// 4. Default fallback (Confidence 0.4) - BR-SP-080
	return result, nil
}

// tryExplicitLabelMatch checks for explicit kubernaut.ai/* labels.
// Returns nil if no explicit labels found.
// Per BR-SP-080: Explicit label confidence = 1.0
func (b *BusinessClassifier) tryExplicitLabelMatch(k8sCtx *signalprocessingv1alpha1.KubernetesContext) *signalprocessingv1alpha1.BusinessClassification {
	var labels map[string]string

	// Check namespace labels first
	if k8sCtx.Namespace != nil && k8sCtx.Namespace.Labels != nil {
		labels = k8sCtx.Namespace.Labels
	}

	if labels == nil {
		return nil
	}

	// Look for explicit kubernaut.ai/* labels
	bu := labels["kubernaut.ai/business-unit"]
	so := labels["kubernaut.ai/service-owner"]
	crit := labels["kubernaut.ai/criticality"]
	sla := labels["kubernaut.ai/sla-tier"]

	// Require at least one explicit label
	if bu == "" && so == "" && crit == "" && sla == "" {
		return nil
	}

	result := &signalprocessingv1alpha1.BusinessClassification{
		OverallConfidence: explicitLabelConfidence,
		BusinessUnit:      normalizeValue(bu, "unknown"),
		ServiceOwner:      normalizeValue(so, "unknown"),
		Criticality:       normalizeValue(crit, "unknown"),
		SLARequirement:    normalizeValue(sla, "unknown"),
	}

	// Validate enum values
	if !validCriticality[result.Criticality] {
		result.Criticality = "unknown"
	}
	if !validSLATier[result.SLARequirement] {
		result.SLARequirement = "unknown"
	}

	b.logger.V(1).Info("Explicit label match found",
		"business_unit", result.BusinessUnit,
		"confidence", explicitLabelConfidence)

	return result
}

// tryPatternMatch attempts to extract business context from naming patterns.
// Returns nil if no pattern matches.
// Per BR-SP-080: Pattern match confidence = 0.8
func (b *BusinessClassifier) tryPatternMatch(k8sCtx *signalprocessingv1alpha1.KubernetesContext) *signalprocessingv1alpha1.BusinessClassification {
	if k8sCtx.Namespace == nil || k8sCtx.Namespace.Name == "" {
		return nil
	}

	name := k8sCtx.Namespace.Name

	// Common patterns: {service}-{env}, {team}-{service}, etc.
	patterns := []struct {
		prefix     string
		unit       string
		criticality string
	}{
		{"billing-", "billing", "high"},
		{"payments-", "payments", "high"},
		{"checkout-", "checkout", "high"},
		{"payment-", "payments", "high"},
		{"order-", "orders", "high"},
		{"api-", "platform", "high"},
		{"auth-", "platform", "critical"},
		{"platform-", "platform", "high"},
		{"infra-", "infrastructure", "high"},
		{"monitoring-", "observability", "medium"},
		{"logging-", "observability", "medium"},
		{"job-", "processing", "low"},
		{"batch-", "processing", "low"},
		{"worker-", "processing", "low"},
	}

	for _, p := range patterns {
		if strings.HasPrefix(name, p.prefix) {
			result := &signalprocessingv1alpha1.BusinessClassification{
				OverallConfidence: patternMatchConfidence,
				BusinessUnit:      p.unit,
				ServiceOwner:      "unknown",
				Criticality:       p.criticality,
				SLARequirement:    "unknown",
			}

			b.logger.V(1).Info("Pattern match found",
				"namespace", name,
				"pattern", p.prefix,
				"business_unit", result.BusinessUnit,
				"confidence", patternMatchConfidence)

			return result
		}
	}

	return nil
}

// evaluateRego evaluates the Rego policy for business classification.
// Per BR-SP-080: Rego inference confidence = 0.6
func (b *BusinessClassifier) evaluateRego(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification) (*signalprocessingv1alpha1.BusinessClassification, error) {
	input := b.buildRegoInput(k8sCtx, envClass)

	// Add timeout per plan (200ms)
	timeoutCtx, cancel := context.WithTimeout(ctx, businessRegoEvalTimeout)
	defer cancel()

	b.mu.RLock()
	query := b.regoQuery
	b.mu.RUnlock()

	results, err := query.Eval(timeoutCtx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("Rego evaluation failed: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		b.logger.V(1).Info("Rego returned no results")
		return nil, nil
	}

	return b.extractAndValidateResult(results)
}

// buildRegoInput constructs the input map for Rego policy evaluation.
func (b *BusinessClassifier) buildRegoInput(k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification) map[string]interface{} {
	input := map[string]interface{}{}

	// Environment
	if envClass != nil {
		input["environment"] = envClass.Environment
	} else {
		input["environment"] = "unknown"
	}

	// Namespace context
	if k8sCtx != nil && k8sCtx.Namespace != nil {
		input["namespace"] = map[string]interface{}{
			"name":        k8sCtx.Namespace.Name,
			"labels":      ensureLabelsMap(k8sCtx.Namespace.Labels),
			"annotations": ensureLabelsMap(k8sCtx.Namespace.Annotations),
		}
	} else {
		input["namespace"] = map[string]interface{}{
			"name":        "",
			"labels":      map[string]interface{}{},
			"annotations": map[string]interface{}{},
		}
	}

	// Deployment context (DeploymentDetails has labels/annotations but no name field)
	if k8sCtx != nil && k8sCtx.Deployment != nil {
		input["deployment"] = map[string]interface{}{
			"labels":      ensureLabelsMap(k8sCtx.Deployment.Labels),
			"annotations": ensureLabelsMap(k8sCtx.Deployment.Annotations),
		}
	} else {
		input["deployment"] = map[string]interface{}{
			"labels":      map[string]interface{}{},
			"annotations": map[string]interface{}{},
		}
	}

	return input
}

// extractAndValidateResult extracts and validates Rego output.
func (b *BusinessClassifier) extractAndValidateResult(results rego.ResultSet) (*signalprocessingv1alpha1.BusinessClassification, error) {
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		b.logger.V(1).Info("Invalid Rego output type")
		return nil, nil
	}

	// Use safe extraction with fallback to "unknown"
	bu, _ := resultMap["business_unit"].(string)
	so, _ := resultMap["service_owner"].(string)
	crit, _ := resultMap["criticality"].(string)
	sla, _ := resultMap["sla"].(string)

	// Normalize empty values
	if bu == "" {
		bu = "unknown"
	}
	if so == "" {
		so = "unknown"
	}
	if crit == "" {
		crit = "unknown"
	}
	if sla == "" {
		sla = "unknown"
	}

	// Validate enum values (BR-SP-081)
	if !validCriticality[crit] {
		crit = "unknown"
	}
	if !validSLATier[sla] {
		sla = "unknown"
	}

	result := &signalprocessingv1alpha1.BusinessClassification{
		BusinessUnit:      bu,
		ServiceOwner:      so,
		Criticality:       crit,
		SLARequirement:    sla,
		OverallConfidence: regoInferenceConfidence, // Rego confidence = 0.6
	}

	b.logger.V(1).Info("Rego classification result",
		"business_unit", result.BusinessUnit,
		"criticality", result.Criticality,
		"confidence", result.OverallConfidence)

	return result, nil
}

// normalizeValue returns the value if non-empty, otherwise returns defaultVal.
// Note: ensureLabelsMap is defined in helpers.go and shared across classifiers.
func normalizeValue(value, defaultVal string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return defaultVal
	}
	return v
}

