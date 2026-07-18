package launcher_test

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

func partConverterBridgeCtx() (*fakeQueue, context.Context) {
	queue := &fakeQueue{}
	ctx := launcher.WithEventBridge(context.Background(), queue, "task-pc-001", "ctx-pc-001", nil)
	return queue, ctx
}

func statusEventTextAt(queue *fakeQueue) string {
	Expect(queue.events).To(HaveLen(1))
	evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
	Expect(ok).To(BeTrue(), "expected TaskStatusUpdateEvent at index 0")
	Expect(evt.Metadata).NotTo(BeNil())
	tp, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
	Expect(ok).To(BeTrue())
	return tp.Text
}

var _ = Describe("GenAIPartConverter (AC 5/AC 10)", func() {
	var convert launcher.PartConverterFunc

	BeforeEach(func() {
		convert = launcher.BuildPartConverterForTest()
	})

	Describe("FunctionCall transformation", func() {
		It("UT-AF-1189-100: kubernaut_investigate -> status text with context", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{
						"namespace": "prod",
						"kind":      "Deployment",
						"name":      "api-server",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "bridge path must not return artifact parts")
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("Investigating"))
			Expect(text).To(ContainSubstring("prod"))
			Expect(text).To(ContainSubstring("api-server"))
		})

		It("UT-AF-1189-101: kubernaut_investigate -> investigating status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{"session_id": "sess-42"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Investigating"))
		})

		It("UT-AF-1189-102: kubernaut_discover_workflows -> discovering status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_discover_workflows",
					Args: map[string]any{"rr_id": "rr-001"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Discovering available remediation workflows"))
		})

		It("UT-AF-1189-103: kubernaut_select_workflow -> selecting status with workflow_id", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_select_workflow",
					Args: map[string]any{"workflow_id": "wf-increase-memory"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("Selecting remediation workflow"))
			Expect(text).To(ContainSubstring("wf-increase-memory"))
		})

		It("UT-AF-1189-104: kubernaut_watch -> watching status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_watch",
					Args: map[string]any{"rr_id": "rr-disk-001"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Watching remediation progress"))
		})

		It("UT-AF-1189-106: unknown tool -> generic fallback", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "some_unknown_tool_xyz",
					Args: map[string]any{},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("...\n\n"))
		})

		It("UT-AF-1189-107: FunctionCall with nil Args -> no panic, status without context", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_watch",
					Args: nil,
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("Watching remediation progress"))
			Expect(text).NotTo(ContainSubstring("<nil>"))
		})

		It("UT-AF-1189-108: FunctionCall with malformed Args -> no panic, status without context", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{
						"namespace": func() {}, // not JSON-serializable
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Investigating"))
		})

		It("UT-AF-1189-109: kubectl_list -> listing cluster resources", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubectl_list",
					Args: map[string]any{"namespace": "production"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Listing cluster resources"))
		})

		It("UT-AF-1189-110: kubectl_list_events -> fetching cluster events", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubectl_list_events",
					Args: map[string]any{"namespace": "kube-system"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Fetching cluster events"))
		})

		It("UT-AF-1189-116: kubernaut_check_existing_remediation -> checking for existing remediation", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_check_existing_remediation",
					Args: map[string]any{"namespace": "prod", "kind": "Deployment", "name": "api"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Checking for existing remediation"))
		})
	})

	Describe("FunctionResponse summarization", func() {
		It("UT-AF-1189-111: kubernaut_investigate response with summary -> extracted text", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_investigate",
					Response: map[string]any{
						"status":  "completed",
						"summary": "Root cause: PostgreSQL uses emptyDir with 8Gi limit on node with 16Gi total disk",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("Root cause: PostgreSQL uses emptyDir"))
			Expect(text).NotTo(ContainSubstring(`"summary"`))
		})

		It("UT-AF-1189-112: kubernaut_investigate response with unknown structure -> fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_investigate",
					Response: map[string]any{"unexpected_field": 42},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Investigation completed"))
		})

		It("UT-AF-1189-113: kubernaut_discover_workflows response with workflows array -> listing", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_discover_workflows",
					Response: map[string]any{
						"workflows": []any{
							map[string]any{"name": "increase-memory", "confidence": 0.92},
							map[string]any{"name": "restart-pod", "confidence": 0.78},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("increase-memory"))
			Expect(text).To(ContainSubstring("restart-pod"))
		})

		It("UT-AF-1189-115: non-key tool response -> nil (dropped)", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_list_remediations",
					Response: map[string]any{"remediations": []any{}},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(queue.events).To(BeEmpty(), "dropped responses must not emit bridge events")
		})

		It("UT-AF-1189-117: kubernaut_select_workflow response with status and message -> summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_select_workflow",
					Response: map[string]any{
						"status":  "selected",
						"message": "Workflow increase-memory-limit selected for execution",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("selected"))
			Expect(text).To(ContainSubstring("Workflow increase-memory-limit selected for execution"))
		})

		It("UT-AF-1189-118: kubernaut_watch response with phase transition -> status", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_watch",
					Response: map[string]any{
						"events": []any{
							map[string]any{"phase": "Executing", "resource": "RemediationRequest"},
						},
						"status": "completed",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Executing"))
		})

		It("UT-AF-1189-119: FunctionResponse with nil Response map -> fallback text, no panic", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_investigate",
					Response: nil,
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Investigation completed"))
		})

		It("UT-AF-1189-120: kubernaut_investigate response with session_id -> started summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_investigate",
					Response: map[string]any{
						"session_id": "sess-abc-123",
						"status":     "started",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("Investigation started"))
			Expect(text).To(ContainSubstring("sess-abc-123"))
		})
	})

	Describe("Text passthrough", func() {
		It("UT-AF-1189-130: LLM reasoning text -> returned as artifact TextPart with trailing paragraph break", func() {
			reasoning := "Node shows DiskPressure, emptyDir usage at 72%. The PostgreSQL StatefulSet is consuming excessive disk."
			part := &genai.Part{Text: reasoning}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue(), "LLM text must be returned as artifact TextPart")
			Expect(tp.Text).To(Equal(reasoning + "\n\n"))
			Expect(queue.events).To(BeEmpty(), "LLM text must NOT emit status events")
		})

		It("UT-AF-1189-131: Text part with Thought=true -> reasoning event with actual content", func() {
			part := &genai.Part{
				Text:    "I should check the node resource utilization next.",
				Thought: true,
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "reasoning"))
			tp, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("check the node resource utilization"))
		})

		It("UT-AF-1189-132: empty text part -> passed through (not dropped)", func() {
			part := &genai.Part{Text: ""}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(BeEmpty(),
				"fallback without bridge must return empty TextPart, not drop the part")
		})
	})

	Describe("ExecutorConfig wiring", func() {
		It("UT-AF-1189-140: buildPartConverter returns non-nil converter", func() {
			Expect(launcher.BuiltConverterIsNonNil()).To(BeTrue())
		})

		It("UT-AF-1189-141: OutputMode is OutputArtifactPerEvent", func() {
			Expect(string(launcher.ExpectedOutputMode())).To(Equal("artifact-per-event"))
		})
	})

	Describe("Adversarial inputs", func() {
		It("UT-AF-1189-150: FunctionCall with 10KB Args -> no OOM, truncated context", func() {
			largeValue := strings.Repeat("x", 10*1024)
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{"namespace": largeValue},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("Investigating"))
			Expect(len(text)).To(BeNumerically("<", 1024), "status text should be bounded")
		})

		It("UT-AF-1189-151: FunctionResponse with 100KB Response -> summary truncated", func() {
			largeValue := strings.Repeat("analysis data... ", 6000)
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_investigate",
					Response: map[string]any{
						"summary": largeValue,
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(len(statusEventTextAt(queue))).To(BeNumerically("<", 2048), "summary should be bounded")
		})

		It("UT-AF-1189-152: FunctionCall with special chars in tool name -> generic fallback", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "../../etc/passwd",
					Args: map[string]any{},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("...\n\n"))
		})
	})

	Describe("Nil and edge-case inputs", func() {
		It("UT-AF-1189-153: nil part -> nil result, no error", func() {
			result, err := convert(context.Background(), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("UT-AF-1189-154: kubernaut_investigate response without session_id -> generic fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_investigate",
					Response: map[string]any{"status": "pending"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("Investigation completed.\n\n"))
		})

		It("UT-AF-1189-155: kubernaut_select_workflow response with message only -> message text", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_select_workflow",
					Response: map[string]any{"message": "Workflow selected for execution"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("Workflow selected for execution\n\n"))
		})

		It("UT-AF-1189-156: kubernaut_select_workflow response with status only -> formatted status", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_select_workflow",
					Response: map[string]any{"status": "queued"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("Workflow queued.\n\n"))
		})

		It("UT-AF-1189-157: kubernaut_select_workflow response with empty fields -> generic fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_select_workflow",
					Response: map[string]any{"other": "data"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("Workflow selection completed.\n\n"))
		})

		It("UT-AF-1189-158: kubernaut_watch response with phase only -> phase text", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_watch",
					Response: map[string]any{
						"events": []any{
							map[string]any{"phase": "Executing"},
						},
						"status": "completed",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("Remediation completed (final phase: Executing)\n\n"))
		})

		It("UT-AF-1189-159: kubernaut_watch response with status only -> status text", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_watch",
					Response: map[string]any{"status": "completed"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("Remediation completed.\n\n"))
		})

		It("UT-AF-1189-160: kubernaut_watch response with empty fields -> generic fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_watch",
					Response: map[string]any{"other": "data"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("Watching remediation...\n\n"))
		})

		It("UT-AF-1189-161: kubernaut_remediate response without rr_id -> human-friendly fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_remediate",
					Response: map[string]any{"already_exists": true},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("already exists"))
			Expect(text).NotTo(ContainSubstring(`"`), "should not contain JSON syntax")
		})
	})

	Describe("Output suppression (#1282 F-OUT)", func() {
		It("UT-AF-1282-OUT-001: kubernaut_remediate new RR → human-friendly text, no raw JSON", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_remediate",
					Response: map[string]any{
						"rr_id":   "rr-oom-abc",
						"message": "RemediationRequest created for Deployment/web by sre-user",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("Remediation request created"))
			Expect(text).NotTo(ContainSubstring(`"rr_id"`))
			Expect(text).NotTo(ContainSubstring(`"message"`))
		})

		It("UT-AF-1282-OUT-002: kubernaut_remediate existing RR → human-friendly, no JSON syntax", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_remediate",
					Response: map[string]any{
						"already_exists": true,
						"rr_id":          "rr-existing",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("already exists"))
			Expect(text).To(ContainSubstring("rr-existing"))
			Expect(text).NotTo(ContainSubstring(`{`))
		})

		It("UT-AF-1282-OUT-003: non-key tool response → nil (payload suppressed)", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubectl_get",
					Response: map[string]any{"data": "large payload"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "non-key tool payloads must be suppressed")
			Expect(queue.events).To(BeEmpty())
		})
	})

	Describe("Truncation edge cases", func() {
		It("UT-AF-1189-162: multi-byte string over byte limit but within rune limit -> not truncated", func() {
			// 400 CJK runes * 3 bytes each = 1200 bytes > maxSummaryLen(1024), but 400 runes < 1024
			multiByteStr := strings.Repeat("\u4e16", 400)
			Expect(len(multiByteStr)).To(BeNumerically(">", 1024), "precondition: byte length exceeds maxSummaryLen")
			Expect(len([]rune(multiByteStr))).To(BeNumerically("<=", 1024), "precondition: rune count within limit")

			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_investigate",
					Response: map[string]any{"summary": multiByteStr},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal(multiByteStr+"\n\n"),
				"rune-safe truncation should preserve string when rune count is within limit")
		})

		It("UT-AF-1189-163: multi-byte string over both byte and rune limits -> rune-safe truncation", func() {
			multiByteStr := strings.Repeat("\u4e16", 1100)
			Expect(len([]rune(multiByteStr))).To(BeNumerically(">", 1024), "precondition: rune count exceeds limit")

			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_investigate",
					Response: map[string]any{"summary": multiByteStr},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("..."),
				"converter truncation produces ellipsis for text exceeding maxSummaryLen")
			Expect(len([]rune(text))).To(BeNumerically("<=", 1030),
				"converter truncates at maxSummaryLen (1024); #1435 raised bridge reasoning limit to 4096 so no further truncation")
		})
	})

	Describe("Discover workflows edge cases", func() {
		It("UT-AF-1189-164: non-map entry in workflows array -> skipped gracefully", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_discover_workflows",
					Response: map[string]any{
						"workflows": []any{
							"not-a-map",
							42,
							map[string]any{"name": "valid-workflow", "confidence": 0.85},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("valid-workflow"))
			Expect(text).NotTo(ContainSubstring("not-a-map"))
		})

		It("UT-AF-1189-165: workflow entry with empty name -> skipped", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_discover_workflows",
					Response: map[string]any{
						"workflows": []any{
							map[string]any{"name": "", "confidence": 0.5},
							map[string]any{"name": "restart-pod", "confidence": 0.9},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("restart-pod"))
		})

		It("UT-AF-1189-169: workflows key is not an array -> no workflows discovered", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_discover_workflows",
					Response: map[string]any{
						"workflows": "not-an-array",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(Equal("No workflows discovered.\n\n"))
		})

		It("UT-AF-1189-166: workflow entry with zero confidence -> name without percentage", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_discover_workflows",
					Response: map[string]any{
						"workflows": []any{
							map[string]any{"name": "scale-up"},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			text := statusEventTextAt(queue)
			Expect(text).To(ContainSubstring("- scale-up"))
			Expect(text).NotTo(ContainSubstring("confidence"))
		})
	})

	Describe("Thought-to-activity indicator (AC 10)", func() {
		It("UT-AF-1189-167: Thought=true with long reasoning -> reasoning event with actual content", func() {
			part := &genai.Part{
				Text:    "Let me analyze the disk pressure situation. The node shows high emptyDir usage which is likely caused by the PostgreSQL StatefulSet writing temporary data.",
				Thought: true,
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "reasoning"))
			tp, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("disk pressure situation"))
		})

		It("UT-AF-1189-168: Thought=false with text -> returned as artifact TextPart with trailing paragraph break", func() {
			text := "Based on the analysis, the root cause is disk pressure from emptyDir."
			part := &genai.Part{
				Text:    text,
				Thought: false,
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue(), "LLM text must be returned as artifact TextPart")
			Expect(tp.Text).To(Equal(text + "\n\n"))
			Expect(queue.events).To(BeEmpty(), "LLM text must NOT emit status events")
		})
	})

	Describe("Fallback without EventBridge", func() {
		It("UT-AF-1189-105: FunctionCall returns TextPart artifact when no bridge in context", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{"namespace": "prod", "name": "api"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigating"))
		})

		It("UT-AF-1189-114: FunctionResponse returns TextPart artifact when no bridge in context", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_investigate",
					Response: map[string]any{
						"summary": "Disk pressure detected on node-1",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Disk pressure detected"))
		})

		It("UT-AF-1189-133: Text returns TextPart artifact when no bridge in context", func() {
			text := "Root cause is memory pressure."
			part := &genai.Part{Text: text}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal(text + "\n\n"))
		})
	})
})

