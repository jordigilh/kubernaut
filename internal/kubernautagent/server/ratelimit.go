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
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

// RateLimitConfig configures per-IP rate limiting.
type RateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
	CleanupInterval   time.Duration
	MaxAge            time.Duration
	TrustedProxyCIDRs []string
}

// DefaultRateLimitConfig returns sensible defaults: 5 req/s with burst of 10,
// cleaning idle entries every 5 minutes.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 5,
		Burst:             10,
		CleanupInterval:   5 * time.Minute,
		MaxAge:            10 * time.Minute,
	}
}

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter tracks per-IP token-bucket limiters.
type RateLimiter struct {
	mu                 sync.Mutex
	limiters           map[string]*ipLimiter
	cfg                RateLimitConfig
	stopOnce           sync.Once
	stopCh             chan struct{}
	rateLimitedCounter prometheus.Counter
	auditStore         audit.AuditStore
	logger             logr.Logger
	trustedNets        []*net.IPNet
}

// RateLimiterOption configures optional RateLimiter behavior.
type RateLimiterOption func(*RateLimiter)

// WithAuditStore enables AU-12 audit events for rate-limit denials (H5).
func WithAuditStore(store audit.AuditStore, logger logr.Logger) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.auditStore = store
		rl.logger = logger
	}
}

// NewRateLimiter creates a per-IP rate limiter and starts a background
// goroutine to evict stale entries.
// rateLimitedCounter may be nil (no metrics emitted).
func NewRateLimiter(cfg RateLimitConfig, rateLimitedCounter prometheus.Counter, opts ...RateLimiterOption) *RateLimiter {
	rl := &RateLimiter{
		limiters:           make(map[string]*ipLimiter),
		cfg:                cfg,
		stopCh:             make(chan struct{}),
		rateLimitedCounter: rateLimitedCounter,
	}
	for _, cidr := range cfg.TrustedProxyCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			rl.trustedNets = append(rl.trustedNets, ipNet)
		}
	}
	for _, opt := range opts {
		opt(rl)
	}
	go rl.cleanup()
	return rl
}

// Stop halts the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stopCh) })
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, ok := rl.limiters[ip]
	if !ok {
		entry = &ipLimiter{
			limiter: rate.NewLimiter(rate.Limit(rl.cfg.RequestsPerSecond), rl.cfg.Burst),
		}
		rl.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.cfg.MaxAge)
			for ip, entry := range rl.limiters {
				if entry.lastSeen.Before(cutoff) {
					delete(rl.limiters, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Middleware returns an http.Handler middleware that rejects requests
// exceeding the per-IP rate limit with HTTP 429.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r, rl.trustedNets)
		if !rl.getLimiter(ip).Allow() {
			if rl.rateLimitedCounter != nil {
				rl.rateLimitedCounter.Inc()
			}
			if rl.auditStore != nil {
				evt := audit.NewEvent(audit.EventTypeRateLimitDenied, "")
				evt.EventAction = audit.ActionRateLimitDenied
				evt.EventOutcome = audit.OutcomeFailure
				evt.Data["source_ip"] = ip
				evt.Data["path"] = r.URL.Path
				evt.Data["method"] = r.Method
				audit.StoreBestEffort(r.Context(), rl.auditStore, evt, rl.logger)
			}
			w.Header().Set("Retry-After", "1")
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractIP(r *http.Request, trustedNets []*net.IPNet) string {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" && isTrustedProxy(remoteIP, trustedNets) {
		if idx := strings.Index(xff, ","); idx != -1 {
			xff = strings.TrimSpace(xff[:idx])
		}
		if ip, _, err := net.SplitHostPort(xff); err == nil {
			return ip
		}
		return strings.TrimSpace(xff)
	}

	return remoteIP
}

func isTrustedProxy(ip string, trustedNets []*net.IPNet) bool {
	if len(trustedNets) == 0 {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range trustedNets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}
