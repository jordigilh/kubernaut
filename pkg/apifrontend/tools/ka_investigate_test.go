package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// kaInvestigateHandler builds an HTTP handler that simulates KA endpoints for
// the merged kubernaut_investigate tool. It handles:
//   - POST /api/v1/incident (Analyze)
//   - GET  /api/v1/incident/session/{id} (Status)
//   - GET  /api/v1/incident/session/{id}/result (Result)
//   - GET  /api/v1/incident/session/{id}/stream (StreamEvents)
func kaInvestigateHandler(sessionID, status string, streamFn func(w http.ResponseWriter), summary string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/analyze":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": sessionID})

		case r.URL.Path == fmt.Sprintf("/api/v1/incident/session/%s/stream", sessionID):
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if streamFn != nil {
				streamFn(w)
			}

		case r.URL.Path == fmt.Sprintf("/api/v1/incident/session/%s/result", sessionID):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ka.IncidentResponse{SessionID: sessionID, Summary: summary})

		case r.URL.Path == fmt.Sprintf("/api/v1/incident/session/%s", sessionID):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: sessionID, Status: status})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// =============================================================================
// TP-1307-MERGE §4.1: Core Handler Logic — FedRAMP AU-2, AU-12, SI-4, SC-7
// =============================================================================
var _ = Describe("HandleInvestigation — merged tool (TP-1307-MERGE)", func() {
	var (
		ctx    context.Context
		server *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	// --- BR-INVESTIGATE-001: New investigation (no session_id) ---

	It("UT-AF-INV-001: new investigation calls Analyze then StreamEvents, returns summary (AU-2, AU-12)", func() {
		spy := &spyEmitter{}
		server = httptest.NewServer(kaInvestigateHandler("sess-new-001", "in_progress",
			func(w http.ResponseWriter) {
				_, _ = fmt.Fprint(w, sseText("reasoning_delta", "Analyzing pod logs..."))
				_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "OOM detected in web-api"}))
			}, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			Namespace: "prod",
			Name:      "web-api",
			Kind:      "Deployment",
		}, spy)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SessionID).To(Equal("sess-new-001"))
		Expect(result.Status).To(Equal("completed"))
		Expect(result.Summary).To(ContainSubstring("OOM detected"))

		delegated := spy.eventsByType(audit.EventKADelegated)
		Expect(delegated).NotTo(BeEmpty(), "AU-2: audit event for delegation must be emitted")
		Expect(delegated[0].Detail["session_id"]).To(Equal("sess-new-001"))

		received := spy.eventsByType(audit.EventKAResultReceived)
		Expect(received).NotTo(BeEmpty(), "AU-12: ka.result_received must be emitted when stream completes")
		Expect(received[0].Detail["result_type"]).To(Equal("rca_complete"))
		Expect(received[0].Detail["ka_correlation_id"]).To(Equal("sess-new-001"))
	})

	It("UT-AF-INV-015: new investigation emits ka.result_received with rca_failed on stream error (AU-12)", func() {
		spy := &spyEmitter{}
		server = httptest.NewServer(kaInvestigateHandler("sess-fail-015", "in_progress",
			func(w http.ResponseWriter) {
				_, _ = fmt.Fprint(w, sseText("error", "LLM provider timeout"))
			}, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			Namespace: "prod", Name: "web-api",
		}, spy)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("failed"))

		received := spy.eventsByType(audit.EventKAResultReceived)
		Expect(received).NotTo(BeEmpty(), "AU-12: ka.result_received must be emitted on stream failure")
		Expect(received[0].Detail["result_type"]).To(Equal("rca_failed"))
	})

	It("UT-AF-INV-016: resume in-progress emits ka.result_received on stream completion (AU-12)", func() {
		spy := &spyEmitter{}
		server = httptest.NewServer(kaInvestigateHandler("sess-resume-016", "in_progress",
			func(w http.ResponseWriter) {
				_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "Memory leak identified"}))
			}, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			SessionID: "sess-resume-016",
		}, spy)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		received := spy.eventsByType(audit.EventKAResultReceived)
		Expect(received).NotTo(BeEmpty(), "AU-12: resume+stream must emit result event")
		Expect(received[0].Detail["result_type"]).To(Equal("rca_complete"))
	})

	It("UT-AF-INV-012: Analyze returns error — no stream attempted (AU-2)", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			Namespace: "prod",
			Name:      "web-api",
		}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("starting investigation"))
	})

	// --- BR-INVESTIGATE-001: Missing args validation ---

	It("UT-AF-INV-005: missing namespace and name with no session_id returns validation error (SC-7)", func() {
		kaClient := ka.NewClient(ka.Config{BaseURL: "http://127.0.0.1:1"})
		_, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("namespace"))
	})

	// --- BR-INVESTIGATE-002: Resume in-flight (session_id present) ---

	It("UT-AF-INV-002: resume in-flight session calls Status then StreamEvents (SI-4)", func() {
		server = httptest.NewServer(kaInvestigateHandler("sess-resume-001", "in_progress",
			func(w http.ResponseWriter) {
				_, _ = fmt.Fprint(w, sseText("token_delta", "Continuing analysis..."))
				_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "CrashLoopBackOff resolved"}))
			}, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			SessionID: "sess-resume-001",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
		Expect(result.Summary).To(ContainSubstring("CrashLoopBackOff"))
	})

	It("UT-AF-INV-006: invalid session_id returns clear error (SC-7)", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			SessionID: "nonexistent-session",
		}, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("does not exist"))
	})

	// --- BR-INVESTIGATE-003: Poll completed (session_id, completed) ---

	It("UT-AF-INV-003: completed session returns result directly (AU-3)", func() {
		spy := &spyEmitter{}
		server = httptest.NewServer(kaInvestigateHandler("sess-done-001", "completed", nil, "Memory leak in pod-xyz"))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			SessionID: "sess-done-001",
		}, spy)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
		Expect(result.Summary).To(ContainSubstring("Memory leak"))

		received := spy.eventsByType(audit.EventKAResultReceived)
		Expect(received).NotTo(BeEmpty(), "AU-3: audit event for result must contain session_id and result_type")
		Expect(received[0].Detail["result_type"]).To(Equal("rca_complete"))
	})

	It("UT-AF-INV-004: failed session returns failed status (AU-12)", func() {
		spy := &spyEmitter{}
		server = httptest.NewServer(kaInvestigateHandler("sess-fail-001", "failed", nil, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			SessionID: "sess-fail-001",
		}, spy)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("failed"))

		received := spy.eventsByType(audit.EventKAResultReceived)
		Expect(received).NotTo(BeEmpty(), "AU-12: audit event must be emitted for failed investigation")
		Expect(received[0].Detail["result_type"]).To(Equal("rca_failed"))
	})

	It("UT-AF-INV-011: cancelled session returns cancelled status", func() {
		server = httptest.NewServer(kaInvestigateHandler("sess-cancel-001", "cancelled", nil, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			SessionID: "sess-cancel-001",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("cancelled"))
	})

	// --- BR-INVESTIGATE-005: Bridge streaming ---

	It("UT-AF-INV-007: bridge emits reasoning_delta and token_delta (SI-4)", func() {
		server = httptest.NewServer(kaInvestigateHandler("sess-bridge-001", "in_progress",
			func(w http.ResponseWriter) {
				_, _ = fmt.Fprint(w, sseText("reasoning_delta", "Checking resource limits"))
				_, _ = fmt.Fprint(w, sseText("token_delta", "Pod is OOMKilled"))
				_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
			}, ""))

		queue := &testEventQueue{}
		bridgeCtx := launcher.WithEventBridge(ctx, queue, "task-inv-007", "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(bridgeCtx, kaClient, tools.InvestigateArgs{
			Namespace: "prod",
			Name:      "web-api",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		var bridgedTexts []string
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						bridgedTexts = append(bridgedTexts, tp.Text)
					}
				}
			}
		}
		joined := strings.Join(bridgedTexts, "")
		Expect(joined).To(ContainSubstring("Checking resource limits"), "reasoning_delta must be bridged")
		Expect(joined).To(ContainSubstring("OOMKilled"), "token_delta must be bridged")
	})

	It("UT-AF-INV-008: bridge emits tool_call with [Tool: ...] format (SI-4)", func() {
		server = httptest.NewServer(kaInvestigateHandler("sess-bridge-002", "in_progress",
			func(w http.ResponseWriter) {
				_, _ = fmt.Fprint(w, sseText("tool_call_start", "kubectl_get_pods"))
				_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
			}, ""))

		queue := &testEventQueue{}
		bridgeCtx := launcher.WithEventBridge(ctx, queue, "task-inv-008", "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleInvestigation(bridgeCtx, kaClient, tools.InvestigateArgs{
			Namespace: "prod",
			Name:      "web-api",
		}, nil)
		Expect(err).NotTo(HaveOccurred())

		var bridgedTexts []string
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						bridgedTexts = append(bridgedTexts, tp.Text)
					}
				}
			}
		}
		Expect(strings.Join(bridgedTexts, "")).To(ContainSubstring("[Tool:"),
			"tool_call must be bridged with [Tool: ...] format")
	})

	// --- Stream lifecycle ---

	It("UT-AF-INV-009: stream disconnect mid-investigation returns disconnected (AU-2)", func() {
		server = httptest.NewServer(kaInvestigateHandler("sess-disc-001", "in_progress",
			func(w http.ResponseWriter) {
				_, _ = fmt.Fprint(w, sseText("reasoning_delta", "Starting..."))
				// Stream ends without complete event
			}, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			Namespace: "prod",
			Name:      "web-api",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("disconnected"))
	})

	It("UT-AF-INV-010: context cancelled during stream returns cancelled", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		server = httptest.NewServer(kaInvestigateHandler("sess-cancel-002", "in_progress",
			func(w http.ResponseWriter) {
				flusher, ok := w.(http.Flusher)
				_, _ = fmt.Fprint(w, sseText("reasoning_delta", "Analyzing..."))
				if ok {
					flusher.Flush()
				}
				<-cancelCtx.Done()
			}, ""))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := tools.HandleInvestigation(cancelCtx, kaClient, tools.InvestigateArgs{
			Namespace: "prod",
			Name:      "web-api",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(SatisfyAny(
			Equal("cancelled"),
			Equal("disconnected"),
		))
	})
})

