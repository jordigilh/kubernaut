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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// IT #2098: drives the full newPhaseGuard before/after pair (the exact
// callbacks root.go wires into NewRootAgent's BeforeToolCallbacks/
// AfterToolCallbacks) through both orderings QE's live evidence and this
// fix's regression concern cover, for full_remediation mode
// (StateKeyPhase2Blocked == false): present_decision called first (must be
// rejected, never reaching the real schema-validated tool), then
// discover_workflows succeeding and present_decision retried (must reach
// HandlePresentDecision and produce a real decision artifact).
var _ = Describe("IT #2098 — present_decision ordering guard through the full phase-guard callback pair", func() {
	It("IT-AF-2098-001 (AC-6): present_decision is rejected before discover_workflows, then succeeds once discovery has run, in full_remediation mode", func() {
		presentTool, err := tools.NewPresentDecisionTool()
		Expect(err).NotTo(HaveOccurred())
		runnable, ok := presentTool.(runnableTool)
		Expect(ok).To(BeTrue())

		state := newMapState()
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		toolCtx := statefulToolContext{
			fakeToolContext: fakeToolContext{Context: ctx},
			state:           state,
		}
		before, after := NewPhaseGuardForTest()

		// A real kubernaut_investigate success declaring full_remediation:
		// sets Phase2Blocked=false (autonomous discovery expected) and
		// grounds the session for enforceGroundingGuard.
		_, invErr := after(toolCtx, fakeTool{name: "kubernaut_investigate"},
			map[string]any{"rr_id": "rr-2098-it", "interaction_mode": session.InteractionModeFullRemediation},
			map[string]any{
				"session_id": "sess-2098-it", "rr_id": "rr-2098-it", "status": "completed",
				"summary": "Deployment stuck in CrashLoopBackOff after a bad command override.",
			}, nil)
		Expect(invErr).NotTo(HaveOccurred())

		// --- Ordering violation: present_decision called before discovery ---
		// rca carries a fabricated tool_calls_count (mirroring a mock/hallucinated
		// LLM payload) to prove the #2105 (v1.6 clone #2106) fix below: even though this
		// call is rejected, part_converter.go's emitDecisionEvent reads these
		// SAME args (by reference) directly off the model's raw FunctionCall to
		// build the AU-3 SSE decision artifact, independent of this before
		// callback's block/allow outcome -- so grounding must still sanitize
		// args in place before the ordering check runs (E2E-AF-1396-001).
		prematureArgs := map[string]any{
			"session_id": "sess-2098-it",
			"summary":    "should never reach HandlePresentDecision",
			"rca": map[string]any{
				"severity": "critical", "confidence": 0.9, "target": "Deployment/checkout-service",
				"tool_calls_count": 19, "llm_turns": 17,
			},
			"options": []any{},
		}
		cbResult, cbErr := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, prematureArgs)
		Expect(cbErr).NotTo(HaveOccurred())
		Expect(cbResult).NotTo(BeNil(), "the premature call must be rejected by the before callback")
		Expect(cbResult["error"]).To(ContainSubstring("kubernaut_discover_workflows"))

		rcaMap, ok := prematureArgs["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rcaMap["tool_calls_count"]).To(Equal(0),
			"#2105 (v1.6 clone #2106): enforceGroundingGuard must still zero the fabricated tool_calls_count in place "+
				"even when the call is rejected by the #2098 ordering guard, since the SSE decision artifact is "+
				"built from these same (by-reference) args regardless of this callback's block/allow outcome")
		Expect(rcaMap["llm_turns"]).To(Equal(0))

		// --- Corrected ordering: discover_workflows runs, then retry ---
		_, dwErr := after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil,
			map[string]any{"workflows": []any{map[string]any{"workflow_id": "wf-1", "name": "crashloop-rollback-v1"}}}, nil)
		Expect(dwErr).NotTo(HaveOccurred())

		retryArgs := map[string]any{
			"session_id": "sess-2098-it",
			"summary":    "Root cause identified: bad command override.",
			"rca": map[string]any{
				"severity": "critical", "confidence": 0.9, "target": "Deployment/checkout-service",
			},
			"options": []any{
				map[string]any{
					"workflow_id": "wf-1", "name": "crashloop-rollback-v1",
					"description": "Roll back to the previous known-good revision", "recommended": true,
				},
			},
		}
		cbResult2, cbErr2 := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, retryArgs)
		Expect(cbErr2).NotTo(HaveOccurred())
		Expect(cbResult2).To(BeNil(), "the retry after discover_workflows succeeded must be allowed through")

		result, runErr := runnable.Run(toolCtx, retryArgs)
		Expect(runErr).NotTo(HaveOccurred())
		Expect(result["presented"]).To(BeTrue())

		message, _ := result["message"].(string)
		Expect(message).To(ContainSubstring("crashloop-rollback-v1"),
			"the real decision must reach the user only after the correct discover_workflows -> present_decision ordering")
	})
})
