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

package tools_test

import (
	"bytes"
	"context"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// countWatchTerminalEventsGoroutines returns the number of currently running
// goroutines executing tools.WatchTerminalEvents, by scanning a full stack
// dump. Used by IT-AF-2094-001 to prove the production-spawned watcher
// goroutine actually exits, rather than relying on an indirect signal.
func countWatchTerminalEventsGoroutines() int {
	buf := make([]byte, 4<<20)
	n := runtime.Stack(buf, true)
	return bytes.Count(buf[:n], []byte("tools.WatchTerminalEvents("))
}

var _ = Describe("IT #2094 — WatchTerminalEvents safety net through production path", func() {

	It("IT-AF-2094-001 (SI-11, AU-3): watcher spawned via HandleInvestigationMCPWithRegistry exits on its own when the terminal signal is permanently lost", func() {
		defer tools.SetWatchTerminalEventsSafetyNetForTest(30 * time.Millisecond)()

		// Unbuffered and never written to: simulates a pool onRelease
		// callback that never fires and a KA session that never emits
		// session_ended -- the exact live #2094 hang scenario.
		eventCh := make(chan ka.InvestigationEvent)
		sess := &mockPoolSession{}

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-it-2094-001",
					Status:    "started",
					Events:    eventCh,
					Closer:    func() {},
					Session:   sess,
				}, nil
			},
		}

		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-2094-001", "ctx-it-2094-001", nil)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		before := countWatchTerminalEventsGoroutines()

		_, err := tools.HandleInvestigationMCPWithRegistry(
			ctx, &tools.InvestigateConfig{
				MCPClient: mockMCP,
				Pool:      pool,
			}, tools.InvestigateMCPArgs{RRID: "rr-it-2094-001"},
			true, "alice",
		)
		Expect(err).NotTo(HaveOccurred())

		// 2s (rather than a tighter window) absorbs scheduling jitter under
		// -race and full-suite CPU contention: this assertion only proves
		// the goroutine was spawned at all, so a generous window costs
		// nothing but flakiness-resistance.
		Eventually(countWatchTerminalEventsGoroutines, 2*time.Second, 5*time.Millisecond).Should(
			BeNumerically(">", before),
			"production path must have spawned the real WatchTerminalEvents goroutine")

		// #2094: before the fix, nothing in this scenario ever unblocks the
		// watcher (no onRelease call, no session_ended, and watchCtx is
		// detached via context.WithoutCancel so this outer ctx's own
		// timeout is irrelevant to it). It must rely entirely on the
		// safety-net timer (shortened to 30ms above) to exit.
		Eventually(countWatchTerminalEventsGoroutines, 3*time.Second, 10*time.Millisecond).Should(
			Equal(before),
			"#2094: WatchTerminalEvents spawned via the real production path must exit on its own "+
				"once the safety-net timer fires, instead of leaking forever when the terminal signal is lost")
	})
})
