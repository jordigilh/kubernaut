<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package context

import (
	"context"
	"strings"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ComplexityClassifier implements BR-CONTEXT-016 to BR-CONTEXT-020
// Dynamic investigation complexity assessment
type ComplexityClassifier struct {
	config *config.ContextOptimizationConfig
}

// NewComplexityClassifier creates a new complexity classifier
func NewComplexityClassifier(cfg *config.ContextOptimizationConfig) *ComplexityClassifier {
	return &ComplexityClassifier{
		config: cfg,
	}
}

// Assess evaluates alert complexity and provides recommendations
func (c *ComplexityClassifier) Assess(ctx context.Context, alert types.Alert) (*ComplexityAssessment, error) {
	// Extract alert characteristics
	characteristics := c.extractCharacteristics(alert)

	// Calculate complexity score
	complexityScore := c.calculateComplexityScore(alert, characteristics)

	// Determine tier based on score
	tier := c.determineTier(complexityScore, alert)

	// Get tier configuration
	tierConfig, exists := c.config.GraduatedReduction.Tiers[tier]
	if !exists {
		// Default to moderate tier
		tierConfig = config.ReductionTier{
			MaxReduction:    0.40,
			MinContextTypes: 2,
		}
	}

	// Determine if escalation is required
	escalationRequired := c.requiresEscalation(alert, tier, characteristics)

	// Calculate confidence score
	confidenceScore := c.calculateConfidenceScore(alert, characteristics, complexityScore)

	return &ComplexityAssessment{
		Tier:                 tier,
		ConfidenceScore:      confidenceScore,
		RecommendedReduction: tierConfig.MaxReduction,
		MinContextTypes:      tierConfig.MinContextTypes,
		Characteristics:      characteristics,
		EscalationRequired:   escalationRequired,
		Metadata: map[string]interface{}{
			"complexity_score":   complexityScore,
			"alert_severity":     alert.Severity,
			"namespace":          alert.Namespace,
			"assessment_method":  "rule_based_heuristic",
			"confidence_factors": c.getConfidenceFactors(alert, characteristics),
		},
	}, nil
}

// extractCharacteristics identifies key characteristics of the alert
func (c *ComplexityClassifier) extractCharacteristics(alert types.Alert) []string {
	var characteristics []string

	alertText := strings.ToLower(alert.Name + " " + alert.Description)

	// System impact characteristics
	if strings.Contains(alertText, "crash") || strings.Contains(alertText, "panic") {
		characteristics = append(characteristics, "system_crash")
	}
	if strings.Contains(alertText, "memory") || strings.Contains(alertText, "oom") {
		characteristics = append(characteristics, "memory_issue")
	}
	if strings.Contains(alertText, "cpu") {
		characteristics = append(characteristics, "cpu_issue")
	}
	if strings.Contains(alertText, "network") || strings.Contains(alertText, "connection") {
		characteristics = append(characteristics, "network_issue")
	}
	if strings.Contains(alertText, "storage") || strings.Contains(alertText, "disk") {
		characteristics = append(characteristics, "storage_issue")
	}

	// Security characteristics
	if strings.Contains(alertText, "security") || strings.Contains(alertText, "breach") || strings.Contains(alertText, "unauthorized") {
		characteristics = append(characteristics, "security_concern")
	}

	// Service impact characteristics
	if strings.Contains(alertText, "service") && (strings.Contains(alertText, "down") || strings.Contains(alertText, "unavailable")) {
		characteristics = append(characteristics, "service_outage")
	}

	// Data characteristics
	if strings.Contains(alertText, "database") || strings.Contains(alertText, "data") {
		characteristics = append(characteristics, "data_related")
	}

	// Kubernetes specific characteristics
	if strings.Contains(alertText, "pod") || strings.Contains(alertText, "deployment") || strings.Contains(alertText, "node") {
		characteristics = append(characteristics, "kubernetes_infrastructure")
	}

	// Namespace characteristics
	if alert.Namespace == "production" || alert.Namespace == "prod" {
		characteristics = append(characteristics, "production_environment")
	}

	return characteristics
}

// calculateComplexityScore computes a numerical complexity score
func (c *ComplexityClassifier) calculateComplexityScore(alert types.Alert, characteristics []string) float64 {
	score := 0.0

	// Base score from severity
	switch strings.ToLower(alert.Severity) {
	case "critical":
		score += 0.4
	case "warning":
		score += 0.2
	case "info":
		score += 0.1
	}

	// Score from characteristics
	for _, char := range characteristics {
		switch char {
		case "system_crash":
			score += 0.3
		case "security_concern":
			score += 0.35
		case "service_outage":
			score += 0.25
		case "network_issue":
			score += 0.2
		case "memory_issue":
			score += 0.15
		case "data_related":
			score += 0.15
		case "production_environment":
			score += 0.1
		case "kubernetes_infrastructure":
			score += 0.1
		default:
			score += 0.05
		}
	}

	// Environmental multipliers
	if alert.Namespace == "production" || alert.Namespace == "prod" {
		score *= 1.2
	}

	// Normalize score to 0-1 range
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// determineTier maps complexity score to tier
func (c *ComplexityClassifier) determineTier(score float64, alert types.Alert) string {
	// Special cases for critical alerts
	if strings.ToLower(alert.Severity) == "critical" {
		if score >= 0.7 {
			return "critical"
		}
		if score >= 0.5 {
			return "complex"
		}
	}

	// Score-based determination
	switch {
	case score >= 0.8:
		return "critical"
	case score >= 0.6:
		return "complex"
	case score >= 0.3:
		return "moderate"
	default:
		return "simple"
	}
}

// requiresEscalation determines if the alert requires escalation
func (c *ComplexityClassifier) requiresEscalation(alert types.Alert, tier string, characteristics []string) bool {
	// Critical tier always requires escalation
	if tier == "critical" {
		return true
	}

	// Security concerns require escalation
	for _, char := range characteristics {
		if char == "security_concern" {
			return true
		}
		if char == "service_outage" && alert.Namespace == "production" {
			return true
		}
	}

	return false
}

// calculateConfidenceScore computes confidence in the assessment
func (c *ComplexityClassifier) calculateConfidenceScore(alert types.Alert, characteristics []string, complexityScore float64) float64 {
	confidence := 0.7 // Base confidence

	// Higher confidence for clear indicators
	if len(characteristics) >= 3 {
		confidence += 0.1
	}

	// Higher confidence for production alerts
	if alert.Namespace == "production" {
		confidence += 0.05
	}

	// Higher confidence for critical severity
	if strings.ToLower(alert.Severity) == "critical" {
		confidence += 0.1
	}

	// Adjust based on complexity score clarity
	if complexityScore >= 0.8 || complexityScore <= 0.2 {
		confidence += 0.05 // Clear high or low complexity
	}

	// Ensure confidence is in valid range
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.5 {
		confidence = 0.5
	}

	return confidence
}

// getConfidenceFactors returns factors contributing to confidence score
func (c *ComplexityClassifier) getConfidenceFactors(alert types.Alert, characteristics []string) map[string]interface{} {
	return map[string]interface{}{
		"characteristic_count":   len(characteristics),
		"severity_clarity":       strings.ToLower(alert.Severity) == "critical",
		"production_environment": alert.Namespace == "production",
		"clear_indicators":       len(characteristics) >= 2,
	}
}
