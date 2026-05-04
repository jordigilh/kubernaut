package ka_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

var _ = Describe("KA REST Client", func() {
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

	It("UT-AF-110-001: POST /api/v1/incident/analyze returns session_id", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.URL.Path).To(Equal("/api/v1/incident/analyze"))
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-123"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		sessionID, err := client.Analyze(ctx, ka.AnalyzeRequest{Namespace: "pay", Kind: "Deployment", Name: "api"})
		Expect(err).NotTo(HaveOccurred())
		Expect(sessionID).To(Equal("sess-123"))
	})

	It("UT-AF-110-002: GET /api/v1/incident/session/{id} returns status", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/api/v1/incident/session/sess-123"))
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-123", Status: "investigating"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		status, err := client.Status(ctx, "sess-123")
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Status).To(Equal("investigating"))
	})

	It("UT-AF-110-003: GET /api/v1/incident/session/{id}/result returns IncidentResponse", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/api/v1/incident/session/sess-123/result"))
			_ = json.NewEncoder(w).Encode(ka.IncidentResponse{SessionID: "sess-123", Summary: "RCA found"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		result, err := client.Result(ctx, "sess-123")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Summary).To(Equal("RCA found"))
	})

	It("UT-AF-110-004: POST /api/v1/incident/session/{id}/cancel cancels investigation", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Method).To(Equal(http.MethodPost))
			Expect(r.URL.Path).To(Equal("/api/v1/incident/session/sess-123/cancel"))
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-123", "status": "cancelled"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		err := client.Cancel(ctx, "sess-123")
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-110-005: forwards JWT via Authorization header", func() {
		var capturedAuth string
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-123"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL, Token: "my-jwt-token"})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedAuth).To(Equal("Bearer my-jwt-token"))
	})

	It("UT-AF-110-006: returns circuit-open error when KA unreachable", func() {
		client := ka.NewClient(ka.Config{BaseURL: "http://127.0.0.1:1", CBMaxRequests: 1, CBFailureThreshold: 1})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).To(HaveOccurred())
	})
})
