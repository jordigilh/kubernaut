package auth_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/agent"
)

var _ = Describe("FilterToolsByRole", func() {
	var allTools []string

	BeforeEach(func() {
		cfg := agent.DefaultTestConfig()
		_, tools, err := agent.NewRootAgent(cfg)
		Expect(err).NotTo(HaveOccurred())
		allTools = make([]string, 0, len(tools))
		for _, t := range tools {
			allTools = append(allTools, t.Name())
		}
	})

	It("UT-AF-130-001: SRE role gets all 14 tools", func() {
		cfg := agent.DefaultTestConfig()
		_, tools, _ := agent.NewRootAgent(cfg)
		filtered := agent.FilterToolsByRole("sre", tools)
		Expect(filtered).To(HaveLen(14))
	})

	It("UT-AF-130-002: CI/CD role gets submit_signal, list, watch, get only", func() {
		cfg := agent.DefaultTestConfig()
		_, tools, _ := agent.NewRootAgent(cfg)
		filtered := agent.FilterToolsByRole("cicd", tools)
		Expect(filtered).To(HaveLen(4))
		names := make([]string, 0, len(filtered))
		for _, t := range filtered {
			names = append(names, t.Name())
		}
		Expect(names).To(ContainElements(
			"kubernaut_submit_signal",
			"kubernaut_list_remediations",
			"kubernaut_get_remediation",
			"kubernaut_watch",
		))
	})

	It("UT-AF-130-003: L3 Audit role gets DS query tools only", func() {
		cfg := agent.DefaultTestConfig()
		_, tools, _ := agent.NewRootAgent(cfg)
		filtered := agent.FilterToolsByRole("l3-audit", tools)
		Expect(filtered).To(HaveLen(6))
		names := make([]string, 0, len(filtered))
		for _, t := range filtered {
			names = append(names, t.Name())
		}
		Expect(names).To(ContainElements(
			"kubernaut_list_workflows",
			"kubernaut_get_remediation_history",
			"kubernaut_get_effectiveness",
			"kubernaut_get_audit_trail",
		))
	})

	It("UT-AF-130-004: AI Orchestrator role gets investigation + CRD tools", func() {
		cfg := agent.DefaultTestConfig()
		_, tools, _ := agent.NewRootAgent(cfg)
		filtered := agent.FilterToolsByRole("ai-orchestrator", tools)
		Expect(filtered).To(HaveLen(10))
	})

	It("UT-AF-130-005: Observability Dashboard role gets list, get, watch, effectiveness, workflows", func() {
		cfg := agent.DefaultTestConfig()
		_, tools, _ := agent.NewRootAgent(cfg)
		filtered := agent.FilterToolsByRole("observability", tools)
		Expect(filtered).To(HaveLen(5))
	})

	It("UT-AF-130-006: unknown role gets empty tool list", func() {
		cfg := agent.DefaultTestConfig()
		_, tools, _ := agent.NewRootAgent(cfg)
		filtered := agent.FilterToolsByRole("unknown", tools)
		Expect(filtered).To(BeEmpty())
	})

	It("UT-AF-130-007: FilterToolsByRole returns new slice, not mutated original", func() {
		cfg := agent.DefaultTestConfig()
		_, tools, _ := agent.NewRootAgent(cfg)
		original := make([]string, len(tools))
		for i, t := range tools {
			original[i] = t.Name()
		}
		_ = agent.FilterToolsByRole("cicd", tools)
		after := make([]string, len(tools))
		for i, t := range tools {
			after[i] = t.Name()
		}
		Expect(after).To(Equal(original))
	})
})
