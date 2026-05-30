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
})
