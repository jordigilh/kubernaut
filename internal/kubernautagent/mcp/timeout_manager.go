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
	"fmt"
	"sync"
	"time"
)

type timeoutEntry struct {
	inactivityTimer *time.Timer
	warningTimers   []*time.Timer
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
func NewTimeoutManager(inactivityTimeout time.Duration, warningIntervals []time.Duration, onExpire func(sessionID string)) *TimeoutManager {
	return &TimeoutManager{
		inactivityTimeout: inactivityTimeout,
		warningIntervals:  warningIntervals,
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

	entry := &timeoutEntry{
		inactivityTimer: time.AfterFunc(m.inactivityTimeout, func() {
			m.onExpire(sessionID)
		}),
	}

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
