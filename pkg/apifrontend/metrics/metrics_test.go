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

var _ = Describe("AF Metrics Registry (DD-TEST-005 Compliant — GA Set)", func() {
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

		It("UT-AF-MET-005: af_http_panics_total increments by 1", func() {
			before := getCounterValue(reg.HTTPPanicsTotal)
			reg.HTTPPanicsTotal.Inc()
			after := getCounterValue(reg.HTTPPanicsTotal)
			Expect(after - before).To(Equal(float64(1)))
		})

	})

	Context("Histogram Metrics — observation verification", func() {
		It("UT-AF-MET-007: af_http_request_duration_seconds records observation", func() {
			obs := reg.HTTPRequestDuration.WithLabelValues("POST", "/mcp", "200")
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.250)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.250, 0.001))
		})

		It("UT-AF-MET-008: af_tool_call_duration_seconds records observation", func() {
			obs := reg.ToolCallDuration.WithLabelValues("af_get_pods", "mcp")
			beforeCount := getHistogramSampleCount(obs)

			obs.Observe(0.075)

			afterCount := getHistogramSampleCount(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
		})

		It("UT-AF-MET-009: af_downstream_request_duration_seconds records observation", func() {
			obs := reg.DownstreamDuration.WithLabelValues("ka", "2xx")
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.120)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.120, 0.001))
		})

		It("UT-AF-MET-010: af_auth_duration_seconds records observation", func() {
			obs := reg.AuthDuration.WithLabelValues("success")
			beforeCount := getHistogramSampleCount(obs)
			beforeSum := getHistogramSampleSum(obs)

			obs.Observe(0.015)

			afterCount := getHistogramSampleCount(obs)
			afterSum := getHistogramSampleSum(obs)
			Expect(afterCount - beforeCount).To(Equal(uint64(1)))
			Expect(afterSum - beforeSum).To(BeNumerically("~", 0.015, 0.001))
		})
	})

	Context("Gauge Metrics — set verification", func() {
		It("UT-AF-MET-011: af_sessions_active reflects set value", func() {
			reg.SessionsActive.WithLabelValues("Active").Set(5)
			value := getGaugeValue(reg.SessionsActive.WithLabelValues("Active"))
			Expect(value).To(Equal(float64(5)))
		})

		It("UT-AF-MET-012: af_circuit_breaker_state reflects set value", func() {
			reg.CircuitBreakerState.WithLabelValues("ka").Set(1)
			value := getGaugeValue(reg.CircuitBreakerState.WithLabelValues("ka"))
			Expect(value).To(Equal(float64(1)))
		})

		It("UT-AF-MET-013: af_sse_active_connections reflects set value", func() {
			reg.SSEActiveConnections.Set(3)
			value := getGaugeValue(reg.SSEActiveConnections)
			Expect(value).To(Equal(float64(3)))
		})
	})

	Context("Registry Completeness — all 13 GA metrics wired after construction", func() {
		It("UT-AF-MET-014: Gather returns exactly 13 af_* metric families after observation", func() {
			reg.HTTPRequestsTotal.WithLabelValues("GET", "/", "200").Inc()
			reg.HTTPRequestDuration.WithLabelValues("GET", "/", "200").Observe(0.01)
			reg.HTTPPanicsTotal.Inc()
			reg.ToolCallsTotal.WithLabelValues("t", "s").Inc()
			reg.ToolCallDuration.WithLabelValues("t", "mcp").Observe(0.01)
			reg.CircuitBreakerState.WithLabelValues("ka").Set(0)
			reg.DownstreamDuration.WithLabelValues("ka", "2xx").Observe(0.01)
			reg.AuthDuration.WithLabelValues("s").Observe(0.01)
			reg.RateLimitDenied.WithLabelValues("u", "r").Inc()
			reg.SSEActiveConnections.Set(1)
			reg.LLMTokensTotal.WithLabelValues("i", "m").Inc()
			reg.SessionsActive.WithLabelValues("Active").Set(1)

			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())

			afNames := make([]string, 0)
			for _, f := range families {
				name := f.GetName()
				if len(name) >= 3 && name[:3] == "af_" {
					afNames = append(afNames, name)
				}
			}

			expected := []string{
				"af_http_requests_total",
				"af_http_request_duration_seconds",
				"af_http_panics_total",
				"af_tool_calls_total",
				"af_tool_call_duration_seconds",
				"af_circuit_breaker_state",
				"af_downstream_request_duration_seconds",
				"af_auth_duration_seconds",
				"af_rate_limit_rejections_total",
				"af_sse_active_connections",
				"af_llm_tokens_total",
				"af_sessions_active",
			}

			for _, name := range expected {
				Expect(afNames).To(ContainElement(name),
					"metric %q must be gatherable from registry after observation", name)
			}
			Expect(afNames).To(HaveLen(len(expected)),
				"expected exactly %d af_* metrics, got %d: %v", len(expected), len(afNames), afNames)
		})
	})
})
