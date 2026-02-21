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
	// Per BR-SP-080: Rego evaluation timeout
	businessRegoTimeout = 200 * time.Millisecond

	// BR-SP-080: Confidence levels by detection method
	confidenceExplicitLabel = 1.0 // Explicit label match
	confidencePatternMatch  = 0.8 // Pattern match (namespace prefix)
	confidenceRegoInference = 0.6 // Rego policy inference
	confidenceDefault       = 0.4 // Default fallback

	// Label keys per BR-SP-002
	labelBusinessUnit = "kubernaut.ai/business-unit"
	labelServiceOwner = "kubernaut.ai/service-owner"
	labelCriticality  = "kubernaut.ai/criticality"
	labelSLATier      = "kubernaut.ai/sla-tier"
)

// Valid enum values per BR-SP-081
var (
	validCriticality = map[string]bool{"critical": true, "high": true, "medium": true, "low": true}
	validSLATier     = map[string]bool{"platinum": true, "gold": true, "silver": true, "bronze": true}
)

// BusinessClassifier determines business context using Rego policy.
// Per IMPLEMENTATION_PLAN_V1.25.md Day 6 specification.
//
// BR-SP-002: Multi-dimensional business categorization
// BR-SP-080: Confidence scoring (labels 1.0, pattern 0.8, Rego 0.6, default 0.4)
// BR-SP-081: businessUnit, serviceOwner, criticality, sla validation
type BusinessClassifier struct {
	regoQuery  *rego.PreparedEvalQuery
	policyPath string      // For hot-reload support
	logger     logr.Logger // DD-005 v2.0: logr.Logger (not *zap.Logger)
	mu         sync.RWMutex
}

// classificationWithConfidence tracks per-field confidence internally.
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2444-2453
type classificationWithConfidence struct {
	*signalprocessingv1alpha1.BusinessClassification
	businessUnitConfidence float64
	serviceOwnerConfidence float64
	criticalityConfidence  float64
	slaConfidence          float64
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
		regoQuery:  &query,
		policyPath: policyPath,
		logger:     log,
	}, nil
}

// Classify performs multi-dimensional business categorization.
// Per BR-SP-002: Classification from namespace/deployment labels OR Rego policies.
// Per BR-SP-080: 4-tier confidence scoring (1.0 label → 0.8 pattern → 0.6 Rego → 0.4 default)
// Per BR-SP-081: businessUnit, serviceOwner, criticality, sla dimensions
//
// NOTE: priority is NOT an input - business classification is independent of priority assignment.
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2229-2266
func (b *BusinessClassifier) Classify(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification) (*signalprocessingv1alpha1.BusinessClassification, error) {
	// Validate inputs
	if k8sCtx == nil {
		return nil, fmt.Errorf("kubernetes context is required")
	}

	// Initialize result with internal confidence tracking
	result := &classificationWithConfidence{
		BusinessClassification: &signalprocessingv1alpha1.BusinessClassification{},
	}

	// BR-SP-080: 4-tier detection with confidence scoring
	// Tier 1: Explicit label match (confidence 1.0)
	b.classifyFromLabels(k8sCtx, result)

	// Tier 2: Pattern match (confidence 0.8) - if label detection incomplete
	if result.BusinessUnit == "" || result.ServiceOwner == "" {
		b.classifyFromPatterns(k8sCtx, result)
	}

	// Tier 3: Rego inference (confidence 0.6) - for remaining fields
	if b.needsRegoClassification(result) {
		if err := b.classifyFromRego(ctx, k8sCtx, envClass, result); err != nil {
			b.logger.V(1).Info("Rego classification failed, using defaults", "error", err)
		}
	}

	// Tier 4: Default fallback (confidence 0.4) - for any remaining unknown fields
	b.applyDefaults(result)

	b.logger.V(1).Info("Business classification complete",
		"business_unit", result.BusinessUnit,
		"service_owner", result.ServiceOwner,
		"criticality", result.Criticality,
		"sla", result.SLARequirement)

	return result.BusinessClassification, nil
}

// classifyFromLabels extracts business fields from explicit kubernaut.ai/ labels.
// Per BR-SP-002 + BR-SP-080: Label-based classification (confidence 1.0)
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2268-2293
func (b *BusinessClassifier) classifyFromLabels(k8sCtx *signalprocessingv1alpha1.KubernetesContext, result *classificationWithConfidence) {
	labels := b.collectLabels(k8sCtx)

	if val, ok := labels[labelBusinessUnit]; ok && val != "" {
		result.BusinessUnit = val
		result.businessUnitConfidence = confidenceExplicitLabel
	}
	if val, ok := labels[labelServiceOwner]; ok && val != "" {
		result.ServiceOwner = val
		result.serviceOwnerConfidence = confidenceExplicitLabel
	}
	if val, ok := labels[labelCriticality]; ok && val != "" {
		normalized := strings.ToLower(val)
		if validCriticality[normalized] {
			result.Criticality = normalized
			result.criticalityConfidence = confidenceExplicitLabel
		}
	}
	if val, ok := labels[labelSLATier]; ok && val != "" {
		normalized := strings.ToLower(val)
		if validSLATier[normalized] {
			result.SLARequirement = normalized
			result.slaConfidence = confidenceExplicitLabel
		}
	}
}

