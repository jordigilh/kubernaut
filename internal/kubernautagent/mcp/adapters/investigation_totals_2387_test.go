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

package adapters_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// Gap 2 wiring guard (#2387): the adapter exposes the investigator's
// cumulative per-RR scope to session assembly points outside the
// investigator package.
type totals2387MockClient struct {
	responses []llm.ChatResponse
	callIdx   int
}

func (m *totals2387MockClient) Close() error { return nil }

func (m *totals2387MockClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	if m.callIdx < len(m.responses) {
		resp := m.responses[m.callIdx]
		m.callIdx++
		return resp, nil
	}
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "done"}}, nil
}

func (m *totals2387MockClient) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return m.Chat(ctx, req)
}

var _ = Describe("InvestigatorRunnerAdapter.InvestigationTotals — #2387 Gap 2", func() {

	Describe("UT-KA-2387-015: adapter reports cumulative scope after interactive turns", func() {
		It("should return turns, tools, and tokens accumulated under the correlationID", func() {
			usage := llm.TokenUsage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10}
			client := &totals2387MockClient{responses: []llm.ChatResponse{
				{
					Message: llm.Message{Role: "assistant", Content: "Investigating..."},
					ToolCalls: []llm.ToolCall{
						{ID: "tc-1", Name: "kubectl_describe", Arguments: `{}`},
					},
					Usage: usage,
				},
				{
					Message: llm.Message{Role: "assistant", Content: "done"},
					ToolCalls: []llm.ToolCall{
						{ID: "tc-2", Name: "submit_result", Arguments: `{"root_cause_analysis":{"summary":"x"}}`},
					},
					Usage: usage,
				},
			}}
			builder, _ := prompt.NewBuilder()
			inv := investigator.New(investigator.Config{
				Client:       client,
				Builder:      builder,
				ResultParser: parser.NewResultParser(),
				AuditStore:   audit.NopAuditStore{},
				Logger:       logr.Discard(),
				MaxTurns:     5,
				PhaseTools:   investigator.DefaultPhaseToolMap(),
			})

			_, err := inv.RunInteractiveTurn(context.Background(), []llm.Message{
				{Role: "system", Content: "system"},
				{Role: "user", Content: "investigate"},
			}, "rr-2387-015")
			Expect(err).NotTo(HaveOccurred())

			totals := adapters.NewInvestigatorRunnerAdapter(inv).InvestigationTotals(context.Background(), "rr-2387-015")
			Expect(totals.LLMTurns).To(Equal(2))
			Expect(totals.ToolCalls).To(Equal(1),
				"the submit_result sentinel is consumed, never executed")
			Expect(totals.PromptTokens).To(Equal(14))
			Expect(totals.CompletionTokens).To(Equal(6))
			Expect(totals.TotalTokens).To(Equal(20))
		})
	})

	Describe("UT-KA-2387-016: adapter satisfies the extended runner interface", func() {
		It("should implement tools.InvestigatorRunner including InvestigationTotals", func() {
			var _ mcptools.InvestigatorRunner = adapters.NewInvestigatorRunnerAdapter(nil)
		})
	})
})
