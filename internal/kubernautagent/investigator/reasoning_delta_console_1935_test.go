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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	afka "github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	aftools "github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// reasoningDeltaTextFromEvents scans events for the first EventTypeReasoningDelta
// whose "text" field contains marker, returning that text (or "" if not found).
func reasoningDeltaTextContaining(events []session.InvestigationEvent, marker string) string {
	for i := range events {
		if events[i].Type != session.EventTypeReasoningDelta {
			continue
		}
		var data map[string]interface{}
		if json.Unmarshal(events[i].Data, &data) != nil {
			continue
		}
		text, _ := data["text"].(string)
		if text != "" && (marker == "" || containsSubstr(text, marker)) {
			return text
		}
	}
	return ""
}

// #1935 finding #3 (BR-AI-086, FedRAMP CC7.2): the console's ThinkingPanel is
// fed by the EventTypeReasoningDelta event, which KA emits to the session
// EventSink and AF forwards verbatim to the A2A artifact channel (see
// pkg/apifrontend/tools/ka_investigate_mcp.go's FormatEventForUser /
// emitEventToA2A -> launcher.EmitReasoningSafe). Confirmed against live
// production audit data for rr-618ac7d3b894-ba320bf0: every claude-sonnet-5
// RCA-phase turn (5/5) had Content=="" while making 1-4 tool calls, so the
// console ThinkingPanel went silently blank for the entire diagnostic phase
// of the investigation. This spec drives a real Investigator through its
// real production event-sink wiring and proves the emitted text now carries
// the thinking-block content instead of an empty string.
var _ = Describe("#1935 finding #3: console ThinkingPanel must not go blank on Sonnet-5 tool-calling turns", func() {

	Describe("IT-KA-1935-012: EventTypeReasoningDelta carries Reasoning.Text when Content is empty", func() {
		It("emits the thinking-block text to the event sink for a tool-calling turn with empty Content", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{
						// Sonnet-5-style turn: all narrative in the thinking
						// block, Content left empty, still makes a tool call.
						Message: llm.Message{
							Role:      "assistant",
							Content:   "",
							Reasoning: &llm.ReasoningBlock{Text: "UNIQUE_CONSOLE_REASONING_MARKER_1935"},
						},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
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

			var deltaEvent *session.InvestigationEvent
			for i := range events {
				if events[i].Type == session.EventTypeReasoningDelta {
					deltaEvent = &events[i]
					break
				}
			}
			Expect(deltaEvent).NotTo(BeNil(), "IT-KA-1935-012: must emit a reasoning_delta event for the tool-calling turn")

			var data map[string]interface{}
			Expect(json.Unmarshal(deltaEvent.Data, &data)).To(Succeed())
			text, _ := data["text"].(string)
			Expect(text).To(ContainSubstring("UNIQUE_CONSOLE_REASONING_MARKER_1935"),
				"IT-KA-1935-012: the console ThinkingPanel's reasoning_delta text must fall back to the "+
					"thinking-block content when Content is empty, or the panel goes silently blank for the "+
					"whole diagnostic phase of a Sonnet-5 investigation (#1935 finding #3)")
		})
	})

	// The other 2 of 3 emitToSink(EventTypeReasoningDelta, ...) call sites use
	// the identical one-line `reasoningDeltaText(resp.Message)` substitution
	// proven by UT-KA-1935-011; these two specs additionally prove each is
	// actually reached on its own real production dispatch path (CHECKPOINT
	// W — one IT test per wiring point, not just the shared helper), mirroring
	// this test plan's existing precedent of IT-KA-1935-008/009 (two separate
	// gates proven independently despite sharing the same underlying fix).
	Describe("IT-KA-1935-013: retryRCASubmit's reasoning_delta carries Reasoning.Text when Content is empty", func() {
		It("emits the thinking-block text during the RCA parse-retry turn", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &gateMockLLMClient{
				responses: []llm.ChatResponse{
					// Turn 1: unparseable plain text (no tool call) -> retryRCASubmit.
					{Message: llm.Message{Role: "assistant", Content: "not valid RCA json, no confidence field here"}},
					// Retry turn: Sonnet-5-style empty Content + Reasoning, valid submit_result.
					{
						Message: llm.Message{
							Role:      "assistant",
							Content:   "",
							Reasoning: &llm.ReasoningBlock{Text: "UNIQUE_RCA_RETRY_REASONING_MARKER_1935"},
						},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_retry", Name: investigator.SubmitResultToolName, Arguments: `{"rca_summary":"OOMKilled","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api","namespace":"ns","api_version":"apps/v1"}}`},
						},
					},
					gateWfToolResp(`{"workflow_id":"increase-memory-limit","confidence":0.9}`),
				},
			}

			logger := logr.Discard()
			store := &gateRecordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: parser.NewResultParser(),
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			go func() {
				_, _ = inv.Investigate(ctx, katypes.SignalContext{
					Name: "pod-retry-1935", Namespace: "default", Severity: "critical", Message: "OOMKilled",
				})
				close(eventCh)
			}()

			events := collectEvents(eventCh)
			text := reasoningDeltaTextContaining(events, "UNIQUE_RCA_RETRY_REASONING_MARKER_1935")
			Expect(text).To(ContainSubstring("UNIQUE_RCA_RETRY_REASONING_MARKER_1935"),
				"IT-KA-1935-013: retryRCASubmit's reasoning_delta event must fall back to Reasoning.Text "+
					"when the retry turn's Content is empty (#1935 finding #3)")
		})
	})

	Describe("IT-KA-1935-014: retryWorkflowSubmit's reasoning_delta carries Reasoning.Text when Content is empty", func() {
		It("emits the thinking-block text during the workflow-discovery parse-retry turn", func() {
			eventCh := make(chan session.InvestigationEvent, 64)
			ctx := session.WithEventSink(context.Background(), eventCh)

			mockClient := &gateMockLLMClient{
				responses: []llm.ChatResponse{
					// RCA phase succeeds immediately.
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9,"remediation_target":{"kind":"Deployment","name":"api","namespace":"ns","api_version":"apps/v1"}}`}},
					// Workflow-discovery turn 1: unparseable plain text -> retryWorkflowSubmit.
					{Message: llm.Message{Role: "assistant", Content: "not valid workflow json, no workflow_id here"}},
					// Retry turn: Sonnet-5-style empty Content + Reasoning, valid submit tool call.
					{
						Message: llm.Message{
							Role:      "assistant",
							Content:   "",
							Reasoning: &llm.ReasoningBlock{Text: "UNIQUE_WF_RETRY_REASONING_MARKER_1935"},
						},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_wf_retry", Name: investigator.SubmitResultWithWorkflowToolName, Arguments: `{"workflow_id":"increase-memory-limit","confidence":0.9}`},
						},
					},
				},
			}

			logger := logr.Discard()
			store := &gateRecordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, store, logger)
			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: parser.NewResultParser(),
				Enricher: enricher, AuditStore: store, Logger: logger,
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			go func() {
				_, _ = inv.Investigate(ctx, katypes.SignalContext{
					Name: "pod-wf-retry-1935", Namespace: "default", Severity: "critical", Message: "OOMKilled",
				})
				close(eventCh)
			}()

			events := collectEvents(eventCh)
			text := reasoningDeltaTextContaining(events, "UNIQUE_WF_RETRY_REASONING_MARKER_1935")
			Expect(text).To(ContainSubstring("UNIQUE_WF_RETRY_REASONING_MARKER_1935"),
				"IT-KA-1935-014: retryWorkflowSubmit's reasoning_delta event must fall back to Reasoning.Text "+
					"when the retry turn's Content is empty (#1935 finding #3)")
		})
	})
})

// #2010 (BR-AI-086, FedRAMP CC7.2/SI-10): drives a real Investigator through
// its real production event-sink wiring with an LLM turn shaped exactly like
// the live PR #2000 dev-environment observation -- Claude repeats the same
// one-line plan in both Reasoning.Text and Content immediately before a tool
// call -- and proves the resulting reasoning_delta event (as it will
// actually reach the Console via AF's real FormatEventForUser) shows the
// sentence exactly once, not as a self-duplicated "X\n\nX" entry.
var _ = Describe("IT-KA-2010-002: reasoning_delta does not self-duplicate identical Reasoning.Text and Content on the wire", func() {
	It("emits a single, non-duplicated sentence when Claude repeats its plan in both blocks", func() {
		eventCh := make(chan session.InvestigationEvent, 64)
		ctx := session.WithEventSink(context.Background(), eventCh)

		duplicateText := "Let me now gather the pod logs and events for complete evidence."
		mockClient := &cancelAwareMockClient{
			responses: []llm.ChatResponse{
				{
					Message: llm.Message{
						Role:      "assistant",
						Content:   duplicateText,
						Reasoning: &llm.ReasoningBlock{Text: duplicateText},
					},
					ToolCalls: []llm.ToolCall{
						{ID: "tc_1", Name: "kubectl_get_logs", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
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

		var kaEvent *session.InvestigationEvent
		for i := range events {
			if events[i].Type == session.EventTypeReasoningDelta {
				kaEvent = &events[i]
				break
			}
		}
		Expect(kaEvent).NotTo(BeNil(), "IT-KA-2010-002: must emit a reasoning_delta event for the turn")

		var data map[string]interface{}
		Expect(json.Unmarshal(kaEvent.Data, &data)).To(Succeed())
		text, _ := data["text"].(string)
		Expect(text).To(Equal(duplicateText),
			"IT-KA-2010-002: KA's own emitted event must not concatenate identical Reasoning.Text and "+
				"Content into a self-duplicated entry")

		// Full KA/AF wire-contract proof (mirrors IT-KA-1771-001): the same
		// wire hop AF actually performs (MCP JSON transport -> ka.InvestigationEvent
		// -> production FormatEventForUser) must not reintroduce duplication.
		wireBytes, err := json.Marshal(kaEvent)
		Expect(err).NotTo(HaveOccurred())
		var afEvent afka.InvestigationEvent
		Expect(json.Unmarshal(wireBytes, &afEvent)).To(Succeed())
		displayText := aftools.FormatEventForUser(afEvent)
		Expect(displayText).To(Equal(duplicateText),
			"IT-KA-2010-002: the operator-observable Console text (AF's production FormatEventForUser "+
				"output) must show the duplicate-prone sentence exactly once")
	})
})
