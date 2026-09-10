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

package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Gap 2 (#2387): interactive assembly converges onto the cumulative per-RR
// accounting. discover_workflows Step 5 stamps the stored RCA from the
// runner totals; complete_no_action applies its totals provider (covering
// the no-discovery path). BR-KA-OBSERVABILITY-001, FedRAMP AU-3.
var _ = Describe("Interactive totals convergence — #2387 Gap 2", func() {

	Describe("UT-KA-2387-013: discover_workflows Step 5 stamps stored RCA with runner totals", func() {
		It("should overwrite stale/absent totals with the cumulative snapshot", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-2387-013",
				CorrelationID: "rr-2387-013",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{
				totals: katypes.InvestigationTotals{
					LLMTurns: 7, ToolCalls: 5,
					PromptTokens: 100, CompletionTokens: 60, TotalTokens: 160,
				},
				workflowDiscoveryResult: &katypes.InvestigationResult{
					RCASummary: "OOM on pod",
					WorkflowID: "mock-workflow",
					Confidence: 0.85,
				},
			}
			recon := &mockContextReconstructor{}
			resolver := &mockSignalResolver{}
			autoMgr := &mockAutoMgrWithHTTPSession{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "OOM on pod",
					Confidence: 0.9,
				},
			}
			completer := &mockHTTPCompleter{foundID: "http-sess-2387-013", found: true}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithSignalContextResolver(resolver),
				mcptools.WithHTTPCompleter(completer),
				mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{
					workflow: &mcptools.CatalogWorkflow{WorkflowID: "mock-workflow", WorkflowName: "Mock Workflow"},
				}))

			_, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2387-013",
				Action: mcptools.ActionDiscoverWorkflows,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())

			Expect(sess.RCAResult).NotTo(BeNil())
			Expect(sess.RCAResult.TotalLLMTurns).To(Equal(7),
				"UT-KA-2387-013: Step 5 must converge the stored RCA onto cumulative totals, not the extraction-only numbers")
			Expect(sess.RCAResult.TotalToolCalls).To(Equal(5))
			Expect(sess.RCAResult.TokenUsage).NotTo(BeNil())
			Expect(sess.RCAResult.TokenUsage.PromptTokens).To(Equal(100))
			Expect(sess.RCAResult.TokenUsage.CompletionTokens).To(Equal(60))
			Expect(sess.RCAResult.TokenUsage.TotalTokens).To(Equal(160))
		})
	})

	Describe("UT-KA-2387-014: complete_no_action applies its totals provider", func() {
		It("should converge the completed result even with no prior discovery", func() {
			sessionMgr := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "mcp-sess-2387-014",
					CorrelationID: "rr-2387-014",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
					RCAResult:     &katypes.InvestigationResult{RCASummary: "false alarm"},
				},
			}
			completer := &mockHTTPCompleter{foundID: "http-sess-2387-014", found: true}

			tool := mcptools.NewCompleteNoActionTool(sessionMgr,
				mcptools.WithCompleteNoActionHTTPCompleter(completer),
				mcptools.WithCompleteNoActionTotalsProvider(func(_ context.Context, _ string) katypes.InvestigationTotals {
					return katypes.InvestigationTotals{
						LLMTurns: 4, ToolCalls: 3,
						PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70,
					}
				}),
			)

			out, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{
				RRID:   "rr-2387-014",
				Reason: "not actionable",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("completed_no_action"))

			_, completedResult := completer.getCompleted()
			Expect(completedResult).NotTo(BeNil())
			Expect(completedResult.TotalLLMTurns).To(Equal(4),
				"UT-KA-2387-014: the no-discovery path must still report cumulative totals, not zeros")
			Expect(completedResult.TotalToolCalls).To(Equal(3))
			Expect(completedResult.TokenUsage).NotTo(BeNil())
			Expect(completedResult.TokenUsage.TotalTokens).To(Equal(70))
		})
	})
})
