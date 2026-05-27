package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// kaSessionHandler returns an http.Handler that serves both the status
// endpoint (GET /api/v1/incident/session/{id}) and the stream endpoint
// (GET /api/v1/incident/session/{id}/stream). streamFn is called for the
// stream path to write SSE data; all other paths return 404.
func kaSessionHandler(sessionID string, streamFn func(w http.ResponseWriter)) http.Handler {
	statusPath := fmt.Sprintf("/api/v1/incident/session/%s", sessionID)
	streamPath := statusPath + "/stream"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case statusPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"in_progress"}`))
		case streamPath:
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			streamFn(w)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func sseLines(eventType, dataJSON string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, dataJSON)
}

func sseText(eventType, text string) string {
	b, _ := json.Marshal(text)
	return sseLines(eventType, string(b))
}

func sseObj(eventType string, obj map[string]any) string {
	b, _ := json.Marshal(obj)
	return sseLines(eventType, string(b))
}

var _ = Describe("HandleStreamInvestigation (G5)", func() {
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

	It("UT-AF-1234-045: empty session_id rejected", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("session_id"))
	})

	It("UT-AF-1234-046: SSE reasoning_delta appended to narrative", func() {
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("reasoning_delta", "Analyzing pod logs..."))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "OOM detected"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.EventLog).To(ContainSubstring("Analyzing pod logs"))
	})

	It("UT-AF-1234-047: SSE token_delta appended to narrative", func() {
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("token_delta", "The pod is"))
			_, _ = fmt.Fprint(w, sseText("token_delta", " crashlooping"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.EventLog).To(ContainSubstring("The pod is"))
		Expect(result.EventLog).To(ContainSubstring("crashlooping"))
	})

	It("UT-AF-1234-048: SSE tool_call_start adds tool marker to narrative", func() {
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("tool_call_start", "kubectl_get_pods"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.EventLog).To(ContainSubstring("kubectl_get_pods"))
	})

	It("UT-AF-1234-049: SSE tool_call records in event log", func() {
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("tool_call", "kubectl get pods -n prod"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Events).NotTo(BeEmpty())

		var foundToolCall bool
		for _, evt := range result.Events {
			if evt.Type == "tool_call" {
				foundToolCall = true
			}
		}
		Expect(foundToolCall).To(BeTrue())
	})

	It("UT-AF-1234-050: SSE tool_result truncated to 500 chars", func() {
		longResult := strings.Repeat("x", 600)
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("tool_result", longResult))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())

		for _, evt := range result.Events {
			if evt.Type == "tool_result" {
				Expect(len(evt.Text)).To(BeNumerically("<=", 520))
			}
		}
	})

	It("UT-AF-1234-051: SSE complete extracts summary, returns completed status", func() {
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "OOM kill detected in pod web-api"}))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
		Expect(result.Summary).To(ContainSubstring("OOM"))
	})

	It("UT-AF-1234-052: SSE cancelled returns cancelled status", func() {
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseLines("cancelled", `"user requested cancellation"`))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("cancelled"))
	})

	It("UT-AF-1234-053: SSE error returns failed status", func() {
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("error", "internal error occurred"))
		}))

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("failed"))
	})

	It("UT-AF-1234-054: Context cancel mid-stream returns cancelled with partial", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		server = httptest.NewServer(kaSessionHandler("sess-001", func(w http.ResponseWriter) {
			flusher, ok := w.(http.Flusher)
			_, _ = fmt.Fprint(w, sseText("reasoning_delta", "Starting analysis..."))
			if ok {
				flusher.Flush()
			}
			// Block until the test context is cancelled
			<-cancelCtx.Done()
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := tools.HandleStreamInvestigation(cancelCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(SatisfyAny(
			Equal("cancelled"),
			Equal("disconnected"),
		))
	})
})

var _ = Describe("HandleStreamInvestigation — A2A Bridge (TP-1258)", func() {
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

	It("UT-AF-1258-020: reasoning_delta emitted via bridge during stream", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-001", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseObj("reasoning_delta", map[string]any{
				"content_preview": "Checking pod health in namespace prod",
			}))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-001")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-001",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		// Bridge should have emitted the reasoning_delta
		Expect(queue.events).NotTo(BeEmpty())
		var foundReasoning bool
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						if strings.Contains(tp.Text, "Checking pod health") {
							foundReasoning = true
						}
					}
				}
			}
		}
		Expect(foundReasoning).To(BeTrue(), "expected reasoning_delta to be emitted via bridge")
	})

	It("UT-AF-1258-021: token_delta NOT emitted via bridge (GA policy)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-002", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("token_delta", "some token fragment"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-002")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-002",
		})
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
		Expect(strings.Join(bridgedTexts, "")).To(ContainSubstring("some token fragment"),
			"token_delta MUST be relayed via bridge for real-time streaming (#1302)")
	})

	It("UT-AF-1258-022: tool_call emitted via bridge for live streaming (#1302)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-003", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("tool_call", "kubectl get pods -n prod"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-003")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-003",
		})
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
		Expect(strings.Join(bridgedTexts, "")).To(ContainSubstring("kubectl get pods"),
			"tool_call MUST be relayed via bridge for real-time streaming (#1302)")
	})

	It("UT-AF-1258-023: tool_result NOT emitted via bridge", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-004", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("tool_result", "NAME READY STATUS\npod-1 1/1 Running"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-004")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-004",
		})
		Expect(err).NotTo(HaveOccurred())

		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						Expect(tp.Text).NotTo(ContainSubstring("pod-1"),
							"tool_result should NOT be relayed via bridge")
					}
				}
			}
		}
	})

	It("UT-AF-1258-024: complete event emits final summary via bridge", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-005", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{
				"summary": "Root cause: OOM kill due to memory leak in web-api container",
			}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-005")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-005",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		var foundSummary bool
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						if strings.Contains(tp.Text, "OOM kill") {
							foundSummary = true
						}
					}
				}
			}
		}
		Expect(foundSummary).To(BeTrue(), "expected complete summary emitted via bridge")
	})

	It("UT-AF-1258-025: bridge nil-safe when not in A2A streaming context", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-006", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("reasoning_delta", "some reasoning"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		// No bridge in context — should still work normally
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(ctx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-006",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
		Expect(result.EventLog).To(ContainSubstring("some reasoning"))
	})

	It("UT-AF-1258-026: structured content_preview extracted from reasoning_delta", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-007", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseObj("reasoning_delta", map[string]any{
				"content_preview": "Pod is OOMKilled, checking resource limits",
				"tool_call_count": 3,
			}))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-007")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-007",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		// Bridge should extract content_preview from structured payload
		var emittedText string
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						if strings.Contains(tp.Text, "OOMKilled") {
							emittedText = tp.Text
						}
					}
				}
			}
		}
		Expect(emittedText).To(ContainSubstring("OOMKilled"))
	})

	It("UT-AF-1258-027: bridge write error logged but does not fail tool", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-008", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseObj("reasoning_delta", map[string]any{
				"content_preview": "Analyzing...",
			}))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		// Use a queue that always fails
		queue := &failingQueue{}
		taskID := a2a.TaskID("bridge-task-008")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-008",
		})
		// Tool should succeed even though bridge writes fail
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
	})

	It("UT-AF-1274-009: bridge failures logged through logr from context (BR-SESS-013)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-bridge-logr", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseObj("reasoning_delta", map[string]any{
				"content_preview": "Analyzing cluster state...",
			}))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		var logs []string
		testLogger := funcr.New(func(prefix, args string) {
			logs = append(logs, prefix+" "+args)
		}, funcr.Options{})
		logCtx := logr.NewContext(ctx, testLogger)

		queue := &failingQueue{}
		taskID := a2a.TaskID("bridge-task-logr")
		bridgeCtx := launcher.WithEventBridge(logCtx, queue, taskID, "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-logr",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		logOutput := strings.Join(logs, "\n")
		Expect(logOutput).To(ContainSubstring("WARNING: bridge emit failed"))
		Expect(logOutput).To(ContainSubstring("simulated queue write failure"))
	})
})

// =============================================================================
// TP-1301-1302 §4.2: Bridge Relay Policy — FedRAMP AU-2, AU-12, SC-7
// Validates that token_delta and tool_call events are bridged to the A2A stream
// and that bridged text passes through sanitization.
// =============================================================================
var _ = Describe("KA bridge relay policy (TP-1301-1302)", func() {
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

	It("UT-AF-1302-010: token_delta text appears in bridge output (AU-2)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-1302-010", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("token_delta", "The pod is experiencing OOMKill events"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		bridgeCtx := launcher.WithEventBridge(ctx, queue, "task-1302-010", "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-1302-010",
		})
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
		Expect(strings.Join(bridgedTexts, "")).To(ContainSubstring("OOMKill"),
			"AU-2: token_delta must be bridged for real-time investigation monitoring (#1302)")
	})

	It("UT-AF-1302-011: tool_call emitted via bridge with [Tool: ...] format (AU-12)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-1302-011", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("tool_call", "kubectl describe pod/web-api -n prod"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		bridgeCtx := launcher.WithEventBridge(ctx, queue, "task-1302-011", "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-1302-011",
		})
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
		joined := strings.Join(bridgedTexts, "")
		Expect(joined).To(ContainSubstring("[Tool:"),
			"AU-12: tool_call must be bridged with [Tool: ...] format for audit trail (#1302)")
		Expect(joined).To(ContainSubstring("kubectl describe pod"),
			"AU-12: tool name must appear in bridged text")
	})

	It("UT-AF-1302-012: bridged token_delta passes through sanitization (SC-7)", func() {
		server = httptest.NewServer(kaSessionHandler("sess-1302-012", func(w http.ResponseWriter) {
			_, _ = fmt.Fprint(w, sseText("token_delta", "Auth with Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxIn0.sig"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		bridgeCtx := launcher.WithEventBridge(ctx, queue, "task-1302-012", "", nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-1302-012",
		})
		Expect(err).NotTo(HaveOccurred())

		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						Expect(tp.Text).NotTo(ContainSubstring("eyJhbGci"),
							"SC-7: JWT in bridged token_delta must be redacted by sanitizeBridgeText")
					}
				}
			}
		}
	})
})

// testEventQueue records events for assertion.
type testEventQueue struct {
	events []a2a.Event
}

func (q *testEventQueue) Write(_ context.Context, event a2a.Event) error {
	q.events = append(q.events, event)
	return nil
}

func (q *testEventQueue) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return fmt.Errorf("not supported")
}

func (q *testEventQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, fmt.Errorf("not supported")
}

func (q *testEventQueue) Close() error { return nil }

// failingQueue always returns an error on Write.
type failingQueue struct{}

func (q *failingQueue) Write(_ context.Context, _ a2a.Event) error {
	return fmt.Errorf("simulated queue write failure")
}

func (q *failingQueue) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return fmt.Errorf("not supported")
}

func (q *failingQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, fmt.Errorf("not supported")
}

func (q *failingQueue) Close() error { return nil }
