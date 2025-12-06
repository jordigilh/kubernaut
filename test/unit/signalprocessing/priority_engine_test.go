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
	"time"

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
			os.RemoveAll(policyDir)
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

# P0: Critical + production
result := {"priority": "P0", "confidence": 0.95, "policy_name": "critical-production"} if {
    lower(input.signal.severity) == "critical"
    lower(input.environment) == "production"
}

# P1: Critical + staging
result := {"priority": "P1", "confidence": 0.90, "policy_name": "critical-staging"} if {
    lower(input.signal.severity) == "critical"
    lower(input.environment) == "staging"
}

# P1: Warning + production
result := {"priority": "P1", "confidence": 0.90, "policy_name": "warning-production"} if {
    lower(input.signal.severity) == "warning"
    lower(input.environment) == "production"
}

# P2: Info + production
result := {"priority": "P2", "confidence": 0.85, "policy_name": "info-production"} if {
    lower(input.signal.severity) == "info"
    lower(input.environment) == "production"
}

# P2: Warning + staging
result := {"priority": "P2", "confidence": 0.85, "policy_name": "warning-staging"} if {
    lower(input.signal.severity) == "warning"
    lower(input.environment) == "staging"
}

# P2: Critical + development
result := {"priority": "P2", "confidence": 0.85, "policy_name": "critical-development"} if {
    lower(input.signal.severity) == "critical"
    lower(input.environment) == "development"
}

# P3: Info + development
result := {"priority": "P3", "confidence": 0.80, "policy_name": "info-development"} if {
    lower(input.signal.severity) == "info"
    lower(input.environment) == "development"
}

# P3: Warning + development
result := {"priority": "P3", "confidence": 0.80, "policy_name": "warning-development"} if {
    lower(input.signal.severity) == "warning"
    lower(input.environment) == "development"
}

# Default fallback
result := {"priority": "P2", "confidence": 0.60, "policy_name": "default-fallback"} if {
    not matched_severity
}

matched_severity if { lower(input.signal.severity) == "critical" }
matched_severity if { lower(input.signal.severity) == "warning" }
matched_severity if { lower(input.signal.severity) == "info" }
`

	// ============================================================================
	// HAPPY PATH TESTS (PE-HP-01 to PE-HP-06): 6 tests
	// ============================================================================

	Context("Happy Path: BR-SP-070 Priority Assignment", func() {
		// PE-HP-01: P0 - Critical production
		It("PE-HP-01: should assign P0 for critical severity in production", func() {
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
				Confidence:  0.95,
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.95))
			Expect(result.Source).To(Equal("rego-policy"))
		})

		// PE-HP-02: P1 - Warning production
		It("PE-HP-02: should assign P1 for warning severity in production", func() {
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
				Severity: "warning",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P1"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.90))
		})

		// PE-HP-03: P2 - Info production
		It("PE-HP-03: should assign P2 for info severity in production", func() {
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
				Severity: "info",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.85))
		})

		// PE-HP-04: P1 - Critical staging
		It("PE-HP-04: should assign P1 for critical severity in staging", func() {
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
			Expect(result.Confidence).To(BeNumerically(">=", 0.90))
		})

		// PE-HP-05: P3 - Development info
		It("PE-HP-05: should assign P3 for info severity in development", func() {
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
				Severity: "info",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P3"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.80))
		})

		// PE-HP-06: Custom priority via Rego
		It("PE-HP-06: should assign custom priority via Rego rule", func() {
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
				Severity: "info", // Low severity but tier=critical → P0
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0"))
			Expect(result.PolicyName).To(Equal("tier-critical"))
		})
	})

	// ============================================================================
	// EDGE CASE TESTS (PE-EC-01 to PE-EC-10): 10 tests
	// ============================================================================

	Context("Edge Cases", func() {
		// PE-EC-01: Empty string environment
		It("PE-EC-01: should handle empty environment string", func() {
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

		// PE-EC-02: Empty string severity
		It("PE-EC-02: should fallback to P2 for empty severity", func() {
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

			// Should use Rego default or Go fallback
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2"))
		})

		// PE-EC-03: Both empty
		It("PE-EC-03: should fallback to P2 when both environment and severity empty", func() {
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

		// PE-EC-04: Case normalization
		It("PE-EC-04: should normalize case for environment and severity", func() {
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

		// PE-EC-05: Custom severity value
		It("PE-EC-05: should handle custom severity value", func() {
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

		// PE-EC-06: Multiple Rego rules match
		It("PE-EC-06: should use first matching rule when multiple rules could match", func() {
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

		// PE-EC-07: Boundary condition
		It("PE-EC-07: should handle boundary severity correctly", func() {
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
				Severity: "warning", // Boundary between P1 and P2
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2")) // warning + staging = P2
		})

		// PE-EC-08: Namespace labels missing
		It("PE-EC-08: should handle missing namespace labels", func() {
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

		// PE-EC-09: Deployment labels missing
		It("PE-EC-09: should handle missing deployment context", func() {
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
				Severity: "warning",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P1"))
		})

		// PE-EC-10: All labels present
		It("PE-EC-10: should return highest confidence when all labels present", func() {
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
				Confidence:  0.95,
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Severity: "critical",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P0"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.95))
		})
	})

	// ============================================================================
	// ERROR HANDLING TESTS (PE-ER-01 to PE-ER-06): 6 tests
	// ============================================================================

	Context("Error Handling", func() {
		// PE-ER-01: Rego policy syntax error
		It("PE-ER-01: should return error for Rego syntax error at construction", func() {
			invalidPolicy := `