var _ = Describe("Status message line breaks (#1301)", func() {
	var convert launcher.PartConverterFunc

	BeforeEach(func() {
		convert = launcher.BuildStreamingPartConverterForTest()
	})

	It("UT-AF-1301-001: FunctionCall status text ends with paragraph break", func() {
		part := &genai.Part{
			FunctionCall: &genai.FunctionCall{
				Name: "kubectl_list",
				Args: map[string]any{"namespace": "demo"},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(statusEventTextAt(queue)).To(HaveSuffix("\n\n"),
			"status messages must end with \\n\\n so concatenated SSE frames are readable (#1301)")
	})

	It("UT-AF-1301-002: Thought activity indicator ends with paragraph break", func() {
		part := &genai.Part{Text: "thinking...", Thought: true}
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(statusEventTextAt(queue)).To(HaveSuffix("\n\n"),
			"thought indicator must end with \\n\\n for readability (#1301)")
	})

	It("UT-AF-1301-003: FunctionResponse summary ends with paragraph break", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     "kubernaut_remediate",
				Response: map[string]any{"rr_id": "rr-test-001"},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(statusEventTextAt(queue)).To(HaveSuffix("\n\n"),
			"tool response summaries must end with \\n\\n for readability (#1301)")
	})

	It("UT-AF-1301-004: LLM text already ending with paragraph break gets no triple newline", func() {
		text := "The root cause is disk pressure.\n\n"
		part := &genai.Part{Text: text}
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		tp, ok := result.(a2a.TextPart)
		Expect(ok).To(BeTrue(), "LLM text must be returned as artifact TextPart")
		Expect(tp.Text).To(Equal(text),
			"text already ending with \\n\\n must NOT get additional newlines")
		Expect(queue.events).To(BeEmpty(), "LLM text must NOT emit status events")
	})

	It("UT-AF-1301-005: consecutive LLM text chunks are separated by paragraph break", func() {
		chunks := []string{
			"I'll start by checking what's running in the demo-crashloop namespace.",
			"Let me investigate the affected deployment.",
		}
		queue := &fakeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-pc-001", "ctx-pc-001", nil)
		var results []string
		for _, chunk := range chunks {
			part := &genai.Part{Text: chunk}
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue(), "LLM text must be returned as artifact TextPart")
			results = append(results, tp.Text)
		}
		concatenated := strings.Join(results, "")
		Expect(concatenated).To(ContainSubstring("namespace.\n\n"),
			"LLM text chunks must get trailing paragraph break appended "+
				"so consecutive chunks render as separate paragraphs (#1301)")
		Expect(queue.events).To(BeEmpty(), "LLM text must NOT emit status events")
	})
})

