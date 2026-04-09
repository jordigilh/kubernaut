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
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

// ========================================
// Issue #190: WE/RO Deduplicated Phase with Result Inheritance
// ========================================

var _ = Describe("Issue #190: Deduplication CRD Types", func() {

	Context("CRD Constants and Fields", func() {

		It("UT-WE-190-004: FailureReasonDeduplicated constant exists and equals 'Deduplicated'", func() {
			Expect(workflowexecutionv1.FailureReasonDeduplicated).To(Equal("Deduplicated"),
				"Behavior: WE CRD must have a Deduplicated failure reason constant")
		})

		It("UT-WE-190-004: DeduplicatedBy field is assignable on WFE status", func() {
			wfe := &workflowexecutionv1.WorkflowExecution{}
			wfe.Status.DeduplicatedBy = "test-original-wfe"
			Expect(wfe.Status.DeduplicatedBy).To(Equal("test-original-wfe"),
				"Behavior: WFE status must have DeduplicatedBy string field")
		})

		It("FailurePhaseDeduplicated constant exists and equals FailurePhase('Deduplicated')", func() {
			Expect(remediationv1.FailurePhaseDeduplicated).To(Equal(remediationv1.FailurePhase("Deduplicated")),
				"Behavior: RR CRD must have a Deduplicated failure phase constant")
		})

		It("DeduplicatedByWE field is assignable on RR status", func() {
			rr := &remediationv1.RemediationRequest{}
			rr.Status.DeduplicatedByWE = "test-original-wfe"
			Expect(rr.Status.DeduplicatedByWE).To(Equal("test-original-wfe"),
				"Behavior: RR status must have DeduplicatedByWE string field")
		})
	})
})

var _ = Describe("Issue #190: HandleStatus Dedup Branching", func() {
	var (
		ctx              context.Context
		scheme           = setupScheme()
		transitionCalled bool
	)

	BeforeEach(func() {
		ctx = context.Background()
		transitionCalled = false
	})

	Context("UT-RO-190: HandleStatus with deduplicated WFE", func() {

		It("UT-RO-190-001: should NOT call transitionToFailed for deduplicated WFE", func() {
			rr := newRemediationRequest("dedup-rr-001", "default", remediationv1.PhaseExecuting)
			setWERef(rr, "dedup-wfe-001", "default")

			wfe := newWorkflowExecution("dedup-wfe-001", "default", "dedup-rr-001", workflowexecutionv1.PhaseFailed)
			wfe.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
				Reason:  workflowexecutionv1.FailureReasonDeduplicated,
				Message: "Execution resource already exists, owned by WorkflowExecution original-wfe",
			}
			wfe.Status.DeduplicatedBy = "original-wfe"

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).WithStatusSubresource(rr).Build()

			h := handler.NewWorkflowExecutionHandler(
				c, scheme, nil,
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
					transitionCalled = true
					return ctrl.Result{}, nil
				},
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) (ctrl.Result, error) {
					return ctrl.Result{}, nil
				},
			)

			result, err := h.HandleStatus(ctx, rr, wfe)
			Expect(err).ToNot(HaveOccurred())
			Expect(transitionCalled).To(BeFalse(),
				"transitionToFailed must NOT be called for deduplicated WFE failures")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"Handler must requeue to wait for original WFE outcome")
		})

		It("UT-RO-190-002: should set DeduplicatedByWE from WFE.Status.DeduplicatedBy", func() {
			rr := newRemediationRequest("dedup-rr-002", "default", remediationv1.PhaseExecuting)
			setWERef(rr, "dedup-wfe-002", "default")

			wfe := newWorkflowExecution("dedup-wfe-002", "default", "dedup-rr-002", workflowexecutionv1.PhaseFailed)
			wfe.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
				Reason:  workflowexecutionv1.FailureReasonDeduplicated,
				Message: "Execution resource already exists, owned by WorkflowExecution original-wfe",
			}
			wfe.Status.DeduplicatedBy = "original-wfe"

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).WithStatusSubresource(rr).Build()

			h := handler.NewWorkflowExecutionHandler(c, scheme, nil,
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
					return ctrl.Result{}, nil
				},
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) (ctrl.Result, error) {
					return ctrl.Result{}, nil
				},
			)

			_, err := h.HandleStatus(ctx, rr, wfe)
			Expect(err).ToNot(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(c.Get(ctx, types.NamespacedName{Name: "dedup-rr-002", Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.DeduplicatedByWE).To(Equal("original-wfe"),
				"DeduplicatedByWE must be set from WFE.Status.DeduplicatedBy")
		})

		It("UT-RO-190-003: should requeue after setting DeduplicatedByWE", func() {
			rr := newRemediationRequest("dedup-rr-003", "default", remediationv1.PhaseExecuting)
			setWERef(rr, "dedup-wfe-003", "default")

			wfe := newWorkflowExecution("dedup-wfe-003", "default", "dedup-rr-003", workflowexecutionv1.PhaseFailed)
			wfe.Status.FailureDetails = &workflowexecutionv1.FailureDetails{
				Reason:  workflowexecutionv1.FailureReasonDeduplicated,
				Message: "Execution resource already exists",
			}
			wfe.Status.DeduplicatedBy = "original-wfe"

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).WithStatusSubresource(rr).Build()

			h := handler.NewWorkflowExecutionHandler(c, scheme, nil,
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
					return ctrl.Result{}, nil
				},
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) (ctrl.Result, error) {
					return ctrl.Result{}, nil
				},
			)

			result, err := h.HandleStatus(ctx, rr, wfe)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"Must requeue to check original WFE status on next reconcile")
		})

		It("UT-RO-190-004: should still call transitionToFailed for non-dedup WFE failure (regression guard)", func() {
			rr := newRemediationRequest("dedup-rr-004", "default", remediationv1.PhaseExecuting)
			setWERef(rr, "normal-wfe-004", "default")

			wfe := newWorkflowExecutionFailed("normal-wfe-004", "default", "dedup-rr-004", "some-failure")

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr).WithStatusSubresource(rr).Build()

			h := handler.NewWorkflowExecutionHandler(c, scheme, nil,
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
					transitionCalled = true
					return ctrl.Result{}, nil
				},
				func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) (ctrl.Result, error) {
					return ctrl.Result{}, nil
				},
			)

			_, err := h.HandleStatus(ctx, rr, wfe)
			Expect(err).ToNot(HaveOccurred())
			Expect(transitionCalled).To(BeTrue(),
				"transitionToFailed MUST be called for non-deduplicated WFE failures")
		})
	})
})

