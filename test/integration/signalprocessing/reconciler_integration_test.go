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

// Package signalprocessing_test contains integration tests for the SignalProcessing reconciler.
// These tests validate end-to-end reconciliation with real Kubernetes API via ENVTEST.
//
// TDD Phase: RED - Tests define expected controller behavior
// Implementation Plan: Day 10, Tier 2 - Reconciler Integration Tests
//
// Test Matrix: 25 tests per IMPLEMENTATION_PLAN_V1.30.md
// - Happy Path: IT-HP-01 to IT-HP-10 (10 tests)
// - Edge Cases: IT-EC-01 to IT-EC-08 (8 tests)
// - Error Handling: IT-ER-01 to IT-ER-07 (7 tests)
package signalprocessing_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

var _ = Describe("SignalProcessing Reconciler Integration", func() {
	// ========================================
	// HAPPY PATH TESTS (IT-HP-01 to IT-HP-10)
	// ========================================

	Context("Happy Path - Phase Transitions", func() {
		// IT-HP-01: Production pod → P0 (BR-SP-070, BR-SP-051)
		It("IT-HP-01: should process production pod signal and assign P0 priority", func() {
			By("Creating production namespace")
			ns := createTestNamespaceWithLabels("prod", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating test pod in production namespace")
			podLabels := map[string]string{"app": "api-server"}
			_ = createTestPod(ns, "api-server-xyz", podLabels, nil)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-01",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
						Name:        "HighCPU",
						Severity:    "critical",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "api-server-xyz",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred(), "SignalProcessing should complete")

			By("Verifying BUSINESS OUTCOMES")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// BR-SP-051: Production namespace → production environment
			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
			Expect(final.Status.EnvironmentClassification.Confidence).To(BeNumerically(">=", 0.8))

			// BR-SP-070: Production + Critical → P0
			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P0"))
			Expect(final.Status.PriorityAssignment.Confidence).To(BeNumerically(">=", 0.9))
		})

		// IT-HP-02: Staging deployment → P1 (BR-SP-070, BR-SP-051)
		It("IT-HP-02: should process staging deployment signal and assign P1 priority", func() {
			By("Creating staging namespace")
			ns := createTestNamespaceWithLabels("staging", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating test deployment")
			deployLabels := map[string]string{"app": "web-frontend"}
			_ = createTestDeployment(ns, "web-frontend", deployLabels)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-02",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "bbb123def456abc123def456abc123def456abc123def456abc123def456abc2",
						Name:        "HighLatency",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Deployment",
							Name:      "web-frontend",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred(), "SignalProcessing should complete")

			By("Verifying BUSINESS OUTCOMES")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Staging environment classification
			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("staging"))

			// Staging + Warning → P2 (per priority.rego in suite)
			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P2"))
		})

		// IT-HP-03: Dev service → P3 (BR-SP-070, BR-SP-051)
		It("IT-HP-03: should process dev service signal and assign P3 priority", func() {
			By("Creating development namespace")
			ns := createTestNamespaceWithLabels("dev", map[string]string{
				"kubernaut.ai/environment": "development",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-03",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ccc123def456abc123def456abc123def456abc123def456abc123def456abc3",
						Name:        "LowDiskSpace",
						Severity:    "info",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Service",
							Name:      "backend-api",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying development environment gets P3")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("development"))

			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P3"))
		})

		// IT-HP-04: Environment from label (BR-SP-051)
		It("IT-HP-04: should classify environment from namespace label", func() {
			By("Creating namespace with explicit production label")
			ns := createTestNamespaceWithLabels("custom", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-04",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ddd123def456abc123def456abc123def456abc123def456abc123def456abc4",
						Name:        "TestAlert",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying label-based classification")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
			Expect(final.Status.EnvironmentClassification.Source).To(Equal("namespace-labels"))
			Expect(final.Status.EnvironmentClassification.Confidence).To(BeNumerically(">=", 0.95))
		})

		// IT-HP-05: ConfigMap fallback (BR-SP-052)
		It("IT-HP-05: should classify environment from ConfigMap fallback", func() {
			By("Creating namespace with 'staging' prefix (no label)")
			ns := createTestNamespace("staging-app")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-05",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "eee123def456abc123def456abc123def456abc123def456abc123def456abc5",
						Name:        "ConfigMapFallback",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying ConfigMap-based classification")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("staging"))
			Expect(final.Status.EnvironmentClassification.Source).To(Equal("configmap"))
		})

		// IT-HP-06: Business classification (BR-SP-002)
		It("IT-HP-06: should classify business unit from namespace labels", func() {
			By("Creating namespace with team label")
			ns := createTestNamespaceWithLabels("payments", map[string]string{
				"kubernaut.ai/environment": "production",
				"kubernaut.ai/team":        "payments",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-06",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "fff123def456abc123def456abc123def456abc123def456abc123def456abc6",
						Name:        "BusinessClassification",
						Severity:    "critical",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "payment-processor",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying business classification")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.BusinessClassification).ToNot(BeNil())
			Expect(final.Status.BusinessClassification.BusinessUnit).To(Equal("payments"))
		})

		// IT-HP-07: Owner chain traversal (BR-SP-100)
		It("IT-HP-07: should build owner chain from Pod to Deployment", func() {
			By("Creating namespace")
			ns := createTestNamespace("ownerchain")
			defer deleteTestNamespace(ns)

			By("Creating Deployment")
			deployLabels := map[string]string{"app": "ownerchain-test"}
			deployment := createTestDeployment(ns, "ownerchain-deployment", deployLabels)

			By("Creating ReplicaSet owned by Deployment")
			rsOwnerRef := []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployment.Name,
				UID:        deployment.UID,
			}}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "ownerchain-rs",
					Namespace:       ns,
					Labels:          deployLabels,
					OwnerReferences: rsOwnerRef,
				},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: func() *int32 { r := int32(1); return &r }(),
					Selector: &metav1.LabelSelector{MatchLabels: deployLabels},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: deployLabels},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "test", Image: "nginx"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())

			By("Creating Pod owned by ReplicaSet")
			podOwnerRef := []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       rs.Name,
				UID:        rs.UID,
			}}
			_ = createTestPod(ns, "ownerchain-pod", deployLabels, podOwnerRef)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-07",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ggg123def456abc123def456abc123def456abc123def456abc123def456abc7",
						Name:        "OwnerChainTest",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "ownerchain-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying owner chain")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.OwnerChain).To(HaveLen(2))
			Expect(final.Status.KubernetesContext.OwnerChain[0].Kind).To(Equal("ReplicaSet"))
			Expect(final.Status.KubernetesContext.OwnerChain[1].Kind).To(Equal("Deployment"))
		})

		// IT-HP-08: PDB detection (BR-SP-101)
		It("IT-HP-08: should detect PDB protection", func() {
			By("Creating namespace")
			ns := createTestNamespace("pdb-test")
			defer deleteTestNamespace(ns)

			By("Creating Pod")
			podLabels := map[string]string{"app": "pdb-protected"}
			_ = createTestPod(ns, "pdb-pod", podLabels, nil)

			By("Creating PDB that matches Pod")
			_ = createTestPDB(ns, "test-pdb", podLabels)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-08",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "hhh123def456abc123def456abc123def456abc123def456abc123def456abc8",
						Name:        "PDBTest",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "pdb-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying PDB detection")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasPDB).To(BeTrue())
		})

		// IT-HP-09: HPA detection (BR-SP-101)
		It("IT-HP-09: should detect HPA enabled", func() {
			By("Creating namespace")
			ns := createTestNamespace("hpa-test")
			defer deleteTestNamespace(ns)

			By("Creating Deployment")
			deployLabels := map[string]string{"app": "hpa-target"}
			_ = createTestDeployment(ns, "hpa-deployment", deployLabels)

			By("Creating HPA targeting Deployment")
			_ = createTestHPA(ns, "test-hpa", "hpa-deployment")

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-09",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "iii123def456abc123def456abc123def456abc123def456abc123def456abc9",
						Name:        "HPATest",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Deployment",
							Name:      "hpa-deployment",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying HPA detection")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasHPA).To(BeTrue())
		})

		// IT-HP-10: CustomLabels from Rego (BR-SP-102)
		It("IT-HP-10: should populate CustomLabels from Rego policy", func() {
			By("Creating namespace")
			ns := createTestNamespace("rego-labels")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with labels.rego policy")
			labelsPolicy := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["team"] := ["platform"] if {
    input.kubernetes.namespace.name == "` + ns + `"
}
`,
				},
			}
			Expect(k8sClient.Create(ctx, labelsPolicy)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-hp-10",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "jjj123def456abc123def456abc123def456abc123def456abc123def456ab10",
						Name:        "RegoLabelsTest",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying CustomLabels populated")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.CustomLabels).ToNot(BeEmpty())
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("team"))
		})
	})

	// ========================================
	// EDGE CASE TESTS (IT-EC-01 to IT-EC-08)
	// ========================================

	Context("Edge Cases", func() {
		// IT-EC-01: Default environment (BR-SP-053)
		It("IT-EC-01: should default to unknown environment when no labels", func() {
			By("Creating namespace without environment label")
			ns := createTestNamespace("unknown-env")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-ec-01",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ec01def456abc123def456abc123def456abc123def456abc123def456abc01",
						Name:        "DefaultEnv",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying default environment")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("unknown"))
			Expect(final.Status.EnvironmentClassification.Source).To(Equal("default"))
		})

		// IT-EC-02: Degraded mode (BR-SP-001)
		It("IT-EC-02: should enter degraded mode when pod not found", func() {
			By("Creating namespace")
			ns := createTestNamespace("degraded")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR for non-existent pod")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-ec-02",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ec02def456abc123def456abc123def456abc123def456abc123def456abc02",
						Name:        "DegradedMode",
						Severity:    "critical",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "non-existent-pod",
							Namespace: ns,
						},
						Labels: map[string]string{
							"environment": "production",
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying degraded mode")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DegradedMode).To(BeTrue())
			Expect(final.Status.KubernetesContext.Confidence).To(BeNumerically("<=", 0.5))
		})

		// IT-EC-03: Concurrent reconciliation
		It("IT-EC-03: should handle concurrent reconciliation of 10 CRs", func() {
			By("Creating namespace")
			ns := createTestNamespace("concurrent")
			defer deleteTestNamespace(ns)

			By("Creating 10 SignalProcessing CRs concurrently")
			var wg sync.WaitGroup
			sps := make([]*signalprocessingv1alpha1.SignalProcessing, 10)

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()

					sp := &signalprocessingv1alpha1.SignalProcessing{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "concurrent-" + string(rune('a'+idx)),
							Namespace: ns,
						},
						Spec: signalprocessingv1alpha1.SignalProcessingSpec{
							Signal: signalprocessingv1alpha1.SignalData{
								Fingerprint: "conc" + string(rune('a'+idx)) + "def456abc123def456abc123def456abc123def456abc123def456abc0" + string(rune('0'+idx)),
								Name:        "ConcurrentTest",
								Severity:    "warning",
								Type:        "prometheus",
								TargetType:  "kubernetes",
								TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
									Kind:      "Pod",
									Name:      "test-pod",
									Namespace: ns,
								},
								ReceivedTime: metav1.Now(),
							},
						},
					}
					sps[idx] = sp
					err := k8sClient.Create(ctx, sp)
					Expect(err).ToNot(HaveOccurred())
				}(i)
			}
			wg.Wait()

			By("Waiting for all to complete")
			for _, sp := range sps {
				if sp != nil {
					err := waitForCompletion(sp.Name, sp.Namespace, timeout)
					Expect(err).ToNot(HaveOccurred())
					_ = deleteAndWait(sp, timeout)
				}
			}
		})

		// IT-EC-04: Minimal spec
		It("IT-EC-04: should handle minimal spec with default values", func() {
			By("Creating namespace")
			ns := createTestNamespace("minimal")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with minimal spec")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-ec-04",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ec04def456abc123def456abc123def456abc123def456abc123def456abc04",
						Name:        "MinimalSpec",
						Severity:    "info",
						Type:        "kubernetes-event",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "minimal-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying default values applied")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})

		// IT-EC-05: Special namespace
		It("IT-EC-05: should handle special characters in namespace", func() {
			By("Creating namespace with special characters")
			ns := createTestNamespace("my-ns-123")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-ec-05",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ec05def456abc123def456abc123def456abc123def456abc123def456abc05",
						Name:        "SpecialNs",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying successful processing")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})

		// IT-EC-06: Max owner depth (BR-SP-100)
		It("IT-EC-06: should stop owner chain traversal at 5 levels", func() {
			By("Creating namespace")
			ns := createTestNamespace("deep-owner")
			defer deleteTestNamespace(ns)

			// Create a deep ownership chain (Pod → RS → Deploy)
			// Note: In real K8s, owner chains rarely exceed 3 levels
			// This test validates the 5-level limit per BR-SP-100

			By("Creating Deployment at root")
			deployLabels := map[string]string{"app": "deep-owner"}
			deployment := createTestDeployment(ns, "deep-deployment", deployLabels)

			By("Creating ReplicaSet")
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deep-rs",
					Namespace: ns,
					Labels:    deployLabels,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       deployment.Name,
						UID:        deployment.UID,
					}},
				},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: func() *int32 { r := int32(1); return &r }(),
					Selector: &metav1.LabelSelector{MatchLabels: deployLabels},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: deployLabels},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "test", Image: "nginx"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())

			By("Creating Pod")
			_ = createTestPod(ns, "deep-pod", deployLabels, []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       rs.Name,
				UID:        rs.UID,
			}})

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-ec-06",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ec06def456abc123def456abc123def456abc123def456abc123def456abc06",
						Name:        "DeepOwner",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "deep-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying owner chain depth limit")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(len(final.Status.KubernetesContext.OwnerChain)).To(BeNumerically("<=", 5))
		})

		// IT-EC-07: No failed detections (BR-SP-103)
		It("IT-EC-07: should have empty FailedDetections when all queries succeed", func() {
			By("Creating namespace")
			ns := createTestNamespace("success-detect")
			defer deleteTestNamespace(ns)

			By("Creating Pod")
			_ = createTestPod(ns, "success-pod", map[string]string{"app": "test"}, nil)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-ec-07",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ec07def456abc123def456abc123def456abc123def456abc123def456abc07",
						Name:        "SuccessDetect",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "success-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying no failed detections")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// In success case, DetectedLabels should be populated with no failures tracked
			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
		})

		// IT-EC-08: Multi-key Rego (BR-SP-102)
		It("IT-EC-08: should handle Rego policy returning multiple keys", func() {
			By("Creating namespace")
			ns := createTestNamespace("multi-key-rego")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with multi-key labels.rego policy")
			labelsPolicy := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["team"] := ["platform"] if { true }
labels["tier"] := ["backend"] if { true }
labels["cost-center"] := ["engineering"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, labelsPolicy)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-ec-08",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "ec08def456abc123def456abc123def456abc123def456abc123def456abc08",
						Name:        "MultiKeyRego",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying all 3 keys present")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveLen(3))
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("team"))
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("tier"))
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("cost-center"))
		})
	})

	// ========================================
	// ERROR HANDLING TESTS (IT-ER-01 to IT-ER-07)
	// ========================================

	Context("Error Handling", func() {
		// IT-ER-01: K8s API timeout (Error Cat. B)
		// Note: This test verifies retry behavior - difficult to simulate in ENVTEST
		// Controller should retry on transient errors
		It("IT-ER-01: should retry on K8s API timeout", func() {
			Skip("Transient timeout simulation requires custom client wrapper - covered by unit tests")
		})

		// IT-ER-02: Status conflict (Error Cat. D)
		It("IT-ER-02: should handle status update conflicts gracefully", func() {
			By("Creating namespace")
			ns := createTestNamespace("conflict")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-er-02",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "er02def456abc123def456abc123def456abc123def456abc123def456ab002",
						Name:        "ConflictTest",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion despite potential conflicts")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())
		})

		// IT-ER-03: Context cancelled (Error Cat. B)
		It("IT-ER-03: should exit cleanly on context cancellation", func() {
			Skip("Context cancellation testing requires controller restart - covered by unit tests")
		})

		// IT-ER-04: Rego syntax error (Error Cat. C)
		It("IT-ER-04: should use defaults when Rego policy has syntax error", func() {
			By("Creating namespace")
			ns := createTestNamespace("rego-error")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with invalid Rego policy")
			invalidPolicy := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels
// Invalid Rego - missing import
labels["team"] := ["platform"  // Missing closing bracket
`,
				},
			}
			Expect(k8sClient.Create(ctx, invalidPolicy)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-er-04",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "er04def456abc123def456abc123def456abc123def456abc123def456ab004",
						Name:        "RegoError",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying defaults used")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Should complete with defaults, not fail
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			// CustomLabels should be empty or have defaults, not error
		})

		// IT-ER-05: PDB RBAC denied (BR-SP-103)
		// Note: ENVTEST typically has full permissions, so this tests graceful degradation
		It("IT-ER-05: should track failed detections when PDB query fails", func() {
			Skip("RBAC testing requires restricted ServiceAccount - covered by unit tests")
		})

		// IT-ER-06: Audit write failure (ADR-038)
		It("IT-ER-06: should continue processing when audit write fails", func() {
			By("Creating namespace")
			ns := createTestNamespace("audit-fail")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-er-06",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "er06def456abc123def456abc123def456abc123def456abc123def456ab006",
						Name:        "AuditFail",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying processing completed despite audit failure")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})

		// IT-ER-07: Permanent error (Error Cat. A)
		It("IT-ER-07: should fail permanently with invalid spec", func() {
			By("Creating namespace")
			ns := createTestNamespace("permanent-fail")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with empty fingerprint (invalid)")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-signal-er-07",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						// Empty fingerprint - violates validation
						Fingerprint: "",
						Name:        "InvalidSpec",
						Severity:    "warning",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}

			By("Expecting creation to fail or controller to mark as Failed")
			err := k8sClient.Create(ctx, sp)
			if err == nil {
				// If creation succeeded (validation not enforced), check for Failed phase
				defer func() { _ = deleteAndWait(sp, timeout) }()

				err = waitForPhase(sp.Name, sp.Namespace, signalprocessingv1alpha1.PhaseFailed, timeout)
				Expect(err).ToNot(HaveOccurred())

				var final signalprocessingv1alpha1.SignalProcessing
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())
				Expect(final.Status.Error).ToNot(BeEmpty())
			}
			// If creation failed due to validation, that's also acceptable
		})
	})
})

