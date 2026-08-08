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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// phaseGuardAfter AfterToolCallback contract (#1997, ported to main as #1998).
//
// ADK's invokeAfterToolCallbacks (google.golang.org/adk, internal/llminternal/
// base_flow.go) treats a non-nil result returned by ANY AfterToolCallback as
// an explicit override that stops the chain immediately -- every later
// callback (afterLog in root.go's registration order: afterMetrics,
// afterAudit, afterPhase, afterLog) never runs. A callback that only wants
// to observe/record a side effect (as phaseGuardAfter does -- it never
// modifies resp) MUST return (nil, nil) to signal "pass through unchanged",
// exactly as afterAudit/afterMetrics already do correctly in the same file.
//
// phaseGuardAfter instead re-returns (resp, callErr) from every exit path,
// which -- since resp is non-nil for any tool call that produced a
// structured result -- unconditionally short-circuits the chain and skips
// afterLog for every tool call, not just the driver-entry/terminal/
// discover_workflows tools phaseGuardAfter cares about.
//
// Business requirement: BR-AUDIT-005 (audit completeness) / FedRAMP AU-3
// (content of audit records) and AU-12 (audit generation), as implemented
// by afterLog's "tool call completed" line (root.go, TP-1310 §4.3). This
// suite proves the logic that was silently dropping that audit signal;
// IT-AF-1997-009 (phase_guard_afterlog_wiring_1997_test.go) proves the fix
// closes the gap end-to-end through the real production dispatch path.
var _ = Describe("phaseGuardAfter AfterToolCallback contract (#1997)", func() {
	var (
		state   *mapState
		toolCtx tool.Context
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
		_, after = NewPhaseGuardForTest()
	})

	DescribeTable("must return (nil, nil) on success so later AfterToolCallbacks (afterLog) still run",
		func(toolName string, resp map[string]any) {
			result, err := after(toolCtx, fakeTool{name: toolName}, nil, resp, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(),
				"%s: phaseGuardAfter must return (nil, nil) on success -- returning the original resp short-circuits ADK's AfterToolCallback chain and silently drops afterLog", toolName)
		},
		Entry("UT-AF-1997-001: kubernaut_discover_workflows (phase3 checkpoint path)", "kubernaut_discover_workflows", map[string]any{"workflows": []any{}}),
		Entry("UT-AF-1997-002: kubernaut_investigate (driver-entry path)", "kubernaut_investigate", map[string]any{"session_id": "sess-1997", "status": "active"}),
		Entry("UT-AF-1997-003: kubernaut_reconnect (driver-entry path)", "kubernaut_reconnect", map[string]any{"session_id": "sess-1997b", "status": "active"}),
		Entry("UT-AF-1997-004: kubernaut_complete (session-terminal path)", "kubernaut_complete", map[string]any{"status": "completed"}),
		Entry("UT-AF-1997-005: kubernaut_cancel (session-terminal path)", "kubernaut_cancel", map[string]any{"status": "cancelled"}),
		Entry("UT-AF-1997-006: kubernaut_complete_no_action (unmatched-tool fallthrough)", "kubernaut_complete_no_action", map[string]any{"status": "no_action"}),
		Entry("UT-AF-1997-007: kubernaut_message (unmatched-tool fallthrough, unrelated tool)", "kubernaut_message", map[string]any{"reply": "hi"}),
	)

	It("UT-AF-1997-008: also returns (nil, nil) on a failed call, instead of re-surfacing callErr itself (no functional regression: ADK falls back to the original fErr when every callback passes through)", func() {
		result, err := after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, nil, fmt.Errorf("boom"))
		Expect(result).To(BeNil())
		Expect(err).NotTo(HaveOccurred(),
			"phaseGuardAfter must pass through (nil, nil) on failure too, so later AfterToolCallbacks still observe the failed call instead of the chain terminating early on phaseGuardAfter's own re-returned error")
	})
})
