/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
		Expect(instruction).To(ContainSubstring("MUST call kubernaut_present_decision"))
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

	// Issue #1658 (BR-AI-023): any tool failure must be translated into a
	// natural-language explanation, never surfaced as raw tool-call args/JSON.
	It("UT-AF-1658-010: prompt mandates natural-language explanation on any tool error, never raw args", func() {
		Expect(instruction).To(ContainSubstring("NEVER surface raw tool-call arguments"))
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
			result := agentpkg.BuildInstruction("kubernaut-system", true)
			Expect(result).To(ContainSubstring("You are the Kubernaut API Frontend agent"))
			Expect(result).To(ContainSubstring("Security Boundaries"))
		})

		It("UT-AF-1275-011: output contains deployment namespace (CM-6)", func() {
			result := agentpkg.BuildInstruction("kubernaut-system", true)
			Expect(result).To(ContainSubstring("kubernaut-system"))
			Expect(result).To(ContainSubstring("Deployment Context"))
		})

		It("UT-AF-1275-012: empty namespace falls back to default (SI-10)", func() {
			result := agentpkg.BuildInstruction("", true)
			Expect(result).To(ContainSubstring("default"))
			Expect(result).NotTo(ContainSubstring("``"))
		})

		It("UT-AF-1275-013: output contains kubernaut.ai CRD types (CM-6)", func() {
			result := agentpkg.BuildInstruction("kubernaut-system", true)
			Expect(result).To(ContainSubstring("RemediationRequest"))
			Expect(result).To(ContainSubstring("InvestigationSession"))
			Expect(result).To(ContainSubstring("WorkflowExecution"))
		})

		It("UT-AF-1275-014: intent group 'investigate' contains expected tools", func() {
			result := agentpkg.BuildInstruction("ns", true)
			Expect(result).To(ContainSubstring("kubernaut_investigate"))
		})

		It("UT-AF-1275-015: intent group 'observe' contains kubectl tools", func() {
			result := agentpkg.BuildInstruction("ns", true)
			Expect(result).To(ContainSubstring("kubectl_get"))
			Expect(result).To(ContainSubstring("kubectl_list"))
		})

		It("UT-AF-1275-016: intent group 'fix' references 4-phase journey", func() {
			result := agentpkg.BuildInstruction("ns", true)
			Expect(result).To(ContainSubstring("kubernaut_discover_workflows"))
			Expect(result).To(ContainSubstring("kubernaut_select_workflow"))
			Expect(result).To(ContainSubstring("kubernaut_watch"))
		})

		It("UT-AF-1275-017: intent group 'approve' contains approval tools", func() {
			result := agentpkg.BuildInstruction("ns", true)
			Expect(result).To(ContainSubstring("kubernaut_approve"))
			Expect(result).To(ContainSubstring("kubernaut_list_approval_requests"))
		})

		It("UT-AF-1275-018: intent group 'audit' contains history tools", func() {
			result := agentpkg.BuildInstruction("ns", true)
			Expect(result).To(ContainSubstring("kubernaut_get_audit_trail"))
			Expect(result).To(ContainSubstring("kubernaut_get_remediation_history"))
		})

		It("UT-AF-1275-019: intent group 'interactive' contains session tools", func() {
			result := agentpkg.BuildInstruction("ns", true)
			Expect(result).To(ContainSubstring("kubernaut_investigate"))
			Expect(result).To(ContainSubstring("kubernaut_reconnect"))
		})
	})

	// Issue #1658 (BR-AI-023, BR-AI-024): the prompt must not advertise alert
	// tools that aren't actually registered (cfg.PromClient == nil), or the
	// model falls back to guessing an invalid kubectl_list kind and leaks the
	// raw, failed tool-call arguments back to the user.
	Describe("BuildInstruction alert tool gating (#1658)", func() {
		It("UT-AF-1658-001: alertToolsEnabled=true advertises alert tools for observation", func() {
			result := agentpkg.BuildInstruction("ns", true)
			Expect(result).To(ContainSubstring("list_alerts"))
			Expect(result).To(ContainSubstring("get_alert_details"))
			Expect(result).To(ContainSubstring("kubernaut_investigate_alert"))
		})

		It("UT-AF-1658-002: alertToolsEnabled=false omits alert tools from the observation permission list", func() {
			result := agentpkg.BuildInstruction("ns", false)
			Expect(result).NotTo(ContainSubstring("kubectl_get, kubectl_list, kubectl_list_events, list_alerts, and get_alert_details are permitted"))
			Expect(result).NotTo(ContainSubstring("list_alerts and get_alert_details query Prometheus/Thanos"))
		})

		It("UT-AF-1658-003: alertToolsEnabled=false explicitly states alert querying is unavailable", func() {
			result := agentpkg.BuildInstruction("ns", false)
			Expect(result).To(ContainSubstring("NOT available in this deployment"))
			Expect(result).To(ContainSubstring("Alerts are not a Kubernetes resource"))
		})

		It("UT-AF-1658-004: alertToolsEnabled=false instructs the model not to guess a kubectl kind for alerts", func() {
			result := agentpkg.BuildInstruction("ns", false)
			Expect(result).To(SatisfyAny(
				ContainSubstring("never call kubectl_get/kubectl_list with kind=\"Alert\""),
				ContainSubstring(`never call kubectl_get/kubectl_list with kind="Alert"`),
			))
		})

		It("UT-AF-1658-005: alertToolsEnabled=false still contains the immutable core prompt (SC-7)", func() {
			result := agentpkg.BuildInstruction("ns", false)
			Expect(result).To(ContainSubstring("You are the Kubernaut API Frontend agent"))
			Expect(result).To(ContainSubstring("Security Boundaries"))
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
			result := agentpkg.BuildInstruction("kubernaut-system", true)
			Expect(result).To(ContainSubstring("kubernaut MCP tools"))
			Expect(result).To(ContainSubstring("NEVER use kubectl"))
		})

		It("UT-AF-1282-PROMPT-002: prompt documents all AF auto-resolved fields", func() {
			result := agentpkg.BuildInstruction("kubernaut-system", true)
			Expect(result).To(ContainSubstring("provide: api_version, namespace, kind, name, description"))
			Expect(result).To(ContainSubstring("workload namespace where the target resource lives"))
			Expect(result).To(ContainSubstring("severity: via the Prometheus severity triage pipeline"))
			Expect(result).To(ContainSubstring("signalName: from AlertManager alerts"))
			Expect(result).To(ContainSubstring("signalSource: hardcoded to a2a-agent"))
		})
	})

	Describe("InstructionProvider (#1276)", func() {
		It("UT-AF-1276-001: preserves core prompt immutability (SC-7)", func() {
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("You are the Kubernaut API Frontend agent"))
			Expect(result).To(ContainSubstring("Security Boundaries"))
		})

		It("UT-AF-1276-008: nil identity returns base instruction only (SC-7)", func() {
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Your Role Context"))
		})

		It("UT-AF-1276-009: empty groups returns base instruction only", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "alice", []string{})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Your Role Context"))
		})

		It("UT-AF-1276-002: SRE group adds full-access guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "alice", []string{"sre"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Your Role Context"))
			Expect(result).To(ContainSubstring("full operational access"))
		})

		It("UT-AF-1276-003: viewer group adds read-only guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "bob", []string{"observability"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("read-only"))
		})

		It("UT-AF-1276-004: approver group adds approval guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "carol", []string{"remediation-approver"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("approval"))
		})

		It("UT-AF-1276-005: CICD group adds automation guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "bot", []string{"cicd"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("automation"))
		})

		It("UT-AF-1276-006: audit group adds compliance guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "auditor", []string{"l3-audit"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("compliance"))
		})

		It("UT-AF-1276-007: multi-role user gets additive guidance (AC-6)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "multi", []string{"sre", "remediation-approver"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("full operational access"))
			Expect(result).To(ContainSubstring("approval"))
		})

		It("UT-AF-1276-010: unknown groups produce no extra guidance (SC-7)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "unknown", []string{"custom-team", "random"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
			result, err := provider(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(ContainSubstring("Your Role Context"))
		})

		It("UT-AF-1276-011: raw group names not leaked into prompt (SC-7)", func() {
			ctx := agentpkg.MockReadonlyContext(context.Background(), "alice", []string{"sre", "l3-audit"})
			provider := agentpkg.NewInstructionProvider("kubernaut-system", true)
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
		built := agentpkg.BuildInstruction("kubernaut-system", true)
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

	It("UT-AF-1407-011: SI-4 prompt contains proceed-to-discovery in OTHERWISE branch", func() {
		Expect(instruction).To(ContainSubstring("Proceed to Phase 2"),
			"SI-4: OTHERWISE branch must auto-proceed to workflow discovery for audit completeness")
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

// =============================================================================
// Issue #1430: Skip workflow discovery when RCA concludes no action required
// BR-HAPI-200: Handling non-actionable outcomes
// =============================================================================

var _ = Describe("Prompt #1430 / BR-HAPI-200: No-action exception in Phase 1 CRITICAL block", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	// UT-AF-1430-001: Phase 1 CRITICAL uses conditional branching with no-action as first branch.
	// FedRAMP SI-4: the agent's decision logic must be observable/auditable
	// in the prompt contract.
	It("UT-AF-1430-001: SI-4 prompt uses conditional branching for Phase 1 to Phase 2 decision", func() {
		Expect(instruction).To(ContainSubstring("evaluate the RCA conclusion"),
			"#1430 / SI-4: Phase 1 must require RCA evaluation before deciding next step")
		Expect(instruction).To(ContainSubstring("Do NOT call kubernaut_discover_workflows"),
			"#1430 / SI-4: no-action branch must explicitly prohibit workflow discovery")
		Expect(instruction).To(ContainSubstring("self-resolved"),
			"#1430 / SI-4: no-action branch must mention self-resolved signals")
		Expect(instruction).NotTo(MatchRegexp(
			`MUST automatically proceed to Phase 2.*without waiting`),
			"#1430: unconditional MUST proceed directive must not appear (causes LLM to skip branch evaluation)")
	})

	// UT-AF-1430-002: Edge Scenarios section retained and consistent.
	// FedRAMP AU-3: present_decision with empty options is the audit record
	// for no-action outcomes.
	It("UT-AF-1430-002: AU-3 prompt retains present_decision with options: [] for no-action", func() {
		Expect(instruction).To(ContainSubstring("present_decision"),
			"#1430 / AU-3: present_decision must remain in prompt for no-action path")
		Expect(instruction).To(ContainSubstring("options: []"),
			"#1430 / AU-3: prompt must document empty options list for no-action scenario")
	})
})

// =============================================================================
// Issue #1899: Alert Prioritization must not override the Observation Mode
// consent gate. Live-repro'd on a shared cluster: a bare "list active alerts"
// query caused the LLM to autonomously call kubernaut_investigate_alert with
// zero user consent, because the unconditional "Always use
// kubernaut_investigate_alert..." directive had no gate on user intent/mode.
// This is a defense-in-depth textual fix alongside the harness-enforced
// consent guard (DD-AF-011): even within a single genuine turn (no
// reinvocation involved), the prompt must not let Alert Prioritization
// override Observation Mode's "unless the user EXPLICITLY requests it" rule.
// =============================================================================

var _ = Describe("Prompt — Alert Prioritization Consent Gate (#1899)", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-1899-010: AC-6/SI-10 Alert Prioritization declares itself subordinate to Observation Mode", func() {
		Expect(instruction).To(ContainSubstring("SUBORDINATE TO THE DECISION ALGORITHM AND OBSERVATION MODE"),
			"#1899: Alert Prioritization must explicitly state it never overrides the consent gate")
	})

	It("UT-AF-1899-011: AC-6 prompt explicitly forbids treating a bare list/show/status as an implicit investigate request", func() {
		Expect(instruction).To(ContainSubstring("Do NOT treat a `prioritized` field in that response as an implicit instruction to investigate anything"),
			"#1899: a list_alerts response's prioritized field must not be read as investigation consent")
	})

	It("UT-AF-1899-012: AC-6/SI-10 kubernaut_investigate_alert directive is gated on a prior investigate/fix/diagnose trigger", func() {
		Expect(instruction).To(ContainSubstring("ONLY when the user's message already matched the Interactive/Autonomous/Full-Interactive-Remediation triggers above"),
			"#1899: kubernaut_investigate_alert must only fire once the user's intent already matched an investigate/fix trigger, not a bare list/show/status query")
		Expect(instruction).To(ContainSubstring("If the user's message was purely informational, do NOT call `kubernaut_investigate_alert`"),
			"#1899: prompt must explicitly forbid investigate_alert on purely informational messages")
	})

	It("UT-AF-1899-013: AC-6 prompt does not contain the old unconditional investigate_alert directive", func() {
		Expect(instruction).NotTo(ContainSubstring("Always use `kubernaut_investigate_alert` with `alerts[selected_index]` unless the user explicitly picks a different one from `tied_indices`.\n\n## Output Style"),
			"#1899: the unconditional (unqualified) directive must not survive verbatim adjacent to the next section")
	})
})

