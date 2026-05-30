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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue(), "expected *a2a.TextPart")
			Expect(tp.Text).To(ContainSubstring("Investigating"))
			Expect(tp.Text).To(ContainSubstring("prod"))
			Expect(tp.Text).To(ContainSubstring("api-server"))
		})

		It("UT-AF-1189-101: kubernaut_investigate -> investigating status", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{"session_id": "sess-42"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigating"))
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
			Expect(tp.Text).To(Equal("...\n\n"))
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
					Name: "kubernaut_investigate",
					Args: map[string]any{
						"namespace": func() {}, // not JSON-serializable
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigating"))
		})

		It("UT-AF-1189-109: kubectl_list -> listing cluster resources", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubectl_list",
					Args: map[string]any{"namespace": "production"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Listing cluster resources"))
		})

		It("UT-AF-1189-110: kubectl_list_events -> fetching cluster events", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubectl_list_events",
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Root cause: PostgreSQL uses emptyDir"))
			Expect(tp.Text).NotTo(ContainSubstring(`"summary"`))
		})

		It("UT-AF-1189-112: kubernaut_investigate response with unknown structure -> fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_investigate",
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

		It("UT-AF-1189-114: af_create_rr response with rr_id -> human-friendly summary", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "af_create_rr",
					Response: map[string]any{
						"rr_id":   "rr-disk-pressure-abc123",
						"message": "RemediationRequest created for Deployment/web by alice",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Remediation request created"))
			Expect(tp.Text).NotTo(ContainSubstring(`"rr_id"`), "should not dump raw JSON keys")
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
						"events": []any{
							map[string]any{"phase": "Executing", "resource": "RemediationRequest"},
						},
						"status": "completed",
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
					Name:     "kubernaut_investigate",
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigation started"))
			Expect(tp.Text).To(ContainSubstring("sess-abc-123"))
		})
	})

	Describe("Text passthrough", func() {
		It("UT-AF-1189-130: LLM reasoning text -> passed through with trailing paragraph break", func() {
			reasoning := "Node shows DiskPressure, emptyDir usage at 72%. The PostgreSQL StatefulSet is consuming excessive disk."
			part := &genai.Part{Text: reasoning}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal(reasoning + "\n\n"),
				"LLM text must get trailing paragraph break so consecutive chunks are readable (#1301)")
		})

		It("UT-AF-1189-131: Text part with Thought=true -> activity indicator, not raw thought", func() {
			part := &genai.Part{
				Text:    "I should check the node resource utilization next.",
				Thought: true,
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Analyzing...\n\n"))
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
					Name: "kubernaut_investigate",
					Args: map[string]any{"namespace": largeValue},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigating"))
			Expect(len(tp.Text)).To(BeNumerically("<", 1024), "status text should be bounded")
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
			Expect(tp.Text).To(Equal("...\n\n"))
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Investigation completed.\n\n"))
		})

		It("UT-AF-1189-155: kubernaut_select_workflow response with message only -> message text", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_select_workflow",
					Response: map[string]any{"message": "Workflow selected for execution"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Workflow selected for execution\n\n"))
		})

		It("UT-AF-1189-156: kubernaut_select_workflow response with status only -> formatted status", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_select_workflow",
					Response: map[string]any{"status": "queued"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Workflow queued.\n\n"))
		})

		It("UT-AF-1189-157: kubernaut_select_workflow response with empty fields -> generic fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_select_workflow",
					Response: map[string]any{"other": "data"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Workflow selection completed.\n\n"))
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Remediation completed (final phase: Executing)\n\n"))
		})

		It("UT-AF-1189-159: kubernaut_watch response with status only -> status text", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_watch",
					Response: map[string]any{"status": "completed"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Remediation completed.\n\n"))
		})

		It("UT-AF-1189-160: kubernaut_watch response with empty fields -> generic fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubernaut_watch",
					Response: map[string]any{"other": "data"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Watching remediation...\n\n"))
		})

		It("UT-AF-1189-161: af_create_rr response without rr_id -> human-friendly fallback", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "af_create_rr",
					Response: map[string]any{"already_exists": true},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("already exists"))
			Expect(tp.Text).NotTo(ContainSubstring(`"`), "should not contain JSON syntax")
		})
	})

	Describe("Output suppression (#1282 F-OUT)", func() {
		It("UT-AF-1282-OUT-001: af_create_rr new RR → human-friendly text, no raw JSON", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "af_create_rr",
					Response: map[string]any{
						"rr_id":   "rr-oom-abc",
						"message": "RemediationRequest created for Deployment/web by sre-user",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Remediation request created"))
			Expect(tp.Text).NotTo(ContainSubstring(`"rr_id"`))
			Expect(tp.Text).NotTo(ContainSubstring(`"message"`))
		})

		It("UT-AF-1282-OUT-002: af_create_rr existing RR → human-friendly, no JSON syntax", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "af_create_rr",
					Response: map[string]any{
						"already_exists": true,
						"rr_id":          "rr-existing",
					},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("already exists"))
			Expect(tp.Text).To(ContainSubstring("rr-existing"))
			Expect(tp.Text).NotTo(ContainSubstring(`{`))
		})

		It("UT-AF-1282-OUT-003: non-key tool response → nil (payload suppressed)", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubectl_get",
					Response: map[string]any{"data": "large payload"},
				},
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "non-key tool payloads must be suppressed")
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal(multiByteStr+"\n\n"), "rune-safe truncation should preserve string when rune count is within limit")
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(HaveSuffix("...\n\n"))
			Expect(len([]rune(tp.Text))).To(BeNumerically("<=", 1026))
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("valid-workflow"))
			Expect(tp.Text).NotTo(ContainSubstring("not-a-map"))
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("restart-pod"))
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("No workflows discovered.\n\n"))
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
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("- scale-up"))
			Expect(tp.Text).NotTo(ContainSubstring("confidence"))
		})
	})

	Describe("Thought-to-activity indicator (AC 10)", func() {
		It("UT-AF-1189-167: Thought=true with long reasoning -> activity indicator, not raw thought", func() {
			part := &genai.Part{
				Text:    "Let me analyze the disk pressure situation. The node shows high emptyDir usage which is likely caused by the PostgreSQL StatefulSet writing temporary data.",
				Thought: true,
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Analyzing...\n\n"))
		})

		It("UT-AF-1189-168: Thought=false with text -> text gets trailing paragraph break", func() {
			text := "Based on the analysis, the root cause is disk pressure from emptyDir."
			part := &genai.Part{
				Text:    text,
				Thought: false,
			}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal(text + "\n\n"),
				"LLM text must get trailing paragraph break so consecutive chunks are readable (#1301)")
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
		result, err := convert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		tp, ok := result.(*a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(HaveSuffix("\n\n"),
			"status messages must end with \\n\\n so concatenated SSE frames are readable (#1301)")
	})

	It("UT-AF-1301-002: Thought activity indicator ends with paragraph break", func() {
		part := &genai.Part{Text: "thinking...", Thought: true}
		result, err := convert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		tp, ok := result.(*a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(HaveSuffix("\n\n"),
			"thought indicator must end with \\n\\n for readability (#1301)")
	})

	It("UT-AF-1301-003: FunctionResponse summary ends with paragraph break", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     "af_create_rr",
				Response: map[string]any{"rr_id": "rr-test-001"},
			},
		}
		result, err := convert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		tp, ok := result.(*a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(HaveSuffix("\n\n"),
			"tool response summaries must end with \\n\\n for readability (#1301)")
	})

	It("UT-AF-1301-004: LLM text already ending with paragraph break gets no triple newline", func() {
		text := "The root cause is disk pressure.\n\n"
		part := &genai.Part{Text: text}
		result, err := convert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		tp, ok := result.(*a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(Equal(text),
			"text already ending with \\n\\n must NOT get additional newlines")
	})

	It("UT-AF-1301-005: consecutive LLM text chunks are separated by paragraph break", func() {
		chunks := []string{
			"I'll start by checking what's running in the demo-crashloop namespace.",
			"Let me investigate the affected deployment.",
		}
		var results []string
		for _, chunk := range chunks {
			part := &genai.Part{Text: chunk}
			result, err := convert(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			results = append(results, tp.Text)
		}
		concatenated := strings.Join(results, "")
		Expect(concatenated).To(ContainSubstring("namespace.\n\n"),
			"LLM text chunks must get trailing paragraph break appended "+
				"so consecutive chunks render as separate paragraphs (#1301)")
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
		result, err := convert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(),
			"tool errors must be surfaced on the SSE stream, not silently dropped (#1302)")
		tp, ok := result.(*a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(ContainSubstring("cannot resolve GVK"))
	})

	It("UT-AF-1302-002: non-key tool success response still nil (unchanged)", func() {
		part := &genai.Part{
			FunctionResponse: &genai.FunctionResponse{
				Name:     "kubectl_get",
				Response: map[string]any{"data": "large payload"},
			},
		}
		result, err := convert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(BeNil(),
			"non-key tool success payloads must still be suppressed")
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
		result, err := convert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(),
			"MCP tool errors with status=error must be surfaced on SSE stream (#1302)")
		tp, ok := result.(*a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(ContainSubstring("not_driving"),
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
		result, err := nonStreamConvert(context.Background(), nil, part)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil(), "tool errors must always be surfaced (#1302)")
		tp, ok := result.(*a2a.TextPart)
		Expect(ok).To(BeTrue())
		Expect(tp.Text).To(ContainSubstring("connection refused"),
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
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "kubernaut_investigate FunctionResponse should be suppressed in streaming mode")
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
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("workflow"))
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
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).NotTo(BeEmpty())
		})

		It("UT-AF-1258-033: af_create_rr FunctionResponse -> brief status", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name: "af_create_rr",
					Response: map[string]any{
						"rr_id":     "rr-oom-001",
						"namespace": "prod",
						"status":    "created",
					},
				},
			}
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).NotTo(BeEmpty())
		})

		It("UT-AF-1258-034: kubectl_* FunctionResponse -> nil (unchanged behavior)", func() {
			part := &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     "kubectl_get_pods",
					Response: map[string]any{"output": "NAME READY STATUS\npod-1 1/1 Running"},
				},
			}
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil(), "kubectl FunctionResponse should still be nil in streaming mode")
		})

		It("UT-AF-1258-035: FunctionCall parts unchanged in streaming mode", func() {
			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: "kubernaut_investigate",
					Args: map[string]any{"session_id": "sess-42"},
				},
			}
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Investigating"))
		})

		It("UT-AF-1258-036: Thought parts unchanged in streaming mode", func() {
			part := &genai.Part{
				Text:    "Analyzing the disk usage patterns...",
				Thought: true,
			}
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(Equal("Analyzing...\n\n"))
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
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Creating remediation request"))
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
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("Remediation request created"))
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
			result, err := convertStreaming(context.Background(), nil, part)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			tp, ok := result.(*a2a.TextPart)
			Expect(ok).To(BeTrue())
			Expect(tp.Text).To(ContainSubstring("already exists"))
		})
	})
})
