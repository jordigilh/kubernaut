package launcher_test

import (
	"bytes"
	"context"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

var _ = Describe("StreamingExecutor", func() {
	var (
		inner    *mockExecutor
		executor a2asrv.AgentExecutor
	)

	BeforeEach(func() {
		inner = &mockExecutor{}
		executor = launcher.NewStreamingExecutor(inner, logr.Logger{}, nil, nil)
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
			exec := launcher.NewStreamingExecutor(cleanerInner, logr.Logger{}, nil, nil)

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

// BR-AUDIT-AU6: Audit Review — stream lifecycle events (open/close) are logged
// for forensic timeline reconstruction. These use structured logging (not audit
// store) because no OpenAPI payload schema exists yet in data-storage-v1.yaml.
// The A2A task lifecycle is already audited by buildBeforeExecuteCallback /
// buildAfterExecuteCallback.

// captureLogr returns a logr.Logger that writes to buf.
func captureLogr(buf *bytes.Buffer) logr.Logger {
	return funcr.New(func(prefix, args string) {
		_, _ = buf.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{})
}

var _ = Describe("StreamingExecutor — AU-6 Lifecycle Logging", func() {
	It("UT-AF-1258-040: stream opened is logged when execution begins (forensic timeline start)", func() {
		inner := &mockExecutor{}
		var buf bytes.Buffer
		logger := captureLogr(&buf)
		executor := launcher.NewStreamingExecutor(inner, logger, nil, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-forensic-001"}
		queue := &fakeQueue{}

		err := executor.Execute(context.Background(), reqCtx, queue)
		Expect(err).NotTo(HaveOccurred())

		logOutput := buf.String()
		Expect(logOutput).To(ContainSubstring("a2a stream opened"))
		Expect(logOutput).To(ContainSubstring("task-forensic-001"))
	})

	It("UT-AF-1258-041: stream closed is logged after normal completion (forensic timeline end)", func() {
		inner := &mockExecutor{}
		var buf bytes.Buffer
		logger := captureLogr(&buf)
		executor := launcher.NewStreamingExecutor(inner, logger, nil, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-forensic-002"}
		queue := &fakeQueue{}

		err := executor.Execute(context.Background(), reqCtx, queue)
		Expect(err).NotTo(HaveOccurred())

		logOutput := buf.String()
		Expect(logOutput).To(ContainSubstring("a2a stream closed"))
		Expect(logOutput).To(ContainSubstring("task-forensic-002"))
		Expect(logOutput).To(ContainSubstring(`"error"=false`))
	})

	It("UT-AF-1258-042: stream closed records error disposition for failure analysis", func() {
		inner := &mockExecutorFailing{}
		var buf bytes.Buffer
		logger := captureLogr(&buf)
		executor := launcher.NewStreamingExecutor(inner, logger, nil, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-forensic-003"}
		queue := &fakeQueue{}

		err := executor.Execute(context.Background(), reqCtx, queue)
		Expect(err).To(HaveOccurred())

		logOutput := buf.String()
		Expect(logOutput).To(ContainSubstring("a2a stream closed"))
		Expect(logOutput).To(ContainSubstring(`"error"=true`))
	})

	It("UT-AF-1258-043: zero-value logger does not prevent execution (graceful degradation)", func() {
		inner := &mockExecutor{}
		executor := launcher.NewStreamingExecutor(inner, logr.Logger{}, nil, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-no-logger"}
		queue := &fakeQueue{}

		Expect(func() {
			_ = executor.Execute(context.Background(), reqCtx, queue)
		}).NotTo(Panic())
		Expect(inner.executeCalled).To(BeTrue(),
			"execution must proceed regardless of logger availability")
	})
})

var _ = Describe("StreamingExecutor logr injection", func() {
	It("UT-AF-1274-006: constructor accepts logr.Logger and stores provided logger (BR-SESS-013)", func() {
		inner := &mockExecutor{}
		var logs []string
		testLogger := funcr.New(func(prefix, args string) {
			logs = append(logs, prefix+" "+args)
		}, funcr.Options{})
		exec := launcher.NewStreamingExecutor(inner, testLogger, nil, nil)
		launcher.StreamingExecutorLoggerForTest(exec).Info("stored logger")
		Expect(logs).To(ContainElement(ContainSubstring("stored logger")))
	})

	It("UT-AF-1274-007: logs stream open/close through logr (BR-SESS-013)", func() {
		inner := &mockExecutor{}
		var buf bytes.Buffer
		logger := captureLogr(&buf)
		exec := launcher.NewStreamingExecutor(inner, logger, nil, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-logr-001"}
		err := exec.Execute(context.Background(), reqCtx, &fakeQueue{})
		Expect(err).NotTo(HaveOccurred())

		logOutput := buf.String()
		Expect(logOutput).To(ContainSubstring("a2a stream opened"))
		Expect(logOutput).To(ContainSubstring("a2a stream closed"))
		Expect(logOutput).To(ContainSubstring("task-logr-001"))
	})
})

// BR-SESS-003 / SI-4: Disconnect detection — when the client's SSE connection
// is canceled, the StreamingExecutor must transition materialized sessions to
// Disconnected so the CRD reflects actual connection state.
var _ = Describe("StreamingExecutor — STREAM-03 Disconnect Detection (BR-SESS-003, SI-4)", func() {
	It("UT-AF-STREAM03-001: calls UpdatePhase(Disconnected) when ctx is canceled and session is materialized", func() {
		spu := &mockSessionPhaseUpdater{
			materialized: map[string]bool{"sess-abc": true},
		}
		inner := &mockExecutor{}
		exec := launcher.NewStreamingExecutor(inner, logr.Discard(), nil, spu)

		ctx, cancel := context.WithCancel(context.Background())
		sc := &session.CreateContext{SessionID: "sess-abc", TaskID: "task-1"}
		ctx = session.WithCreateContext(ctx, sc)
		cancel()

		reqCtx := &a2asrv.RequestContext{TaskID: "task-1", ContextID: "sess-abc"}
		err := exec.Execute(ctx, reqCtx, &fakeQueue{})
		Expect(err).NotTo(HaveOccurred())

		Expect(spu.updatePhaseCalls).To(HaveLen(1))
		Expect(spu.updatePhaseCalls[0].Phase).To(Equal(string(isv1alpha1.SessionPhaseDisconnected)))
		Expect(spu.updatePhaseCalls[0].SessionID).To(Equal("sess-abc"))
		Expect(spu.updatePhaseCalls[0].Message).To(ContainSubstring("disconnect"))
	})

	It("UT-AF-STREAM03-002: does NOT call UpdatePhase when session is not materialized", func() {
		spu := &mockSessionPhaseUpdater{
			materialized: map[string]bool{},
		}
		inner := &mockExecutor{}
		exec := launcher.NewStreamingExecutor(inner, logr.Discard(), nil, spu)

		ctx, cancel := context.WithCancel(context.Background())
		sc := &session.CreateContext{SessionID: "sess-unmaterialized", TaskID: "task-2"}
		ctx = session.WithCreateContext(ctx, sc)
		cancel()

		reqCtx := &a2asrv.RequestContext{TaskID: "task-2", ContextID: "sess-unmaterialized"}
		_ = exec.Execute(ctx, reqCtx, &fakeQueue{})
		Expect(spu.updatePhaseCalls).To(BeEmpty())
	})

	It("UT-AF-STREAM03-003: does NOT call UpdatePhase on normal (non-canceled) completion", func() {
		spu := &mockSessionPhaseUpdater{
			materialized: map[string]bool{"sess-ok": true},
		}
		inner := &mockExecutor{}
		exec := launcher.NewStreamingExecutor(inner, logr.Discard(), nil, spu)

		ctx := context.Background()
		sc := &session.CreateContext{SessionID: "sess-ok", TaskID: "task-3"}
		ctx = session.WithCreateContext(ctx, sc)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-3", ContextID: "sess-ok"}
		_ = exec.Execute(ctx, reqCtx, &fakeQueue{})
		Expect(spu.updatePhaseCalls).To(BeEmpty())
	})

	It("UT-AF-STREAM03-004: nil sessionSvc does not panic on disconnect", func() {
		inner := &mockExecutor{}
		exec := launcher.NewStreamingExecutor(inner, logr.Discard(), nil, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reqCtx := &a2asrv.RequestContext{TaskID: "task-4"}
		Expect(func() {
			_ = exec.Execute(ctx, reqCtx, &fakeQueue{})
		}).NotTo(Panic())
	})

	It("UT-AF-STREAM03-005: UpdatePhase error is logged but does not change return value", func() {
		spu := &mockSessionPhaseUpdater{
			materialized:   map[string]bool{"sess-err": true},
			updatePhaseErr: context.DeadlineExceeded,
		}
		inner := &mockExecutor{}
		var buf bytes.Buffer
		logger := captureLogr(&buf)
		exec := launcher.NewStreamingExecutor(inner, logger, nil, spu)

		ctx, cancel := context.WithCancel(context.Background())
		sc := &session.CreateContext{SessionID: "sess-err", TaskID: "task-5"}
		ctx = session.WithCreateContext(ctx, sc)
		cancel()

		reqCtx := &a2asrv.RequestContext{TaskID: "task-5", ContextID: "sess-err"}
		err := exec.Execute(ctx, reqCtx, &fakeQueue{})
		Expect(err).NotTo(HaveOccurred(), "inner executor succeeded; UpdatePhase failure must not propagate")

		logOutput := buf.String()
		Expect(logOutput).To(ContainSubstring("failed to transition session to Disconnected"))
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

// mockSessionPhaseUpdater implements launcher.SessionPhaseUpdater for testing.
type mockSessionPhaseUpdater struct {
	materialized     map[string]bool
	updatePhaseCalls []phaseUpdateCall
	updatePhaseErr   error
}

type phaseUpdateCall struct {
	SessionID string
	Phase     string
	Message   string
	UserID    string
}

func (m *mockSessionPhaseUpdater) IsMaterialized(sessionID string) bool {
	if m.materialized == nil {
		return false
	}
	return m.materialized[sessionID]
}

func (m *mockSessionPhaseUpdater) UpdatePhase(_ context.Context, sessionID string, to isv1alpha1.SessionPhase, message, userID string) error {
	m.updatePhaseCalls = append(m.updatePhaseCalls, phaseUpdateCall{
		SessionID: sessionID,
		Phase:     string(to),
		Message:   message,
		UserID:    userID,
	})
	return m.updatePhaseErr
}

// mockExecutorCanceled simulates the inner executor returning context.Canceled
// to replicate client SSE disconnect behavior.
type mockExecutorCanceled struct {
	executeCalled bool
}

func (m *mockExecutorCanceled) Execute(_ context.Context, _ *a2asrv.RequestContext, _ eventqueue.Queue) error {
	m.executeCalled = true
	return context.Canceled
}

func (m *mockExecutorCanceled) Cancel(_ context.Context, _ *a2asrv.RequestContext, _ eventqueue.Queue) error {
	return nil
}
