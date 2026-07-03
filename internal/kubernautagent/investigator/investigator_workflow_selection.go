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
	"errors"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

func (inv *Investigator) runWorkflowSelection(ctx context.Context, signal katypes.SignalContext, rcaSummary string, enrichData *prompt.EnrichmentData, p1Ctx *prompt.Phase1Data, llmCtx LLMInvocationContext) (result *katypes.InvestigationResult, retErr error) {
	correlationID := llmCtx.CorrelationID
	// Apply signal label overrides (target_resource_kind / target_resource_name)
	// before attaching to context. This ensures workflow discovery tools
	// (list_available_actions, list_workflows) filter by the correct component.
	// Defense-in-depth for #1064/#1065: even if enrichment resolved a container
	// kind (e.g. Namespace), the label override corrects it for tool context.
	overriddenSignal := ApplySignalLabelOverrides(signal)
	ctx = katypes.WithSignalContext(ctx, overriddenSignal)
	inv.logger.Info("runWorkflowSelection: post-override signal",
		"component_gvk", overriddenSignal.ComponentGVK(),
		"resource_kind", overriddenSignal.ResourceKind,
		"resource_api_version", overriddenSignal.ResourceAPIVersion,
		"correlation_id", correlationID)

	wfPromptSignal := SignalToPrompt(signal)
	LogLabelOverrideOrRejection(inv.logger, signal, wfPromptSignal, correlationID, "workflow selection")
	systemPrompt, err := inv.builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
		Signal:     wfPromptSignal,
		RCASummary: rcaSummary,
		EnrichData: enrichData,
		Phase1:     p1Ctx,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering workflow selection prompt: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("RCA findings: %s\n\nSelect the appropriate remediation workflow.", rcaSummary)},
	}

	loopRes, err := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, llmCtx)
	if err != nil {
		return nil, err
	}

	content, early, err := inv.handleWorkflowSelectionLoopResult(ctx, loopRes, rcaSummary, messages, llmCtx)
	if early != nil || err != nil {
		return early, err
	}

	result, parseErr := inv.resultParser.Parse(content)
	if parseErr != nil {
		return inv.workflowSelectionRetryOrHumanReview(ctx, content, messages, rcaSummary, llmCtx, parseErr,
			fmt.Sprintf("workflow selection: LLM did not produce parseable result: %s", parseErr)), nil
	}

	if inv.pipeline.CatalogFetcher == nil {
		return result, nil
	}
	return inv.selfCorrectWorkflowSelection(ctx, result, content, messages, rcaSummary, correlationID, llmCtx)
}

// handleWorkflowSelectionLoopResult classifies the runLLMLoop outcome for
// PhaseWorkflowDiscovery. When early is non-nil, the caller must return it
// (and err) immediately without further parsing. Otherwise content holds the
// tool-call (or text) payload to parse into an InvestigationResult.
func (inv *Investigator) handleWorkflowSelectionLoopResult(ctx context.Context, loopRes LoopResult, rcaSummary string, messages []llm.Message, llmCtx LLMInvocationContext) (content string, early *katypes.InvestigationResult, err error) {
	correlationID := llmCtx.CorrelationID
	switch r := loopRes.(type) {
	case *CancelledResult:
		cancelledResult := &katypes.InvestigationResult{
			RCASummary:          rcaSummary,
			Cancelled:           true,
			CancelledPhase:      string(katypes.PhaseWorkflowDiscovery),
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
		return "", cancelledResult, nil
	case *ExhaustedResult:
		return "", &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			Reason:            fmt.Sprintf("%s during workflow selection (maxTurns=%d)", r.Reason, inv.maxTurns),
		}, nil
	case *SubmitNoWorkflowResult:
		inv.logger.Info("submit_result_no_workflow sentinel: classifying as no_matching_workflows",
			"correlation_id", correlationID)
		return "", &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			HumanReviewReason: "no_matching_workflows",
			Reason:            "LLM explicitly declined workflow selection via submit_result_no_workflow",
		}, nil
	case *SubmitWithWorkflowResult:
		return r.Content, nil, nil
	case *SubmitResult:
		return r.Content, nil, nil
	case *TextResult:
		// #760 v2: LLM returned text instead of a tool call. Try parsing
		// first — the text may contain a valid investigation result (e.g.
		// problem_resolved or predictive_no_action where no workflow is
		// expected). Only fall through to parse-level retry when the
		// content cannot be parsed at all.
		if _, textErr := inv.resultParser.Parse(r.Content); textErr == nil {
			inv.logger.Info("workflow selection: parsed text response directly (no tool call)",
				"correlation_id", correlationID)
			return r.Content, nil, nil
		}
		return "", inv.workflowSelectionRetryOrHumanReview(ctx, r.Content, messages, rcaSummary, llmCtx, nil,
			"workflow selection: LLM did not use submit tool after retries"), nil
	}
	return "", nil, nil
}

