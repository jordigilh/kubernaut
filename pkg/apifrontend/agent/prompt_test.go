package agent_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
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
		Expect(instruction).To(ContainSubstring("kubernaut_approve"))
		Expect(instruction).To(ContainSubstring("kubernaut_watch"))
		Expect(instruction).To(ContainSubstring("kubernaut_start_investigation"))
		Expect(instruction).To(ContainSubstring("kubernaut_select_workflow"))
		Expect(instruction).To(ContainSubstring("present_decision"))
		Expect(instruction).To(ContainSubstring("kubernaut_list_workflows"))
		Expect(instruction).To(ContainSubstring("kubernaut_get_audit_trail"))
	})

	It("UT-AF-1189-030: prompt includes kubernaut_stream_investigation tool", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_stream_investigation"))
		Expect(instruction).To(ContainSubstring("Stream live investigation events"))
	})

	It("UT-AF-1189-031: prompt includes kubernaut_discover_workflows tool", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_discover_workflows"))
		Expect(instruction).To(ContainSubstring("Discover available workflows"))
	})

	It("UT-AF-1189-032: prompt includes 4-phase interactive remediation journey", func() {
		Expect(instruction).To(ContainSubstring("4-Phase Interactive Remediation Journey"))
		Expect(instruction).To(ContainSubstring("Phase 1: Investigate"))
		Expect(instruction).To(ContainSubstring("Phase 2: Discover"))
		Expect(instruction).To(ContainSubstring("Phase 3: User selects"))
		Expect(instruction).To(ContainSubstring("Phase 4: Watch"))
	})

	It("UT-AF-1189-033: prompt includes autonomous mode rules for A2A delegation", func() {
		Expect(instruction).To(ContainSubstring("Autonomous mode"))
		Expect(instruction).To(ContainSubstring("fix"))
		Expect(instruction).To(ContainSubstring("remediate"))
		Expect(instruction).To(ContainSubstring("highest-confidence workflow"))
	})

	It("UT-AF-1189-034: prompt enforces kubernaut_watch after workflow selection", func() {
		Expect(instruction).To(ContainSubstring("MUST call kubernaut_watch"))
	})

	It("UT-AF-1189-035: prompt requires session_id/rr_id preservation across phases", func() {
		Expect(instruction).To(ContainSubstring("Preserve session_id and rr_id across all phases"))
	})
})
