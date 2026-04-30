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
	"errors"
	"fmt"
	"sync"
	"time"
)

type rateLimitWindow struct {
	mu         sync.Mutex
	timestamps []time.Time
}

// ErrRateLimited is the sentinel error returned when a session exceeds its
// message rate limit. The tools layer wraps this into a structured MCPError.
var ErrRateLimited = errors.New("rate limited")

// SessionRateLimiter enforces per-session message rate limits using a sliding
// window. SEC-06: prevents abuse of interactive endpoints.
type SessionRateLimiter struct {
	maxPerMinute   int
	maxMessageSize int
	windows        sync.Map // sessionID -> *rateLimitWindow
}

// NewSessionRateLimiter creates a rate limiter with the given limits.
func NewSessionRateLimiter(maxPerMinute, maxMessageSize int) *SessionRateLimiter {
	return &SessionRateLimiter{
		maxPerMinute:   maxPerMinute,
		maxMessageSize: maxMessageSize,
	}
}

// Remove cleans up the rate limit window for a disconnected session,
// preventing sync.Map memory leaks (#28).
func (r *SessionRateLimiter) Remove(sessionID string) {
	r.windows.Delete(sessionID)
}

// Allow checks whether a message from the given session is allowed.
// Returns ErrRateLimited if the session exceeds max_messages_per_minute or
// the message exceeds maxMessageSize.
func (r *SessionRateLimiter) Allow(sessionID string, messageSize int) error {
	if messageSize > r.maxMessageSize {
		return fmt.Errorf("message size %d exceeds limit %d: %w", messageSize, r.maxMessageSize, ErrRateLimited)
	}

	raw, _ := r.windows.LoadOrStore(sessionID, &rateLimitWindow{})
	w := raw.(*rateLimitWindow)

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)

	valid := w.timestamps[:0]
	for _, ts := range w.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	w.timestamps = valid

	if len(w.timestamps) >= r.maxPerMinute {
		return fmt.Errorf("session %s exceeded %d messages/minute: %w", sessionID, r.maxPerMinute, ErrRateLimited)
	}

	w.timestamps = append(w.timestamps, now)
	return nil
}
