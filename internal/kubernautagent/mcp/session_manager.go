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

package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"

	"github.com/google/uuid"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrLeaseHeld indicates the Lease is held by another interactive driver.
	ErrLeaseHeld = errors.New("lease held by another driver")

	// ErrSessionNotFound indicates no session exists with the given ID.
	ErrSessionNotFound = errors.New("session not found")

	// ErrEmptyUsername rejects sessions with no authenticated identity (SEC-01).
	ErrEmptyUsername = errors.New("username must not be empty")

	// ErrMaxSessionsReached rejects new sessions when capacity is exhausted (SEC-03).
	ErrMaxSessionsReached = errors.New("maximum concurrent sessions reached")

	// ErrSessionExpired indicates the session TTL has been exceeded (SEC-04).
	ErrSessionExpired = errors.New("session expired")
)

const (
	leasePrefix = "kubernaut-interactive-"

	// leaseTTLAnnotation is set on each Lease so an external janitor can
	// garbage-collect orphaned Leases whose sessions crashed without Release.
	leaseTTLAnnotation = "kubernaut.io/session-ttl"
)

// DefaultSessionTTL is the default Lease TTL for interactive sessions.
// Overridden by InteractiveConfig.SessionTTL at runtime.
var DefaultSessionTTL = 30 * time.Minute

// LeaseSessionManager implements SessionManager with K8s coordination/v1 Lease
// for single-driver guarantee (BR-INTERACTIVE-002).
type LeaseSessionManager struct {
	client            client.Client
	namespace         string
	sessionTTL        time.Duration
	inactivityTimeout time.Duration
	maxSessions       int
	sessions          sync.Map // sessionID -> *sessionEntry
	rrIndex           sync.Map // rrID -> sessionID
	activeCount       atomic.Int32
	logger            logr.Logger
	onSessionExpired  func(sessionID, rrID, reason string) // called on TTL/inactivity auto-release
	onReconnect       func(sessionID string)               // called when Takeover detects same-user reconnect
}

type sessionEntry struct {
	session      *InteractiveSession
	rrID         string
	signalMeta   map[string]string
	lastActivity atomic.Value // stores time.Time
}

// LeaseOption configures optional parameters for LeaseSessionManager.
type LeaseOption func(*LeaseSessionManager)

// WithSessionTTL overrides the default session TTL used for Lease duration.
func WithSessionTTL(ttl time.Duration) LeaseOption {
	return func(m *LeaseSessionManager) {
		m.sessionTTL = ttl
	}
}

// WithInactivityTimeout sets the per-session inactivity timeout (SEC-04).
func WithInactivityTimeout(timeout time.Duration) LeaseOption {
	return func(m *LeaseSessionManager) {
		m.inactivityTimeout = timeout
	}
}

// WithMaxConcurrentSessions sets the session capacity limit (SEC-03).
func WithMaxConcurrentSessions(max int) LeaseOption {
	return func(m *LeaseSessionManager) {
		m.maxSessions = max
	}
}

// WithSessionExpiredCallback sets a callback invoked when GetDriver auto-releases
// a session due to TTL or inactivity timeout. Enables audit emission (M1) for
// expiry paths that bypass InvestigateTool's explicit complete/cancel handlers.
func WithSessionExpiredCallback(fn func(sessionID, rrID, reason string)) LeaseOption {
	return func(m *LeaseSessionManager) {
		m.onSessionExpired = fn
	}
}

// SetReconnectCallback sets a callback invoked when Takeover detects a same-user
// reconnect for an existing session. Used to cancel pending grace-period releases
// in GracefulSessionClosedHandler (BR-INTERACTIVE-001).
func (m *LeaseSessionManager) SetReconnectCallback(fn func(sessionID string)) {
	m.onReconnect = fn
}

// NewLeaseSessionManager creates a LeaseSessionManager backed by the given K8s client.
func NewLeaseSessionManager(c client.Client, namespace string, logger logr.Logger, opts ...LeaseOption) SessionManager {
	return NewLeaseSessionManagerConcrete(c, namespace, logger, opts...)
}

