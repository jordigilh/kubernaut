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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// paramCapturingDS captures the filters/args passed to each catalog method
// for assertions, while returning canned responses. Satisfies
// custom.WorkflowCatalog (#1677 Phase 2e: replaces the former
// ogen-client-shaped fake now that the discovery tools are catalog-backed,
// not DS-ogen-client-backed -- mirrors internal/kubernautagent/tools/custom's
// fakeWorkflowDS ported in Phase 2d).
type paramCapturingDS struct {
	listActionsFilters *models.WorkflowDiscoveryFilters
	actionsCalled      bool

	listWorkflowsActionType string
	listWorkflowsFilters    *models.WorkflowDiscoveryFilters
	workflowsCalled         bool
}

func (p *paramCapturingDS) ListActions(_ context.Context, filters *models.WorkflowDiscoveryFilters, _, _ int) ([]models.ActionTypeEntry, int, error) {
	p.listActionsFilters = filters
	p.actionsCalled = true
	return []models.ActionTypeEntry{
		{ActionType: "RestartPod", Description: models.ActionTypeDescription{What: "restart", WhenToUse: "crash"}, WorkflowCount: 1},
	}, 1, nil
}

func (p *paramCapturingDS) ListWorkflowsByActionType(_ context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, _, _ int) ([]models.RemediationWorkflow, int, error) {
	p.listWorkflowsActionType = actionType
	p.listWorkflowsFilters = filters
	p.workflowsCalled = true
	return []models.RemediationWorkflow{
		{WorkflowID: "wf-restart-v1", WorkflowName: "restart-v1", Name: "Restart Pod", Description: models.StructuredDescription{What: "restart", WhenToUse: "crash"}},
	}, 1, nil
}

func (p *paramCapturingDS) GetWorkflowWithContextFilters(_ context.Context, _ string, _ *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	return &models.RemediationWorkflow{}, nil
}

var _ = Describe("IT-KA-779: Signal context propagation through investigator to DS tool params", func() {

	var (
		invLogger  logr.Logger
		auditStore *capturingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = newCapturingAuditStore(suiteAuditStore)
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-779-001: Signal severity/environment/priority flow from Investigate() to list_available_actions DS params", func() {
		It("should forward staging/high/P1 signal context to DS, not hardcoded production/critical/P0", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS, nil, invLogger) {
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

			k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{}}
			enricher := enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:               "api-server",
				Namespace:          "staging-ns",
				Severity:           "high",
				Message:            "OOMKilled",
				ResourceKind:       "StatefulSet",
				ResourceAPIVersion: "apps/v1",
				ResourceName:       "api-server-0",
				Environment:        "staging",
				Priority:           "P1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.actionsCalled).To(BeTrue(),
				"list_available_actions must have been called during workflow selection")

			Expect(capturingDS.listActionsFilters.Severity).To(Equal("high"),
				"DS Severity should match signal, not hardcoded 'critical'")
			Expect(capturingDS.listActionsFilters.Component).To(Equal("apps/v1/StatefulSet"),
				"DS Component should be signal ComponentGVK (apiVersion/kind)")
			Expect(capturingDS.listActionsFilters.Environment).To(Equal("staging"),
				"DS Environment should match signal, not hardcoded 'production'")
			Expect(capturingDS.listActionsFilters.Priority).To(Equal("P1"),
				"DS Priority should match signal, not hardcoded 'P0'")
		})
	})

	Describe("IT-KA-779-002: Signal context flows to list_workflows DS params", func() {
		It("should forward signal fields when LLM calls list_workflows", func() {
			capturingDS := &paramCapturingDS{}
			reg := registry.New()
			for _, t := range custom.NewAllTools(capturingDS, nil, invLogger) {
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

			k8sClient := &k8sFixtureClient{ownerChain: []enrichment.OwnerChainEntry{}}
			enricher := enrichment.NewEnricher(k8sClient, suiteDSAdapter, auditStore, invLogger)

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools, Registry: reg,
			})

			_, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name:               "web-app",
				Namespace:          "dev-ns",
				Severity:           "warning",
				Message:            "CrashLoopBackOff",
				ResourceKind:       "Deployment",
				ResourceAPIVersion: "apps/v1",
				ResourceName:       "web-app",
				Environment:        "development",
				Priority:           "P2",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturingDS.workflowsCalled).To(BeTrue(),
				"list_workflows must have been called during workflow selection")

			Expect(capturingDS.listWorkflowsFilters.Severity).To(Equal("warning"),
				"DS Severity should match signal")
			Expect(capturingDS.listWorkflowsFilters.Component).To(Equal("apps/v1/Deployment"),
				"DS Component should be signal ComponentGVK (apiVersion/kind)")
			Expect(capturingDS.listWorkflowsFilters.Environment).To(Equal("development"),
				"DS Environment should match signal")
			Expect(capturingDS.listWorkflowsFilters.Priority).To(Equal("P2"),
				"DS Priority should match signal")
			Expect(capturingDS.listWorkflowsActionType).To(Equal("RestartPod"),
				"ActionType should come from LLM tool call args")
		})
	})
})

// filterEvents helper is defined in investigator_test.go and shared across this package.
// wfToolResp helper is defined in investigator_test.go and shared across this package.
