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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// Unit Tests: Environment Classifier (Rego-based)
// Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification
var _ = Describe("Environment Classifier (Rego)", func() {
	var (
		ctx           context.Context
		envClassifier *classifier.EnvironmentClassifier
		logger        logr.Logger
		policyDir     string
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()

		// Create temp directory for test policies
		var err error
		policyDir, err = os.MkdirTemp("", "rego-test-*")
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
		policyPath := filepath.Join(policyDir, "environment.rego")
		err := os.WriteFile(policyPath, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
		return policyPath
	}

	// Standard Rego policy per plan specification (OPA v1 syntax with 'if' keyword)
	standardPolicy := `
package signalprocessing.environment

# Primary: Namespace labels (kubernaut.ai/environment)
# Confidence: 0.95
result := {"environment": env, "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# Default fallback
# Confidence: 0.0
result := {"environment": "unknown", "confidence": 0.0, "source": "default"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
}
`

	// ============================================================================
	// BR-SP-051: Environment from namespace labels (Primary)
	// ============================================================================

	Context("BR-SP-051: Namespace Label Classification via Rego", func() {
		It("should classify environment from kubernaut.ai/environment label", func() {
			// Arrange
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-namespace",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Environment).To(Equal("production"))
			Expect(result.Confidence).To(Equal(0.95))
			Expect(result.Source).To(Equal("namespace-labels"))
		})

		It("should classify staging environment", func() {
			// Arrange
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "staging-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "staging",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("staging"))
			Expect(result.Confidence).To(Equal(0.95))
		})

		It("should classify development environment", func() {
			// Arrange
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "dev-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "development",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("development"))
		})

		It("should classify test environment", func() {
			// Arrange
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "test",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("test"))
		})
	})

	// ============================================================================
	// BR-SP-053: Default Environment (Last Resort)
	// ============================================================================

	Context("BR-SP-053: Default Environment via Rego", func() {
		It("should return unknown when namespace has no environment label", func() {
			// Arrange
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "unlabeled-ns",
					Labels: map[string]string{},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
			Expect(result.Source).To(Equal("default"))
		})

		It("should return unknown when namespace is nil", func() {
			// Arrange
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: nil,
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
		})

		It("should return unknown for empty kubernaut.ai/environment label value", func() {
			// Arrange
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "", // Empty value
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})

		It("should never fail - always return a valid result", func() {
			// Arrange: Worst case - nil context
			policyPath := createPolicy(standardPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			// Act
			result, err := envClassifier.Classify(ctx, nil, nil)

			// Assert: Should not fail, return default
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Environment).To(Equal("unknown"))
		})
	})

	// ============================================================================
	// Rego Policy Error Handling (Graceful Degradation)
	// ============================================================================

	Context("Rego Policy Error Handling", func() {
		It("should gracefully degrade when policy file not found", func() {
			// Arrange: Non-existent policy path
			envClassifier = classifier.NewEnvironmentClassifier("/nonexistent/path/environment.rego", logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert: Should not error, return default with degraded flag
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Source).To(Equal("default"))
		})

		It("should gracefully degrade when policy has syntax error", func() {
			// Arrange: Invalid Rego syntax
			invalidPolicy := `
package signalprocessing.environment
result = { this is not valid rego
`
			policyPath := createPolicy(invalidPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert: Should not error, return default
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})
	})

	// ============================================================================
	// Custom Rego Policies
	// ============================================================================

	Context("Custom Rego Policies", func() {
		It("should support custom environment values via Rego", func() {
			// Arrange: Custom policy that accepts "canary" environment (OPA v1 syntax)
			customPolicy := `
package signalprocessing.environment

result := {"environment": env, "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

result := {"environment": "unknown", "confidence": 0.0, "source": "default"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
}
`
			policyPath := createPolicy(customPolicy)
			envClassifier = classifier.NewEnvironmentClassifier(policyPath, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "canary-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "canary",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx, nil)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("canary"))
		})
	})
})

