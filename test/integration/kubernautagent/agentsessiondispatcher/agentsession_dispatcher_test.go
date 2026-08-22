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

package agentsessiondispatcher_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

const testNamespace = "default"

// fakeInvestigationRunner duplicates
// internal/kubernautagent/agentsession/dispatcher_test.go's identical test
// double -- this is a deliberately separate package (see suite_test.go)
// exercising a real envtest apiserver rather than a fake client, so it does
// not import that internal test package's unexported helpers.
type fakeInvestigationRunner struct {
	calls  atomic.Int32
	delay  time.Duration
	result *katypes.InvestigationResult
	err    error
}

func (f *fakeInvestigationRunner) Investigate(ctx context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.result, f.err
}

// dispatcherClientScheme mirrors cmd/kubernautagent/agentsession_wiring.go's
// buildAgentSessionScheme: the built-in K8s scheme (coordination/v1 Lease)
// plus AgentSession and InvestigationSession, used for the Dispatcher's own
// raw, uncached client -- kept deliberately distinct from the Manager's
// scheme (managerScheme, below), matching production's split.
func dispatcherClientScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentsessionv1.AddToScheme(scheme))
	utilruntime.Must(isv1alpha1.AddToScheme(scheme))
	return scheme
}

// managerScheme mirrors cmd/kubernautagent/agentsession_wiring.go's
// buildAgentSessionManagerScheme: only AgentSession is needed since the
// Manager's cache exclusively drives Reconcile dispatch (watch ->
// workqueue -> Reconcile), never direct reads/writes -- see dispatcher.go's
// Dispatcher doc comment.
func managerScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(agentsessionv1.AddToScheme(scheme))
	return scheme
}

func newAgentSession(name string) *agentsessionv1.AgentSession {
	return &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: agentsessionv1.AgentSessionSpec{
			RemediationRequestRef: agentsessionv1.ObjectRef{Name: "rr-" + name, Namespace: testNamespace},
			IncidentID:            "incident-" + name,
			RemediationID:         "rr-" + name,
			SignalName:            "OOMKilled",
			Severity:              "critical",
		},
	}
}

// startEnv spins up a dedicated envtest apiserver, the Dispatcher's own raw
// crclient.WithWatch client, a controller-runtime Manager with the
// Dispatcher registered as its sole Reconciler (SetupWithManager), and a
// session.Manager with the Dispatcher's hooks wired -- mirroring
// cmd/kubernautagent/agentsession_wiring.go's startAgentSessionDispatcher
// end to end (#2231 / DD-AA-KA-001 Amendment). Each spec gets its own
// envtest+Manager instance (SkipNameValidation, matching
// agentsessionclose_test's identical rationale: controller-runtime's
// global controller-name registry otherwise rejects a second
// "agentsession-dispatcher" registration in the same test binary).
func startEnv(runner agentsession.InvestigationRunner) (crclient.WithWatch, *session.Manager, func()) {
	Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
		"KUBEBUILDER_ASSETS must be set — run 'make setup-envtest' first")

	env := &envtest.Environment{
		BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	Expect(err).NotTo(HaveOccurred())

	cli, err := crclient.NewWithWatch(cfg, crclient.Options{Scheme: dispatcherClientScheme()})
	Expect(err).NotTo(HaveOccurred())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  managerScheme(),
		Metrics: metricsserver.Options{BindAddress: "0"},
		// See this function's doc comment: each spec starts its own
		// envtest+Manager pair, registering the same-named controller
		// ("agentsession-dispatcher", dispatcher.go's SetupWithManager)
		// more than once in this test binary's process-wide registry.
		Controller: ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	Expect(err).NotTo(HaveOccurred())

	sessions := session.NewManager(session.NewStore(5*time.Minute), logr.Discard(), nil, kametrics.NewMetrics())
	dispatcher := agentsession.NewDispatcher(cli, testNamespace, "replica-it", sessions, runner, logr.Discard(),
		agentsession.WithResyncInterval(200*time.Millisecond))
	sessions.SetTerminalHook(dispatcher.OnTerminal)
	sessions.SetInteractiveUpgradeHook(dispatcher.OnInteractiveUpgrade)

	Expect(dispatcher.SetupWithManager(mgr)).To(Succeed())

	healthy := &atomic.Bool{}
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	mgrDone := make(chan struct{})
	go func() {
		defer close(mgrDone)
		_ = mgr.Start(mgrCtx)
	}()
	go func() {
		syncCtx, syncCancel := context.WithTimeout(mgrCtx, 60*time.Second)
		defer syncCancel()
		if mgr.GetCache().WaitForCacheSync(syncCtx) {
			healthy.Store(true)
		}
	}()
	Eventually(func() bool { return healthy.Load() }, 30*time.Second).Should(BeTrue())

	// Mirrors agentsessionclose_test's identical rationale: wait for the
	// Manager to fully stop, not just signal cancellation, before tearing
	// down envtest, so the next spec's fresh envtest+Manager does not race
	// this one's still-winding-down informers/workers.
	stop := func() {
		mgrCancel()
		select {
		case <-mgrDone:
		case <-time.After(30 * time.Second):
		}
		_ = env.Stop()
	}
	return cli, sessions, stop
}

