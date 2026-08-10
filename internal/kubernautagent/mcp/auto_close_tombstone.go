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
	"sync"
	"time"
)

// defaultAutoCloseTombstoneTTL matches the order of magnitude of the
// existing disconnectGracePeriod (#2075) used elsewhere in the interactive
// session lifecycle.
const defaultAutoCloseTombstoneTTL = 60 * time.Second

// AutoCloseTombstone is a small, self-expiring record of interactive
// sessions the backend recently auto-closed on its own (#2075).
//
// investigate.go's no_matching_workflows handling releases the MCP
// interactive lease asynchronously, in a goroutine, after the
// discover_workflows response has already been sent to the caller. If the
// console's Dismiss button (kubernaut_complete_no_action) races in after
// that release completes, IsDriverActive(rrID) has already gone false and
// complete_no_action would otherwise fail with "no active interactive
// session" even though the investigation was already correctly concluded.
//
// This is deliberately not a general-purpose cache: it is written only at
// the exact call site that causes the race (the no_matching_workflows
// auto-close goroutine) and read only at the exact call site affected by
// it (complete_no_action.Handle) — no changes to the SessionManager or
// HTTPSessionCompleter interfaces, and no interaction with the #1654/#1949
// inactivity-timer-correctness code.
type AutoCloseTombstone struct {
	mu      sync.Mutex
	entries map[string]struct{}
	ttl     time.Duration
}

// NewAutoCloseTombstone creates a tombstone that forgets a marked rr_id
// after ttl elapses. A ttl <= 0 falls back to defaultAutoCloseTombstoneTTL.
func NewAutoCloseTombstone(ttl time.Duration) *AutoCloseTombstone {
	if ttl <= 0 {
		ttl = defaultAutoCloseTombstoneTTL
	}
	return &AutoCloseTombstone{
		entries: make(map[string]struct{}),
		ttl:     ttl,
	}
}

// Mark records that rrID's interactive session was just auto-closed by the
// backend (not by an explicit user action), self-expiring after ttl. A nil
// receiver is a safe no-op, so callers can treat the tombstone as an
// optional dependency (matching the WithHTTPCompleter/WithTimeoutTracker
// nil-safety convention used elsewhere in this package).
func (a *AutoCloseTombstone) Mark(rrID string) {
	if a == nil || rrID == "" {
		return
	}
	a.mu.Lock()
	a.entries[rrID] = struct{}{}
	a.mu.Unlock()

	time.AfterFunc(a.ttl, func() {
		a.mu.Lock()
		delete(a.entries, rrID)
		a.mu.Unlock()
	})
}

// WasRecentlyAutoClosed reports whether rrID's session was auto-closed by
// the backend within the last ttl. A nil receiver always reports false.
func (a *AutoCloseTombstone) WasRecentlyAutoClosed(rrID string) bool {
	if a == nil || rrID == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.entries[rrID]
	return ok
}
