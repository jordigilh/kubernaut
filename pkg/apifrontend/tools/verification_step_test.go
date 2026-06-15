package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func float64Ptr(f float64) *float64 { return &f }

var _ = Describe("DiffEASteps — verification_step event mapping (#1427)", func() {

	It("UT-AF-1427-001: detects health_check completion with step_status and detail", func() {
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
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusCompleted))
		Expect(steps[0].Data["detail"]).To(ContainSubstring("Health check completed"))
	})

	It("UT-AF-1427-002: detects spec_hash_computed with truncated hash in detail", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					HashComputed:            true,
					PostRemediationSpecHash: "4a6f72646947696c68cafb",
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("spec_hash_computed"))
		Expect(steps[0].Data["post_remediation_spec_hash"]).To(Equal("4a6f72646947696c68cafb"))
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusCompleted))
		Expect(steps[0].Data["detail"]).To(ContainSubstring("4a6f72646947"))
	})

	It("UT-AF-1427-003: detects alert_check completion with step_status and score", func() {
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
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusCompleted))
		Expect(steps[0].Data["detail"]).To(ContainSubstring("completed"))
	})

	It("UT-AF-1427-004: detects multiple steps in single diff including stabilization_elapsed", func() {
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
		Expect(stepNames).To(ContainElements("stabilization_elapsed", "spec_hash_computed", "health_check"))
		for _, s := range steps {
			Expect(s.Data).To(HaveKey("step_status"))
			Expect(s.Data).To(HaveKey("detail"))
		}
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
		Expect(stepNames).To(ContainElements("stabilization_elapsed", "health_check", "alert_check"))
	})

	It("UT-AF-1427-007: alert decay retry emits alert_check with in_progress status", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:      "Assessing",
				Components: eav1alpha1.EAComponents{AlertDecayRetries: 1},
			},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Spec: eav1alpha1.EffectivenessAssessmentSpec{
				SignalName: "KubePodCrashLooping",
			},
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:      "Assessing",
				Components: eav1alpha1.EAComponents{AlertDecayRetries: 2},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("alert_check"))
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusInProgress))
		Expect(steps[0].Data["retry_count"]).To(Equal(int32(2)))
		Expect(steps[0].Data["detail"]).To(ContainSubstring("KubePodCrashLooping"))
	})

	It("UT-AF-1427-008: detects metrics_check completion with step_status", func() {
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
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusCompleted))
		Expect(steps[0].Data["detail"]).To(ContainSubstring("Metrics comparison completed"))
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

	It("UT-AF-1427-010: stabilization_elapsed emitted for Stabilizing->Assessing transition", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Stabilizing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("stabilization_elapsed"))
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusCompleted))
		Expect(steps[0].Message).To(Equal("Stabilization window elapsed"))
	})

	It("UT-AF-1427-011: phase_transition emitted for non-stabilization transitions", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Completed"},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("phase_transition"))
		Expect(steps[0].Data["phase"]).To(Equal("Completed"))
		Expect(steps[0].Data["previous_phase"]).To(Equal("Assessing"))
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusCompleted))
	})

	It("UT-AF-1427-012: Failed phase sets step_status to failed", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Failed"},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Step).To(Equal("phase_transition"))
		Expect(steps[0].Data["step_status"]).To(Equal(tools.StepStatusFailed))
	})

	It("UT-AF-1427-013: alert decay retry suppressed when AlertAssessed is already true", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					AlertDecayRetries: 2,
				},
			},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					AlertDecayRetries: 3,
					AlertAssessed:     true,
					AlertScore:        float64Ptr(1.0),
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		stepNames := make([]string, len(steps))
		statuses := make([]string, len(steps))
		for i, s := range steps {
			stepNames[i] = s.Step
			statuses[i] = s.Data["step_status"].(string)
		}
		Expect(stepNames).To(Equal([]string{"alert_check"}))
		Expect(statuses).To(Equal([]string{tools.StepStatusCompleted}))
	})

	It("UT-AF-1427-014: alert decay without signal name uses generic detail", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:      "Assessing",
				Components: eav1alpha1.EAComponents{AlertDecayRetries: 0},
			},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:      "Assessing",
				Components: eav1alpha1.EAComponents{AlertDecayRetries: 1},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Data["detail"]).To(Equal("Waiting for alert to clear (retry 1)"))
	})

	It("UT-AF-1427-015: spec_hash_computed without hash has generic detail", func() {
		prev := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Assessing"},
		}
		curr := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
				Components: eav1alpha1.EAComponents{
					HashComputed: true,
				},
			},
		}

		steps := tools.DiffEASteps(prev, curr)
		Expect(steps).To(HaveLen(1))
		Expect(steps[0].Data["detail"]).To(Equal("Spec hash comparison completed"))
		Expect(steps[0].Data).NotTo(HaveKey("post_remediation_spec_hash"))
	})
})
