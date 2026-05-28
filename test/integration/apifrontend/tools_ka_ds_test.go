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
					RRID: "default/test-app",
				},
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-it-1195"))
			Expect(result.Status).To(Equal("autonomous_started"))
		})
	})

	Describe("AC-26: DS tools query real DS container", func() {
		It("IT-AF-1195-040: list_workflows queries real DS", func() {
			dsClient, err := newAuthenticatedDSClient()
			Expect(err).NotTo(HaveOccurred())

			result, err := tools.HandleListWorkflows(
				context.Background(),
				dsClient,
				tools.ListWorkflowsArgs{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Workflows).NotTo(BeNil())
		})
	})
})
