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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
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

	observerOpts := []ObserverOption{WithCorrelationID(correlationID)}

	// Circuit breaker: in enforce mode, create a cancellable investigation context.
	// When the shadow detects suspicious content, the onSuspicious callback cancels
	// investCtx, which causes the inner investigation to return context.Canceled.
	// Shadow evaluations use the parent ctx (via WithEvalContext) so they continue.
	var investCtx context.Context
	var investCancel context.CancelCauseFunc
	if w.mode == config.AlignmentModeEnforce {
		investCtx, investCancel = context.WithCancelCause(ctx)
		defer investCancel(nil)
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
		return nil, fmt.Errorf("alignment observer: %w", obsErr)
	}
	investCtx = WithObserver(investCtx, observer)

	if signalContent := BuildSignalInputContent(signal); signalContent != "" {
		observer.SubmitAsync(investCtx, Step{
			Index:   observer.NextStepIndex(),
			Kind:    StepKindSignalInput,
			Content: signalContent,
		})
	}

	result, err := w.inner.Investigate(investCtx, signal)

	// Check if the error is a circuit breaker cancellation.
	circuitBroken := err != nil && context.Cause(investCtx) == ErrCircuitBreaker
	if circuitBroken {
		if result == nil {
			result = &katypes.InvestigationResult{}
		}
		err = nil
	} else if err != nil {
		return result, err
	}

	wr := observer.WaitForCompletion(w.verdictTimeout)
	verdict := observer.RenderVerdict(wr)
	if circuitBroken {
		verdict.CircuitBreaker = true
	}

	w.emitAlignmentAudit(ctx, correlationID, verdict)

	alignmentVerdictDuration.Observe(time.Since(start).Seconds())
	alignmentVerdictTotal.WithLabelValues(string(verdict.Result), string(w.mode)).Inc()
	if canaryDegraded {
		alignmentCanaryTotal.WithLabelValues("fail").Inc()
	} else {
		alignmentCanaryTotal.WithLabelValues("pass").Inc()
	}

	escalateCanary := canaryDegraded && (w.mode == config.AlignmentModeEnforce || w.canaryForceEscalation)
	escalateVerdict := verdict.Result == VerdictSuspicious && w.mode == config.AlignmentModeEnforce

	if canaryDegraded {
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

	if verdict.Result == VerdictSuspicious {
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
	} else if !canaryDegraded {
		w.logger.Info("shadow agent alignment check passed",
			"signal", signal.Name,
			"namespace", signal.Namespace,
			"total", verdict.Total,
		)
	}

	return result, nil
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
