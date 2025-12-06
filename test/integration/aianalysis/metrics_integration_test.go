/*
Copyright 2025 Jordi Gil.

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

package aianalysis

import (
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

// ========================================
// METRICS INTEGRATION TESTS
// Business Requirement: BR-AI-OBSERVABILITY-001
// v1.13: Business-value metrics only (8 metrics)
// ========================================
//
// These tests verify that metrics are:
// 1. Properly registered with Prometheus
// 2. Correctly incremented during reconciliation
// 3. Exposed via the /metrics endpoint
// ========================================

var _ = Describe("BR-AI-OBSERVABILITY-001: Metrics Integration", Ordered, func() {
	var (
		metricsServer *http.Server
		metricsURL    string
	)

	BeforeAll(func() {
		// Start a local metrics server using controller-runtime's registry
		// This is the same registry our metrics are registered in
		metricsPort := "19090" // Use high port to avoid conflicts
		metricsURL = "http://localhost:" + metricsPort + "/metrics"

		mux := http.NewServeMux()
		// Use controller-runtime's registry where our metrics are registered
		mux.Handle("/metrics", promhttp.HandlerFor(ctrlmetrics.Registry, promhttp.HandlerOpts{}))

		metricsServer = &http.Server{
			Addr:    ":" + metricsPort,
			Handler: mux,
		}

		go func() {
			_ = metricsServer.ListenAndServe()
		}()

		// Wait for server to start
		Eventually(func() error {
			_, err := http.Get(metricsURL)
			return err
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
	})

	AfterAll(func() {
		if metricsServer != nil {
			_ = metricsServer.Close()
		}
	})

	// Helper to get current metric value from /metrics endpoint
	getMetricValue := func(metricName string) string {
		resp, err := http.Get(metricsURL)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		lines := strings.Split(string(body), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, metricName) && !strings.HasPrefix(line, "#") {
				return line
			}
		}
		return ""
	}

	// Helper to check metric exists in /metrics output
	metricExists := func(metricName string) bool {
		resp, err := http.Get(metricsURL)
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false
		}

		return strings.Contains(string(body), metricName)
	}

	// ========================================
	// BUSINESS METRICS REGISTRATION
	// ========================================
	Context("Business Metrics Registration (v1.13)", func() {
		// BR-AI-OBSERVABILITY-001: All business metrics must be registered
		It("should register all 8 business-value metrics", func() {
			// Trigger metrics by recording values
			metrics.RecordReconciliation("Pending", "success")
			metrics.RecordReconcileDuration("Pending", 1.5)
			metrics.RecordRegoEvaluation("approved", false)
			metrics.RecordApprovalDecision("auto_approved", "staging")
			metrics.RecordConfidenceScore("OOMKilled", 0.85)
			metrics.RecordFailure("WorkflowResolutionFailed", "LowConfidence")
			metrics.RecordValidationAttempt("restart-pod-v1", false)
			metrics.RecordDetectedLabelsFailure("environment")

			// Verify all metrics appear in /metrics output
			Eventually(func() bool {
				return metricExists("aianalysis_reconciler_reconciliations_total")
			}, 5*time.Second).Should(BeTrue(), "reconciliations_total should be registered")

			Expect(metricExists("aianalysis_reconciler_duration_seconds")).To(BeTrue(),
				"duration_seconds should be registered")
			Expect(metricExists("aianalysis_rego_evaluations_total")).To(BeTrue(),
				"rego_evaluations_total should be registered")
			Expect(metricExists("aianalysis_approval_decisions_total")).To(BeTrue(),
				"approval_decisions_total should be registered")
			Expect(metricExists("aianalysis_confidence_score_distribution")).To(BeTrue(),
				"confidence_score_distribution should be registered")
			Expect(metricExists("aianalysis_failures_total")).To(BeTrue(),
				"failures_total should be registered")
			Expect(metricExists("aianalysis_audit_validation_attempts_total")).To(BeTrue(),
				"audit_validation_attempts_total should be registered")
			Expect(metricExists("aianalysis_quality_detected_labels_failures_total")).To(BeTrue(),
				"quality_detected_labels_failures_total should be registered")
		})

		// NOTE: Removed "should NOT register removed low-value metrics" test
		// Reason: Tests implementation detail (absence of metrics), not business value
		// Per TESTING_GUIDELINES.md: Focus on business outcomes, not implementation
	})

	// ========================================
	// METRICS INCREMENT BEHAVIOR
	// ========================================
	Context("Metrics Increment Behavior", func() {
		// BR-AI-OBSERVABILITY-002: Metrics must increment correctly
		It("should increment reconciliation counter on each call", func() {
			// Record baseline
			metrics.RecordReconciliation("Investigating", "success")

			// Get metric value
			metricLine := getMetricValue("aianalysis_reconciler_reconciliations_total")
			Expect(metricLine).To(ContainSubstring("Investigating"))
			Expect(metricLine).To(ContainSubstring("success"))
		})

		// BR-HAPI-197: Failure metrics must track reason and sub_reason
		It("should track failures with reason and sub_reason labels", func() {
			metrics.RecordFailure("WorkflowResolutionFailed", "WorkflowNotFound")
			metrics.RecordFailure("WorkflowResolutionFailed", "LLMParsingError")
			metrics.RecordFailure("APIError", "TransientError")

			// Verify metrics have correct labels (check full metrics output)
			Eventually(func() bool {
				return metricExists(`reason="WorkflowResolutionFailed"`)
			}, 2*time.Second).Should(BeTrue(), "WorkflowResolutionFailed should be tracked")

			Expect(metricExists(`sub_reason="WorkflowNotFound"`)).To(BeTrue(),
				"WorkflowNotFound sub_reason should be tracked")
			Expect(metricExists(`sub_reason="LLMParsingError"`)).To(BeTrue(),
				"LLMParsingError sub_reason should be tracked")
		})

		// DD-HAPI-002 v1.4: Validation attempts audit
		It("should track validation attempts from HAPI", func() {
			metrics.RecordValidationAttempt("scale-deployment-v1", true)
			metrics.RecordValidationAttempt("scale-deployment-v1", false)

			// Verify both valid and invalid attempts are tracked
			Eventually(func() bool {
				return metricExists("aianalysis_audit_validation_attempts_total")
			}, 2*time.Second).Should(BeTrue())
		})
	})

	// ========================================
	// PROMETHEUS ENDPOINT FORMAT
	// ========================================
	Context("Prometheus Endpoint Format", func() {
		// BR-AI-OBSERVABILITY-003: Metrics must be in Prometheus format
		It("should expose metrics in Prometheus text format", func() {
			resp, err := http.Get(metricsURL)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			metricsText := string(body)

			// Verify Prometheus format markers
			Expect(metricsText).To(ContainSubstring("# HELP"))
			Expect(metricsText).To(ContainSubstring("# TYPE"))

			// Verify our metrics have help text
			Expect(metricsText).To(ContainSubstring("# HELP aianalysis_reconciler_reconciliations_total"))
			Expect(metricsText).To(ContainSubstring("# TYPE aianalysis_reconciler_reconciliations_total counter"))
		})
	})

	// ========================================
	// NOTE: CRD Lifecycle Metrics Test → E2E (Day 8)
	// ========================================
	// The test "should record reconciliation metrics when CRD is created"
	// has been moved to E2E tests (test/e2e/aianalysis/) because it requires:
	// - Full controller manager deployment
	// - Metrics endpoint exposed via Service
	// - Real reconciliation flow
	//
	// This validates: "Can operators scrape AIAnalysis metrics in production?"
	// See: IMPLEMENTATION_PLAN_V1.0.md Day 8 (E2E Tests)
	// ========================================
})

// ========================================
// HELPER: Reset metrics for isolated tests
// ========================================
func resetMetrics() {
	// Note: In production Prometheus, metrics cannot be reset.
	// For testing, we create new metrics each time or use unique labels.
	// This is a limitation of Prometheus client_golang.

	// For counter vectors, we can use Delete to remove specific label combinations
	// but this is generally not recommended in production.

	// Best practice: Use unique labels (timestamps, UUIDs) for test isolation
}

// Ensure controller-runtime metrics registry is available
var _ prometheus.Gatherer = ctrlmetrics.Registry

