// Package classifier provides classification logic for Signal Processing.
// Business classifier implements BR-SP-080 (Confidence Scoring) and BR-SP-081 (Multi-dimensional Categorization).
package classifier

import (
	"context"

	"github.com/go-logr/logr"
)

// BusinessClassification holds the result of business context classification.
// BR-SP-081: Multi-dimensional categorization fields.
type BusinessClassification struct {
	// BusinessUnit identifies the business unit (e.g., "payments", "platform", "general")
	BusinessUnit string `json:"businessUnit"`

	// ServiceOwner is the team/individual responsible for the service
	ServiceOwner string `json:"serviceOwner"`

	// Criticality indicates business criticality ("critical", "high", "medium", "low")
	Criticality string `json:"criticality"`

	// SLARequirement is the SLA tier ("platinum", "gold", "silver", "bronze")
	SLARequirement string `json:"slaRequirement"`

	// OverallConfidence is the confidence score for the classification (0.0 to 1.0)
	// BR-SP-080: Confidence scoring based on data completeness
	OverallConfidence float64 `json:"overallConfidence"`
}

// BusinessClassifier performs multi-dimensional business context classification.
// DD-005 v2.0: Uses logr.Logger (unified interface for all Kubernaut services)
type BusinessClassifier struct {
	logger logr.Logger
}

// NewBusinessClassifier creates a new business classifier.
func NewBusinessClassifier(logger logr.Logger) *BusinessClassifier {
	return &BusinessClassifier{
		logger: logger.WithName("business-classifier"),
	}
}

// Classify performs multi-dimensional business categorization.
// BR-SP-080: Confidence scoring based on data completeness
// BR-SP-081: businessUnit, serviceOwner, criticality, sla
func (b *BusinessClassifier) Classify(ctx context.Context, namespaceLabels map[string]string, annotations map[string]string) *BusinessClassification {
	result := &BusinessClassification{
		BusinessUnit:      "general",
		ServiceOwner:      "unknown",
		Criticality:       "low",
		SLARequirement:    "bronze",
		OverallConfidence: 0.5, // Base confidence floor
	}

	// Track confidence adjustments
	confidenceBoost := 0.0

	// Extract business unit from namespace labels
	if namespaceLabels != nil {
		if team := namespaceLabels["team"]; team != "" {
			result.BusinessUnit = team
			confidenceBoost += 0.2
		}

		// Determine criticality based on labels
		if criticality := namespaceLabels["criticality"]; criticality != "" {
			result.Criticality = criticality
			confidenceBoost += 0.15
		} else if env := namespaceLabels["environment"]; env != "" {
			// Derive criticality from environment
			switch env {
			case "production":
				result.Criticality = "critical"
				confidenceBoost += 0.2 // Production has higher confidence
			case "staging":
				result.Criticality = "high"
				confidenceBoost += 0.15
			case "development":
				result.Criticality = "low"
				confidenceBoost += 0.1
			default:
				result.Criticality = "medium"
				confidenceBoost += 0.1
			}
		}

		// Check compliance labels for criticality boost
		if compliance := namespaceLabels["compliance"]; compliance != "" {
			if compliance == "pci-dss" || compliance == "hipaa" || compliance == "sox" {
				result.Criticality = "critical"
				confidenceBoost += 0.25 // High confidence for compliance data
			}
		}
	}

	// Extract service owner from annotations
	if annotations != nil {
		if owner := annotations["kubernaut.io/owner"]; owner != "" {
			result.ServiceOwner = owner
			confidenceBoost += 0.1
		} else if owner := annotations["owner"]; owner != "" {
			result.ServiceOwner = owner
			confidenceBoost += 0.1
		}

		// Extract SLA from annotations
		if sla := annotations["kubernaut.io/sla"]; sla != "" {
			result.SLARequirement = sla
			confidenceBoost += 0.1
		} else if sla := annotations["sla"]; sla != "" {
			result.SLARequirement = sla
			confidenceBoost += 0.1
		}
	}

	// Derive SLA from criticality if not explicitly set
	if result.SLARequirement == "bronze" {
		hasSLAAnnotation := false
		if annotations != nil {
			hasSLAAnnotation = annotations["sla"] != "" || annotations["kubernaut.io/sla"] != ""
		}
		if !hasSLAAnnotation {
			switch result.Criticality {
			case "critical":
				result.SLARequirement = "gold"
			case "high":
				result.SLARequirement = "silver"
			}
		}
	}

	// BR-SP-080: Calculate final confidence
	result.OverallConfidence += confidenceBoost

	// Cap at 1.0
	if result.OverallConfidence > 1.0 {
		result.OverallConfidence = 1.0
	}

	b.logger.V(1).Info("Business classification completed",
		"businessUnit", result.BusinessUnit,
		"criticality", result.Criticality,
		"confidence", result.OverallConfidence)

	return result
}

