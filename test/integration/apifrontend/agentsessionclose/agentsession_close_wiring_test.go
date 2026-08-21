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

package agentsessionclose_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	adksession "google.golang.org/adk/v2/session"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

const testNamespace = "default"

// recordingEmitter is a minimal audit.Emitter recording emitted events for
// assertions (AU-2/AU-3), mirroring the pattern already used by
// internal/controller/apifrontend's testAuditEmitter and the main AF
// integration suite's recordingEmitter, duplicated here because this is a
// deliberately separate, lightweight package (see suite_test.go) that does
// not share the main apifrontend_test package's heavy DS/KA/JWKS bootstrap.
type recordingEmitter struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (e *recordingEmitter) Emit(_ context.Context, event *audit.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *recordingEmitter) eventsOfType(t audit.EventType) []*audit.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []*audit.Event
	for _, ev := range e.events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

// FedRAMP control mapping for this suite (#2214, DD-AA-KA-001 Amendment):
//   AU-2/AU-3 -- audit events, content of audit records: FinalizeSessionByRR
//     -> UpdatePhase emits IS transition audit events with actor attribution;
//     asserted explicitly below rather than assumed "covered elsewhere".
//   CC7.2 -- monitoring/reconstruction: a correlation_id (RR)-driven
//     "AgentSession terminal -> IS terminal" reconstruction must hold with
//     AA removed from the path, proven end-to-end against a real envtest
//     apiserver (not a manual claim).
//   AC-6 -- least privilege: this reconciler needs no RBAC beyond AF's
//     existing get/list/watch on agentsessions.
var _ = Describe("AgentSessionTerminalCloseReconciler wiring (#2214) [AU-2, AU-3, CC7.2]", func() {

	newScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(agentsessionv1.AddToScheme(s)).To(Succeed())
		Expect(isv1alpha1.AddToScheme(s)).To(Succeed())
		return s
	}

	newIS := func(name, rrName string) *isv1alpha1.InvestigationSession {
		return &isv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
			Spec: isv1alpha1.InvestigationSessionSpec{
				A2ATaskID:             "task-2214",
				UserIdentity:          isv1alpha1.SessionUser{Username: "jane.doe"},
				JoinMode:              isv1alpha1.SessionJoinModeStart,
				RemediationRequestRef: isv1alpha1.ObjectRef{Name: rrName, Namespace: testNamespace},
			},
		}
	}

	// createActiveIS creates is and sets its Status.Phase to Active (the
	// only phase ValidateTransition allows a Completed/Failed/Cancelled
	// closure from), mirroring the real MaterializeCRD-created state.
	createActiveIS := func(ctx context.Context, k8sClient client.Client, is *isv1alpha1.InvestigationSession) {
		Expect(k8sClient.Create(ctx, is)).To(Succeed())
		is.Status.Phase = isv1alpha1.SessionPhaseActive
		Expect(k8sClient.Status().Update(ctx, is)).To(Succeed())
	}

	newAgentSession := func(name, rrName string) *agentsessionv1.AgentSession {
		return &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
				// Mirrors production: AA's AgentSessionCreator.GetOrCreate
				// sets this synchronously at creation (#2214 CI RCA, PR
				// #2222) rather than relying solely on the reconciler's own
				// reactive add, which otherwise races an immediate
				// create-then-delete (exactly what IT-AF-2214-002 below
				// does) landing before the reconciler's first reconcile.
				Finalizers: []string{agentsessionv1.TerminalCloseFinalizer},
			},
			Spec: agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: rrName, Namespace: testNamespace},
				IncidentID:            "ai-" + rrName,
				RemediationID:         "rr-uid-" + rrName,
				SignalName:            "OOMKilled",
				Severity:              "warning",
			},
		}
	}

	// startEnv spins up a dedicated envtest + manager with
	// AgentSessionTerminalCloseReconciler registered, returning the client
	// and a recording audit emitter for assertions.
	startEnv := func() (client.Client, *recordingEmitter, func()) {
		Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
			"KUBEBUILDER_ASSETS must be set — run 'make setup-envtest' first")

		env := &envtest.Environment{
			BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
			CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}
		cfg, err := env.Start()
		Expect(err).NotTo(HaveOccurred())

		s := newScheme()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  s,
			Metrics: metricsserver.Options{BindAddress: "0"},
			// Each spec starts its own envtest + manager in the same test
			// binary/process; controller-runtime's global name registry
			// would otherwise reject the second manager's same-named
			// controller (matches the existing IT-AF-220-008 rationale in
			// ttl_controller_test.go for the same underlying reason).
			Controller: ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(session.RegisterFieldIndexes(context.Background(), mgr.GetFieldIndexer())).To(Succeed())

		emitter := &recordingEmitter{}
		svc := session.NewCRDSessionService(
			adksession.InMemoryService(), mgr.GetClient(), s, testNamespace,
			session.WithAuditor(emitter),
			// Matches production wiring (cmd/apifrontend/session_infra.go):
			// UpdatePhase's internal read-modify-write (including the
			// separate label-update round trip) must bypass the informer
			// cache -- otherwise a cache-staleness window between the
			// Status().Update and the follow-up label Update causes a
			// spurious conflict-retry that re-validates the transition
			// against an already-terminal phase and fails, even though the
			// original transition itself succeeded (DD-STATUS-001).
			session.WithAPIReader(mgr.GetAPIReader()),
		)

		r := controller.NewAgentSessionTerminalCloseReconciler(mgr.GetClient(), svc, ctrl.Log.WithName("agentsession-terminal-close"))
		Expect(r.SetupWithManager(mgr)).To(Succeed())

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

		// Wait for the manager to fully stop (not just signal cancellation)
		// before tearing down envtest -- otherwise the next spec's fresh
		// envtest+manager starts while this one's informers/workers are
		// still winding down, under CPU contention that this suite's own
		// flakiness investigation traced to spurious multi-second delays in
		// reconcile delivery (#2214 GREEN-phase hardening).
		stop := func() {
			mgrCancel()
			select {
			case <-mgrDone:
			case <-time.After(30 * time.Second):
			}
			_ = env.Stop()
		}
		return mgr.GetClient(), emitter, stop
	}

	// IT-AF-2214-001: Update-driven Completed/Failed path.
	DescribeTable("IT-AF-2214-001: closes IS to the matching terminal phase when AgentSession.Status.Phase transitions [CC7.2]",
		func(asPhase agentsessionv1.AgentSessionPhase, wantISPhase isv1alpha1.SessionPhase) {
			k8sClient, emitter, stop := startEnv()
			defer stop()
			ctx := context.Background()

			rrName := fmt.Sprintf("rr-2214-%s", strings.ToLower(string(asPhase)))
			asName := "as-" + rrName
			isName := "is-" + rrName

			is := newIS(isName, rrName)
			createActiveIS(ctx, k8sClient, is)

			as := newAgentSession(asName, rrName)
			Expect(k8sClient.Create(ctx, as)).To(Succeed())

			// Retry-on-conflict, re-Get'ing fresh each attempt: the
			// reconciler's own terminalCloseFinalizer add (#2214 finalizer
			// redesign) races this Status write for the same object's
			// shared resourceVersion, exactly like KA's production
			// updateStatus helper (status_writer.go) already retries
			// against -- this test must tolerate the same, real,
			// production-shaped conflict rather than assuming a single
			// attempt always wins.
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: asName, Namespace: testNamespace}, as)).To(Succeed())
				as.Status.Phase = asPhase
				g.Expect(k8sClient.Status().Update(ctx, as)).To(Succeed())
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

			Eventually(func(g Gomega) {
				var fetched isv1alpha1.InvestigationSession
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: isName, Namespace: testNamespace}, &fetched)).To(Succeed())
				g.Expect(fetched.Status.Phase).To(Equal(wantISPhase))
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed(), "IS must reach the terminal phase correlated to the AgentSession")

			// AU-2/AU-3: FinalizeSessionByRR -> UpdatePhase must emit an
			// audit event for this closure, not just perform the write.
			Eventually(func() []*audit.Event {
				return emitter.eventsOfType(audit.EventSessionPhaseChanged)
			}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty(),
				"closing IS via the new AgentSession-watch path must still emit an audit event (AU-2/AU-3)")
		},
		Entry("Completed", agentsessionv1.AgentSessionPhaseCompleted, isv1alpha1.SessionPhaseCompleted),
		Entry("Failed", agentsessionv1.AgentSessionPhaseFailed, isv1alpha1.SessionPhaseFailed),
	)

	// IT-AF-2214-002: Delete-driven Cancelled path, proven end-to-end against
	// a real envtest apiserver. Originally written against a raw
	// handler.Funcs.DeleteFunc capture design; that design's delete event
	// was reproducibly dropped under CPU contention (CI RCA, PR #2222, run
	// 32513171970, reproduced 3/13 locally against the exact CI-built images
	// under --race --procs=4) and was replaced by the terminalCloseFinalizer
	// pattern (internal/controller/apifrontend/agentsession_close.go) -- this
	// test's assertions are unchanged, since the finalizer redesign is an
	// internal delivery-reliability fix with no change to the observable
	// Create-then-Delete-then-IS-Cancelled contract asserted below.
	It("IT-AF-2214-002: closes IS to Cancelled when the AgentSession is deleted [CC7.2]", func() {
		k8sClient, emitter, stop := startEnv()
		defer stop()
		ctx := context.Background()

		const rrName = "rr-2214-delete"
		const asName = "as-" + rrName
		const isName = "is-" + rrName

		is := newIS(isName, rrName)
		createActiveIS(ctx, k8sClient, is)

		as := newAgentSession(asName, rrName)
		Expect(k8sClient.Create(ctx, as)).To(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: asName, Namespace: testNamespace}, as)).To(Succeed())
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Delete(ctx, as)).To(Succeed())

		Eventually(func(g Gomega) {
			var fetched isv1alpha1.InvestigationSession
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: isName, Namespace: testNamespace}, &fetched)).To(Succeed())
			g.Expect(fetched.Status.Phase).To(Equal(isv1alpha1.SessionPhaseCancelled))
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed(), "IS must reach Cancelled when the correlated AgentSession is deleted")

		Eventually(func() []*audit.Event {
			return emitter.eventsOfType(audit.EventSessionPhaseChanged)
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty(),
			"closing IS via AgentSession deletion must still emit an audit event (AU-2/AU-3)")
	})
})
