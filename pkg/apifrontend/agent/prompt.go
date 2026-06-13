package agent

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// DefaultNamespaceFile is the K8s downward API path for the pod's namespace.
const DefaultNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// ResolveNamespace determines the operational namespace using a two-tier strategy:
// 1. Config override (if non-empty)
// 2. K8s downward API file
// Falls back to "default" if both are absent.
func ResolveNamespace(configOverride, namespaceFile string) string {
	if configOverride != "" {
		return configOverride
	}
	data, err := os.ReadFile(namespaceFile) //nolint:gosec // G304: path is either DefaultNamespaceFile (constant) or config-supplied override, not user input
	if err == nil {
		ns := strings.TrimSpace(string(data))
		if ns != "" {
			return ns
		}
	}
	return "default"
}

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
	sb.Grow(len(base) + 1024)
	sb.WriteString(base)
	sb.WriteString("\n\n## Deployment Context\n")
	sb.WriteString(fmt.Sprintf("- Kubernaut is deployed in the `%s` namespace\n", namespace))
	sb.WriteString("- All remediation queries default to this namespace unless the user specifies otherwise\n")
	sb.WriteString("- Known kubernaut.ai resource types for kubectl_get/kubectl_list: ")
	sb.WriteString("RemediationRequest, RemediationWorkflow, InvestigationSession, AIAnalysis, ")
	sb.WriteString("SignalProcessing, EffectivenessAssessment, WorkflowExecution, ActionType, NotificationRequest\n")
	sb.WriteString("\n## Tool Usage Rules\n")
	sb.WriteString("- For investigation and remediation, always use kubernaut MCP tools (kubernaut_investigate, ")
	sb.WriteString("kubernaut_investigate_alert, kubernaut_remediate, kubernaut_discover_workflows, ")
	sb.WriteString("kubernaut_select_workflow, kubernaut_watch). ")
	sb.WriteString("NEVER use kubectl tools directly for investigation or remediation actions.\n")
	sb.WriteString("- kubectl_get, kubectl_list, kubectl_list_events, list_alerts, and get_alert_details are permitted ONLY for observation (reading cluster state).\n")
	sb.WriteString("- list_alerts and get_alert_details query Prometheus/Thanos for currently firing or pending alerts. ")
	sb.WriteString("Use them for preflight investigation context before delegating to kubernaut_investigate or kubernaut_investigate_alert.\n")
	sb.WriteString("- kubernaut_investigate_alert: use when you know the specific Prometheus alert name to investigate. ")
	sb.WriteString("Provide alert_name, api_version, kind, name, and namespace (optional for cluster-scoped). ")
	sb.WriteString("The backend validates the alert exists.\n")
	sb.WriteString("- kubernaut_investigate: use when investigating by resource identity (api_version, kind, name, namespace). ")
	sb.WriteString("The backend determines the relevant alert via triage.\n")
	sb.WriteString("- When calling kubernaut_remediate, provide: api_version, namespace, kind, name, description. ")
	sb.WriteString("The namespace is the workload namespace where the target resource lives.\n")
	sb.WriteString("  AF auto-resolves the remaining fields:\n")
	sb.WriteString("  - severity: via the Prometheus severity triage pipeline\n")
	sb.WriteString("  - signalName: from AlertManager alerts, rule names, or K8s events\n")
	sb.WriteString("  - signalSource: hardcoded to a2a-agent\n")
	return sb.String()
}

// roleGuidanceMap maps known JWT group names to behavioral guidance text.
// Only known groups produce guidance; unknown groups are silently ignored (SC-7).
// Raw group names are never included in output (AC-6).
var roleGuidanceMap = map[string]string{
	"sre": "You have full operational access. You may investigate, remediate, " +
		"and manage all incidents. Proactively suggest root cause analysis " +
		"and remediation workflows when issues are detected. " +
		"Approval/rejection actions are handled exclusively via the Console UI.",
	"ai-orchestrator": "You have full operational access. You may investigate, remediate, " +
		"and manage all incidents. Proactively suggest root cause analysis " +
		"and remediation workflows when issues are detected. " +
		"Approval/rejection actions are handled exclusively via the Console UI.",
	"observability": "You have read-only access. Focus on presenting cluster state, " +
		"investigation results, and audit trails. Do not initiate remediations or approvals.",
	"remediation-approver": "Your primary role is approval governance. Focus on reviewing " +
		"pending approval requests, assessing risk, and presenting investigation context. " +
		"Approval/rejection actions are performed via the Console UI buttons, not through chat.",
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
