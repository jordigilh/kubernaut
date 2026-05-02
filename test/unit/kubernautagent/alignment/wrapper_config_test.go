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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("AlignmentCheck mode and config — PROD-1/PROD-2", func() {

	Describe("UT-PROD1-001: DefaultConfig sets mode=enforce", func() {
		It("should default to enforce mode", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.AI.AlignmentCheck.Mode).To(Equal(config.AlignmentModeEnforce))
		})
	})

	Describe("UT-PROD1-002: Validate rejects invalid mode", func() {
		It("should reject unknown mode strings", func() {
			cfg := config.DefaultConfig()
			cfg.AI.AlignmentCheck.Enabled = true
			cfg.AI.AlignmentCheck.Mode = "invalid"
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mode"))
		})
	})

	Describe("UT-PROD1-003: Monitor mode logs but does not escalate", func() {
		It("should not set HumanReviewNeeded when mode=monitor and verdict is suspicious", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "rca", Confidence: 0.9, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponse(), // canary (pass)
				suspiciousResponse(), // signal step → suspicious
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeFalse(),
				"monitor mode must NOT escalate to human review")
		})
	})

	Describe("UT-PROD1-004: Canary failure in monitor mode with forceEscalation=true still escalates", func() {
		It("should force HumanReviewNeeded when canary fails even in monitor mode", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "rca", Confidence: 0.9, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				cleanResponse(), // canary → clean (FAIL)
				cleanResponse(), // signal step → clean
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:                 inner,
				Evaluator:             evaluator,
				VerdictTimeout:        5 * time.Second,
				Logger:                logr.Discard(),
				Mode:                  config.AlignmentModeMonitor,
				CanaryForceEscalation: true,
			})

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeTrue(),
				"canary failure with forceEscalation=true must override monitor mode")
		})
	})

	Describe("UT-PROD1-005: Canary failure in monitor mode with forceEscalation=false does not escalate", func() {
		It("should NOT force HumanReviewNeeded when canary fails and forceEscalation=false in monitor mode", func() {
			innerRes := &katypes.InvestigationResult{
				RCASummary: "rca", Confidence: 0.9, HumanReviewNeeded: false,
			}
			inner := &mockInvestigationRunner{result: innerRes}
			client := &mockLLMClient{responses: []llm.ChatResponse{
				cleanResponse(), // canary → clean (FAIL)
				cleanResponse(), // signal step → clean
			}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			wrapper := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:                 inner,
				Evaluator:             evaluator,
				VerdictTimeout:        5 * time.Second,
				Logger:                logr.Discard(),
				Mode:                  config.AlignmentModeMonitor,
				CanaryForceEscalation: false,
			})

			sig := katypes.SignalContext{Name: "s", Namespace: "ns", Severity: "high", Message: "m"}
			res, err := wrapper.Investigate(context.Background(), sig)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.HumanReviewNeeded).To(BeFalse(),
				"canary failure with forceEscalation=false in monitor mode should NOT escalate")
		})
	})

	Describe("UT-PROD2-001: DefaultConfig includes maxRetries and verdictTimeout", func() {
		It("should provide sensible defaults for alignment config fields", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.AI.AlignmentCheck.MaxRetries).To(Equal(1))
			Expect(cfg.AI.AlignmentCheck.VerdictTimeout).To(Equal(30 * time.Second))
			Expect(cfg.AI.AlignmentCheck.Canary.ForceEscalation).To(BeTrue(),
				"canary forceEscalation should default to true (safe default)")
		})
	})
})
