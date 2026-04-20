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
	"k8s.io/apimachinery/pkg/runtime"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
)

var _ = Describe("EffectivenessAssessment CRD Types (ADR-EM-001)", func() {

	// ========================================
	// Phase Constants
	// ========================================
	Describe("Phase Constants", func() {

		It("should define all six phases", func() {
			Expect(eav1.PhasePending).To(Equal("Pending"))
			Expect(eav1.PhaseWaitingForPropagation).To(Equal("WaitingForPropagation"))
			Expect(eav1.PhaseStabilizing).To(Equal("Stabilizing"))
			Expect(eav1.PhaseAssessing).To(Equal("Assessing"))
			Expect(eav1.PhaseCompleted).To(Equal("Completed"))
			Expect(eav1.PhaseFailed).To(Equal("Failed"))
		})
	})

	// ========================================
	// Assessment Reason Constants
	// ========================================
	Describe("AssessmentReason Constants", func() {

		It("should define all five assessment reasons", func() {
			Expect(eav1.AssessmentReasonFull).To(Equal("Full"))
			Expect(eav1.AssessmentReasonPartial).To(Equal("Partial"))
			Expect(eav1.AssessmentReasonNoExecution).To(Equal("NoExecution"))
			Expect(eav1.AssessmentReasonMetricsTimedOut).To(Equal("MetricsTimedOut"))
			Expect(eav1.AssessmentReasonExpired).To(Equal("Expired"))
		})

		// UT-EM-749-001: All AssessmentReason constants are PascalCase (BR-EM-749)
		It("UT-EM-749-001: all AssessmentReason constants must be PascalCase", func() {
			reasons := []string{
				eav1.AssessmentReasonFull,
				eav1.AssessmentReasonPartial,
				eav1.AssessmentReasonNoExecution,
				eav1.AssessmentReasonMetricsTimedOut,
				eav1.AssessmentReasonExpired,
				eav1.AssessmentReasonSpecDrift,
				eav1.AssessmentReasonAlertDecayTimeout,
			}

			for _, reason := range reasons {
				Expect(reason).To(MatchRegexp(`^[A-Z][a-zA-Z]+$`),
					"AssessmentReason %q must be PascalCase (no underscores, starts with uppercase)", reason)
			}
		})

		// UT-EM-749-002: Unrecoverable reason must be a PascalCase constant (BR-EM-749)
		It("UT-EM-749-002: unrecoverable reason must become PascalCase Unrecoverable", func() {
			Expect(eav1.AssessmentReasonUnrecoverable).To(Equal("Unrecoverable"))
		})
	})

	// ========================================
	// Scheme Registration (DD-CRD-001)
	// ========================================
	Describe("Scheme Registration (DD-CRD-001)", func() {

		It("should register with kubernaut.ai group", func() {
			Expect(eav1.GroupVersion.Group).To(Equal("kubernaut.ai"))
			Expect(eav1.GroupVersion.Version).To(Equal("v1alpha1"))
		})

		It("should register types with scheme", func() {
			s := runtime.NewScheme()
			err := eav1.AddToScheme(s)
			Expect(err).ToNot(HaveOccurred())

			// Verify the types are registered
			gvk := eav1.GroupVersion.WithKind("EffectivenessAssessment")
			obj, err := s.New(gvk)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).To(BeAssignableToTypeOf(&eav1.EffectivenessAssessment{}))

			listGVK := eav1.GroupVersion.WithKind("EffectivenessAssessmentList")
			listObj, err := s.New(listGVK)
			Expect(err).ToNot(HaveOccurred())
			Expect(listObj).To(BeAssignableToTypeOf(&eav1.EffectivenessAssessmentList{}))
		})
	})

	// ========================================
	// DeepCopy (generated)
	// ========================================
	Describe("DeepCopy", func() {

		It("should deep copy EffectivenessAssessment with all fields", func() {
			score := 0.85
			now := metav1.Now()

			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ea-test",
					Namespace: "default",
				},
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID: "rr-001",
					SignalTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "app",
						Namespace: "production",
					},
					RemediationTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "app",
						Namespace: "production",
					},
					Config: eav1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
					},
				},
				Status: eav1.EffectivenessAssessmentStatus{
					Phase: eav1.PhaseAssessing,
					Components: eav1.EAComponents{
						HealthAssessed: true,
						HealthScore:    &score,
					},
					AssessmentReason: eav1.AssessmentReasonPartial,
					CompletedAt:      &now,
					Message:          "assessment in progress",
				},
			}

			copy := ea.DeepCopy()

			// Verify deep copy
			Expect(copy.Name).To(Equal("ea-test"))
			Expect(copy.Spec.CorrelationID).To(Equal("rr-001"))
			Expect(copy.Spec.SignalTarget.Kind).To(Equal("Deployment"))
			Expect(copy.Status.Phase).To(Equal(eav1.PhaseAssessing))
			Expect(copy.Status.Components.HealthAssessed).To(BeTrue())
			Expect(*copy.Status.Components.HealthScore).To(Equal(0.85))

			// Verify independence - mutating original should not affect copy
			ea.Spec.CorrelationID = "rr-002"
			Expect(copy.Spec.CorrelationID).To(Equal("rr-001"))

			*ea.Status.Components.HealthScore = 0.5
			Expect(*copy.Status.Components.HealthScore).To(Equal(0.85))
		})

		It("should deep copy EffectivenessAssessmentList", func() {
			list := &eav1.EffectivenessAssessmentList{
				Items: []eav1.EffectivenessAssessment{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ea-1"},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "ea-2"},
					},
				},
			}

			copy := list.DeepCopy()
			Expect(copy.Items).To(HaveLen(2))
			Expect(copy.Items[0].Name).To(Equal("ea-1"))

			// Verify independence
			list.Items[0].Name = "ea-modified"
			Expect(copy.Items[0].Name).To(Equal("ea-1"))
		})
	})

	// ========================================
	// Spec structure validation
	// ========================================
	Describe("Spec Structure", func() {

		It("should construct a well-formed EA spec", func() {
			spec := eav1.EffectivenessAssessmentSpec{
				CorrelationID: "rr-test-001",
				SignalTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "my-app",
					Namespace: "production",
				},
				RemediationTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "my-app",
					Namespace: "production",
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
				},
			}

			Expect(spec.CorrelationID).To(Equal("rr-test-001"))
			Expect(spec.SignalTarget.Kind).To(Equal("Deployment"))
			Expect(spec.Config.StabilizationWindow.Duration).To(Equal(5 * time.Minute))
		})
	})

	// ========================================
	// Status structure validation
	// ========================================
	Describe("Status Structure", func() {

		It("should represent component status with scores", func() {
			healthScore := 1.0
			alertScore := 1.0
			metricsScore := 0.85

			components := eav1.EAComponents{
				HealthAssessed:          true,
				HealthScore:             &healthScore,
				HashComputed:            true,
				PostRemediationSpecHash: "abc123",
				AlertAssessed:           true,
				AlertScore:              &alertScore,
				MetricsAssessed:         true,
				MetricsScore:            &metricsScore,
			}

			Expect(components.HealthAssessed).To(BeTrue())
			Expect(*components.HealthScore).To(Equal(1.0))
			Expect(components.PostRemediationSpecHash).To(Equal("abc123"))
			Expect(*components.AlertScore).To(Equal(1.0))
			Expect(*components.MetricsScore).To(Equal(0.85))
		})

		It("should handle partially assessed components", func() {
			healthScore := 0.5
			components := eav1.EAComponents{
				HealthAssessed:  true,
				HealthScore:     &healthScore,
				HashComputed:    true,
				AlertAssessed:   false,
				AlertScore:      nil, // Not assessed
				MetricsAssessed: false,
				MetricsScore:    nil, // Not assessed
			}

			Expect(components.HealthAssessed).To(BeTrue())
			Expect(components.AlertAssessed).To(BeFalse())
			Expect(components.AlertScore).To(BeNil())
			Expect(components.MetricsAssessed).To(BeFalse())
			Expect(components.MetricsScore).To(BeNil())
		})
	})
})