// =============================================================================
// TP-1310 §4.2: Error Bridging During Investigation Streaming — FedRAMP SI-4
// =============================================================================
var _ = Describe("streamInvestigation — error bridging (TP-1310)", func() {
	var (
		ctx    context.Context
		server *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("UT-AF-1310-001: tool_result with error IS bridged via EventBridge (SI-4)", func() {
		errPayload := struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}{Status: "error", Error: "nodes.config.openshift.io not found"}
		errJSON, _ := json.Marshal(errPayload)

		server = httptest.NewServer(kaSessionHandler("sess-1310-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseToolResult("kubectl_describe", string(errJSON)))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "investigation complete"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("task-1310-001")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(bridgeCtx, kaClient, tools.InvestigateArgs{
			Namespace: "default", Name: "test-pod",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		var bridgedTexts []string
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						bridgedTexts = append(bridgedTexts, tp.Text)
					}
				}
			}
		}
		joined := strings.Join(bridgedTexts, "\n")
		Expect(joined).To(ContainSubstring("[Error: kubectl_describe"),
			"SI-4: tool errors MUST be bridged for progressive monitoring")
	})

	It("UT-AF-1310-002: tool_result with success is NOT bridged as error (SI-4)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-1310-002", func(w http.ResponseWriter) {
			inner := map[string]any{
				"tool_name":      "kubectl_get_pods",
				"tool_index":     0,
				"result_preview": "NAME READY STATUS\npod-1 1/1 Running",
			}
			envelope := map[string]any{"type": "tool_result", "turn": 1, "data": inner}
			b, _ := json.Marshal(envelope)
			_, _ = fmt.Fprint(w, sseLines("tool_result", string(b)))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("task-1310-002")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(bridgeCtx, kaClient, tools.InvestigateArgs{
			Namespace: "default", Name: "test-pod",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						Expect(tp.Text).NotTo(ContainSubstring("[Error:"),
							"successful tool_result MUST NOT be bridged as error")
					}
				}
			}
		}
	})

	It("UT-AF-1310-003: multiple tool errors accumulated in ToolErrors (AU-3)", func() {
		errJSON1, _ := json.Marshal(struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}{Status: "error", Error: "Node not found"})
		errJSON2, _ := json.Marshal(struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}{Status: "error", Error: "permission denied"})

		server = httptest.NewServer(kaSessionHandler("sess-1310-003", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseToolResult("kubectl_describe", string(errJSON1)))
			_, _ = fmt.Fprint(w, sseToolResult("kubectl_get_yaml", string(errJSON2)))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "partial"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			Namespace: "default", Name: "test-pod",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ToolErrors).To(HaveLen(2))
		Expect(result.ToolErrors[0]).To(ContainSubstring("kubectl_describe"))
		Expect(result.ToolErrors[1]).To(ContainSubstring("kubectl_get_yaml"))
	})

	It("UT-AF-1310-004: zero tool errors means ToolErrors is empty", func() {
		server = httptest.NewServer(kaSessionHandler("sess-1310-004", func(w http.ResponseWriter) {
			inner := map[string]any{
				"tool_name":      "kubectl_get_pods",
				"tool_index":     0,
				"result_preview": "pod-1 Running",
			}
			envelope := map[string]any{"type": "tool_result", "turn": 1, "data": inner}
			b, _ := json.Marshal(envelope)
			_, _ = fmt.Fprint(w, sseLines("tool_result", string(b)))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			Namespace: "default", Name: "test-pod",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.ToolErrors).To(BeEmpty())
	})

	It("UT-AF-1310-005: error event bridged with investigation error prefix (SI-4)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-1310-005", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("error", "internal timeout from KA"))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("task-1310-005")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(bridgeCtx, kaClient, tools.InvestigateArgs{
			Namespace: "default", Name: "test-pod",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("failed"))

		var bridgedTexts []string
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						bridgedTexts = append(bridgedTexts, tp.Text)
					}
				}
			}
		}
		joined := strings.Join(bridgedTexts, "\n")
		Expect(joined).To(ContainSubstring("[Investigation error:"),
			"SI-4: error events MUST be bridged with [Investigation error:] prefix")
	})

	It("UT-AF-1310-006: ToolErrors populated on disconnected stream (AU-12)", func() {
		errJSON, _ := json.Marshal(struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}{Status: "error", Error: "connection refused"})

		server = httptest.NewServer(kaSessionHandler("sess-1310-006", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseToolResult("kubectl_describe", string(errJSON)))
			// Stream ends without complete event — simulates disconnect
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleInvestigation(ctx, kaClient, tools.InvestigateArgs{
			Namespace: "default", Name: "test-pod",
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("disconnected"))
		Expect(result.ToolErrors).To(HaveLen(1))
		Expect(result.ToolErrors[0]).To(ContainSubstring("kubectl_describe"))
	})
})

