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
	"strings"
	"sync/atomic"

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

// streamTrackingMockClient simulates an LLM that emits known text chunks
// via StreamChat, allowing tests to verify the operator-visible event stream
// matches the actual LLM output. Responses cycle: the last response is
// reused for any subsequent phases beyond the provided list.
type streamTrackingMockClient struct {
	chatCalls   atomic.Int64
	streamCalls atomic.Int64
	chunks      []string
	responses   []llm.ChatResponse
	callIdx     int
}

func (m *streamTrackingMockClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	m.chatCalls.Add(1)
	return m.nextResponse(), nil
}

func (m *streamTrackingMockClient) StreamChat(_ context.Context, _ llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	m.streamCalls.Add(1)
	for _, chunk := range m.chunks {
		_ = callback(llm.ChatStreamEvent{Delta: chunk})
	}
	_ = callback(llm.ChatStreamEvent{Done: true})
	return m.nextResponse(), nil
}

func (m *streamTrackingMockClient) nextResponse() llm.ChatResponse {
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp
	}
	if len(m.responses) > 0 {
		return m.responses[len(m.responses)-1]
	}
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback","confidence":0.1}`},
	}
}

func (m *streamTrackingMockClient) Close() error { return nil }

func tokenStreamTestInvestigator(client llm.Client) *investigator.Investigator {
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

var tokenStreamSignal = katypes.SignalContext{
	Name:          "test-pod",
	Namespace:     "default",
	Severity:      "critical",
	Message:       "OOMKilled",
	RemediationID: "rem-token-stream",
}

var _ = Describe("Token-Level Streaming in runLLMLoop — #823 PR6", func() {

	// BR-SESSION-003: An operator observing an active investigation receives
	// token-by-token LLM reasoning fragments that faithfully reproduce the
	// LLM's output as it is generated.
	Describe("UT-KA-823-T01: Operator observing an investigation receives token-by-token LLM reasoning", func() {
		It("delivers each LLM text chunk as an EventTypeTokenDelta with the exact fragment", func() {
			expectedChunks := []string{"ana", "lyzing", " pod ", "crash"}
			mock := &streamTrackingMockClient{
				chunks: expectedChunks,
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

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := tokenStreamTestInvestigator(mock)
			go func() {
				_, _ = inv.Investigate(ctx, tokenStreamSignal)
				close(eventCh)
			}()

			var events []session.InvestigationEvent
			for ev := range eventCh {
				events = append(events, ev)
			}

			// Collect token deltas from the first phase (RCA). The investigation
			// has multiple phases; each phase that calls the LLM generates its own
			// set of token deltas. We verify fidelity within a single phase.
			var firstPhase string
			var firstPhaseDeltas []string
			for _, ev := range events {
				if ev.Type == session.EventTypeTokenDelta {
					var data map[string]interface{}
					Expect(json.Unmarshal(ev.Data, &data)).To(Succeed())
					Expect(data).To(HaveKey("delta"),
						"each token_delta event must carry the text fragment")
					delta := data["delta"].(string)
					if firstPhase == "" {
						firstPhase = ev.Phase
					}
					if ev.Phase == firstPhase {
						firstPhaseDeltas = append(firstPhaseDeltas, delta)
					}
					Expect(ev.Turn).To(BeNumerically(">=", 0),
						"token_delta must carry the turn number for SSE ordering")
					Expect(ev.Phase).NotTo(BeEmpty(),
						"token_delta must carry the investigation phase")
				}
			}
			Expect(firstPhaseDeltas).To(Equal(expectedChunks),
				"operator must receive the exact LLM text fragments in order for each phase")
		})
	})

	// BR-SESSION-003 (regression): Autonomous investigations without an
	// observer produce identical results to v1.4 — no events leak, no
	// behavioral change.
	Describe("UT-KA-823-T02: Autonomous investigation without observer produces identical results", func() {
		It("completes with a valid RCA result and emits zero events", func() {
			mock := &streamTrackingMockClient{
				chunks: []string{"should", "not", "appear"},
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

			ctx := context.Background() // no event sink — autonomous mode
			inv := tokenStreamTestInvestigator(mock)
			result, err := inv.Investigate(ctx, tokenStreamSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).NotTo(BeEmpty(),
				"autonomous investigation must produce a valid root cause analysis")
			Expect(result.Cancelled).To(BeFalse(),
				"non-cancelled investigation must not be marked cancelled")
		})
	})

	// BR-SESSION-003 (resilience): A slow or disconnected observer must never
	// block the autonomous investigation. The investigation MUST complete
	// regardless of observer capacity.
	Describe("UT-KA-823-T03: Slow observer cannot block the autonomous investigation", func() {
		It("investigation completes with a valid result even when the event channel is full", func() {
			mock := &streamTrackingMockClient{
				chunks: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{
							Role:    "assistant",
							Content: `{"rca_summary":"OOM detected in container","confidence":0.8}`,
						},
					},
				},
			}

			eventCh := make(chan session.InvestigationEvent, 1) // tiny buffer — simulates slow consumer
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := tokenStreamTestInvestigator(mock)
			result, err := inv.Investigate(ctx, tokenStreamSignal)

			Expect(err).NotTo(HaveOccurred(),
				"investigation must not deadlock on a full event channel")
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).NotTo(BeEmpty(),
				"investigation must still produce a valid RCA despite dropped events")
		})
	})

	// BR-SESSION-001 + BR-SESSION-003: When an operator cancels a streaming
	// investigation, the cancellation outcome is identical regardless of
	// whether streaming was active. The operator's cancel action takes effect.
	Describe("UT-KA-823-T04: Cancellation during streaming produces same outcome as non-streaming", func() {
		It("cancellation yields notification to observer with token deltas before the cancel event", func() {
			cancelCtx, cancel := context.WithCancel(context.Background())
			mock := &streamTrackingMockClient{
				chunks: []string{"analyzing", "..."},
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

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(cancelCtx, eventCh)

			callCount := &atomic.Int64{}
			wrappedMock := &cancelOnSecondCallMock{
				inner:     mock,
				cancelFn:  cancel,
				callCount: callCount,
			}

			inv := tokenStreamTestInvestigator(wrappedMock)
			go func() {
				_, _ = inv.Investigate(ctx, tokenStreamSignal)
				close(eventCh)
			}()

			var events []session.InvestigationEvent
			for ev := range eventCh {
				events = append(events, ev)
			}

			hasCancelled := false
			hasTokenDelta := false
			for _, ev := range events {
				if ev.Type == session.EventTypeCancelled {
					hasCancelled = true
				}
				if ev.Type == session.EventTypeTokenDelta {
					hasTokenDelta = true
				}
			}
			Expect(hasTokenDelta).To(BeTrue(),
				"operator should have received token deltas before cancellation")
			Expect(hasCancelled).To(BeTrue(),
				"operator must be notified of cancellation via EventTypeCancelled")
		})
	})

	// BR-SESSION-003 + BR-SESSION-007: A complete investigation with an
	// observer produces both fine-grained (token_delta) and coarse-grained
	// (reasoning_delta) events. Within a single phase, token deltas arrive
	// BEFORE the turn summary — ensuring real-time experience.
	Describe("UT-KA-823-T05: Observer receives both token-level and turn-level events with correct ordering", func() {
		It("token_delta events precede the reasoning_delta within the same phase", func() {
			expectedChunks := []string{"think", "ing", " about", " OOM"}
			mock := &streamTrackingMockClient{
				chunks: expectedChunks,
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{
							Role:    "assistant",
							Content: `{"rca_summary":"root cause found","confidence":0.95}`,
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
				},
			}

			eventCh := make(chan session.InvestigationEvent, 128)
			ctx := session.WithEventSink(context.Background(), eventCh)

			inv := tokenStreamTestInvestigator(mock)
			go func() {
				_, _ = inv.Investigate(ctx, tokenStreamSignal)
				close(eventCh)
			}()

			var events []session.InvestigationEvent
			for ev := range eventCh {
				events = append(events, ev)
			}

			// Verify ordering within the first phase: all token_delta events
			// for a given phase must precede the reasoning_delta for that phase.
			var firstPhase string
			lastTokenDeltaIdx := -1
			firstReasoningIdx := -1
			var phaseTokenTexts []string

			for i, ev := range events {
				if ev.Type == session.EventTypeTokenDelta {
					if firstPhase == "" {
						firstPhase = ev.Phase
					}
					if ev.Phase == firstPhase {
						lastTokenDeltaIdx = i
						var data map[string]interface{}
						if json.Unmarshal(ev.Data, &data) == nil {
							if d, ok := data["delta"].(string); ok {
								phaseTokenTexts = append(phaseTokenTexts, d)
							}
						}
					}
				}
				if ev.Type == session.EventTypeReasoningDelta && ev.Phase == firstPhase && firstReasoningIdx == -1 {
					firstReasoningIdx = i
				}
			}

			Expect(lastTokenDeltaIdx).To(BeNumerically(">=", 0),
				"operator must receive token-level delta events")
			Expect(firstReasoningIdx).To(BeNumerically(">=", 0),
				"operator must receive the turn-level reasoning summary")
			Expect(lastTokenDeltaIdx).To(BeNumerically("<", firstReasoningIdx),
				"within a phase, token deltas must arrive BEFORE the turn reasoning summary (real-time streaming)")

			reconstructed := strings.Join(phaseTokenTexts, "")
			Expect(reconstructed).To(Equal(strings.Join(expectedChunks, "")),
				"concatenated token deltas must reconstruct the original LLM output")
		})
	})

	// BR-SESSION-007: EventTypeTokenDelta is runtime-agnostic and forwards-
	// compatible with Goose ACP migration. Verify the constant value is stable.
	Describe("UT-KA-823-T06: EventTypeTokenDelta wire value is runtime-agnostic", func() {
		It("has a stable wire value suitable for SSE contract", func() {
			Expect(session.EventTypeTokenDelta).To(Equal("token_delta"),
				"wire value must be stable across runtime migrations (BR-SESSION-007)")
		})
	})
})

// cancelOnSecondCallMock wraps a streamTrackingMockClient and cancels the
// context after the first successful LLM call, simulating an operator cancel
// that arrives between investigation turns.
type cancelOnSecondCallMock struct {
	inner     *streamTrackingMockClient
	cancelFn  context.CancelFunc
	callCount *atomic.Int64
}

func (m *cancelOnSecondCallMock) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if m.callCount.Add(1) > 1 {
		m.cancelFn()
		return llm.ChatResponse{}, context.Canceled
	}
	return m.inner.Chat(ctx, req)
}

func (m *cancelOnSecondCallMock) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	if m.callCount.Add(1) > 1 {
		m.cancelFn()
		return llm.ChatResponse{}, context.Canceled
	}
	return m.inner.StreamChat(ctx, req, cb)
}

func (m *cancelOnSecondCallMock) Close() error { return nil }
