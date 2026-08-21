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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adksession "google.golang.org/adk/session"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// BR-INTERACTIVE-010 SC-9 (#2214, DD-AA-KA-001 Amendment): AF closes the
// InvestigationSession correlated to an AgentSession's terminal state,
// replacing AA's retired K8sISPhaseUpdater. Phase-mapping logic is proven
// here in isolation (fake client); wiring through a live manager watch
// (including the delete-event path) is proven by the envtest ITs in
// test/integration/apifrontend/agentsession_close_wiring_test.go.
var _ = Describe("AgentSessionTerminalCloseReconciler [AU-2, AU-3, CC7.2]", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = agentsessionv1.AddToScheme(scheme)
		_ = isv1alpha1.AddToScheme(scheme)
	})

	newFakeClientWithIndex := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&isv1alpha1.InvestigationSession{}).
			WithIndex(&isv1alpha1.InvestigationSession{}, session.FieldIndexRRName,
				func(obj client.Object) []string {
					is := obj.(*isv1alpha1.InvestigationSession)
					if is.Spec.RemediationRequestRef.Name == "" {
						return nil
					}
					return []string{is.Spec.RemediationRequestRef.Name}
				}).
			Build()
	}

	newAgentSession := func(name, rrName string, phase agentsessionv1.AgentSessionPhase) *agentsessionv1.AgentSession {
		return &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
			Spec: agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: rrName, Namespace: "test-ns"},
				IncidentID:            "ai-" + rrName,
			},
			Status: agentsessionv1.AgentSessionStatus{Phase: phase},
		}
	}

	newIS := func(name, rrName string) *isv1alpha1.InvestigationSession {
		return &isv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
			Spec: isv1alpha1.InvestigationSessionSpec{
				A2ATaskID:             "task-1",
				UserIdentity:          isv1alpha1.SessionUser{Username: "jane.doe"},
				JoinMode:              isv1alpha1.SessionJoinModeStart,
				RemediationRequestRef: isv1alpha1.ObjectRef{Name: rrName, Namespace: "test-ns"},
			},
			Status: isv1alpha1.InvestigationSessionStatus{Phase: isv1alpha1.SessionPhaseActive},
		}
	}

	reconcile := func(k8s client.Client, name string) (ctrl.Result, error) {
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")
		r := controller.NewAgentSessionTerminalCloseReconciler(k8s, svc, logr.Discard())
		return r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: "test-ns"},
		})
	}

	// UT-AF-2214-001: Completed AgentSession closes the correlated IS to Completed.
	It("UT-AF-2214-001: closes IS to Completed when AgentSession.Status.Phase=Completed [CC7.2]", func() {
		as := newAgentSession("as-rr-1", "rr-1", agentsessionv1.AgentSessionPhaseCompleted)
		is := newIS("is-1", "rr-1")
		k8s := newFakeClientWithIndex(as, is)

		_, err := reconcile(k8s, "as-rr-1")
		Expect(err).NotTo(HaveOccurred())

		var fetched isv1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "is-1", Namespace: "test-ns"}, &fetched)).To(Succeed())
		Expect(fetched.Status.Phase).To(Equal(isv1alpha1.SessionPhaseCompleted))
	})

	// UT-AF-2214-002: Failed AgentSession closes the correlated IS to Failed.
	It("UT-AF-2214-002: closes IS to Failed when AgentSession.Status.Phase=Failed [CC7.2]", func() {
		as := newAgentSession("as-rr-2", "rr-2", agentsessionv1.AgentSessionPhaseFailed)
		is := newIS("is-2", "rr-2")
		k8s := newFakeClientWithIndex(as, is)

		_, err := reconcile(k8s, "as-rr-2")
		Expect(err).NotTo(HaveOccurred())

		var fetched isv1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "is-2", Namespace: "test-ns"}, &fetched)).To(Succeed())
		Expect(fetched.Status.Phase).To(Equal(isv1alpha1.SessionPhaseFailed))
	})

	// UT-AF-2214-003: non-terminal / unmapped phases (Pending, Investigating,
	// and the KA-driven Cancelled value) must NOT touch IS -- Cancelled-via-
	// Update is intentionally excluded (see agentSessionPhaseToIS doc comment);
	// only Cancelled-via-Delete (a separate wiring-level IT) triggers closure.
	DescribeTable("UT-AF-2214-003: does not touch IS for non-terminal or excluded AgentSession phases [AC-6]",
		func(phase agentsessionv1.AgentSessionPhase) {
			as := newAgentSession("as-rr-3", "rr-3", phase)
			is := newIS("is-3", "rr-3")
			k8s := newFakeClientWithIndex(as, is)

			_, err := reconcile(k8s, "as-rr-3")
			Expect(err).NotTo(HaveOccurred())

			var fetched isv1alpha1.InvestigationSession
			Expect(k8s.Get(ctx, types.NamespacedName{Name: "is-3", Namespace: "test-ns"}, &fetched)).To(Succeed())
			Expect(fetched.Status.Phase).To(Equal(isv1alpha1.SessionPhaseActive), "IS must remain untouched")
		},
		Entry("Pending", agentsessionv1.AgentSessionPhasePending),
		Entry("Investigating", agentsessionv1.AgentSessionPhaseInvestigating),
		Entry("Cancelled (via Update, excluded by design)", agentsessionv1.AgentSessionPhaseCancelled),
	)

	// UT-AF-2214-003b: a not-found AgentSession (already deleted -- handled
	// separately by the Delete-event path) must not error.
	It("UT-AF-2214-003b: is a no-op when the AgentSession no longer exists [AC-6]", func() {
		k8s := newFakeClientWithIndex()
		result, err := reconcile(k8s, "as-gone")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	// UT-AF-2214-004/005 (finalizer redesign, CI RCA PR #2222 run 32513171970 --
	// see terminalCloseFinalizer's doc comment): a raw watch DeleteFunc could
	// drop the AgentSession delete event outright under load. The fake client
	// honors finalizers the same way a real apiserver does (Delete() sets
	// DeletionTimestamp instead of removing the object while a finalizer is
	// present), so these prove the finalizer add/remove lifecycle directly
	// against Reconcile -- no separate raw-watch simulation needed.

	// UT-AF-2214-004: a fresh, non-deleting AgentSession gets the finalizer.
	It("UT-AF-2214-004: adds terminalCloseFinalizer to a fresh AgentSession [CC7.2]", func() {
		as := newAgentSession("as-rr-4", "rr-4", agentsessionv1.AgentSessionPhasePending)
		k8s := newFakeClientWithIndex(as)

		_, err := reconcile(k8s, "as-rr-4")
		Expect(err).NotTo(HaveOccurred())

		var fetched agentsessionv1.AgentSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "as-rr-4", Namespace: "test-ns"}, &fetched)).To(Succeed())
		Expect(controllerutil.ContainsFinalizer(&fetched, "apifrontend.kubernaut.ai/agentsession-terminal-close")).To(BeTrue())
	})

	// UT-AF-2214-005: deleting a finalizer-bearing AgentSession closes IS to
	// Cancelled and removes the finalizer, letting the delete complete.
	It("UT-AF-2214-005: closes IS to Cancelled and removes the finalizer when a finalizer-bearing AgentSession is deleted [CC7.2]", func() {
		as := newAgentSession("as-rr-5", "rr-5", agentsessionv1.AgentSessionPhaseInvestigating)
		is := newIS("is-5", "rr-5")
		k8s := newFakeClientWithIndex(as, is)

		// First reconcile adds the finalizer (mirrors the real sequence: the
		// finalizer is added on an earlier Create/Update before any delete).
		_, err := reconcile(k8s, "as-rr-5")
		Expect(err).NotTo(HaveOccurred())

		Expect(k8s.Delete(ctx, as)).To(Succeed())

		_, err = reconcile(k8s, "as-rr-5")
		Expect(err).NotTo(HaveOccurred())

		var fetchedIS isv1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "is-5", Namespace: "test-ns"}, &fetchedIS)).To(Succeed())
		Expect(fetchedIS.Status.Phase).To(Equal(isv1alpha1.SessionPhaseCancelled))

		// The finalizer's removal let the fake client complete the delete.
		var fetchedAS agentsessionv1.AgentSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "as-rr-5", Namespace: "test-ns"}, &fetchedAS)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "AgentSession must be fully removed once the finalizer is dropped")
	})

	// UT-AF-2214-005b: an AgentSession deleted without ever carrying the
	// finalizer (the narrow pre-upgrade transitional gap documented on
	// reconcileDelete) is a no-op -- it must not error, and IS is untouched.
	It("UT-AF-2214-005b: is a no-op when a finalizer-less AgentSession is deleted [AC-6]", func() {
		as := newAgentSession("as-rr-6", "rr-6", agentsessionv1.AgentSessionPhaseInvestigating)
		is := newIS("is-6", "rr-6")
		k8s := newFakeClientWithIndex(as, is)

		// No finalizer added: fake client (like a real apiserver with no
		// finalizers present) removes the object immediately on Delete.
		Expect(k8s.Delete(ctx, as)).To(Succeed())

		_, err := reconcile(k8s, "as-rr-6")
		Expect(err).NotTo(HaveOccurred())

		var fetchedIS isv1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "is-6", Namespace: "test-ns"}, &fetchedIS)).To(Succeed())
		Expect(fetchedIS.Status.Phase).To(Equal(isv1alpha1.SessionPhaseActive), "IS must remain untouched")
	})
})
