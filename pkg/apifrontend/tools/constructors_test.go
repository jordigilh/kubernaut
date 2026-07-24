package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Tool Constructors", func() {
	type constructorEntry struct {
		name string
		fn   func() (interface{ Name() string }, error)
	}

	entries := []constructorEntry{
		{"kubernaut_list_remediations", func() (interface{ Name() string }, error) { return tools.NewListRemediationsTool(nil, "test-ns") }},
		{"kubernaut_get_remediation", func() (interface{ Name() string }, error) { return tools.NewGetRemediationTool(nil, "test-ns") }},
		{"kubernaut_approve", func() (interface{ Name() string }, error) {
			return tools.NewApproveTool(newTypedFakeClient(), "test-ns")
		}},
		{"kubernaut_cancel_remediation", func() (interface{ Name() string }, error) { return tools.NewCancelRemediationTool(nil, "test-ns") }},
		{"kubernaut_watch", func() (interface{ Name() string }, error) { return tools.NewWatchTool(nil, "test-ns") }},
		{"kubernaut_investigate", func() (interface{ Name() string }, error) {
			return tools.NewInvestigateMCPTool(&tools.InvestigateConfig{}, nil)
		}},
		{"kubernaut_select_workflow", func() (interface{ Name() string }, error) { return tools.NewSelectWorkflowTool(nil, nil) }},
		{"kubernaut_present_decision", func() (interface{ Name() string }, error) { return tools.NewPresentDecisionTool() }},
		// kubernaut_list_workflows: #1677 Phase 2g (DD-WORKFLOW-019) removed
		// NewListWorkflowsTool (DS-backed) as dead code. The KA-backed
		// replacement (HandleListWorkflowsKA) is registered directly via
		// registerTool in mcp_bridge.go, with no standalone tools.NewXxxTool
		// constructor -- see ka_list_workflows_test.go for its coverage.
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
