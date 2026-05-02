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
	"errors"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Canary integrity mechanism — SEC-3", func() {

	Describe("UT-SEC3-001: RunCanary returns Passed=true when shadow flags canary payload", func() {
		It("should pass when evaluator correctly identifies the canonical malicious payload", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{suspiciousResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			result := alignment.RunCanary(context.Background(), evaluator)
			Expect(result.Passed).To(BeTrue(), "canary must pass when shadow flags known-malicious content")
			Expect(result.Suspicious).To(BeTrue())
			Expect(client.chatCalls()).To(Equal(1), "canary must issue exactly one shadow LLM call")
		})
	})

	Describe("UT-SEC3-002: RunCanary returns Passed=false when shadow misses canary payload", func() {
		It("should fail when evaluator returns clean for known-malicious content (compromised shadow)", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			result := alignment.RunCanary(context.Background(), evaluator)
			Expect(result.Passed).To(BeFalse(), "canary must fail when shadow returns clean for known-malicious")
			Expect(result.Suspicious).To(BeFalse())
		})
	})

	Describe("UT-SEC3-003: RunCanary returns Passed=true on evaluator error (fail-closed)", func() {
		It("should pass because fail-closed returns Suspicious=true which matches canary expectation", func() {
			client := &mockLLMClient{errs: []error{errors.New("shadow unavailable")}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			result := alignment.RunCanary(context.Background(), evaluator)
			Expect(result.Passed).To(BeTrue(),
				"fail-closed returns Suspicious=true, which is the expected canary outcome")
		})
	})

	Describe("UT-SEC3-004: RunCanary uses canonical payload with _canary tool and index -1", func() {
		It("should send a step with index=-1, tool=_canary, and CanaryPayload content", func() {
			client := &mockLLMClient{responses: []llm.ChatResponse{suspiciousResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
			}, "")

			result := alignment.RunCanary(context.Background(), evaluator)
			Expect(result.Passed).To(BeTrue())
			Expect(result.Step.Index).To(Equal(-1))
			Expect(result.Step.Tool).To(Equal("_canary"))
			Expect(result.Step.Kind).To(Equal(alignment.StepKindToolResult))
			Expect(result.Step.Content).To(Equal(alignment.CanaryPayload))
		})
	})

	Describe("UT-SEC3-005: Wrapper degrades investigation when canary fails", func() {
		It("should force HumanReviewNeeded=true when canary detects compromised shadow", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "valid rca", Confidence: 0.95, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}

			// Shadow always returns clean -- compromised model
			client := &mockLLMClient{responses: []llm.ChatResponse{
				cleanResponse(), // canary call → clean (FAIL: shadow didn't flag malicious)
				cleanResponse(), // signal step → clean
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				Logger:         logr.Discard(),
			})

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeTrue(),
				"canary failure must force HumanReviewNeeded regardless of shadow verdict")
			Expect(res.Warnings).To(ContainElement(ContainSubstring("canary")),
				"warnings must mention canary failure")
		})
	})

	Describe("UT-SEC3-006: Wrapper operates normally when canary passes", func() {
		It("should not interfere with clean investigation when canary passes", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "valid rca", Confidence: 0.95, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}

			client := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(), // canary call → suspicious (PASS)
				cleanResponse(),      // signal step → clean
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				Logger:         logr.Discard(),
			})

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeFalse(),
				"passing canary + clean verdict should not trigger human review")
		})
	})

	Describe("UT-SEC3-007: Wrapper canary timeout degrades (fail-closed)", func() {
		It("should pass canary (fail-closed returns Suspicious=true) when shadow times out", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "rca", Confidence: 0.9, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}

			// Slow client that exceeds the evaluator timeout
			slowClient := &slowMockLLMClient{delay: 10 * time.Second}
			evaluator := alignment.NewEvaluator(slowClient, alignment.EvaluatorConfig{
				Timeout: 100 * time.Millisecond, MaxRetries: 1,
			}, "")
			wrapper := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				Logger:         logr.Discard(),
			})

			sig := katypes.SignalContext{Name: "s", Namespace: "ns"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).ToNot(HaveOccurred())
			// Timeout → fail-closed → Suspicious=true → canary passes
			// But all subsequent shadow calls also timeout → fail-closed → all suspicious
			// → HumanReviewNeeded=true from regular verdict, not canary degradation
			Expect(res.HumanReviewNeeded).To(BeTrue(),
				"shadow timeout causes fail-closed on both canary and steps")
		})
	})
})
