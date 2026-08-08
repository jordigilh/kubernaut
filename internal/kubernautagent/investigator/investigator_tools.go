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
	"strings"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
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

func lastUserMessage(messages []llm.Message, maxLen int) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return truncatePreview(messages[i].Content, maxLen)
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

// reasoningDeltaText builds the "text" payload for the EventTypeReasoningDelta
// event that streams into the console's ThinkingPanel (KA -> AF -> A2A
// artifact channel; see pkg/apifrontend/tools/ka_investigate_mcp.go's
// FormatEventForUser/emitEventToA2A -> launcher.EmitReasoningSafe). Sourcing
// this exclusively from msg.Content (the pre-#1935 behavior) leaves the panel
// silently blank on Anthropic extended-thinking turns: confirmed against live
// production audit data for rr-618ac7d3b894-ba320bf0, where every
// claude-sonnet-5 RCA-phase turn (5/5) had Content=="" while making 1-4 tool
// calls, because Sonnet 5 puts its narrative in the private, signed thinking
// block instead of visible Content. Authority: BR-AI-086, FedRAMP CC7.2
// (#1935 finding #3).
//
// #2010: Claude's extended-thinking + tool-use behavior frequently ends the
// private thinking block with a short one-line plan and then repeats that
// identical line as the visible Content right before the tool_use block, so
// Reasoning.Text and Content are genuine, byte-for-byte (or whitespace-only)
// duplicates on a meaningful fraction of turns. Concatenating both
// unconditionally in that case produced a single reasoning_delta event whose
// text was literally "X\n\nX", so the Console's ThinkingPanel showed the
// same sentence twice separated by a blank line. When the two match (after
// trimming), only Reasoning.Text is surfaced -- no information is lost
// since Content is identical modulo whitespace. Authority: BR-AI-086,
// FedRAMP CC7.2/SI-10.
func reasoningDeltaText(msg llm.Message) string {
	if msg.Reasoning == nil || msg.Reasoning.Text == "" {
		return msg.Content
	}
	if msg.Content == "" || strings.TrimSpace(msg.Content) == strings.TrimSpace(msg.Reasoning.Text) {
		return msg.Reasoning.Text
	}
	return msg.Reasoning.Text + "\n\n" + msg.Content
}

func toolNames(defs []llm.ToolDefinition) []string {
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func (inv *Investigator) toolDefinitionsForPhase(phase katypes.Phase) []llm.ToolDefinition {
	var defs []llm.ToolDefinition
	if inv.registry != nil {
		phaseTools := inv.registry.ToolsForPhase(phase, inv.phaseTools)
		defs = make([]llm.ToolDefinition, 0, len(phaseTools)+2)
		for _, t := range phaseTools {
			defs = append(defs, llm.ToolDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			})
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

func submitResultSchemaForPhase(phase katypes.Phase) json.RawMessage {
	if phase == katypes.PhaseRCA {
		return parser.RCAResultSchema()
	}
	return parser.InvestigationResultSchema()
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

	result, err := inv.registry.Execute(ctx, name, args)
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
