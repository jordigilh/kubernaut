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
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// InvestigatorWrapper wraps an InvestigationRunner. On each Investigate call
// it creates a fresh Observer (scoped to THIS investigation), injects it into
// the context, delegates to the inner runner, then collects and applies the verdict.
type InvestigatorWrapper struct {
	inner                 kaserver.InvestigationRunner
	evaluator             *Evaluator
	verdictTimeout        time.Duration
	auditStore            audit.AuditStore
	logger                logr.Logger
	mode                  config.AlignmentMode
	canaryForceEscalation bool
	groundingEnabled      bool
}

var _ kaserver.InvestigationRunner = (*InvestigatorWrapper)(nil)

// InvestigatorWrapperConfig holds construction parameters for InvestigatorWrapper.
type InvestigatorWrapperConfig struct {
	Inner                 kaserver.InvestigationRunner
	Evaluator             *Evaluator
	VerdictTimeout        time.Duration
	AuditStore            audit.AuditStore
	Logger                logr.Logger
	Mode                  config.AlignmentMode
	CanaryForceEscalation bool
	GroundingEnabled      bool
}

// NewInvestigatorWrapper creates an InvestigatorWrapper.
// Returns an error if Inner or Evaluator is nil to prevent nil deref during Investigate.
func NewInvestigatorWrapper(cfg InvestigatorWrapperConfig) (*InvestigatorWrapper, error) {
	if cfg.Inner == nil {
		return nil, fmt.Errorf("alignment.NewInvestigatorWrapper: Inner must not be nil")
	}
	if cfg.Evaluator == nil {
		return nil, fmt.Errorf("alignment.NewInvestigatorWrapper: Evaluator must not be nil")
	}
	logger := cfg.Logger
	var zero logr.Logger
	if logger == zero {
		logger = logr.Discard()
	}
	mode := cfg.Mode
	if mode == "" {
		mode = config.AlignmentModeEnforce
	}
	return &InvestigatorWrapper{
		inner:                 cfg.Inner,
		evaluator:             cfg.Evaluator,
		verdictTimeout:        cfg.VerdictTimeout,
		auditStore:            cfg.AuditStore,
		logger:                logger,
		mode:                  mode,
		canaryForceEscalation: cfg.CanaryForceEscalation,
		groundingEnabled:      cfg.GroundingEnabled,
	}, nil
}

// Investigate runs a canary integrity check, creates a per-request Observer,
// injects it into the context, delegates to the inner runner, waits for shadow
// observations, then applies the verdict.
//
// Canary: before each investigation, a known-malicious payload is sent to the
// shadow evaluator. If the shadow fails to flag it (canary failure), the
// investigation result is degraded to HumanReviewNeeded regardless of the
// shadow verdict. This detects compromised or misconfigured shadow models.
//
// Fail-closed: on timeout, pending evaluations, or evaluator unavailability,
// the result is escalated to human review.
//
// The signal context (error message, severity, resource identity) is submitted
// to the shadow as step 0 before delegation. This ensures injection-like content
// in incident fields (e.g. ErrorMessage) is evaluated even if the primary LLM
// does not echo it in its response — matching the BR-AI-601 intent that ALL
// content entering the investigation pipeline is subject to alignment checks.
func (w *InvestigatorWrapper) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	start := time.Now()

	canary := RunCanary(ctx, w.evaluator)
	canaryDegraded := !canary.Passed
	if canaryDegraded {
		w.logger.Info("shadow agent canary failed: shadow model did not flag known-malicious content",
			"signal", signal.Name,
			"namespace", signal.Namespace,
			"explanation", canary.Explanation,
		)
	}

	correlationID := signalCorrelationID(signal)

	investCtx, investCancel, observer, obsErr := w.setupObservedContext(ctx, signal, correlationID)
	if investCancel != nil {
		defer investCancel(nil)
	}
	if obsErr != nil {
		return nil, fmt.Errorf("alignment observer: %w", obsErr)
	}

	result, circuitBroken, err := w.runInnerInvestigation(investCtx, signal)
	if err != nil && !circuitBroken {
		return result, err
	}

	wr := observer.WaitForCompletion(w.verdictTimeout)
	verdict := observer.RenderVerdict(wr)
	if circuitBroken {
		verdict.CircuitBreaker = true
	}

	result.AlignmentVerdict = mapVerdictToResult(verdict)

	w.emitVerdictEvent(ctx, result.AlignmentVerdict)
	w.emitAlignmentAudit(ctx, correlationID, verdict)

	alignmentVerdictDuration.Observe(time.Since(start).Seconds())
	alignmentVerdictTotal.WithLabelValues(string(verdict.Result), string(w.mode)).Inc()
	if canaryDegraded {
		alignmentCanaryTotal.WithLabelValues("fail").Inc()
	} else {
		alignmentCanaryTotal.WithLabelValues("pass").Inc()
	}

	w.escalateOnCanaryDegradation(result, signal, canaryDegraded)
	w.escalateOnSuspiciousVerdict(result, signal, verdict, circuitBroken, canaryDegraded)

	return result, nil
}

