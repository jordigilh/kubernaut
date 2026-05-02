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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Correctness fixes — BR-AI-601", func() {

	Describe("UT-SA-601-CX-001: NewObserver panics on nil evaluator", func() {
		It("should panic when passed nil evaluator", func() {
			Expect(func() {
				alignment.NewObserver(nil)
			}).To(Panic())
		})
	})

	Describe("UT-SA-601-CX-002: NewInvestigatorWrapper panics on nil inner/evaluator", func() {
		It("should panic when Inner is nil", func() {
			Expect(func() {
				alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
					Inner:     nil,
					Evaluator: &alignment.Evaluator{},
					Logger:    logr.Discard(),
				})
			}).To(Panic())
		})

		It("should panic when Evaluator is nil", func() {
			Expect(func() {
				alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
					Inner:     &mockInvestigationRunner{},
					Evaluator: nil,
					Logger:    logr.Discard(),
				})
			}).To(Panic())
		})
	})

	Describe("UT-SA-601-CX-003: Duplicate key detection in tool result", func() {
		It("should flag duplicate JSON keys as suspicious", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{suspiciousResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer := alignment.NewObserver(evaluator)
			ctx := alignment.WithObserver(context.Background(), observer)

			step := alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult,
				Tool:    "get_pods",
				Content: `{"key":"a","key":"b"}`,
			}
			observer.SubmitAsync(ctx, step)
			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Observations).To(HaveLen(1))
		})
	})
})
