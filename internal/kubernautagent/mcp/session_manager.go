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
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

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
	logger            *slog.Logger
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

// NewLeaseSessionManager creates a LeaseSessionManager backed by the given K8s client.
func NewLeaseSessionManager(c client.Client, namespace string, logger *slog.Logger, opts ...LeaseOption) SessionManager {
	return NewLeaseSessionManagerConcrete(c, namespace, logger, opts...)
}

// NewLeaseSessionManagerConcrete returns the concrete *LeaseSessionManager type
// for callers that need access to signal metadata storage (e.g., disconnect handler).
func NewLeaseSessionManagerConcrete(c client.Client, namespace string, logger *slog.Logger, opts ...LeaseOption) *LeaseSessionManager {
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

	// UX-03: Check local index first and provide holder context in error.
	if existingSessionID, ok := m.rrIndex.Load(rrID); ok {
		if raw, found := m.sessions.Load(existingSessionID); found {
			entry := raw.(*sessionEntry)
			return nil, fmt.Errorf("%w: held by %q since %s",
				ErrLeaseHeld, entry.session.ActingUser.Username, entry.session.StartedAt.Format(time.RFC3339))
		}
		return nil, ErrLeaseHeld
	}

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

	if err := m.client.Create(ctx, lease); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, ErrLeaseHeld
		}
		return nil, fmt.Errorf("create lease for rr %q: %w", rrID, err)
	}

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
		slog.String("session_id", sessionID),
		slog.String("rr_id", rrID),
		slog.String("user", user.Username),
	)

	return session, nil
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
		slog.String("session_id", sessionID),
		slog.String("rr_id", entry.rrID),
		slog.String("reason", reason),
	)

	return nil
}

func (m *LeaseSessionManager) GetDriver(rrID string) (*InteractiveSession, error) {
	raw, ok := m.rrIndex.Load(rrID)
	if !ok {
		return nil, nil
	}
	sessionID := raw.(string)

	raw, ok = m.sessions.Load(sessionID)
	if !ok {
		return nil, nil
	}
	entry := raw.(*sessionEntry)

	// SEC-04: Check session TTL expiry.
	if m.sessionTTL > 0 && time.Since(entry.session.StartedAt) > m.sessionTTL {
		m.logger.Warn("session TTL expired, auto-releasing",
			slog.String("session_id", sessionID),
			slog.String("rr_id", rrID))
		_ = m.Release(sessionID, "ttl_expired")
		return nil, ErrSessionExpired
	}

	// SEC-04: Check inactivity timeout.
	if m.inactivityTimeout > 0 {
		if lastAct, ok := entry.lastActivity.Load().(time.Time); ok {
			if time.Since(lastAct) > m.inactivityTimeout {
				m.logger.Warn("session inactivity timeout, auto-releasing",
					slog.String("session_id", sessionID),
					slog.String("rr_id", rrID))
				_ = m.Release(sessionID, "inactivity_timeout")
				return nil, ErrSessionExpired
			}
		}
	}

	return entry.session, nil
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

func leaseName(rrID string) string {
	name := leasePrefix + rrID
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
