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
	"bytes"
	"context"
	"encoding/gob"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// #2110 (v1.6 clone #2111): a2a-go@v0.3.15's task manager deep-copies every
// TaskArtifactUpdateEvent's artifact via a gob encode/decode round-trip
// (internal/utils/utils.go's DeepCopy) before fanning it out to subscriber
// goroutines. encoding/gob requires any concrete type stored behind an
// interface{} to be registered via gob.Register before it can be encoded --
// a2a-go's own internal/taskstore/store.go registers map[string]any and
// []any at init() (imported transitively via pkg/apifrontend/launcher's use
// of a2a-go types), which is why the pre-#2098 code -- and the fixed code
// below -- works. tools.RCAData is never gob.Register'd anywhere in this
// repo, so canonicalGroundedRCA's *tools.RCAData return value, previously
// assigned directly into args["rca"] (a map[string]any) by
// enforceGroundingGuard, crashed every grounded kubernaut_present_decision
// call in production with "gob: type not registered for interface:
// tools.RCAData" (confirmed live on a v1.5.6-rc4 dev cluster, 7/7
// reproductions, 0 successful grounded present_decision completions).
//
// These tests gob-encode the exact map[string]any shape that
// EventBridge.EmitArtifact (launcher/event_bridge.go) hands to a2a-go as
// a2a.DataPart.Data -- the literal operation that crashed -- rather than
// depending on a2a-go's internal (unexported) DeepCopy/taskupdate.Manager,
// which cannot be called directly from outside that module.
var _ = Describe("present_decision rca gob-encodability (#2110, v1.6 clone #2111)", func() {
	// registerA2AGobTypes mirrors a2a-go@v0.3.15's internal/taskstore/
	// store.go init() so this test is deterministic regardless of which
	// other packages happen to be linked into the test binary and have
	// already triggered that init() as a side effect. gob.Register is
	// idempotent, so calling this alongside that real init() is safe.
	registerA2AGobTypes := func() {
		gob.Register(map[string]any{})
		gob.Register([]any{})
	}

	// encodeAsArtifactData replicates EventBridge.EmitArtifact's exact
	// wrapping of kubernaut_present_decision's FunctionCall.Args into an
	// a2a.DataPart-shaped payload, then gob round-trips it the same way
	// a2a-go's task manager does internally.
	encodeAsArtifactData := func(data map[string]any) error {
		registerA2AGobTypes()
		var buf bytes.Buffer
		return gob.NewEncoder(&buf).Encode(data)
	}

	groundedInvestigateResult := map[string]any{
		"session_id": "sess-2110", "status": "completed",
		"summary": "Real investigation summary.",
		"rca": map[string]any{
			"severity": "critical", "confidence": 0.9,
			"causal_chain":     []any{"MemoryPressure", "Evicted"},
			"target":           "pod/checkout-service",
			"total_tool_calls": 7, "total_llm_turns": 3,
		},
	}

	fabricatedPresentDecisionArgs := func() map[string]any {
		return map[string]any{
			"session_id": "sess-2110",
			"summary":    "Fabricated LLM narrative.",
			"rca": map[string]any{
				"severity": "critical", "confidence": 0.5,
			},
			"options": []any{},
		}
	}

	It("UT-AF-2110-001 (regression, BeforeToolCallback path): the rca enforceGroundingGuard substitutes for a fully-grounded investigate result must be gob-encodable", func() {
		state := newMapState()
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		toolCtx := statefulToolContext{
			fakeToolContext: fakeToolContext{Context: ctx},
			state:           state,
		}
		before, after := NewPhaseGuardForTest()

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, groundedInvestigateResult, nil)

		args := fabricatedPresentDecisionArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		Expect(encodeAsArtifactData(args)).To(Succeed(),
			"args[\"rca\"] must be gob-encodable -- a *tools.RCAData struct pointer here crashes a2a-go's "+
				"real task-manager DeepCopy in production with 'gob: type not registered for interface: tools.RCAData'")
	})

	It("UT-AF-2110-002 (regression, AfterModelCallback / SSE-emission path): sanitizePresentDecisionResponse's grounded rca substitution must be gob-encodable", func() {
		// This is the actual runtime path that crashed in production
		// (#2105's own sanitize tests never covered this scenario -- they
		// never populated session.StateKeyGroundedRCA with a genuine
		// *tools.InvestigateRCA, only StateKeyGroundedContentAvailable, so
		// they always hit enforceGroundingGuard's "backfill zeros onto the
		// LLM's own map" branch, never canonicalGroundedRCA's struct-typed
		// substitution branch that actually crashed).
		state := newMapState()
		Expect(state.Set(session.StateKeyGroundedContentAvailable, true)).To(Succeed())
		Expect(state.Set(session.StateKeyGroundedRCA, &tools.InvestigateRCA{
			Severity: "critical", Confidence: 0.9,
			CausalChain:    []string{"MemoryPressure", "Evicted"},
			Target:         "pod/checkout-service",
			TotalToolCalls: 7, TotalLLMTurns: 3,
		})).To(Succeed())

		resp := &model.LLMResponse{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{
					FunctionCall: &genai.FunctionCall{
						Name: presentDecisionTool,
						Args: fabricatedPresentDecisionArgs(),
					},
				}},
			},
		}

		out, err := sanitizePresentDecisionResponse(&statefulCallbackContext{
			stubCallbackContext: &stubCallbackContext{Context: context.Background()},
			state:               state,
		}, resp, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeNil())

		fc := resp.Content.Parts[0].FunctionCall
		Expect(encodeAsArtifactData(fc.Args)).To(Succeed(),
			"the exact FunctionCall.Args ADK streams to the SSE client as the AU-3 decision artifact must be "+
				"gob-encodable by a2a-go's real task-manager DeepCopy")
	})

	It("UT-AF-2110-003: enforceGroundingGuard's grounded rca substitution is a map[string]any, not a *tools.RCAData struct pointer", func() {
		state := newMapState()
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		toolCtx := statefulToolContext{
			fakeToolContext: fakeToolContext{Context: ctx},
			state:           state,
		}
		before, after := NewPhaseGuardForTest()

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, groundedInvestigateResult, nil)

		args := fabricatedPresentDecisionArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue(), "rca must be a map[string]any -- the concrete type a2a-go's own "+
			"internal/taskstore/store.go registers for gob -- not a *tools.RCAData struct pointer, "+
			"which is never gob.Register'd anywhere in this repo")
		Expect(rca["severity"]).To(Equal("critical"))
		Expect(rca["confidence"]).To(Equal(0.9))
		Expect(rca["target"]).To(Equal("pod/checkout-service"))
		Expect(rca["causal_chain"]).To(Equal([]string{"MemoryPressure", "Evicted"}))
		Expect(rca["tool_calls_count"]).To(Equal(7))
		Expect(rca["llm_turns"]).To(Equal(3))
	})
})
