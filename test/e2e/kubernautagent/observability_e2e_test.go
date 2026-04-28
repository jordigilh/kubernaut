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
// Validates that all PR10 Prometheus metrics (9 after pruning) are exposed
// through the /metrics endpoint on the dedicated metrics port (kaMetricsURL)
// after a real investigation flow in the Kind cluster.

var _ = Describe("E2E-KA-OBS: Observability / Prometheus Metrics (BR-KA-OBSERVABILITY-001)", Label("e2e", "ka", "observability"), func() {

	fetchMetrics := func() string {
		resp, err := http.Get(kaMetricsURL + "/metrics")
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "metrics endpoint should be reachable")
		defer func() { _ = resp.Body.Close() }()
		ExpectWithOffset(1, resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return string(body)
	}

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
	// E2E-KA-OBS-001: All 9 metric families are exposed
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-001: Metric families are exposed after investigation", func() {
		It("all 9 metrics appear in /metrics after a completed investigation", func() {
			By("Triggering an investigation to warm up metric paths")
			result := triggerInvestigation("obs-001", "OOMKilled")
			Expect(result).NotTo(BeNil())

			By("Hitting a non-existent session to trigger authz_denied metric")
			req, err := http.NewRequestWithContext(ctx, "GET",
				kaURL+"/api/v1/incident/session/non-existent-obs-001/status", nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := authHTTPClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()

			By("Checking all 9 metric families appear")
			allMetrics := []string{
				metrics.MetricNameSessionsStartedTotal,
				metrics.MetricNameSessionsCompletedTotal,
				metrics.MetricNameSessionsActive,
				metrics.MetricNameSessionDurationSeconds,
				metrics.MetricNameHTTPRequestDurationSeconds,
				metrics.MetricNameHTTPRequestsInFlight,
				metrics.MetricNameAuthzDeniedTotal,
				metrics.MetricNameAuditEventsEmittedTotal,
			}
			// http_rate_limited_total only appears as HELP when no 429 has occurred.
			helpOnlyMetrics := []string{
				metrics.MetricNameHTTPRateLimitedTotal,
			}

			Eventually(func() []string {
				body := fetchMetrics()
				var missing []string
				for _, name := range allMetrics {
					if !strings.Contains(body, name) {
						missing = append(missing, name)
					}
				}
				return missing
			}, "30s", "2s").Should(BeEmpty(),
				"all sample-producing metric families should be present")

			body := fetchMetrics()
			for _, name := range helpOnlyMetrics {
				Expect(body).To(ContainSubstring("# HELP "+name),
					fmt.Sprintf("%s should be registered (HELP line)", name))
			}
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// E2E-KA-OBS-002: Session lifecycle metrics have correct labels
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("E2E-KA-OBS-002: Session lifecycle labels", func() {
		It("sessions_started_total has signal_name label from the incident", func() {
			By("Triggering investigation with known signal_name")
			triggerInvestigation("obs-002a", "OOMKilled")

			By("Verifying signal_name label in /metrics output")
			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, `signal_name="OOMKilled"`) &&
					strings.Contains(body, metrics.MetricNameSessionsStartedTotal)
			}, "15s", "2s").Should(BeTrue(),
				"sessions_started_total should contain signal_name=\"OOMKilled\"")
		})

		It("sessions_completed_total has outcome label", func() {
			By("Triggering investigation to completion")
			triggerInvestigation("obs-002b", "OOMKilled")

			By("Verifying outcome label in /metrics output")
			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, metrics.MetricNameSessionsCompletedTotal+`{outcome=`)
			}, "15s", "2s").Should(BeTrue(),
				"sessions_completed_total should have outcome label")
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
		It("http_request_duration_seconds has endpoint, method, and status labels", func() {
			By("The investigation generates HTTP traffic; check labels")
			triggerInvestigation("obs-003", "OOMKilled")

			Eventually(func() bool {
				body := fetchMetrics()
				return strings.Contains(body, metrics.MetricNameHTTPRequestDurationSeconds+`_count{`) ||
					strings.Contains(body, metrics.MetricNameHTTPRequestDurationSeconds+`_bucket{`)
			}, "15s", "2s").Should(BeTrue(),
				"HTTP request duration should expose count/bucket lines")

			body := fetchMetrics()
			Expect(body).To(MatchRegexp(
				`aiagent_http_request_duration_seconds_\w+\{.*method="POST".*\}`),
				"HTTP metrics should include method=\"POST\"")
			Expect(body).To(MatchRegexp(
				`aiagent_http_request_duration_seconds_\w+\{.*status="\d+".*\}`),
				"HTTP metrics should include numeric status label")
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
})

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