// NewLeaseSessionManagerConcrete returns the concrete *LeaseSessionManager type
// for callers that need access to signal metadata storage (e.g., disconnect handler).
func NewLeaseSessionManagerConcrete(c client.Client, namespace string, logger logr.Logger, opts ...LeaseOption) *LeaseSessionManager {
	m := &LeaseSessionManager{
		client:     c,
		namespace:  namespace,
		sessionTTL: DefaultSessionTTL,
		logger:     logger,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// StoreSignalMetadata attaches signal context metadata to an active session entry.
// Used by handleTakeover to persist autonomous session metadata for later reconstruction.
func (m *LeaseSessionManager) StoreSignalMetadata(sessionID string, metadata map[string]string) {
	raw, ok := m.sessions.Load(sessionID)
	if !ok {
		return
	}
	entry := raw.(*sessionEntry)
	entry.signalMeta = metadata
}

// GetSignalMetadata retrieves stored signal metadata for a session.
// Returns nil if session not found or no metadata stored.
func (m *LeaseSessionManager) GetSignalMetadata(sessionID string) map[string]string {
	raw, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil
	}
	return raw.(*sessionEntry).signalMeta
}

// GetSessionInfo returns the correlationID (rrID) and signal metadata for a session.
// Must be called BEFORE Release, which deletes the session entry.
// Returns empty values if session not found.
func (m *LeaseSessionManager) GetSessionInfo(sessionID string) (rrID string, signalMeta map[string]string) {
	raw, ok := m.sessions.Load(sessionID)
	if !ok {
		return "", nil
	}
	entry := raw.(*sessionEntry)
	return entry.rrID, entry.signalMeta
}

func (m *LeaseSessionManager) Takeover(ctx context.Context, rrID string, user UserInfo) (*InteractiveSession, error) {
	// SEC-01: Reject anonymous/empty identity.
	if user.Username == "" {
		return nil, ErrEmptyUsername
	}

	// SEC-03: Enforce max concurrent sessions.
	if m.maxSessions > 0 && int(m.activeCount.Load()) >= m.maxSessions {
		return nil, ErrMaxSessionsReached
	}

	// UX-03: Check local index first. If the same user already holds the
	// session, allow reconnect (e.g., after network loss) by returning the
	// existing session with a refreshed activity timestamp. A different user
	// gets ErrLeaseHeld with holder context.
	if sess, err, handled := m.reconnectOrRejectExistingLease(rrID, user); handled {
		return sess, err
	}

	sessionID, err := m.acquireLease(ctx, rrID)
	if err != nil {
		return nil, err
	}

	return m.registerNewInteractiveSession(sessionID, rrID, user), nil
}

// reconnectOrRejectExistingLease checks whether rrID already has an active
// lease. When held by the same user, it refreshes activity and returns the
// existing session (reconnect). When held by a different user, or the local
// index is inconsistent with session state, it returns ErrLeaseHeld. The
// third return value reports whether the takeover request was fully handled
// here (true) or should fall through to lease acquisition (false, meaning no
// existing lease was found).
// nolint:nilnil // the trailing `handled` bool already disambiguates this
// (nil, nil, false): callers must (and do, see Takeover above) check
// `handled` before touching session/err, so there is no scenario where a
// nil session + nil error is misread as "found, no error" — that is
// precisely the ambiguity a sentinel error would solve, already solved here
// by the bool (Issue #1546 Tier 2).
func (m *LeaseSessionManager) reconnectOrRejectExistingLease(rrID string, user UserInfo) (*InteractiveSession, error, bool) {
	existingSessionID, ok := m.rrIndex.Load(rrID)
	if !ok {
		return nil, nil, false // nolint:nilnil
	}
	raw, found := m.sessions.Load(existingSessionID)
	if !found {
		return nil, ErrLeaseHeld, true
	}
	entry := raw.(*sessionEntry)
	if entry.session.ActingUser.Username != user.Username {
		return nil, fmt.Errorf("%w: held by %q since %s",
			ErrLeaseHeld, entry.session.ActingUser.Username, entry.session.StartedAt.Format(time.RFC3339)), true
	}
	entry.lastActivity.Store(time.Now())
	entry.session.Reconnected = true
	m.logger.Info("interactive session reconnected (same user)",
		"session_id", entry.session.SessionID,
		"rr_id", rrID,
		"user", user.Username,
	)
	if m.onReconnect != nil {
		m.onReconnect(entry.session.SessionID)
	}
	return entry.session, nil, true
}

// acquireLease creates a new coordination.k8s.io Lease for rrID, reclaiming
// an expired lease and retrying once if the lease name is already taken.
func (m *LeaseSessionManager) acquireLease(ctx context.Context, rrID string) (string, error) {
	sessionID := uuid.New().String()
	leaseName := leaseName(rrID)
	leaseDuration := int32(m.sessionTTL.Seconds())

	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: m.namespace,
			Annotations: map[string]string{
				leaseTTLAnnotation: m.sessionTTL.String(),
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &sessionID,
			LeaseDurationSeconds: &leaseDuration,
			AcquireTime:          nowMicroTime(),
		},
	}

	err := m.client.Create(ctx, lease)
	if err == nil {
		return sessionID, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("create lease for rr %q: %w", rrID, err)
	}
	if !m.tryReclaimExpiredLease(ctx, leaseName, rrID) {
		return "", ErrLeaseHeld
	}
	if retryErr := m.client.Create(ctx, lease); retryErr != nil {
		return "", fmt.Errorf("create lease after reclaim for rr %q: %w", rrID, retryErr)
	}
	return sessionID, nil
}

