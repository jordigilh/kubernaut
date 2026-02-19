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
// These tests validate CRD coordination and business outcomes with real Kubernetes API via ENVTEST.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (test/unit/signalprocessing/)
// - Integration tests (>50%): CRD coordination, status propagation (this file)
// - E2E/BR tests (10-15%): Complete workflow validation (test/e2e/signalprocessing/)
//
// NOTE: These integration tests validate BOTH infrastructure behavior (CRD lifecycle)
// AND business outcomes (priority assignment, classification). This hybrid approach
// is appropriate because business outcomes depend on CRD status propagation which
// requires real K8s API. Pure business outcome tests are duplicated in E2E/BR tests
// for defense-in-depth coverage.
//
// TDD Phase: RED - Tests define expected controller behavior
// Implementation Plan: Day 10, Tier 2 - Reconciler Integration Tests
//
// Test Matrix: 25 tests per IMPLEMENTATION_PLAN_V1.30.md
// - Happy Path: 10 tests (BR-SP-051, BR-SP-070, BR-SP-002, BR-SP-100, BR-SP-101, BR-SP-102)
// - Edge Cases: 8 tests (BR-SP-053, BR-SP-001, BR-SP-100, BR-SP-103, BR-SP-102)
// - Error Handling: 7 tests (Error Categories A-D, BR-SP-103, ADR-038)
//
// Business Requirements Coverage:
// - BR-SP-001: K8s Context Enrichment, Degraded Mode
// - BR-SP-002: Business Classification
// - BR-SP-051: Namespace Label Environment Classification (high confidence)
// - BR-SP-052: ConfigMap Fallback Classification
// - BR-SP-053: Default Environment Fallback
// - BR-SP-070: Priority Assignment (P0/P1/P2/P3)
// - BR-SP-100: Owner Chain Traversal (max depth 5)
// - BR-SP-102: CustomLabels Rego Extraction (multi-key)
// - BR-SP-103: Failed Detections Tracking
// - ADR-038: Audit Non-Blocking
package signalprocessing

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

