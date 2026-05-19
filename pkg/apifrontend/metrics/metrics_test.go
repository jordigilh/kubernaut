package metrics_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
)

func TestMetricsSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

func getCounterValue(counter prometheus.Counter) float64 {
	metric := &dto.Metric{}
	if err := counter.Write(metric); err != nil {
		return 0
	}
	return metric.GetCounter().GetValue()
}

func getGaugeValue(gauge prometheus.Gauge) float64 {
	metric := &dto.Metric{}
	if err := gauge.Write(metric); err != nil {
		return 0
	}
	return metric.GetGauge().GetValue()
}

func getHistogramSampleCount(obs prometheus.Observer) uint64 {
	metric := &dto.Metric{}
	obs.(prometheus.Metric).Write(metric) //nolint:errcheck
	return metric.GetHistogram().GetSampleCount()
}

func getHistogramSampleSum(obs prometheus.Observer) float64 {
	metric := &dto.Metric{}
	obs.(prometheus.Metric).Write(metric) //nolint:errcheck
	return metric.GetHistogram().GetSampleSum()
}

var _ = Describe("AF Metrics Registry (DD-TEST-005 Compliant)", func() {
	var reg *metrics.Registry

	BeforeEach(func() {
		reg = metrics.NewRegistry()
	})

	Context("Counter Metrics — value verification", func() {
		It("UT-AF-MET-001: af_http_requests_total increments by 1", func() {
			c := reg.HTTPRequestsTotal.WithLabelValues("POST", "/mcp", "200")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-002: af_tool_calls_total increments by 1", func() {
			c := reg.ToolCallsTotal.WithLabelValues("af_get_pods", "success")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-003: af_llm_tokens_total adds exact value", func() {
			c := reg.LLMTokensTotal.WithLabelValues("input", "test-model")
			before := getCounterValue(c)
			c.Add(150)
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(150)))
		})

		It("UT-AF-MET-004: af_rate_limit_rejections_total increments by 1", func() {
			c := reg.RateLimitDenied.WithLabelValues("user", "burst_exceeded")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-005: af_downstream_retry_total increments by 1", func() {
			c := reg.DownstreamRetryTotal.WithLabelValues("ka", "2")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-006: af_audit_events_total increments by 1", func() {
			c := reg.AuditEventsTotal.WithLabelValues("mcp.tool_call")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-007: af_session_ttl_actions_total increments by 1", func() {
			c := reg.SessionTTLActionsTotal.WithLabelValues("evicted")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-008: af_mcp_rbac_denied_total increments by 1", func() {
			c := reg.MCPRBACDeniedTotal.WithLabelValues("kubernaut_approve")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-009: af_http_panics_total increments by 1", func() {
			before := getCounterValue(reg.HTTPPanicsTotal)
			reg.HTTPPanicsTotal.Inc()
			after := getCounterValue(reg.HTTPPanicsTotal)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-010: af_audit_buffer_overflow_total increments by 1", func() {
			before := getCounterValue(reg.AuditBufferOverflow)
			reg.AuditBufferOverflow.Inc()
			after := getCounterValue(reg.AuditBufferOverflow)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-011: af_severity_triage_total increments by 1", func() {
			c := reg.SeverityTriageTotal.WithLabelValues("rule", "high")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-012: af_severity_triage_errors_total increments by 1", func() {
			c := reg.SeverityTriageErrorsTotal.WithLabelValues("llm", "timeout")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-013: af_discover_workflows_total increments by 1", func() {
			c := reg.DiscoverWorkflowsTotal.WithLabelValues("success")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})

		It("UT-AF-MET-014: af_discover_workflows_errors_total increments by 1", func() {
			c := reg.DiscoverWorkflowsErrorsTotal.WithLabelValues("mcp_unavailable")
			before := getCounterValue(c)
			c.Inc()
			after := getCounterValue(c)
			Expect(after - before).To(Equal(float64(1)))
		})
	})

	Context("Histogram Metrics — observation verification", func() {
		It("UT-AF-MET-015: af_http_request_duration_seconds records observation", func() {
			obs := reg.HTTPRequestDuration.WithLabelValues("POST", "/mcp", "200")
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.250)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.250, 0.001))
		})

		It("UT-AF-MET-016: af_tool_call_duration_seconds records observation", func() {
			obs := reg.ToolCallDuration.WithLabelValues("af_get_pods", "mcp")
			beforeCount := getHistogramSampleCount(obs)

			obs.Observe(0.075)

			afterCount := getHistogramSampleCount(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
		})

		It("UT-AF-MET-017: af_downstream_request_duration_seconds records observation", func() {
			obs := reg.DownstreamDuration.WithLabelValues("ka", "2xx")
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.120)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.120, 0.001))
		})

		It("UT-AF-MET-018: af_auth_duration_seconds records observation", func() {
			obs := reg.AuthDuration.WithLabelValues("success")
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.015)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.015, 0.001))
		})

		It("UT-AF-MET-019: af_severity_triage_duration_seconds records observation", func() {
			obs := reg.SeverityTriageDuration.WithLabelValues("rule")
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.042)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.042, 0.001))
		})

		It("UT-AF-MET-020: af_discover_workflows_duration_seconds records observation", func() {
			obs := reg.DiscoverWorkflowsDuration.WithLabelValues()
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.088)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.088, 0.001))
		})
	})

	Context("Gauge Metrics — set verification", func() {
		It("UT-AF-MET-021: af_sessions_active reflects set value", func() {
			reg.SessionsActive.WithLabelValues("active").Set(5)
			value := getGaugeValue(reg.SessionsActive.WithLabelValues("active"))
			Expect(value).To(Equal(float64(5)))
		})

		It("UT-AF-MET-022: af_circuit_breaker_state reflects set value", func() {
			reg.CircuitBreakerState.WithLabelValues("ka").Set(1)
			value := getGaugeValue(reg.CircuitBreakerState.WithLabelValues("ka"))
			Expect(value).To(Equal(float64(1)))
		})

		It("UT-AF-MET-023: af_sse_active_connections reflects set value", func() {
			reg.SSEActiveConnections.Set(3)
			value := getGaugeValue(reg.SSEActiveConnections)
			Expect(value).To(Equal(float64(3)))
		})
	})

	Context("Registry Completeness — all fields wired after construction", func() {
		It("UT-AF-MET-024: Gather returns all 23 af_* metric families after observation", func() {
			reg.HTTPRequestsTotal.WithLabelValues("GET", "/", "200").Inc()
			reg.HTTPRequestDuration.WithLabelValues("GET", "/", "200").Observe(0.01)
			reg.ToolCallsTotal.WithLabelValues("t", "s").Inc()
			reg.ToolCallDuration.WithLabelValues("t", "mcp").Observe(0.01)
			reg.SessionsActive.WithLabelValues("a").Set(1)
			reg.LLMTokensTotal.WithLabelValues("i", "m").Inc()
			reg.RateLimitDenied.WithLabelValues("u", "r").Inc()
			reg.CircuitBreakerState.WithLabelValues("ka").Set(0)
			reg.DownstreamDuration.WithLabelValues("ka", "2xx").Observe(0.01)
			reg.DownstreamRetryTotal.WithLabelValues("ka", "1").Inc()
			reg.AuthDuration.WithLabelValues("s").Observe(0.01)
			reg.AuditEventsTotal.WithLabelValues("t").Inc()
			reg.SessionTTLActionsTotal.WithLabelValues("e").Inc()
			reg.MCPRBACDeniedTotal.WithLabelValues("t").Inc()
			reg.HTTPPanicsTotal.Inc()
			reg.SSEActiveConnections.Set(1)
			reg.AuditBufferOverflow.Inc()
			reg.SeverityTriageTotal.WithLabelValues("r", "h").Inc()
			reg.SeverityTriageDuration.WithLabelValues("r").Observe(0.01)
			reg.SeverityTriageErrorsTotal.WithLabelValues("r", "t").Inc()
			reg.DiscoverWorkflowsTotal.WithLabelValues("s").Inc()
			reg.DiscoverWorkflowsDuration.WithLabelValues().Observe(0.01)
			reg.DiscoverWorkflowsErrorsTotal.WithLabelValues("u").Inc()

			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())

			names := make(map[string]bool)
			for _, f := range families {
				names[f.GetName()] = true
			}

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
				Expect(names).To(HaveKey(name),
					"metric %q must be gatherable from registry after observation", name)
			}
		})
	})
})
