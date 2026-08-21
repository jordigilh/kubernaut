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

package agentsession_test

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// fakeInvestigationRunner is a test double for agentsession.InvestigationRunner
// that counts invocations and returns a configurable, possibly-delayed result.
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

func newTestDispatcherClient() crclient.WithWatch {
	scheme := runtime.NewScheme()
	Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
	Expect(agentsessionv1.AddToScheme(scheme)).To(Succeed())
	Expect(isv1.AddToScheme(scheme)).To(Succeed())
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&agentsessionv1.AgentSession{}).
		Build()
}

func newTestSessionManager() *session.Manager {
	store := session.NewStore(5 * time.Minute)
	return session.NewManager(store, logr.Discard(), nil, kametrics.NewMetrics())
}

// wireHooks mirrors cmd/kubernautagent/agentsession_wiring.go's production
// registration of session.Manager's TerminalHook/InteractiveUpgradeHook
// (DD-AA-KA-001 Amendment Gap 2) onto one or more test Dispatcher
// instances. When multiple dispatchers share one Manager (the
// two-racing-replicas test), each dispatcher's own OnTerminal is a safe
// no-op log when its own remediationID->ObjectKey map has no entry, so
// trying all of them finds whichever one actually won the dispatch.
func wireHooks(mgr *session.Manager, dispatchers ...*agentsession.Dispatcher) {
	mgr.SetTerminalHook(func(snap session.TerminalSnapshot) {
		for _, d := range dispatchers {
			d.OnTerminal(snap)
		}
	})
	mgr.SetInteractiveUpgradeHook(func(sessionID, remediationID, username string, groups []string) {
		for _, d := range dispatchers {
			d.OnInteractiveUpgrade(sessionID, remediationID, username, groups)
		}
	})
}

