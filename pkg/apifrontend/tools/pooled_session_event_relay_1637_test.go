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

// #1637 / DD-AF-009: WatchTerminalEvents must relay non-terminal events to
// whichever pooled call's ctx is currently attached to the EventRelay, and
// must preserve #1438's original idle/terminal-only behavior byte-for-byte
// when no relay is attached (relay is nil, or relay.Current() is nil).
var _ = Describe("WatchTerminalEvents live relay — #1637", func() {

	It("IT-AF-1637-004: relays a non-terminal event to the attached ctx's EventBridge", func() {
		watchQueue := &bridgeQueue{}
		watchCtx := launcher.WithEventBridge(context.Background(), watchQueue, "task-1637-004-watch", "ctx-1637-004-watch", nil)

		liveQueue := &bridgeQueue{}
		liveCtx := launcher.WithEventBridge(context.Background(), liveQueue, "task-1637-004-live", "ctx-1637-004-live", nil)

		relay := &ka.EventRelay{}
		detach := relay.Attach(liveCtx)
		defer detach()

		events := make(chan ka.InvestigationEvent, 5)
		done := make(chan struct{})
		defer close(done)

		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(watchCtx, events, "rr-1637-004", done, relay)
			close(exited)
		}()

		events <- ka.InvestigationEvent{
			Type: ka.EventTypeReasoningContentDelta,
			Data: json.RawMessage(`{"text":"analyzing pod OOMKill event","redacted":false}`),
		}

		Eventually(func() []a2a.Event {
			return liveQueue.Events()
		}, 3*time.Second).ShouldNot(BeEmpty(),
			"IT-AF-1637-004: the live (currently-attached) ctx's EventBridge must receive the relayed event")

		Expect(watchQueue.Events()).To(BeEmpty(),
			"IT-AF-1637-004: the watcher's own detached ctx must NOT receive events while a live call is attached")

		found := false
		for _, evt := range liveQueue.Events() {
			statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok {
				continue
			}
			if statusEvt.Metadata["type"] == launcher.MetaTypeReasoningContent {
				found = true
			}
		}
		Expect(found).To(BeTrue(),
			"IT-AF-1637-004: relayed event must carry metadata.type=reasoning_content")
	})

	It("IT-AF-1637-004: idle (relay.Current() nil) drops non-terminal events exactly like #1438's original behavior", func() {
		watchQueue := &bridgeQueue{}
		watchCtx := launcher.WithEventBridge(context.Background(), watchQueue, "task-1637-005-watch", "ctx-1637-005-watch", nil)

		relay := &ka.EventRelay{} // never attached — idle

		events := make(chan ka.InvestigationEvent, 5)
		done := make(chan struct{})
		defer close(done)

		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(watchCtx, events, "rr-1637-005", done, relay)
			close(exited)
		}()

		events <- ka.InvestigationEvent{
			Type: ka.EventTypeReasoningContentDelta,
			Data: json.RawMessage(`{"text":"should be dropped","redacted":false}`),
		}

		Consistently(func() []a2a.Event {
			return watchQueue.Events()
		}, 200*time.Millisecond).Should(BeEmpty(),
			"IT-AF-1637-004: with no live call attached, non-terminal events must still be dropped (regression guard for #1438)")
	})

	It("IT-AF-1637-004: nil relay (backward compatible with pre-#1637 4-arg semantics) drops non-terminal events", func() {
		watchQueue := &bridgeQueue{}
		watchCtx := launcher.WithEventBridge(context.Background(), watchQueue, "task-1637-006-watch", "ctx-1637-006-watch", nil)

		events := make(chan ka.InvestigationEvent, 5)
		done := make(chan struct{})
		defer close(done)

		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(watchCtx, events, "rr-1637-006", done, nil)
			close(exited)
		}()

		events <- ka.InvestigationEvent{
			Type: ka.EventTypeReasoningContentDelta,
			Data: json.RawMessage(`{"text":"should be dropped","redacted":false}`),
		}

		Consistently(func() []a2a.Event {
			return watchQueue.Events()
		}, 200*time.Millisecond).Should(BeEmpty(),
			"a nil relay must behave identically to an idle relay")
	})

	It("IT-AF-1637-004: terminal event still exits the watcher and prefers the live ctx when a call is in flight", func() {
		watchQueue := &bridgeQueue{}
		watchCtx := launcher.WithEventBridge(context.Background(), watchQueue, "task-1637-007-watch", "ctx-1637-007-watch", nil)

		liveQueue := &bridgeQueue{}
		liveCtx := launcher.WithEventBridge(context.Background(), liveQueue, "task-1637-007-live", "ctx-1637-007-live", nil)

		relay := &ka.EventRelay{}
		detach := relay.Attach(liveCtx)
		defer detach()

		events := make(chan ka.InvestigationEvent, 5)
		done := make(chan struct{})

		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(watchCtx, events, "rr-1637-007", done, relay)
			close(exited)
		}()

		events <- ka.InvestigationEvent{
			Type:  ka.EventTypeSessionEnded,
			Phase: "inactivity_timeout",
		}

		Eventually(exited, 3*time.Second).Should(BeClosed(),
			"terminal event must still exit the watcher")
		Expect(liveQueue.Events()).NotTo(BeEmpty(),
			"terminal event must be routed to the live ctx when a call is attached at the time it arrives")
	})
})
