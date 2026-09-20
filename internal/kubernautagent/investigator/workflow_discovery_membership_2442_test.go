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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/tools/custom"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type discoveredWorkflowCatalog2442 struct{}

func (discoveredWorkflowCatalog2442) ListActions(context.Context, *models.WorkflowDiscoveryFilters, int, int) ([]models.ActionTypeEntry, int, error) {
	return nil, 0, nil
}

func (discoveredWorkflowCatalog2442) ListWorkflowsByActionType(context.Context, string, *models.WorkflowDiscoveryFilters, int, int) ([]models.RemediationWorkflow, int, error) {
	return []models.RemediationWorkflow{{WorkflowID: "workflow-presented"}}, 1, nil
}

func (discoveredWorkflowCatalog2442) GetWorkflowWithContextFilters(context.Context, string, *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	return &models.RemediationWorkflow{}, nil
}

var _ = Describe("Workflow selection discovery membership — #2442", func() {
	It("IT-KA-2442-002: rejects a catalog-valid undiscovered workflow and accepts a discovered correction (BR-KA-017-003/004, FedRAMP AC-6/SI-10, ASVS V2/V8)", func() {
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())

		client := &gateMockLLMClient{responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"pod memory limit exceeded","confidence":0.9}`}},
			{Message: llm.Message{Role: "assistant"}, ToolCalls: []llm.ToolCall{{
				ID: "list-workflows", Name: "list_workflows", Arguments: `{"action_type":"ScaleReplicas"}`,
			}}},
			gateWfToolResp(`{"workflow_id":"workflow-hidden","confidence":0.9}`),
			gateWfToolResp(`{"workflow_id":"workflow-presented","confidence":0.9}`),
		}}

		reg := registry.New()
		for _, tool := range custom.NewAllTools(discoveredWorkflowCatalog2442{}, nil, logr.Discard()) {
			reg.Register(tool)
		}

		validator := parser.NewValidator([]string{"workflow-presented", "workflow-hidden"})
		inv := investigator.New(investigator.Config{
			Client: client, Builder: builder, ResultParser: parser.NewResultParser(),
			Enricher:   enrichment.NewEnricher(&gateK8sClient{}, &gateDSClient{}, audit.NopAuditStore{}, logr.Discard()),
			AuditStore: audit.NopAuditStore{}, Logger: logr.Discard(), MaxTurns: 15,
			PhaseTools: investigator.DefaultPhaseToolMap(), Registry: reg,
			Pipeline: investigator.Pipeline{CatalogFetcher: &stubCatalogFetcher{validator: validator}},
		})

		result, err := inv.Investigate(context.Background(), katypes.SignalContext{
			Name: "OOMKilled", Namespace: "default", ResourceKind: "Pod", ResourceName: "api-server",
			Severity: "critical", Environment: "Production", Priority: "P0",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.WorkflowID).To(Equal("workflow-presented"))
		Expect(result.HumanReviewNeeded).To(BeFalse())
		Expect(client.calls).To(HaveLen(4), "RCA, discovery, invalid selection, and corrected selection")
	})
})
