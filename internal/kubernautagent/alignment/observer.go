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
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

type observerContextKey struct{}

// WithObserver returns a child context carrying the given Observer.
func WithObserver(ctx context.Context, o *Observer) context.Context {
	return context.WithValue(ctx, observerContextKey{}, o)
}

// ObserverFromContext retrieves the Observer from context, or nil if absent.
func ObserverFromContext(ctx context.Context) *Observer {
	o, _ := ctx.Value(observerContextKey{}).(*Observer)
	return o
}

// DefaultMaxConcurrentEvals limits goroutines spawned per investigation.
const DefaultMaxConcurrentEvals = 10

// MaxObservationContentLen caps the stored content in observations to limit memory.
const MaxObservationContentLen = 4096

// Observer collects observations from concurrent evaluations and renders a verdict.
// Each Observer instance is scoped to a single investigation — a fresh Observer
// must be created per Investigate() call to avoid cross-request state leakage.
//
// ARCH-3: evalCtx is stored in the struct to decouple shadow evaluation context
// from the investigation context. When the circuit breaker cancels investCtx,
// shadow evaluations must continue using the parent context so WaitForCompletion
// can collect results.
type Observer struct {
	evaluator        *Evaluator
	correlationID    string
	logger           logr.Logger
	mu               sync.Mutex
	observations     []Observation
	wg               sync.WaitGroup
	stepIdx          atomic.Int64
	sem              chan struct{}
	evalCtx          context.Context
	onSuspicious     func()
	suspiciousOnce   sync.Once
	groundingEnabled bool
	groundingWg      sync.WaitGroup
	groundingMu      sync.Mutex
	groundingObs     *GroundingObservation
}

// ObserverOption configures optional Observer behaviour.
type ObserverOption func(*Observer)

// WithCorrelationID stamps every step submitted via SubmitAsync with the given
// correlation ID so the evaluator can emit audit events tied to the incident.
func WithCorrelationID(id string) ObserverOption {
	return func(o *Observer) { o.correlationID = id }
}

// WithMaxConcurrent overrides the default goroutine concurrency limit.
func WithMaxConcurrent(n int) ObserverOption {
	return func(o *Observer) {
		if n > 0 {
			o.sem = make(chan struct{}, n)
		}
	}
}

// WithEvalContext sets the context used for EvaluateStep calls inside SubmitAsync.
// This decouples shadow evaluation from the investigation context (ARCH-3),
// ensuring shadow evaluations continue even when the circuit breaker cancels
// the investigation context.
func WithEvalContext(ctx context.Context) ObserverOption {
	return func(o *Observer) { o.evalCtx = ctx }
}

// WithOnSuspicious registers a callback invoked (at most once) when any
// evaluation returns Suspicious=true. Used by the circuit breaker to cancel
// the investigation context. The callback is guarded by sync.Once and runs
// inside a recover() block that logs panics with stack traces.
func WithOnSuspicious(fn func()) ObserverOption {
	return func(o *Observer) { o.onSuspicious = fn }
}

// WithObserverLogger sets the logger used by the Observer for panic recovery
// and diagnostic logging.
func WithObserverLogger(l logr.Logger) ObserverOption {
	return func(o *Observer) { o.logger = l }
}

// WithGroundingEnabled enables or disables the full-context grounding review.
// When disabled, StartGroundingReview is a no-op.
func WithGroundingEnabled(enabled bool) ObserverOption {
	return func(o *Observer) { o.groundingEnabled = enabled }
}

// NewObserver creates an Observer backed by the given evaluator.
// Returns an error if evaluator is nil to prevent nil deref in SubmitAsync.
func NewObserver(evaluator *Evaluator, opts ...ObserverOption) (*Observer, error) {
	if evaluator == nil {
		return nil, fmt.Errorf("alignment.NewObserver: evaluator must not be nil")
	}
	obs := &Observer{evaluator: evaluator, sem: make(chan struct{}, DefaultMaxConcurrentEvals)}
	for _, opt := range opts {
		opt(obs)
	}
	return obs, nil
}

