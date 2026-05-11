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

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Fix 7: AlignmentVerdict mapping on InvestigationResult", func() {

	var (
		signal katypes.SignalContext
	)

	BeforeEach(func() {
		signal = katypes.SignalContext{
			Name:          "test-alert",
			Namespace:     "default",
			Severity:      "critical",
			Message:       "pod is crashing",
			RemediationID: "rem-schema-001",
		}
	})

	// UT-SA-SCHEMA-001: Clean verdict -> AlignmentVerdict.Result == "aligned", Findings empty
	It("UT-SA-SCHEMA-001: clean verdict populates AlignmentVerdict with result=aligned and no findings", func() {
		shadowClient := &mockLLMClient{
			responses: []llm.ChatResponse{
				suspiciousResponse(), // canary (must pass)
				cleanResponse(),      // signal_input
			},
		}
		evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
			Timeout: 5 * time.Second, MaxRetries: 1,
		}, "")

		inner := &mockInvestigationRunner{
			result: &katypes.InvestigationResult{
				RCASummary: "OOM killed",
				Confidence: 0.9,
			},
		}

		wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:          inner,
			Evaluator:      evaluator,
			VerdictTimeout: 5 * time.Second,
			Logger:         logr.Discard(),
			Mode:           config.AlignmentModeMonitor,
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := wrapper.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		// This field does not exist yet — RED test must fail
		Expect(result.AlignmentVerdict).NotTo(BeNil(), "AlignmentVerdict must always be populated")
		Expect(result.AlignmentVerdict.Result).To(Equal(string(alignment.VerdictClean)))
		Expect(result.AlignmentVerdict.CircuitBreakerActivated).To(BeFalse())
		Expect(result.AlignmentVerdict.Findings).To(BeEmpty())
	})

	// UT-SA-SCHEMA-002: Suspicious verdict -> AlignmentVerdict has findings
	It("UT-SA-SCHEMA-002: suspicious verdict populates AlignmentVerdict with findings", func() {
		shadowClient := &mockLLMClient{
			responses: []llm.ChatResponse{
				suspiciousResponse(), // canary
				suspiciousResponse(), // signal_input — flagged
				suspiciousResponse(), // step 1 — flagged
			},
		}
		evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
			Timeout: 5 * time.Second, MaxRetries: 1,
		}, "")

		innerResult := &katypes.InvestigationResult{
			RCASummary: "suspicious RCA",
			Confidence: 0.5,
		}
		inner := &delayedInnerRunner{
			result:    innerResult,
			stepDelay: 50 * time.Millisecond,
			stepCount: 1,
		}

		wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:          inner,
			Evaluator:      evaluator,
			VerdictTimeout: 5 * time.Second,
			Logger:         logr.Discard(),
			Mode:           config.AlignmentModeMonitor,
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := wrapper.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		Expect(result.AlignmentVerdict).NotTo(BeNil(), "AlignmentVerdict must be populated")
		Expect(result.AlignmentVerdict.Result).To(Equal("suspicious"))
		Expect(result.AlignmentVerdict.Flagged).To(BeNumerically(">", 0))
		Expect(result.AlignmentVerdict.Total).To(BeNumerically(">", 0))
		Expect(len(result.AlignmentVerdict.Findings)).To(BeNumerically(">", 0))

		finding := result.AlignmentVerdict.Findings[0]
		Expect(finding.StepKind).NotTo(BeEmpty())
		Expect(finding.Explanation).NotTo(BeEmpty())
	})

	// UT-SA-SCHEMA-003: Circuit breaker -> CircuitBreakerActivated == true
	It("UT-SA-SCHEMA-003: circuit breaker sets CircuitBreakerActivated on AlignmentVerdict", func() {
		shadowClient := &mockLLMClient{
			responses: []llm.ChatResponse{
				suspiciousResponse(), // canary
				suspiciousResponse(), // signal_input — triggers circuit breaker
				suspiciousResponse(), // step 1 (may not be reached)
			},
		}
		evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
			Timeout: 5 * time.Second, MaxRetries: 1,
		}, "")

		inner := &delayedInnerRunner{
			result: &katypes.InvestigationResult{
				RCASummary: "should be circuit broken",
				Confidence: 0.3,
			},
			stepDelay: 200 * time.Millisecond,
			stepCount: 5,
		}

		wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:          inner,
			Evaluator:      evaluator,
			VerdictTimeout: 10 * time.Second,
			Logger:         logr.Discard(),
			Mode:           config.AlignmentModeEnforce,
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := wrapper.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		Expect(result.AlignmentVerdict).NotTo(BeNil())
		Expect(result.AlignmentVerdict.CircuitBreakerActivated).To(BeTrue(),
			"circuit breaker must set CircuitBreakerActivated=true")
		Expect(result.AlignmentVerdict.Result).To(Equal("suspicious"))
	})

	// UT-SA-SCHEMA-004: AlignmentVerdict is ALWAYS populated (for ALL investigations)
	It("UT-SA-SCHEMA-004: AlignmentVerdict is populated for ALL investigations including aligned ones", func() {
		shadowClient := &mockLLMClient{
			responses: []llm.ChatResponse{
				suspiciousResponse(), // canary (must pass)
				cleanResponse(),      // signal_input
			},
		}
		evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
			Timeout: 5 * time.Second, MaxRetries: 1,
		}, "")

		inner := &mockInvestigationRunner{
			result: &katypes.InvestigationResult{
				RCASummary: "Normal RCA",
				Confidence: 0.95,
				WorkflowID: "restart-pod",
			},
		}

		wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:          inner,
			Evaluator:      evaluator,
			VerdictTimeout: 5 * time.Second,
			Logger:         logr.Discard(),
			Mode:           config.AlignmentModeMonitor,
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := wrapper.Investigate(context.Background(), signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		Expect(result.AlignmentVerdict).NotTo(BeNil(),
			"AlignmentVerdict must be present for ALL investigations, not just suspicious ones")
		Expect(result.AlignmentVerdict.Result).To(Equal(string(alignment.VerdictClean)))
		Expect(result.AlignmentVerdict.Total).To(BeNumerically(">", 0),
			"Total should reflect the number of evaluated steps")
	})
})
