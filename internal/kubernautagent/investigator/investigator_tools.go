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

package investigator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func escalateMaxTokens(completionTokens int) int {
	if completionTokens > 0 {
		escalated := completionTokens * 2
		if escalated > 16384 {
			return 16384
		}
		return escalated
	}
	return 8192
}

func totalPromptLength(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	return total
}

// lastUserMessagePreviewLen is the fixed truncation length for audit
// prompt_preview fields derived from the last user message. Every caller of
// lastUserMessage uses this same length (unlike truncatePreview's other call
// sites, which vary), so it is hardcoded here rather than threaded as a
// parameter (100 Go Mistakes: unparam).
const lastUserMessagePreviewLen = 500

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return truncatePreview(messages[i].Content, lastUserMessagePreviewLen)
		}
	}
	return ""
}

func truncatePreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// emitReasoningContentEvent live-streams BR-AI-086's captured LLM reasoning
// content via the dedicated EventTypeReasoningContentDelta event, distinct
// from EventTypeReasoningDelta's orchestration narration (#1634, #1635,
// DD-LLM-009). No-op when reasoning is nil (BR-AI-086 AC2 default-disabled
// parity — Reasoning is nil unless an operator opts in). Shared by the three
// call sites that already emit EventTypeReasoningDelta right before this
// call: the main LLM loop (investigator_loop.go) and the RCA/workflow-
// selection parse-retry paths (investigator_rca.go,
// investigator_workflow_selection.go).
func emitReasoningContentEvent(ctx context.Context, reasoning *llm.ReasoningBlock, turn int, phase string) {
	if reasoning == nil {
		return
	}
	emitToSink(ctx, session.EventTypeReasoningContentDelta, turn, phase, map[string]interface{}{
		"text":     reasoning.Text,
		"redacted": reasoning.Redacted,
	})
}

// retryAuditParams groups the per-attempt fields for emitRetryAudit. Kept as
// a config struct (rather than individual parameters) per the Go
// Anti-Pattern Checklist's 8+-parameter rule.
type retryAuditParams struct {
	correlationID string
	modelName     string
	messages      []llm.Message
	attempt       int
	maxAttempts   int
	phase         katypes.Phase
	retryReason   string
}

// emitRetryAudit records a best-effort audit event for one parse-level retry
// attempt (workflow-submit or RCA-submit). Shared by retryWorkflowSubmit and
// retryRCASubmit so the AU-3 field set (model, prompt length/preview, retry
// attempt/max, phase, retry reason) stays byte-identical between both call
// sites.
func (inv *Investigator) emitRetryAudit(ctx context.Context, p retryAuditParams) {
	retryEvent := audit.NewEvent(audit.EventTypeLLMRequest, p.correlationID)
	retryEvent.EventAction = audit.ActionLLMRequest
	retryEvent.EventOutcome = audit.OutcomeSuccess
	retryEvent.Data["model"] = p.modelName
	retryEvent.Data["prompt_length"] = totalPromptLength(p.messages)
	retryEvent.Data["prompt_preview"] = lastUserMessage(p.messages)
	retryEvent.Data["retry_attempt"] = p.attempt
	retryEvent.Data["retry_max"] = p.maxAttempts
	retryEvent.Data["phase"] = string(p.phase)
	retryEvent.Data["retry_reason"] = p.retryReason
	audit.StoreBestEffort(ctx, inv.auditStore, retryEvent, inv.auditLog())
}

