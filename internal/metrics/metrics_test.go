package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/metrics"
)

func TestMetricsSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

var _ = Describe("Metrics Registry", func() {
	var reg *metrics.Registry

	BeforeEach(func() {
		reg = metrics.NewRegistry()
	})

	It("UT-AF-MET-001: creates a non-nil registry with all collectors", func() {
		Expect(reg).NotTo(BeNil())
		Expect(reg.HTTPRequestsTotal).NotTo(BeNil())
		Expect(reg.HTTPRequestDuration).NotTo(BeNil())
		Expect(reg.ToolCallsTotal).NotTo(BeNil())
		Expect(reg.ToolCallDuration).NotTo(BeNil())
		Expect(reg.SessionsActive).NotTo(BeNil())
		Expect(reg.LLMTokensTotal).NotTo(BeNil())
		Expect(reg.RateLimitDenied).NotTo(BeNil())
		Expect(reg.CircuitBreakerState).NotTo(BeNil())
		Expect(reg.AuthDuration).NotTo(BeNil())
		Expect(reg.AuditEventsTotal).NotTo(BeNil())
	})

	It("UT-AF-MET-002: Handler returns valid Prometheus exposition", func() {
		reg.HTTPRequestsTotal.WithLabelValues("POST", "/a2a", "200").Inc()
		reg.SessionsActive.WithLabelValues("Active").Set(3)

		rec := httptest.NewRecorder()
		reg.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", http.NoBody))

		Expect(rec.Code).To(Equal(200))
		body, err := io.ReadAll(rec.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("af_http_requests_total"))
		Expect(string(body)).To(ContainSubstring("af_sessions_active"))
	})

	It("UT-AF-MET-003: counter labels are properly constrained", func() {
		Expect(func() {
			reg.HTTPRequestsTotal.WithLabelValues("POST", "/a2a", "200").Inc()
		}).NotTo(Panic())

		Expect(func() {
			reg.ToolCallsTotal.WithLabelValues("af_list_events", "success").Inc()
		}).NotTo(Panic())

		Expect(func() {
			reg.LLMTokensTotal.WithLabelValues("input", "claude-sonnet-4-6").Inc()
		}).NotTo(Panic())

		Expect(func() {
			reg.AuditEventsTotal.WithLabelValues("triage_started").Inc()
		}).NotTo(Panic())
	})

	It("UT-AF-MET-004: histogram records observations without error", func() {
		reg.HTTPRequestDuration.WithLabelValues("POST", "/a2a", "200").Observe(0.150)
		reg.ToolCallDuration.WithLabelValues("af_get_pods", "internal").Observe(0.050)
		reg.AuthDuration.WithLabelValues("success").Observe(0.025)

		rec := httptest.NewRecorder()
		reg.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", http.NoBody))

		body, err := io.ReadAll(rec.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("af_http_request_duration_seconds"))
		Expect(string(body)).To(ContainSubstring("af_tool_call_duration_seconds"))
		Expect(string(body)).To(ContainSubstring("af_auth_duration_seconds"))
	})

	It("UT-AF-MET-005: go runtime and process collectors are present", func() {
		rec := httptest.NewRecorder()
		reg.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", http.NoBody))

		body, err := io.ReadAll(rec.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(ContainSubstring("go_goroutines"))
		Expect(string(body)).To(ContainSubstring("process_resident_memory_bytes"))
	})

	It("UT-AF-MET-006: rate limit metric supports tier and reason labels", func() {
		Expect(func() {
			reg.RateLimitDenied.WithLabelValues("ip", "burst_exceeded").Inc()
			reg.RateLimitDenied.WithLabelValues("user", "request_rate").Inc()
		}).NotTo(Panic())
	})
})
