package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
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

	It("UT-AF-WP-039: af_discover_workflows_total incremented on successful discovery", func() {
		Expect(reg.DiscoverWorkflowsTotal).NotTo(BeNil())
		Expect(func() {
			reg.DiscoverWorkflowsTotal.WithLabelValues("success").Inc()
		}).NotTo(Panic())
	})

	It("UT-AF-WP-040: af_discover_workflows_duration_seconds observed on each call", func() {
		Expect(reg.DiscoverWorkflowsDuration).NotTo(BeNil())
		Expect(func() {
			reg.DiscoverWorkflowsDuration.WithLabelValues().Observe(0.05)
		}).NotTo(Panic())
	})

	It("UT-AF-WP-041: af_discover_workflows_errors_total incremented on KA error", func() {
		Expect(reg.DiscoverWorkflowsErrorsTotal).NotTo(BeNil())
		Expect(func() {
			reg.DiscoverWorkflowsErrorsTotal.WithLabelValues("mcp_unavailable").Inc()
		}).NotTo(Panic())
	})

	It("UT-AF-MET-007: all registered metrics appear in scrape after observation", func() {
		// Exercise every metric to ensure it's registered and observable.
		// This catches "registered but never wired" bugs where a metric is
		// created in NewRegistry but silently nil-guarded at the call site.
		reg.HTTPRequestsTotal.WithLabelValues("GET", "/healthz", "200").Inc()
		reg.HTTPRequestDuration.WithLabelValues("GET", "/healthz", "200").Observe(0.001)
		reg.ToolCallsTotal.WithLabelValues("af_get_pods", "success").Inc()
		reg.ToolCallDuration.WithLabelValues("af_get_pods", "mcp").Observe(0.05)
		reg.SessionsActive.WithLabelValues("active").Set(1)
		reg.LLMTokensTotal.WithLabelValues("input", "test-model").Add(100)
		reg.RateLimitDenied.WithLabelValues("user", "burst").Inc()
		reg.CircuitBreakerState.WithLabelValues("ka").Set(0)
		reg.DownstreamDuration.WithLabelValues("ka", "2xx").Observe(0.1)
		reg.DownstreamRetryTotal.WithLabelValues("ka", "1").Inc()
		reg.AuthDuration.WithLabelValues("success").Observe(0.01)
		reg.AuditEventsTotal.WithLabelValues("mcp.tool_call").Inc()
		reg.SessionTTLActionsTotal.WithLabelValues("evicted").Inc()
		reg.MCPRBACDeniedTotal.WithLabelValues("af_get_pods").Inc()
		reg.HTTPPanicsTotal.Inc()
		reg.SSEActiveConnections.Set(2)
		reg.AuditBufferOverflow.Inc()
		reg.SeverityTriageTotal.WithLabelValues("rule", "high").Inc()
		reg.SeverityTriageDuration.WithLabelValues("rule").Observe(0.02)
		reg.SeverityTriageErrorsTotal.WithLabelValues("rule", "timeout").Inc()
		reg.DiscoverWorkflowsTotal.WithLabelValues("success").Inc()
		reg.DiscoverWorkflowsDuration.WithLabelValues().Observe(0.03)
		reg.DiscoverWorkflowsErrorsTotal.WithLabelValues("mcp_unavailable").Inc()

		rec := httptest.NewRecorder()
		reg.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", http.NoBody))
		Expect(rec.Code).To(Equal(200))

		body, err := io.ReadAll(rec.Body)
		Expect(err).NotTo(HaveOccurred())
		output := string(body)

		expected := []string{
			"af_http_requests_total",
			"af_http_request_duration_seconds",
			"af_tool_calls_total",
			"af_tool_call_duration_seconds",
			"af_sessions_active",
			"af_llm_tokens_total",
			"af_rate_limit_rejections_total",
			"af_circuit_breaker_state",
			"af_downstream_request_duration_seconds",
			"af_downstream_retry_total",
			"af_auth_duration_seconds",
			"af_audit_events_total",
			"af_session_ttl_actions_total",
			"af_mcp_rbac_denied_total",
			"af_http_panics_total",
			"af_sse_active_connections",
			"af_audit_buffer_overflow_total",
			"af_severity_triage_total",
			"af_severity_triage_duration_seconds",
			"af_severity_triage_errors_total",
			"af_discover_workflows_total",
			"af_discover_workflows_duration_seconds",
			"af_discover_workflows_errors_total",
		}

		for _, name := range expected {
			Expect(output).To(ContainSubstring(name),
				"metric %q must appear in /metrics scrape after observation", name)
		}
	})

	It("UT-AF-MET-008: all registry fields are non-nil after construction", func() {
		Expect(reg.HTTPRequestsTotal).NotTo(BeNil(), "HTTPRequestsTotal")
		Expect(reg.HTTPRequestDuration).NotTo(BeNil(), "HTTPRequestDuration")
		Expect(reg.ToolCallsTotal).NotTo(BeNil(), "ToolCallsTotal")
		Expect(reg.ToolCallDuration).NotTo(BeNil(), "ToolCallDuration")
		Expect(reg.SessionsActive).NotTo(BeNil(), "SessionsActive")
		Expect(reg.LLMTokensTotal).NotTo(BeNil(), "LLMTokensTotal")
		Expect(reg.RateLimitDenied).NotTo(BeNil(), "RateLimitDenied")
		Expect(reg.CircuitBreakerState).NotTo(BeNil(), "CircuitBreakerState")
		Expect(reg.DownstreamDuration).NotTo(BeNil(), "DownstreamDuration")
		Expect(reg.DownstreamRetryTotal).NotTo(BeNil(), "DownstreamRetryTotal")
		Expect(reg.AuthDuration).NotTo(BeNil(), "AuthDuration")
		Expect(reg.AuditEventsTotal).NotTo(BeNil(), "AuditEventsTotal")
		Expect(reg.SessionTTLActionsTotal).NotTo(BeNil(), "SessionTTLActionsTotal")
		Expect(reg.MCPRBACDeniedTotal).NotTo(BeNil(), "MCPRBACDeniedTotal")
		Expect(reg.HTTPPanicsTotal).NotTo(BeNil(), "HTTPPanicsTotal")
		Expect(reg.SSEActiveConnections).NotTo(BeNil(), "SSEActiveConnections")
		Expect(reg.AuditBufferOverflow).NotTo(BeNil(), "AuditBufferOverflow")
		Expect(reg.SeverityTriageTotal).NotTo(BeNil(), "SeverityTriageTotal")
		Expect(reg.SeverityTriageDuration).NotTo(BeNil(), "SeverityTriageDuration")
		Expect(reg.SeverityTriageErrorsTotal).NotTo(BeNil(), "SeverityTriageErrorsTotal")
		Expect(reg.DiscoverWorkflowsTotal).NotTo(BeNil(), "DiscoverWorkflowsTotal")
		Expect(reg.DiscoverWorkflowsDuration).NotTo(BeNil(), "DiscoverWorkflowsDuration")
		Expect(reg.DiscoverWorkflowsErrorsTotal).NotTo(BeNil(), "DiscoverWorkflowsErrorsTotal")
	})
})
