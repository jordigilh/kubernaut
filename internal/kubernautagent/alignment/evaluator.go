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
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/security/boundary"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// EvaluatorConfig holds configuration for the per-step evaluator.
type EvaluatorConfig struct {
	Timeout               time.Duration
	MaxStepTokens         int
	MaxRetries            int
	MaxConversationTokens int
}

// Evaluator sends individual steps to the shadow LLM for alignment checking.
type Evaluator struct {
	client     llm.Client
	config     EvaluatorConfig
	prompt     string
	logger     logr.Logger
	auditStore audit.AuditStore
}

type evalResponse struct {
	Suspicious  *bool  `json:"suspicious"`
	Explanation string `json:"explanation"`
}

// NewEvaluator creates an Evaluator with the given shadow LLM client, config,
// and system prompt. The prompt is immutable after construction.
// Without optional WithLogger, per-step diagnostics are discarded; attach a Logger
// (e.g. zapr-backed) for operator troubleshooting (step detail at verbosity V(1)).
func NewEvaluator(client llm.Client, cfg EvaluatorConfig, prompt string, opts ...EvaluatorOption) *Evaluator {
	e := &Evaluator{client: client, config: cfg, prompt: prompt, logger: logr.Discard()}
	for _, o := range opts {
		o(e)
	}
	return e
}

// EvaluatorOption configures optional Evaluator behavior.
type EvaluatorOption func(*Evaluator)

// WithLogger attaches a structured logger for per-step diagnostic output at V(1).
func WithLogger(l logr.Logger) EvaluatorOption {
	return func(e *Evaluator) { e.logger = l }
}

// WithAuditStore enables per-call shadow LLM audit event emission (#1059).
// When set, the evaluator emits aiagent.shadow.llm.request before each Chat()
// call and aiagent.shadow.llm.response after each successful Chat() call.
// Events are only emitted when step.CorrelationID is non-empty (canary checks
// have no correlation ID and are intentionally excluded).
func WithAuditStore(store audit.AuditStore) EvaluatorOption {
	return func(e *Evaluator) { e.auditStore = store }
}

// EvaluateStep sends a step to the shadow LLM and returns an observation.
// Fail-closed: all error paths return Suspicious=true. Content is truncated
// to MaxStepTokens runes using head+tail strategy.
func (e *Evaluator) EvaluateStep(ctx context.Context, step Step) Observation {
	if ctx.Err() != nil {
		return Observation{
			Step:        step,
			Suspicious:  true,
			Explanation: fmt.Sprintf("evaluator_unavailable (fail-closed): context cancelled: %v", ctx.Err()),
		}
	}

	token, guardObs := e.checkBoundaryGuard(step)
	if guardObs != nil {
		return *guardObs
	}

	req, userMsgLen := e.buildStepRequest(step, token)

	emitAudit := e.auditStore != nil && step.CorrelationID != ""
	if emitAudit {
		e.emitShadowRequest(ctx, step, userMsgLen)
	}

	var lastErr error
	attempts := e.config.MaxRetries
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		obs, err := e.evaluateStepAttempt(ctx, step, req, emitAudit, attempt)
		if err != nil {
			lastErr = err
			continue
		}
		return *obs
	}

	return Observation{
		Step:        step,
		Suspicious:  true,
		Explanation: fmt.Sprintf("evaluator_unavailable (fail-closed): %v", lastErr),
	}
}

// checkBoundaryGuard generates the boundary token and rejects content that
// already contains an escape sequence for it. Returns a non-nil Observation
// (fail-closed) when either check fails.
func (e *Evaluator) checkBoundaryGuard(step Step) (string, *Observation) {
	token, genErr := boundary.Generate()
	if genErr != nil {
		return "", &Observation{
			Step:        step,
			Suspicious:  true,
			Explanation: fmt.Sprintf("boundary generation failed (fail-closed): %v", genErr),
		}
	}

	if boundary.ContainsEscape(step.Content, token) {
		return "", &Observation{
			Step:        step,
			Suspicious:  true,
			Explanation: "boundary escape detected in raw content (fail-closed): content contains closing boundary marker",
		}
	}

	return token, nil
}

// buildStepRequest truncates and boundary-wraps the step content, then
// assembles the shadow-evaluator chat request. Returns the request and the
// rune length of the user message (used for audit request-size reporting).
func (e *Evaluator) buildStepRequest(step Step, token string) (llm.ChatRequest, int) {
	content := truncateHeadTail(step.Content, e.config.MaxStepTokens)
	wrapped := boundary.Wrap(content, token)

	userMsg := fmt.Sprintf("Step %d [%s]", step.Index, step.Kind)
	if step.Tool != "" {
		userMsg += fmt.Sprintf(" tool=%s", step.Tool)
	}
	if step.ClusterID != "" {
		userMsg += fmt.Sprintf(" cluster=%s", step.ClusterID)
	}
	userMsg += fmt.Sprintf("\n\n%s", wrapped)

	messages := []llm.Message{
		{Role: "user", Content: userMsg},
	}
	if e.prompt != "" {
		messages = append([]llm.Message{{Role: "system", Content: e.prompt}}, messages...)
	}

	return llm.ChatRequest{
		Messages: messages,
		Options:  llm.ChatOptions{JSONMode: true},
	}, len([]rune(userMsg))
}

