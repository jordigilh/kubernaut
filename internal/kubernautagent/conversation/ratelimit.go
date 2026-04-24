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

package conversation

import (
	"sync"
	"time"
)

// RateLimiter enforces per-user and per-session rate limits.
type RateLimiter struct {
	mu               sync.Mutex
	perUserPerMinute int
	perSession       int
	userWindows      map[string][]time.Time
}

// NewRateLimiter creates a rate limiter with the given limits.
func NewRateLimiter(perUserPerMinute, perSession int) *RateLimiter {
	return &RateLimiter{
		perUserPerMinute: perUserPerMinute,
		perSession:       perSession,
		userWindows:      make(map[string][]time.Time),
	}
}

// AllowUser checks if the user has not exceeded per-user rate limits
// using a sliding window of 1 minute.
func (r *RateLimiter) AllowUser(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	window := r.userWindows[userID]
	var active []time.Time
	for _, t := range window {
		if t.After(cutoff) {
			active = append(active, t)
		}
	}

	if len(active) >= r.perUserPerMinute {
		r.userWindows[userID] = active
		return false
	}

	r.userWindows[userID] = append(active, now)
	return true
}

// AllowSession checks if the session's turn count has not exceeded the per-session limit.
func (r *RateLimiter) AllowSession(_ string, turnCount int) bool {
	return turnCount <= r.perSession
}
