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

// Coverage for #1635 / BR-AI-086 AC10 / DD-LLM-009: AF must relay KA's new,
// dedicated EventTypeReasoningContentDelta event to a distinct SSE metadata
// type ("reasoning_content") rather than converging it onto the pre-existing
// "reasoning" channel used by orchestration narration (EventTypeReasoningDelta)
// and AF's own ADK Thought-part reasoning.
var _ = Describe("AF/KA reasoning content live-stream relay — #1635", func() {

	Describe("UT-AF-1635-001: FormatEventForUser extracts text from the new event type", func() {
		It("should return the text field for EventTypeReasoningContentDelta", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeReasoningContentDelta,
				Turn: 1,
				Data: json.RawMessage(`{"text":"considering memory limits and recent deploys","redacted":false}`),
			}

			text := tools.FormatEventForUser(evt)

			Expect(text).To(Equal("considering memory limits and recent deploys"))
		})

		It("should return empty text for a redacted reasoning content event", func() {
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeReasoningContentDelta,
				Turn: 1,
				Data: json.RawMessage(`{"text":"","redacted":true}`),
			}

			text := tools.FormatEventForUser(evt)

			Expect(text).To(BeEmpty(),
				"#1635: KA never sends visible text on a redacted turn; AF must not synthesize placeholder text")
		})
	})

	Describe("UT-AF-1635-002: isStatusEvent treats reasoning content as LLM content, not orchestration status", func() {
		It("should return false for EventTypeReasoningContentDelta", func() {
			Expect(tools.IsStatusEvent(ka.EventTypeReasoningContentDelta)).To(BeFalse(),
				"#1635: reasoning content is LLM-generated content, must route to the artifact/reasoning channel, not the status channel")
		})
	})

	Describe("IT-AF-1635-001: emitEventToA2A relays the new event type to a distinct SSE metadata type", func() {
		It("should write a TaskStatusUpdateEvent with metadata.type=reasoning_content, not reasoning", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1635-001", "ctx-1635-001", nil)

			events := make(chan ka.InvestigationEvent, 5)
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningContentDelta,
				Turn: 1,
				Data: json.RawMessage(`{"text":"considering memory limits","redacted":false}`),
			}
			close(events)

			_, _, _ = tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			queuedEvents := queue.Events()
			Expect(queuedEvents).NotTo(BeEmpty(),
				"#1635: a real KA reasoning_content_delta event must be relayed to the A2A queue")

			foundReasoningContentType := false
			foundLegacyReasoningType := false
			for _, evt := range queuedEvents {
				raw, _ := json.Marshal(evt)
				if containsAll(string(raw), "considering memory limits", `"reasoning_content"`) {
					foundReasoningContentType = true
				}
				if containsAll(string(raw), `"type":"reasoning"`) {
					foundLegacyReasoningType = true
				}
			}
			Expect(foundReasoningContentType).To(BeTrue(),
				"#1635: the relayed event must carry the reasoning content text and metadata.type=reasoning_content")
			Expect(foundLegacyReasoningType).To(BeFalse(),
				"#1635: reasoning content must NOT be tagged with the legacy metadata.type=reasoning used by orchestration narration")
		})
	})

	// #1716 / DD-LLM-009 (redaction sub-decision, revisited): a redacted turn
	// now relays a content-free signal (metadata.redacted=true, empty text)
	// instead of the full no-op this test originally asserted. Re-tagged
	// UT-AF-1716-001 per docs/testing/1716/TEST_PLAN.md Section 9 — the old
	// UT-AF-1635-003 assertion described the bug being fixed, not a contract
	// to preserve.
	Describe("UT-AF-1716-001: a redacted reasoning content event relays a content-free signal", func() {
		It("should write one event with metadata.redacted=true and empty text when KA sends a redacted payload", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1716-001", "ctx-1716-001", nil)

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
				"#1716: a redacted turn must produce exactly one event, not a no-op; the audit trail remains the durable record of the actual content")

			statusEvt, ok := queuedEvents[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(statusEvt.Metadata["type"]).To(Equal(launcher.MetaTypeReasoningContent))
			Expect(statusEvt.Metadata["redacted"]).To(Equal(true),
				"#1716: metadata.redacted must be true so Console can render a placeholder")
		})
	})

	Describe("UT-AF-1635-004: reasoning content never leaks into the accumulated summary", func() {
		It("should not include reasoning_content_delta text in processBridgeEvent's summary accumulation", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1635-004", "ctx-1635-004", nil)

			events := make(chan ka.InvestigationEvent, 5)
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningContentDelta,
				Turn: 1,
				Data: json.RawMessage(`{"text":"raw model deliberation that must not leak","redacted":false}`),
			}
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Turn: 2,
				Data: json.RawMessage(`{"rca_summary":"pod OOM killed","severity":"critical","confidence":0.9,"target":"pod/test"}`),
			}
			close(events)

			summary, _, _ := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(summary).NotTo(ContainSubstring("raw model deliberation"),
				"#1635 / DD-LLM-009: genuine reasoning content must never leak into the final chat-answer/RCA summary text")
		})
	})
})
