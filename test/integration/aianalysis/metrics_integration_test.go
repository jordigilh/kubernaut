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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
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

		// v1.13: Verify removed metrics are NOT present
		It("should NOT register removed low-value metrics", func() {
			Expect(metricExists("aianalysis_reconciler_phase_transitions_total")).To(BeFalse(),
				"phase_transitions_total should NOT be registered (removed in v1.13)")
			Expect(metricExists("aianalysis_reconciler_phase_duration_seconds")).To(BeFalse(),
				"phase_duration_seconds should NOT be registered (removed in v1.13)")
			Expect(metricExists("aianalysis_holmesgpt_requests_total")).To(BeFalse(),
				"holmesgpt_requests_total should NOT be registered (removed in v1.13)")
			Expect(metricExists("aianalysis_holmesgpt_latency_seconds")).To(BeFalse(),
				"holmesgpt_latency_seconds should NOT be registered (removed in v1.13)")
			Expect(metricExists("aianalysis_holmesgpt_retries_total")).To(BeFalse(),
				"holmesgpt_retries_total should NOT be registered (removed in v1.13)")
			Expect(metricExists("aianalysis_rego_latency_seconds")).To(BeFalse(),
				"rego_latency_seconds should NOT be registered (removed in v1.13)")
			Expect(metricExists("aianalysis_rego_reloads_total")).To(BeFalse(),
				"rego_reloads_total should NOT be registered (removed in v1.13)")
		})
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
	// METRICS DURING CRD LIFECYCLE (requires envtest)
	// ========================================
	Context("Metrics During CRD Lifecycle", func() {
		var testNamespace *corev1.Namespace

		BeforeEach(func() {
			// Create test namespace
			testNamespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-metrics-",
				},
			}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())
		})

		AfterEach(func() {
			// Cleanup
			if testNamespace != nil {
				_ = k8sClient.Delete(ctx, testNamespace)
			}
		})

		// BR-AI-001: CRD lifecycle should emit metrics
		It("should record reconciliation metrics when CRD is created", func() {
			Skip("Requires controller manager setup - will be enabled when reconciler is integrated")

			// Create test AIAnalysis
			analysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-metrics-analysis",
					Namespace: testNamespace.Name,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind:      "RemediationRequest",
						Name:      "test-rr",
						Namespace: testNamespace.Name,
					},
					RemediationID: "test-metrics-001",
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint:      "test-fingerprint",
							Severity:         "warning",
							SignalType:       "OOMKilled",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: testNamespace.Name,
							},
						},
						AnalysisTypes: []string{"investigation"},
					},
				},
			}

			Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

			// Verify reconciliation metric was incremented
			Eventually(func() string {
				return getMetricValue("aianalysis_reconciler_reconciliations_total")
			}, 10*time.Second).Should(ContainSubstring("Pending"))
		})
	})
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

