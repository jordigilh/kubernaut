package tools_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("SessionDependentTools (#1366)", func() {

	expectedSessionTools := []string{
		"kubernaut_investigate",
		"kubernaut_discover_workflows",
		"kubernaut_select_workflow",
		"kubernaut_present_decision",
		"kubernaut_message",
		"kubernaut_complete",
		"kubernaut_cancel",
		"kubernaut_status",
		"kubernaut_reconnect",
		"kubernaut_await_session",
	}

	expectedStatelessTools := []string{
		"kubernaut_list_remediations",
		"kubernaut_get_remediation",
		"kubernaut_list_approval_requests",
		"kubernaut_get_approval_request",
		"kubernaut_approve",
		"kubernaut_cancel_remediation",
		"kubernaut_watch",
		"kubernaut_list_workflows",
		"kubernaut_get_remediation_history",
		"kubernaut_get_effectiveness",
		"kubernaut_get_audit_trail",
	}

	It("UT-AF-1366-001: contains exactly 10 session-dependent tools", func() {
		Expect(tools.SessionDependentTools).To(HaveLen(10))
	})

	It("UT-AF-1366-002: all expected session-dependent tools are present", func() {
		for _, name := range expectedSessionTools {
			Expect(tools.SessionDependentTools).To(HaveKey(name),
				"expected session-dependent tool %q to be in SessionDependentTools", name)
		}
	})

	It("UT-AF-1366-003: stateless tools are NOT in SessionDependentTools", func() {
		for _, name := range expectedStatelessTools {
			Expect(tools.SessionDependentTools).NotTo(HaveKey(name),
				"stateless tool %q should NOT be in SessionDependentTools", name)
		}
	})
})
