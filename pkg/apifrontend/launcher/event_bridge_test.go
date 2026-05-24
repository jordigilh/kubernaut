package launcher_test

import (
	"context"
	"errors"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("EventBridge", func() {
	Describe("Context helpers", func() {
		It("UT-AF-1258-014: EventBridgeFromContext returns nil for non-streaming context", func() {
			ctx := context.Background()
			bridge := launcher.EventBridgeFromContext(ctx)
			Expect(bridge).To(BeNil())
		})

		It("UT-AF-1258-002: WithEventBridge stores bridge retrievable from context", func() {
			ctx := context.Background()
			queue := &fakeQueue{}
			taskID := a2a.TaskID("test-task-123")

			ctx = launcher.WithEventBridge(ctx, queue, taskID, nil)
			bridge := launcher.EventBridgeFromContext(ctx)
			Expect(bridge).NotTo(BeNil())
		})
	})

	Describe("EmitReasoning", func() {
		It("UT-AF-1258-010: writes TaskArtifactUpdateEvent to queue", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-emit-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "Checking pod status...")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue(), "expected TaskArtifactUpdateEvent")
			Expect(string(evt.TaskID)).To(Equal("task-emit-001"))
			Expect(evt.Artifact).NotTo(BeNil())
			Expect(evt.Artifact.Parts).To(HaveLen(1))

			textPart, ok := evt.Artifact.Parts[0].(*a2a.TextPart)
			Expect(ok).To(BeTrue(), "expected TextPart")
			Expect(textPart.Text).To(Equal("Checking pod status..."))
		})

		It("UT-AF-1258-015: sets Append=true on artifact", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-append-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "some reasoning")
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(evt.Append).To(BeTrue())
		})

		It("UT-AF-1258-011: nil-safe when bridge not in context", func() {
			bridge := launcher.EventBridgeFromContext(context.Background())
			Expect(bridge).To(BeNil())
			err := launcher.EmitReasoningSafe(context.Background(), "text")
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-1258-013: respects context cancellation", func() {
			queue := &blockingQueue{}
			taskID := a2a.TaskID("task-cancel-001")
			ctx, cancel := context.WithCancel(context.Background())
			ctx = launcher.WithEventBridge(ctx, queue, taskID, nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			cancel()
			err := bridge.EmitReasoning(ctx, "should fail")
			Expect(err).To(MatchError(context.Canceled))
		})

		It("UT-AF-1258-016: truncates text > 512 chars", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-trunc-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longText := make([]byte, 600)
			for i := range longText {
				longText[i] = 'a'
			}
			err := bridge.EmitReasoning(ctx, string(longText))
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			textPart := evt.Artifact.Parts[0].(*a2a.TextPart)
			Expect(len(textPart.Text)).To(BeNumerically("<=", 515)) // 512 + "..."
		})
	})

	// BR-SECURITY-SC7: Boundary Protection — secrets in outbound reasoning
	// text MUST be redacted before reaching external A2A agents. If an LLM
	// or tool handler accidentally includes a JWT, bearer token, or API key
	// in reasoning output, the bridge MUST strip it to prevent credential leakage.
	Describe("SC-7 Boundary Protection — outbound secret redaction", func() {
		It("UT-AF-1258-030: JWT embedded in reasoning is redacted before reaching the queue", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-sc7-jwt", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			reasoning := "Authenticating with Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0.sig to KA"
			err := bridge.EmitReasoning(ctx, reasoning)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			emittedText := evt.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(emittedText).NotTo(ContainSubstring("eyJhbGci"),
				"SC-7 violation: JWT token leaked to external agent via bridge")
		})

		It("UT-AF-1258-031: bearer token in standalone text is redacted", func() {
			input := "token=Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.Signature_value_here"
			result := launcher.SanitizeBridgeTextForTest(input)
			Expect(result).NotTo(ContainSubstring("eyJhbGci"),
				"SC-7 violation: sanitizeBridgeText passes bearer token to output")
		})
	})

	// BR-SECURITY-SI10: Information Input Validation — outbound text MUST NOT
	// contain control characters that could enable log injection, terminal escape
	// sequences, or output corruption in downstream consumers.
	Describe("SI-10 Input Validation — control character stripping", func() {
		It("UT-AF-1258-032: null bytes and non-printable chars are stripped before emission", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-si10-ctrl", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			reasoning := "Pod status\x00is\x01healthy\x7f"
			err := bridge.EmitReasoning(ctx, reasoning)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			emittedText := evt.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(emittedText).To(Equal("Pod statusishealthy"),
				"SI-10 violation: control characters reached external agent")
		})

		It("UT-AF-1258-033: newline and tab are preserved (legitimate formatting)", func() {
			result := launcher.SanitizeBridgeTextForTest("line1\nline2\ttabbed")
			Expect(result).To(Equal("line1\nline2\ttabbed"),
				"SI-10 should not strip legitimate whitespace chars")
		})

		It("UT-AF-1258-034: all-control-char input produces no event (prevents empty artifacts)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-si10-empty", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "\x00\x01\x02\x03")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(BeEmpty(),
				"SI-10: sanitized-to-empty text should not emit a vacuous artifact")
		})

		It("UT-AF-1258-035: long text is truncated after sanitization to bound memory", func() {
			input := strings.Repeat("Hello world. ", 50) // ~650 chars of natural text
			result := launcher.SanitizeBridgeTextForTest(input)
			Expect(len([]rune(result))).To(BeNumerically("<=", 515),
				"bridge text must be bounded to prevent memory exhaustion")
			Expect(result).To(HaveSuffix("..."))
		})
	})

	// BR-MONITORING: Operational visibility — the ops team MUST have real-time
	// counters for bridge event throughput and write failures to drive alerting
	// (e.g. af_a2a_bridge_write_failures_total > 0 triggers PagerDuty).
	Describe("Observability — bridge metrics for incident response", func() {
		It("UT-AF-1258-036: successful emission increments event counter for throughput visibility", func() {
			queue := &fakeQueue{}
			m := &spyBridgeMetrics{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-ok", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			_ = bridge.EmitReasoning(ctx, "reasoning step 1")
			_ = bridge.EmitReasoning(ctx, "reasoning step 2")

			Expect(m.eventsInc).To(Equal(2),
				"each successful bridge emission must be counted for throughput monitoring")
			Expect(m.failuresInc).To(Equal(0))
		})

		It("UT-AF-1258-037: queue write failure increments failure counter for alerting", func() {
			queue := &failingQueue{err: errors.New("queue full")}
			m := &spyBridgeMetrics{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-fail", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			_ = bridge.EmitReasoning(ctx, "reasoning text")

			Expect(m.failuresInc).To(Equal(1),
				"queue write failures must be counted so ops can alert on data loss")
			Expect(m.eventsInc).To(Equal(0),
				"failed writes must not inflate the success counter")
		})

		It("UT-AF-1258-038: nil metrics does not block emission (graceful degradation)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-nil", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "text without metrics")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1),
				"absence of metrics collector must not prevent event delivery")
		})
	})
})

// spyBridgeMetrics records metric increment calls for assertion.
type spyBridgeMetrics struct {
	eventsInc   int
	failuresInc int
}

func (s *spyBridgeMetrics) IncBridgeEvents()       { s.eventsInc++ }
func (s *spyBridgeMetrics) IncBridgeWriteFailures() { s.failuresInc++ }

// failingQueue always returns an error from Write.
type failingQueue struct {
	err error
}

func (q *failingQueue) Write(_ context.Context, _ a2a.Event) error {
	return q.err
}

func (q *failingQueue) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return q.err
}

func (q *failingQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, q.err
}

func (q *failingQueue) Close() error { return nil }

var _ eventqueue.Queue = (*failingQueue)(nil)
