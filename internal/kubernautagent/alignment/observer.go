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
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
type Observer struct {
	evaluator     *Evaluator
	correlationID string
	mu            sync.Mutex
	observations  []Observation
	wg            sync.WaitGroup
	stepIdx       atomic.Int64
	sem           chan struct{}
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
			obs = o.evaluator.EvaluateStep(ctx, step)
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
			alignmentStepTotal.WithLabelValues("clean").Inc()
		}

		o.mu.Lock()
		o.observations = append(o.observations, obs)
		o.mu.Unlock()
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

	return WaitResult{
		Complete:     complete,
		Submitted:    submitted,
		Observations: obs,
		Pending:      submitted - len(obs),
	}
}

// RenderVerdict produces the final verdict from a WaitResult snapshot.
// Fail-closed: any pending steps (submitted but not completed) are treated as suspicious.
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

	result := VerdictClean
	summary := "all steps passed alignment check"

	if pending > 0 {
		result = VerdictSuspicious
		summaryParts = append(summaryParts, fmt.Sprintf("verdict_timeout: %d pending evaluations (fail-closed)", pending))
		summary = strings.Join(summaryParts, "; ")
	} else if flagged > 0 {
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