// registerNewInteractiveSession builds the InteractiveSession for a freshly
// acquired lease and records it in the manager's session/rrID indexes.
func (m *LeaseSessionManager) registerNewInteractiveSession(sessionID, rrID string, user UserInfo) *InteractiveSession {
	session := &InteractiveSession{
		SessionID:     sessionID,
		CorrelationID: rrID,
		ActingUser:    user,
		StartedAt:     time.Now(),
	}

	entry := &sessionEntry{session: session, rrID: rrID}
	entry.lastActivity.Store(time.Now())
	m.sessions.Store(sessionID, entry)
	m.rrIndex.Store(rrID, sessionID)
	m.activeCount.Add(1)

	m.logger.Info("interactive session started",
		"session_id", sessionID,
		"rr_id", rrID,
		"user", user.Username,
	)

	return session
}

func (m *LeaseSessionManager) Release(sessionID string, reason string) error {
	raw, ok := m.sessions.Load(sessionID)
	if !ok {
		return ErrSessionNotFound
	}
	entry := raw.(*sessionEntry)

	leaseName := leaseName(entry.rrID)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: m.namespace,
		},
	}
	if err := m.client.Delete(context.Background(), lease); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete lease for session %q: %w", sessionID, err)
	}

	now := time.Now()
	entry.session.CompletedAt = &now
	entry.session.Reason = reason

	m.sessions.Delete(sessionID)
	m.rrIndex.Delete(entry.rrID)
	m.activeCount.Add(-1)

	m.logger.Info("interactive session released",
		"session_id", sessionID,
		"rr_id", entry.rrID,
		"reason", reason,
	)

	return nil
}

// nolint:nilnil // intentional "no active driver" sentinel, not an error —
// canonical repository/lookup idiom; every caller already guards with
// `if err != nil || sess == nil` before use (Issue #1546 Tier 2).
func (m *LeaseSessionManager) GetDriver(rrID string) (*InteractiveSession, error) {
	raw, ok := m.rrIndex.Load(rrID)
	if !ok {
		return nil, nil // nolint:nilnil
	}
	sessionID := raw.(string)

	raw, ok = m.sessions.Load(sessionID)
	if !ok {
		return nil, nil // nolint:nilnil
	}
	entry := raw.(*sessionEntry)

	// SEC-04: Check session TTL expiry.
	if m.sessionTTL > 0 && time.Since(entry.session.StartedAt) > m.sessionTTL {
		return nil, m.expireSession(sessionID, rrID, "ttl_expired", "session TTL expired, auto-releasing")
	}

	// SEC-04: Check inactivity timeout.
	if m.isInactivityExpired(entry) {
		return nil, m.expireSession(sessionID, rrID, "inactivity_timeout", "session inactivity timeout, auto-releasing")
	}

	return entry.session, nil
}

// isInactivityExpired reports whether entry has exceeded the configured
// inactivity timeout (SEC-04).
func (m *LeaseSessionManager) isInactivityExpired(entry *sessionEntry) bool {
	if m.inactivityTimeout <= 0 {
		return false
	}
	lastAct, ok := entry.lastActivity.Load().(time.Time)
	if !ok {
		return false
	}
	return time.Since(lastAct) > m.inactivityTimeout
}

// expireSession releases sessionID for the given expiry reason, notifies
// onSessionExpired if configured, and returns ErrSessionExpired for the
// caller (GetDriver) to propagate.
func (m *LeaseSessionManager) expireSession(sessionID, rrID, reason, logMsg string) error {
	m.logger.Info(logMsg, "session_id", sessionID, "rr_id", rrID)
	_ = m.Release(sessionID, reason)
	if m.onSessionExpired != nil {
		m.onSessionExpired(sessionID, rrID, reason)
	}
	return ErrSessionExpired
}

