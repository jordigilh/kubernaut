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
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func (inv *Investigator) runRCA(ctx context.Context, signal katypes.SignalContext, enrichData *prompt.EnrichmentData, llmCtx LLMInvocationContext) (result *katypes.InvestigationResult, retErr error) {
	correlationID := llmCtx.CorrelationID
	promptSignal := SignalToPrompt(signal)
	LogLabelOverrideOrRejection(inv.logger, signal, promptSignal, correlationID, "RCA")
	systemPrompt, err := inv.builder.RenderInvestigation(promptSignal)
	if err != nil {
		return nil, fmt.Errorf("rendering investigation prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("Investigate: %s %s in %s — %s", signal.Severity, signal.Name, signal.Namespace, signal.Message)},
	}

	loopRes, err := inv.runLLMLoop(ctx, messages, katypes.PhaseRCA, llmCtx)
	if err != nil {
		return nil, err
	}

	alignment.NotifyRCAComplete(ctx, messages)

	var content string
	switch r := loopRes.(type) {
	case *CancelledResult:
		cancelledResult := &katypes.InvestigationResult{
			Cancelled:           true,
			CancelledPhase:      string(katypes.PhaseRCA),
			CancelledAtTurn:     r.Turn,
			AccumulatedMessages: messagesToAuditFormat(r.Messages),
		}
		if r.Tokens != nil {
			s := r.Tokens.Summary()
			cancelledResult.TokenUsage = &katypes.TokenUsageSummary{
				PromptTokens:     s.PromptTokens,
				CompletionTokens: s.CompletionTokens,
				TotalTokens:      s.TotalTokens,
			}
		}
		return cancelledResult, nil
	case *ExhaustedResult:
		return &katypes.InvestigationResult{
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("%s during RCA (maxTurns=%d)", r.Reason, inv.maxTurns),
		}, nil
	case *SubmitResult:
		content = r.Content
	case *TextResult:
		content = r.Content
	default:
		content = ""
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		if retried := inv.retryRCASubmit(ctx, content, messages, llmCtx); retried != nil {
			result = retried
			parseErr = nil
		}
	}
	if parseErr != nil && ctx.Err() != nil {
		return &katypes.InvestigationResult{
			Cancelled:      true,
			CancelledPhase: string(katypes.PhaseRCA),
		}, nil
	}
	if parseErr != nil {
		inv.logger.Info("RCA parse failed after retry, treating as summary",
			"error", parseErr.Error(),
			"correlation_id", correlationID)
		return &katypes.InvestigationResult{
			RCASummary: content,
		}, nil
	}

	// Same-kind sentinel validation gate (Issue #847): when the LLM's
	// remediation_target.kind matches the signal's resource_kind, the LLM may
	// be targeting the symptom reporter instead of the actual root cause.
	// Inject a single correction message and re-run. Max 1 retry.
	result = inv.sameKindValidationGate(ctx, result, signal, messages, llmCtx)

	// Defense-in-depth: RCA phase must never abort the pipeline via
	// needs_human_review from the parser. Only gate-level exhaustion (#1044)
	// is a valid RCA abort. Clear parser-set values BEFORE running the
	// apiVersion gate so the gate's decision is authoritative.
	// Aligned with HAPI v1.2.1 where needs_human_review is parser-driven in Phase 3.
	if result.HumanReviewNeeded {
		inv.logger.Info("clearing parser-set HumanReviewNeeded during RCA (Phase 3 only)",
			"reason", result.HumanReviewReason,
			"correlation_id", correlationID)
		result.HumanReviewNeeded = false
		result.HumanReviewReason = ""
	}

	// apiVersion validation gate (Issue #1044): when the remediation target kind
	// exists in multiple API groups and api_version is missing, the gate retries
	// once. On exhaustion it sets HumanReviewNeeded=true — a valid RCA abort.
	// Runs after clearing so its decision is authoritative and not cleared.
	result = inv.apiVersionValidationGate(ctx, result, messages, llmCtx)

	return result, nil
}

const maxRCAParseRetries = 1

// retryRCASubmit performs a single correction attempt when the RCA parse fails
// (e.g. double-serialized JSON that wasn't caught by the unwrap heuristic, or
// garbage fields). Mirrors retryWorkflowSubmit but scoped to the RCA phase.
func (inv *Investigator) retryRCASubmit(ctx context.Context, lastContent string, history []llm.Message, llmCtx LLMInvocationContext) *katypes.InvestigationResult {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultToolName,
			Description: "Submit root cause analysis result.",
			Parameters:  parser.RCAResultSchema(),
		},
	}

	correctionMsg := `Your response could not be parsed. You MUST call submit_result with a JSON object like:
{"root_cause_analysis":{"summary":"...","severity":"critical","signal_name":"SignalName","contributing_factors":["factor1"],"remediation_target":{"kind":"Deployment","name":"resource","namespace":"ns","api_version":"apps/v1"}},"confidence":0.9}

CRITICAL: root_cause_analysis must be a JSON object, NOT a string. Do NOT wrap it in quotes.`

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "assistant", Content: lastContent},
	)

	for attempt := 0; attempt < maxRCAParseRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		inv.logger.Info("parse-level retry for RCA submit",
			"attempt", attempt+1,
			"max", maxRCAParseRetries,
			"correlation_id", correlationID)

		retryMessages = append(retryMessages,
			llm.Message{Role: "user", Content: correctionMsg},
		)

		retryEvent := audit.NewEvent(audit.EventTypeLLMRequest, correlationID)
		retryEvent.EventAction = audit.ActionLLMRequest
		retryEvent.EventOutcome = audit.OutcomeSuccess
		retryEvent.Data["model"] = modelName
		retryEvent.Data["prompt_length"] = totalPromptLength(retryMessages)
		retryEvent.Data["prompt_preview"] = lastUserMessage(retryMessages, 500)
		retryEvent.Data["retry_attempt"] = attempt + 1
		retryEvent.Data["retry_max"] = maxRCAParseRetries
		retryEvent.Data["phase"] = string(katypes.PhaseRCA)
		retryEvent.Data["retry_reason"] = "rca_parse_correction"
		audit.StoreBestEffort(ctx, inv.auditStore, retryEvent, inv.auditLog())

		resp, err := inv.chatOrStream(ctx, client, llm.ChatRequest{
			Messages: retryMessages,
			Tools:    submitOnlyTools,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
		}, attempt+1, string(katypes.PhaseRCA), modelName, runtimeParams)
		if err != nil {
			inv.logger.Error(err, "RCA retry LLM call failed",
				"correlation_id", correlationID)
			continue
		}
		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		emitToSink(ctx, session.EventTypeReasoningDelta, attempt+1, string(katypes.PhaseRCA), map[string]interface{}{
			"content":       resp.Message.Content,
			"retry_attempt": attempt + 1,
		})

		for _, tc := range resp.ToolCalls {
			if tc.Name == SubmitResultToolName {
				result, parseErr := inv.resultParser.Parse(tc.Arguments)
				if parseErr != nil {
					inv.logger.Error(parseErr, "RCA retry parse still failed",
						"correlation_id", correlationID)
					retryMessages = append(retryMessages, resp.Message)
					continue
				}
				inv.logger.Info("RCA retry succeeded",
					"correlation_id", correlationID)
				return result
			}
		}

		if resp.Message.Content != "" {
			result, parseErr := inv.resultParser.Parse(resp.Message.Content)
			if parseErr == nil {
				inv.logger.Info("RCA retry succeeded from message content",
					"correlation_id", correlationID)
				return result
			}
		}

		retryMessages = append(retryMessages, resp.Message)
	}
	return nil
}
