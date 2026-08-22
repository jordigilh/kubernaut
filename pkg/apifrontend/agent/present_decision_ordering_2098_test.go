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

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// #2098: live evidence (RR rr-6a97f7a00ba9-79ad53ed) showed
// kubernaut_present_decision invoked and succeeding nine seconds before
// kubernaut_discover_workflows even started in full_remediation mode --
// the model rendered an empty decision despite a real workflow being
// discovered moments later. newPhaseGuard's presentDecisionTool branch had
// no ordering precondition at all. These tests prove a new before-callback
// gate rejects present_decision (instructing the model to call
// discover_workflows first) in exactly the modes where that ordering is
// load-bearing (StateKeyPhase2Blocked == false), while never blocking the
// legitimate cases where discover_workflows is skipped by design
// (interactive mode, or #1918's rcaConcludedNotActionable override --
// both of which force Phase2Blocked == true).
var _ = Describe("Phase Guard — present_decision ordering guard (#2098)", func() {
	var (
		state   *mapState
		toolCtx agent.Context
		before  func(agent.Context, tool.Tool, map[string]any) (map[string]any, error)
		after   func(agent.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)
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
	})

	It("UT-AF-2098-001 (AC-6): rejects present_decision when Phase2Blocked==false and discover_workflows has not yet succeeded", func() {
		Expect(state.Set(session.StateKeyPhase2Blocked, false)).To(Succeed())

		args := map[string]any{
			"session_id": "sess-2098",
			"summary":    "should not be reached",
		}
		result, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(), "present_decision must be rejected before discover_workflows has run")

		errMsg, ok := result["error"].(string)
		Expect(ok).To(BeTrue())
		Expect(errMsg).To(ContainSubstring("kubernaut_discover_workflows"),
			"the rejection must instruct the model to call discover_workflows first, mirroring DD-AF-011's reject-and-retry pattern")
	})

	It("UT-AF-2098-002 (AC-6): a successful discover_workflows call unblocks a subsequent present_decision call", func() {
		Expect(state.Set(session.StateKeyPhase2Blocked, false)).To(Succeed())

		_, dwErr := after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil,
			map[string]any{"workflows": []any{map[string]any{"workflow_id": "wf-1"}}}, nil)
		Expect(dwErr).NotTo(HaveOccurred())

		args := map[string]any{"session_id": "sess-2098", "summary": "root cause identified"}
		result, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(), "present_decision must be allowed once discover_workflows has succeeded")
	})

	It("UT-AF-2098-003 (regression guard): present_decision is not blocked when Phase2Blocked==true even though discover_workflows never ran", func() {
		Expect(state.Set(session.StateKeyPhase2Blocked, true)).To(Succeed())

		args := map[string]any{"session_id": "sess-2098", "summary": "no remediation warranted"}
		result, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"interactive mode (and #1918's rcaConcludedNotActionable override) both force Phase2Blocked=true and "+
				"legitimately skip discover_workflows -- the ordering gate must not block these")
	})

	It("UT-AF-2098-004 (regression guard): a fresh kubernaut_investigate success resets discover_workflows_succeeded to false", func() {
		Expect(state.Set(session.StateKeyPhase2Blocked, false)).To(Succeed())
		_, dwErr := after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil,
			map[string]any{"workflows": []any{}}, nil)
		Expect(dwErr).NotTo(HaveOccurred())

		v, getErr := state.Get(session.StateKeyDiscoverWorkflowsSucceeded)
		Expect(getErr).NotTo(HaveOccurred())
		Expect(v).To(BeTrue())

		_, invErr := after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2098-2", "rr_id": "rr-2", "status": "completed",
			"summary": "a new, unrelated investigation",
		}, nil)
		Expect(invErr).NotTo(HaveOccurred())

		v2, getErr2 := state.Get(session.StateKeyDiscoverWorkflowsSucceeded)
		Expect(getErr2).NotTo(HaveOccurred())
		Expect(v2).To(BeFalse(), "a second investigation in the same chat session must not inherit a stale success flag from a prior RR")
	})
})