// NextStepIndex returns the next monotonically increasing step index for this
// investigation. Safe for concurrent use.
func (o *Observer) NextStepIndex() int {
	return int(o.stepIdx.Add(1) - 1)
}

// SubmitAsync queues a step for asynchronous evaluation. The WaitGroup counter
// is incremented synchronously (before the goroutine launches) to prevent
// a race between Add and Wait.
//
// Panics inside EvaluateStep (e.g. crypto/rand failure in boundary.Generate)
// are recovered and converted to fail-closed observations to prevent a single
// evaluation failure from crashing the entire KA process.
func (o *Observer) SubmitAsync(ctx context.Context, step Step) {
	step.CorrelationID = o.correlationID

	// Use evalCtx for shadow evaluation if set (ARCH-3), otherwise use caller ctx.
	evalCtx := ctx
	if o.evalCtx != nil {
		evalCtx = o.evalCtx
	}

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()

		o.sem <- struct{}{}
		defer func() { <-o.sem }()

		var obs Observation
		func() {
			defer func() {
				if r := recover(); r != nil {
					obs = Observation{
						Step:        step,
						Suspicious:  true,
						Explanation: fmt.Sprintf("evaluator_panic (fail-closed): %v", r),
					}
				}
			}()
			obs = o.evaluator.EvaluateStep(evalCtx, step)
		}()

		if len(obs.Step.Content) > MaxObservationContentLen {
			obs.Step.Content = obs.Step.Content[:MaxObservationContentLen] + "...[capped]"
		}

		switch {
		case strings.Contains(obs.Explanation, "evaluator_panic"):
			alignmentStepTotal.WithLabelValues("panic").Inc()
		case obs.Suspicious:
			alignmentStepTotal.WithLabelValues("suspicious").Inc()
		default:
			alignmentStepTotal.WithLabelValues("aligned").Inc()
		}

		o.mu.Lock()
		o.observations = append(o.observations, obs)
		o.mu.Unlock()

		if obs.Suspicious && o.onSuspicious != nil {
			o.suspiciousOnce.Do(func() {
				defer func() {
					if r := recover(); r != nil {
						o.logger.Error(
							fmt.Errorf("onSuspicious callback panic: %v", r),
							"panic in circuit breaker callback",
							"stack", string(debug.Stack()),
						)
					}
				}()
				o.onSuspicious()
			})
		}
	}()
}

// StartGroundingReview launches an asynchronous full-context grounding review
// of the RCA conversation. The review runs in a separate goroutine and its
// result is incorporated into WaitForCompletion/RenderVerdict.
//
// Fail-closed: nil or empty messages produce an ungrounded observation
// immediately without calling the LLM. When grounding is disabled
// (WithGroundingEnabled(false)), this is a no-op.
func (o *Observer) StartGroundingReview(messages []llm.Message) {
	if !o.groundingEnabled {
		alignmentGroundingTotal.WithLabelValues("disabled").Inc()
		return
	}

	if len(messages) == 0 {
		failObs := &GroundingObservation{
			Grounded:    false,
			Explanation: "grounding_review_failed (fail-closed): nil or empty conversation",
		}
		o.groundingMu.Lock()
		o.groundingObs = failObs
		o.groundingMu.Unlock()
		alignmentGroundingTotal.WithLabelValues("error").Inc()
		return
	}

	evalCtx := context.Background()
	if o.evalCtx != nil {
		evalCtx = o.evalCtx
	}

	o.groundingWg.Add(1)
	go func() {
		defer o.groundingWg.Done()
		obs := o.evaluator.EvaluateGrounding(evalCtx, messages, o.correlationID)

		o.groundingMu.Lock()
		o.groundingObs = &obs
		o.groundingMu.Unlock()

		switch {
		case !obs.Grounded && strings.Contains(obs.Explanation, "fail-closed"):
			alignmentGroundingTotal.WithLabelValues("error").Inc()
		case obs.Grounded:
			alignmentGroundingTotal.WithLabelValues("grounded").Inc()
		default:
			alignmentGroundingTotal.WithLabelValues("ungrounded").Inc()
		}
		alignmentGroundingDuration.Observe(obs.Duration.Seconds())

		if !obs.Grounded && o.onSuspicious != nil {
			o.suspiciousOnce.Do(func() {
				defer func() {
					if r := recover(); r != nil {
						o.logger.Error(
							fmt.Errorf("onSuspicious callback panic: %v", r),
							"panic in circuit breaker callback (grounding)",
							"stack", string(debug.Stack()),
						)
					}
				}()
				o.onSuspicious()
			})
		}
	}()
}

