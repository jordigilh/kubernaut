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
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	goredis "github.com/go-redis/redis/v8"
)

// Constants for rate limiting
const (
	rateLimitKeyPrefix = "ratelimit"
)

// createRateLimitKey creates a Redis key for rate limiting a specific source IP
func createRateLimitKey(sourceIP string) string {
	return fmt.Sprintf("%s:%s", rateLimitKeyPrefix, sourceIP)
}

// NewRedisRateLimiter creates rate limiting middleware using Redis.
//
// Business Requirements:
// - BR-GATEWAY-071: Rate limit webhook requests per source IP
// - BR-GATEWAY-072: Prevent DoS attacks through request throttling
//
// Security:
// - VULN-GATEWAY-003: Prevents DoS attacks (CVSS 6.5 - MEDIUM)
//
// This middleware implements a sliding window rate limiter using Redis.
// Each source IP is tracked independently with its own rate limit counter.
//
// Parameters:
// - redisClient: Redis client for distributed rate limiting
// - limit: Maximum number of requests allowed per time window
// - window: Time window for rate limiting (e.g., 1 minute)
//
// Rate Limiting Strategy:
// - Per-source IP tracking
// - Sliding window algorithm
// - Fail-open when Redis unavailable (prioritize availability)
//
// Error Handling:
// - 429 Too Many Requests: Rate limit exceeded
// - Retry-After header: Seconds until rate limit resets
// - Fail-open: Allow requests if Redis unavailable
func NewRedisRateLimiter(redisClient *goredis.Client, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract source IP from request
			sourceIP := extractSourceIP(r.RemoteAddr)

			// Create Redis key for this source IP
			key := createRateLimitKey(sourceIP)

			ctx := context.Background()

			// Increment counter and get current count
			pipe := redisClient.Pipeline()
			incr := pipe.Incr(ctx, key)
			pipe.Expire(ctx, key, window)
			_, err := pipe.Exec(ctx)

			// Fail-open if Redis unavailable (prioritize availability over strict rate limiting)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			count := incr.Val()

			// Check if rate limit exceeded
			if count > int64(limit) {
				// Calculate seconds until window expires
				ttl, _ := redisClient.TTL(ctx, key).Result()
				retryAfter := int(ttl.Seconds())
				if retryAfter <= 0 {
					retryAfter = int(window.Seconds())
				}

				// Return 429 Too Many Requests with Retry-After header
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
				return
			}

			// Continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// extractSourceIP extracts the IP address from RemoteAddr (format: "IP:port")
func extractSourceIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// If parsing fails, return the whole address
		return remoteAddr
	}
	return host
}
