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
	client     client.Client
	namespace  string
	sessionTTL time.Duration
	sessions   sync.Map // sessionID -> *sessionEntry
	rrIndex    sync.Map // rrID -> sessionID
	logger     *slog.Logger
}

type sessionEntry struct {
	session    *InteractiveSession
	rrID       string
	signalMeta map[string]string
}

// LeaseOption configures optional parameters for LeaseSessionManager.
type LeaseOption func(*LeaseSessionManager)

// WithSessionTTL overrides the default session TTL used for Lease duration.
func WithSessionTTL(ttl time.Duration) LeaseOption {
	return func(m *LeaseSessionManager) {
		m.sessionTTL = ttl
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

func (m *LeaseSessionManager) Takeover(ctx context.Context, rrID string, user UserInfo) (*InteractiveSession, error) {
	if _, ok := m.rrIndex.Load(rrID); ok {
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
	m.sessions.Store(sessionID, entry)
	m.rrIndex.Store(rrID, sessionID)

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
	return raw.(*sessionEntry).session, nil
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
