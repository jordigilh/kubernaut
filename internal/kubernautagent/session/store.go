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

package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Status represents the lifecycle state of an investigation session.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	// StatusCancelled indicates the investigation was stopped — either by explicit
	// operator cancellation (CancelInvestigation) or interactive takeover suspension
	// (SuspendInvestigation). Both share this terminal state; the audit event type
	// distinguishes them (session.cancelled vs session.suspended).
	StatusCancelled Status = "cancelled"

	// StatusUserDriving indicates an interactive user has taken over the
	// investigation via MCP dynamic takeover (BR-INTERACTIVE-004). The session
	// remains pollable (NOT terminal) so AA can observe identity and completion.
	StatusUserDriving Status = "user_driving"
)

// Session holds the state of a single investigation session.
type Session struct {
	ID        string
	Status    Status
	Result    *katypes.InvestigationResult
	Error     error
	CreatedAt time.Time
	Context   SessionContext
	Metadata  map[string]string // Deprecated: use Context. Retained for backward compat.

	// cancel, eventChan, and lazySink are manager-managed internal fields.
	// They are NOT part of the public copy surface (clone excludes them).
	cancel    context.CancelFunc
	eventChan chan InvestigationEvent
	lazySink  *LazySink

	// interactiveUpgrade is set by UpgradeToInteractive under store.mu.
	// Checked by store.Update under the same lock to guarantee deterministic
	// upgrade when the goroutine completes (#1390).
	interactiveUpgrade *atomic.Bool

	// deferredFn holds the investigation function for interactive sessions
	// created in pending state (BR-INTERACTIVE-010). Consumed by
	// LaunchDeferredInvestigation.
	deferredFn InvestigateFunc
}

// ErrSessionNotFound is returned when a session ID does not exist in the store.
var ErrSessionNotFound = errors.New("session not found")

// ErrSessionTerminal is returned when an operation is attempted on a session
// that has already reached a terminal state (completed, cancelled, or failed).
var ErrSessionTerminal = errors.New("session is in terminal state")

// ErrMaxInvestigationsReached is returned when the store has reached its
// configured maximum number of concurrent (non-terminal) investigations (M2).
var ErrMaxInvestigationsReached = errors.New("maximum concurrent investigations reached")

// Store provides thread-safe session storage with TTL-based cleanup.
// Terminal sessions (Completed, Failed, Cancelled) are evicted at TTL.
// Non-terminal sessions (Pending, Running) are evicted at MaxSessionAge
// as a safety net to prevent unbounded memory growth.
type Store struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	ttl            time.Duration
	maxSessionAge  time.Duration
	maxConcurrent  int
	logger         logr.Logger
}

// StoreOption configures optional Store behaviour.
type StoreOption func(*Store)

// WithLogger sets the logger for the Store.
func WithLogger(l logr.Logger) StoreOption {
	return func(s *Store) { s.logger = l }
}

// WithMaxSessionAge sets the hard eviction age for non-terminal sessions.
// Must be >= TTL. Defaults to 2 * TTL if not set.
func WithMaxSessionAge(d time.Duration) StoreOption {
	return func(s *Store) { s.maxSessionAge = d }
}

// WithMaxConcurrent sets the maximum number of concurrent (non-terminal)
// investigations. When the cap is reached, Create returns
// ErrMaxInvestigationsReached. A value of 0 disables the cap.
func WithMaxConcurrent(n int) StoreOption {
	return func(s *Store) { s.maxConcurrent = n }
}

// NewStore creates a new session store with the given TTL for cleanup.
// Non-terminal sessions are evicted at MaxSessionAge (default: 2 * TTL).
func NewStore(ttl time.Duration, opts ...StoreOption) *Store {
	s := &Store{
		sessions:      make(map[string]*Session),
		ttl:           ttl,
		maxSessionAge: 2 * ttl,
		logger:        logr.Discard(),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.maxSessionAge < s.ttl {
		panic("session.NewStore: MaxSessionAge must be >= TTL")
	}
	return s
}

// Create stores a new session and returns its ID.
// Returns ErrMaxInvestigationsReached when the concurrent cap is hit.
func (s *Store) Create() (string, error) {
	id := uuid.New().String()
	sess := &Session{
		ID:                 id,
		Status:             StatusPending,
		CreatedAt:          time.Now(),
		lazySink:           &LazySink{},
		interactiveUpgrade: &atomic.Bool{},
	}
	s.mu.Lock()
	if s.maxConcurrent > 0 {
		active := 0
		for _, existing := range s.sessions {
			if !IsTerminal(existing.Status) {
				active++
			}
		}
		if active >= s.maxConcurrent {
			s.mu.Unlock()
			return "", ErrMaxInvestigationsReached
		}
	}
	s.sessions[id] = sess
	s.mu.Unlock()
	return id, nil
}

// Get retrieves a snapshot of a session by ID.
// Returns a copy to avoid data races between the caller and background goroutines.
func (s *Store) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess.clone(), nil
}