// =============================================================================
// TP-1301-1302 §4.1: ensureTrailingParagraphBreak helper — FedRAMP SI-4 / SC-4
// Validates the paragraph-break normalization logic in isolation.
// =============================================================================
var _ = Describe("ensureTrailingParagraphBreak helper (TP-1301-1302)", func() {
	It("UT-AF-1301-010: text without trailing newline gets \\n\\n (SI-4)", func() {
		Expect(launcher.EnsureTrailingParagraphBreakForTest("hello")).To(Equal("hello\n\n"))
	})

	It("UT-AF-1301-011: text with single \\n gets upgraded to \\n\\n (SC-4)", func() {
		Expect(launcher.EnsureTrailingParagraphBreakForTest("hello\n")).To(Equal("hello\n\n"))
	})

	It("UT-AF-1301-012: text already ending with \\n\\n is unchanged (SC-4)", func() {
		Expect(launcher.EnsureTrailingParagraphBreakForTest("hello\n\n")).To(Equal("hello\n\n"))
	})

	It("UT-AF-1301-013: empty string is unchanged", func() {
		Expect(launcher.EnsureTrailingParagraphBreakForTest("")).To(Equal(""))
	})

	It("UT-AF-1301-014: text already containing \\n\\n is left unchanged (SC-4)", func() {
		Expect(launcher.EnsureTrailingParagraphBreakForTest("hello\n\n\n")).To(Equal("hello\n\n\n"),
			"text already ending with \\n\\n should not be modified further")
	})
})

