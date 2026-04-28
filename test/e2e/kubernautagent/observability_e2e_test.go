/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernautagent

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

// E2E Observability Tests — BR-KA-OBSERVABILITY-001
//
// Validates that all PR10 Prometheus metrics are exposed through the
// /metrics endpoint on the dedicated metrics port (kaMetricsURL) after
// a real investigation flow in the Kind cluster.
//
// Pattern: Same as gateway (30_observability_test.go) and notification
// (05_metrics_validation_test.go) — direct HTTP GET to /metrics, no
// Prometheus operator required.

var _ = Describe("E2E-KA-OBS: Observability / Prometheus Metrics (BR-KA-OBSERVABILITY-001)", Label("e2e", "ka", "observability"), func() {

	// fetchMetrics GETs the /metrics endpoint and returns the raw text body.
	fetchMetrics := func() string {
		resp, err := http.Get(kaMetricsURL + "/metrics")
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "metrics endpoint should be reachable")
		defer func() { _ = resp.Body.Close() }()
		ExpectWithOffset(1, resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return string(body)
	}

	// triggerInvestigation runs a full investigation flow using the Mock LLM.
	// Returns the response so callers can verify it completed.
	triggerInvestigation := func(id string, signalName string) *agentclient.IncidentResponse {
		req := &agentclient.IncidentRequest{
			IncidentID:        id,
			RemediationID:     "rem-obs-" + id,
			SignalName:        signalName,
			Severity:          agentclient.SeverityHigh,
			SignalSource:      "kubernetes",
			ResourceNamespace: "production",
			ResourceKind:      "Pod",
			ResourceName:      "obs-test-pod",
		}
		result, err := sessionClient.Investigate(ctx, req)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "investigation should complete without error")
		return result
	}

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// E2E-KA-OBS-001: All PR10 metric families are exposed
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-001: Metric families are exposed after investigation", func() {
		It("all 13 PR10 metrics appear in /metrics after a completed investigation", func() {
			By("Triggering an investigation to warm up all metric paths")
			result := triggerInvestigation("obs-001", "OOMKilled")
			Expect(result).NotTo(BeNil(), "investigation should return a result")

			By("Fetching /metrics and checking all metric families")
			Eventually(func() []string {
				body := fetchMetrics()
				var missing []string
				for _, name := range []string{
					metrics.MetricNameSessionsStartedTotal,
					metrics.MetricNameSessionsCompletedTotal,
					metrics.MetricNameSessionsActive,
					metrics.MetricNameSessionDurationSeconds,
					metrics.MetricNameInvestigationPhasesTotal,
					metrics.MetricNameInvestigationToolCallsTotal,
					metrics.MetricNameInvestigationTurnsTotal,
					metrics.MetricNameLLMCostDollarsTotal,
					metrics.MetricNameHTTPRequestDurationSeconds,
					metrics.MetricNameHTTPRequestsInFlight,
					metrics.MetricNameAuthzDeniedTotal,
					metrics.MetricNameAuditEventsEmittedTotal,
					// MetricNameHTTPRateLimitedTotal may not appear until
					// a rate-limited request occurs; check registration via HELP line.
				} {
					if !strings.Contains(body, name) {
						missing = append(missing, name)
					}
				}
				return missing
			}, "30s", "2s").Should(BeEmpty(),
				"all PR10 metric families should be present in /metrics output")

			By("Verifying rate-limiter metric is at least registered (HELP line)")
			body := fetchMetrics()
			Expect(body).To(ContainSubstring(metrics.MetricNameHTTPRateLimitedTotal),
				"rate limiter metric should be registered (HELP line or zero-value)")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// E2E-KA-OBS-002: Session lifecycle metrics have correct labels
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-002: Session lifecycle labels", func() {
		It("sessions_started_total has signal_name label from the incident", func() {
			By("Triggering investigation with known signal_name")
			triggerInvestigation("obs-002", "OOMKilled")

			By("Verifying signal_name label in /metrics output")
			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, fmt.Sprintf(`%s{signal_name="OOMKilled"}`, metrics.MetricNameSessionsStartedTotal))
			}, "15s", "2s").Should(BeTrue(),
				"sessions_started_total should have signal_name=\"OOMKilled\" label")
		})

		It("sessions_completed_total has status label", func() {
			By("Triggering investigation to completion")
			triggerInvestigation("obs-002b", "OOMKilled")

			By("Verifying status label in /metrics output")
			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, fmt.Sprintf(`%s{status=`, metrics.MetricNameSessionsCompletedTotal))
			}, "15s", "2s").Should(BeTrue(),
				"sessions_completed_total should have status label")
		})

		It("session_duration_seconds has histogram buckets", func() {
			By("Triggering investigation to generate duration observation")
			triggerInvestigation("obs-002c", "OOMKilled")

			By("Verifying histogram _bucket suffix in /metrics output")
			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, metrics.MetricNameSessionDurationSeconds+"_bucket")
			}, "15s", "2s").Should(BeTrue(),
				"session_duration_seconds should expose histogram buckets")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// E2E-KA-OBS-003: HTTP request duration has method/endpoint/status labels
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-003: HTTP request metrics labels", func() {
		It("http_request_duration_seconds has endpoint, method, and status_code labels", func() {
			By("The investigation itself generates HTTP traffic; check labels")
			triggerInvestigation("obs-003", "OOMKilled")

			Eventually(func() bool {
				body := fetchMetrics()
				// Look for the histogram _count line which exposes label keys
				return strings.Contains(body, metrics.MetricNameHTTPRequestDurationSeconds+`_count{endpoint=`) ||
					strings.Contains(body, metrics.MetricNameHTTPRequestDurationSeconds+`_bucket{endpoint=`)
			}, "15s", "2s").Should(BeTrue(),
				"HTTP request duration should have endpoint label")

			body := fetchMetrics()
			Expect(body).To(MatchRegexp(
				`aiagent_http_request_duration_seconds_\w+\{.*method="POST".*\}`),
				"HTTP metrics should include method=\"POST\" from /incident/analyze")
			Expect(body).To(MatchRegexp(
				`aiagent_http_request_duration_seconds_\w+\{.*status_code="\d+".*\}`),
				"HTTP metrics should include numeric status_code label")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// E2E-KA-OBS-004: Audit events emitted metric increments
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-004: Audit pipeline metrics", func() {
		It("audit_events_emitted_total increments after investigation", func() {
			By("Capturing baseline audit metric")
			baselineBody := fetchMetrics()
			baselineCount := countMetricOccurrences(baselineBody, metrics.MetricNameAuditEventsEmittedTotal)

			By("Triggering investigation to generate audit events")
			triggerInvestigation("obs-004", "OOMKilled")

			By("Verifying audit events emitted total increased")
			Eventually(func() bool {
				body := fetchMetrics()
				return countMetricOccurrences(body, metrics.MetricNameAuditEventsEmittedTotal) > baselineCount
			}, "30s", "2s").Should(BeTrue(),
				"audit_events_emitted_total should increase after investigation")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// E2E-KA-OBS-005: Investigation quality metrics
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-005: Investigation quality metrics", func() {
		It("investigation_phases_total has phase and outcome labels", func() {
			By("Triggering investigation")
			triggerInvestigation("obs-005a", "OOMKilled")

			By("Verifying phase labels")
			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, fmt.Sprintf(`%s{`, metrics.MetricNameInvestigationPhasesTotal))
			}, "15s", "2s").Should(BeTrue(),
				"investigation_phases_total should have entries after investigation")

			body := fetchMetrics()
			Expect(body).To(MatchRegexp(
				`aiagent_investigation_phases_total\{.*phase="(rca|workflow_selection)".*\}`),
				"investigation_phases_total should include phase label")
			Expect(body).To(MatchRegexp(
				`aiagent_investigation_phases_total\{.*outcome="(success|failure)".*\}`),
				"investigation_phases_total should include outcome label")
		})

		It("investigation_turns_total has phase label and histogram buckets", func() {
			By("Triggering investigation")
			triggerInvestigation("obs-005b", "OOMKilled")

			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, metrics.MetricNameInvestigationTurnsTotal+"_bucket")
			}, "15s", "2s").Should(BeTrue(),
				"investigation_turns_total should expose histogram buckets")
		})

		It("investigation_tool_calls_total has tool_name label", func() {
			By("Triggering investigation to exercise tool calls")
			triggerInvestigation("obs-005c", "OOMKilled")

			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, fmt.Sprintf(`%s{tool_name=`, metrics.MetricNameInvestigationToolCallsTotal))
			}, "15s", "2s").Should(BeTrue(),
				"investigation_tool_calls_total should have tool_name label")
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// E2E-KA-OBS-006: LLM cost metric
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-006: LLM cost metric", func() {
		It("llm_cost_dollars_total is present with model label after investigation", func() {
			By("Triggering investigation to generate LLM cost")
			triggerInvestigation("obs-006", "OOMKilled")

			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, fmt.Sprintf(`%s{model=`, metrics.MetricNameLLMCostDollarsTotal))
			}, "15s", "2s").Should(BeTrue(),
				"llm_cost_dollars_total should have model label after investigation")
		})
	})
})

// countMetricOccurrences counts how many sample lines (non-comment, non-empty)
// contain the given metric name prefix. This captures the sum of all label
// combinations, giving a rough "has it grown" signal for Eventually assertions.
func countMetricOccurrences(body, metricName string) int {
	count := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, metricName) {
			count++
		}
	}
	return count
}
