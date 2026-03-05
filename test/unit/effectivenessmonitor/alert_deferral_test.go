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
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/alert"
)

// ========================================
// Alert Deferral Tests (Issue #277, BR-EM-009)
// ========================================
//
// AlertCheckDelay causes the EM to defer alert resolution checks until
// AlertManagerCheckAfter (persisted in EA status). This allows proactive
// (predictive) alerts like predict_linear to resolve before being checked.
//
// Unlike hash deferral (which blocks ALL checks), alert deferral only
// blocks alert assessment — health and metrics proceed independently.

var _ = Describe("CheckAlertDeferral (#277, BR-EM-009)", func() {

	Describe("Alert deferred when AlertManagerCheckAfter is in the future", func() {

		It("UT-EM-277-010: should defer alert assessment when check time has not arrived", func() {
			futureTime := metav1.NewTime(time.Now().Add(5 * time.Minute))
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					AlertManagerCheckAfter: &futureTime,
				},
			}

			result := alert.CheckAlertDeferral(ea)

			Expect(result.ShouldDefer).To(BeTrue(),
				"BR-EM-009: alert assessment must be deferred until AlertManagerCheckAfter")
			Expect(result.RequeueAfter).To(BeNumerically(">", 4*time.Minute),
				"BR-EM-009: requeue must be approximately time.Until(AlertManagerCheckAfter)")
			Expect(result.RequeueAfter).To(BeNumerically("<=", 5*time.Minute),
				"BR-EM-009: requeue must not exceed the deferral window")
		})
	})

	Describe("Alert proceeds when AlertManagerCheckAfter is in the past", func() {

		It("UT-EM-277-011: should proceed with alert assessment when check time has elapsed", func() {
			pastTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					AlertManagerCheckAfter: &pastTime,
				},
			}

			result := alert.CheckAlertDeferral(ea)

			Expect(result.ShouldDefer).To(BeFalse(),
				"BR-EM-009: alert check time has passed, proceed immediately")
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Describe("Alert proceeds when AlertManagerCheckAfter is nil (no delay)", func() {

		It("UT-EM-277-012: should proceed immediately when nil (reactive signal, backward compat)", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{},
			}

			result := alert.CheckAlertDeferral(ea)

			Expect(result.ShouldDefer).To(BeFalse(),
				"BR-EM-009: nil AlertManagerCheckAfter means no alert-specific delay")
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Describe("Short deferral accuracy", func() {

		It("UT-EM-277-013: short deferral (30s) should produce proportional requeue", func() {
			shortFuture := metav1.NewTime(time.Now().Add(30 * time.Second))
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					AlertManagerCheckAfter: &shortFuture,
				},
			}

			result := alert.CheckAlertDeferral(ea)

			Expect(result.ShouldDefer).To(BeTrue())
			Expect(result.RequeueAfter).To(BeNumerically(">", 25*time.Second),
				"BR-EM-009: requeue must track remaining time")
			Expect(result.RequeueAfter).To(BeNumerically("<=", 30*time.Second))
		})
	})
})
