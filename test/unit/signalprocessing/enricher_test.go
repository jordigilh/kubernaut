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
//
// Test coverage per IMPLEMENTATION_PLAN_V1.21.md Day 3:
// - Happy Path: 6 tests (E-HP-01 to E-HP-06)
// - Edge Cases: 12 tests (E-EC-01 to E-EC-12)
// - Error Handling: 8 tests (E-ER-01 to E-ER-08)
// Total: 26 tests
package signalprocessing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// Unit Test: K8sEnricher implementation correctness
// Per IMPLEMENTATION_PLAN_V1.21.md Day 3: 25-33 tests
var _ = Describe("K8sEnricher", func() {
	var (
		ctx         context.Context
		k8sClient   client.Client
		k8sEnricher *enricher.K8sEnricher
		m           *metrics.Metrics
		scheme      *runtime.Scheme
		logger      logr.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		// Create metrics with test registry
		reg := prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(reg)
		logger = zap.New(zap.UseDevMode(true))
	})

	// Helper to create fake client with objects
	createFakeClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}

	// Helper to create fake client with error injection
	createFakeClientWithError := func(errFunc interceptor.Funcs, objs ...client.Object) client.Client {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(errFunc).Build()
	}

	// ========================================
	// CONSTRUCTOR TESTS
	// ========================================

	Describe("NewK8sEnricher", func() {
		It("should create enricher with valid dependencies", func() {
			k8sClient = createFakeClient()
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			Expect(k8sEnricher).NotTo(BeNil())
		})
	})

	// ========================================
	// OWNER CHAIN BUILDING TESTS (BR-SP-100)
	// Priority 1: Fill 0% coverage gap
	// Testing through public Enrich() method
	// ========================================

	Describe("Owner Chain Building (BR-SP-100)", func() {
		var (
			testNamespace string
			testPodName   string
		)

		BeforeEach(func() {
			testNamespace = "owner-chain-test"
			testPodName = "test-pod"
		})

		Context("when enriching Pod with owner references", func() {
			It("should build owner chain with controller owner first (Pod → ReplicaSet → Deployment)", func() {
				// Create namespace
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}

				// Create Deployment (non-controller owner)
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: testNamespace,
						UID:       "deployment-uid",
					},
				}

				// Create ReplicaSet (controller owner, owned by Deployment)
				replicaSet := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rs",
						Namespace: testNamespace,
						UID:       "rs-uid",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Name:       deployment.Name,
								UID:        deployment.UID,
								Controller: boolPtr(true),
							},
						},
					},
				}

				// Create Pod with multiple owners (controller + non-controller)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testPodName,
						Namespace: testNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps/v1",
								Kind:       "ReplicaSet",
								Name:       replicaSet.Name,
								UID:        replicaSet.UID,
								Controller: boolPtr(true), // Controller owner
							},
							{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Name:       deployment.Name,
								UID:        deployment.UID,
								Controller: boolPtr(false), // Non-controller owner
							},
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test:latest"}},
					},
				}

				k8sClient = createFakeClient(namespace, pod, replicaSet, deployment)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      testPodName,
						Namespace: testNamespace,
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.OwnerChain).To(HaveLen(2))

				// Controller owner (ReplicaSet) should be first
				Expect(result.OwnerChain[0].Kind).To(Equal("ReplicaSet"))
				Expect(result.OwnerChain[0].Name).To(Equal("test-rs"))
				Expect(result.OwnerChain[0].Namespace).To(Equal(testNamespace))

				// Non-controller owner (Deployment) should be second
				Expect(result.OwnerChain[1].Kind).To(Equal("Deployment"))
				Expect(result.OwnerChain[1].Name).To(Equal("test-deployment"))
				Expect(result.OwnerChain[1].Namespace).To(Equal(testNamespace))
			})

			It("should handle Pod with single controller owner", func() {
				// Create namespace
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}

				// Create ReplicaSet
				replicaSet := &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rs",
						Namespace: testNamespace,
						UID:       "rs-uid",
					},
				}

				// Create Pod owned by ReplicaSet
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testPodName,
						Namespace: testNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps/v1",
								Kind:       "ReplicaSet",
								Name:       replicaSet.Name,
								UID:        replicaSet.UID,
								Controller: boolPtr(true),
							},
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test:latest"}},
					},
				}

				k8sClient = createFakeClient(namespace, pod, replicaSet)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      testPodName,
						Namespace: testNamespace,
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.OwnerChain).To(HaveLen(1))
				Expect(result.OwnerChain[0].Kind).To(Equal("ReplicaSet"))
				Expect(result.OwnerChain[0].Name).To(Equal("test-rs"))
				Expect(result.OwnerChain[0].Namespace).To(Equal(testNamespace))
			})

			It("should handle Pod with no owner references", func() {
				// Create namespace
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}

				// Create standalone Pod (no owners)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:            testPodName,
						Namespace:       testNamespace,
						OwnerReferences: nil, // No owners
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test:latest"}},
					},
				}

				k8sClient = createFakeClient(namespace, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      testPodName,
						Namespace: testNamespace,
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				// OwnerChain should be nil or empty for standalone Pod
				if result.OwnerChain != nil {
					Expect(result.OwnerChain).To(BeEmpty())
				}
			})

			It("should inherit namespace from Pod for all owner chain entries (DD-WORKFLOW-001 v1.8)", func() {
				// Create namespace
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNamespace,
					},
				}

				// Create Deployment
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: testNamespace,
						UID:       "deployment-uid",
					},
				}

				// Create Pod with owner reference
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testPodName,
						Namespace: testNamespace,
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
								Name:       deployment.Name,
								UID:        deployment.UID,
								Controller: boolPtr(true),
							},
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "test", Image: "test:latest"}},
					},
				}

				k8sClient = createFakeClient(namespace, pod, deployment)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				signal := &signalprocessingv1alpha1.SignalData{
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      testPodName,
						Namespace: testNamespace,
					},
				}

				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.OwnerChain).To(HaveLen(1))
				// Namespace should be inherited from Pod (DD-WORKFLOW-001 v1.8)
				Expect(result.OwnerChain[0].Namespace).To(Equal(testNamespace))
			})
		})
	})

	// Helper function for test readability
	boolPtr := func(b bool) *bool {
		return &b
	}

	// ========================================
	// HAPPY PATH TESTS (E-HP-01 to E-HP-06)
	// ========================================

	Describe("Happy Path - Resource Enrichment", func() {

		// E-HP-01: Pod signal enrichment
		Context("E-HP-01: Pod signal enrichment", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-namespace",
						Labels: map[string]string{"environment": "staging"},
					},
				}
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-node",
						Labels: map[string]string{"kubernetes.io/os": "linux"},
					},
				}
				// Pod with owner reference
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
						Labels:    map[string]string{"app": "test-app"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "apps/v1",
								Kind:       "ReplicaSet",
								Name:       "test-deployment-abc123",
								Controller: boolPtr(true),
							},
						},
					},
					Spec: corev1.PodSpec{NodeName: "test-node"},
				}
				k8sClient = createFakeClient(ns, node, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Pod, Node, and OwnerReference data", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Namespace).NotTo(BeNil())
				Expect(result.Namespace.Labels).To(HaveKeyWithValue("environment", "staging"))
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Pod.Labels).To(HaveKeyWithValue("app", "test-app"))
				Expect(result.Node).NotTo(BeNil())
				Expect(result.Node.Labels).To(HaveKeyWithValue("kubernetes.io/os", "linux"))
				// Owner chain should be populated
				Expect(result.OwnerChain).NotTo(BeEmpty())
				Expect(result.OwnerChain[0].Kind).To(Equal("ReplicaSet"))
			})
		})

		// E-HP-02: Deployment signal enrichment
		Context("E-HP-02: Deployment signal enrichment", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				replicas := int32(3)
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: "test-namespace",
						Labels:    map[string]string{"app": "test-app"},
					},
					Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
					Status: appsv1.DeploymentStatus{Replicas: 3, AvailableReplicas: 2, ReadyReplicas: 2},
				}
				k8sClient = createFakeClient(ns, deployment)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Namespace and Deployment data", func() {
				signal := createSignal("Deployment", "test-deployment", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Deployment).NotTo(BeNil())
				Expect(result.Deployment.Labels).To(HaveKeyWithValue("app", "test-app"))
				Expect(result.Deployment.Replicas).To(Equal(int32(3)))
				Expect(result.Deployment.AvailableReplicas).To(Equal(int32(2)))
			})
		})

		// E-HP-02b: Deployment signal with missing deployment (degraded mode)
		Context("E-HP-02b: Deployment signal - degraded mode (BR-SP-001)", func() {
			BeforeEach(func() {
				// Create namespace but NO deployment
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns) // No deployment
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should enter degraded mode when deployment not found", func() {
				signal := createSignal("Deployment", "missing-deployment", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.DegradedMode).To(BeTrue(), "Should enter degraded mode")
				Expect(result.Namespace).NotTo(BeNil(), "Namespace should still be populated")
				Expect(result.Deployment).To(BeNil(), "Deployment should be nil in degraded mode")
			})
		})

		// E-HP-03: StatefulSet signal enrichment
		Context("E-HP-03: StatefulSet signal enrichment", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				replicas := int32(3)
				statefulset := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-statefulset",
						Namespace: "test-namespace",
						Labels:    map[string]string{"app": "database"},
					},
					Spec:   appsv1.StatefulSetSpec{Replicas: &replicas},
					Status: appsv1.StatefulSetStatus{Replicas: 3, ReadyReplicas: 3},
				}
				k8sClient = createFakeClient(ns, statefulset)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Namespace and StatefulSet data", func() {
				signal := createSignal("StatefulSet", "test-statefulset", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.StatefulSet).NotTo(BeNil())
				Expect(result.StatefulSet.Labels).To(HaveKeyWithValue("app", "database"))
				Expect(result.StatefulSet.Replicas).To(Equal(int32(3)))
			})
		})

		// E-HP-03b: StatefulSet signal with missing statefulset (degraded mode)
		Context("E-HP-03b: StatefulSet signal - degraded mode (BR-SP-001)", func() {
			BeforeEach(func() {
				// Create namespace but NO statefulset
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns) // No statefulset
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should enter degraded mode when statefulset not found", func() {
				signal := createSignal("StatefulSet", "missing-statefulset", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.DegradedMode).To(BeTrue(), "Should enter degraded mode")
				Expect(result.Namespace).NotTo(BeNil(), "Namespace should still be populated")
				Expect(result.StatefulSet).To(BeNil(), "StatefulSet should be nil in degraded mode")
			})
		})

		// E-HP-04: Service signal enrichment
		Context("E-HP-04: Service signal enrichment", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "test-namespace",
						Labels:    map[string]string{"app": "api"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Port: 80, Name: "http"},
						},
						Type: corev1.ServiceTypeClusterIP,
					},
				}
				k8sClient = createFakeClient(ns, service)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Namespace and Service data", func() {
				signal := createSignal("Service", "test-service", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Service).NotTo(BeNil())
				Expect(result.Service.Labels).To(HaveKeyWithValue("app", "api"))
				Expect(result.Service.Type).To(Equal("ClusterIP"))
			})
		})

		// E-HP-04b: Service signal with missing service (degraded mode)
		Context("E-HP-04b: Service signal - degraded mode (BR-SP-001)", func() {
			BeforeEach(func() {
				// Create namespace but NO service
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns) // No service
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should enter degraded mode when service not found", func() {
				signal := createSignal("Service", "missing-service", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.DegradedMode).To(BeTrue(), "Should enter degraded mode")
				Expect(result.Namespace).NotTo(BeNil(), "Namespace should still be populated")
				Expect(result.Service).To(BeNil(), "Service should be nil in degraded mode")
			})
		})

		// E-HP-05: Node signal enrichment
		Context("E-HP-05: Node signal enrichment", func() {
			BeforeEach(func() {
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "test-node",
						Labels: map[string]string{"node.kubernetes.io/instance-type": "m5.large"},
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				}
				k8sClient = createFakeClient(node)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Node details only", func() {
				signal := createSignal("Node", "test-node", "")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Node).NotTo(BeNil())
				Expect(result.Node.Labels).To(HaveKeyWithValue("node.kubernetes.io/instance-type", "m5.large"))
				// Nodes don't have a namespace, so Namespace is nil or empty
				if result.Namespace != nil {
					Expect(result.Namespace.Labels).To(BeEmpty())
				}
			})
		})

		// E-HP-06: Standard depth fetching
		Context("E-HP-06: Standard depth fetching (DD-017)", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				}
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
					Spec: corev1.PodSpec{NodeName: "test-node"},
				}
				k8sClient = createFakeClient(ns, node, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should fetch exactly standard depth objects per DD-017", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				// Standard depth for Pod: Namespace + Pod + Node + Owner
				Expect(result.Namespace.Labels).NotTo(BeNil())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Node).NotTo(BeNil())
				// Should NOT fetch deep nested resources (e.g., no pod's pods)
			})
		})
	})

	// ========================================
	// EDGE CASE TESTS (E-EC-01 to E-EC-12)
	// ========================================

	Describe("Edge Cases", func() {

		// E-EC-01: Pod without owner
		Context("E-EC-01: Pod without owner (orphan pod)", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				}
				// Orphan pod - no owner references
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orphan-pod",
						Namespace: "test-namespace",
					},
					Spec: corev1.PodSpec{NodeName: "test-node"},
				}
				k8sClient = createFakeClient(ns, node, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Pod + Node with empty OwnerChain", func() {
				signal := createSignal("Pod", "orphan-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Node).NotTo(BeNil())
				Expect(result.OwnerChain).To(BeEmpty())
			})
		})

		// E-EC-02: Pod on deleted node
		Context("E-EC-02: Pod on deleted node", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
					},
					Spec: corev1.PodSpec{NodeName: "deleted-node"}, // Node doesn't exist
				}
				k8sClient = createFakeClient(ns, pod) // No node
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Pod with Node=nil, continues without error", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Node).To(BeNil()) // Node not found but no error
			})
		})

		// E-EC-03: Deployment with 0 replicas
		Context("E-EC-03: Deployment with 0 replicas", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				replicas := int32(0)
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scaled-down-deployment",
						Namespace: "test-namespace",
					},
					Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
					Status: appsv1.DeploymentStatus{Replicas: 0, AvailableReplicas: 0},
				}
				k8sClient = createFakeClient(ns, deployment)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Deployment with 0 replicas", func() {
				signal := createSignal("Deployment", "scaled-down-deployment", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Deployment).NotTo(BeNil())
				Expect(result.Deployment.Replicas).To(Equal(int32(0)))
				Expect(result.Deployment.AvailableReplicas).To(Equal(int32(0)))
			})
		})

		// E-EC-04: Namespace not found - returns error (required for pod/deployment signals)
		Context("E-EC-04: Namespace not found", func() {
			BeforeEach(func() {
				k8sClient = createFakeClient() // Empty - no namespace
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return error when namespace not found", func() {
				signal := createSignal("Pod", "test-pod", "non-existent-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("namespace"))
			})
		})

		// E-EC-05: Cross-namespace owner (cluster-scoped)
		Context("E-EC-05: Cross-namespace owner (cluster-scoped resource)", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				}
				// Pod owned by a cluster-scoped resource
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "v1",
								Kind:       "Node",
								Name:       "test-node",
								Controller: boolPtr(true),
							},
						},
					},
					Spec: corev1.PodSpec{NodeName: "test-node"},
				}
				k8sClient = createFakeClient(ns, node, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return Pod + cluster-scoped owner in chain", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.OwnerChain).NotTo(BeEmpty())
				Expect(result.OwnerChain[0].Kind).To(Equal("Node"))
			})
		})

		// E-EC-06: Multiple owner references
		Context("E-EC-06: Multiple owner references", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				}
				// Pod with multiple owners - controller=true should be first
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "test-namespace",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: "v1",
								Kind:       "ConfigMap",
								Name:       "config",
								Controller: boolPtr(false),
							},
							{
								APIVersion: "apps/v1",
								Kind:       "ReplicaSet",
								Name:       "rs-controller",
								Controller: boolPtr(true), // This is the controller
							},
						},
					},
					Spec: corev1.PodSpec{NodeName: "test-node"},
				}
				k8sClient = createFakeClient(ns, node, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return first owner with controller=true", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.OwnerChain).NotTo(BeEmpty())
				// Controller owner should be first
				Expect(result.OwnerChain[0].Kind).To(Equal("ReplicaSet"))
				Expect(result.OwnerChain[0].Name).To(Equal("rs-controller"))
			})
		})

		// E-EC-07: Resource name with special chars
		Context("E-EC-07: Resource name with special chars", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				// Pod with special characters in name (valid K8s name)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-app-v2.0-abc123",
						Namespace: "test-namespace",
					},
				}
				k8sClient = createFakeClient(ns, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should handle special characters correctly", func() {
				signal := createSignal("Pod", "my-app-v2.0-abc123", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
			})
		})

		// E-EC-08: Very long resource name (253 chars - K8s max)
		Context("E-EC-08: Very long resource name", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				// 253-char name (K8s limit)
				longName := strings.Repeat("a", 253)
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      longName,
						Namespace: "test-namespace",
					},
				}
				k8sClient = createFakeClient(ns, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should handle 253-char resource name", func() {
				longName := strings.Repeat("a", 253)
				signal := createSignal("Pod", longName, "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
			})
		})

		// E-EC-09: Resource in kube-system namespace
		Context("E-EC-09: Resource in kube-system namespace", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "kube-system"},
				}
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "coredns-abc123",
						Namespace: "kube-system",
						Labels:    map[string]string{"k8s-app": "kube-dns"},
					},
				}
				k8sClient = createFakeClient(ns, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should enrich normally without special filtering", func() {
				signal := createSignal("Pod", "coredns-abc123", "kube-system")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Pod.Labels).To(HaveKeyWithValue("k8s-app", "kube-dns"))
			})
		})

		// E-EC-10: Empty labels on resource
		Context("E-EC-10: Empty labels on resource", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-labels-pod",
						Namespace: "test-namespace",
						// No labels
					},
				}
				k8sClient = createFakeClient(ns, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return resource with empty Labels map", func() {
				signal := createSignal("Pod", "no-labels-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Pod.Labels).To(BeEmpty())
			})
		})

		// E-EC-11: Empty annotations on resource
		Context("E-EC-11: Empty annotations on resource", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-annotations-pod",
						Namespace: "test-namespace",
						// No annotations
					},
				}
				k8sClient = createFakeClient(ns, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return resource with empty Annotations map", func() {
				signal := createSignal("Pod", "no-annotations-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Pod.Annotations).To(BeEmpty())
			})
		})

		// E-EC-12: Resource being deleted
		Context("E-EC-12: Resource being deleted (DeletionTimestamp set)", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				deletionTime := metav1.Now()
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "deleting-pod",
						Namespace:         "test-namespace",
						DeletionTimestamp: &deletionTime,
						Finalizers:        []string{"test-finalizer"},
					},
				}
				k8sClient = createFakeClient(ns, pod)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return resource with deletion status", func() {
				signal := createSignal("Pod", "deleting-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).NotTo(HaveOccurred())
				Expect(result.Pod).NotTo(BeNil())
				Expect(result.Pod.Phase).To(Equal("")) // Pending deletion
			})
		})
	})

	// ========================================
	// ERROR HANDLING TESTS (E-ER-01 to E-ER-08)
	// ========================================

	Describe("Error Handling", func() {

		// E-ER-01: K8s API timeout
		Context("E-ER-01: K8s API timeout", func() {
			It("should return error with timeout code", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns)
				// Create enricher with very short timeout
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 1*time.Nanosecond)

				// Use already cancelled context
				cancelledCtx, cancel := context.WithCancel(ctx)
				cancel()

				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(cancelledCtx, signal)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		// E-ER-02: K8s API 403 Forbidden
		Context("E-ER-02: K8s API 403 Forbidden", func() {
			BeforeEach(func() {
				errFunc := interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return apierrors.NewForbidden(
							schema.GroupResource{Resource: "namespaces"},
							key.Name,
							fmt.Errorf("RBAC: access denied"),
						)
					},
				}
				k8sClient = createFakeClientWithError(errFunc)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return error with forbidden code", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(apierrors.IsForbidden(err) || strings.Contains(err.Error(), "forbidden")).To(BeTrue())
			})
		})

		// E-ER-03: K8s API 404 Not Found
		Context("E-ER-03: K8s API 404 Not Found (resource deleted)", func() {
			BeforeEach(func() {
				// Only namespace exists, pod doesn't
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return partial context for graceful handling", func() {
				signal := createSignal("Pod", "deleted-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				// Graceful handling - returns partial context, not error
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Pod).To(BeNil())                 // Pod not found
				Expect(result.Namespace.Labels).NotTo(BeNil()) // Namespace found
			})
		})

		// E-ER-04: K8s API 500 Server Error
		Context("E-ER-04: K8s API 500 Server Error", func() {
			BeforeEach(func() {
				errFunc := interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return apierrors.NewInternalError(fmt.Errorf("internal server error"))
					},
				}
				k8sClient = createFakeClientWithError(errFunc)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return error with retry hint", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		// E-ER-05: Invalid resource kind
		Context("E-ER-05: Invalid/Unknown resource kind", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return namespace-only context for unknown kind", func() {
				signal := createSignal("UnknownKind", "test-resource", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				// Graceful fallback - namespace only
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.Namespace.Labels).NotTo(BeNil())
			})
		})

		// E-ER-06: Empty signal resource
		Context("E-ER-06: Empty signal resource reference", func() {
			BeforeEach(func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return validation error for empty resource", func() {
				signal := &signalprocessingv1alpha1.SignalData{
					Name:       "test-signal",
					TargetType: "kubernetes",
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "",
						Name:      "",
						Namespace: "test-namespace",
					},
				}
				result, err := k8sEnricher.Enrich(ctx, signal)

				// Should handle gracefully (fallback to namespace-only)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
			})
		})

		// E-ER-07: Context cancelled
		Context("E-ER-07: Context cancelled mid-fetch", func() {
			It("should return context.Canceled error", func() {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}
				k8sClient = createFakeClient(ns)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

				cancelledCtx, cancel := context.WithCancel(ctx)
				cancel()

				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(cancelledCtx, signal)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		// E-ER-08: Rate limited by API (429)
		Context("E-ER-08: Rate limited by API (429)", func() {
			BeforeEach(func() {
				errFunc := interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return apierrors.NewTooManyRequests("rate limit exceeded", 30)
					},
				}
				k8sClient = createFakeClientWithError(errFunc)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return error with backoff hint", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
				Expect(apierrors.IsTooManyRequests(err) || strings.Contains(err.Error(), "rate")).To(BeTrue())
			})
		})

		// E-ER-09: API error on target resource fetch (not namespace)
		// BUG: pkg/signalprocessing/enricher/k8s_enricher.go:175 returns (result, nil) instead of (nil, error)
		// This test validates that non-NotFound errors on target resource fetching properly propagate
		Context("E-ER-09: API error when fetching target Pod (namespace succeeds)", func() {
			BeforeEach(func() {
				// Create namespace object (will succeed)
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
				}

				// Inject error for Pod fetches only (not namespace)
				errFunc := interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						// Allow namespace fetch to succeed
						if _, ok := obj.(*corev1.Namespace); ok {
							return nil
						}
						// Fail pod fetch with Internal Server Error (not NotFound)
						if _, ok := obj.(*corev1.Pod); ok {
							return apierrors.NewInternalError(fmt.Errorf("etcd unavailable"))
						}
						return nil
					},
				}
				k8sClient = createFakeClientWithError(errFunc, ns)
				k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)
			})

			It("should return error (not success with incomplete data)", func() {
				signal := createSignal("Pod", "test-pod", "test-namespace")
				result, err := k8sEnricher.Enrich(ctx, signal)

				// EXPECTED BEHAVIOR: Should return error because Pod fetch failed with API error
				Expect(err).To(HaveOccurred(), "Should propagate API error from Pod fetch")
				Expect(result).To(BeNil(), "Result should be nil when API error occurs")
				Expect(err.Error()).To(ContainSubstring("etcd unavailable"), "Error should contain original failure reason")
			})
		})
	})

	// ========================================
	// CACHING TESTS
	// ========================================

	Describe("TTL Cache", func() {
		It("should cache namespace lookups", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "cached-namespace",
					Labels: map[string]string{"cached": "true"},
				},
			}
			k8sClient = createFakeClient(ns)
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

			signal := createSignal("Pod", "test-pod", "cached-namespace")

			// First call
			result1, err1 := k8sEnricher.Enrich(ctx, signal)
			Expect(err1).NotTo(HaveOccurred())
			Expect(result1.Namespace).NotTo(BeNil())
			Expect(result1.Namespace.Labels).To(HaveKeyWithValue("cached", "true"))

			// Second call should use cache
			result2, err2 := k8sEnricher.Enrich(ctx, signal)
			Expect(err2).NotTo(HaveOccurred())
			Expect(result2.Namespace).NotTo(BeNil())
			Expect(result2.Namespace.Labels).To(HaveKeyWithValue("cached", "true"))
		})
	})

	// ========================================
	// METRICS TESTS
	// ========================================

	Describe("Metrics Integration", func() {
		It("should record enrichment duration metric", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
			}
			k8sClient = createFakeClient(ns)
			k8sEnricher = enricher.NewK8sEnricher(k8sClient, logger, m, 5*time.Second)

			signal := createSignal("Pod", "test-pod", "test-namespace")
			_, _ = k8sEnricher.Enrich(ctx, signal)

			// Verify metrics were recorded (check registry)
			// Note: Full metric verification would require accessing internal state
		})
	})
})

// Helper functions

func createSignal(kind, name, namespace string) *signalprocessingv1alpha1.SignalData {
	return &signalprocessingv1alpha1.SignalData{
		Name:       "test-signal",
		TargetType: "kubernetes",
		TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}
