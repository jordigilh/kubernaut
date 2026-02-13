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
		// Hash should be a hex-encoded SHA256 (64 chars)
		Expect(len(fetchedEA.Status.Components.PostRemediationSpecHash)).To(Equal(64),
			"hash should be 64 character hex (SHA256)")
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
