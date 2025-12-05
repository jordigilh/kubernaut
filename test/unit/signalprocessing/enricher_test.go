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
// See docs/development/business-requirements/TESTING_GUIDELINES.md
package signalprocessing

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// Unit Test: K8sEnricher implementation correctness
var _ = Describe("K8sEnricher", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		k8sEnricher *enricher.K8sEnricher
		m          *metrics.Metrics
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		// Create metrics with test registry
		reg := prometheus.NewRegistry()
		m = metrics.NewMetrics(reg)
	})

	// Helper to create fake client with objects
	createFakeClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}

	// Test 1: Constructor creates enricher with valid dependencies
	Describe("NewK8sEnricher", func() {
		It("should create enricher with valid dependencies", func() {
			k8sClient = createFakeClient()
			logger := zap.New(zap.UseDevMode(true))

			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

			Expect(k8sEnricher).NotTo(BeNil())
		})
	})

	// Test 2: Pod signal enrichment returns Pod, Node, Namespace data
	Describe("Enrich - Pod Signal", func() {
		BeforeEach(func() {
			// Create test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					Labels: map[string]string{
						"environment": "staging",
					},
				},
			}

			// Create test node
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"kubernetes.io/os": "linux",
					},
				},
			}

			// Create test pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"app": "test-app",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node",
				},
			}

			k8sClient = createFakeClient(ns, node, pod)
			logger := zap.New(zap.UseDevMode(true))
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
		})

		It("should return Pod, Node, and Namespace context for Pod signal", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:       "test-signal",
				TargetType: "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			}

			result, err := k8sEnricher.Enrich(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Verify namespace was enriched
			Expect(result.NamespaceLabels).To(HaveKeyWithValue("environment", "staging"))
			// Verify pod was enriched
			Expect(result.Pod).NotTo(BeNil())
			Expect(result.Pod.Labels).To(HaveKeyWithValue("app", "test-app"))
			// Verify node was enriched
			Expect(result.Node).NotTo(BeNil())
			Expect(result.Node.Labels).To(HaveKeyWithValue("kubernetes.io/os", "linux"))
		})
	})

	// Test 3: Deployment signal enrichment returns Namespace and Deployment data
	Describe("Enrich - Deployment Signal", func() {
		BeforeEach(func() {
			// Create test namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
				},
			}

			// Create test deployment
			replicas := int32(3)
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"app": "test-app",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
				},
				Status: appsv1.DeploymentStatus{
					Replicas:          3,
					AvailableReplicas: 2,
					ReadyReplicas:     2,
				},
			}

			k8sClient = createFakeClient(ns, deployment)
			logger := zap.New(zap.UseDevMode(true))
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
		})

		It("should return Namespace and Deployment context for Deployment signal", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:       "test-signal",
				TargetType: "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "test-deployment",
					Namespace: "test-namespace",
				},
			}

			result, err := k8sEnricher.Enrich(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Verify deployment was enriched
			Expect(result.Deployment).NotTo(BeNil())
			Expect(result.Deployment.Labels).To(HaveKeyWithValue("app", "test-app"))
			Expect(result.Deployment.Replicas).To(Equal(int32(3)))
			Expect(result.Deployment.AvailableReplicas).To(Equal(int32(2)))
		})
	})

	// Test 4: Node signal enrichment returns Node data only
	Describe("Enrich - Node Signal", func() {
		BeforeEach(func() {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "m5.large",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			k8sClient = createFakeClient(node)
			logger := zap.New(zap.UseDevMode(true))
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
		})

		It("should return Node context only for Node signal", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:       "test-signal",
				TargetType: "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind: "Node",
					Name: "test-node",
					// No namespace for node signals
				},
			}

			result, err := k8sEnricher.Enrich(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Verify node was enriched
			Expect(result.Node).NotTo(BeNil())
			Expect(result.Node.Labels).To(HaveKeyWithValue("node.kubernetes.io/instance-type", "m5.large"))
			// Namespace should be empty for node signals
			Expect(result.NamespaceLabels).To(BeEmpty())
		})
	})

	// Test 5: Unknown resource kind falls back to namespace-only enrichment
	Describe("Enrich - Unknown Resource Kind", func() {
		BeforeEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					Labels: map[string]string{
						"team": "platform",
					},
				},
			}

			k8sClient = createFakeClient(ns)
			logger := zap.New(zap.UseDevMode(true))
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
		})

		It("should return namespace-only context for unknown resource kind", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:       "test-signal",
				TargetType: "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "CustomResource",
					Name:      "test-cr",
					Namespace: "test-namespace",
				},
			}

			result, err := k8sEnricher.Enrich(ctx, signal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Only namespace should be enriched
			Expect(result.NamespaceLabels).To(HaveKeyWithValue("team", "platform"))
			// Other fields should be nil
			Expect(result.Pod).To(BeNil())
			Expect(result.Deployment).To(BeNil())
			Expect(result.Node).To(BeNil())
		})
	})

	// Test 6: Pod not found returns partial context (namespace only)
	Describe("Enrich - Pod Not Found", func() {
		BeforeEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
				},
			}

			k8sClient = createFakeClient(ns) // No pod created
			logger := zap.New(zap.UseDevMode(true))
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
		})

		It("should return partial context when pod not found", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:       "test-signal",
				TargetType: "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "non-existent-pod",
					Namespace: "test-namespace",
				},
			}

			result, err := k8sEnricher.Enrich(ctx, signal)

			// Should not error, but return partial context
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Namespace should be enriched
			Expect(result.NamespaceLabels).NotTo(BeNil())
			// Pod should be nil
			Expect(result.Pod).To(BeNil())
		})
	})

	// Test 7: Namespace not found returns error
	Describe("Enrich - Namespace Not Found", func() {
		BeforeEach(func() {
			k8sClient = createFakeClient() // No namespace created
			logger := zap.New(zap.UseDevMode(true))
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
		})

		It("should return error when namespace not found for Pod signal", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:       "test-signal",
				TargetType: "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "non-existent-namespace",
				},
			}

			result, err := k8sEnricher.Enrich(ctx, signal)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("namespace"))
		})
	})

	// Test 8: Context timeout returns error
	Describe("Enrich - Context Timeout", func() {
		BeforeEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
				},
			}

			k8sClient = createFakeClient(ns)
			logger := zap.New(zap.UseDevMode(true))
			// Very short timeout
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 1*time.Nanosecond)
		})

		It("should return error when context times out", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:       "test-signal",
				TargetType: "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			}

			// Use an already cancelled context
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()

			result, err := k8sEnricher.Enrich(cancelledCtx, signal)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})
})
