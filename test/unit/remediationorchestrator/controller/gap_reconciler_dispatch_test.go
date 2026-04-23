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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// ════════════════════════════════════════════════════════════════════════════
// BR-ORCH-036 Gap Remediation: Reconciler Dispatch Tests (PR A)
// ════════════════════════════════════════════════════════════════════════════
//
// Business Requirements: BR-ORCH-036 (manual review notification)
// Issues: #805 (GAP-1), #807 (GAP-3)
//
// GAP-1: NeedsHumanReview with no selected workflow → ManualReview NR
// GAP-3: WFE PhaseFailed → ManualReview NR before transitionToFailed
// ════════════════════════════════════════════════════════════════════════════

var _ = Describe("BR-ORCH-036 GAP-1: NeedsHumanReview dispatch (#805)", func() {

	It("UT-RO-805-001: should create ManualReview NR when AIAnalysis has NeedsHumanReview and no SelectedWorkflow", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-805-001"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysis("ai-"+rrName, "default", rrName, aianalysisv1.PhaseCompleted)
		now := metav1.Now()
		ai.Status.CompletedAt = &now
		ai.Status.NeedsHumanReview = true
		ai.Status.HumanReviewReason = "no_matching_workflows"
		ai.Status.Reason = "WorkflowResolutionFailed"
		ai.Status.SubReason = "NoMatchingWorkflows"
		ai.Status.Message = "No workflows matched the search criteria"
		ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			Summary:  "Test root cause",
			Severity: "high",
			RemediationTarget: &aianalysisv1.RemediationTarget{
				Kind:      "Deployment",
				Name:      "test-deployment",
				Namespace: "default",
			},
		}

		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-manual-review-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "ManualReview NR should exist for NeedsHumanReview with no workflow")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))

		updatedRR := &remediationv1.RemediationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: "default"}, updatedRR)
		Expect(err).ToNot(HaveOccurred())
		Expect(updatedRR.Status.Outcome).To(Equal("ManualReviewRequired"))
		Expect(updatedRR.Status.RequiresManualReview).To(BeTrue())
	})

	It("UT-RO-805-002: should NOT create ManualReview NR when AIAnalysis has SelectedWorkflow even if NeedsHumanReview", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-805-002"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseAnalyzing, "sp-"+rrName, "ai-"+rrName, "")
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		ai.Status.NeedsHumanReview = true

		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nrList := &notificationv1.NotificationRequestList{}
		err = fakeClient.List(ctx, nrList)
		Expect(err).ToNot(HaveOccurred())
		for _, nr := range nrList.Items {
			Expect(nr.Name).ToNot(HavePrefix("nr-manual-review-"),
				"Should NOT create ManualReview NR when SelectedWorkflow exists, found: %s", nr.Name)
		}
	})
})

var _ = Describe("BR-ORCH-036 GAP-3: WFE PhaseFailed ManualReview NR (#807)", func() {

	It("UT-RO-807-001: should create ManualReview NR when WorkflowExecution fails", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-807-001"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseExecuting, "sp-"+rrName, "ai-"+rrName, "we-"+rrName)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}
		execStart := metav1.Now()
		rr.Status.ExecutingStartTime = &execStart

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)
		we := newWorkflowExecutionFailed("we-"+rrName, "default", rrName, "Pipeline run timed out")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, we).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "nr-manual-review-" + rrName,
			Namespace: "default",
		}, nr)
		Expect(err).ToNot(HaveOccurred(), "ManualReview NR should exist after WFE failure")
		Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeManualReview))
		Expect(nr.Spec.ReviewSource).To(Equal(notificationv1.ReviewSourceWorkflowExecution))
		Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical))
	})

	It("UT-RO-807-002: should emit NotificationCreated event when WFE fails", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-807-002"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseExecuting, "sp-"+rrName, "ai-"+rrName, "we-"+rrName)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}
		execStart := metav1.Now()
		rr.Status.ExecutingStartTime = &execStart

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)
		we := newWorkflowExecutionFailed("we-"+rrName, "default", rrName, "Pipeline failed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, we).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		evts := drainEvents(recorder)
		Expect(containsEvent(evts, events.EventReasonNotificationCreated)).To(BeTrue(),
			"Expected NotificationCreated event for WFE failure NR, got: %v", evts)
	})

	It("UT-RO-807-003: should NOT duplicate NR on re-reconcile of failed WFE", func() {
		ctx := context.Background()
		scheme := setupScheme()
		recorder := record.NewFakeRecorder(20)

		rrName := "test-rr-807-003"
		rr := newRemediationRequestWithChildRefs(rrName, "default",
			remediationv1.PhaseExecuting, "sp-"+rrName, "ai-"+rrName, "we-"+rrName)
		rr.Status.StartTime = &metav1.Time{Time: time.Now()}
		execStart := metav1.Now()
		rr.Status.ExecutingStartTime = &execStart

		ai := newAIAnalysisCompleted("ai-"+rrName, "default", rrName, 0.95, "restart-pod")
		sp := newSignalProcessingCompleted("sp-"+rrName, "default", rrName)
		we := newWorkflowExecutionFailed("we-"+rrName, "default", rrName, "Pipeline failed")

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr, ai, sp, we).
			WithStatusSubresource(&remediationv1.RemediationRequest{}).
			Build()

		reconciler := prodcontroller.NewReconciler(
			fakeClient, fakeClient, scheme,
			nil, recorder,
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			prodcontroller.TimeoutConfig{},
			&MockRoutingEngine{},
		)

		// First reconcile
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		drainEvents(recorder)

		// Second reconcile
		_, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: rrName, Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		nrList := &notificationv1.NotificationRequestList{}
		err = fakeClient.List(ctx, nrList)
		Expect(err).ToNot(HaveOccurred())
		manualReviewCount := 0
		for _, nr := range nrList.Items {
			if nr.Name == "nr-manual-review-"+rrName {
				manualReviewCount++
			}
		}
		Expect(manualReviewCount).To(Equal(1), "Should have exactly 1 ManualReview NR after re-reconcile")

		evts := drainEvents(recorder)
		notifCreatedCount := 0
		for _, evt := range evts {
			if containsEvent([]string{evt}, events.EventReasonNotificationCreated) {
				notifCreatedCount++
			}
		}
		Expect(notifCreatedCount).To(Equal(0),
			"Should NOT emit NotificationCreated on re-reconcile, got events: %v", evts)
	})
})
