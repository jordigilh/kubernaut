package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("#1351 AF Input Validation", func() {

	Describe("UT-AF-1351-030: HandleDiscoverWorkflows validates rr_id format (AF-MED-2)", func() {
		It("should reject rr_id with invalid characters", func() {
			mockMCP := &ka.MockMCPClient{}
			_, err := tools.HandleDiscoverWorkflows(context.Background(), mockMCP, tools.DiscoverWorkflowsArgs{
				RRID: "../../etc/passwd",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid rr_id"))
		})
	})

	Describe("UT-AF-1351-031: HandleSelectWorkflow validates rr_id format (AF-MED-2)", func() {
		It("should reject rr_id with invalid characters", func() {
			mockMCP := &ka.MockMCPClient{}
			_, err := tools.HandleSelectWorkflow(context.Background(), mockMCP, tools.SelectWorkflowArgs{
				RRID:       "drop table; --",
				WorkflowID: "wf-1",
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid rr_id"))
		})
	})

	Describe("UT-AF-1351-032: HandleInvestigateMCP validates rr_id format (AF-MED-2)", func() {
		It("should reject rr_id with invalid characters", func() {
			mockMCP := &ka.MockMCPClient{}
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "ns",
				tools.InvestigateMCPArgs{RRID: "../../etc/passwd"},
				nil, nil, nil, false, nil, "", nil, nil,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid rr_id"))
		})

		It("should accept valid namespace/name format", func() {
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(ctx context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{SessionID: "sess-1", Status: "investigating"}, nil
				},
			}
			// Valid rr_id should pass validation (may fail later for other reasons)
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "default",
				tools.InvestigateMCPArgs{RRID: "prod/my-rr-001"},
				nil, nil, nil, false, nil, "user1", nil, nil,
			)
			// Should not fail with validation error
			if err != nil {
				Expect(err.Error()).NotTo(ContainSubstring("invalid rr_id"))
			}
		})
	})
})
