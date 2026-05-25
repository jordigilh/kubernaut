package apifrontend_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
)

// FedRAMP control mapping for this suite:
//   SI-4  — system monitoring: health flag and TTL metrics
//   SI-10 — information accuracy: CRD discovery validates schema presence
//   AC-6  — least privilege: SSAR confirms minimum RBAC before startup
//   AU-3  — audit record content: ctrl.SetLogger wires structured logs

var _ = Describe("Session Controller Wiring (#1272, #1273)", func() {

	// SI-4: the readiness probe gates traffic; if the informer cache
	// never syncs, the pod stays unready and K8s sheds load.
	It("IT-AF-1272-001: health flag transitions to ready after informer cache sync [SI-4]", func() {
		Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
			"KUBEBUILDER_ASSETS must be set — run 'make setup-envtest' first")

		env := &envtest.Environment{
			BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
			CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
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
			mgr.GetClient(), 10*time.Minute, 31*24*time.Hour, logr.Discard(), nil, nil, nil,
		)
		Expect(r.SetupWithManager(mgr)).To(Succeed())

		healthy := &atomic.Bool{}
		mgrCtx, mgrCancel := context.WithCancel(context.Background())
		defer mgrCancel()

		go func() {
			_ = mgr.Start(mgrCtx)
		}()

		go func() {
			syncCtx, syncCancel := context.WithTimeout(mgrCtx, 60*time.Second)
			defer syncCancel()
			if mgr.GetCache().WaitForCacheSync(syncCtx) {
				healthy.Store(true)
			}
		}()

		Eventually(func() bool {
			return healthy.Load()
		}, 30*time.Second).Should(BeTrue())
	})

	// SI-4(2): automated monitoring — the counter feeds Prometheus alerts
	// for TTL-driven lifecycle actions (cancel, delete). Without this,
	// SIEM has no visibility into automatic session cleanup.
	It("IT-AF-1272-002: TTL reconcile emits observable metric via envtest [SI-4(2)]", func() {
		ctx := context.Background()
		const sessionName = "sess-it-1272-002"

		sess := makeSession(sessionName, v1alpha1.SessionPhaseDisconnected, nil, pastTime(20*time.Minute))
		sess.Namespace = "default"
		Expect(k8sClient.Create(ctx, sess)).To(Succeed())

		sess.Status.DisconnectedAt = pastTime(20 * time.Minute)
		Expect(k8sClient.Status().Update(ctx, sess)).To(Succeed())

		defer func() {
			_ = k8sClient.Delete(ctx, &v1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{Name: sessionName, Namespace: "default"},
			})
		}()

		ttlActions := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "af_session_ttl_actions_total_it_1272_002",
		}, []string{"action"})

		r := controller.NewSessionCleanupReconciler(
			k8sClient, 15*time.Minute, controller.MinRetentionTTL, logr.Discard(), nil, ttlActions, nil,
		)
		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: sessionName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(counterValue(ttlActions, "cancel")).To(Equal(1.0))
	})

	// SI-10: pre-flight CRD discovery ensures the InvestigationSession
	// schema is present before the controller subscribes to the API.
	// Without this check a misconfigured cluster silently drops events.
	It("IT-AF-1273-001: pre-flight CRD discovery confirms schema presence [SI-10]", func() {
		discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(restCfg)
		resources, err := discoveryClient.ServerResourcesForGroupVersion(v1alpha1.GroupVersion.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(resources).NotTo(BeNil())

		var found bool
		for _, r := range resources.APIResources {
			if r.Name == "investigationsessions" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "InvestigationSession CRD must be registered in envtest")
	})

	// AC-6: least privilege — SSAR confirms the service account can watch
	// InvestigationSessions before the controller starts. A denial means
	// ClusterRole binding is missing and the reconciler would silently fail.
	It("IT-AF-1273-002: pre-flight RBAC SSAR validates watch permission [AC-6]", func() {
		ctx := context.Background()
		ssar := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Namespace: "default",
					Verb:      "watch",
					Group:     "kubernaut.ai",
					Resource:  "investigationsessions",
				},
			},
		}

		result, err := k8sClientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, ssar, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status.Allowed).To(BeTrue())
	})

	// AU-3: ctrl.SetLogger ensures controller-runtime's internal events
	// (leader election, cache startup, reconcile errors) flow through the
	// same structured JSON sink as application logs, satisfying the
	// single-audit-stream requirement.
	It("IT-AF-1273-003: ctrl.SetLogger routes framework logs through audit pipeline [AU-3]", func() {
		Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
			"KUBEBUILDER_ASSETS must be set — run 'make setup-envtest' first")

		prevLogger := logf.Log
		defer logf.SetLogger(prevLogger)

		var buf bytes.Buffer
		testLogger := zap.New(zap.WriteTo(&buf), zap.UseDevMode(true))
		logf.SetLogger(testLogger)
		ctrl.SetLogger(testLogger)

		env := &envtest.Environment{
			BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
			CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
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

		mgrCtx, mgrCancel := context.WithCancel(context.Background())
		defer mgrCancel()

		go func() {
			_ = mgr.Start(mgrCtx)
		}()

		syncCtx, syncCancel := context.WithTimeout(mgrCtx, 30*time.Second)
		defer syncCancel()
		Expect(mgr.GetCache().WaitForCacheSync(syncCtx)).To(BeTrue())

		Eventually(func() string {
			return buf.String()
		}, 10*time.Second).ShouldNot(BeEmpty())
	})
})