// clone returns an isolated copy of the session. Internal control fields
// (cancel, eventChan) are excluded to prevent callers from interfering
// with active investigations. SessionContext is a value type except for
// SignalContext.SignalAnnotations and SignalLabels which are deep-copied.
func (s *Session) clone() *Session {
	cp := *s
	cp.cancel = nil
	cp.eventChan = nil
	cp.lazySink = nil
	if s.Metadata != nil {
		cp.Metadata = make(map[string]string, len(s.Metadata))
		for k, v := range s.Metadata {
			cp.Metadata[k] = v
		}
	}
	if s.Context.Signal.SignalAnnotations != nil {
		cp.Context.Signal.SignalAnnotations = make(map[string]string, len(s.Context.Signal.SignalAnnotations))
		for k, v := range s.Context.Signal.SignalAnnotations {
			cp.Context.Signal.SignalAnnotations[k] = v
		}
	}
	if s.Context.Signal.SignalLabels != nil {
		cp.Context.Signal.SignalLabels = make(map[string]string, len(s.Context.Signal.SignalLabels))
		for k, v := range s.Context.Signal.SignalLabels {
			cp.Context.Signal.SignalLabels[k] = v
		}
	}
	return &cp
}

// IsTerminal reports whether the given status represents a final state
// that cannot be changed (completed, failed, or cancelled).
func IsTerminal(st Status) bool {
	return st == StatusCompleted || st == StatusFailed || st == StatusCancelled
}

// IsTerminal reports whether this status represents a final state.
func (st Status) IsTerminal() bool {
	return IsTerminal(st)
}

// SetMetadata stores request-level metadata on an existing session.
// Deprecated: Use SetContext for typed access. Retained for backward compatibility.
func (s *Store) SetMetadata(id string, metadata map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Metadata = metadata
	}
}

// SetContext stores typed SessionContext on an existing session.
func (s *Store) SetContext(id string, ctx SessionContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Context = ctx
	}
}

// Update modifies an existing session. Returns ErrSessionTerminal if the
// session has already reached a terminal state (completed, cancelled, failed).
//
// Deterministic upgrade (#1390): if the goroutine completes with StatusCompleted
// but UpgradeToInteractive has set interactiveUpgrade=true (under this same mu),
// the status is forced to StatusUserDriving and result.InteractiveHold is set.
// This serialization eliminates the race between upgrade and completion.
func (s *Store) Update(id string, status Status, result *katypes.InvestigationResult, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if IsTerminal(sess.Status) || sess.Status == StatusUserDriving {
		return ErrSessionTerminal
	}
	isSecurityEscalation := result != nil && result.HumanReviewNeeded && result.HumanReviewReason == katypes.HumanReviewReasonAlignmentCheckFailed
	if status == StatusCompleted && sess.interactiveUpgrade != nil && sess.interactiveUpgrade.Load() && !isSecurityEscalation {
		status = StatusUserDriving
		if result != nil {
			result.InteractiveHold = true
		}
	}
	sess.Status = status
	sess.Result = result
	sess.Error = err
	return nil
}

// SetResult attaches a result to an existing session without changing its
// status. Used to persist partial investigation state on cancelled sessions
// where Store.Update would reject the status transition (BR-SESSION-002).
// First-write-wins: on UserDriving sessions, accepts the first write (nil
// result) so the cancelled investigation goroutine can preserve its result
// for discover_workflows (#1425). Blocks subsequent writes to prevent the
// goroutine from overwriting a user-provided result. Also blocks overwrites
// on completed sessions.
func (s *Store) SetResult(id string, result *katypes.InvestigationResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		if sess.Status == StatusUserDriving && sess.Result != nil {
			return
		}
		if sess.Status == StatusCompleted && sess.Result != nil {
			return
		}
		sess.Result = result
	}
}

// CompleteUserDriving transitions a session from StatusUserDriving to
// StatusCompleted with the given InvestigationResult. This is the only path
// that allows a user-driven session to reach completion (Update blocks this
// transition). Used by select_workflow and complete_no_action MCP tools.
func (s *Store) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if sess.Status != StatusUserDriving {
		return fmt.Errorf("cannot complete: status is %s, expected %s", sess.Status, StatusUserDriving)
	}
	sess.Status = StatusCompleted
	if result != nil {
		sess.Result = result
	}
	return nil
}

// StartCleanupLoop runs Cleanup periodically until the context is cancelled.
func (s *Store) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.Cleanup()
			}
		}
	}()
}

// Cleanup removes sessions using two-tier eviction (#1078):
// - Terminal sessions (Completed, Failed, Cancelled) are evicted after TTL
// - Non-terminal sessions (Pending, Running, UserDriving) are evicted after MaxSessionAge
// Returns the number of sessions removed.
func (s *Store) Cleanup() int {
	now := time.Now()
	terminalCutoff := now.Add(-s.ttl)
	nonTerminalCutoff := now.Add(-s.maxSessionAge)
	removed := 0
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.sessions {
		switch sess.Status {
		case StatusCompleted, StatusFailed, StatusCancelled:
			if sess.CreatedAt.Before(terminalCutoff) {
				delete(s.sessions, id)
				removed++
			}
		default:
			if sess.CreatedAt.Before(nonTerminalCutoff) {
				if sess.cancel != nil {
					sess.cancel()
				}
				if sess.Status == StatusPending {
					s.logger.Info("evicting pending interactive session (user never connected)",
						"session_id", id,
						"remediation_id", sess.Metadata["remediation_id"],
						"age", now.Sub(sess.CreatedAt).String())
				} else {
					s.logger.Info("evicting non-terminal session (TTL exceeded)",
						"session_id", id,
						"status", sess.Status,
						"age", now.Sub(sess.CreatedAt).String())
				}
				delete(s.sessions, id)
				removed++
			}
		}
	}
	return removed
}
