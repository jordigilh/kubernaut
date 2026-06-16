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
	"encoding/json"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Terminal event emission to A2A — #1438", func() {

	Describe("UT-AF-1438-015: FormatEventForUser returns user-readable text for session_ended", func() {
		It("should return 'Session ended: <reason>' for EventTypeSessionEnded", func() {
			evt := ka.InvestigationEvent{
				Type:  ka.EventTypeSessionEnded,
				Phase: "inactivity_timeout",
			}
			text := tools.FormatEventForUser(evt)
			Expect(text).To(Equal("Session ended: inactivity_timeout"),
				"AU-3: user-facing text must include the release reason")
		})
	})

	Describe("UT-AF-1438-016: isStatusEvent returns true for EventTypeSessionEnded", func() {
		It("should classify session_ended as a status event", func() {
			Expect(tools.IsStatusEvent(ka.EventTypeSessionEnded)).To(BeTrue(),
				"session_ended is an orchestration event and belongs on the status channel")
		})
	})

	Describe("UT-AF-1438-010 (AU-3): bridgeEventsCollectSummary emits terminal event on session_ended", func() {
		It("should emit TaskStatusUpdateEvent with terminal metadata on session_ended", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-010", "ctx-1438-010", nil)
			ctx = tools.WithRRID(ctx, "rr-1438-010")

			events := make(chan ka.InvestigationEvent, 5)
			events <- ka.InvestigationEvent{
				Type:  ka.EventTypeSessionEnded,
				Phase: "inactivity_timeout",
			}

			summary, _ := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)
			_ = summary

			queuedEvents := queue.Events()
			Expect(queuedEvents).NotTo(BeEmpty(),
				"AU-3: session_ended must emit at least one status event to A2A queue")

			var foundTerminal bool
			for _, evt := range queuedEvents {
				if status, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
					raw, _ := json.Marshal(status)
					if containsTerminalMarker(raw) {
						foundTerminal = true
						break
					}
				}
			}
			Expect(foundTerminal).To(BeTrue(),
				"AU-3: terminal status event must be emitted for session_ended")
		})
	})

	Describe("UT-AF-1438-011 (AU-3): BridgeEventsToA2A emits terminal event and exits on session_ended", func() {
		It("should emit status event and exit on session_ended", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-011", "ctx-1438-011", nil)

			events := make(chan ka.InvestigationEvent, 5)
			events <- ka.InvestigationEvent{
				Type:  ka.EventTypeSessionEnded,
				Phase: "disconnect",
			}

			tools.BridgeEventsToA2A(ctx, events, 5*time.Second)

			queuedEvents := queue.Events()
			Expect(queuedEvents).NotTo(BeEmpty(),
				"AU-3: BridgeEventsToA2A must emit at least one event for session_ended")
		})
	})

	Describe("UT-AF-1438-020 (SI-4): watchTerminalEvents emits to A2A bridge and exits on session_ended", func() {
		It("should emit terminal status event and return", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-020", "ctx-1438-020", nil)

			events := make(chan ka.InvestigationEvent, 5)
			done := make(chan struct{})

			events <- ka.InvestigationEvent{
				Type:  ka.EventTypeSessionEnded,
				Phase: "inactivity_timeout",
			}

			exited := make(chan struct{})
			go func() {
				tools.WatchTerminalEvents(ctx, events, "rr-1438-020", done)
				close(exited)
			}()

			Eventually(exited, 3*time.Second).Should(BeClosed(),
				"SI-4: watchTerminalEvents must exit after emitting session_ended")

			queuedEvents := queue.Events()
			Expect(queuedEvents).NotTo(BeEmpty(),
				"SI-4: watchTerminalEvents must emit a terminal status event")
		})
	})

	Describe("UT-AF-1438-021: watchTerminalEvents exits cleanly on channel close", func() {
		It("should return without error when events channel is closed", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-021", "ctx-1438-021", nil)

			events := make(chan ka.InvestigationEvent, 5)
			done := make(chan struct{})

			close(events)

			exited := make(chan struct{})
			go func() {
				tools.WatchTerminalEvents(ctx, events, "rr-1438-021", done)
				close(exited)
			}()

			Eventually(exited, 3*time.Second).Should(BeClosed(),
				"watchTerminalEvents must exit cleanly on channel close")
		})
	})

	Describe("UT-AF-1438-022: watchTerminalEvents exits on done channel close", func() {
		It("should return when done channel is closed (pool-driven deterministic cleanup)", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-022", "ctx-1438-022", nil)

			events := make(chan ka.InvestigationEvent, 5)
			done := make(chan struct{})

			exited := make(chan struct{})
			go func() {
				tools.WatchTerminalEvents(ctx, events, "rr-1438-022", done)
				close(exited)
			}()

			time.Sleep(50 * time.Millisecond)
			Consistently(exited, 100*time.Millisecond).ShouldNot(BeClosed(),
				"watcher must not exit before done signal")

			close(done)

			Eventually(exited, 3*time.Second).Should(BeClosed(),
				"watchTerminalEvents must exit deterministically on done channel close")
		})
	})

	Describe("UT-AF-1438-027 (AU-3): bridgeEventsCollectSummary emits exactly one status event for session_ended", func() {
		It("should not double-emit: only the structured terminal event, not a plain one followed by structured", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-027", "ctx-1438-027", nil)
			ctx = tools.WithRRID(ctx, "rr-1438-027")

			events := make(chan ka.InvestigationEvent, 5)
			events <- ka.InvestigationEvent{
				Type:  ka.EventTypeSessionEnded,
				Phase: "inactivity_timeout",
			}

			tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			queuedEvents := queue.Events()
			statusCount := 0
			for _, evt := range queuedEvents {
				if _, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
					statusCount++
				}
			}
			Expect(statusCount).To(Equal(1),
				"AU-3: session_ended must produce exactly one TaskStatusUpdateEvent, not two (double-emit bug)")

			raw, _ := json.Marshal(queuedEvents[0])
			Expect(string(raw)).To(ContainSubstring(`"terminal"`),
				"the single emitted event must carry terminal metadata")
		})
	})

	Describe("UT-AF-1438-026 (SI-4): watchTerminalEvents drains buffered session_ended when done fires simultaneously", func() {
		It("should always emit the terminal event even when done and events are both ready", func() {
			const iterations = 50
			misses := 0
			for i := 0; i < iterations; i++ {
				queue := &bridgeQueue{}
				ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-026", "ctx-1438-026", nil)

				events := make(chan ka.InvestigationEvent, 1)
				done := make(chan struct{})

				events <- ka.InvestigationEvent{
					Type:  ka.EventTypeSessionEnded,
					Phase: "inactivity_timeout",
				}
				close(done)

				exited := make(chan struct{})
				go func() {
					tools.WatchTerminalEvents(ctx, events, "rr-1438-026", done)
					close(exited)
				}()

				Eventually(exited, 3*time.Second).Should(BeClosed())

				queuedEvents := queue.Events()
				if len(queuedEvents) == 0 {
					misses++
				}
			}
			Expect(misses).To(Equal(0),
				"SI-4: watchTerminalEvents must always drain a buffered session_ended even when done is closed simultaneously")
		})
	})

	Describe("UT-AF-1438-025 (AU-3, SC-7): terminal event metadata correctness", func() {
		It("should include phase, reason, rr_id, terminal — only identifiers, no sensitive data", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1438-025", "ctx-1438-025", nil)

			events := make(chan ka.InvestigationEvent, 5)
			done := make(chan struct{})

			events <- ka.InvestigationEvent{
				Type:  ka.EventTypeSessionEnded,
				Phase: "inactivity_timeout",
			}

			exited := make(chan struct{})
			go func() {
				tools.WatchTerminalEvents(ctx, events, "rr-1438-025", done)
				close(exited)
			}()
			Eventually(exited, 3*time.Second).Should(BeClosed())

			queuedEvents := queue.Events()
			Expect(queuedEvents).NotTo(BeEmpty())

			var foundWithMeta bool
			for _, evt := range queuedEvents {
				if status, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
					raw, _ := json.Marshal(status)
					s := string(raw)
					if containsAll(s, "investigation", "terminal", "inactivity_timeout") {
						foundWithMeta = true
						Expect(s).NotTo(ContainSubstring("password"),
							"SC-7: no sensitive data in metadata")
						Expect(s).NotTo(ContainSubstring("secret"),
							"SC-7: no sensitive data in metadata")
					}
				}
			}
			Expect(foundWithMeta).To(BeTrue(),
				"AU-3: terminal event must carry type=investigation, terminal=true, and reason")
		})
	})
})

var _ = Describe("Reason-to-phase mapping — #1438", func() {
	Describe("UT-AF-1438-028: mapReasonToPhase covers all documented reasons", func() {
		entries := []struct {
			reason   string
			expected string
		}{
			{"inactivity_timeout", "TimedOut"},
			{"ttl_expired", "TimedOut"},
			{"disconnect", "Disconnected"},
			{"unknown_reason", "unknown_reason"},
			{"", ""},
		}
		for _, e := range entries {
			e := e
			It("should map '"+e.reason+"' to '"+e.expected+"'", func() {
				Expect(tools.MapReasonToPhase(e.reason)).To(Equal(e.expected))
			})
		}
	})
})

func containsTerminalMarker(data []byte) bool {
	return containsAll(string(data), "terminal")
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
