package launcher_test

import (
	"context"
	"errors"
	"strings"
	"sync"

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

			ctx = launcher.WithEventBridge(ctx, queue, taskID, "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)
			Expect(bridge).NotTo(BeNil())
		})
	})

	Describe("EmitReasoning", func() {
		It("UT-AF-1258-010: writes TaskStatusUpdateEvent with metadata.type=reasoning to queue", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-emit-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-emit-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "Checking pod status...")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "expected TaskStatusUpdateEvent")
			Expect(string(evt.TaskID)).To(Equal("task-emit-001"))
			Expect(evt.Status.Message).NotTo(BeNil())
			Expect(evt.Status.Message.Parts).To(HaveLen(1))

			textPart, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue(), "expected TextPart")
			Expect(textPart.Text).To(Equal("Checking pod status..."))
			Expect(evt.Metadata).NotTo(BeNil())
			Expect(evt.Metadata["type"]).To(Equal("reasoning"))
		})

		It("UT-AF-1297-001: emitted event includes ContextID matching the bridge value", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-ctx-001")
			contextID := "019e67dc-eab1-70f9-8987-9302a245f5e0"
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, contextID, nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "Investigating pod crash loop")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.ContextID).To(Equal(contextID),
				"ContextID must propagate through bridge events to satisfy a2a-go task update validation")
			Expect(string(evt.TaskID)).To(Equal("task-ctx-001"))
			Expect(evt.Metadata["type"]).To(Equal("reasoning"))
		})

		It("UT-AF-1298-001: multiple emissions produce independent status events with reasoning metadata", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-art-001", "ctx-art-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "Step 1: checking pods")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "Step 2: analyzing logs")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "Step 3: root cause found")).To(Succeed())
			Expect(queue.events).To(HaveLen(3))

			for i, expectedText := range []string{
				"Step 1: checking pods",
				"Step 2: analyzing logs",
				"Step 3: root cause found",
			} {
				evt := queue.events[i].(*a2a.TaskStatusUpdateEvent)
				text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
				Expect(text).To(Equal(expectedText),
					"each reasoning emission must be an independent status event")
				Expect(evt.Metadata["type"]).To(Equal("reasoning"))
			}
		})

		It("UT-AF-1258-015: multiple emissions produce independent status events", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-append-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "first")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "second")).To(Succeed())

			first := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(first.Status.Message.Parts[0].(a2a.TextPart).Text).To(Equal("first"))
			Expect(first.Metadata["type"]).To(Equal("reasoning"))

			second := queue.events[1].(*a2a.TaskStatusUpdateEvent)
			Expect(second.Status.Message.Parts[0].(a2a.TextPart).Text).To(Equal("second"))
			Expect(second.Metadata["type"]).To(Equal("reasoning"))
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
			ctx = launcher.WithEventBridge(ctx, queue, taskID, "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			cancel()
			err := bridge.EmitReasoning(ctx, "should fail")
			Expect(err).To(MatchError(context.Canceled))
		})

		It("UT-AF-1258-016: truncates text > 512 chars", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-trunc-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longText := make([]byte, 600)
			for i := range longText {
				longText[i] = 'a'
			}
			err := bridge.EmitReasoning(ctx, string(longText))
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			textPart := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(len(textPart.Text)).To(BeNumerically("<=", 515)) // 512 + "..."
			Expect(evt.Metadata["type"]).To(Equal("reasoning"))
		})
	})

	// BR-SECURITY-SC7: Boundary Protection — secrets in outbound reasoning
	// text MUST be redacted before reaching external A2A agents. If an LLM
	// or tool handler accidentally includes a JWT, bearer token, or API key
	// in reasoning output, the bridge MUST strip it to prevent credential leakage.
	Describe("SC-7 Boundary Protection — outbound secret redaction", func() {
		It("UT-AF-1258-030: JWT embedded in reasoning is redacted before reaching the queue", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-sc7-jwt", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			reasoning := "Authenticating with Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0.sig to KA"
			err := bridge.EmitReasoning(ctx, reasoning)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			emittedText := evt.Status.Message.Parts[0].(a2a.TextPart).Text
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
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-si10-ctrl", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			reasoning := "Pod status\x00is\x01healthy\x7f"
			err := bridge.EmitReasoning(ctx, reasoning)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			emittedText := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(emittedText).To(Equal("Pod statusishealthy"),
				"SI-10 violation: control characters reached external agent")
		})

		It("UT-AF-1258-033: newline and tab are preserved (legitimate formatting)", func() {
			result := launcher.SanitizeBridgeTextForTest("line1\nline2\ttabbed")
			Expect(result).To(Equal("line1\nline2\ttabbed"),
				"SI-10 should not strip legitimate whitespace chars")
		})

		It("UT-AF-1258-034: all-control-char input produces no event (prevents empty status events)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-si10-empty", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "\x00\x01\x02\x03")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(BeEmpty(),
				"SI-10: sanitized-to-empty text should not emit a vacuous status event")
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
		It("UT-AF-1258-036: successful emission increments status event counter for throughput visibility", func() {
			queue := &fakeQueue{}
			m := &spyBridgeMetrics{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-ok", "", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			_ = bridge.EmitReasoning(ctx, "reasoning step 1")
			_ = bridge.EmitReasoning(ctx, "reasoning step 2")

			Expect(m.statusEventsInc).To(Equal(2),
				"each successful reasoning emission must increment the status counter")
			Expect(m.eventsInc).To(Equal(0),
				"reasoning emissions must not inflate the legacy artifact event counter")
			Expect(m.failuresInc).To(Equal(0))
		})

		It("UT-AF-1258-037: queue write failure increments status failure counter for alerting", func() {
			queue := &failingQueue{err: errors.New("queue full")}
			m := &spyBridgeMetrics{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-fail", "", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			_ = bridge.EmitReasoning(ctx, "reasoning text")

			Expect(m.statusFailuresInc).To(Equal(1),
				"queue write failures must be counted so ops can alert on data loss")
			Expect(m.statusEventsInc).To(Equal(0),
				"failed writes must not inflate the success counter")
			Expect(m.failuresInc).To(Equal(0),
				"reasoning write failures must not inflate the legacy artifact failure counter")
		})

		It("UT-AF-1258-038: nil metrics does not block emission (graceful degradation)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-nil", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "text without metrics")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1),
				"absence of metrics collector must not prevent event delivery")
		})
	})

	Describe("EmitOutput", func() {
		It("UT-AF-OUTPUT-001: EmitOutput writes TaskStatusUpdateEvent with metadata.type=output", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-output-001", "ctx-output-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitOutput(ctx, "Final answer: pod restarted successfully.")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "EmitOutput must produce TaskStatusUpdateEvent")
			Expect(string(evt.TaskID)).To(Equal("task-output-001"))
			Expect(evt.ContextID).To(Equal("ctx-output-001"))
			Expect(evt.Status.State).To(Equal(a2a.TaskStateWorking))
			Expect(evt.Status.Message).NotTo(BeNil())
			Expect(evt.Status.Message.Parts).To(HaveLen(1))
			Expect(evt.Status.Message.Parts[0].(a2a.TextPart).Text).
				To(Equal("Final answer: pod restarted successfully."))
			Expect(evt.Metadata).NotTo(BeNil())
			Expect(evt.Metadata["type"]).To(Equal("output"))
		})
	})

	Describe("EmitStatusWithMeta", func() {
		It("UT-AF-META-001: EmitStatusWithMeta writes TaskStatusUpdateEvent with caller-supplied metadata", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-meta-001", "ctx-meta-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			customMeta := map[string]any{
				"type":    "investigation",
				"phase":   "root-cause",
				"attempt": 2,
			}
			err := bridge.EmitStatusWithMeta(ctx, "Analyzing crash loop backoff", customMeta)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(string(evt.TaskID)).To(Equal("task-meta-001"))
			Expect(evt.ContextID).To(Equal("ctx-meta-001"))
			Expect(evt.Status.Message).NotTo(BeNil())
			Expect(evt.Status.Message.Parts[0].(a2a.TextPart).Text).
				To(Equal("Analyzing crash loop backoff"))
			Expect(evt.Metadata).To(Equal(customMeta))
		})
	})

	Describe("EmitStatus — status channel separation", func() {
		It("UT-AF-STATUS-001: EmitStatus writes TaskStatusUpdateEvent to queue", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-001", "ctx-status-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitStatus(ctx, "Connecting to KA...")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "EmitStatus must produce TaskStatusUpdateEvent")
			Expect(string(evt.TaskID)).To(Equal("task-status-001"))
			Expect(evt.ContextID).To(Equal("ctx-status-001"))
			Expect(evt.Status.State).To(Equal(a2a.TaskStateWorking))
			Expect(evt.Status.Timestamp).NotTo(BeNil())
			Expect(evt.Status.Message).NotTo(BeNil())
			Expect(evt.Status.Message.Role).To(Equal(a2a.MessageRoleAgent))
			Expect(evt.Status.Message.Parts).To(HaveLen(1))

			textPart, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(textPart.Text).To(Equal("Connecting to KA..."))
			Expect(evt.Metadata).NotTo(BeNil())
			Expect(evt.Metadata["type"]).To(Equal("status"))
		})

		It("UT-AF-STATUS-002: EmitStatusSafe is nil-safe when bridge absent", func() {
			err := launcher.EmitStatusSafe(context.Background(), "no bridge here")
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-STATUS-003: EmitStatus applies SC-7 sanitization", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-sc7", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitStatus(ctx, "token=Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0.sig")
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).NotTo(ContainSubstring("eyJhbGci"),
				"SC-7: status channel must redact secrets")
			Expect(evt.Metadata["type"]).To(Equal("status"))
		})

		It("UT-AF-STATUS-004: EmitStatus skips empty text after sanitization", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-empty", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitStatus(ctx, "\x00\x01\x02")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(BeEmpty(),
				"sanitized-to-empty status text must not produce an event")
		})

		It("UT-AF-STATUS-005: status and reasoning emissions are independent status events with distinct metadata types", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-iso", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitStatus(ctx, "status message")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "first reasoning")).To(Succeed())

			statusEvt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(statusEvt.Metadata["type"]).To(Equal("status"))
			Expect(statusEvt.Status.Message.Parts[0].(a2a.TextPart).Text).To(Equal("status message"))

			reasoningEvt := queue.events[1].(*a2a.TaskStatusUpdateEvent)
			Expect(reasoningEvt.Metadata["type"]).To(Equal("reasoning"))
			Expect(reasoningEvt.Status.Message.Parts[0].(a2a.TextPart).Text).To(Equal("first reasoning"),
				"reasoning after status must be an independent event with original text")
		})

		It("UT-AF-STATUS-006: IncBridgeStatusEvents incremented on successful status emission", func() {
			queue := &fakeQueue{}
			m := &spyBridgeMetrics{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-metrics", "", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			_ = bridge.EmitStatus(ctx, "status 1")
			_ = bridge.EmitStatus(ctx, "status 2")

			Expect(m.statusEventsInc).To(Equal(2),
				"each successful status emission must increment the status counter")
			Expect(m.eventsInc).To(Equal(0),
				"status emissions must not inflate the legacy artifact event counter")
		})

		It("UT-AF-STATUS-006a: EmitStatus queue write failure increments status failure counter (F5, F7)", func() {
			queue := &failingQueue{err: errors.New("queue full")}
			m := &spyBridgeMetrics{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-fail", "", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			_ = bridge.EmitStatus(ctx, "status text")

			Expect(m.statusFailuresInc).To(Equal(1),
				"status write failures must increment the status-specific failure counter (SI-4)")
			Expect(m.failuresInc).To(Equal(0),
				"status write failures must NOT inflate the legacy artifact failure counter")
			Expect(m.statusEventsInc).To(Equal(0),
				"failed status writes must not inflate the success counter")
		})

		It("UT-AF-STATUS-006b: EmitStatus respects context cancellation (F6 SC-10)", func() {
			queue := &blockingQueue{}
			ctx, cancel := context.WithCancel(context.Background())
			ctx = launcher.WithEventBridge(ctx, queue, "task-status-cancel", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			cancel()
			err := bridge.EmitStatus(ctx, "should fail")
			Expect(err).To(MatchError(context.Canceled))
		})

		It("UT-AF-STATUS-007: EmitKeepaliveDot produces metadata-only TaskStatusUpdateEvent", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-dot-status", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(queue.events).To(HaveLen(1))

			evt, isStatus := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(isStatus).To(BeTrue(),
				"keepalive dots must produce TaskStatusUpdateEvent")
			Expect(evt.Status.Message).To(BeNil(),
				"keepalive dots must have nil Message to avoid Task.History pollution")
			Expect(evt.Metadata).NotTo(BeNil())
			Expect(evt.Metadata["type"]).To(Equal("keepalive"))
			Expect(evt.Metadata["dot"]).To(Equal("."))
		})

		It("UT-AF-STATUS-008: EmitKeepaliveDot does not affect subsequent reasoning emissions", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-dot-iso", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "reasoning after dots")).To(Succeed())

			reasoning := queue.events[2].(*a2a.TaskStatusUpdateEvent)
			text := reasoning.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(Equal("reasoning after dots"),
				"reasoning after keepalive dots must be an independent status event with original text")
			Expect(reasoning.Metadata["type"]).To(Equal("reasoning"))
		})

		It("UT-AF-STATUS-009: real status then keepalive dots only produce one History-relevant message", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-hist-001", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitStatus(ctx, "Connecting to KA...")).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(queue.events).To(HaveLen(4))

			var historyRelevant int
			for _, evt := range queue.events {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if ok && statusEvt.Status.Message != nil {
					historyRelevant++
				}
			}
			Expect(historyRelevant).To(Equal(1),
				"only the real status message should carry Status.Message; keepalive dots must have nil Message")
		})
	})

	Describe("Independent status events — no newline prepending between emissions", func() {
		It("UT-AF-NL-001: consecutive reasoning emissions produce independent events without newline prepending", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-001", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "first message")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "second message")).To(Succeed())

			second := queue.events[1].(*a2a.TaskStatusUpdateEvent)
			text := second.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).NotTo(HavePrefix("\n"),
				"each reasoning emission is independent — no newline prepending")
			Expect(text).To(Equal("second message"))
			Expect(second.Metadata["type"]).To(Equal("reasoning"))
		})

		It("UT-AF-NL-002: reasoning then keepalive dot produces status events on separate metadata types", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-002", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "status text")).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(queue.events).To(HaveLen(2))

			reasoning, isStatus := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(isStatus).To(BeTrue(), "reasoning must be a status event")
			Expect(reasoning.Metadata["type"]).To(Equal("reasoning"))
			Expect(reasoning.Status.Message.Parts[0].(a2a.TextPart).Text).To(Equal("status text"))

			dot, isStatus := queue.events[1].(*a2a.TaskStatusUpdateEvent)
			Expect(isStatus).To(BeTrue(), "dot must be status event")
			Expect(dot.Status.Message).To(BeNil(),
				"keepalive dot must have nil Message to avoid Task.History pollution")
			Expect(dot.Metadata).NotTo(BeNil())
			Expect(dot.Metadata["type"]).To(Equal("keepalive"))
		})

		It("UT-AF-NL-003: consecutive dots produce metadata-only status events", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-003", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())

			for i := 0; i < 3; i++ {
				evt, isStatus := queue.events[i].(*a2a.TaskStatusUpdateEvent)
				Expect(isStatus).To(BeTrue(), "all dots must be status events")
				Expect(evt.Status.Message).To(BeNil(),
					"keepalive dots must have nil Message to avoid Task.History pollution")
				Expect(evt.Metadata).NotTo(BeNil())
				Expect(evt.Metadata["type"]).To(Equal("keepalive"))
				Expect(evt.Metadata["dot"]).To(Equal("."),
					"each keepalive carries dot in metadata for renderer UX cues")
			}
		})

		It("UT-AF-NL-004: reasoning after dots has no leading newline (status channel isolation)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-004", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "reasoning after dots")).To(Succeed())

			reasoning := queue.events[2].(*a2a.TaskStatusUpdateEvent)
			text := reasoning.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(Equal("reasoning after dots"),
				"reasoning after keepalive dots must not have leading newline")
			Expect(reasoning.Metadata["type"]).To(Equal("reasoning"))
		})

		It("UT-AF-NL-005: first emission has no leading newline", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-005", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "very first message")).To(Succeed())

			first := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := first.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(Equal("very first message"),
				"first emission must not have leading newline")
			Expect(first.Metadata["type"]).To(Equal("reasoning"))
		})

		It("UT-AF-NL-006: full remediation watch scenario separates reasoning status events from metadata-only keepalive status events", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-006", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "Watching remediation progress...\n")).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "Remediation phase: Executing\n")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "Approval granted by admin\n")).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "Remediation phase: Verifying\n")).To(Succeed())

			var reasoningTexts []string
			var keepaliveCount int
			for _, evt := range queue.events {
				switch e := evt.(type) {
				case *a2a.TaskStatusUpdateEvent:
					switch e.Metadata["type"] {
					case "reasoning":
						tp := e.Status.Message.Parts[0].(a2a.TextPart)
						reasoningTexts = append(reasoningTexts, strings.TrimRight(tp.Text, " \t\n\r"))
					case "keepalive":
						keepaliveCount++
						Expect(e.Status.Message).To(BeNil(),
							"keepalive dots must have nil Message to avoid Task.History pollution")
						Expect(e.Metadata).NotTo(BeNil())
						Expect(e.Metadata["type"]).To(Equal("keepalive"))
					}
				}
			}

			Expect(keepaliveCount).To(Equal(5), "5 keepalive dots must be metadata-only status events")
			Expect(reasoningTexts).To(HaveLen(4), "4 reasoning messages must be status events with type=reasoning")

			Expect(reasoningTexts).To(Equal([]string{
				"Watching remediation progress...",
				"Remediation phase: Executing",
				"Approval granted by admin",
				"Remediation phase: Verifying",
			}), "each reasoning emission is an independent status event with its own text")
		})

		It("UT-AF-NL-007: concurrent dot and reasoning emissions do not race", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-007", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					_ = bridge.EmitKeepaliveDot(ctx)
				}
			}()
			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					_ = bridge.EmitReasoning(ctx, "reasoning")
				}
			}()
			wg.Wait()

			Expect(queue.events).To(HaveLen(100),
				"all 100 concurrent emissions must complete without data loss")
		})
	})
})

