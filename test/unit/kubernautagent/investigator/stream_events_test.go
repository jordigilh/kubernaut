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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func streamTestInvestigator(client llm.Client) *investigator.Investigator {
	logger := slog.Default()
	builder, _ := prompt.NewBuilder()
	rp := parser.NewResultParser()
	enricher := enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, logger)
	return investigator.New(investigator.Config{
		Client:       client,
		Builder:      builder,
		ResultParser: rp,
		Enricher:     enricher,
		AuditStore:   audit.NopAuditStore{},
		Logger:       logger,
		MaxTurns:     15,
		PhaseTools:   investigator.DefaultPhaseToolMap(),
	})
}

var streamSignal = katypes.SignalContext{
	Name:          "test-pod",
	Namespace:     "default",
	Severity:      "critical",
	Message:       "OOMKilled",
	RemediationID: "rem-stream-test",
}

func collectEvents(ch <-chan session.InvestigationEvent) []session.InvestigationEvent {
	var events []session.InvestigationEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

var _ = Describe("Kubernaut Agent Investigator Stream Events — #823 PR4", func() {

	Describe("UT-KA-823-S01: Turn-level events emitted to event sink", func() {
		It("investigation with event sink produces turn-level events for each LLM interaction", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "investigating..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
					{
						Message: llm.Message{
							Role:    "assistant",
							Content: `{"rca_summary":"pod crash loop due to OOM","confidence":0.85}`,
						},
						Usage: llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300},
					},
				},
			}

			inv := streamTestInvestigator(mockClient)
			go func() {
				_, _ = inv.Investigate(ctx, streamSignal)
				close(eventCh)
			}()

			events := collectEvents(eventCh)
			Expect(len(events)).To(BeNumerically(">=", 2),
				"at least 2 events expected: LLM response + tool execution")

			hasToolEvent := false
			for _, ev := range events {
				if ev.Type == session.EventTypeToolCallStart || ev.Type == session.EventTypeToolResult {
					hasToolEvent = true
				}
			}
			Expect(hasToolEvent).To(BeTrue(), "should emit tool call events")
		})
	})

	Describe("UT-KA-823-S02: LLM response event structure", func() {
		It("LLM response event carries turn, phase, and content preview", func() {
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

			var responseEvent *session.InvestigationEvent
			for i := range events {
				if events[i].Type == session.EventTypeReasoningDelta {
					responseEvent = &events[i]
					break
				}
			}
			Expect(responseEvent).NotTo(BeNil(), "should emit a reasoning_delta event for LLM response")
			Expect(responseEvent.Turn).To(BeNumerically(">=", 0))
			Expect(responseEvent.Phase).NotTo(BeEmpty())

			var data map[string]interface{}
			Expect(json.Unmarshal(responseEvent.Data, &data)).To(Succeed())
			Expect(data).To(HaveKey("content_preview"))
		})
	})

	Describe("UT-KA-823-S03: Tool call events emitted before and after execution", func() {
		It("each tool call produces a start and result event with tool name", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "analyzing..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
							{ID: "tc_2", Name: "kubectl_logs", Arguments: `{"name":"test","namespace":"default"}`},
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
					{
						Message: llm.Message{
							Role:    "assistant",
							Content: `{"rca_summary":"pod OOM killed","confidence":0.9}`,
						},
						Usage: llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300},
					},
				},
			}

			inv := streamTestInvestigator(mockClient)
			go func() {
				_, _ = inv.Investigate(ctx, streamSignal)
				close(eventCh)
			}()

			events := collectEvents(eventCh)

			toolStarts := 0
			toolResults := 0
			for _, ev := range events {
				if ev.Type == session.EventTypeToolCallStart {
					toolStarts++
					var data map[string]interface{}
					Expect(json.Unmarshal(ev.Data, &data)).To(Succeed())
					Expect(data).To(HaveKey("tool_name"))
				}
				if ev.Type == session.EventTypeToolResult {
					toolResults++
					var data map[string]interface{}
					Expect(json.Unmarshal(ev.Data, &data)).To(Succeed())
					Expect(data).To(HaveKey("tool_name"))
				}
			}
			Expect(toolStarts).To(Equal(2), "2 tool calls should produce 2 start events")
			Expect(toolResults).To(Equal(2), "2 tool calls should produce 2 result events")
		})
	})

	Describe("UT-KA-823-S04: Non-blocking send — full channel does not block investigation", func() {
		It("investigation completes even when event channel is full", func() {
			eventCh := make(chan session.InvestigationEvent, 1) // minimal buffer
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "analyzing..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
							{ID: "tc_2", Name: "kubectl_logs", Arguments: `{"name":"test","namespace":"default"}`},
							{ID: "tc_3", Name: "kubectl_get", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
					{
						Message: llm.Message{
							Role:    "assistant",
							Content: `{"rca_summary":"OOM","confidence":0.8}`,
						},
					},
				},
			}

			inv := streamTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, streamSignal)

			Expect(err).NotTo(HaveOccurred(), "investigation must complete even with full channel")
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).NotTo(BeEmpty(), "investigation should produce a valid result")
		})
	})

	Describe("UT-KA-823-S07: Nil event sink — zero events, identical v1.4 behavior", func() {
		It("investigation without event sink produces no events and completes normally", func() {
			ctx := context.Background() // no event sink

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
			result, err := inv.Investigate(ctx, streamSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-823-S08: Cancelled event emitted to sink on cancellation", func() {
		It("cancellation produces EventTypeCancelled on the event sink", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx, cancel := context.WithCancel(context.Background())
			ctx = session.WithEventSink(ctx, eventCh)

			mockClient := &cancelAwareMockClient{
				cancelAfter: 1,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "investigating..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
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

			hasCancelledEvent := false
			for _, ev := range events {
				if ev.Type == session.EventTypeCancelled {
					hasCancelledEvent = true
				}
			}
			Expect(hasCancelledEvent).To(BeTrue(), "should emit EventTypeCancelled on cancellation")
		})
	})
})
