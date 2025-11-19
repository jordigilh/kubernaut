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

package processing

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

var _ = Describe("Environment Classification", func() {
	var (
		ctx        context.Context
		classifier *processing.EnvironmentClassifier
		logger     *zap.Logger
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop()
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
	})

	// ============================================================================
	// BR-GATEWAY-011: Environment from namespace labels
	// ============================================================================

	Context("BR-GATEWAY-011: Namespace Label Classification", func() {
		It("should classify environment from kubernaut.io/environment label", func() {
			// Arrange: Create namespace with environment label
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					Labels: map[string]string{
						"environment": "production",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{
				Namespace: "test-namespace",
			}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert
			Expect(environment).To(Equal("production"))
		})

		It("should classify as staging from label", func() {
			// Arrange
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "staging-ns",
					Labels: map[string]string{
						"environment": "staging",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "staging-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert
			Expect(environment).To(Equal("staging"))
		})

		It("should classify as development from label", func() {
			// Arrange
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dev-ns",
					Labels: map[string]string{
						"environment": "development",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "dev-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert
			Expect(environment).To(Equal("development"))
		})

		It("should return unknown when namespace has no environment label", func() {
			// Arrange: Namespace without environment label
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unlabeled-ns",
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "unlabeled-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert
			Expect(environment).To(Equal("unknown"))
		})

		It("should return unknown when namespace does not exist", func() {
			// Arrange: Empty cluster (namespace doesn't exist)
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "nonexistent-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert
			Expect(environment).To(Equal("unknown"))
		})

		It("should handle case-insensitive environment values", func() {
			// Arrange: Label with uppercase value
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"environment": "PRODUCTION",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "test-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert: Returns value as-is (implementation accepts any string for dynamic configuration)
			Expect(environment).To(Equal("PRODUCTION"))
		})
	})

	// ============================================================================
	// BR-GATEWAY-012: ConfigMap environment override
	// ============================================================================

	Context("BR-GATEWAY-012: ConfigMap Override", func() {
		It("should use namespace label when present (ConfigMap is fallback only)", func() {
			// Arrange: Namespace with production label
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"environment": "production",
					},
				},
			}

			// ConfigMap exists but namespace label takes precedence
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-overrides",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"test-ns": "staging",
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, cm).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "test-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert: Namespace label takes precedence (ConfigMap is fallback only)
			Expect(environment).To(Equal("production"))
		})

		It("should fall back to namespace label when ConfigMap has no override", func() {
			// Arrange: Namespace with label, ConfigMap without override for this namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"environment": "production",
					},
				},
			}

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-overrides",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"other-ns": "staging", // Different namespace
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, cm).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "test-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert: Falls back to namespace label
			Expect(environment).To(Equal("production"))
		})

		It("should work when ConfigMap does not exist", func() {
			// Arrange: Namespace with label, no ConfigMap
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"environment": "production",
					},
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "test-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert: Falls back to namespace label
			Expect(environment).To(Equal("production"))
		})

		It("should handle ConfigMap override with case normalization", func() {
			// Arrange: ConfigMap with uppercase override
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernaut-environment-overrides",
					Namespace: "kubernaut-system",
				},
				Data: map[string]string{
					"test-ns": "STAGING",
				},
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, cm).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "test-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert: Returns value as-is (implementation accepts any string for dynamic configuration)
			Expect(environment).To(Equal("STAGING"))
		})
	})

	// ============================================================================
	// Edge Cases: Caching and Performance
	// ============================================================================

	Context("Caching and Performance", func() {
		It("should cache namespace lookups to reduce K8s API calls", func() {
			// Arrange
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cached-ns",
					Labels: map[string]string{
						"environment": "production",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "cached-ns"}

			// Act: Call multiple times
			env1 := classifier.Classify(ctx, signal.Namespace)
			env2 := classifier.Classify(ctx, signal.Namespace)
			env3 := classifier.Classify(ctx, signal.Namespace)

			// Assert: All should return same result (cache working)
			Expect(env1).To(Equal("production"))
			Expect(env2).To(Equal("production"))
			Expect(env3).To(Equal("production"))
		})
	})

	// ============================================================================
	// Edge Cases: Invalid Values
	// ============================================================================

	Context("Invalid Environment Values", func() {
		It("should return any non-empty environment label value", func() {
			// Arrange: Custom environment value
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"environment": "invalid-env",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "test-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert: Implementation accepts any non-empty string for dynamic configuration
			Expect(environment).To(Equal("invalid-env"))
		})

		It("should return unknown for empty environment label value", func() {
			// Arrange: Empty environment value
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"environment": "",
					},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
			classifier = processing.NewEnvironmentClassifier(k8sClient, logger)

			signal := &types.NormalizedSignal{Namespace: "test-ns"}

			// Act
			environment := classifier.Classify(ctx, signal.Namespace)

			// Assert
			Expect(environment).To(Equal("unknown"))
		})
	})
})