// classifyFromPatterns uses namespace naming patterns.
// Per BR-SP-080: Pattern match (confidence 0.8)
// Examples: "payments-prod" → business_unit="payments", "billing-staging" → "billing"
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2295-2313
func (b *BusinessClassifier) classifyFromPatterns(k8sCtx *signalprocessingv1alpha1.KubernetesContext, result *classificationWithConfidence) {
	if k8sCtx.Namespace == nil {
		return
	}

	nsName := k8sCtx.Namespace.Name
	parts := strings.Split(nsName, "-")
	if len(parts) > 0 && result.BusinessUnit == "" {
		// First segment before hyphen as potential business unit
		potentialUnit := parts[0]
		if len(potentialUnit) > 2 { // Avoid short prefixes like "ns"
			result.BusinessUnit = potentialUnit
			result.businessUnitConfidence = confidencePatternMatch
		}
	}
}

// classifyFromRego evaluates business Rego policy for inference.
// Per BR-SP-080: Rego inference (confidence 0.6)
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2315-2337
func (b *BusinessClassifier) classifyFromRego(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, result *classificationWithConfidence) error {
	input := b.buildRegoInput(k8sCtx, envClass)

	timeoutCtx, cancel := context.WithTimeout(ctx, businessRegoTimeout)
	defer cancel()

	b.mu.RLock()
	query := b.regoQuery
	b.mu.RUnlock()

	results, err := query.Eval(timeoutCtx, rego.EvalInput(input))
	if err != nil {
		return fmt.Errorf("rego evaluation failed: %w", err)
	}

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return nil // No results, will use defaults
	}

	return b.extractRegoResults(results, result)
}

// extractRegoResults safely extracts Rego output with type checking.
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2339-2369
func (b *BusinessClassifier) extractRegoResults(results rego.ResultSet, result *classificationWithConfidence) error {
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid Rego output type")
	}

	// Safe extraction with validation - only fill fields that are still empty
	if val, ok := resultMap["business_unit"].(string); ok && val != "" && result.BusinessUnit == "" {
		result.BusinessUnit = val
		result.businessUnitConfidence = confidenceRegoInference
	}
	if val, ok := resultMap["service_owner"].(string); ok && val != "" && result.ServiceOwner == "" {
		result.ServiceOwner = val
		result.serviceOwnerConfidence = confidenceRegoInference
	}
	if val, ok := resultMap["criticality"].(string); ok && val != "" && result.Criticality == "" {
		normalized := strings.ToLower(val)
		if validCriticality[normalized] {
			result.Criticality = normalized
			result.criticalityConfidence = confidenceRegoInference
		}
	}
	if val, ok := resultMap["sla"].(string); ok && val != "" && result.SLARequirement == "" {
		normalized := strings.ToLower(val)
		if validSLATier[normalized] {
			result.SLARequirement = normalized
			result.slaConfidence = confidenceRegoInference
		}
	}

	return nil
}

// applyDefaults sets safe defaults for any unclassified fields.
// Per BR-SP-081: "unknown" if not determinable
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2371-2390
func (b *BusinessClassifier) applyDefaults(result *classificationWithConfidence) {
	if result.BusinessUnit == "" {
		result.BusinessUnit = "unknown"
		result.businessUnitConfidence = confidenceDefault
	}
	if result.ServiceOwner == "" {
		result.ServiceOwner = "unknown"
		result.serviceOwnerConfidence = confidenceDefault
	}
	if result.Criticality == "" {
		result.Criticality = "medium" // Safe default per plan
		result.criticalityConfidence = confidenceDefault
	}
	if result.SLARequirement == "" {
		result.SLARequirement = "bronze" // Lowest tier default per plan
		result.slaConfidence = confidenceDefault
	}
}

// collectLabels merges namespace and workload labels.
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2399-2413
func (b *BusinessClassifier) collectLabels(k8sCtx *signalprocessingv1alpha1.KubernetesContext) map[string]string {
	labels := make(map[string]string)
	if k8sCtx.Namespace != nil {
		for k, v := range k8sCtx.Namespace.Labels {
			labels[k] = v
		}
	}
	if k8sCtx.Workload != nil {
		for k, v := range k8sCtx.Workload.Labels {
			labels[k] = v
		}
	}
	return labels
}

// needsRegoClassification checks if any fields still need classification.
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2415-2418
func (b *BusinessClassifier) needsRegoClassification(result *classificationWithConfidence) bool {
	return result.BusinessUnit == "" || result.ServiceOwner == "" ||
		result.Criticality == "" || result.SLARequirement == ""
}

// buildRegoInput constructs the input map for Rego policy evaluation.
// Per IMPLEMENTATION_PLAN_V1.25.md lines 2420-2439
func (b *BusinessClassifier) buildRegoInput(k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification) map[string]interface{} {
	input := map[string]interface{}{
		"environment": "",
	}
	if envClass != nil {
		input["environment"] = envClass.Environment
	}
	if k8sCtx != nil && k8sCtx.Namespace != nil {
		input["namespace"] = map[string]interface{}{
			"name":        k8sCtx.Namespace.Name,
			"labels":      ensureLabelsMap(k8sCtx.Namespace.Labels),
			"annotations": ensureLabelsMap(k8sCtx.Namespace.Annotations),
		}
	}
	if k8sCtx != nil && k8sCtx.Workload != nil {
		input["workload"] = map[string]interface{}{
			"kind":        k8sCtx.Workload.Kind,
			"name":        k8sCtx.Workload.Name,
			"labels":      ensureLabelsMap(k8sCtx.Workload.Labels),
			"annotations": ensureLabelsMap(k8sCtx.Workload.Annotations),
		}
	}
	return input
}
