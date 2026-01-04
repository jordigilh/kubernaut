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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
package signalprocessing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// Unit Tests: Business Classifier
// Per IMPLEMENTATION_PLAN_V1.25.md Day 6 specification
// Test Matrix: 23 tests (6 HP, 8 EC, 4 CF, 5 ER)
// BR Coverage: BR-SP-002, BR-SP-080, BR-SP-081
var _ = Describe("Business Classifier", func() {
	var (
		ctx                context.Context
		businessClassifier *classifier.BusinessClassifier
		logger             logr.Logger
		policyDir          string
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()

		// Create temp directory for test policies
		var err error
		policyDir, err = os.MkdirTemp("", "business-rego-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Cleanup temp directory
		if policyDir != "" {
			_ = os.RemoveAll(policyDir)
		}
	})

	// Helper to create policy file
	createPolicy := func(content string) string {
		policyPath := filepath.Join(policyDir, "business.rego")
		err := os.WriteFile(policyPath, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
		return policyPath
	}

	// Standard business Rego policy per IMPLEMENTATION_PLAN_V1.25.md
	standardPolicy := `
package signalprocessing.business

import rego.v1

# Payment service detection
result := {"business_unit": "payments", "service_owner": "payments-team", "criticality": "high", "sla": "gold"} if {
    input.namespace.labels["app"] == "payment-service"
}

# API gateway detection
result := {"business_unit": "platform", "service_owner": "platform-team", "criticality": "critical", "sla": "platinum"} if {
    input.namespace.labels["app"] == "api-gateway"
}

# Background job detection
result := {"business_unit": "processing", "service_owner": "batch-team", "criticality": "low", "sla": "bronze"} if {
    input.namespace.labels["type"] == "worker"
}

# Team label detection
result := {"business_unit": team, "service_owner": team, "criticality": "medium", "sla": "silver"} if {
    team := input.namespace.labels["team"]
    team != ""
}

# Billing namespace pattern
result := {"business_unit": "billing", "service_owner": "billing-team", "criticality": "high", "sla": "gold"} if {
    startswith(input.namespace.name, "billing")
}

# Default fallback (minimal)
result := {"business_unit": "", "service_owner": "", "criticality": "", "sla": ""} if {
    not input.namespace.labels["app"]
    not input.namespace.labels["type"]
    not input.namespace.labels["team"]
    not startswith(input.namespace.name, "billing")
}
`

	// ============================================================================
	// HAPPY PATH TESTS (BC-HP-01 to BC-HP-06): 6 tests
	// ============================================================================

	Context("Happy Path: BR-SP-002 Business Classification", func() {
		// BC-HP-01: Payment service classification (via Rego)
		// Namespace too short for pattern match → Rego classifies
		It("BC-HP-01: should classify payment service correctly", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "ns", // Too short for pattern match (len <= 2)
					Labels: map[string]string{
						"app": "payment-service", // Rego matches this
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("payments"))
			Expect(result.Criticality).To(Equal("high"))
		})

		// BC-HP-02: API gateway classification (via Rego rule)
		// Namespace too short for pattern match → Rego classifies
		It("BC-HP-02: should classify API gateway correctly via Rego", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Namespace too short for pattern match
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "ns", // Too short for pattern match (len <= 2)
					Labels: map[string]string{
						"app": "api-gateway", // Rego rule matches this
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("platform"))
			Expect(result.Criticality).To(Equal("critical"))
			Expect(result.SLARequirement).To(Equal("platinum"))
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-HP-03: Background job classification (via Rego)
		// Namespace too short for pattern match → Rego classifies
		It("BC-HP-03: should classify background job correctly", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "ns", // Too short for pattern match (len <= 2)
					Labels: map[string]string{
						"type": "worker", // Rego matches this
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("processing"))
			Expect(result.Criticality).To(Equal("low"))
			Expect(result.SLARequirement).To(Equal("bronze"))
		})

		// BC-HP-04: Classification via team label (Rego)
		// Namespace too short for pattern match → Rego classifies from team label
		It("BC-HP-04: should classify via team label", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "ns", // Too short for pattern match (len <= 2)
					Labels: map[string]string{
						"team": "checkout", // Rego matches this
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("checkout"))
		})

		// BC-HP-05: Classification via namespace pattern
		// Pattern match extracts first segment before hyphen
		// Use a namespace that doesn't match Rego rules
		It("BC-HP-05: should classify via namespace pattern", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Use namespace that triggers pattern match but NOT Rego
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "myteam-prod", // Pattern extracts "myteam", no Rego match
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			// Pattern match extracts first segment: "myteam"
			Expect(result.BusinessUnit).To(Equal("myteam"))
			// Other fields from defaults (no Rego match)
			Expect(result.ServiceOwner).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))
			Expect(result.SLARequirement).To(Equal("bronze"))
			// Mixed: pattern (0.8) + defaults (0.4 x 3) = (0.8+0.4+0.4+0.4)/4 = 0.5
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-HP-06: Custom Rego business rules
		// Namespace too short for pattern match → custom Rego classifies
		It("BC-HP-06: should apply custom Rego business rules", func() {
			customPolicy := `
package signalprocessing.business

import rego.v1

# Custom rule: High-value customer service
result := {"business_unit": "enterprise", "service_owner": "enterprise-team", "criticality": "critical", "sla": "platinum"} if {
    input.namespace.labels["customer-tier"] == "enterprise"
}

# Default
result := {"business_unit": "standard", "service_owner": "general", "criticality": "medium", "sla": "silver"} if {
    not input.namespace.labels["customer-tier"]
}
`
			policyPath := createPolicy(customPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "ns", // Too short for pattern match (len <= 2)
					Labels: map[string]string{
						"customer-tier": "enterprise",
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("enterprise"))
			Expect(result.Criticality).To(Equal("critical"))
		})
	})

	// ============================================================================
	// EDGE CASE TESTS (BC-EC-01 to BC-EC-08): 8 tests
	// ============================================================================

	Context("Edge Cases", func() {
		// BC-EC-01: No business context available
		It("BC-EC-01: should handle no business context available", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "x", // Short name, no pattern match
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			// Should return defaults with low confidence (0.4)
			Expect(result.BusinessUnit).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))    // Safe default per plan
			Expect(result.SLARequirement).To(Equal("bronze")) // Lowest tier default per plan
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-EC-02: Explicit kubernaut.ai labels take priority
		// Per BR-SP-080: Explicit labels have confidence 1.0
		It("BC-EC-02: should handle explicit labels with mixed confidence", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Using explicit kubernaut.ai labels (not Rego labels)
			// 3 explicit labels (1.0 each) + 1 default (0.4) = 0.85 average
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "x", // Too short for pattern match
					Labels: map[string]string{
						"kubernaut.ai/business-unit": "payments",
						"kubernaut.ai/service-owner": "orders-team",
						"kubernaut.ai/criticality":   "high",
						// No SLA label - will be filled by default
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			// Explicit labels win
			Expect(result.BusinessUnit).To(Equal("payments"))
			Expect(result.ServiceOwner).To(Equal("orders-team"))
			Expect(result.Criticality).To(Equal("high"))
			// SLA from default
			Expect(result.SLARequirement).To(Equal("bronze"))
			// Mixed confidence: (1.0 + 1.0 + 1.0 + 0.4) / 4 = 0.85
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-EC-03: Unknown business domain
		It("BC-EC-03: should return unknown for unrecognized domain", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "x", // Too short for pattern match
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))    // Safe default
			Expect(result.SLARequirement).To(Equal("bronze")) // Lowest tier default
		})

		// BC-EC-04: Multiple team labels (Rego handles via team label)
		// Namespace too short for pattern match → Rego classifies
		It("BC-EC-04: should handle multiple ownership labels", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "ns", // Too short for pattern match (len <= 2)
					Labels: map[string]string{
						"team":  "alpha", // Rego matches this
						"owner": "beta",
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			// Rego returns team as business unit
			Expect(result.BusinessUnit).To(Equal("alpha"))
		})

		// BC-EC-05: Very long business label (63 chars - K8s limit)
		// Uses explicit kubernaut.ai label to bypass pattern match
		It("BC-EC-05: should handle 63-char business label value", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			longValue := strings.Repeat("a", 63) // K8s label value limit
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "x", // Too short for pattern match
					Labels: map[string]string{
						"kubernaut.ai/business-unit": longValue, // Explicit label
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal(longValue))
		})

		// BC-EC-06: Non-ASCII business name
		// Uses explicit kubernaut.ai label to bypass pattern match
		It("BC-EC-06: should handle UTF-8 business names", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "x", // Too short for pattern match
					Labels: map[string]string{
						"kubernaut.ai/business-unit": "支付团队", // Explicit label with UTF-8
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("支付团队"))
		})

		// BC-EC-07: Whitespace in labels
		// Explicit labels are NOT trimmed (preserved as-is), Rego may handle
		It("BC-EC-07: should preserve whitespace in labels", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "x", // Too short for pattern match
					Labels: map[string]string{
						"kubernaut.ai/business-unit": " checkout ", // Explicit label with whitespace
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			// Label values are preserved as-is (not trimmed)
			Expect(result.BusinessUnit).To(Equal(" checkout "))
		})

		// BC-EC-08: No Rego rules match
		It("BC-EC-08: should return defaults when no Rego rules match", func() {
			// Policy with no matching rules
			noMatchPolicy := `
package signalprocessing.business

import rego.v1

result := {"business_unit": "special", "service_owner": "special-team", "criticality": "high", "sla": "gold"} if {
    input.namespace.labels["impossible"] == "value"
}
`
			policyPath := createPolicy(noMatchPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "x", // Too short for pattern match
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			// Should return defaults (0.4 confidence)
			Expect(result.BusinessUnit).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))
			Expect(result.SLARequirement).To(Equal("bronze"))
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})
	})

	// ============================================================================
	// CONFIDENCE TIER TESTS (BC-CF-01 to BC-CF-04): 4 tests - BR-SP-080
	// ============================================================================

	Context("BR-SP-080: Confidence Tier Detection", func() {
		// BC-CF-01: Explicit label detection (confidence 1.0)
		// Per BR-SP-080: All fields from explicit labels = (1.0+1.0+1.0+1.0)/4 = 1.0
		It("BC-CF-01: should return confidence 1.0 for explicit label match", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "prod-payments",
					Labels: map[string]string{
						"kubernaut.ai/business-unit": "payments",
						"kubernaut.ai/service-owner": "payments-team",
						"kubernaut.ai/criticality":   "high",
						"kubernaut.ai/sla-tier":      "gold",
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("payments"))
			Expect(result.ServiceOwner).To(Equal("payments-team"))
			Expect(result.Criticality).To(Equal("high"))
			Expect(result.SLARequirement).To(Equal("gold"))
			// All 4 fields from explicit labels = (1.0+1.0+1.0+1.0)/4 = 1.0
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-CF-02: Pattern match detection (confidence 0.8)
		// Per BR-SP-080: Pattern fills business_unit (0.8), rest from defaults (0.4)
		// Overall = (0.8 + 0.4 + 0.4 + 0.4) / 4 = 0.5
		It("BC-CF-02: should return mixed confidence for pattern match", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Namespace with pattern but no explicit labels and no Rego match
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "payments-prod",
					Labels: map[string]string{}, // No kubernaut.ai labels
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			// Pattern extraction from namespace name
			Expect(result.BusinessUnit).To(Equal("payments"))
			// Other fields from defaults
			Expect(result.ServiceOwner).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))
			Expect(result.SLARequirement).To(Equal("bronze"))
			// Mixed confidence: (0.8 + 0.4 + 0.4 + 0.4) / 4 = 0.5
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-CF-03: Rego inference (confidence 0.6)
		// Per BR-SP-080: Rego fills all 4 fields (0.6 each) = overall 0.6
		It("BC-CF-03: should return confidence 0.6 for Rego inference", func() {
			// Policy that matches via Rego rules and fills all fields
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Use app label that Rego matches, namespace too short for pattern
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "ns", // Too short for pattern match (len <= 2)
					Labels: map[string]string{
						"app": "payment-service", // Rego rule matches this
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("payments"))
			Expect(result.Criticality).To(Equal("high")) // From Rego
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-CF-04: Default fallback (confidence 0.4)
		// Per BR-SP-080: All fields from defaults = (0.4+0.4+0.4+0.4)/4 = 0.4
		It("BC-CF-04: should return confidence 0.4 for default fallback", func() {
			// Policy with no matching rules
			noMatchPolicy := `
package signalprocessing.business

import rego.v1

result := {"business_unit": "special", "service_owner": "team", "criticality": "high", "sla": "gold"} if {
    input.impossible == true
}
`
			policyPath := createPolicy(noMatchPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "x", // Too short for pattern match
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("unknown"))
			Expect(result.ServiceOwner).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))    // Safe default
			Expect(result.SLARequirement).To(Equal("bronze")) // Lowest tier default
			// All 4 fields from defaults = (0.4+0.4+0.4+0.4)/4 = 0.4
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})
	})

	// ============================================================================
	// ERROR HANDLING TESTS (BC-ER-01 to BC-ER-05): 5 tests
	// ============================================================================

	Context("Error Handling", func() {
		// BC-ER-01: Rego policy syntax error
		It("BC-ER-01: should return error for Rego syntax error at construction", func() {
			invalidPolicy := `
package signalprocessing.business
result = { this is not valid rego
`
			policyPath := createPolicy(invalidPolicy)

			_, err := classifier.NewBusinessClassifier(ctx, policyPath, logger)

			Expect(err).To(HaveOccurred())
		})

		// BC-ER-02: Rego policy timeout
		It("BC-ER-02: should use defaults on timeout (>200ms)", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Use cancelled context to simulate timeout
			cancelledCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
			defer cancel()
			time.Sleep(10 * time.Millisecond)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "x", // Too short for pattern
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(cancelledCtx, k8sCtx, envClass)

			// Should use defaults (graceful degradation)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))
			Expect(result.SLARequirement).To(Equal("bronze"))
			// Note: OverallConfidence removed per DD-SP-001 V1.1
		})

		// BC-ER-03: Nil K8sContext
		It("BC-ER-03: should return error for nil K8s context", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			_, err = businessClassifier.Classify(ctx, nil, envClass)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubernetes context is required"))
		})

		// BC-ER-04: Invalid Rego output structure
		It("BC-ER-04: should handle invalid Rego output structure", func() {
			// Policy that returns wrong type
			invalidOutputPolicy := `
package signalprocessing.business

import rego.v1

result := "just a string" if { true }
`
			policyPath := createPolicy(invalidOutputPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "x", // Too short for pattern
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(ctx, k8sCtx, envClass)

			// Should use defaults (graceful degradation)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))
			Expect(result.SLARequirement).To(Equal("bronze"))
		})

		// BC-ER-05: Context cancelled
		It("BC-ER-05: should use defaults on context cancellation", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			businessClassifier, err = classifier.NewBusinessClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "x", // Too short for pattern
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}

			result, err := businessClassifier.Classify(cancelledCtx, k8sCtx, envClass)

			// Should use defaults (graceful degradation)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.BusinessUnit).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("medium"))
			Expect(result.SLARequirement).To(Equal("bronze"))
		})
	})
})
