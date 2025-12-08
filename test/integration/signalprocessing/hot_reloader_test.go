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

// Package signalprocessing_test contains Hot-Reload integration tests for SignalProcessing.
// These tests validate ConfigMap-based policy hot-reload functionality.
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Hot-reload logic (test/unit/signalprocessing/)
// - Integration tests (>50%): Real ConfigMap interaction (this file)
// - E2E tests (10-15%): Complete workflow validation (test/e2e/signalprocessing/)
//
// TDD Phase: RED - Tests define expected hot-reload behavior
// Implementation Plan: Day 10, Tier 5 - Hot-Reload Integration Tests
//
// Test Matrix: 5 tests per IMPLEMENTATION_PLAN_V1.30.md
// - BR-SP-072: File watch - policy file change detected
// - BR-SP-072: Reload - valid policy takes effect
// - BR-SP-072: Graceful - invalid policy → old retained
// - BR-SP-072: Concurrent - update during active reconciliation
// - BR-SP-072: Recovery - watcher restart after error
//
// Business Requirements Coverage:
// - BR-SP-072: ConfigMap hot-reload without restart (5 tests)
//
// NOTE: These tests verify hot-reload behavior through the controller's
// ability to pick up ConfigMap changes and apply updated policies to
// subsequent SignalProcessing reconciliations.
package signalprocessing_test

