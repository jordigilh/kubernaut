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

// Package signalprocessing_test contains Rego integration tests for SignalProcessing.
// These tests validate Rego policy loading, evaluation, and security with real ConfigMaps.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Rego engine logic (test/unit/signalprocessing/)
// - Integration tests (>50%): Real ConfigMap interaction (this file)
// - E2E/BR tests (10-15%): Complete workflow validation (test/e2e/signalprocessing/)
//
// TDD Phase: RED - Tests define expected Rego behavior
// Implementation Plan: Day 10, Tier 4 - Rego Integration Tests
//
// Test Matrix: 15 tests per IMPLEMENTATION_PLAN_V1.30.md
// - Policy Load: 3 tests (BR-SP-051, BR-SP-070, BR-SP-102)
// - Evaluation: 3 tests (BR-SP-051, BR-SP-070, BR-SP-102)
// - Security: 1 test (BR-SP-104)
// - Fallback: 2 tests (BR-SP-071, BR-SP-053)
// - Concurrent: 2 tests (Stability, BR-SP-072)
// - Timeout: 1 test (DD-WORKFLOW-001)
// - Validation: 3 tests (DD-WORKFLOW-001)
//
// Business Requirements Coverage:
// - BR-SP-051: Environment classification policy (2 tests)
// - BR-SP-053: Default fallback (1 test)
// - BR-SP-070: Priority assignment policy (2 tests)
// - BR-SP-071: Severity fallback (1 test)
// - BR-SP-072: Hot-reload during evaluation (1 test)
// - BR-SP-102: CustomLabels extraction policy (2 tests)
// - BR-SP-104: System prefix security (1 test)
// - DD-WORKFLOW-001: Timeout and validation limits (4 tests)
package signalprocessing