func newPendingAgentSession(name string) *agentsessionv1.AgentSession {
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

// BR-AA-KA-065.3/.4: KA dispatches exactly once per AgentSession via the
// dispatch Lease, and exclusively writes Status through the full
// Pending -> Investigating -> Completed/Failed lifecycle.
var _ = Describe("Dispatcher — BR-AA-KA-065.3/.4 dispatch + Status lifecycle", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		cli    crclient.WithWatch
		mgr    *session.Manager
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		cli = newTestDispatcherClient()
		mgr = newTestSessionManager()
	})

	AfterEach(func() {
		cancel()
	})

	Describe("UT-AA-KA-065-020: a Pending, non-interactive AgentSession is dispatched and transitions to Completed", func() {
		It("should win the dispatch Lease, run the investigation, and write the curated Result", func() {
			runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "leak found", Confidence: 0.9}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession("as-1")
			Expect(cli.Create(ctx, as)).To(Succeed())

			key := crclient.ObjectKeyFromObject(as)
			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted))

			got := &agentsessionv1.AgentSession{}
			Expect(cli.Get(ctx, key, got)).To(Succeed())
			Expect(got.Status.SessionID).NotTo(BeEmpty())
			Expect(got.Status.DispatchedAt).NotTo(BeNil())
			Expect(got.Status.CompletedAt).NotTo(BeNil())
			Expect(got.Status.Result).NotTo(BeNil())
			Expect(got.Status.Result.Analysis).To(Equal("leak found"))
			Expect(int(runner.calls.Load())).To(Equal(1))

			lease := &coordinationv1.Lease{}
			Expect(cli.Get(ctx, crclient.ObjectKey{Name: "dispatch-as-1", Namespace: testNamespace}, lease)).To(Succeed())
			Expect(*lease.Spec.HolderIdentity).To(Equal("replica-a"))
		})
	})

	Describe("IT-AA-2170-DISPATCH-LEASE-NAME: a long AgentSession name that would truncate the dispatch Lease name to a trailing separator", func() {
		It("should still produce a valid DNS-1123 subdomain Lease name and dispatch successfully", func() {
			// "dispatch-" (9 chars) + this 58-char name is exactly 67 chars,
			// and the naive name[:63] cut used to land exactly on a "-"
			// (reproduces the real name shape seen live on helios08:
			// "as-test-remediation-hr-<19-digit>-<10-digit>-26" truncated
			// mid-separator by dispatchLeaseName, rejected by the API
			// server as an invalid metadata.name, and retried forever on
			// every resync -- IT-AA #2170).
			longName := "as-test-remediation-hr-1787063451227311451-1787063336-26"
			runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "ok", Confidence: 0.8}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession(longName)
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted),
				"dispatch must not get permanently stuck retrying an invalid Lease name every resync")

			leaseList := &coordinationv1.LeaseList{}
			Expect(cli.List(ctx, leaseList, crclient.InNamespace(testNamespace))).To(Succeed())
			Expect(leaseList.Items).To(HaveLen(1))
			Expect(validation.IsDNS1123Subdomain(leaseList.Items[0].Name)).To(BeEmpty(),
				"the dispatch Lease name must be a valid DNS-1123 subdomain (start/end alphanumeric)")
		})
	})

	Describe("UT-AA-KA-065-025 / BR-AI-009: dispatch rejected by KA's own per-process capacity cap is tagged CapacityExceeded, distinct from any other dispatch failure", func() {
		It("should set Status.Reason=CapacityExceeded when session.Manager.StartInvestigationWithContext returns ErrMaxInvestigationsReached", func() {
			// A dedicated, capacity-capped Store/Manager (WithMaxConcurrent(1)):
			// the first AgentSession's investigation is held open by
			// fakeInvestigationRunner's delay so a second, concurrently-
			// dispatched AgentSession deterministically hits the cap instead
			// of racing it.
			cappedStore := session.NewStore(5*time.Minute, session.WithMaxConcurrent(1))
			cappedMgr := session.NewManager(cappedStore, logr.Discard(), nil, kametrics.NewMetrics())

			runner := &fakeInvestigationRunner{delay: 2 * time.Second, result: &katypes.InvestigationResult{RCASummary: "first, still running"}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", cappedMgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(cappedMgr, d)
			go d.Start(ctx)

			first := newPendingAgentSession("as-cap-1")
			Expect(cli.Create(ctx, first)).To(Succeed())
			firstKey := crclient.ObjectKeyFromObject(first)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, firstKey, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseInvestigating),
				"the first AgentSession must occupy the capped Manager's sole concurrent slot before the second is created")

			second := newPendingAgentSession("as-cap-2")
			Expect(cli.Create(ctx, second)).To(Succeed())
			secondKey := crclient.ObjectKeyFromObject(second)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, secondKey, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseFailed))

			got := &agentsessionv1.AgentSession{}
			Expect(cli.Get(ctx, secondKey, got)).To(Succeed())
			Expect(got.Status.Reason).To(Equal(agentsessionv1.AgentSessionReasonCapacityExceeded),
				"a capacity-exceeded dispatch failure must be distinguishable (via Status.Reason) from a genuine investigation failure so AA can retry it")
			Expect(got.Status.Error).NotTo(BeEmpty(), "Status.Error must still carry the curated, user-facing message")

			// BR-AI-009 / DD-AA-KA-001 amendment Gap 6 (2026-08-20): the
			// investigation never started for "as-cap-2" (rejected before
			// session.Manager accepted it), so its dispatch Lease must not
			// survive the failed attempt -- otherwise AA's retry (delete +
			// recreate an AgentSession with this same name) races a stale-
			// but-not-yet-expired Lease (dispatchLeaseDuration=15m) and
			// tryDispatch's tryReclaimStaleLease silently treats it as a
			// lost race, permanently blocking the retry for up to 15
			// minutes (confirmed live: E2E-AA-065 CI run 2026-08-20,
			// AgentSession Status.Phase stuck at "" for the full 300s test
			// window with zero further dispatch attempts logged).
			//
			// #2189 CI flake follow-up (2026-08-20): this must be an
			// Eventually, not a bare synchronous Get. This test's
			// WithResyncInterval(20ms) is ~1500x faster than production's
			// defaultResyncInterval (30s), deliberately so the Eventually
			// above observes Phase=Failed quickly -- but that same fast
			// tick means a resync call that snapshotted "as-cap-2" as
			// still-non-terminal a moment *before* the winning tryDispatch's
			// writeFailedStatus committed can still be in flight when this
			// assertion runs: it re-acquires a lease (the old one was just
			// deleted), re-fetches the AgentSession, now observes it as
			// terminal via tryDispatch's own isTerminalPhase branch, and
			// deletes the lease again -- a self-correcting oscillation that
			// always converges to "no lease", just not necessarily by the
			// exact instant this line runs. A bare Get can catch that
			// re-created-but-not-yet-cleaned-up lease mid-oscillation
			// (observed once in CI, unreproduced in 25/25 local runs
			// including --race --procs=4 matching CI); this changes no
			// production code path (production's 30s resync makes the
			// window negligible), only how patiently the test itself waits.
			Eventually(func() bool {
				lease := &coordinationv1.Lease{}
				leaseErr := cli.Get(ctx, crclient.ObjectKey{Name: "dispatch-as-cap-2", Namespace: testNamespace}, lease)
				return apierrors.IsNotFound(leaseErr)
			}, "1s", "10ms").Should(BeTrue(),
				"a rejected (never-started) dispatch must release its Lease promptly, not hold it for the full stale-Lease window")
		})
	})

	Describe("UT-AA-KA-065-026 / BR-AI-009: a capacity-exceeded AgentSession deleted and recreated under the same name dispatches promptly once capacity frees up", func() {
		It("should not be blocked by a stale dispatch Lease left behind by the earlier rejected attempt", func() {
			// Mirrors the real AA retry flow (InvestigatingHandler.retryCapacityExceeded
			// -> AgentSessionCreator.DeleteForRetry -> GetOrCreate): a fresh
			// AgentSession object with the SAME NAME as the one that was
			// just rejected for capacity.
			cappedStore := session.NewStore(5*time.Minute, session.WithMaxConcurrent(1))
			cappedMgr := session.NewManager(cappedStore, logr.Discard(), nil, kametrics.NewMetrics())

			runner := &fakeInvestigationRunner{delay: 50 * time.Millisecond, result: &katypes.InvestigationResult{RCASummary: "first"}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", cappedMgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(cappedMgr, d)
			go d.Start(ctx)

			first := newPendingAgentSession("as-retry-1")
			Expect(cli.Create(ctx, first)).To(Succeed())
			firstKey := crclient.ObjectKeyFromObject(first)
			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, firstKey, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseInvestigating),
				"the first AgentSession must occupy the capped Manager's sole concurrent slot before the second is created")

			rejected := newPendingAgentSession("as-retry-2")
			Expect(cli.Create(ctx, rejected)).To(Succeed())
			rejectedKey := crclient.ObjectKeyFromObject(rejected)
			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, rejectedKey, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseFailed),
				"the second AgentSession must be capacity-rejected while the first is still occupying the sole slot")

			// AA's retry: delete the rejected AgentSession, then recreate a
			// fresh one under the identical name -- exactly what
			// AgentSessionCreator.DeleteForRetry + the next GetOrCreate does.
			Expect(cli.Delete(ctx, rejected)).To(Succeed())
			// The first investigation finishes on its own (50ms delay),
			// freeing the sole slot for the retry attempt.
			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, firstKey, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted))

			retry := newPendingAgentSession("as-retry-2")
			Expect(cli.Create(ctx, retry)).To(Succeed())

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, rejectedKey, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted),
				"the retried AgentSession must dispatch promptly once capacity frees up, not be blocked for the full stale-Lease window by the earlier rejected attempt's leftover Lease")
		})
	})

	Describe("UT-AA-KA-065-021: a failed investigation writes a curated Failed status", func() {
		It("should set Phase=Failed with a curated, non-raw error message", func() {
			runner := &fakeInvestigationRunner{err: errors.New("llm timeout: raw internal detail")}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession("as-2")
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseFailed))

			got := &agentsessionv1.AgentSession{}
			Expect(cli.Get(ctx, key, got)).To(Succeed())
			Expect(got.Status.Error).To(ContainSubstring("investigation failed"))
			Expect(got.Status.Result).To(BeNil())
		})
	})

	Describe("UT-AA-KA-065-022: two replicas racing the same AgentSession dispatch exactly once", func() {
		It("should let only one replica win the Lease and call Investigate", func() {
			runner := &fakeInvestigationRunner{delay: 100 * time.Millisecond, result: &katypes.InvestigationResult{RCASummary: "ok", Confidence: 0.5}}
			d1 := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			d2 := agentsession.NewDispatcher(cli, testNamespace, "replica-b", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			// Two Dispatcher instances sharing one session.Manager is a test
			// simplification (real replicas each have their own Manager);
			// the hook is bound to whichever dispatcher actually won the
			// Lease and registered the remediationID mapping -- try both,
			// OnTerminal is a no-op log when its own map has no entry.
			wireHooks(mgr, d1, d2)
			go d1.Start(ctx)
			go d2.Start(ctx)

			as := newPendingAgentSession("as-3")
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted))

			Expect(int(runner.calls.Load())).To(Equal(1), "exactly one replica must have won the dispatch Lease and run the investigation")
		})
	})

	Describe("UT-INTERACTIVE-010-030 / DD-AA-KA-001 Amendment Gap 1: a fresh pre-emptive interactive start (InvestigationSession already exists at dispatch time) is registered but not launched until deferred start", func() {
		It("should record SessionID and Status.Interactive=true while Phase stays Pending, and never call Investigate", func() {
			runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "should not run"}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession("as-4")
			// A human called AF's MCP `start` action before AA even reached
			// Investigating -- the InvestigationSession CRD already exists at
			// the moment AA creates AgentSession. KA's dispatcher, not AA, is
			// responsible for observing this (Gap 1): AA never sets anything
			// on Spec to signal interactivity.
			isCR := &isv1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{Name: "is-" + as.Name, Namespace: testNamespace},
				Spec: isv1.InvestigationSessionSpec{
					RemediationRequestRef: isv1.ObjectRef{Name: as.Spec.RemediationRequestRef.Name, Namespace: testNamespace},
					A2ATaskID:             "task-" + as.Name,
					UserIdentity:          isv1.SessionUser{Username: "human-driver"},
					JoinMode:              isv1.SessionJoinModeStart,
				},
			}
			Expect(cli.Create(ctx, isCR)).To(Succeed())
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			Eventually(func() string {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.SessionID
			}, "2s", "10ms").ShouldNot(BeEmpty())

			got := &agentsessionv1.AgentSession{}
			Expect(cli.Get(ctx, key, got)).To(Succeed())
			Expect(got.Status.Phase).To(Equal(agentsessionv1.AgentSessionPhasePending),
				"BR-INTERACTIVE-010: deferred-interactive sessions await MCP action=start, Phase must not advance to Investigating")
			Expect(got.Status.Interactive).To(BeTrue(),
				"BR-AA-KA-065.5: KA's dispatch-time IS-existence check must write Status.Interactive=true immediately, not wait for a later UpgradeToInteractive")

			Consistently(func() int32 {
				return runner.calls.Load()
			}, "100ms", "10ms").Should(Equal(int32(0)), "Investigate must not be called until the deferred MCP action=start arrives")
		})
	})

	Describe("UT-AA-KA-065-024 / DD-AA-KA-001 Amendment Gap 1: no InvestigationSession at dispatch time dispatches autonomously", func() {
		It("should launch the investigation immediately and never set Status.Interactive", func() {
			runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "autonomous, no IS"}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession("as-6")
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted))

			got := &agentsessionv1.AgentSession{}
			Expect(cli.Get(ctx, key, got)).To(Succeed())
			Expect(got.Status.Interactive).To(BeFalse())
			Expect(int(runner.calls.Load())).To(Equal(1))
		})
	})

	Describe("UT-AA-KA-065-023: a stale dispatch Lease from a crashed replica is reclaimed", func() {
		It("should redispatch a Pending AgentSession whose dispatch Lease is expired", func() {
			staleDuration := int32(1)
			staleRenew := metav1.NewMicroTime(time.Now().Add(-1 * time.Hour))
			as := newPendingAgentSession("as-5")
			Expect(cli.Create(ctx, as)).To(Succeed())

			staleLease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{Name: "dispatch-as-5", Namespace: testNamespace},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptrString("dead-replica"),
					LeaseDurationSeconds: &staleDuration,
					RenewTime:            &staleRenew,
				},
			}
			Expect(cli.Create(ctx, staleLease)).To(Succeed())

			runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "recovered", Confidence: 0.7}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-new", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			key := crclient.ObjectKeyFromObject(as)
			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted))

			lease := &coordinationv1.Lease{}
			Expect(cli.Get(ctx, crclient.ObjectKey{Name: "dispatch-as-5", Namespace: testNamespace}, lease)).To(Succeed())
			Expect(*lease.Spec.HolderIdentity).To(Equal("replica-new"), "the new replica must have reclaimed the stale Lease")
		})
	})

	Describe("UT-AA-2170-DELETE-001 / DD-AA-KA-001 Amendment N: deleting an AgentSession (directly, or transitively via RR/AIAnalysis cascade deletion) stops the in-flight investigation goroutine", func() {
		It("should cancel the running investigation instead of leaking the goroutine forever", func() {
			started := make(chan struct{})
			runner := &fakeInvestigationRunner{delay: 30 * time.Second, result: &katypes.InvestigationResult{RCASummary: "should never complete"}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession("as-delete-1")
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			var sessionID string
			Eventually(func() bool {
				id, ok := mgr.FindByRemediationID(as.Spec.RemediationID)
				if ok {
					sessionID = id
					close(started)
				}
				return ok
			}, "2s", "10ms").Should(BeTrue(), "the investigation must actually be running (StatusRunning) before delete is exercised")
			<-started

			got := &agentsessionv1.AgentSession{}
			Expect(cli.Get(ctx, key, got)).To(Succeed())
			Expect(cli.Delete(ctx, got)).To(Succeed())

			Eventually(func() session.Status {
				s, err := mgr.GetSession(sessionID)
				if err != nil {
					return ""
				}
				return s.Status
			}, "2s", "10ms").Should(Equal(session.StatusCancelled),
				"AgentSession deletion must cancel the in-memory investigation session, not leave it running forever")
		})
	})

	Describe("UT-AA-2170-TIMEOUT-001 / DD-AA-KA-001 Amendment N: an AgentSession created already past its Spec.TimesOutAt deadline is never dispatched", func() {
		It("should self-enforce the deadline and mark Failed without calling Investigate", func() {
			runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "should never run"}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession("as-timeout-1")
			pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			as.Spec.TimesOutAt = &pastDeadline
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseFailed))

			Expect(int(runner.calls.Load())).To(Equal(0), "an already-timed-out AgentSession must never be dispatched (self-enforced deadline takes priority)")

			got := &agentsessionv1.AgentSession{}
			Expect(cli.Get(ctx, key, got)).To(Succeed())
			Expect(got.Status.Error).To(ContainSubstring("TimesOutAt"))
		})
	})

	Describe("UT-AA-2170-TIMEOUT-002: an AgentSession still within its Spec.TimesOutAt deadline dispatches normally", func() {
		It("should not be affected by a future deadline", func() {
			runner := &fakeInvestigationRunner{result: &katypes.InvestigationResult{RCASummary: "ok", Confidence: 0.7}}
			d := agentsession.NewDispatcher(cli, testNamespace, "replica-a", mgr, runner, logr.Discard(), agentsession.WithResyncInterval(20*time.Millisecond))
			wireHooks(mgr, d)
			go d.Start(ctx)

			as := newPendingAgentSession("as-timeout-2")
			futureDeadline := metav1.NewTime(time.Now().Add(1 * time.Hour))
			as.Spec.TimesOutAt = &futureDeadline
			Expect(cli.Create(ctx, as)).To(Succeed())
			key := crclient.ObjectKeyFromObject(as)

			Eventually(func() agentsessionv1.AgentSessionPhase {
				got := &agentsessionv1.AgentSession{}
				Expect(cli.Get(ctx, key, got)).To(Succeed())
				return got.Status.Phase
			}, "2s", "10ms").Should(Equal(agentsessionv1.AgentSessionPhaseCompleted))

			Expect(int(runner.calls.Load())).To(Equal(1))
		})
	})
})

func ptrString(s string) *string { return &s }
