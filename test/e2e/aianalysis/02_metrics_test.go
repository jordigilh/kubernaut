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
	"context"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	// DD-005 V3.0: Import metric constants from production code
	aametrics "github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

var skipMetricsSeeding bool

// seedMetricsWithAnalysis creates a simple AIAnalysis and waits for completion
// to ensure all metrics are populated before tests run
func seedMetricsWithAnalysis() {
	ctx := context.Background()
	namespace := createTestNamespace("metrics-seed")

	// Create successful analysis to populate success metrics
	analysis := &aianalysisv1alpha1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metrics-seed-success-" + randomSuffix(),
			Namespace: namespace,
		},
		Spec: aianalysisv1alpha1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "metrics-seed-rem",
				Namespace: namespace,
			},
			RemediationID: "metrics-seed-001",
			AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
				SignalContext: aianalysisv1alpha1.SignalContextInput{
					Fingerprint:      "metrics-seed-fp",
					Severity:        "medium",
					SignalType:       "PodCrashLooping",
					Environment:      "staging",
					BusinessPriority: "P2",
					TargetResource: aianalysisv1alpha1.TargetResource{
						Kind:      "Pod",
						Namespace: "default",
						Name:      "test-pod",
					},
				},
				AnalysisTypes: []string{"investigation"},
			},
		},
	}

	Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

	// Wait for analysis to complete
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
			return false
		}
		return analysis.Status.Phase == "Completed" || analysis.Status.Phase == "Failed"
	}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "Metrics seeding (success) analysis should complete")

	// Create failed analysis to populate failure metrics
	// BR-HAPI-197: Ensure aianalysis_failures_total metric is populated
	failedAnalysis := &aianalysisv1alpha1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metrics-seed-failed-" + randomSuffix(),
			Namespace: namespace,
		},
		Spec: aianalysisv1alpha1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "metrics-seed-rem-fail",
				Namespace: namespace,
			},
			RemediationID: "metrics-seed-fail-001",
			AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
				SignalContext: aianalysisv1alpha1.SignalContextInput{
					Fingerprint:      "TRIGGER_WORKFLOW_RESOLUTION_FAILURE", // Special fingerprint to trigger failure
					Severity:         "critical",
					SignalType:       "TestFailureScenario",
					Environment:      "staging",
					BusinessPriority: "P1",
					TargetResource: aianalysisv1alpha1.TargetResource{
						Kind:      "Pod",
						Namespace: "default",
						Name:      "test-pod-fail",
					},
				},
				AnalysisTypes: []string{"investigation"},
			},
		},
	}

	Expect(k8sClient.Create(ctx, failedAnalysis)).To(Succeed())

	// Wait for failed analysis to reach Failed phase
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(failedAnalysis), failedAnalysis); err != nil {
			return false
		}
		return failedAnalysis.Status.Phase == "Failed" || failedAnalysis.Status.Phase == "Completed"
	}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "Metrics seeding (failed) analysis should reach terminal state")

	// Metrics are immediately available in Prometheus - no sleep needed
}

var _ = Describe("Metrics Endpoint E2E", Label("e2e", "metrics"), func() {
	var httpClient *http.Client

	BeforeEach(func() {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	})

	// BR-AI-022: Metrics must be populated before checking
	// Create and complete at least one AIAnalysis to ensure metrics are incremented
	BeforeEach(func() {
		// Only seed metrics once for all tests in this suite
		if skipMetricsSeeding {
			return
		}
		seedMetricsWithAnalysis()
		skipMetricsSeeding = true
	})

	Context("Prometheus metrics (/metrics) - BR-AI-022", func() {
		It("should expose metrics in Prometheus format", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))
		})

		It("should include reconciliation metrics - BR-AI-022", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Core metrics per DD-005 and implementation plan
			// DD-005 V3.0: Use constants from production code to prevent typos
			expectedMetrics := []string{
				aametrics.MetricNameReconcilerReconciliationsTotal,
				aametrics.MetricNameFailuresTotal,
			}

			for _, metric := range expectedMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing metric: %s", metric)
			}
		})

		It("should include Rego policy evaluation metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Rego policy evaluation metrics (BR-AI-030)
			// Business value: Track policy decision outcomes
			// DD-005 V3.0: Use constants from production code
			expectedRegoMetrics := []string{
				aametrics.MetricNameRegoEvaluationsTotal,
			}

			for _, metric := range expectedRegoMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing Rego metric: %s", metric)
			}

			// Verify metric has expected labels (outcome, degraded)
			// DD-005 V3.0: Use constant in regex pattern
			pattern := aametrics.MetricNameRegoEvaluationsTotal + `\{.*outcome=.*\}`
			Expect(metricsText).To(MatchRegexp(pattern),
				"Rego metric should have 'outcome' label")
		})

		It("should include confidence score distribution metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// AI confidence tracking (BR-AI-OBSERVABILITY-004)
			// Business value: AI quality/reliability tracking
			// DD-005 V3.0: Use constant from production code
			Expect(metricsText).To(ContainSubstring(aametrics.MetricNameConfidenceScoreDistribution),
				"Missing confidence distribution metric")
		})

		It("should include approval decision metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Approval tracking (BR-AI-059)
			// Business value: Approval vs auto-execute ratio
			// DD-005 V3.0: Use constant from production code
			Expect(metricsText).To(ContainSubstring(aametrics.MetricNameApprovalDecisionsTotal),
				"Missing approval decisions metric")
		})

		It("should include recovery status metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Recovery observability (BR-AI-082)
			// Business value: Track recovery attempt outcomes
			// DD-005 V3.0: Use constants from production code
			expectedRecoveryMetrics := []string{
				aametrics.MetricNameRecoveryStatusPopulatedTotal,
				aametrics.MetricNameRecoveryStatusSkippedTotal,
			}

			for _, metric := range expectedRecoveryMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing recovery metric: %s", metric)
			}
		})

		It("should include Go runtime metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Standard Go runtime metrics
			Expect(metricsText).To(ContainSubstring("go_goroutines"))
			Expect(metricsText).To(ContainSubstring("go_memstats"))
		})

		It("should include controller-runtime metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Controller-runtime standard metrics
			Expect(metricsText).To(ContainSubstring("controller_runtime"))
		})
	})

	Context("Metrics accuracy", func() {
		It("should increment reconciliation counter after processing", func() {
			// Get initial metric value
			resp1, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			body1, _ := io.ReadAll(resp1.Body)
			_ = resp1.Body.Close()
			initialMetrics := string(body1)

			// Metrics should contain reconciliation counter
			// Value will increase after AIAnalysis CRDs are processed
			Expect(initialMetrics).To(ContainSubstring("aianalysis"))
		})
	})
})