func toolNames(defs []llm.ToolDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

// toolDefinitionsForPhase builds the LLM-facing tool schema for phase. When
// ctx carries a fleet tool overlay (DD-FLEET-004), a phase tool whose name
// also appears in the overlay is described using the overlay's BridgeTool
// instead of the local registry's tool of the same name — the LLM sees one
// entry per name either way, so the schema is byte-identical to a hub-local
// investigation's regardless of which cluster backs it (AC-6).
//
// Issue #1729: that override-only behavior left a tool-transparency gap for
// any overlay tool whose name has no local-registry namesake at all — e.g.
// kube-mcp-server's own naming convention (resources_get/resources_list/...,
// pkg/fleet/mcpclient/tool_names.go) never collides with KA's local k8s-tool
// naming convention (kubectl_get_by_name/kubectl_list/...). Such tools were
// present in the resolved overlay and reachable by executeResolved, but never
// advertised to the LLM at all, making them permanently uncallable regardless
// of Helm/gateway wiring. appendNonCollidingOverlayTools below closes that gap
// for the RCA phase (where the local read/k8s tool set already lives) only —
// WorkflowDiscovery/Validation schemas are intentionally left untouched, to
// avoid widening the LLM's action surface for phases that have never exposed
// read tools of any kind (least privilege, AC-6).
func (inv *Investigator) toolDefinitionsForPhase(ctx context.Context, phase katypes.Phase) []llm.ToolDefinition {
	var defs []llm.ToolDefinition
	if inv.registry != nil {
		phaseTools := inv.registry.ToolsForPhase(phase, inv.phaseTools)
		overlay, _ := FleetOverlayFromContext(ctx)
		defs = make([]llm.ToolDefinition, 0, len(phaseTools)+len(overlay)+2)
		seen := make(map[string]struct{}, len(phaseTools))
		for _, t := range phaseTools {
			eff := t
			if ov, found := resolveTool(overlay, t.Name()); found {
				eff = ov
			}
			defs = append(defs, llm.ToolDefinition{
				Name:        eff.Name(),
				Description: eff.Description(),
				Parameters:  eff.Parameters(),
			})
			seen[t.Name()] = struct{}{}
		}
		if phase == katypes.PhaseRCA {
			defs = appendNonCollidingOverlayTools(defs, overlay, seen)
		}
	}

	if phase == katypes.PhaseWorkflowDiscovery {
		defs = append(defs,
			llm.ToolDefinition{
				Name:        SubmitResultWithWorkflowToolName,
				Description: "Submit investigation result WITH a selected workflow. Call this when you have identified a matching workflow.",
				Parameters:  parser.WithWorkflowResultSchema(),
			},
			llm.ToolDefinition{
				Name:        SubmitResultNoWorkflowToolName,
				Description: "Submit investigation result when NO matching workflow exists. Call this when none of the available workflows can remediate the incident.",
				Parameters:  parser.NoWorkflowResultSchema(),
			},
		)
	} else {
		defs = append(defs, llm.ToolDefinition{
			Name:        SubmitResultToolName,
			Description: "Submit the final investigation result as structured JSON. Call this tool when your analysis is complete.",
			Parameters:  submitResultSchemaForPhase(phase),
		})
	}
	return defs
}

// appendNonCollidingOverlayTools appends the fleet overlay's own tool
// definitions to defs for every overlay name not already in seen (i.e. every
// name that was NOT already advertised via a same-named local-registry tool
// in toolDefinitionsForPhase's override loop). Overlay names are sorted
// before appending so the resulting schema is deterministic across calls —
// Go map iteration order is randomized, and a nondeterministic tool schema
// would be both hard to test and, worse, would give the LLM a
// non-reproducible view of its own toolset from one turn to the next.
func appendNonCollidingOverlayTools(defs []llm.ToolDefinition, overlay map[string]tools.Tool, seen map[string]struct{}) []llm.ToolDefinition {
	if len(overlay) == 0 {
		return defs
	}
	names := make([]string, 0, len(overlay))
	for name := range overlay {
		if _, dup := seen[name]; !dup {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		t := overlay[name]
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

func submitResultSchemaForPhase(phase katypes.Phase) json.RawMessage {
	if phase == katypes.PhaseRCA {
		return parser.RCAResultSchema()
	}
	return parser.InvestigationResultSchema()
}

// executeResolved executes name via the fleet tool overlay (DD-FLEET-004)
// when ctx carries one and name is present in it, otherwise via the local
// tool registry unchanged. Callers see identical (string, error) semantics
// regardless of which backend served the call.
//
// For a hub-local investigation (no overlay in ctx), a miss surfaces the
// registry's plain registry.ErrToolNotFound unchanged — unambiguous on its
// own, no cluster context to add. For a fleet-target investigation (overlay
// present but name isn't in it), a miss is wrapped with the cluster ID: a
// bare "tool not found: X" would otherwise leave an operator unable to tell
// whether X was never a valid tool name or whether the fleet overlay simply
// didn't expose it for this cluster (AC-6 — the two failure modes look
// identical without this context).
func (inv *Investigator) executeResolved(ctx context.Context, name string, args json.RawMessage) (string, error) {
	overlay, hasOverlay := FleetOverlayFromContext(ctx)
	if hasOverlay {
		if t, found := resolveTool(overlay, name); found {
			return t.Execute(ctx, args)
		}
	}

	result, err := inv.registry.Execute(ctx, name, args)
	if err != nil && hasOverlay {
		var notFound *registry.ErrToolNotFound
		if errors.As(err, &notFound) {
			clusterID, _ := audit.ClusterIDFromContext(ctx)
			return "", fmt.Errorf("tool %q not found in fleet overlay for cluster %q or local registry: %w", name, clusterID, err)
		}
	}
	return result, err
}

func (inv *Investigator) executeTool(ctx context.Context, name string, args json.RawMessage, correlationID string) string {
	if inv.registry == nil {
		return toolErrorJSON("no registry configured for tool " + name)
	}

	detector := inv.anomalyDetectorFor(correlationID)

	if ar := detector.CheckToolCall(name, args); !ar.Allowed {
		inv.logger.Info("anomaly detector rejected tool call",
			"tool", name,
			"reason", ar.Reason,
		)
		return toolErrorJSON(ar.Reason)
	}

	result, err := inv.executeResolved(ctx, name, args)
	if err != nil {
		inv.logger.Error(err, "tool execution failed",
			"tool", name,
		)
		if ar := detector.RecordFailure(name, args); !ar.Allowed {
			errResult := toolErrorJSON(ar.Reason)
			alignment.SubmitToolStep(ctx, name, errResult)
			return errResult
		}
		errResult := toolErrorJSON(err.Error())
		alignment.SubmitToolStep(ctx, name, errResult)
		return errResult
	}

	if inv.pipeline.Sanitizer != nil {
		sanitized, sanitizeErr := inv.pipeline.Sanitizer.Run(ctx, result)
		if sanitizeErr != nil {
			inv.logger.Error(sanitizeErr, "sanitization failed, fail-closed for SOC2 compliance",
				"tool", name,
			)
			errResult := toolErrorJSON("sanitization failed: tool output withheld")
			alignment.SubmitToolStep(ctx, name, errResult)
			return errResult
		}
		result = sanitized
	}

	alignment.SubmitToolStep(ctx, name, result)

	if inv.pipeline.Summarizer != nil {
		summarized, sumErr := inv.pipeline.Summarizer.MaybeSummarize(ctx, name, result)
		if sumErr != nil {
			inv.logger.Error(sumErr, "summarization failed, returning unsummarized output",
				"tool", name,
			)
		} else {
			result = summarized
		}
	}

	if inv.pipeline.MaxToolOutputSize > 0 {
		result = summarizer.TruncateToolOutput(result, name, inv.pipeline.MaxToolOutputSize)
	}

	return result
}
