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

package controller_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	testNs = "test-ns"
)

// ============================================================================
// AUDIT COVERAGE GAP TESTS (BR-AUDIT-005, SOC2 CC8.1, FedRAMP AU-2/AU-3)
//
// Preflight (2026-07-01) found that emitEACreatedAudit, emitVerificationCompletedAudit,
// emitVerificationTimedOutAudit and emitCompletionAudit had 15-36% line coverage.
// Every existing caller wires AuditStore: nil (see ea_creation_test.go,
// verifying_phase_test.go, cascade_terminal_test.go, apply_transition_test.go,
// characterization_test.go), so only the "auditStore == nil" guard clause was
// exercised — the actual event-construction business logic (field extraction
// from the RR, propagation-delay breakdown, EA name/duration propagation) was
// never verified. These tests close that gap ahead of the Phase 3 file split
// so regressions during the split are caught by tests, not by production.
// ============================================================================
var _ = Describe("BR-AUDIT-005: Audit Coverage Gap Closure (SOC2 CC8.1, FedRAMP AU-2/AU-3)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// emitEACreatedAudit (DD-AUDIT-003: orchestrator.ea.created, Issue #277)
	// ========================================
	Context("emitEACreatedAudit", func() {
		It("AE-COV-001: emits orchestrator.ea.created with isGitopsManaged=false, isCrd=false for a plain Deployment target", func() {
			scheme := setupScheme()
			rrName := "rr-cov-ea-001"
			namespace := testNs
			weName := "we-" + rrName

			rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", "", weName)
			we := newWorkflowExecutionCompleted(weName, namespace, rrName)

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, we).
				WithStatusSubresource(rr).
				Build()

			roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			recorder := record.NewFakeRecorder(20)
			mockAuditStore := &MockAuditStore{}
			eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, 30*time.Second)
			reconciler := controller.NewReconciler(controller.ReconcilerDeps{
				Client:        k8sClient,
				APIReader:     k8sClient,
				Scheme:        scheme,
				AuditStore:    mockAuditStore,
				Recorder:      recorder,
				Metrics:       roMetrics,
				Timeouts:      controller.TimeoutConfig{},
				RoutingEngine: &MockRoutingEngine{},
			}, eaCreator)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			events := mockAuditStore.GetEventsByType(roaudit.EventTypeEACreated)
			Expect(events).To(HaveLen(1), "orchestrator.ea.created must be emitted exactly once (BR-AUDIT-005)")

			payload := events[0].EventData.RemediationOrchestratorAuditPayload
			Expect(payload.RrName).To(Equal(rrName))
			Expect(payload.EaName.Value).To(Equal("ea-" + rrName))
			Expect(events[0].CorrelationID).To(Equal(rrName), "DD-AUDIT-CORRELATION-002: correlation ID must be rr.Name")

			Expect(payload.IsGitopsManaged.IsSet()).To(BeTrue())
			Expect(payload.IsGitopsManaged.Value).To(BeFalse(), "plain Deployment target is not GitOps-managed")
			Expect(payload.IsCrd.IsSet()).To(BeTrue())
			Expect(payload.IsCrd.Value).To(BeFalse(), "Deployment is a built-in kind, not a CRD")
			Expect(payload.GitopsSyncDelay.IsSet()).To(BeFalse(), "no GitOps delay expected for a non-GitOps target")
			Expect(payload.OperatorReconcileDelay.IsSet()).To(BeFalse(), "no operator reconcile delay expected for a built-in kind")
		})

		It("AE-COV-002: emits orchestrator.ea.created with isGitopsManaged=true and GitOpsSyncDelay populated when RCA detected GitOps management", func() {
			scheme := setupScheme()
			rrName := "rr-cov-ea-002"
			namespace := testNs
			aiName := "ai-" + rrName
			weName := "we-" + rrName

			rr := newRemediationRequestWithChildRefs(rrName, namespace, remediationv1.PhaseExecuting, "", aiName, weName)
			we := newWorkflowExecutionCompleted(weName, namespace, rrName)

			ai := newAIAnalysisCompleted(aiName, namespace, rrName, 0.9, "restart-pod")
			ai.Status.PostRCAContext = &aianalysisv1.PostRCAContext{
				DetectedLabels: &aianalysisv1.DetectedLabels{GitOpsManaged: true},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, we, ai).
				WithStatusSubresource(rr).
				Build()

			roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			recorder := record.NewFakeRecorder(20)
			mockAuditStore := &MockAuditStore{}
			eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, 30*time.Second)
			reconciler := controller.NewReconciler(controller.ReconcilerDeps{
				Client:        k8sClient,
				APIReader:     k8sClient,
				Scheme:        scheme,
				AuditStore:    mockAuditStore,
				Recorder:      recorder,
				Metrics:       roMetrics,
				Timeouts:      controller.TimeoutConfig{},
				RoutingEngine: &MockRoutingEngine{},
			}, eaCreator)
			reconciler.SetAsyncPropagation(roconfig.AsyncPropagationConfig{
				GitOpsSyncDelay:        3 * time.Minute,
				OperatorReconcileDelay: 1 * time.Minute,
			})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			events := mockAuditStore.GetEventsByType(roaudit.EventTypeEACreated)
			Expect(events).To(HaveLen(1))

			payload := events[0].EventData.RemediationOrchestratorAuditPayload
			Expect(payload.IsGitopsManaged.Value).To(BeTrue(),
				"BR-RO-103.2: RCA-detected GitOps management must propagate into the audit event")
			Expect(payload.GitopsSyncDelay.IsSet()).To(BeTrue())
			Expect(payload.GitopsSyncDelay.Value).To(Equal((3 * time.Minute).String()))
			Expect(payload.HashComputeDelay.IsSet()).To(BeTrue(),
				"GitOps-managed target must set a non-zero hash compute delay (async propagation)")
		})
	})

	// ========================================
	// emitVerificationCompletedAudit (#280: orchestrator.lifecycle.verification_completed)
	// ========================================
	Context("emitVerificationCompletedAudit", func() {
		It("AE-COV-003: emits verification_completed with EA name and duration when Verifying -> Completed", func() {
			scheme := setupScheme()
			rrName := "rr-cov-verify-001"
			namespace := testNs

			startTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
			deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: namespace},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseVerifying,
					StartTime:    &startTime,
					EffectivenessAssessmentRef: &corev1.ObjectReference{
						Kind:      "EffectivenessAssessment",
						Name:      "ea-" + rrName,
						Namespace: namespace,
					},
					VerificationDeadline: &deadline,
				},
			}
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-" + rrName, Namespace: namespace},
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
					Message:          "Assessment completed: Full",
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(rr, ea).
				Build()

			roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			recorder := record.NewFakeRecorder(20)
			mockAuditStore := &MockAuditStore{}
			eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, 30*time.Second)
			reconciler := controller.NewReconciler(controller.ReconcilerDeps{
				Client:        k8sClient,
				APIReader:     k8sClient,
				Scheme:        scheme,
				AuditStore:    mockAuditStore,
				Recorder:      recorder,
				Metrics:       roMetrics,
				Timeouts:      controller.TimeoutConfig{},
				RoutingEngine: &MockRoutingEngine{},
			}, eaCreator)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			events := mockAuditStore.GetEventsByType(roaudit.EventTypeLifecycleVerificationCompleted)
			Expect(events).To(HaveLen(1),
				"orchestrator.lifecycle.verification_completed must be emitted on EA terminal (#280, BR-AUDIT-005)")

			event := events[0]
			Expect(event.CorrelationID).To(Equal(rrName))
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.EaName.Value).To(Equal("ea-"+rrName),
				"EA name must be read from RR.Status.EffectivenessAssessmentRef")
			Expect(payload.DurationMs.IsSet()).To(BeTrue())
			Expect(payload.DurationMs.Value).To(BeNumerically(">=", 0))
		})
	})

	// ========================================
	// emitVerificationTimedOutAudit (#280: orchestrator.lifecycle.verification_timed_out)
	// AND emitCompletionAudit outcome=VerificationTimedOut (shared code path)
	// ========================================
	Context("emitVerificationTimedOutAudit", func() {
		It("AE-COV-004: emits verification_timed_out AND lifecycle.completed(VerificationTimedOut) when VerificationDeadline expires", func() {
			scheme := setupScheme()
			rrName := "rr-cov-verify-timeout-001"
			namespace := testNs

			startTime := metav1.NewTime(time.Now().Add(-20 * time.Minute))
			expiredDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: namespace},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseVerifying,
					StartTime:    &startTime,
					EffectivenessAssessmentRef: &corev1.ObjectReference{
						Kind:      "EffectivenessAssessment",
						Name:      "ea-" + rrName,
						Namespace: namespace,
					},
					VerificationDeadline: &expiredDeadline,
				},
			}
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-" + rrName, Namespace: namespace},
				Status: eav1.EffectivenessAssessmentStatus{
					Phase: eav1.PhaseAssessing, // still in progress — deadline expiry drives the timeout, not EA completion
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(rr, ea).
				Build()

			roMetrics := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			recorder := record.NewFakeRecorder(20)
			mockAuditStore := &MockAuditStore{}
			eaCreator := creator.NewEffectivenessAssessmentCreator(k8sClient, scheme, roMetrics, recorder, 30*time.Second)
			reconciler := controller.NewReconciler(controller.ReconcilerDeps{
				Client:        k8sClient,
				APIReader:     k8sClient,
				Scheme:        scheme,
				AuditStore:    mockAuditStore,
				Recorder:      recorder,
				Metrics:       roMetrics,
				Timeouts:      controller.TimeoutConfig{},
				RoutingEngine: &MockRoutingEngine{},
			}, eaCreator)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: rrName, Namespace: namespace},
			})
			Expect(err).ToNot(HaveOccurred())

			fetchedRR := &remediationv1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, fetchedRR)).To(Succeed())
			Expect(fetchedRR.Status.Outcome).To(Equal("VerificationTimedOut"))

			timedOutEvents := mockAuditStore.GetEventsByType(roaudit.EventTypeLifecycleVerificationTimedOut)
			Expect(timedOutEvents).To(HaveLen(1),
				"orchestrator.lifecycle.verification_timed_out must be emitted on deadline expiry (#280, BR-AUDIT-005)")
			Expect(timedOutEvents[0].CorrelationID).To(Equal(rrName))
			timedOutPayload := timedOutEvents[0].EventData.RemediationOrchestratorAuditPayload
			Expect(timedOutPayload.EaName.Value).To(Equal("ea-" + rrName))

			completedEvents := mockAuditStore.GetEventsByType(roaudit.EventTypeLifecycleCompleted)
			Expect(completedEvents).To(HaveLen(1),
				"emitCompletionAudit must also fire with the VerificationTimedOut outcome (shared code path)")
			completedPayload := completedEvents[0].EventData.RemediationOrchestratorAuditPayload
			Expect(completedPayload.CrdOutcome.Value).To(Equal("VerificationTimedOut"))
			Expect(completedPayload.DurationMs.IsSet()).To(BeTrue())
		})
	})

	// ========================================
	// emitCompletionAudit (DD-AUDIT-003: orchestrator.lifecycle.completed)
	// InheritedCompleted and DryRun outcomes — reached via ApplyTransition, not the
	// full Reconcile loop, mirroring apply_transition_test.go's dispatch-level tests.
	// ========================================
	Context("emitCompletionAudit", func() {
		It("AE-COV-005: emits lifecycle.completed with crd_outcome=InheritedCompleted on inherited completion", func() {
			scheme := setupScheme()
			rrName := "rr-cov-inherit-001"
			namespace := testNs
			startTime := metav1.NewTime(time.Now().Add(-90 * time.Second))
			rr := newRemediationRequest(rrName, namespace, remediationv1.PhasePending)
			rr.Status.StartTime = &startTime

			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			recorder := record.NewFakeRecorder(20)
			mockAuditStore := &MockAuditStore{}
			reconciler := controller.NewReconciler(controller.ReconcilerDeps{
				Client:        c,
				APIReader:     c,
				Scheme:        scheme,
				AuditStore:    mockAuditStore,
				Recorder:      recorder,
				Metrics:       metrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      controller.TimeoutConfig{},
				RoutingEngine: &MockRoutingEngine{},
			})

			intent := phase.InheritComplete("original-rr", "RemediationRequest", "inherited from original")
			_, err := reconciler.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())

			events := mockAuditStore.GetEventsByType(roaudit.EventTypeLifecycleCompleted)
			Expect(events).To(HaveLen(1),
				"emitCompletionAudit must fire for inherited completion (BR-AUDIT-005 reconstruction)")
			payload := events[0].EventData.RemediationOrchestratorAuditPayload
			Expect(payload.CrdOutcome.Value).To(Equal("InheritedCompleted"))
			Expect(events[0].CorrelationID).To(Equal(rrName))
			Expect(payload.DurationMs.IsSet()).To(BeTrue())
		})

		It("AE-COV-006: emits lifecycle.completed with crd_outcome=DryRun on dry-run completion", func() {
			scheme := setupScheme()
			rrName := "rr-cov-dryrun-001"
			namespace := testNs
			startTime := metav1.NewTime(time.Now().Add(-45 * time.Second))
			rr := newRemediationRequest(rrName, namespace, remediationv1.PhaseAnalyzing)
			rr.Status.StartTime = &startTime

			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			recorder := record.NewFakeRecorder(20)
			mockAuditStore := &MockAuditStore{}
			reconciler := controller.NewReconciler(controller.ReconcilerDeps{
				Client:        c,
				APIReader:     c,
				Scheme:        scheme,
				AuditStore:    mockAuditStore,
				Recorder:      recorder,
				Metrics:       metrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				Timeouts:      controller.TimeoutConfig{},
				RoutingEngine: &MockRoutingEngine{},
			})
			reconciler.SetDryRun(true, 1*time.Hour)

			intent := phase.CompleteWithoutVerification("dry-run mode enabled")
			_, err := reconciler.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())

			events := mockAuditStore.GetEventsByType(roaudit.EventTypeLifecycleCompleted)
			Expect(events).To(HaveLen(1),
				"emitCompletionAudit must fire for dry-run completion (#712, #736, BR-AUDIT-005)")
			payload := events[0].EventData.RemediationOrchestratorAuditPayload
			Expect(payload.CrdOutcome.Value).To(Equal("DryRun"))
			Expect(events[0].CorrelationID).To(Equal(rrName))
			Expect(payload.DurationMs.IsSet()).To(BeTrue())
		})
	})
})
