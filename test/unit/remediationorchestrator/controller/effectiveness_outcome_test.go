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

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
)

// ============================================================================
// SCORE-AWARE OUTCOME DERIVATION TESTS (Issue #722)
// Business Requirement: BR-EM-012 — alertScore=0 must NOT yield Remediated
//
// DeriveOutcomeFromEA logic:
//   alertAssessed && alertScore == 0 → "Inconclusive" (alert still firing)
//   alertAssessed && alertScore > 0  → "Remediated" (alert resolved)
//   !alertAssessed                   → "Remediated" (fail-open, AM unavailable)
// ============================================================================
var _ = Describe("Score-Aware Outcome Derivation (Issue #722, BR-EM-012)", func() {

	// UT-RO-722-001: alertScore=0 yields Inconclusive
	It("UT-RO-722-001: should return Inconclusive when alertAssessed=true and alertScore=0", func() {
		ea := &eav1.EffectivenessAssessment{
			Status: eav1.EffectivenessAssessmentStatus{
				Phase: eav1.PhaseCompleted,
				Components: eav1.EAComponents{
					AlertAssessed: true,
					AlertScore:    float64Ptr(0.0),
					HealthScore:   float64Ptr(0.75),
				},
			},
		}

		outcome := controller.DeriveOutcomeFromEA(ea)
		Expect(outcome).To(Equal("Inconclusive"),
			"Alert still firing (alertScore=0) must yield Inconclusive regardless of health score")
	})

	// UT-RO-722-002: alertScore>0 yields Remediated
	It("UT-RO-722-002: should return Remediated when alertAssessed=true and alertScore>0", func() {
		ea := &eav1.EffectivenessAssessment{
			Status: eav1.EffectivenessAssessmentStatus{
				Phase: eav1.PhaseCompleted,
				Components: eav1.EAComponents{
					AlertAssessed: true,
					AlertScore:    float64Ptr(1.0),
					HealthScore:   float64Ptr(0.5),
				},
			},
		}

		outcome := controller.DeriveOutcomeFromEA(ea)
		Expect(outcome).To(Equal("Remediated"),
			"Alert resolved (alertScore>0) must yield Remediated")
	})

	// UT-RO-722-003: !alertAssessed fails open to Remediated
	It("UT-RO-722-003: should return Remediated when alertAssessed=false (fail-open)", func() {
		ea := &eav1.EffectivenessAssessment{
			Status: eav1.EffectivenessAssessmentStatus{
				Phase: eav1.PhaseCompleted,
				Components: eav1.EAComponents{
					AlertAssessed: false,
					AlertScore:    nil,
					HealthScore:   float64Ptr(1.0),
				},
			},
		}

		outcome := controller.DeriveOutcomeFromEA(ea)
		Expect(outcome).To(Equal("Remediated"),
			"AM unavailable (alertAssessed=false) must fail-open to Remediated")
	})
})
