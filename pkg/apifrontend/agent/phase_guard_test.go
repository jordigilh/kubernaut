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
	"iter"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// statefulToolContext extends fakeToolContext with a working session.State
// for phase guard testing. State persists across calls within the same test.
type statefulToolContext struct {
	fakeToolContext
	state     *mapState
	sessionID string
}

func (s statefulToolContext) State() adksession.State { return s.state }
func (s statefulToolContext) SessionID() string {
	if s.sessionID != "" {
		return s.sessionID
	}
	return s.fakeToolContext.SessionID()
}

// mapState is a minimal session.State backed by a map.
type mapState struct {
	data map[string]any
}

func newMapState() *mapState {
	return &mapState{data: make(map[string]any)}
}

func (m *mapState) Get(key string) (any, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, adksession.ErrStateKeyNotExist
	}
	return v, nil
}

func (m *mapState) Set(key string, value any) error {
	m.data[key] = value
	return nil
}

func (m *mapState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range m.data {
			if !yield(k, v) {
				return
			}
		}
	}
}

var _ = Describe("Phase Guard (#1307)", func() {
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
	})

	DescribeTable("blocks MCP-dependent tools without prior investigate",
		func(toolName string) {
			result, err := before(toolCtx, fakeTool{name: toolName}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(),
				"%s must be blocked without prior investigate (#1307)", toolName)
			errMsg, ok := result["error"].(string)
			Expect(ok).To(BeTrue())
			Expect(errMsg).To(ContainSubstring("kubernaut_investigate"),
				"error must guide LLM to call investigate first")
		},
		Entry("UT-AF-1307-001: discover_workflows", "kubernaut_discover_workflows"),
		Entry("UT-AF-1307-002: select_workflow", "kubernaut_select_workflow"),
		Entry("UT-AF-1307-003: message", "kubernaut_message"),
		Entry("UT-AF-1307-004: complete", "kubernaut_complete"),
		Entry("UT-AF-1307-005: cancel", "kubernaut_cancel"),
		Entry("UT-AF-1307-006: status", "kubernaut_status"),
	)

	DescribeTable("always allows non-MCP-dependent tools",
		func(toolName string) {
			result, err := before(toolCtx, fakeTool{name: toolName}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(),
				"%s must always be allowed (no investigate prerequisite)", toolName)
		},
		Entry("UT-AF-1307-008: investigate", "kubernaut_investigate"),
		Entry("UT-AF-1307-009: kubectl_get", "kubectl_get"),
		Entry("UT-AF-1307-012: reconnect", "kubernaut_reconnect"),
	)

	It("UT-AF-1307-010: after investigate succeeds, discover_workflows is allowed", func() {
		// DD-AF-011 (#1899): declares full_remediation so the new consent
		// gate doesn't block phase 2 -- this test exercises driver-active
		// gating, not the consent gate itself (see phase_guard_test.go's
		// "DD-AF-011" Describe block for that coverage).
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-001", "status": "active",
		}, nil)

		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"discover_workflows must be allowed after successful investigate")
	})

	It("UT-AF-1307-011: error message contains guidance to call investigate", func() {
		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		errMsg := result["error"].(string)
		Expect(errMsg).To(ContainSubstring("kubernaut_investigate"),
			"error must name the required prerequisite tool")
	})

	It("UT-AF-1307-013: after investigate succeeds, discover_workflows is allowed", func() {
		// Simulate successful investigation via AfterToolCallback.
		// DD-AF-011 (#1899): declares full_remediation to bypass the new
		// consent gate, since this test is about driver-active gating.
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-inv-001", "status": "completed",
		}, nil)

		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"discover_workflows must be allowed after successful investigate")
	})

	It("UT-AF-1307-014: after investigate succeeds, select_workflow is allowed", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-inv-002", "status": "completed",
		}, nil)

		result, err := before(toolCtx, fakeTool{name: "kubernaut_select_workflow"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"select_workflow must be allowed after successful investigate")
	})

	It("UT-AF-1307-015: investigate error does not activate driver", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"error": "investigation failed",
		}, nil)

		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(),
			"discover_workflows must be blocked when investigate returned an error")
	})

	// --- BR-INTERACTIVE-010: rr_id session state propagation (AU-3 audit continuity) ---

	It("UT-AF-1307-020: after successful investigate, rr_id is stored in session state", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-rr-020", "rr_id": "rr-abc-123", "status": "completed",
		}, nil)

		stored, err := state.Get("af_active_rr_id")
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal("rr-abc-123"),
			"rr_id must be persisted in session state for cross-turn propagation (AU-3)")
	})

	It("UT-AF-1307-021: before callback injects rr_id from state when LLM omits it", func() {
		// Simulate successful investigation storing rr_id.
		// DD-AF-011 (#1899): declares full_remediation to bypass the new
		// consent gate, since this test is about rr_id injection.
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-rr-021", "rr_id": "rr-inject-me", "status": "completed",
		}, nil)

		// LLM calls discover_workflows without rr_id (lost due to history trimming)
		args := map[string]any{}
		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(), "tool must proceed (not be blocked)")
		Expect(args["rr_id"]).To(Equal("rr-inject-me"),
			"phase guard must inject rr_id from state when LLM omits it (BR-INTERACTIVE-010)")
	})

	It("UT-AF-1307-022: LLM-provided rr_id is NOT overwritten by state injection", func() {
		// Store one rr_id in state.
		// DD-AF-011 (#1899): declares full_remediation to bypass the new
		// consent gate, since this test is about rr_id precedence.
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-rr-022", "rr_id": "rr-stale-state", "status": "completed",
		}, nil)

		// LLM explicitly provides a different rr_id
		args := map[string]any{"rr_id": "rr-llm-explicit"}
		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(), "tool must proceed")
		Expect(args["rr_id"]).To(Equal("rr-llm-explicit"),
			"LLM-provided rr_id must take priority over state (no silent override)")
	})

	It("UT-AF-1307-023: after successful investigate, session_id is stored in state", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-ka-456", "rr_id": "rr-023", "status": "completed",
		}, nil)

		stored, err := state.Get("af_active_session_id")
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal("sess-ka-456"),
			"session_id must be persisted in session state for audit correlation (AU-12)")
	})

	It("UT-AF-1307-024: investigate error does not store rr_id in state", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"error": "failed", "rr_id": "rr-should-not-store",
		}, nil)

		_, err := state.Get("af_active_rr_id")
		Expect(err).To(MatchError(adksession.ErrStateKeyNotExist),
			"rr_id must NOT be stored when investigate fails")
	})

	It("UT-AF-1307-025: injection applies to select_workflow as well", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-rr-025", "rr_id": "rr-select-test", "status": "completed",
		}, nil)

		args := map[string]any{"workflow_id": "wf-rollback"}
		result, err := before(toolCtx, fakeTool{name: "kubernaut_select_workflow"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(args["rr_id"]).To(Equal("rr-select-test"),
			"injection must work for all MCP-dependent tools (BR-INTERACTIVE-010)")
	})

	It("UT-AF-1307-026: reconnect stores rr_id from input args when response lacks it", func() {
		// kubernaut_reconnect takes rr_id as input but InteractiveActionResult
		// does not echo it in the response. The after callback must fall back
		// to input args to keep state current.
		inputArgs := map[string]any{"rr_id": "rr-reconnect-target"}
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_reconnect"}, inputArgs, map[string]any{
			"session_id": "sess-reconnect-099", "status": "reconnected",
		}, nil)

		stored, err := state.Get("af_active_rr_id")
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal("rr-reconnect-target"),
			"reconnect must store rr_id from input args for cross-turn propagation (AU-3)")
	})

	It("UT-AF-1307-027: response rr_id takes priority over input args rr_id", func() {
		inputArgs := map[string]any{"rr_id": "rr-input-old"}
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, inputArgs, map[string]any{
			"session_id": "sess-027", "rr_id": "rr-response-new", "status": "completed",
		}, nil)

		stored, err := state.Get("af_active_rr_id")
		Expect(err).NotTo(HaveOccurred())
		Expect(stored).To(Equal("rr-response-new"),
			"response rr_id must take priority over input args rr_id")
	})

	// --- DD-AF-011 (#1899): phase-transition consent gate ---

	It("IT-AF-1899-002: successful investigate with no interaction_mode defaults to interactive and blocks phase 2", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-1899-a", "rr_id": "rr-1899-a", "status": "completed",
		}, nil)

		mode, err := state.Get(session.StateKeyInteractionMode)
		Expect(err).NotTo(HaveOccurred())
		Expect(mode).To(Equal(session.InteractionModeInteractive),
			"fail-safe default: an omitted interaction_mode must resolve to interactive (AC-6)")

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(true),
			"interactive mode must block phase 2 (discover_workflows) until a genuine user turn")
	})

	It("IT-AF-1899-002b: successful investigate with interaction_mode=full_remediation does NOT block phase 2", func() {
		inputArgs := map[string]any{"interaction_mode": "full_remediation"}
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, inputArgs, map[string]any{
			"session_id": "sess-1899-b", "rr_id": "rr-1899-b", "status": "completed",
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(false),
			"full_remediation must auto-proceed through workflow discovery")
	})

	It("IT-AF-1899-002c: an unrecognized interaction_mode value fails safe to interactive", func() {
		inputArgs := map[string]any{"interaction_mode": "definitely-not-a-real-mode"}
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, inputArgs, map[string]any{
			"session_id": "sess-1899-c", "rr_id": "rr-1899-c", "status": "completed",
		}, nil)

		mode, err := state.Get(session.StateKeyInteractionMode)
		Expect(err).NotTo(HaveOccurred())
		Expect(mode).To(Equal(session.InteractionModeInteractive),
			"SI-10: an invalid mode value must never grant MORE autonomy than the safest default")
	})

	It("IT-AF-1899-003: successful discover_workflows blocks phase 3 unless mode is full_remediation_autonomous", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-1899-d", "rr_id": "rr-1899-d", "status": "completed",
		}, nil)
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil, map[string]any{
			"workflows": []any{"wf-1"},
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase3Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(true),
			"full_remediation must still wait for genuine user confirmation before executing a workflow")
	})

	It("IT-AF-1899-003b: full_remediation_autonomous does NOT block phase 3", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation_autonomous"}, map[string]any{
			"session_id": "sess-1899-e", "rr_id": "rr-1899-e", "status": "completed",
		}, nil)
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil, map[string]any{
			"workflows": []any{"wf-1"},
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase3Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(false),
			"full_remediation_autonomous must auto-proceed through workflow selection")
	})

	It("IT-AF-1899-003c: a failed discover_workflows does not set phase3_blocked", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-1899-f", "rr_id": "rr-1899-f", "status": "completed",
		}, nil)
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil, map[string]any{
			"error": "discovery failed",
		}, nil)

		_, err := state.Get(session.StateKeyPhase3Blocked)
		Expect(err).To(MatchError(adksession.ErrStateKeyNotExist),
			"a failed discover_workflows must not set a checkpoint — nothing succeeded to gate yet")
	})

	It("IT-AF-1899-005: before hard-rejects discover_workflows while phase2 is blocked, even with an active driver", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-1899-g", "rr_id": "rr-1899-g", "status": "completed",
		}, nil)

		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(),
			"discover_workflows must be hard-rejected while phase 2 is blocked, even though a driver is active")
		errMsg, ok := result["error"].(string)
		Expect(ok).To(BeTrue())
		Expect(errMsg).To(ContainSubstring("wait"),
			"error must guide the LLM to wait for the user, not retry")
	})

	It("IT-AF-1899-005b: before hard-rejects select_workflow while phase3 is blocked", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-1899-h", "rr_id": "rr-1899-h", "status": "completed",
		}, nil)
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil, map[string]any{
			"workflows": []any{"wf-1"},
		}, nil)

		result, err := before(toolCtx, fakeTool{name: "kubernaut_select_workflow"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(),
			"select_workflow must be hard-rejected while phase 3 is blocked")
	})

	It("IT-AF-1899-005c: select_workflow is allowed when the harness never blocked phase 3 (full_remediation_autonomous)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation_autonomous"}, map[string]any{
			"session_id": "sess-1899-i", "rr_id": "rr-1899-i", "status": "completed",
		}, nil)
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil, map[string]any{
			"workflows": []any{"wf-1"},
		}, nil)

		result, err := before(toolCtx, fakeTool{name: "kubernaut_select_workflow"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(), "select_workflow must be allowed when the harness never blocked phase 3")
	})

	// IT-AF-1915-001 deliberately reuses the same full_remediation mechanism
	// already proven by IT-AF-1899-002b/003/005b above (auto-discover,
	// pause-before-select) -- #1915's bug was never in this harness
	// mechanism (it worked correctly and was already covered here under
	// #1899's own test IDs). The gap was purely that prompt.txt never
	// instructed the model to declare full_remediation for a plain
	// "investigate" request (see prompt_test.go's UT-AF-1915-* for that
	// fix). This test exists only for #1915's own BR/audit traceability --
	// so a regression search for "1915" finds its regression coverage --
	// not to duplicate #1899's mechanism assertions.
	It("IT-AF-1915-001: full_remediation (the new plain-investigate default, #1915) auto-discovers but still pauses before select", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-1915-a", "rr_id": "rr-1915-a", "status": "completed",
		}, nil)

		phase2Blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(phase2Blocked).To(Equal(false),
			"#1915: full_remediation must auto-proceed through workflow discovery for a plain investigate request")

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil, map[string]any{
			"workflows": []any{"wf-1"},
		}, nil)

		phase3Blocked, err := state.Get(session.StateKeyPhase3Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(phase3Blocked).To(Equal(true),
			"#1915: full_remediation must still require a genuine user turn before executing a workflow -- auto-discovery is not auto-execution")
	})

	// --- #1918: harness-enforced actionability gate ---
	//
	// A structured, model-independent override: when KA's RCA (surfaced via
	// InvestigateRCA.IsActionable/HasWorkflow, see ka_investigate_mcp.go)
	// concluded no remediation is warranted (is_actionable=false and no
	// workflow already identified -- the same condition investigator.go's
	// own internal guard treats as authoritative), phase_guard.go forces
	// phase2_blocked=true regardless of the declared interaction_mode. This
	// only ever tightens an existing autonomy grant (full_remediation /
	// full_remediation_autonomous); it never loosens interactive mode's
	// already-blocked default, and it never overrides a genuinely
	// actionable RCA.

	It("IT-AF-1918-001: forces phase2_blocked=true on full_remediation_autonomous when RCA is not actionable and has no workflow", func() {
		notActionable := false
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation_autonomous"}, map[string]any{
			"session_id": "sess-1918-a", "rr_id": "rr-1918-a", "status": "completed",
			"rca": map[string]any{
				"severity": "info", "rca_summary": "Problem self-resolved",
				"is_actionable": notActionable,
			},
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(true),
			"#1918: a not-actionable RCA with no workflow must force phase2_blocked=true even under full_remediation_autonomous, "+
				"independent of the model's own reading of the RCA narrative")
	})

	It("IT-AF-1918-002: forces phase2_blocked=true on full_remediation (not just autonomous) when RCA is not actionable", func() {
		notActionable := false
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "sess-1918-b", "rr_id": "rr-1918-b", "status": "completed",
			"rca": map[string]any{
				"severity": "info", "rca_summary": "Problem self-resolved",
				"is_actionable": notActionable,
			},
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(true), "#1918: the override applies to full_remediation too, not just the autonomous variant")
	})

	It("IT-AF-1918-003: does NOT override a genuinely actionable RCA under full_remediation_autonomous", func() {
		actionable := true
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation_autonomous"}, map[string]any{
			"session_id": "sess-1918-c", "rr_id": "rr-1918-c", "status": "completed",
			"rca": map[string]any{
				"severity": "critical", "rca_summary": "OOMKill",
				"is_actionable": actionable,
			},
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(false),
			"#1918: a genuinely actionable RCA must never be second-guessed by this gate")
	})

	It("IT-AF-1918-004: does NOT override when is_actionable=false but a workflow was already identified (defense-in-depth, mirrors investigator.go)", func() {
		notActionable := false
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation_autonomous"}, map[string]any{
			"session_id": "sess-1918-d", "rr_id": "rr-1918-d", "status": "completed",
			"rca": map[string]any{
				"severity": "warning", "rca_summary": "Contradictory RCA",
				"is_actionable": notActionable,
				"has_workflow":  true,
			},
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(false),
			"#1918: has_workflow=true must suppress the override, matching investigator.go's own "+
				"defense-in-depth guard (actionable=false && workflow_id==\"\")")
	})

	It("IT-AF-1918-005: does NOT change the already-blocked interactive default when is_actionable is absent", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-1918-e", "rr_id": "rr-1918-e", "status": "completed",
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(true), "interactive mode's own default already blocks phase 2 -- the gate must not need to intervene here")
	})

	It("IT-AF-1918-006: does NOT override full_remediation_autonomous when the response carries no rca payload at all", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation_autonomous"}, map[string]any{
			"session_id": "sess-1918-f", "rr_id": "rr-1918-f", "status": "completed",
		}, nil)

		blocked, err := state.Get(session.StateKeyPhase2Blocked)
		Expect(err).NotTo(HaveOccurred())
		Expect(blocked).To(Equal(false),
			"#1918: an absent rca payload must never be treated as a not-actionable signal -- only a genuine computed false")
	})
})