var _ = Describe("Tool error surfacing (#1302)", func() {
	var convert launcher.PartConverterFunc

	BeforeEach(func() {
		convert = launcher.BuildStreamingPartConverterForTest()
	})

	It("UT-AF-1302-001: non-key tool with error in response emits error text", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     "kubectl_list",
				Response: map[string]any{"error": "cannot resolve GVK for kind \"pods\""},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"bridge path must not return artifact parts; errors go via status channel (#1302)")
		Expect(statusEventTextAt(queue)).To(ContainSubstring("cannot resolve GVK"))
	})

	It("UT-AF-1302-002: non-key tool success response still nil (unchanged)", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     "kubectl_get",
				Response: map[string]any{"data": "large payload"},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"non-key tool success payloads must still be suppressed")
		Expect(queue.events).To(BeEmpty())
	})

	It("UT-AF-1302-004: non-key tool with status=error JSON pattern emits error text", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name: "kubernaut_discover_workflows",
				Response: map[string]any{
					"status": "error",
					"error":  "not_driving: You must send action=takeover before sending messages",
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"MCP tool errors with status=error must be surfaced via bridge (#1302)")
		Expect(statusEventTextAt(queue)).To(ContainSubstring("not_driving"),
			"error message from MCP tool must be visible to the user")
	})

	It("UT-AF-1302-003: key tool with error surfaces error, not misleading summary", func() {
		nonStreamConvert := launcher.BuildPartConverterForTest()
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     "kubernaut_investigate",
				Response: map[string]any{"error": "connection refused"},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := nonStreamConvert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(), "tool errors must always be surfaced via bridge (#1302)")
		Expect(statusEventTextAt(queue)).To(ContainSubstring("connection refused"),
			"even key tool errors must be surfaced so the user knows what failed (#1302)")
	})
})

var _ = Describe("GenAIPartConverter — Streaming Mode (TP-1258)", func() {
	var convertStreaming launcher.PartConverterFunc

	BeforeEach(func() {
		convertStreaming = launcher.BuildStreamingPartConverterForTest()
	})

	Describe("FunctionResponse suppression when streaming", func() {
		It("UT-AF-1258-030: investigate FunctionResponse -> nil when streaming mode", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_investigate",
					Response: map[string]any{
						"status":  "completed",
						"summary": "OOM kill detected",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "kubernaut_investigate FunctionResponse should be suppressed in streaming mode")
			Expect(queue.events).To(BeEmpty(), "suppressed investigate response must not emit bridge events")
		})

		It("UT-AF-1258-031: discover_workflows FunctionResponse -> brief summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_discover_workflows",
					Response: map[string]any{
						"workflows": []any{
							map[string]any{"id": "wf-1", "confidence": 0.95},
							map[string]any{"id": "wf-2", "confidence": 0.72},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("workflow"))
		})

		It("UT-AF-1258-032: select_workflow FunctionResponse -> brief status", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_select_workflow",
					Response: map[string]any{
						"status":  "selected",
						"message": "Workflow wf-increase-memory selected",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).NotTo(BeEmpty())
		})

		It("UT-AF-1258-033: kubernaut_remediate FunctionResponse -> brief status", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_remediate",
					Response: map[string]any{
						"rr_id":     "rr-oom-001",
						"namespace": "prod",
						"status":    "created",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).NotTo(BeEmpty())
		})

		It("UT-AF-1258-034: kubectl_* FunctionResponse -> nil (unchanged behavior)", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubectl_get_pods",
					Response: map[string]any{"output": "NAME READY STATUS\npod-1 1/1 Running"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "kubectl FunctionResponse should still be nil in streaming mode")
			Expect(queue.events).To(BeEmpty())
		})

		It("UT-AF-1258-035: FunctionCall parts unchanged in streaming mode", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{"session_id": "sess-42"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Investigating"))
		})

		It("UT-AF-1258-036: Thought parts emit reasoning in streaming mode", func() {
			part := &genai.Part{
				Text:    "Analyzing the disk usage patterns...",
				Thought: true,
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "reasoning"))
			tp, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("disk usage patterns"))
		})
	})

	Describe("kubernaut_remediate status/summary (#1332)", func() {
		It("UT-AF-1332-030: kubernaut_remediate FunctionCall -> status message", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_remediate",
					Args: map[string]any{
						"namespace": "prod",
						"kind":      "Deployment",
						"name":      "web",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Creating remediation request"))
		})

		It("UT-AF-1332-031: kubernaut_remediate response with new RR -> summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_remediate",
					Response: map[string]any{
						"rr_id":   "rr-web-001",
						"message": "RemediationRequest created for Deployment/web by sre-user",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("Remediation request created"))
		})

		It("UT-AF-1332-032: kubernaut_remediate response with already_exists -> summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_remediate",
					Response: map[string]any{
						"already_exists": true,
						"rr_id":          "rr-existing-002",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(statusEventTextAt(queue)).To(ContainSubstring("already exists"))
		})
	})
})

// ===========================================================================
// AU-3: Content of Audit Records — Remediation Progress Observability
// Proves that remediation execution steps are emitted as structured, machine-
// parseable audit records so operators can observe and reconstruct the full
// remediation timeline without reading free-form text.
// ===========================================================================

