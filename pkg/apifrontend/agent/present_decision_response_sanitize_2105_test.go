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

package agent

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// #2105 (v1.6 clone #2106; E2E-AF-1396-001 regression from #2098):
// part_converter.go's emitDecisionEvent builds the AU-3 SSE decision
// artifact directly from the model's raw kubernaut_present_decision
// FunctionCall, which ADK streams to the client BEFORE BeforeToolCallback
// (phaseGuardBefore/enforceGroundingGuard) ever runs -- see
// sanitizePresentDecisionResponse's doc comment (phase_guard.go) for the
// exact ADK call sequence proving this ordering. These tests prove this
// AfterModelCallback sanitizes the model's raw FunctionCall.Args in place,
// at the one point in the pipeline that runs before that yield, so the
// emitted decision artifact is grounded/honest regardless of whether the
// tool call is later blocked (#2098) or succeeds.
var _ = Describe("sanitizePresentDecisionResponse AfterModelCallback (#2105, v1.6 clone #2106)", func() {
	newCtx := func(state *mapState) *statefulCallbackContext {
		return &statefulCallbackContext{
			stubCallbackContext: &stubCallbackContext{Context: context.Background()},
			state:               state,
		}
	}

	presentDecisionResponse := func(fabricatedToolCallsCount, fabricatedLLMTurns int) *model.LLMResponse {
		return &model.LLMResponse{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							Name: presentDecisionTool,
							Args: map[string]any{
								"session_id": "sess-2105",
								"summary":    "Root cause identified: bad command override.",
								"rca": map[string]any{
									"severity": "critical", "confidence": 0.9,
									"target":           "Deployment/checkout-service",
									"tool_calls_count": fabricatedToolCallsCount,
									"llm_turns":        fabricatedLLMTurns,
								},
								"options": []any{},
							},
						},
					},
				},
			},
		}
	}

	It("UT-AF-2105-001 (AU-3): zeros a fabricated tool_calls_count/llm_turns on the raw FunctionCall.Args when grounded", func() {
		state := newMapState()
		Expect(state.Set(session.StateKeyGroundedContentAvailable, true)).To(Succeed())

		resp := presentDecisionResponse(19, 17)
		out, err := sanitizePresentDecisionResponse(newCtx(state), resp, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil(), "must mutate in place and return nil to let ADK use the original (now-sanitized) response")

		fc := resp.Content.Parts[0].FunctionCall
		rcaMap, ok := fc.Args["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rcaMap["tool_calls_count"]).To(Equal(0),
			"the model's fabricated count must be zeroed on the SAME object ADK streams to the SSE client")
		Expect(rcaMap["llm_turns"]).To(Equal(0))
	})

	It("UT-AF-2105-002 (regression guard): overwrites args with the honest no-data payload when ungrounded", func() {
		state := newMapState() // StateKeyGroundedContentAvailable never set -> ungrounded

		resp := presentDecisionResponse(19, 17)
		out, err := sanitizePresentDecisionResponse(newCtx(state), resp, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())

		fc := resp.Content.Parts[0].FunctionCall
		Expect(fc.Args["summary"]).To(Equal(noGroundedContentSummary))
		Expect(fc.Args["rca"]).To(Equal(emptyRCAPayload))
	})

	It("UT-AF-2105-003 (regression guard): leaves unrelated FunctionCalls untouched", func() {
		state := newMapState()
		resp := &model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{
					{FunctionCall: &genai.FunctionCall{Name: "kubernaut_investigate", Args: map[string]any{"name": "pod-1"}}},
				},
			},
		}

		out, err := sanitizePresentDecisionResponse(newCtx(state), resp, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())
		Expect(resp.Content.Parts[0].FunctionCall.Args).To(Equal(map[string]any{"name": "pod-1"}),
			"only kubernaut_present_decision FunctionCalls are sanitized")
	})

	It("UT-AF-2105-004 (regression guard): handles a nil/errored/contentless response without panicking", func() {
		state := newMapState()

		out, err := sanitizePresentDecisionResponse(newCtx(state), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())

		out, err = sanitizePresentDecisionResponse(newCtx(state), &model.LLMResponse{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())

		out, err = sanitizePresentDecisionResponse(newCtx(state), presentDecisionResponse(19, 17), context.DeadlineExceeded)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())
	})
})
