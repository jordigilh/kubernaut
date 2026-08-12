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

// --- #2023: harness-enforced content-grounding guard for present_decision ---
//
// QE reported the model fabricating a plausible-sounding RCA/audit narrative
// in kubernaut_present_decision's summary/rca fields when the underlying
// kubernaut_investigate call produced no real content (rejected for scope,
// a tool error, session_active, or -- pre-#2022-secondary-fix -- an empty
// conversation). This guard tracks whether the most recent kubernaut_
// investigate call actually produced groundable content and, if not,
// forcibly overwrites present_decision's summary/rca/options with a fixed,
// honest "no data" payload before the tool executes -- present_decision
// itself still runs afterward, so the AU-3 structured-artifact mandate
// (#1408) is preserved; only a fabricated narrative is blocked, never the
// artifact. Absence of any prior recorded state fails closed to "not
// grounded", mirroring #2022's own safe-default posture.
var _ = Describe("Phase Guard — Content Grounding Guard (#2023)", func() {
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

	fabricatedArgs := func() map[string]any {
		return map[string]any{
			"session_id": "sess-2023",
			"summary":    "The Deployment was rolled back at 14:32 after three consecutive OOMKills.",
			"rca": map[string]any{
				"severity": "critical", "confidence": 0.92,
				"causal_chain": []any{"OOMKill", "CrashLoopBackOff"},
			},
			"options": []any{map[string]any{"workflow_id": "wf-rollback", "name": "Rollback"}},
		}
	}

	It("UT-AF-2023-001: overrides present_decision content when kubernaut_investigate was never called (fail-closed default)", func() {
		args := fabricatedArgs()
		result, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(), "present_decision must still be allowed to execute (AU-3 artifact mandate)")

		summary, _ := args["summary"].(string)
		Expect(summary).NotTo(ContainSubstring("OOMKill"),
			"fabricated narrative must be replaced when no investigation ever ran")
		Expect(summary).To(ContainSubstring("No investigation content is available"))
		Expect(args["options"]).To(BeEmpty(), "options must be forced empty alongside the overridden summary")
	})

	It("UT-AF-2023-002: overrides present_decision content when kubernaut_investigate was rejected for scope (#2022)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"status": "unmanaged", "error": "resource is outside Kubernaut's management scope",
		}, nil)

		args := fabricatedArgs()
		result, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())

		summary, _ := args["summary"].(string)
		Expect(summary).To(ContainSubstring("No investigation content is available"),
			"a scope-rejected investigation has no RCA to ground a summary in")
	})

	It("UT-AF-2023-003: overrides present_decision content when kubernaut_investigate returned a generic tool error", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"error": "no_conversation_context: session had no conversation history",
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		summary, _ := args["summary"].(string)
		Expect(summary).To(ContainSubstring("No investigation content is available"))
		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue(),
			"rca must remain present (not deleted) -- it is a required property in present_decision's "+
				"ADK schema (#1396); deleting it makes ADK's own validation reject the call before the "+
				"AU-3 artifact can ever be emitted")
		Expect(rca["severity"]).To(BeEmpty(), "rca payload must be cleared, not left carrying invented fields")
		Expect(rca["target"]).To(BeEmpty())
	})

	It("UT-AF-2023-004: overrides present_decision content when kubernaut_investigate returned session_active", func() {
		// session_active means a DIFFERENT user is already driving; this
		// caller has no fresh RCA of its own to report even though
		// session_active has its own dedicated fallback card (#1922).
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"status": "session_active", "error": "investigation already in progress, driven by bob",
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		summary, _ := args["summary"].(string)
		Expect(summary).To(ContainSubstring("No investigation content is available"))
	})

	It("UT-AF-2023-005: does NOT override present_decision content after a successful investigate with a real summary", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-g", "status": "completed",
			"summary": "OOMKilled 3 times in the last 10 minutes; memory limit is too low for observed usage.",
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		Expect(args["summary"]).To(Equal(fabricatedArgs()["summary"]),
			"a genuinely grounded summary must pass through untouched")
		Expect(args["options"]).To(HaveLen(1), "options must pass through untouched when content is grounded")
	})

	It("UT-AF-2023-006: does NOT override present_decision content after a successful investigate with only an rca payload (no summary)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-h", "status": "completed",
			"rca": map[string]any{"severity": "warning", "is_actionable": false},
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		Expect(args["summary"]).To(Equal(fabricatedArgs()["summary"]),
			"a non-nil rca payload counts as grounded content even without a summary string")
	})

	It("UT-AF-2023-007: handles a nil args map without panicking", func() {
		Expect(func() {
			_, _ = before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, nil)
		}).NotTo(Panic())
	})

	It("UT-AF-2023-008: present_decision is never hard-rejected by this guard, even when overriding content", func() {
		args := fabricatedArgs()
		result, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"the guard must mutate args in place, never short-circuit the call -- the AU-3 artifact must still be emitted")
	})

	It("UT-AF-2023-010: overwrites present_decision's rca argument with KA's own reported RCA, discarding the LLM's transcription", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-rca-1", "status": "completed",
			"summary": "Real investigation summary.",
			"rca": map[string]any{
				"severity": "warning", "confidence": 0.55,
				"causal_chain":     []any{"MemoryPressure", "Evicted"},
				"target":           "pod/real-target",
				"total_tool_calls": 7, "total_llm_turns": 3,
			},
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		// #2110 (v1.6 clone #2111): rca must be a map[string]any, NOT a
		// *tools.RCAData struct pointer -- a2a-go's task manager gob-encodes
		// every SSE artifact for its deep-copy fan-out, and tools.RCAData is
		// never gob.Register'd anywhere in this repo, so a raw struct
		// pointer here crashed every grounded present_decision call in
		// production ("gob: type not registered for interface:
		// tools.RCAData"). map[string]any IS safe: a2a-go's own
		// internal/taskstore/store.go registers it at init().
		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue(), "rca must be overwritten with a gob-safe map[string]any, not a *tools.RCAData struct pointer (#2110)")
		Expect(rca["severity"]).To(Equal("warning"), "severity must come from KA's own report, not the LLM's fabricated 'critical'")
		Expect(rca["confidence"]).To(Equal(0.55))
		Expect(rca["target"]).To(Equal("pod/real-target"))
		Expect(rca["causal_chain"]).To(Equal([]string{"MemoryPressure", "Evicted"}))
		Expect(rca["tool_calls_count"]).To(Equal(7), "total_tool_calls must be renamed to RCAData's tool_calls_count field")
		Expect(rca["llm_turns"]).To(Equal(3), "total_llm_turns must be renamed to RCAData's llm_turns field")
	})

	It("UT-AF-2023-011: leaves the rca argument untouched when investigate reported no structured rca payload (summary-only grounding)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-rca-2", "status": "completed",
			"summary": "OOMKilled 3 times in the last 10 minutes; memory limit is too low for observed usage.",
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["severity"]).To(Equal(fabricatedArgs()["rca"].(map[string]any)["severity"]),
			"with no structured rca to pass through, the harness has nothing authoritative to substitute for severity/confidence/causal_chain")
		Expect(rca["confidence"]).To(Equal(fabricatedArgs()["rca"].(map[string]any)["confidence"]))
		Expect(rca["causal_chain"]).To(Equal(fabricatedArgs()["rca"].(map[string]any)["causal_chain"]))
		Expect(rca["tool_calls_count"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
		Expect(rca["llm_turns"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
	})

	It("UT-AF-2023-012: clears a stale rca pass-through after a later investigate call that reported no rca", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-rca-3a", "status": "completed",
			"summary": "First investigation.",
			"rca":     map[string]any{"severity": "critical", "confidence": 0.9},
		}, nil)

		// A second, re-checked investigate call is grounded via summary only.
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-rca-3b", "status": "completed",
			"summary": "Second investigation, no structured RCA this time.",
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["severity"]).To(Equal(fabricatedArgs()["rca"].(map[string]any)["severity"]),
			"the first call's rca must not leak into a present_decision grounded by the second, rca-less call")
		Expect(rca["tool_calls_count"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
		Expect(rca["llm_turns"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
	})

	It("UT-AF-2023-013: overrides present_decision content when kubernaut_investigate's shadow-agent alignment verdict is not aligned, even with summary/rca present", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-align-1", "status": "completed",
			"summary": "Looks like a real investigation summary.",
			"rca":     map[string]any{"severity": "critical", "confidence": 0.9},
			"alignment_verdict": map[string]any{
				"result": "suspicious", "circuit_breaker_activated": true,
			},
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		summary, _ := args["summary"].(string)
		Expect(summary).To(ContainSubstring("No investigation content is available"),
			"KA's own shadow-agent flagging the RCA as ungrounded must override present_decision content, "+
				"even though summary/rca look superficially legitimate")
	})

	It("UT-AF-2023-014: does NOT override present_decision content when the shadow-agent alignment verdict is aligned", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-align-2", "status": "completed",
			"summary":           "A genuinely grounded summary.",
			"alignment_verdict": map[string]any{"result": "aligned"},
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		Expect(args["summary"]).To(Equal(fabricatedArgs()["summary"]),
			"an aligned shadow-agent verdict must not itself trigger the override")
	})

	It("IT-AF-2023-009: grounded state correctly flips to false after a second, failed investigate call following an earlier successful one", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2023-i-1", "status": "completed",
			"summary": "First investigation found a real root cause.",
		}, nil)

		argsFirst := fabricatedArgs()
		_, _ = before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, argsFirst)
		Expect(argsFirst["summary"]).To(Equal(fabricatedArgs()["summary"]),
			"sanity check: first present_decision call must have passed through ungrounded")

		// A second investigate call for a different/re-checked target fails.
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"status": "unmanaged", "error": "resource is outside Kubernaut's management scope",
		}, nil)

		argsSecond := fabricatedArgs()
		_, _ = before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, argsSecond)
		summary, _ := argsSecond["summary"].(string)
		Expect(summary).To(ContainSubstring("No investigation content is available"),
			"stale grounded=true from the FIRST investigate must not leak into the SECOND, failed attempt")
	})

	It("UT-AF-2068-001: does NOT overwrite present_decision's rca with a Provisional severity-triage fallback (RCA never genuinely investigated by KA)", func() {
		// #2068 spike: reproduces ka_investigate_mcp.go's severity-triage
		// fallback shape (Severity/Confidence only, no causal_chain/target/
		// tool_calls_count/llm_turns -- KA never ran a real investigation).
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2068-1", "status": "completed",
			"summary": "Severity assessed from resource metadata (full investigation pending)",
			"rca": map[string]any{
				"severity": "warning", "confidence": 0.6, "provisional": true,
			},
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["severity"]).To(Equal(fabricatedArgs()["rca"].(map[string]any)["severity"]),
			"#2068: a Provisional (AF-synthesized severity-triage guess, not a genuine KA finding) rca must "+
				"not clobber present_decision's own rca -- same treatment as 'no structured rca at all' (UT-AF-2023-011)")
		Expect(rca["tool_calls_count"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
		Expect(rca["llm_turns"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
	})

	It("UT-AF-2068-004: still overwrites present_decision's rca when kubernaut_investigate's rca is genuinely KA-reported (Provisional unset/false)", func() {
		// Companion to UT-AF-2023-010, restated in #2068 terms: a real,
		// non-Provisional rca must keep working exactly as before -- #2068
		// must gate ONLY on Provisional=true, not regress the original
		// #2023 caching behavior for a genuinely-investigated result.
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "sess-2068-4", "status": "completed",
			"summary": "Real investigation summary.",
			"rca": map[string]any{
				"severity": "warning", "confidence": 0.55,
				"causal_chain": []any{"MemoryPressure", "Evicted"},
			},
		}, nil)

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		// #2110 (v1.6 clone #2111): map[string]any, not *tools.RCAData -- see
		// UT-AF-2071-014's comment above for why.
		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["severity"]).To(Equal("warning"), "a non-Provisional rca must still overwrite present_decision's rca (#2023's original guarantee)")
	})

	It("UT-AF-2068-005: treats a malformed (non-object) rca payload the same as no rca at all, without panicking", func() {
		// decodeInvestigateRCA must fail closed: a type-mismatched "rca"
		// value (here a bare string instead of an object) must not cache
		// anything usable, and must never panic the after-callback.
		Expect(func() {
			_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
				"session_id": "sess-2068-5", "status": "completed",
				"summary": "Real investigation summary.",
				"rca":     "not-an-object",
			}, nil)
		}).NotTo(Panic())

		args := fabricatedArgs()
		_, err := before(toolCtx, fakeTool{name: "kubernaut_present_decision"}, args)
		Expect(err).NotTo(HaveOccurred())

		rca, ok := args["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["severity"]).To(Equal(fabricatedArgs()["rca"].(map[string]any)["severity"]),
			"a malformed rca payload has nothing authoritative to substitute, same as UT-AF-2023-011")
		Expect(rca["tool_calls_count"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
		Expect(rca["llm_turns"]).To(Equal(0),
			"#2073: backfilled with an honest zero rather than left for the LLM to fabricate a plausible-looking count")
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
		if err == nil {
			Expect(active).To(Equal(false),
				"driverActive must be cleared after kubernaut_complete succeeds -- a stale true leaves reinvocation incorrectly eligible (#1912)")
		}
		// A missing key (ErrStateKeyNotExist) is equally acceptable: NeedsReinvocationCtx's
		// driverActive(state) helper treats "absent" and "false" identically.
	})

	It("UT-AF-1912-002: Clears driverActive on kubernaut_cancel success, alongside the registry (#1912)", func() {
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_cancel"}, nil, map[string]any{
			"status": "cancelled",
		}, nil)

		active, err := state.Get(session.StateKeyDriverActive)
		if err == nil {
			Expect(active).To(Equal(false),
				"driverActive must be cleared after kubernaut_cancel succeeds -- a stale true leaves reinvocation incorrectly eligible (#1912)")
		}
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

	It("UT-AF-1446-007: AU-3 — Refresh called on successful non-entry/non-terminal tool call (#1446)", func() {
		registry.Set("alice", "ctx-session-abc")

		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
			"session_id": "ka-sess-001", "rr_id": "rr-123",
		}, nil)

		clock := launcher.NewMockClock(time.Now())
		shortIdleRegistry := launcher.NewActiveContextRegistryWithClock(2*time.Hour, 200*time.Millisecond, clock)
		shortIdleRegistry.Set("alice", "ctx-session-abc")
		_, afterShort := NewPhaseGuardWithRegistryForTest(shortIdleRegistry)

		clock.Advance(50 * time.Millisecond)

		_, _ = afterShort(toolCtx, fakeTool{name: "kubectl_get"}, nil, map[string]any{
			"result": "pod/nginx Running",
		}, nil)

		clock.Advance(100 * time.Millisecond)

		contextID, ok := shortIdleRegistry.Get("alice")
		Expect(ok).To(BeTrue(),
			"AU-3: successful non-entry tool call must refresh idle timer to keep session alive for audit scope accuracy")
		Expect(contextID).To(Equal("ctx-session-abc"))
	})

	It("UT-AF-1446-008: AU-3 — Refresh NOT called on failed tool call (#1446)", func() {
		clock := launcher.NewMockClock(time.Now())
		shortIdleRegistry := launcher.NewActiveContextRegistryWithClock(2*time.Hour, 200*time.Millisecond, clock)
		shortIdleRegistry.Set("alice", "ctx-session-abc")
		_, afterShort := NewPhaseGuardWithRegistryForTest(shortIdleRegistry)

		clock.Advance(50 * time.Millisecond)

		_, _ = afterShort(toolCtx, fakeTool{name: "kubectl_get"}, nil, map[string]any{
			"error": "forbidden",
		}, nil)

		clock.Advance(250 * time.Millisecond)

		_, ok := shortIdleRegistry.Get("alice")
		Expect(ok).To(BeFalse(),
			"AU-3: failed tool calls must not extend session lifetime — prevents phantom sessions from corrupting audit scope boundaries")
	})
})
