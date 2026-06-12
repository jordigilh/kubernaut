package agent_test

import (
	"context"
	"os"
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

	It("UT-AF-131-002: prompt contains investigate single-call instruction", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_investigate"))
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
		Expect(instruction).To(ContainSubstring("kubernaut_investigate"))
		Expect(instruction).To(ContainSubstring("kubernaut_select_workflow"))
		Expect(instruction).To(ContainSubstring("present_decision"))
		Expect(instruction).To(ContainSubstring("kubernaut_get_audit_trail"))
	})

	It("UT-AF-1189-030: prompt includes kubernaut_investigate tool", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_investigate"))
		Expect(instruction).To(ContainSubstring("streams live events"))
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
		Expect(instruction).To(ContainSubstring("call kubernaut_watch"))
	})

	It("UT-AF-1189-035: prompt requires session_id/rr_id preservation across phases", func() {
		Expect(instruction).To(ContainSubstring("Preserve session_id and rr_id across all phases"))
	})

	Describe("BuildInstruction (#1275)", func() {
		It("UT-AF-1275-010: output contains core embedded prompt (SC-7 immutability)", func() {
			result := agentpkg.BuildInstruction("kubernaut-system")
			Expect(result).To(ContainSubstring("You are the Kubernaut API Frontend agent"))
			Expect(result).To(ContainSubstring("Security Boundaries"))
		})

		It("UT-AF-1275-011: output contains deployment namespace (CM-6)", func() {
			result := agentpkg.BuildInstruction("kubernaut-system")
			Expect(result).To(ContainSubstring("kubernaut-system"))
			Expect(result).To(ContainSubstring("Deployment Context"))
		})

		It("UT-AF-1275-012: empty namespace falls back to default (SI-10)", func() {
			result := agentpkg.BuildInstruction("")
			Expect(result).To(ContainSubstring("default"))
			Expect(result).NotTo(ContainSubstring("``"))
		})

		It("UT-AF-1275-013: output contains kubernaut.ai CRD types (CM-6)", func() {
			result := agentpkg.BuildInstruction("kubernaut-system")
			Expect(result).To(ContainSubstring("RemediationRequest"))
			Expect(result).To(ContainSubstring("InvestigationSession"))
			Expect(result).To(ContainSubstring("WorkflowExecution"))
		})

		It("UT-AF-1275-014: intent group 'investigate' contains expected tools", func() {
			result := agentpkg.BuildInstruction("ns")
			Expect(result).To(ContainSubstring("kubernaut_investigate"))
		})

		It("UT-AF-1275-015: intent group 'observe' contains kubectl tools", func() {
			result := agentpkg.BuildInstruction("ns")
			Expect(result).To(ContainSubstring("kubectl_get"))
			Expect(result).To(ContainSubstring("kubectl_list"))
		})

		It("UT-AF-1275-016: intent group 'fix' references 4-phase journey", func() {
			result := agentpkg.BuildInstruction("ns")
			Expect(result).To(ContainSubstring("kubernaut_discover_workflows"))
			Expect(result).To(ContainSubstring("kubernaut_select_workflow"))
			Expect(result).To(ContainSubstring("kubernaut_watch"))
		})

		It("UT-AF-1275-017: intent group 'approve' contains approval tools", func() {
			result := agentpkg.BuildInstruction("ns")
			Expect(result).To(ContainSubstring("kubernaut_approve"))
			Expect(result).To(ContainSubstring("kubernaut_list_approval_requests"))
		})

		It("UT-AF-1275-018: intent group 'audit' contains history tools", func() {
			result := agentpkg.BuildInstruction("ns")
			Expect(result).To(ContainSubstring("kubernaut_get_audit_trail"))
			Expect(result).To(ContainSubstring("kubernaut_get_remediation_history"))
		})

		It("UT-AF-1275-019: intent group 'interactive' contains session tools", func() {
			result := agentpkg.BuildInstruction("ns")
			Expect(result).To(ContainSubstring("kubernaut_investigate"))
			Expect(result).To(ContainSubstring("kubernaut_reconnect"))
		})
	})

	Describe("ResolveNamespace (#1282)", func() {
		It("UT-AF-1282-NS-001: reads namespace from downward API file", func() {
			dir := GinkgoT().TempDir()
			nsFile := dir + "/namespace"
			Expect(os.WriteFile(nsFile, []byte("kubernaut-system"), 0o644)).To(Succeed())

			ns := agentpkg.ResolveNamespace("", nsFile)
			Expect(ns).To(Equal("kubernaut-system"))
		})

		It("UT-AF-1282-NS-002: config override takes precedence over downward API", func() {
			dir := GinkgoT().TempDir()
			nsFile := dir + "/namespace"
			Expect(os.WriteFile(nsFile, []byte("from-downward-api"), 0o644)).To(Succeed())

			ns := agentpkg.ResolveNamespace("custom-ns", nsFile)
			Expect(ns).To(Equal("custom-ns"))
		})

		It("UT-AF-1282-NS-003: falls back to default when both sources absent", func() {
			ns := agentpkg.ResolveNamespace("", "/nonexistent/path/namespace")
			Expect(ns).To(Equal("default"))
		})

		It("UT-AF-1282-NS-004: trims whitespace and newlines from downward API file", func() {
			dir := GinkgoT().TempDir()
			nsFile := dir + "/namespace"
			Expect(os.WriteFile(nsFile, []byte("  kubernaut-system\n"), 0o644)).To(Succeed())

			ns := agentpkg.ResolveNamespace("", nsFile)
			Expect(ns).To(Equal("kubernaut-system"))
		})

		It("UT-AF-1282-NS-005: empty config override falls through to downward API", func() {
			dir := GinkgoT().TempDir()
			nsFile := dir + "/namespace"
			Expect(os.WriteFile(nsFile, []byte("from-api"), 0o644)).To(Succeed())

			ns := agentpkg.ResolveNamespace("", nsFile)
			Expect(ns).To(Equal("from-api"))
		})
	})

	Describe("Prompt hardening (#1282 F-PROMPT)", func() {
		It("UT-AF-1282-PROMPT-001: prompt mandates kubernaut MCP tools for investigation", func() {
			result := agentpkg.BuildInstruction("kubernaut-system")
			Expect(result).To(ContainSubstring("kubernaut MCP tools"))
			Expect(result).To(ContainSubstring("NEVER use kubectl"))
		})

		It("UT-AF-1282-PROMPT-002: prompt documents all AF auto-resolved fields", func() {
			result := agentpkg.BuildInstruction("kubernaut-system")
			Expect(result).To(ContainSubstring("provide: api_version, namespace, kind, name, description"))
			Expect(result).To(ContainSubstring("workload namespace where the target resource lives"))
			Expect(result).To(ContainSubstring("severity: via the Prometheus severity triage pipeline"))
			Expect(result).To(ContainSubstring("signalName: from AlertManager alerts"))
			Expect(result).To(ContainSubstring("signalSource: hardcoded to a2a-agent"))
		})
	})

	Describe("InstructionProvider (#1276)", func() {
		It("UT-AF-1276-001: preserves core prompt immutability (SC-7)", func() {
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("You are the Kubernaut API Frontend agent"))
			Expect(result).To(ContainSubstring("Security Boundaries"))
		})

		It("UT-AF-1276-008: nil identity returns base instruction only (SC-7)", func() {
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Your Role Context"))
		})

		It("UT-AF-1276-009: empty groups returns base instruction only", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "alice", []string{})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Your Role Context"))
		})

		It("UT-AF-1276-002: SRE group adds full-access guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "alice", []string{"sre"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Your Role Context"))
			Expect(result).To(ContainSubstring("full operational access"))
		})

		It("UT-AF-1276-003: viewer group adds read-only guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "bob", []string{"observability"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("read-only"))
		})

		It("UT-AF-1276-004: approver group adds approval guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "carol", []string{"remediation-approver"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("approval"))
		})

		It("UT-AF-1276-005: CICD group adds automation guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "bot", []string{"cicd"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("automation"))
		})

		It("UT-AF-1276-006: audit group adds compliance guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "auditor", []string{"l3-audit"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("compliance"))
		})

		It("UT-AF-1276-007: multi-role user gets additive guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "multi", []string{"sre", "remediation-approver"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("full operational access"))
			Expect(result).To(ContainSubstring("approval"))
		})

		It("UT-AF-1276-010: unknown groups produce no extra guidance (SC-7)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "unknown", []string{"custom-team", "random"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Your Role Context"))
		})

		It("UT-AF-1276-011: raw group names not leaked into prompt (SC-7)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "alice", []string{"sre", "l3-audit"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system")
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("\"sre\""))
			Expect(result).NotTo(ContainSubstring("\"l3-audit\""))
		})
	})
})

