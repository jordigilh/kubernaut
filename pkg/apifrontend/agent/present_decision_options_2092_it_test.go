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

	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// runnableTool matches the unexported ADK runnableTool interface via
// structural typing (same technique root_test.go's findTool already relies
// on in this package) so this IT test can invoke the real,
// schema-validating functiontool.New-constructed present_decision tool
// directly -- the exact tool tools.NewPresentDecisionTool wires into
// NewRootAgent's tool list in production.
type runnableTool interface {
	Run(ctx tool.Context, args any) (map[string]any, error)
}

// IT #2092: unlike present_decision_options_2092_test.go (which only
// asserts on args mutation), this proves the repaired options value
// actually survives ADK's real JSON-schema validation
// (typeutil.ConvertToWithJSONSchema, invoked inside functionTool.Run) --
// the exact step live evidence showed rejecting a JSON-double-encoded
// options string before HandlePresentDecision ever ran. It mirrors
// google.golang.org/adk's internal/llminternal/base_flow.go
// Flow.callTool call sequence verbatim: BeforeToolCallbacks run first
// (mutating fArgs in place), THEN tool.Run is invoked with the
// (possibly-mutated) same map -- with neither step mocked out here.
var _ = Describe("IT #2092 — present_decision options repair through real ADK schema validation", func() {
	It("IT-AF-2092-001 (SI-10, SI-11): a JSON-double-encoded options string survives real functiontool.New schema validation once phaseGuardBefore repairs it", func() {
		presentTool, err := tools.NewPresentDecisionTool()
		Expect(err).NotTo(HaveOccurred())
		runnable, ok := presentTool.(runnableTool)
		Expect(ok).To(BeTrue(), "kubernaut_present_decision must implement the runnable Run(ctx, args) contract")

		state := newMapState()
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		toolCtx := statefulToolContext{
			fakeToolContext: fakeToolContext{Context: ctx},
			state:           state,
		}
		before, after := NewPhaseGuardForTest()

		// Ground the session exactly as a real kubernaut_investigate success
		// would, so enforceGroundingGuard takes the passthrough branch this
		// fix targets (the fail-closed ungrounded branch already forces
		// options to a clean []any{} and can never carry a stringified
		// value in the first place).
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2092-it", "status": "completed",
			"summary": "Deployment stuck in CrashLoopBackOff after a bad command override.",
		}, nil)

		optionsJSON := `[{"workflow_id":"wf-1","name":"crashloop-rollback-v1",` +
			`"description":"Roll back to the previous known-good revision","recommended":true}]`
		args := map[string]any{
			"session_id": "sess-2092-it",
			"summary":    "Root cause identified: bad command override.",
			"rca": map[string]any{
				"severity":   "critical",
				"confidence": 0.9,
				"target":     "Deployment/checkout-service",
			},
			"options": optionsJSON,
		}

		cbResult, cbErr := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(cbErr).NotTo(HaveOccurred())
		Expect(cbResult).To(BeNil(), "present_decision must still be allowed to execute (AU-3 artifact mandate)")

		result, runErr := runnable.Run(toolCtx, args)
		Expect(runErr).NotTo(HaveOccurred(),
			"#2092: real ADK schema validation (ConvertToWithJSONSchema) must accept the repaired native array -- "+
				"before this fix, a JSON-double-encoded options string reached this point unmodified and was rejected here")
		Expect(result).NotTo(BeNil())
		Expect(result["presented"]).To(Equal(true))

		message, _ := result["message"].(string)
		Expect(message).To(ContainSubstring("crashloop-rollback-v1"),
			"the repaired option must actually reach HandlePresentDecision's output, not just pass schema validation")
	})

	It("IT-AF-2092-002 (SI-11, regression guard): a genuinely malformed options string still fails real schema validation, proving the repair does not mask real errors", func() {
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

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2092-it-2", "status": "completed",
			"summary": "Deployment stuck in CrashLoopBackOff after a bad command override.",
		}, nil)

		args := map[string]any{
			"session_id": "sess-2092-it-2",
			"summary":    "Root cause identified.",
			"rca": map[string]any{
				"severity":   "critical",
				"confidence": 0.9,
				"target":     "Deployment/checkout-service",
			},
			"options": "this is not valid JSON at all {{{",
		}

		_, cbErr := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(cbErr).NotTo(HaveOccurred())

		_, runErr := runnable.Run(toolCtx, args)
		Expect(runErr).To(HaveOccurred(),
			"a genuinely malformed options string must still be rejected by real schema validation, not silently masked as valid")
	})
})