// workflowSelectionRetryOrHumanReview performs one retryWorkflowSubmit
// attempt and, on exhaustion, classifies the investigation as
// no_matching_workflows (or cancelled, if the context ended). Shared by the
// "LLM returned unparseable text" and "LLM's tool payload failed structural
// parsing" branches of runWorkflowSelection. When parseErr is non-nil the
// exhaustion is logged at Error level (a real parse failure); otherwise at
// Info level (the LLM simply never called a submit tool).
func (inv *Investigator) workflowSelectionRetryOrHumanReview(ctx context.Context, lastContent string, messages []llm.Message, rcaSummary string, llmCtx LLMInvocationContext, parseErr error, failureReason string) *katypes.InvestigationResult {
	retryResult := inv.retryWorkflowSubmit(ctx, lastContent, messages, rcaSummary, llmCtx)
	if retryResult != nil {
		return retryResult
	}
	if ctx.Err() != nil {
		return &katypes.InvestigationResult{
			RCASummary:     rcaSummary,
			Cancelled:      true,
			CancelledPhase: string(katypes.PhaseWorkflowDiscovery),
		}
	}
	if parseErr != nil {
		inv.logger.Error(parseErr, "workflow selection parse failed after retries, classifying as no_matching_workflows",
			"correlation_id", llmCtx.CorrelationID)
	} else {
		inv.logger.Info("workflow selection: all retries exhausted, classifying as no_matching_workflows",
			"correlation_id", llmCtx.CorrelationID)
	}
	return &katypes.InvestigationResult{
		RCASummary:        rcaSummary,
		HumanReviewNeeded: true,
		HumanReviewReason: "no_matching_workflows",
		Reason:            failureReason,
	}
}

