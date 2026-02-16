/*
Copyright 2026 Jordi Gil.

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

package effectivenessmonitor

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

var _ = Describe("Spec Hash Integration (BR-EM-004)", func() {

	// ========================================
	// IT-EM-SH-001: Hash computed -> hash present in status
	// ========================================
	It("IT-EM-SH-001: should compute and store post-remediation spec hash", func() {
		ns := createTestNamespace("em-sh-001")
		defer deleteTestNamespace(ns)

		By("Creating an EA targeting a resource")
		ea := createEffectivenessAssessment(ns, "ea-sh-001", "rr-sh-001")

		By("Verifying the EA completes with hash computed")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"hash should be computed")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).NotTo(BeEmpty(),
			"post-remediation spec hash should be set")
		// Hash uses DD-EM-002 canonical format: "sha256:<64-hex>" (71 chars)
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"),
			"hash should use canonical sha256: prefix (DD-EM-002)")
		Expect(len(fetchedEA.Status.Components.PostRemediationSpecHash)).To(Equal(71),
			"hash should be 71 characters: 'sha256:' (7) + 64 hex digits")
	})

	// ========================================
	// IT-EM-SH-002: No pre-remediation hash -> hash event still emitted
	// ========================================
	It("IT-EM-SH-002: should compute hash even without pre-remediation baseline", func() {
		ns := createTestNamespace("em-sh-002")
		defer deleteTestNamespace(ns)

		By("Creating an EA (no pre-remediation hash stored anywhere)")
		ea := createEffectivenessAssessment(ns, "ea-sh-002", "rr-sh-002")

		By("Verifying the EA completes with hash computed")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Hash is always computed even without a pre-remediation baseline
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue())
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).NotTo(BeEmpty())
	})

	// ========================================
	// IT-EM-SH-004: EM completes full assessment using CRD-first pre-remediation baseline
	// BR: BR-EM-004 (Spec Hash Comparison), DD-EM-002 v2.0 (CRD-first path)
	//
	// Business outcome: When the RO provides a pre-remediation hash in the EA spec,
	// the EM uses it to compare pre vs post-remediation state AND completes a full
	// assessment with all components assessed. The assessment produces a trustworthy
	// baseline (PostRemediationSpecHash = CurrentSpecHash) for future drift detection.
	// ========================================
	It("IT-EM-SH-004: should complete full assessment with CRD-sourced pre-remediation baseline", func() {
		ns := createTestNamespace("em-sh-004")
		defer deleteTestNamespace(ns)

		By("Creating an EA with PreRemediationSpecHash set in spec (CRD-first path)")
		// Synthetic pre-hash simulates the hash the RO captured before the workflow ran.
		// It won't match the real post-remediation hash — this is the expected scenario
		// because the workflow should have changed the target resource.
		preHash := "sha256:0000111122223333444455556666777788889999aaaabbbbccccddddeeeeffff"
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-sh-004",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-sh-004",
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "test-app",
					Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 1 * time.Second},
				},
				PreRemediationSpecHash: preHash,
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		By("Waiting for EA to reach Completed phase — all components assessed")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying the assessment completed successfully with all components")
		// Business outcome 1: Hash component assessed — the EM computed the post-hash
		// and established a drift detection baseline.
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"EM must compute the hash component to establish a drift detection baseline")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"),
			"Post-remediation hash must use canonical sha256: format for deterministic comparison")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HaveLen(71),
			"Post-remediation hash must be 71 chars: 'sha256:' (7) + 64 hex digits")

		// Business outcome 2: The post-remediation snapshot becomes the baseline for
		// future drift detection — CurrentSpecHash must match PostRemediationSpecHash.
		Expect(fetchedEA.Status.Components.CurrentSpecHash).To(
			Equal(fetchedEA.Status.Components.PostRemediationSpecHash),
			"CurrentSpecHash must equal PostRemediationSpecHash to serve as the drift detection baseline")

		// Business outcome 3: Pre-remediation hash was preserved in the spec (immutable),
		// confirming the EM had access to the baseline without needing a DataStorage query.
		Expect(fetchedEA.Spec.PreRemediationSpecHash).To(Equal(preHash),
			"Pre-remediation hash must be preserved in the immutable EA spec for auditability")

		// Business outcome 4: Health and alert components were also assessed — the
		// assessment didn't short-circuit due to hash-first ordering.
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"Health component must be assessed even when hash runs first")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"Alert component must be assessed even when hash runs first")

		// Business outcome 5: Assessment completed with a definitive reason.
		Expect(fetchedEA.Status.AssessmentReason).To(
			BeElementOf("full", "partial", "metrics_timed_out"),
			"Assessment must complete with a definitive reason, not hang or fail")
	})

	// ========================================
	// IT-EM-SH-005: EM completes full assessment without pre-remediation hash (backward compat)
	// BR: BR-EM-004 (Spec Hash Comparison), DD-EM-002 v2.1 (Two-Phase Hash Model)
	//
	// Business outcome: Legacy EAs (or EAs where the RO didn't capture a pre-hash)
	// must still get fully assessed. The EM establishes a post-remediation baseline
	// for drift detection and assesses all other components normally. Assessment
	// quality does not degrade — the only difference is that pre vs post comparison
	// is skipped (no baseline to compare against).
	// ========================================
	It("IT-EM-SH-005: should complete full assessment without pre-hash degradation (backward compat)", func() {
		ns := createTestNamespace("em-sh-005")
		defer deleteTestNamespace(ns)

		By("Creating an EA without PreRemediationSpecHash (backward compatibility)")
		ea := createEffectivenessAssessment(ns, "ea-sh-005", "rr-sh-005")

		By("Waiting for EA to reach Completed phase")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea.Name, Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Verifying no assessment degradation despite missing pre-hash")
		// Business outcome 1: Hash component still assessed — the EM captures
		// the post-remediation snapshot for drift detection even without a pre-hash.
		Expect(fetchedEA.Status.Components.HashComputed).To(BeTrue(),
			"Hash must still be computed for drift detection even without pre-remediation baseline")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HavePrefix("sha256:"),
			"Post-remediation hash must use canonical format regardless of pre-hash availability")
		Expect(fetchedEA.Status.Components.PostRemediationSpecHash).To(HaveLen(71),
			"Post-remediation hash must be 71 chars: 'sha256:' (7) + 64 hex digits")

		// Business outcome 2: Drift detection baseline is established.
		Expect(fetchedEA.Status.Components.CurrentSpecHash).To(
			Equal(fetchedEA.Status.Components.PostRemediationSpecHash),
			"Drift detection baseline must be established even without pre-hash")

		// Business outcome 3: All other components assessed normally — no degradation.
		Expect(fetchedEA.Status.Components.HealthAssessed).To(BeTrue(),
			"Health assessment must not be affected by missing pre-hash")
		Expect(fetchedEA.Status.Components.AlertAssessed).To(BeTrue(),
			"Alert assessment must not be affected by missing pre-hash")

		// Business outcome 4: Assessment completes with a definitive reason.
		Expect(fetchedEA.Status.AssessmentReason).To(
			BeElementOf("full", "partial", "metrics_timed_out"),
			"Assessment must complete definitively, not fail due to missing pre-hash")

		// Confirm the EA spec didn't fabricate a pre-hash
		Expect(fetchedEA.Spec.PreRemediationSpecHash).To(BeEmpty(),
			"EM must not fabricate a pre-hash — absence is a valid state for backward compat")
	})

	// ========================================
	// IT-EM-SH-003: Hash event payload verified (correlation_id, hash)
	// ========================================
	It("IT-EM-SH-003: should produce deterministic hash for same target spec", func() {
		ns := createTestNamespace("em-sh-003")
		defer deleteTestNamespace(ns)

		correlationID := fmt.Sprintf("rr-sh-003-%d", time.Now().UnixNano())

		By("Creating first EA")
		ea1 := createEffectivenessAssessment(ns, "ea-sh-003a", correlationID)

		By("Waiting for first EA to complete")
		fetchedEA1 := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea1.Name, Namespace: ea1.Namespace,
			}, fetchedEA1)).To(Succeed())
			g.Expect(fetchedEA1.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		By("Creating second EA targeting the same resource")
		ea2 := createEffectivenessAssessment(ns, "ea-sh-003b", correlationID+"-2")

		By("Waiting for second EA to complete")
		fetchedEA2 := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: ea2.Name, Namespace: ea2.Namespace,
			}, fetchedEA2)).To(Succeed())
			g.Expect(fetchedEA2.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// Same target resource -> same hash (deterministic)
		Expect(fetchedEA1.Status.Components.PostRemediationSpecHash).To(
			Equal(fetchedEA2.Status.Components.PostRemediationSpecHash),
			"same target spec should produce identical hashes")
	})
})
