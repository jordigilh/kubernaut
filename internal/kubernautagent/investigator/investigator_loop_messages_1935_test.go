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

// loopMsgsNopAuditStore discards audit events; only runLLMLoop's Messages
// propagation is under test here, not audit persistence.
type loopMsgsNopAuditStore struct{}

func (loopMsgsNopAuditStore) StoreAudit(_ context.Context, _ *audit.AuditEvent) error { return nil }

// White-box (package investigator, not investigator_test) because these
// specs exercise runLLMLoop and its LoopResult types directly, which are
// intentionally unexported (#1935 root cause #2): runLLMLoop accumulates
// tool-call/tool-result turns in a local `messages` slice as the loop
// progresses, but until this fix neither SubmitResult nor TextResult
// carried that accumulated history back to the caller — only
// CancelledResult did. runRCA's own `messages` variable therefore stayed
// frozen at its initial [system, user] value for the rest of the RCA
// phase, so validation-gate retries (sameKindValidationGate,
// apiVersionValidationGate) and the parse-error retry (retryRCASubmit)
// always operated on an empty tool-call history regardless of how many
// tools were actually called earlier in the same investigation. Confirmed
// present on release/v1.5 and main; reproduced against live production
// audit traces for rr-618ac7d3b894-ba320bf0 (same symptom on both
// claude-sonnet-5 and claude-sonnet-4-6, ruling out a model-specific
// cause).

// loopMsgsMockClient is a minimal llm.Client that returns pre-configured
// responses in order, driving runLLMLoop through one or more turns.
type loopMsgsMockClient struct {
	responses []llm.ChatResponse
	callIdx   int
}

func (m *loopMsgsMockClient) Close() error { return nil }

func (m *loopMsgsMockClient) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return m.Chat(ctx, req)
}

func (m *loopMsgsMockClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp, nil
	}
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"fallback"}`}}, nil
}

func newLoopMsgsTestInvestigator(client llm.Client) *Investigator {
	return New(Config{
		Client:     client,
		Logger:     logr.Discard(),
		MaxTurns:   10,
		PhaseTools: DefaultPhaseToolMap(),
		AuditStore: loopMsgsNopAuditStore{},
	})
}

var _ = Describe("#1935 root cause #2: runLLMLoop must return its accumulated message history", func() {

	Describe("UT-KA-1935-006: SubmitResult carries prior tool-call/tool-result turns", func() {
		It("populates Messages with the fully-paired assistant/tool turns accumulated before the sentinel fires", func() {
			client := &loopMsgsMockClient{
				responses: []llm.ChatResponse{
					{
						Message: llm.Message{Role: "assistant", Content: "Investigating..."},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_1", Name: "kubectl_describe", Arguments: `{"kind":"Pod","name":"api-server","namespace":"prod"}`},
						},
					},
					{
						Message: llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{
							{ID: "tc_2", Name: SubmitResultToolName, Arguments: `{"rca_summary":"OOMKilled","confidence":0.9}`},
						},
					},
				},
			}
			inv := newLoopMsgsTestInvestigator(client)

			initial := []llm.Message{
				{Role: "system", Content: "system prompt"},
				{Role: "user", Content: "Investigate: critical api-server in prod"},
			}
			res, err := inv.runLLMLoop(context.Background(), initial, katypes.PhaseRCA, nil, "corr-1935-006", client, "test-model", llm.RuntimeParams{})
			Expect(err).NotTo(HaveOccurred())

			sr, ok := res.(*SubmitResult)
			Expect(ok).To(BeTrue(), "expected a SubmitResult when the LLM calls submit_result")

			Expect(sr.Messages).NotTo(BeEmpty(),
				"UT-KA-1935-006: SubmitResult.Messages must carry the accumulated history, "+
					"not be left empty/nil (#1935 root cause #2)")

			var sawToolCall, sawToolResult bool
			for _, msg := range sr.Messages {
				for _, tc := range msg.ToolCalls {
					if tc.Name == "kubectl_describe" {
						sawToolCall = true
					}
				}
				if msg.Role == "tool" && msg.ToolName == "kubectl_describe" {
					sawToolResult = true
				}
			}
			Expect(sawToolCall).To(BeTrue(), "UT-KA-1935-006: Messages must include the earlier kubectl_describe tool_use turn")
			Expect(sawToolResult).To(BeTrue(), "UT-KA-1935-006: Messages must include the paired tool_result turn")

			for _, msg := range sr.Messages {
				for _, tc := range msg.ToolCalls {
					Expect(tc.Name).NotTo(Equal(SubmitResultToolName),
						"UT-KA-1935-006: the final, still-unpaired submit_result call must NOT be included "+
							"in Messages (would violate the tool_use/tool_result pairing contract on replay)")
				}
			}
		})
	})

	Describe("UT-KA-1935-007: TextResult carries prior tool-call/tool-result turns", func() {
		It("populates Messages with the accumulated turns when the final response is plain text with no tool call", func() {
			client := &loopMsgsMockClient{
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
			inv := newLoopMsgsTestInvestigator(client)

			initial := []llm.Message{
				{Role: "system", Content: "system prompt"},
				{Role: "user", Content: "Investigate: critical api-server in prod"},
			}
			res, err := inv.runLLMLoop(context.Background(), initial, katypes.PhaseRCA, nil, "corr-1935-007", client, "test-model", llm.RuntimeParams{})
			Expect(err).NotTo(HaveOccurred())

			tr, ok := res.(*TextResult)
			Expect(ok).To(BeTrue(), "expected a TextResult when the LLM responds with plain text and no tool call")

			Expect(tr.Messages).NotTo(BeEmpty(),
				"UT-KA-1935-007: TextResult.Messages must carry the accumulated history (#1935 root cause #2)")

			var sawToolResult bool
			for _, msg := range tr.Messages {
				if msg.Role == "tool" && msg.ToolName == "kubectl_logs" {
					sawToolResult = true
				}
			}
			Expect(sawToolResult).To(BeTrue(), "UT-KA-1935-007: Messages must include the earlier kubectl_logs tool_result turn")
		})
	})
})