// selfCorrectWorkflowSelection runs the catalog-validated self-correction
// loop (up to maxSelfCorrectionAttempts) for the parsed workflow-selection
// result, then applies catalog enrichment and target-alignment checks to the
// final accepted (or exhausted) result.
func (inv *Investigator) selfCorrectWorkflowSelection(ctx context.Context, result *katypes.InvestigationResult, content string, messages []llm.Message, rcaSummary, correlationID string, llmCtx LLMInvocationContext) (*katypes.InvestigationResult, error) {
	validator, fetchErr := inv.pipeline.CatalogFetcher.FetchValidator(ctx)
	if fetchErr != nil {
		inv.logger.Error(fetchErr, "workflow catalog unavailable, requiring human review")
		result.HumanReviewNeeded = true
		result.HumanReviewReason = "catalog_unavailable"
		result.Reason = fmt.Sprintf("workflow catalog unavailable: %s", fetchErr)
		return result, nil
	}

	attempt := 0
	correctionFn := func(r *katypes.InvestigationResult, validationErr error) (*katypes.InvestigationResult, error) {
		attempt++
		var errStrs []string
		if validationErr != nil {
			errStrs = []string{validationErr.Error()}
		}
		inv.emitValidationEvent(ctx, attempt, maxSelfCorrectionAttempts, false, errStrs, r.WorkflowID, correlationID)

		correctionMsg, renderErr := inv.renderCorrectionMessage(validationErr, attempt, maxSelfCorrectionAttempts)
		if renderErr != nil {
			inv.logger.Error(renderErr, "failed to render validation error template, using fallback")
			correctionMsg = fmt.Sprintf("Validation failed: %s. Please select a valid workflow.", validationErr)
		}
		messages = append(messages, llm.Message{Role: "assistant", Content: content})
		messages = append(messages, llm.Message{Role: "user", Content: correctionMsg})

		corrLoopRes, corrErr := inv.runLLMLoop(ctx, messages, katypes.PhaseWorkflowDiscovery, llmCtx)
		if corrErr != nil {
			return nil, corrErr
		}
		switch cr := corrLoopRes.(type) {
		case *CancelledResult:
			return nil, context.Canceled
		case *ExhaustedResult:
			r.HumanReviewNeeded = true
			r.Reason = fmt.Sprintf("self-correction: %s", cr.Reason)
			return r, nil
		case *SubmitNoWorkflowResult:
			return &katypes.InvestigationResult{
				RCASummary:        rcaSummary,
				HumanReviewNeeded: true,
				HumanReviewReason: "no_matching_workflows",
				Reason:            "LLM declined workflow during self-correction via submit_result_no_workflow",
			}, nil
		case *SubmitWithWorkflowResult:
			content = cr.Content
		case *SubmitResult:
			content = cr.Content
		case *TextResult:
			content = cr.Content
		}
		return inv.resultParser.Parse(content)
	}

	corrected, corrErr := validator.SelfCorrect(result, maxSelfCorrectionAttempts, correctionFn)
	if corrErr != nil {
		if errors.Is(corrErr, context.Canceled) || errors.Is(corrErr, context.DeadlineExceeded) {
			return &katypes.InvestigationResult{
				RCASummary:     rcaSummary,
				Cancelled:      true,
				CancelledPhase: string(katypes.PhaseWorkflowDiscovery),
			}, nil
		}
		return nil, fmt.Errorf("validation self-correction failed: %w", corrErr)
	}
	isValid := !corrected.HumanReviewNeeded
	var finalErrors []string
	if !isValid {
		finalErrors = []string{"validation exhausted all attempts"}
	}
	inv.emitValidationEvent(ctx, attempt+1, maxSelfCorrectionAttempts, isValid, finalErrors, corrected.WorkflowID, correlationID)
	enrichFromCatalog(corrected, validator)
	CheckWorkflowTargetAlignment(ctx, corrected, validator, correlationID, inv.auditStore, inv.logger)
	return corrected, nil
}

const maxParseRetries = 2

