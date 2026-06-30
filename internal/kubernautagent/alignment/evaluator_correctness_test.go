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

	Describe("UT-SA-601-CX-001: NewObserver returns error on nil evaluator", func() {
		It("should return an error when passed nil evaluator", func() {
			_, err := alignment.NewObserver(nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("evaluator must not be nil"))
		})
	})

	Describe("UT-SA-601-CX-002: NewInvestigatorWrapper returns error on nil inner/evaluator", func() {
		It("should return error when Inner is nil", func() {
			_, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:     nil,
				Evaluator: &alignment.Evaluator{},
				Logger:    logr.Discard(),
			})
			Expect(err).To(HaveOccurred())
		})

		It("should return error when Evaluator is nil", func() {
			_, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:     &mockInvestigationRunner{},
				Evaluator: nil,
				Logger:    logr.Discard(),
			})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-SA-601-CX-003: Duplicate key detection in tool result", func() {
		It("should flag duplicate JSON keys as suspicious", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{suspiciousResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
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

	Describe("UT-SA-601-FLEET-001: Cluster-origin metadata in shadow evaluation", func() {
		It("should include cluster= in evaluator prompt for fleet tools", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithLogger(logr.Discard()))

			step := alignment.Step{
				Index:     0,
				Kind:      alignment.StepKindToolResult,
				Tool:      "prod-east__resources_get",
				ClusterID: "prod-east",
				Content:   `{"kind":"Pod","metadata":{"name":"nginx"}}`,
			}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeFalse())

			Expect(client.capturedRequestContents).To(HaveLen(1))
			Expect(client.capturedRequestContents[0]).To(ContainSubstring("cluster=prod-east"))
			Expect(client.capturedRequestContents[0]).To(ContainSubstring("tool=prod-east__resources_get"))
		})

		It("should not include cluster= for local tools", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "", alignment.WithLogger(logr.Discard()))

			step := alignment.Step{
				Index:   0,
				Kind:    alignment.StepKindToolResult,
				Tool:    "get_pods",
				Content: `{"kind":"Pod"}`,
			}
			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeFalse())

			Expect(client.capturedRequestContents).To(HaveLen(1))
			Expect(client.capturedRequestContents[0]).NotTo(ContainSubstring("cluster="))
		})
	})

	Describe("UT-SA-601-FLEET-002: SubmitToolStep extracts cluster ID from tool name", func() {
		It("should populate ClusterID for fleet tool names", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
			ctx := alignment.WithObserver(context.Background(), observer)

			alignment.SubmitToolStep(ctx, "staging__resources_list", `{"items":[]}`)
			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Step.ClusterID).To(Equal("staging"))
			Expect(wr.Observations[0].Step.Tool).To(Equal("staging__resources_list"))
		})

		It("should leave ClusterID empty for local tool names", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())
			ctx := alignment.WithObserver(context.Background(), observer)

			alignment.SubmitToolStep(ctx, "get_pods", `{"items":[]}`)
			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Step.ClusterID).To(BeEmpty())
		})
	})
})
