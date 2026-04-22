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
	"log/slog"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// paramCapturingDS captures DS params for assertions while returning valid responses.
type paramCapturingDS struct {
	listActionsParams   ogenclient.ListAvailableActionsParams
	listWorkflowsParams ogenclient.ListWorkflowsByActionTypeParams
	actionsCalled       bool
	workflowsCalled     bool
}

func (p *paramCapturingDS) ListAvailableActions(_ context.Context, params ogenclient.ListAvailableActionsParams) (ogenclient.ListAvailableActionsRes, error) {
	p.listActionsParams = params
	p.actionsCalled = true
	return &ogenclient.ActionTypeListResponse{
		ActionTypes: []ogenclient.ActionTypeEntry{
			{ActionType: "RestartPod", Description: ogenclient.StructuredDescription{What: "restart", WhenToUse: "crash"}, WorkflowCount: 1},
		},
		Pagination: ogenclient.PaginationMetadata{TotalCount: 1, HasMore: false},
	}, nil
}

func (p *paramCapturingDS) ListWorkflowsByActionType(_ context.Context, params ogenclient.ListWorkflowsByActionTypeParams) (ogenclient.ListWorkflowsByActionTypeRes, error) {
	p.listWorkflowsParams = params
	p.workflowsCalled = true
	return &ogenclient.WorkflowDiscoveryResponse{
		ActionType: params.ActionType,
		Workflows: []ogenclient.WorkflowDiscoveryEntry{
			{WorkflowId: uuid.New(), WorkflowName: "restart-v1", Name: "Restart Pod", Description: ogenclient.StructuredDescription{What: "restart", WhenToUse: "crash"}},
		},
		Pagination: ogenclient.PaginationMetadata{TotalCount: 1, HasMore: false},
	}, nil
}

func (p *paramCapturingDS) GetWorkflowByID(_ context.Context, _ ogenclient.GetWorkflowByIDParams) (ogenclient.GetWorkflowByIDRes, error) {
	return &ogenclient.RemediationWorkflow{}, nil
}

var _ = Describe("IT-KA-779: Signal context propagation through investigator to DS tool params", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-779-001: Signal severity/environment/priority flow from Investigate() to list_available_actions DS params", func() {
		It("should forward staging/high/P1 signal context to DS, not hardcoded production/critical/P0", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

			// Phase 1 (RCA): LLM returns RCA summary directly
			// Phase 3 (Workflow): LLM calls list_available_actions, then submits result
			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOMKilled in staging"}`}},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "list_available_actions", Arguments: `{}`}},
					},
					wfToolResp(`{"workflow_id":"restart","confidence":0.9}`),
				},
			}

			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "api-server",
				Namespace:    "staging-ns",
				Severity:     "high",
				Message:      "OOMKilled",
				ResourceKind: "StatefulSet",
				ResourceName: "api-server-0",
				Environment:  "staging",
				Priority:     "P1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.actionsCalled).To(BeTrue(),
				"list_available_actions must have been called during workflow selection")

			Expect(string(capturingDS.listActionsParams.Severity)).To(Equal("high"),
				"DS Severity should match signal, not hardcoded 'critical'")
			Expect(capturingDS.listActionsParams.Component).To(Equal("statefulset"),
				"DS Component should be signal ResourceKind lowercased, not hardcoded 'deployment'")
			Expect(capturingDS.listActionsParams.Environment).To(Equal("staging"),
				"DS Environment should match signal, not hardcoded 'production'")
			Expect(string(capturingDS.listActionsParams.Priority)).To(Equal("P1"),
				"DS Priority should match signal, not hardcoded 'P0'")
		})
	})

	Describe("IT-KA-779-002: Signal context flows to list_workflows DS params", func() {
		It("should forward signal fields when LLM calls list_workflows", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS) {
				reg.Register(t)
			}

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"crash loop"}`}},
					{
						Message:   llm.Message{Role: "assistant", Content: ""},
						ToolCalls: []llm.ToolCall{{ID: "tc_1", Name: "list_workflows", Arguments: `{"action_type":"RestartPod"}`}},
					},
					wfToolResp(`{"workflow_id":"restart","confidence":0.85}`),
				},
			}

			k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
			dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			enricher := enrichment.NewEnricher(k8sClient, dsClient, auditStore, logger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: logger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:         "web-app",
				Namespace:    "dev-ns",
				Severity:     "medium",
				Message:      "CrashLoopBackOff",
				ResourceKind: "Deployment",
				ResourceName: "web-app",
				Environment:  "development",
				Priority:     "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.workflowsCalled).To(BeTrue(),
				"list_workflows must have been called during workflow selection")

			Expect(string(capturingDS.listWorkflowsParams.Severity)).To(Equal("medium"),
				"DS Severity should match signal")
			Expect(capturingDS.listWorkflowsParams.Component).To(Equal("deployment"),
				"DS Component should be signal ResourceKind lowercased")
			Expect(capturingDS.listWorkflowsParams.Environment).To(Equal("development"),
				"DS Environment should match signal")
			Expect(string(capturingDS.listWorkflowsParams.Priority)).To(Equal("P2"),
				"DS Priority should match signal")
			Expect(capturingDS.listWorkflowsParams.ActionType).To(Equal("RestartPod"),
				"ActionType should come from LLM tool call args")
		})
	})
})

// filterEvents helper is defined in investigator_test.go and shared across this package.
// wfToolResp helper is defined in investigator_test.go and shared across this package.