// retryWorkflowSubmit performs up to maxParseRetries correction attempts when
// the LLM returns text or unparseable JSON instead of calling a submit tool.
// Each retry sends a correction message with examples of both submit tools,
// with only the two submit tools available (prevents re-investigation).
// Returns non-nil *InvestigationResult on success or nil when retries exhaust.
func (inv *Investigator) retryWorkflowSubmit(ctx context.Context, lastContent string, history []llm.Message, rcaSummary string, llmCtx LLMInvocationContext) *katypes.InvestigationResult {
	tokens, correlationID, client, modelName, runtimeParams :=
		llmCtx.Tokens, llmCtx.CorrelationID, llmCtx.Client, llmCtx.ModelName, llmCtx.RuntimeParams
	submitOnlyTools := []llm.ToolDefinition{
		{
			Name:        SubmitResultWithWorkflowToolName,
			Description: "Submit investigation result WITH a selected workflow.",
			Parameters:  parser.WithWorkflowResultSchema(),
		},
		{
			Name:        SubmitResultNoWorkflowToolName,
			Description: "Submit investigation result when NO matching workflow exists.",
			Parameters:  parser.NoWorkflowResultSchema(),
		},
	}

	correctionTemplate := `Your response could not be parsed. You MUST call one of these tools:

1. If a workflow matches: call submit_result_with_workflow with JSON like:
   {"root_cause_analysis":{"summary":"..."},"selected_workflow":{"workflow_id":"...","confidence":0.9},"confidence":0.9}

2. If NO workflow matches: call submit_result_no_workflow with JSON like:
   {"root_cause_analysis":{"summary":"..."},"reasoning":"explanation why no workflow applies"}

Do NOT respond with plain text. You MUST call one of the above tools.`

	retryMessages := make([]llm.Message, len(history))
	copy(retryMessages, history)
	retryMessages = append(retryMessages,
		llm.Message{Role: "assistant", Content: lastContent},
	)

	for attempt := 0; attempt < maxParseRetries; attempt++ {
		if ctx.Err() != nil {
			return nil
		}
		inv.logger.Info("parse-level retry for workflow submit",
			"attempt", attempt+1,
			"max", maxParseRetries,
			"correlation_id", correlationID)

		retryMessages = append(retryMessages,
			llm.Message{Role: "user", Content: correctionTemplate},
		)

		inv.emitRetryAudit(ctx, retryAuditParams{
			correlationID: correlationID,
			modelName:     modelName,
			messages:      retryMessages,
			attempt:       attempt + 1,
			maxAttempts:   maxParseRetries,
			phase:         katypes.PhaseWorkflowDiscovery,
			retryReason:   "parse_level_correction",
		})

		resp, err := inv.chatOrStream(ctx, client, llm.ChatRequest{
			Messages: retryMessages,
			Tools:    submitOnlyTools,
			Options:  llm.ChatOptions{JSONMode: true, OutputSchema: parser.InvestigationResultSchema()},
		}, attempt+1, string(katypes.PhaseWorkflowDiscovery), modelName, runtimeParams)
		if err != nil {
			inv.logger.Error(err, "retry LLM call failed",
				"correlation_id", correlationID)
			continue
		}
		if tokens != nil {
			tokens.Add(resp.Usage)
		}

		emitToSink(ctx, session.EventTypeReasoningDelta, attempt+1, string(katypes.PhaseWorkflowDiscovery), map[string]interface{}{
			"content":       resp.Message.Content,
			"retry_attempt": attempt + 1,
		})

		if len(resp.ToolCalls) > 0 {
			for _, tc := range resp.ToolCalls {
				result, matched := inv.classifyWorkflowSubmitToolCall(tc, rcaSummary, correlationID)
				if !matched {
					continue
				}
				if result == nil {
					retryMessages = append(retryMessages, resp.Message)
					continue
				}
				return result
			}
		}

		retryMessages = append(retryMessages, resp.Message)
	}
	return nil
}

// classifyWorkflowSubmitToolCall inspects a single tool call from a
// retryWorkflowSubmit LLM response for one of the two submit-tool names.
// Returns matched=true when tc.Name recognized a submit tool; result is nil
// in that case only when SubmitResultWithWorkflowToolName's arguments failed
// to parse, signaling the caller to append resp.Message and keep retrying.
// Returns matched=false when tc.Name did not match either submit tool.
func (inv *Investigator) classifyWorkflowSubmitToolCall(tc llm.ToolCall, rcaSummary, correlationID string) (result *katypes.InvestigationResult, matched bool) {
	switch tc.Name {
	case SubmitResultNoWorkflowToolName:
		inv.logger.Info("retry succeeded: submit_result_no_workflow",
			"correlation_id", correlationID)
		return &katypes.InvestigationResult{
			RCASummary:        rcaSummary,
			HumanReviewNeeded: true,
			HumanReviewReason: "no_matching_workflows",
			Reason:            "LLM used submit_result_no_workflow after retry",
		}, true
	case SubmitResultWithWorkflowToolName:
		inv.logger.Info("retry succeeded: submit_result_with_workflow",
			"correlation_id", correlationID)
		parsed, parseErr := inv.resultParser.Parse(tc.Arguments)
		if parseErr != nil {
			inv.logger.Error(parseErr, "retry submit_result_with_workflow parse failed",
				"correlation_id", correlationID)
			return nil, true
		}
		return parsed, true
	}
	return nil, false
}
