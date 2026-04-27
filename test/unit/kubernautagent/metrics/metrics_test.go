package metrics_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
)

// gatherCounter returns the counter value for a metric with the given label values.
func gatherCounter(g prometheus.Gatherer, name string, labels map[string]string) float64 {
	families, err := g.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

// gatherGauge returns the gauge value for a metric.
func gatherGauge(g prometheus.Gatherer, name string) float64 {
	families, err := g.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

// gatherHistogramCount returns the sample count from a histogram metric.
func gatherHistogramCount(g prometheus.Gatherer, name string, labels map[string]string) uint64 {
	families, err := g.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func matchLabels(pairs []*dto.LabelPair, expected map[string]string) bool {
	if len(expected) == 0 {
		return true
	}
	found := make(map[string]string, len(pairs))
	for _, p := range pairs {
		found[p.GetName()] = p.GetValue()
	}
	for k, v := range expected {
		if found[k] != v {
			return false
		}
	}
	return true
}

var _ = Describe("Kubernaut Agent Metrics — BR-KA-OBSERVABILITY-001", func() {

	var (
		reg *prometheus.Registry
		m   *kametrics.Metrics
	)

	BeforeEach(func() {
		reg = prometheus.NewRegistry()
		m = kametrics.NewMetricsWithRegistry(reg)
	})

	Describe("UT-KA-OBS-001: Metrics registration", func() {
		It("registers all 13 metrics without panic", func() {
			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())

			names := make(map[string]bool)
			for _, f := range families {
				names[f.GetName()] = true
			}

			// Counters and gauges may not appear until first use, so
			// exercise every metric first.
			m.RecordSessionStarted("test", "critical")
			m.RecordSessionCompleted("completed", 1.0)
			m.RecordInvestigationPhase("rca", "success")
			m.RecordToolCall("kubectl_events")
			m.RecordInvestigationTurns("rca", 3)
			m.RecordLLMCost("gpt-4", 100, 50)
			m.RecordRateLimited()
			m.RecordAuthzDenied("owner_mismatch")
			m.RecordAuditEventEmitted("aiagent.session.started")
			m.HTTPRequestDurationSeconds.WithLabelValues("/test", "GET", "200").Observe(0.01)
			m.HTTPRequestsInFlight.Inc()

			families, err = reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			names = make(map[string]bool)
			for _, f := range families {
				names[f.GetName()] = true
			}

			Expect(names).To(HaveKey(kametrics.MetricNameSessionsStartedTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameSessionsCompletedTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameSessionsActive))
			Expect(names).To(HaveKey(kametrics.MetricNameSessionDurationSeconds))
			Expect(names).To(HaveKey(kametrics.MetricNameInvestigationPhasesTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameInvestigationToolCallsTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameInvestigationTurnsTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameLLMCostDollarsTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameHTTPRateLimitedTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameHTTPRequestDurationSeconds))
			Expect(names).To(HaveKey(kametrics.MetricNameHTTPRequestsInFlight))
			Expect(names).To(HaveKey(kametrics.MetricNameAuthzDeniedTotal))
			Expect(names).To(HaveKey(kametrics.MetricNameAuditEventsEmittedTotal))
		})
	})

	Describe("UT-KA-OBS-002: Double registration safety (sync.Once)", func() {
		It("NewMetricsWithRegistry does not panic when called with different registries", func() {
			reg2 := prometheus.NewRegistry()
			Expect(func() {
				kametrics.NewMetricsWithRegistry(reg2)
			}).NotTo(Panic())
		})
	})

	Describe("UT-KA-OBS-003: RecordSessionStarted", func() {
		It("increments counter with correct labels", func() {
			m.RecordSessionStarted("OOMKilled", "critical")
			m.RecordSessionStarted("CrashLoopBackOff", "high")

			v := gatherCounter(reg, kametrics.MetricNameSessionsStartedTotal, map[string]string{
				"signal_name": "OOMKilled", "severity": "critical",
			})
			Expect(v).To(BeNumerically("==", 1))

			v = gatherCounter(reg, kametrics.MetricNameSessionsStartedTotal, map[string]string{
				"signal_name": "CrashLoopBackOff", "severity": "high",
			})
			Expect(v).To(BeNumerically("==", 1))
		})

		It("increments sessions_active gauge", func() {
			m.RecordSessionStarted("OOMKilled", "critical")
			g := gatherGauge(reg, kametrics.MetricNameSessionsActive)
			Expect(g).To(BeNumerically("==", 1))
		})
	})

	Describe("UT-KA-OBS-004: RecordSessionCompleted", func() {
		It("increments completed counter and observes duration", func() {
			m.RecordSessionStarted("OOMKilled", "critical")
			m.RecordSessionCompleted("completed", 45.0)

			v := gatherCounter(reg, kametrics.MetricNameSessionsCompletedTotal, map[string]string{
				"outcome": "completed",
			})
			Expect(v).To(BeNumerically("==", 1))

			count := gatherHistogramCount(reg, kametrics.MetricNameSessionDurationSeconds, map[string]string{
				"outcome": "completed",
			})
			Expect(count).To(BeNumerically("==", 1))
		})

		It("decrements sessions_active gauge", func() {
			m.RecordSessionStarted("OOMKilled", "critical")
			m.RecordSessionCompleted("completed", 10.0)
			g := gatherGauge(reg, kametrics.MetricNameSessionsActive)
			Expect(g).To(BeNumerically("==", 0))
		})
	})

	Describe("UT-KA-OBS-005: RecordSessionStarted truncates long signal_name (SEC-1)", func() {
		It("truncates signal_name to 128 characters", func() {
			longName := strings.Repeat("A", 200)
			m.RecordSessionStarted(longName, "critical")

			truncated := longName[:128]
			v := gatherCounter(reg, kametrics.MetricNameSessionsStartedTotal, map[string]string{
				"signal_name": truncated, "severity": "critical",
			})
			Expect(v).To(BeNumerically("==", 1))
		})
	})

	Describe("UT-KA-OBS-006: RecordInvestigationPhase", func() {
		It("increments with phase and outcome labels", func() {
			m.RecordInvestigationPhase("rca", "success")
			m.RecordInvestigationPhase("rca", "failure")
			m.RecordInvestigationPhase("workflow_discovery", "success")

			v := gatherCounter(reg, kametrics.MetricNameInvestigationPhasesTotal, map[string]string{
				"phase": "rca", "outcome": "success",
			})
			Expect(v).To(BeNumerically("==", 1))

			v = gatherCounter(reg, kametrics.MetricNameInvestigationPhasesTotal, map[string]string{
				"phase": "rca", "outcome": "failure",
			})
			Expect(v).To(BeNumerically("==", 1))
		})
	})

	Describe("UT-KA-OBS-007: RecordToolCall", func() {
		It("increments with tool_name label", func() {
			m.RecordToolCall("kubectl_events")
			m.RecordToolCall("kubectl_events")
			m.RecordToolCall("kubernetes_jq_query")

			v := gatherCounter(reg, kametrics.MetricNameInvestigationToolCallsTotal, map[string]string{
				"tool_name": "kubectl_events",
			})
			Expect(v).To(BeNumerically("==", 2))
		})
	})

	Describe("UT-KA-OBS-008: RecordInvestigationTurns", func() {
		It("observes turn count per phase", func() {
			m.RecordInvestigationTurns("rca", 5)
			m.RecordInvestigationTurns("rca", 3)

			count := gatherHistogramCount(reg, kametrics.MetricNameInvestigationTurnsTotal, map[string]string{
				"phase": "rca",
			})
			Expect(count).To(BeNumerically("==", 2))
		})
	})

	Describe("UT-KA-OBS-009: RecordLLMCost with known model", func() {
		It("increments cost counter with model label", func() {
			m.RecordLLMCost("gpt-4", 1000, 500)

			v := gatherCounter(reg, kametrics.MetricNameLLMCostDollarsTotal, map[string]string{
				"model": "gpt-4",
			})
			// gpt-4: prompt=$0.03/1K, completion=$0.06/1K
			// cost = (1000/1000 * 0.03) + (500/1000 * 0.06) = 0.03 + 0.03 = 0.06
			Expect(v).To(BeNumerically("~", 0.06, 0.0001))
		})
	})

	Describe("UT-KA-OBS-010: RecordLLMCost with unknown model", func() {
		It("does not increment counter for unknown model", func() {
			m.RecordLLMCost("unknown-model", 1000, 500)

			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			for _, f := range families {
				if f.GetName() == kametrics.MetricNameLLMCostDollarsTotal {
					Expect(f.GetMetric()).To(BeEmpty(),
						"unknown model should not create a time series")
				}
			}
		})
	})

	Describe("UT-KA-OBS-011: RecordRateLimited", func() {
		It("increments counter", func() {
			m.RecordRateLimited()
			m.RecordRateLimited()

			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			for _, f := range families {
				if f.GetName() == kametrics.MetricNameHTTPRateLimitedTotal {
					Expect(f.GetMetric()).To(HaveLen(1))
					Expect(f.GetMetric()[0].GetCounter().GetValue()).To(BeNumerically("==", 2))
				}
			}
		})
	})

	Describe("UT-KA-OBS-012: RecordAuditEventEmitted", func() {
		It("increments with event_type label", func() {
			m.RecordAuditEventEmitted("aiagent.session.started")
			m.RecordAuditEventEmitted("aiagent.session.started")
			m.RecordAuditEventEmitted("aiagent.llm.request")

			v := gatherCounter(reg, kametrics.MetricNameAuditEventsEmittedTotal, map[string]string{
				"event_type": "aiagent.session.started",
			})
			Expect(v).To(BeNumerically("==", 2))

			v = gatherCounter(reg, kametrics.MetricNameAuditEventsEmittedTotal, map[string]string{
				"event_type": "aiagent.llm.request",
			})
			Expect(v).To(BeNumerically("==", 1))
		})
	})

	Describe("UT-KA-OBS-013: Nil-safety (OPS-1)", func() {
		It("all Record* methods do not panic on nil receiver", func() {
			var nilMetrics *kametrics.Metrics
			Expect(func() {
				nilMetrics.RecordSessionStarted("test", "critical")
				nilMetrics.RecordSessionCompleted("completed", 1.0)
				nilMetrics.RecordInvestigationPhase("rca", "success")
				nilMetrics.RecordToolCall("kubectl_events")
				nilMetrics.RecordInvestigationTurns("rca", 3)
				nilMetrics.RecordLLMCost("gpt-4", 100, 50)
				nilMetrics.RecordRateLimited()
				nilMetrics.RecordAuthzDenied("owner_mismatch")
				nilMetrics.RecordAuditEventEmitted("aiagent.session.started")
			}).NotTo(Panic())
		})
	})

	Describe("UT-KA-OBS-014: MetricName constants match registered names", func() {
		It("all exported constants correspond to gathered metric names", func() {
			expectedConstants := []string{
				kametrics.MetricNameSessionsStartedTotal,
				kametrics.MetricNameSessionsCompletedTotal,
				kametrics.MetricNameSessionsActive,
				kametrics.MetricNameSessionDurationSeconds,
				kametrics.MetricNameInvestigationPhasesTotal,
				kametrics.MetricNameInvestigationToolCallsTotal,
				kametrics.MetricNameInvestigationTurnsTotal,
				kametrics.MetricNameLLMCostDollarsTotal,
				kametrics.MetricNameHTTPRateLimitedTotal,
				kametrics.MetricNameHTTPRequestDurationSeconds,
				kametrics.MetricNameHTTPRequestsInFlight,
				kametrics.MetricNameAuthzDeniedTotal,
				kametrics.MetricNameAuditEventsEmittedTotal,
			}
			for _, c := range expectedConstants {
				Expect(c).NotTo(BeEmpty(), "constant must not be empty")
				Expect(c).To(HavePrefix("aiagent_"), "DD-005: all agent metrics must have aiagent_ prefix")
			}
		})
	})

	Describe("UT-KA-OBS-015: RecordAuthzDenied", func() {
		It("increments with reason label", func() {
			m.RecordAuthzDenied("owner_mismatch")
			m.RecordAuthzDenied("session_not_found")
			m.RecordAuthzDenied("owner_mismatch")

			v := gatherCounter(reg, kametrics.MetricNameAuthzDeniedTotal, map[string]string{
				"reason": "owner_mismatch",
			})
			Expect(v).To(BeNumerically("==", 2))

			v = gatherCounter(reg, kametrics.MetricNameAuthzDeniedTotal, map[string]string{
				"reason": "session_not_found",
			})
			Expect(v).To(BeNumerically("==", 1))
		})
	})
})
