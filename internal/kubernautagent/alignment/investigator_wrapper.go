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
	"log/slog"
	"time"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

// InvestigatorWrapper wraps an InvestigationRunner. On each Investigate call
// it creates a fresh Observer (scoped to THIS investigation), injects it into
// the context, delegates to the inner runner, then collects and applies the verdict.
type InvestigatorWrapper struct {
	inner                 kaserver.InvestigationRunner
	evaluator             *Evaluator
	verdictTimeout        time.Duration
	auditStore            audit.AuditStore
	logger                *slog.Logger
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
	Logger                *slog.Logger
	Mode                  config.AlignmentMode
	CanaryForceEscalation bool
}

// NewInvestigatorWrapper creates an InvestigatorWrapper.
// Panics if Inner or Evaluator is nil to prevent nil deref during Investigate.
func NewInvestigatorWrapper(cfg InvestigatorWrapperConfig) *InvestigatorWrapper {
	if cfg.Inner == nil {
		panic("alignment.NewInvestigatorWrapper: Inner must not be nil")
	}
	if cfg.Evaluator == nil {
		panic("alignment.NewInvestigatorWrapper: Evaluator must not be nil")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
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
	}
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
		w.logger.Warn("shadow agent canary failed: shadow model did not flag known-malicious content",
			slog.String("signal", signal.Name),
			slog.String("namespace", signal.Namespace),
			slog.String("explanation", canary.Explanation),
		)
	}

	observer := NewObserver(w.evaluator)
	ctx = WithObserver(ctx, observer)

	if signalContent := buildSignalInputContent(signal); signalContent != "" {
		observer.SubmitAsync(ctx, Step{
			Index:   observer.NextStepIndex(),
			Kind:    StepKindSignalInput,
			Content: signalContent,
		})
	}

	result, err := w.inner.Investigate(ctx, signal)
	if err != nil {
		return result, err
	}

	wr := observer.WaitForCompletion(w.verdictTimeout)
	verdict := observer.RenderVerdict(wr)

	w.emitAlignmentAudit(ctx, signal, verdict)

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
		w.logger.Warn("canary degradation",
			slog.String("signal", signal.Name),
			slog.String("namespace", signal.Namespace),
			slog.Bool("escalated", escalateCanary),
			slog.String("mode", string(w.mode)),
		)
	}

	if verdict.Result == VerdictSuspicious {
		if escalateVerdict {
			result.HumanReviewNeeded = true
			result.HumanReviewReason = "alignment_check_failed"
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Shadow agent alignment check flagged suspicious content: %s", verdict.Summary))
		}
		w.logger.Warn("shadow agent flagged suspicious content",
			slog.String("signal", signal.Name),
			slog.String("namespace", signal.Namespace),
			slog.Int("flagged", verdict.Flagged),
			slog.Int("total", verdict.Total),
			slog.Int("pending", verdict.Pending),
			slog.Bool("timed_out", verdict.TimedOut),
			slog.String("summary", verdict.Summary),
			slog.Bool("escalated", escalateVerdict),
			slog.String("mode", string(w.mode)),
		)
	} else if !canaryDegraded {
		w.logger.Info("shadow agent alignment check passed",
			slog.String("signal", signal.Name),
			slog.String("namespace", signal.Namespace),
			slog.Int("total", verdict.Total),
		)
	}

	return result, nil
}

func (w *InvestigatorWrapper) emitAlignmentAudit(ctx context.Context, signal katypes.SignalContext, verdict Verdict) {
	if w.auditStore == nil {
		return
	}

	correlationID := signal.RemediationID

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
	audit.StoreBestEffort(ctx, w.auditStore, event, w.logger)
}

// buildSignalInputContent assembles the signal fields that enter the primary
// LLM as system/user prompt content. Mirrors the user message format from
// investigator.runRCA so the shadow evaluates the same text the model sees.
func buildSignalInputContent(signal katypes.SignalContext) string {
	if signal.Message == "" && signal.Name == "" {
		return ""
	}
	return fmt.Sprintf("Investigate: %s %s in %s — %s",
		signal.Severity, signal.Name, signal.Namespace, signal.Message)
}
