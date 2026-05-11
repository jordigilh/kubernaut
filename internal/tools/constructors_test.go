package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("Tool Constructors", func() {
	type constructorEntry struct {
		name string
		fn   func() (interface{ Name() string }, error)
	}

	entries := []constructorEntry{
		{"kubernaut_list_remediations", func() (interface{ Name() string }, error) { return tools.NewListRemediationsTool(nil) }},
		{"kubernaut_get_remediation", func() (interface{ Name() string }, error) { return tools.NewGetRemediationTool(nil) }},
		{"kubernaut_submit_signal", func() (interface{ Name() string }, error) { return tools.NewSubmitSignalTool(nil, nil) }},
		{"kubernaut_approve", func() (interface{ Name() string }, error) { return tools.NewApproveTool(nil) }},
		{"kubernaut_cancel_remediation", func() (interface{ Name() string }, error) { return tools.NewCancelRemediationTool(nil) }},
		{"kubernaut_watch", func() (interface{ Name() string }, error) { return tools.NewWatchTool(nil) }},
		{"kubernaut_start_investigation", func() (interface{ Name() string }, error) { return tools.NewStartInvestigationTool(nil) }},
		{"kubernaut_poll_investigation", func() (interface{ Name() string }, error) { return tools.NewPollInvestigationTool(nil) }},
		{"kubernaut_select_workflow", func() (interface{ Name() string }, error) { return tools.NewSelectWorkflowTool(nil) }},
		{"present_decision", func() (interface{ Name() string }, error) { return tools.NewPresentDecisionTool() }},
		{"kubernaut_list_workflows", func() (interface{ Name() string }, error) { return tools.NewListWorkflowsTool(nil) }},
		{"kubernaut_get_remediation_history", func() (interface{ Name() string }, error) { return tools.NewGetRemediationHistoryTool(nil) }},
		{"kubernaut_get_effectiveness", func() (interface{ Name() string }, error) { return tools.NewGetEffectivenessTool(nil) }},
		{"kubernaut_get_audit_trail", func() (interface{ Name() string }, error) { return tools.NewGetAuditTrailTool(nil) }},
	}

	for _, e := range entries {
		e := e
		It("constructs "+e.name+" without error", func() {
			t, err := e.fn()
			Expect(err).NotTo(HaveOccurred())
			Expect(t.Name()).To(Equal(e.name))
		})
	}
})
