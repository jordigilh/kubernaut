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

package agentsession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// dispatchLeaseDuration is the coordination/v1 Lease duration used for
// dispatch-ownership (BR-AA-KA-065.3): long enough that a healthy,
// actively-renewed investigation is never mistaken for stale, short enough
// that a crashed replica's AgentSession is reclaimed within a bounded
// window (DD-AA-KA-001's "KA pod-restart mid-dispatch" recovery case).
const dispatchLeaseDuration = 15 * time.Minute

// dispatchLeaseRenewInterval is how often the owning replica refreshes its
// dispatch Lease while an investigation (or a deferred-interactive pending
// wait) is in flight.
const dispatchLeaseRenewInterval = dispatchLeaseDuration / 3

// defaultResyncInterval is how often a non-terminal AgentSession is
// re-Reconciled even with no new watch event (Reconcile's own
// ctrl.Result{RequeueAfter: ...}, #2231 / DD-AA-KA-001 Amendment -- the
// per-object successor to the retired ticker-driven resync's blanket List).
// Needed for two reasons a pure watch-driven Reconcile would not otherwise
// cover on its own: (1) it drives the stale dispatch-Lease reclaim path
// (isLeaseStale) for a long-Investigating AgentSession whose owning replica
// crashed -- without a periodic revisit, nothing re-examines an
// Investigating AgentSession once its initial Create/Update has been
// reconciled, and (2) it self-enforces an expired Spec.TimesOutAt deadline
// (isTimedOut) even when nothing else changes the object in the meantime.
const defaultResyncInterval = 30 * time.Second

// dispatchCleanupFinalizer is agentsessionv1.DispatchCleanupFinalizer,
// aliased locally for readability. See that constant's doc comment for the
// full rationale and the CI-confirmed failure mode (AF's structurally
// identical prior design, DD-AA-KA-001's "Post-merge correction" amendment)
// this replaces the old raw watch.Deleted handling to avoid.
const dispatchCleanupFinalizer = agentsessionv1.DispatchCleanupFinalizer

// listTimeout bounds the one-shot List/Get/Create/Update calls this
// Dispatcher makes outside of the long-lived Watch itself (resync's List,
// dispatch-Lease acquisition/reclaim). Mirrors renewDispatchLease's existing
// per-call timeout; see buildAgentSessionDispatcherClient's doc comment for
// why this can't just be a blanket rest.Config.Timeout on the client.
const listTimeout = 10 * time.Second

// InvestigationRunner abstracts the investigation entry point (mirrors
// internal/kubernautagent/server.InvestigationRunner without importing that
// package, to keep the dispatcher decoupled from the retired HTTP handler).
type InvestigationRunner interface {
	Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error)
}

// Dispatcher is a controller-runtime Reconciler for AgentSession (#2231 /
// DD-AA-KA-001 Amendment: superseding this type's original raw
// crclient.WithWatch watch loop). Every actual API read/write inside
// Reconcile/reconcileDelete/tryDispatch/etc. still goes through Dispatcher's
// own client field -- the same uncached, direct client used since
// DD-AA-KA-001 (buildAgentSessionDispatcherClient,
// cmd/kubernautagent/agentsession_wiring.go) -- preserving the
// dispatch-Lease race's existing consistency semantics unchanged; only
// event *delivery* (watch -> workqueue -> Reconcile) moves onto a
// controller-runtime Manager's informer/cache, registered via
// SetupWithManager. Races other KA replicas for a per-AgentSession dispatch
// Lease, and launches exactly one investigation per AgentSession via the
// existing session.Manager.
type Dispatcher struct {
	client         crclient.WithWatch
	namespace      string
	holderIdentity string
	sessions       *session.Manager
	investigator   InvestigationRunner
	logger         logr.Logger
	resyncInterval time.Duration

	// dispatchedMu guards dispatched, the remediationID -> AgentSession
	// ObjectKey map that lets OnTerminal/OnInteractiveUpgrade (registered
	// on session.Manager as hooks, DD-AA-KA-001 Amendment Gap 2) resolve
	// which CRD to write without threading an ObjectKey through
	// session.Manager itself, which has no CRD awareness.
	dispatchedMu sync.RWMutex
	dispatched   map[string]crclient.ObjectKey
}

// DispatcherOption configures optional Dispatcher behavior beyond
// NewDispatcher's required parameters (AGENTS.md Options-pattern
// convention, mirroring session.NewStore's StoreOption).
type DispatcherOption func(*Dispatcher)