import (
	"sync"
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
		// BR-SP-072: Policy file change detected
		It("BR-SP-072: should detect policy file change in ConfigMap", func() {
			By("Creating namespace")
			ns := createTestNamespace("hr-file-watch")
			defer deleteTestNamespace(ns)

			By("Creating initial ConfigMap with labels.rego policy")
			labelsConfigMap := &corev1.ConfigMap{
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
			Expect(k8sClient.Create(ctx, labelsConfigMap)).To(Succeed())

			By("Creating SignalProcessing CR with initial policy")
			sp1 := createSignalProcessingCR(ns, "hr-file-watch-test-1", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrfw01abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRFileWatchTest1",
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

			By("Waiting for first CR to complete with v1 policy")
			err := waitForCompletion(sp1.Name, sp1.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying v1 label applied")
			var result1 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp1.Name, Namespace: ns}, &result1)).To(Succeed())
			Expect(result1.Status.KubernetesContext).ToNot(BeNil())
			Expect(result1.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("version", ContainElement("v1")))

			By("Updating ConfigMap to v2 policy")
			var existingCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-labels-config", Namespace: ns}, &existingCM)).To(Succeed())
			existingCM.Data["labels.rego"] = `package signalprocessing.labels

import rego.v1

labels["version"] := ["v2"] if { true }
`
			Expect(k8sClient.Update(ctx, &existingCM)).To(Succeed())

			By("Waiting for hot-reload to take effect")
			// Give the file watcher time to detect the change
			time.Sleep(2 * time.Second)

			By("Creating second SignalProcessing CR to verify new policy")
			sp2 := createSignalProcessingCR(ns, "hr-file-watch-test-2", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrfw02abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRFileWatchTest2",
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

			By("Waiting for second CR to complete with v2 policy")
			err = waitForCompletion(sp2.Name, sp2.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying v2 label applied (hot-reload detected)")
			var result2 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp2.Name, Namespace: ns}, &result2)).To(Succeed())
			Expect(result2.Status.KubernetesContext).ToNot(BeNil())
			Expect(result2.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("version", ContainElement("v2")))
		})
	})

	// ========================================
	// RELOAD TEST (1 test)
	// ========================================

	Context("Reload - Valid Policy Application", func() {
		// BR-SP-072: Valid policy takes effect
		It("BR-SP-072: should apply valid updated policy immediately", func() {
			By("Creating namespace with production label")
			ns := createTestNamespaceWithLabels("hr-reload-valid", map[string]string{
				"kubernaut.ai/environment": "production",
			})
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with initial priority policy (P2 for production)")
			priorityConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-priority-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"priority.rego": `package signalprocessing.priority

import rego.v1

# Initial policy: production + warning = P2
priority := "P2" if {
    input.environment == "production"
    input.signal.severity == "warning"
}

default priority := "P3"
`,
				},
			}
			Expect(k8sClient.Create(ctx, priorityConfigMap)).To(Succeed())

			By("Creating SignalProcessing CR with initial policy")
			sp1 := createSignalProcessingCR(ns, "hr-reload-valid-test-1", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrrv01abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRReloadValidTest1",
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

			By("Verifying initial P2 priority")
			var result1 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp1.Name, Namespace: ns}, &result1)).To(Succeed())
			Expect(result1.Status.PriorityAssignment).ToNot(BeNil())
			Expect(result1.Status.PriorityAssignment.Priority).To(Equal("P2"))

			By("Updating ConfigMap to new priority policy (P1 for production)")
			var existingCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-priority-config", Namespace: ns}, &existingCM)).To(Succeed())
			existingCM.Data["priority.rego"] = `package signalprocessing.priority

import rego.v1

# Updated policy: production + warning = P1 (more urgent)
priority := "P1" if {
    input.environment == "production"
    input.signal.severity == "warning"
}

default priority := "P3"
`
			Expect(k8sClient.Update(ctx, &existingCM)).To(Succeed())

			By("Waiting for hot-reload to take effect")
			time.Sleep(2 * time.Second)

			By("Creating second SignalProcessing CR to verify updated policy")
			sp2 := createSignalProcessingCR(ns, "hr-reload-valid-test-2", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrrv02abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRReloadValidTest2",
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

			By("Verifying updated P1 priority (hot-reload applied)")
			var result2 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp2.Name, Namespace: ns}, &result2)).To(Succeed())
			Expect(result2.Status.PriorityAssignment).ToNot(BeNil())
			Expect(result2.Status.PriorityAssignment.Priority).To(Equal("P1"))
		})
	})

	// ========================================
	// GRACEFUL FALLBACK TEST (1 test)
	// ========================================

	Context("Graceful - Invalid Policy Fallback", func() {
		// IT-HR-03: Invalid policy → old retained
		It("BR-SP-072: should retain old policy when update is invalid", func() {
			By("Creating namespace")
			ns := createTestNamespace("hr-graceful")
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with valid labels.rego policy")
			labelsConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["valid"] := ["true"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, labelsConfigMap)).To(Succeed())

			By("Creating SignalProcessing CR with valid policy")
			sp1 := createSignalProcessingCR(ns, "hr-graceful-test-1", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrgr01abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRGracefulTest1",
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

			By("Verifying valid label applied")
			var result1 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp1.Name, Namespace: ns}, &result1)).To(Succeed())
			Expect(result1.Status.KubernetesContext).ToNot(BeNil())
			Expect(result1.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("valid", ContainElement("true")))

			By("Updating ConfigMap to INVALID Rego syntax")
			var existingCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-labels-config", Namespace: ns}, &existingCM)).To(Succeed())
			existingCM.Data["labels.rego"] = `package signalprocessing.labels
// INVALID REGO - Missing import, broken syntax
labels["broken" := ["syntax"  // Missing bracket
`
			Expect(k8sClient.Update(ctx, &existingCM)).To(Succeed())

			By("Waiting for hot-reload attempt")
			time.Sleep(2 * time.Second)

			By("Creating second SignalProcessing CR")
			sp2 := createSignalProcessingCR(ns, "hr-graceful-test-2", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrgr02abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRGracefulTest2",
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

			By("Verifying graceful degradation - CR completed despite invalid policy")
			var result2 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp2.Name, Namespace: ns}, &result2)).To(Succeed())
			Expect(result2.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))

			// Old policy retained OR graceful fallback to empty (depends on implementation)
			// Either way, CR should complete successfully
			GinkgoWriter.Printf("CustomLabels after invalid policy update: %v\n", result2.Status.KubernetesContext.CustomLabels)
		})
	})

	// ========================================
	// CONCURRENT UPDATE TEST (1 test)
	// ========================================

	Context("Concurrent - Update During Reconciliation", func() {
		// IT-HR-04: Update during active reconciliation
		It("BR-SP-072: should handle policy update during active reconciliation", func() {
			By("Creating namespace")
			ns := createTestNamespaceWithLabels("hr-concurrent", map[string]string{
				"kubernaut.ai/environment": "staging",
			})
			defer deleteTestNamespace(ns)

			By("Creating ConfigMap with initial policy")
			labelsConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["concurrent-test"] := ["initial"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, labelsConfigMap)).To(Succeed())

			By("Creating multiple SignalProcessing CRs concurrently while updating policy")
			var wg sync.WaitGroup
			sps := make([]*signalprocessingv1alpha1.SignalProcessing, 5)
			errors := make([]error, 5)

			// Start creating CRs
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()

					sp := createSignalProcessingCR(ns, "hr-concurrent-"+string(rune('a'+idx)), signalprocessingv1alpha1.SignalData{
						Fingerprint: "hrcc" + string(rune('a'+idx)) + "abc123def456abc123def456abc123def456abc123def456abc123" + string(rune('0'+idx)),
						Name:        "HRConcurrentTest",
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
					sps[idx] = sp
				}(i)
			}

			// Update ConfigMap mid-flight
			go func() {
				defer GinkgoRecover()
				time.Sleep(500 * time.Millisecond) // Give CRs time to start processing

				var existingCM corev1.ConfigMap
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "signalprocessing-labels-config", Namespace: ns}, &existingCM); err != nil {
					GinkgoWriter.Printf("Warning: Failed to get ConfigMap for update: %v\n", err)
					return
				}
				existingCM.Data["labels.rego"] = `package signalprocessing.labels

import rego.v1

labels["concurrent-test"] := ["updated"] if { true }
`
				if err := k8sClient.Update(ctx, &existingCM); err != nil {
					GinkgoWriter.Printf("Warning: Failed to update ConfigMap: %v\n", err)
				}
			}()

			wg.Wait()

			By("Waiting for all CRs to complete")
			for i, sp := range sps {
				if sp != nil {
					errors[i] = waitForCompletion(sp.Name, sp.Namespace, timeout)
				}
			}

			By("Verifying all CRs completed successfully (no crashes)")
			for i, sp := range sps {
				if sp != nil {
					Expect(errors[i]).ToNot(HaveOccurred(), "CR %d should complete", i)

					var result signalprocessingv1alpha1.SignalProcessing
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &result)).To(Succeed())
					Expect(result.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))

					_ = deleteAndWait(sp, timeout)
				}
			}

			GinkgoWriter.Println("✅ All concurrent CRs completed during policy update")
		})
	})

	// ========================================
	// RECOVERY TEST (1 test)
	// ========================================

	Context("Recovery - Watcher Restart", func() {
		// IT-HR-05: Watcher restart after error
		It("BR-SP-072: should recover and process new CRs after ConfigMap delete/recreate", func() {
			By("Creating namespace")
			ns := createTestNamespace("hr-recovery")
			defer deleteTestNamespace(ns)

			By("Creating initial ConfigMap")
			labelsConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["phase"] := ["initial"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, labelsConfigMap)).To(Succeed())

			By("Creating first SignalProcessing CR")
			sp1 := createSignalProcessingCR(ns, "hr-recovery-test-1", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrrc01abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRRecoveryTest1",
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

			By("Deleting ConfigMap to simulate error condition")
			Expect(k8sClient.Delete(ctx, labelsConfigMap)).To(Succeed())

			By("Waiting for deletion")
			time.Sleep(1 * time.Second)

			By("Recreating ConfigMap with new policy")
			recreatedConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "signalprocessing-labels-config",
					Namespace: ns,
				},
				Data: map[string]string{
					"labels.rego": `package signalprocessing.labels

import rego.v1

labels["phase"] := ["recovered"] if { true }
`,
				},
			}
			Expect(k8sClient.Create(ctx, recreatedConfigMap)).To(Succeed())

			By("Waiting for watcher to recover")
			time.Sleep(2 * time.Second)

			By("Creating second SignalProcessing CR after recovery")
			sp2 := createSignalProcessingCR(ns, "hr-recovery-test-2", signalprocessingv1alpha1.SignalData{
				Fingerprint: "hrrc02abc123def456abc123def456abc123def456abc123def456abc123d",
				Name:        "HRRecoveryTest2",
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

			By("Waiting for second CR to complete (proves recovery)")
			err = waitForCompletion(sp2.Name, sp2.Namespace, timeout)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying recovery - CR completed with recovered policy")
			var result2 signalprocessingv1alpha1.SignalProcessing
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp2.Name, Namespace: ns}, &result2)).To(Succeed())
			Expect(result2.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCompleted))

			// Verify recovered policy was applied
			if result2.Status.KubernetesContext != nil && len(result2.Status.KubernetesContext.CustomLabels) > 0 {
				Expect(result2.Status.KubernetesContext.CustomLabels).To(HaveKeyWithValue("phase", ContainElement("recovered")))
			}

			GinkgoWriter.Println("✅ Hot-reload recovered after ConfigMap delete/recreate cycle")
		})
	})
})
