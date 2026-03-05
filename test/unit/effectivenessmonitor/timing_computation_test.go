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
// EM Timing Computation Tests
// ========================================
//
// Business Requirements:
// - BR-EM-009:   Derived timing computation (ValidityDeadline, CheckAfter, AlertCheckAfter)
// - BR-EM-010.4: Stabilization anchored to HashCheckDelay for async targets
//
// Design Document:
// - DD-EM-004 v2.0: Timing formulas (sync vs async)
//
// Issue #277: AlertCheckDelay additive semantics, Duration-based HashCheckDelay

var _ = Describe("EM Timing Computation (#253, #277, BR-EM-009, BR-EM-010.4)", func() {

	var baseTime time.Time

	BeforeEach(func() {
		baseTime = time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
	})

	// ========================================
	// UT-EM-253-003: Async target timing anchor (Duration-based)
	// ========================================
	Describe("UT-EM-253-003: Async target timing anchor", Label("UT-EM-253-003"), func() {

		It("should anchor CheckAfter to creation+HashCheckDelay, not CreationTimestamp alone", func() {
			creation := metav1.NewTime(baseTime)
			hashCheckDelay := &metav1.Duration{Duration: 4 * time.Minute}
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, hashCheckDelay, nil)

			expectedCheckAfter := baseTime.Add(4*time.Minute + 5*time.Minute) // T+9m
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"async: CheckAfter = creation + HashCheckDelay + StabilizationWindow = T+0 + 4m + 5m = T+9m")
		})
	})

	// ========================================
	// UT-EM-253-004: Async target validity extension
	// ========================================
	Describe("UT-EM-253-004: Async validity (guard not triggered)", Label("UT-EM-253-004"), func() {

		It("should set ValidityDeadline = creation + hashCheckDelay + stab + validity", func() {
			creation := metav1.NewTime(baseTime)
			hashCheckDelay := &metav1.Duration{Duration: 4 * time.Minute}
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, hashCheckDelay, nil)

			expectedDeadline := baseTime.Add(4*time.Minute + 5*time.Minute + 10*time.Minute) // T+19m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"async: ValidityDeadline = creation + hashCheckDelay + stab + validity = T+19m")

			expectedEffective := 5*time.Minute + 10*time.Minute // 15m
			Expect(dt.EffectiveValidity).To(Equal(expectedEffective),
				"async: effectiveValidity = stab + validity = 15m (always compounded for async)")
		})
	})

	// ========================================
	// UT-EM-253-005: Sync target timing (nil HashCheckDelay)
	// ========================================
	Describe("UT-EM-253-005: Sync timing (formula contrast)", Label("UT-EM-253-005"), func() {

		It("should anchor to creation and use unextended validity (nil HashCheckDelay)", func() {
			creation := metav1.NewTime(baseTime)
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, nil, nil)

			expectedCheckAfter := baseTime.Add(5 * time.Minute) // T+5m
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"sync: CheckAfter = creation + stab = T+5m")

			expectedDeadline := baseTime.Add(10 * time.Minute) // T+10m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"sync: ValidityDeadline = creation + validity = T+10m (no extension)")

			Expect(dt.EffectiveValidity).To(Equal(validityWindow),
				"sync: effectiveValidity = validityWindow = 10m (guard not triggered: 5m < 10m)")
			Expect(dt.Extended).To(BeFalse(),
				"sync: guard not triggered")
		})

		It("should treat zero-duration HashCheckDelay as sync (same as nil)", func() {
			creation := metav1.NewTime(baseTime)
			zeroDelay := &metav1.Duration{Duration: 0}
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, zeroDelay, nil)

			expectedCheckAfter := baseTime.Add(5 * time.Minute)
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"zero HashCheckDelay treated as sync: CheckAfter = creation + stab")
		})
	})

	// ========================================
	// UT-EM-253-006: Async target + runtime guard interaction
	// ========================================
	Describe("UT-EM-253-006: Async + runtime guard interaction", Label("UT-EM-253-006"), func() {

		It("should compound propagation with guard extension", func() {
			creation := metav1.NewTime(baseTime)
			hashCheckDelay := &metav1.Duration{Duration: 4 * time.Minute}
			stabilizationWindow := 15 * time.Minute // guard triggered: 15m >= 10m
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, hashCheckDelay, nil)

			expectedCheckAfter := baseTime.Add(4*time.Minute + 15*time.Minute) // T+19m
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"async+guard: CheckAfter = creation + hashCheckDelay + stab = T+19m")

			expectedEffective := 15*time.Minute + 10*time.Minute // 25m
			Expect(dt.EffectiveValidity).To(Equal(expectedEffective),
				"async+guard: effectiveValidity = stab + validity = 25m")

			expectedDeadline := baseTime.Add(4*time.Minute + 25*time.Minute) // T+29m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"async+guard: ValidityDeadline = creation + hashCheckDelay + effectiveValidity = T+29m")
		})
	})

	// ========================================
	// UT-EM-277-001: AlertCheckDelay — nil means no additional delay
	// ========================================
	Describe("UT-EM-277-001: AlertCheckDelay nil (no additional delay)", Label("UT-EM-277-001"), func() {

		It("should set AlertCheckAfter == CheckAfter when AlertCheckDelay is nil", func() {
			creation := metav1.NewTime(baseTime)
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, nil, nil)

			Expect(dt.AlertCheckAfter.Time).To(BeTemporally("~", dt.CheckAfter.Time, time.Second),
				"nil AlertCheckDelay: AlertCheckAfter == CheckAfter")
		})
	})

	// ========================================
	// UT-EM-277-002: AlertCheckDelay additive on StabilizationWindow
	// ========================================
	Describe("UT-EM-277-002: AlertCheckDelay additive semantics", Label("UT-EM-277-002"), func() {

		It("should compute AlertCheckAfter = creation + stab + alertCheckDelay", func() {
			creation := metav1.NewTime(baseTime)
			stabilizationWindow := 1 * time.Minute
			validityWindow := 10 * time.Minute
			alertCheckDelay := &metav1.Duration{Duration: 4 * time.Minute}

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, nil, alertCheckDelay)

			expectedCheckAfter := baseTime.Add(1 * time.Minute) // T+1m (Prometheus)
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"Prometheus CheckAfter NOT affected by AlertCheckDelay")

			expectedAlertCheckAfter := baseTime.Add(1*time.Minute + 4*time.Minute) // T+5m
			Expect(dt.AlertCheckAfter.Time).To(BeTemporally("~", expectedAlertCheckAfter, time.Second),
				"AlertCheckAfter = creation + stab + alertCheckDelay = T+1m + 4m = T+5m")
		})
	})

	// ========================================
	// UT-EM-277-003: AlertCheckDelay triggers validity guard extension
	// ========================================
	Describe("UT-EM-277-003: AlertCheckDelay triggers validity extension", Label("UT-EM-277-003"), func() {

		It("should extend validity when stab + alertCheckDelay >= validityWindow", func() {
			creation := metav1.NewTime(baseTime)
			stabilizationWindow := 3 * time.Minute
			validityWindow := 5 * time.Minute
			alertCheckDelay := &metav1.Duration{Duration: 4 * time.Minute}
			// stab(3m) + alert(4m) = 7m >= validity(5m) → guard triggers

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, nil, alertCheckDelay)

			expectedEffective := 3*time.Minute + 4*time.Minute + 5*time.Minute // 12m
			Expect(dt.EffectiveValidity).To(Equal(expectedEffective),
				"guard triggered: effectiveValidity = stab + alertCheckDelay + validity = 12m")

			expectedDeadline := baseTime.Add(expectedEffective) // T+12m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"ValidityDeadline = creation + effectiveValidity = T+12m")

			Expect(dt.Extended).To(BeTrue(),
				"guard triggered by stab + alertCheckDelay >= validity")
		})

		It("should NOT extend validity when stab + alertCheckDelay < validityWindow", func() {
			creation := metav1.NewTime(baseTime)
			stabilizationWindow := 1 * time.Minute
			validityWindow := 10 * time.Minute
			alertCheckDelay := &metav1.Duration{Duration: 4 * time.Minute}
			// stab(1m) + alert(4m) = 5m < validity(10m) → guard NOT triggered

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, nil, alertCheckDelay)

			Expect(dt.EffectiveValidity).To(Equal(validityWindow),
				"guard NOT triggered: effectiveValidity = validityWindow = 10m")

			expectedDeadline := baseTime.Add(10 * time.Minute) // T+10m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"ValidityDeadline = creation + validity = T+10m (no extension)")

			Expect(dt.Extended).To(BeFalse(),
				"guard NOT triggered: stab + alertCheckDelay < validity")
		})
	})

	// ========================================
	// UT-EM-277-004: Async + AlertCheckDelay combined
	// ========================================
	Describe("UT-EM-277-004: Async + AlertCheckDelay combined", Label("UT-EM-277-004"), func() {

		It("should add AlertCheckDelay on top of async timing", func() {
			creation := metav1.NewTime(baseTime)
			hashCheckDelay := &metav1.Duration{Duration: 4 * time.Minute}
			stabilizationWindow := 5 * time.Minute
			validityWindow := 10 * time.Minute
			alertCheckDelay := &metav1.Duration{Duration: 3 * time.Minute}

			dt := emtiming.ComputeDerivedTiming(creation, stabilizationWindow, validityWindow, hashCheckDelay, alertCheckDelay)

			// Prometheus: creation + hashCheckDelay + stab = T+4m + 5m = T+9m
			expectedCheckAfter := baseTime.Add(4*time.Minute + 5*time.Minute)
			Expect(dt.CheckAfter.Time).To(BeTemporally("~", expectedCheckAfter, time.Second),
				"async: CheckAfter = creation + hashCheckDelay + stab = T+9m")

			// Alert: creation + hashCheckDelay + stab + alertCheckDelay = T+4m + 5m + 3m = T+12m
			expectedAlertCheckAfter := baseTime.Add(4*time.Minute + 5*time.Minute + 3*time.Minute)
			Expect(dt.AlertCheckAfter.Time).To(BeTemporally("~", expectedAlertCheckAfter, time.Second),
				"async: AlertCheckAfter = creation + hashCheckDelay + stab + alertCheckDelay = T+12m")

			// Validity: creation + hashCheckDelay + stab + alertCheckDelay + validity
			//         = T+0 + 4m + 5m + 3m + 10m = T+22m
			expectedEffective := 5*time.Minute + 3*time.Minute + 10*time.Minute // 18m from anchor
			Expect(dt.EffectiveValidity).To(Equal(expectedEffective),
				"async+alert: effectiveValidity = stab + alertCheckDelay + validity = 18m")

			expectedDeadline := baseTime.Add(4*time.Minute + expectedEffective) // T+22m
			Expect(dt.ValidityDeadline.Time).To(BeTemporally("~", expectedDeadline, time.Second),
				"async+alert: ValidityDeadline = creation + hashCheckDelay + effectiveValidity = T+22m")
		})
	})

	// ========================================
	// UT-EM-253-007: Validity checker stabilization anchor for async targets
	// ========================================
	Describe("UT-EM-253-007: Validity checker stabilization anchor", Label("UT-EM-253-007"), func() {

		DescribeTable("check window state with HashCheckDelay anchor",
			func(elapsed time.Duration, expectedState validity.WindowState, reason string) {
				// hashCheckDelay = 4m means hashDeadline = creation + 4m = T+4m
				hashDeadline := metav1.NewTime(baseTime.Add(4 * time.Minute))
				stabilizationWindow := 5 * time.Minute
				validityDeadline := metav1.NewTime(baseTime.Add(19 * time.Minute)) // T+19m

				checkTime := baseTime.Add(elapsed)
				state := checkWindowStateAt(checkTime, hashDeadline, stabilizationWindow, validityDeadline)
				Expect(state).To(Equal(expectedState), reason)
			},
			Entry("T+3m: before hash deadline → Stabilizing",
				3*time.Minute, validity.WindowStabilizing,
				"Before hash deadline; stabilization hasn't even started"),
			Entry("T+5m: after hash deadline but before deadline+stab → Stabilizing",
				5*time.Minute, validity.WindowStabilizing,
				"After hash deadline(T+4m) but before T+4m+5m=T+9m"),
			Entry("T+8m: still before deadline+stab → Stabilizing",
				8*time.Minute, validity.WindowStabilizing,
				"Still before T+9m"),
			Entry("T+9m: exactly at deadline+stab → Active",
				9*time.Minute, validity.WindowActive,
				"Exactly at hashDeadline+stab=T+9m; stabilization complete"),
			Entry("T+10m: within validity window → Active",
				10*time.Minute, validity.WindowActive,
				"Within validity window after stabilization"),
		)

		It("contrast: same inputs with creation anchor would give premature Active at T+5m", func() {
			creation := metav1.NewTime(baseTime)
			stabilizationWindow := 5 * time.Minute
			validityDeadline := metav1.NewTime(baseTime.Add(19 * time.Minute))

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