var _ = Describe("Prompt — Intent-Based Tool Redesign (#1332)", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-1332-035: prompt contains kubernaut_remediate", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_remediate"))
	})

	It("UT-AF-1332-036: prompt does NOT contain deprecated af_ tool names", func() {
		Expect(instruction).NotTo(ContainSubstring("af_create_rr"))
		Expect(instruction).NotTo(ContainSubstring("af_check_existing_rr"))
	})

	It("UT-AF-1332-037: autonomous mode keywords map to kubernaut_remediate", func() {
		Expect(instruction).To(ContainSubstring("kubernaut_remediate"))
		Expect(instruction).To(SatisfyAny(
			ContainSubstring("fix"),
			ContainSubstring("remediate"),
		))
	})

	It("UT-AF-1332-038: BuildInstruction references kubernaut_remediate without deprecated af_ names", func() {
		built := agentpkg.BuildInstruction("kubernaut-system")
		Expect(built).To(ContainSubstring("kubernaut_remediate"))
		Expect(built).NotTo(ContainSubstring("af_create_rr"))
		Expect(built).NotTo(ContainSubstring("af_check_existing_rr"))
	})

	It("UT-AF-1332-039: prompt contains kubectl bypass prevention rule", func() {
		Expect(instruction).To(ContainSubstring("NEVER use kubectl tools to perform root-cause analysis"))
	})

	It("UT-AF-1332-040: prompt contains decision algorithm", func() {
		Expect(instruction).To(ContainSubstring("Does the user just want it fixed"))
	})

	It("UT-AF-1332-041: prompt contains WHAT/WHY boundary for observation mode", func() {
		Expect(instruction).To(ContainSubstring("kubectl queries answer WHAT"))
	})
})

