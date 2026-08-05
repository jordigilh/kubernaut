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

package investigator

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// White-box (package investigator, not investigator_test) because
// sentinelResult/loopResultMessages/runLoopTurn are intentionally
// unexported. #1936 ports #1935/PR #1939 (RCA phase) and #1945/PR #1947
// (workflow-discovery phase) from release/v1.5 to main. Unlike v1.5, main's
// LoopResult types (SubmitResult/SubmitWithWorkflowResult/
// SubmitNoWorkflowResult/TextResult) carry NO Messages field at all before
// this fix -- only CancelledResult does -- so runRCA's and
// runWorkflowSelection's gates, parse-retries, self-correction loop, and
// the alignment shadow-agent grounding review all operate on a stale
// [system, user]-only history today, regardless of model or how many tools
// actually ran. Confirmed via direct source read of origin/main (see
// docs/testing/1936/TEST_PLAN.md Section 1).

var _ = Describe("#1936: sentinelResult and loopResultMessages must propagate accumulated message history", func() {

	accumulated := []llm.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "Investigate: critical api-server in prod"},
		{Role: "assistant", Content: "Investigating...", ToolCalls: []llm.ToolCall{
			{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api-server"}`},
		}},
		{Role: "tool", Content: `{"status":"OOMKilled"}`, ToolCallID: "tc_1", ToolName: "kubectl_describe"},
	}

	Describe("UT-KA-1936-001: sentinelResult populates Messages on all three sentinel types", func() {
		DescribeTable("returns a LoopResult carrying the accumulated history, not just Content/Reasoning",
			func(toolName string, assertType func(LoopResult) []llm.Message) {
				reasoning := &llm.ReasoningBlock{Text: "thinking..."}
				tc := llm.ToolCall{ID: "tc_final", Name: toolName, Arguments: `{"confidence":0.9}`}

				result := sentinelResult(tc, reasoning, accumulated)
				Expect(result).NotTo(BeNil())

				got := assertType(result)
				Expect(got).NotTo(BeEmpty(),
					"UT-KA-1936-001: sentinelResult must populate Messages from its messages parameter, "+
						"not leave it as a zero-value nil slice (#1935/#1945 root cause #2, main port)")
				Expect(got).To(Equal(accumulated))
			},
			Entry("submit_result -> SubmitResult", SubmitResultToolName, func(r LoopResult) []llm.Message {
				sr, ok := r.(*SubmitResult)
				Expect(ok).To(BeTrue(), "expected *SubmitResult")
				return sr.Messages
			}),
			Entry("submit_result_with_workflow -> SubmitWithWorkflowResult", SubmitResultWithWorkflowToolName, func(r LoopResult) []llm.Message {
				sr, ok := r.(*SubmitWithWorkflowResult)
				Expect(ok).To(BeTrue(), "expected *SubmitWithWorkflowResult")
				return sr.Messages
			}),
			Entry("submit_result_no_workflow -> SubmitNoWorkflowResult", SubmitResultNoWorkflowToolName, func(r LoopResult) []llm.Message {
				sr, ok := r.(*SubmitNoWorkflowResult)
				Expect(ok).To(BeTrue(), "expected *SubmitNoWorkflowResult")
				return sr.Messages
			}),
		)
	})

	Describe("UT-KA-1936-003: loopResultMessages covers all four message-carrying LoopResult types", func() {
		It("returns SubmitResult.Messages", func() {
			got := loopResultMessages(&SubmitResult{Content: "x", Messages: accumulated})
			Expect(got).To(Equal(accumulated))
		})
		It("returns TextResult.Messages", func() {
			got := loopResultMessages(&TextResult{Content: "x", Messages: accumulated})
			Expect(got).To(Equal(accumulated))
		})
		It("returns SubmitWithWorkflowResult.Messages", func() {
			got := loopResultMessages(&SubmitWithWorkflowResult{Content: "x", Messages: accumulated})
			Expect(got).To(Equal(accumulated))
		})
		It("returns SubmitNoWorkflowResult.Messages", func() {
			got := loopResultMessages(&SubmitNoWorkflowResult{Content: "x", Messages: accumulated})
			Expect(got).To(Equal(accumulated))
		})
		It("returns nil for CancelledResult (handled separately by its own callers)", func() {
			got := loopResultMessages(&CancelledResult{Messages: accumulated})
			Expect(got).To(BeNil(),
				"UT-KA-1936-003: CancelledResult's Messages is read directly by cancellation-snapshot "+
					"callers, not via loopResultMessages -- this must remain nil to avoid masking that contract")
		})
		It("returns nil for ExhaustedResult (carries no message history)", func() {
			got := loopResultMessages(&ExhaustedResult{Reason: "max turns exhausted"})
			Expect(got).To(BeNil())
		})
	})
})

// msgPropNopAuditStore discards audit events; only runLLMLoop's Messages
// propagation is under test here, not audit persistence.
type msgPropNopAuditStore struct{}

func (msgPropNopAuditStore) StoreAudit(_ context.Context, _ *audit.AuditEvent) error { return nil }

// msgPropMockClient is a minimal llm.Client that returns pre-configured
// responses in order, driving runLLMLoop through one or more turns.
type msgPropMockClient struct {
	responses []llm.ChatResponse
	callIdx   int
}

func (m *msgPropMockClient) Close() error { return nil }

func (m *msgPropMockClient) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return m.Chat(ctx, req)
}

func (m *msgPropMockClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp, nil
	}
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback"}`}}, nil
}

func newMsgPropTestInvestigator(client llm.Client) *Investigator {
	return New(Config{
		Client:     client,
		Logger:     logr.Discard(),
		MaxTurns:   10,
		PhaseTools: DefaultPhaseToolMap(),
		AuditStore: msgPropNopAuditStore{},
	})
}

var _ = Describe("#1936: runLoopTurn's final TextResult must carry accumulated message history", func() {

	Describe("UT-KA-1936-002: TextResult carries prior tool-call/tool-result turns", func() {
		It("populates Messages when the final response is plain text with no tool call", func() {
			client := &msgPropMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "Investigating..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_logs", Arguments: `{"name":"api-server","namespace":"prod"}`},
						},
					},
					{
						Message: llm.Message{Role: "assistant", Content: "Plain-text analysis with no tool call."},
					},
				},
			}
			inv := newMsgPropTestInvestigator(client)

			initial := []llm.Message{
				{Role: "system", Content: "system prompt"},
				{Role: "user", Content: "Investigate: critical api-server in prod"},
			}
			res, err := inv.runLLMLoop(context.Background(), initial, katypes.PhaseRCA, LLMInvocationContext{
				CorrelationID: "corr-1936-002",
				Client:        client,
				ModelName:     "test-model",
			})
			Expect(err).NotTo(HaveOccurred())

			tr, ok := res.(*TextResult)
			Expect(ok).To(BeTrue(), "expected a TextResult when the LLM responds with plain text and no tool call")

			Expect(tr.Messages).NotTo(BeEmpty(),
				"UT-KA-1936-002: TextResult.Messages must carry the accumulated history (#1935/#1945 root cause #2, main port)")

			var sawToolResult bool
			for _, msg := range tr.Messages {
				if msg.Role == "tool" && msg.ToolName == "kubectl_logs" {
					sawToolResult = true
				}
			}
			Expect(sawToolResult).To(BeTrue(), "UT-KA-1936-002: Messages must include the earlier kubectl_logs tool_result turn")
		})
	})
})
