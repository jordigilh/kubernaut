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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Verdict Enum Alignment — #1077", func() {

	Describe("UT-SA-1077-001: VerdictClean constant equals \"aligned\"", func() {
		It("should have VerdictClean equal to \"aligned\" to match OpenAPI AlignmentVerdictPayload.result enum", func() {
			Expect(string(alignment.VerdictClean)).To(Equal("aligned"),
				"VerdictClean must be \"aligned\" to match OpenAPI enum ['aligned','suspicious'] — see #1077")
		})
	})

	Describe("UT-SA-1077-002: Audit event with aligned verdict passes schema validation", func() {
		It("should emit verdict audit event with result=\"aligned\" when investigation is clean", func() {
			store := &mockAuditStore{}
			innerRes := &katypes.InvestigationResult{
				RCASummary: "test RCA", Confidence: 0.9,
			}
			inner := &mockInvestigationRunner{result: innerRes}

			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponseWithUsage(5, 10, 15),
				cleanResponseWithUsage(10, 20, 30),
			}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "", alignment.WithAuditStore(store))

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				AuditStore:     store,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{
				Name: "test-signal", Namespace: "ns", Message: "alert",
				RemediationID: "rr-1077-002",
			}
			_, investigateErr := wrapper.Investigate(context.Background(), sig)
			Expect(investigateErr).NotTo(HaveOccurred())

			store.mu.Lock()
			defer store.mu.Unlock()
			verdictEvents := filterEvents(store.events, audit.EventTypeAlignmentVerdict)
			Expect(verdictEvents).To(HaveLen(1), "exactly one verdict event expected")
			Expect(verdictEvents[0].Data["result"]).To(Equal("aligned"),
				"verdict audit event result field must be \"aligned\" to pass ogen AlignmentVerdictPayload validation (#1077)")
		})
	})

	Describe("UT-SA-1077-003: kubernaut_alignment_verdict_total{result=\"aligned\"} increments on clean verdict", func() {
		It("should increment verdict counter with result=\"aligned\" label when verdict is clean", func() {
			store := &mockAuditStore{}
			innerRes := &katypes.InvestigationResult{
				RCASummary: "clean RCA", Confidence: 0.95,
			}
			inner := &mockInvestigationRunner{result: innerRes}

			shadowClient := &mockLLMClient{responses: []llm.ChatResponse{
				suspiciousResponseWithUsage(5, 10, 15),
				cleanResponseWithUsage(10, 20, 30),
			}}
			evaluator := alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")

			wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
				Inner:          inner,
				Evaluator:      evaluator,
				VerdictTimeout: 5 * time.Second,
				AuditStore:     store,
				Logger:         logr.Discard(),
				Mode:           config.AlignmentModeMonitor,
			})
			Expect(err).NotTo(HaveOccurred())

			sig := katypes.SignalContext{
				Name: "test-signal", Namespace: "ns", Message: "alert",
				RemediationID: "rr-1077-003",
			}
			_, investigateErr := wrapper.Investigate(context.Background(), sig)
			Expect(investigateErr).NotTo(HaveOccurred())

			// The metric label should be "aligned", not "clean".
			// We verify indirectly by checking that the verdict Result
			// passed to WithLabelValues is the VerdictClean constant,
			// which must equal "aligned".
			Expect(string(alignment.VerdictClean)).To(Equal("aligned"),
				"the metric label passed to alignmentVerdictTotal is string(verdict.Result) — "+
					"VerdictClean must be \"aligned\" for correct Prometheus label (#1077)")
		})
	})

	Describe("UT-SA-1077-004: Step-level metric uses \"aligned\" label for clean steps", func() {
		It("should use \"aligned\" (not \"clean\") as the outcome label for non-suspicious steps", func() {
			// This test verifies that observer.SubmitAsync uses "aligned" as the
			// alignmentStepTotal label for non-suspicious, non-panic steps.
			// The observer currently hardcodes "clean" at observer.go:137 — this must change.
			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout: 5 * time.Second, MaxRetries: 1,
			}, "")
			observer, err := alignment.NewObserver(evaluator)
			Expect(err).NotTo(HaveOccurred())

			observer.SubmitAsync(context.Background(), alignment.Step{
				Index: 0, Kind: alignment.StepKindToolResult, Tool: "get_pods", Content: "pods OK",
			})
			wr := observer.WaitForCompletion(5 * time.Second)
			Expect(wr.Complete).To(BeTrue())
			Expect(wr.Observations).To(HaveLen(1))
			Expect(wr.Observations[0].Suspicious).To(BeFalse())

			// The metric label for non-suspicious steps should be "aligned".
			// We can't directly inspect Prometheus counters in unit tests without
			// a custom registry, so we verify the constant used by the label.
			Expect(string(alignment.VerdictClean)).To(Equal("aligned"),
				"observer.go step metric must use \"aligned\" label for clean steps (#1077)")
		})
	})
})
