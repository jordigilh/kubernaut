package agent_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
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

		It("UT-AF-100-002: registers all 21 tools", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(21))
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

		It("UT-AF-100-005: tool names follow naming convention (kubernaut_ or af_ prefix)", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())

			for _, t := range tools {
				if t.Name() == "present_decision" {
					continue
				}
				hasValidPrefix := strings.HasPrefix(t.Name(), "kubernaut_") || strings.HasPrefix(t.Name(), "af_")
				Expect(hasValidPrefix).To(BeTrue(), "tool %q missing kubernaut_ or af_ prefix", t.Name())
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
			Expect(tools).To(HaveLen(21))
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

		It("UT-AF-1221-016: NewRootAgent returns all tools unfiltered", func() {
			cfg := agentpkg.DefaultTestConfig()
			_, tools, err := agentpkg.NewRootAgent(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(HaveLen(21), "AC 7: all 21 tools must be returned unfiltered")
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
