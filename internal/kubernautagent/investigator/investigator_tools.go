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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
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

func (inv *Investigator) executeTool(ctx context.Context, name string, args json.RawMessage) string {
	if inv.registry == nil {
		return toolErrorJSON("no registry configured for tool " + name)
	}

	if ar := inv.pipeline.AnomalyDetector.CheckToolCall(name, args); !ar.Allowed {
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
		if ar := inv.pipeline.AnomalyDetector.RecordFailure(name, args); !ar.Allowed {
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
