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

package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	"golang.org/x/time/rate"
)

// UserRateLimitConfig configures per-user token-bucket rate limiting (SEC-02).
type UserRateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
	CleanupInterval   time.Duration
	MaxAge            time.Duration
}

// DefaultUserRateLimitConfig returns sensible defaults for MCP interactive.
func DefaultUserRateLimitConfig(rps int) UserRateLimitConfig {
	if rps <= 0 {
		rps = 10
	}
	return UserRateLimitConfig{
		RequestsPerSecond: float64(rps),
		Burst:             rps * 2,
		CleanupInterval:   5 * time.Minute,
		MaxAge:            10 * time.Minute,
	}
}

type userLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// UserRateLimiter tracks per-authenticated-user token-bucket limiters.
// Keyed by the username extracted from request context (set by auth middleware).
type UserRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*userLimiter
	cfg      UserRateLimitConfig
	stopOnce sync.Once
	stopCh   chan struct{}
	counter  prometheus.Counter
}

// NewUserRateLimiter creates a per-user rate limiter and starts background cleanup.
func NewUserRateLimiter(cfg UserRateLimitConfig, counter prometheus.Counter) *UserRateLimiter {
	rl := &UserRateLimiter{
		limiters: make(map[string]*userLimiter),
		cfg:      cfg,
		stopCh:   make(chan struct{}),
		counter:  counter,
	}
	go rl.cleanup()
	return rl
}

// Stop halts the background cleanup goroutine.
func (rl *UserRateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stopCh) })
}

func (rl *UserRateLimiter) getLimiter(username string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, ok := rl.limiters[username]
	if !ok {
		entry = &userLimiter{
			limiter: rate.NewLimiter(rate.Limit(rl.cfg.RequestsPerSecond), rl.cfg.Burst),
		}
		rl.limiters[username] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

func (rl *UserRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.cfg.MaxAge)
			for user, entry := range rl.limiters {
				if entry.lastSeen.Before(cutoff) {
					delete(rl.limiters, user)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Middleware returns an http.Handler middleware that rate-limits by authenticated user.
// Requests without an authenticated user identity are rejected with 401.
func (rl *UserRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := sharedauth.GetUserFromContext(r.Context())
		if username == "" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		if !rl.getLimiter(username).Allow() {
			if rl.counter != nil {
				rl.counter.Inc()
			}
			w.Header().Set("Retry-After", "1")
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
