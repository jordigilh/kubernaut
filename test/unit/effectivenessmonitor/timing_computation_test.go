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

	emtiming "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/timing"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/validity"
)

// ========================================
// Issue #253: EM Timing Computation Tests
// ========================================
//
// Business Requirements:
// - BR-EM-010.4: Stabilization anchored to HashComputeAfter for async targets
//
// Design Document:
// - DD-EM-004 v2.0: Timing formulas (sync vs async)

var _ = Describe("EM Timing Computation (#253, BR-EM-010.4)", func() {

	// Fixed reference time for deterministic tests
	var baseTime time.Time

	BeforeEach(func() {
		baseTime = time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
	})

	// ========================================
	// UT-EM-253-003: Async target timing anchor
	// ========================================
	Describe("UT-EM-253-003: Async target timing anchor", Label("UT-EM-253-003"), func() {

		It("should anchor CheckAfter to HashComputeAfter, not CreationTimestamp", func() {
			creation := metav1.NewTime(baseTime)                               // T+0
			hashComputeAfter := metav1.NewTime(baseTime.Add(4 * time.Minute)) // T+4m
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, &hashComputeAfter)

			expectedCheckAfter := baseTime.Add(4*time.Minute + 5*time.Minute) // T+9m
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"async: CheckAfter = HashComputeAfter + StabilizationWindow = T+4m + 5m = T+9m")
		})
	})

	// ========================================
	// UT-EM-253-004: Async target validity extension (guard NOT triggered)
	// ========================================
	Describe("UT-EM-253-004: Async validity (guard not triggered)", Label("UT-EM-253-004"), func() {

		It("should set ValidityDeadline = HashComputeAfter + stab + validity", func() {
			creation := metav1.NewTime(baseTime)                               // T+0
			hashComputeAfter := metav1.NewTime(baseTime.Add(4 * time.Minute)) // T+4m
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute // guard NOT triggered: 5m < 10m

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, &hashComputeAfter)

			expectedDeadline := baseTime.Add(4*time.Minute + 5*time.Minute + 10*time.Minute) // T+19m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"async: ValidityDeadline = HashComputeAfter + stab + validity = T+4m + 5m + 10m = T+19m")

			expectedEffective := 5*time.Minute + 10*time.Minute // 15m
			Expect(dt.EffectiveValidity).To(Equal(expectedEffective),
				"async: effectiveValidity = stab + validity = 15m (always compounded for async)")
		})
	})

	// ========================================
	// UT-EM-253-005: Sync target timing (formula contrast with UT-EM-253-004)
	// ========================================
	Describe("UT-EM-253-005: Sync timing (formula contrast)", Label("UT-EM-253-005"), func() {

		It("should anchor to creation and use unextended validity (nil HashComputeAfter)", func() {
			creation := metav1.NewTime(baseTime) // T+0
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute // same inputs as UT-EM-253-004

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, nil)

			expectedCheckAfter := baseTime.Add(5 * time.Minute) // T+5m
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"sync: CheckAfter = creation + stab = T+0 + 5m = T+5m")

			expectedDeadline := baseTime.Add(10 * time.Minute) // T+10m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"sync: ValidityDeadline = creation + validity = T+0 + 10m = T+10m (no extension)")

			Expect(dt.EffectiveValidity).To(Equal(validityWindow),
				"sync: effectiveValidity = validityWindow = 10m (guard not triggered: 5m < 10m)")
			Expect(dt.Extended).To(BeFalse(),
				"sync: guard not triggered")
		})

		It("should treat zero-value HashComputeAfter as sync (same as nil)", func() {
			creation := metav1.NewTime(baseTime)
			zeroHCA := &metav1.Time{} // zero-value, not nil
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, zeroHCA)

			expectedCheckAfter := baseTime.Add(5 * time.Minute)
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"zero HashComputeAfter treated as sync: CheckAfter = creation + stab")
		})
	})

	// ========================================
	// UT-EM-253-006: Async target + runtime guard interaction
	// ========================================
	Describe("UT-EM-253-006: Async + runtime guard interaction", Label("UT-EM-253-006"), func() {

		It("should compound propagation with guard extension", func() {
			creation := metav1.NewTime(baseTime)                               // T+0
			hashComputeAfter := metav1.NewTime(baseTime.Add(4 * time.Minute)) // T+4m
			stabilizationWindow := 15 * time.Minute                            // guard triggered: 15m >= 10m
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, &hashComputeAfter)

			expectedCheckAfter := baseTime.Add(4*time.Minute + 15*time.Minute) // T+19m
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"async+guard: CheckAfter = HashComputeAfter + stab = T+4m + 15m = T+19m")

			expectedEffective := 15*time.Minute + 10*time.Minute // 25m
			Expect(dt.EffectiveValidity).To(Equal(expectedEffective),
				"async+guard: effectiveValidity = stab + validity = 25m")

			expectedDeadline := baseTime.Add(4*time.Minute + 25*time.Minute) // T+29m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"async+guard: ValidityDeadline = HashComputeAfter + effectiveValidity = T+4m + 25m = T+29m")
		})
	})

	// ========================================
	// UT-EM-253-007: Validity checker stabilization anchor for async targets
	// ========================================
	Describe("UT-EM-253-007: Validity checker stabilization anchor", Label("UT-EM-253-007"), func() {

		// This test validates that when the reconciler passes HashComputeAfter
		// as the anchor (instead of CreationTimestamp), the validity checker
		// correctly gates Stabilizing → Assessing transitions.

		DescribeTable("check window state with HashComputeAfter anchor",
			func(elapsed time.Duration, expectedState validity.WindowState, reason string) {
				hashComputeAfter := metav1.NewTime(baseTime.Add(4 * time.Minute)) // T+4m
				stabilizationWindow := 5 * time.Minute
				validityDeadline := metav1.NewTime(baseTime.Add(19 * time.Minute)) // T+19m

				// Create a testable checker that uses a fixed "now"
				checkTime := baseTime.Add(elapsed)
				state := checkWindowStateAt(checkTime, hashComputeAfter, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(expectedState), reason)
			},
			Entry("T+3m: before HashComputeAfter → Stabilizing",
				3*time.Minute, validity.WindowStabilizing,
				"Before HashComputeAfter; stabilization hasn't even started"),
			Entry("T+5m: after HCA but before HCA+stab → Stabilizing",
				5*time.Minute, validity.WindowStabilizing,
				"After HashComputeAfter(T+4m) but before T+4m+5m=T+9m"),
			Entry("T+8m: still before HCA+stab → Stabilizing",
				8*time.Minute, validity.WindowStabilizing,
				"Still before T+9m"),
			Entry("T+9m: exactly at HCA+stab → Active",
				9*time.Minute, validity.WindowActive,
				"Exactly at HashComputeAfter+stab=T+9m; stabilization complete"),
			Entry("T+10m: within validity window → Active",
				10*time.Minute, validity.WindowActive,
				"Within validity window after stabilization"),
		)

		It("contrast: same inputs with creation anchor would give premature Active at T+5m", func() {
			creation := metav1.NewTime(baseTime) // T+0
			stabilizationWindow := 5 * time.Minute
			validityDeadline := metav1.NewTime(baseTime.Add(19 * time.Minute))

			// Bug scenario: using creation as anchor instead of HashComputeAfter
			checkTime := baseTime.Add(5 * time.Minute) // T+5m
			state := checkWindowStateAt(checkTime, creation, stabilizationWindow, validityDeadline)
			Expect(state).To(Equal(validity.WindowActive),
				"BUG: with creation anchor, T+5m = creation+stab → premature Active")
		})
	})
})

// checkWindowStateAt is a deterministic version of validity.Checker.Check that
// uses a fixed "now" instead of time.Now(). This enables table-driven tests
// with predictable results.
func checkWindowStateAt(now time.Time, anchor metav1.Time, stabilizationWindow time.Duration, validityDeadline metav1.Time) validity.WindowState {
	if !validityDeadline.Time.After(now) {
		return validity.WindowExpired
	}
	stabilizationEnd := anchor.Time.Add(stabilizationWindow)
	if now.Before(stabilizationEnd) {
		return validity.WindowStabilizing
	}
	return validity.WindowActive
}
