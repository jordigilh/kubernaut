package integration_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/controller"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

func gaugeValue(g *prometheus.GaugeVec, labels ...string) float64 {
	m := &dto.Metric{}
	gauge, err := g.GetMetricWithLabelValues(labels...)
	if err != nil {
		return 0
	}
	if err := gauge.Write(m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

var _ = Describe("InvestigationSession CRD Lifecycle (IT-SESS)", Ordered, func() {
	var (
		k8sClient      client.Client
		apiReader      client.Reader
		scheme         *runtime.Scheme
		sessAuditor    *testAuditor
		sessionsActive *prometheus.GaugeVec
		ttlActions     *prometheus.CounterVec
		svc            *session.CRDSessionService
		reconciler     *controller.SessionCleanupReconciler
	)

	BeforeAll(func() {
		Expect(itEnvtestCfg).ToNot(BeNil(), "envtest config must be available from suite setup")

		ctrl.SetLogger(logr.Discard())

		scheme = runtime.NewScheme()
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

		mgr, err := ctrl.NewManager(itEnvtestCfg, ctrl.Options{
			Scheme:  scheme,
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).ToNot(HaveOccurred(), "session controller manager must start")

		sessAuditor = &testAuditor{}
		sessionsActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "it_sess_af_sessions_active",
		}, []string{"phase"})
		ttlActions = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "it_sess_af_session_ttl_actions_total",
		}, []string{"action"})

		for _, phase := range []string{"Active", "Disconnected", "Completed", "Cancelled", "Failed"} {
			sessionsActive.WithLabelValues(phase)
		}

		k8sClient = mgr.GetClient()
		apiReader = mgr.GetAPIReader()

		svc = session.NewCRDSessionService(
			adksession.InMemoryService(),
			k8sClient,
			scheme,
			"default",
			session.WithAuditor(sessAuditor),
			session.WithSessionsActive(sessionsActive),
			session.WithAPIReader(apiReader),
		)

		reconciler = controller.NewSessionCleanupReconciler(
			k8sClient,
			1*time.Second,
			controller.MinRetentionTTL,
			sessAuditor,
			ttlActions,
			svc,
		)

		Expect(reconciler.SetupWithManager(mgr)).To(Succeed())

		mgrCtx, mgrCancel := context.WithCancel(context.Background())
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()

		DeferCleanup(func() {
			mgrCancel()
		})

		// Wait for cache sync
		Eventually(func() bool {
			return mgr.GetCache().WaitForCacheSync(context.Background())
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	createSession := func(id string) {
		req := &adksession.CreateRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "sre@kubernaut.ai",
			SessionID: id,
			State: map[string]any{
				session.StateKeyCreateConfig: &session.CreateConfig{
					A2ATaskID: "task-" + id,
					UserIdentity: v1alpha1.SessionUser{
						Username: "sre@kubernaut.ai",
						Groups:   []string{"sre", "platform-eng"},
					},
					JoinMode: v1alpha1.SessionJoinModeStart,
					RemediationRef: v1alpha1.ObjectRef{
						Name:      "rr-payment-api",
						Namespace: "default",
					},
				},
			},
		}
		_, err := svc.Create(context.Background(), req)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
	}

	waitForActive := func(id string) {
		EventuallyWithOffset(1, func() v1alpha1.SessionPhase {
			var crd v1alpha1.InvestigationSession
			if err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name: id, Namespace: "default",
			}, &crd); err != nil {
				return ""
			}
			return crd.Status.Phase
		}, 10*time.Second, 200*time.Millisecond).Should(Equal(v1alpha1.SessionPhaseActive))
	}

	// -------------------------------------------------------------------------
	// IT-SESS-001: Session create persists CRD to API server
	// -------------------------------------------------------------------------
	It("IT-SESS-001: session create persists InvestigationSession CRD to envtest", func() {
		createSession("it-sess-001")

		Eventually(func() v1alpha1.SessionPhase {
			var crd v1alpha1.InvestigationSession
			if err := apiReader.Get(context.Background(), types.NamespacedName{
				Name: "it-sess-001", Namespace: "default",
			}, &crd); err != nil {
				return ""
			}
			return crd.Status.Phase
		}, 5*time.Second, 200*time.Millisecond).Should(Equal(v1alpha1.SessionPhaseActive))
	})

	// -------------------------------------------------------------------------
	// IT-SESS-002: CRD spec fields match CreateConfig input
	// -------------------------------------------------------------------------
	It("IT-SESS-002: CRD spec fields match CreateConfig input", func() {
		createSession("it-sess-002")

		var crd v1alpha1.InvestigationSession
		Eventually(func() error {
			return apiReader.Get(context.Background(), types.NamespacedName{
				Name: "it-sess-002", Namespace: "default",
			}, &crd)
		}, 5*time.Second, 200*time.Millisecond).Should(Succeed())

		Expect(crd.Spec.A2ATaskID).To(Equal("task-it-sess-002"))
		Expect(crd.Spec.UserIdentity.Username).To(Equal("sre@kubernaut.ai"))
		Expect(crd.Spec.UserIdentity.Groups).To(ContainElements("sre", "platform-eng"))
		Expect(crd.Spec.JoinMode).To(Equal(v1alpha1.SessionJoinModeStart))
		Expect(crd.Spec.RemediationRequestRef.Name).To(Equal("rr-payment-api"))
		Expect(crd.Spec.RemediationRequestRef.Namespace).To(Equal("default"))
	})

	// -------------------------------------------------------------------------
	// IT-SESS-003: Phase label matches status.phase on creation
	// -------------------------------------------------------------------------
	It("IT-SESS-003: phase label matches status.phase on creation", func() {
		createSession("it-sess-003")

		var crd v1alpha1.InvestigationSession
		Eventually(func() error {
			return apiReader.Get(context.Background(), types.NamespacedName{
				Name: "it-sess-003", Namespace: "default",
			}, &crd)
		}, 5*time.Second, 200*time.Millisecond).Should(Succeed())

		Expect(crd.Labels).To(HaveKeyWithValue("apifrontend.kubernaut.ai/phase", "Active"))
	})

	// -------------------------------------------------------------------------
	// IT-SESS-004: Session delete removes CRD from API server
	// -------------------------------------------------------------------------
	It("IT-SESS-004: session delete removes CRD from API server", func() {
		createSession("it-sess-004")

		var crd v1alpha1.InvestigationSession
		Eventually(func() error {
			return apiReader.Get(context.Background(), types.NamespacedName{
				Name: "it-sess-004", Namespace: "default",
			}, &crd)
		}, 5*time.Second, 200*time.Millisecond).Should(Succeed())

		err := svc.Delete(context.Background(), &adksession.DeleteRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "sre@kubernaut.ai",
			SessionID: "it-sess-004",
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			err := apiReader.Get(context.Background(), types.NamespacedName{
				Name: "it-sess-004", Namespace: "default",
			}, &crd)
			return err != nil
		}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())
	})

	// -------------------------------------------------------------------------
	// IT-SESS-005: Disconnected session auto-cancels after TTL
	// -------------------------------------------------------------------------
	It("IT-SESS-005: disconnected session auto-cancels after TTL expiry", func() {
		createSession("it-sess-005")
		waitForActive("it-sess-005")

		err := svc.UpdatePhase(context.Background(), "it-sess-005",
			v1alpha1.SessionPhaseDisconnected, "client disconnected", "sre@kubernaut.ai")
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() v1alpha1.SessionPhase {
			var crd v1alpha1.InvestigationSession
			if err := apiReader.Get(context.Background(), types.NamespacedName{
				Name: "it-sess-005", Namespace: "default",
			}, &crd); err != nil {
				return ""
			}
			return crd.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(v1alpha1.SessionPhaseCancelled))
	})

	// -------------------------------------------------------------------------
	// IT-SESS-006: Terminal session deleted after retention TTL
	// -------------------------------------------------------------------------
	It("IT-SESS-006: terminal session deleted after retention TTL", func() {
		createSession("it-sess-006")
		waitForActive("it-sess-006")

		err := svc.UpdatePhase(context.Background(), "it-sess-006",
			v1alpha1.SessionPhaseCompleted, "investigation complete", "sre@kubernaut.ai")
		Expect(err).ToNot(HaveOccurred())

		// Manually backdate completedAt to simulate retention TTL expiry
		var crd v1alpha1.InvestigationSession
		Expect(apiReader.Get(context.Background(), types.NamespacedName{
			Name: "it-sess-006", Namespace: "default",
		}, &crd)).To(Succeed())

		pastTime := metav1.NewTime(time.Now().Add(-(controller.MinRetentionTTL + time.Hour)))
		crd.Status.CompletedAt = &pastTime
		Expect(k8sClient.Status().Update(context.Background(), &crd)).To(Succeed())

		// Trigger reconcile manually since the watch may not re-fire for status-only changes
		_, err = reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "it-sess-006", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			err := apiReader.Get(context.Background(), types.NamespacedName{
				Name: "it-sess-006", Namespace: "default",
			}, &crd)
			return err != nil
		}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())
	})

	// -------------------------------------------------------------------------
	// IT-SESS-007: Gauge increments on create and decrements on delete
	// -------------------------------------------------------------------------
	It("IT-SESS-007: af_sessions_active gauge increments on create and decrements on delete", func() {
		before := gaugeValue(sessionsActive, "Active")

		createSession("it-sess-007")
		Eventually(func() float64 {
			return gaugeValue(sessionsActive, "Active")
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(before + 1))

		err := svc.Delete(context.Background(), &adksession.DeleteRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "sre@kubernaut.ai",
			SessionID: "it-sess-007",
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() float64 {
			return gaugeValue(sessionsActive, "Active")
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(before))
	})

	// -------------------------------------------------------------------------
	// IT-SESS-008: Gauge adjusts on TTL auto-cancel
	// -------------------------------------------------------------------------
	It("IT-SESS-008: gauge adjusts when TTL auto-cancels a session", func() {
		createSession("it-sess-008")
		waitForActive("it-sess-008")

		err := svc.UpdatePhase(context.Background(), "it-sess-008",
			v1alpha1.SessionPhaseDisconnected, "network drop", "sre@kubernaut.ai")
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() float64 {
			return gaugeValue(sessionsActive, "Cancelled")
		}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1))
	})

	// -------------------------------------------------------------------------
	// IT-SESS-009: SessionCreated audit event on create
	// -------------------------------------------------------------------------
	It("IT-SESS-009: audit emitter receives SessionCreated event on create", func() {
		sessAuditor.Reset()
		createSession("it-sess-009")

		Eventually(func() bool {
			for _, e := range sessAuditor.Events() {
				if e.Type == audit.EventSessionCreated {
					return e.Detail["session_id"] == "it-sess-009"
				}
			}
			return false
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	// -------------------------------------------------------------------------
	// IT-SESS-010: SessionAutoCancelled audit event on TTL expiry
	// -------------------------------------------------------------------------
	It("IT-SESS-010: audit emitter receives SessionAutoCancelled on TTL expiry", func() {
		sessAuditor.Reset()
		createSession("it-sess-010")
		waitForActive("it-sess-010")

		err := svc.UpdatePhase(context.Background(), "it-sess-010",
			v1alpha1.SessionPhaseDisconnected, "timeout", "sre@kubernaut.ai")
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() bool {
			for _, e := range sessAuditor.Events() {
				if e.Type == audit.EventSessionAutoCancelled {
					return true
				}
			}
			return false
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	})

	// -------------------------------------------------------------------------
	// IT-SESS-011: Concurrent session creation produces distinct CRDs
	// -------------------------------------------------------------------------
	It("IT-SESS-011: 5 concurrent creates produce 5 distinct CRDs", func() {
		var wg sync.WaitGroup
		errs := make([]error, 5)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()
				id := fmt.Sprintf("it-sess-011-%d", idx)
				req := &adksession.CreateRequest{
					AppName:   "kubernaut-apifrontend",
					UserID:    "sre@kubernaut.ai",
					SessionID: id,
					State: map[string]any{
						session.StateKeyCreateConfig: &session.CreateConfig{
							A2ATaskID:    "task-" + id,
							UserIdentity: v1alpha1.SessionUser{Username: "sre@kubernaut.ai"},
							JoinMode:     v1alpha1.SessionJoinModeStart,
							RemediationRef: v1alpha1.ObjectRef{
								Name: "rr-concurrent", Namespace: "default",
							},
						},
					},
				}
				_, errs[idx] = svc.Create(context.Background(), req)
			}(i)
		}
		wg.Wait()

		for i, e := range errs {
			Expect(e).ToNot(HaveOccurred(), fmt.Sprintf("concurrent create %d failed", i))
		}

		var list v1alpha1.InvestigationSessionList
		Eventually(func() int {
			if err := apiReader.List(context.Background(), &list, client.InNamespace("default"),
				client.MatchingLabels{"app.kubernetes.io/managed-by": "kubernaut-apifrontend"}); err != nil {
				return 0
			}
			count := 0
			for _, s := range list.Items {
				if len(s.Name) > 11 && s.Name[:11] == "it-sess-011" {
					count++
				}
			}
			return count
		}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 5))
	})

	// -------------------------------------------------------------------------
	// IT-SESS-012: Non-RFC-1123 session ID rejected
	// -------------------------------------------------------------------------
	It("IT-SESS-012: non-RFC-1123 session ID rejected with descriptive error", func() {
		req := &adksession.CreateRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "sre@kubernaut.ai",
			SessionID: "UPPER_CASE_ID",
			State: map[string]any{
				session.StateKeyCreateConfig: &session.CreateConfig{
					A2ATaskID:    "task-invalid",
					UserIdentity: v1alpha1.SessionUser{Username: "sre@kubernaut.ai"},
					JoinMode:     v1alpha1.SessionJoinModeStart,
				},
			},
		}
		_, err := svc.Create(context.Background(), req)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(SatisfyAny(
			ContainSubstring("invalid session ID"),
			ContainSubstring("RFC 1123"),
		))

		// Verify no CRD was created
		var crd v1alpha1.InvestigationSession
		err = apiReader.Get(context.Background(), types.NamespacedName{
			Name: "UPPER_CASE_ID", Namespace: "default",
		}, &crd)
		Expect(err).To(HaveOccurred())
	})
})
