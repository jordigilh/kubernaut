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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// runRCA does not consume enrichment data: per ADR-055, enrichment moves to
// HAPI-driven EnrichmentService (Phase 2) and is only sent to the LLM in
// Phase 3 (workflow selection). The caller (Investigate) retains its own
// enrichData reference and applies it via InjectRemediationTarget after
// this returns, independently of this function.
func (inv *Investigator) runRCA(ctx context.Context, signal katypes.SignalContext, llmCtx LLMInvocationContext) (result *katypes.InvestigationResult, retErr error) {
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

	finalized, content, reasoning, done := inv.classifyRCALoopResult(loopRes)
	if done {
		return finalized, nil
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		if retried, retriedReasoning := inv.retryRCASubmit(ctx, content, messages, llmCtx); retried != nil {
			result = retried
			reasoning = retriedReasoning
			parseErr = nil
		}
	}
	if parseErr != nil && ctx.Err() != nil {
		// nolint:nilerr // intentional: cancellation is converted into a
		// typed Cancelled result, not propagated as err -- matches the
		// same idiom in investigator_loop.go (Issue #1546 Tier 3).
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
			Reasoning:  toReasoningSummary(reasoning),
		}, nil
	}
	result.Reasoning = toReasoningSummary(reasoning)

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

// classifyRCALoopResult converts the raw LoopResult from runLLMLoop into
// either a fully-finalized InvestigationResult (Cancelled/Exhausted — done
// true, caller returns it immediately) or the raw content string still to be
// parsed plus its turn's reasoning block, if any (SubmitResult/TextResult —
// done false).
func (inv *Investigator) classifyRCALoopResult(loopRes LoopResult) (result *katypes.InvestigationResult, content string, reasoning *llm.ReasoningBlock, done bool) {
	switch r := loopRes.(type) {
	case *CancelledResult:
		return buildCancelledRCAResult(r), "", nil, true
	case *ExhaustedResult:
		return &katypes.InvestigationResult{
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("%s during RCA (maxTurns=%d)", r.Reason, inv.maxTurns),
		}, "", nil, true
	case *SubmitResult:
		return nil, r.Content, r.Reasoning, false
	case *TextResult:
		return nil, r.Content, r.Reasoning, false
	default:
		return nil, "", nil, false
	}
}

// toReasoningSummary converts an llm.ReasoningBlock (LLM-protocol layer,
// carries the opaque replay Signature) into the audit-safe
// katypes.ReasoningSummary (business/audit layer, text+redacted only) —
// see ReasoningSummary's doc comment for the compliance rationale. Returns
// nil when b is nil, so callers can assign unconditionally.
func toReasoningSummary(b *llm.ReasoningBlock) *katypes.ReasoningSummary {
	if b == nil {
		return nil
	}
	return &katypes.ReasoningSummary{Text: b.Text, Redacted: b.Redacted}
}

// buildCancelledRCAResult constructs the Cancelled InvestigationResult
// snapshot from a CancelledResult loop outcome, including token usage when
// available.
func buildCancelledRCAResult(r *CancelledResult) *katypes.InvestigationResult {
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
	return cancelledResult
}

const maxRCAParseRetries = 1

// retryRCASubmit performs a single correction attempt when the RCA parse fails
// (e.g. double-serialized JSON that wasn't caught by the unwrap heuristic, or
// garbage fields). Mirrors retryWorkflowSubmit but scoped to the RCA phase.
// The returned reasoning (nil unless a reasoning-capable model responded)
// belongs to the retry turn that produced result, not the original turn.
func (inv *Investigator) retryRCASubmit(ctx context.Context, lastContent string, history []llm.Message, llmCtx LLMInvocationContext) (result *katypes.InvestigationResult, reasoning *llm.ReasoningBlock) {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	submitOnlyTools := submitOnlyRCATools()

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
			return nil, nil
		}
		inv.logger.Info("parse-level retry for RCA submit",
			"attempt", attempt+1,
			"max", maxRCAParseRetries,
			"correlation_id", correlationID)

		retryMessages = append(retryMessages, llm.Message{Role: "user", Content: correctionMsg})

		result, retriedReasoning, updated, ok := inv.attemptRCASubmitRetry(ctx, rcaSubmitRetryParams{
			attempt: attempt, retryMessages: retryMessages, tools: submitOnlyTools,
			correlationID: correlationID, modelName: modelName, client: client,
			runtimeParams: runtimeParams, tokens: tokens,
		})
		if ok {
			return result, retriedReasoning
		}
		retryMessages = updated
	}
	return nil, nil
}

