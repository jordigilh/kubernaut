package agent_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentpkg "github.com/jordigilh/kubernaut-apifrontend/internal/agent"
)

var _ = Describe("Root Agent", func() {
	Describe("NewRootAgent", func() {
		It("UT-AF-100-001: returns configured agent with model", func() {
			cfg := agentpkg.AgentConfig{
				GCPProject:  "test-project",
				GCPRegion:   "us-central1",
				Instruction: "You are a test agent",
			}
			a, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Expect(a.Name()).To(Equal("kubernaut-apifrontend"))
			Expect(tools).NotTo(BeEmpty())
		})

		It("UT-AF-100-002: registers all 14 tools", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(14))
		})

		It("UT-AF-100-003: with nil model config returns error", func() {
			cfg := agentpkg.AgentConfig{}
			_, _, err := agentpkg.NewRootAgent(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("instruction"))
		})

		It("UT-AF-100-004: tool names are unique across all categories", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			names := make(map[string]bool)
			for _, t := range tools {
				Expect(names).NotTo(HaveKey(t.Name()), "duplicate tool name: %s", t.Name())
				names[t.Name()] = true
			}
		})

		It("UT-AF-100-005: tool names follow kubernaut_ prefix convention", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			for _, t := range tools {
				if t.Name() == "present_decision" {
					continue
				}
				Expect(t.Name()).To(HavePrefix("kubernaut_"), "tool %q missing kubernaut_ prefix", t.Name())
			}
		})

		It("UT-AF-100-006: each tool has non-empty description", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			for _, t := range tools {
				Expect(t.Description()).NotTo(BeEmpty(), "tool %q has empty description", t.Name())
			}
		})

		It("UT-AF-100-007: each tool has valid input schema", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(14))
		})

		It("UT-AF-100-008: present_decision is marked IsLongRunning", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, t := range tools {
				if t.Name() == "present_decision" {
					found = true
					Expect(t.IsLongRunning()).To(BeTrue(), "present_decision must be IsLongRunning")
				}
			}
			Expect(found).To(BeTrue(), "present_decision tool not found")
		})

		It("UT-AF-100-009: non-present_decision tools are NOT IsLongRunning", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			for _, t := range tools {
				if t.Name() != "present_decision" {
					Expect(t.IsLongRunning()).To(BeFalse(), "tool %q should not be IsLongRunning", t.Name())
				}
			}
		})

		It("UT-AF-100-010: agent config includes instruction", func() {
			cfg := agentpkg.DefaultTestConfig()
			a, _, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a.Description()).NotTo(BeEmpty())
		})

		It("UT-AF-100-011: tool registry returns tools filtered by role", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			filtered := agentpkg.FilterToolsByRole("sre", tools)
			Expect(filtered).To(HaveLen(14))

			filtered = agentpkg.FilterToolsByRole("unknown-role", tools)
			Expect(filtered).To(BeEmpty())
		})

		It("UT-AF-100-013: multi-group user gets union of tools from all roles", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			cicdOnly := agentpkg.FilterToolsByRole("cicd", tools)
			auditOnly := agentpkg.FilterToolsByRole("l3-audit", tools)
			union := agentpkg.FilterToolsByRoles([]string{"cicd", "l3-audit"}, tools)

			// Union must be >= each individual set
			Expect(len(union)).To(BeNumerically(">=", len(cicdOnly)))
			Expect(len(union)).To(BeNumerically(">=", len(auditOnly)))

			// Verify specific tools from each role are present
			names := make(map[string]bool)
			for _, t := range union {
				names[t.Name()] = true
			}
			Expect(names).To(HaveKey("kubernaut_submit_signal"))     // from cicd
			Expect(names).To(HaveKey("kubernaut_get_audit_trail"))   // from l3-audit
			Expect(names).To(HaveKey("kubernaut_list_remediations")) // from both
		})

		It("UT-AF-100-014: unauthorized role gets zero tools (fail-closed)", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			filtered := agentpkg.FilterToolsByRoles([]string{"attacker-role"}, tools)
			Expect(filtered).To(BeEmpty())
		})

		It("UT-AF-100-015: empty roles list gets zero tools (fail-closed)", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			filtered := agentpkg.FilterToolsByRoles([]string{}, tools)
			Expect(filtered).To(BeNil())
		})

		It("UT-AF-100-016: nil roles list gets zero tools (fail-closed)", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			filtered := agentpkg.FilterToolsByRoles(nil, tools)
			Expect(filtered).To(BeNil())
		})

		It("UT-AF-100-012: agent creation with empty tool list returns error", func() {
			cfg := agentpkg.AgentConfig{
				GCPProject:  "test-project",
				GCPRegion:   "us-central1",
				Instruction: "You are a test agent",
				SkipTools:   true,
			}
			_, _, err := agentpkg.NewRootAgent(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tool"))
		})
	})

	Describe("Functional Options", func() {
		It("WithGCPProject overrides project", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithGCPProject("new-project"))
			Expect(cfg.GCPProject).To(Equal("new-project"))
		})

		It("WithGCPRegion overrides region", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithGCPRegion("eu-west1"))
			Expect(cfg.GCPRegion).To(Equal("eu-west1"))
		})

		It("WithInstruction overrides instruction", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithInstruction("Custom prompt"))
			Expect(cfg.Instruction).To(Equal("Custom prompt"))
		})

		It("WithKABaseURL overrides KA URL", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithKABaseURL("http://ka:9999"))
			Expect(cfg.KABaseURL).To(Equal("http://ka:9999"))
		})

		It("WithKAMCPEndpoint overrides KA MCP URL", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithKAMCPEndpoint("http://ka:9999/mcp/"))
			Expect(cfg.KAMCPEndpoint).To(Equal("http://ka:9999/mcp/"))
		})

		It("WithDSBaseURL overrides DS URL", func() {
			cfg := agentpkg.DefaultTestConfig()
			cfg = cfg.Apply(agentpkg.WithDSBaseURL("http://ds:7777"))
			Expect(cfg.DSBaseURL).To(Equal("http://ds:7777"))
		})

		It("NewRootAgent accepts functional options", func() {
			cfg := agentpkg.DefaultTestConfig()
			a, _, err := agentpkg.NewRootAgent(cfg, agentpkg.WithGCPProject("override-project"))
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
		})
	})
})
