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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

// ========================================
// METRICS INTEGRATION TESTS
// Business Requirement: BR-AI-OBSERVABILITY-001
// v1.13: Business-value metrics only (8 metrics)
// ========================================
//
// Integration Test Strategy (per DD-TEST-001):
// - CRD controllers use envtest (no HTTP server)
// - Metrics verified via REGISTRY INSPECTION (not HTTP endpoint)
// - HTTP endpoint testing deferred to E2E tier (Kind cluster)
//
// These tests verify that metrics are:
// 1. Properly registered with controller-runtime registry
// 2. Correctly incremented when Record*() functions are called
// 3. Follow DD-005 naming convention
//
// Note: HTTP /metrics endpoint accessibility is tested in E2E tier
// See: test/e2e/aianalysis/02_metrics_test.go
// ========================================

var _ = Describe("BR-AI-OBSERVABILITY-001: Metrics Integration", Label("integration", "metrics"), func() {

	// Helper to gather all metrics from controller-runtime registry
	gatherMetrics := func() (map[string]*dto.MetricFamily, error) {
		families, err := ctrlmetrics.Registry.Gather()
		if err != nil {
			return nil, err
		}
		result := make(map[string]*dto.MetricFamily)
		for _, family := range families {
			result[family.GetName()] = family
		}
		return result, nil
	}

	// Helper to check if a metric exists in the registry
	metricExists := func(name string) bool {
		families, err := gatherMetrics()
		if err != nil {
			return false
		}
		_, exists := families[name]
		return exists
	}

	// Helper to get metric with specific labels
	getMetricWithLabels := func(name string, labels map[string]string) *dto.Metric {
		families, err := gatherMetrics()
		if err != nil {
			return nil
		}
		family, exists := families[name]
		if !exists {
			return nil
		}
		for _, m := range family.GetMetric() {
			labelMatch := true
			for wantKey, wantValue := range labels {
				found := false
				for _, l := range m.GetLabel() {
					if l.GetName() == wantKey && l.GetValue() == wantValue {
						found = true
						break
					}
				}
				if !found {
					labelMatch = false
					break
				}
			}
			if labelMatch {
				return m
			}
		}
		return nil
	}

	// ========================================
	// BUSINESS METRICS REGISTRATION
	// ========================================
	Context("Business Metrics Registration (v1.13)", func() {
		// BR-AI-OBSERVABILITY-001: All business metrics must be registered
		It("should register all 8 business-value metrics - BR-AI-OBSERVABILITY-001", func() {
			// Trigger metrics by recording values (ensures counters are initialized)
			metrics.RecordReconciliation("Pending", "success")
			metrics.RecordReconcileDuration("Pending", 1.5)
			metrics.RecordRegoEvaluation("approved", false)
			metrics.RecordApprovalDecision("auto_approved", "staging")
			metrics.RecordConfidenceScore("OOMKilled", 0.85)
			metrics.RecordFailure("WorkflowResolutionFailed", "LowConfidence")
			metrics.RecordValidationAttempt("restart-pod-v1", false)
			metrics.RecordDetectedLabelsFailure("environment")

			// Verify all 8 metrics are registered via registry inspection
			Expect(metricExists("aianalysis_reconciler_reconciliations_total")).To(BeTrue(),
				"reconciliations_total should be registered")
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

		// DD-005: Metrics naming convention
		It("should follow DD-005 naming convention - DD-005", func() {
			families, err := gatherMetrics()
			Expect(err).ToNot(HaveOccurred())

			// Check our metrics follow {service}_{component}_{metric}_{unit} pattern
			expectedPrefixes := []string{
				"aianalysis_reconciler_",
				"aianalysis_rego_",
				"aianalysis_approval_",
				"aianalysis_failures_",
				"aianalysis_confidence_",
				"aianalysis_audit_",
				"aianalysis_quality_",
			}

			for _, prefix := range expectedPrefixes {
				found := false
				for name := range families {
					if strings.HasPrefix(name, prefix) {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "Expected metric with prefix %s", prefix)
			}
		})
	})

	// ========================================
	// METRICS INCREMENT BEHAVIOR
	// ========================================
	Context("Metrics Increment Behavior", func() {
		// BR-AI-OBSERVABILITY-002: Metrics must increment correctly
		It("should increment reconciliation counter on each call - BR-AI-OBSERVABILITY-002", func() {
			// Record a reconciliation
			metrics.RecordReconciliation("Investigating", "success")

			// Verify the metric exists with correct labels
			m := getMetricWithLabels("aianalysis_reconciler_reconciliations_total", map[string]string{
				"phase":  "Investigating",
				"result": "success",
			})
			Expect(m).ToNot(BeNil(), "Metric with labels phase=Investigating, result=success should exist")
			Expect(m.GetCounter().GetValue()).To(BeNumerically(">", 0),
				"Counter should have been incremented")
		})

		// BR-HAPI-197: Failure metrics must track reason and sub_reason
		It("should track failures with reason and sub_reason labels - BR-HAPI-197", func() {
			metrics.RecordFailure("WorkflowResolutionFailed", "WorkflowNotFound")
			metrics.RecordFailure("WorkflowResolutionFailed", "LLMParsingError")
			metrics.RecordFailure("APIError", "TransientError")

			// Verify metrics with specific labels exist
			m1 := getMetricWithLabels("aianalysis_failures_total", map[string]string{
				"reason":     "WorkflowResolutionFailed",
				"sub_reason": "WorkflowNotFound",
			})
			Expect(m1).ToNot(BeNil(), "WorkflowNotFound failure should be tracked")

			m2 := getMetricWithLabels("aianalysis_failures_total", map[string]string{
				"reason":     "WorkflowResolutionFailed",
				"sub_reason": "LLMParsingError",
			})
			Expect(m2).ToNot(BeNil(), "LLMParsingError failure should be tracked")

			m3 := getMetricWithLabels("aianalysis_failures_total", map[string]string{
				"reason":     "APIError",
				"sub_reason": "TransientError",
			})
			Expect(m3).ToNot(BeNil(), "TransientError failure should be tracked")
		})

		// DD-HAPI-002 v1.4: Validation attempts audit
		It("should track validation attempts from HAPI - DD-HAPI-002", func() {
			metrics.RecordValidationAttempt("scale-deployment-v1", true)
			metrics.RecordValidationAttempt("scale-deployment-v1", false)

			// Verify both valid and invalid attempts are tracked
			valid := getMetricWithLabels("aianalysis_audit_validation_attempts_total", map[string]string{
				"workflow_id": "scale-deployment-v1",
				"is_valid":    "true",
			})
			Expect(valid).ToNot(BeNil(), "Valid attempt should be tracked")

			invalid := getMetricWithLabels("aianalysis_audit_validation_attempts_total", map[string]string{
				"workflow_id": "scale-deployment-v1",
				"is_valid":    "false",
			})
			Expect(invalid).ToNot(BeNil(), "Invalid attempt should be tracked")
		})

		// BR-AI-022: Confidence score tracking
		It("should record confidence scores as histogram observations - BR-AI-022", func() {
			metrics.RecordConfidenceScore("CrashLoopBackOff", 0.75)
			metrics.RecordConfidenceScore("CrashLoopBackOff", 0.85)
			metrics.RecordConfidenceScore("OOMKilled", 0.95)

			// Verify histogram has observations
			families, err := gatherMetrics()
			Expect(err).ToNot(HaveOccurred())

			family, exists := families["aianalysis_confidence_score_distribution"]
			Expect(exists).To(BeTrue(), "Histogram should exist")

			// Check that observations were recorded (sum > 0)
			totalSum := 0.0
			for _, m := range family.GetMetric() {
				totalSum += m.GetHistogram().GetSampleSum()
			}
			Expect(totalSum).To(BeNumerically(">", 0), "Histogram should have observations")
		})
	})

	// ========================================
	// METRICS TYPE VERIFICATION
	// ========================================
	Context("Metrics Type Verification", func() {
		It("should have correct metric types - DD-005", func() {
			families, err := gatherMetrics()
			Expect(err).ToNot(HaveOccurred())

			// Counters
			counters := []string{
				"aianalysis_reconciler_reconciliations_total",
				"aianalysis_rego_evaluations_total",
				"aianalysis_approval_decisions_total",
				"aianalysis_failures_total",
				"aianalysis_audit_validation_attempts_total",
				"aianalysis_quality_detected_labels_failures_total",
			}
			for _, name := range counters {
				family, exists := families[name]
				if exists {
					Expect(family.GetType().String()).To(Equal("COUNTER"),
						"%s should be a COUNTER", name)
				}
			}

			// Histograms
			histograms := []string{
				"aianalysis_reconciler_duration_seconds",
				"aianalysis_confidence_score_distribution",
			}
			for _, name := range histograms {
				family, exists := families[name]
				if exists {
					Expect(family.GetType().String()).To(Equal("HISTOGRAM"),
						"%s should be a HISTOGRAM", name)
				}
			}
		})
	})

	// ========================================
	// NOTE: HTTP Endpoint Tests → E2E (Day 8)
	// ========================================
	// HTTP /metrics endpoint accessibility tests are in E2E tier:
	// - test/e2e/aianalysis/02_metrics_test.go
	//
	// Rationale (per DD-TEST-001):
	// - Integration tests use envtest (no HTTP server for CRD controllers)
	// - E2E tests deploy full controller with Service (HTTP endpoint available)
	//
	// E2E validates: "Can operators scrape AIAnalysis metrics in production?"
	// ========================================
})