// setupObservedContext builds the alignment observer for this investigation
// and, in enforce mode, wraps ctx in a cancellable investigation context
// whose onSuspicious callback trips the circuit breaker. Shadow evaluations
// always use the parent ctx (via WithEvalContext) so they continue even
// after the investigation context is cancelled. The returned cancel func is
// nil outside enforce mode (nothing to cancel).
func (w *InvestigatorWrapper) setupObservedContext(ctx context.Context, signal katypes.SignalContext, correlationID string) (context.Context, context.CancelCauseFunc, *Observer, error) {
	observerOpts := []ObserverOption{
		WithCorrelationID(correlationID),
		WithObserverLogger(w.logger),
		WithGroundingEnabled(w.groundingEnabled),
	}

	var investCtx context.Context
	var investCancel context.CancelCauseFunc
	if w.mode == config.AlignmentModeEnforce {
		investCtx, investCancel = context.WithCancelCause(ctx)
		observerOpts = append(observerOpts,
			WithEvalContext(ctx),
			WithOnSuspicious(func() {
				investCancel(ErrCircuitBreaker)
				alignmentCircuitBreakerTotal.WithLabelValues(string(w.mode)).Inc()
				w.logger.Info("circuit_breaker_triggered",
					"correlationID", correlationID,
					"mode", string(w.mode),
				)
			}),
		)
	} else {
		investCtx = ctx
	}

	observer, obsErr := NewObserver(w.evaluator, observerOpts...)
	if obsErr != nil {
		return nil, investCancel, nil, obsErr
	}
	investCtx = WithObserver(investCtx, observer)

	if signalContent := BuildSignalInputContent(signal); signalContent != "" {
		observer.SubmitAsync(investCtx, Step{
			Index:   observer.NextStepIndex(),
			Kind:    StepKindSignalInput,
			Content: signalContent,
		})
	}

	return investCtx, investCancel, observer, nil
}

// runInnerInvestigation delegates to the wrapped investigator and detects
// whether the returned error is a circuit-breaker cancellation (in which
// case a non-nil placeholder result is guaranteed so the caller can still
// attach an alignment verdict) versus a genuine investigation failure.
func (w *InvestigatorWrapper) runInnerInvestigation(investCtx context.Context, signal katypes.SignalContext) (result *katypes.InvestigationResult, circuitBroken bool, err error) {
	result, err = w.inner.Investigate(investCtx, signal)
	circuitBroken = err != nil && errors.Is(context.Cause(investCtx), ErrCircuitBreaker)
	if circuitBroken && result == nil {
		result = &katypes.InvestigationResult{}
	}
	return result, circuitBroken, err
}

// escalateOnCanaryDegradation forces human review when the shadow-agent
// canary integrity check failed and the current mode/config calls for
// escalation, and logs the degradation either way.
func (w *InvestigatorWrapper) escalateOnCanaryDegradation(result *katypes.InvestigationResult, signal katypes.SignalContext, canaryDegraded bool) {
	if !canaryDegraded {
		return
	}
	escalateCanary := w.mode == config.AlignmentModeEnforce || w.canaryForceEscalation
	if escalateCanary {
		result.HumanReviewNeeded = true
		result.HumanReviewReason = "alignment_check_failed"
		result.Warnings = append(result.Warnings,
			"Shadow agent canary integrity check failed: shadow model may be compromised — forcing human review")
	}
	w.logger.Info("canary degradation",
		"signal", signal.Name,
		"namespace", signal.Namespace,
		"escalated", escalateCanary,
		"mode", string(w.mode),
	)
}

// escalateOnSuspiciousVerdict forces human review when the shadow agent
// flagged suspicious content in enforce mode, and logs the outcome
// (suspicious-flagged, or passed when the canary was healthy).
func (w *InvestigatorWrapper) escalateOnSuspiciousVerdict(result *katypes.InvestigationResult, signal katypes.SignalContext, verdict Verdict, circuitBroken, canaryDegraded bool) {
	if verdict.Result != VerdictSuspicious {
		if !canaryDegraded {
			w.logger.Info("shadow agent alignment check passed",
				"signal", signal.Name,
				"namespace", signal.Namespace,
				"total", verdict.Total,
			)
		}
		return
	}

	escalateVerdict := w.mode == config.AlignmentModeEnforce
	if escalateVerdict {
		result.HumanReviewNeeded = true
		result.HumanReviewReason = "alignment_check_failed"
		if circuitBroken {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Shadow agent circuit breaker activated: %s", verdict.Summary))
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Shadow agent alignment check flagged suspicious content: %s", verdict.Summary))
		}
	}
	w.logger.Info("shadow agent flagged suspicious content",
		"signal", signal.Name,
		"namespace", signal.Namespace,
		"flagged", verdict.Flagged,
		"total", verdict.Total,
		"pending", verdict.Pending,
		"timed_out", verdict.TimedOut,
		"summary", verdict.Summary,
		"escalated", escalateVerdict,
		"circuit_breaker", circuitBroken,
		"mode", string(w.mode),
	)
}