// =============================================================================
// Issue #1407: Progressive Flow — auto-proceed from investigation to discovery
// =============================================================================

var _ = Describe("Prompt — Progressive Flow (#1407)", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-1407-010: SI-4 prompt does NOT contain unconditional MUST STOP after investigation", func() {
		Expect(instruction).NotTo(ContainSubstring(
			"you MUST STOP and ask the user what they want to do next"),
			"SI-4: unconditional stop blocks progressive flow and audit trail continuity")
	})

	It("UT-AF-1407-011: SI-4 prompt contains auto-proceed directive from investigation to discovery", func() {
		Expect(instruction).To(ContainSubstring("automatically proceed"),
			"SI-4: progressive flow must auto-proceed to workflow discovery for audit completeness")
	})

	It("UT-AF-1407-012: AU-3 prompt preserves investigate-only exception for explicit user requests", func() {
		Expect(instruction).To(SatisfyAny(
			ContainSubstring("just investigate"),
			ContainSubstring("investigate only"),
			ContainSubstring("only investigate"),
		), "AU-3: user must be able to request investigation-only mode for audit separation")
	})

	It("UT-AF-1407-013: SI-4 prompt retains present_decision as final structured artifact", func() {
		Expect(instruction).To(ContainSubstring("present_decision"),
			"SI-4: present_decision must remain the structured decision artifact for audit trail")
	})
})

// =============================================================================
// Issue #1408: Structured investigation_summary — prompt directives
// =============================================================================

var _ = Describe("Prompt — Structured Artifact Contract (#1408)", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-1408-020: SI-4 — prompt prohibits free-text narration after present_decision", func() {
		Expect(instruction).To(ContainSubstring("NEVER narrate"),
			"SI-4: free-text RCA narration after structured artifact causes double-render UX regression")
	})

	It("UT-AF-1408-021: SI-4 — prompt mandates present_decision for no-action scenarios", func() {
		Expect(instruction).To(ContainSubstring("No remediation is needed"),
			"SI-4: present_decision must be called even for no-action scenarios to ensure structured artifact")
	})

	It("UT-AF-1408-022: SI-4 — prompt mandates present_decision when no workflows found", func() {
		Expect(instruction).To(ContainSubstring("No workflows are discovered"),
			"SI-4: empty workflow discovery must still produce structured artifact via present_decision")
	})

	It("UT-AF-1408-023: SI-4 — prompt mandates present_decision on tool error paths", func() {
		Expect(instruction).To(ContainSubstring("tool call fails"),
			"SI-4: tool errors must still produce structured artifact for audit traceability")
	})
})

var _ = Describe("Prompt — Tool Call Silence (#1408 Issue 1)", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-1408-040: prompt prohibits text output before tool calls", func() {
		Expect(instruction).To(ContainSubstring("NEVER produce text output before calling a tool"),
			"Tool Call Silence: LLM must not narrate before invoking tools")
	})

	It("UT-AF-1408-041: prompt instructs direct tool invocation without preamble", func() {
		Expect(instruction).To(ContainSubstring("Call tools directly"),
			"Tool Call Silence: tools must be invoked without preamble narration")
	})
})
