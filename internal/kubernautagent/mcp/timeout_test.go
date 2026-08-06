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

package mcp_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

var _ = Describe("TimeoutManager — PR4 BR-INTERACTIVE-003 DD-INTERACTIVE-002", func() {

	Describe("UT-KA-SESS-004: Inactivity timeout releases session after no activity", func() {
		It("should fire the onExpire callback when inactivity timeout elapses", func() {
			expired := make(chan string, 1)
			mgr := mcpinternal.NewTimeoutManager(
				100*time.Millisecond,  // inactivity timeout
				[]time.Duration{},     // no warnings for this test
				func(sessionID string) { expired <- sessionID },
			)

			mgr.StartTracking("sess-timeout-001", func(_ string) {})
			Eventually(expired, 500*time.Millisecond).Should(Receive(Equal("sess-timeout-001")))
			mgr.StopTracking("sess-timeout-001")
		})
	})

	Describe("UT-KA-SESS-005: Absolute timeout warnings at T-10m and T-2m (scaled down)", func() {
		It("should deliver warning notifications at configured intervals", func() {
			var mu sync.Mutex
			warnings := []string{}
			mgr := mcpinternal.NewTimeoutManager(
				300*time.Millisecond, // inactivity
				[]time.Duration{50 * time.Millisecond, 150 * time.Millisecond}, // warning intervals from start
				func(_ string) {},
			)

			mgr.StartTracking("sess-warn-001", func(msg string) {
				mu.Lock()
				warnings = append(warnings, msg)
				mu.Unlock()
			})

			Eventually(func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(warnings)
			}, 500*time.Millisecond, 10*time.Millisecond).Should(BeNumerically(">=", 2))

			mgr.StopTracking("sess-warn-001")
		})
	})

	Describe("UT-KA-TAKE-006: Timeout visibility: warning messages are human-readable", func() {
		It("should deliver warnings with human-readable text", func() {
			var mu sync.Mutex
			var lastWarning string
			mgr := mcpinternal.NewTimeoutManager(
				200*time.Millisecond,
				[]time.Duration{50 * time.Millisecond},
				func(_ string) {},
			)

			mgr.StartTracking("sess-readable-001", func(msg string) {
				mu.Lock()
				lastWarning = msg
				mu.Unlock()
			})

			Eventually(func() string {
				mu.Lock()
				defer mu.Unlock()
				return lastWarning
			}, 300*time.Millisecond, 10*time.Millisecond).ShouldNot(BeEmpty())

			mu.Lock()
			Expect(lastWarning).To(ContainSubstring("session"))
			mu.Unlock()

			mgr.StopTracking("sess-readable-001")
		})
	})

	Describe("UT-KA-TAKE-007: Inactivity reset prevents timeout", func() {
		It("should not fire timeout when activity is detected", func() {
			expired := make(chan string, 1)
			mgr := mcpinternal.NewTimeoutManager(
				200*time.Millisecond,
				[]time.Duration{},
				func(sessionID string) { expired <- sessionID },
			)

			mgr.StartTracking("sess-active-001", func(_ string) {})

			for i := 0; i < 5; i++ {
				time.Sleep(80 * time.Millisecond)
				mgr.ResetInactivity("sess-active-001")
			}

			// After last reset, timer fires in 200ms. Check for only 100ms.
			Consistently(expired, 100*time.Millisecond).ShouldNot(Receive())
			mgr.StopTracking("sess-active-001")
		})
	})

	Describe("UT-KA-1949-004: SetActiveCancel is invoked on expiry, before onExpire (BR-KA-267, #1949)", func() {
		It("calls the registered CancelFunc when the inactivity timer fires", func() {
			var mu sync.Mutex
			var cancelCalled, expireCalled bool
			expired := make(chan struct{}, 1)
			mgr := mcpinternal.NewTimeoutManager(
				100*time.Millisecond,
				[]time.Duration{},
				func(_ string) {
					mu.Lock()
					expireCalled = true
					// The cancel must have already been invoked by the time
					// onExpire runs, so the in-flight investigation context
					// is torn down before (or at worst alongside) session
					// bookkeeping cleanup — never after.
					Expect(cancelCalled).To(BeTrue(), "SetActiveCancel's CancelFunc must fire before onExpire")
					mu.Unlock()
					expired <- struct{}{}
				},
			)

			mgr.StartTracking("sess-cascade-001", func(_ string) {})
			mgr.SetActiveCancel("sess-cascade-001", func() {
				mu.Lock()
				cancelCalled = true
				mu.Unlock()
			})

			Eventually(expired, 500*time.Millisecond).Should(Receive())

			mu.Lock()
			Expect(cancelCalled).To(BeTrue(), "registered CancelFunc must be invoked on expiry")
			Expect(expireCalled).To(BeTrue())
			mu.Unlock()

			mgr.StopTracking("sess-cascade-001")
		})
	})

	Describe("UT-KA-1949-005: ClearActiveCancel prevents a stale cancel from firing after normal completion", func() {
		It("does not invoke a cleared CancelFunc when the session later times out for an unrelated reason", func() {
			cancelCalled := make(chan struct{}, 1)
			mgr := mcpinternal.NewTimeoutManager(
				60*time.Millisecond,
				[]time.Duration{},
				func(_ string) {},
			)

			mgr.StartTracking("sess-clear-001", func(_ string) {})
			mgr.SetActiveCancel("sess-clear-001", func() { cancelCalled <- struct{}{} })

			// Simulate the action completing normally: the cancel is
			// cleared before the inactivity timer would otherwise fire.
			mgr.ClearActiveCancel("sess-clear-001")

			Consistently(cancelCalled, 200*time.Millisecond).ShouldNot(Receive(),
				"a cleared CancelFunc must never fire, even once the session's inactivity timer later expires")

			mgr.StopTracking("sess-clear-001")
		})
	})
})