// WaitForCompletion blocks until all submitted evaluations finish or timeout expires.
// Returns a WaitResult snapshot capturing the state at that moment.
//
// Known limitation (SEC-10/PERF-5): the waiter goroutine (wg.Wait) is not
// cancelled when timeout fires. It self-heals within config.Timeout (default
// 10s) as all pending evaluations carry a per-step context.WithTimeout. A
// clean fix (Observer-scoped context cancellation) would change NewObserver's
// signature, impacting ~15 call sites — deferred to a follow-up PR.
func (o *Observer) WaitForCompletion(timeout time.Duration) WaitResult {
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		o.groundingWg.Wait()
		close(done)
	}()

	complete := false
	select {
	case <-done:
		complete = true
	case <-time.After(timeout):
	}

	submitted := int(o.stepIdx.Load())

	o.mu.Lock()
	obs := make([]Observation, len(o.observations))
	copy(obs, o.observations)
	o.mu.Unlock()

	o.groundingMu.Lock()
	groundingObs := o.groundingObs
	o.groundingMu.Unlock()

	if !complete && o.groundingEnabled && groundingObs == nil {
		groundingObs = &GroundingObservation{
			Grounded:    false,
			Explanation: "grounding_review_timeout (fail-closed): grounding review did not complete within verdict timeout",
		}
		alignmentGroundingTotal.WithLabelValues("timeout").Inc()
	}

	return WaitResult{
		Complete:             complete,
		Submitted:            submitted,
		Observations:         obs,
		Pending:              submitted - len(obs),
		GroundingObservation: groundingObs,
	}
}

// RenderVerdict produces the final verdict from a WaitResult snapshot.
// Fail-closed: any pending steps (submitted but not completed) are treated as suspicious.
// An ungrounded grounding review also forces a Suspicious verdict.
func (o *Observer) RenderVerdict(wr WaitResult) Verdict {
	obs := wr.Observations
	flagged := 0
	var summaryParts []string
	for _, ob := range obs {
		if ob.Suspicious {
			flagged++
			label := ob.Step.Tool
			if label == "" {
				label = string(ob.Step.Kind)
			}
			summaryParts = append(summaryParts, fmt.Sprintf("step %d (%s): %s",
				ob.Step.Index, label, SanitizeExplanation(ob.Explanation)))
		}
	}

	timedOut := !wr.Complete
	pending := wr.Pending

	groundingFailed := false
	if wr.GroundingObservation != nil && !wr.GroundingObservation.Grounded {
		groundingFailed = true
		summaryParts = append(summaryParts, fmt.Sprintf("grounding_review: %s",
			SanitizeExplanation(wr.GroundingObservation.Explanation)))
	}

	result := VerdictClean
	summary := "all steps passed alignment check"

	if pending > 0 {
		result = VerdictSuspicious
		summaryParts = append(summaryParts, fmt.Sprintf("verdict_timeout: %d pending evaluations (fail-closed)", pending))
		summary = strings.Join(summaryParts, "; ")
	} else if flagged > 0 || groundingFailed {
		result = VerdictSuspicious
		summary = strings.Join(summaryParts, "; ")
	}

	return Verdict{
		Result:       result,
		Summary:      summary,
		Observations: obs,
		Flagged:      flagged,
		Total:        len(obs),
		Pending:      pending,
		TimedOut:     timedOut,
	}
}