// =============================================================================
// TP-1310 §4.4: ToolErrors JSON Serialization (WT tier)
// =============================================================================
var _ = Describe("InvestigateResult.ToolErrors serialization (TP-1310)", func() {
	It("WT-AF-1310-031: empty ToolErrors omitted from JSON with omitempty", func() {
		result := tools.InvestigateResult{
			SessionID: "sess-xyz",
			Status:    "completed",
			Summary:   "done",
		}
		b, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(b)).NotTo(ContainSubstring("tool_errors"))
	})

	It("WT-AF-1310-031b: non-empty ToolErrors present in JSON output", func() {
		result := tools.InvestigateResult{
			SessionID:  "sess-xyz",
			Status:     "completed",
			Summary:    "done",
			ToolErrors: []string{"kubectl_describe: Node not found"},
		}
		b, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(b)).To(ContainSubstring("tool_errors"))
		Expect(string(b)).To(ContainSubstring("Node not found"))
	})
})

// =============================================================================
// TP-1307-MERGE §4.2: Constructor and Tool Metadata
// =============================================================================
var _ = Describe("NewInvestigateTool constructor (TP-1307-MERGE)", func() {
	It("UT-AF-INV-020: creates tool with name kubernaut_investigate", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		t, err := tools.NewInvestigateTool(kaClient, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.Name()).To(Equal("kubernaut_investigate"))
	})

	It("UT-AF-INV-021: tool description mentions investigation", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		t, err := tools.NewInvestigateTool(kaClient, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.Description()).To(ContainSubstring("investigation"))
	})
})