// spyBridgeMetrics records metric increment calls for assertion.
type spyBridgeMetrics struct {
	eventsInc         int
	failuresInc       int
	statusEventsInc   int
	statusFailuresInc int
}

func (s *spyBridgeMetrics) IncBridgeEvents()              { s.eventsInc++ }
func (s *spyBridgeMetrics) IncBridgeWriteFailures()        { s.failuresInc++ }
func (s *spyBridgeMetrics) IncBridgeStatusEvents()         { s.statusEventsInc++ }
func (s *spyBridgeMetrics) IncBridgeStatusWriteFailures()  { s.statusFailuresInc++ }

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

var _ = Describe("stripEmoji (#1399)", func() {
	DescribeTable("UT-AF-1399-005: removes common emoji codepoints",
		func(input, expected string) {
			result := launcher.StripEmojiForTest(input)
			Expect(result).To(Equal(expected))
		},
		Entry("rocket emoji", "Hello \U0001F680 world", "Hello  world"),
		Entry("check mark emoji", "Status: \u2705 Done", "Status:  Done"),
		Entry("warning emoji", "Alert \u26A0\uFE0F critical", "Alert  critical"),
		Entry("multiple emoji", "\U0001F525 Fire \U0001F4A5 Boom \U0001F389 Party", " Fire  Boom  Party"),
		Entry("emoji at boundaries", "\U0001F600Hello\U0001F600", "Hello"),
		Entry("no emoji", "Plain text without emoji", "Plain text without emoji"),
		Entry("empty string", "", ""),
	)

	DescribeTable("UT-AF-1399-006: preserves non-emoji Unicode",
		func(input string) {
			result := launcher.StripEmojiForTest(input)
			Expect(result).To(Equal(input))
		},
		Entry("math symbols", "\u2200x \u2208 \u2124: x\u00B2 \u2265 0"),
		Entry("currency symbols", "Price: \u00A3100, \u00A5500, \u20AC75"),
		Entry("CJK characters", "\u4F60\u597D\u4E16\u754C"),
		Entry("accented Latin", "caf\u00E9, na\u00EFve, r\u00E9sum\u00E9"),
		Entry("arrows", "\u2190 \u2191 \u2192 \u2193"),
		Entry("box drawing", "\u250C\u2500\u2510\u2502\u2514\u2518"),
	)
})

