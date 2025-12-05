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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// Unit Tests: Environment Classifier
// Per IMPLEMENTATION_PLAN_V1.21.md Day 4 specification
// Ported from Gateway service to centralize classification
var _ = Describe("Environment Classifier", func() {
	var (
		ctx        context.Context
		envClassifier *classifier.EnvironmentClassifier
		logger     logr.Logger
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
	})

	// ============================================================================
	// BR-SP-051: Environment from namespace labels (Primary)
	// ============================================================================

	Context("BR-SP-051: Namespace Label Classification", func() {
		It("should classify environment from kubernaut.ai/environment label", func() {
			// Arrange: Create namespace with kubernaut.ai/environment label
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-namespace",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Environment).To(Equal("production"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.95))
			Expect(result.Source).To(Equal("namespace-label"))
		})

		It("should classify staging environment", func() {
			// Arrange
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "staging-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "staging",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("staging"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.95))
		})

		It("should classify development environment", func() {
			// Arrange
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "dev-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "development",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("development"))
		})

		It("should classify test environment", func() {
			// Arrange
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "test",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("test"))
		})

		It("should handle case-insensitive environment values", func() {
			// Arrange: Label with uppercase value
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "PRODUCTION",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert: Should normalize to lowercase
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("production"))
		})

		It("should NOT use non-kubernaut.ai labels", func() {
			// Arrange: Namespace has 'environment' label but NOT kubernaut.ai prefixed
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"environment": "production",  // NOT kubernaut.ai/ prefixed - should be ignored
						"env":         "production",  // NOT kubernaut.ai/ prefixed - should be ignored
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert: Should return unknown (ignores non-kubernaut.ai labels)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Source).To(Equal("default"))
		})
	})

	// ============================================================================
	// BR-SP-052: ConfigMap environment override (Fallback)
	// ============================================================================

	Context("BR-SP-052: ConfigMap Fallback", func() {
		It("should fall back to ConfigMap when namespace label is absent", func() {
			// Arrange: Namespace without label, ConfigMap with mapping
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unlabeled-ns",
				},
			}

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-overrides",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"unlabeled-ns": "staging",
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, cm).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "unlabeled-ns",
					Labels: map[string]string{}, // No kubernaut.ai/environment
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("staging"))
			Expect(result.Confidence).To(BeNumerically(">=", 0.70))
			Expect(result.Confidence).To(BeNumerically("<", 0.95))
			Expect(result.Source).To(Equal("configmap"))
		})

		It("should prefer namespace label over ConfigMap", func() {
			// Arrange: Namespace with label AND ConfigMap with different value
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "labeled-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-overrides",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"labeled-ns": "staging", // Different from label
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, cm).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "labeled-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert: Namespace label takes precedence
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("production"))
			Expect(result.Source).To(Equal("namespace-label"))
		})

		It("should work when ConfigMap does not exist", func() {
			// Arrange: No ConfigMap, namespace without label
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "test-ns",
					Labels: map[string]string{},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert: Should fall through to default
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})
	})

	// ============================================================================
	// BR-SP-053: Default Environment (Last Resort)
	// ============================================================================

	Context("BR-SP-053: Default Environment", func() {
		It("should return unknown when namespace has no environment label", func() {
			// Arrange: Namespace without kubernaut.ai/environment label
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name:   "unlabeled-ns",
					Labels: map[string]string{},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
			Expect(result.Source).To(Equal("default"))
		})

		It("should return unknown when namespace is nil", func() {
			// Arrange
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: nil, // No namespace info
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
			Expect(result.Confidence).To(Equal(0.0))
		})

		It("should return unknown for empty kubernaut.ai/environment label value", func() {
			// Arrange: Empty environment value
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "", // Empty value
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("unknown"))
		})

		It("should never fail - always return a valid result", func() {
			// Arrange: Worst case - nil context
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			// Act
			result, err := envClassifier.Classify(ctx, nil)

			// Assert: Should not fail, return default
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Environment).To(Equal("unknown"))
		})
	})

	// ============================================================================
	// Caching and Performance
	// ============================================================================

	Context("Caching and Performance", func() {
		It("should cache namespace lookups to reduce K8s API calls", func() {
			// Arrange
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cached-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "cached-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "production",
					},
				},
			}

			// Act: Call multiple times
			result1, _ := envClassifier.Classify(ctx, k8sCtx)
			result2, _ := envClassifier.Classify(ctx, k8sCtx)
			result3, _ := envClassifier.Classify(ctx, k8sCtx)

			// Assert: All should return same result (cache working)
			Expect(result1.Environment).To(Equal("production"))
			Expect(result2.Environment).To(Equal("production"))
			Expect(result3.Environment).To(Equal("production"))
		})

		It("should allow custom cache TTL", func() {
			// Arrange
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			customTTL := 5 * time.Second
			envClassifier = classifier.NewEnvironmentClassifierWithTTL(k8sClient, logger, customTTL)

			// Assert: Should create without error
			Expect(envClassifier).NotTo(BeNil())
		})
	})

	// ============================================================================
	// Edge Cases
	// ============================================================================

	Context("Edge Cases", func() {
		It("should handle custom environment values", func() {
			// Arrange: Custom environment value (organization-specific)
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			k8sCtx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Name: "test-ns",
					Labels: map[string]string{
						"kubernaut.ai/environment": "canary",
					},
				},
			}

			// Act
			result, err := envClassifier.Classify(ctx, k8sCtx)

			// Assert: Should accept any non-empty value
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Environment).To(Equal("canary"))
		})

		It("should clear cache", func() {
			// Arrange
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			envClassifier = classifier.NewEnvironmentClassifier(k8sClient, logger)

			// Act
			envClassifier.ClearCache()

			// Assert: Should not panic
			Expect(envClassifier.GetCacheSize()).To(Equal(0))
		})
	})
})

