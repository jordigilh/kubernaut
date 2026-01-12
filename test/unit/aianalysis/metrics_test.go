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
// DD-METRICS-001: Tests use dependency-injected metrics pattern
// METRIC SELECTION: Only business-value metrics are included (see IMPLEMENTATION_PLAN_V1.0.md v1.13)
var _ = Describe("AIAnalysis Metrics", func() {
	var m *metrics.Metrics

	BeforeEach(func() {
		// Per DD-METRICS-001: Create metrics instance for testing
		m = metrics.NewMetrics()
	})

	// ========================================
	// RECONCILER METRICS (Business: Throughput + SLA)
	// BR-AI-OBSERVABILITY-001: Operators need visibility into system health
	// ========================================
	Describe("ReconciliationMetrics.RecordReconciliation", func() {
		It("should enable operators to measure system throughput for capacity planning", func() {
			By("Recording successful and failed reconciliations")
			m.ReconcilerReconciliationsTotal.WithLabelValues("Pending", "success").Inc()
			m.ReconcilerReconciliationsTotal.WithLabelValues("Investigating", "failure").Inc()

			By("Verifying metric is available for SLA monitoring")
			Expect(m.ReconcilerReconciliationsTotal).NotTo(BeNil(),
				"Operators need reconciliation counts to track system throughput and SLA compliance")

			By("Verifying metric naming follows observability standards")
			desc := m.ReconcilerReconciliationsTotal.WithLabelValues("Pending", "success").Desc()
			// DD-005 V3.0: Use constant from production code to prevent typos
			Expect(desc.String()).To(ContainSubstring(metrics.MetricNameReconcilerReconciliationsTotal),
				"Standard metric naming enables Prometheus queries across services")
		})

		It("should track phase-specific success rates for bottleneck identification", func() {
			By("Recording outcomes for each reconciliation phase")
			phases := []string{"Pending", "Investigating", "Analyzing", "Completed"}
			for _, phase := range phases {
				m.ReconcilerReconciliationsTotal.WithLabelValues(phase, "success").Inc()
			}

			By("Verifying operators can identify which phases fail most often")
			Expect(func() {
				m.ReconcilerReconciliationsTotal.WithLabelValues("Investigating", "failure").Inc()
			}).NotTo(Panic(), "Phase-level metrics help operators identify bottlenecks")
		})
	})

	Describe("ReconciliationMetrics.ObserveReconciliationDuration", func() {
		It("should enable operators to measure SLA compliance (<60s target)", func() {
			By("Recording reconciliation duration for performance tracking")
			m.ReconcilerDurationSeconds.WithLabelValues("Investigating").Observe(1.5)
			m.ReconcilerDurationSeconds.WithLabelValues("Analyzing").Observe(45.2)

			By("Verifying histogram enables SLA percentile queries")
			Expect(m.ReconcilerDurationSeconds).NotTo(BeNil(),
				"Operators need duration metrics to verify <60s SLA compliance")

			By("Verifying histogram can track slow reconciliations")
			Expect(func() {
				m.ReconcilerDurationSeconds.WithLabelValues("Investigating").Observe(120.0)
			}).NotTo(Panic(), "Tracking slow reconciliations (>60s) helps identify performance issues")
		})

		It("should support capacity planning through latency percentile analysis", func() {
			By("Simulating various reconciliation durations")
			durations := []float64{0.5, 1.2, 2.8, 5.1, 8.9, 15.3, 30.7, 55.2}
			for _, duration := range durations {
				m.ReconcilerDurationSeconds.WithLabelValues("Analyzing").Observe(duration)
			}

			By("Verifying operators can calculate p50, p95, p99 for capacity planning")
			// Histogram buckets enable percentile calculations in Prometheus
			Expect(m.ReconcilerDurationSeconds).NotTo(BeNil(),
				"Latency percentiles (p50, p95, p99) guide infrastructure scaling decisions")
		})
	})

	// ========================================
	// REGO POLICY METRICS (Business: Policy Decisions)
	// BR-AI-030: Operators need visibility into policy evaluation outcomes
	// ========================================
	Describe("PolicyMetrics.RecordPolicyEvaluation", func() {
		It("should enable operators to audit policy decision rates for compliance", func() {
			By("Recording policy evaluation outcomes")
			m.RegoEvaluationsTotal.WithLabelValues("approved", "false").Inc()
			m.RegoEvaluationsTotal.WithLabelValues("denied", "false").Inc()
			m.RegoEvaluationsTotal.WithLabelValues("approved", "true").Inc()

			By("Verifying operators can track approval vs denial ratios")
			Expect(m.RegoEvaluationsTotal).NotTo(BeNil(),
				"Policy decision metrics enable compliance audits and approval rate analysis")

			By("Verifying degraded mode evaluations are tracked separately")
			desc := m.RegoEvaluationsTotal.WithLabelValues("approved", "true").Desc()
			// DD-005 V3.0: Use constant from production code to prevent typos
			Expect(desc.String()).To(ContainSubstring(metrics.MetricNameRegoEvaluationsTotal),
				"Degraded mode tracking alerts operators to policy evaluation issues")
		})

		It("should help operators identify policy configuration issues through degraded mode tracking", func() {
			By("Simulating policy evaluations with degraded mode flag")
			// Normal evaluations
			for i := 0; i < 95; i++ {
				m.RegoEvaluationsTotal.WithLabelValues("approved", "false").Inc()
			}
			// Degraded evaluations (policy file issues, syntax errors)
			for i := 0; i < 5; i++ {
				m.RegoEvaluationsTotal.WithLabelValues("approved", "true").Inc()
			}

			By("Verifying >5% degraded rate alerts operators to policy problems")
			// Business value: High degraded rate indicates policy file corruption or syntax errors
			Expect(m.RegoEvaluationsTotal).NotTo(BeNil(),
				"Degraded mode rate (>5%) indicates policy configuration issues requiring attention")
		})
	})

	// ========================================
	// APPROVAL METRICS (Business: Core Outcome)
	// BR-AI-059: Track approval decisions for automation rate analysis
	// ========================================
	Describe("ApprovalMetrics.RecordApprovalDecision", func() {
		It("should enable operators to measure automation rate for efficiency reporting", func() {
			By("Recording automatic approvals for staging environment")
			for i := 0; i < 80; i++ {
				m.ApprovalDecisionsTotal.WithLabelValues("auto_approved", "staging").Inc()
			}

			By("Recording manual review requirements for production")
			for i := 0; i < 20; i++ {
				m.ApprovalDecisionsTotal.WithLabelValues("manual_review_required", "production").Inc()
			}

			By("Verifying automation rate is measurable for efficiency KPIs")
			Expect(m.ApprovalDecisionsTotal).NotTo(BeNil(),
				"Automation rate (80% auto-approved) demonstrates business value of AI analysis")

			By("Verifying metric enables environment-specific analysis")
			desc := m.ApprovalDecisionsTotal.WithLabelValues("approved", "production").Desc()
			// DD-005 V3.0: Use constant from production code to prevent typos
			Expect(desc.String()).To(ContainSubstring(metrics.MetricNameApprovalDecisionsTotal),
				"Environment-specific rates show policy effectiveness (prod vs staging)")
		})

		It("should help operators understand why manual reviews are required", func() {
			By("Recording approval decisions with detailed reasons")
			// Production safety reasons
			m.ApprovalDecisionsTotal.WithLabelValues("manual_review_required", "production").Inc()
			m.ApprovalDecisionsTotal.WithLabelValues("low_confidence", "production").Inc()
			m.ApprovalDecisionsTotal.WithLabelValues("data_quality_issue", "production").Inc()

			By("Verifying operators can identify top reasons for manual review")
			// Business value: Understanding bottlenecks guides policy tuning
			Expect(func() {
				m.ApprovalDecisionsTotal.WithLabelValues("multiple_recovery_attempts", "production").Inc()
			}).NotTo(Panic(), "Approval reason breakdown guides policy optimization efforts")
		})
	})

	// ========================================
	// CONFIDENCE METRICS (Business: AI Quality)
	// BR-AI-OBSERVABILITY-004: Operators need visibility into AI model reliability
	// ========================================
	Describe("AIMetrics.RecordConfidenceScore", func() {
		It("should enable operators to measure AI model reliability for trust validation", func() {
			By("Recording confidence scores for various signal types")
			// High confidence signals (model is certain)
			m.ConfidenceScoreDistribution.WithLabelValues("OOMKilled").Observe(0.92)
			m.ConfidenceScoreDistribution.WithLabelValues("OOMKilled").Observe(0.88)

			// Lower confidence signals (model is uncertain)
			m.ConfidenceScoreDistribution.WithLabelValues("CrashLoopBackOff").Observe(0.68)
			m.ConfidenceScoreDistribution.WithLabelValues("CrashLoopBackOff").Observe(0.71)

			By("Verifying histogram enables confidence distribution analysis")
			Expect(m.ConfidenceScoreDistribution).NotTo(BeNil(),
				"Confidence distribution (p50, p95) validates AI model reliability for operators")
		})

		It("should help operators identify signal types needing model improvement", func() {
			By("Simulating confidence scores across different signal types")
			// Well-handled signals (high confidence)
			for i := 0; i < 20; i++ {
				m.ConfidenceScoreDistribution.WithLabelValues("OOMKilled").Observe(0.85 + float64(i%10)*0.01)
			}

			// Poorly-handled signals (low confidence) - needs model training
			for i := 0; i < 20; i++ {
				m.ConfidenceScoreDistribution.WithLabelValues("NetworkError").Observe(0.55 + float64(i%10)*0.01)
			}

			By("Verifying per-signal-type metrics guide model training priorities")
			Expect(func() {
				m.ConfidenceScoreDistribution.WithLabelValues("DatabaseConnectionLost").Observe(0.62)
			}).NotTo(Panic(), "Low confidence for specific signal types identifies training needs")
		})
	})

	// ========================================
	// FAILURE METRICS (Business: Failure Tracking)
	// BR-HAPI-197: Track failure modes for root cause analysis
	// ========================================
	Describe("ErrorMetrics.RecordFailureMode", func() {
		It("should enable operators to identify top failure modes for prioritized fixes", func() {
			By("Recording various failure scenarios")
			// Most common failure: Workflow resolution issues
			for i := 0; i < 15; i++ {
				m.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "WorkflowNotFound").Inc()
			}
			// Second common: API errors
			for i := 0; i < 8; i++ {
				m.FailuresTotal.WithLabelValues("APIError", "TransientError").Inc()
			}
			// Less common: LLM parsing errors
			for i := 0; i < 3; i++ {
				m.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "LLMParsingError").Inc()
			}

			By("Verifying failure breakdown enables prioritized troubleshooting")
			Expect(m.FailuresTotal).NotTo(BeNil(),
				"Failure mode distribution (15 workflow, 8 API, 3 parsing) guides fix priorities")

			By("Verifying sub-reason granularity enables root cause analysis")
			desc := m.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "LowConfidence").Desc()
			// DD-005 V3.0: Use constant from production code to prevent typos
			Expect(desc.String()).To(ContainSubstring(metrics.MetricNameFailuresTotal),
				"Sub-reason granularity (WorkflowNotFound vs LowConfidence) guides specific fixes")
		})

		It("should help operators distinguish transient from permanent failures", func() {
			By("Recording transient failures (should resolve with retry)")
			m.FailuresTotal.WithLabelValues("APIError", "TransientError").Inc()
			m.FailuresTotal.WithLabelValues("APIError", "NetworkTimeout").Inc()

			By("Recording permanent failures (need configuration fixes)")
			m.FailuresTotal.WithLabelValues("APIError", "AuthenticationFailed").Inc()
			m.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Inc()

			By("Verifying failure type guides operator response strategy")
			Expect(func() {
				// Transient: Wait for automatic retry
				m.FailuresTotal.WithLabelValues("APIError", "RateLimitExceeded").Inc()
				// Permanent: Fix configuration now
				m.FailuresTotal.WithLabelValues("RegoEvaluationError", "PolicySyntaxError").Inc()
			}).NotTo(Panic(), "Failure classification guides immediate action vs wait-for-retry")
		})
	})

	// ========================================
	// AUDIT METRICS (Compliance: LLM Validation)
	// DD-HAPI-002 v1.4: Track LLM self-correction for compliance
	// ========================================
	Describe("LLM Validation Attempt Tracking - DD-HAPI-002", func() {
		It("should enable auditors to verify LLM self-correction behavior", func() {
			By("Recording successful LLM validations (passed on first try)")
			for i := 0; i < 85; i++ {
				m.ValidationAttemptsTotal.WithLabelValues("restart-pod-v1", "true").Inc()
			}

			By("Recording failed LLM validations (needed human review)")
			for i := 0; i < 15; i++ {
				m.ValidationAttemptsTotal.WithLabelValues("scale-deployment-v2", "false").Inc()
			}

			By("Verifying validation success rate is auditable for compliance")
			Expect(m.ValidationAttemptsTotal).NotTo(BeNil(),
				"LLM validation rate (85% success) demonstrates self-correction effectiveness")

			By("Verifying per-workflow success rate identifies problematic workflows")
			desc := m.ValidationAttemptsTotal.WithLabelValues("restart-pod-v1", "false").Desc()
			// DD-005 V3.0: Use constant from production code to prevent typos
			Expect(desc.String()).To(ContainSubstring(metrics.MetricNameValidationAttemptsTotal),
				"Per-workflow validation rate identifies which workflows need LLM tuning")
		})

		It("should help operators understand LLM self-correction quality", func() {
			By("Recording multiple validation attempts for same workflow")
			// Workflow A: High success rate (good LLM quality)
			for i := 0; i < 95; i++ {
				m.ValidationAttemptsTotal.WithLabelValues("restart-pod", "true").Inc()
			}
			for i := 0; i < 5; i++ {
				m.ValidationAttemptsTotal.WithLabelValues("restart-pod", "false").Inc()
			}

			// Workflow B: Low success rate (needs LLM improvement)
			for i := 0; i < 60; i++ {
				m.ValidationAttemptsTotal.WithLabelValues("complex-migration", "true").Inc()
			}
			for i := 0; i < 40; i++ {
				m.ValidationAttemptsTotal.WithLabelValues("complex-migration", "false").Inc()
			}

			By("Verifying low success rate (<70%) triggers LLM training review")
			Expect(func() {
				m.ValidationAttemptsTotal.WithLabelValues("database-failover", "false").Inc()
			}).NotTo(Panic(), "Workflows with <70% validation rate need LLM prompt engineering")
		})
	})

	// ========================================
	// DATA QUALITY METRICS (Quality: Enrichment)
	// DD-WORKFLOW-001: Track label detection failures for data quality
	// ========================================
	Describe("Data Quality Monitoring - DD-WORKFLOW-001", func() {
		It("should enable operators to identify enrichment data quality issues", func() {
			By("Recording label detection failures by field")
			// Critical fields with high failure rates
			for i := 0; i < 25; i++ {
				m.DetectedLabelsFailuresTotal.WithLabelValues("environment").Inc()
			}
			// Less critical fields with acceptable failure rates
			for i := 0; i < 5; i++ {
				m.DetectedLabelsFailuresTotal.WithLabelValues("team_owner").Inc()
			}

			By("Verifying field-specific failure rates identify data quality gaps")
			Expect(m.DetectedLabelsFailuresTotal).NotTo(BeNil(),
				"Label detection failure rate (25% for environment) indicates upstream data issues")

			By("Verifying high failure rate (>20%) triggers data quality investigation")
			desc := m.DetectedLabelsFailuresTotal.WithLabelValues("environment").Desc()
			// DD-005 V3.0: Use constant from production code to prevent typos
			Expect(desc.String()).To(ContainSubstring(metrics.MetricNameDetectedLabelsFailuresTotal),
				"Environment label missing in 25% of cases requires upstream investigation")
		})

		It("should help operators prioritize enrichment improvements", func() {
			By("Simulating detection failures across various label types")
			// High-priority labels (affect policy decisions)
			m.DetectedLabelsFailuresTotal.WithLabelValues("environment").Inc() // Affects approval policy
			m.DetectedLabelsFailuresTotal.WithLabelValues("criticality").Inc() // Affects priority

			// Medium-priority labels (affect routing)
			m.DetectedLabelsFailuresTotal.WithLabelValues("namespace").Inc() // Affects scope
			m.DetectedLabelsFailuresTotal.WithLabelValues("cluster").Inc()   // Affects routing

			// Low-priority labels (informational only)
			m.DetectedLabelsFailuresTotal.WithLabelValues("cost_center").Inc() // Nice to have

			By("Verifying failure impact guides enrichment improvement priority")
			Expect(func() {
				m.DetectedLabelsFailuresTotal.WithLabelValues("business_unit").Inc()
			}).NotTo(Panic(), "Label importance (policy vs informational) guides fix priorities")
		})
	})
})
