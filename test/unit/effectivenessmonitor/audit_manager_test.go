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

package effectivenessmonitor

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// spyAuditStore captures the last event passed to StoreAudit for inspection.
type spyAuditStore struct {
	lastEvent *ogenclient.AuditEventRequest
	err       error
}

var _ pkgaudit.AuditStore = (*spyAuditStore)(nil)

func (s *spyAuditStore) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	s.lastEvent = event
	return s.err
}

func (s *spyAuditStore) Flush(_ context.Context) error { return nil }
func (s *spyAuditStore) Close() error                  { return nil }

// newTestEA creates a minimal EffectivenessAssessment for audit manager tests.
func newTestEA() *eav1.EffectivenessAssessment {
	return &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ea-test-001",
			Namespace: "default",
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID: "rr-test-001",
			TargetResource: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "nginx",
				Namespace: "default",
			},
		},
	}
}

var _ = Describe("Audit Manager — Typed Sub-Objects (DD-017 v2.5)", func() {

	var (
		spy *spyAuditStore
		mgr *audit.Manager
		ea  *eav1.EffectivenessAssessment
		ctx context.Context
	)

	BeforeEach(func() {
		spy = &spyAuditStore{}
		mgr = audit.NewManager(spy, logr.Discard())
		ea = newTestEA()
		ctx = context.Background()
	})

	// ========================================
	// UT-EM-AM-001: RecordHealthAssessed sets health_checks sub-object
	// ========================================
	Describe("RecordHealthAssessed (UT-EM-AM-001 through UT-EM-AM-003)", func() {

		It("UT-EM-AM-001: should set health_checks sub-object with all fields", func() {
			score := 1.0
			result := types.ComponentResult{
				Component: types.ComponentHealth,
				Assessed:  true,
				Score:     &score,
				Details:   "all 3 pods ready, no restarts",
			}

			healthData := audit.HealthAssessedData{
				TotalReplicas:           3,
				ReadyReplicas:           3,
				RestartsSinceRemediation: 0,
				CrashLoops:             false,
				OOMKilled:              false,
				PendingCount:           0,
			}

			err := mgr.RecordHealthAssessed(ctx, ea, result, healthData)
			Expect(err).ToNot(HaveOccurred())
			Expect(spy.lastEvent).ToNot(BeNil())

			// Extract the EffectivenessAssessmentAuditPayload from EventData
			payload := extractPayload(spy.lastEvent)

			// Verify health_checks sub-object is set
			Expect(payload.HealthChecks.Set).To(BeTrue(), "health_checks should be set")
			hc := payload.HealthChecks.Value
			Expect(hc.PodRunning.Value).To(BeTrue())
			Expect(hc.ReadinessPass.Value).To(BeTrue())
			Expect(hc.TotalReplicas.Value).To(Equal(int32(3)))
			Expect(hc.ReadyReplicas.Value).To(Equal(int32(3)))
			Expect(hc.RestartDelta.Value).To(Equal(int32(0)))
			Expect(hc.CrashLoops.Value).To(BeFalse())
			Expect(hc.OomKilled.Value).To(BeFalse())
			Expect(hc.PendingCount.Value).To(Equal(int32(0)))
		})

		It("UT-EM-AM-002: should set CrashLoops=true and OOMKilled=true when detected", func() {
			score := 0.0
			result := types.ComponentResult{
				Component: types.ComponentHealth,
				Assessed:  true,
				Score:     &score,
				Details:   "CrashLoopBackOff detected",
			}

			healthData := audit.HealthAssessedData{
				TotalReplicas:           3,
				ReadyReplicas:           1,
				RestartsSinceRemediation: 10,
				CrashLoops:             true,
				OOMKilled:              true,
				PendingCount:           1,
			}

			err := mgr.RecordHealthAssessed(ctx, ea, result, healthData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			hc := payload.HealthChecks.Value
			Expect(hc.CrashLoops.Value).To(BeTrue())
			Expect(hc.OomKilled.Value).To(BeTrue())
			Expect(hc.PendingCount.Value).To(Equal(int32(1)))
			Expect(hc.ReadinessPass.Value).To(BeFalse()) // 1 of 3 ready
		})

		It("UT-EM-AM-003: should not set metric_deltas or alert_resolution for health events", func() {
			score := 1.0
			result := types.ComponentResult{
				Component: types.ComponentHealth,
				Assessed:  true,
				Score:     &score,
			}

			healthData := audit.HealthAssessedData{
				TotalReplicas: 1,
				ReadyReplicas: 1,
			}

			err := mgr.RecordHealthAssessed(ctx, ea, result, healthData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			Expect(payload.HealthChecks.Set).To(BeTrue())
			Expect(payload.MetricDeltas.Set).To(BeFalse(), "metric_deltas should not be set for health events")
			Expect(payload.AlertResolution.Set).To(BeFalse(), "alert_resolution should not be set for health events")
		})
	})

	// ========================================
	// UT-EM-AM-004: RecordAlertAssessed sets alert_resolution sub-object
	// ========================================
	Describe("RecordAlertAssessed (UT-EM-AM-004 through UT-EM-AM-006)", func() {

		It("UT-EM-AM-004: should set alert_resolution with resolved=true", func() {
			score := 1.0
			result := types.ComponentResult{
				Component: types.ComponentAlert,
				Assessed:  true,
				Score:     &score,
				Details:   "alert resolved",
			}

			alertData := audit.AlertAssessedData{
				AlertResolved: true,
				ActiveCount:   0,
			}

			err := mgr.RecordAlertAssessed(ctx, ea, result, alertData)
			Expect(err).ToNot(HaveOccurred())
			Expect(spy.lastEvent).ToNot(BeNil())

			payload := extractPayload(spy.lastEvent)
			Expect(payload.AlertResolution.Set).To(BeTrue(), "alert_resolution should be set")
			ar := payload.AlertResolution.Value
			Expect(ar.AlertResolved.Value).To(BeTrue())
			Expect(ar.ActiveCount.Value).To(Equal(int32(0)))
		})

		It("UT-EM-AM-005: should set alert_resolution with resolved=false and active count", func() {
			score := 0.0
			result := types.ComponentResult{
				Component: types.ComponentAlert,
				Assessed:  true,
				Score:     &score,
				Details:   "alert still active",
			}

			alertData := audit.AlertAssessedData{
				AlertResolved: false,
				ActiveCount:   2,
			}

			err := mgr.RecordAlertAssessed(ctx, ea, result, alertData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			ar := payload.AlertResolution.Value
			Expect(ar.AlertResolved.Value).To(BeFalse())
			Expect(ar.ActiveCount.Value).To(Equal(int32(2)))
		})

		// ========================================
		// UT-EM-AM-012: resolution_time_seconds populated when alert resolved (FINDING 2)
		// ADR-EM-001 Section 9.2.3: resolution_time_seconds = time from remediation to alert resolution
		// ========================================
		It("UT-EM-AM-012: should set resolution_time_seconds when alert resolved and value provided", func() {
			score := 1.0
			result := types.ComponentResult{
				Component: types.ComponentAlert,
				Assessed:  true,
				Score:     &score,
				Details:   "alert resolved",
			}

			resTime := 180.5 // 3 minutes to resolve
			alertData := audit.AlertAssessedData{
				AlertResolved:         true,
				ActiveCount:           0,
				ResolutionTimeSeconds: &resTime,
			}

			err := mgr.RecordAlertAssessed(ctx, ea, result, alertData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			ar := payload.AlertResolution.Value
			Expect(ar.AlertResolved.Value).To(BeTrue())
			Expect(ar.ResolutionTimeSeconds.Set).To(BeTrue(),
				"resolution_time_seconds should be set when alert is resolved")
			Expect(ar.ResolutionTimeSeconds.Value).To(BeNumerically("~", 180.5, 0.1))
		})

		It("UT-EM-AM-006: should not set health_checks or metric_deltas for alert events", func() {
			score := 1.0
			result := types.ComponentResult{
				Component: types.ComponentAlert,
				Assessed:  true,
				Score:     &score,
			}

			alertData := audit.AlertAssessedData{
				AlertResolved: true,
				ActiveCount:   0,
			}

			err := mgr.RecordAlertAssessed(ctx, ea, result, alertData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			Expect(payload.AlertResolution.Set).To(BeTrue())
			Expect(payload.HealthChecks.Set).To(BeFalse(), "health_checks should not be set for alert events")
			Expect(payload.MetricDeltas.Set).To(BeFalse(), "metric_deltas should not be set for alert events")
		})
	})

	// ========================================
	// UT-EM-AM-007: RecordMetricsAssessed sets metric_deltas sub-object
	// ========================================
	Describe("RecordMetricsAssessed (UT-EM-AM-007 through UT-EM-AM-009)", func() {

		It("UT-EM-AM-007: should set metric_deltas with cpu_before and cpu_after (Phase A)", func() {
			score := 0.85
			result := types.ComponentResult{
				Component: types.ComponentMetrics,
				Assessed:  true,
				Score:     &score,
				Details:   "CPU improved",
			}

			cpuBefore := 0.8
			cpuAfter := 0.3
			metricsData := audit.MetricsAssessedData{
				CPUBefore: &cpuBefore,
				CPUAfter:  &cpuAfter,
			}

			err := mgr.RecordMetricsAssessed(ctx, ea, result, metricsData)
			Expect(err).ToNot(HaveOccurred())
			Expect(spy.lastEvent).ToNot(BeNil())

			payload := extractPayload(spy.lastEvent)
			Expect(payload.MetricDeltas.Set).To(BeTrue(), "metric_deltas should be set")
			md := payload.MetricDeltas.Value
			Expect(md.CPUBefore.Set).To(BeTrue())
			Expect(md.CPUBefore.Value).To(BeNumerically("~", 0.8, 0.001))
			Expect(md.CPUAfter.Set).To(BeTrue())
			Expect(md.CPUAfter.Value).To(BeNumerically("~", 0.3, 0.001))
		})

		It("UT-EM-AM-008: should leave Phase B fields unset (nil) in Phase A", func() {
			score := 0.85
			result := types.ComponentResult{
				Component: types.ComponentMetrics,
				Assessed:  true,
				Score:     &score,
			}

			cpuBefore := 0.8
			cpuAfter := 0.3
			metricsData := audit.MetricsAssessedData{
				CPUBefore: &cpuBefore,
				CPUAfter:  &cpuAfter,
				// Phase B fields intentionally nil
			}

			err := mgr.RecordMetricsAssessed(ctx, ea, result, metricsData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			md := payload.MetricDeltas.Value
			// Phase B fields should not be set
			Expect(md.MemoryBefore.Set).To(BeFalse(), "memory_before should not be set in Phase A")
			Expect(md.MemoryAfter.Set).To(BeFalse(), "memory_after should not be set in Phase A")
			Expect(md.LatencyP95BeforeMs.Set).To(BeFalse(), "latency_p95_before_ms should not be set in Phase A")
			Expect(md.LatencyP95AfterMs.Set).To(BeFalse(), "latency_p95_after_ms should not be set in Phase A")
			Expect(md.ErrorRateBefore.Set).To(BeFalse(), "error_rate_before should not be set in Phase A")
			Expect(md.ErrorRateAfter.Set).To(BeFalse(), "error_rate_after should not be set in Phase A")
		})

		It("UT-EM-AM-009: should not set health_checks or alert_resolution for metrics events", func() {
			score := 0.5
			result := types.ComponentResult{
				Component: types.ComponentMetrics,
				Assessed:  true,
				Score:     &score,
			}

			cpuBefore := 0.5
			cpuAfter := 0.5
			metricsData := audit.MetricsAssessedData{
				CPUBefore: &cpuBefore,
				CPUAfter:  &cpuAfter,
			}

			err := mgr.RecordMetricsAssessed(ctx, ea, result, metricsData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			Expect(payload.MetricDeltas.Set).To(BeTrue())
			Expect(payload.HealthChecks.Set).To(BeFalse(), "health_checks should not be set for metrics events")
			Expect(payload.AlertResolution.Set).To(BeFalse(), "alert_resolution should not be set for metrics events")
		})

		// ========================================
		// Phase B: All metric deltas populated (DD-017 v2.5 Phase B)
		// ========================================

		It("UT-EM-AM-010: should set all Phase B metric_deltas fields when provided", func() {
			score := 0.6
			result := types.ComponentResult{
				Component: types.ComponentMetrics,
				Assessed:  true,
				Score:     &score,
				Details:   "2 of 4 metrics improved",
			}

			cpuBefore := 0.8
			cpuAfter := 0.3
			memBefore := 512.0
			memAfter := 256.0
			latBefore := 150.0
			latAfter := 80.0
			errBefore := 0.05
			errAfter := 0.01

			metricsData := audit.MetricsAssessedData{
				CPUBefore:          &cpuBefore,
				CPUAfter:           &cpuAfter,
				MemoryBefore:       &memBefore,
				MemoryAfter:        &memAfter,
				LatencyP95BeforeMs: &latBefore,
				LatencyP95AfterMs:  &latAfter,
				ErrorRateBefore:    &errBefore,
				ErrorRateAfter:     &errAfter,
			}

			err := mgr.RecordMetricsAssessed(ctx, ea, result, metricsData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			Expect(payload.MetricDeltas.Set).To(BeTrue())
			md := payload.MetricDeltas.Value

			// Phase A fields
			Expect(md.CPUBefore.Set).To(BeTrue())
			Expect(md.CPUBefore.Value).To(BeNumerically("~", 0.8, 0.001))
			Expect(md.CPUAfter.Set).To(BeTrue())
			Expect(md.CPUAfter.Value).To(BeNumerically("~", 0.3, 0.001))

			// Phase B fields
			Expect(md.MemoryBefore.Set).To(BeTrue())
			Expect(md.MemoryBefore.Value).To(BeNumerically("~", 512.0, 0.001))
			Expect(md.MemoryAfter.Set).To(BeTrue())
			Expect(md.MemoryAfter.Value).To(BeNumerically("~", 256.0, 0.001))

			Expect(md.LatencyP95BeforeMs.Set).To(BeTrue())
			Expect(md.LatencyP95BeforeMs.Value).To(BeNumerically("~", 150.0, 0.001))
			Expect(md.LatencyP95AfterMs.Set).To(BeTrue())
			Expect(md.LatencyP95AfterMs.Value).To(BeNumerically("~", 80.0, 0.001))

			Expect(md.ErrorRateBefore.Set).To(BeTrue())
			Expect(md.ErrorRateBefore.Value).To(BeNumerically("~", 0.05, 0.001))
			Expect(md.ErrorRateAfter.Set).To(BeTrue())
			Expect(md.ErrorRateAfter.Value).To(BeNumerically("~", 0.01, 0.001))
		})

		It("UT-EM-AM-011: should leave Phase B fields unset when only CPU available (partial query)", func() {
			score := 0.5
			result := types.ComponentResult{
				Component: types.ComponentMetrics,
				Assessed:  true,
				Score:     &score,
			}

			cpuBefore := 0.5
			cpuAfter := 0.3
			metricsData := audit.MetricsAssessedData{
				CPUBefore: &cpuBefore,
				CPUAfter:  &cpuAfter,
				// Phase B fields nil (queries failed or data unavailable)
			}

			err := mgr.RecordMetricsAssessed(ctx, ea, result, metricsData)
			Expect(err).ToNot(HaveOccurred())

			payload := extractPayload(spy.lastEvent)
			md := payload.MetricDeltas.Value

			// CPU set
			Expect(md.CPUBefore.Set).To(BeTrue())
			Expect(md.CPUAfter.Set).To(BeTrue())

			// Phase B unset
			Expect(md.MemoryBefore.Set).To(BeFalse())
			Expect(md.MemoryAfter.Set).To(BeFalse())
			Expect(md.LatencyP95BeforeMs.Set).To(BeFalse())
			Expect(md.LatencyP95AfterMs.Set).To(BeFalse())
			Expect(md.ErrorRateBefore.Set).To(BeFalse())
			Expect(md.ErrorRateAfter.Set).To(BeFalse())
		})
	})
})

// ============================================================================
// ASSESSMENT COMPLETED AUDIT PAYLOAD TESTS (ADR-EM-001, Batch 3)
// Verifies all 5 audit payload gaps are populated in RecordAssessmentCompleted.
// ============================================================================
var _ = Describe("Audit Manager — Assessment Completed Payload (ADR-EM-001, Batch 3)", func() {

	var (
		spy *spyAuditStore
		mgr *audit.Manager
		ctx context.Context
	)

	BeforeEach(func() {
		spy = &spyAuditStore{}
		mgr = audit.NewManager(spy, logr.Discard())
		ctx = context.Background()
	})

	// ========================================
	// UT-EM-AC-001: All 5 audit payload fields populated when data is available
	// ========================================
	It("UT-EM-AC-001: should populate alert_name, components_assessed, completed_at, resolution_time_seconds", func() {
		rrCreatedAt := metav1.NewTime(metav1.Now().Add(-30 * 60 * 1e9)) // 30 min ago
		completedAt := metav1.Now()

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-ac-001",
				Namespace: "production",
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-payment-restart",
				SignalName:              "PaymentHighCPU",
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "payment-api", Namespace: "production",
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{},
				},
				RemediationCreatedAt: &rrCreatedAt,
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
				CompletedAt:      &completedAt,
				Components: eav1.EAComponents{
					HealthAssessed:  true,
					HashComputed:    true,
					AlertAssessed:   true,
					MetricsAssessed: true,
				},
			},
		}

		err := mgr.RecordAssessmentCompleted(ctx, ea, eav1.AssessmentReasonFull)
		Expect(err).ToNot(HaveOccurred())

		payload := extractPayload(spy.lastEvent)

		// 1. alert_name (OBS-1: uses SignalName, not CorrelationID)
		Expect(payload.AlertName.Set).To(BeTrue(), "alert_name should be set")
		Expect(payload.AlertName.Value).To(Equal("PaymentHighCPU"))

		// 2. components_assessed
		Expect(payload.ComponentsAssessed).To(ConsistOf("health", "hash", "alert", "metrics"))

		// 3. completed_at
		Expect(payload.CompletedAt.Set).To(BeTrue(), "completed_at should be set")

		// 4. assessment_duration_seconds (OBS-2: renamed from resolution_time_seconds)
		Expect(payload.AssessmentDurationSeconds.Set).To(BeTrue(), "assessment_duration_seconds should be set")
		Expect(payload.AssessmentDurationSeconds.Value).To(BeNumerically(">", 0),
			"assessment_duration_seconds should be positive")
	})

	// ========================================
	// UT-EM-AC-002: Partial components_assessed when only some assessed
	// ========================================
	It("UT-EM-AC-002: should only include assessed components in components_assessed", func() {
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-ac-002",
				Namespace: "staging",
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-partial",
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "api", Namespace: "staging",
				},
				Config: eav1.EAConfig{StabilizationWindow: metav1.Duration{}},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonPartial,
				Components: eav1.EAComponents{
					HealthAssessed:  true,
					HashComputed:    true,
					AlertAssessed:   false, // Not assessed
					MetricsAssessed: false, // Not assessed
				},
			},
		}

		err := mgr.RecordAssessmentCompleted(ctx, ea, eav1.AssessmentReasonPartial)
		Expect(err).ToNot(HaveOccurred())

		payload := extractPayload(spy.lastEvent)
		Expect(payload.ComponentsAssessed).To(ConsistOf("health", "hash"))
	})

	// ========================================
	// UT-EM-AC-004: alert_name uses SignalName (not CorrelationID) when set (OBS-1)
	// ========================================
	It("UT-EM-AC-004: should use SignalName for alert_name when available", func() {
		completedAt := metav1.Now()

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-ac-004",
				Namespace: "production",
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-payment-restart",
				SignalName:              "HighCPUAlert",
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "payment-api", Namespace: "production",
				},
				Config: eav1.EAConfig{StabilizationWindow: metav1.Duration{}},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonFull,
				CompletedAt:      &completedAt,
				Components: eav1.EAComponents{
					HealthAssessed: true,
					HashComputed:   true,
				},
			},
		}

		err := mgr.RecordAssessmentCompleted(ctx, ea, eav1.AssessmentReasonFull)
		Expect(err).ToNot(HaveOccurred())

		payload := extractPayload(spy.lastEvent)

		// OBS-1: alert_name should be the actual alert name, not the correlationID
		Expect(payload.AlertName.Set).To(BeTrue(), "alert_name should be set")
		Expect(payload.AlertName.Value).To(Equal("HighCPUAlert"),
			"alert_name should use SignalName, not CorrelationID")
	})

	// ========================================
	// UT-EM-AC-003: resolution_time_seconds nil when remediationCreatedAt missing
	// ========================================
	It("UT-EM-AC-003: should not set resolution_time_seconds when remediationCreatedAt is nil", func() {
		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-ac-003",
				Namespace: "default",
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-no-created-at",
				RemediationRequestPhase: "Completed",
				TargetResource: eav1.TargetResource{
					Kind: "Deployment", Name: "app", Namespace: "default",
				},
				Config: eav1.EAConfig{StabilizationWindow: metav1.Duration{}},
				// RemediationCreatedAt intentionally nil
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:            eav1.PhaseCompleted,
				AssessmentReason: eav1.AssessmentReasonExpired,
				Components:       eav1.EAComponents{},
			},
		}

		err := mgr.RecordAssessmentCompleted(ctx, ea, eav1.AssessmentReasonExpired)
		Expect(err).ToNot(HaveOccurred())

		payload := extractPayload(spy.lastEvent)
		Expect(payload.AssessmentDurationSeconds.Set).To(BeFalse(),
			"assessment_duration_seconds should not be set when remediationCreatedAt is nil")
	})
})

// extractPayload extracts the EffectivenessAssessmentAuditPayload from an AuditEventRequest.
// The ogen discriminated union stores all EM event types in the same EffectivenessAssessmentAuditPayload field.
func extractPayload(event *ogenclient.AuditEventRequest) ogenclient.EffectivenessAssessmentAuditPayload {
	return event.EventData.EffectivenessAssessmentAuditPayload
}
