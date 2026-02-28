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
)

// ============================================================================
// EA DUAL-TARGET TYPE TESTS (Issue #188, DD-EM-003)
// Business Requirement: BR-EM-004 (spec hash drift detection with correct target)
//
// Validates that the EA CRD spec carries separate SignalTarget and
// RemediationTarget fields, replacing the single TargetResource field.
// ============================================================================
var _ = Describe("EA Dual-Target Types (DD-EM-003)", func() {

	// UT-EM-188-001: EA spec has SignalTarget field of type TargetResource
	It("UT-EM-188-001: EA spec should have SignalTarget field", func() {
		spec := eav1.EffectivenessAssessmentSpec{
			CorrelationID:           "rr-hpa-001",
			RemediationRequestPhase: "Completed",
			SignalTarget: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "api-frontend",
				Namespace: "demo-hpa",
			},
			RemediationTarget: eav1.TargetResource{
				Kind:      "HorizontalPodAutoscaler",
				Name:      "api-frontend",
				Namespace: "demo-hpa",
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 30 * time.Second},
			},
		}

		Expect(spec.SignalTarget.Kind).To(Equal("Deployment"))
		Expect(spec.SignalTarget.Name).To(Equal("api-frontend"))
		Expect(spec.SignalTarget.Namespace).To(Equal("demo-hpa"))
	})

	// UT-EM-188-002: EA spec has RemediationTarget field of type TargetResource
	It("UT-EM-188-002: EA spec should have RemediationTarget field", func() {
		spec := eav1.EffectivenessAssessmentSpec{
			CorrelationID:           "rr-hpa-001",
			RemediationRequestPhase: "Completed",
			SignalTarget: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "api-frontend",
				Namespace: "demo-hpa",
			},
			RemediationTarget: eav1.TargetResource{
				Kind:      "HorizontalPodAutoscaler",
				Name:      "api-frontend",
				Namespace: "demo-hpa",
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 30 * time.Second},
			},
		}

		Expect(spec.RemediationTarget.Kind).To(Equal("HorizontalPodAutoscaler"))
		Expect(spec.RemediationTarget.Name).To(Equal("api-frontend"))
		Expect(spec.RemediationTarget.Namespace).To(Equal("demo-hpa"))
	})

	// UT-EM-188-003: SignalTarget and RemediationTarget can differ (hpa-maxed scenario)
	It("UT-EM-188-003: signal and remediation targets should be independent", func() {
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-hpa-001",
				Namespace: "demo-hpa",
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-hpa-001",
				RemediationRequestPhase: "Completed",
				SignalTarget: eav1.TargetResource{
					Kind:      "Deployment",
					Name:      "api-frontend",
					Namespace: "demo-hpa",
				},
				RemediationTarget: eav1.TargetResource{
					Kind:      "HorizontalPodAutoscaler",
					Name:      "api-frontend",
					Namespace: "demo-hpa",
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 30 * time.Second},
				},
			},
		}

		Expect(ea.Spec.SignalTarget.Kind).ToNot(Equal(ea.Spec.RemediationTarget.Kind),
			"hpa-maxed: signal target (Deployment) should differ from remediation target (HPA)")

		copy := ea.DeepCopy()
		Expect(copy.Spec.SignalTarget.Kind).To(Equal("Deployment"))
		Expect(copy.Spec.RemediationTarget.Kind).To(Equal("HorizontalPodAutoscaler"))

		ea.Spec.SignalTarget.Name = "mutated"
		Expect(copy.Spec.SignalTarget.Name).To(Equal("api-frontend"),
			"deep copy should be independent")
	})
})