// =============================================================================
// DD-AF-011 (#1899): Full Interactive Remediation must declare interaction_mode
//
// The harness-enforced phase-transition consent gate fails safe to
// "interactive" whenever kubernaut_investigate's interaction_mode argument
// is omitted or unrecognized. Before this fix, "Full Interactive
// Remediation"'s autonomous-interactive sub-case (line: "select the
// highest-confidence workflow automatically") told the LLM to auto-proceed
// through discover_workflows/select_workflow, but never instructed it to
// declare interaction_mode: full_remediation_autonomous on the triggering
// kubernaut_investigate call. Once the consent gate went live, that
// omission would have silently regressed the autonomous-interactive flow:
// the gate would always fail-safe-block discover_workflows waiting for a
// user turn that, in this mode, is never coming -- turning "investigate
// and fix" (with oversight-implying context) into a permanent stall. This
// closes that gap with an explicit instruction, verified textually here
// (a live-LLM behavioral guarantee is out of scope for a prompt-content
// test, consistent with every other prompt.txt directive in this suite).
// =============================================================================

var _ = Describe("Prompt — Full Interactive Remediation declares interaction_mode (#1899)", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-1899-014: AC-6/SI-10 autonomous-interactive sub-case instructs declaring full_remediation_autonomous", func() {
		Expect(instruction).To(ContainSubstring(`interaction_mode: "full_remediation_autonomous"`),
			"#1899: without this explicit instruction, the consent gate's fail-safe interactive default would permanently stall the autonomous-interactive flow at discover_workflows, waiting for a user turn that never comes")
	})

	It("UT-AF-1899-015: prompt explains WHY the mode must be declared, not just what value to use", func() {
		Expect(instruction).To(ContainSubstring("consent gate will block workflow discovery/selection"),
			"#1899: the instruction must explain the consequence of omitting interaction_mode, not just state the value, so the directive survives future prompt edits/summarization with its rationale intact")
	})

	It("UT-AF-1899-016: interactive sub-case explicitly notes interaction_mode is omitted (fail-safe default applies)", func() {
		Expect(instruction).To(ContainSubstring(`omit interaction_mode`),
			"#1899: the interactive sub-case should explicitly note it relies on the fail-safe default, disambiguating it from the autonomous-interactive sub-case's explicit declaration")
	})
})

