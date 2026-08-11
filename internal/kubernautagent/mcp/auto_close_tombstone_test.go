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
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
)

// UT-KA-2075-001: AutoCloseTombstone is the small, self-expiring record
// (#2076, v1.6 clone of #2075) that lets kubernaut_complete_no_action
// recognize a session it raced against investigate_discovery.go's
// no_matching_workflows auto-close, instead of erroring with "no active
// interactive session".
var _ = Describe("AutoCloseTombstone — #2076 (v1.6 clone of #2075)", func() {
	It("UT-KA-2075-001a: WasRecentlyAutoClosed reports false before Mark is ever called", func() {
		tomb := mcpinternal.NewAutoCloseTombstone(50 * time.Millisecond)
		Expect(tomb.WasRecentlyAutoClosed("rr-1")).To(BeFalse())
	})

	It("UT-KA-2075-001b: WasRecentlyAutoClosed reports true immediately after Mark", func() {
		tomb := mcpinternal.NewAutoCloseTombstone(1 * time.Second)
		tomb.Mark("rr-2")
		Expect(tomb.WasRecentlyAutoClosed("rr-2")).To(BeTrue())
	})

	It("UT-KA-2075-001c: entry expires after the TTL elapses", func() {
		tomb := mcpinternal.NewAutoCloseTombstone(30 * time.Millisecond)
		tomb.Mark("rr-3")
		Expect(tomb.WasRecentlyAutoClosed("rr-3")).To(BeTrue())
		Eventually(func() bool {
			return tomb.WasRecentlyAutoClosed("rr-3")
		}, 500*time.Millisecond, 5*time.Millisecond).Should(BeFalse(),
			"a stale tombstone entry must not permanently mask a genuinely new race/miss for the same rr_id")
	})

	It("UT-KA-2075-001d: Mark and WasRecentlyAutoClosed are concurrent-safe", func() {
		tomb := mcpinternal.NewAutoCloseTombstone(200 * time.Millisecond)
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				id := fmt.Sprintf("rr-concurrent-%d", n)
				tomb.Mark(id)
				_ = tomb.WasRecentlyAutoClosed(id)
			}(i)
		}
		Expect(func() { wg.Wait() }).NotTo(Panic())
	})

	It("UT-KA-2075-001e: entries are isolated per rr_id -- marking one rr_id does not affect another", func() {
		tomb := mcpinternal.NewAutoCloseTombstone(1 * time.Second)
		tomb.Mark("rr-a")
		Expect(tomb.WasRecentlyAutoClosed("rr-a")).To(BeTrue())
		Expect(tomb.WasRecentlyAutoClosed("rr-b")).To(BeFalse())
	})

	It("UT-KA-2075-001f: a nil *AutoCloseTombstone is safe to call (optional dependency, matches WithHTTPCompleter/WithTimeoutTracker nil-safety convention)", func() {
		var tomb *mcpinternal.AutoCloseTombstone
		Expect(func() { tomb.Mark("rr-nil") }).NotTo(Panic())
		Expect(tomb.WasRecentlyAutoClosed("rr-nil")).To(BeFalse())
	})
})
