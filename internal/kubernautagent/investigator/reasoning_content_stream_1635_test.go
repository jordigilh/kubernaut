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

package investigator_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Coverage for #1635 / BR-AI-086 AC10: KA must live-stream captured
// reasoning/thinking content (llm.Message.Reasoning) via a new, dedicated
// event type (session.EventTypeReasoningContentDelta), distinct from the
// pre-existing orchestration-narration event (session.EventTypeReasoningDelta,
// see #1634). Per DD-LLM-009, emission is gated only by
// resp.Message.Reasoning != nil.
var _ = Describe("KA reasoning content live-stream — #1635 / BR-AI-086 AC10", func() {

	findEventByType := func(events []session.InvestigationEvent, eventType string) *session.InvestigationEvent {
		for i := range events {
			if events[i].Type == eventType {
				return &events[i]
			}
		}
		return nil
	}

	Describe("UT-KA-1635-001: captured reasoning is emitted as a dedicated event", func() {
		It("should emit EventTypeReasoningContentDelta with text and redacted=false when Reasoning is present", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{
							Role:      "assistant",
							Content:   `{"rca_summary":"pod OOM killed","confidence":0.9}`,
							Reasoning: &llm.ReasoningBlock{Text: "considering memory limits and recent deploys"},
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
				},
			}

			inv := streamTestInvestigator(mockClient)
			go func() {
				_, _ = inv.Investigate(ctx, streamSignal)
				close(eventCh)
			}()

			events := collectEvents(eventCh)

			reasoningContentEvent := findEventByType(events, session.EventTypeReasoningContentDelta)
			Expect(reasoningContentEvent).NotTo(BeNil(),
				"#1635: captured reasoning must be live-streamed via a dedicated event type")

			var data map[string]interface{}
			Expect(json.Unmarshal(reasoningContentEvent.Data, &data)).To(Succeed())
			Expect(data).To(HaveKeyWithValue("text", "considering memory limits and recent deploys"))
			Expect(data).To(HaveKeyWithValue("redacted", false))

			// The pre-existing orchestration-narration event must still fire,
			// unaffected — #1635 is purely additive alongside it.
			Expect(findEventByType(events, session.EventTypeReasoningDelta)).NotTo(BeNil())
		})
	})

	Describe("UT-KA-1635-002: no captured reasoning means no live event (default-disabled parity)", func() {
		It("should not emit EventTypeReasoningContentDelta when Reasoning is nil", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{
							Role:    "assistant",
							Content: `{"rca_summary":"pod OOM killed","confidence":0.9}`,
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
				},
			}

			inv := streamTestInvestigator(mockClient)
			go func() {
				_, _ = inv.Investigate(ctx, streamSignal)
				close(eventCh)
			}()

			events := collectEvents(eventCh)

			Expect(findEventByType(events, session.EventTypeReasoningContentDelta)).To(BeNil(),
				"#1635: no reasoning content event should be emitted when the LLM response carries no Reasoning (BR-AI-086 AC2 default-disabled parity)")
		})
	})

	Describe("UT-KA-1635-003: redacted reasoning is emitted transparently (KA side)", func() {
		It("should emit EventTypeReasoningContentDelta with empty text and redacted=true when Reasoning.Redacted is true", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{
							Role:      "assistant",
							Content:   `{"rca_summary":"pod OOM killed","confidence":0.9}`,
							Reasoning: &llm.ReasoningBlock{Text: "", Redacted: true, Signature: "opaque-sig"},
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
				},
			}

			inv := streamTestInvestigator(mockClient)
			go func() {
				_, _ = inv.Investigate(ctx, streamSignal)
				close(eventCh)
			}()

			events := collectEvents(eventCh)

			reasoningContentEvent := findEventByType(events, session.EventTypeReasoningContentDelta)
			Expect(reasoningContentEvent).NotTo(BeNil(),
				"#1635: KA's wire payload is redaction-transparent — the event still fires so 'redacted' reaches AF")

			var data map[string]interface{}
			Expect(json.Unmarshal(reasoningContentEvent.Data, &data)).To(Succeed())
			Expect(data).To(HaveKeyWithValue("text", ""))
			Expect(data).To(HaveKeyWithValue("redacted", true))
		})
	})
})
