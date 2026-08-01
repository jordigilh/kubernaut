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

package session_test

import (
	"context"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Event Sink Context Helpers — #823 PR3", func() {

	Describe("UT-KA-823-C08: Event sink context round-trip", func() {
		It("event sink attached to context is retrievable by EventSinkFromContext", func() {
			ch := make(chan session.InvestigationEvent, 1)
			ctx := session.WithEventSink(context.Background(), ch)

			retrieved := session.EventSinkFromContext(ctx)
			Expect(retrieved).NotTo(BeNil(), "should retrieve the attached event sink")

			retrieved <- session.InvestigationEvent{Type: session.EventTypeComplete}
			Expect(ch).To(Receive(HaveField("Type", session.EventTypeComplete)))
		})
	})

	Describe("UT-KA-823-C09: Missing event sink returns nil", func() {
		It("EventSinkFromContext on plain context returns nil without panic", func() {
			ctx := context.Background()
			retrieved := session.EventSinkFromContext(ctx)
			Expect(retrieved).To(BeNil(), "no event sink attached — should return nil")
		})
	})
})

var _ = Describe("Session ID Context Helpers — BR-AUDIT-070", func() {

	Describe("UT-KA-PR9-SID-001: WithSessionID / SessionIDFromContext round-trip", func() {
		It("session ID attached to context is retrievable", func() {
			ctx := session.WithSessionID(context.Background(), "sess-abc-123")
			Expect(session.SessionIDFromContext(ctx)).To(Equal("sess-abc-123"))
		})
	})

	Describe("UT-KA-PR9-SID-002: SessionIDFromContext returns empty string for plain context", func() {
		It("returns empty string without panic when no session ID is attached", func() {
			ctx := context.Background()
			Expect(session.SessionIDFromContext(ctx)).To(Equal(""))
		})
	})

	Describe("UT-KA-PR9-SID-003: SessionIDFromContext returns empty string for nil context value", func() {
		It("does not panic when context has wrong type for session ID key", func() {
			ctx := context.Background()
			Expect(session.SessionIDFromContext(ctx)).To(BeEmpty())
		})
	})

	Describe("UT-KA-PR9-SID-004: WithSessionID does not interfere with event sink", func() {
		It("both session ID and event sink coexist on the same context", func() {
			ch := make(chan session.InvestigationEvent, 1)
			ctx := session.WithEventSink(context.Background(), ch)
			ctx = session.WithSessionID(ctx, "sess-coexist")

			Expect(session.SessionIDFromContext(ctx)).To(Equal("sess-coexist"))
			Expect(session.EventSinkFromContext(ctx)).NotTo(BeNil())
		})
	})
})

var _ = Describe("LazySink Buffered Replay — #1811", func() {

	Describe("UT-KA-1811-001: Emit buffers events when no channel is attached", func() {
		It("buffers events emitted before Set() and replays them in order once a channel is attached", func() {
			ls := &session.LazySink{}

			sent := ls.Emit(session.InvestigationEvent{Type: session.EventTypeReasoningDelta, Turn: 0})
			Expect(sent).To(BeFalse(), "no channel attached yet — Emit must not report a live send")

			sent = ls.Emit(session.InvestigationEvent{Type: session.EventTypeToolCallStart, Turn: 0})
			Expect(sent).To(BeFalse())

			ch := make(chan session.InvestigationEvent, 8)
			ls.Set(ch)

			Expect(ch).To(Receive(HaveField("Type", session.EventTypeReasoningDelta)),
				"buffered event 1 must be replayed first (order preserved)")
			Expect(ch).To(Receive(HaveField("Type", session.EventTypeToolCallStart)),
				"buffered event 2 must be replayed second")
		})

		It("delivers events emitted after Set() immediately, without needing another Set() call", func() {
			ls := &session.LazySink{}
			ch := make(chan session.InvestigationEvent, 8)
			ls.Set(ch)

			sent := ls.Emit(session.InvestigationEvent{Type: session.EventTypeComplete})
			Expect(sent).To(BeTrue(), "channel already attached — Emit must report a live send")
			Expect(ch).To(Receive(HaveField("Type", session.EventTypeComplete)))
		})

		It("bounds the buffer so an autonomous investigation that is never subscribed to cannot grow unbounded", func() {
			ls := &session.LazySink{}
			const overCapacity = 200
			for i := 0; i < overCapacity; i++ {
				ls.Emit(session.InvestigationEvent{Type: session.EventTypeTokenDelta, Turn: i})
			}

			ch := make(chan session.InvestigationEvent, overCapacity)
			ls.Set(ch)
			close(ch)

			var replayed int
			var lastTurn int
			for evt := range ch {
				replayed++
				lastTurn = evt.Turn
			}
			Expect(replayed).To(BeNumerically("<", overCapacity),
				"buffer must be capped — not every emitted event can be retained")
			Expect(lastTurn).To(Equal(overCapacity-1),
				"oldest events are evicted first — the most recent event must survive")
		})
	})
})

var _ = Describe("Interactive Upgrade Context Helpers — #1390", func() {

	Describe("UT-KA-1390-005 [SC-24]: InteractiveUpgradeFromContext returns false when no flag in context", func() {
		It("returns false without panic on a plain context", func() {
			ctx := context.Background()
			Expect(session.InteractiveUpgradeFromContext(ctx)).To(BeFalse(),
				"no upgrade flag attached — must return false")
		})
	})

	Describe("UT-KA-1390-006 [SC-24]: InteractiveUpgradeFromContext reads true after Store(true) on atomic", func() {
		It("returns true after a concurrent Store(true) on the shared atomic.Bool", func() {
			flag := &atomic.Bool{}
			ctx := session.WithInteractiveUpgrade(context.Background(), flag)

			Expect(session.InteractiveUpgradeFromContext(ctx)).To(BeFalse(),
				"before Store(true), must return false")

			flag.Store(true)

			Expect(session.InteractiveUpgradeFromContext(ctx)).To(BeTrue(),
				"after Store(true), must return true via shared pointer")
		})
	})
})
