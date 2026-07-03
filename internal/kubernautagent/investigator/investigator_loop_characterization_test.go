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
	"errors"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// This file characterizes runLLMLoop branches left uncovered by the existing
// suite (verified via `go tool cover -func`): the generic (non-cancellation)
// LLM error path, tool-call budget exhaustion, the truncation-retry escalation,
// and the max-turns-exhausted fallthrough. Wave 5 (GO-ANTIPATTERN-AUDIT
// 2026-07-01) RED phase — these tests must pass unchanged after runLLMLoop is
// decomposed in Phase 3.

// scriptedStep is one scripted turn for scriptedMockClient: either a response
// or an error, returned in call order.
type scriptedStep struct {
	resp llm.ChatResponse
	err  error
}

// scriptedMockClient is a minimal llm.Client fake that returns pre-scripted
// responses/errors in call order, falling back to a generic parseable RCA
// response once the script is exhausted (mirrors cancelAwareMockClient's
// fallback in cancel_test.go, but without cancellation semantics — these
// tests target error/exhaustion/truncation branches instead).
type scriptedMockClient struct {
	steps   []scriptedStep
	callIdx int
	calls   []llm.ChatRequest
}

func (m *scriptedMockClient) Close() error { return nil }

func (m *scriptedMockClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.calls = append(m.calls, req)
	if m.callIdx >= len(m.steps) {
		return llm.ChatResponse{
			Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback","confidence":0.1}`},
		}, nil
	}
	step := m.steps[m.callIdx]
	m.callIdx++
	if step.err != nil {
		return llm.ChatResponse{}, step.err
	}
	return step.resp, nil
}

func (m *scriptedMockClient) StreamChat(_ context.Context, req llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := m.Chat(context.Background(), req)
	if err == nil {
		_ = callback(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

// stubToolRegistry is a minimal registry.ToolRegistry fake that executes any
// tool call with a fixed success payload. A registry must be configured for
// AnomalyDetector.CheckToolCall to be invoked at all (see executeTool in
// investigator_tools.go, which short-circuits when Registry is nil).
type stubToolRegistry struct{}

func (stubToolRegistry) Execute(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	return `{"ok":true}`, nil
}
func (stubToolRegistry) ToolsForPhase(_ katypes.Phase, _ katypes.PhaseToolMap) []tools.Tool {
	return nil
}
func (stubToolRegistry) All() []tools.Tool { return nil }

// loopCharTestInvestigator builds an Investigator for runLLMLoop
// characterization, reusing the nop K8s/DS clients already established by
// cancel_test.go in this package.
func loopCharTestInvestigator(client llm.Client, auditStore audit.AuditStore, maxTurns int, pipeline investigator.Pipeline, reg registry.ToolRegistry) *investigator.Investigator {
	logger := logr.Discard()
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
		MaxTurns:     maxTurns,
		PhaseTools:   investigator.DefaultPhaseToolMap(),
		Pipeline:     pipeline,
		Registry:     reg,
	})
}

var _ = Describe("Kubernaut Agent Investigator — runLLMLoop characterization (Wave 5 RED)", func() {

	Describe("UT-KA-WAVE5-L01: generic (non-cancellation) LLM error", func() {
		It("wraps the error, emits a response-failed audit event (AU-3), and returns no result", func() {
			spy := &cancelTestSpyAuditStore{}
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{err: errors.New("upstream 500")},
				},
			}
			inv := loopCharTestInvestigator(mockClient, spy, 15, investigator.Pipeline{}, nil)

			result, err := inv.Investigate(context.Background(), testSignal)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("RCA invocation"))
			Expect(err.Error()).To(ContainSubstring("upstream 500"))
			Expect(result).To(BeNil())

			failEvents := spy.eventsByType(audit.EventTypeResponseFailed)
			Expect(failEvents).To(HaveLen(1), "exactly one response-failed audit event expected (AU-3)")
			Expect(failEvents[0].EventAction).To(Equal(audit.ActionResponseFailed))
			Expect(failEvents[0].EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(failEvents[0].Data).To(HaveKeyWithValue("error_message", "upstream 500"))
			Expect(failEvents[0].Data).To(HaveKeyWithValue("phase", string(katypes.PhaseRCA)))
			Expect(failEvents[0].Data).To(HaveKey("duration_seconds"))
		})
	})

	Describe("UT-KA-WAVE5-L02: tool-call budget exhaustion (AC-6 least-privilege enforcement)", func() {
		It("classifies as human-review-needed with a tool-budget-exhausted reason once AnomalyDetector trips", func() {
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{
						Message: llm.Message{Role: "assistant", Content: "investigating"},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`},
							{ID: "tc_2", Name: "kubectl_logs", Arguments: `{}`},
						},
					}},
				},
			}
			detector := investigator.NewAnomalyDetector(investigator.AnomalyConfig{
				MaxTotalToolCalls:   1,
				MaxToolCallsPerTool: 10,
			}, nil)
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15,
				investigator.Pipeline{AnomalyDetector: detector}, stubToolRegistry{})

			result, err := inv.Investigate(context.Background(), testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("tool budget exhausted"))
			Expect(result.Reason).To(ContainSubstring("during RCA"))
		})
	})

	Describe("UT-KA-WAVE5-L03: truncation-retry escalates MaxTokens exactly once", func() {
		It("retries once with escalated MaxTokens after a length-truncated response, then succeeds", func() {
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{
						Message:      llm.Message{Role: "assistant", Content: "partial analysis..."},
						FinishReason: llm.FinishReasonLength,
						Usage:        llm.TokenUsage{CompletionTokens: 4096},
					}},
					{resp: llm.ChatResponse{
						Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled","confidence":0.9}`},
					}},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 15, investigator.Pipeline{}, nil)

			// Force RCA-only completion (BR-INTERACTIVE-010 short-circuit) so
			// this test stays isolated to the RCA-phase truncation-retry
			// mechanism and doesn't spill into workflow-discovery calls
			// against the same scripted client.
			sig := testSignal
			sig.Interactive = true

			result, err := inv.Investigate(context.Background(), sig)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RCASummary).To(Equal("OOMKilled"))
			Expect(mockClient.callIdx).To(Equal(2),
				"truncation retry must fire exactly once (truncationRetried guard), not loop indefinitely")
		})
	})

	Describe("UT-KA-WAVE5-L04: max-turns-exhausted fallthrough", func() {
		It("classifies as human-review-needed with a max-turns-exhausted reason when MaxTurns=1 and the LLM keeps calling tools", func() {
			mockClient := &scriptedMockClient{
				steps: []scriptedStep{
					{resp: llm.ChatResponse{
						Message: llm.Message{Role: "assistant", Content: "investigating"},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{}`},
						},
					}},
				},
			}
			inv := loopCharTestInvestigator(mockClient, audit.NopAuditStore{}, 1, investigator.Pipeline{}, stubToolRegistry{})

			result, err := inv.Investigate(context.Background(), testSignal)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.HumanReviewNeeded).To(BeTrue())
			Expect(result.Reason).To(ContainSubstring("max turns exhausted"))
			Expect(result.Reason).To(ContainSubstring("during RCA (maxTurns=1)"))
		})
	})
})
