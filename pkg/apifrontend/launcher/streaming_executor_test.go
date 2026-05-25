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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("StreamingExecutor", func() {
	var (
		inner    *mockExecutor
		executor a2asrv.AgentExecutor
	)

	BeforeEach(func() {
		inner = &mockExecutor{}
		executor = launcher.NewStreamingExecutor(inner, logr.Logger{}, nil)
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
			exec := launcher.NewStreamingExecutor(cleanerInner, logr.Logger{}, nil)

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
		executor := launcher.NewStreamingExecutor(inner, logger, nil)

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
		executor := launcher.NewStreamingExecutor(inner, logger, nil)

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
		executor := launcher.NewStreamingExecutor(inner, logger, nil)

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
		executor := launcher.NewStreamingExecutor(inner, logr.Logger{}, nil)

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
		exec := launcher.NewStreamingExecutor(inner, testLogger, nil)
		launcher.StreamingExecutorLoggerForTest(exec).Info("stored logger")
		Expect(logs).To(ContainElement(ContainSubstring("stored logger")))
	})

	It("UT-AF-1274-007: logs stream open/close through logr (BR-SESS-013)", func() {
		inner := &mockExecutor{}
		var buf bytes.Buffer
		logger := captureLogr(&buf)
		exec := launcher.NewStreamingExecutor(inner, logger, nil)

		reqCtx := &a2asrv.RequestContext{TaskID: "task-logr-001"}
		err := exec.Execute(context.Background(), reqCtx, &fakeQueue{})
		Expect(err).NotTo(HaveOccurred())

		logOutput := buf.String()
		Expect(logOutput).To(ContainSubstring("a2a stream opened"))
		Expect(logOutput).To(ContainSubstring("a2a stream closed"))
		Expect(logOutput).To(ContainSubstring("task-logr-001"))
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
