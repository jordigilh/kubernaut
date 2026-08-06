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
	"fmt"
	"sync"
	"time"
)

type timeoutEntry struct {
	inactivityTimer *time.Timer
	warningTimers   []*time.Timer
	// activeCancel, when non-nil, is invoked before onExpire fires (BR-KA-267,
	// #1949) so an in-flight investigation's context is actively torn down
	// on session expiry, rather than the session merely being marked expired
	// while its goroutine keeps running unbounded. Set via SetActiveCancel,
	// cleared via ClearActiveCancel once the action it guards completes.
	activeCancel context.CancelFunc
}

// TimeoutManager tracks per-session inactivity and fires warnings and
// expiration callbacks. BR-INTERACTIVE-003: inactivity timeout releases session.
// Warnings are human-readable strings delivered via the notify callback.
type TimeoutManager struct {
	inactivityTimeout time.Duration
	warningIntervals  []time.Duration
	onExpire          func(sessionID string)

	mu       sync.Mutex
	sessions map[string]*timeoutEntry
}

// NewTimeoutManager creates a manager with the given inactivity timeout,
// warning intervals (from session start), and expiration callback.
// Intervals that exceed the timeout are silently discarded.
func NewTimeoutManager(inactivityTimeout time.Duration, warningIntervals []time.Duration, onExpire func(sessionID string)) *TimeoutManager {
	var validIntervals []time.Duration
	for _, iv := range warningIntervals {
		if iv > 0 && iv < inactivityTimeout {
			validIntervals = append(validIntervals, iv)
		}
	}
	return &TimeoutManager{
		inactivityTimeout: inactivityTimeout,
		warningIntervals:  validIntervals,
		onExpire:          onExpire,
		sessions:          make(map[string]*timeoutEntry),
	}
}

// StartTracking begins monitoring the session for inactivity and sets up
// warning timers. The notify callback receives human-readable warning messages.
// Safe to call multiple times: old timers are stopped before replacement.
func (m *TimeoutManager) StartTracking(sessionID string, notify func(msg string)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if old, exists := m.sessions[sessionID]; exists {
		old.inactivityTimer.Stop()
		for _, t := range old.warningTimers {
			t.Stop()
		}
	}

	entry := &timeoutEntry{}
	entry.inactivityTimer = time.AfterFunc(m.inactivityTimeout, func() {
		// BR-KA-267, #1949: cascade expiry into cancellation of whatever
		// in-flight action's context was registered via SetActiveCancel,
		// before onExpire runs its own (unrelated) session-cleanup logic.
		m.mu.Lock()
		cancel := entry.activeCancel
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		m.onExpire(sessionID)
	})

	for _, interval := range m.warningIntervals {
		remaining := m.inactivityTimeout - interval
		msg := fmt.Sprintf("Your interactive session will timeout in %s due to inactivity.", remaining.Round(time.Second))
		t := time.AfterFunc(interval, func() {
			notify(msg)
		})
		entry.warningTimers = append(entry.warningTimers, t)
	}

	m.sessions[sessionID] = entry
}

// ResetInactivity resets the inactivity timer for the given session,
// preventing timeout when the user is active.
func (m *TimeoutManager) ResetInactivity(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return
	}
	entry.inactivityTimer.Reset(m.inactivityTimeout)
}

// SetActiveCancel registers cancel to be invoked when sessionID's inactivity
// timer expires, before onExpire fires (BR-KA-267, #1949). This lets a
// caller cascade session-inactivity expiry into cancellation of whatever
// context an in-flight action (e.g. a tool-dispatch loop) derived from, so
// an expired session cannot leave that action running unbounded. A no-op if
// sessionID is not currently tracked (StartTracking was never called, or
// the session already stopped).
func (m *TimeoutManager) SetActiveCancel(sessionID string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sessions[sessionID]; ok {
		entry.activeCancel = cancel
	}
}

// ClearActiveCancel removes the CancelFunc registered via SetActiveCancel for
// sessionID, so a later inactivity expiry (for the next, unrelated action
// sharing the same session) does not invoke a stale CancelFunc bound to an
// action that already completed.
func (m *TimeoutManager) ClearActiveCancel(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.sessions[sessionID]; ok {
		entry.activeCancel = nil
	}
}

// StopAll stops all tracked sessions and removes all entries.
// Called during shutdown to prevent timer/goroutine leaks.
func (m *TimeoutManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, entry := range m.sessions {
		entry.inactivityTimer.Stop()
		for _, t := range entry.warningTimers {
			t.Stop()
		}
		delete(m.sessions, id)
	}
}

// StopTracking stops all timers and removes the session entry.
func (m *TimeoutManager) StopTracking(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.sessions[sessionID]
	if !ok {
		return
	}

	entry.inactivityTimer.Stop()
	for _, t := range entry.warningTimers {
		t.Stop()
	}
	delete(m.sessions, sessionID)
}
