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
)

// #2092: live evidence showed a model emitting kubernaut_present_decision's
// "options" argument as a JSON-encoded string (the array serialized once,
// then the whole blob wrapped again as a string value) instead of a native
// JSON array. ADK's functionTool.Run (ConvertToWithJSONSchema) rejects this
// before HandlePresentDecision ever runs, and the agent's observed fallback
// was to retry with empty options -- masking a real, high-confidence
// workflow discovery as a false "no matching workflows". These tests prove
// phaseGuardBefore repairs the stringified value in place, using the same
// "mutate args before schema validation" technique #2023's grounding guard
// already relies on for rca/summary/options.
var _ = Describe("Phase Guard — present_decision options repair (#2092)", func() {
	var (
		state   *mapState
		toolCtx tool.Context
		before  func(tool.Context, tool.Tool, map[string]any) (map[string]any, error)
		after   func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)
	)

	BeforeEach(func() {
		state = newMapState()
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		toolCtx = statefulToolContext{
			fakeToolContext: fakeToolContext{Context: ctx},
			state:           state,
		}
		before, after = NewPhaseGuardForTest()

		// Ground the session so enforceGroundingGuard's options-passthrough
		// branch is exercised (the ungrounded branch already unconditionally
		// resets options to []any{}, so it needs no repair).
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2092", "status": "completed",
			"summary": "Deployment stuck in CrashLoopBackOff after a bad command override.",
		}, nil)
	})

	It("UT-AF-2092-001 (SI-10): repairs a JSON-double-encoded options string into a native array", func() {
		optionsJSON := `[{"workflow_id":"wf-1","name":"crashloop-rollback-v1","recommended":true},` +
			`{"workflow_id":"wf-2","name":"rollback-deployment-v1"}]`
		args := map[string]any{
			"session_id": "sess-2092",
			"summary":    "Root cause identified.",
			"options":    optionsJSON,
		}

		result, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(), "present_decision must still be allowed to execute (AU-3 artifact mandate)")

		options, ok := args["options"].([]any)
		Expect(ok).To(BeTrue(), "options must become a native []any, not remain a JSON string")
		Expect(options).To(HaveLen(2))

		first, ok := options[0].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(first["workflow_id"]).To(Equal("wf-1"))
		Expect(first["name"]).To(Equal("crashloop-rollback-v1"))
		Expect(first["recommended"]).To(BeTrue())
	})

	It("UT-AF-2092-002 (SI-10, SI-11, regression guard): leaves a genuinely malformed options string untouched", func() {
		args := map[string]any{
			"session_id": "sess-2092",
			"summary":    "Root cause identified.",
			"options":    "this is not valid JSON at all {{{",
		}

		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		Expect(args["options"]).To(Equal("this is not valid JSON at all {{{"),
			"a genuinely malformed options string must be left as-is so ADK's real schema validation "+
				"still rejects it with a useful error, instead of being silently masked as an empty-but-valid array")
	})

	It("UT-AF-2092-003 (regression guard): leaves an already-native options array untouched", func() {
		args := map[string]any{
			"session_id": "sess-2092",
			"summary":    "Root cause identified.",
			"options": []any{
				map[string]any{"workflow_id": "wf-1", "name": "crashloop-rollback-v1"},
			},
		}

		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		options, ok := args["options"].([]any)
		Expect(ok).To(BeTrue())
		Expect(options).To(HaveLen(1))
		first, _ := options[0].(map[string]any)
		Expect(first["workflow_id"]).To(Equal("wf-1"), "an already-native options array must not be double-processed")
	})

	It("UT-AF-2092-004 (regression guard): an empty options string is left untouched, not coerced to an empty array", func() {
		args := map[string]any{
			"session_id": "sess-2092",
			"summary":    "Root cause identified.",
			"options":    "",
		}

		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		Expect(args["options"]).To(Equal(""),
			"an empty string is a distinct, already-ADK-schema-invalid case this repair should not mask either")
	})
})
