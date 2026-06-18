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

package aianalysis_test

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// spyEvaluator wraps a real rego.Evaluator to capture inputs for test
// assertions while delegating actual policy evaluation to the real engine.
// This satisfies the Kubernaut testing methodology: real business logic
// is executed (no output mocking), while the spy captures observability
// data analogous to structured telemetry.
type spyEvaluator struct {
	real      rego.EvaluatorInterface
	LastInput *rego.PolicyInput
	CallCount int
}

func (s *spyEvaluator) Evaluate(ctx context.Context, input *rego.PolicyInput) (*rego.PolicyResult, error) {
	s.CallCount++
	s.LastInput = input
	return s.real.Evaluate(ctx, input)
}

// newSpyEvaluator creates a spy wrapping a real evaluator loaded from the
// given policy path. The evaluator is fully initialized with StartHotReload.
func newSpyEvaluator(ctx context.Context, policyPath string) *spyEvaluator {
	eval := rego.NewEvaluator(rego.Config{PolicyPath: policyPath}, logr.Discard())
	if err := eval.StartHotReload(ctx); err != nil {
		panic("newSpyEvaluator: failed to start hot-reload for " + policyPath + ": " + err.Error())
	}
	return &spyEvaluator{real: eval}
}

// newDegradedSpyEvaluator creates a spy wrapping a real evaluator configured
// with a nonexistent policy path. This triggers the evaluator's graceful
// degradation path (BR-AI-014) without StartHotReload.
func newDegradedSpyEvaluator() *spyEvaluator {
	eval := rego.NewEvaluator(rego.Config{
		PolicyPath: "testdata/nonexistent_policy_for_degradation.rego",
	}, logr.Discard())
	return &spyEvaluator{real: eval}
}
