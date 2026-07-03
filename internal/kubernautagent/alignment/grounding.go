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

package alignment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

type groundingResponse struct {
	Grounded    *bool  `json:"grounded"`
	Explanation string `json:"explanation"`
}

// EvaluateGrounding sends an entire RCA conversation to the shadow LLM for
// full-context grounding review. It answers: "given the tool evidence, are the
// RCA conclusions well-grounded?"
//
// Fail-closed: all error paths return Grounded=false. The conversation is
// truncated to maxConversationTokens runes to prevent OOM.
func (e *Evaluator) EvaluateGrounding(ctx context.Context, conversation []llm.Message, correlationID string) GroundingObservation {
	start := time.Now()

	if len(conversation) == 0 {
		return GroundingObservation{
			Grounded:    false,
			Explanation: "grounding_review_failed (fail-closed): empty conversation",
			Duration:    time.Since(start),
		}
	}

	maxTokens := e.config.MaxConversationTokens
	if maxTokens <= 0 {
		maxTokens = e.config.MaxStepTokens * 50
	}
	conversationText := renderConversation(conversation, maxTokens)

	messages := []llm.Message{
		{Role: "user", Content: conversationText},
	}
	if e.prompt != "" {
		messages = append([]llm.Message{{Role: "system", Content: e.prompt}}, messages...)
	}

	req := llm.ChatRequest{
		Messages: messages,
		Options:  llm.ChatOptions{JSONMode: true},
	}

	emitAudit := e.auditStore != nil && correlationID != ""
	if emitAudit {
		e.emitGroundingRequest(ctx, correlationID, len(conversation), len([]rune(conversationText)))
	}

	evalCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	resp, err := e.client.Chat(evalCtx, req)
	cancel()

	if err != nil {
		obs := GroundingObservation{
			Grounded:    false,
			Explanation: fmt.Sprintf("grounding_review_failed (fail-closed): %v", err),
			Duration:    time.Since(start),
		}
		return e.finalizeGroundingObservation(ctx, obs, correlationID, emitAudit, "error")
	}

	if hasDuplicateGroundedKey(resp.Message.Content) {
		obs := GroundingObservation{
			Grounded:    false,
			Explanation: "duplicate_key_attack (fail-closed): shadow LLM response contains duplicate 'grounded' key",
			Usage:       resp.Usage,
			Duration:    time.Since(start),
		}
		return e.finalizeGroundingObservation(ctx, obs, correlationID, emitAudit, "malformed_response")
	}

	content := extractJSON(resp.Message.Content)
	var parsed groundingResponse
	if jsonErr := json.Unmarshal([]byte(content), &parsed); jsonErr != nil {
		obs := GroundingObservation{
			Grounded:    false,
			Explanation: fmt.Sprintf("grounding_review_failed (fail-closed): parse error: %v", jsonErr),
			Usage:       resp.Usage,
			Duration:    time.Since(start),
		}
		return e.finalizeGroundingObservation(ctx, obs, correlationID, emitAudit, "parse_error")
	}

	if parsed.Grounded == nil {
		obs := GroundingObservation{
			Grounded:    false,
			Explanation: "grounding_review_failed (fail-closed): shadow LLM response missing 'grounded' field",
			Usage:       resp.Usage,
			Duration:    time.Since(start),
		}
		return e.finalizeGroundingObservation(ctx, obs, correlationID, emitAudit, "missing_field")
	}

	obs := GroundingObservation{
		Grounded:    *parsed.Grounded,
		Explanation: parsed.Explanation,
		Usage:       resp.Usage,
		Duration:    time.Since(start),
	}
	result := "grounded"
	if !obs.Grounded {
		result = "ungrounded"
	}
	e.debugLog("grounding review completed",
		"correlation_id", correlationID,
		"grounded", obs.Grounded,
		"conversation_messages", len(conversation))
	return e.finalizeGroundingObservation(ctx, obs, correlationID, emitAudit, result)
}

// finalizeGroundingObservation emits the grounding-response audit event
// (when auditing is enabled for this call) and returns obs unchanged.
func (e *Evaluator) finalizeGroundingObservation(ctx context.Context, obs GroundingObservation, correlationID string, emitAudit bool, outcome string) GroundingObservation {
	if emitAudit {
		e.emitGroundingResponse(ctx, correlationID, obs, outcome)
	}
	return obs
}

// renderConversation flattens a multi-turn conversation into a single string
// for the grounding review prompt. Content is truncated to maxTokens runes.
func renderConversation(messages []llm.Message, maxTokens int) string {
	var b strings.Builder
	for i, msg := range messages {
		fmt.Fprintf(&b, "[%s] %s\n", msg.Role, msg.Content)
		if i < len(messages)-1 {
			b.WriteString("---\n")
		}
	}
	result := b.String()
	if maxTokens > 0 {
		runes := []rune(result)
		if len(runes) > maxTokens {
			result = string(runes[:maxTokens]) + truncationMarker
		}
	}
	return result
}

// hasDuplicateGroundedKey performs a raw-byte pre-scan for duplicate
// "grounded" keys in JSON. Mirrors hasDuplicateSuspiciousKey from evaluator.go.
func hasDuplicateGroundedKey(raw string) bool {
	return strings.Count(raw, `"grounded"`) > 1
}

func (e *Evaluator) emitGroundingRequest(ctx context.Context, correlationID string, conversationLen int, tokenEstimate int) {
	event := audit.NewEvent(audit.EventTypeGroundingRequest, correlationID)
	event.EventAction = audit.ActionGroundingRequest
	event.EventOutcome = audit.OutcomePending
	event.Data["conversation_length"] = conversationLen
	event.Data["conversation_tokens"] = tokenEstimate
	e.logger.V(2).Info("emitting alignment.grounding.request",
		"correlation_id", correlationID,
		"conversation_length", conversationLen,
		"conversation_tokens", tokenEstimate)
	audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
}

func (e *Evaluator) emitGroundingResponse(ctx context.Context, correlationID string, obs GroundingObservation, result string) {
	event := audit.NewEvent(audit.EventTypeGroundingResponse, correlationID)
	event.EventAction = audit.ActionGroundingResponse
	switch result {
	case "grounded":
		event.EventOutcome = audit.OutcomeSuccess
	default:
		event.EventOutcome = audit.OutcomeFailure
	}
	event.Data["grounded"] = obs.Grounded
	event.Data["duration_ms"] = obs.Duration.Milliseconds()
	event.Data["result"] = result
	event.Data["prompt_tokens"] = obs.Usage.PromptTokens
	event.Data["completion_tokens"] = obs.Usage.CompletionTokens
	event.Data["total_tokens"] = obs.Usage.TotalTokens
	e.logger.V(2).Info("emitting alignment.grounding.response",
		"correlation_id", correlationID,
		"grounded", obs.Grounded,
		"duration_ms", obs.Duration.Milliseconds(),
		"result", result)
	audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
}
