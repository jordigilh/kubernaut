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
		It("UT-AF-1258-010: writes TaskArtifactUpdateEvent to queue", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-emit-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "ctx-emit-001", nil)
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

		It("UT-AF-1297-001: emitted event includes ContextID matching the bridge value", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-ctx-001")
			contextID := "019e67dc-eab1-70f9-8987-9302a245f5e0"
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, contextID, nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoning(ctx, "Investigating pod crash loop")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.ContextID).To(Equal(contextID),
				"ContextID must propagate through bridge events to satisfy a2a-go task update validation")
			Expect(string(evt.TaskID)).To(Equal("task-ctx-001"))
		})

		It("UT-AF-1298-001: first emission creates artifact (Append=false), subsequent appends", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-art-001", "ctx-art-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "Step 1: checking pods")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "Step 2: analyzing logs")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "Step 3: root cause found")).To(Succeed())
			Expect(queue.events).To(HaveLen(3))

			first := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(first.Append).To(BeFalse(),
				"first bridge emission must create a new artifact (Append=false) per a2a-go contract")
			Expect(first.Artifact).NotTo(BeNil())
			Expect(string(first.Artifact.ID)).NotTo(BeEmpty(),
				"first emission must assign an ArtifactID so subsequent appends can reference it")

			artifactID := first.Artifact.ID

			second := queue.events[1].(*a2a.TaskArtifactUpdateEvent)
			Expect(second.Append).To(BeTrue(),
				"subsequent emissions must append to the existing artifact")
			Expect(second.Artifact.ID).To(Equal(artifactID),
				"subsequent emissions must reference the same ArtifactID created by the first emission")

			third := queue.events[2].(*a2a.TaskArtifactUpdateEvent)
			Expect(third.Append).To(BeTrue())
			Expect(third.Artifact.ID).To(Equal(artifactID))
		})

		It("UT-AF-1258-015: first emission creates artifact (Append=false), second appends", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-append-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "first")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "second")).To(Succeed())

			first := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(first.Append).To(BeFalse(), "first emission creates a new artifact")
			Expect(string(first.Artifact.ID)).NotTo(BeEmpty())

			second := queue.events[1].(*a2a.TaskArtifactUpdateEvent)
			Expect(second.Append).To(BeTrue(), "subsequent emissions append")
			Expect(second.Artifact.ID).To(Equal(first.Artifact.ID))
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
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-sc7-jwt", "", nil)
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
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-si10-ctrl", "", nil)
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
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-si10-empty", "", nil)
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
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-ok", "", m)
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
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-obs-fail", "", m)
			bridge := launcher.EventBridgeFromContext(ctx)

			_ = bridge.EmitReasoning(ctx, "reasoning text")

			Expect(m.failuresInc).To(Equal(1),
				"queue write failures must be counted so ops can alert on data loss")
			Expect(m.eventsInc).To(Equal(0),
				"failed writes must not inflate the success counter")
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

	Describe("EmitStatus — status channel separation", func() {
		It("UT-AF-STATUS-001: EmitStatus writes TaskStatusUpdateEvent to queue", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-001", "ctx-status-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitStatus(ctx, "Connecting to KA...")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "EmitStatus must produce TaskStatusUpdateEvent, not TaskArtifactUpdateEvent")
			Expect(string(evt.TaskID)).To(Equal("task-status-001"))
			Expect(evt.ContextID).To(Equal("ctx-status-001"))
			Expect(evt.Status.State).To(Equal(a2a.TaskStateWorking))
			Expect(evt.Status.Timestamp).NotTo(BeNil())
			Expect(evt.Status.Message).NotTo(BeNil())
			Expect(evt.Status.Message.Role).To(Equal(a2a.MessageRoleAgent))
			Expect(evt.Status.Message.Parts).To(HaveLen(1))

			textPart, ok := evt.Status.Message.Parts[0].(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(textPart.Text).To(Equal("Connecting to KA..."))
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
			text := evt.Status.Message.Parts[0].(*a2a.TextPart).Text
			Expect(text).NotTo(ContainSubstring("eyJhbGci"),
				"SC-7: status channel must redact secrets")
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

		It("UT-AF-STATUS-005: status events do not affect artifact hasEmitted state", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-status-iso", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitStatus(ctx, "status message")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "first reasoning")).To(Succeed())

			reasoning := queue.events[1].(*a2a.TaskArtifactUpdateEvent)
			text := reasoning.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(text).To(Equal("first reasoning"),
				"reasoning after status must not have leading newline — status does not set hasEmitted")
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
				"status emissions must not inflate the artifact event counter")
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
				"status write failures must NOT inflate the artifact failure counter")
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
				"keepalive dots must produce TaskStatusUpdateEvent, not TaskArtifactUpdateEvent")
			Expect(evt.Status.Message).To(BeNil(),
				"keepalive dots must have nil Message to avoid Task.History pollution")
			Expect(evt.Metadata).NotTo(BeNil())
			Expect(evt.Metadata["type"]).To(Equal("keepalive"))
			Expect(evt.Metadata["dot"]).To(Equal("."))
		})

		It("UT-AF-STATUS-008: EmitKeepaliveDot does not affect artifact stream state", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-dot-iso", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "reasoning after dots")).To(Succeed())

			reasoning := queue.events[2].(*a2a.TaskArtifactUpdateEvent)
			text := reasoning.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(text).To(Equal("reasoning after dots"),
				"reasoning after keepalive dots must not have leading newline — dots are on status channel")
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

	Describe("Newline separation — surviving client trailing-whitespace stripping", func() {
		It("UT-AF-NL-001: text-to-text transition prepends newline on second emission", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-001", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "first message")).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "second message")).To(Succeed())

			second := queue.events[1].(*a2a.TaskArtifactUpdateEvent)
			text := second.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(text).To(HavePrefix("\n"),
				"second reasoning emission must start with \\n so it renders on its own line after kagenti trailing-strip")
			Expect(text).To(Equal("\nsecond message"))
		})

		It("UT-AF-NL-002: text-to-dot produces metadata-only status event, not artifact", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-002", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "status text")).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(queue.events).To(HaveLen(2))

			_, isArtifact := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(isArtifact).To(BeTrue(), "reasoning must be artifact event")

			dot, isStatus := queue.events[1].(*a2a.TaskStatusUpdateEvent)
			Expect(isStatus).To(BeTrue(), "dot must be status event after channel separation")
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

			reasoning := queue.events[2].(*a2a.TaskArtifactUpdateEvent)
			text := reasoning.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(text).To(Equal("reasoning after dots"),
				"reasoning after keepalive dots must not have leading newline — dots are on status channel")
		})

		It("UT-AF-NL-005: first emission has no leading newline", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-005", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "very first message")).To(Succeed())

			first := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			text := first.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(text).To(Equal("very first message"),
				"first emission must not have leading newline")
		})

		It("UT-AF-NL-006: full remediation watch scenario separates artifacts from metadata-only keepalive status events", func() {
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

			var artifactTexts []string
			var keepaliveCount int
			for _, evt := range queue.events {
				switch e := evt.(type) {
				case *a2a.TaskArtifactUpdateEvent:
					tp := e.Artifact.Parts[0].(*a2a.TextPart)
					artifactTexts = append(artifactTexts, strings.TrimRight(tp.Text, " \t\n\r"))
				case *a2a.TaskStatusUpdateEvent:
					keepaliveCount++
					Expect(e.Status.Message).To(BeNil(),
						"keepalive dots must have nil Message to avoid Task.History pollution")
					Expect(e.Metadata).NotTo(BeNil())
					Expect(e.Metadata["type"]).To(Equal("keepalive"))
				}
			}

			Expect(keepaliveCount).To(Equal(5), "5 keepalive dots must be metadata-only status events")
			Expect(artifactTexts).To(HaveLen(4), "4 reasoning messages must be artifact events")

			concatenated := strings.Join(artifactTexts, "")
			expected := "Watching remediation progress..." +
				"\nRemediation phase: Executing" +
				"\nApproval granted by admin" +
				"\nRemediation phase: Verifying"
			Expect(concatenated).To(Equal(expected),
				"artifact stream must contain only reasoning text with newline separation between consecutive reasoning emissions")
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
