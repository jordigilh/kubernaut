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

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
)

var _ = Describe("Phase State Machine (BR-EM-005)", func() {

	// ========================================
	// UT-EM-PH-001 through UT-EM-PH-005: Phase transitions
	// ========================================
	Describe("IsTerminal", func() {

		DescribeTable("terminal state detection",
			func(p phase.Phase, expectTerminal bool, reason string) {
				result := phase.IsTerminal(p)
				Expect(result).To(Equal(expectTerminal), reason)
			},
			// Terminal states
			Entry("UT-EM-PH-001: Completed is terminal",
				phase.Completed, true, "Completed indicates assessment finished"),
			Entry("UT-EM-PH-002: Failed is terminal",
				phase.Failed, true, "Failed indicates assessment could not be performed"),

			// Non-terminal states
			Entry("UT-EM-PH-003: Pending is NOT terminal",
				phase.Pending, false, "Pending indicates EA created but not reconciled"),
			Entry("UT-EM-PH-003b: Stabilizing is NOT terminal",
				phase.Stabilizing, false, "Stabilizing indicates stabilization window active"),
			Entry("UT-EM-PH-004: Assessing is NOT terminal",
				phase.Assessing, false, "Assessing indicates checks in progress"),

			// Edge cases
			Entry("Unknown phase is NOT terminal",
				phase.Phase("Unknown"), false, "Unknown phases should not be terminal"),
			Entry("Empty phase is NOT terminal",
				phase.Phase(""), false, "Empty phases should not be terminal"),
		)
	})

	Describe("CanTransition", func() {

		// UT-EM-PH-005: Valid transitions
		Context("valid transitions", func() {

			DescribeTable("allowed transitions",
				func(from, to phase.Phase) {
					Expect(phase.CanTransition(from, to)).To(BeTrue(),
						"transition %s -> %s should be allowed", from, to)
				},
				Entry("Pending -> Stabilizing", phase.Pending, phase.Stabilizing),
				Entry("Pending -> Assessing (no stabilization window)", phase.Pending, phase.Assessing),
				Entry("Pending -> Failed", phase.Pending, phase.Failed),
				Entry("Stabilizing -> Assessing", phase.Stabilizing, phase.Assessing),
				Entry("Stabilizing -> Failed", phase.Stabilizing, phase.Failed),
				Entry("Assessing -> Completed", phase.Assessing, phase.Completed),
				Entry("Assessing -> Failed", phase.Assessing, phase.Failed),
			)
		})

		// Invalid transitions
		Context("invalid transitions", func() {

			DescribeTable("rejected transitions",
				func(from, to phase.Phase) {
					Expect(phase.CanTransition(from, to)).To(BeFalse(),
						"transition %s -> %s should be rejected", from, to)
				},
				// Terminal states cannot transition
				Entry("Completed -> Pending", phase.Completed, phase.Pending),
				Entry("Completed -> Assessing", phase.Completed, phase.Assessing),
				Entry("Failed -> Pending", phase.Failed, phase.Pending),
				Entry("Failed -> Assessing", phase.Failed, phase.Assessing),

				// Cannot skip phases
				Entry("Pending -> Completed (skip Stabilizing+Assessing)", phase.Pending, phase.Completed),
				Entry("Stabilizing -> Completed (skip Assessing)", phase.Stabilizing, phase.Completed),

				// Cannot go backwards
				Entry("Assessing -> Pending (backwards)", phase.Assessing, phase.Pending),
				Entry("Assessing -> Stabilizing (backwards)", phase.Assessing, phase.Stabilizing),
				Entry("Stabilizing -> Pending (backwards)", phase.Stabilizing, phase.Pending),
			)
		})

		Context("edge cases", func() {
			It("should reject unknown source phase", func() {
				Expect(phase.CanTransition("Unknown", phase.Assessing)).To(BeFalse())
			})

			It("should reject empty source phase", func() {
				Expect(phase.CanTransition("", phase.Assessing)).To(BeFalse())
			})
		})
	})

	Describe("Validate", func() {

		DescribeTable("valid phases",
			func(p phase.Phase) {
				Expect(phase.Validate(p)).To(Succeed())
			},
			Entry("Pending", phase.Pending),
			Entry("Stabilizing", phase.Stabilizing),
			Entry("Assessing", phase.Assessing),
			Entry("Completed", phase.Completed),
			Entry("Failed", phase.Failed),
		)

		DescribeTable("invalid phases",
			func(p phase.Phase) {
				Expect(phase.Validate(p)).To(HaveOccurred())
			},
			Entry("Unknown", phase.Phase("Unknown")),
			Entry("Empty", phase.Phase("")),
			Entry("Processing (RO-only phase)", phase.Phase("Processing")),
		)
	})
})
