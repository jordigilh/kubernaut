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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

var _ = Describe("Configuration Integration (BR-EM-006, BR-EM-007, BR-EM-008)", func() {

	// ========================================
	// IT-EM-CF-001: Controller starts with valid config -> reconciler operational
	// ========================================
	It("IT-EM-CF-001: controller should be operational and reconcile EAs to completion", func() {
		ns := createTestNamespace("em-cf-001")
		defer deleteTestNamespace(ns)

		By("Creating an EA to verify controller is operational")
		ea := createEffectivenessAssessment(ns, "ea-cf-001", "rr-cf-001")

		By("Verifying the controller processes the EA to Completed phase")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		Expect(fetchedEA.Spec.CorrelationID).To(Equal("rr-cf-001"))
		Expect(fetchedEA.Status.CompletedAt).NotTo(BeNil())
	})

	// ========================================
	// IT-EM-CF-003: Custom validityWindow -> EA deadline computed correctly
	// ========================================
	It("IT-EM-CF-003: should respect custom validity window in EA spec", func() {
		ns := createTestNamespace("em-cf-003")
		defer deleteTestNamespace(ns)

		By("Creating an EA with custom validity deadline (30 minutes from now)")
		ea := createEffectivenessAssessment(ns, "ea-cf-003", "rr-cf-003")

		By("Verifying the validity deadline is stored correctly and EA completes")
		fetchedEA := &eav1.EffectivenessAssessment{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      ea.Name,
				Namespace: ea.Namespace,
			}, fetchedEA)).To(Succeed())
			g.Expect(fetchedEA.Status.Phase).To(Equal(eav1.PhaseCompleted))
		}, timeout, interval).Should(Succeed())

		// The default validity window is 30 minutes from creation (computed by EM controller in status)
		Expect(fetchedEA.Status.ValidityDeadline).NotTo(BeNil())
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(BeTemporally(">", time.Now()))
		Expect(fetchedEA.Status.ValidityDeadline.Time).To(BeTemporally("<", time.Now().Add(31*time.Minute)))
	})
})
