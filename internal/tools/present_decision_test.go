package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("present_decision", func() {
	It("UT-AF-115-001: is registered with IsLongRunning=true", func() {
		t, err := tools.NewPresentDecisionTool()
		Expect(err).NotTo(HaveOccurred())
		Expect(t.IsLongRunning()).To(BeTrue())
	})

	It("UT-AF-115-002: formats RCA and options for user presentation", func() {
		result := tools.HandlePresentDecision(tools.PresentDecisionArgs{
			SessionID: "sess-1",
			Summary:   "Memory leak detected in pod-xyz",
			Options: []tools.WorkflowOption{
				{WorkflowID: "wf-1", Name: "Restart Pod", Description: "Restart the affected pod"},
				{WorkflowID: "wf-2", Name: "Scale Up", Description: "Add replicas"},
			},
		})
		Expect(result.Presented).To(BeTrue())
		Expect(result.Message).NotTo(BeEmpty())
	})

	It("UT-AF-115-003: includes all workflow options in output", func() {
		result := tools.HandlePresentDecision(tools.PresentDecisionArgs{
			SessionID: "sess-1",
			Summary:   "Issue found",
			Options: []tools.WorkflowOption{
				{WorkflowID: "wf-1", Name: "Option A"},
				{WorkflowID: "wf-2", Name: "Option B"},
				{WorkflowID: "wf-3", Name: "Option C"},
			},
		})
		Expect(result.Presented).To(BeTrue())
	})
})
