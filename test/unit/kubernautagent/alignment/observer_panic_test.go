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

package alignment_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
)

var _ = Describe("Panic recovery in SubmitAsync — SEC-6", func() {

	Describe("UT-SEC6-001: SubmitAsync recovers from EvaluateStep panic", func() {
		It("should record a fail-closed observation without crashing the process", func() {
			panicClient := &panicMockLLMClient{}
			evaluator := alignment.NewEvaluator(panicClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
			ctx := context.Background()

			step := alignment.Step{Index: 0, Kind: alignment.StepKindToolResult, Tool: "get_pods", Content: "data"}
			observer.SubmitAsync(ctx, step)

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Complete).To(BeTrue(), "observer must complete even after panic")
			Expect(wr.Observations).To(HaveLen(1), "panic must produce a fail-closed observation")
			Expect(wr.Observations[0].Suspicious).To(BeTrue(), "panic must be fail-closed (suspicious)")
			Expect(wr.Observations[0].Explanation).To(ContainSubstring("panic"),
				"explanation must indicate panic recovery")
		})
	})

	Describe("UT-SEC6-002: Multiple SubmitAsync with one panic does not affect others", func() {
		It("should handle mix of panicking and normal evaluations correctly", func() {
			panicClient := &panicMockLLMClient{}
			evaluator := alignment.NewEvaluator(panicClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
			ctx := context.Background()

			observer.SubmitAsync(ctx, alignment.Step{Index: 0, Kind: alignment.StepKindToolResult, Content: "a"})
			observer.SubmitAsync(ctx, alignment.Step{Index: 1, Kind: alignment.StepKindToolResult, Content: "b"})

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Complete).To(BeTrue())
			Expect(wr.Observations).To(HaveLen(2), "both observations must be recorded")
			for _, obs := range wr.Observations {
				Expect(obs.Suspicious).To(BeTrue(), "all panicked evaluations must be fail-closed")
			}
		})
	})
})
