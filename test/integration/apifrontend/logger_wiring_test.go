package apifrontend_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

func newFuncrLogger(buf *bytes.Buffer) logr.Logger {
	return funcr.New(func(prefix, args string) {
		buf.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{Verbosity: 10})
}

// FedRAMP AU-3 (audit record content): every component that emits
// security-relevant events must route through the logr -> zapr -> JSON
// pipeline. These ITs prove the wiring by capturing output via funcr
// and asserting that business events appear in the captured buffer.

var _ = Describe("Logger Wiring (#1274)", func() {

	// AU-3: TTL cancel is a security-relevant lifecycle event (session
	// destroyed). The reconciler must emit it through the injected logger
	// so the JSON sink and SIEM scraper can ingest it.
	It("IT-AF-1274-001: TTL reconciler emits cancel event through injected logr [AU-3]", func() {
		ctx := context.Background()
		const sessionName = "sess-it-1274-001"

		sess := makeSession(sessionName, v1alpha1.SessionPhaseDisconnected, nil, pastTime(20*time.Minute))
		sess.Namespace = "default"
		Expect(k8sClient.Create(ctx, sess)).To(Succeed())

		sess.Status.Phase = v1alpha1.SessionPhaseDisconnected
		sess.Status.DisconnectedAt = pastTime(20 * time.Minute)
		Expect(k8sClient.Status().Update(ctx, sess)).To(Succeed())

		defer func() {
			_ = k8sClient.Delete(ctx, &v1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{Name: sessionName, Namespace: "default"},
			})
		}()

		Eventually(func(g Gomega) {
			var fetched v1alpha1.InvestigationSession
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sessionName, Namespace: "default"}, &fetched)).To(Succeed())
			g.Expect(fetched.Status.Phase).To(Equal(v1alpha1.SessionPhaseDisconnected))
			g.Expect(fetched.Status.DisconnectedAt).NotTo(BeNil(), "status update must propagate")
		}, 10*time.Second, 200*time.Millisecond).Should(Succeed())

		var buf bytes.Buffer
		logger := newFuncrLogger(&buf)

		r := controller.NewSessionCleanupReconciler(
			k8sClient, 15*time.Minute, controller.MinRetentionTTL, logger, nil, nil, nil,
		)
		_, err := r.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: sessionName, Namespace: "default"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(buf.String()).To(ContainSubstring("session auto-cancelled"))
	})

	// AU-3: session creation is an auditable event. The service must
	// log "session created" through logr so the JSON sink captures
	// the sessionID, userID, and timestamp for compliance reporting.
	It("IT-AF-1274-002: CRDSessionService emits create event through injected logr [AU-3]", func() {
		var buf bytes.Buffer
		logger := newFuncrLogger(&buf)

		svc := session.NewCRDSessionService(
			adksession.InMemoryService(),
			k8sClient,
			scheme,
			"default",
			session.WithLogger(logger),
		)

		_, err := svc.Create(context.Background(), sessionCreateRequest("1274-002", "logger-test-user"))
		Expect(err).NotTo(HaveOccurred())
		Expect(buf.String()).To(ContainSubstring("session created"))
	})

	// AU-3: A2A task launch is auditable. The launcher must accept a
	// logr.Logger so task lifecycle events flow through the JSON sink.
	It("IT-AF-1274-003: A2A launcher accepts logr without panic [AU-3]", func() {
		logger := newFuncrLogger(&bytes.Buffer{})

		rootAgent, _, err := agentpkg.NewRootAgent(agentpkg.DefaultTestConfig())
		Expect(err).NotTo(HaveOccurred())

		cfg := launcher.A2AConfig{
			Agent:          rootAgent,
			SessionService: adksession.InMemoryService(),
			AppName:        "kubernaut-apifrontend-it",
			Logger:         logger,
		}

		var h http.Handler
		Expect(func() {
			var buildErr error
			h, buildErr = launcher.NewA2AHandler(cfg)
			Expect(buildErr).NotTo(HaveOccurred())
		}).NotTo(Panic())
		Expect(h).NotTo(BeNil())
		Expect(cfg.Logger.GetSink()).NotTo(BeNil())
	})

	// CM-3(5): configuration change monitoring. The FileWatcher must log
	// reload/reject events through logr so configuration drift is auditable.
	It("IT-AF-1274-004: FileWatcher emits reload event through injected logr [CM-3(5)]", func() {
		dir, err := os.MkdirTemp("", "af-logger-wiring-*")
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = os.RemoveAll(dir) }()

		cfgPath := filepath.Join(dir, "config.yaml")
		initial := []byte("logging:\n  level: INFO\n")
		Expect(os.WriteFile(cfgPath, initial, 0o644)).To(Succeed())

		var buf bytes.Buffer
		logger := newFuncrLogger(&buf)

		w, err := config.NewFileWatcher(cfgPath, func([]byte) error { return nil }, config.WithLogger(logger))
		Expect(err).NotTo(HaveOccurred())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Expect(w.Start(ctx)).To(Succeed())
		defer w.Stop()

		updated := append(initial, '\n')
		Expect(os.WriteFile(cfgPath, updated, 0o644)).To(Succeed())

		Eventually(func() string {
			return buf.String()
		}, 5*time.Second, 100*time.Millisecond).Should(ContainSubstring("config reloaded"))

		Expect(buf.String()).NotTo(ContainSubstring("level=INFO"))
	})
})
