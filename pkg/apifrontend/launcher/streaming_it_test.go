package launcher_test

import (
	"context"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

// =============================================================================
// IT-AF-1399: Integration Wiring Tests — Reasoning Routing + Structured Artifacts
// Proves that components from Phase A (reasoning routing, emoji strip) and
// Phase B (EmitArtifact, schema validation) are correctly wired through the
// production converter chain.
// =============================================================================

var _ = Describe("IT-AF-1399: A2A Streaming Pipeline Wiring", func() {
	Describe("Reasoning routing through production streaming converter", func() {
		It("IT-AF-1399-001: Thought part routes to reasoning through streaming pipeline", func() {
			convert := launcher.BuildStreamingPartConverterForTest()
			part := &genai.Part{
				Text:    "Let me check the pod resource utilization to identify the root cause.",
				Thought: true,
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-001", "ctx-it-001", nil)

			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "Thought must not produce artifact in streaming mode")

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "reasoning"),
				"SI-4: Thought parts must emit reasoning metadata through streaming path")
			tp := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(tp.Text).To(ContainSubstring("pod resource utilization"))
		})
	})

	Describe("Decision artifact wiring through streaming converter", func() {
		It("IT-AF-1399-002: present_decision emits TaskArtifactUpdateEvent through streaming pipeline", func() {
			convert := launcher.BuildStreamingPartConverterForTest()
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-it-1399",
						"summary":    "Disk pressure detected",
						"rca":        map[string]any{"severity": "high", "confidence": 0.85},
						"options":    []any{map[string]any{"workflow_id": "wf-1", "name": "Evict Pod"}},
					},
				},
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-002", "ctx-it-002", nil)

			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue(), "AU-3: decision must emit TaskArtifactUpdateEvent")
			Expect(artifactEvt.Artifact.Parts).To(HaveLen(2))

			dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue())
			Expect(dp.Data).To(HaveKey("rca"))
			Expect(dp.Data).To(HaveKey("options"))

			_, ok = artifactEvt.Artifact.Parts[1].(a2a.TextPart)
			Expect(ok).To(BeTrue(), "second part must be TextPart fallback")
		})
	})

	Describe("Emoji strip in full pipeline", func() {
		It("IT-AF-1399-003: emoji stripped from final text in streaming pipeline", func() {
			convert := launcher.BuildStreamingPartConverterForTest()
			part := &genai.Part{
				Text: "\U0001F680 Investigation complete. Root cause is disk pressure \U0001F525.",
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-003", "ctx-it-003", nil)

			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())

			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).NotTo(ContainSubstring("\U0001F680"),
				"SC-7: emoji must be stripped in streaming pipeline")
			Expect(tp.Text).To(ContainSubstring("Investigation complete"))
		})
	})

	Describe("Schema validation wiring", func() {
		It("IT-AF-1399-004: ValidatePayload rejects invalid payload", func() {
			err := launcher.ValidatePayloadForTest("investigation_summary", map[string]any{
				"type":    "investigation_summary",
				"summary": "Missing required fields",
			})
			Expect(err).To(HaveOccurred(),
				"SI-10: schema validation must catch missing required fields")
		})
	})

	Describe("outputMetaTools routing preserved", func() {
		It("IT-AF-1399-005: kubernaut_watch FunctionResponse still uses status type", func() {
			convert := launcher.BuildPartConverterForTest()
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_watch",
					Response: map[string]any{
						"events": []any{
							map[string]any{"phase": "Executing", "resource": "RR"},
						},
						"status": "completed",
					},
				},
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-005", "ctx-it-005", nil)

			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "status"),
				"AC-3: kubernaut_watch must still emit status-type events")
		})
	})
})
