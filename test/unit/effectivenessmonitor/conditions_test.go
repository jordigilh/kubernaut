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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
)

// ============================================================================
// UT-EM-COND: EA Conditions Infrastructure Tests (DD-CRD-002)
// BR-EM-004: Spec drift detection via conditions
// ============================================================================

var _ = Describe("EA Conditions Infrastructure (DD-CRD-002)", func() {

	var ea *eav1.EffectivenessAssessment

	BeforeEach(func() {
		ea = &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-cond-test",
				Namespace: "test-ns",
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID: "rr-cond-test",
			},
		}
	})

	// ========================================
	// UT-EM-COND-001: Condition type constants exist
	// ========================================
	It("UT-EM-COND-001: should define AssessmentComplete and SpecIntegrity condition types", func() {
		Expect(conditions.ConditionAssessmentComplete).To(Equal("AssessmentComplete"))
		Expect(conditions.ConditionSpecIntegrity).To(Equal("SpecIntegrity"))
	})

	// ========================================
	// UT-EM-COND-002: Reason constants for AssessmentComplete
	// ========================================
	It("UT-EM-COND-002: should define all AssessmentComplete reason constants", func() {
		Expect(conditions.ReasonAssessmentFull).To(Equal("AssessmentFull"))
		Expect(conditions.ReasonAssessmentPartial).To(Equal("AssessmentPartial"))
		Expect(conditions.ReasonAssessmentExpired).To(Equal("AssessmentExpired"))
		Expect(conditions.ReasonSpecDrift).To(Equal("SpecDrift"))
		Expect(conditions.ReasonMetricsTimedOut).To(Equal("MetricsTimedOut"))
		Expect(conditions.ReasonNoExecution).To(Equal("NoExecution"))
	})

	// ========================================
	// UT-EM-COND-003: Reason constants for SpecIntegrity
	// ========================================
	It("UT-EM-COND-003: should define SpecIntegrity reason constants", func() {
		Expect(conditions.ReasonSpecUnchanged).To(Equal("SpecUnchanged"))
		Expect(conditions.ReasonSpecDrifted).To(Equal("SpecDrifted"))
	})

	// ========================================
	// UT-EM-COND-004: SetCondition sets a new condition
	// ========================================
	It("UT-EM-COND-004: should set a new condition on EA", func() {
		conditions.SetCondition(ea,
			conditions.ConditionSpecIntegrity,
			metav1.ConditionTrue,
			conditions.ReasonSpecUnchanged,
			"Target resource spec unchanged since post-remediation hash",
		)

		Expect(ea.Status.Conditions).To(HaveLen(1))
		c := ea.Status.Conditions[0]
		Expect(c.Type).To(Equal(conditions.ConditionSpecIntegrity))
		Expect(c.Status).To(Equal(metav1.ConditionTrue))
		Expect(c.Reason).To(Equal(conditions.ReasonSpecUnchanged))
		Expect(c.Message).To(ContainSubstring("unchanged"))
	})

	// ========================================
	// UT-EM-COND-005: SetCondition updates existing condition
	// ========================================
	It("UT-EM-COND-005: should update an existing condition", func() {
		// Set initial condition
		conditions.SetCondition(ea,
			conditions.ConditionSpecIntegrity,
			metav1.ConditionTrue,
			conditions.ReasonSpecUnchanged,
			"Unchanged",
		)
		Expect(ea.Status.Conditions).To(HaveLen(1))

		// Update to SpecDrifted
		conditions.SetCondition(ea,
			conditions.ConditionSpecIntegrity,
			metav1.ConditionFalse,
			conditions.ReasonSpecDrifted,
			"Target spec hash changed: sha256:abc -> sha256:def",
		)

		// Should still be 1 condition (updated, not appended)
		Expect(ea.Status.Conditions).To(HaveLen(1))
		c := ea.Status.Conditions[0]
		Expect(c.Status).To(Equal(metav1.ConditionFalse))
		Expect(c.Reason).To(Equal(conditions.ReasonSpecDrifted))
		Expect(c.Message).To(ContainSubstring("sha256:abc"))
	})

	// ========================================
	// UT-EM-COND-006: GetCondition retrieves existing condition
	// ========================================
	It("UT-EM-COND-006: should retrieve an existing condition by type", func() {
		conditions.SetCondition(ea,
			conditions.ConditionAssessmentComplete,
			metav1.ConditionTrue,
			conditions.ReasonAssessmentFull,
			"All components assessed",
		)

		c := conditions.GetCondition(ea, conditions.ConditionAssessmentComplete)
		Expect(c).NotTo(BeNil())
		Expect(c.Reason).To(Equal(conditions.ReasonAssessmentFull))
	})

	// ========================================
	// UT-EM-COND-007: GetCondition returns nil for missing condition
	// ========================================
	It("UT-EM-COND-007: should return nil for a missing condition", func() {
		c := conditions.GetCondition(ea, conditions.ConditionAssessmentComplete)
		Expect(c).To(BeNil())
	})

	// ========================================
	// UT-EM-COND-008: IsConditionTrue returns true for True condition
	// ========================================
	It("UT-EM-COND-008: should return true when condition exists and is True", func() {
		conditions.SetCondition(ea,
			conditions.ConditionSpecIntegrity,
			metav1.ConditionTrue,
			conditions.ReasonSpecUnchanged,
			"OK",
		)
		Expect(conditions.IsConditionTrue(ea, conditions.ConditionSpecIntegrity)).To(BeTrue())
	})

	// ========================================
	// UT-EM-COND-009: IsConditionTrue returns false for False or missing
	// ========================================
	It("UT-EM-COND-009: should return false when condition is False or missing", func() {
		// Missing condition
		Expect(conditions.IsConditionTrue(ea, conditions.ConditionSpecIntegrity)).To(BeFalse())

		// Set to False
		conditions.SetCondition(ea,
			conditions.ConditionSpecIntegrity,
			metav1.ConditionFalse,
			conditions.ReasonSpecDrifted,
			"Drifted",
		)
		Expect(conditions.IsConditionTrue(ea, conditions.ConditionSpecIntegrity)).To(BeFalse())
	})

	// ========================================
	// UT-EM-COND-010: Multiple conditions coexist
	// ========================================
	It("UT-EM-COND-010: should support multiple conditions on the same EA", func() {
		conditions.SetCondition(ea,
			conditions.ConditionSpecIntegrity,
			metav1.ConditionTrue,
			conditions.ReasonSpecUnchanged,
			"Spec OK",
		)
		conditions.SetCondition(ea,
			conditions.ConditionAssessmentComplete,
			metav1.ConditionTrue,
			conditions.ReasonAssessmentFull,
			"All done",
		)

		Expect(ea.Status.Conditions).To(HaveLen(2))
		Expect(conditions.IsConditionTrue(ea, conditions.ConditionSpecIntegrity)).To(BeTrue())
		Expect(conditions.IsConditionTrue(ea, conditions.ConditionAssessmentComplete)).To(BeTrue())
	})

	// ========================================
	// UT-EM-COND-011: AssessmentReasonSpecDrift constant exists
	// ========================================
	It("UT-EM-COND-011: should define AssessmentReasonSpecDrift in EA types", func() {
		Expect(eav1.AssessmentReasonSpecDrift).To(Equal("spec_drift"))
	})
})
