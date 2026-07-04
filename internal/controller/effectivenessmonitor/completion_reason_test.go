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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
)

func TestEffectivenessMonitorCompletion(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EffectivenessMonitor Completion Reason Test Suite")
}

func float64Ptr(f float64) *float64 { return &f }

// Characterization tests written before the Wave E complexity remediation of
// determineAssessmentReason (gocyclo finding), pinning current behavior since
// this function previously had zero test coverage.
var _ = Describe("determineAssessmentReason", func() {
	var r *Reconciler

	newEA := func(components eav1.EAComponents, validityDeadline *metav1.Time) *eav1.EffectivenessAssessment {
		return &eav1.EffectivenessAssessment{
			Status: eav1.EffectivenessAssessmentStatus{
				Components:       components,
				ValidityDeadline: validityDeadline,
			},
		}
	}

	pastDeadline := func() *metav1.Time {
		t := metav1.NewTime(time.Now().Add(-1 * time.Hour))
		return &t
	}
	futureDeadline := func() *metav1.Time {
		t := metav1.NewTime(time.Now().Add(1 * time.Hour))
		return &t
	}

	BeforeEach(func() {
		r = &Reconciler{
			Config: ReconcilerConfig{
				PrometheusEnabled:   true,
				AlertManagerEnabled: true,
			},
			validityChecker: validity.NewChecker(),
		}
	})

	It("returns Full when health, hash, alert, and metrics are all assessed", func() {
		ea := newEA(eav1.EAComponents{
			HealthAssessed:  true,
			HashComputed:    true,
			AlertAssessed:   true,
			MetricsAssessed: true,
		}, futureDeadline())
		Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonFull))
	})

	It("returns Full when alert/metrics are not assessed but their checks are disabled", func() {
		r.Config.AlertManagerEnabled = false
		r.Config.PrometheusEnabled = false
		ea := newEA(eav1.EAComponents{
			HealthAssessed: true,
			HashComputed:   true,
		}, futureDeadline())
		Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonFull))
	})

	Context("when the validity deadline has passed", func() {
		It("returns AlertDecayTimeout when alert decay retries occurred but alert was never assessed", func() {
			ea := newEA(eav1.EAComponents{
				HealthAssessed:    true,
				HashComputed:      true,
				AlertAssessed:     false,
				AlertDecayRetries: 3,
			}, pastDeadline())
			Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonAlertDecayTimeout))
		})

		It("returns MetricsTimedOut when only metrics failed to complete before expiry", func() {
			ea := newEA(eav1.EAComponents{
				HealthAssessed:  true,
				HashComputed:    true,
				AlertAssessed:   true,
				MetricsAssessed: false,
			}, pastDeadline())
			Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonMetricsTimedOut))
		})

		It("returns Partial when some but not all components were assessed and neither special timeout applies", func() {
			ea := newEA(eav1.EAComponents{
				HealthAssessed: true,
				HashComputed:   false,
			}, pastDeadline())
			Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonPartial))
		})

		It("returns Expired when no components were assessed at all", func() {
			ea := newEA(eav1.EAComponents{}, pastDeadline())
			Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonExpired))
		})
	})

	Context("when the validity deadline has not passed (or is unset)", func() {
		It("returns Partial when some components were assessed", func() {
			ea := newEA(eav1.EAComponents{HealthAssessed: true}, futureDeadline())
			Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonPartial))
		})

		It("returns Expired when no components were assessed and there is no validity deadline yet", func() {
			ea := newEA(eav1.EAComponents{}, nil)
			Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonExpired))
		})
	})

	It("treats HealthScore/AlertScore pointer fields as irrelevant to the reason decision", func() {
		ea := newEA(eav1.EAComponents{
			HealthAssessed:  true,
			HealthScore:     float64Ptr(0.5),
			HashComputed:    true,
			AlertAssessed:   true,
			AlertScore:      float64Ptr(1.0),
			MetricsAssessed: true,
		}, futureDeadline())
		Expect(r.determineAssessmentReason(ea)).To(Equal(eav1.AssessmentReasonFull))
	})
})
