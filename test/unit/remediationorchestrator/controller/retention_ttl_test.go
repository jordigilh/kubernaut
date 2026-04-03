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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// ========================================
// Issue #265: CRD 24h TTL Enforcement
// ========================================
//
// Tests for:
// - RetentionExpiryTime set on terminal RRs (UT-RO-265-001 through 005)
// - Cleanup of expired CRDs (UT-RO-265-006 through 008)
// - CompletedAt fix on Failed/GlobalTimeout (UT-RO-265-009, 010)

var _ = Describe("Issue #265: CRD Retention TTL Enforcement", func() {
	var (
		ctx    context.Context
		scheme = setupScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("RetentionExpiryTime assignment on terminal RRs", func() {

		It("UT-RO-265-001: should set RetentionExpiryTime on Completed RR", func() {
			rr := newRemediationRequest("test-rr", "default", remediationv1.PhaseCompleted)
			rr.Status.ObservedGeneration = rr.Generation
			now := metav1.Now()
			rr.Status.CompletedAt = &now

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
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
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.RetentionExpiryTime).NotTo(BeNil(),
				"Behavior: RetentionExpiryTime must be set on terminal Completed RR")
			Expect(updated.Status.RetentionExpiryTime.Time).To(BeTemporally("~", time.Now().Add(24*time.Hour), 5*time.Second),
				"Accuracy: expiry should be ~24h from now (default retention period)")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0),
				"Behavior: must requeue for cleanup after TTL")
		})

		It("UT-RO-265-002: should set RetentionExpiryTime on Failed RR", func() {
			rr := newRemediationRequest("test-rr-failed", "default", remediationv1.PhaseFailed)
			rr.Status.ObservedGeneration = rr.Generation
			now := metav1.Now()
			rr.Status.CompletedAt = &now
			reason := "test failure"
			failPhase := remediationv1.FailurePhaseWorkflowExecution
			rr.Status.FailureReason = &reason
			rr.Status.FailurePhase = &failPhase

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
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
				NamespacedName: types.NamespacedName{Name: "test-rr-failed", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-failed", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.RetentionExpiryTime).NotTo(BeNil(),
				"Behavior: RetentionExpiryTime must be set on terminal Failed RR")
		})

		It("UT-RO-265-003: should set RetentionExpiryTime on TimedOut RR", func() {
			rr := newRemediationRequest("test-rr-timedout", "default", remediationv1.PhaseTimedOut)
			rr.Status.ObservedGeneration = rr.Generation
			now := metav1.Now()
			rr.Status.CompletedAt = &now
			phase := remediationv1.RemediationPhase("Processing")
			rr.Status.TimeoutPhase = &phase

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
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
				NamespacedName: types.NamespacedName{Name: "test-rr-timedout", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-timedout", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.RetentionExpiryTime).NotTo(BeNil(),
				"Behavior: RetentionExpiryTime must be set on terminal TimedOut RR")
		})

		It("UT-RO-265-004: should NOT set RetentionExpiryTime on non-terminal Pending RR", func() {
			rr := newRemediationRequest("test-rr-pending", "default", remediationv1.PhasePending)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, _ = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr-pending", Namespace: "default"},
			})

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-pending", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.RetentionExpiryTime).To(BeNil(),
				"Behavior: non-terminal Pending RR must NOT get RetentionExpiryTime")
		})

		It("UT-RO-265-005: should NOT overwrite existing RetentionExpiryTime", func() {
			rr := newRemediationRequest("test-rr-existing", "default", remediationv1.PhaseCompleted)
			rr.Status.ObservedGeneration = rr.Generation
			now := metav1.Now()
			rr.Status.CompletedAt = &now
			existingExpiry := metav1.NewTime(time.Now().Add(48 * time.Hour))
			rr.Status.RetentionExpiryTime = &existingExpiry

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
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
				NamespacedName: types.NamespacedName{Name: "test-rr-existing", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-existing", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.RetentionExpiryTime.Time).To(BeTemporally("~", existingExpiry.Time, 1*time.Second),
				"Behavior: existing RetentionExpiryTime must not be overwritten")
		})
	})

	Context("Cleanup of expired CRDs", func() {

		It("UT-RO-265-006: should delete CRD when RetentionExpiryTime has expired", func() {
			rr := newRemediationRequest("test-rr-expired", "default", remediationv1.PhaseCompleted)
			rr.Status.ObservedGeneration = rr.Generation
			now := metav1.Now()
			rr.Status.CompletedAt = &now
			expired := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			rr.Status.RetentionExpiryTime = &expired

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
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
				NamespacedName: types.NamespacedName{Name: "test-rr-expired", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero(),
				"Behavior: no requeue after deletion")

			deleted := &remediationv1.RemediationRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-expired", Namespace: "default"}, deleted)
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
			Expect(err).To(HaveOccurred(),
				"Behavior: expired CRD must be deleted from cluster")
		})

		It("UT-RO-265-008: should emit retention_cleanup audit event before deleting expired CRD", func() {
			rr := newRemediationRequest("test-rr-audit-del", "default", remediationv1.PhaseCompleted)
			rr.Status.ObservedGeneration = rr.Generation
			now := metav1.Now()
			rr.Status.CompletedAt = &now
			expired := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			rr.Status.RetentionExpiryTime = &expired

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			mockAudit := &MockAuditStore{}
			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, mockAudit,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr-audit-del", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero(), "Behavior: no requeue after deletion")

			deleted := &remediationv1.RemediationRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-audit-del", Namespace: "default"}, deleted)
			Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
			Expect(err).To(HaveOccurred(), "Precondition: CRD must have been deleted")

			cleanupEvents := mockAudit.GetEventsByType(roaudit.EventTypeLifecycleCompleted)
			Expect(cleanupEvents).NotTo(BeEmpty(),
				"Behavior: retention_cleanup audit event must be emitted before deletion")
			var found bool
			for _, evt := range cleanupEvents {
				if evt.EventAction == "retention_cleanup" {
					found = true
					Expect(evt.EventType).To(Equal(roaudit.EventTypeLifecycleCompleted))
					break
				}
			}
			Expect(found).To(BeTrue(),
				"Behavior: audit event with action=retention_cleanup must exist")
		})

		It("UT-RO-265-007: should requeue when RetentionExpiryTime is not yet expired", func() {
			rr := newRemediationRequest("test-rr-future", "default", remediationv1.PhaseCompleted)
			rr.Status.ObservedGeneration = rr.Generation
			now := metav1.Now()
			rr.Status.CompletedAt = &now
			future := metav1.NewTime(time.Now().Add(2 * time.Hour))
			rr.Status.RetentionExpiryTime = &future

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
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
				NamespacedName: types.NamespacedName{Name: "test-rr-future", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 1*time.Hour),
				"Behavior: RequeueAfter should be approximately time until expiry")
			Expect(result.RequeueAfter).To(BeNumerically("<=", 2*time.Hour+1*time.Second),
				"Accuracy: RequeueAfter should not exceed time until expiry")
		})
	})

	Context("CompletedAt consistency fix (F3)", func() {

		It("UT-RO-265-009: transitionToFailed should set CompletedAt", func() {
			rr := newRemediationRequest("test-rr-fail-ts", "default", remediationv1.PhaseExecuting)
			rr.Status.ObservedGeneration = rr.Generation
			rr.Status.StartTime = &metav1.Time{Time: time.Now().Add(-30 * time.Minute)}
			rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{
					Global:     1 * time.Hour,
					Processing: 5 * time.Minute,
					Analyzing:  10 * time.Minute,
					Executing:  30 * time.Minute,
				},
				&MockRoutingEngine{},
			)

			_, _ = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr-fail-ts", Namespace: "default"},
			})

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-fail-ts", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
				"Precondition: RR must have transitioned to Failed (executing phase timeout)")
			Expect(updated.Status.CompletedAt).NotTo(BeNil(),
				"Behavior: CompletedAt must be set when RR transitions to Failed")
		})

		It("UT-RO-265-010: handleGlobalTimeout should set CompletedAt", func() {
			rr := newRemediationRequest("test-rr-timeout-ts", "default", remediationv1.PhasePending)
			rr.Status.ObservedGeneration = 0
			rr.Status.StartTime = &metav1.Time{Time: time.Now().Add(-2 * time.Hour)}
			rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler := prodcontroller.NewReconciler(
				fakeClient, fakeClient, scheme, nil,
				record.NewFakeRecorder(20),
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				prodcontroller.TimeoutConfig{Global: 1 * time.Hour},
				&MockRoutingEngine{},
			)

			_, _ = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr-timeout-ts", Namespace: "default"},
			})

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-timeout-ts", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseTimedOut),
				"Precondition: RR must have transitioned to TimedOut (global timeout)")
			Expect(updated.Status.CompletedAt).NotTo(BeNil(),
				"Behavior: CompletedAt must be set when RR transitions to TimedOut via global timeout")
		})
	})
})