// rcaSubmitRetryParams groups the fields needed by attemptRCASubmitRetry.
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type rcaSubmitRetryParams struct {
	attempt       int
	retryMessages []llm.Message
	tools         []llm.ToolDefinition
	correlationID string
	modelName     string
	client        llm.Client
	runtimeParams llm.RuntimeParams
	tokens        *TokenAccumulator
}

// attemptRCASubmitRetry runs one parse-retry attempt: emits the retry audit
// event, calls the LLM with p.retryMessages (which already include the
// correction message), and tries to parse a valid result from the response.
// Returns ok=false with updatedMessages appended for the next attempt when
// the LLM call failed or no valid result could be parsed.
func (inv *Investigator) attemptRCASubmitRetry(ctx context.Context, p rcaSubmitRetryParams) (result *katypes.InvestigationResult, reasoning *llm.ReasoningBlock, updatedMessages []llm.Message, ok bool) {
	inv.emitRetryAudit(ctx, retryAuditParams{
		correlationID: p.correlationID,
		modelName:     p.modelName,
		messages:      p.retryMessages,
		attempt:       p.attempt + 1,
		maxAttempts:   maxRCAParseRetries,
		phase:         katypes.PhaseRCA,
		retryReason:   "rca_parse_correction",
	})

	resp, err := inv.chatOrStream(ctx, p.client, llm.ChatRequest{
		Messages: p.retryMessages,
		Tools:    p.tools,
		Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.RCAResultSchema()},
	}, p.attempt+1, string(katypes.PhaseRCA), p.runtimeParams)
	if err != nil {
		inv.logger.Error(err, "RCA retry LLM call failed",
			"correlation_id", p.correlationID)
		return nil, nil, p.retryMessages, false
	}
	if p.tokens != nil {
		p.tokens.Add(resp.Usage)
	}

	emitToSink(ctx, session.EventTypeReasoningDelta, p.attempt+1, string(katypes.PhaseRCA), map[string]interface{}{
		"content":       resp.Message.Content,
		"retry_attempt": p.attempt + 1,
	})

	if parsed, ok := inv.tryParseRCASubmitToolCall(resp, p.correlationID); ok {
		return parsed, resp.Message.Reasoning, p.retryMessages, true
	}

	return nil, nil, append(p.retryMessages, resp.Message), false
}

// tryParseRCASubmitToolCall attempts to extract a valid InvestigationResult
// from a single retryRCASubmit LLM response: first by scanning tool calls for
// SubmitResultToolName, then — if none parsed — by parsing the raw message
// content as a fallback (some models emit the RCA JSON as plain text instead
// of calling the tool). Returns ok=false when neither path yielded a result.
func (inv *Investigator) tryParseRCASubmitToolCall(resp llm.ChatResponse, correlationID string) (result *katypes.InvestigationResult, ok bool) {
	for _, tc := range resp.ToolCalls {
		if tc.Name != SubmitResultToolName {
			continue
		}
		parsed, parseErr := inv.resultParser.Parse(tc.Arguments)
		if parseErr != nil {
			inv.logger.Error(parseErr, "RCA retry parse still failed",
				"correlation_id", correlationID)
			continue
		}
		inv.logger.Info("RCA retry succeeded",
			"correlation_id", correlationID)
		return parsed, true
	}

	if resp.Message.Content != "" {
		parsed, parseErr := inv.resultParser.Parse(resp.Message.Content)
		if parseErr == nil {
			inv.logger.Info("RCA retry succeeded from message content",
				"correlation_id", correlationID)
			return parsed, true
		}
	}

	return nil, false
}
