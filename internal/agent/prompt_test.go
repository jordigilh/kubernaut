package agent_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentpkg "github.com/jordigilh/kubernaut-apifrontend/internal/agent"
)

var _ = Describe("System Prompt", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-131-001: prompt contains no-internals constraint", func() {
		Expect(instruction).To(ContainSubstring("Never reference internal system names"))
	})

	It("UT-AF-131-002: prompt contains polling re-call instruction", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_poll_investigation"))
		Expect(instruction).To(ContainSubstring("MUST call"))
	})

	It("UT-AF-131-003: prompt contains present_decision handoff instruction", func() {
		Expect(instruction).To(ContainSubstring("present_decision"))
		Expect(instruction).To(ContainSubstring("MUST call present_decision"))
	})

	It("UT-AF-131-004: prompt does not contain internal system names outside the constraint rule", func() {
		lines := strings.Split(strings.ToLower(instruction), "\n")
		for _, line := range lines {
			if strings.Contains(line, "never reference internal") {
				continue
			}
			for _, forbidden := range []string{"remediationrequest", "aianalysis", "signalprocessing", "etcd"} {
				Expect(line).NotTo(ContainSubstring(forbidden),
					"prompt line %q should not reference internal name %q", line, forbidden)
			}
		}
	})

	It("UT-AF-131-005: prompt includes tool inventory summary", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_list_remediations"))
		Expect(instruction).To(ContainSubstring("kubernaut_get_remediation"))
		Expect(instruction).To(ContainSubstring("kubernaut_submit_signal"))
		Expect(instruction).To(ContainSubstring("kubernaut_approve"))
		Expect(instruction).To(ContainSubstring("kubernaut_watch"))
		Expect(instruction).To(ContainSubstring("kubernaut_start_investigation"))
		Expect(instruction).To(ContainSubstring("kubernaut_select_workflow"))
		Expect(instruction).To(ContainSubstring("present_decision"))
		Expect(instruction).To(ContainSubstring("kubernaut_list_workflows"))
		Expect(instruction).To(ContainSubstring("kubernaut_get_audit_trail"))
	})
})
