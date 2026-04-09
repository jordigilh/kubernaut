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

// Observer collects observations from concurrent evaluations and renders a verdict.
// Each Observer instance is scoped to a single investigation — a fresh Observer
// must be created per Investigate() call to avoid cross-request state leakage.
type Observer struct {
	evaluator    *Evaluator
	mu           sync.Mutex
	observations []Observation
	wg           sync.WaitGroup
	stepIdx      atomic.Int64
}

// NewObserver creates an Observer backed by the given evaluator.
func NewObserver(evaluator *Evaluator) *Observer {
	return &Observer{evaluator: evaluator}
}

// NextStepIndex returns the next monotonically increasing step index for this
// investigation. Safe for concurrent use.
func (o *Observer) NextStepIndex() int {
	return int(o.stepIdx.Add(1) - 1)
}

// SubmitAsync queues a step for asynchronous evaluation. The WaitGroup counter
// is incremented synchronously (before the goroutine launches) to prevent
// a race between Add and Wait.
func (o *Observer) SubmitAsync(ctx context.Context, step Step) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		obs := o.evaluator.EvaluateStep(ctx, step)
		o.mu.Lock()
		o.observations = append(o.observations, obs)
		o.mu.Unlock()
	}()
}

// WaitForCompletion blocks until all submitted evaluations finish or timeout expires.
// Returns all observations collected so far (even on timeout).
func (o *Observer) WaitForCompletion(timeout time.Duration) []Observation {
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}

	o.mu.Lock()
	result := make([]Observation, len(o.observations))
	copy(result, o.observations)
	o.mu.Unlock()
	return result
}

// RenderVerdict produces the final verdict from all collected observations.
func (o *Observer) RenderVerdict() Verdict {
	o.mu.Lock()
	obs := make([]Observation, len(o.observations))
	copy(obs, o.observations)
	o.mu.Unlock()

	flagged := 0
	var summaryParts []string
	for _, ob := range obs {
		if ob.Suspicious {
			flagged++
			summaryParts = append(summaryParts, fmt.Sprintf("step %d (%s): %s", ob.Step.Index, ob.Step.Tool, ob.Explanation))
		}
	}

	result := VerdictClean
	summary := "all steps passed alignment check"
	if flagged > 0 {
		result = VerdictSuspicious
		summary = strings.Join(summaryParts, "; ")
	}

	return Verdict{
		Result:       result,
		Summary:      summary,
		Observations: obs,
		Flagged:      flagged,
		Total:        len(obs),
	}
}