var _ = Describe("AU-3: Remediation progress produces structured audit records", func() {
	var convertStreaming launcher.PartConverterFunc

	BeforeEach(func() {
		convertStreaming = launcher.BuildStreamingPartConverterForTest()
	})

	It("UT-AF-WATCH-OUTPUT-001: completed remediation produces structured step record with terminal state", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name: "kubernaut_watch",
				Response: map[string]any{
					"events": []any{
						map[string]any{"phase": "Executing", "resource": "RemediationRequest", "message": "Starting rollout undo"},
						map[string]any{"phase": "Verifying", "resource": "RemediationRequest", "message": "Checking pod health"},
					},
					"status": "completed",
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convertStreaming(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())

		Expect(queue.events).To(HaveLen(1))
		evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
		Expect(ok).To(BeTrue())
		Expect(evt.Metadata).NotTo(BeNil())
		Expect(evt.Metadata["type"]).To(Equal("output"),
			"AU-3: remediation steps must be classified as 'output' for audit separation")

		tp, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(ContainSubstring(`"steps"`))
		Expect(tp.Text).To(ContainSubstring(`"completed":true`),
			"AU-3: terminal state must be explicitly recorded")
		Expect(tp.Text).To(ContainSubstring(`"s1"`))
		Expect(tp.Text).To(ContainSubstring(`"s2"`))
		Expect(tp.Text).To(ContainSubstring(`"done"`),
			"AU-3: each step must record its final disposition")
	})

	It("UT-AF-WATCH-OUTPUT-002: in-progress remediation distinguishes pending from completed steps", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name: "kubernaut_watch",
				Response: map[string]any{
					"events": []any{
						map[string]any{"phase": "Submitted", "resource": "RemediationRequest", "message": "RR created"},
						map[string]any{"phase": "Executing", "resource": "RemediationRequest", "message": "Rolling back"},
					},
					"status": "awaiting_approval",
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convertStreaming(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())

		Expect(queue.events).To(HaveLen(1))
		evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
		Expect(evt.Metadata["type"]).To(Equal("output"))

		tp := evt.Status.Message.Parts[0].(a2a.TextPart)
		Expect(tp.Text).To(ContainSubstring(`"completed":false`),
			"AU-3: non-terminal state must not claim completion")
		Expect(tp.Text).To(ContainSubstring(`"state":"running"`),
			"AU-3: active step must be distinguishable from historical steps")
		Expect(tp.Text).To(ContainSubstring(`"state":"done"`))
	})

	It("UT-AF-WATCH-OUTPUT-003: remediation with no events still produces a valid audit record", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name: "kubernaut_watch",
				Response: map[string]any{
					"events": []any{},
					"status": "completed",
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convertStreaming(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())

		Expect(queue.events).To(HaveLen(1))
		evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
		Expect(evt.Metadata["type"]).To(Equal("output"))

		tp := evt.Status.Message.Parts[0].(a2a.TextPart)
		Expect(tp.Text).To(ContainSubstring(`"steps":[]`),
			"AU-3: empty remediation must still produce valid structured record")
		Expect(tp.Text).To(ContainSubstring(`"completed":true`))
	})

	It("UT-AF-WATCH-OUTPUT-004: SI-17 fail-safe — nil response degrades gracefully without data loss", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     "kubernaut_watch",
				Response: nil,
			},
		}
		queue, ctx := partConverterBridgeCtx()
		result, err := convertStreaming(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(queue.events).To(BeEmpty(),
			"SI-17: nil response must not produce corrupt audit records")
	})

	It("UT-AF-WATCH-OUTPUT-005: AU-3 human-readable — step labels carry actionable context", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name: "kubernaut_watch",
				Response: map[string]any{
					"events": []any{
						map[string]any{"phase": "Executing", "resource": "RR"},
						map[string]any{"phase": "Verifying", "resource": "RR", "message": "All pods healthy"},
					},
					"status": "completed",
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		_, _ = convertStreaming(ctx, nil, part)

		evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
		tp := evt.Status.Message.Parts[0].(a2a.TextPart)
		Expect(tp.Text).To(ContainSubstring(`"label":"Executing"`),
			"AU-3: phase name used as fallback when no descriptive message present")
		Expect(tp.Text).To(ContainSubstring(`"label":"All pods healthy"`),
			"AU-3: descriptive message preferred over phase name for readability")
	})
})

// ===========================================================================
// AC-3: Access Enforcement — Streaming Mode Isolation
// Proves that the streaming-only structured output path does not leak into
// the non-streaming (send) path, ensuring that clients using message/send
// receive the legacy text format and are not exposed to internal metadata.
// ===========================================================================

var _ = Describe("AC-3: Structured output confined to streaming path", func() {
	It("UT-AF-WATCH-OUTPUT-006: non-streaming path preserves legacy text format for backward compatibility", func() {
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
		queue, ctx := partConverterBridgeCtx()
		result, err := convert(ctx, nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil())
		Expect(queue.events).To(HaveLen(1))
		evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
		Expect(evt.Metadata["type"]).To(Equal("status"),
			"AC-3: non-streaming clients must receive status-type events, not structured output")
	})
})

// ===========================================================================
// SI-4/AC-6: Decision Transparency & Least Privilege
// Proves that remediation decisions present ALL options to the operator with
// explicit recommendation markers, ensuring no hidden actions and auditable
// human-in-the-loop decision points.
// ===========================================================================