// FedRAMP control mapping for this suite (#2231, DD-AA-KA-001 Amendment):
//
//	AU-2/CC7.2 -- monitoring/reconstruction: this suite proves the
//	  workqueue-backed delete path (not an at-most-once raw watch.Deleted
//	  event) reliably stops an in-flight investigation, closing the
//	  reliability gap this amendment addresses.
//	AC-6 -- least privilege: the Dispatcher's finalizer add/remove is a
//	  metadata-only write on the base AgentSession resource, matching the
//	  scoped RBAC grant in charts/kubernaut/templates/kubernaut-agent/
//	  kubernaut-agent.yaml.
var _ = Describe("AgentSession Dispatcher Reconciler wiring (#2231) [AU-2, CC7.2, AC-6]", func() {
	It("IT-AA-2231-001: deleting a dispatched AgentSession against a real API server reliably stops the in-memory investigation via the finalizer-driven Reconcile path [CC7.2]", func() {
		runner := &fakeInvestigationRunner{delay: 30 * time.Second, result: &katypes.InvestigationResult{RCASummary: "should never complete"}}
		cli, sessions, stop := startEnv(runner)
		defer stop()
		ctx := context.Background()

		as := newAgentSession("as-2231-it-1")
		Expect(cli.Create(ctx, as)).To(Succeed())
		key := crclient.ObjectKeyFromObject(as)

		Eventually(func() agentsessionv1.AgentSessionPhase {
			got := &agentsessionv1.AgentSession{}
			if err := cli.Get(ctx, key, got); err != nil {
				return ""
			}
			return got.Status.Phase
		}, 10*time.Second, 100*time.Millisecond).Should(Equal(agentsessionv1.AgentSessionPhaseInvestigating),
			"the real Manager-driven Reconciler must dispatch a freshly-Created AgentSession without any manual Reconcile call")

		var sessionID string
		Eventually(func() bool {
			id, ok := sessions.FindByRemediationID(as.Spec.RemediationID)
			if ok {
				sessionID = id
			}
			return ok
		}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "the investigation must actually be running before delete is exercised")

		got := &agentsessionv1.AgentSession{}
		Expect(cli.Get(ctx, key, got)).To(Succeed())
		Expect(got.GetFinalizers()).To(ContainElement(agentsessionv1.DispatchCleanupFinalizer),
			"the Reconciler's earlier pass must have added dispatchCleanupFinalizer before this delete is exercised")
		Expect(cli.Delete(ctx, got)).To(Succeed())

		Eventually(func() session.Status {
			s, err := sessions.GetSession(sessionID)
			if err != nil {
				return ""
			}
			return s.Status
		}, 10*time.Second, 100*time.Millisecond).Should(Equal(session.StatusCancelled),
			"BR-AA-KA-065 (delete-reliability, #2231): the workqueue-backed delete Reconcile must stop the in-memory investigation session")

		Eventually(func() bool {
			afterDelete := &agentsessionv1.AgentSession{}
			err := cli.Get(ctx, key, afterDelete)
			return apierrors.IsNotFound(err)
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue(),
			"once dispatchCleanupFinalizer is removed by Reconcile, the real API server must let the deletion actually complete")
	})

	It("IT-AA-2231-002: the Dispatcher Reconciler, registered via SetupWithManager on a real Manager, actually dispatches a freshly-Created AgentSession to Completed [AC-6]", func() {
		runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "wiring proof", Confidence: 0.8}}
		cli, _, stop := startEnv(runner)
		defer stop()
		ctx := context.Background()

		as := newAgentSession("as-2231-it-2")
		Expect(cli.Create(ctx, as)).To(Succeed())
		key := crclient.ObjectKeyFromObject(as)

		Eventually(func() agentsessionv1.AgentSessionPhase {
			got := &agentsessionv1.AgentSession{}
			if err := cli.Get(ctx, key, got); err != nil {
				return ""
			}
			return got.Status.Phase
		}, 10*time.Second, 100*time.Millisecond).Should(Equal(agentsessionv1.AgentSessionPhaseCompleted),
			"production wiring (SetupWithManager + a real Manager) must dispatch and complete this AgentSession without any test-driven Reconcile() call")

		got := &agentsessionv1.AgentSession{}
		Expect(cli.Get(ctx, key, got)).To(Succeed())
		Expect(got.Status.Result).NotTo(BeNil())
		Expect(got.Status.Result.Analysis).To(Equal("wiring proof"))
		Expect(int(runner.calls.Load())).To(Equal(1))
		Expect(got.GetFinalizers()).To(ContainElement(agentsessionv1.DispatchCleanupFinalizer),
			fmt.Sprintf("dispatchCleanupFinalizer must be present on %s even after reaching a terminal phase (removed only on delete)", got.Name))
	})
})
