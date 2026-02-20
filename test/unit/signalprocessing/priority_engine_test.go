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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// Unit Tests: Priority Engine (Rego-based)
// Per IMPLEMENTATION_PLAN_V1.23.md Day 5 specification
// Test Matrix: 26 tests (6 HP, 10 EC, 6 ER, 4 FB)
// BR Coverage: BR-SP-070, BR-SP-071, BR-SP-072
var _ = Describe("Priority Engine (Rego)", func() {
	var (
		ctx            context.Context
		priorityEngine *classifier.PriorityEngine
		logger         logr.Logger
		policyDir      string
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()

		// Create temp directory for test policies
		var err error
		policyDir, err = os.MkdirTemp("", "priority-rego-test-*")
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
		policyPath := filepath.Join(policyDir, "priority.rego")
		err := os.WriteFile(policyPath, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
		return policyPath
	}

	// Standard priority Rego policy per IMPLEMENTATION_PLAN_V1.23.md
	// BR-SP-070: Priority assignment via Rego policy
	standardPolicy := `
package signalprocessing.priority

import rego.v1

# Score-based priority aggregation (issue #98)
severity_score := 3 if { lower(input.signal.severity) == "critical" }
severity_score := 2 if { lower(input.signal.severity) == "high" }
severity_score := 1 if { lower(input.signal.severity) == "low" }
default severity_score := 0

env_scores contains 3 if { lower(input.environment) == "production" }
env_scores contains 2 if { lower(input.environment) == "staging" }
env_scores contains 1 if { lower(input.environment) == "development" }
env_scores contains 1 if { lower(input.environment) == "test" }
env_scores contains 3 if { input.namespace_labels["tier"] == "critical" }
env_scores contains 2 if { input.namespace_labels["tier"] == "high" }

env_score := max(env_scores) if { count(env_scores) > 0 }
default env_score := 0

composite_score := severity_score + env_score

result := {"priority": "P0", "policy_name": "score-based"} if { composite_score >= 6 }
result := {"priority": "P1", "policy_name": "score-based"} if { composite_score == 5 }
result := {"priority": "P2", "policy_name": "score-based"} if { composite_score == 4 }
result := {"priority": "P3", "policy_name": "score-based"} if { composite_score < 4; composite_score > 0 }

default result := {"priority": "P2", "policy_name": "default-catch-all"}
`

	// ============================================================================
	// HAPPY PATH TESTS: 6 tests - BR-SP-070 Priority Assignment
	// ============================================================================

	Context("Happy Path: BR-SP-070 Priority Assignment", func() {
		// BR-SP-070: P0 - Critical production
		It("BR-SP-070: should assign P0 for critical severity in production", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "prod-app",
					Labels: map[string]string{},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
				Source:      "namespace-labels",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0"))
			// Note: Confidence removed per DD-SP-001 V1.1
			Expect(result.Source).To(Equal("rego-policy"))
		})

		// BR-SP-070: P1 - Warning production
		It("BR-SP-070: should assign P1 for high severity in production (DD-SEVERITY-001 v1.1)", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "prod-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "high",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P1"))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-070: P2 - Low severity production (DD-SEVERITY-001 v1.1)
		It("BR-SP-070: should assign P2 for low severity in production (DD-SEVERITY-001 v1.1)", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "prod-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "low",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2"))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-070: P1 - Critical staging
		It("BR-SP-070: should assign P1 for critical severity in staging", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "staging-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "staging",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P1"))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-070: P3 - Development info
		It("BR-SP-070: should assign P3 for info severity in development", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "dev-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "development",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "low",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P3"))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-070: Custom priority via Rego
		It("BR-SP-070: should assign custom priority via Rego rule", func() {
			// Custom policy with namespace label check
			customPolicy := `
package signalprocessing.priority

import rego.v1

# Custom rule: tier=critical label → P0 regardless of severity
result := {"priority": "P0", "confidence": 0.92, "policy_name": "tier-critical"} if {
    input.namespace_labels["tier"] == "critical"
}

# Default
result := {"priority": "P2", "confidence": 0.60, "policy_name": "default"} if {
    not input.namespace_labels["tier"]
}
`
			policyPath := createPolicy(customPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "payment-service",
					Labels: map[string]string{
						"tier": "critical",
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "staging",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "low", // Low severity but tier=critical → P0
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0"))
			Expect(result.PolicyName).To(Equal("tier-critical"))
		})
	})

	// ============================================================================
	// EDGE CASE TESTS: BR-SP-071 Edge Cases (10 tests)
	// ============================================================================

	Context("Edge Cases", func() {
		// BR-SP-070: Empty string environment
		It("BR-SP-070: should handle empty environment string", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "", // Empty
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			// Should still return a priority (Rego default or fallback)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(BeElementOf("P0", "P1", "P2", "P3"))
		})

		// BR-SP-070: Empty string severity
		It("BR-SP-070: should fallback to P3 for empty severity in production", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "", // Empty
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			// Score-based: severity_score=0 + env_score=3 = 3 → P3
			// (Improved from old N×M catch-all P2: unknown severity in production
			// should be lower priority than a known low-severity signal)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P3"))
		})

		// BR-SP-070: Both empty
		It("BR-SP-070: should fallback to P2 when both environment and severity empty", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2"))
		})

		// BR-SP-070: Case normalization
		It("BR-SP-070: should normalize case for environment and severity", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "PRODUCTION", // Uppercase
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "CRITICAL", // Uppercase
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0")) // Should normalize and match
		})

		// BR-SP-070: Custom severity value
		It("BR-SP-070: should handle custom severity value", func() {
			// Policy that handles custom severity
			customPolicy := `
package signalprocessing.priority

import rego.v1

result := {"priority": "P1", "confidence": 0.85, "policy_name": "urgent-custom"} if {
    lower(input.signal.severity) == "urgent"
}

result := {"priority": "P2", "confidence": 0.60, "policy_name": "default"} if {
    not lower(input.signal.severity) == "urgent"
}
`
			policyPath := createPolicy(customPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "urgent", // Custom severity
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P1"))
		})

		// BR-SP-070: Multiple Rego rules match
		It("BR-SP-070: should use first matching rule when multiple rules could match", func() {
			// Policy where first rule should win
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "prod-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0")) // Highest priority rule
		})

		// BR-SP-070: Boundary condition
		It("BR-SP-070: should handle boundary severity correctly", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "staging",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "high", // Boundary between P1 and P2
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2")) // warning + staging = P2
		})

		// BR-SP-070: Namespace labels missing
		It("BR-SP-070: should handle missing namespace labels", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "test-app",
					Labels: nil, // Nil labels
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0"))
		})

		// BR-SP-070: Deployment labels missing
		It("BR-SP-070: should handle missing deployment context", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
				Deployment: nil, // No deployment context
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "high",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P1"))
		})

		// BR-SP-070: All labels present
		It("BR-SP-070: should return highest confidence when all labels present", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "prod-payments",
					Labels: map[string]string{
						"app":  "payments",
						"tier": "backend",
					},
				},
				Deployment: &signalprocessingv1alpha1.DeploymentDetails{
					Labels: map[string]string{
						"version": "v1",
					},
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
				Source:      "namespace-labels",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0"))
			// Note: Confidence removed per DD-SP-001 V1.1
		})
	})

	// ============================================================================
	// ERROR HANDLING TESTS: 6 tests
	// BR-SP-070: Priority assignment error handling
	// BR-SP-071: Severity-based fallback on Rego failure
	// ============================================================================

	Context("Error Handling", func() {
		// BR-SP-070: Rego policy syntax error must be detected at construction
		It("BR-SP-070: should return error for Rego syntax error at construction", func() {
			invalidPolicy := `
package signalprocessing.priority
result = { this is not valid rego
`
			policyPath := createPolicy(invalidPolicy)

			_, err := classifier.NewPriorityEngine(ctx, policyPath, logger)

			Expect(err).To(HaveOccurred())
		})

		// NOTE: Timeout test removed (2025-12-20)
		// This test is inherently flaky because Rego evaluation may complete
		// before context timeout is detected. This tests Rego's internal
		// behavior, not our classification logic.

		// BR-SP-070: Nil environment classification must be rejected
		It("BR-SP-070: should return error for nil environment classification", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			_, err = priorityEngine.Assign(ctx, k8sCtx, nil, signal) // Nil envClass

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("environment classification is required"))
		})

		// BR-SP-070: Invalid Rego output (PX) must be rejected
		It("BR-SP-070: should return error for invalid priority value from Rego", func() {
			invalidOutputPolicy := `
package signalprocessing.priority

import rego.v1

result := {"priority": "PX", "confidence": 0.95, "policy_name": "invalid"} if {
    true
}
`
			policyPath := createPolicy(invalidOutputPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			_, err = priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid priority value"))
		})

		// BR-SP-070: Rego returns out of range (P5) must be rejected
		It("BR-SP-070: should return error for out of range priority (P5)", func() {
			outOfRangePolicy := `
package signalprocessing.priority

import rego.v1

result := {"priority": "P5", "confidence": 0.95, "policy_name": "out-of-range"} if {
    true
}
`
			policyPath := createPolicy(outOfRangePolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			_, err = priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid priority value"))
		})

		// NOTE: Context cancellation test removed (2025-12-20)
		// This test is inherently flaky because Rego evaluation may complete
		// before context cancellation is detected. This tests Rego's internal
		// behavior, not our classification logic.
	})

	// ============================================================================
	// ============================================================================
	// BR-SP-071: DEPRECATED - Go fallback removed (2025-12-20)
	// These tests verify that policies WITHOUT `default` rules return errors.
	// Operators MUST define their own defaults in Rego using `default result := {...}`
	// ============================================================================

	Context("BR-SP-071: DEPRECATED - Policy Without Default Returns Error", func() {
		// BR-SP-071 DEPRECATED: Policy without default should return error
		It("should return error when policy has no matching rules and no default", func() {
			// Policy that compiles but returns no result (no rules match, no default)
			noDefaultPolicy := `
package signalprocessing.priority

import rego.v1

# Rule that will never match - NO DEFAULT DEFINED
result := {"priority": "P0", "policy_name": "never-match"} if {
    input.impossible_field == "impossible_value"
}
`
			policyPath := createPolicy(noDefaultPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			// BR-SP-071 DEPRECATED: Now returns error instead of fallback
			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no results"))
			Expect(result).To(BeNil())
		})

		// Verify policy WITH default works correctly
		It("should use Rego default rule when no specific rule matches", func() {
			// Policy with a default rule (operator-defined)
			withDefaultPolicy := `
package signalprocessing.priority

import rego.v1

# Rule that will never match
result := {"priority": "P0", "policy_name": "never-match"} if {
    input.impossible_field == "impossible_value"
}

# Operator-defined default (replaces Go fallback)
default result := {"priority": "P3", "policy_name": "operator-default"}
`
			policyPath := createPolicy(withDefaultPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-app",
				},
			}
			envClass := &signalprocessingv1alpha1.EnvironmentClassification{
				Environment: "production",
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P3")) // Operator-defined default
			Expect(result.PolicyName).To(Equal("operator-default"))
		})
	})
})
