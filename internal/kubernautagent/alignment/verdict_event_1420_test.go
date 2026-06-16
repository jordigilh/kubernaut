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
	"encoding/json"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Issue #1420: EventTypeAlignmentVerdict Emission (AU-2)", func() {

	var (
		shadowClient *mockLLMClient
		evaluator    *alignment.Evaluator
		signal       katypes.SignalContext
	)

	BeforeEach(func() {
		signal = katypes.SignalContext{
			Name: "test-pod", Namespace: "default", Severity: "critical",
			Message: "OOM detected",
		}
		shadowClient = &mockLLMClient{}
	})

	createEvaluator := func() {
		evaluator = alignment.NewEvaluator(shadowClient, alignment.EvaluatorConfig{
			Timeout: 5 * time.Second, MaxStepTokens: 4000, MaxRetries: 1,
		}, "You are an alignment checker.", alignment.WithLogger(logr.Discard()))
	}

	It("UT-KA-1420-020: alignment_verdict event precedes complete on event channel", func() {
		shadowClient.responses = []llm.ChatResponse{
			cleanResponse(),
			suspiciousResponse(),
		}
		createEvaluator()

		inner := &mockInvestigationRunnerWithObserver{
			result: &katypes.InvestigationResult{RCASummary: "partial RCA"},
			onInvestigate: func(ctx context.Context) {
				obs := alignment.ObserverFromContext(ctx)
				if obs != nil {
					obs.SubmitAsync(ctx, alignment.Step{
						Index: 1, Kind: alignment.StepKindToolResult,
						Tool: "kubectl_get", Content: "pod data",
					})
				}
			},
		}

		wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:            inner,
			Evaluator:        evaluator,
			VerdictTimeout:   5 * time.Second,
			Logger:           logr.Discard(),
			Mode:             config.AlignmentModeEnforce,
			GroundingEnabled: false,
		})
		Expect(err).NotTo(HaveOccurred())

		eventCh := make(chan session.InvestigationEvent, 10)
		ctx := session.WithEventSink(context.Background(), eventCh)

		result, err := wrapper.Investigate(ctx, signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.AlignmentVerdict).NotTo(BeNil())

		eventCh <- session.InvestigationEvent{Type: session.EventTypeComplete}

		var events []session.InvestigationEvent
		for {
			select {
			case evt := <-eventCh:
				events = append(events, evt)
			default:
				goto done
			}
		}
	done:

		var verdictIdx, completeIdx int
		verdictIdx = -1
		completeIdx = -1
		for i, evt := range events {
			if evt.Type == session.EventTypeAlignmentVerdict {
				verdictIdx = i
			}
			if evt.Type == session.EventTypeComplete {
				completeIdx = i
			}
		}

		Expect(verdictIdx).To(BeNumerically(">=", 0),
			"AU-2.d: alignment_verdict event must be emitted on event stream")
		Expect(completeIdx).To(BeNumerically(">=", 0),
			"complete event must be present for ordering verification")
		Expect(verdictIdx).To(BeNumerically("<", completeIdx),
			"AU-2.d: alignment_verdict must precede complete event")
	})

	It("UT-KA-1420-021: alignment_verdict event contains full AlignmentVerdictResult payload", func() {
		shadowClient.responses = []llm.ChatResponse{
			cleanResponse(),
			suspiciousResponse(),
		}
		createEvaluator()

		inner := &mockInvestigationRunnerWithObserver{
			result: &katypes.InvestigationResult{RCASummary: "partial RCA"},
			onInvestigate: func(ctx context.Context) {
				obs := alignment.ObserverFromContext(ctx)
				if obs != nil {
					obs.SubmitAsync(ctx, alignment.Step{
						Index: 1, Kind: alignment.StepKindToolResult,
						Tool: "kubectl_get_by_name", Content: "malicious content",
					})
				}
			},
		}

		wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:            inner,
			Evaluator:        evaluator,
			VerdictTimeout:   5 * time.Second,
			Logger:           logr.Discard(),
			Mode:             config.AlignmentModeEnforce,
			GroundingEnabled: false,
		})
		Expect(err).NotTo(HaveOccurred())

		eventCh := make(chan session.InvestigationEvent, 10)
		ctx := session.WithEventSink(context.Background(), eventCh)

		result, err := wrapper.Investigate(ctx, signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		var verdictEvent *session.InvestigationEvent
		for {
			select {
			case evt := <-eventCh:
				if evt.Type == session.EventTypeAlignmentVerdict {
					verdictEvent = &evt
				}
			default:
				goto done2
			}
		}
	done2:

		Expect(verdictEvent).NotTo(BeNil(), "AU-2.a: alignment_verdict event must be emitted")
		Expect(verdictEvent.Data).NotTo(BeEmpty(), "AU-2.a: event Data must contain serialized verdict")

		var avr katypes.AlignmentVerdictResult
		err = json.Unmarshal(verdictEvent.Data, &avr)
		Expect(err).NotTo(HaveOccurred())
		Expect(avr.Result).NotTo(BeEmpty(), "AU-2.a: Result field must be populated")
		Expect(avr.Total).To(BeNumerically(">", 0), "AU-2.a: Total field must reflect evaluated steps")
	})

	It("UT-KA-1420-022: aligned verdict still emits alignment_verdict event for audit completeness", func() {
		shadowClient.responses = []llm.ChatResponse{
			suspiciousResponse(),
			cleanResponse(),
			cleanResponse(),
			cleanResponse(),
		}
		createEvaluator()

		inner := &mockInvestigationRunnerWithObserver{
			result: &katypes.InvestigationResult{RCASummary: "clean RCA"},
			onInvestigate: func(ctx context.Context) {
				obs := alignment.ObserverFromContext(ctx)
				if obs != nil {
					obs.SubmitAsync(ctx, alignment.Step{
						Index: 1, Kind: alignment.StepKindToolResult,
						Tool: "kubectl_describe", Content: "clean output",
					})
				}
			},
		}

		wrapper, err := alignment.NewInvestigatorWrapper(alignment.InvestigatorWrapperConfig{
			Inner:            inner,
			Evaluator:        evaluator,
			VerdictTimeout:   5 * time.Second,
			Logger:           logr.Discard(),
			Mode:             config.AlignmentModeEnforce,
			GroundingEnabled: false,
		})
		Expect(err).NotTo(HaveOccurred())

		eventCh := make(chan session.InvestigationEvent, 10)
		ctx := session.WithEventSink(context.Background(), eventCh)

		result, err := wrapper.Investigate(ctx, signal)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.AlignmentVerdict).NotTo(BeNil())
		Expect(result.AlignmentVerdict.Result).To(Equal("aligned"))

		var foundVerdict bool
		for {
			select {
			case evt := <-eventCh:
				if evt.Type == session.EventTypeAlignmentVerdict {
					foundVerdict = true
					var avr katypes.AlignmentVerdictResult
					Expect(json.Unmarshal(evt.Data, &avr)).To(Succeed())
					Expect(avr.Result).To(Equal("aligned"))
				}
			default:
				goto done3
			}
		}
	done3:

		Expect(foundVerdict).To(BeTrue(),
			"AU-2.d: aligned verdict must still emit alignment_verdict event for audit completeness")
	})
})