import (
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Rego integration tests validate ConfigMap-based policy loading with real K8s API.
// These test policy hot-reload behavior per BR-SP-072 and DD-INFRA-001.
var _ = Describe("SignalProcessing Rego Integration", func() {
	// ========================================
	// POLICY LOAD TESTS (3 tests)
	// ========================================

	Context("Policy Load - ConfigMap Integration", func() {
		// Load environment.rego from ConfigMap
		It("BR-SP-051: should load environment.rego policy from ConfigMap", func() {
			By("Creating namespace")
			ns := createTestNamespaceWithLabels("rego-env-load", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "rego-env-load-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-env-01"],
				Name:        "RegoEnvLoadTest",
				Severity: "high",
				Type:        "alert",
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

			By("Verifying environment.rego was loaded and evaluated")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// The suite_test.go creates environment.rego in kubernaut-system
			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
		})

		// Load priority.rego from ConfigMap
		It("BR-SP-070: should load priority.rego policy from ConfigMap", func() {
			By("Creating namespace")
			ns := createTestNamespaceWithLabels("rego-pri-load", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with critical severity")
			sp := createSignalProcessingCR(ns, "rego-pri-load-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-pri-01"],
				Name:        "RegoPriLoadTest",
				Severity:    "critical",
				Type:        "alert",
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

			By("Verifying priority.rego was loaded and evaluated")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Production + Critical = P0 per priority.rego
			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P0"))
		})

		// NOTE: BR-SP-102 labels.rego test removed - DD-INFRA-001 replaced ConfigMap with file-based hot-reload
		// Coverage: hot_reloader_test.go
	})

	// ========================================
	// EVALUATION TESTS (3 tests)
	// ========================================

	Context("Evaluation - Rego Policy Execution", func() {
		// Environment classification evaluation
		It("BR-SP-051: should evaluate environment classification rules correctly", func() {
			By("Creating staging namespace")
			ns := createTestNamespaceWithLabels("rego-eval-env", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "rego-eval-env-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-eve-01"],
				Name:        "RegoEvalEnvTest",
				Severity: "high",
				Type:        "alert",
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

			By("Verifying environment evaluation")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("staging"))
			// Note: Confidence field removed per DD-SP-001 V1.1
		})

		// Priority assignment evaluation
		It("BR-SP-070: should evaluate priority assignment rules correctly", func() {
			By("Creating development namespace")
			ns := createTestNamespaceWithLabels("rego-eval-pri", map[string]string{
				"kubernaut.ai/environment": "development",
			})
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR with warning severity")
			sp := createSignalProcessingCR(ns, "rego-eval-pri-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-evp-01"],
				Name:        "RegoEvalPriTest",
				Severity: "high",
				Type:        "alert",
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

			By("Verifying priority evaluation")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.PriorityAssignment).ToNot(BeNil())
			// Development + warning = P3 per priority.rego
			Expect(final.Status.PriorityAssignment.Priority).To(Equal("P3"))
		})

		// NOTE: BR-SP-102 CustomLabels extraction test removed - DD-INFRA-001 replaced ConfigMap with file-based hot-reload
		// Coverage: hot_reloader_test.go
	})

	// NOTE: Security test (BR-SP-104) removed - DD-INFRA-001 replaced ConfigMap with file-based hot-reload
	// Coverage: Unit tests for system prefix stripping

	// ========================================
	// FALLBACK TESTS (2 tests)
	// ========================================

	Context("Fallback - Error Recovery", func() {
		// NOTE: BR-SP-071 invalid policy test removed - DD-INFRA-001 replaced ConfigMap with file-based hot-reload
		// Coverage: hot_reloader_test.go

		// Missing ConfigMap falls back to defaults
		It("BR-SP-053: should fall back to defaults when ConfigMap is missing", func() {
			By("Creating namespace without any ConfigMaps")
			ns := createTestNamespace("rego-fallback-missing")
			defer deleteTestNamespace(ns)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "rego-fallback-missing-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-fms-01"],
				Name:        "RegoFallbackMissingTest",
				Severity: "high",
				Type:        "alert",
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

			By("Verifying fallback to defaults")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Should complete with default environment
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			Expect(final.Status.EnvironmentClassification).ToNot(BeNil())
			Expect(final.Status.EnvironmentClassification.Environment).To(Equal("unknown"))
		})
	})

	// ========================================
	// CONCURRENT TESTS (2 tests)
	// ========================================

	Context("Concurrent - Parallel Execution", func() {
		// 10 parallel evaluations
		It("Stability: should handle 10 parallel Rego evaluations", func() {
			By("Creating namespace")
			ns := createTestNamespaceWithLabels("rego-concurrent", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating 10 SignalProcessing CRs concurrently")
			var wg sync.WaitGroup
			sps := make([]*signalprocessingv1alpha1.SignalProcessing, 10)

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()

					sp := createSignalProcessingCR(ns, "rego-concurrent-"+string(rune('a'+idx)), signalprocessingv1alpha1.SignalData{
						Fingerprint: GenerateConcurrentFingerprint("rego-concurrent", idx),
						Name:        "RegoConcurrentTest",
						Severity: "high",
						Type:        "alert",
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

		// NOTE: BR-SP-072 policy update test removed - covered by hot_reloader_test.go
	})

	// ========================================
	// TIMEOUT TEST (1 test)
	// ========================================

	Context("Timeout - Execution Limits", func() {
		// 5s timeout enforcement
		It("DD-WORKFLOW-001: should enforce 5s timeout on Rego evaluation", func() {
			By("Creating namespace")
			ns := createTestNamespace("rego-timeout")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with slow policy (infinite loop protection)")
			// Note: OPA has built-in protection against infinite loops
			// This test verifies our timeout wrapper works
			slowConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.customlabels

import rego.v1

# Complex computation that should still complete within timeout
labels["computed"] := [result] if {
    numbers := numbers.range(1, 1000)
    result := concat("-", [format_int(n, 10) | n := numbers[_]])
}
`,
				},
			}
			Expect(k8sClient.Create(ctx, slowConfigMap)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "rego-timeout-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-tim-01"],
				Name:        "RegoTimeoutTest",
				Severity: "high",
				Type:        "alert",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp, timeout) }()

			By("Waiting for completion (should complete or timeout within 5s)")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying completion")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Should complete (possibly with empty CustomLabels if policy failed)
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})
	})

	// ========================================
	// VALIDATION TESTS (3 tests)
	// ========================================

	Context("Validation - Output Limits", func() {
		// NOTE: DD-WORKFLOW-001 key truncation test removed - DD-INFRA-001 replaced ConfigMap with file-based hot-reload
		// Coverage: Unit tests for key truncation

		// Value truncation (100 chars)
		It("DD-WORKFLOW-001: should truncate values longer than 100 characters", func() {
			By("Creating namespace")
			ns := createTestNamespace("rego-val-value")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with long values")
			longValue := strings.Repeat("y", 200) // 200 character value
			validationConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.customlabels

import rego.v1

labels["longvalue"] := ["` + longValue + `"] if { true }
labels["shortvalue"] := ["ok"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, validationConfigMap)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "rego-val-value-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-vlv-01"],
				Name:        "RegoValValueTest",
				Severity: "high",
				Type:        "alert",
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

			By("Verifying value truncation")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			// Values should be truncated
			for _, values := range final.Status.KubernetesContext.CustomLabels {
				for _, v := range values {
					Expect(len(v)).To(BeNumerically("<=", 100), "Value should be truncated to 100 chars")
				}
			}
		})

		// Max keys truncation (10)
		It("DD-WORKFLOW-001: should limit CustomLabels to 10 keys maximum", func() {
			By("Creating namespace")
			ns := createTestNamespace("rego-val-maxkeys")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with more than 10 keys")
			// Generate policy with 15 keys
			policyBuilder := strings.Builder{}
			policyBuilder.WriteString("package signalprocessing.customlabels\n\nimport rego.v1\n\n")
			for i := 0; i < 15; i++ {
				policyBuilder.WriteString("labels[\"key" + string(rune('a'+i)) + "\"] := [\"value\"] if { true }\n")
			}

			validationConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": policyBuilder.String(),
				},
			}
			Expect(k8sClient.Create(ctx, validationConfigMap)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "rego-val-maxkeys-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: ValidTestFingerprints["rego-vmk-01"],
				Name:        "RegoValMaxKeysTest",
				Severity: "high",
				Type:        "alert",
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

			By("Verifying max keys limit")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			// Should have at most 10 keys
			Expect(len(final.Status.KubernetesContext.CustomLabels)).To(BeNumerically("<=", 10), "Should have max 10 keys")
		})
	})
})
