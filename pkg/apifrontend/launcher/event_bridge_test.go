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

// buildLongReasoningContentText builds a string of exactly n runes, reused
// across the EmitReasoningContent/per-type truncation-limit tests (#1635,
// #1435).
func buildLongReasoningContentText(n int) string {
	unit := "The alert KubePodCrashLooping is firing for pod web-frontend in namespace demo-webui. "
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString(unit)
	}
	return sb.String()[:n]
}

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

		It("UT-AF-1258-016: reasoning text under 4096 chars passes through (#1435 raised limit)", func() {
			queue := &fakeQueue{}
			taskID := a2a.TaskID("task-trunc-001")
			ctx := launcher.WithEventBridge(context.Background(), queue, taskID, "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longText := buildLongReasoningContentText(600)
			err := bridge.EmitReasoning(ctx, longText)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			textPart := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(len(textPart.Text)).To(Equal(600),
				"#1435: reasoning limit raised to 4096, 600 chars should not be truncated")
			Expect(evt.Metadata["type"]).To(Equal("reasoning"))
		})
	})

	Describe("Per-type truncation limits (#1435)", func() {
		It("UT-AF-1435-001: reasoning text up to 4096 runes is NOT truncated", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1435-001", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longReasoning := buildLongReasoningContentText(4000)
			err := bridge.EmitReasoning(ctx, longReasoning)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(Equal(longReasoning),
				"BR-AF-STREAM-001: reasoning text under 4096 runes must not be truncated")
		})

		It("UT-AF-1435-002: reasoning text beyond 4096 runes IS truncated with ellipsis", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1435-002", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longReasoning := buildLongReasoningContentText(5000)
			err := bridge.EmitReasoning(ctx, longReasoning)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(len([]rune(text))).To(BeNumerically("<=", 4099),
				"reasoning text must be truncated at 4096 runes + ellipsis")
			Expect(text).To(HaveSuffix("..."))
		})

		It("UT-AF-1435-003: status text still truncated at 512 runes", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1435-003", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longStatus := buildLongReasoningContentText(600)
			err := bridge.EmitStatus(ctx, longStatus)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(len([]rune(text))).To(BeNumerically("<=", 515),
				"status text must still be bounded at 512 runes")
			Expect(text).To(HaveSuffix("..."))
		})

		It("UT-AF-1435-004: output text uses 4096-rune limit (same as reasoning)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1435-004", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longOutput := buildLongReasoningContentText(4000)
			err := bridge.EmitOutput(ctx, longOutput)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(Equal(longOutput),
				"output text under 4096 runes must not be truncated")
		})

		It("UT-AF-1435-005: 700-char reasoning (issue scenario) passes through intact", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1435-005", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			reasoning := "The target resource appears to be a Deployment called web-frontend in demo-webui namespace. " +
				"Let me use kubernaut_investigate_alert with:\n\n" +
				"alert_name: KubePodCrashLooping\n" +
				"api_version: apps/v1\n" +
				"kind: Deployment\n" +
				"name: web-frontend\n" +
				"namespace: demo-webui\n\n" +
				"This will initiate a full investigation using the KA engine which will examine pod logs, events, " +
				"and recent changes to determine the root cause of the crash loop. The investigation will also check " +
				"for resource limits, OOM kills, and image pull failures as common causes."
			err := bridge.EmitReasoning(ctx, reasoning)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(Equal(reasoning),
				"BR-AF-STREAM-001: real-world reasoning text (~600 chars) must not be truncated")
			Expect(text).To(ContainSubstring("kind: Deployment"),
				"the truncated field from issue #1434 must now be present")
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
			result := launcher.SanitizeBridgeTextForTest(context.Background(), input)
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
			result := launcher.SanitizeBridgeTextForTest(context.Background(), "line1\nline2\ttabbed")
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
			result := launcher.SanitizeBridgeTextForTest(context.Background(), input)
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

	// #1635 / BR-AI-086 AC10: dedicated live-stream channel for KA's captured
	// LLM reasoning/thinking content, distinct from EmitReasoning's
	// orchestration-narration channel (metadata.type="reasoning"). Per
	// DD-LLM-009, this mirrors EmitReasoning/EmitOutput's exact shape
	// (emitWithLimit, 4096-rune limit, no-op on empty text).
	Describe("EmitReasoningContent", func() {
		It("UT-AF-1635-EB-001: writes TaskStatusUpdateEvent with metadata.type=reasoning_content", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-rc-001", "ctx-rc-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoningContent(ctx, "considering memory limits and recent deploys", false)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "EmitReasoningContent must produce TaskStatusUpdateEvent")
			Expect(string(evt.TaskID)).To(Equal("task-rc-001"))
			Expect(evt.ContextID).To(Equal("ctx-rc-001"))
			Expect(evt.Status.Message).NotTo(BeNil())
			Expect(evt.Status.Message.Parts).To(HaveLen(1))
			Expect(evt.Status.Message.Parts[0].(a2a.TextPart).Text).
				To(Equal("considering memory limits and recent deploys"))
			Expect(evt.Metadata).NotTo(BeNil())
			Expect(evt.Metadata["type"]).To(Equal("reasoning_content"),
				"#1635: must be distinguishable from EmitReasoning's metadata.type=reasoning")
		})

		It("UT-AF-1635-EB-002: reasoning content up to 4096 runes is NOT truncated (same limit as EmitReasoning)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-rc-002", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			longText := buildLongReasoningContentText(4000)
			err := bridge.EmitReasoningContent(ctx, longText, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			text := evt.Status.Message.Parts[0].(a2a.TextPart).Text
			Expect(text).To(Equal(longText))
		})

		It("UT-AF-1635-EB-003: EmitReasoningContentSafe is nil-safe when bridge absent", func() {
			err := launcher.EmitReasoningContentSafe(context.Background(), "no bridge here", false)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-1635-EB-004: EmitReasoningContent is a no-op on empty text (redacted-reasoning parity with EmitReasoning)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-rc-004", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoningContent(ctx, "", false)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(BeEmpty(),
				"#1635: a non-redacted turn's empty text must produce zero events, matching EmitReasoning's existing behavior")
		})

		// #1716 / DD-LLM-009 (redaction sub-decision, revisited): a redacted
		// turn must now emit a content-free signal (metadata.redacted=true,
		// no text) instead of the full no-op above, so Console can render a
		// "reasoning hidden by provider" placeholder.
		It("UT-AF-1716-EB-001: a redacted turn emits a content-free signal instead of a no-op", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-rc-005", "ctx-rc-005", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoningContent(ctx, "", true)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1),
				"#1716: a redacted turn must produce exactly one event, not a no-op")

			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "EmitReasoningContent must produce TaskStatusUpdateEvent")
			Expect(evt.Metadata).NotTo(BeNil())
			Expect(evt.Metadata["type"]).To(Equal("reasoning_content"))
			Expect(evt.Metadata["redacted"]).To(Equal(true),
				"#1716: metadata.redacted must be true so Console can render a placeholder")
		})

		It("UT-AF-1716-EB-002: a non-redacted empty text still no-ops (regression guard)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-rc-006", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitReasoningContent(ctx, "", false)
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(BeEmpty(),
				"#1716: a genuinely empty, non-redacted turn must remain a no-op — only redacted=true changes behavior")
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
func (s *spyBridgeMetrics) IncBridgeWriteFailures()       { s.failuresInc++ }
func (s *spyBridgeMetrics) IncBridgeStatusEvents()        { s.statusEventsInc++ }
func (s *spyBridgeMetrics) IncBridgeStatusWriteFailures() { s.statusFailuresInc++ }

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

var _ = Describe("RR Context Enrichment — #1423 (AU-3, SI-4, SC-7)", func() {

	Describe("SetRRContext + EmitStatus (AU-3: audit record content traceability)", func() {
		It("UT-AF-1423-001: status events include all RR context fields after SetRRContext", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-001", "ctx-1423-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:      "rr-47ec5289",
				Namespace: "demo-gateway",
				Kind:      "Deployment",
				Target:    "api-frontend",
				AlertName: "ScalingLimited",
				Phase:     "Investigating",
			})

			err := bridge.EmitStatus(ctx, "Investigation starting...")
			Expect(err).NotTo(HaveOccurred())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "status"))
			Expect(evt.Metadata).To(HaveKeyWithValue("rr_id", "rr-47ec5289"),
				"AU-3: rr_id enables cross-event correlation in audit trail")
			Expect(evt.Metadata).To(HaveKeyWithValue("namespace", "demo-gateway"))
			Expect(evt.Metadata).To(HaveKeyWithValue("kind", "Deployment"))
			Expect(evt.Metadata).To(HaveKeyWithValue("target", "api-frontend"))
			Expect(evt.Metadata).To(HaveKeyWithValue("alert_name", "ScalingLimited"))
			Expect(evt.Metadata).To(HaveKeyWithValue("phase", "Investigating"),
				"SI-4: phase enables real-time monitoring of remediation lifecycle")
		})

		It("UT-AF-1423-002: status events without SetRRContext contain no RR fields (backward compat)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-002", "ctx-1423-002", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			err := bridge.EmitStatus(ctx, "No RR context yet")
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "status"))
			Expect(evt.Metadata).NotTo(HaveKey("rr_id"),
				"SI-10: no RR fields injected when context is absent")
			Expect(evt.Metadata).NotTo(HaveKey("namespace"))
			Expect(evt.Metadata).NotTo(HaveKey("kind"))
			Expect(evt.Metadata).NotTo(HaveKey("target"))
			Expect(evt.Metadata).NotTo(HaveKey("alert_name"))
			Expect(evt.Metadata).NotTo(HaveKey("phase"))
		})
	})

	Describe("Caller metadata precedence (SI-10: server-sourced authority)", func() {
		It("UT-AF-1423-003: caller-supplied metadata keys take precedence over RR context", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-003", "ctx-1423-003", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:      "rr-from-context",
				Namespace: "ns-from-context",
				Phase:     "Investigating",
			})

			callerMeta := map[string]any{
				"type":  launcher.MetaTypeAlignmentCheckFailed,
				"rr_id": "rr-from-caller",
			}
			err := bridge.EmitStatusWithMeta(ctx, "Alignment check", callerMeta)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata["rr_id"]).To(Equal("rr-from-caller"),
				"SI-10: caller-supplied rr_id must NOT be overwritten by RR context")
			Expect(evt.Metadata["namespace"]).To(Equal("ns-from-context"),
				"AU-3: non-conflicting RR fields still merged")
			Expect(evt.Metadata["phase"]).To(Equal("Investigating"))
		})
	})

	Describe("EmitReasoning includes RR context (AU-3)", func() {
		It("UT-AF-1423-004: reasoning events carry RR context for audit correlation", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-004", "ctx-1423-004", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:   "rr-reasoning-001",
				Target: "memory-eater",
				Phase:  "Investigating",
			})

			err := bridge.EmitReasoning(ctx, "Checking pod status...")
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata["type"]).To(Equal("reasoning"))
			Expect(evt.Metadata["rr_id"]).To(Equal("rr-reasoning-001"),
				"AU-3: reasoning events include rr_id for audit trail correlation")
			Expect(evt.Metadata["target"]).To(Equal("memory-eater"))
		})
	})

	Describe("EmitKeepaliveDot includes RR context (SI-4)", func() {
		It("UT-AF-1423-005: keepalive dots carry RR context for Console banner persistence", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-005", "ctx-1423-005", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:      "rr-keepalive-001",
				Namespace: "payments",
				Kind:      "StatefulSet",
				Target:    "db-primary",
				Phase:     "Investigating",
			})

			err := bridge.EmitKeepaliveDot(ctx)
			Expect(err).NotTo(HaveOccurred())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata["type"]).To(Equal("keepalive"))
			Expect(evt.Metadata["dot"]).To(Equal("."))
			Expect(evt.Metadata["rr_id"]).To(Equal("rr-keepalive-001"),
				"SI-4: keepalive events carry RR context so Console banner survives SSE reconnect")
			Expect(evt.Metadata["namespace"]).To(Equal("payments"))
			Expect(evt.Metadata["kind"]).To(Equal("StatefulSet"))
			Expect(evt.Metadata["target"]).To(Equal("db-primary"))
		})
	})

	Describe("UpdatePhase (SI-4: lifecycle monitoring)", func() {
		It("UT-AF-1423-006: UpdatePhase changes phase on subsequent status events", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-006", "ctx-1423-006", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:  "rr-phase-001",
				Phase: "Investigating",
			})

			Expect(bridge.EmitStatus(ctx, "Starting investigation")).To(Succeed())

			bridge.UpdatePhase("AwaitingApproval")
			Expect(bridge.EmitStatus(ctx, "Approval required")).To(Succeed())

			evt0 := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt0.Metadata["phase"]).To(Equal("Investigating"))

			evt1 := queue.events[1].(*a2a.TaskStatusUpdateEvent)
			Expect(evt1.Metadata["phase"]).To(Equal("AwaitingApproval"),
				"SI-4: phase must reflect current lifecycle state for real-time monitoring")
		})

		It("UT-AF-1423-007: UpdatePhase is no-op when no RR context set", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-007", "ctx-1423-007", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.UpdatePhase("AwaitingApproval")
			Expect(bridge.EmitStatus(ctx, "No RR context")).To(Succeed())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).NotTo(HaveKey("phase"),
				"UpdatePhase must be no-op without prior SetRRContext")
		})
	})

	Describe("SetRRContextSafe (SC-7: nil-safe boundary protection)", func() {
		It("UT-AF-1423-008: SetRRContextSafe is nil-safe when no bridge in context", func() {
			ctx := context.Background()
			Expect(func() {
				launcher.SetRRContextSafe(ctx, &launcher.RRContext{RRID: "rr-nil-001"})
			}).NotTo(Panic(), "SC-7: nil bridge must not panic")
		})

		It("UT-AF-1423-009: SetRRContextSafe enriches subsequent status events via context", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-009", "ctx-1423-009", nil)

			launcher.SetRRContextSafe(ctx, &launcher.RRContext{
				RRID:      "rr-safe-001",
				Namespace: "demo",
				Kind:      "Deployment",
				Target:    "web",
				AlertName: "CrashLooping",
				Phase:     "Investigating",
			})

			Expect(launcher.EmitStatusSafe(ctx, "After SetRRContextSafe")).To(Succeed())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata["rr_id"]).To(Equal("rr-safe-001"),
				"SC-7: SetRRContextSafe must propagate RR context through EventBridge")
			Expect(evt.Metadata["alert_name"]).To(Equal("CrashLooping"))
		})

		It("UT-AF-1423-010: UpdatePhaseSafe is nil-safe when no bridge in context", func() {
			ctx := context.Background()
			Expect(func() {
				launcher.UpdatePhaseSafe(ctx, "Executing")
			}).NotTo(Panic())
		})
	})

	Describe("Empty fields are omitted (SI-10: minimal data exposure)", func() {
		It("UT-AF-1423-011: empty RR context fields are not included in metadata", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-011", "ctx-1423-011", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:  "rr-partial-001",
				Phase: "Investigating",
			})

			Expect(bridge.EmitStatus(ctx, "Partial context")).To(Succeed())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).To(HaveKeyWithValue("rr_id", "rr-partial-001"))
			Expect(evt.Metadata).To(HaveKeyWithValue("phase", "Investigating"))
			Expect(evt.Metadata).NotTo(HaveKey("namespace"),
				"SI-10: empty fields must not appear in metadata to minimize data exposure")
			Expect(evt.Metadata).NotTo(HaveKey("kind"))
			Expect(evt.Metadata).NotTo(HaveKey("target"))
			Expect(evt.Metadata).NotTo(HaveKey("alert_name"))
		})
	})

	Describe("Kind/Name target format — #1492 (AU-3: unambiguous resource identity)", func() {
		It("IT-AF-1492-001: Kind/Name target propagates through RRContext to SSE metadata", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1492-001", "ctx-1492-001", nil)

			launcher.SetRRContextSafe(ctx, &launcher.RRContext{
				RRID:      "rr-1492-001",
				Namespace: "demo-gateway",
				Kind:      "Deployment",
				Target:    "Deployment/api-frontend",
				AlertName: "ScalingLimited",
				Phase:     "Investigating",
			})

			Expect(launcher.EmitStatusSafe(ctx, "Investigation starting...")).To(Succeed())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).To(HaveKeyWithValue("target", "Deployment/api-frontend"),
				"AU-3: target in SSE metadata must use Kind/Name format so audit consumers can unambiguously identify the resource without namespace inference")
			Expect(evt.Metadata).To(HaveKeyWithValue("kind", "Deployment"),
				"SC-7: server-sourced kind field crosses the SSE boundary for structured consumers")
			Expect(evt.Metadata).To(HaveKeyWithValue("namespace", "demo-gateway"),
				"SC-7: server-sourced namespace prevents client-side guessing at the boundary")
		})
	})

	Describe("Thread safety (SC-7: concurrent access protection)", func() {
		It("UT-AF-1423-012: concurrent SetRRContext and EmitStatus do not race", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1423-012", "ctx-1423-012", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					bridge.SetRRContext(&launcher.RRContext{
						RRID:  "rr-race-001",
						Phase: "Investigating",
					})
				}
			}()

			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					_ = bridge.EmitStatus(ctx, "concurrent emit")
				}
			}()

			wg.Wait()
			Expect(queue.events).NotTo(BeEmpty(),
				"SC-7: concurrent SetRRContext + EmitStatus must not panic or deadlock")
		})
	})
})