// emitVerdictEvent sends the alignment verdict onto the session event channel.
// Uses the parent ctx (not investCtx) because the LazySink is attached to the
// parent context in Manager.launchInvestigation. This also ensures the emit
// succeeds even when the circuit breaker has cancelled investCtx.
func (w *InvestigatorWrapper) emitVerdictEvent(ctx context.Context, avr *katypes.AlignmentVerdictResult) {
	sink := session.EventSinkFromContext(ctx)
	if sink == nil || avr == nil {
		return
	}
	data, err := json.Marshal(avr)
	if err != nil {
		w.logger.Error(err, "failed to marshal AlignmentVerdictResult for event emission")
		return
	}
	evt := session.InvestigationEvent{
		Type: session.EventTypeAlignmentVerdict,
		Data: data,
	}
	select {
	case sink <- evt:
	default:
		w.logger.V(1).Info("alignment_verdict event dropped (channel full)")
	}
}

func (w *InvestigatorWrapper) emitAlignmentAudit(ctx context.Context, correlationID string, verdict Verdict) {
	if w.auditStore == nil {
		return
	}

	for _, obs := range verdict.Observations {
		if obs.Suspicious {
			event := audit.NewEvent(audit.EventTypeAlignmentStep, correlationID)
			event.EventAction = audit.ActionAlignmentEvaluate
			event.EventOutcome = audit.OutcomeFailure
			event.Data["step_index"] = obs.Step.Index
			event.Data["step_kind"] = string(obs.Step.Kind)
			event.Data["tool"] = obs.Step.Tool
			event.Data["explanation"] = SanitizeExplanation(obs.Explanation)
			audit.StoreBestEffort(ctx, w.auditStore, event, w.logger)
		}
	}

	event := audit.NewEvent(audit.EventTypeAlignmentVerdict, correlationID)
	event.EventAction = audit.ActionAlignmentVerdict
	if verdict.Result == VerdictSuspicious {
		event.EventOutcome = audit.OutcomeFailure
	} else {
		event.EventOutcome = audit.OutcomeSuccess
	}
	event.Data["result"] = string(verdict.Result)
	event.Data["summary"] = verdict.Summary
	event.Data["flagged"] = verdict.Flagged
	event.Data["total"] = verdict.Total
	if verdict.CircuitBreaker {
		event.Data["circuit_breaker"] = true
	}
	if verdict.GroundingReview != nil {
		event.Data["grounding_grounded"] = verdict.GroundingReview.Grounded
		event.Data["grounding_explanation"] = SanitizeExplanation(verdict.GroundingReview.Explanation)
	}

	var shadowPrompt, shadowCompletion, shadowTotal int
	for _, obs := range verdict.Observations {
		shadowPrompt += obs.Usage.PromptTokens
		shadowCompletion += obs.Usage.CompletionTokens
		shadowTotal += obs.Usage.TotalTokens
	}
	if shadowTotal > 0 {
		event.Data["shadow_prompt_tokens"] = shadowPrompt
		event.Data["shadow_completion_tokens"] = shadowCompletion
		event.Data["shadow_total_tokens"] = shadowTotal
	}

	audit.StoreBestEffort(ctx, w.auditStore, event, w.logger)
}

// signalCorrelationID derives the audit correlation ID from a signal context.
// Prefers RemediationID (stable RR name) with fallback to signal Name.
func signalCorrelationID(signal katypes.SignalContext) string {
	if signal.RemediationID != "" {
		return signal.RemediationID
	}
	return signal.Name
}

// mapVerdictToResult converts an internal Verdict to the API-facing AlignmentVerdictResult.
// Populated for ALL investigations so consumers always see the alignment status.
func mapVerdictToResult(verdict Verdict) *katypes.AlignmentVerdictResult {
	avr := &katypes.AlignmentVerdictResult{
		Result:                  string(verdict.Result),
		CircuitBreakerActivated: verdict.CircuitBreaker,
		Summary:                 verdict.Summary,
		Flagged:                 verdict.Flagged,
		Total:                   verdict.Total,
	}
	for _, obs := range verdict.Observations {
		if obs.Suspicious {
			avr.Findings = append(avr.Findings, katypes.AlignmentFinding{
				StepIndex:   obs.Step.Index,
				StepKind:    string(obs.Step.Kind),
				Tool:        obs.Step.Tool,
				Explanation: SanitizeExplanation(obs.Explanation),
			})
		}
	}
	if verdict.GroundingReview != nil {
		avr.GroundingReview = &katypes.AlignmentGroundingResult{
			Grounded:    verdict.GroundingReview.Grounded,
			Explanation: SanitizeExplanation(verdict.GroundingReview.Explanation),
		}
	}
	return avr
}

// BuildSignalInputContent assembles the signal fields that enter the primary
// LLM as system/user prompt content. Mirrors the user message format from
// investigator.runRCA so the shadow evaluates the same text the model sees.
func BuildSignalInputContent(signal katypes.SignalContext) string {
	if signal.Message == "" && signal.Name == "" {
		return ""
	}
	return fmt.Sprintf("Investigate: %s %s in %s — %s",
		signal.Severity, signal.Name, signal.Namespace, signal.Message)
}
