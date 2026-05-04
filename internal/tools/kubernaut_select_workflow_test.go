package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_select_workflow", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-114-001: calls KA MCP kubernaut_select_workflow with rr_id and workflow_id", func() {
		mockMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(ctx context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				Expect(args.RRID).To(Equal("pay/rr-1"))
				Expect(args.WorkflowID).To(Equal("wf-restart"))
				return &ka.SelectWorkflowResult{Status: "accepted", Message: "Workflow selected"}, nil
			},
		}
		result, err := tools.HandleSelectWorkflow(ctx, mockMCP, tools.SelectWorkflowArgs{
			RRID: "pay/rr-1", WorkflowID: "wf-restart",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("accepted"))
	})

	It("UT-AF-114-002: confirms execution started", func() {
		mockMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(ctx context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				return &ka.SelectWorkflowResult{Status: "accepted", Message: "Enrichment and selection started"}, nil
			},
		}
		result, err := tools.HandleSelectWorkflow(ctx, mockMCP, tools.SelectWorkflowArgs{
			RRID: "pay/rr-1", WorkflowID: "wf-restart",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Message).To(ContainSubstring("started"))
	})

	It("UT-AF-114-003: handles workflow not found", func() {
		mockMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(ctx context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				return nil, fmt.Errorf("workflow %q not found", args.WorkflowID)
			},
		}
		_, err := tools.HandleSelectWorkflow(ctx, mockMCP, tools.SelectWorkflowArgs{
			RRID: "pay/rr-1", WorkflowID: "nonexistent",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})
})
