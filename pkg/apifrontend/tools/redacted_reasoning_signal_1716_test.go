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

// Coverage for #1716 / BR-AI-086 AC10 / DD-LLM-009 (redaction sub-decision,
// revisited): AF must relay a content-free live signal (metadata.redacted=
// true, empty text) for a redacted reasoning turn on both production
// dispatch paths — the initial-investigation path (BridgeEventsCollectSummary)
// and the pooled/interactive live-relay path (WatchTerminalEvents, #1637) —
// rather than the full no-op both paths previously produced.
var _ = Describe("AF redacted reasoning content live signal — #1716", func() {

	It("IT-AF-1716-001: a redacted turn relays a content-free signal via the initial-investigation dispatch path", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-1716-it-001", "ctx-1716-it-001", nil)

		events := make(chan ka.InvestigationEvent, 5)
		events <- ka.InvestigationEvent{
			Type: ka.EventTypeReasoningContentDelta,
			Turn: 1,
			Data: json.RawMessage(`{"text":"","redacted":true}`),
		}
		close(events)

		_, _, _ = tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

		queuedEvents := queue.Events()
		Expect(queuedEvents).To(HaveLen(1),
			"IT-AF-1716-001: the production dispatch path must relay the redacted signal, not just the EventBridge unit")

		statusEvt, ok := queuedEvents[0].(*a2a.TaskStatusUpdateEvent)
		Expect(ok).To(BeTrue())
		Expect(statusEvt.Metadata["type"]).To(Equal(launcher.MetaTypeReasoningContent))
		Expect(statusEvt.Metadata["redacted"]).To(Equal(true))
		if statusEvt.Status.Message != nil {
			for _, part := range statusEvt.Status.Message.Parts {
				if textPart, ok := part.(a2a.TextPart); ok {
					Expect(textPart.Text).To(BeEmpty(),
						"IT-AF-1716-001: no visible text may ever be synthesized for a redacted turn")
				}
			}
		}
	})

	It("UT-AF-1716-002: a non-redacted empty-text turn still produces zero queue writes on the real dispatch path (regression guard)", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-1716-ut-002", "ctx-1716-ut-002", nil)

		events := make(chan ka.InvestigationEvent, 5)
		events <- ka.InvestigationEvent{
			Type: ka.EventTypeReasoningContentDelta,
			Turn: 1,
			Data: json.RawMessage(`{"text":"","redacted":false}`),
		}
		close(events)

		_, _, _ = tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

		Expect(queue.Events()).To(BeEmpty(),
			"UT-AF-1716-002: a genuinely empty, non-redacted turn must remain a no-op on the real dispatch path — only redacted=true changes behavior")
	})

	It("IT-AF-1716-002: a redacted turn relays a content-free signal via the pooled/interactive live-relay path (#1637)", func() {
		watchQueue := &bridgeQueue{}
		watchCtx := launcher.WithEventBridge(context.Background(), watchQueue, "task-1716-it-002-watch", "ctx-1716-it-002-watch", nil)

		liveQueue := &bridgeQueue{}
		liveCtx := launcher.WithEventBridge(context.Background(), liveQueue, "task-1716-it-002-live", "ctx-1716-it-002-live", nil)

		relay := &ka.EventRelay{}
		detach := relay.Attach(liveCtx)
		defer detach()

		events := make(chan ka.InvestigationEvent, 5)
		done := make(chan struct{})
		defer close(done)

		go tools.WatchTerminalEvents(watchCtx, events, "rr-1716-it-002", done, relay)

		events <- ka.InvestigationEvent{
			Type: ka.EventTypeReasoningContentDelta,
			Data: json.RawMessage(`{"text":"","redacted":true}`),
		}

		Eventually(func() []a2a.Event {
			return liveQueue.Events()
		}, 3*time.Second).ShouldNot(BeEmpty(),
			"IT-AF-1716-002: the attached ctx's EventBridge must receive the relayed redacted signal")

		Expect(watchQueue.Events()).To(BeEmpty(),
			"IT-AF-1716-002: the watcher's own detached ctx must NOT receive events while a live call is attached")

		found := false
		for _, evt := range liveQueue.Events() {
			statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok {
				continue
			}
			if statusEvt.Metadata["type"] == launcher.MetaTypeReasoningContent && statusEvt.Metadata["redacted"] == true {
				found = true
			}
		}
		Expect(found).To(BeTrue(),
			"IT-AF-1716-002: relayed event must carry metadata.type=reasoning_content and metadata.redacted=true")
	})
})
