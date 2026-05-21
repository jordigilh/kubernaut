package agent

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/agent"
	adkmemory "google.golang.org/adk/memory"
	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// fakeToolContext satisfies tool.Context for unit testing callbacks.
// Only context.Value is exercised; all ADK-specific methods return zero values.
type fakeToolContext struct {
	context.Context
}

func (fakeToolContext) UserContent() *genai.Content                                          { return nil }
func (fakeToolContext) InvocationID() string                                                 { return "" }
func (fakeToolContext) AgentName() string                                                    { return "" }
func (fakeToolContext) ReadonlyState() adksession.ReadonlyState                              { return nil }
func (fakeToolContext) UserID() string                                                       { return "" }
func (fakeToolContext) AppName() string                                                      { return "" }
func (fakeToolContext) SessionID() string                                                    { return "" }
func (fakeToolContext) Branch() string                                                       { return "" }
func (fakeToolContext) Artifacts() agent.Artifacts                                           { return nil }
func (fakeToolContext) State() adksession.State                                              { return nil }
func (fakeToolContext) FunctionCallID() string                                               { return "" }
func (fakeToolContext) Actions() *adksession.EventActions                                    { return nil }
func (fakeToolContext) SearchMemory(context.Context, string) (*adkmemory.SearchResponse, error) {
	return nil, nil
}
func (fakeToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }
func (fakeToolContext) RequestConfirmation(string, any) error                { return nil }

// Compile-time interface satisfaction check.
var _ tool.Context = fakeToolContext{}

// fakeTool satisfies tool.Tool for unit testing callbacks.
type fakeTool struct {
	name string
}

func (t fakeTool) Name() string        { return t.name }
func (fakeTool) Description() string   { return "" }
func (fakeTool) IsLongRunning() bool   { return false }

// capturingAuditor captures audit events for test assertions.
type capturingAuditor struct {
	events []*audit.Event
}

func (a *capturingAuditor) Emit(_ context.Context, event *audit.Event) {
	a.events = append(a.events, event)
}

var _ = Describe("newAuditToolCallback (#1189)", func() {
	var (
		auditor  *capturingAuditor
		callback func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)
	)

	BeforeEach(func() {
		auditor = &capturingAuditor{}
		callback = newAuditToolCallback(auditor)
	})

	It("UT-AF-1189-001: includes a2a_task_id when session CreateContext is present", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-abc-123"})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-alice"})

		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "af_list_events"}, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].Detail["a2a_task_id"]).To(Equal("task-abc-123"))
	})

	It("UT-AF-1189-002: omits a2a_task_id when no session CreateContext", func() {
		ctx := context.Background()
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-bob"})

		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "af_list_events"}, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasTaskID := auditor.events[0].Detail["a2a_task_id"]
		Expect(hasTaskID).To(BeFalse())
	})

	It("UT-AF-1189-003: includes rr_id when af_create_rr succeeds with rr_id output", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-xyz"})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-carol"})

		output := map[string]any{"rr_id": "rr-prod-001"}
		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "af_create_rr"}, nil, output, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].Detail["rr_id"]).To(Equal("rr-prod-001"))
		Expect(auditor.events[0].Detail["a2a_task_id"]).To(Equal("task-xyz"))
	})

	It("UT-AF-1189-004: omits rr_id when af_create_rr fails", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-fail"})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-dave"})

		tc := fakeToolContext{Context: ctx}
		toolErr := fmt.Errorf("creation failed")
		_, err := callback(tc, fakeTool{name: "af_create_rr"}, nil, nil, toolErr)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasRRID := auditor.events[0].Detail["rr_id"]
		Expect(hasRRID).To(BeFalse())
		Expect(auditor.events[0].Detail["tool_outcome"]).To(Equal("failure"))
	})

	It("UT-AF-1189-005: omits rr_id for non-af_create_rr tools", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-other"})

		output := map[string]any{"rr_id": "should-not-appear"}
		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "af_list_events"}, nil, output, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasRRID := auditor.events[0].Detail["rr_id"]
		Expect(hasRRID).To(BeFalse())
	})

	It("UT-AF-1189-006: records user identity in audit event", func() {
		ctx := context.Background()
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-eve"})

		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "af_get_pods"}, map[string]any{"namespace": "prod"}, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].UserID).To(Equal("sre-eve"))
		Expect(auditor.events[0].Detail["namespace"]).To(Equal("prod"))
	})
})
