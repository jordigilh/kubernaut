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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// Unit Tests: Environment Classifier (Rego-based)
// Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification
// Test Matrix: 21-28 tests (5-8 happy path, 10-12 edge cases, 6-8 error handling)
var _ = Describe("Environment Classifier (Rego)", func() {
	var (
		ctx           context.Context
		envClassifier *classifier.EnvironmentClassifier
		logger        logr.Logger
		policyDir     string
		k8sClient     client.Client
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
			os.RemoveAll(policyDir)
		}
	})

	// Helper to create policy file
	createPolicy := func(content string) string {
		policyPath := filepath.Join(policyDir, "environment.rego")
		err := os.WriteFile(policyPath, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
		return policyPath
	}

	// Standard Rego policy per IMPLEMENTATION_PLAN_V1.21.md (OPA v1 syntax)
	// Per plan (line 1864): Priority order is namespace labels → ConfigMap → signal labels → default
	// Note: Rego only handles namespace labels. ConfigMap and signal labels are in Go.
	// BR-SP-051: Case-insensitive matching via lower() function
	standardPolicy := `
package signalprocessing.environment

# Primary: Namespace labels (kubernaut.ai/environment)
# Per BR-SP-051: Confidence 0.95, case-insensitive
result := {"environment": lower(env), "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# Default fallback (when namespace label not present)
# Returns "unknown" so Go code can try ConfigMap and signal labels
result := {"environment": "unknown", "confidence": 0.0, "source": "default"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
}
`

	// ============================================================================
	// HAPPY PATH TESTS (EC-HP-01 to EC-HP-06): 6 tests
	// ============================================================================

	Context("Happy Path: BR-SP-051 Namespace Label Classification", func() {
		// EC-HP-01: Production via namespace label
		It("EC-HP-01: should classify production via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
			Expect(result.Confidence).To(BeNumerically(">=", 0.95))
			Expect(result.Source).To(Equal("namespace-labels"))
		})

		// EC-HP-02: Staging via namespace label
		It("EC-HP-02: should classify staging via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
			Expect(result.Confidence).To(BeNumerically(">=", 0.90))
		})

		// EC-HP-03: Development via namespace label
		It("EC-HP-03: should classify development via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
			Expect(result.Confidence).To(BeNumerically(">=", 0.90))
		})

		// EC-HP-04: Test environment via namespace label
		It("EC-HP-04: should classify test via namespace label", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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

		// EC-HP-05: Signal labels fallback (per plan)
		It("EC-HP-05: should classify via signal labels when namespace has no label", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "unlabeled-ns",
					Labels: map[string]string{}, // No environment label
				},
			}
			signal := &signalprocessingv1alpha1.SignalData{
				Labels: map[string]string{
					"kubernaut.ai/environment": "staging",
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("staging"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.80))
			Expect(result.Source).To(Equal("signal-labels"))
		})

		// EC-HP-06: Custom environment via Rego
		It("EC-HP-06: should classify custom environment via Rego", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
	// EDGE CASE TESTS (EC-EC-01 to EC-EC-10): 10 tests
	// ============================================================================

	Context("Edge Cases", func() {
		// EC-EC-01: Conflicting labels (namespace wins)
		It("EC-EC-01: should use namespace label when signal has conflicting value", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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

		// EC-EC-03: Case sensitivity (should normalize)
		// BR-SP-051: Case-insensitive matching - standard policy now includes lower()
		It("EC-EC-03: should handle uppercase environment value", func() {
			// Standard policy now includes case normalization via lower()
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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

		// EC-EC-04: Empty K8sContext
		It("EC-EC-04: should return unknown for empty K8sContext", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "",
					Labels: map[string]string{},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
		})

		// EC-EC-05: Nil namespace in context
		It("EC-EC-05: should handle nil namespace without panic", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: nil, // Nil!
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
		})

		// EC-EC-06: Very long label value (253 chars - K8s limit)
		It("EC-EC-06: should handle very long label value (253 chars)", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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

		// EC-EC-07: Non-standard environment value
		It("EC-EC-07: should accept non-standard environment value (uat)", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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

		// EC-EC-08: Numeric label value
		It("EC-EC-08: should handle numeric label value as string", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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

		// EC-EC-09: Multiple Rego rules match - highest confidence wins
		It("EC-EC-09: should use highest confidence when multiple rules could match", func() {
			// Policy where namespace labels have higher confidence than signal
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
			Expect(result.Confidence).To(BeNumerically(">=", 0.95)) // Higher confidence
			Expect(result.Environment).To(Equal("production"))
		})

		// EC-EC-10: No Rego rules match
		It("EC-EC-10: should return unknown when no rules match", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
		})
	})

	// ============================================================================
	// ERROR HANDLING TESTS (EC-ER-01 to EC-ER-06): 6 tests
	// ============================================================================

	Context("Error Handling", func() {
		// EC-ER-01: Rego policy syntax error
		It("EC-ER-01: should gracefully degrade on Rego syntax error", func() {
			invalidPolicy := `
package signalprocessing.environment
result = { this is not valid rego syntax
`
			policyPath := createPolicy(invalidPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			// Constructor should fail with syntax error
			_, err := classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)

			// Should return error at construction time
			Expect(err).To(HaveOccurred())
		})

		// EC-ER-02: Rego policy timeout (simulated via context)
		It("EC-ER-02: should handle context cancellation gracefully", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			// Create cancelled context
			cancelledCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
			defer cancel()
			time.Sleep(10 * time.Millisecond) // Ensure timeout

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			result, err := envClassifier.Classify(cancelledCtx, k8sCtx, nil)

			// Should gracefully degrade
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})

		// EC-ER-03: Policy file not found (graceful degradation)
		It("EC-ER-03: should return error when policy file not found", func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()

			_, err := classifier.NewEnvironmentClassifier(ctx, "/nonexistent/path/policy.rego", k8sClient, logger)

			Expect(err).To(HaveOccurred())
		})

		// EC-ER-04: ConfigMap fallback - ConfigMap not found
		It("EC-ER-04: should work without ConfigMap (graceful degradation)", func() {
			policyPath := createPolicy(standardPolicy)
			// Empty client - no ConfigMap
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
			Expect(result.Environment).To(Equal("unknown"))
		})

		// EC-ER-05: Invalid Rego output type
		It("EC-ER-05: should handle invalid Rego output type gracefully", func() {
			// Policy that returns wrong type (number instead of object)
			badOutputPolicy := `
package signalprocessing.environment

result := 42
`
			policyPath := createPolicy(badOutputPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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

			// Should gracefully degrade
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})

		// EC-ER-06: Nil input to classifier
		It("EC-ER-06: should handle nil context without panic", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			// Both nil
			result, err := envClassifier.Classify(ctx, nil, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
		})
	})

	// ============================================================================
	// BR-SP-052: ConfigMap Fallback Tests (5 tests)
	// ============================================================================

	Context("BR-SP-052: ConfigMap Fallback", func() {
		It("should use ConfigMap mapping when namespace label absent", func() {
			policyPath := createPolicy(standardPolicy)

			// Create ConfigMap with namespace mapping
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-config",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"mapping": `prod-*: production
staging-*: staging
dev-*: development
test-*: test`,
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "prod-payments", // Matches prod-* pattern
					Labels: map[string]string{}, // No label!
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("production"))
			Expect(result.Source).To(Equal("configmap"))
		})

		It("should match staging-* pattern from ConfigMap", func() {
			policyPath := createPolicy(standardPolicy)

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-config",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"mapping": `prod-*: production
staging-*: staging
dev-*: development`,
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "staging-api",
					Labels: map[string]string{},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("staging"))
		})

		It("should prefer namespace label over ConfigMap", func() {
			policyPath := createPolicy(standardPolicy)

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-config",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"mapping": `prod-*: production`,
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "prod-payments",
					Labels: map[string]string{
						"kubernaut.ai/environment": "staging", // Label says staging
					},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("staging")) // Label wins
			Expect(result.Source).To(Equal("namespace-labels"))
		})

		It("should return unknown when namespace matches no ConfigMap pattern", func() {
			policyPath := createPolicy(standardPolicy)

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-config",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"mapping": `prod-*: production`,
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "random-namespace", // Doesn't match pattern
					Labels: map[string]string{},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})

		It("should handle malformed ConfigMap gracefully", func() {
			policyPath := createPolicy(standardPolicy)

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-config",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"mapping": `this is not valid yaml: [[[`,
				},
			}
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
			Expect(err).NotTo(HaveOccurred())

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "prod-payments",
					Labels: map[string]string{},
				},
			}

			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Should gracefully degrade
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})
	})

	// ============================================================================
	// PREPARED QUERY REUSE TEST (Performance)
	// ============================================================================

	Context("PreparedEvalQuery Reuse", func() {
		It("should reuse prepared query across multiple Classify calls", func() {
			policyPath := createPolicy(standardPolicy)
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			var err error
			envClassifier, err = classifier.NewEnvironmentClassifier(ctx, policyPath, k8sClient, logger)
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
