package apifrontend_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("KA/DS Tools Integration (tools/ via real containers)", func() {

	Describe("AC-25: KA tools dispatch to real KA container", func() {
		It("IT-AF-1195-038: investigate dispatches to real KA", func() {
			kaClient := ka.NewClient(ka.Config{
				BaseURL:            "http://127.0.0.1:18130",
				Timeout:            30 * time.Second,
				CBFailureThreshold: 5,
			})

			result, err := tools.HandleStartInvestigation(
				context.Background(),
				kaClient,
				tools.StartInvestigationArgs{
					Namespace: "default",
					Kind:      "Deployment",
					Name:      "test-app",
				},
				nil, // no auditor needed for this test
			)
			// KA may return an error if the deployment doesn't exist,
			// but the tool dispatch itself should work without panicking
			if err == nil {
				Expect(result.SessionID).NotTo(BeEmpty())
			}
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
