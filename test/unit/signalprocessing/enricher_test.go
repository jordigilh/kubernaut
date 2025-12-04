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

package signalprocessing

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// BR-SP-051/052: K8s Context Enrichment - validates namespace, pod, node context fetching
var _ = Describe("BR-SP-051/052: K8s Enricher", func() {
	var (
		ctx      context.Context
		scheme   *runtime.Scheme
		m        *metrics.Metrics
		testNs   *corev1.Namespace
		testPod  *corev1.Pod
		testNode *corev1.Node
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Use a fresh registry per test to avoid "already registered" errors
		reg := prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(reg)

		testNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-namespace",
				Labels: map[string]string{
					"env": "production",
				},
			},
		}

		testPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "test-namespace",
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node",
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "nginx:latest",
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}

		testNode = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Labels: map[string]string{
					"kubernetes.io/os": "linux",
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
	})

	// Test 1: BR-SP-051 - Pod signal enrichment
	// BR-SP-051: Enricher must fetch namespace, pod, and node context for pod signals
	It("should enrich Pod signal with namespace, pod, and node context", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(testNs, testPod, testNode).
			Build()

		e := enricher.NewK8sEnricher(fakeClient, ctrl.Log.WithName("test"), m, 10*time.Second)

		result, err := e.EnrichPodSignal(ctx, "test-namespace", "test-pod")

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		// Verify namespace context
		Expect(result.NamespaceLabels).To(HaveKeyWithValue("env", "production"))

		// Verify pod context
		Expect(result.Pod).NotTo(BeNil())
		Expect(result.Pod.Name).To(Equal("test-pod"))
		Expect(result.Pod.Phase).To(Equal(string(corev1.PodRunning)))

		// Verify node context
		Expect(result.Node).NotTo(BeNil())
		Expect(result.Node.Name).To(Equal("test-node"))
	})

	// Test 2: Graceful degradation - missing pod
	// BR-SP-053: Enricher must gracefully degrade when pod not found (partial context)
	It("should return partial context when pod is not found", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(testNs). // No pod
			Build()

		e := enricher.NewK8sEnricher(fakeClient, ctrl.Log.WithName("test"), m, 10*time.Second)

		result, err := e.EnrichPodSignal(ctx, "test-namespace", "missing-pod")

		// Should NOT return error - graceful degradation
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		// Should have namespace context
		Expect(result.NamespaceLabels).To(HaveKeyWithValue("env", "production"))

		// Pod should be nil (not found)
		Expect(result.Pod).To(BeNil())
	})

	// Test 3: Error when namespace not found
	// BR-SP-051: Enricher must return error when required namespace not found
	It("should return error when namespace is not found", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build() // No namespace

		e := enricher.NewK8sEnricher(fakeClient, ctrl.Log.WithName("test"), m, 10*time.Second)

		result, err := e.EnrichPodSignal(ctx, "missing-namespace", "test-pod")

		// Should return error - namespace is required
		Expect(err).To(HaveOccurred())
		Expect(result).To(BeNil())
	})

	// Test 4: Namespace-only signal enrichment (for cluster-level or namespace-level signals)
	// BR-SP-052: Enricher should support namespace-only context when pod info unavailable
	It("should enrich namespace-only signal with namespace context", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(testNs).
			Build()

		e := enricher.NewK8sEnricher(fakeClient, ctrl.Log.WithName("test"), m, 10*time.Second)

		result, err := e.EnrichNamespaceOnly(ctx, "test-namespace")

		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		// Verify namespace labels are populated
		Expect(result.NamespaceLabels).To(HaveKeyWithValue("env", "production"))
		// Verify pod/node are NOT populated (namespace-only enrichment)
		Expect(result.Pod).To(BeNil())
		Expect(result.Node).To(BeNil())
	})
})

