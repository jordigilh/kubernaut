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

	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
)

// Day 5: Metrics Unit Tests (v1.13 - Business Value Metrics Only)
// DD-005 Compliant: Tests verify correct metric naming and behavior
// METRIC SELECTION: Only business-value metrics are included (see IMPLEMENTATION_PLAN_V1.0.md v1.13)
var _ = Describe("AIAnalysis Metrics", func() {
	// ========================================
	// RECONCILER METRICS (Business: Throughput + SLA)
	// ========================================
	Describe("Reconciler Metrics (DD-005 Compliant)", func() {
		// BR-AI-OBSERVABILITY-001: Track reconciliation outcomes
		// Business: "How many analyses completed?" - Throughput SLA
		It("should register ReconcilerReconciliationsTotal counter", func() {
			Expect(metrics.ReconcilerReconciliationsTotal).NotTo(BeNil())
			// Verify metric name follows DD-005 convention: {service}_{component}_{metric}_{unit}
			desc := metrics.ReconcilerReconciliationsTotal.WithLabelValues("Pending", "success").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_reconciler_reconciliations_total"))
		})

		// Business: "Did we meet <60s SLA target?" - Latency SLA
		It("should register ReconcilerDurationSeconds histogram", func() {
			Expect(metrics.ReconcilerDurationSeconds).NotTo(BeNil())
			// Verify metric can be used (histograms return Observer from WithLabelValues)
			Expect(func() {
				metrics.ReconcilerDurationSeconds.WithLabelValues("Investigating").Observe(1.5)
			}).NotTo(Panic())
		})

		// Business outcome: Track phase transition success rate
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
	// REGO POLICY METRICS (Business: Policy Decisions)
	// ========================================
	Describe("Rego Policy Metrics (DD-005 Compliant)", func() {
		// BR-AI-030: Track policy evaluation outcomes
		// Business: "How many policy decisions were made?"
		It("should register RegoEvaluationsTotal counter", func() {
			Expect(metrics.RegoEvaluationsTotal).NotTo(BeNil())
			desc := metrics.RegoEvaluationsTotal.WithLabelValues("approved", "false").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_rego_evaluations_total"))
		})
	})

	// ========================================
	// APPROVAL METRICS (Business: Core Outcome)
	// ========================================
	Describe("Approval Metrics (DD-005 Compliant)", func() {
		// BR-AI-059: Track approval decisions
		// Business: "What's the approval vs auto-execute ratio?"
		It("should register ApprovalDecisionsTotal counter", func() {
			Expect(metrics.ApprovalDecisionsTotal).NotTo(BeNil())
			desc := metrics.ApprovalDecisionsTotal.WithLabelValues("approved", "production").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_approval_decisions_total"))
		})

		// Business outcome: Track approval reasons for operator dashboards
		It("should support recording approval decisions by environment", func() {
			Expect(func() {
				metrics.ApprovalDecisionsTotal.WithLabelValues("manual_review_required", "production").Inc()
				metrics.ApprovalDecisionsTotal.WithLabelValues("auto_approved", "staging").Inc()
			}).NotTo(Panic())
		})
	})

	// ========================================
	// CONFIDENCE METRICS (Business: AI Quality)
	// ========================================
	Describe("Confidence Metrics (DD-005 Compliant)", func() {
		// BR-AI-OBSERVABILITY-004: Track AI confidence distribution
		// Business: "How reliable is the AI model?"
		It("should register ConfidenceScoreDistribution histogram", func() {
			Expect(metrics.ConfidenceScoreDistribution).NotTo(BeNil())
			// Verify metric can be used (histograms return Observer from WithLabelValues)
			Expect(func() {
				metrics.ConfidenceScoreDistribution.WithLabelValues("OOMKilled").Observe(0.85)
			}).NotTo(Panic())
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
	// FAILURE METRICS (Business: Failure Tracking)
	// ========================================
	Describe("Failure Metrics (DD-005 Compliant)", func() {
		// BR-HAPI-197: Track workflow resolution failures
		// Business: "What failure modes are occurring?"
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
	// AUDIT METRICS (Compliance: LLM Validation)
	// ========================================
	Describe("Audit Metrics (DD-005 Compliant)", func() {
		// DD-HAPI-002 v1.4: Track validation attempts from HAPI
		// Audit: "How many LLM self-correction attempts occurred?"
		It("should register ValidationAttemptsTotal counter", func() {
			Expect(metrics.ValidationAttemptsTotal).NotTo(BeNil())
			desc := metrics.ValidationAttemptsTotal.WithLabelValues("restart-pod-v1", "false").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_audit_validation_attempts_total"))
		})
	})

	// ========================================
	// DATA QUALITY METRICS (Quality: Enrichment)
	// ========================================
	Describe("Data Quality Metrics (DD-005 Compliant)", func() {
		// DD-WORKFLOW-001: Track label detection failures
		// Quality: "Are there enrichment/detection issues?"
		It("should register DetectedLabelsFailuresTotal counter", func() {
			Expect(metrics.DetectedLabelsFailuresTotal).NotTo(BeNil())
			desc := metrics.DetectedLabelsFailuresTotal.WithLabelValues("environment").Desc()
			Expect(desc.String()).To(ContainSubstring("aianalysis_quality_detected_labels_failures_total"))
		})
	})
})
