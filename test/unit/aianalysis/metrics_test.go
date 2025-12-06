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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

// Day 5: Metrics Unit Tests
// DD-005 Compliant: Tests verify correct metric naming and behavior
var _ = Describe("AIAnalysis Metrics", func() {
	// ========================================
	// RECONCILER METRICS
	// ========================================
	Describe("Reconciler Metrics (DD-005 Compliant)", func() {
		// BR-AI-OBSERVABILITY-001: Track reconciliation outcomes
		It("should register ReconcilerReconciliationsTotal counter", func() {
			Expect(metrics.ReconcilerReconciliationsTotal).NotTo(BeNil())
			// Verify metric name follows DD-005 convention: {service}_{component}_{metric}_{unit}
			desc := metrics.ReconcilerReconciliationsTotal.WithLabelValues("Pending", "success").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_reconciler_reconciliations_total"))
		})

		It("should register ReconcilerDurationSeconds histogram", func() {
			Expect(metrics.ReconcilerDurationSeconds).NotTo(BeNil())
			desc := metrics.ReconcilerDurationSeconds.WithLabelValues("Investigating").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_reconciler_duration_seconds"))
		})

		It("should register ReconcilerPhaseTransitionsTotal counter", func() {
			Expect(metrics.ReconcilerPhaseTransitionsTotal).NotTo(BeNil())
			desc := metrics.ReconcilerPhaseTransitionsTotal.WithLabelValues("Pending", "Investigating").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_reconciler_phase_transitions_total"))
		})

		It("should register ReconcilerPhaseDurationSeconds histogram", func() {
			Expect(metrics.ReconcilerPhaseDurationSeconds).NotTo(BeNil())
			desc := metrics.ReconcilerPhaseDurationSeconds.WithLabelValues("Analyzing").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_reconciler_phase_duration_seconds"))
		})

		// BR-AI-OBSERVABILITY-002: Business outcome - track phase transition success rate
		It("should support recording reconciliation outcomes", func() {
			// Record successful reconciliation
			metrics.ReconcilerReconciliationsTotal.WithLabelValues("Pending", "success").Inc()
			// Record failed reconciliation
			metrics.ReconcilerReconciliationsTotal.WithLabelValues("Investigating", "failure").Inc()

			// Verify no panic (metrics are functional)
			Expect(func() {
				metrics.ReconcilerReconciliationsTotal.WithLabelValues("Analyzing", "success").Inc()
			}).NotTo(Panic())
		})
	})

	// ========================================
	// HOLMESGPT-API METRICS
	// ========================================
	Describe("HolmesGPT-API Metrics (DD-005 Compliant)", func() {
		// BR-AI-006: Track API call metrics
		It("should register HolmesGPTRequestsTotal counter", func() {
			Expect(metrics.HolmesGPTRequestsTotal).NotTo(BeNil())
			desc := metrics.HolmesGPTRequestsTotal.WithLabelValues("/api/v1/incident/analyze", "200").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_holmesgpt_requests_total"))
		})

		It("should register HolmesGPTLatencySeconds histogram", func() {
			Expect(metrics.HolmesGPTLatencySeconds).NotTo(BeNil())
			desc := metrics.HolmesGPTLatencySeconds.WithLabelValues("/api/v1/incident/analyze").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_holmesgpt_latency_seconds"))
		})

		It("should register HolmesGPTRetriesTotal counter", func() {
			Expect(metrics.HolmesGPTRetriesTotal).NotTo(BeNil())
			desc := metrics.HolmesGPTRetriesTotal.WithLabelValues("/api/v1/incident/analyze").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_holmesgpt_retries_total"))
		})

		// DD-HAPI-002 v1.4: Track validation attempts from HAPI
		It("should register ValidationAttemptsTotal counter", func() {
			Expect(metrics.ValidationAttemptsTotal).NotTo(BeNil())
			desc := metrics.ValidationAttemptsTotal.WithLabelValues("restart-pod-v1", "false").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_holmesgpt_validation_attempts_total"))
		})
	})

	// ========================================
	// REGO POLICY METRICS
	// ========================================
	Describe("Rego Policy Metrics (DD-005 Compliant)", func() {
		// BR-AI-030: Track policy evaluation outcomes
		It("should register RegoEvaluationsTotal counter", func() {
			Expect(metrics.RegoEvaluationsTotal).NotTo(BeNil())
			desc := metrics.RegoEvaluationsTotal.WithLabelValues("approved", "false").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_rego_evaluations_total"))
		})

		It("should register RegoLatencySeconds histogram", func() {
			Expect(metrics.RegoLatencySeconds).NotTo(BeNil())
			// No labels for this metric
			var regoHist *prometheus.HistogramVec
			regoHist = metrics.RegoLatencySeconds
			Expect(regoHist).NotTo(BeNil())
		})

		It("should register RegoReloadsTotal counter", func() {
			Expect(metrics.RegoReloadsTotal).NotTo(BeNil())
		})
	})

	// ========================================
	// APPROVAL METRICS
	// ========================================
	Describe("Approval Metrics (DD-005 Compliant)", func() {
		// BR-AI-059: Track approval decisions
		It("should register ApprovalDecisionsTotal counter", func() {
			Expect(metrics.ApprovalDecisionsTotal).NotTo(BeNil())
			desc := metrics.ApprovalDecisionsTotal.WithLabelValues("approved", "production").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_approval_decisions_total"))
		})

		// BR-AI-OBSERVABILITY-003: Track approval reasons for audit
		It("should support recording approval decisions by environment", func() {
			Expect(func() {
				metrics.ApprovalDecisionsTotal.WithLabelValues("manual_review_required", "production").Inc()
				metrics.ApprovalDecisionsTotal.WithLabelValues("auto_approved", "staging").Inc()
			}).NotTo(Panic())
		})
	})

	// ========================================
	// CONFIDENCE METRICS
	// ========================================
	Describe("Confidence Metrics (DD-005 Compliant)", func() {
		// BR-AI-OBSERVABILITY-004: Track AI confidence distribution
		It("should register ConfidenceScoreDistribution histogram", func() {
			Expect(metrics.ConfidenceScoreDistribution).NotTo(BeNil())
			desc := metrics.ConfidenceScoreDistribution.WithLabelValues("OOMKilled").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_confidence_score_distribution"))
		})

		// Business outcome: Track confidence by signal type for tuning
		It("should support recording confidence scores by signal type", func() {
			Expect(func() {
				metrics.ConfidenceScoreDistribution.WithLabelValues("OOMKilled").Observe(0.85)
				metrics.ConfidenceScoreDistribution.WithLabelValues("CrashLoopBackOff").Observe(0.72)
			}).NotTo(Panic())
		})
	})

	// ========================================
	// FAILURE METRICS
	// ========================================
	Describe("Failure Metrics (DD-005 Compliant)", func() {
		// BR-HAPI-197: Track workflow resolution failures
		It("should register FailuresTotal counter", func() {
			Expect(metrics.FailuresTotal).NotTo(BeNil())
			desc := metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "LowConfidence").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_failures_total"))
		})

		// Business outcome: Track failure reasons for operator dashboards
		It("should support recording failures by reason and sub_reason", func() {
			Expect(func() {
				metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "WorkflowNotFound").Inc()
				metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "LLMParsingError").Inc()
				metrics.FailuresTotal.WithLabelValues("APIError", "TransientError").Inc()
			}).NotTo(Panic())
		})
	})

	// ========================================
	// DETECTED LABELS METRICS
	// ========================================
	Describe("DetectedLabels Metrics (DD-005 Compliant)", func() {
		// DD-WORKFLOW-001: Track label detection failures
		It("should register DetectedLabelsFailuresTotal counter", func() {
			Expect(metrics.DetectedLabelsFailuresTotal).NotTo(BeNil())
			desc := metrics.DetectedLabelsFailuresTotal.WithLabelValues("environment").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_detected_labels_failures_total"))
		})
	})
})

