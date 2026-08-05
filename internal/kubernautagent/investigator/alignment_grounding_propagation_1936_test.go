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
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// shadowCapturingClient1936 is a minimal llm.Client for the shadow/grounding
// evaluator that records the rendered content of every request it receives,
// so specs can inspect exactly what the shadow agent saw.
type shadowCapturingClient1936 struct {
	mu    sync.Mutex
	calls []string
}

func (c *shadowCapturingClient1936) Close() error { return nil }

func (c *shadowCapturingClient1936) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return c.Chat(ctx, req)
}

func (c *shadowCapturingClient1936) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
	}
	c.mu.Lock()
	c.calls = append(c.calls, b.String())
	c.mu.Unlock()
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"grounded":true,"explanation":"clean"}`}}, nil
}

func (c *shadowCapturingClient1936) allCalls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.calls))
	copy(out, c.calls)
	return out
}

// #1936 (main port of #1935 root cause #2, extended to the shadow/alignment
// agent; BR-AUDIT-005 v2.0, FedRAMP CC7.2/CC8.1): the same message-staleness
// bug that starves sameKindValidationGate/apiVersionValidationGate of
// tool-call history also starves alignment.NotifyRCAComplete's full-context
// grounding review -- the shadow agent's core defense against
// prompt-injection/hallucination has been evaluating an almost-empty
// [system, user] conversation regardless of how many tools were actually
// called. Additionally, renderConversation on main only renders msg.Content,
// so even once propagation is fixed the shadow LLM would still never see
// the model's Reasoning.Text (BR-AI-086) without the accompanying one-line
// fix to grounding.go. This spec drives a real Investigator through its
// real production wiring (Investigate -> runRCA -> alignment.NotifyRCAComplete
// -> Observer.StartGroundingReview -> Evaluator.EvaluateGrounding) with a
// real Observer/Evaluator pair and proves the shadow LLM's request actually
// contains both.
var _ = Describe("#1936 (alignment): grounding review must see real tool-call history and thinking text", func() {

	Describe("IT-KA-1936-006: shadow agent's grounding request includes tool-call history and Reasoning text", func() {
		It("renders the earlier kubectl_describe tool_use/tool_result pair and the thinking-block text into the grounding LLM request", func() {
			primaryClient := &gateMockLLMClient{
				responses: []llm.ChatResponse{
					// Turn 1: real tool call, with a thinking block attached (Sonnet-5-style).
					{
						Message: llm.Message{
							Role:      "assistant",
							Content:   "Investigating...",
							Reasoning: &llm.ReasoningBlock{Text: "UNIQUE_REASONING_MARKER_1936"},
						},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api-server-xyz","namespace":"production"}`},
						},
					},
					// Turn 2: submit_result.
					{Message: llm.Message{Role: "assistant", Content: `{
						"rca_summary":"Pod OOMKilled",
						"confidence":0.85,
						"remediation_target":{"kind":"Deployment","name":"api-server","namespace":"production","api_version":"apps/v1"}
					}`}},
					gateWfToolResp(`{"workflow_id":"increase-memory-limit","confidence":0.9}`),
				},
			}

			shadowClient := &shadowCapturingClient1936{}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout:       2 * time.Second,
				MaxStepTokens: 2000,
				MaxRetries:    1,
				// The real investigation system prompt (rendered by
				// prompt.Builder) is several KB — a small budget here would
				// truncate renderConversation before reaching the tool-call
				// turns this spec asserts on, which is a test artifact, not
				// the behavior under test.
				MaxConversationTokens: 200000,
			}, "You are a grounding reviewer.")
			obs, err := alignment.NewObserver(evaluator,
				alignment.WithCorrelationID("corr-1936-006"),
				alignment.WithObserverLogger(logr.Discard()),
				alignment.WithGroundingEnabled(true),
			)
			Expect(err).NotTo(HaveOccurred())

			auditStore := &gateRecordingAuditStore{}
			builder, _ := prompt.NewBuilder()
			enricher := enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, auditStore, logr.Discard())

			// investigator.New (not newTestInvestigator): a nil
			// CatalogFetcher makes workflow-discovery fail closed
			// immediately after the RCA phase under test, matching
			// #1935's alignment_grounding_propagation_1935_test.go
			// precedent.
			inv := investigator.New(investigator.Config{
				Client: primaryClient, Builder: builder, ResultParser: parser.NewResultParser(),
				Enricher: enricher, AuditStore: auditStore, Logger: logr.Discard(),
				MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			})

			signal := katypes.SignalContext{
				ResourceKind: "Pod",
				ResourceName: "api-server-xyz",
				Name:         "api-server-xyz",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "Pod OOMKilled repeatedly",
			}

			ctx := alignment.WithObserver(context.Background(), obs)
			_, err = inv.Investigate(ctx, signal)
			Expect(err).NotTo(HaveOccurred())

			waitRes := obs.WaitForCompletion(5 * time.Second)
			Expect(waitRes.Complete).To(BeTrue(), "IT-KA-1936-006: grounding review must complete before assertions")

			calls := shadowClient.allCalls()
			Expect(calls).NotTo(BeEmpty())

			var groundingCall string
			for _, c := range calls {
				if strings.Contains(c, "[tool]") {
					groundingCall = c
					break
				}
			}
			Expect(groundingCall).NotTo(BeEmpty(),
				"IT-KA-1936-006: the grounding review's rendered conversation must include a \"[tool]\" turn "+
					"from the earlier kubectl_describe call (#1935/#1936 root cause #2, extended to alignment)")
			Expect(groundingCall).To(ContainSubstring("kubectl_describe"),
				"IT-KA-1936-006: grounding review must include the actual tool name that was called")
			Expect(groundingCall).To(ContainSubstring("UNIQUE_REASONING_MARKER_1936"),
				"IT-KA-1936-006: grounding review must include the model's thinking-block text (BR-AI-086), "+
					"not just Content — renderConversation must render Reasoning too")
		})
	})
})
