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
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
)

var _ = Describe("CheckHashDeferral (DD-EM-004, BR-EM-010.1)", func() {

	Describe("Async-managed target: hash deferred until external controller reconciles", func() {

		It("UT-EM-251-001: should defer hash computation when HashComputeAfter is in the future", func() {
			futureTime := metav1.NewTime(time.Now().Add(5 * time.Minute))
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					HashComputeAfter: &futureTime,
				},
			}

			result := hash.CheckHashDeferral(ea)

			By("deferring hash computation so the EM does not capture stale pre==post hash")
			Expect(result.ShouldDefer).To(BeTrue(),
				"BR-EM-010.1: EM must NOT compute hash while async target has not reconciled")

			By("providing a requeue duration matching the remaining time")
			Expect(result.RequeueAfter).To(BeNumerically(">", 4*time.Minute),
				"BR-EM-010.1: requeue must be approximately time.Until(HashComputeAfter)")
			Expect(result.RequeueAfter).To(BeNumerically("<=", 5*time.Minute),
				"BR-EM-010.1: requeue must not exceed the original deferral window")
		})
	})

	Describe("Sync target: hash computed immediately (backward compatible)", func() {

		It("UT-EM-251-002: should compute immediately when HashComputeAfter is in the past", func() {
			pastTime := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					HashComputeAfter: &pastTime,
				},
			}

			result := hash.CheckHashDeferral(ea)

			Expect(result.ShouldDefer).To(BeFalse(),
				"BR-EM-010.1: past timestamp means deferral window has elapsed")
			Expect(result.RequeueAfter).To(BeZero(),
				"BR-EM-010.1: no requeue needed when deferral window passed")
		})

		It("UT-EM-251-003: should compute immediately when HashComputeAfter is nil (backward compat)", func() {
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{},
			}

			result := hash.CheckHashDeferral(ea)

			Expect(result.ShouldDefer).To(BeFalse(),
				"BR-EM-010.1: nil HashComputeAfter preserves existing behavior for sync targets")
			Expect(result.RequeueAfter).To(BeZero(),
				"BR-EM-010.1: no requeue for sync targets")
		})

		It("UT-EM-251-004: should compute immediately when HashComputeAfter is zero time", func() {
			zeroTime := metav1.NewTime(time.Time{})
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					HashComputeAfter: &zeroTime,
				},
			}

			result := hash.CheckHashDeferral(ea)

			Expect(result.ShouldDefer).To(BeFalse(),
				"BR-EM-010.1: zero time treated as nil for backward compatibility")
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Describe("Requeue accuracy for different deferral windows", func() {

		It("UT-EM-251-005: short deferral (30s) should produce proportional requeue", func() {
			shortFuture := metav1.NewTime(time.Now().Add(30 * time.Second))
			ea := &eav1.EffectivenessAssessment{
				Spec: eav1.EffectivenessAssessmentSpec{
					HashComputeAfter: &shortFuture,
				},
			}

			result := hash.CheckHashDeferral(ea)

			Expect(result.ShouldDefer).To(BeTrue())
			Expect(result.RequeueAfter).To(BeNumerically(">", 25*time.Second),
				"BR-EM-010.1: requeue must track remaining time, not a fixed interval")
			Expect(result.RequeueAfter).To(BeNumerically("<=", 30*time.Second))
		})
	})
})
