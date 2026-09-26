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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type discoveryAutoMgr2439 struct {
	reconAutoMgr
	rca *katypes.InvestigationResult
}

func (m *discoveryAutoMgr2439) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return m.rca, m.rca != nil
}

type signalResolver2439 struct {
	signal katypes.SignalContext
}

func (r signalResolver2439) ResolveSignalContext(context.Context, string) (*katypes.SignalContext, error) {
	return &r.signal, nil
}

type workflowCatalog2439 struct{}

func (workflowCatalog2439) GetWorkflowByID(context.Context, string) (*mcptools.CatalogWorkflow, error) {
	return &mcptools.CatalogWorkflow{WorkflowID: "wf-2439", WorkflowName: "restart-pod"}, nil
}

type discoveryLLMClient2439 struct {
	response llm.ChatResponse
	partials []*llm.PartialToolCall
}

func (c discoveryLLMClient2439) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	return c.response, nil
}

func (c discoveryLLMClient2439) StreamChat(_ context.Context, _ llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	for _, partial := range c.partials {
		if err := callback(llm.ChatStreamEvent{ToolCallDelta: partial}); err != nil {
			return llm.ChatResponse{}, err
		}
	}
	if err := callback(llm.ChatStreamEvent{Done: true}); err != nil {
		return llm.ChatResponse{}, err
	}
	return c.response, nil
}

func (discoveryLLMClient2439) Close() error { return nil }

var _ = Describe("MCP workflow discovery tool-call delta wiring — #2439", func() {
	It("IT-KA-2439-002: relays deltas through handleDiscoverWorkflows into the session sink", func() {
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())

		client := discoveryLLMClient2439{
			partials: []*llm.PartialToolCall{
				{Index: 0, ID: "call-2439", Name: "submit_result_with_workflow", ArgumentsDelta: `{"workflow_id":`},
				{Index: 0, ArgumentsDelta: `"wf-2439"}`},
			},
			response: llm.ChatResponse{
				Message: llm.Message{Role: "assistant"},
				ToolCalls: []llm.ToolCall{{
					ID:        "call-2439",
					Name:      investigator.SubmitResultWithWorkflowToolName,
					Arguments: `{"workflow_id":"wf-2439","confidence":0.9,"remediation_target":{"kind":"Pod","name":"test-pod","namespace":"default"}}`,
				}},
			},
		}
		inv := investigator.New(investigator.Config{
			Client:       client,
			Builder:      builder,
			ResultParser: parser.NewResultParser(),
			AuditStore:   audit.NopAuditStore{},
			Logger:       logr.Discard(),
			MaxTurns:     5,
			PhaseTools:   investigator.DefaultPhaseToolMap(),
			Pipeline: investigator.Pipeline{
				CatalogFetcher: discoveryCatalogFetcher2439{},
			},
		})

		const rrID = "rr-2439"
		sessionID := "session-2439"
		signal := katypes.SignalContext{
			Name:          "OOMKilled",
			Namespace:     "default",
			ResourceKind:  "Pod",
			ResourceName:  "test-pod",
			Severity:      "critical",
			RemediationID: rrID,
		}
		autoMgr := &discoveryAutoMgr2439{
			rca: &katypes.InvestigationResult{RCASummary: "pod memory limit exceeded"},
		}
		sessions := &mockSessionManager{
			isActive: true,
			getDriverResult: &mcpinternal.InteractiveSession{
				SessionID:     sessionID,
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			},
		}
		eventCh := make(chan session.InvestigationEvent, 32)
		ctx := session.WithEventSink(context.Background(), eventCh)
		tool := mcptools.NewInvestigateTool(
			sessions,
			adapters.NewInvestigatorRunnerAdapter(inv),
			&mockContextReconstructor{},
			autoMgr,
			mcptools.WithWorkflowCatalog(workflowCatalog2439{}),
			mcptools.WithSignalContextResolver(signalResolver2439{signal: signal}),
		)

		output, err := tool.Handle(ctx, mcptools.InvestigateInput{
			RRID:   rrID,
			Action: mcptools.ActionDiscoverWorkflows,
		}, mcpinternal.UserInfo{Username: "alice"})

		Expect(err).NotTo(HaveOccurred())
		Expect(output.Status).To(Equal("workflows_discovered"))
		var found int
		for len(eventCh) > 0 {
			event := <-eventCh
			if event.Type != session.EventTypeToolCallDelta {
				continue
			}
			found++
			var data map[string]interface{}
			Expect(json.Unmarshal(event.Data, &data)).To(Succeed())
			Expect(data).To(HaveKey("arguments_delta"))
			Expect(event.Phase).To(Equal(string(katypes.PhaseWorkflowDiscovery)))
		}
		Expect(found).To(Equal(2))
	})
})

type discoveryCatalogFetcher2439 struct{}

func (discoveryCatalogFetcher2439) FetchValidator(ctx context.Context) (*parser.Validator, error) {
	if state, ok := katypes.DiscoveredWorkflowStateFromContext(ctx); ok {
		state.Add("wf-2439")
	}
	return parser.NewValidator([]string{"wf-2439"}), nil
}
