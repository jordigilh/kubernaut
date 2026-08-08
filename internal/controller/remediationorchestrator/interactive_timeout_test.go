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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// DD-INTERACTIVE-002: Dynamic Timeout Extension Tests
//
// These tests validate that the RO reconciler extends the Analyzing phase
// timeout when an interactive session is active on the associated AIAnalysis.
//
// Coverage target: >= 80% of checkPhaseTimeouts interactive extension paths.
// ========================================

var _ = Describe("DD-INTERACTIVE-002: Interactive Timeout Extension", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme)).To(Succeed())
	})

	makeReconciler := func(c client.Client, timeouts prodcontroller.TimeoutConfig) *prodcontroller.Reconciler {
		recorder := record.NewFakeRecorder(20)
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewReconciler(prodcontroller.ReconcilerDeps{
			Client:        c,
			APIReader:     c,
			Scheme:        scheme,
			AuditStore:    nil,
			Recorder:      recorder,
			Metrics:       m,
			Timeouts:      timeouts,
			RoutingEngine: nil,
		})
	}

	analyzingRR := func(name string, aiRefName string, analyzingStartedAgo time.Duration) *remediationv1.RemediationRequest {
		startTime := metav1.NewTime(time.Now().Add(-analyzingStartedAgo))
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         defaultFixture,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-analyzingStartedAgo - 5*time.Minute)),
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "abc123def456abc123def456abc123def456abc123def456abc123def456abc123de",
				SignalName:        "TestAlert",
				Severity:          "warning",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: defaultFixture,
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:       remediationv1.PhaseAnalyzing,
				AnalyzingStartTime: &startTime,
			},
		}
		if aiRefName != "" {
			rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: aiRefName, Namespace: defaultFixture}
		}
		return rr
	}

	aiWithInteractiveSession := func(name string, startedAt *metav1.Time, completedAt *metav1.Time) *aianalysisv1.AIAnalysis {
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultFixture},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: "Investigating",
			},
		}
		if startedAt != nil {
			ai.Status.InteractiveSession = &aianalysisv1.InteractiveSessionInfo{
				SessionID:  "sess-usr-001",
				ActingUser: "user-a@corp",
				StartedAt:  startedAt,
			}
			if completedAt != nil {
				ai.Status.InteractiveSession.CompletedAt = completedAt
			}
		}
		return ai
	}

	// UT-RO-703-001: Analyzing timeout uses default (10m) when AIAnalysisRef is nil
	// BR: BR-ORCH-028 (AC-028-5: per-phase defaults)
	Context("UT-RO-703-001: No AIAnalysisRef -- default timeout applies", func() {
		It("should timeout RR after default 10m when no AA reference exists", func() {
			rr := analyzingRR("rr-no-aa-ref", "", 11*time.Minute)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(rr).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch RR to check phase transition
			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseTimedOut),
				"RR with no AA ref should timeout at default 10m")
		})
	})

	// UT-RO-703-002: Analyzing timeout extends to maxAnalyzingTimeout when InteractiveSession active
	// BR: DD-INTERACTIVE-002 (dynamic timeout extension)
	Context("UT-RO-703-002: Active InteractiveSession -- extended timeout", func() {
		It("should NOT timeout at 11m when AA has active InteractiveSession", func() {
			startedAt := metav1.NewTime(time.Now().Add(-5 * time.Minute))
			ai := aiWithInteractiveSession("ai-active", &startedAt, nil)
			rr := analyzingRR("rr-active-session", ai.Name, 11*time.Minute)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).NotTo(Equal(remediationv1.PhaseTimedOut),
				"RR should NOT timeout when interactive session is active (extended to 45m)")
		})
	})

	// UT-RO-703-003: Analyzing timeout returns to default when InteractiveSession.CompletedAt is set
	// BR: DD-INTERACTIVE-002 (timeout returns to normal after disconnect)
	Context("UT-RO-703-003: Completed InteractiveSession -- default timeout resumes", func() {
		It("should timeout at default 10m when InteractiveSession is completed", func() {
			startedAt := metav1.NewTime(time.Now().Add(-20 * time.Minute))
			completedAt := metav1.NewTime(time.Now().Add(-15 * time.Minute))
			ai := aiWithInteractiveSession("ai-completed", &startedAt, &completedAt)
			rr := analyzingRR("rr-completed-session", ai.Name, 11*time.Minute)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseTimedOut),
				"RR should timeout at default when interactive session is completed")
		})
	})

	// UT-RO-2012-001: reproduces #2012 (main companion of #2011) -- AIAnalysis
	// reaches Completed (CompletedAt set on InteractiveSession in the same
	// instant) past the base 10m timeout but within the 45m extension window.
	// checkPhaseTimeouts recomputes the DD-INTERACTIVE-002 extension fresh on
	// every reconcile, so it is revoked in the exact same reconcile a
	// same-tick completion needs it most, discarding a valid, policy-
	// evaluated AutoApproved remediation decision (BR-ORCH-028; ties to
	// IR-4/SI-4 -- an incident's remediation decision must survive to the
	// point of use and phase-timeout logic must not misclassify a completed
	// analysis as timed out).
	Context("UT-RO-2012-001: AIAnalysis Completed races the extension revocation", func() {
		It("should NOT timeout when AIAnalysis is Completed even though CompletedAt just revoked the extension", func() {
			startedAt := metav1.NewTime(time.Now().Add(-9 * time.Minute))
			completedAt := metav1.NewTime(time.Now().Add(-1 * time.Second))
			ai := aiWithInteractiveSession("ai-race-completed", &startedAt, &completedAt)
			ai.Status.Phase = "Completed"
			ai.Status.ApprovalRequired = false
			ai.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					WorkflowID:      "wf-2012-completed",
					WorkflowName:    "wf-2012-completed",
					ActionType:      "RestartPod",
					Version:         "1.0.0",
					ExecutionBundle: "oci://example/bundle@sha256:deadbeef",
				},
				Confidence: 0.95,
			}
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				RemediationTarget: &aianalysisv1.RemediationTarget{
					Kind: "Deployment", Name: "target-completed", Namespace: "default",
				},
			}
			// Elapsed 10m47s: past base 10m timeout, well within 45m extension.
			rr := analyzingRR("rr-race-completed", ai.Name, 10*time.Minute+47*time.Second)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).NotTo(Equal(remediationv1.PhaseTimedOut),
				"#2012: a Completed AIAnalysis result must not be discarded by the same-reconcile timeout race")
		})
	})

	// UT-RO-2012-002: symmetric case for a Failed AIAnalysis -- AnalyzingHandler.Handle
	// is equally prepared to finalize a Failed AIAnalysis, so the same
	// same-tick race must not discard a Failed result into TimedOut either
	// (BR-ORCH-028).
	Context("UT-RO-2012-002: AIAnalysis Failed races the extension revocation", func() {
		It("should NOT timeout when AIAnalysis is Failed even though CompletedAt just revoked the extension", func() {
			startedAt := metav1.NewTime(time.Now().Add(-9 * time.Minute))
			completedAt := metav1.NewTime(time.Now().Add(-1 * time.Second))
			ai := aiWithInteractiveSession("ai-race-failed", &startedAt, &completedAt)
			ai.Status.Phase = "Failed"
			ai.Status.Message = "LLM investigation exhausted retries"
			// Elapsed 10m47s: past base 10m timeout, well within 45m extension.
			rr := analyzingRR("rr-race-failed", ai.Name, 10*time.Minute+47*time.Second)
			// handleFailed's manual-review notification path sets an owner
			// reference on the RR, which requires a non-empty UID (the fake
			// client does not auto-assign one for pre-supplied objects).
			rr.UID = types.UID("rr-race-failed-uid")

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).NotTo(Equal(remediationv1.PhaseTimedOut),
				"#2012: a Failed AIAnalysis result must not be discarded by the same-reconcile timeout race")
		})
	})

	// UT-RO-703-004: Per-RR override takes precedence when larger than MaxAnalyzing
	// BR: BR-ORCH-028 (AC-028-5: per-RR overrides)
	Context("UT-RO-703-004: Per-RR override larger than MaxAnalyzing wins", func() {
		It("should use per-RR override (60m) even when interactive session is active", func() {
			startedAt := metav1.NewTime(time.Now().Add(-5 * time.Minute))
			ai := aiWithInteractiveSession("ai-override", &startedAt, nil)
			rr := analyzingRR("rr-override", ai.Name, 50*time.Minute)
			// Per-RR override: 60 minutes (larger than MaxAnalyzing default 45m)
			analyzingOverride := metav1.Duration{Duration: 60 * time.Minute}
			rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{Analyzing: &analyzingOverride}

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).NotTo(Equal(remediationv1.PhaseTimedOut),
				"Per-RR override (60m) should prevent timeout at 50m")
		})
	})

	// UT-RO-703-005: Processing/Executing timeouts unaffected by interactive session
	// BR: DD-INTERACTIVE-002 (scope: Analyzing only)
	Context("UT-RO-703-005: Non-Analyzing phases unaffected", func() {
		It("should timeout Processing phase normally even when AA has interactive session", func() {
			startedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			ai := aiWithInteractiveSession("ai-processing", &startedAt, nil)

			processingStart := metav1.NewTime(time.Now().Add(-6 * time.Minute))
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "rr-processing-timeout",
					Namespace:         defaultFixture,
					CreationTimestamp: metav1.NewTime(time.Now().Add(-20 * time.Minute)),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "proc123proc123proc123proc123proc123proc123proc123proc123proc123pr",
					SignalName:        "ProcessingAlert",
					Severity:          "warning",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: defaultFixture,
					},
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase:        remediationv1.PhaseProcessing,
					ProcessingStartTime: &processingStart,
					AIAnalysisRef:       &corev1.ObjectReference{Name: ai.Name, Namespace: defaultFixture},
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseTimedOut),
				"Processing phase should timeout normally regardless of interactive session")
		})
	})

	// UT-RO-703-006: MaxAnalyzing is configurable via TimeoutConfig
	// BR: BR-ORCH-028 (configurability)
	Context("UT-RO-703-006: MaxAnalyzing configurable", func() {
		It("should use configured MaxAnalyzing (30m) instead of default 45m", func() {
			startedAt := metav1.NewTime(time.Now().Add(-5 * time.Minute))
			ai := aiWithInteractiveSession("ai-custom-max", &startedAt, nil)
			rr := analyzingRR("rr-custom-max", ai.Name, 35*time.Minute)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{
				MaxAnalyzing: 30 * time.Minute,
			})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseTimedOut),
				"Should timeout at configured MaxAnalyzing (30m) when elapsed is 35m")
		})
	})

	// UT-RO-703-007: AA fetch failure (not found) falls back to default timeout
	// BR: BR-ORCH-028 (graceful degradation)
	Context("UT-RO-703-007: AA not found -- graceful fallback", func() {
		It("should use default timeout when AA referenced but not found", func() {
			rr := analyzingRR("rr-aa-missing", "ai-nonexistent", 11*time.Minute)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(rr).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseTimedOut),
				"Missing AA should fall back to default timeout (10m)")
		})
	})

	// UT-RO-703-008: AA fetch transient error falls back to default timeout
	// BR: BR-ORCH-028 (graceful degradation)
	Context("UT-RO-703-008: AA fetch error -- graceful fallback", func() {
		It("should use default timeout when AA fetch returns transient error", func() {
			rr := analyzingRR("rr-aa-error", "ai-error-target", 11*time.Minute)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(rr).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if key.Name == "ai-error-target" {
							return apierrors.NewInternalError(fmt.Errorf("transient API server error"))
						}
						return cl.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseTimedOut),
				"Transient AA fetch error should fall back to default timeout")
		})
	})

	// UT-RO-703-009: MaxAnalyzing defaults to 45m when not explicitly configured
	// BR: DD-INTERACTIVE-002 (safe defaults)
	Context("UT-RO-703-009: MaxAnalyzing default value", func() {
		It("should not timeout at 40m with active session (default MaxAnalyzing is 45m)", func() {
			startedAt := metav1.NewTime(time.Now().Add(-5 * time.Minute))
			ai := aiWithInteractiveSession("ai-default-max", &startedAt, nil)
			rr := analyzingRR("rr-default-max", ai.Name, 40*time.Minute)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(rr, ai).
				Build()

			// Empty TimeoutConfig -- MaxAnalyzing should default to 45m
			reconciler := makeReconciler(c, prodcontroller.TimeoutConfig{})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: rr.Name, Namespace: rr.Namespace}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).NotTo(Equal(remediationv1.PhaseTimedOut),
				"Default MaxAnalyzing (45m) should prevent timeout at 40m elapsed")
		})
	})
})
