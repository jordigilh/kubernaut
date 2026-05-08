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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Performance hardening — PERF-1/PERF-4", func() {

	Describe("UT-PERF1-001: Observer semaphore bounds concurrent goroutines", func() {
		It("should complete all evaluations with bounded concurrency", func() {
			client := &mockLLMClient{responses: make([]llm.ChatResponse, 20)}
			for i := range client.responses {
				client.responses[i] = cleanResponse()
			}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator, "", 3)
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 20; i++ {
				observer.SubmitAsync(context.Background(), alignment.Step{
					Index: observer.NextStepIndex(), Kind: alignment.StepKindToolResult,
					Content: fmt.Sprintf("step %d", i),
				})
			}

			wr := observer.WaitForCompletion(30 * time.Second)
			Expect(wr.Complete).To(BeTrue())
			Expect(wr.Observations).To(HaveLen(20),
				"all 20 evaluations must complete despite bounded concurrency")
		})
	})

	Describe("UT-PERF4-001: Observation content is capped at MaxObservationContentLen", func() {
		It("should truncate stored content exceeding the cap", func() {
			longContent := strings.Repeat("x", alignment.MaxObservationContentLen+1000)
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 0, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator, "")
			Expect(err).NotTo(HaveOccurred())

			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: longContent,
			})

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Complete).To(BeTrue())
			Expect(wr.Observations).To(HaveLen(1))
			Expect(len(wr.Observations[0].Step.Content)).To(BeNumerically("<=",
				alignment.MaxObservationContentLen+len("...[capped]")),
				"stored content must be capped")
			Expect(wr.Observations[0].Step.Content).To(HaveSuffix("...[capped]"))
		})
	})

	Describe("UT-PERF4-002: Short observation content is not capped", func() {
		It("should not modify content within the cap", func() {
			shortContent := "normal tool output"
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator, "")
			Expect(err).NotTo(HaveOccurred())

			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Content: shortContent,
			})

			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Complete).To(BeTrue())
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Step.Content).To(Equal(shortContent))
		})
	})
})
