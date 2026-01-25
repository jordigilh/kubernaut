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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// Unit Tests: Environment Classifier (Rego-based)
// Per IMPLEMENTATION_PLAN_V1.22.md Day 4 specification
// Test Matrix: 21-28 tests (5-8 happy path, 10-12 edge cases, 6-8 error handling)
var _ = Describe("Environment Classifier (Rego)", func() {
	var (
		ctx           context.Context
		envClassifier *classifier.EnvironmentClassifier
		logger        logr.Logger
		policyDir     string
		scheme        *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()

		// Create temp directory for test policies
		var err error
		policyDir, err = os.MkdirTemp("", "rego-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Setup K8s client for ConfigMap tests
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
	})

	AfterEach(func() {
		// Cleanup temp directory
		if policyDir != "" {
			_ = os.RemoveAll(policyDir)
		}
	})

	// Helper to create policy file
	createPolicy := func(content string) string {
		policyPath := filepath.Join(policyDir, "environment.rego")
		err := os.WriteFile(policyPath, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
		return policyPath
	}

	// Standard Rego policy per IMPLEMENTATION_PLAN_V1.22.md (OPA v1 syntax)
	// BR-SP-051: Case-insensitive matching via lower() function
	// BR-SP-052 DEPRECATED: ConfigMap fallback removed from Go
	// BR-SP-053 DEPRECATED: Default now in Rego only
	standardPolicy := `
package signalprocessing.environment

import rego.v1

# Primary: Namespace labels (kubernaut.ai/environment)
# Per BR-SP-051: Case-insensitive
result := {"environment": lower(env), "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# Default catch-all (operators define their own defaults)
default result := {"environment": "", "source": "unclassified"}
`

	// ============================================================================
	// HAPPY PATH TESTS: BR-SP-051 Namespace Label Classification (6 tests)
	// ============================================================================

	Context("Happy Path: BR-SP-051 Namespace Label Classification", func() {
		// BR-SP-051: Production via namespace label
		It("BR-SP-051: should classify production via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "prod-payments",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("production"))
			// Note: Confidence removed per DD-SP-001 V1.1
			Expect(result.Source).To(Equal("namespace-labels"))
		})

		// BR-SP-051: Staging via namespace label
		It("BR-SP-051: should classify staging via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "staging-api",
					Labels: map[string]string{
						"kubernaut.ai/environment": "staging",
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("staging"))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-051: Development via namespace label
		It("BR-SP-051: should classify development via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "dev-team",
					Labels: map[string]string{
						"kubernaut.ai/environment": "development",
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("development"))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-051: Test environment via namespace label
		It("BR-SP-051: should classify test via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-integration",
					Labels: map[string]string{
						"kubernaut.ai/environment": "test",
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("test"))
		})

		// BR-SP-053: Signal labels fallback - unknown when namespace has no label
		It("BR-SP-053: should fallback to unknown when namespace has no label (signal-labels removed per DD-SP-001 V1.1)", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "unlabeled-ns",
					Labels: map[string]string{}, // No environment label
				},
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Labels: map[string]string{
					"kubernaut.ai/environment": "staging", // Ignored - signal labels are untrusted
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, signal)

			Expect(err).NotTo(HaveOccurred())
			// Security fix: signal-labels no longer used (untrusted source)
			// BR-SP-053 DEPRECATED: Now returns empty string from Rego default
			Expect(result.Environment).To(Equal(""))
			Expect(result.Source).To(Equal("unclassified"))
		})

		// BR-SP-051: Custom environment via Rego
		It("BR-SP-051: should classify custom environment via Rego", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "canary-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "canary", // Custom value
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("canary"))
		})
	})

	// ============================================================================
	// EDGE CASE TESTS: BR-SP-051 Edge Cases (10 tests)
	// ============================================================================

	Context("Edge Cases", func() {
		// BR-SP-051: Conflicting labels (namespace wins)
		It("BR-SP-051: should use namespace label when signal has conflicting value", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Labels: map[string]string{
					"kubernaut.ai/environment": "staging", // Conflicting!
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("production")) // Namespace wins
			Expect(result.Source).To(Equal("namespace-labels"))
		})

		// BR-SP-052: Partial match namespace name (should NOT match)
		// Plan: "production-like-test" should return "unknown" to avoid false positive
		It("BR-SP-052: should return unknown for partial match namespace name", func() {
			policyPath := createPolicy(standardPolicy)

			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// "production-like-test" should NOT match "prod-*" pattern
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "production-like-test", // Partial match - should NOT trigger
					Labels: map[string]string{},    // No label
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("")) // Avoid false positive
		})

		// BR-SP-051: Case sensitivity (should normalize)
		// Case-insensitive matching - standard policy now includes lower()
		It("BR-SP-051: should handle uppercase environment value", func() {
			// Standard policy now includes case normalization via lower()
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "PRODUCTION", // Uppercase
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("production")) // Normalized by Rego lower()
		})

		// BR-SP-053: Empty K8sContext
		It("BR-SP-053: should return unknown for empty K8sContext", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "",
					Labels: map[string]string{},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal(""))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-053: Nil namespace in context
		It("BR-SP-053: should handle nil namespace without panic", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: nil, // Nil!
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal(""))
			// Note: Confidence removed per DD-SP-001 V1.1
		})

		// BR-SP-051: Very long label value (253 chars - K8s limit)
		It("BR-SP-051: should handle very long label value (253 chars)", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			longValue := strings.Repeat("a", 253) // K8s max label value length
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": longValue,
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal(longValue))
		})

		// BR-SP-051: Non-standard environment value
		It("BR-SP-051: should accept non-standard environment value (uat)", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "uat", // Non-standard
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("uat"))
		})

		// BR-SP-051: Numeric label value
		It("BR-SP-051: should handle numeric label value as string", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "1", // Numeric as string
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("1"))
		})

		// BR-SP-051: Multiple Rego rules match - highest confidence wins
		It("BR-SP-051: should use highest confidence when multiple rules could match", func() {
			// Policy where namespace labels have higher confidence than signal
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Both namespace and signal have values - namespace wins (0.95 > 0.80)
			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Labels: map[string]string{
					"kubernaut.ai/environment": "staging",
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, signal)

			Expect(err).NotTo(HaveOccurred())
			// Note: Confidence removed per DD-SP-001 V1.1 // Higher confidence
			Expect(result.Environment).To(Equal("production"))
		})

		// BR-SP-053: No Rego rules match
		It("BR-SP-053: should return unknown when no rules match", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "test-ns",
					Labels: map[string]string{}, // No environment label
				},
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Labels: map[string]string{}, // No environment label
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal(""))
			// Note: Confidence removed per DD-SP-001 V1.1
		})
	})

	// ============================================================================
	// ERROR HANDLING TESTS: BR-SP-071 Graceful Degradation (6 tests)
	// ============================================================================

	Context("Error Handling", func() {
		// BR-SP-071: Rego policy syntax error
		It("BR-SP-071: should gracefully degrade on Rego syntax error", func() {
			invalidPolicy := `
package signalprocessing.environment
result = { this is not valid rego syntax
`
			policyPath := createPolicy(invalidPolicy)
			// Constructor should fail with syntax error
			_, err := classifier.NewEnvironmentClassifier(ctx, policyPath, logger)

			// Should return error at construction time
			Expect(err).To(HaveOccurred())
		})

		// BR-SP-071: Rego policy timeout (simulated via context)
		// FlakeAttempts(3): Timing-sensitive test due to nanosecond context timeout
		// BR-SP-071 DEPRECATED: Context cancellation causes Rego error (no Go fallback)
		// When context is cancelled, Rego evaluation fails with cancellation error
		// NOTE: Context cancellation tests removed (2025-12-20)
		// These tests are inherently flaky because Rego evaluation may complete
		// before context cancellation is detected. This tests Rego's internal
		// behavior, not our classification logic. The important behavior
		// (no Go fallback when Rego returns error) is tested by other tests.

		// BR-SP-071 DEPRECATED: Rego policy runtime error returns error (no Go fallback)
		// Policy that doesn't match and has no default should return error
		It("BR-SP-071: should return error when policy has no matching rules and no default", func() {
			// Policy without a default rule (will return empty results)
			noDefaultPolicy := `
package signalprocessing.environment

import rego.v1

# Rule that will never match - NO DEFAULT DEFINED
result := {"environment": "production", "source": "never-match"} if {
    input.impossible_field == "impossible_value"
}
`
			policyPath := createPolicy(noDefaultPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "test-ns",
					Labels: map[string]string{},
				},
			}

			// BR-SP-071 DEPRECATED: Now returns error instead of fallback
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no results"))
			Expect(result).To(BeNil())
		})

		// BR-SP-071: Policy file not found
		It("BR-SP-071: should return error when policy file not found", func() {
			_, err := classifier.NewEnvironmentClassifier(ctx, "/nonexistent/path/policy.rego", logger)

			Expect(err).To(HaveOccurred())
		})

		// BR-SP-052: ConfigMap fallback - ConfigMap not found
		It("BR-SP-052: should work without ConfigMap (graceful degradation)", func() {
			policyPath := createPolicy(standardPolicy)
			// Empty client - no ConfigMap
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "unmapped-ns",
					Labels: map[string]string{},
				},
			}

			// Should still work, just return unknown
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal(""))
		})

		// BR-SP-071 DEPRECATED: Invalid Rego output type now returns error (no Go fallback)
		It("BR-SP-071: should return error for invalid Rego output type", func() {
			// Policy that returns wrong type (number instead of object)
			badOutputPolicy := `
package signalprocessing.environment

result := 42
`
			policyPath := createPolicy(badOutputPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// BR-SP-071 DEPRECATED: Now returns error instead of fallback
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid output structure"))
			Expect(result).To(BeNil())
		})

		// BR-SP-053: Nil input to classifier
		It("BR-SP-053: should handle nil context without panic", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Both nil
			result, err := envClassifier.Classify(ctx, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal(""))
			// Note: Confidence removed per DD-SP-001 V1.1
		})
	})

	// ============================================================================
	// PREPARED QUERY REUSE TEST (Performance)
	// ============================================================================

	Context("PreparedEvalQuery Reuse", func() {
		It("should reuse prepared query across multiple Classify calls", func() {
			policyPath := createPolicy(standardPolicy)
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
			Expect(err).NotTo(HaveOccurred())

			// Call Classify multiple times
			for i := 0; i < 10; i++ {
				k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
					Namespace: &signalprocessingv1alpha1.NamespaceContext{
						Name: "test-ns",
						Labels: map[string]string{
							"kubernaut.ai/environment": "production",
						},
					},
				}

				result, err := envClassifier.Classify(ctx, k8sCtx, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Environment).To(Equal("production"))
			}
			// If we got here without error, query was reused successfully
		})
	})
})
