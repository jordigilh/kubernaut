/*
Copyright 2025 Jordi Gil.

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
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
)

// RateLimiter implements per-IP rate limiting using token bucket algorithm
//
// Rate limiting is essential to prevent:
// 1. Denial of Service (DoS) attacks
// 2. Accidental alert storms (misconfigured Prometheus rules)
// 3. Resource exhaustion (CPU, memory, Redis connections, Kubernetes API calls)
//
// Algorithm: Token bucket (from golang.org/x/time/rate)
// - Each IP gets a bucket with N tokens (burst capacity)
// - Tokens refill at R tokens/second (sustained rate)
// - Each request consumes 1 token
// - If bucket is empty: request is rejected with HTTP 429
//
// Default limits (per source IP):
// - Rate: 100 requests/minute (1.67 requests/second)
// - Burst: 10 requests (allows short bursts)
//
// Example scenarios:
// - Normal traffic: 1 request/second → Always allowed
// - Burst traffic: 10 requests in 1 second → Allowed (uses burst capacity)
// - Storm traffic: 200 requests/minute → First 10 allowed, rest rejected
//
// Performance:
// - Typical overhead: p95 ~0.5ms (map lookup + atomic counter)
// - Memory: ~200 bytes per unique IP (limiter + metadata)
// - Cleanup: Stale IPs removed every 10 minutes
type RateLimiter struct {
	// limiters maps source IP → token bucket limiter
	limiters map[string]*rate.Limiter

	// mu protects limiters map (concurrent access)
	mu sync.RWMutex

	// rate is the sustained requests/second allowed per IP
	rate rate.Limit

	// burst is the maximum burst size (token bucket capacity)
	burst int

	// cleanupInterval is how often to remove stale IP entries
	cleanupInterval time.Duration

	// logger for rate limiting events
	logger *logrus.Logger
}

// NewRateLimiter creates a new rate limiter middleware
//
// Parameters:
// - requestsPerMinute: Sustained rate limit (e.g., 100 for 100 req/min)
// - burst: Maximum burst size (e.g., 10 for short bursts)
// - logger: Structured logger
//
// Example:
//
//	// 100 requests/minute with burst of 10
//	limiter := NewRateLimiter(100, 10, logger)
func NewRateLimiter(requestsPerMinute int, burst int, logger *logrus.Logger) *RateLimiter {
	// Convert requests/minute to requests/second for rate.Limiter
	requestsPerSecond := float64(requestsPerMinute) / 60.0

	rl := &RateLimiter{
		limiters:        make(map[string]*rate.Limiter),
		rate:            rate.Limit(requestsPerSecond),
		burst:           burst,
		cleanupInterval: 10 * time.Minute,
		logger:          logger,
	}

	// Start background cleanup goroutine
	go rl.cleanupStaleIPs()

	return rl
}

// Middleware returns an HTTP middleware function
//
// This middleware:
// 1. Extracts source IP from request
// 2. Gets or creates rate limiter for this IP
// 3. Checks if request is allowed (token available)
// 4. Rejects with HTTP 429 if rate limit exceeded
// 5. Passes allowed requests to next handler
// 6. Records rate limiting metrics
//
// HTTP responses:
// - 429 Too Many Requests: Rate limit exceeded
// - 200/202: Request allowed (passed to next handler)
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract source IP
		ip := rl.extractIP(r)

		// Get or create limiter for this IP
		limiter := rl.getLimiter(ip)

		// Check if request is allowed
		if !limiter.Allow() {
			// Rate limit exceeded
			metrics.RateLimitingDroppedSignalsTotal.WithLabelValues(ip).Inc()

			rl.logger.WithFields(logrus.Fields{
				"remote_addr": ip,
				"path":        r.URL.Path,
			}).Warn("Rate limit exceeded")

			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		// Request allowed
		next.ServeHTTP(w, r)
	})
}

// getLimiter retrieves or creates a rate limiter for the given IP
//
// This method:
// 1. Checks if limiter exists for IP (fast path, read lock)
// 2. If not, creates new limiter (slow path, write lock)
// 3. Returns limiter for token bucket check
//
// Thread safety: Uses RWMutex for concurrent map access
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	// Fast path: check if limiter exists (read lock)
	rl.mu.RLock()
	limiter, exists := rl.limiters[ip]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	// Slow path: create new limiter (write lock)
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check: another goroutine might have created it
	limiter, exists = rl.limiters[ip]
	if exists {
		return limiter
	}

	// Create new limiter
	limiter = rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = limiter

	rl.logger.WithFields(logrus.Fields{
		"ip":    ip,
		"rate":  rl.rate,
		"burst": rl.burst,
	}).Debug("Created new rate limiter for IP")

	return limiter
}

// extractIP extracts source IP from request
//
// This method checks multiple sources:
// 1. X-Forwarded-For header (if behind load balancer/proxy)
// 2. X-Real-IP header (alternative proxy header)
// 3. RemoteAddr field (direct connection)
//
// X-Forwarded-For format: "client-ip, proxy1-ip, proxy2-ip"
// We use the first IP (client-ip) for rate limiting.
//
// Security note: X-Forwarded-For can be spoofed by malicious clients.
// In production, configure your load balancer to:
// - Strip X-Forwarded-For headers from untrusted sources
// - Add trusted X-Forwarded-For header with real client IP
func (rl *RateLimiter) extractIP(r *http.Request) string {
	// Check X-Forwarded-For (comma-separated list)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take first IP (client IP)
		for idx := 0; idx < len(xff); idx++ {
			if xff[idx] == ',' {
				return xff[:idx]
			}
		}
		return xff
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fallback to RemoteAddr
	// RemoteAddr format: "ip:port" or "[ipv6]:port"
	remoteAddr := r.RemoteAddr

	// Strip port
	for i := len(remoteAddr) - 1; i >= 0; i-- {
		if remoteAddr[i] == ':' {
			return remoteAddr[:i]
		}
	}

	return remoteAddr
}

// cleanupStaleIPs removes rate limiters for IPs that haven't been seen recently
//
// This background goroutine:
// 1. Runs every cleanupInterval (default: 10 minutes)
// 2. Removes limiters that have been idle (no recent requests)
// 3. Prevents memory leaks from long-lived limiters
//
// Cleanup strategy:
// - If limiter has burst tokens available: IP hasn't sent requests recently
// - Remove limiter to free memory
// - Next request from this IP will create a new limiter
func (rl *RateLimiter) cleanupStaleIPs() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()

		// Track IPs to remove
		toRemove := make([]string, 0)

		// Check each limiter
		for ip, limiter := range rl.limiters {
			// If limiter has full burst capacity: no recent requests
			// This is a heuristic, not perfect, but simple and effective
			if limiter.Tokens() >= float64(rl.burst) {
				toRemove = append(toRemove, ip)
			}
		}

		// Remove stale IPs
		for _, ip := range toRemove {
			delete(rl.limiters, ip)
		}

		rl.mu.Unlock()

		if len(toRemove) > 0 {
			rl.logger.WithField("removed_count", len(toRemove)).Debug("Cleaned up stale IP rate limiters")
		}
	}
}
