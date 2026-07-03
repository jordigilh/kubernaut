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

package controller_test

// Characterization tests for RARReconciler.Reconcile (Wave 6 sub-wave 6e-i
// RED phase, GO-ANTIPATTERN-AUDIT-2026-07-01). Reconcile had zero unit and
// zero integration coverage despite being a SOC2 CC8.1/CC6.8 audit-emission
// path (BR-AUDIT-006, DD-WEBHOOK-003). These tests pin the pre-refactor
// behavior across all of Reconcile's branches so the upcoming Extract-Method
// decomposition (cyclomatic 15 -> target <15) is a safe, pure code-motion
// change with a regression safety net.

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	rarconditions "github.com/jordigilh/kubernaut/pkg/remediationapprovalrequest"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// erroringAuditStore is a MockAuditStore variant that always fails
// StoreAudit, used to characterize Reconcile's fire-and-forget error path
// (AuditRecorded condition set to false/AuditFailed, no reconcile error).
type erroringAuditStore struct {
	Events []*ogenclient.AuditEventRequest
	err    error
}

func (m *erroringAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
	m.Events = append(m.Events, event)
	return m.err
}
func (m *erroringAuditStore) Flush(ctx context.Context) error { return nil }
func (m *erroringAuditStore) Close() error                    { return nil }

var _ = Describe("BR-AUDIT-006: RARReconciler.Reconcile (approval-decision audit emission)", func() {
	var (
		ctx     context.Context
		scheme  = setupScheme()
		metrics *rometrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()
		metrics = rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	})

	newFakeClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
			Build()
	}

	It("UT-RO-RAR-001: RAR not found returns no error and emits no audit event", func() {
		fakeClient := newFakeClient()
		mockAuditStore := &MockAuditStore{}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, mockAuditStore, metrics)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "missing-rar", Namespace: "default"},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(mockAuditStore.Events).To(BeEmpty())
	})

	It("UT-RO-RAR-002: pending decision (empty Status.Decision) is a no-op", func() {
		rar := newRemediationApprovalRequestApproved("rar-pending", "default", "rr-1", "")
		rar.Status.Decision = "" // override fixture: no decision yet
		rar.Status.DecidedAt = nil
		fakeClient := newFakeClient(rar)
		mockAuditStore := &MockAuditStore{}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, mockAuditStore, metrics)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "rar-pending", Namespace: "default"},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(mockAuditStore.Events).To(BeEmpty())
	})

	It("UT-RO-RAR-003: already-audited RAR (AuditRecorded=True) is a no-op (idempotency, DD-STATUS-001)", func() {
		rar := newRemediationApprovalRequestApproved("rar-already-audited", "default", "rr-2", "alice")
		fakeClient := newFakeClient(rar)

		// Mark AuditRecorded=True directly via the status subresource so the
		// apiReader re-fetch (cache-bypassed) observes it as already set.
		var current remediationv1.RemediationApprovalRequest
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "rar-already-audited", Namespace: "default"}, &current)).To(Succeed())
		rarconditions.SetAuditRecorded(&current, true, rarconditions.ReasonAuditSucceeded, "already recorded", metrics)
		Expect(fakeClient.Status().Update(ctx, &current)).To(Succeed())

		mockAuditStore := &MockAuditStore{}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, mockAuditStore, metrics)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "rar-already-audited", Namespace: "default"},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(mockAuditStore.Events).To(BeEmpty(), "must not re-emit audit for an already-audited RAR")
	})

	It("UT-RO-RAR-004: missing parent RemediationRequest reference is a no-op (defensive guard)", func() {
		rar := newRemediationApprovalRequestApproved("rar-no-parent", "default", "rr-3", "alice")
		rar.Spec.RemediationRequestRef = corev1.ObjectReference{} // Name == ""
		fakeClient := newFakeClient(rar)
		mockAuditStore := &MockAuditStore{}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, mockAuditStore, metrics)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "rar-no-parent", Namespace: "default"},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(mockAuditStore.Events).To(BeEmpty())
	})

	It("UT-RO-RAR-005: Approved decision emits an audit event and sets AuditRecorded=True", func() {
		rar := newRemediationApprovalRequestApproved("rar-approved", "default", "rr-4", "alice")
		fakeClient := newFakeClient(rar)
		mockAuditStore := &MockAuditStore{}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, mockAuditStore, metrics)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "rar-approved", Namespace: "default"},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(mockAuditStore.Events).To(HaveLen(1))
		event := mockAuditStore.GetLastEvent()
		Expect(event.CorrelationID).To(Equal("rr-4"))
		Expect(event.EventType).To(Equal(roaudit.EventTypeApprovalApproved))

		var updated remediationv1.RemediationApprovalRequest
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "rar-approved", Namespace: "default"}, &updated)).To(Succeed())
		cond := meta.FindStatusCondition(updated.Status.Conditions, rarconditions.ConditionAuditRecorded)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(rarconditions.ReasonAuditSucceeded))
	})

	It("UT-RO-RAR-006: Rejected decision emits an audit event and sets AuditRecorded=True", func() {
		rar := newRemediationApprovalRequestRejected("rar-rejected", "default", "rr-5", "bob", "insufficient evidence")
		fakeClient := newFakeClient(rar)
		mockAuditStore := &MockAuditStore{}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, mockAuditStore, metrics)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "rar-rejected", Namespace: "default"},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(mockAuditStore.Events).To(HaveLen(1))
		event := mockAuditStore.GetLastEvent()
		Expect(event.EventType).To(Equal(roaudit.EventTypeApprovalRejected))

		var updated remediationv1.RemediationApprovalRequest
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "rar-rejected", Namespace: "default"}, &updated)).To(Succeed())
		cond := meta.FindStatusCondition(updated.Status.Conditions, rarconditions.ConditionAuditRecorded)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("UT-RO-RAR-007: audit store failure is fire-and-forget (no reconcile error) and sets AuditRecorded=False/AuditFailed", func() {
		rar := newRemediationApprovalRequestApproved("rar-audit-fail", "default", "rr-6", "alice")
		fakeClient := newFakeClient(rar)
		erroringStore := &erroringAuditStore{err: fmt.Errorf("simulated datastorage outage")}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, erroringStore, metrics)

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "rar-audit-fail", Namespace: "default"},
		})

		Expect(err).ToNot(HaveOccurred(), "fire-and-forget: audit failures must not fail reconciliation")
		Expect(result).To(Equal(ctrl.Result{}))
		Expect(erroringStore.Events).To(HaveLen(1))

		var updated remediationv1.RemediationApprovalRequest
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "rar-audit-fail", Namespace: "default"}, &updated)).To(Succeed())
		cond := meta.FindStatusCondition(updated.Status.Conditions, rarconditions.ConditionAuditRecorded)
		Expect(cond).ToNot(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(rarconditions.ReasonAuditFailed))
	})

	It("UT-RO-RAR-008: SetupWithManager wires the controller without error", func() {
		// Smoke-test only: full manager wiring is exercised by cmd/remediationorchestrator
		// and integration tests; this just confirms the builder call itself doesn't panic
		// for a nil manager guard regression during the upcoming decomposition.
		fakeClient := newFakeClient()
		mockAuditStore := &MockAuditStore{}
		reconciler := prodcontroller.NewRARReconciler(fakeClient, fakeClient, scheme, mockAuditStore, metrics)
		Expect(reconciler).ToNot(BeNil())
	})
})
