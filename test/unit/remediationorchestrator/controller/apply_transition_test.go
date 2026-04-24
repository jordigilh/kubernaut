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

package controller

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

var _ = Describe("Issue #666: ApplyTransition (BR-ORCH-025)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	// ========================================
	// TransitionNone result mapping
	// ========================================
	Describe("TransitionNone result mapping", func() {

		It("UT-AT-001: NoOp intent produces empty ctrl.Result", func() {
			rr := newRemediationRequest("test-noop", "default", remediationv1.PhasePending)
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.NoOp("already terminal")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("UT-AT-002: Requeue intent produces ctrl.Result with RequeueAfter", func() {
			rr := newRemediationRequest("test-requeue", "default", remediationv1.PhaseProcessing)
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.Requeue(5*time.Second, "SP in progress")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{RequeueAfter: 5 * time.Second}))
		})

		It("UT-AT-003: RequeueNow intent produces ctrl.Result with Requeue=true", func() {
			rr := newRemediationRequest("test-requeue-now", "default", remediationv1.PhasePending)
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.RequeueNow("event-based block cleared")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{Requeue: true}))
		})
	})

	// ========================================
	// Validation errors
	// ========================================
	Describe("Validation errors", func() {

		It("UT-AT-004: Advance without TargetPhase returns validation error", func() {
			rr := newRemediationRequest("test-invalid", "default", remediationv1.PhasePending)
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.TransitionIntent{Type: phase.TransitionAdvance, Reason: "missing target"}
			_, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid transition intent"))
		})

		It("UT-AT-005: Unknown TransitionType returns error", func() {
			rr := newRemediationRequest("test-unknown", "default", remediationv1.PhasePending)
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.TransitionIntent{Type: phase.TransitionType(99), Reason: "invalid"}
			_, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid transition intent"))
		})
	})

	// ========================================
	// Dispatch to transition methods
	// ========================================
	Describe("Dispatch to transition methods", func() {

		It("UT-AT-006: Advance dispatches to transitionPhase, updating status", func() {
			rr := newRemediationRequest("test-advance", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.Advance(phase.Processing, "SP created successfully")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(rr.Status.OverallPhase).To(Equal(phase.Processing))
		})

		It("UT-AT-007: Failed dispatches to transitionToFailed, setting FailurePhase", func() {
			rr := newRemediationRequest("test-failed", "default", remediationv1.PhaseProcessing)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.Fail(remediationv1.FailurePhaseSignalProcessing, errors.New("SP timeout"), "SP creation failed")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(rr.Status.OverallPhase).To(Equal(phase.Failed))
			Expect(rr.Status.FailurePhase).To(HaveValue(Equal(remediationv1.FailurePhaseSignalProcessing)))
		})

		It("UT-AT-008: Verifying dispatches to transitionToVerifying", func() {
			rr := newRemediationRequest("test-verifying", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.Verify("remediationSucceeded", "WFE completed")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(rr.Status.OverallPhase).To(Equal(phase.Verifying))
		})

		It("UT-AT-009: InheritedCompleted dispatches to transitionToInheritedCompleted", func() {
			rr := newRemediationRequest("test-inherit-complete", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.InheritComplete("original-rr", "RemediationRequest", "inherited from original")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(rr.Status.OverallPhase).To(Equal(phase.Completed))
		})

		It("UT-AT-010: InheritedFailed dispatches to transitionToInheritedFailed", func() {
			rr := newRemediationRequest("test-inherit-fail", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationRequest{}).WithObjects(rr).Build()
			r, _ := newCharReconciler(c, c, scheme, &MockRoutingEngine{})

			intent := phase.InheritFail(errors.New("original failed"), "original-wfe", "WorkflowExecution", "inherited failure")
			result, err := r.ApplyTransition(ctx, rr, intent)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(rr.Status.OverallPhase).To(Equal(phase.Failed))
		})
	})

	// ========================================
	// ToBlockingCondition mapping
	// ========================================
	Describe("ToBlockingCondition mapping", func() {

		It("UT-AT-011: nil BlockMeta returns nil BlockingCondition", func() {
			bc := prodcontroller.ToBlockingCondition(nil)
			Expect(bc).To(BeNil())
		})

		It("UT-AT-012: maps all BlockMeta fields to routing.BlockingCondition", func() {
			blockedUntil := time.Now().Add(5 * time.Minute)
			meta := &phase.BlockMeta{
				Reason:                    "ConsecutiveFailures",
				Message:                   "3 consecutive failures",
				RequeueAfter:              30 * time.Second,
				BlockedUntil:              &blockedUntil,
				BlockingWorkflowExecution: "wfe-abc123",
				DuplicateOf:               "original-rr-1",
				FromPhase:                 phase.Pending,
				WorkflowID:                "wf-001",
			}

			bc := prodcontroller.ToBlockingCondition(meta)

			Expect(bc.Blocked).To(BeTrue())
			Expect(bc.Reason).To(Equal("ConsecutiveFailures"))
			Expect(bc.Message).To(Equal("3 consecutive failures"))
			Expect(bc.RequeueAfter).To(Equal(30 * time.Second))
			Expect(bc.BlockedUntil).To(Equal(&blockedUntil))
			Expect(bc.BlockingWorkflowExecution).To(Equal("wfe-abc123"))
			Expect(bc.DuplicateOf).To(Equal("original-rr-1"))
		})
	})
})