var _ = Describe("SI-4: Decision records preserve all options with recommendation transparency", func() {
	It("IT-AF-WATCH-OUTPUT-001: AU-3 — remediation progress record is valid JSON matching wire contract", func() {
		convertStreaming := launcher.BuildStreamingPartConverterForTest()
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name: "kubernaut_watch",
				Response: map[string]any{
					"events": []any{
						map[string]any{"phase": "Submitted", "resource": "RemediationRequest", "message": "RR created"},
						map[string]any{"phase": "Executing", "resource": "RemediationRequest", "message": "Rolling back deployment"},
						map[string]any{"phase": "Verifying", "resource": "RemediationRequest", "message": "Pod health check"},
					},
					"status": "completed",
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		_, _ = convertStreaming(ctx, nil, part)

		evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
		tp := evt.Status.Message.Parts[0].(a2a.TextPart)

		var payload struct {
			Steps []struct {
				ID    string `json:"id"`
				Label string `json:"label"`
				State string `json:"state"`
			} `json:"steps"`
			Completed bool `json:"completed"`
		}
		err := json.Unmarshal([]byte(tp.Text), &payload)
		Expect(err).NotTo(HaveOccurred(),
			"AU-3: output payload must be machine-parseable for downstream audit tooling")
		Expect(payload.Completed).To(BeTrue())
		Expect(payload.Steps).To(HaveLen(3),
			"AU-3: every observed phase transition must be recorded as a step")
		Expect(payload.Steps[0].ID).To(Equal("s1"))
		Expect(payload.Steps[0].Label).To(Equal("RR created"))
		Expect(payload.Steps[0].State).To(Equal("done"))
		Expect(payload.Steps[1].State).To(Equal("done"))
		Expect(payload.Steps[2].State).To(Equal("done"))
	})

	It("IT-AF-WATCH-OUTPUT-002: AC-6 — decision payload preserves recommended flag for all workflow options", func() {
		convertStreaming := launcher.BuildStreamingPartConverterForTest()
		part := &genai.Part{
			FunctionCall: &genai.FunctionCall{
				Name: "kubernaut_present_decision",
				Args: map[string]any{
					"session_id": "sess-1",
					"summary":    "OOM detected",
					"options": []any{
						map[string]any{"workflow_id": "wf-1", "name": "Rollback", "description": "undo last deploy", "recommended": true},
						map[string]any{"workflow_id": "wf-2", "name": "Scale", "description": "add replicas"},
					},
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		_, _ = convertStreaming(ctx, nil, part)

		Expect(queue.events).To(HaveLen(1))
		artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
		Expect(ok).To(BeTrue(), "decision must emit TaskArtifactUpdateEvent")
		Expect(artifactEvt.Artifact.Metadata["type"]).To(Equal("decision"),
			"SI-4: decision events must be classified separately for audit tracing")

		dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
		Expect(ok).To(BeTrue(), "first part must be DataPart")
		options, ok := dp.Data["options"].([]any)
		Expect(ok).To(BeTrue())
		opt0 := options[0].(map[string]any)
		Expect(opt0["recommended"]).To(BeTrue(),
			"AC-6: recommended flag must be preserved in structured DataPart")
	})
})

// =============================================================================
// TP-1395-1396 Integration Tests — prove wiring through production dispatch path
// =============================================================================

var _ = Describe("Structured Decision Payload Integration — TP-1395-1396", func() {

	It("IT-AF-1395-001: SI-10 — decision payload > 512 chars passes through streaming converter without truncation", func() {
		convertStreaming := launcher.BuildStreamingPartConverterForTest()
		part := &genai.Part{
			FunctionCall: &genai.FunctionCall{
				Name: "kubernaut_present_decision",
				Args: map[string]any{
					"session_id": "sess-it-001",
					"summary":    "OOMKill detected in production data-processor pod with critical severity and high confidence",
					"rca": map[string]any{
						"severity":         "critical",
						"confidence":       0.92,
						"causal_chain":     []any{"Memory leak in data-processor worker goroutine", "Container hit 512Mi memory limit", "Kernel sent OOMKill signal to container"},
						"target":           "Deployment/data-processor in production",
						"tool_calls_count": 19,
						"llm_turns":        17,
					},
					"options": []any{
						map[string]any{"workflow_id": "wf-restart", "name": "Restart Pod", "description": "Rolling restart of affected deployment pods to recover from OOM state immediately", "risk": "low", "recommended": true, "parameters": map[string]any{"namespace": "production", "deployment": "data-processor"}},
						map[string]any{"workflow_id": "wf-scale", "name": "Increase Memory Limit", "description": "Scale memory limit from 512Mi to 1Gi to prevent future OOMKill events", "risk": "medium", "parameters": map[string]any{"new_limit": "1Gi"}},
						map[string]any{"workflow_id": "wf-rollback", "name": "Rollback Deployment", "description": "Roll back to the previous stable revision that did not exhibit the memory leak", "risk": "low", "ruled_out_reason": "No previous revision available in cluster deployment history"},
					},
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		_, _ = convertStreaming(ctx, nil, part)

		Expect(queue.events).To(HaveLen(1))
		artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
		Expect(ok).To(BeTrue(), "decision must emit TaskArtifactUpdateEvent")
		Expect(artifactEvt.Artifact.Metadata["type"]).To(Equal("decision"))

		dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
		Expect(ok).To(BeTrue(), "first part must be DataPart with structured JSON")
		Expect(dp.Data).To(HaveKey("rca"), "SI-10: RCA field must be preserved without truncation")
		Expect(dp.Data).To(HaveKey("options"), "SI-10: options must be preserved without truncation")

		rca, ok := dp.Data["rca"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(rca["severity"]).To(Equal("critical"))
		Expect(rca["confidence"]).To(BeNumerically("~", 0.92, 0.001))
	})

	It("IT-AF-1396-001: AU-3 — RCA fields flow through decision event for audit tracing", func() {
		convertStreaming := launcher.BuildStreamingPartConverterForTest()
		part := &genai.Part{
			FunctionCall: &genai.FunctionCall{
				Name: "kubernaut_present_decision",
				Args: map[string]any{
					"session_id": "sess-it-002",
					"summary":    "Certificate expired",
					"rca": map[string]any{
						"severity":         "high",
						"confidence":       0.88,
						"causal_chain":     []any{"TLS cert expired", "Mutual TLS handshake failed"},
						"target":           "Secret/tls-cert in istio-system",
						"tool_calls_count": 5,
						"llm_turns":        3,
					},
					"options": []any{
						map[string]any{"workflow_id": "wf-renew", "name": "Renew Certificate", "description": "Issue new cert via cert-manager", "recommended": true},
					},
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		_, _ = convertStreaming(ctx, nil, part)

		Expect(queue.events).To(HaveLen(1))
		artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
		Expect(ok).To(BeTrue())

		dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
		Expect(ok).To(BeTrue())
		rca, ok := dp.Data["rca"].(map[string]any)
		Expect(ok).To(BeTrue(), "AU-3: RCA must be present for audit tracing")
		Expect(rca["severity"]).To(Equal("high"))
		Expect(rca["confidence"]).To(BeNumerically("~", 0.88, 0.001))
		causalChain, ok := rca["causal_chain"].([]any)
		Expect(ok).To(BeTrue())
		Expect(causalChain).To(HaveLen(2))
		Expect(rca["target"]).To(Equal("Secret/tls-cert in istio-system"))
		Expect(rca["tool_calls_count"]).To(BeNumerically("==", 5))
		Expect(rca["llm_turns"]).To(BeNumerically("==", 3))
	})

	It("IT-AF-1396-002: AC-6 — extended WorkflowOption fields (Parameters, RuledOutReason) flow through", func() {
		convertStreaming := launcher.BuildStreamingPartConverterForTest()
		part := &genai.Part{
			FunctionCall: &genai.FunctionCall{
				Name: "kubernaut_present_decision",
				Args: map[string]any{
					"session_id": "sess-it-003",
					"summary":    "Pod crash loop",
					"rca": map[string]any{
						"severity":   "warning",
						"confidence": 0.75,
						"target":     "Pod/worker-abc in staging",
					},
					"options": []any{
						map[string]any{
							"workflow_id": "wf-fix",
							"name":        "Apply Fix",
							"description": "Apply configuration fix",
							"parameters":  map[string]any{"image": "v2.1.0", "replicas": "3"},
							"recommended": true,
						},
						map[string]any{
							"workflow_id":      "wf-migrate",
							"name":             "Migrate Database",
							"description":      "Run database migration",
							"ruled_out_reason": "No pending migrations detected in schema diff",
						},
					},
				},
			},
		}
		queue, ctx := partConverterBridgeCtx()
		_, _ = convertStreaming(ctx, nil, part)

		Expect(queue.events).To(HaveLen(1))
		artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
		Expect(ok).To(BeTrue())

		dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
		Expect(ok).To(BeTrue())
		options, ok := dp.Data["options"].([]any)
		Expect(ok).To(BeTrue())
		Expect(options).To(HaveLen(2))

		opt0 := options[0].(map[string]any)
		params0, ok := opt0["parameters"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(params0["image"]).To(Equal("v2.1.0"))
		Expect(params0["replicas"]).To(Equal("3"))
		Expect(opt0["recommended"]).To(BeTrue())

		opt1 := options[1].(map[string]any)
		Expect(opt1["ruled_out_reason"]).To(Equal("No pending migrations detected in schema diff"))
	})
})

// ═══════════════════════════════════════════════════════════════════════════════
// #1399: A2A Streaming — Separate thinking from final output
// Phase A RED: reasoning routing + emoji suppression
// ═══════════════════════════════════════════════════════════════════════════════

var _ = Describe("Reasoning Routing (#1399)", func() {
	var convert launcher.PartConverterFunc

	BeforeEach(func() {
		convert = launcher.BuildPartConverterForTest()
	})

	Describe("Thought parts route to reasoning channel", func() {
		It("UT-AF-1399-001: Thought=true emits metadata.type=reasoning with actual thought text", func() {
			part := &genai.Part{
				Text:    "I should check the node resource utilization next.",
				Thought: true,
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "Thought parts must not produce an artifact")

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "expected TaskStatusUpdateEvent")
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "reasoning"),
				"SI-4: Thought parts must be classified as reasoning")

			tp, ok := evt.Status.Message.Parts[0].(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("check the node resource utilization"),
				"SI-4: Thought content must be forwarded, not replaced with placeholder")
		})
	})

	Describe("FunctionCall (non-decision) routes to reasoning channel", func() {
		It("UT-AF-1399-002: non-decision FunctionCall emits metadata.type=reasoning", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{
						"namespace": "prod",
						"kind":      "Deployment",
						"name":      "api-server",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "reasoning"),
				"SI-4: Non-decision FunctionCall must be classified as reasoning")
		})
	})

	Describe("FunctionResponse (non-decision) routes to reasoning channel", func() {
		It("UT-AF-1399-003: non-decision FunctionResponse emits metadata.type=reasoning", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_investigate",
					Response: map[string]any{"status": "completed", "findings": "disk pressure"},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			Expect(queue.events).To(HaveLen(1))
			evt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(evt.Metadata).To(HaveKeyWithValue("type", "reasoning"),
				"SI-4: Non-decision FunctionResponse must be classified as reasoning")
		})
	})

	Describe("present_decision FunctionCall still returns nil (no ADK artifact)", func() {
		It("UT-AF-1399-004: present_decision FunctionCall returns nil from converter", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-001",
						"summary":    "OOMKill detected",
						"rca": map[string]any{
							"severity":   "critical",
							"confidence": 0.92,
						},
						"options": []any{
							map[string]any{"workflow_id": "restart", "name": "Restart"},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(),
				"AC-6: present_decision must return nil to prevent ADK artifact duplication")
			Expect(queue.events).NotTo(BeEmpty(),
				"present_decision must still emit events via EventBridge")
		})
	})

	Describe("Final LLM text output has emoji stripped", func() {
		It("UT-AF-1399-007: emoji in final text output is stripped before delivery", func() {
			part := &genai.Part{
				Text: "\U0001F680 Investigation complete! The root cause is \U0001F525 disk pressure.\n\n",
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())

			tp, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue(), "final text must be returned as TextPart")
			Expect(tp.Text).NotTo(ContainSubstring("\U0001F680"),
				"SC-7: emoji must be stripped from final output")
			Expect(tp.Text).NotTo(ContainSubstring("\U0001F525"),
				"SC-7: emoji must be stripped from final output")
			Expect(tp.Text).To(ContainSubstring("Investigation complete"),
				"non-emoji text must be preserved")
			Expect(tp.Text).To(ContainSubstring("disk pressure"),
				"non-emoji text must be preserved")
			Expect(queue.events).To(BeEmpty(),
				"final text must NOT emit status events")
		})
	})

	Describe("Decision event uses EmitArtifact", func() {
		It("UT-AF-1399-011: present_decision emits TaskArtifactUpdateEvent (not status-update)", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-structured-001",
						"summary":    "OOMKill detected in production data-processor pod",
						"rca": map[string]any{
							"severity":   "critical",
							"confidence": 0.92,
							"causal_chain": []any{
								"Memory leak in worker goroutine",
								"Container hit 512Mi limit",
							},
						},
						"options": []any{
							map[string]any{"workflow_id": "restart", "name": "Restart Pod", "recommended": true},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convert(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())

			var hasArtifactEvent bool
			for _, evt := range queue.events {
				if _, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
					hasArtifactEvent = true
					break
				}
			}
			Expect(hasArtifactEvent).To(BeTrue(),
				"AU-3: present_decision must emit TaskArtifactUpdateEvent with structured data")
		})
	})
})

// =============================================================================
// #1408: Contract compliance — FunctionResponse suppression + schema fields
// =============================================================================

var _ = Describe("Contract Compliance #1408 — Structured investigation_summary", func() {
	var convertStreaming launcher.PartConverterFunc

	BeforeEach(func() {
		convertStreaming = launcher.BuildStreamingPartConverterForTest()
	})

	Describe("FunctionResponse suppression", func() {
		It("UT-AF-1408-001: SI-4 — present_decision FunctionResponse must NOT emit any event", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_present_decision",
					Response: map[string]any{
						"presented": true,
						"message":   "Investigation complete.\n\nSummary: OOM detected\nSeverity: critical",
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			result, err := convertStreaming(ctx, nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(),
				"SI-4: present_decision FunctionResponse must return nil (suppressed)")
			Expect(queue.events).To(BeEmpty(),
				"SI-4: present_decision FunctionResponse must NOT emit status events (prevents double-render)")
		})
	})

	Describe("DataPart schema compliance", func() {
		It("UT-AF-1408-002: SI-10 — DataPart.Data contains LLM-produced args without injected fields", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-1408-001",
						"summary":    "OOM detected",
						"rca": map[string]any{
							"severity":    "critical",
							"confidence":  0.92,
							"explanation": "Memory limit exceeded",
						},
						"options": []any{
							map[string]any{"workflow_id": "wf-1", "name": "Restart"},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			_, _ = convertStreaming(ctx, nil, part)

			Expect(queue.events).To(HaveLen(1))
			artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue())
			dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue())
			Expect(dp.Data).NotTo(HaveKey("type"),
				"SI-10: DataPart.Data must NOT contain injected type — prevents LLM context pollution (#1411)")
			Expect(dp.Data).NotTo(HaveKey("schema_version"),
				"SI-10: DataPart.Data must NOT contain injected schema_version — prevents LLM context pollution (#1411)")
			Expect(dp.Data).To(HaveKey("summary"))
			Expect(dp.Data).To(HaveKey("rca"))
		})

		It("UT-AF-1408-003: SI-10 — artifact metadata carries schema identification", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-1408-002",
						"summary":    "Cert expired",
						"rca": map[string]any{
							"severity":    "high",
							"confidence":  0.88,
							"explanation": "TLS cert expired",
						},
						"options": []any{},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			_, _ = convertStreaming(ctx, nil, part)

			Expect(queue.events).To(HaveLen(1))
			artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue())
			Expect(artifactEvt.Artifact.Metadata).To(HaveKeyWithValue("schema", "investigation_summary"),
				"SI-10: schema identification must live in artifact metadata, not body")
			Expect(artifactEvt.Artifact.Metadata).To(HaveKeyWithValue("schema_version", "1.0"),
				"SI-10: schema_version must live in artifact metadata, not body")
		})

		It("UT-AF-1408-004: SI-10 — DataPart.Data passes ValidatePayload for investigation_summary schema", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-1408-003",
						"summary":    "OOM detected in production",
						"rca": map[string]any{
							"severity":    "critical",
							"confidence":  0.95,
							"explanation": "Container OOMKilled due to memory limit",
							"causal_chain": []any{
								"Memory leak in worker goroutine",
								"Container hit 512Mi limit",
							},
							"target": "Deployment/data-processor",
						},
						"options": []any{
							map[string]any{"workflow_id": "wf-restart", "name": "Restart Pod"},
						},
					},
				},
			}
			queue, ctx := partConverterBridgeCtx()
			_, _ = convertStreaming(ctx, nil, part)

			Expect(queue.events).To(HaveLen(1))
			artifactEvt, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue())
			dp, ok := artifactEvt.Artifact.Parts[0].(a2a.DataPart)
			Expect(ok).To(BeTrue())

			err := launcher.ValidatePayloadForTest("investigation_summary", dp.Data)
			Expect(err).NotTo(HaveOccurred(),
				"SI-10: emitted DataPart.Data must pass schema validation for investigation_summary")
		})
	})

	Describe("Single artifact emission (no duplicates)", func() {
		It("UT-AF-1408-005: AU-3 — full FunctionCall+FunctionResponse cycle produces exactly one event", func() {
			fcPart := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_present_decision",
					Args: map[string]any{
						"session_id": "sess-1408-004",
						"summary":    "Pod crash loop",
						"rca": map[string]any{
							"severity":    "warning",
							"confidence":  0.75,
							"explanation": "Bad config",
						},
						"options": []any{
							map[string]any{"workflow_id": "wf-fix", "name": "Apply Fix"},
						},
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
			queue, ctx := partConverterBridgeCtx()
			_, _ = convertStreaming(ctx, nil, fcPart)
			_, _ = convertStreaming(ctx, nil, frPart)

			Expect(queue.events).To(HaveLen(1),
				"AU-3: full present_decision cycle must produce exactly 1 event (artifact only, no status duplicate)")
			_, ok := queue.events[0].(*a2a.TaskArtifactUpdateEvent)
			Expect(ok).To(BeTrue(),
				"AU-3: the single event must be TaskArtifactUpdateEvent (not TaskStatusUpdateEvent)")
		})
	})
})