// WithResyncInterval overrides defaultResyncInterval. Primarily for tests
// that need a resync fast enough to observe within a short Eventually
// window; production callers should rarely need this.
func WithResyncInterval(d time.Duration) DispatcherOption {
	return func(disp *Dispatcher) { disp.resyncInterval = d }
}

// NewDispatcher constructs a Dispatcher. namespace scopes the watch (""
// watches all namespaces the client's RBAC permits). holderIdentity
// identifies this KA replica in the dispatch Lease's HolderIdentity field.
func NewDispatcher(client crclient.WithWatch, namespace, holderIdentity string, sessions *session.Manager, investigator InvestigationRunner, logger logr.Logger, opts ...DispatcherOption) *Dispatcher {
	d := &Dispatcher{
		client:         client,
		namespace:      namespace,
		holderIdentity: holderIdentity,
		sessions:       sessions,
		investigator:   investigator,
		logger:         logger.WithName("agentsession-dispatcher"),
		resyncInterval: defaultResyncInterval,
		dispatched:     make(map[string]crclient.ObjectKey),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Reconcile is the controller-runtime entry point (#2231 / DD-AA-KA-001
// Amendment): replaces the retired raw-watch watchLoop/handleEvent/resync
// with the standard Get -> finalizer -> dispatch shape already proven by
// AF's AgentSessionTerminalCloseReconciler
// (internal/controller/apifrontend/agentsession_close.go).
// dispatchCleanupFinalizer defers actual deletion until reconcileDelete has
// stopped any in-memory investigation goroutine, closing the exact gap a
// raw watch.Deleted event left open: that event was delivered at-most-once
// outside controller-runtime's workqueue retry machinery, so a dropped
// delivery (confirmed live for AF's structurally-identical prior design,
// DD-AA-KA-001's "Post-merge correction" amendment) had no recovery short
// of the informer's next full relist (typically ~5-10 minutes) -- during
// which KA keeps investigating a remediation nothing will ever read the
// result of, bounded today only by session.Store's 60-minute maxSessionAge
// backstop.
//
// A not-found AgentSession is a benign no-op (fully deleted, finalizer
// already removed by a prior reconcile). tryDispatch/cancelOnTimeout are
// launched via goroutines here exactly as considerAgentSession always did
// pre-#2231: Reconcile itself must return quickly, since
// controller-runtime's default MaxConcurrentReconciles=1 would otherwise
// serialize every AgentSession's dispatch attempt behind this one worker --
// a throughput regression this change must not introduce.
func (d *Dispatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	as := &agentsessionv1.AgentSession{}
	if err := d.client.Get(ctx, req.NamespacedName, as); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get agentsession %s: %w", req.NamespacedName, err)
	}

	if !as.GetDeletionTimestamp().IsZero() {
		return d.reconcileDelete(ctx, as)
	}

	if !controllerutil.ContainsFinalizer(as, dispatchCleanupFinalizer) {
		controllerutil.AddFinalizer(as, dispatchCleanupFinalizer)
		if err := d.client.Update(ctx, as); err != nil {
			return ctrl.Result{}, fmt.Errorf("add %s finalizer: %w", dispatchCleanupFinalizer, err)
		}
	}

	d.considerAgentSession(ctx, as)
	if isTerminalPhase(as.Status.Phase) {
		return ctrl.Result{}, nil
	}
	// Re-visit this non-terminal AgentSession even with no new watch event
	// -- see defaultResyncInterval's doc comment for why (stale
	// dispatch-Lease reclaim, self-enforced TimesOutAt expiry).
	return ctrl.Result{RequeueAfter: d.resyncInterval}, nil
}

// reconcileDelete stops any in-memory investigation goroutine for a
// DeletionTimestamp-set AgentSession (cancelOnDelete, unchanged), then
// removes dispatchCleanupFinalizer to let the actual delete proceed.
// Mirrors AgentSessionTerminalCloseReconciler.reconcileDelete's finalizer-
// removal shape; see Reconcile's doc comment for why this replaces the
// prior raw watch.Deleted handling. Re-Gets before removing the finalizer
// (see removeDispatchCleanupFinalizer) rather than reusing as directly: as
// may be stale by the time this runs, since cancelOnDelete's
// ForceCancelByRemediationID call can itself synchronously trigger a
// Status write on this exact object via the TerminalHook
// (OnTerminal -> writeTerminalStatus) before returning here.
//
// An AgentSession without the finalizer (e.g. one created by a pre-upgrade
// replica before this Reconciler first observed it) falls through
// untouched -- a narrow, self-healing transitional gap bounded to objects
// already in flight at upgrade time; session.Store's maxSessionAge backstop
// remains the safety net for that window, same as it always has been for
// any missed cleanup.
func (d *Dispatcher) reconcileDelete(ctx context.Context, as *agentsessionv1.AgentSession) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(as, dispatchCleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	d.cancelOnDelete(as) //nolint:contextcheck // cancelOnDelete's own ForceCancelByRemediationID call is a pure in-memory map operation (see cancelOnDelete's doc comment) with no I/O to cancel; ctx is otherwise unused on this path

	if err := d.removeDispatchCleanupFinalizer(ctx, crclient.ObjectKeyFromObject(as)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// removeDispatchCleanupFinalizer re-Gets key and removes
// dispatchCleanupFinalizer, retrying on Conflict (mirrors updateStatus's
// retry loop, status_writer.go). A confirmed-live race during #2231
// development (UT-AA-2170-DELETE-001): cancelOnDelete's synchronous
// ForceCancelByRemediationID call can itself trigger a Status write via
// the TerminalHook on this exact object before this function's own Update
// runs, bumping resourceVersion out from under a stale copy -- a plain
// single-shot Update (no re-Get, no retry) intermittently failed with
// Conflict as a result.
func (d *Dispatcher) removeDispatchCleanupFinalizer(ctx context.Context, key crclient.ObjectKey) error {
	for attempt := 0; attempt < maxStatusUpdateRetries; attempt++ {
		fresh := &agentsessionv1.AgentSession{}
		if err := d.client.Get(ctx, key, fresh); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("get agentsession %s before removing finalizer: %w", key, err)
		}
		if !controllerutil.RemoveFinalizer(fresh, dispatchCleanupFinalizer) {
			return nil
		}
		err := d.client.Update(ctx, fresh)
		if err == nil {
			return nil
		}
		if apierrors.IsConflict(err) {
			continue
		}
		return fmt.Errorf("remove %s finalizer: %w", dispatchCleanupFinalizer, err)
	}
	return fmt.Errorf("remove %s finalizer: retries exhausted for agentsession %s", dispatchCleanupFinalizer, key)
}

// SetupWithManager registers the Dispatcher as a controller-runtime
// Reconciler for AgentSession (#2231 / DD-AA-KA-001 Amendment). mgr's cache
// is used exclusively to drive reliable Reconcile dispatch (watch ->
// workqueue -> Reconcile, with automatic reconnect/relist and per-key
// retry-with-backoff on error, none of which a raw watch.Interface
// provided) -- never for reads; see Dispatcher's doc comment. The
// WithEventFilter namespace predicate is defense-in-depth, redundant with
// (but not solely reliant on) the Manager's own namespace-scoped
// cache.Options in production (cmd/kubernautagent/agentsession_wiring.go).
func (d *Dispatcher) SetupWithManager(mgr ctrl.Manager) error {
	bldr := ctrl.NewControllerManagedBy(mgr).
		Named("agentsession-dispatcher").
		For(&agentsessionv1.AgentSession{})
	if d.namespace != "" {
		ns := d.namespace
		bldr = bldr.WithEventFilter(predicate.NewPredicateFuncs(func(obj crclient.Object) bool {
			return obj.GetNamespace() == ns
		}))
	}
	return bldr.Complete(d)
}

// considerAgentSession is Reconcile's shared dispatch-decision logic: skip
// terminal AgentSessions, self-enforce an expired Spec.TimesOutAt deadline
// (#2170, DD-AA-KA-001 Amendment N) in preference to dispatching, and
// otherwise attempt normal dispatch.
func (d *Dispatcher) considerAgentSession(ctx context.Context, as *agentsessionv1.AgentSession) {
	// #2204 follow-up (2026-08-20, IT-AA CI RCA): unexplained silent-stall
	// RCA target -- an Integration(aianalysis) run observed an AgentSession
	// created and immediately requeued by AA (backstop ~25m away), then
	// zero further activity of any kind (no enrichment, no LLM call, no
	// dispatch-lease log) for the entire 90s test window, despite this
	// replica's Reconciler having started well before the AgentSession
	// existed. This entry-level log is the cheapest possible proof point:
	// if it is missing for a given AgentSession name in a future
	// occurrence, Reconcile itself never visited the object (a KA-side
	// observation gap); if present, the stall is further downstream (Lease
	// race, session.Manager capacity, or the investigation itself). Debug,
	// not Info: fires on every Reconcile call for every non-terminal
	// AgentSession, so at production's 30s requeue interval under real
	// fleet volume this would otherwise be noisy at Info.
	d.logger.V(1).Info("considering agentsession for dispatch",
		"agentSession", as.Name, "namespace", as.Namespace, "phase", as.Status.Phase)
	if isTerminalPhase(as.Status.Phase) {
		return
	}
	if isTimedOut(as) {
		go d.cancelOnTimeout(ctx, as)
		return
	}
	go d.tryDispatch(ctx, as)
}

// isTimedOut reports whether as has an authoritative Spec.TimesOutAt
// deadline (propagated from AA, itself propagated from RO's
// RemediationRequest.Status.TimeoutConfig.Analyzing, DD-TIMEOUT-002) that
// has already passed. AgentSessions with no TimesOutAt set are never
// considered timed out here -- AA's own checkInvestigationTimeout
// fallback-duration enforcement still applies independently in that case.
func isTimedOut(as *agentsessionv1.AgentSession) bool {
	return as.Spec.TimesOutAt != nil && metav1.Now().After(as.Spec.TimesOutAt.Time)
}

// cancelOnDelete stops the in-memory investigation goroutine (if any) for a
// deleted AgentSession, called from reconcileDelete once a DeletionTimestamp
// is observed. #2170: this is the only stop mechanism left now that HTTP
// polling's CancelSession RPC is gone -- without it, deleting the owning
// RR/AIAnalysis (which cascade-deletes the AgentSession, Kubernetes garbage-
// collecting the owner chain transitively) would leave KA investigating a
// remediation nothing will ever read the result of, forever burning
// LLM/tool budget. ErrSessionNotFound is expected and silent: most
// deletions arrive after the investigation has already reached a terminal
// phase and been cleaned up.
func (d *Dispatcher) cancelOnDelete(as *agentsessionv1.AgentSession) {
	if as.Spec.RemediationID == "" {
		return
	}
	if err := d.sessions.ForceCancelByRemediationID(as.Spec.RemediationID); err != nil { //nolint:contextcheck // ForceCancelByRemediationID is a pure in-memory map operation (mirrors ForceCompleteByRemediationID, #1654) with no I/O to cancel; it intentionally takes no context
		if !errors.Is(err, session.ErrSessionNotFound) {
			d.logger.Error(err, "failed to cancel investigation on AgentSession deletion",
				"agentSession", as.Name, "namespace", as.Namespace, "remediationID", as.Spec.RemediationID)
		}
		return
	}
	d.logger.Info("AgentSession deleted, cancelled in-flight investigation",
		"agentSession", as.Name, "namespace", as.Namespace, "remediationID", as.Spec.RemediationID)
}

// cancelOnTimeout self-enforces an expired Spec.TimesOutAt deadline (#2170,
// DD-AA-KA-001 Amendment N): best-effort stops this replica's in-memory
// investigation goroutine if it happens to be the one running it, then
// unconditionally writes a terminal Failed Status directly -- unlike
// cancelOnDelete, the AgentSession CRD still exists here, so writing the
// terminal status directly (rather than routing through the
// dispatched-map/OnTerminal-hook machinery, which assumes an in-memory
// session this replica dispatched) is both simpler and correct even after a
// KA replica restart wipes the in-memory map that owned it.
func (d *Dispatcher) cancelOnTimeout(ctx context.Context, as *agentsessionv1.AgentSession) {
	if as.Spec.RemediationID != "" {
		if err := d.sessions.ForceCancelByRemediationID(as.Spec.RemediationID); err != nil && !errors.Is(err, session.ErrSessionNotFound) { //nolint:contextcheck // ForceCancelByRemediationID is a pure in-memory map operation (mirrors ForceCompleteByRemediationID, #1654) with no I/O to cancel; it intentionally takes no context
			d.logger.Error(err, "failed to cancel in-memory investigation on TimesOutAt expiry",
				"agentSession", as.Name, "namespace", as.Namespace, "remediationID", as.Spec.RemediationID)
		}
	}

	key := crclient.ObjectKeyFromObject(as)
	d.updateStatus(ctx, key, func(fresh *agentsessionv1.AgentSession) {
		if isTerminalPhase(fresh.Status.Phase) {
			return
		}
		now := metav1.Now()
		fresh.Status.Phase = agentsessionv1.AgentSessionPhaseFailed
		fresh.Status.CompletedAt = &now
		fresh.Status.Error = "investigation exceeded its TimesOutAt deadline"
	})
	d.logger.Info("AgentSession exceeded TimesOutAt, marked Failed",
		"agentSession", as.Name, "namespace", as.Namespace, "timesOutAt", as.Spec.TimesOutAt)
}

func isTerminalPhase(p agentsessionv1.AgentSessionPhase) bool {
	switch p {
	case agentsessionv1.AgentSessionPhaseCompleted, agentsessionv1.AgentSessionPhaseFailed, agentsessionv1.AgentSessionPhaseCancelled:
		return true
	default:
		return false
	}
}

// tryDispatch attempts to win as's dispatch Lease and, on success, launches
// the investigation. A lost race (another replica already holds a fresh
// Lease) or an already-terminal Status observed on re-fetch are silent
// no-ops, not errors -- this is the expected, common outcome of every
// replica reacting to the same watch event (BR-AA-KA-065.3).
func (d *Dispatcher) tryDispatch(ctx context.Context, as *agentsessionv1.AgentSession) {
	won, err := d.acquireDispatchLease(ctx, as)
	if err != nil {
		d.logger.Error(err, "dispatch lease acquisition failed", "agentSession", as.Name, "namespace", as.Namespace)
		return
	}
	if !won {
		// #2204 follow-up: this is the expected, common outcome of every
		// replica reacting to the same event (see this function's doc
		// comment) -- V(1), not Error/Info, so it stays silent at
		// production's default verbosity. Its value is purely diagnostic:
		// without it, "lost the Lease race every single time this
		// AgentSession was considered" (a genuine stall, e.g. a Lease held
		// by a replica that itself never progresses) was previously
		// indistinguishable from "never considered at all" -- both produced
		// zero log output.
		d.logger.V(1).Info("dispatch lease race lost, another replica owns this agentsession",
			"agentSession", as.Name, "namespace", as.Namespace)
		return
	}

	getCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	fresh := &agentsessionv1.AgentSession{}
	if err := d.client.Get(getCtx, crclient.ObjectKeyFromObject(as), fresh); err != nil {
		d.logger.Error(err, "failed to re-fetch agentsession before dispatch", "agentSession", as.Name)
		return
	}
	if isTerminalPhase(fresh.Status.Phase) {
		// BR-AI-009 / DD-AA-KA-001 amendment Gap 6 follow-up (2026-08-20,
		// CI RCA): this tryDispatch call just won (Created or reclaimed) the
		// dispatch Lease above, but the fresh Get shows the AgentSession
		// already terminal -- a benign race where a concurrent tryDispatch
		// call for the same AgentSession (typically a redundant Reconcile
		// firing against a stale, pre-rejection snapshot) reached
		// acquireDispatchLease after the original attempt's own
		// deleteDispatchLease had already run, winning a brand-new Lease
		// for work that's already done. Without this, that Lease is
		// orphaned fresh (dispatchLeaseDuration=15m) and blocks any retry
		// under the same AgentSession name for up to 15 minutes, exactly
		// the failure mode Gap 6 originally fixed for the direct rejection
		// path -- confirmed via UT-AA-KA-065-025/026 CI flakes surfacing a
		// leftover Lease despite the rejection path's own cleanup.
		d.deleteDispatchLease(ctx, as.Name, as.Namespace)
		return
	}

	d.dispatch(ctx, fresh)
}

// dispatch launches the investigation for a won-Lease AgentSession: starts
// the dispatch-Lease renewal goroutine, performs KA's own dispatch-time
// InvestigationSession-existence check (DD-AA-KA-001 Amendment Gap 1 -- the
// sole source of truth for interactivity, never a Spec field), registers
// the remediationID->ObjectKey mapping the TerminalHook/InteractiveUpgradeHook
// need to resolve a CRD to write (Gap 2), and calls the appropriate
// session.Manager entry point (autonomous vs. BR-INTERACTIVE-010
// deferred-interactive).
func (d *Dispatcher) dispatch(ctx context.Context, as *agentsessionv1.AgentSession) {
	key := crclient.ObjectKeyFromObject(as)
	signal := MapSpecToSignal(as.Spec)

	interactive, isErr := d.hasInvestigationSession(ctx, as.Namespace, as.Spec.RemediationRequestRef.Name)
	if isErr != nil {
		d.logger.Error(isErr, "dispatch-time InvestigationSession-existence check failed, defaulting to autonomous dispatch",
			"agentSession", as.Name, "namespace", as.Namespace)
	}
	signal.Interactive = interactive

	sctx := session.SessionContext{
		IncidentID:    as.Spec.IncidentID,
		RemediationID: as.Spec.RemediationID,
		Signal:        signal,
	}
	d.registerDispatched(as.Spec.RemediationID, key)

	var stopRenew sync.Once
	done := make(chan struct{})
	//nolint:contextcheck // renewal must outlive ctx's cancellation (investigation may finish/be cancelled while the Lease still needs refreshing until stop()); renewOnce uses its own bounded context by design.
	go d.renewDispatchLease(dispatchLeaseName(as.Name), as.Namespace, done)
	stop := func() { stopRenew.Do(func() { close(done) }) }

	// dispatched gates investigateFn's actual work on writeDispatchedStatus
	// (below) having completed first. Without this, a fast-returning
	// investigator can race writeDispatchedStatus: the terminal write
	// (now delivered asynchronously via OnTerminal, but still driven by
	// this same goroutine's return) would land first (observing Phase=="",
	// writing Completed/Failed) and then writeDispatchedStatus's own Get
	// would see an already-terminal Phase and bail out via its
	// no-regression guard -- silently dropping SessionID/DispatchedAt from
	// the final, terminal Status forever, since nothing revisits a
	// terminal AgentSession.
	dispatched := make(chan struct{})
	investigateFn := func(bgCtx context.Context) (*katypes.InvestigationResult, error) {
		defer stop()
		select {
		case <-dispatched:
		case <-bgCtx.Done():
			return nil, bgCtx.Err()
		}
		return d.investigator.Investigate(bgCtx, signal)
	}

	var (
		sessionID string
		err       error
	)
	if interactive {
		sessionID, err = d.sessions.StartInteractiveSessionWithContext(ctx, investigateFn, sctx)
	} else {
		sessionID, err = d.sessions.StartInvestigationWithContext(ctx, investigateFn, sctx)
	}
	if err != nil {
		stop()
		close(dispatched)
		d.logger.Error(err, "failed to start investigation", "agentSession", as.Name)
		reason := ""
		if errors.Is(err, session.ErrMaxInvestigationsReached) {
			// BR-AI-009: a transient, self-resolving capacity rejection --
			// not a genuine investigation failure -- tagged so AA's
			// InvestigatingHandler can retry instead of permanently
			// failing the AIAnalysis (DD-AA-KA-001 amendment).
			reason = agentsessionv1.AgentSessionReasonCapacityExceeded
		}
		d.writeFailedStatus(ctx, key, fmt.Sprintf("failed to start investigation: %v", err), reason)
		// BR-AI-009 / DD-AA-KA-001 amendment Gap 6 (2026-08-20): the
		// investigation never started (session.Manager rejected it before
		// accepting ownership), so this replica's dispatch Lease no longer
		// protects anything in-flight -- release it immediately. Without
		// this, AA's retry (DeleteForRetry + a fresh AgentSession under
		// the identical name) races this Lease while it is still fresh
		// (dispatchLeaseDuration=15m), and tryReclaimStaleLease's
		// isLeaseStale check silently treats the retry's tryDispatch as a
		// lost race, blocking it for up to 15 minutes. Confirmed live:
		// E2E-AA-065 (2026-08-20 CI run) -- retried AgentSessions' Status
		// stuck at Phase="" for the full 300s test window, zero further
		// dispatch attempts logged.
		d.deleteDispatchLease(ctx, as.Name, as.Namespace)
		return
	}

	d.writeDispatchedStatus(ctx, key, sessionID, interactive)
	close(dispatched)
}

// hasInvestigationSession reports whether any InvestigationSession CRD
// exists for rrName in namespace -- KA's own dispatch-time check
// (DD-AA-KA-001 Amendment Gap 1), replacing the retired
// Spec.Interactive/AA-side pre-check. Lists client-side rather than via a
// field-selector MatchingFields query: this raw (no informer, no
// registered field index) client can't rely on the fake client's
// WithIndex machinery in tests, and per-namespace InvestigationSession
// volume is small (interactive sessions only), so a client-side filter is
// simpler and behaves identically against a real API server.
func (d *Dispatcher) hasInvestigationSession(ctx context.Context, namespace, rrName string) (bool, error) {
	if rrName == "" {
		return false, nil
	}
	listCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	list := &isv1.InvestigationSessionList{}
	var opts []crclient.ListOption
	if namespace != "" {
		opts = append(opts, crclient.InNamespace(namespace))
	}
	if err := d.client.List(listCtx, list, opts...); err != nil {
		return false, fmt.Errorf("list investigationsessions for rr %q: %w", rrName, err)
	}
	for i := range list.Items {
		if list.Items[i].Spec.RemediationRequestRef.Name == rrName {
			return true, nil
		}
	}
	return false, nil
}

// registerDispatched records the AgentSession ObjectKey that
// OnTerminal/OnInteractiveUpgrade must resolve remediationID to.
func (d *Dispatcher) registerDispatched(remediationID string, key crclient.ObjectKey) {
	d.dispatchedMu.Lock()
	defer d.dispatchedMu.Unlock()
	d.dispatched[remediationID] = key
}

// lookupDispatched resolves remediationID to its AgentSession ObjectKey.
func (d *Dispatcher) lookupDispatched(remediationID string) (crclient.ObjectKey, bool) {
	d.dispatchedMu.RLock()
	defer d.dispatchedMu.RUnlock()
	key, ok := d.dispatched[remediationID]
	return key, ok
}

// unregisterDispatched removes remediationID's mapping once its session
// has reached a genuine terminal outcome, bounding the map's size to
// non-terminal sessions plus a small trailing window.
func (d *Dispatcher) unregisterDispatched(remediationID string) {
	d.dispatchedMu.Lock()
	defer d.dispatchedMu.Unlock()
	delete(d.dispatched, remediationID)
}

// OnTerminal implements session.TerminalHookFunc (registered on
// session.Manager at wiring time, cmd/kubernautagent/agentsession_wiring.go).
// It is the SOLE status-writing path for outcomes reached after dispatch
// (DD-AA-KA-001 Amendment Gap 2): whichever call site inside session.Manager
// actually won a session's terminal or InteractiveHold transition is the
// only one that ever fires this hook for that session, so an out-of-band
// MCP completion (CompleteUserDriving/ForceCompleteByRemediationID) can
// never be overwritten by a stale return value from the original dispatch
// goroutine.
func (d *Dispatcher) OnTerminal(snap session.TerminalSnapshot) {
	key, ok := d.lookupDispatched(snap.RemediationID)
	if !ok {
		d.logger.Info("terminal hook: no agentsession mapping for remediation id, skipping status write",
			"remediation_id", snap.RemediationID, "session_id", snap.SessionID)
		return
	}
	d.writeTerminalStatus(context.Background(), key, snap.Status, snap.Result, snap.Err)
	if snap.Status != session.StatusUserDriving {
		d.unregisterDispatched(snap.RemediationID)
	}
}

// OnInteractiveUpgrade implements session.InteractiveUpgradeHookFunc,
// recording a human driver's explicit takeover (UpgradeToInteractive,
// TransitionToUserDriving, ForceTransitionToUserDriving) on
// AgentSession.Status -- distinct from OnTerminal's StatusUserDriving case
// (an autonomous InteractiveHold with no acting user yet).
func (d *Dispatcher) OnInteractiveUpgrade(sessionID, remediationID, username string, groups []string) {
	key, ok := d.lookupDispatched(remediationID)
	if !ok {
		d.logger.Info("interactive upgrade hook: no agentsession mapping for remediation id, skipping status write",
			"remediation_id", remediationID, "session_id", sessionID)
		return
	}
	d.updateStatus(context.Background(), key, func(as *agentsessionv1.AgentSession) {
		if isTerminalPhase(as.Status.Phase) {
			return
		}
		as.Status.Interactive = true
		as.Status.ActingUser = username
		as.Status.ActingUserGroups = groups
	})
}

func dispatchLeaseName(agentSessionName string) string {
	name := "dispatch-" + agentSessionName
	if len(name) > 63 {
		name = name[:63]
	}
	// DNS-1123 subdomain (metadata.name) must start and end with an
	// alphanumeric character; a naive length cut can leave a trailing
	// separator (e.g. "...1787063336-26" chopped to "...1787063336-"),
	// which the API server rejects at Lease-Create time. That rejection is
	// swallowed as a per-tryDispatch error and retried every resync with
	// the same invalid name, permanently blocking dispatch for that
	// AgentSession -- confirmed live via a stuck-forever poll loop
	// (helios08 repro, 2026-08-18, IT-AA #2170). Trim any trailing
	// separators left by the cut.
	name = strings.TrimRight(name, "-.")
	return name
}

// acquireDispatchLease attempts to Create the dispatch Lease for as. On
// AlreadyExists, it attempts to reclaim a stale Lease (owning replica
// crashed without completing) rather than treating every AlreadyExists as
// a lost race.
func (d *Dispatcher) acquireDispatchLease(ctx context.Context, as *agentsessionv1.AgentSession) (bool, error) {
	duration := int32(dispatchLeaseDuration.Seconds())
	now := nowMicroTime()
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dispatchLeaseName(as.Name),
			Namespace: as.Namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &d.holderIdentity,
			LeaseDurationSeconds: &duration,
			AcquireTime:          now,
			RenewTime:            now,
		},
	}

	createCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	err := d.client.Create(createCtx, lease)
	if err == nil {
		return true, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return false, fmt.Errorf("create dispatch lease for agentsession %q: %w", as.Name, err)
	}
	return d.tryReclaimStaleLease(ctx, lease)
}

// tryReclaimStaleLease reclaims lease.Name/Namespace only if the currently
// held Lease's renew deadline has passed. A benign lost-race (fresh Lease
// held by another replica, or the Lease was deleted between Create and
// Get) returns (false, nil), not an error.
func (d *Dispatcher) tryReclaimStaleLease(ctx context.Context, want *coordinationv1.Lease) (bool, error) {
	getCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	existing := &coordinationv1.Lease{}
	if err := d.client.Get(getCtx, crclient.ObjectKeyFromObject(want), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get dispatch lease %q: %w", want.Name, err)
	}
	if !isLeaseStale(existing) {
		return false, nil
	}

	existing.Spec.HolderIdentity = want.Spec.HolderIdentity
	existing.Spec.AcquireTime = want.Spec.AcquireTime
	existing.Spec.RenewTime = want.Spec.RenewTime
	existing.Spec.LeaseDurationSeconds = want.Spec.LeaseDurationSeconds

	updateCtx, cancel2 := context.WithTimeout(ctx, listTimeout)
	defer cancel2()
	if err := d.client.Update(updateCtx, existing); err != nil {
		if apierrors.IsConflict(err) {
			return false, nil
		}
		return false, fmt.Errorf("reclaim stale dispatch lease %q: %w", want.Name, err)
	}
	d.logger.Info("reclaimed stale dispatch lease", "lease", want.Name, "namespace", want.Namespace)
	return true, nil
}

// deleteDispatchLease removes the dispatch Lease for agentSessionName so a
// subsequent dispatch attempt (a resync re-considering the same
// AgentSession, or a brand-new AgentSession created under the identical
// name by an AA retry) can immediately acquire a fresh Lease rather than
// waiting out isLeaseStale's dispatchLeaseDuration window (BR-AI-009,
// DD-AA-KA-001 amendment Gap 6). Idempotent and best-effort: NotFound is
// expected (nothing to clean up) and any other error is logged, not fatal
// -- worst case the caller falls back to the pre-existing stale-Lease
// reclaim path once dispatchLeaseDuration elapses.
func (d *Dispatcher) deleteDispatchLease(ctx context.Context, agentSessionName, namespace string) {
	deleteCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dispatchLeaseName(agentSessionName),
			Namespace: namespace,
		},
	}
	if err := d.client.Delete(deleteCtx, lease); err != nil && !apierrors.IsNotFound(err) {
		d.logger.Error(err, "failed to delete dispatch lease after rejected dispatch",
			"agentSession", agentSessionName, "namespace", namespace)
	}
}

