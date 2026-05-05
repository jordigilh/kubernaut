package controller_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/controller"
)

func TestControllerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func newFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.InvestigationSession{}).
		Build()
}

func pastTime(d time.Duration) *metav1.Time {
	t := metav1.NewTime(time.Now().Add(-d))
	return &t
}

func makeSession(name string, phase v1alpha1.SessionPhase, completedAt, disconnectedAt *metav1.Time) *v1alpha1.InvestigationSession {
	sess := &v1alpha1.InvestigationSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
			Labels: map[string]string{
				"apifrontend.kubernaut.ai/phase": string(phase),
			},
		},
		Spec: v1alpha1.InvestigationSessionSpec{
			A2ATaskID: "task-1",
			UserIdentity: v1alpha1.SessionUser{
				Username: "jane.doe",
			},
			JoinMode: v1alpha1.SessionJoinModeStart,
			RemediationRequestRef: v1alpha1.ObjectRef{
				Name:      "rr-1",
				Namespace: "test-ns",
			},
		},
		Status: v1alpha1.InvestigationSessionStatus{
			Phase:          phase,
			CompletedAt:    completedAt,
			DisconnectedAt: disconnectedAt,
		},
	}
	return sess
}

var _ = Describe("SessionCleanupReconciler", func() {
	var (
		scheme        *runtime.Scheme
		ctx           context.Context
		disconnectTTL time.Duration
		retentionTTL  time.Duration
	)

	BeforeEach(func() {
		scheme = newScheme()
		ctx = context.Background()
		disconnectTTL = 15 * time.Minute
		retentionTTL = controller.MinRetentionTTL
	})

	reconcile := func(k8s client.Client, name string) (ctrl.Result, error) {
		r := controller.NewSessionCleanupReconciler(k8s, disconnectTTL, retentionTTL, nil, nil, nil)
		return r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: "test-ns"},
		})
	}

	It("UT-AF-220-001: transitions Disconnected -> Cancelled after TTL", func() {
		sess := makeSession("sess-disc", v1alpha1.SessionPhaseDisconnected, nil, pastTime(20*time.Minute))
		k8s := newFakeClient(scheme, sess)
		sess.Status.DisconnectedAt = pastTime(20 * time.Minute)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-disc")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-disc", Namespace: "test-ns"}, &updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Phase).To(Equal(v1alpha1.SessionPhaseCancelled))
	})

	It("UT-AF-220-002: deletes Completed session after retention", func() {
		expired := controller.MinRetentionTTL + time.Hour
		sess := makeSession("sess-done", v1alpha1.SessionPhaseCompleted, pastTime(expired), nil)
		k8s := newFakeClient(scheme, sess)
		sess.Status.CompletedAt = pastTime(expired)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-done")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-done", Namespace: "test-ns"}, &updated)
		Expect(err).To(HaveOccurred()) // deleted
	})

	It("UT-AF-220-003: deletes Cancelled session after retention", func() {
		expired := controller.MinRetentionTTL + time.Hour
		sess := makeSession("sess-cancel", v1alpha1.SessionPhaseCancelled, pastTime(expired), nil)
		k8s := newFakeClient(scheme, sess)
		sess.Status.CompletedAt = pastTime(expired)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-cancel")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-cancel", Namespace: "test-ns"}, &updated)
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-220-004: deletes Failed session after retention", func() {
		expired := controller.MinRetentionTTL + time.Hour
		sess := makeSession("sess-fail", v1alpha1.SessionPhaseFailed, pastTime(expired), nil)
		k8s := newFakeClient(scheme, sess)
		sess.Status.CompletedAt = pastTime(expired)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-fail")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-fail", Namespace: "test-ns"}, &updated)
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-220-005: does not touch Active session", func() {
		sess := makeSession("sess-active", v1alpha1.SessionPhaseActive, nil, nil)
		k8s := newFakeClient(scheme, sess)

		result, err := reconcile(k8s, "sess-active")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-active", Namespace: "test-ns"}, &updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Phase).To(Equal(v1alpha1.SessionPhaseActive))
	})

	It("UT-AF-220-006: does not delete recent terminal session", func() {
		sess := makeSession("sess-recent", v1alpha1.SessionPhaseCompleted, pastTime(5*time.Minute), nil)
		k8s := newFakeClient(scheme, sess)
		sess.Status.CompletedAt = pastTime(5 * time.Minute)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-recent")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-recent", Namespace: "test-ns"}, &updated)
		Expect(err).NotTo(HaveOccurred()) // not deleted
	})

	It("UT-AF-220-007: requeues Disconnected with correct delay", func() {
		sess := makeSession("sess-disc-recent", v1alpha1.SessionPhaseDisconnected, nil, pastTime(5*time.Minute))
		k8s := newFakeClient(scheme, sess)
		sess.Status.DisconnectedAt = pastTime(5 * time.Minute)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-disc-recent")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", 15*time.Minute))
	})

	It("UT-AF-220-009: handles CRD not found gracefully", func() {
		k8s := newFakeClient(scheme)

		result, err := reconcile(k8s, "nonexistent")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())
	})

	It("UT-AF-220-010: zero-disconnect-TTL still respects boundary", func() {
		disconnectTTL = 0
		sess := makeSession("sess-zero", v1alpha1.SessionPhaseDisconnected, nil, pastTime(1*time.Second))
		k8s := newFakeClient(scheme, sess)
		sess.Status.DisconnectedAt = pastTime(1 * time.Second)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-zero")
		Expect(err).NotTo(HaveOccurred())
		_ = result

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-zero", Namespace: "test-ns"}, &updated)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Status.Phase).To(Equal(v1alpha1.SessionPhaseCancelled))
		Expect(updated.Status.Message).To(Equal("auto-cancelled: disconnect TTL expired"))
	})
})
