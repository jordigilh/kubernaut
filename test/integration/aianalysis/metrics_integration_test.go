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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// METRICS INTEGRATION TESTS
// Business Requirement: BR-AI-OBSERVABILITY-001
// v2.0: BUSINESS FLOW VALIDATION (not direct method calls)
// ========================================
//
// Integration Test Strategy (per DD-TEST-001 and METRICS_ANTI_PATTERN_TRIAGE):
// ✅ CORRECT: Validate metrics as SIDE EFFECTS of business logic
// ❌ WRONG: Direct calls to metrics methods (testMetrics.RecordXxx())
//
// These tests verify that metrics are:
// 1. Emitted during actual AIAnalysis CRD reconciliation
// 2. Correctly reflect business outcomes (phase transitions, failures, etc.)
// 3. Available in Prometheus registry after business flows complete
//
// Pattern: CREATE CRD → WAIT FOR RECONCILIATION → VERIFY METRICS
// ========================================

// SERIAL EXECUTION: AA integration suite runs serially for 100% reliability.
// See audit_flow_integration_test.go for detailed rationale.
var _ = Describe("Metrics Integration via Business Flows", Serial, Label("integration", "metrics"), func() {
	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default" // Use default namespace for integration tests
	})

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

	// Helper to get counter value with specific labels
	getCounterValue := func(name string, labels map[string]string) float64 {
		families, err := gatherMetrics()
		if err != nil {
			return 0
		}
		family, exists := families[name]
		if !exists {
			return 0
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
			if labelMatch && m.GetCounter() != nil {
				return m.GetCounter().GetValue()
			}
		}
		return 0
	}

	// Helper to get histogram sample count
	getHistogramCount := func(name string, labels map[string]string) uint64 {
		families, err := gatherMetrics()
		if err != nil {
			return 0
		}
		family, exists := families[name]
		if !exists {
			return 0
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
			if labelMatch && m.GetHistogram() != nil {
				return m.GetHistogram().GetSampleCount()
			}
		}
		return 0
	}

	// ========================================
	// RECONCILIATION METRICS (BR-AI-OBSERVABILITY-001)
	// ========================================
	Context("Reconciliation Metrics via AIAnalysis Lifecycle", Serial, func() {
		// NOTE: Running serially due to metrics registry state interference
		// Parallel execution causes timeout failures due to shared Prometheus registry
		It("should emit reconciliation metrics during successful AIAnalysis flow - BR-AI-OBSERVABILITY-001", func() {
			// 1. Create AIAnalysis CRD (triggers business logic)
			aianalysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("metrics-test-success-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr",
						Namespace: namespace,
					},
					RemediationID: "test-rem-001",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fp-001",
							Severity:         "critical",
							SignalType:       "OOMKilled",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "test-pod",
								Namespace: namespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
									GitOpsTool:    "argocd",
								},
							},
						},
						AnalysisTypes: []string{"incident-analysis", "workflow-selection"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

		// 2. Wait for business outcome (reconciliation completes)
		Eventually(func() string {
			var updated aianalysisv1alpha1.AIAnalysis
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &updated); err != nil {
				return ""
			}
			return string(updated.Status.Phase)
		}, 60*time.Second, 500*time.Millisecond).Should(Equal("Completed"))

		// 3. Verify metrics were emitted as side effect of reconciliation
		// Note: Metrics are recorded with phase BEFORE transition, so after Completed:
		// - Pending→Investigating: metric "Pending/success"
		// - Investigating→Analyzing: metric "Investigating/success" ✅
		// - Analyzing→Completed: metric "Analyzing/success" ✅
		Eventually(func() float64 {
			return getCounterValue(metrics.MetricNameReconcilerReconciliationsTotal, map[string]string{
				"phase":  "Investigating",
				"result": "success",
			})
		}, 60*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"Reconciliation metric should be emitted during Investigating phase")

		Eventually(func() float64 {
			return getCounterValue(metrics.MetricNameReconcilerReconciliationsTotal, map[string]string{
				"phase":  "Analyzing",
				"result": "success",
			})
		}, 60*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"Reconciliation metric should be emitted during Analyzing phase")

		// Verify duration histogram was populated
		Eventually(func() uint64 {
			return getHistogramCount(metrics.MetricNameReconcilerDurationSeconds, map[string]string{
				"phase": "Investigating",
			})
		}, 60*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"Duration histogram should record Investigating phase duration")
		})

		// NOTE: Flaky in parallel execution - metrics registry state interference
	It("should NOT emit failure metrics when AIAnalysis completes successfully - BR-HAPI-197", func() {
			// 1. Capture baseline failure metrics before test
			baselineFailures := getCounterValue(metrics.MetricNameFailuresTotal, map[string]string{
				"reason": "WorkflowResolutionFailed",
			}) + getCounterValue(metrics.MetricNameFailuresTotal, map[string]string{
				"reason": "APIError",
			}) + getCounterValue(metrics.MetricNameFailuresTotal, map[string]string{
				"reason": "NoWorkflowSelected",
			})

			// 2. Create AIAnalysis that will complete successfully
			// Note: Mock HolmesGPT client returns success, so this tests the happy path
			aianalysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("metrics-test-success-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-success",
						Namespace: namespace,
					},
					RemediationID: "test-rem-002",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fp-002",
							Severity:         "critical",
							SignalType:       "OOMKilled",
							Environment:      "staging",
							BusinessPriority: "P1",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "success-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"incident-analysis", "workflow-selection"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

			// 3. Wait for successful completion
			Eventually(func() string {
				var updated aianalysisv1alpha1.AIAnalysis
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &updated); err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 60*time.Second, 500*time.Millisecond).Should(Equal("Completed"),
				"AIAnalysis should complete successfully with mock returning success")

			// 4. Verify failure metrics were NOT incremented
			// Success path should not emit failure metrics
			Consistently(func() float64 {
				currentFailures := getCounterValue(metrics.MetricNameFailuresTotal, map[string]string{
					"reason": "WorkflowResolutionFailed",
				}) + getCounterValue(metrics.MetricNameFailuresTotal, map[string]string{
					"reason": "APIError",
				}) + getCounterValue(metrics.MetricNameFailuresTotal, map[string]string{
					"reason": "NoWorkflowSelected",
				})
				return currentFailures - baselineFailures
			}, 60*time.Second, 500*time.Millisecond).Should(Equal(float64(0)),
				"Failure metrics should NOT be emitted when AIAnalysis completes successfully")
		})
	})

	// ========================================
	// APPROVAL DECISION METRICS (BR-AI-022)
	// ========================================
	Context("Approval Decision Metrics via Policy Evaluation", Serial, func() {
		// NOTE: Running serially due to metrics registry state interference
		// Parallel execution causes timeout failures due to shared Prometheus registry
		It("should emit approval decision metrics based on environment - BR-AI-022", func() {
			// 1. Create AIAnalysis for production (should require approval)
			aianalysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("metrics-test-approval-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-prod",
						Namespace: namespace,
					},
					RemediationID: "test-rem-003",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fp-003",
							Severity:         "critical",
							SignalType:       "CrashLoopBackOff",
							Environment:      "production", // Production should require approval
							BusinessPriority: "P0",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "prod-pod",
								Namespace: namespace,
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{
								DetectedLabels: &sharedtypes.DetectedLabels{
									GitOpsManaged: true,
									GitOpsTool:    "argocd",
								},
							},
						},
						AnalysisTypes: []string{"incident-analysis", "workflow-selection"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

		// 2. Wait for analysis phase to complete
		Eventually(func() bool {
			var updated aianalysisv1alpha1.AIAnalysis
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &updated); err != nil {
				return false
			}
			return updated.Status.Phase == "AwaitingApproval" || updated.Status.Phase == "Completed"
		}, 60*time.Second, 500*time.Millisecond).Should(BeTrue())

		// 3. Verify approval decision metrics were emitted
		Eventually(func() float64 {
			// Look for any approval decision metric
			total := getCounterValue(metrics.MetricNameApprovalDecisionsTotal, map[string]string{
				"environment": "production",
			})
			if total > 0 {
				return total
			}
			// Also check for auto-approved or requires_approval
			return getCounterValue(metrics.MetricNameApprovalDecisionsTotal, map[string]string{
				"decision": "requires_approval",
			})
		}, 60*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"Approval decision metric should be emitted during policy evaluation")
		})
	})

	// ========================================
	// CONFIDENCE SCORE METRICS
	// ========================================
	Context("Confidence Score Metrics via Workflow Selection", Serial, func() {
		// NOTE: Running serially due to metrics registry state interference
		It("should emit confidence score histogram during workflow selection - BR-AI-022", FlakeAttempts(3), func() {
			// 1. Create AIAnalysis that will select a workflow
			aianalysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("metrics-test-confidence-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-confidence",
						Namespace: namespace,
					},
					RemediationID: "test-rem-004",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fp-004",
							Severity:         "critical",
							SignalType:       "ImagePullBackOff",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "confidence-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"incident-analysis", "workflow-selection"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

		// 2. Wait for workflow selection to complete
		Eventually(func() bool {
			var updated aianalysisv1alpha1.AIAnalysis
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &updated); err != nil {
				return false
			}
			return updated.Status.SelectedWorkflow != nil
		}, 60*time.Second, 500*time.Millisecond).Should(BeTrue())

		// 3. Verify confidence score histogram was populated
		Eventually(func() uint64 {
			return getHistogramCount(metrics.MetricNameConfidenceScoreDistribution, map[string]string{
				"signal_type": "ImagePullBackOff",
			})
		}, 60*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
			"Confidence score histogram should be populated during workflow selection")
		})
	})

	// ========================================
	// REGO EVALUATION METRICS
	// ========================================
	Context("Rego Evaluation Metrics via Policy Processing", Serial, func() {
		// NOTE: Running serially due to metrics registry state interference
		It("should emit Rego evaluation metrics during analysis phase", func() {
			// 1. Create AIAnalysis that will trigger policy evaluation
			aianalysis := &aianalysisv1alpha1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("metrics-test-rego-%s", uuid.New().String()[:8]),
					Namespace: namespace,
				},
				Spec: aianalysisv1alpha1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      "test-rr-rego",
						Namespace: namespace,
					},
					RemediationID: "test-rem-005",
					AnalysisRequest: aianalysisv1alpha1.AnalysisRequest{
						SignalContext: aianalysisv1alpha1.SignalContextInput{
							Fingerprint:      "test-fp-005",
							Severity:         "warning",
							SignalType:       "PodEviction",
							Environment:      "development",
							BusinessPriority: "P3",
							TargetResource: aianalysisv1alpha1.TargetResource{
								Kind:      "Pod",
								Name:      "rego-pod",
								Namespace: namespace,
							},
						},
						AnalysisTypes: []string{"incident-analysis"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aianalysis)).To(Succeed())

			// 2. Wait for analysis to complete
			Eventually(func() string {
				var updated aianalysisv1alpha1.AIAnalysis
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(aianalysis), &updated); err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 60*time.Second, 500*time.Millisecond).Should(Or(Equal("Completed"), Equal("AwaitingApproval")))

			// 3. Verify Rego evaluation metrics were emitted
			Eventually(func() float64 {
				// Look for any Rego evaluation outcome (approved or rejected)
				// Metric uses labels: "outcome" and "degraded"
				total := getCounterValue(metrics.MetricNameRegoEvaluationsTotal, map[string]string{
					"outcome":  "approved",
					"degraded": "false",
				})
				if total > 0 {
					return total
				}
				return getCounterValue(metrics.MetricNameRegoEvaluationsTotal, map[string]string{
					"outcome":  "rejected",
					"degraded": "false",
				})
			}, 60*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"Rego evaluation metric should be emitted during analysis phase")
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