// TouchActivity updates the last activity timestamp for a session (SEC-04).
// Called by tool handlers on each interaction to reset the inactivity timer.
func (m *LeaseSessionManager) TouchActivity(rrID string) {
	raw, ok := m.rrIndex.Load(rrID)
	if !ok {
		return
	}
	sessionID := raw.(string)
	raw, ok = m.sessions.Load(sessionID)
	if !ok {
		return
	}
	raw.(*sessionEntry).lastActivity.Store(time.Now())
}

func (m *LeaseSessionManager) IsDriverActive(rrID string) bool {
	_, ok := m.rrIndex.Load(rrID)
	return ok
}

// ActiveSessionIDs returns the session IDs of all active sessions.
// Used by SessionDrainer during graceful shutdown (BR-OPS-013).
func (m *LeaseSessionManager) ActiveSessionIDs() []string {
	var ids []string
	m.sessions.Range(func(key, _ any) bool {
		ids = append(ids, key.(string))
		return true
	})
	return ids
}

// tryReclaimExpiredLease checks if an existing K8s Lease is expired (e.g.,
// orphaned after a pod restart where in-memory state was lost). If the Lease's
// AcquireTime + LeaseDurationSeconds is in the past, it deletes the Lease so
// the caller can create a fresh one. Returns true if the Lease was reclaimed.
func (m *LeaseSessionManager) tryReclaimExpiredLease(ctx context.Context, name, rrID string) bool {
	existing := &coordinationv1.Lease{}
	key := client.ObjectKey{Namespace: m.namespace, Name: name}
	if err := m.client.Get(ctx, key, existing); err != nil {
		return false
	}

	if existing.Spec.AcquireTime == nil || existing.Spec.LeaseDurationSeconds == nil {
		return false
	}

	expiry := existing.Spec.AcquireTime.Add(
		time.Duration(*existing.Spec.LeaseDurationSeconds) * time.Second,
	)
	if time.Now().Before(expiry) {
		return false
	}

	if err := m.client.Delete(ctx, existing); err != nil {
		m.logger.Error(err, "failed to delete expired orphaned Lease",
			"lease", name, "rr_id", rrID,
			"expired_at", expiry.Format(time.RFC3339),
		)
		return false
	}

	m.logger.Info("reclaimed expired orphaned Lease (likely pod restart)",
		"lease", name, "rr_id", rrID,
		"expired_at", expiry.Format(time.RFC3339),
	)
	return true
}

// ReconcileOrphanedLeases scans for K8s Leases with the interactive prefix in
// the namespace and deletes any that have expired. This handles the pod-restart
// scenario where in-memory session state is lost but K8s Leases persist, blocking
// new sessions until TTL expiry. Should be called once at startup.
func (m *LeaseSessionManager) ReconcileOrphanedLeases(ctx context.Context) int {
	leaseList := &coordinationv1.LeaseList{}
	if err := m.client.List(ctx, leaseList, client.InNamespace(m.namespace)); err != nil {
		m.logger.Error(err, "failed to list Leases for orphan reconciliation")
		return 0
	}

	reclaimed := 0
	now := time.Now()
	for i := range leaseList.Items {
		lease := &leaseList.Items[i]
		if len(lease.Name) <= len(leasePrefix) || lease.Name[:len(leasePrefix)] != leasePrefix {
			continue
		}
		if lease.Spec.AcquireTime == nil || lease.Spec.LeaseDurationSeconds == nil {
			continue
		}

		expiry := lease.Spec.AcquireTime.Add(
			time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second,
		)
		if now.Before(expiry) {
			continue
		}

		rrID := lease.Name[len(leasePrefix):]
		if err := m.client.Delete(ctx, lease); err != nil {
			m.logger.Error(err, "failed to delete orphaned Lease during reconciliation",
				"lease", lease.Name, "rr_id", rrID,
			)
			continue
		}

		reclaimed++
		m.logger.Info("reconciled orphaned Lease at startup",
			"lease", lease.Name, "rr_id", rrID,
			"expired_at", expiry.Format(time.RFC3339),
		)
	}

	if reclaimed > 0 {
		m.logger.Info("orphaned Lease reconciliation complete",
			"reclaimed", reclaimed,
			"total_scanned", len(leaseList.Items),
		)
	}
	return reclaimed
}

func leaseName(rrID string) string {
	// rrID may be namespace-qualified ("default/rr-name"); K8s metadata.name
	// forbids '/', so replace with '-'.
	name := leasePrefix + strings.ReplaceAll(rrID, "/", "-")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func nowMicroTime() *metav1.MicroTime {
	t := metav1.NewMicroTime(time.Now())
	return &t
}

var _ SessionManager = (*LeaseSessionManager)(nil)
