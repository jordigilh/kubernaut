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

		It("UT-AF-NL-002: text-to-dot transition prepends newline before first dot", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-002", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitReasoning(ctx, "status text")).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())

			dot := queue.events[1].(*a2a.TaskArtifactUpdateEvent)
			text := dot.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(text).To(Equal("\n."),
				"first dot after reasoning text must start with \\n so dots render on their own line")
		})

		It("UT-AF-NL-003: dot-to-dot accumulates inline without newline", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-003", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())

			for i := 0; i < 3; i++ {
				evt := queue.events[i].(*a2a.TaskArtifactUpdateEvent)
				text := evt.Artifact.Parts[0].(*a2a.TextPart).Text
				Expect(text).To(Equal("."),
					"consecutive dots must accumulate inline without newline prefix")
			}
		})

		It("UT-AF-NL-004: dot-to-text transition prepends newline", func() {
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-nl-004", "", nil)
			bridge := launcher.EventBridgeFromContext(ctx)

			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitKeepaliveDot(ctx)).To(Succeed())
			Expect(bridge.EmitReasoning(ctx, "reasoning after dots")).To(Succeed())

			reasoning := queue.events[2].(*a2a.TaskArtifactUpdateEvent)
			text := reasoning.Artifact.Parts[0].(*a2a.TextPart).Text
			Expect(text).To(Equal("\nreasoning after dots"),
				"reasoning after keepalive dots must start with \\n")
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

		It("UT-AF-NL-006: full remediation watch scenario produces correct concatenated output", func() {
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

			var parts []string
			for _, evt := range queue.events {
				ae := evt.(*a2a.TaskArtifactUpdateEvent)
				tp := ae.Artifact.Parts[0].(*a2a.TextPart)
				parts = append(parts, strings.TrimRight(tp.Text, " \t\n\r"))
			}
			concatenated := strings.Join(parts, "")

			expected := "Watching remediation progress..." +
				"\n.." +
				"\nRemediation phase: Executing" +
				"\nApproval granted by admin" +
				"\n..." +
				"\nRemediation phase: Verifying"
			Expect(concatenated).To(Equal(expected),
				"concatenated output after simulated kagenti trailing-strip must match expected compact format")
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
