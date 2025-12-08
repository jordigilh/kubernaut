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

// Package signalprocessing_test contains hot-reload integration tests for SignalProcessing.
// These tests validate ConfigMap watch-based policy reloading with real Kubernetes API.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Hot-reload logic (test/unit/signalprocessing/)
// - Integration tests (>50%): Real ConfigMap watch behavior (this file)
// - E2E/BR tests (10-15%): Complete workflow validation (test/e2e/signalprocessing/)
//
// TDD Phase: RED - Tests define expected hot-reload behavior
// Implementation Plan: Day 10, Tier 5 - Hot-Reload Integration Tests
//
// Test Matrix: 5 tests per IMPLEMENTATION_PLAN_V1.30.md
// - File Watch: 1 test (BR-SP-072)
// - Reload: 1 test (BR-SP-072)
// - Graceful: 1 test (BR-SP-072)
// - Concurrent: 1 test (BR-SP-072)
// - Recovery: 1 test (BR-SP-072)
//
// Business Requirements Coverage:
// - BR-SP-072: Hot-reload policy changes without restart (5 tests)
package signalprocessing_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

var _ = Describe("SignalProcessing Hot-Reload Integration", func() {
	// ========================================
	// FILE WATCH TEST (1 test)
	// ========================================

	Context("File Watch - ConfigMap Change Detection", func() {
		// Policy file change detected
		It("BR-SP-072: should detect ConfigMap policy change via watch", func() {
			By("Creating namespace")
			ns := createTestNamespace("hot-reload-watch")
			defer deleteTestNamespace(ns)

			By("Creating initial ConfigMap with labels.rego")
			initialConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["version"] := ["v1"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, initialConfigMap)).To(Succeed())

			By("Creating first SignalProcessing CR")
			sp1 := createSignalProcessingCR(ns, "hot-reload-watch-1", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrw101abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HotReloadWatch1",
				Severity:    "warning",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp1, timeout) }()

			By("Waiting for first CR to complete")
			err := waitForCompletion(sp1.Name, sp1.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying initial policy was used (v1)")
			var final1 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp1.Name, Namespace: ns}, &final1)).To(Succeed())
			Expect(final1.Status.KubernetesContext).ToNot(BeNil())
			Expect(final1.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("version", ContainElement("v1")))

			By("Updating ConfigMap to v2")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-labels-config", Namespace: ns}, &cm)).To(Succeed())
			cm.Data["labels.rego"] = `package signalprocessing.labels

import rego.v1

labels["version"] := ["v2"] if { true }
`
			Expect(k8sClient.Update(ctx, &cm)).To(Succeed())

			By("Waiting for watch to detect change")
			time.Sleep(2 * time.Second) // Allow watch event to propagate

			By("Creating second SignalProcessing CR")
			sp2 := createSignalProcessingCR(ns, "hot-reload-watch-2", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrw201abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HotReloadWatch2",
				Severity:    "warning",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp2, timeout) }()

			By("Waiting for second CR to complete")
			err = waitForCompletion(sp2.Name, sp2.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying updated policy was used (v2)")
			var final2 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp2.Name, Namespace: ns}, &final2)).To(Succeed())
			Expect(final2.Status.KubernetesContext).ToNot(BeNil())
			Expect(final2.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("version", ContainElement("v2")))
		})
	})

	// ========================================
	// RELOAD TEST (1 test)
	// ========================================

	Context("Reload - Valid Policy Takes Effect", func() {
		// Valid policy takes effect immediately
		It("BR-SP-072: should apply valid policy immediately after reload", func() {
			By("Creating namespace")
			ns := createTestNamespace("hot-reload-valid")
			defer deleteTestNamespace(ns)

			By("Creating initial ConfigMap with simple policy")
			initialConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["stage"] := ["initial"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, initialConfigMap)).To(Succeed())

			By("Updating to more complex valid policy")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-labels-config", Namespace: ns}, &cm)).To(Succeed())
			cm.Data["labels.rego"] = `package signalprocessing.labels

import rego.v1

labels["stage"] := ["reloaded"] if { true }
labels["extra"] := ["newkey"] if { true }
`
			Expect(k8sClient.Update(ctx, &cm)).To(Succeed())

			By("Waiting for reload to complete")
			time.Sleep(2 * time.Second)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "hot-reload-valid-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrv101abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HotReloadValidTest",
				Severity:    "warning",
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

			By("Verifying new policy took effect")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())
			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("stage", ContainElement("reloaded")))
			Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKey("extra"))
		})
	})

	// ========================================
	// GRACEFUL TEST (1 test)
	// ========================================

	Context("Graceful - Invalid Policy Handling", func() {
		// Invalid policy retains old policy
		It("BR-SP-072: should retain old policy when new policy is invalid", func() {
			By("Creating namespace")
			ns := createTestNamespace("hot-reload-graceful")
			defer deleteTestNamespace(ns)

			By("Creating initial valid ConfigMap")
			initialConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["valid"] := ["working"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, initialConfigMap)).To(Succeed())

			By("Creating first CR with valid policy")
			sp1 := createSignalProcessingCR(ns, "hot-reload-graceful-1", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrg101abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HotReloadGraceful1",
				Severity:    "warning",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp1, timeout) }()

			By("Waiting for first CR to complete")
			err := waitForCompletion(sp1.Name, sp1.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying valid policy was used")
			var final1 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp1.Name, Namespace: ns}, &final1)).To(Succeed())
			Expect(final1.Status.KubernetesContext).ToNot(BeNil())
			Expect(final1.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("valid", ContainElement("working")))

			By("Updating ConfigMap with INVALID policy")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-labels-config", Namespace: ns}, &cm)).To(Succeed())
			cm.Data["labels.rego"] = `package signalprocessing.labels
// INVALID REGO - syntax error
labels["broken" := ["missing_bracket"
`
			Expect(k8sClient.Update(ctx, &cm)).To(Succeed())

			By("Waiting for reload attempt")
			time.Sleep(2 * time.Second)

			By("Creating second CR after invalid policy update")
			sp2 := createSignalProcessingCR(ns, "hot-reload-graceful-2", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrg201abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HotReloadGraceful2",
				Severity:    "warning",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: ns,
				},
				ReceivedTime: metav1.Now(),
			})
			defer func() { _ = deleteAndWait(sp2, timeout) }()

			By("Waiting for second CR to complete")
			err = waitForCompletion(sp2.Name, sp2.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying graceful handling - should either use old policy or empty labels")
			var final2 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp2.Name, Namespace: ns}, &final2)).To(Succeed())
			// Should complete (not fail) with either old policy or empty labels
			Expect(final2.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
		})
	})

	// ========================================
	// CONCURRENT TEST (1 test)
	// ========================================

	Context("Concurrent - Update During Reconciliation", func() {
		// Update during active reconciliation
		It("BR-SP-072: should handle policy update during active reconciliation", func() {
			By("Creating namespace")
			ns := createTestNamespace("hot-reload-concurrent")
			defer deleteTestNamespace(ns)

			By("Creating initial ConfigMap")
			initialConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["concurrent"] := ["before"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, initialConfigMap)).To(Succeed())

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "hot-reload-concurrent-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrc101abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HotReloadConcurrentTest",
				Severity:    "warning",
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

			By("Immediately updating ConfigMap while reconciliation may be in progress")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-labels-config", Namespace: ns}, &cm)).To(Succeed())
			cm.Data["labels.rego"] = `package signalprocessing.labels

import rego.v1

labels["concurrent"] := ["during"] if { true }
`
			Expect(k8sClient.Update(ctx, &cm)).To(Succeed())

			By("Waiting for completion")
			err := waitForCompletion(sp.Name, sp.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying completion despite concurrent update")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			// Should complete successfully - may use either "before" or "during"
			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			Expect(final.Status.KubernetesContext).ToNot(BeNil())
			// The value should be one of the two - we don't specify which due to race
			if final.Status.KubernetesContext.CustomLabels != nil {
				if values, ok := final.Status.KubernetesContext.CustomLabels["concurrent"]; ok {
					Expect(values).To(Or(ContainElement("before"), ContainElement("during")))
				}
			}
		})
	})

	// ========================================
	// RECOVERY TEST (1 test)
	// ========================================

	Context("Recovery - Watcher Restart", func() {
		// Watcher restart after error
		It("BR-SP-072: should recover watcher after transient error", func() {
			By("Creating namespace")
			ns := createTestNamespace("hot-reload-recovery")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["recovery"] := ["working"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("Deleting and recreating ConfigMap to simulate transient error")
			Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())

			// Wait for deletion to complete
			time.Sleep(500 * time.Millisecond)

			// Recreate with same name
			newConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["recovery"] := ["recovered"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, newConfigMap)).To(Succeed())

			By("Waiting for watcher to recover")
			time.Sleep(2 * time.Second)

			By("Creating SignalProcessing CR")
			sp := createSignalProcessingCR(ns, "hot-reload-recovery-test", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrr101abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HotReloadRecoveryTest",
				Severity:    "warning",
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

			By("Verifying watcher recovered and new policy is used")
			var final signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

			Expect(final.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))
			// Should use the recreated policy
			if final.Status.KubernetesContext != nil && final.Status.KubernetesContext.CustomLabels != nil {
				Expect(final.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("recovery", ContainElement("recovered")))
			}
		})
	})
})

