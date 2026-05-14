package ka_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

var _ = Describe("KA REST Client Resilience", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-038-040: retries GET on 503 up to RetryMax", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempts.Add(1)
			if n <= 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "s1", Status: "done"})
		}))
		defer server.Close()

		client := ka.NewClient(ka.Config{
			BaseURL:           server.URL,
			RetryMax:          3,
			RetryInitBackoff:  1 * time.Millisecond,
			RetryMaxBackoff:   5 * time.Millisecond,
			RetryableStatuses: []int{503},
		})
		status, err := client.Status(ctx, "s1")
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Status).To(Equal("done"))
		Expect(attempts.Load()).To(BeNumerically("==", 3))
	})

	It("UT-AF-038-041: does NOT retry POST /analyze (IdempotentOnly)", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		client := ka.NewClient(ka.Config{
			BaseURL:           server.URL,
			RetryMax:          3,
			RetryInitBackoff:  1 * time.Millisecond,
			RetryMaxBackoff:   5 * time.Millisecond,
			RetryableStatuses: []int{503},
		})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{Namespace: "ns", Name: "n"})
		Expect(err).To(HaveOccurred())
		Expect(attempts.Load()).To(BeNumerically("==", 1))
	})

	It("UT-AF-038-042: CB opens after consecutive failures", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		client := ka.NewClient(ka.Config{
			BaseURL:            server.URL,
			RetryMax:           0,
			CBFailureThreshold: 2,
			CBMaxRequests:      1,
			CBInterval:         10 * time.Second,
			CBTimeout:          50 * time.Millisecond,
		})

		Expect(client.Healthy()).To(BeTrue())

		for i := 0; i < 3; i++ {
			_, _ = client.Analyze(ctx, ka.AnalyzeRequest{})
		}

		Expect(client.Healthy()).To(BeFalse())
	})

	It("UT-AF-038-043: Healthy() returns true when CB is closed", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "s1"})
		}))
		defer server.Close()

		client := ka.NewClient(ka.Config{BaseURL: server.URL})
		Expect(client.Healthy()).To(BeTrue())
	})

	It("UT-AF-038-044: user-friendly error on connection refused", func() {
		client := ka.NewClient(ka.Config{
			BaseURL:            "http://127.0.0.1:1",
			Timeout:            100 * time.Millisecond,
			RetryMax:           0,
			CBFailureThreshold: 10,
		})
		_, err := client.Status(ctx, "s1")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unavailable"))
	})

	It("UT-AF-038-045: user-friendly error on 403", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()

		client := ka.NewClient(ka.Config{BaseURL: server.URL, RetryMax: 0, CBFailureThreshold: 10})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("denied"))
	})

	It("UT-AF-038-046: user-friendly error on 500", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := ka.NewClient(ka.Config{BaseURL: server.URL, RetryMax: 0, CBFailureThreshold: 10})
		_, err := client.Analyze(ctx, ka.AnalyzeRequest{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("error"))
	})

	Describe("Metrics Instrumentation (TC-P1-03)", func() {
		newMetrics := func() (*ka.ClientMetrics, *prometheus.Registry) {
			reg := prometheus.NewRegistry()
			stateGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: "af", Name: "circuit_breaker_state",
			}, []string{"dependency"})
			durationHist := prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: "af", Name: "downstream_request_duration_seconds",
				Buckets: prometheus.DefBuckets,
			}, []string{"dependency", "status"})
			retryCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace: "af", Name: "downstream_retry_total",
			}, []string{"dependency", "attempt"})
			reg.MustRegister(stateGauge, durationHist, retryCounter)
			return &ka.ClientMetrics{
				StateGauge:   stateGauge,
				DurationHist: durationHist,
				RetryCounter: retryCounter,
			}, reg
		}

		scrapeMetrics := func(reg *prometheus.Registry) string {
			h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			body, _ := io.ReadAll(rec.Result().Body)
			return string(body)
		}

		It("TC-P1-03a: CB opens after threshold → state gauge goes to 1", func() {
			m, reg := newMetrics()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer server.Close()

			client := ka.NewClient(ka.Config{
				BaseURL:            server.URL,
				RetryMax:           0,
				CBFailureThreshold: 2,
				CBMaxRequests:      1,
				CBInterval:         10 * time.Second,
				CBTimeout:          50 * time.Millisecond,
			}, m)

			for i := 0; i < 3; i++ {
				_, _ = client.Analyze(ctx, ka.AnalyzeRequest{})
			}

			metricsText := scrapeMetrics(reg)
			Expect(metricsText).To(ContainSubstring(`af_circuit_breaker_state{dependency="ka"}`))
		})

		It("TC-P1-03b: retry counter increments on 503", func() {
			m, _ := newMetrics()
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
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
			}, m)
			_, _ = client.Status(ctx, "s1")

			Expect(testutil.ToFloat64(m.RetryCounter.WithLabelValues("ka", "2"))).To(BeNumerically(">=", 1))
		})

		It("TC-P1-03c: duration histogram populated on successful call", func() {
			m, reg := newMetrics()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "s1", Status: "done"})
			}))
			defer server.Close()

			client := ka.NewClient(ka.Config{BaseURL: server.URL}, m)
			_, err := client.Status(ctx, "s1")
			Expect(err).NotTo(HaveOccurred())

			metricsText := scrapeMetrics(reg)
			Expect(metricsText).To(ContainSubstring("af_downstream_request_duration_seconds"))
			Expect(metricsText).To(ContainSubstring(`dependency="ka"`))
		})
	})
})
