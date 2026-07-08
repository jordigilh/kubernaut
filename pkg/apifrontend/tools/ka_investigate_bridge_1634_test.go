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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// Regression coverage for #1634: AF's FormatEventForUser read JSON field
// "text" from EventTypeReasoningDelta events, but every real KA producer
// (internal/kubernautagent/investigator/investigator_loop.go,
// investigator_rca.go, investigator_workflow_selection.go) sent
// "content_preview" or "content" instead — never "text". Net effect:
// extractJSONField always returned "", so emitEventToA2A's text=="" guard
// silently dropped every reasoning_delta event, in production, regardless
// of investigation content.
//
// KA's producers have since been normalized to emit "text" (the fix for
// #1634) — these tests now pin that normalized, real payload shape so a
// future accidental key-rename on either the KA producer or AF consumer
// side is caught immediately, per AU-3 (content traceability for
// operator-facing text).
var _ = Describe("AF/KA reasoning_delta relay key-mismatch — #1634", func() {

	Describe("UT-AF-1634-001: FormatEventForUser on investigator_loop.go's payload shape", func() {
		It("should return non-empty text for the text+tool_call_count shape emitted by investigator_loop.go", func() {
			// Mirrors internal/kubernautagent/investigator/investigator_loop.go's
			// post-#1634 emitToSink payload: {"text": ..., "tool_call_count": ...}.
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Turn: 1,
				Data: json.RawMessage(`{"text":"investigating pod crash loop","tool_call_count":1}`),
			}

			text := tools.FormatEventForUser(evt)

			Expect(text).NotTo(BeEmpty(),
				"#1634: reasoning_delta events from investigator_loop.go must not be silently dropped")
			Expect(text).To(Equal("investigating pod crash loop"))
		})
	})

	Describe("UT-AF-1634-002: FormatEventForUser on investigator_rca.go / investigator_workflow_selection.go's payload shape", func() {
		It("should return non-empty text for the text+retry_attempt shape emitted by the retry paths", func() {
			// Mirrors internal/kubernautagent/investigator/investigator_rca.go and
			// investigator_workflow_selection.go's post-#1634 emitToSink payload:
			// {"text": ..., "retry_attempt": ...}.
			evt := ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Turn: 2,
				Data: json.RawMessage(`{"text":"re-evaluating after validation failure","retry_attempt":1}`),
			}

			text := tools.FormatEventForUser(evt)

			Expect(text).NotTo(BeEmpty(),
				"#1634: reasoning_delta retry events must not be silently dropped")
			Expect(text).To(Equal("re-evaluating after validation failure"))
		})
	})

	Describe("IT-AF-1634-001: BridgeEventsCollectSummary relays the payload shape end-to-end to the A2A queue", func() {
		It("should write a TaskStatusUpdateEvent with metadata.type=reasoning and non-empty text", func() {
			queue := &bridgeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1634-001", "ctx-1634-001", nil)

			events := make(chan ka.InvestigationEvent, 5)
			events <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Turn: 1,
				Data: json.RawMessage(`{"text":"checking pod events","tool_call_count":0}`),
			}
			close(events)

			_, _, _ = tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			queuedEvents := queue.Events()
			Expect(queuedEvents).NotTo(BeEmpty(),
				"#1634: a real KA reasoning_delta event must produce at least one A2A event, not be silently dropped")

			foundReasoningText := false
			for _, evt := range queuedEvents {
				raw, _ := json.Marshal(evt)
				// containsAll is defined in terminal_event_1438_test.go (same package).
				if containsAll(string(raw), "checking pod events", `"reasoning"`) {
					foundReasoningText = true
				}
			}
			Expect(foundReasoningText).To(BeTrue(),
				"#1634: the relayed event must carry the reasoning text and metadata.type=reasoning")
		})
	})
})
