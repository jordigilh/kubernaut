package launcher_test

import (
	"context"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("GenAIPartConverter (AC 5/AC 10)", func() {
	var convert launcher.PartConverterFunc

	BeforeEach(func() {
		convert = launcher.BuildPartConverterForTest()
	})

	Describe("FunctionCall transformation", func() {
		It("UT-AF-1189-100: kubernaut_start_investigation -> status text with context", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_start_investigation",
					Args: map[string]any{
						"namespace": "prod",
						"kind":      "Deployment",
						"name":      "api-server",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue(), "expected *a2a.TextPart")
			Expect(tp.Text).To(ContainSubstring("Starting investigation"))
			Expect(tp.Text).To(ContainSubstring("prod"))
			Expect(tp.Text).To(ContainSubstring("api-server"))
		})

		It("UT-AF-1189-101: kubernaut_stream_investigation -> streaming status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_stream_investigation",
					Args: map[string]any{"session_id": "sess-42"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Streaming live investigation events"))
		})

		It("UT-AF-1189-102: kubernaut_discover_workflows -> discovering status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_discover_workflows",
					Args: map[string]any{"rr_id": "rr-001"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Discovering available remediation workflows"))
		})

		It("UT-AF-1189-103: kubernaut_select_workflow -> selecting status with workflow_id", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_select_workflow",
					Args: map[string]any{"workflow_id": "wf-increase-memory"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Selecting remediation workflow"))
			Expect(tp.Text).To(ContainSubstring("wf-increase-memory"))
		})

		It("UT-AF-1189-104: kubernaut_watch -> watching status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_watch",
					Args: map[string]any{"rr_id": "rr-disk-001"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Watching remediation progress"))
		})

		It("UT-AF-1189-105: af_create_rr -> creating RR status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "af_create_rr",
					Args: map[string]any{
						"namespace": "staging",
						"kind":      "Pod",
						"name":      "web-0",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Creating remediation request"))
		})

		It("UT-AF-1189-106: unknown tool -> generic fallback", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "some_unknown_tool_xyz",
					Args: map[string]any{},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Processing"))
		})

		It("UT-AF-1189-107: FunctionCall with nil Args -> no panic, status without context", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_watch",
					Args: nil,
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Watching remediation progress"))
			Expect(tp.Text).NotTo(ContainSubstring("<nil>"))
		})

		It("UT-AF-1189-108: FunctionCall with malformed Args -> no panic, status without context", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_start_investigation",
					Args: map[string]any{
						"namespace": func() {}, // not JSON-serializable
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Starting investigation"))
		})

		It("UT-AF-1189-109: af_get_pods -> fetching pod status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "af_get_pods",
					Args: map[string]any{"namespace": "production"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Fetching pod status"))
		})

		It("UT-AF-1189-110: af_list_events -> fetching cluster events", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "af_list_events",
					Args: map[string]any{"namespace": "kube-system"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Fetching cluster events"))
		})

		It("UT-AF-1189-116: af_check_existing_rr -> checking for existing remediation", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "af_check_existing_rr",
					Args: map[string]any{"namespace": "prod", "kind": "Deployment", "name": "api"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Checking for existing remediation"))
		})
	})

	Describe("FunctionResponse summarization", func() {
		It("UT-AF-1189-111: kubernaut_stream_investigation response with summary -> extracted text", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_stream_investigation",
					Response: map[string]any{
						"status":  "completed",
						"summary": "Root cause: PostgreSQL uses emptyDir with 8Gi limit on node with 16Gi total disk",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Root cause: PostgreSQL uses emptyDir"))
			Expect(tp.Text).NotTo(ContainSubstring(`"summary"`))
		})

		It("UT-AF-1189-112: kubernaut_stream_investigation response with unknown structure -> fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_stream_investigation",
					Response: map[string]any{"unexpected_field": 42},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigation completed"))
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("increase-memory"))
			Expect(tp.Text).To(ContainSubstring("restart-pod"))
		})

		It("UT-AF-1189-114: af_create_rr response with rr_id -> creation summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "af_create_rr",
					Response: map[string]any{
						"rr_id":  "rr-disk-pressure-abc123",
						"status": "created",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Remediation request created"))
			Expect(tp.Text).To(ContainSubstring("rr-disk-pressure-abc123"))
		})

		It("UT-AF-1189-115: non-key tool response -> nil (dropped)", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_list_remediations",
					Response: map[string]any{"remediations": []any{}},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("selected"))
			Expect(tp.Text).To(ContainSubstring("Workflow increase-memory-limit selected for execution"))
		})

		It("UT-AF-1189-118: kubernaut_watch response with phase transition -> status", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_watch",
					Response: map[string]any{
						"phase":  "Executing",
						"status": "WorkflowRunning",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Executing"))
		})

		It("UT-AF-1189-119: FunctionResponse with nil Response map -> fallback text, no panic", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_stream_investigation",
					Response: nil,
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigation completed"))
		})

		It("UT-AF-1189-120: kubernaut_start_investigation response with session_id -> started summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_start_investigation",
					Response: map[string]any{
						"session_id": "sess-abc-123",
						"status":     "started",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigation started"))
			Expect(tp.Text).To(ContainSubstring("sess-abc-123"))
		})
	})

	Describe("Text passthrough", func() {
		It("UT-AF-1189-130: LLM reasoning text -> passed through unchanged", func() {
			reasoning := "Node shows DiskPressure, emptyDir usage at 72%. The PostgreSQL StatefulSet is consuming excessive disk."
			part := &genai.Part{Text: reasoning}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal(reasoning))
		})

		It("UT-AF-1189-131: Text part with Thought=true -> passed through with metadata", func() {
			part := &genai.Part{
				Text:    "I should check the node resource utilization next.",
				Thought: true,
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("I should check the node resource utilization next."))
		})

		It("UT-AF-1189-132: empty text part -> passed through (not dropped)", func() {
			part := &genai.Part{Text: ""}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(BeEmpty())
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
					Name: "kubernaut_start_investigation",
					Args: map[string]any{"namespace": largeValue},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Starting investigation"))
			Expect(len(tp.Text)).To(BeNumerically("<", 1024), "status text should be bounded")
		})

		It("UT-AF-1189-151: FunctionResponse with 100KB Response -> summary truncated", func() {
			largeValue := strings.Repeat("analysis data... ", 6000)
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "kubernaut_stream_investigation",
					Response: map[string]any{
						"summary": largeValue,
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(len(tp.Text)).To(BeNumerically("<", 2048), "summary should be bounded")
		})

		It("UT-AF-1189-152: FunctionCall with special chars in tool name -> generic fallback", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "../../etc/passwd",
					Args: map[string]any{},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Processing"))
		})
	})
})
