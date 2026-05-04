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
		{"kubernaut_list_remediations", func() (interface{ Name() string }, error) { return tools.NewListRemediationsTool() }},
		{"kubernaut_get_remediation", func() (interface{ Name() string }, error) { return tools.NewGetRemediationTool() }},
		{"kubernaut_submit_signal", func() (interface{ Name() string }, error) { return tools.NewSubmitSignalTool() }},
		{"kubernaut_approve", func() (interface{ Name() string }, error) { return tools.NewApproveTool() }},
		{"kubernaut_cancel_remediation", func() (interface{ Name() string }, error) { return tools.NewCancelRemediationTool() }},
		{"kubernaut_watch", func() (interface{ Name() string }, error) { return tools.NewWatchTool() }},
		{"kubernaut_start_investigation", func() (interface{ Name() string }, error) { return tools.NewStartInvestigationTool() }},
		{"kubernaut_poll_investigation", func() (interface{ Name() string }, error) { return tools.NewPollInvestigationTool() }},
		{"kubernaut_select_workflow", func() (interface{ Name() string }, error) { return tools.NewSelectWorkflowTool() }},
		{"present_decision", func() (interface{ Name() string }, error) { return tools.NewPresentDecisionTool() }},
		{"kubernaut_list_workflows", func() (interface{ Name() string }, error) { return tools.NewListWorkflowsTool() }},
		{"kubernaut_get_remediation_history", func() (interface{ Name() string }, error) { return tools.NewGetRemediationHistoryTool() }},
		{"kubernaut_get_effectiveness", func() (interface{ Name() string }, error) { return tools.NewGetEffectivenessTool() }},
		{"kubernaut_get_audit_trail", func() (interface{ Name() string }, error) { return tools.NewGetAuditTrailTool() }},
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