var _ = Describe("Phase Guard — ActiveContextRegistry Integration (BR-SESS-020, BR-SESS-022)", func() {
	var (
		registry *launcher.ActiveContextRegistry
		state    *mapState
		toolCtx  tool.Context
		before   func(tool.Context, tool.Tool, map[string]any) (map[string]any, error)
		after    func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)
	)

	BeforeEach(func() {
		registry = launcher.NewActiveContextRegistry(2*time.Hour, 10*time.Minute)
		state = newMapState()
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		toolCtx = statefulToolContext{
			fakeToolContext: fakeToolContext{Context: ctx},
			state:           state,
			sessionID:       "ctx-session-abc",
		}
		before, after = NewPhaseGuardWithRegistryForTest(registry)
	})

	It("UT-AF-SESS-020-020: Stores context in registry after successful investigate (SC-7)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		contextID, ok := registry.Get("alice")
		Expect(ok).To(BeTrue(), "Registry must store context after successful investigate")
		Expect(contextID).To(Equal("ctx-session-abc"),
			"Registry must store the SessionID from tool.Context")
	})

	It("UT-AF-SESS-020-021: Does NOT store context on investigate failure/error (SC-7)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"error": "investigation failed",
		}, nil)

		_, ok := registry.Get("alice")
		Expect(ok).To(BeFalse(),
			"Registry must NOT store context when investigate returns an error response")
	})

	It("UT-AF-SESS-020-022: Clears context on kubernaut_complete success (AC-2)", func() {
		registry.Set("alice", "ctx-session-abc")
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001",
		}, nil)

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_complete"}, nil, map[string]any{
			"status": "completed",
		}, nil)

		_, ok := registry.Get("alice")
		Expect(ok).To(BeFalse(),
			"Registry must be cleared after kubernaut_complete succeeds")
	})

	It("UT-AF-SESS-020-023: Clears context on kubernaut_cancel success (AC-2)", func() {
		registry.Set("alice", "ctx-session-abc")
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001",
		}, nil)

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_cancel"}, nil, map[string]any{
			"status": "cancelled",
		}, nil)

		_, ok := registry.Get("alice")
		Expect(ok).To(BeFalse(),
			"Registry must be cleared after kubernaut_cancel succeeds")
	})

	It("UT-AF-SESS-020-024: Does NOT clear context on complete/cancel failure (AC-2)", func() {
		registry.Set("alice", "ctx-session-abc")

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_complete"}, nil, map[string]any{
			"error": "complete failed",
		}, nil)

		contextID, ok := registry.Get("alice")
		Expect(ok).To(BeTrue(),
			"Registry must NOT be cleared when complete returns an error")
		Expect(contextID).To(Equal("ctx-session-abc"))
	})

	It("UT-AF-SESS-020-025: No-op when registry is nil (backward compat)", func() {
		beforeNil, afterNil := NewPhaseGuardWithRegistryForTest(nil)
		// DD-AF-011 (#1899): declares full_remediation to bypass the new
		// consent gate, since this test is about registry backward-compat.
		_, _ = afterNil(toolCtx, fakeTool{name: "kubernaut_investigate"}, map[string]any{"interaction_mode": "full_remediation"}, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		result, err := beforeNil(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"Phase guard must still function when registry is nil")
	})

	It("UT-AF-SESS-020-026: Phase guard blocking still works with registry present", func() {
		result, err := before(toolCtx, fakeTool{name: "kubernaut_discover_workflows"}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(),
			"Phase guard must still block MCP-dependent tools before investigate when registry is present")
	})

	It("UT-AF-1496-001: Clears context on kubernaut_complete_no_action success (#1496, BR-SESS-022)", func() {
		registry.Set("alice", "ctx-session-abc")
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001",
		}, nil)

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_complete_no_action"}, nil, map[string]any{
			"status": "completed_no_action",
		}, nil)

		_, ok := registry.Get("alice")
		Expect(ok).To(BeFalse(),
			"Registry must be cleared after kubernaut_complete_no_action succeeds (#1496)")
	})

	It("UT-AF-1496-002: Does NOT clear context on kubernaut_complete_no_action failure (#1496)", func() {
		registry.Set("alice", "ctx-session-abc")

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_complete_no_action"}, nil, map[string]any{
			"error": "no active session",
		}, nil)

		contextID, ok := registry.Get("alice")
		Expect(ok).To(BeTrue(),
			"Registry must NOT be cleared when kubernaut_complete_no_action returns an error")
		Expect(contextID).To(Equal("ctx-session-abc"))
	})

	It("UT-AF-1496-003: kubernaut_complete_no_action does NOT refresh idle timer (#1496)", func() {
		shortIdleRegistry := launcher.NewActiveContextRegistry(2*time.Hour, 200*time.Millisecond)
		shortIdleRegistry.Set("alice", "ctx-session-abc")
		_, afterShort := NewPhaseGuardWithRegistryForTest(shortIdleRegistry)

		time.Sleep(50 * time.Millisecond)

		_, _ = afterShort(toolCtx, fakeTool{name: "kubernaut_complete_no_action"}, nil, map[string]any{
			"status": "completed_no_action",
		}, nil)

		// Terminal tools clear (not refresh). After clearing, Get must return false.
		_, ok := shortIdleRegistry.Get("alice")
		Expect(ok).To(BeFalse(),
			"terminal tools must clear the registry, not refresh the idle timer (#1496)")
	})

	It("UT-AF-1912-001: Clears driverActive on kubernaut_complete success, alongside the registry (#1912)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		active, err := state.Get(session.StateKeyDriverActive)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(Equal(true), "precondition: driverActive must be true after a successful investigate")

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_complete"}, nil, map[string]any{
			"status": "completed",
		}, nil)

		active, err = state.Get(session.StateKeyDriverActive)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(Equal(false),
			"driverActive must be cleared after kubernaut_complete succeeds -- a stale true leaves reinvocation incorrectly eligible (#1912)")
	})

	It("UT-AF-1912-002: Clears driverActive on kubernaut_cancel success, alongside the registry (#1912)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_cancel"}, nil, map[string]any{
			"status": "cancelled",
		}, nil)

		active, err := state.Get(session.StateKeyDriverActive)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(Equal(false),
			"driverActive must be cleared after kubernaut_cancel succeeds -- a stale true leaves reinvocation incorrectly eligible (#1912)")
	})

	It("UT-AF-1912-003: Does NOT clear driverActive on kubernaut_complete failure (#1912)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_complete"}, nil, map[string]any{
			"error": "complete failed",
		}, nil)

		active, err := state.Get(session.StateKeyDriverActive)
		Expect(err).NotTo(HaveOccurred())
		Expect(active).To(Equal(true),
			"driverActive must NOT be cleared when kubernaut_complete returns an error -- the driver session is still legitimately active")
	})

	It("UT-AF-1446-007: AU-3 — Refresh called on successful non-entry/non-terminal tool call (#1446)", func() {
		registry.Set("alice", "ctx-session-abc")

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		shortIdleRegistry := launcher.NewActiveContextRegistry(2*time.Hour, 200*time.Millisecond)
		shortIdleRegistry.Set("alice", "ctx-session-abc")
		_, afterShort := NewPhaseGuardWithRegistryForTest(shortIdleRegistry)

		time.Sleep(50 * time.Millisecond)

		_, _ = afterShort(toolCtx, fakeTool{name: "kubectl_get"}, nil, map[string]any{
			"result": "pod/nginx Running",
		}, nil)

		time.Sleep(100 * time.Millisecond)

		contextID, ok := shortIdleRegistry.Get("alice")
		Expect(ok).To(BeTrue(),
			"AU-3: successful non-entry tool call must refresh idle timer to keep session alive for audit scope accuracy")
		Expect(contextID).To(Equal("ctx-session-abc"))
	})

	It("UT-AF-1446-008: AU-3 — Refresh NOT called on failed tool call (#1446)", func() {
		shortIdleRegistry := launcher.NewActiveContextRegistry(2*time.Hour, 200*time.Millisecond)
		shortIdleRegistry.Set("alice", "ctx-session-abc")
		_, afterShort := NewPhaseGuardWithRegistryForTest(shortIdleRegistry)

		time.Sleep(50 * time.Millisecond)

		_, _ = afterShort(toolCtx, fakeTool{name: "kubectl_get"}, nil, map[string]any{
			"error": "forbidden",
		}, nil)

		time.Sleep(250 * time.Millisecond)

		_, ok := shortIdleRegistry.Get("alice")
		Expect(ok).To(BeFalse(),
			"AU-3: failed tool calls must not extend session lifetime — prevents phantom sessions from corrupting audit scope boundaries")
	})
})
