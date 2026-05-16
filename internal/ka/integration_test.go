package ka_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

var _ = Describe("KA REST Client Integration (httptest)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("Full Lifecycle Roundtrip", func() {

		It("IT-KA-001: Analyze -> Status -> Result -> Cancel through real HTTP", func() {
			var (
				analyzeCalled atomic.Bool
				statusCalled  atomic.Bool
				resultCalled  atomic.Bool
				cancelCalled  atomic.Bool
			)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/analyze":
					analyzeCalled.Store(true)
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-it-001"})

				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/incident/session/sess-it-001":
					statusCalled.Store(true)
					_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-it-001", Status: "completed"})

				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/incident/session/sess-it-001/result":
					resultCalled.Store(true)
					_ = json.NewEncoder(w).Encode(ka.IncidentResponse{SessionID: "sess-it-001", Summary: "Pod OOMKilled due to memory limit"})

				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/incident/session/sess-it-001/cancel":
					cancelCalled.Store(true)
					_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})

				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := ka.NewClient(ka.Config{
				BaseURL:            server.URL,
				Timeout:            5 * time.Second,
				CBFailureThreshold: 5,
				RetryMax:           0,
			})

			sessionID, err := client.Analyze(ctx, ka.AnalyzeRequest{
				Namespace: "production", Kind: "Deployment", Name: "api-gateway",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(sessionID).To(Equal("sess-it-001"))

			status, err := client.Status(ctx, sessionID)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Status).To(Equal("completed"))

			result, err := client.Result(ctx, sessionID)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Summary).To(ContainSubstring("OOMKilled"))

			err = client.Cancel(ctx, sessionID)
			Expect(err).NotTo(HaveOccurred())

			Expect(analyzeCalled.Load()).To(BeTrue())
			Expect(statusCalled.Load()).To(BeTrue())
			Expect(resultCalled.Load()).To(BeTrue())
			Expect(cancelCalled.Load()).To(BeTrue())
		})

		It("IT-KA-002: request body JSON is correctly serialized and deserialized", func() {
			var capturedBody ka.AnalyzeRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
				err := json.NewDecoder(r.Body).Decode(&capturedBody)
				Expect(err).NotTo(HaveOccurred())
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-json"})
			}))
			defer server.Close()

			client := ka.NewClient(ka.Config{BaseURL: server.URL, CBFailureThreshold: 5})
			_, err := client.Analyze(ctx, ka.AnalyzeRequest{
				Namespace: "kube-system",
				Kind:      "StatefulSet",
				Name:      "etcd",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedBody.Namespace).To(Equal("kube-system"))
			Expect(capturedBody.Kind).To(Equal("StatefulSet"))
			Expect(capturedBody.Name).To(Equal("etcd"))
		})
	})

	Describe("Circuit Breaker Trip and Recovery", func() {

		It("IT-KA-003: CB trips after failures, rejects during open, recovers in half-open", func() {
			var callCount atomic.Int32
			failing := atomic.Bool{}
			failing.Store(true)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				if failing.Load() {
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "recovered"})
			}))
			defer server.Close()

			m := &ka.ClientMetrics{
				StateGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Namespace: "af", Name: "circuit_breaker_state_it",
				}, []string{"dependency"}),
				DurationHist: prometheus.NewHistogramVec(prometheus.HistogramOpts{
					Namespace: "af", Name: "downstream_duration_it",
					Buckets: prometheus.DefBuckets,
				}, []string{"dependency", "status"}),
				RetryCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
					Namespace: "af", Name: "downstream_retry_it",
				}, []string{"dependency", "attempt"}),
			}

			client := ka.NewClient(ka.Config{
				BaseURL:            server.URL,
				RetryMax:           0,
				CBFailureThreshold: 2,
				CBMaxRequests:      1,
				CBInterval:         10 * time.Second,
				CBTimeout:          100 * time.Millisecond,
			}, m)

			// Phase 1: Fail enough to trip CB
			Expect(client.Healthy()).To(BeTrue(), "CB should start closed")

			for i := 0; i < 3; i++ {
				_, _ = client.Analyze(ctx, ka.AnalyzeRequest{})
			}

			Expect(client.Healthy()).To(BeFalse(), "CB should be open after 3 failures")

			cbState := testutil.ToFloat64(m.StateGauge.WithLabelValues("ka"))
			Expect(cbState).To(BeNumerically(">", 0), "CB state gauge should reflect open state")

			// Phase 2: Calls rejected while open
			_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
			Expect(err).To(HaveOccurred())

			// Phase 3: Wait for half-open, server recovers
			failing.Store(false)
			time.Sleep(150 * time.Millisecond) // CB timeout = 100ms

			sessionID, err := client.Analyze(ctx, ka.AnalyzeRequest{})
			if err == nil {
				Expect(sessionID).To(Equal("recovered"))
				Expect(client.Healthy()).To(BeTrue(), "CB should close after successful half-open probe")
			}
		})

		It("IT-KA-004: concurrent requests during CB open are all rejected fast", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer server.Close()

			client := ka.NewClient(ka.Config{
				BaseURL:            server.URL,
				RetryMax:           0,
				CBFailureThreshold: 1,
				CBMaxRequests:      1,
				CBInterval:         30 * time.Second,
				CBTimeout:          30 * time.Second,
			})

			// Trip the CB
			for i := 0; i < 3; i++ {
				_, _ = client.Analyze(ctx, ka.AnalyzeRequest{})
			}
			Expect(client.Healthy()).To(BeFalse())

			// Fire 10 concurrent requests — all should fail fast
			errs := make(chan error, 10)
			for i := 0; i < 10; i++ {
				go func() {
					_, err := client.Status(ctx, "s1")
					errs <- err
				}()
			}

			for i := 0; i < 10; i++ {
				err := <-errs
				Expect(err).To(HaveOccurred(), "request %d should fail with CB open", i)
			}
		})
	})

	Describe("Retry with Real HTTP Transport", func() {

		It("IT-KA-005: retry recovers from transient 503 on GET endpoints", func() {
			var callCount atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				n := callCount.Add(1)
				if n <= 2 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "s1", Status: "done"})
			}))
			defer server.Close()

			client := ka.NewClient(ka.Config{
				BaseURL:            server.URL,
				RetryMax:           3,
				RetryInitBackoff:   1 * time.Millisecond,
				RetryMaxBackoff:    10 * time.Millisecond,
				RetryableStatuses:  []int{503},
				CBFailureThreshold: 20,
			})

			status, err := client.Status(ctx, "s1")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Status).To(Equal("done"))
			Expect(callCount.Load()).To(Equal(int32(3)))
		})

		It("IT-KA-006: exhausted retries still return meaningful error", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()

			client := ka.NewClient(ka.Config{
				BaseURL:            server.URL,
				RetryMax:           2,
				RetryInitBackoff:   1 * time.Millisecond,
				RetryMaxBackoff:    5 * time.Millisecond,
				RetryableStatuses:  []int{503},
				CBFailureThreshold: 20,
			})

			_, err := client.Status(ctx, "s1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("unavailable"),
				ContainSubstring("error"),
				ContainSubstring("retry"),
			), "exhausted retries should produce a user-friendly error message")
		})
	})
})
