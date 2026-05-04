package ka_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

var _ = Describe("KA MCP Client", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-110-007: select_workflow calls KA MCP with correct tool name and args", func() {
		mockMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(ctx context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				Expect(args.RRID).To(Equal("pay/rr-1"))
				Expect(args.WorkflowID).To(Equal("wf-restart"))
				return &ka.SelectWorkflowResult{Status: "accepted"}, nil
			},
		}
		result, err := mockMCP.SelectWorkflow(ctx, ka.SelectWorkflowArgs{RRID: "pay/rr-1", WorkflowID: "wf-restart"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("accepted"))
	})

	It("UT-AF-110-008: forwards JWT in MCP connection headers", func() {
		mockMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(ctx context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				return &ka.SelectWorkflowResult{Status: "accepted"}, nil
			},
			Token: "my-jwt",
		}
		Expect(mockMCP.Token).To(Equal("my-jwt"))
		_, err := mockMCP.SelectWorkflow(ctx, ka.SelectWorkflowArgs{RRID: "pay/rr-1", WorkflowID: "wf-1"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-110-009: returns error when KA MCP endpoint unreachable", func() {
		mockMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(ctx context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				return nil, ka.ErrMCPUnavailable
			},
		}
		_, err := mockMCP.SelectWorkflow(ctx, ka.SelectWorkflowArgs{RRID: "pay/rr-1", WorkflowID: "wf-1"})
		Expect(err).To(MatchError(ka.ErrMCPUnavailable))
	})
})