var _ = Describe("Event-aware text routing (#1410)", func() {
	var convertStreaming launcher.PartConverterFunc

	BeforeEach(func() {
		convertStreaming = launcher.BuildStreamingPartConverterForTest()
	})

	Describe("Text in partial event", func() {
		It("UT-AF-1410-001: text in partial event routes to reasoning, not artifact", func() {
			event := &session.Event{
				LLMResponse: model.LLMResponse{
					Content: &genai.Content{
						Parts: []*genai.Part{{Text: "Let me check the logs..."}},
					},
					Partial: true,
				},
			}
			part := &genai.Part{Text: "Let me check the logs..."}
			queue, ctx := partConverterBridgeCtx()

			result, err := convertStreaming(ctx, event, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(),
				"Text in partial events must NOT become an artifact")

			Expect(queue.events).To(HaveLen(1))
			statusEvt, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "Partial text must be routed to reasoning via EventBridge")
			Expect(statusEvt.Status.Message).NotTo(BeNil())
		})
	})

	Describe("Text in event with FunctionCall", func() {
		It("UT-AF-1410-002: text coexisting with FunctionCall routes to reasoning", func() {
			event := &session.Event{
				LLMResponse: model.LLMResponse{
					Content: &genai.Content{
						Parts: []*genai.Part{
							{Text: "I'll investigate this pod."},
							{FunctionCall: &genai.FunctionCall{Name: "kubernaut_investigate", Args: map[string]any{"namespace": "prod"}}},
						},
					},
					Partial: false,
				},
			}
			part := &genai.Part{Text: "I'll investigate this pod."}
			queue, ctx := partConverterBridgeCtx()

			result, err := convertStreaming(ctx, event, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(),
				"Text preamble in event with FunctionCall must NOT become an artifact")

			Expect(queue.events).To(HaveLen(1))
			_, ok := queue.events[0].(*a2a.TaskStatusUpdateEvent)
			Expect(ok).To(BeTrue(), "Preamble text must be routed to reasoning")
		})
	})

	Describe("Text in final event without FunctionCall", func() {
		It("UT-AF-1410-003: text in final non-partial event becomes artifact", func() {
			event := &session.Event{
				LLMResponse: model.LLMResponse{
					Content: &genai.Content{
						Parts: []*genai.Part{{Text: "The investigation is complete. Here are the results."}},
					},
					Partial: false,
				},
			}
			part := &genai.Part{Text: "The investigation is complete. Here are the results."}
			_, ctx := partConverterBridgeCtx()

			result, err := convertStreaming(ctx, event, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(),
				"Text in final event without FunctionCall MUST become an artifact")

			textPart, ok := result.(a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(textPart.Text).To(ContainSubstring("investigation is complete"))
		})
	})
})