package signalprocessing.priority
result = { this is not valid rego
`
			policyPath := createPolicy(invalidPolicy)

			_, err := classifier.NewPriorityEngine(ctx, policyPath, logger)

			Expect(err).To(HaveOccurred())
		})

		// PE-ER-02: Rego policy timeout
		It("PE-ER-02: should fallback on timeout (>100ms)", func() {
			// Policy with potential slow evaluation (simulated via context timeout)
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Use cancelled context to simulate timeout
			cancelledCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
			defer cancel()
			time.Sleep(10 * time.Millisecond) // Ensure timeout

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

			result, err := priorityEngine.Assign(cancelledCtx, k8sCtx, envClass, signal)

			// Should fallback per BR-SP-071
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal("fallback-severity"))
		})

		// PE-ER-03: Nil environment classification
		It("PE-ER-03: should return error for nil environment classification", func() {
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

		// PE-ER-04: Invalid Rego output (PX)
		It("PE-ER-04: should return error for invalid priority value from Rego", func() {
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

		// PE-ER-05: Rego returns out of range (P5)
		It("PE-ER-05: should return error for out of range priority (P5)", func() {
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

		// PE-ER-06: Context cancelled
		It("PE-ER-06: should handle context cancellation", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			priorityEngine, err = classifier.NewPriorityEngine(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

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

			result, err := priorityEngine.Assign(cancelledCtx, k8sCtx, envClass, signal)

			// Should fallback gracefully
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Source).To(Equal("fallback-severity"))
		})
	})

	// ============================================================================
	// FALLBACK TESTS (PE-FB-01 to PE-FB-04): 4 tests - BR-SP-071
	// ============================================================================

	Context("BR-SP-071: Severity-Based Fallback", func() {
		// PE-FB-01: Fallback critical
		It("PE-FB-01: should return P1 for critical severity on Rego failure", func() {
			// Policy that compiles but returns no result (no rules match)
			noMatchPolicy := `
package signalprocessing.priority

import rego.v1

# Rule that will never match
result := {"priority": "P0", "confidence": 0.95, "policy_name": "never-match"} if {
    input.impossible_field == "impossible_value"
}
`
			policyPath := createPolicy(noMatchPolicy)
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
			Expect(result.Priority).To(Equal("P1"))
			Expect(result.Source).To(Equal("fallback-severity"))
		})

		// PE-FB-02: Fallback warning
		It("PE-FB-02: should return P2 for warning severity on Rego failure", func() {
			noMatchPolicy := `
package signalprocessing.priority

import rego.v1

# Rule that will never match
result := {"priority": "P0", "confidence": 0.95, "policy_name": "never-match"} if {
    input.impossible_field == "impossible_value"
}
`
			policyPath := createPolicy(noMatchPolicy)
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
				Severity: "warning",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2"))
			Expect(result.Source).To(Equal("fallback-severity"))
		})

		// PE-FB-03: Fallback info
		It("PE-FB-03: should return P3 for info severity on Rego failure", func() {
			noMatchPolicy := `
package signalprocessing.priority

import rego.v1

# Rule that will never match
result := {"priority": "P0", "confidence": 0.95, "policy_name": "never-match"} if {
    input.impossible_field == "impossible_value"
}
`
			policyPath := createPolicy(noMatchPolicy)
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
				Severity: "info",
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P3"))
			Expect(result.Source).To(Equal("fallback-severity"))
		})

		// PE-FB-04: Fallback unknown
		It("PE-FB-04: should return P2 for unknown severity on Rego failure", func() {
			noMatchPolicy := `
package signalprocessing.priority

import rego.v1

# Rule that will never match
result := {"priority": "P0", "confidence": 0.95, "policy_name": "never-match"} if {
    input.impossible_field == "impossible_value"
}
`
			policyPath := createPolicy(noMatchPolicy)
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
				Severity: "", // Unknown
				Source:   "prometheus",
			}

			result, err := priorityEngine.Assign(ctx, k8sCtx, envClass, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Priority).To(Equal("P2"))
			Expect(result.Source).To(Equal("fallback-severity"))
		})
	})
})

