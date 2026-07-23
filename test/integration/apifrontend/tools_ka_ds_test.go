package apifrontend_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("KA/DS Tools Integration (tools/ via real containers)", func() {

	Describe("AC-25: KA tools dispatch to real KA container", func() {
		It("IT-AF-1195-038: investigate dispatches to real KA via MCP", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-it-1195",
						Status:    "autonomous_started",
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCP(
				context.Background(),
				mockMCP,
				nil, "",
				tools.InvestigateMCPArgs{
					RRID: "rr-test-app",
				},
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-it-1195"))
			Expect(result.Status).To(Equal("autonomous_started"))
		})
	})

	// AC-26 ("DS tools query real DS container", IT-AF-1195-040: list_workflows
	// queries real DS) removed -- #1677 Phase 2g (DD-WORKFLOW-019): DS's GET
	// /api/v1/workflows was retired along with tools.HandleListWorkflows.
	// AC-27 below is the successor: kubernaut_list_workflows now dispatches to
	// KubernautAgent's workflow catalog, not a real DS container, so its
	// "real backend" coverage lives in KA's own IT/E2E suites (see
	// test/integration/kubernautagent/workflowcatalog/ and
	// test/e2e/kubernautagent/three_step_discovery_test.go) rather than here.

	// #1677 Phase 2f (DD-WORKFLOW-019): kubernaut_list_workflows moved off DS
	// onto KA's workflow catalog (cache-backed). This proves the AF-side
	// handler wiring (HandleListWorkflowsKA -> ka.MCPClient.ListWorkflows)
	// independent of the MCP bridge dispatch already covered by
	// pkg/apifrontend/handler's UT-AF-B-011/IT-BRIDGE-004.
	Describe("AC-27: kubernaut_list_workflows dispatches to KA's catalog (#1677 Phase 2f)", func() {
		It("IT-AF-1677-001: HandleListWorkflowsKA narrows KA catalog results to WorkflowSummary", func() {
			mockMCP := &ka.MockMCPClient{
				ListWorkflowsFn: func(_ context.Context, args ka.ListWorkflowsArgs) (*ka.ListWorkflowsResult, error) {
					Expect(args.Kind).To(Equal("Deployment"))
					return &ka.ListWorkflowsResult{
						Workflows: []ka.WorkflowSummary{
							{ID: "wf-restart", Name: "Restart Pod", Description: "Restarts the pod", Kind: "Deployment"},
						},
						Count: 1,
					}, nil
				},
			}

			result, err := tools.HandleListWorkflowsKA(context.Background(), mockMCP, tools.ListWorkflowsArgs{Kind: "Deployment"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
			Expect(result.Workflows).To(ConsistOf(tools.WorkflowSummary{
				ID: "wf-restart", Name: "Restart Pod", Description: "Restarts the pod", Kind: "Deployment",
			}))
		})

		It("IT-AF-1677-002: HandleListWorkflowsKA returns a clear error when the KA MCP client is nil", func() {
			_, err := tools.HandleListWorkflowsKA(context.Background(), nil, tools.ListWorkflowsArgs{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not available"))
		})

		It("IT-AF-1677-003: HandleListWorkflowsKA propagates ka.ErrMCPUnavailable", func() {
			unreachableMCP := &ka.MockMCPClient{
				ListWorkflowsFn: func(_ context.Context, _ ka.ListWorkflowsArgs) (*ka.ListWorkflowsResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}
			_, err := tools.HandleListWorkflowsKA(context.Background(), unreachableMCP, tools.ListWorkflowsArgs{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unavailable"))
		})
	})
})
