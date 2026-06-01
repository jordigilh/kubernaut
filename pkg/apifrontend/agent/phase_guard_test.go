package agent

import (
	"context"
	"iter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// statefulToolContext extends fakeToolContext with a working session.State
// for phase guard testing. State persists across calls within the same test.
type statefulToolContext struct {
	fakeToolContext
	state *mapState
}

func (s statefulToolContext) State() adksession.State { return s.state }

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
		state    *mapState
		toolCtx  tool.Context
		before   func(tool.Context, tool.Tool, map[string]any) (map[string]any, error)
		after    func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)
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
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
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
		// Simulate successful investigation via AfterToolCallback
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
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
		// Simulate successful investigation storing rr_id
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
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
		// Store one rr_id in state
		_, _ = after(toolCtx, fakeTool{name: "kubernaut_investigate"}, nil, map[string]any{
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
})