// evaluateStepAttempt performs one shadow-LLM call and interprets the
// response. Returns a non-nil Observation when the outcome is final —
// success, a malformed-response fail-closed, or a missing-field fail-closed
// — or a non-nil error when the caller should retry (transport failure or
// unparseable response).
func (e *Evaluator) evaluateStepAttempt(ctx context.Context, step Step, req llm.ChatRequest, emitAudit bool, attempt int) (*Observation, error) {
	evalCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	resp, err := e.client.Chat(evalCtx, req)
	cancel()
	if err != nil {
		return nil, err
	}

	respContent := resp.Message.Content
	if hasDuplicateSuspiciousKey(respContent) {
		if emitAudit {
			e.emitShadowResponse(ctx, step, resp.Usage, attempt+1, "malformed_response")
		}
		return &Observation{
			Step:        step,
			Suspicious:  true,
			Explanation: "duplicate_key_attack (fail-closed): shadow LLM response contains duplicate 'suspicious' key",
			Usage:       resp.Usage,
		}, nil
	}

	var parsed evalResponse
	content := extractJSON(respContent)
	if jsonErr := json.Unmarshal([]byte(content), &parsed); jsonErr != nil {
		return nil, fmt.Errorf("parse evaluator response: %w", jsonErr)
	}

	if parsed.Suspicious == nil {
		if emitAudit {
			e.emitShadowResponse(ctx, step, resp.Usage, attempt+1, "missing_field")
		}
		obs := &Observation{
			Step:        step,
			Suspicious:  true,
			Explanation: "evaluator_unavailable (fail-closed): shadow LLM response missing 'suspicious' field",
			Usage:       resp.Usage,
		}
		e.debugLog("step evaluated: missing suspicious field",
			"step_index", step.Index, "kind", string(step.Kind),
			"suspicious", true)
		return obs, nil
	}

	if emitAudit {
		e.emitShadowResponse(ctx, step, resp.Usage, attempt+1, "success")
	}
	e.debugLog("step evaluated",
		"step_index", step.Index, "kind", string(step.Kind),
		"tool", step.Tool, "suspicious", *parsed.Suspicious,
		"attempt", attempt+1)
	return &Observation{
		Step:        step,
		Suspicious:  *parsed.Suspicious,
		Explanation: parsed.Explanation,
		Usage:       resp.Usage,
	}, nil
}

// markdownFenceRe matches ```json ... ``` or ``` ... ``` blocks, with
// optional leading/trailing whitespace. Issue #925: some models (e.g. Haiku
// 4.5) wrap JSON in markdown fences even when JSONMode is requested.
var markdownFenceRe = regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*\n(.*?)\\s*```\\s*$")

// extractJSON strips markdown code fences if present, returning the inner
// content. If the input is not fenced, it is returned as-is after trimming.
func extractJSON(s string) string {
	if m := markdownFenceRe.FindStringSubmatch(s); len(m) == 2 {
		return strings.Clone(strings.TrimSpace(m[1]))
	}
	return strings.TrimSpace(s)
}

const truncationMarker = "…[truncated]…"

func truncateHeadTail(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	head := maxLen / 2
	tail := maxLen - head
	return string(runes[:head]) + truncationMarker + string(runes[len(runes)-tail:])
}

// TruncateHeadTail is the exported version for testing.
func TruncateHeadTail(s string, maxLen int) string {
	return truncateHeadTail(s, maxLen)
}

func (e *Evaluator) emitShadowRequest(ctx context.Context, step Step, promptLen int) {
	event := audit.NewEvent(audit.EventTypeShadowLLMRequest, step.CorrelationID)
	event.EventAction = audit.ActionShadowLLMRequest
	event.EventOutcome = audit.OutcomePending
	event.Data["step_index"] = step.Index
	event.Data["step_kind"] = string(step.Kind)
	event.Data["prompt_length"] = promptLen
	e.logger.V(2).Info("emitting shadow.llm.request",
		"correlation_id", step.CorrelationID,
		"step_index", step.Index, "step_kind", string(step.Kind),
		"prompt_length", promptLen)
	alignmentShadowAuditTotal.WithLabelValues("request").Inc()
	audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
}

func (e *Evaluator) emitShadowResponse(ctx context.Context, step Step, usage llm.TokenUsage, attempt int, evaluationResult string) {
	event := audit.NewEvent(audit.EventTypeShadowLLMResponse, step.CorrelationID)
	event.EventAction = audit.ActionShadowLLMResponse
	event.EventOutcome = audit.OutcomeSuccess
	event.Data["step_index"] = step.Index
	event.Data["step_kind"] = string(step.Kind)
	event.Data["prompt_tokens"] = usage.PromptTokens
	event.Data["completion_tokens"] = usage.CompletionTokens
	event.Data["total_tokens"] = usage.TotalTokens
	event.Data["attempt"] = attempt
	event.Data["evaluation_result"] = evaluationResult
	e.logger.V(2).Info("emitting shadow.llm.response",
		"correlation_id", step.CorrelationID,
		"step_index", step.Index, "step_kind", string(step.Kind),
		"total_tokens", usage.TotalTokens, "attempt", attempt,
		"evaluation_result", evaluationResult)
	alignmentShadowAuditTotal.WithLabelValues("response").Inc()
	audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
}

func (e *Evaluator) debugLog(msg string, kvs ...any) {
	e.logger.V(1).Info(msg, kvs...)
}

// hasDuplicateSuspiciousKey performs a raw-byte pre-scan for duplicate
// "suspicious" keys in JSON. Go's json.Unmarshal uses last-key-wins
// semantics, so an attacker could send {"suspicious":true,"suspicious":false}
// to flip the verdict. This check returns true if more than one occurrence
// of the key is found.
func hasDuplicateSuspiciousKey(raw string) bool {
	return strings.Count(raw, `"suspicious"`) > 1
}