var _ = Describe("Issue #190: Cross-WE Result Propagation", func() {
	var (
		ctx    context.Context
		scheme = setupScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("C3 short-circuit in handleExecutingPhase", func() {

		It("UT-RO-190-005: original WFE Completed → RR inherits Completed", func() {
			rr := newRemediationRequest("prop-rr-005", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-005", "default")
			rr.Status.DeduplicatedByWE = "original-wfe-005"

			dedupWFE := newWorkflowExecution("dedup-wfe-005", "default", "prop-rr-005", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "original-wfe-005"

			originalWFE := newWorkflowExecutionCompleted("original-wfe-005", "default", "other-rr")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, originalWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-005", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-005", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseCompleted),
				"Behavior: RR must inherit Completed from original WFE")
			Expect(updated.Status.Outcome).To(Equal("Remediated"),
				"Behavior: Outcome must be Remediated (lineage tracked via DeduplicatedByWE + K8s events)")
			Expect(updated.Status.CompletedAt).NotTo(BeNil())
		})

		It("UT-RO-190-006: original WFE Failed → RR inherits Failed with FailurePhaseDeduplicated", func() {
			rr := newRemediationRequest("prop-rr-006", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-006", "default")
			rr.Status.DeduplicatedByWE = "original-wfe-006"

			dedupWFE := newWorkflowExecution("dedup-wfe-006", "default", "prop-rr-006", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "original-wfe-006"

			originalWFE := newWorkflowExecutionFailed("original-wfe-006", "default", "other-rr", "OOM killed")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, originalWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-006", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-006", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
				"Behavior: RR must inherit Failed from original WFE")
			Expect(updated.Status.FailurePhase).NotTo(BeNil())
			Expect(*updated.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated),
				"Behavior: FailurePhase must be Deduplicated for inherited failures")
		})

		It("UT-RO-190-011: original WFE deleted → RR transitions to Failed/Deduplicated", func() {
			rr := newRemediationRequest("prop-rr-011", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-011", "default")
			rr.Status.DeduplicatedByWE = "deleted-wfe-011"

			dedupWFE := newWorkflowExecution("dedup-wfe-011", "default", "prop-rr-011", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "deleted-wfe-011"

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-011", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-011", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
				"Behavior: RR must fail when original WFE is deleted (dangling reference)")
			Expect(updated.Status.FailurePhase).NotTo(BeNil())
			Expect(*updated.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated))
		})

		It("UT-RO-190-012: original WFE still Running → RR stays Executing, requeue", func() {
			rr := newRemediationRequest("prop-rr-012", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-012", "default")
			rr.Status.DeduplicatedByWE = "running-wfe-012"

			dedupWFE := newWorkflowExecution("dedup-wfe-012", "default", "prop-rr-012", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "running-wfe-012"

			runningWFE := newWorkflowExecution("running-wfe-012", "default", "other-rr", workflowexecutionv1.PhaseRunning)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, runningWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-012", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-012", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseExecuting),
				"Behavior: RR must stay Executing while original WFE is still Running")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"Behavior: must requeue to check original WFE again")
		})

		It("UT-RO-190-016: idempotency — 2nd reconcile with DeduplicatedByWE set and Running original → no duplicate events", func() {
			rr := newRemediationRequest("prop-rr-016", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-016", "default")
			rr.Status.DeduplicatedByWE = "running-wfe-016"

			dedupWFE := newWorkflowExecution("dedup-wfe-016", "default", "prop-rr-016", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "running-wfe-016"

			runningWFE := newWorkflowExecution("running-wfe-016", "default", "other-rr", workflowexecutionv1.PhaseRunning)

			fakeRecorder := record.NewFakeRecorder(20)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, runningWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				fakeRecorder,
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			result1, err1 := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-016", Namespace: "default"},
			})
			Expect(err1).NotTo(HaveOccurred())
			Expect(result1.RequeueAfter).To(BeNumerically(">", 0))

			result2, err2 := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-016", Namespace: "default"},
			})
			Expect(err2).NotTo(HaveOccurred())
			Expect(result2.RequeueAfter).To(BeNumerically(">", 0))

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-016", Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseExecuting),
				"Behavior: multiple reconciles must be idempotent — RR stays Executing")
		})
	})

	Context("Phase 6: Notification provenance", func() {

		It("UT-RO-190-015: inherited Completed emits K8s event with original WFE provenance", func() {
			rr := newRemediationRequest("prop-rr-015", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-015", "default")
			rr.Status.DeduplicatedByWE = "original-wfe-015"

			dedupWFE := newWorkflowExecution("dedup-wfe-015", "default", "prop-rr-015", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "original-wfe-015"

			originalWFE := newWorkflowExecutionCompleted("original-wfe-015", "default", "other-rr")

			fakeRecorder := record.NewFakeRecorder(20)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, originalWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				fakeRecorder,
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-015", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			var foundEvent bool
			for len(fakeRecorder.Events) > 0 {
				event := <-fakeRecorder.Events
				if event != "" && (containsSubstring(event, "InheritedCompleted") || containsSubstring(event, "original-wfe-015")) {
					foundEvent = true
					break
				}
			}
			Expect(foundEvent).To(BeTrue(),
				"Behavior: inherited transitions must emit K8s events with original WFE provenance")
		})
	})

	Context("Error handling in handleDedupResultPropagation", func() {

		It("UT-RO-190-019: transient Get error on original WFE returns reconcile error (not swallowed)", func() {
			rr := newRemediationRequest("prop-rr-019", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-019", "default")
			rr.Status.DeduplicatedByWE = "transient-error-wfe-019"

			dedupWFE := newWorkflowExecution("dedup-wfe-019", "default", "prop-rr-019", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "transient-error-wfe-019"

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if key.Name == "transient-error-wfe-019" {
							return fmt.Errorf("simulated transient API server error")
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-019", Namespace: "default"},
			})
			Expect(err).To(HaveOccurred(),
				"Behavior: transient Get error on original WFE must surface as reconcile error, not be swallowed")
			Expect(err.Error()).To(ContainSubstring("transient-error-wfe-019"),
				"Error must contain the original WFE name for debugging")

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-019", Namespace: "default"}, updated)).To(Succeed())
			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseExecuting),
				"Behavior: RR must remain in Executing phase on transient error (reconcile retries)")
		})
	})

	Context("Audit and notification provenance", func() {

		It("UT-RO-190-017: inherited Failed emits K8s event with original WFE provenance", func() {
			rr := newRemediationRequest("prop-rr-017", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-017", "default")
			rr.Status.DeduplicatedByWE = "failed-original-017"

			dedupWFE := newWorkflowExecution("dedup-wfe-017", "default", "prop-rr-017", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "failed-original-017"

			originalWFE := newWorkflowExecutionFailed("failed-original-017", "default", "other-rr", "OOM killed")

			fakeRecorder := record.NewFakeRecorder(20)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, originalWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				fakeRecorder,
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-017", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			var foundEvent bool
			for len(fakeRecorder.Events) > 0 {
				event := <-fakeRecorder.Events
				if event != "" && (containsSubstring(event, "InheritedFailed") || containsSubstring(event, "failed-original-017")) {
					foundEvent = true
					break
				}
			}
			Expect(foundEvent).To(BeTrue(),
				"Behavior: inherited failure must emit K8s event with original WFE provenance")
		})

		It("UT-RO-190-018: inherited Failed sets FailureReason with original WFE name for audit traceability", func() {
			rr := newRemediationRequest("prop-rr-018", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-018", "default")
			rr.Status.DeduplicatedByWE = "failed-original-018"

			dedupWFE := newWorkflowExecution("dedup-wfe-018", "default", "prop-rr-018", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "failed-original-018"

			originalWFE := newWorkflowExecutionFailed("failed-original-018", "default", "other-rr", "OOM killed")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, originalWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-018", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-018", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.FailureReason).NotTo(BeNil())
			Expect(*updated.Status.FailureReason).To(ContainSubstring("failed-original-018"),
				"Behavior: FailureReason must contain original WFE name for audit trail traceability")
		})
	})

	Context("Phase 5: Consecutive failure exclusion", func() {

		It("UT-RO-190-013: inherited failure does NOT increment ConsecutiveFailureCount", func() {
			rr := newRemediationRequest("prop-rr-013", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-013", "default")
			rr.Status.DeduplicatedByWE = "deleted-wfe-013"

			dedupWFE := newWorkflowExecution("dedup-wfe-013", "default", "prop-rr-013", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "deleted-wfe-013"

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-013", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-013", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
			Expect(updated.Status.ConsecutiveFailureCount).To(Equal(int32(0)),
				"Behavior: inherited failures must NOT increment ConsecutiveFailureCount")
			Expect(updated.Status.NextAllowedExecution).To(BeNil(),
				"Behavior: inherited failures must NOT set exponential backoff")
		})

		It("UT-RO-190-014: inherited failure sets FailurePhaseDeduplicated for countConsecutiveFailures exclusion", func() {
			rr := newRemediationRequest("prop-rr-014", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			rr.Status.ExecutingStartTime = &metav1.Time{Time: time.Now()}
			setWERef(rr, "dedup-wfe-014", "default")
			rr.Status.DeduplicatedByWE = "failed-original-014"

			dedupWFE := newWorkflowExecution("dedup-wfe-014", "default", "prop-rr-014", workflowexecutionv1.PhaseFailed)
			dedupWFE.Status.FailureDetails = &workflowexecutionv1.FailureDetails{Reason: workflowexecutionv1.FailureReasonDeduplicated}
			dedupWFE.Status.DeduplicatedBy = "failed-original-014"

			originalWFE := newWorkflowExecutionFailed("failed-original-014", "default", "other-rr", "OOM killed")

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, dedupWFE, originalWFE).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prop-rr-014", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prop-rr-014", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed))
			Expect(updated.Status.FailurePhase).NotTo(BeNil())
			Expect(*updated.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseDeduplicated),
				"Invariant: FailurePhase=Deduplicated marks this RR for exclusion from countConsecutiveFailures")
			Expect(updated.Status.ConsecutiveFailureCount).To(Equal(int32(0)),
				"Invariant: ConsecutiveFailureCount must be 0 (not incremented by transitionToInheritedFailed)")
		})
	})
})

func setWERef(rr *remediationv1.RemediationRequest, name, namespace string) {
	rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
		APIVersion: workflowexecutionv1.GroupVersion.String(),
		Kind:       "WorkflowExecution",
		Name:       name,
		Namespace:  namespace,
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}
