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

package middleware

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"golang.org/x/time/rate"
)

// IPLimiterConfig configures the per-IP rate limiter (BR-STORAGE-1505, GAP-09, Issue #1505).
type IPLimiterConfig struct {
	// RequestsPerSecond is the sustained per-IP request rate.
	RequestsPerSecond float64
	// Burst is the maximum burst size for the token bucket.
	Burst int
	// CleanupInterval controls how often stale IP entries are evicted (default 5m).
	CleanupInterval time.Duration
	// MaxAge is the idle duration after which an IP entry is evicted (default 10m).
	MaxAge time.Duration
}

// IPLimiter provides per-IP token bucket rate limiting for the Data Storage
// HTTP API — a pre-authentication defense against a single client (or a
// small set of IPs) exhausting server resources (FedRAMP SC-5, GAP-09).
type IPLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipEntry
	cfg      IPLimiterConfig
	stopCh   chan struct{}
	stopOnce sync.Once
}

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPLimiter creates a per-IP rate limiter and starts background eviction
// of stale entries. Callers must call Stop() to release the eviction goroutine.
func NewIPLimiter(cfg IPLimiterConfig) *IPLimiter {
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 10 * time.Minute
	}
	l := &IPLimiter{
		limiters: make(map[string]*ipEntry),
		cfg:      cfg,
		stopCh:   make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// Allow reports whether the given IP is within its rate limit, creating a
// fresh token bucket for previously unseen IPs.
func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	entry, ok := l.limiters[ip]
	if !ok {
		entry = &ipEntry{
			limiter: rate.NewLimiter(rate.Limit(l.cfg.RequestsPerSecond), l.cfg.Burst),
		}
		l.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	l.mu.Unlock()

	return entry.limiter.Allow()
}

// Stop halts the background eviction goroutine. Safe to call multiple times.
func (l *IPLimiter) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}

func (l *IPLimiter) cleanup() {
	ticker := time.NewTicker(l.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.mu.Lock()
			cutoff := time.Now().Add(-l.cfg.MaxAge)
			for ip, entry := range l.limiters {
				if entry.lastSeen.Before(cutoff) {
					delete(l.limiters, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}

// RatelimitAuditFunc is called (best-effort, non-blocking) when a request is
// denied by the IP rate limiter, so the denial can be recorded as a
// self-audit event (FedRAMP AU-12, GAP-09). Implementations must not block.
type RatelimitAuditFunc func(ctx context.Context, sourceIP, path, method string)

// IPRateLimitMiddlewareConfig holds dependencies for the per-IP rate-limit middleware.
type IPRateLimitMiddlewareConfig struct {
	Limiter *IPLimiter
	Logger  logr.Logger
	// AuditFunc records a rate-limit denial as a self-audit event. Optional —
	// when nil, denials are still logged and enforced but not separately audited.
	AuditFunc RatelimitAuditFunc
}

// IPRateLimitMiddleware returns Chi middleware enforcing per-IP rate limits on
// the Data Storage HTTP API (GAP-09, Issue #1505 — SC-5 DoS protection).
// Returns RFC 7807 429 with Retry-After when the limit is exceeded.
//
// Placed before authentication in the middleware chain: an unauthenticated
// flood should not reach (and pay the cost of) TokenReview/SAR calls.
func IPRateLimitMiddleware(cfg IPRateLimitMiddlewareConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractClientIP(r)
			if !cfg.Limiter.Allow(ip) {
				cfg.Logger.Info("Rejecting request: per-IP rate limit exceeded (GAP-09, SC-5)",
					"request_id", middleware.GetReqID(r.Context()),
					"source_ip", ip,
					"method", r.Method,
					"path", r.URL.Path)
				if cfg.AuditFunc != nil {
					cfg.AuditFunc(r.Context(), ip, r.URL.Path, r.Method)
				}
				retryAfter := 1
				if cfg.Limiter.cfg.RequestsPerSecond > 0 {
					retryAfter = int(1.0 / cfg.Limiter.cfg.RequestsPerSecond)
					if retryAfter < 1 {
						retryAfter = 1
					}
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writeRateLimitExceeded(w, cfg.Logger)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeRateLimitExceeded writes a 429 RFC 7807 response.
func writeRateLimitExceeded(w http.ResponseWriter, logger logr.Logger) {
	problem := map[string]interface{}{
		"type":   "https://kubernaut.ai/problems/rate-limit-exceeded",
		"title":  "Rate Limit Exceeded",
		"status": http.StatusTooManyRequests,
		"detail": "Too many requests from your IP address. Please retry later.",
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusTooManyRequests)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		logger.Error(err, "Failed to encode 429 RFC 7807 response")
	}
}

// extractClientIP returns the request's client IP, preferring the value
// already resolved by chi's RealIP middleware (X-Forwarded-For/X-Real-IP)
// which MUST run earlier in the chain (see server.go Handler()).
func extractClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
