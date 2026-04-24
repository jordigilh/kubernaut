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
	"fmt"
	"log/slog"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// cancelAwareMockClient is a mock LLM client that respects context cancellation
// and can trigger cancellation after a configurable number of Chat calls.
type cancelAwareMockClient struct {
	calls       []llm.ChatRequest
	responses   []llm.ChatResponse
	callIdx     int
	cancelAfter int
	cancelFn    context.CancelFunc
}

func (m *cancelAwareMockClient) Close() error { return nil }

func (m *cancelAwareMockClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls = append(m.calls, req)

	if ctx.Err() != nil {
		return llm.ChatResponse{}, fmt.Errorf("mock chat: %w", ctx.Err())
	}

	var resp llm.ChatResponse
	if m.callIdx < len(m.responses) {
		resp = m.responses[m.callIdx]
		m.callIdx++
	} else {
		resp = llm.ChatResponse{
			Message: llm.Message{
				Role:    "assistant",
				Content: `{"rca_summary":"fallback","confidence":0.1}`,
			},
		}
	}

	if m.cancelAfter > 0 && m.callIdx >= m.cancelAfter && m.cancelFn != nil {
		m.cancelFn()
	}

	return resp, nil
}

// nopK8sClient satisfies enrichment.K8sClient with no-op responses.
type nopK8sClient struct{}

func (nopK8sClient) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "test-deploy", Namespace: "default"}}, nil
}
func (nopK8sClient) GetSpecHash(_ context.Context, _, _, _ string) (string, error) { return "", nil }

// nopDSClient satisfies enrichment.DataStorageClient with no-op responses.
type nopDSClient struct{}

func (nopDSClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return &enrichment.RemediationHistoryResult{}, nil
}

type cancelTestSpyAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *cancelTestSpyAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *cancelTestSpyAuditStore) getEvents() []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*audit.AuditEvent, len(s.events))
	copy(cp, s.events)
	return cp
}

func (s *cancelTestSpyAuditStore) eventsByType(eventType string) []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*audit.AuditEvent
	for _, e := range s.events {
		if e.EventType == eventType {
			result = append(result, e)
		}
	}
	return result
}

func cancelTestInvestigator(client llm.Client) *investigator.Investigator {
	return cancelTestInvestigatorWithAudit(client, audit.NopAuditStore{})
}

func cancelTestInvestigatorWithAudit(client llm.Client, auditStore audit.AuditStore) *investigator.Investigator {
	logger := slog.Default()
	builder, _ := prompt.NewBuilder()
	rp := parser.NewResultParser()
	enricher := enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, logger)
	return investigator.New(investigator.Config{
		Client:       client,
		Builder:      builder,
		ResultParser: rp,
		Enricher:     enricher,
		AuditStore:   auditStore,
		Logger:       logger,
		MaxTurns:     15,
		PhaseTools:   investigator.DefaultPhaseToolMap(),
	})
}

var testSignal = katypes.SignalContext{
	Name:          "test-pod",
	Namespace:     "default",
	Severity:      "critical",
	Message:       "OOMKilled",
	RemediationID: "rem-cancel-test",
}