// =============================================================================
// Issue #1915: plain "investigate" must default to full_remediation
//
// Before this fix, a plain "investigate X" request never declared
// interaction_mode on kubernaut_investigate, so the harness fail-safe
// defaulted to bare interactive mode (Phase2Blocked=true) and removed
// kubernaut_discover_workflows from the model's tool list. The prompt's own
// "CRITICAL — Phase 1 to Phase 2" section then contradictorily told the
// model to "proceed to Phase 2" anyway, causing the model to find the tool
// missing and incorrectly conclude "no matching workflows" instead of
// pausing correctly. The fix makes full_remediation (auto-discover, pause
// only before workflow selection) the prompt-declared default for plain
// investigate requests, reserving bare interactive (RCA-only, no
// auto-discovery) for an explicit opt-out phrase.
// =============================================================================

var _ = Describe("Prompt — Plain Investigate Defaults to full_remediation (#1915)", func() {
	var instruction string

	BeforeEach(func() {
		cfg := agentpkg.DefaultTestConfig()
		instruction = cfg.Instruction
	})

	It("UT-AF-1915-001: AC-6/SI-10 Mode Detection declares full_remediation for the default plain-investigate trigger", func() {
		Expect(instruction).To(ContainSubstring(`interaction_mode: "full_remediation"`),
			"#1915: plain investigate/what's wrong with/diagnose/look into must now explicitly declare full_remediation on kubernaut_investigate, not rely on the (bare-interactive) omitted default")
	})

	It("UT-AF-1915-002: AC-6 an explicit RCA-only opt-out phrase still maps to omitted interaction_mode (bare interactive)", func() {
		Expect(instruction).To(ContainSubstring("Interactive Mode — RCA Only"),
			"#1915: bare interactive (no auto-discovery) must remain reachable via an explicit, distinctly-named opt-out sub-case")
		Expect(instruction).To(SatisfyAny(
			ContainSubstring("just investigate"),
			ContainSubstring("investigate only"),
		), "#1915: the RCA-only opt-out sub-case must still list the explicit trigger phrases")
	})

	It("UT-AF-1915-003: SI-4 the Phase 1->2 CRITICAL section ties the discover_workflows decision to the already-declared mode, not to the model's own free choice", func() {
		Expect(instruction).To(ContainSubstring("depends on the interaction_mode you already declared"),
			"#1915: the CRITICAL section must explain that tool availability is harness-gated by the earlier Mode Detection decision, resolving the #1915 contradiction where the prompt said \"proceed\" while the harness had already hidden the tool")
	})

	It("UT-AF-1915-004: the autonomous-interactive sub-case is unambiguously distinct from the new full_remediation default", func() {
		Expect(instruction).To(ContainSubstring("Autonomous Selection"),
			"#1915: full_remediation_autonomous's own sub-case must be clearly named apart from the new full_remediation default, so the model does not conflate 'investigate' with 'investigate and fix'")
	})
})