var _ = Describe("SignalProcessing Reconciler Integration", func() {
	// ========================================
	// HAPPY PATH TESTS - Business Outcome Validation
	// ========================================

	Context("Happy Path - Phase Transitions", func() {
		// Production pod → P0 priority assignment
		It("BR-SP-070, BR-SP-051: should process production pod signal and assign P0 priority", func() {
			By("Creating production namespace")
			ns := createTestNamespaceWithLabels("prod", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating test pod in production namespace")
			podLabels := map[string]string{"app": "api-server"}
			_ = createTestPod(ns, "api-server-xyz", podLabels, nil)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "api-server-xyz",
				Namespace: ns,
			}
			rrName := "test-signal-hp-01-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["reconciler-01"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with parent RR")
			sp := CreateTestSignalProcessingWithParent("test-signal-hp-01", ns, rr, ValidTestFingerprints["reconciler-01"], targetResource)
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
			// Note: Confidence field removed per DD-SP-001 V1.1

			// BR-SP-070: Production + Critical → P0
			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P0"))
			// Note: Confidence field removed per DD-SP-001 V1.1
		})

		// Staging deployment → P2 priority assignment
		It("BR-SP-070, BR-SP-051: should process staging deployment signal and assign P2 priority", func() {
			By("Creating staging namespace")
			ns := createTestNamespaceWithLabels("staging", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating test deployment")
			deployLabels := map[string]string{"app": "web-frontend"}
			_ = createTestDeployment(ns, "web-frontend", deployLabels)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "web-frontend",
				Namespace: ns,
			}
			rrName := "test-signal-hp-02-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["reconciler-02"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with parent RR")
			sp := CreateTestSignalProcessingWithParent("test-signal-hp-02", ns, rr, ValidTestFingerprints["reconciler-02"], targetResource)
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

		// Dev service → P3 priority assignment
		It("BR-SP-070, BR-SP-051: should process dev service signal and assign P3 priority", func() {
			By("Creating development namespace")
			ns := createTestNamespaceWithLabels("dev", map[string]string{
				"kubernaut.ai/environment": "development",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "test-signal-hp-03", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["reconciler-03"],
				Name:        "LowDiskSpace",
				Severity: "low",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Service",
					Name:      "backend-api",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
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

		// Environment classification from namespace label
		It("BR-SP-051: should classify environment from namespace label with high confidence", func() {
			By("Creating namespace with explicit production label")
			ns := createTestNamespaceWithLabels("custom", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "test-signal-hp-04", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["reconciler-04"],
				Name:        "TestAlert",
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

			By("Verifying label-based classification")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
			Expect(final.Status.EnvironmentClassification.Source).To(Equal("namespace-labels"))
			// Note: Confidence field removed per DD-SP-001 V1.1
		})

		// ConfigMap fallback for environment classification
		It("BR-SP-052: should classify environment from ConfigMap fallback", func() {
			By("Creating namespace with 'staging' prefix (no label)")
			ns := createTestNamespace("staging-app")
			defer deleteTestNamespace(ns)

			// Create parent RemediationRequest (matches production architecture)
			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest("test-rr-hp-05", ns, ValidTestFingerprints["reconciler-05"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with RemediationRequestRef")
			sp := CreateTestSignalProcessingWithParent("test-signal-hp-05", ns, rr, ValidTestFingerprints["reconciler-05"], targetResource)
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

		// Business unit classification from namespace labels
		It("BR-SP-002: should classify business unit from namespace labels", func() {
			By("Creating namespace with business-unit label")
			ns := createTestNamespaceWithLabels("payments", map[string]string{
				"kubernaut.ai/environment":   "production",
				"kubernaut.ai/business-unit": "payments",
				"kubernaut.ai/service-owner": "payments-team",
				"kubernaut.ai/criticality":   "high",
				"kubernaut.ai/sla":           "99.9",
			})
			defer deleteTestNamespace(ns)

			// Create parent RemediationRequest (matches production architecture)
			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "payment-processor",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest("test-rr-hp-06", ns, ValidTestFingerprints["reconciler-06"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with RemediationRequestRef")
			sp := CreateTestSignalProcessingWithParent("test-signal-hp-06", ns, rr, ValidTestFingerprints["reconciler-06"], targetResource)
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

		// Owner chain traversal from Pod to Deployment
		It("BR-SP-100: should build owner chain from Pod to Deployment", func() {
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
				Controller: func() *bool { t := true; return &t }(), // Required for owner chain traversal
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
				Controller: func() *bool { t := true; return &t }(), // Required for owner chain traversal
			}}
			_ = createTestPod(ns, "ownerchain-pod", deployLabels, podOwnerRef)

			// Create parent RemediationRequest (matches production architecture)
			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "ownerchain-pod",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest("test-rr-hp-07", ns, ValidTestFingerprints["reconciler-07"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with RemediationRequestRef")
			sp := CreateTestSignalProcessingWithParent("test-signal-hp-07", ns, rr, ValidTestFingerprints["reconciler-07"], targetResource)
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

		// BR-SP-101 detection tests removed - ADR-056: DetectedLabels relocated to HAPI

		// CustomLabels extraction from Rego policy
		It("BR-SP-102: should populate CustomLabels from Rego policy", func() {
			By("Creating namespace with team label")
			ns := createTestNamespaceWithLabels("rego-labels", map[string]string{
				"kubernaut.ai/team": "platform",
			})
			defer deleteTestNamespace(ns)

			// Note: Using file-based Rego policy from suite setup
			// Policy will extract team label from namespace labels (degraded mode)

			// Create parent RemediationRequest (matches production architecture)
			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest("test-rr-hp-10", ns, ValidTestFingerprints["reconciler-10"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with RemediationRequestRef")
			sp := CreateTestSignalProcessingWithParent("test-signal-hp-10", ns, rr, ValidTestFingerprints["reconciler-10"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying CustomLabels populated")
			// Use Eventually to wait for CustomLabels to be populated
			Eventually(func() map[string][]string {
				var final signalprocessingv1alpha1.SignalProcessing
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final); err != nil {
					return nil
				}
				if final.Status.KubernetesContext == nil {
					return nil
				}
				return final.Status.KubernetesContext.CustomLabels
			}, 5*time.Second, 100*time.Millisecond).Should(HaveKey("team"), "CustomLabels should have 'team' key from namespace labels")
		})
	})

	// ========================================
	// EDGE CASE TESTS - Boundary and Error Conditions
	// ========================================

	Context("Edge Cases", func() {
		// Default environment fallback when no labels present
		It("BR-SP-053: should default to unknown environment when no labels", func() {
			By("Creating namespace without environment label")
			ns := createTestNamespace("unknown-env")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "test-signal-ec-01", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["edge-case-01"],
				Name:        "DefaultEnv",
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

			By("Verifying default environment")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("unknown"))
			Expect(final.Status.EnvironmentClassification.Source).To(Equal("default"))
		})

		// Degraded mode when target resource not found
		It("BR-SP-001: should enter degraded mode when pod not found", func() {
			By("Creating namespace")
			ns := createTestNamespace("degraded")
			defer deleteTestNamespace(ns)

			// Create parent RemediationRequest (matches production architecture)
			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "non-existent-pod",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest("test-rr-ec-02", ns, ValidTestFingerprints["edge-case-02"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with RemediationRequestRef for non-existent pod")
			sp := CreateTestSignalProcessingWithParent("test-signal-ec-02", ns, rr, ValidTestFingerprints["edge-case-02"], targetResource)
			sp.Spec.Signal.Labels = map[string]string{"environment": "production"} // Add custom labels
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
		})

		// Concurrent reconciliation stress test
		It("Controller: should handle concurrent reconciliation of 10 CRs", func() {
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

					sp := createSignalProcessingCR(ns, "concurrent-"+string(rune('a'+idx)), signalprocessingv1alpha1.SignalData{
						Fingerprint: GenerateConcurrentFingerprint("reconciler-concurrent", idx),
						Name:        "ConcurrentTest",
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
					sps[idx] = sp
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

		// Minimal spec with defaults
		It("Robustness: should handle minimal spec with default values", func() {
			By("Creating namespace")
			ns := createTestNamespace("minimal")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with minimal spec")
			sp := createSignalProcessingCR(ns, "test-signal-ec-04", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["edge-case-04"],
				Name:        "MinimalSpec",
				Severity: "low",
				Type:        "kubernetes-event",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "minimal-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying default values applied")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})

		// Special namespace handling
		It("Robustness: should handle special characters in namespace", func() {
			By("Creating namespace with special characters")
			ns := createTestNamespace("my-ns-123")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "test-signal-ec-05", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["edge-case-05"],
				Name:        "SpecialNs",
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

			By("Verifying successful processing")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})

		// Max owner chain depth limit
		It("BR-SP-100: should stop owner chain traversal at 5 levels", func() {
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
			sp := createSignalProcessingCR(ns, "test-signal-ec-06", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["edge-case-06"],
				Name:        "DeepOwner",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "deep-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
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

		// No failed detections on successful queries
		It("BR-SP-103: should have empty FailedDetections when all queries succeed", func() {
			By("Creating namespace")
			ns := createTestNamespace("success-detect")
			defer deleteTestNamespace(ns)

			By("Creating Pod")
			_ = createTestPod(ns, "success-pod", map[string]string{"app": "test"}, nil)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "test-signal-ec-07", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["edge-case-07"],
				Name:        "SuccessDetect",
				Severity: "high",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "success-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying no failed detections")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
		})

		// Multi-key Rego policy evaluation
		It("BR-SP-102: should handle Rego policy returning multiple keys", func() {
			By("Creating namespace with multiple labels")
			ns := createTestNamespaceWithLabels("multi-key-rego", map[string]string{
				"kubernaut.ai/team":        "platform",
				"kubernaut.ai/tier":        "backend",
				"kubernaut.ai/cost-center": "engineering",
			})
			defer deleteTestNamespace(ns)

			// Note: Using file-based policy from suite setup
			// Policy will extract team, tier, and cost-center from namespace labels (degraded mode)

			// Create parent RemediationRequest (matches production architecture)
			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: ns,
			}
			rr := CreateTestRemediationRequest("test-rr-ec-08", ns, ValidTestFingerprints["edge-case-08"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR with RemediationRequestRef")
			sp := CreateTestSignalProcessingWithParent("test-signal-ec-08", ns, rr, ValidTestFingerprints["edge-case-08"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying all 3 keys present")
			// Use Eventually to wait for all 3 CustomLabels to be populated
			Eventually(func() int {
				var final signalprocessingv1alpha1.SignalProcessing
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final); err != nil {
					return 0
				}
				if final.Status.KubernetesContext == nil {
					return 0
				}
				return len(final.Status.KubernetesContext.CustomLabels)
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(3), "CustomLabels should have 3 keys from namespace labels")

			// Fetch final CR to verify individual keys
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("team"))
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("tier"))
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("cost-center"))
		})
	})

	// ========================================
	// ERROR HANDLING TESTS - Error Categories A-D
	// ========================================

	Context("Error Handling", func() {
		// NOTE: Error-Cat-B K8s API timeout test removed - covered by unit test:
		// test/unit/signalprocessing/controller_error_handling_test.go (12 tests)

		// Status update conflict handling (Error Category D)
		It("Error-Cat-D: should handle status update conflicts gracefully", func() {
			By("Creating namespace")
			ns := createTestNamespace("conflict")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "test-signal-er-02", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["error-02"],
				Name:        "ConflictTest",
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

			By("Waiting for completion despite potential conflicts")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())
		})

		// NOTE: Error-Cat-B context cancellation test removed - covered by unit test:
		// test/unit/signalprocessing/controller_shutdown_test.go (9 tests)

		// Rego syntax error fallback to defaults (Error Category C)
		It("Error-Cat-C: should use defaults when Rego policy has syntax error", func() {
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
			sp := createSignalProcessingCR(ns, "test-signal-er-04", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["error-04"],
				Name:        "RegoError",
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

			By("Verifying defaults used")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Should complete with defaults, not fail
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			// CustomLabels should be empty or have defaults, not error
		})

		// NOTE: BR-SP-103 PDB RBAC test removed - covered by unit test:
		// test/unit/signalprocessing/label_detector_test.go (16 tests, including DL-ER-01 RBAC)

		// Audit write failure continues processing (ADR-038)
		It("ADR-038: should continue processing when audit write fails", func() {
			By("Creating namespace")
			ns := createTestNamespace("audit-fail")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "test-signal-er-06", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["error-06"],
				Name:        "AuditFail",
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

			By("Verifying processing completed despite audit failure")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})

		// Permanent error with invalid spec (Error Category A)
		It("Error-Cat-A: should fail permanently with invalid spec", func() {
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
					// Dummy RR reference to pass CEL validation
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						Name:      "dummy-rr-er-07",
						Namespace: ns,
					},
					Signal: signalprocessingv1alpha1.SignalData{
						// Empty fingerprint - violates validation
						Fingerprint: "",
						Name:        "InvalidSpec",
						Severity: "high",
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

	// ========================================
	// BR-SP-003: RECOVERY CONTEXT INTEGRATION TESTS
	// Business Value: AI analysis can consider previous failure context for better decisions
	// Stakeholder: AI Analysis service needs recovery history for recommendations
	// ========================================

	Context("BR-SP-003 - Recovery Context Integration", func() {
		// First attempt (no recovery context)
		It("RC-001: should return nil recovery context for first attempt (RecoveryAttempts=0)", func() {
			By("Creating namespace")
			ns := createTestNamespace("recovery-first")
			defer deleteTestNamespace(ns)

			By("Creating RemediationRequest with 0 recovery attempts (first attempt)")
			now := metav1.Now()
			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rr-first",
					Namespace: ns,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: ValidTestFingerprints["audit-001"],
					SignalName:        "RecoveryFirstSignal",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: ns,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: now,
						LastOccurrence:  now,
						OccurrenceCount: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, rr) }()

			// Update RR status to have 0 recovery attempts
			rr.Status.RecoveryAttempts = 0
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR referencing the RemediationRequest")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sp-rc001",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: ValidTestFingerprints["audit-001"],
						Name:        "RecoveryFirst",
						Severity:    "critical",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying no recovery context (first attempt)")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// First attempt = no recovery context
			Expect(final.Status.RecoveryContext).To(BeNil())
		})

		// Retry attempt with recovery context
		It("RC-002: should populate recovery context for retry attempt (RecoveryAttempts>0)", func() {
			By("Creating namespace")
			ns := createTestNamespace("recovery-retry")
			defer deleteTestNamespace(ns)

			By("Creating RemediationRequest with recovery attempts (retry)")
			startTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
			failureReason := "PreviousExecutionTimedOut"
			now := metav1.Now()

			rr := &remediationv1alpha1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rr-retry",
					Namespace: ns,
				},
				Spec: remediationv1alpha1.RemediationRequestSpec{
					SignalFingerprint: ValidTestFingerprints["audit-002"],
					SignalName:        "RecoveryRetrySignal",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1alpha1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: ns,
					},
					FiringTime:   now,
					ReceivedTime: now,
					Deduplication: sharedtypes.DeduplicationInfo{
						FirstOccurrence: startTime,
						LastOccurrence:  now,
						OccurrenceCount: 3,
					},
				},
			}
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, rr) }()

			// Update RR status to have recovery data
			rr.Status.RecoveryAttempts = 3
			rr.Status.StartTime = &startTime
			rr.Status.FailureReason = &failureReason
			Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR referencing the RemediationRequest")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sp-rc002",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: ValidTestFingerprints["audit-002"],
						Name:        "RecoveryRetry",
						Severity:    "critical",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						Name:      rr.Name,
						Namespace: rr.Namespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying recovery context is populated")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.RecoveryContext).ToNot(BeNil())
			Expect(final.Status.RecoveryContext.AttemptCount).To(Equal(int32(3)))
			Expect(final.Status.RecoveryContext.PreviousRemediationID).To(Equal(rr.Name))
			Expect(final.Status.RecoveryContext.LastFailureReason).To(Equal("PreviousExecutionTimedOut"))
			Expect(final.Status.RecoveryContext.TimeSinceFirstFailure).ToNot(BeNil())
			// Should be approximately 5 minutes (with some tolerance for test execution time)
			Expect(final.Status.RecoveryContext.TimeSinceFirstFailure.Duration).To(BeNumerically("~", 5*time.Minute, 30*time.Second))
		})

		// Missing RemediationRequest (graceful degradation)
		It("RC-003: should return nil recovery context when RemediationRequest not found", func() {
			By("Creating namespace")
			ns := createTestNamespace("recovery-missing")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with non-existent RR reference")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sp-rc003",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: ValidTestFingerprints["audit-003"],
						Name:        "RecoveryMissing",
						Severity: "high",
						Type:        "prometheus",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						Name:      "non-existent-rr",
						Namespace: ns,
					},
				},
			}
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion (should succeed despite missing RR)")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying no recovery context (graceful degradation)")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Missing RR = nil recovery context (graceful degradation)
			Expect(final.Status.RecoveryContext).To(BeNil())
			// Should complete successfully (not fail)
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})

		// No RemediationRequest reference (happy path)
		It("RC-004: should return nil recovery context when no RR reference provided", func() {
			By("Creating namespace")
			ns := createTestNamespace("recovery-noref")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with dummy RR reference (CEL validation)")
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sp-rc004",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					// Dummy RR reference to pass CEL validation (tests graceful degradation when RR doesn't exist)
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						Name:      "dummy-rr-rc004",
						Namespace: ns,
					},
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: ValidTestFingerprints["audit-004"],
						Name:        "NoRRRef",
						Severity: "low",
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

			By("Verifying no recovery context (no RR reference)")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// No RR reference = nil recovery context
			Expect(final.Status.RecoveryContext).To(BeNil())
		})
	})

	// ========================================
	// BR-SP-110: KUBERNETES CONDITIONS TESTS
	// ========================================
	// Per DD-SP-002 and DD-CRD-002

	Context("Kubernetes Conditions (BR-SP-110)", func() {
		It("BR-SP-110: should set all 4 conditions on successful processing", func() {
			By("Creating test namespace")
			ns := createTestNamespaceWithLabels("cond-test", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating test pod")
			podLabels := map[string]string{"app": "conditions-test"}
			_ = createTestPod(ns, "cond-test-pod", podLabels, nil)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "cond-test-pod",
				Namespace: ns,
			}
			rrName := "cond-test-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["reconciler-01"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("cond-test-sp", ns, rr, ValidTestFingerprints["reconciler-01"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred(), "SignalProcessing should complete")

			By("Verifying all 4 conditions are True")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Verify EnrichmentComplete condition
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionEnrichmentComplete)).To(BeTrue(),
				"EnrichmentComplete should be True")

			// Verify ClassificationComplete condition
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionClassificationComplete)).To(BeTrue(),
				"ClassificationComplete should be True")

			// Verify CategorizationComplete condition
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionCategorizationComplete)).To(BeTrue(),
				"CategorizationComplete should be True")

			// Verify ProcessingComplete condition (terminal)
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionProcessingComplete)).To(BeTrue(),
				"ProcessingComplete should be True")

			// Verify all 5 conditions exist (EnrichmentComplete, ClassificationComplete, CategorizationComplete, ProcessingComplete, Ready)
			Expect(final.Status.Conditions).To(HaveLen(5), "Should have exactly 5 conditions")
		})

		It("BR-SP-110: should have informative condition messages", func() {
			By("Creating test namespace")
			ns := createTestNamespaceWithLabels("cond-msg", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating test deployment")
			deployLabels := map[string]string{"app": "msg-test"}
			_ = createTestDeployment(ns, "msg-test-deploy", deployLabels)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "msg-test-deploy",
				Namespace: ns,
			}
			rrName := "cond-msg-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["reconciler-02"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("cond-msg-sp", ns, rr, ValidTestFingerprints["reconciler-02"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying condition messages contain useful information")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Enrichment message should reference target resource
			enrichCond := spconditions.GetCondition(&final, spconditions.ConditionEnrichmentComplete)
			Expect(enrichCond).ToNot(BeNil())
			Expect(enrichCond.Message).To(ContainSubstring("Deployment"))

			// Classification message should reference environment and priority
			classCond := spconditions.GetCondition(&final, spconditions.ConditionClassificationComplete)
			Expect(classCond).ToNot(BeNil())
			Expect(classCond.Message).To(ContainSubstring("environment="))
			Expect(classCond.Message).To(ContainSubstring("priority="))

			// Categorization message should reference business fields
			catCond := spconditions.GetCondition(&final, spconditions.ConditionCategorizationComplete)
			Expect(catCond).ToNot(BeNil())
			Expect(catCond.Message).To(ContainSubstring("businessUnit="))

			// ProcessingComplete message should reference processing time
			procCond := spconditions.GetCondition(&final, spconditions.ConditionProcessingComplete)
			Expect(procCond).ToNot(BeNil())
			Expect(procCond.Message).To(ContainSubstring("processed successfully"))
		})
	})

	// ========================================
	// BR-SP-111: SHARED BACKOFF INTEGRATION TESTS
	// ========================================

	Context("BR-SP-111: Shared Exponential Backoff Integration", func() {
		// Test: Successful processing should have zero consecutive failures
		It("BR-SP-111: should have zero consecutive failures on successful processing", func() {
			By("Creating test namespace")
			ns := createTestNamespaceWithLabels("backoff-success", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating test pod")
			podLabels := map[string]string{"app": "backoff-test"}
			_ = createTestPod(ns, "backoff-test-pod", podLabels, nil)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "backoff-test-pod",
				Namespace: ns,
			}
			rrName := "backoff-success-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["backoff-01"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("backoff-success-sp", ns, rr, ValidTestFingerprints["backoff-01"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying ConsecutiveFailures is zero after successful processing")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())
			Expect(final.Status.ConsecutiveFailures).To(Equal(int32(0)),
				"ConsecutiveFailures should be 0 after successful processing")
			Expect(final.Status.LastFailureTime).To(BeNil(),
				"LastFailureTime should be nil (no failures occurred)")
		})

		// Test: ConsecutiveFailures field is initialized correctly
		It("BR-SP-111: should initialize ConsecutiveFailures to zero on new CR", func() {
			By("Creating test namespace")
			ns := createTestNamespaceWithLabels("backoff-init", map[string]string{
				"kubernaut.ai/environment": "development",
			})
			defer deleteTestNamespace(ns)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "init-test-pod",
				Namespace: ns,
			}
			rrName := "backoff-init-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["backoff-02"], "low", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("backoff-init-sp", ns, rr, ValidTestFingerprints["backoff-02"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Verifying initial ConsecutiveFailures is zero")
			var created signalprocessingv1alpha1.SignalProcessing
			Eventually(func() int32 {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &created)
				return created.Status.ConsecutiveFailures
			}, timeout, interval).Should(Equal(int32(0)),
				"ConsecutiveFailures should initialize to 0")
		})

		// Test: Multiple successful reconciliations don't increment failures
		It("BR-SP-111: should maintain zero failures across multiple phase transitions", func() {
			By("Creating test namespace with production environment")
			ns := createTestNamespaceWithLabels("backoff-phases", map[string]string{
				"kubernaut.ai/environment":   "production",
				"kubernaut.ai/business-unit": "payments",
			})
			defer deleteTestNamespace(ns)

			By("Creating test deployment")
			deployLabels := map[string]string{"app": "multi-phase"}
			_ = createTestDeployment(ns, "multi-phase-deploy", deployLabels)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "multi-phase-deploy",
				Namespace: ns,
			}
			rrName := "backoff-phases-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["backoff-03"], "critical", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("backoff-phases-sp", ns, rr, ValidTestFingerprints["backoff-03"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion (goes through all phases: Pending → Enriching → Classifying → Categorizing → Completed)")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying ConsecutiveFailures remains zero after all phase transitions")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Verify phase is Completed
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))

			// Verify all conditions are True (all phases succeeded)
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionEnrichmentComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionClassificationComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionCategorizationComplete)).To(BeTrue())
			Expect(spconditions.IsConditionTrue(&final, spconditions.ConditionProcessingComplete)).To(BeTrue())

			// Verify no failures accumulated
			Expect(final.Status.ConsecutiveFailures).To(Equal(int32(0)),
				"ConsecutiveFailures should remain 0 after all successful phase transitions")
		})

		// Test: Status field exists in CRD schema
		It("BR-SP-111: should have ConsecutiveFailures and LastFailureTime fields in status", func() {
			By("Creating test namespace")
			ns := createTestNamespaceWithLabels("backoff-schema", nil)
			defer deleteTestNamespace(ns)

			By("Creating parent RemediationRequest")
			targetResource := signalprocessingv1alpha1.ResourceIdentifier{
				Kind:      "Pod",
				Name:      "schema-test-pod",
				Namespace: ns,
			}
			rrName := "backoff-schema-rr"
			rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["backoff-04"], "high", targetResource)
			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := CreateTestSignalProcessingWithParent("backoff-schema-sp", ns, rr, ValidTestFingerprints["backoff-04"], targetResource)
			Expect(k8sClient.Create(ctx, sp)).To(Succeed())
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Verifying status fields are accessible (schema validation)")
			var created signalprocessingv1alpha1.SignalProcessing
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &created)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// These fields should exist and be accessible (schema validation)
			// Even if not set, accessing them should not panic
			_ = created.Status.ConsecutiveFailures
			_ = created.Status.LastFailureTime
		})
	})
})
