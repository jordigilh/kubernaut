package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func float64Ptr(f float64) *float64 { return &f }

var _ = Describe("DiffEASteps — verification_step event mapping (#1427)", func() {

	It("UT-AF-1427-001: detects health_check completion", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:      "Assessing",
				Components: eav1alpha1.EAComponents{},
			},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					HealthAssessed: true,
					HealthScore:    float64Ptr(1.0),
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("health_check"))
		Expect(steps[0].Data["score"]).To(Equal(1.0))
	})

	It("UT-AF-1427-002: detects spec_hash_computed", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					HashComputed:            true,
					PostRemediationSpecHash: "abc123",
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("spec_hash_computed"))
		Expect(steps[0].Data["post_remediation_spec_hash"]).To(Equal("abc123"))
	})

	It("UT-AF-1427-003: detects alert_check completion with score", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					AlertAssessed: true,
					AlertScore:    float64Ptr(1.0),
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("alert_check"))
		Expect(steps[0].Data["score"]).To(Equal(1.0))
	})

	It("UT-AF-1427-004: detects multiple steps in single diff", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Stabilizing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					HashComputed:   true,
					HealthAssessed: true,
					HealthScore:    float64Ptr(0.8),
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		stepNames := make([]string, len(steps))
		for i, s := range steps {
			stepNames[i] = s.Step
		}
		Expect(stepNames).To(ContainElements("phase_transition", "spec_hash_computed", "health_check"))
	})

	It("UT-AF-1427-005: returns nil for nil curr", func() {
		steps := tools.DiffEASteps(nil, nil)
		Expect(steps).To(BeNil())
	})

	It("UT-AF-1427-006: nil prev reports all non-zero fields in curr", func() {
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					HealthAssessed: true,
					HealthScore:    float64Ptr(1.0),
					AlertAssessed:  true,
					AlertScore:     float64Ptr(1.0),
				},
			},
		}

		steps := tools.DiffEASteps(nil, curr)
		stepNames := make([]string, len(steps))
		for i, s := range steps {
			stepNames[i] = s.Step
		}
		Expect(stepNames).To(ContainElements("phase_transition", "health_check", "alert_check"))
	})

	It("UT-AF-1427-007: detects alert_decay_retry increment", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:      "Assessing",
				Components: eav1alpha1.EAComponents{AlertDecayRetries: 1},
			},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:      "Assessing",
				Components: eav1alpha1.EAComponents{AlertDecayRetries: 2},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("alert_decay_retry"))
		Expect(steps[0].Data["retry_count"]).To(Equal(int32(2)))
	})

	It("UT-AF-1427-008: detects metrics_check completion", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					MetricsAssessed: true,
					MetricsScore:    float64Ptr(0.95),
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("metrics_check"))
		Expect(steps[0].Data["score"]).To(Equal(0.95))
	})

	It("UT-AF-1427-009: no steps when no change", func() {
		ea := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					HealthAssessed: true,
					HealthScore:    float64Ptr(1.0),
				},
			},
		}

		steps := tools.DiffEASteps(ea, ea)
		Expect(steps).To(BeEmpty())
	})
})
