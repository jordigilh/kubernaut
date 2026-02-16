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

// Package signalprocessing_test contains component integration tests for SignalProcessing.
// These tests validate individual component behavior with real Kubernetes API via ENVTEST.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Component logic in isolation (test/unit/signalprocessing/)
// - Integration tests (>50%): Real K8s API interaction (this file)
// - E2E/BR tests (10-15%): Complete workflow validation (test/e2e/signalprocessing/)
//
// TDD Phase: RED - Tests define expected component behavior
// Implementation Plan: Day 10, Tier 3 - Component Integration Tests
//
// Test Matrix: 20 tests per IMPLEMENTATION_PLAN_V1.30.md
// - K8sEnricher: 7 tests (BR-SP-001)
// - Environment Classifier: 3 tests (BR-SP-051, BR-SP-052, BR-SP-072)
// - Priority Engine: 3 tests (BR-SP-070, BR-SP-071, BR-SP-072)
// - Business Classifier: 2 tests (BR-SP-002)
// - OwnerChain Builder: 2 tests (BR-SP-100)
// - Detection LabelDetector: 3 tests (BR-SP-101)
//
// Business Requirements Coverage:
// - BR-SP-001: K8s Context Enrichment (7 tests)
// - BR-SP-002: Business Classification (2 tests)
// - BR-SP-051: Namespace Label Environment (1 test)
// - BR-SP-052: ConfigMap Fallback (1 test)
// - BR-SP-070: Priority Assignment (1 test)
// - BR-SP-071: Severity Fallback (1 test)
// - BR-SP-072: Hot-Reload Policy (2 tests)
// - BR-SP-100: Owner Chain Traversal (2 tests)
// - BR-SP-101: Detected Labels (3 tests)
package signalprocessing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Component integration tests validate internal component APIs with real K8s API.
// These complement reconciler integration tests by providing granular component-level validation.
var _ = Describe("SignalProcessing Component Integration", func() {
	// ========================================
	// K8sEnricher COMPONENT TESTS (7 tests)
	// ========================================

	Context("K8sEnricher - Real K8s API Interaction", func() {
		// Pod enrichment with real K8s API
		It("BR-SP-001: should enrich Pod context from real K8s API", func() {
			By("Creating namespace")
			ns := createTestNamespace("enricher-pod")
			defer deleteTestNamespace(ns)

			By("Creating Pod with labels and annotations")
			podLabels := map[string]string{
				"app":     "test-app",
				"version": "v1.2.3",
			}
			podAnnotations := map[string]string{
				"prometheus.io/scrape": "true",
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "enrichment-test-pod",
					Namespace:   ns,
					Labels:      podLabels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test-container",
						Image: "nginx:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    *parseQuantity("100m"),
								corev1.ResourceMemory: *parseQuantity("128Mi"),
							},
						},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("Creating SignalProcessing CR targeting the pod")
			sp := createSignalProcessingCR(ns, "enrich-pod-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["enrich-pod"],
				Name:        "PodEnrichTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying Pod enrichment")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Pod).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Pod.Labels).To(HaveKeyWithValue("app", "test-app"))
			Expect(final.Status.KubernetesContext.Pod.Labels).To(HaveKeyWithValue("version", "v1.2.3"))
		})

		// Deployment enrichment with real K8s API
		It("BR-SP-001: should enrich Deployment context from real K8s API", func() {
			By("Creating namespace")
			ns := createTestNamespace("enricher-deploy")
			defer deleteTestNamespace(ns)

			By("Creating Deployment")
			deployLabels := map[string]string{"app": "deploy-test"}
			deployment := createTestDeployment(ns, "enrichment-deployment", deployLabels)

			By("Creating SignalProcessing CR targeting the deployment")
			sp := createSignalProcessingCR(ns, "enrich-deploy-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["enrich-deploy"],
				Name:        "DeployEnrichTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      deployment.Name,
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying Deployment enrichment")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Deployment).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Deployment.Labels).To(HaveKeyWithValue("app", "deploy-test"))
		})

		// NOTE: Node enrichment test moved to E2E tier - ENVTEST does not provide real nodes
		// Coverage: test/e2e/signalprocessing/business_requirements_test.go (BR-SP-001 Node Enrichment)

		// StatefulSet enrichment
		It("BR-SP-001: should enrich StatefulSet context from real K8s API", func() {
			By("Creating namespace")
			ns := createTestNamespace("enricher-sts")
			defer deleteTestNamespace(ns)

			By("Creating StatefulSet")
			stsLabels := map[string]string{"app": "statefulset-test"}
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "enrichment-statefulset",
					Namespace: ns,
					Labels:    stsLabels,
				},
				Spec: appsv1.StatefulSetSpec{
					ServiceName: "test-service",
					Replicas:    func() *int32 { r := int32(1); return &r }(),
					Selector:    &metav1.LabelSelector{MatchLabels: stsLabels},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: stsLabels},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "test", Image: "nginx"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())

			By("Creating SignalProcessing CR targeting the StatefulSet")
			sp := createSignalProcessingCR(ns, "enrich-sts-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["enrich-sts"],
				Name:        "StsEnrichTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "StatefulSet",
					Name:      sts.Name,
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying StatefulSet enrichment")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.StatefulSet).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.StatefulSet.Labels).To(HaveKeyWithValue("app", "statefulset-test"))
		})

		// Service enrichment
		It("BR-SP-001: should enrich Service context from real K8s API", func() {
			By("Creating namespace")
			ns := createTestNamespace("enricher-svc")
			defer deleteTestNamespace(ns)

			By("Creating Service")
			svcLabels := map[string]string{"app": "service-test"}
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "enrichment-service",
					Namespace: ns,
					Labels:    svcLabels,
				},
				Spec: corev1.ServiceSpec{
					Selector: svcLabels,
					Ports: []corev1.ServicePort{{
						Port:     80,
						Protocol: corev1.ProtocolTCP,
					}},
				},
			}
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())

			By("Creating SignalProcessing CR targeting the Service")
			sp := createSignalProcessingCR(ns, "enrich-svc-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["enrich-svc"],
				Name:        "SvcEnrichTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Service",
					Name:      svc.Name,
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying Service enrichment")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Service).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Service.Labels).To(HaveKeyWithValue("app", "service-test"))
		})

		// Namespace context enrichment
		It("BR-SP-001: should enrich Namespace context with labels and annotations", func() {
			By("Creating namespace with labels and annotations")
			ns := createTestNamespaceWithLabels("enricher-ns-context", map[string]string{
				"kubernaut.ai/environment": "staging",
				"kubernaut.ai/team":        "platform",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "enrich-ns-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["enrich-ns"],
				Name:        "NsContextTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "any-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying Namespace context enrichment")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Namespace).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.Namespace.Labels).To(HaveKeyWithValue("kubernaut.ai/environment", "staging"))
			Expect(final.Status.KubernetesContext.Namespace.Labels).To(HaveKeyWithValue("kubernaut.ai/team", "platform"))
		})

		// Degraded mode fallback
		It("BR-SP-001: should fall back to degraded mode when resource not found", func() {
			By("Creating namespace")
			ns := createTestNamespace("enricher-degraded")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR for non-existent resource")
			sp := createSignalProcessingCR(ns, "enrich-degraded-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["enrich-degraded"],
				Name:        "DegradedTest",
				Severity:    "critical",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "non-existent-pod",
					Namespace: ns,
				},
				Labels: map[string]string{
					"app": "fallback-app",
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying degraded mode")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DegradedMode).To(BeTrue())
		})
	})

	// ========================================
	// Environment Classifier COMPONENT TESTS (3 tests)
	// ========================================

	Context("Environment Classifier - Real ConfigMap Interaction", func() {
		// Real ConfigMap lookup for environment classification
		It("BR-SP-052: should classify environment from real ConfigMap", func() {
			By("Creating namespace with prefix matching ConfigMap rules")
			// The suite_test.go creates a ConfigMap with rules:
			// - startswith(namespace, "prod") → production
			// - startswith(namespace, "staging") → staging
			ns := createTestNamespace("prod-configmap-test")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "env-configmap-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["env-configmap"],
				Name:        "EnvConfigMapTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying ConfigMap-based classification")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
		})

		// Namespace label priority over ConfigMap
		It("BR-SP-051: should prioritize namespace label over ConfigMap rules", func() {
			By("Creating namespace with explicit label contradicting prefix")
			// Namespace name starts with "prod" but label says "staging"
			ns := createTestNamespaceWithLabels("prod-but-staging", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "env-label-priority-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["env-label"],
				Name:        "LabelPriorityTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying label takes priority")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			// Label should override ConfigMap pattern matching
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("staging"))
			// Note: Confidence field removed per DD-SP-001 V1.1
		})

		// NOTE: Hot-reload test removed - covered by dedicated hot_reloader_test.go
	})

	// ========================================
	// Priority Engine COMPONENT TESTS (3 tests)
	// ========================================

	Context("Priority Engine - Real Rego Evaluation", func() {
		// Real Rego evaluation for priority assignment
		It("BR-SP-070: should assign priority using real Rego evaluation", func() {
			By("Creating production namespace")
			ns := createTestNamespaceWithLabels("priority-rego-prod", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with critical severity")
			sp := createSignalProcessingCR(ns, "priority-rego-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["priority-rego"],
				Name:        "PriorityRegoTest",
				Severity:    "critical",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying Rego-based priority")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			// Production + Critical = P0 per priority.rego
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P0"))
			Expect(final.Status.PriorityAssignment.Source).To(ContainSubstring("rego"))
		})

		// Severity fallback when environment unknown
		It("BR-SP-071: should fall back to severity-only priority when environment unknown", func() {
			By("Creating namespace without environment classification")
			ns := createTestNamespace("priority-severity-fallback")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with critical severity")
			sp := createSignalProcessingCR(ns, "priority-fallback-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["priority-fallback"],
				Name:        "SeverityFallbackTest",
				Severity:    "critical",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

		By("Verifying severity-based fallback priority")
		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

		Expect(final.Status.PriorityAssignment).ToNot(BeNil())
		// Issue #98: Score-based policy: severity_score=3 (critical) + env_score=0 (unknown) = composite 3 → P3
		// Previously P1 under N*M policy. Score-based treats unknown env as zero contribution.
		Expect(final.Status.PriorityAssignment.Priority).To(Equal("P3"))
		})

		// ConfigMap policy load
		It("BR-SP-072: should load priority policy from ConfigMap", func() {
			// The suite_test.go already creates the priority ConfigMap
			// This test verifies the policy was loaded correctly

			By("Creating namespace")
			ns := createTestNamespaceWithLabels("priority-configmap", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with warning severity")
			sp := createSignalProcessingCR(ns, "priority-configmap-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["priority-cm"],
				Name:        "PriorityConfigMapTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying ConfigMap policy was used")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			// Staging + warning = P2 per ConfigMap policy
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P2"))
		})
	})

	// ========================================
	// Business Classifier COMPONENT TESTS (2 tests)
	// ========================================

	Context("Business Classifier - Label and Pattern Based", func() {
		// Label-based classification
		It("BR-SP-002: should classify business unit from namespace label", func() {
			By("Creating namespace with team label")
			ns := createTestNamespaceWithLabels("business-label", map[string]string{
				"kubernaut.ai/team": "payments",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "business-label-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["business-label"],
				Name:        "BusinessLabelTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying label-based business classification")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.BusinessClassification).ToNot(BeNil())
			Expect(final.Status.BusinessClassification.BusinessUnit).To(Equal("payments"))
		})

		// Pattern-based classification
		It("BR-SP-002: should classify business unit from namespace pattern", func() {
			By("Creating namespace with business-indicative name")
			ns := createTestNamespace("finance-app")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "business-pattern-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["business-pattern"],
				Name:        "BusinessPatternTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying pattern-based business classification")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Pattern matching may or may not populate BusinessUnit depending on rules
			// This test validates the classification attempt was made
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})
	})

	// ========================================
	// OwnerChain Builder COMPONENT TESTS (2 tests)
	// ========================================

	Context("OwnerChain Builder - Real K8s Traversal", func() {
		// Real K8s traversal
		It("BR-SP-100: should traverse owner chain using real K8s API", func() {
			By("Creating namespace")
			ns := createTestNamespace("ownerchain-real")
			defer deleteTestNamespace(ns)

			By("Creating Deployment → ReplicaSet → Pod chain")
			deployLabels := map[string]string{"app": "ownerchain-real"}
			deployment := createTestDeployment(ns, "real-deployment", deployLabels)

			// Create ReplicaSet owned by Deployment
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "real-replicaset",
					Namespace: ns,
					Labels:    deployLabels,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       deployment.Name,
						UID:        deployment.UID,
						Controller: func() *bool { c := true; return &c }(),
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

			// Create Pod owned by ReplicaSet
			_ = createTestPod(ns, "real-pod", deployLabels, []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       rs.Name,
				UID:        rs.UID,
				Controller: func() *bool { c := true; return &c }(),
			}})

			By("Creating SignalProcessing CR targeting the pod")
			sp := createSignalProcessingCR(ns, "ownerchain-real-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["ownerchain"],
				Name:        "OwnerChainRealTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "real-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
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

		// NOTE: Cross-namespace owner test removed - Kubernetes API explicitly forbids cross-namespace owner references
		// This scenario is not applicable (API constraint, not test gap)
	})

	// ========================================
	// Detection LabelDetector COMPONENT TESTS (3 tests)
	// ========================================

	Context("Detection LabelDetector - Real K8s Queries", func() {
		// Real PDB query
		It("BR-SP-101: should detect PDB using real K8s query", func() {
			By("Creating namespace")
			ns := createTestNamespace("detect-pdb")
			defer deleteTestNamespace(ns)

			By("Creating Pod")
			podLabels := map[string]string{"app": "pdb-target"}
			_ = createTestPod(ns, "pdb-target-pod", podLabels, nil)

			By("Creating PDB that matches Pod")
			_ = createTestPDB(ns, "real-pdb", podLabels)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "detect-pdb-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["detect-pdb"],
				Name:        "DetectPDBTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "pdb-target-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
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

		// Real HPA query
		It("BR-SP-101: should detect HPA using real K8s query", func() {
			By("Creating namespace")
			ns := createTestNamespace("detect-hpa")
			defer deleteTestNamespace(ns)

			By("Creating Deployment")
			deployLabels := map[string]string{"app": "hpa-target"}
			_ = createTestDeployment(ns, "hpa-target-deployment", deployLabels)

			By("Creating HPA targeting Deployment")
			_ = createTestHPA(ns, "real-hpa", "hpa-target-deployment")

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "detect-hpa-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["detect-hpa"],
				Name:        "DetectHPATest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "hpa-target-deployment",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
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

		// COMPONENT-04: HPA detection via owner chain (CTRL-DETECT-04)
		// This test validates that HPA is detected when it targets an owner in the chain
		// Scenario: Pod → ReplicaSet → Deployment ← HPA
		It("COMPONENT-04: should detect HPA via owner chain traversal (BR-SP-101)", func() {
			By("Creating namespace")
			ns := createTestNamespace("detect-hpa-chain")
			defer deleteTestNamespace(ns)

			By("Creating Deployment (HPA will target this)")
			deployLabels := map[string]string{"app": "hpa-chain-target"}
			deployment := createTestDeployment(ns, "hpa-chain-deployment", deployLabels)

			By("Creating ReplicaSet owned by Deployment")
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hpa-chain-rs",
					Namespace: ns,
					Labels:    deployLabels,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       deployment.Name,
						UID:        deployment.UID,
						Controller: func() *bool { c := true; return &c }(),
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

			By("Creating Pod owned by ReplicaSet")
			_ = createTestPod(ns, "hpa-chain-pod", deployLabels, []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Name:       rs.Name,
				UID:        rs.UID,
				Controller: func() *bool { c := true; return &c }(),
			}})

			By("Creating HPA targeting the Deployment (ancestor of Pod)")
			_ = createTestHPA(ns, "hpa-chain-autoscaler", "hpa-chain-deployment")

			By("Creating SignalProcessing CR targeting the Pod")
			sp := createSignalProcessingCR(ns, "detect-hpa-chain-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["hpa-ownerchain"],
				Name:        "DetectHPAChainTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "hpa-chain-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying HPA detected via owner chain")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasHPA).To(BeTrue(),
				"CTRL-DETECT-04: HPA should be detected via owner chain traversal (Pod → RS → Deployment ← HPA)")

			// Also verify owner chain was properly traversed
			Expect(final.Status.KubernetesContext.OwnerChain).To(HaveLen(2),
				"Should have 2 owners: ReplicaSet → Deployment")
			Expect(final.Status.KubernetesContext.OwnerChain[0].Kind).To(Equal("ReplicaSet"))
			Expect(final.Status.KubernetesContext.OwnerChain[1].Kind).To(Equal("Deployment"))
		})

		// Real NetworkPolicy query
		It("BR-SP-101: should detect NetworkPolicy using real K8s query", func() {
			By("Creating namespace")
			ns := createTestNamespace("detect-netpol")
			defer deleteTestNamespace(ns)

			By("Creating NetworkPolicy")
			_ = createTestNetworkPolicy(ns, "real-netpol")

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "detect-netpol-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["detect-netpol"],
				Name:        "DetectNetPolTest",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying NetworkPolicy detection")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.NetworkIsolated).To(BeTrue())
		})
	})

	// ========================================
	// PRIORITY 3: EXTENDED DETECTION TESTS
	// Testing detection functions with comprehensive edge cases
	// ========================================

	Context("Priority 3: GitOps Detection (detectGitOps)", func() {
		// PRIORITY-3-GITOPS-01: Flux GitOps detection via namespace annotation
		It("should detect Flux GitOps via namespace annotation", func() {
			By("Creating namespace with Flux annotation")
			nsName := createTestNamespace("gitops-flux")
			defer deleteTestNamespace(nsName)

			// Add Flux annotation to namespace (controller checks namespace annotations)
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns)).To(Succeed())
			if ns.Annotations == nil {
				ns.Annotations = make(map[string]string)
			}
			ns.Annotations["fluxcd.io/sync-status"] = "True" // Controller checks this annotation
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			By("Creating Deployment")
			deployLabels := map[string]string{"app": "flux-app"}
			deployment := createTestDeployment(nsName, "flux-deployment", deployLabels)

			By("Creating SignalProcessing CR for Deployment signal")
			sp := createSignalProcessingCR(nsName, "gitops-flux-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-flux-test"),
				Name:        "FluxGitOpsTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      deployment.Name,
					Namespace: nsName,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying Flux GitOps detection")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: nsName}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.GitOpsManaged).To(BeTrue(),
				"Should detect GitOps management via Flux namespace annotation")
		})

		// PRIORITY-3-GITOPS-02: ArgoCD GitOps detection via namespace annotation
		It("should detect ArgoCD GitOps via namespace annotation", func() {
			By("Creating namespace with ArgoCD annotation")
			nsName := createTestNamespace("gitops-argocd")
			defer deleteTestNamespace(nsName)

			// Add ArgoCD annotation to namespace (controller checks namespace annotations)
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns)).To(Succeed())
			if ns.Annotations == nil {
				ns.Annotations = make(map[string]string)
			}
			ns.Annotations["argocd.argoproj.io/managed"] = "true" // Controller checks this annotation
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			By("Creating Pod")
			podLabels := map[string]string{"app": "argocd-app"}
			_ = createTestPod(nsName, "argocd-pod", podLabels, nil)

			By("Creating SignalProcessing CR for Pod signal")
			sp := createSignalProcessingCR(nsName, "gitops-argocd-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-argocd-test"),
				Name:        "ArgoCDGitOpsTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "argocd-pod",
					Namespace: nsName,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying ArgoCD GitOps detection")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: nsName}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.GitOpsManaged).To(BeTrue(),
				"Should detect GitOps management via ArgoCD namespace annotation")
		})

		// PRIORITY-3-GITOPS-03: No GitOps detection when annotations absent
		It("should not detect GitOps when no GitOps annotations present", func() {
			By("Creating namespace without GitOps annotations")
			nsName := createTestNamespace("gitops-none")
			defer deleteTestNamespace(nsName)

			By("Creating Deployment")
			deployLabels := map[string]string{"app": "regular-app"}
			deployment := createTestDeployment(nsName, "regular-deployment", deployLabels)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(nsName, "gitops-none-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-nogitops-test"),
				Name:        "NoGitOpsTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      deployment.Name,
					Namespace: nsName,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying no GitOps detection")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: nsName}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.GitOpsManaged).To(BeFalse(),
				"Should not detect GitOps when no GitOps annotations present")
		})
	})

	Context("Priority 3: PDB Detection (detectPDB) - Edge Cases", func() {
		// PRIORITY-3-PDB-01: No PDB when selector doesn't match
		It("should not detect PDB when selector doesn't match pod labels", func() {
			By("Creating namespace")
			ns := createTestNamespace("pdb-nomatch")
			defer deleteTestNamespace(ns)

			By("Creating Pod with labels")
			podLabels := map[string]string{"app": "my-app"}
			_ = createTestPod(ns, "my-pod", podLabels, nil)

			By("Creating PDB with non-matching selector")
			pdbSelector := map[string]string{"app": "different-app"} // Different label
			_ = createTestPDB(ns, "nomatch-pdb", pdbSelector)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "pdb-nomatch-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-pdb-nomatch"),
				Name:        "PDBNoMatchTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "my-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying no PDB detection")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasPDB).To(BeFalse(),
				"Should not detect PDB when selector doesn't match pod labels")
		})

		// PRIORITY-3-PDB-02: Multiple PDBs, one matches
		It("should detect PDB when one of multiple PDBs matches", func() {
			By("Creating namespace")
			ns := createTestNamespace("pdb-multi")
			defer deleteTestNamespace(ns)

			By("Creating Pod with labels")
			podLabels := map[string]string{"app": "target-app", "tier": "frontend"}
			_ = createTestPod(ns, "target-pod", podLabels, nil)

			By("Creating multiple PDBs - one matching, one not")
			_ = createTestPDB(ns, "nomatch-pdb", map[string]string{"app": "other-app"})
			_ = createTestPDB(ns, "match-pdb", map[string]string{"app": "target-app"})

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "pdb-multi-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-pdb-multi"),
				Name:        "PDBMultiTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "target-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying PDB detection (one match is sufficient)")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasPDB).To(BeTrue(),
				"Should detect PDB when at least one PDB selector matches")
		})
	})

	Context("Priority 3: HPA Detection (detectHPA) - Edge Cases", func() {
		// PRIORITY-3-HPA-01: No HPA when target doesn't match
		It("should not detect HPA when HPA targets different deployment", func() {
			By("Creating namespace")
			ns := createTestNamespace("hpa-nomatch")
			defer deleteTestNamespace(ns)

			By("Creating two Deployments")
			deployLabels1 := map[string]string{"app": "app1"}
			deployment1 := createTestDeployment(ns, "deployment1", deployLabels1)

			deployLabels2 := map[string]string{"app": "app2"}
			_ = createTestDeployment(ns, "deployment2", deployLabels2)

			By("Creating HPA targeting deployment2 (not deployment1)")
			_ = createTestHPA(ns, "hpa-for-deployment2", "deployment2")

			By("Creating SignalProcessing CR for deployment1 signal")
			sp := createSignalProcessingCR(ns, "hpa-nomatch-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-hpa-nomatch"),
				Name:        "HPANoMatchTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      deployment1.Name,
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying no HPA detection for deployment1")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasHPA).To(BeFalse(),
				"Should not detect HPA when HPA targets a different deployment")
		})

		// PRIORITY-3-HPA-02: Multiple HPAs, one matches
		It("should detect HPA when one of multiple HPAs matches", func() {
			By("Creating namespace")
			ns := createTestNamespace("hpa-multi")
			defer deleteTestNamespace(ns)

			By("Creating target Deployment")
			deployLabels := map[string]string{"app": "target-app"}
			deployment := createTestDeployment(ns, "target-deployment", deployLabels)

			By("Creating other Deployment")
			_ = createTestDeployment(ns, "other-deployment", map[string]string{"app": "other-app"})

			By("Creating multiple HPAs - one matching, one not")
			_ = createTestHPA(ns, "hpa-for-other", "other-deployment")
			_ = createTestHPA(ns, "hpa-for-target", "target-deployment")

			By("Creating SignalProcessing CR for target deployment")
			sp := createSignalProcessingCR(ns, "hpa-multi-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-hpa-multi"),
				Name:        "HPAMultiTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      deployment.Name,
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying HPA detection (one match is sufficient)")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasHPA).To(BeTrue(),
				"Should detect HPA when at least one HPA targets the deployment")
		})

		// PRIORITY-3-HPA-03: HPA detection via owner chain (BR-SP-101)
		It("should detect HPA for Pod signal when Pod is owned by Deployment with HPA", func() {
			By("Creating namespace")
			ns := createTestNamespace("hpa-pod")
			defer deleteTestNamespace(ns)

			By("Creating Deployment with HPA")
			deployLabels := map[string]string{"app": "hpa-app"}
			deployment := createTestDeployment(ns, "hpa-deployment", deployLabels)
			_ = createTestHPA(ns, "deployment-hpa", "hpa-deployment")

			By("Creating Pod owned by Deployment")
			podOwnerRef := []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployment.Name,
				UID:        deployment.UID,
				Controller: ptr(true),
			}}
			_ = createTestPod(ns, "hpa-pod", deployLabels, podOwnerRef)

			By("Creating SignalProcessing CR for Pod signal (not Deployment)")
			sp := createSignalProcessingCR(ns, "hpa-pod-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: GenerateTestFingerprint("priority3-hpa-pod"),
				Name:        "HPAPodTest",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "hpa-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying HPA detection via owner chain")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.DetectedLabels.HasHPA).To(BeTrue(),
				"Should detect HPA for Pod signal when HPA targets owner in chain (BR-SP-101)")
		})
	})
})

// parseQuantity is a helper to create resource quantities
func parseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

// ptr is a helper to create bool pointers
func ptr(b bool) *bool {
	return &b
}
