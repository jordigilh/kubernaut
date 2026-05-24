package launcher_test

import (
	"context"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("StreamingExecutor", func() {
	var (
		inner    *mockExecutor
		executor a2asrv.AgentExecutor
	)

	BeforeEach(func() {
		inner = &mockExecutor{}
		executor = launcher.NewStreamingExecutor(inner, nil, nil)
	})

	Describe("Execute", func() {
		It("UT-AF-1258-001: delegates to inner executor", func() {
			reqCtx := &a2asrv.RequestContext{
				TaskID: "task-001",
			}
			queue := &fakeQueue{}

			err := executor.Execute(context.Background(), reqCtx, queue)
			Expect(err).NotTo(HaveOccurred())
			Expect(inner.executeCalled).To(BeTrue())
			Expect(inner.lastReqCtx).To(Equal(reqCtx))
		})

		It("UT-AF-1258-002: stores bridge in context passed to inner executor", func() {
			reqCtx := &a2asrv.RequestContext{
				TaskID: "task-bridge-001",
			}
			queue := &fakeQueue{}

			err := executor.Execute(context.Background(), reqCtx, queue)
			Expect(err).NotTo(HaveOccurred())
			Expect(inner.lastCtxHasBridge).To(BeTrue())
		})
	})

	Describe("Cancel", func() {
		It("UT-AF-1258-003: delegates to inner executor", func() {
			reqCtx := &a2asrv.RequestContext{
				TaskID: "task-cancel-001",
			}
			queue := &fakeQueue{}

			err := executor.Cancel(context.Background(), reqCtx, queue)
			Expect(err).NotTo(HaveOccurred())
			Expect(inner.cancelCalled).To(BeTrue())
		})
	})

	Describe("Cleanup", func() {
		It("UT-AF-1258-004: delegates to inner executor when it implements AgentExecutionCleaner", func() {
			cleanerInner := &mockExecutorWithCleaner{}
			exec := launcher.NewStreamingExecutor(cleanerInner, nil, nil)

			reqCtx := &a2asrv.RequestContext{TaskID: "task-cleanup-001"}

			cleaner, ok := interface{}(exec).(a2asrv.AgentExecutionCleaner)
			Expect(ok).To(BeTrue(), "StreamingExecutor should implement AgentExecutionCleaner")
			cleaner.Cleanup(context.Background(), reqCtx, nil, nil)
			Expect(cleanerInner.cleanupCalled).To(BeTrue())
		})

		It("UT-AF-1258-005: nil-safe when inner lacks AgentExecutionCleaner", func() {
			cleaner, ok := interface{}(executor).(a2asrv.AgentExecutionCleaner)
			Expect(ok).To(BeTrue(), "StreamingExecutor should always implement AgentExecutionCleaner")
			Expect(func() {
				cleaner.Cleanup(context.Background(), &a2asrv.RequestContext{}, nil, nil)
			}).NotTo(Panic())
		})
	})
})

// mockExecutor implements a2asrv.AgentExecutor for testing delegation.
type mockExecutor struct {
	executeCalled    bool
	cancelCalled     bool
	lastReqCtx       *a2asrv.RequestContext
	lastCtxHasBridge bool
}

func (m *mockExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, _ eventqueue.Queue) error {
	m.executeCalled = true
	m.lastReqCtx = reqCtx
	bridge := launcher.EventBridgeFromContext(ctx)
	m.lastCtxHasBridge = bridge != nil
	return nil
}

func (m *mockExecutor) Cancel(_ context.Context, _ *a2asrv.RequestContext, _ eventqueue.Queue) error {
	m.cancelCalled = true
	return nil
}

// mockExecutorWithCleaner implements both AgentExecutor and AgentExecutionCleaner.
type mockExecutorWithCleaner struct {
	mockExecutor
	cleanupCalled bool
}

func (m *mockExecutorWithCleaner) Cleanup(_ context.Context, _ *a2asrv.RequestContext, _ a2a.SendMessageResult, _ error) {
	m.cleanupCalled = true
}

// BR-AUDIT-AU6: Audit Review — the audit trail MUST record stream lifecycle
// events (open/close) so that security analysts can reconstruct the timeline
// of an A2A streaming session during incident forensics. Without these events,
// there is no way to determine when a stream was active or whether it terminated
// normally vs. due to error.
var _ = Describe("StreamingExecutor — AU-6 Audit Lifecycle", func() {
	It("UT-AF-1258-040: stream_opened is audited when execution begins (forensic timeline start)", func() {
		inner := &mockExecutor{}
		spy := &auditSpy{}
		executor := launcher.NewStreamingExecutor(inner, spy, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-forensic-001"}
		queue := &fakeQueue{}

		err := executor.Execute(context.Background(), reqCtx, queue)
		Expect(err).NotTo(HaveOccurred())

		openEvents := spy.eventsOfType(audit.EventA2AStreamOpened)
		Expect(openEvents).To(HaveLen(1),
			"AU-6 violation: no stream_opened event — analysts cannot determine session start time")
		Expect(openEvents[0].Detail["task_id"]).To(Equal("task-forensic-001"),
			"AU-6: audit event must identify the A2A task for correlation")
	})

	It("UT-AF-1258-041: stream_closed is audited after normal completion (forensic timeline end)", func() {
		inner := &mockExecutor{}
		spy := &auditSpy{}
		executor := launcher.NewStreamingExecutor(inner, spy, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-forensic-002"}
		queue := &fakeQueue{}

		err := executor.Execute(context.Background(), reqCtx, queue)
		Expect(err).NotTo(HaveOccurred())

		closeEvents := spy.eventsOfType(audit.EventA2AStreamClosed)
		Expect(closeEvents).To(HaveLen(1),
			"AU-6 violation: no stream_closed event — analysts cannot confirm session ended cleanly")
		Expect(closeEvents[0].Detail["task_id"]).To(Equal("task-forensic-002"))
		Expect(closeEvents[0].Detail).NotTo(HaveKey("error"),
			"successful completion should not carry an error flag")
	})

	It("UT-AF-1258-042: stream_closed records error disposition for failure analysis", func() {
		inner := &mockExecutorFailing{}
		spy := &auditSpy{}
		executor := launcher.NewStreamingExecutor(inner, spy, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-forensic-003"}
		queue := &fakeQueue{}

		err := executor.Execute(context.Background(), reqCtx, queue)
		Expect(err).To(HaveOccurred())

		closeEvents := spy.eventsOfType(audit.EventA2AStreamClosed)
		Expect(closeEvents).To(HaveLen(1),
			"AU-6 violation: stream_closed must be emitted even on failure for forensics")
		Expect(closeEvents[0].Detail["error"]).To(Equal("true"),
			"AU-6: error flag enables analysts to filter abnormal terminations in SIEM queries")
	})

	It("UT-AF-1258-043: nil auditor does not prevent execution (graceful degradation)", func() {
		inner := &mockExecutor{}
		executor := launcher.NewStreamingExecutor(inner, nil, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-no-auditor"}
		queue := &fakeQueue{}

		Expect(func() {
			_ = executor.Execute(context.Background(), reqCtx, queue)
		}).NotTo(Panic())
		Expect(inner.executeCalled).To(BeTrue(),
			"execution must proceed regardless of auditor availability")
	})
})

// BR-AUDIT-AU2/AU3: Auditable Events — the audit trail MUST correctly
// identify whether a request used message/send (synchronous) or message/stream
// (progressive). This distinction is critical for:
// 1. Forensic analysis: correlating real-time stream events vs batch responses
// 2. Compliance reporting: streaming sessions have different retention requirements
// 3. Capacity planning: streaming consumes long-lived connections
var _ = Describe("resolveA2AMethod (AU-2/AU-3 Audit Method Identification)", func() {
	It("UT-AF-1258-044: defaults to message/send when no CallContext exists", func() {
		method := launcher.ResolveA2AMethodForTest(context.Background())
		Expect(method).To(Equal("message/send"),
			"AU-2: absent CallContext must default to message/send (non-streaming fallback)")
	})

	It("UT-AF-1258-045: defaults to message/send for CallContext with unset method", func() {
		ctx, _ := a2asrv.WithCallContext(context.Background(), nil)
		method := launcher.ResolveA2AMethodForTest(ctx)
		Expect(method).To(Equal("message/send"),
			"AU-2: empty method field must be treated as message/send (pre-dispatch state)")
	})
})

// mockExecutorFailing always returns an error from Execute.
type mockExecutorFailing struct{}

func (m *mockExecutorFailing) Execute(_ context.Context, _ *a2asrv.RequestContext, _ eventqueue.Queue) error {
	return errMockFailure
}

func (m *mockExecutorFailing) Cancel(_ context.Context, _ *a2asrv.RequestContext, _ eventqueue.Queue) error {
	return nil
}

var errMockFailure = context.DeadlineExceeded

// auditSpy captures emitted audit events for behavioral assertion.
type auditSpy struct {
	events []*audit.Event
}

func (s *auditSpy) Emit(_ context.Context, event *audit.Event) {
	s.events = append(s.events, event)
}

func (s *auditSpy) eventsOfType(t audit.EventType) []*audit.Event {
	var result []*audit.Event
	for _, e := range s.events {
		if e.Type == t {
			result = append(result, e)
		}
	}
	return result
}