func isLeaseStale(l *coordinationv1.Lease) bool {
	if l.Spec.RenewTime == nil || l.Spec.LeaseDurationSeconds == nil {
		return true
	}
	deadline := l.Spec.RenewTime.Add(time.Duration(*l.Spec.LeaseDurationSeconds) * time.Second)
	return time.Now().After(deadline)
}

// renewDispatchLease refreshes the dispatch Lease's RenewTime on a fixed
// interval until done is closed (investigation finished, or dispatch
// failed to start). Best-effort: a renewal failure is logged, not fatal --
// the next tick retries, and worst case the Lease goes stale and is
// reclaimed by another replica (a safe outcome, not data loss, since a
// terminal Status write always precedes stop() being deferred-called; a
// duplicate reclaim of an actually-still-running investigation is bounded
// by dispatchLeaseDuration and logged on both sides for operator visibility).
func (d *Dispatcher) renewDispatchLease(name, namespace string, done <-chan struct{}) {
	ticker := time.NewTicker(dispatchLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			d.renewOnce(name, namespace)
		}
	}
}

func (d *Dispatcher) renewOnce(name, namespace string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lease := &coordinationv1.Lease{}
	if err := d.client.Get(ctx, crclient.ObjectKey{Name: name, Namespace: namespace}, lease); err != nil {
		d.logger.Error(err, "dispatch lease renewal: get failed", "lease", name, "namespace", namespace)
		return
	}
	lease.Spec.RenewTime = nowMicroTime()
	if err := d.client.Update(ctx, lease); err != nil {
		d.logger.Error(err, "dispatch lease renewal: update failed", "lease", name, "namespace", namespace)
	}
}

func nowMicroTime() *metav1.MicroTime {
	t := metav1.NewMicroTime(time.Now())
	return &t
}