// Fleet cluster_id propagation — #1409 (SI-4, SI-10, AU-3).
var _ = Describe("RR Context Fleet cluster_id — #1409 (SI-4, SI-10, AU-3)", func() {

	Describe("RRContext.ClusterID structural carry (SI-4: cross-cluster correlation)", func() {
		It("UT-AF-1409-001: RRContext carries ClusterID through to status event metadata", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1409-001", "ctx-1409-001", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:      "rr-fleet-001",
				Namespace: "demo-gateway",
				ClusterID: "cluster-east-1",
				Phase:     "Investigating",
			})

			Expect(bridge.EmitStatus(ctx, "Investigation starting...")).To(Succeed())
			Expect(queue.events).To(HaveLen(1))

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).To(HaveKeyWithValue("cluster_id", "cluster-east-1"),
				"SI-4: cluster_id enables cross-cluster signal correlation in fleet deployments")

			rc := launcher.RRContextSafe(ctx)
			Expect(rc).NotTo(BeNil())
			Expect(rc.ClusterID).To(Equal("cluster-east-1"),
				"RRContextSafe must round-trip ClusterID for downstream artifact consumers (e.g. emitDecisionEvent)")
		})

		It("UT-AF-1409-001b: status events omit cluster_id entirely for local-hub RRs (no false attribution)", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1409-001b", "ctx-1409-001b", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{RRID: "rr-local-001", Phase: "Investigating"})
			Expect(bridge.EmitStatus(ctx, "Investigation starting...")).To(Succeed())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).NotTo(HaveKey("cluster_id"),
				"SI-10: empty-string cluster_id must not appear as noise for single-cluster deployments")
		})
	})

	Describe("mergeRRContext caller precedence for cluster_id (SI-10: server-sourced authority)", func() {
		It("UT-AF-1409-002: caller-supplied cluster_id metadata takes precedence over RR context", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1409-002", "ctx-1409-002", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			bridge.SetRRContext(&launcher.RRContext{
				RRID:      "rr-fleet-002",
				ClusterID: "cluster-from-context",
				Phase:     "Investigating",
			})

			callerMeta := map[string]any{
				"type":       launcher.MetaTypeStatus,
				"cluster_id": "cluster-from-caller",
			}
			Expect(bridge.EmitStatusWithMeta(ctx, "Caller-scoped update", callerMeta)).To(Succeed())

			evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(evt.Metadata).To(HaveKeyWithValue("cluster_id", "cluster-from-caller"),
				"SI-10: caller-provided cluster_id always takes precedence over merged defaults")
		})
	})

	Describe("RRContextSafe nil-safety (SC-7: boundary protection)", func() {
		It("UT-AF-1409-001c: RRContextSafe returns nil when no bridge is present in context", func() {
			Expect(launcher.RRContextSafe(context.Background())).To(BeNil())
		})

		It("UT-AF-1409-001d: RRContextSafe returns nil when a bridge is present but SetRRContext was never called", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-1409-001d", "ctx-1409-001d", nil)
			Expect(launcher.RRContextSafe(ctx)).To(BeNil())
		})
	})
})
