package ka_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

// staticBearerTransport injects a fixed bearer token into outgoing requests,
// simulating the SA token transport pattern used in production.
type staticBearerTransport struct {
	token string
}

func (t *staticBearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(req)
}

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

	It("UT-AF-110-002b: SessionStatus unmarshals numeric status from KA v1.5", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// KA v1.5 may return status as a number instead of a string
			_, _ = w.Write([]byte(`{"session_id":"sess-num","status":200}`))
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		status, err := client.Status(ctx, "sess-num")
		Expect(err).NotTo(HaveOccurred())
		Expect(status.SessionID).To(Equal("sess-num"))
		Expect(status.Status).To(Equal("200"))
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

	It("UT-AF-110-005: does not inject context JWT (superseded by #1287 SA token model)", func() {
		var capturedAuth string
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-123"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
			Username:  "alice@example.com",
			RawToken:  "jwt-alice-123",
			ExpiresAt: time.Now().Add(10 * time.Minute),
		})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedAuth).To(BeEmpty(),
			"REST client no longer injects context JWT — AF uses SA token transport")
	})

	It("UT-AF-110-005b: no Authorization header when no identity in context", func() {
		var capturedAuth string
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-123"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedAuth).To(BeEmpty())
	})

	It("UT-AF-110-005c: request reaches KA even with expired JWT in context (SA token model)", func() {
		var requestReachedKA bool
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requestReachedKA = true
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-ok"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
			Username:  "alice@example.com",
			RawToken:  "expired-token",
			ExpiresAt: time.Now().Add(-1 * time.Minute),
		})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).NotTo(HaveOccurred(),
			"expired JWT in context should not block request — SA token is used")
		Expect(requestReachedKA).To(BeTrue(),
			"request should reach KA regardless of context JWT state")
	})

	It("UT-AF-110-005d: Status does not forward context JWT", func() {
		var capturedAuth string
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: "investigating"})
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
			Username:  "bob@example.com",
			RawToken:  "jwt-bob-456",
			ExpiresAt: time.Now().Add(10 * time.Minute),
		})
		_, err := client.Status(ctx, "sess-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedAuth).To(BeEmpty(),
			"Status should not inject context JWT")
	})

	It("UT-AF-110-005e: StreamEvents does not forward context JWT", func() {
		var capturedAuth string
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: complete\ndata: {\"type\":\"complete\"}\n\n")
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
			Username:  "carol@example.com",
			RawToken:  "jwt-carol-789",
			ExpiresAt: time.Now().Add(10 * time.Minute),
		})
		ch, err := client.StreamEvents(ctx, "sess-1")
		Expect(err).NotTo(HaveOccurred())
		for range ch {
		}
		Expect(capturedAuth).To(BeEmpty(),
			"StreamEvents should not inject context JWT")
	})

	It("UT-AF-1189-050: StreamEvents rejects non-SSE Content-Type", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"error":"wrong content type"}`)
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		_, err := client.StreamEvents(ctx, "sess-ct")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Content-Type"))
	})

	It("UT-AF-1189-051: StreamEvents accepts text/event-stream with charset", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			_, _ = fmt.Fprint(w, "event: complete\ndata: {}\n\n")
		}))
		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		ch, err := client.StreamEvents(ctx, "sess-ct2")
		Expect(err).NotTo(HaveOccurred())
		for range ch {
		}
	})

	It("UT-AF-110-006: returns circuit-open error when KA unreachable", func() {
		client := ka.NewClient(ka.Config{BaseURL: "http://127.0.0.1:1", CBMaxRequests: 1, CBFailureThreshold: 1})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).To(HaveOccurred())
	})

	Describe("SA Token Transport (#1287)", func() {
		It("UT-AF-1287-002: does NOT inject context JWT into Authorization header", func() {
			var capturedAuth string
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-sa"})
			}))

			client := ka.NewClient(ka.Config{BaseURL: server.URL})
			ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
				Username:  "alice@example.com",
				RawToken:  "jwt-should-not-appear",
				ExpiresAt: time.Now().Add(10 * time.Minute),
			})
			_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedAuth).To(BeEmpty(),
				"REST client should NOT inject context JWT — AF uses SA token transport configured by caller")
		})
	})
})
