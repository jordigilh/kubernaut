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
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

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

// defaultResyncInterval is how often watchLoop re-Lists and re-considers
// every non-terminal AgentSession, independent of watch events. This is the
// standard reflector/informer resync pattern, needed here for two reasons
// this raw (non-cached, no-informer) watch does not otherwise cover: (1) a
// dropped/reconnected watch has a gap between the old connection ending and
// the new one's List establishing continuity, and (2) it is what actually
// drives the stale dispatch-Lease reclaim path (isLeaseStale) for a
// long-Investigating AgentSession whose owning replica crashed -- without a
// resync, nothing re-examines an Investigating AgentSession once its
// initial watch event has been consumed.
const defaultResyncInterval = 30 * time.Second

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

// Dispatcher watches AgentSession Create/Update events on a raw
// crclient.WithWatch (no informer cache, no controller-runtime Manager --
// DD-AA-KA-001 mirrors AF's HandleAwaitSession pattern), races other KA
// replicas for a per-AgentSession dispatch Lease, and launches exactly one
// investigation per AgentSession via the existing session.Manager.
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

// Start runs the watch loop until ctx is cancelled, transparently
// re-establishing the watch (with a short backoff) if the underlying
// connection drops -- a raw watch.Interface has no built-in reconnect.
func (d *Dispatcher) Start(ctx context.Context) {
	for ctx.Err() == nil {
		if err := d.watchLoop(ctx); err != nil {
			d.logger.Error(err, "agentsession watch failed, retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (d *Dispatcher) listOpts() []crclient.ListOption {
	if d.namespace == "" {
		return nil
	}
	return []crclient.ListOption{crclient.InNamespace(d.namespace)}
}

// resync Lists every AgentSession in scope and re-considers each
// non-terminal one for dispatch. Called once before the watch is
// established (closing the gap between "KA starts watching" and "KA has
// seen every AgentSession that already existed") and then periodically on
// resyncInterval (closing the same gap after a watch reconnect, and driving
// the stale dispatch-Lease reclaim path for a stuck Investigating
// AgentSession -- see defaultResyncInterval's doc comment).
func (d *Dispatcher) resync(ctx context.Context) {
	// Bounded per-call deadline: the dispatcher's client (buildAgentSessionDispatcherClient)
	// deliberately carries no rest.Config.Timeout (that would also cut short
	// this same client's long-lived Watch), so this one-shot List call needs
	// its own timeout instead of inheriting one from the client.
	listCtx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	list := &agentsessionv1.AgentSessionList{}
	if err := d.client.List(listCtx, list, d.listOpts()...); err != nil {
		d.logger.Error(err, "agentsession resync: list failed")
		return
	}
	for i := range list.Items {
		as := &list.Items[i]
		if isTerminalPhase(as.Status.Phase) {
			continue
		}
		go d.tryDispatch(ctx, as.DeepCopy())
	}
}

func (d *Dispatcher) watchLoop(ctx context.Context) error {
	d.resync(ctx)

	w, err := d.client.Watch(ctx, &agentsessionv1.AgentSessionList{}, d.listOpts()...)
	if err != nil {
		return fmt.Errorf("watch agentsessions: %w", err)
	}
	defer w.Stop()

	ticker := time.NewTicker(d.resyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			d.resync(ctx)
		case evt, ok := <-w.ResultChan():
			if !ok {
				return fmt.Errorf("agentsession watch channel closed")
			}
			d.handleEvent(ctx, evt)
		}
	}
}

// handleEvent filters watch events to those worth a dispatch attempt: any
// AgentSession not yet in a terminal phase. Re-considering Pending AND
// Investigating (not just Pending) on every resync is what makes crash
// recovery work -- a healthy Investigating session's dispatch Lease is
// still fresh, so tryDispatch's Lease acquisition is a safe no-op; only a
// stale Lease (owning replica crashed) allows a reclaim.
func (d *Dispatcher) handleEvent(ctx context.Context, evt watch.Event) {
	if evt.Type != watch.Added && evt.Type != watch.Modified {
		return
	}
	as, ok := evt.Object.(*agentsessionv1.AgentSession)
	if !ok {
		return
	}
	if isTerminalPhase(as.Status.Phase) {
		return
	}
	go d.tryDispatch(ctx, as.DeepCopy())
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
		d.writeFailedStatus(ctx, key, fmt.Sprintf("failed to start investigation: %v", err))
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
