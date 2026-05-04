package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_poll_investigation", func() {
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

	It("UT-AF-112-001: returns in_progress with user-friendly progress string", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: "investigating"})
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandlePollInvestigation(ctx, kaClient, tools.PollInvestigationArgs{SessionID: "sess-1"}, 1, 100*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("in_progress"))
		Expect(result.Progress).NotTo(BeEmpty())
	})

	It("UT-AF-112-002: returns complete with RCA summary when done", func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/incident/session/sess-1", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: "completed"})
		})
		mux.HandleFunc("/api/v1/incident/session/sess-1/result", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ka.IncidentResponse{SessionID: "sess-1", Summary: "Memory leak in pod-xyz"})
		})
		server = httptest.NewServer(mux)
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandlePollInvestigation(ctx, kaClient, tools.PollInvestigationArgs{SessionID: "sess-1"}, 1, 100*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
		Expect(result.Summary).To(ContainSubstring("Memory leak"))
	})

	It("UT-AF-112-003: blocks polling KA at configured interval", func() {
		var callCount atomic.Int32
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount.Add(1)
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: "investigating"})
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandlePollInvestigation(ctx, kaClient, tools.PollInvestigationArgs{SessionID: "sess-1"}, 3, 50*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("in_progress"))
		Expect(result.PollCount).To(Equal(3))
	})

	It("UT-AF-112-004: returns early on status change", func() {
		var callCount atomic.Int32
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := callCount.Add(1)
			status := "investigating"
			if c >= 2 {
				status = "completed"
			}
			if r.URL.Path == "/api/v1/incident/session/sess-1/result" {
				_ = json.NewEncoder(w).Encode(ka.IncidentResponse{SessionID: "sess-1", Summary: "Done"})
				return
			}
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: status})
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandlePollInvestigation(ctx, kaClient, tools.PollInvestigationArgs{SessionID: "sess-1"}, 10, 50*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
		Expect(result.PollCount).To(BeNumerically("<", 10))
	})

	It("UT-AF-112-005: tracks poll count and returns timeout after max polls", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: "investigating"})
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := tools.HandlePollInvestigation(ctx, kaClient, tools.PollInvestigationArgs{SessionID: "sess-1"}, 3, 10*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("in_progress"))
		Expect(result.PollCount).To(Equal(3))
	})

	It("UT-AF-112-006: respects context cancellation during blocking", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: "investigating"})
		}))
		kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})

		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		_, err := tools.HandlePollInvestigation(cancelCtx, kaClient, tools.PollInvestigationArgs{SessionID: "sess-1"}, 100, 50*time.Millisecond)
		Expect(err).To(HaveOccurred())
	})
})
