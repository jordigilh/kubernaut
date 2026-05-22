package apifrontend_test

import (
	"context"
	"os"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func newFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.InvestigationSession{}).
		Build()
}

func pastTime(d time.Duration) *metav1.Time {
	t := metav1.NewTime(time.Now().Add(-d))
	return &t
}

func makeSession(name string, phase v1alpha1.SessionPhase, completedAt, disconnectedAt *metav1.Time) *v1alpha1.InvestigationSession {
	return &v1alpha1.InvestigationSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
			Labels: map[string]string{
				"kubernaut.ai/phase": string(phase),
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
}

var _ = Describe("SessionCleanupReconciler", func() {
	var (
		s             *runtime.Scheme
		ctx           context.Context
		disconnectTTL time.Duration
		retentionTTL  time.Duration
	)

	BeforeEach(func() {
		s = newTestScheme()
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

	It("IT-AF-220-001: transitions Disconnected -> Cancelled after TTL", func() {
		sess := makeSession("sess-disc", v1alpha1.SessionPhaseDisconnected, nil, pastTime(20*time.Minute))
		k8s := newFakeClient(s, sess)
		sess.Status.DisconnectedAt = pastTime(20 * time.Minute)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-disc")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		var updated v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "sess-disc", Namespace: "test-ns"}, &updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(v1alpha1.SessionPhaseCancelled))
	})

	It("IT-AF-220-002: deletes Completed session after retention", func() {
		expired := controller.MinRetentionTTL + time.Hour
		sess := makeSession("sess-done", v1alpha1.SessionPhaseCompleted, pastTime(expired), nil)
		k8s := newFakeClient(s, sess)
		sess.Status.CompletedAt = pastTime(expired)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-done")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		var updated v1alpha1.InvestigationSession
		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-done", Namespace: "test-ns"}, &updated)
		Expect(err).To(HaveOccurred())
	})

	It("IT-AF-220-003: does not touch Active session", func() {
		sess := makeSession("sess-active", v1alpha1.SessionPhaseActive, nil, nil)
		k8s := newFakeClient(s, sess)

		result, err := reconcile(k8s, "sess-active")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		var updated v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "sess-active", Namespace: "test-ns"}, &updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(v1alpha1.SessionPhaseActive))
	})

	It("IT-AF-220-004: requeues Disconnected with correct delay", func() {
		sess := makeSession("sess-disc-recent", v1alpha1.SessionPhaseDisconnected, nil, pastTime(5*time.Minute))
		k8s := newFakeClient(s, sess)
		sess.Status.DisconnectedAt = pastTime(5 * time.Minute)
		_ = k8s.Status().Update(ctx, sess)

		result, err := reconcile(k8s, "sess-disc-recent")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", 15*time.Minute))
	})

	It("IT-AF-220-005: PruneTerminalEntries called on session deletion", func() {
		k8s := newFakeClient(s)
		svc := session.NewCRDSessionService(
			adksession.InMemoryService(), k8s, s, "test-ns",
		)

		createReq := adksession.CreateRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "jane.doe",
			SessionID: "sess-prune-target",
			State: map[string]any{
				session.StateKeyCreateConfig: &session.CreateConfig{
					A2ATaskID:    "task-1",
					UserIdentity: v1alpha1.SessionUser{Username: "jane.doe"},
					JoinMode:     v1alpha1.SessionJoinModeStart,
					RemediationRef: v1alpha1.ObjectRef{
						Name: "rr-1", Namespace: "test-ns",
					},
				},
			},
		}
		_, err := svc.Create(ctx, &createReq)
		Expect(err).NotTo(HaveOccurred())

		err = svc.MaterializeCRD(ctx, "sess-prune-target", v1alpha1.ObjectRef{Name: "rr-1", Namespace: "test-ns"})
		Expect(err).NotTo(HaveOccurred())

		Expect(svc.UpdatePhase(ctx, "sess-prune-target", v1alpha1.SessionPhaseCompleted, "done", "test-user")).To(Succeed())

		expired := controller.MinRetentionTTL + time.Hour
		var crd v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Name: "sess-prune-target", Namespace: "test-ns"}, &crd)).To(Succeed())
		past := metav1.NewTime(time.Now().Add(-expired))
		crd.Status.CompletedAt = &past
		Expect(k8s.Status().Update(ctx, &crd)).To(Succeed())

		r := controller.NewSessionCleanupReconciler(k8s, disconnectTTL, retentionTTL, nil, nil, svc)
		result, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "sess-prune-target", Namespace: "test-ns"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())

		err = k8s.Get(ctx, types.NamespacedName{Name: "sess-prune-target", Namespace: "test-ns"}, &crd)
		Expect(err).To(HaveOccurred())
	})

	It("IT-AF-220-006: emits audit event on disconnect TTL expiry", func() {
		sess := makeSession("sess-audit-disc", v1alpha1.SessionPhaseDisconnected, nil, pastTime(20*time.Minute))
		k8s := newFakeClient(s, sess)
		sess.Status.DisconnectedAt = pastTime(20 * time.Minute)
		_ = k8s.Status().Update(ctx, sess)

		emitter := &testAuditEmitter{}
		r := controller.NewSessionCleanupReconciler(k8s, 15*time.Minute, 31*24*time.Hour, emitter, nil, nil)

		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "sess-audit-disc", Namespace: "test-ns"},
		})
		Expect(err).NotTo(HaveOccurred())

		events := emitter.Events()
		Expect(events).To(HaveLen(1))
		Expect(events[0].Type).To(Equal(audit.EventSessionAutoCancelled))
	})

	It("IT-AF-220-007: increments delete counter on retention delete", func() {
		expired := controller.MinRetentionTTL + time.Hour
		sess := makeSession("sess-counter-del", v1alpha1.SessionPhaseCompleted, pastTime(expired), nil)
		k8s := newFakeClient(s, sess)
		sess.Status.CompletedAt = pastTime(expired)
		_ = k8s.Status().Update(ctx, sess)

		ttlActions := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "af_session_ttl_actions_total_it",
		}, []string{"action"})

		r := controller.NewSessionCleanupReconciler(k8s, 15*time.Minute, controller.MinRetentionTTL, nil, ttlActions, nil)

		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "sess-counter-del", Namespace: "test-ns"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(counterValue(ttlActions, "delete")).To(Equal(1.0))
	})
})

var _ = Describe("SessionCleanupReconciler SetupWithManager", func() {
	It("IT-AF-220-008: registers controller for InvestigationSession kind", func() {
		Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
			"KUBEBUILDER_ASSETS must be set — run 'make setup-envtest' first")

		env := &envtest.Environment{
			BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
		}
		cfg, err := env.Start()
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = env.Stop() }()

		s := newTestScheme()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  s,
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())

		r := controller.NewSessionCleanupReconciler(
			mgr.GetClient(), 10*time.Minute, 31*24*time.Hour, nil, nil, nil,
		)
		Expect(r.SetupWithManager(mgr)).To(Succeed())
	})
})

type testAuditEmitter struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (e *testAuditEmitter) Emit(_ context.Context, event *audit.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *testAuditEmitter) Events() []*audit.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := make([]*audit.Event, len(e.events))
	copy(cp, e.events)
	return cp
}

func counterValue(cv *prometheus.CounterVec, labels ...string) float64 {
	m := &dto.Metric{}
	if err := cv.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}
