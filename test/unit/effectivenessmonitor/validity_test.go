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

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
)

var _ = Describe("Validity Window (BR-EM-006, BR-EM-007)", func() {

	var checker validity.Checker

	BeforeEach(func() {
		checker = validity.NewChecker()
	})

	// ========================================
	// UT-EM-VW-001: Stabilization window active
	// ========================================
	Describe("Check (UT-EM-VW-001)", func() {

		Context("stabilizing state", func() {

			It("UT-EM-VW-001: should return Stabilizing when within stabilization window", func() {
				now := time.Now()
				creationTime := now.Add(-1 * time.Minute)      // Created 1 min ago
				stabilizationWindow := 5 * time.Minute          // 5 min stabilization
				validityDeadline := now.Add(29 * time.Minute)   // 30 min validity

				state := checker.Check(creationTime, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(validity.WindowStabilizing))
			})

			It("should return Stabilizing at exact creation time", func() {
				now := time.Now()
				stabilizationWindow := 5 * time.Minute
				validityDeadline := now.Add(30 * time.Minute)

				state := checker.Check(now, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(validity.WindowStabilizing))
			})
		})

		// UT-EM-VW-002: Assessment window active
		Context("active state", func() {

			It("UT-EM-VW-002: should return Active when stabilization passed but within validity", func() {
				now := time.Now()
				creationTime := now.Add(-10 * time.Minute)     // Created 10 min ago
				stabilizationWindow := 5 * time.Minute          // 5 min stabilization (passed)
				validityDeadline := now.Add(20 * time.Minute)   // Still within validity

				state := checker.Check(creationTime, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(validity.WindowActive))
			})

			It("should return Active at exact stabilization boundary", func() {
				now := time.Now()
				creationTime := now.Add(-5 * time.Minute)      // Exactly at stabilization boundary
				stabilizationWindow := 5 * time.Minute
				validityDeadline := now.Add(25 * time.Minute)

				state := checker.Check(creationTime, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(validity.WindowActive))
			})
		})

		// UT-EM-VW-003: Validity window expired
		Context("expired state", func() {

			It("UT-EM-VW-003: should return Expired when past validity deadline", func() {
				now := time.Now()
				creationTime := now.Add(-60 * time.Minute)     // Created 60 min ago
				stabilizationWindow := 5 * time.Minute
				validityDeadline := now.Add(-1 * time.Minute)   // Expired 1 min ago

				state := checker.Check(creationTime, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(validity.WindowExpired))
			})

			It("should return Expired at exact validity deadline", func() {
				now := time.Now()
				creationTime := now.Add(-30 * time.Minute)
				stabilizationWindow := 5 * time.Minute
				validityDeadline := now // Exactly at deadline

				state := checker.Check(creationTime, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(validity.WindowExpired))
			})
		})

		// Edge case: validity expired before stabilization
		Context("edge case: validity < stabilization", func() {
			It("should return Expired even if stabilization hasn't passed when validity is expired", func() {
				now := time.Now()
				creationTime := now.Add(-1 * time.Minute)      // Created 1 min ago
				stabilizationWindow := 5 * time.Minute          // Not yet stabilized
				validityDeadline := now.Add(-1 * time.Second)   // But already expired

				state := checker.Check(creationTime, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(validity.WindowExpired))
			})
		})
	})

	// ========================================
	// TimeUntilStabilized
	// ========================================
	Describe("TimeUntilStabilized", func() {

		It("should return remaining time when still stabilizing", func() {
			now := time.Now()
			creationTime := now.Add(-1 * time.Minute)
			stabilizationWindow := 5 * time.Minute

			remaining := checker.TimeUntilStabilized(creationTime, stabilizationWindow)
			// Should be approximately 4 minutes
			Expect(remaining).To(BeNumerically("~", 4*time.Minute, 1*time.Second))
		})

		It("should return 0 when already stabilized", func() {
			now := time.Now()
			creationTime := now.Add(-10 * time.Minute)
			stabilizationWindow := 5 * time.Minute

			remaining := checker.TimeUntilStabilized(creationTime, stabilizationWindow)
			Expect(remaining).To(Equal(time.Duration(0)))
		})
	})

	// ========================================
	// TimeUntilExpired
	// ========================================
	Describe("TimeUntilExpired", func() {

		It("should return remaining time when not yet expired", func() {
			validityDeadline := time.Now().Add(10 * time.Minute)

			remaining := checker.TimeUntilExpired(validityDeadline)
			Expect(remaining).To(BeNumerically("~", 10*time.Minute, 1*time.Second))
		})

		It("should return 0 when already expired", func() {
			validityDeadline := time.Now().Add(-5 * time.Minute)

			remaining := checker.TimeUntilExpired(validityDeadline)
			Expect(remaining).To(Equal(time.Duration(0)))
		})
	})

	// ========================================
	// WindowState.String()
	// ========================================
	Describe("WindowState.String()", func() {

		DescribeTable("string representation",
			func(state validity.WindowState, expected string) {
				Expect(state.String()).To(Equal(expected))
			},
			Entry("Stabilizing", validity.WindowStabilizing, "Stabilizing"),
			Entry("Active", validity.WindowActive, "Active"),
			Entry("Expired", validity.WindowExpired, "Expired"),
		)
	})
})