var _ = Describe("Kubernaut Agent Investigator Cancellation — #823 PR3", func() {

	Describe("UT-KA-823-C01: Between-turn cancellation checkpoint", func() {
		It("operator cancels during multi-turn loop — investigation aborts at next turn boundary with state preserved", func() {
			ctx, cancel := context.WithCancel(context.Background())
			mockClient := &cancelAwareMockClient{
				cancelAfter: 1,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "Investigating..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
					{
						Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"completed"}`},
						Usage:   llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300},
					},
				},
			}

			inv := cancelTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred(), "cancellation should not produce an error")
			Expect(result).NotTo(BeNil(), "cancelled investigation should return a partial result")
			Expect(result.Cancelled).To(BeTrue(), "result must indicate cancellation (BR-SESSION-002)")
			Expect(result.CancelledPhase).To(Equal("rca"), "should record which phase was cancelled")
			Expect(result.CancelledAtTurn).To(BeNumerically(">", 0), "should record the turn at which cancellation was detected (RR-1)")
		})
	})

	Describe("UT-KA-823-C02: Chat error path — context.Canceled produces clean abort", func() {
		It("Chat returns context.Canceled — produces CancelledResult, not an error", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{},
			}

			inv := cancelTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred(), "context.Canceled from Chat should not surface as error")
			Expect(result).NotTo(BeNil(), "cancelled investigation should return partial result")
			Expect(result.Cancelled).To(BeTrue(), "result must indicate cancellation")
		})
	})

	Describe("UT-KA-823-C03: RCA retry fast-abort on cancelled context", func() {
		It("retry loop aborts immediately without making further LLM calls", func() {
			ctx, cancel := context.WithCancel(context.Background())

			mockClient := &cancelAwareMockClient{
				cancelAfter: 1,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: "unparseable garbage"}},
				},
			}

			inv := cancelTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred(), "cancellation should not produce an error")
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue(), "cancelled during retry should mark result as cancelled")
			Expect(mockClient.callIdx).To(Equal(1), "retry loop should not make additional Chat calls after cancel")
		})
	})

	Describe("UT-KA-823-C04: Workflow retry fast-abort on cancelled context", func() {
		It("workflow retry aborts immediately without further LLM calls", func() {
			ctx, cancel := context.WithCancel(context.Background())

			mockClient := &cancelAwareMockClient{
				cancelAfter: 2,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`}},
					{Message: llm.Message{Role: "assistant", Content: "unparseable workflow garbage"}},
				},
			}

			inv := cancelTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred(), "cancellation should not produce an error")
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue(), "cancelled during workflow retry should mark as cancelled")
			Expect(mockClient.callIdx).To(Equal(2), "no additional Chat calls after cancel in workflow retry")
		})
	})

	Describe("UT-KA-823-C05: runRCA returns partial InvestigationResult with Cancelled=true", func() {
		It("cancelled during RCA tool execution produces partial result", func() {
			ctx, cancel := context.WithCancel(context.Background())

			mockClient := &cancelAwareMockClient{
				cancelAfter: 1,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "Investigating..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_rca", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"test","namespace":"default"}`},
						},
						Usage: llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
					},
				},
			}

			inv := cancelTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue())
			Expect(result.CancelledPhase).To(Equal("rca"),
				"cancel fires after turn 0 tool call; between-turn checkpoint catches it in RCA phase")
		})
	})

	Describe("UT-KA-823-C06: runWorkflowSelection returns partial result on cancel", func() {
		It("cancelled workflow selection preserves RCA summary in partial result", func() {
			ctx, cancel := context.WithCancel(context.Background())

			mockClient := &cancelAwareMockClient{
				cancelAfter: 2,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled due to memory limit","severity":"critical","confidence":0.9}`}},
					{
						Message: llm.Message{Role: "assistant", Content: "Searching workflows..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_wf", Name: "list_workflows", Arguments: `{}`},
						},
						Usage: llm.TokenUsage{PromptTokens: 200, CompletionTokens: 100, TotalTokens: 300},
					},
				},
			}

			inv := cancelTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue())
			Expect(result.CancelledPhase).To(Equal("workflow_discovery"))
			Expect(result.RCASummary).To(ContainSubstring("OOMKilled"), "RCA summary from phase 1 must be preserved")
		})
	})

	Describe("UT-KA-823-C07: Investigate short-circuits after cancelled RCA", func() {
		It("does NOT proceed to workflow selection after cancelled RCA", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			mockClient := &cancelAwareMockClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"would-not-reach"}`}},
					{Message: llm.Message{Role: "assistant", Content: `{"workflow_id":"should-not-reach"}`}},
				},
			}

			inv := cancelTestInvestigator(mockClient)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue())
			Expect(result.WorkflowID).To(BeEmpty(), "workflow selection should NOT have been invoked")
			Expect(len(mockClient.calls)).To(BeNumerically("<=", 1),
				"at most 1 Chat call (the failed RCA), no workflow selection calls")
		})
	})

	Describe("UT-KA-823-C11: Cancellation emits investigation-level audit event (RR-4)", func() {
		It("emits aiagent.investigation.cancelled with phase and turn on cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			spy := &cancelTestSpyAuditStore{}
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

			inv := cancelTestInvestigatorWithAudit(mockClient, spy)
			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Cancelled).To(BeTrue())

			cancelEvents := spy.eventsByType(audit.EventTypeInvestigationCancelled)
			Expect(cancelEvents).To(HaveLen(1), "exactly one investigation.cancelled event expected")

			evt := cancelEvents[0]
			Expect(evt.EventAction).To(Equal(audit.ActionInvestigationCancelled))
			Expect(evt.EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(evt.Data).To(HaveKeyWithValue("cancelled_phase", "rca"))
			Expect(evt.Data).To(HaveKey("cancelled_at_turn"))
			Expect(evt.CorrelationID).To(Equal(testSignal.RemediationID))
		})
	})

	Describe("UT-KA-823-C10: Self-correction cancellation propagation", func() {
		It("cancellation during self-correction propagates cleanly as cancelled result", func() {
			ctx, cancel := context.WithCancel(context.Background())

			mockClient := &cancelAwareMockClient{
				cancelAfter: 2,
				cancelFn:    cancel,
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod crash loop","confidence":0.8}`}},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_wf", Name: "submit_result_with_workflow",
								Arguments: `{"workflow_id":"invalid-wf","confidence":0.7,"remediation_target":{"kind":"Deployment","name":"test","namespace":"default"}}`},
						},
					},
				},
			}

			validator := parser.NewValidator([]string{"valid-wf"})

			inv := investigator.New(investigator.Config{
				Client:       mockClient,
				Builder:      func() *prompt.Builder { b, _ := prompt.NewBuilder(); return b }(),
				ResultParser: parser.NewResultParser(),
				Enricher:     enrichment.NewEnricher(nopK8sClient{}, nopDSClient{}, audit.NopAuditStore{}, slog.Default()),
				AuditStore:   audit.NopAuditStore{},
				Logger:       slog.Default(),
				MaxTurns:     15,
				PhaseTools:   investigator.DefaultPhaseToolMap(),
				Pipeline: investigator.Pipeline{
					CatalogFetcher: &staticFetcher{validator: validator},
				},
			})

			result, err := inv.Investigate(ctx, testSignal)

			Expect(err).NotTo(HaveOccurred(), "cancellation during self-correction should not produce an error")
			Expect(result).NotTo(BeNil())
			Expect(result.Cancelled).To(BeTrue(), "result should indicate cancellation")
		})
	})
})

type staticFetcher struct {
	validator *parser.Validator
}

func (f *staticFetcher) FetchValidator(_ context.Context) (*parser.Validator, error) {
	return f.validator, nil
}
