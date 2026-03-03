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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
)

// ========================================
// Issue #253: WaitingForPropagation Phase Tests
// ========================================
//
// Business Requirements:
// - BR-EM-010.3: WaitingForPropagation phase for async-managed targets
//
// Design Document:
// - DD-EM-004 v2.0: Phase diagram (async target path)

var _ = Describe("WaitingForPropagation Phase (#253, BR-EM-010.3)", func() {

	// ========================================
	// UT-EM-253-001: WaitingForPropagation is a valid phase
	// ========================================
	Describe("UT-EM-253-001: WaitingForPropagation is a valid phase", Label("UT-EM-253-001"), func() {

		It("should be accepted by Validate()", func() {
			Expect(phase.Validate(phase.WaitingForPropagation)).To(Succeed(),
				"WaitingForPropagation must be a recognized valid phase")
		})

		It("should NOT be terminal", func() {
			Expect(phase.IsTerminal(phase.WaitingForPropagation)).To(BeFalse(),
				"WaitingForPropagation is a transient phase, not terminal")
		})

		It("should allow Pending → WaitingForPropagation", func() {
			Expect(phase.CanTransition(phase.Pending, phase.WaitingForPropagation)).To(BeTrue(),
				"async targets enter WaitingForPropagation from Pending")
		})

		It("should have the correct API constant value", func() {
			Expect(eav1.PhaseWaitingForPropagation).To(Equal("WaitingForPropagation"))
		})

		It("should be re-exported in the phase package", func() {
			Expect(phase.WaitingForPropagation).To(Equal(eav1.PhaseWaitingForPropagation))
		})
	})

	// ========================================
	// UT-EM-253-002: WaitingForPropagation transition rules
	// ========================================
	Describe("UT-EM-253-002: WaitingForPropagation transition rules", Label("UT-EM-253-002"), func() {

		DescribeTable("allowed transitions FROM WaitingForPropagation",
			func(target phase.Phase) {
				Expect(phase.CanTransition(phase.WaitingForPropagation, target)).To(BeTrue(),
					"WaitingForPropagation → %s should be allowed", target)
			},
			Entry("→ Stabilizing (normal path: propagation complete, hash computed)", phase.Stabilizing),
			Entry("→ Failed (error path: target not found, timeout)", phase.Failed),
		)

		DescribeTable("forbidden transitions FROM WaitingForPropagation",
			func(target phase.Phase) {
				Expect(phase.CanTransition(phase.WaitingForPropagation, target)).To(BeFalse(),
					"WaitingForPropagation → %s should be forbidden", target)
			},
			Entry("→ Assessing (must go through Stabilizing first)", phase.Assessing),
			Entry("→ Completed (must go through Stabilizing+Assessing)", phase.Completed),
			Entry("→ Pending (no backwards transition)", phase.Pending),
		)
	})

	// ========================================
	// EA Spec: New propagation delay fields (BR-EM-010.5)
	// ========================================
	Describe("EA Spec: propagation delay fields (BR-EM-010.5)", func() {

		It("should accept GitOpsSyncDelay and OperatorReconcileDelay in spec", func() {
			gitOpsDelay := metav1.Duration{Duration: 3 * time.Minute}
			operatorDelay := metav1.Duration{Duration: 1 * time.Minute}

			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-test",
					RemediationRequestPhase: "Completed",
					SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "app", Namespace: "default"},
					RemediationTarget:       eav1.TargetResource{Kind: "Certificate", Name: "cert", Namespace: "default"},
					Config:                  eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute}},
					GitOpsSyncDelay:         &gitOpsDelay,
					OperatorReconcileDelay:  &operatorDelay,
				},
			}

			Expect(ea.Spec.GitOpsSyncDelay.Duration).To(Equal(3 * time.Minute))
			Expect(ea.Spec.OperatorReconcileDelay.Duration).To(Equal(1 * time.Minute))
		})

		It("should allow nil delay fields for sync targets", func() {
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-sync",
					RemediationRequestPhase: "Completed",
					SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "app", Namespace: "default"},
					RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "app", Namespace: "default"},
					Config:                  eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute}},
				},
			}

			Expect(ea.Spec.GitOpsSyncDelay).To(BeNil())
			Expect(ea.Spec.OperatorReconcileDelay).To(BeNil())
		})

		It("should deep copy propagation delay fields", func() {
			gitOpsDelay := metav1.Duration{Duration: 3 * time.Minute}
			operatorDelay := metav1.Duration{Duration: 1 * time.Minute}

			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-deep",
					RemediationRequestPhase: "Completed",
					SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "app", Namespace: "default"},
					RemediationTarget:       eav1.TargetResource{Kind: "Certificate", Name: "cert", Namespace: "default"},
					Config:                  eav1.EAConfig{StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute}},
					GitOpsSyncDelay:         &gitOpsDelay,
					OperatorReconcileDelay:  &operatorDelay,
				},
			}

			copy := ea.DeepCopy()
			Expect(copy.Spec.GitOpsSyncDelay.Duration).To(Equal(3 * time.Minute))
			Expect(copy.Spec.OperatorReconcileDelay.Duration).To(Equal(1 * time.Minute))

			// Verify independence
			ea.Spec.GitOpsSyncDelay.Duration = 10 * time.Minute
			Expect(copy.Spec.GitOpsSyncDelay.Duration).To(Equal(3 * time.Minute))
		})
	})
})
