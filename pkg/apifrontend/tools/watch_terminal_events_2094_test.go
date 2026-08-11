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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// #2094: WatchTerminalEvents previously had no timer-based safety net -- a
// lost session_ended event (pool onRelease callback never firing, dropped
// events channel) left the goroutine blocked forever. These tests prove a
// bounded safety-net timer exists and does not regress the two pre-existing
// deterministic exit paths (#1438).
var _ = Describe("WatchTerminalEvents safety-net timer — #2094", func() {
	var restoreSafetyNet func()

	BeforeEach(func() {
		restoreSafetyNet = tools.SetWatchTerminalEventsSafetyNetForTest(30 * time.Millisecond)
	})

	AfterEach(func() {
		restoreSafetyNet()
	})

	It("UT-AF-2094-001 (SI-11, AU-3): exits via the safety-net timer when neither events nor done ever fire", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2094-001", "ctx-2094-001", nil)

		events := make(chan ka.InvestigationEvent)
		done := make(chan struct{})

		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(ctx, events, "rr-2094-001", done)
			close(exited)
		}()

		Eventually(exited, 2*time.Second, 5*time.Millisecond).Should(BeClosed(),
			"#2094: WatchTerminalEvents must exit on its own once the safety-net timer fires, "+
				"instead of blocking forever on a lost terminal signal")
	})

	It("UT-AF-2094-002 (regression guard): still exits immediately on session_ended, safety net does not interfere", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2094-002", "ctx-2094-002", nil)

		events := make(chan ka.InvestigationEvent, 1)
		done := make(chan struct{})
		events <- ka.InvestigationEvent{Type: ka.EventTypeSessionEnded, Phase: "inactivity_timeout"}

		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(ctx, events, "rr-2094-002", done)
			close(exited)
		}()

		// Must exit well before the (shortened) safety-net window elapses,
		// proving the happy path is unaffected by the new timer.
		Eventually(exited, 15*time.Millisecond, time.Millisecond).Should(BeClosed(),
			"session_ended must still exit the watcher immediately, not wait for the safety net")

		Expect(queue.Events()).NotTo(BeEmpty(), "AU-3: terminal status event must still be emitted")
	})

	It("UT-AF-2094-003 (regression guard): still drains a buffered session_ended on done closing before the safety net", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2094-003", "ctx-2094-003", nil)

		events := make(chan ka.InvestigationEvent, 1)
		done := make(chan struct{})
		events <- ka.InvestigationEvent{Type: ka.EventTypeSessionEnded, Phase: "disconnect"}
		close(done)

		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(ctx, events, "rr-2094-003", done)
			close(exited)
		}()

		Eventually(exited, 15*time.Millisecond, time.Millisecond).Should(BeClosed(),
			"#1438 priority-drain on done closing must still exit immediately, not wait for the safety net")

		Expect(queue.Events()).NotTo(BeEmpty(), "AU-3: drained session_ended must still be emitted")
	})
})
