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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
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
