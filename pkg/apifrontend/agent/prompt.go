package agent

import (
	_ "embed"
	"fmt"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

//go:embed prompt.txt
var embeddedPrompt string

// defaultInstruction returns the system prompt loaded from the embedded prompt.txt.
// The file can be overridden at runtime by supplying WithInstruction.
func defaultInstruction() string {
	return strings.TrimSpace(embeddedPrompt)
}

// BuildInstruction constructs the full agent instruction by appending deployment
// context (namespace, CRD types) to the immutable embedded prompt. The core prompt
// is never modified — this function only appends (SC-7 boundary protection).
func BuildInstruction(namespace string) string {
	if namespace == "" {
		namespace = "default"
	}

	base := defaultInstruction()
	var sb strings.Builder
	sb.Grow(len(base) + 512)
	sb.WriteString(base)
	sb.WriteString("\n\n## Deployment Context\n")
	sb.WriteString(fmt.Sprintf("- Kubernaut is deployed in the `%s` namespace\n", namespace))
	sb.WriteString("- All remediation queries default to this namespace unless the user specifies otherwise\n")
	sb.WriteString("- Known kubernaut.ai resource types for kubectl_get/kubectl_list: ")
	sb.WriteString("RemediationRequest, RemediationWorkflow, InvestigationSession, AIAnalysis, ")
	sb.WriteString("SignalProcessing, EffectivenessAssessment, WorkflowExecution, ActionType, NotificationRequest\n")
	return sb.String()
}

// roleGuidanceMap maps known JWT group names to behavioral guidance text.
// Only known groups produce guidance; unknown groups are silently ignored (SC-7).
// Raw group names are never included in output (AC-6).
var roleGuidanceMap = map[string]string{
	"sre": "You have full operational access. You may investigate, remediate, " +
		"approve, and manage all incidents. Proactively suggest root cause analysis " +
		"and remediation workflows when issues are detected.",
	"ai-orchestrator": "You have full operational access. You may investigate, remediate, " +
		"approve, and manage all incidents. Proactively suggest root cause analysis " +
		"and remediation workflows when issues are detected.",
	"observability": "You have read-only access. Focus on presenting cluster state, " +
		"investigation results, and audit trails. Do not initiate remediations or approvals.",
	"remediation-approver": "Your primary role is approval governance. Focus on reviewing " +
		"pending approval requests, assessing risk, and providing approve/reject decisions " +
		"with justification.",
	"cicd": "You are operating in an automation context. Prefer terse, structured responses. " +
		"Focus on status checks, remediation watching, and programmatic workflows. " +
		"Minimize conversational output.",
	"l3-audit": "Your primary focus is compliance and audit. Emphasize audit trails, " +
		"effectiveness assessments, and remediation history. Present data in formats " +
		"suitable for compliance reporting.",
}

// roleGuidance builds an additive composition of role guidance paragraphs for
// the given groups. Each recognized group contributes its paragraph; unrecognized
// groups are silently skipped.
func roleGuidance(groups []string) string {
	var paragraphs []string
	seen := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		if text, ok := roleGuidanceMap[g]; ok {
			paragraphs = append(paragraphs, text)
		}
	}
	if len(paragraphs) == 0 {
		return ""
	}
	return strings.Join(paragraphs, "\n\n")
}

// NewInstructionProvider returns an ADK InstructionProvider that dynamically
// constructs the agent instruction per-request. It appends role-aware behavioral
// guidance based on the authenticated user's JWT groups (AC-6). The base prompt
// (from BuildInstruction) is always included; role guidance is additive only.
func NewInstructionProvider(namespace string) llmagent.InstructionProvider {
	base := BuildInstruction(namespace)
	return func(ctx agent.ReadonlyContext) (string, error) {
		if ctx == nil {
			return base, nil
		}
		identity := auth.UserIdentityFromContext(ctx)
		if identity == nil || len(identity.Groups) == 0 {
			return base, nil
		}
		guidance := roleGuidance(identity.Groups)
		if guidance == "" {
			return base, nil
		}
		var sb strings.Builder
		sb.Grow(len(base) + len(guidance) + 64)
		sb.WriteString(base)
		sb.WriteString("\n\n## Your Role Context\n")
		sb.WriteString(guidance)
		sb.WriteByte('\n')
		return sb.String(), nil
	}
}
