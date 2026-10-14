package launcher_test

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	It("UT-AF-2496-002: a text-only artifact cannot invent structured selection context (BR-KA-016, SI-10)", func() {
		queue, ctx := partConverterBridgeCtx()
		Expect(launcher.EmitArtifactSafe(ctx, nil, "Authoritative selection context unavailable", map[string]any{"schema": "investigation_summary", "schema_version": "1.0"})).To(Succeed())
		Expect(queue.events).To(HaveLen(1))
		artifact, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
		Expect(ok).To(BeTrue())
		Expect(artifact.Artifact.Parts).To(Equal(a2a.ContentParts{a2a.TextPart{Text: "Authoritative selection context unavailable"}}))
	})

	DescribeTable("UT-AF-2496-002: live selection schema failure is observable without logging payload contents (BR-KA-016, SI-10)",
		func(field string, selection any) {
			queue, ctx := partConverterBridgeCtx()
			core, logs := observer.New(zap.InfoLevel)
			ctx = logr.NewContext(ctx, zapr.NewLogger(zap.New(core)))
			data := map[string]any{"session_id": "rr-2496", "summary": "Current pressure", "rca": map[string]any{"explanation": "Current pressure"}, field: selection}
			Expect(launcher.EmitArtifactSafe(ctx, data, "Current pressure", map[string]any{"schema": "investigation_summary", "schema_version": "1.0"})).To(Succeed())
			Expect(queue.events).To(HaveLen(1))
			artifact, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(artifact.Artifact.Parts).To(Equal(a2a.ContentParts{a2a.TextPart{Text: "Current pressure"}}))
			Expect(logs.Len()).To(BeNumerically(">", 0), "structured fallback is observable")
			for _, entry := range logs.All() {
				Expect(entry.Message).NotTo(ContainSubstring("private-private"))
				fields, err := json.Marshal(entry.ContextMap())
				Expect(err).NotTo(HaveOccurred())
				Expect(string(fields)).NotTo(ContainSubstring("private-private"))
			}
		},
		Entry("malformed", "selected_workflow", "private-private"),
		Entry("oversized reasoning", "selected_workflow", map[string]any{"workflow_id": "wf-2496", "workflow_name": "Increase memory", "repeat_justification": map[string]any{"evidence_basis": strings.Repeat("a", 2001), "prior_outcome_relevance": "Prior Full zero", "expected_mechanism": "Increase limit"}}),
		Entry("private execution fields", "selected_workflow", map[string]any{"workflow_id": "wf-2496", "workflow_name": "Increase memory", "parameters": map[string]any{"PASSWORD": "private-private"}}),
		Entry("rejected null context", "selection_assessment", nil),
		Entry("rejected execution identity", "selection_assessment", map[string]any{"proposed_workflow_id": "wf-2496", "workflow_id": "private-private"}),
		Entry("rejected private prior links", "selection_assessment", map[string]any{"proposed_workflow_id": "wf-2496", "prior_remediation_uids": []any{"private-private"}}),
		Entry("rejected invalid confidence", "selection_assessment", map[string]any{"proposed_workflow_id": "wf-2496", "workflow_selection_confidence": 1.1}),
		Entry("rejected blank optional reason", "selection_assessment", map[string]any{"proposed_workflow_id": "wf-2496", "repeat_justification": map[string]any{"evidence_basis": " \t"}}),
	)

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
				"summary": "Missing required fields",
			})
			Expect(err).To(HaveOccurred(),
				"SI-10: schema validation must catch missing required fields")
		})
	})

	Describe("Contract compliance (#1408)", func() {
		It("IT-AF-1408-001: AU-3 — DataPart.Data contains type + schema_version through streaming pipeline", func() {
			convert := launcher.BuildStreamingPartConverterForTest()
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-it-1408",
						"summary":    "OOM detected in data-processor",
						"rca": map[string]any{
							"severity":    "critical",
							"confidence":  0.92,
							"explanation": "Container OOMKilled",
						},
						"options": []any{
							map[string]any{"workflow_id": "wf-restart", "name": "Restart Pod"},
						},
					},
				},
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-1408", "ctx-it-1408", nil)

			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue())

			dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue())
			Expect(dp.Data).To(HaveKey("summary"),
				"AU-3: DataPart.Data must contain the LLM-produced investigation summary")
			Expect(artifactEvt.Artifact.Metadata).To(HaveKeyWithValue("schema", "investigation_summary"),
				"AU-3: artifact metadata must identify schema for consumer routing")
			Expect(artifactEvt.Artifact.Metadata).To(HaveKeyWithValue("schema_version", "1.0"),
				"AU-3: artifact metadata must include schema_version for contract versioning")
		})

		It("IT-AF-1408-002: SI-4 — FunctionResponse suppression prevents duplicate event through streaming pipeline", func() {
			convert := launcher.BuildStreamingPartConverterForTest()
			fcPart := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-it-1408-dup",
						"summary":    "Cert expired",
						"rca":        map[string]any{"severity": "high", "confidence": 0.88, "explanation": "TLS expired"},
						"options":    []any{},
					},
				},
			}
			frPart := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_present_decision",
					Response: map[string]any{
						"presented": true,
						"message":   "Investigation complete.",
					},
				},
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-1408-dup", "ctx-it-1408-dup", nil)

			_, _ = convert(ctx, nil, fcPart)
			_, _ = convert(ctx, nil, frPart)

			Expect(queue.events).To(HaveLen(1),
				"SI-4: full present_decision cycle through streaming pipeline must produce exactly 1 event")
			_, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue(),
				"SI-4: the single event must be TaskArtifactUpdateEvent")
		})
	})

	Describe("Terminal state contract (#1408 Issue 3)", func() {
		It("IT-AF-1408-003: present_decision converter returns nil — ADK precondition for input-required frame", func() {
			convert := launcher.BuildStreamingPartConverterForTest()
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-it-terminal",
						"summary":    "Investigation complete",
						"rca":        map[string]any{"severity": "high", "confidence": 0.9},
						"options":    []any{},
					},
				},
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-terminal", "ctx-it-terminal", nil)

			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(),
				"Converter MUST return nil for present_decision FunctionCall so ADK's "+
					"inputRequiredProcessor includes it in LongRunningToolIDs and emits "+
					"state=input-required with final=true on the SSE output queue")

			Expect(queue.events).To(HaveLen(1),
				"Decision artifact must still be emitted via EventBridge side-channel")
		})
	})

	Describe("Fleet cluster_id propagation (#1409)", func() {
		It("IT-AF-1409-009: cluster_id reaches the investigation_summary DataPart through the streaming pipeline", func() {
			convert := launcher.BuildStreamingPartConverterForTest()
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-it-1409-009",
						"summary":    "Disk pressure detected",
						"rca":        map[string]any{"severity": "high", "confidence": 0.85, "explanation": "disk full"},
						"options":    []any{map[string]any{"workflow_id": "wf-1", "name": "Evict Pod"}},
					},
				},
			}
			queue := &fakeQueue{}
			ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-1409-009", "ctx-it-1409-009", nil)
			launcher.SetRRContextSafe(ctx, &launcher.RRContext{RRID: "rr-it-1409-009", ClusterID: "cluster-fleet-it"})

			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue(), "AU-3: decision must emit TaskArtifactUpdateEvent")

			dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue())
			Expect(dp.Data).To(HaveKeyWithValue("cluster_id", "cluster-fleet-it"),
				"SI-10, AU-3: cluster_id must reach the investigation_summary DataPart through the wired streaming pipeline")

			err = launcher.ValidatePayloadForTest("investigation_summary", dp.Data)
			Expect(err).NotTo(HaveOccurred(),
				"SI-10: emitted DataPart.Data with cluster_id must still pass schema validation")
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
