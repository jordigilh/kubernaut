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
	"fmt"
	"time"

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

type extractionRetryClient2586 struct {
	responses []llm.ChatResponse
	requests  []llm.ChatRequest
}

func (c *extractionRetryClient2586) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.requests = append(c.requests, req)
	if len(c.requests) > len(c.responses) {
		return llm.ChatResponse{}, fmt.Errorf("unexpected llm call")
	}
	return c.responses[len(c.requests)-1], nil
}

func (c *extractionRetryClient2586) StreamChat(ctx context.Context, req llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return c.Chat(ctx, req)
}

func (*extractionRetryClient2586) Close() error { return nil }

var _ = Describe("MCP workflow discovery tool-call delta wiring — #2439", func() {
	DescribeTable("IT-KA-2586-035: interactive discover_workflows retries invalid extraction and preserves corrected presence (BR-AI-012, BR-INTERACTIVE-010, SI-10)", func(corrected string, expected *float64) {
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		client := &extractionRetryClient2586{responses: []llm.ChatResponse{
			{Message: llm.Message{Reasoning: &llm.ReasoningBlock{Text: "rejected reasoning", Signature: "replay-signature"}}, ToolCalls: []llm.ToolCall{{ID: "invalid", Name: investigator.SubmitResultToolName, Arguments: `{"rca_summary":"invalid","confidence":1.1}`}}},
			{Message: llm.Message{Reasoning: &llm.ReasoningBlock{Text: "accepted correction evidence", Signature: "provider-secret"}}, ToolCalls: []llm.ToolCall{{ID: "corrected", Name: investigator.SubmitResultToolName, Arguments: `{"rca_summary":"corrected"` + corrected + `}`}}},
			{ToolCalls: []llm.ToolCall{{ID: "workflow", Name: investigator.SubmitResultWithWorkflowToolName, Arguments: `{"workflow_id":"wf-2439","confidence":0.9,"remediation_target":{"kind":"Pod","name":"test-pod","namespace":"default"}}`}}},
		}}
		inv := investigator.New(investigator.Config{Client: client, Builder: builder, ResultParser: parser.NewResultParser(), AuditStore: audit.NopAuditStore{}, Logger: logr.Discard(), MaxTurns: 5, PhaseTools: investigator.DefaultPhaseToolMap(), Pipeline: investigator.Pipeline{CatalogFetcher: discoveryCatalogFetcher2439{}}})
		const rrID = "rr-interactive-2586"
		sess := &mcpinternal.InteractiveSession{SessionID: "session-2586", CorrelationID: rrID, ActingUser: mcpinternal.UserInfo{Username: "alice"}}
		sessions := &mockSessionManager{isActive: true, getDriverResult: sess}
		recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{{Role: "user", Content: "investigate the crash"}}}
		signal := katypes.SignalContext{Name: "OOMKilled", Namespace: "default", ResourceKind: "Pod", ResourceName: "test-pod", Severity: "critical", RemediationID: rrID}
		tool := mcptools.NewInvestigateTool(sessions, adapters.NewInvestigatorRunnerAdapter(inv), recon, &discoveryAutoMgr2439{}, mcptools.WithWorkflowCatalog(workflowCatalog2439{}), mcptools.WithSignalContextResolver(signalResolver2439{signal: signal}))
		output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{RRID: rrID, Action: mcptools.ActionDiscoverWorkflows}, mcpinternal.UserInfo{Username: "alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(output.Status).To(Equal("workflows_discovered"))
		Expect(sess.DiscoveryResult).NotTo(BeNil())
		Expect(sess.DiscoveryResult.Recommended).NotTo(BeNil())
		Expect(sess.DiscoveryResult.Recommended.WorkflowID).To(Equal("wf-2439"))
		Expect(sess.DiscoveryResult.FullResult.RCAConfidence).To(Equal(expected))
		if expected == nil {
			Expect(sess.RCAResult.RCAConfidence).To(BeNil())
		} else {
			Expect(sess.RCAResult.RCAConfidence).To(HaveValue(Equal(*expected)))
		}
		Expect(sess.RCAResult.RCASummary).To(Equal("corrected"))
		Expect(sess.RCAResult.Reasoning).NotTo(BeNil())
		Expect(sess.RCAResult.Reasoning.Text).To(Equal("accepted correction evidence"))
		reasoningJSON, err := json.Marshal(sess.RCAResult.Reasoning)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(reasoningJSON)).NotTo(ContainSubstring("provider-secret"))
		Expect(client.requests).To(HaveLen(3))
		Expect(sess.RCAResult.TotalLLMTurns).To(Equal(3), "extraction, correction, and discovery are counted once each")
		Expect(client.requests[1].Messages).To(ContainElement(HaveField("Content", ContainSubstring("between 0 and 1"))))
		Expect(client.requests[1].Messages).To(ContainElement(And(HaveField("Role", "assistant"), HaveField("Reasoning", HaveValue(HaveField("Signature", "replay-signature"))))))
		Expect(client.requests[1].Messages).To(ContainElement(And(HaveField("Role", "assistant"), HaveField("ToolCalls", ContainElement(And(HaveField("Name", Equal(investigator.SubmitResultToolName)), HaveField("Arguments", ContainSubstring(`"confidence":1.1`))))))))
	}, Entry("zero", `,"confidence":0`, func() *float64 { v := 0.0; return &v }()), Entry("missing", "", (*float64)(nil)), Entry("nonzero", `,"confidence":0.65`, func() *float64 { v := 0.65; return &v }()))

	It("IT-KA-2586-043: re-extracts corrected RCA after takeover leaves a cancelled partial result", func() {
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		client := &extractionRetryClient2586{responses: []llm.ChatResponse{
			{Message: llm.Message{Reasoning: &llm.ReasoningBlock{Text: "rejected reasoning", Signature: "replay-signature"}}, ToolCalls: []llm.ToolCall{{ID: "invalid", Name: investigator.SubmitResultToolName, Arguments: `{"rca_summary":"invalid","confidence":1.1` + `}`}}},
			{Message: llm.Message{Reasoning: &llm.ReasoningBlock{Text: "accepted correction evidence", Signature: "provider-secret"}}, ToolCalls: []llm.ToolCall{{ID: "corrected", Name: investigator.SubmitResultToolName, Arguments: `{"rca_summary":"corrected","confidence":0.65}`}}},
			{ToolCalls: []llm.ToolCall{{ID: "workflow", Name: investigator.SubmitResultWithWorkflowToolName, Arguments: `{"workflow_id":"wf-2439","confidence":0.9,"remediation_target":{"kind":"Pod","name":"test-pod","namespace":"default"}}`}}},
		}}
		inv := investigator.New(investigator.Config{Client: client, Builder: builder, ResultParser: parser.NewResultParser(), AuditStore: audit.NopAuditStore{}, Logger: logr.Discard(), MaxTurns: 5, PhaseTools: investigator.DefaultPhaseToolMap(), Pipeline: investigator.Pipeline{CatalogFetcher: discoveryCatalogFetcher2439{}}})

		const rrID = "rr-interactive-partial-2586"
		store := session.NewStore(1 * time.Hour)
		autoMgr := session.NewManager(store, logr.Discard(), nil, nil)
		storedSessionID, err := store.Create()
		Expect(err).NotTo(HaveOccurred())
		store.SetMetadata(storedSessionID, map[string]string{"remediation_id": rrID})
		partial := &katypes.InvestigationResult{Cancelled: true, CancelledPhase: string(katypes.PhaseRCA)}
		Expect(store.Update(storedSessionID, session.StatusUserDriving, partial, nil)).To(Succeed())

		storedResult, found := autoMgr.GetLatestRCAResultByRemediationID(rrID)
		Expect(found).To(BeFalse(), "cancelled partial RCA must not win discovery eligibility")
		Expect(storedResult).To(BeNil())

		sess := &mcpinternal.InteractiveSession{SessionID: "session-partial-2586", CorrelationID: rrID, ActingUser: mcpinternal.UserInfo{Username: "alice"}}
		sessions := &mockSessionManager{isActive: true, getDriverResult: sess}
		recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{{Role: "user", Content: "investigate the crash"}}}
		signal := katypes.SignalContext{Name: "OOMKilled", Namespace: "default", ResourceKind: "Pod", ResourceName: "test-pod", Severity: "critical", RemediationID: rrID}
		tool := mcptools.NewInvestigateTool(sessions, adapters.NewInvestigatorRunnerAdapter(inv), recon, autoMgr, mcptools.WithWorkflowCatalog(workflowCatalog2439{}), mcptools.WithSignalContextResolver(signalResolver2439{signal: signal}))

		output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{RRID: rrID, Action: mcptools.ActionDiscoverWorkflows}, mcpinternal.UserInfo{Username: "alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(output.Status).To(Equal("workflows_discovered"))
		Expect(sess.RCAResult).NotTo(BeNil())
		Expect(sess.RCAResult.RCASummary).To(Equal("corrected"))
		Expect(sess.RCAResult.RCAConfidence).To(HaveValue(Equal(0.65)))
		Expect(sess.RCAResult.Reasoning).NotTo(BeNil())
		Expect(sess.RCAResult.Reasoning.Text).To(Equal("accepted correction evidence"))
		Expect(client.requests).To(HaveLen(3), "RCA extraction, correction, and workflow discovery must all run")
	})

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
	v := parser.NewValidator([]string{"wf-2439"})
	v.SetWorkflowMeta("wf-2439", parser.WorkflowMeta{WorkflowName: "restart-pod"})
	return v, nil
}