var _ = Describe("EmitArtifact (#1399)", func() {
	It("UT-AF-1399-008: constructs TaskArtifactUpdateEvent with DataPart + TextPart", func() {
		queue := &fakeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-art-008", "ctx-art-008", nil)

		data := map[string]any{
			"type":           "investigation_summary",
			"schema_version": "1.0",
			"session_id":     "sess-001",
			"summary":        "OOMKill detected",
			"rca":            map[string]any{"severity": "critical", "confidence": 0.92},
		}
		textFallback := "Investigation complete. Severity: critical (confidence: 0.92)"
		meta := map[string]any{
			"type":           "investigation_summary",
			"schema_version": "1.0",
		}

		err := launcher.EmitArtifactForTest(ctx, data, textFallback, meta)
		Expect(err).NotTo(HaveOccurred())

		Expect(queue.events).To(HaveLen(1))
		evt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
		Expect(ok).To(BeTrue(), "expected TaskArtifactUpdateEvent")
		Expect(evt.Artifact).NotTo(BeNil())
		Expect(evt.Artifact.Parts).To(HaveLen(2))

		dp, ok := evt.Artifact.Parts[0].(a2a.DataPart)
		Expect(ok).To(BeTrue(), "first part must be DataPart")
		Expect(dp.Data).To(HaveKeyWithValue("type", "investigation_summary"))

		tp, ok := evt.Artifact.Parts[1].(a2a.TextPart)
		Expect(ok).To(BeTrue(), "second part must be TextPart")
		Expect(tp.Text).To(Equal(textFallback))
	})

	It("UT-AF-1399-009: sets artifact metadata from caller params", func() {
		queue := &fakeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-art-009", "ctx-art-009", nil)

		meta := map[string]any{
			"type":           "investigation_summary",
			"schema":         "investigation_summary",
			"schema_version": "1.0",
		}
		err := launcher.EmitArtifactForTest(ctx, map[string]any{"type": "test"}, "fallback", meta)
		Expect(err).NotTo(HaveOccurred())

		evt := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
		Expect(evt.Artifact.Metadata).To(HaveKeyWithValue("type", "investigation_summary"))
		Expect(evt.Artifact.Metadata).To(HaveKeyWithValue("schema_version", "1.0"))
	})

	It("UT-AF-1399-010: nil bridge returns nil (no panic)", func() {
		ctx := context.Background()
		err := launcher.EmitArtifactForTest(ctx, map[string]any{"x": 1}, "text", nil)
		Expect(err).NotTo(HaveOccurred())
	})
})
