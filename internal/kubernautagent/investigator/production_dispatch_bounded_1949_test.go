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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// hangingToolName is the real Phase-3 tool name ("kubectl_describe", per
// DefaultPhaseToolMap) that this test's fake tool and scripted LLM response
// both reference, so the LLM's tool call and the registry lookup agree on
// which (hanging) implementation gets dispatched.
const hangingToolName = "kubectl_describe"

// hangingKubectlDescribeTool re-registers hangingToolName with an
// implementation that blocks on ctx.Done() forever — standing in for
// #1949's evidenced hang candidates (a stuck client-go/net-http call), so
// this test exercises the exact production dispatch path
// (Investigator.RunWorkflowDiscoveryFromRCA -> runWorkflowSelection ->
// processToolCalls -> registry.Execute) rather than processToolCalls in
// isolation (that's UT-KA-1949-001/002).
type hangingKubectlDescribeTool struct{}

func (hangingKubectlDescribeTool) Name() string               { return hangingToolName }
func (hangingKubectlDescribeTool) Description() string        { return "test: hangs forever" }
func (hangingKubectlDescribeTool) Parameters() json.RawMessage { return json.RawMessage(`{}`) }
func (hangingKubectlDescribeTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

var _ tools.Tool = hangingKubectlDescribeTool{}

var _ = Describe("Kubernaut Agent Investigator — production dispatch path bounded execution (#1949, BR-KA-267, SC-5)", func() {

	Describe("IT-KA-1949-001: RunWorkflowDiscoveryFromRCA (Investigator.RunWorkflowDiscovery, the production entry InvestigateTool.handleDiscoverWorkflows drives via InvestigatorRunnerAdapter) returns within bounded time when a real registered tool hangs", func() {
		It("does not hang past ToolCallTimeout+margin, and completes the turn once the LLM sees the timeout tool-result and issues its final decision", func() {
			reg := registry.New()
			reg.Register(hangingKubectlDescribeTool{})

			client := &gateMockLLMClient{
				responses: []llm.ChatResponse{
					// Turn 1: the LLM requests the real (hanging) tool —
					// dispatched through the actual errgroup path in
					// processToolCalls, not a sentinel short-circuit.
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1949_001", Name: hangingToolName, Arguments: "{}"}},
					},
					// Turn 2: once the tool call times out and its error is
					// fed back as a tool-result message, the LLM (scripted
					// here, but standing in for a real one reacting to the
					// timeout) issues its final decision via the sentinel.
					gateWfToolResp(`{"workflow_id":"restart-pod","confidence":0.9}`),
				},
			}

			builder, buildErr := prompt.NewBuilder()
			Expect(buildErr).NotTo(HaveOccurred())

			inv := newTestInvestigator(investigator.Config{
				Client:          client,
				Builder:         builder,
				ResultParser:    parser.NewResultParser(),
				AuditStore:      audit.NopAuditStore{},
				Logger:          logr.Discard(),
				MaxTurns:        15,
				PhaseTools:      investigator.DefaultPhaseToolMap(),
				Registry:        reg,
				ToolCallTimeout: 50 * time.Millisecond,
			})

			rcaResult := &katypes.InvestigationResult{RCASummary: "OOMKilled", Confidence: 0.9}
			signal := katypes.SignalContext{Name: "test-pod", Namespace: "default", ResourceKind: "Pod", ResourceName: "test-pod"}

			type callResult struct {
				result *katypes.InvestigationResult
				err    error
			}
			done := make(chan callResult, 1)
			go func() {
				defer GinkgoRecover()
				result, err := inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-it-1949-001")
				done <- callResult{result: result, err: err}
			}()

			var out callResult
			Eventually(done, "2s", "10ms").Should(Receive(&out),
				"RunWorkflowDiscoveryFromRCA must return within ToolCallTimeout+margin even when a real registered tool never returns on its own — the regression guard for #1949's production dispatch path (before the fix, this Eventually times out because the underlying errgroup dispatch never returns)")

			Expect(out.err).NotTo(HaveOccurred())
			Expect(out.result).NotTo(BeNil())
			Expect(out.result.WorkflowID).To(Equal("restart-pod"),
				"the investigation must still reach a final decision after recovering from the bounded tool-call timeout")
		})
	})
})
