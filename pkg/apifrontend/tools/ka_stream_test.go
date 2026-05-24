package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

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

	sessionStreamPath := func(sessionID string) string {
		return fmt.Sprintf("/api/v1/incident/session/%s/stream", sessionID)
	}

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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseText("reasoning_delta", "Starting analysis..."))
			if ok {
				flusher.Flush()
			}
			// Block until the test context is cancelled
			<-r.Context().Done()
		}))

		cancelCtx, cancel := context.WithCancel(ctx)
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

	sessionStreamPath := func(sessionID string) string {
		return fmt.Sprintf("/api/v1/incident/session/%s/stream", sessionID)
	}

	It("UT-AF-1258-020: reasoning_delta emitted via bridge during stream", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-001") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseObj("reasoning_delta", map[string]any{
				"content_preview": "Checking pod health in namespace prod",
			}))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-001")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, nil)

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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-002") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseText("token_delta", "some token fragment"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-002")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-002",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		// Bridge should NOT have emitted token_delta (GA policy: reasoning_delta only)
		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						Expect(tp.Text).NotTo(ContainSubstring("some token fragment"),
							"token_delta should NOT be relayed via bridge")
					}
				}
			}
		}
	})

	It("UT-AF-1258-022: tool_call NOT emitted via bridge", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-003") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseText("tool_call", "kubectl get pods -n prod"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-003")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-003",
		})
		Expect(err).NotTo(HaveOccurred())

		for _, evt := range queue.events {
			if artifact, ok := evt.(*a2a.TaskArtifactUpdateEvent); ok {
				for _, part := range artifact.Artifact.Parts {
					if tp, ok := part.(*a2a.TextPart); ok {
						Expect(tp.Text).NotTo(ContainSubstring("kubectl get pods"),
							"tool_call should NOT be relayed via bridge")
					}
				}
			}
		}
	})

	It("UT-AF-1258-023: tool_result NOT emitted via bridge", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-004") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseText("tool_result", "NAME READY STATUS\npod-1 1/1 Running"))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-004")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, nil)

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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-005") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{
				"summary": "Root cause: OOM kill due to memory leak in web-api container",
			}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-005")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, nil)

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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-006") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-007") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseObj("reasoning_delta", map[string]any{
				"content_preview": "Pod is OOMKilled, checking resource limits",
				"tool_call_count": 3,
			}))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		queue := &testEventQueue{}
		taskID := a2a.TaskID("bridge-task-007")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, nil)

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
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sessionStreamPath("sess-bridge-008") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, sseObj("reasoning_delta", map[string]any{
				"content_preview": "Analyzing...",
			}))
			_, _ = fmt.Fprint(w, sseObj("complete", map[string]any{"summary": "done"}))
		}))

		// Use a queue that always fails
		queue := &failingQueue{}
		taskID := a2a.TaskID("bridge-task-008")
		bridgeCtx := launcher.WithEventBridge(ctx, queue, taskID, nil)

		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandleStreamInvestigation(bridgeCtx, kaClient, tools.StreamInvestigationArgs{
			SessionID: "sess-bridge-008",
		})
		// Tool should succeed even though bridge writes fail
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
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
