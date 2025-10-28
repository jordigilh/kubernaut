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
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

var _ = Describe("Rate Limiting (VULN-GATEWAY-003)", func() {
	var (
		redisServer *miniredis.Miniredis
		redisClient *goredis.Client
		recorder    *httptest.ResponseRecorder
		testHandler http.Handler
	)

	BeforeEach(func() {
		// Setup in-memory Redis for testing
		var err error
		redisServer, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())

		redisClient = goredis.NewClient(&goredis.Options{
			Addr: redisServer.Addr(),
		})

		recorder = httptest.NewRecorder()

		// Create test handler that always returns 200 OK
		testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		})
	})

	AfterEach(func() {
		if redisClient != nil {
			_ = redisClient.Close()
		}
		if redisServer != nil {
			redisServer.Close()
		}
	})

	// TDD RED Phase - Test 1: Allow requests within rate limit
	Context("Within Rate Limit", func() {
		It("should allow requests within rate limit (100 req/min)", func() {
			// Arrange: Create rate limiter with 100 req/min limit
			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 100, time.Minute)
			handler := rateLimiter(testHandler)

			// Act: Send 50 requests (well within limit)
			for i := 0; i < 50; i++ {
				recorder = httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req.RemoteAddr = "192.168.1.100:12345"
				handler.ServeHTTP(recorder, req)

				// Assert: All requests should succeed
				Expect(recorder.Code).To(Equal(http.StatusOK), fmt.Sprintf("Request %d should succeed", i+1))
			}
		})

		It("should track rate limit per source IP", func() {
			// Arrange
			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 10, time.Minute)
			handler := rateLimiter(testHandler)

			// Act: Send 10 requests from IP1 and 10 from IP2
			for i := 0; i < 10; i++ {
				// IP1 requests
				recorder = httptest.NewRecorder()
				req1 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req1.RemoteAddr = "192.168.1.100:12345"
				handler.ServeHTTP(recorder, req1)
				Expect(recorder.Code).To(Equal(http.StatusOK))

				// IP2 requests
				recorder = httptest.NewRecorder()
				req2 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req2.RemoteAddr = "192.168.1.200:12345"
				handler.ServeHTTP(recorder, req2)
				Expect(recorder.Code).To(Equal(http.StatusOK))
			}

			// Assert: Both IPs should have 10 requests each (not shared)
			// This test verifies per-IP tracking works correctly
		})
	})

	// TDD RED Phase - Test 2-3: Reject requests exceeding rate limit
	Context("Exceeding Rate Limit", func() {
		It("should reject requests exceeding rate limit with 429", func() {
			// Arrange: Create rate limiter with very low limit (5 req/min)
			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 5, time.Minute)
			handler := rateLimiter(testHandler)

			sourceIP := "192.168.1.100:12345"

			// Act: Send 10 requests (exceeds limit of 5)
			successCount := 0
			rejectedCount := 0

			for i := 0; i < 10; i++ {
				recorder = httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req.RemoteAddr = sourceIP
				handler.ServeHTTP(recorder, req)

				switch recorder.Code {
				case http.StatusOK:
					successCount++
				case http.StatusTooManyRequests:
					rejectedCount++
				}
			}

			// Assert: First 5 should succeed, next 5 should be rejected
			Expect(successCount).To(Equal(5), "First 5 requests should succeed")
			Expect(rejectedCount).To(Equal(5), "Next 5 requests should be rejected with 429")
		})

		It("should include Retry-After header in 429 response", func() {
			// Arrange
			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 1, time.Minute)
			handler := rateLimiter(testHandler)

			sourceIP := "192.168.1.100:12345"

			// Act: Send 2 requests (exceeds limit of 1)
			req1 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req1.RemoteAddr = sourceIP
			handler.ServeHTTP(httptest.NewRecorder(), req1)

			recorder = httptest.NewRecorder()
			req2 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req2.RemoteAddr = sourceIP
			handler.ServeHTTP(recorder, req2)

			// Assert: Second request should have Retry-After header
			Expect(recorder.Code).To(Equal(http.StatusTooManyRequests))
			Expect(recorder.Header().Get("Retry-After")).ToNot(BeEmpty())
		})

		It("should reset rate limit after time window expires", func() {
			// Arrange: Create rate limiter with 2 req/second window
			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 2, time.Second)
			handler := rateLimiter(testHandler)

			sourceIP := "192.168.1.100:12345"

			// Act: Send 2 requests (at limit)
			for i := 0; i < 2; i++ {
				recorder = httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req.RemoteAddr = sourceIP
				handler.ServeHTTP(recorder, req)
				Expect(recorder.Code).To(Equal(http.StatusOK))
			}

			// Third request should be rejected
			recorder = httptest.NewRecorder()
			req3 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req3.RemoteAddr = sourceIP
			handler.ServeHTTP(recorder, req3)
			Expect(recorder.Code).To(Equal(http.StatusTooManyRequests))

			// Wait for window to expire
			redisServer.FastForward(2 * time.Second)

			// Fourth request should succeed (window reset)
			recorder = httptest.NewRecorder()
			req4 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req4.RemoteAddr = sourceIP
			handler.ServeHTTP(recorder, req4)
			Expect(recorder.Code).To(Equal(http.StatusOK), "Request should succeed after window reset")
		})
	})

	// TDD RED Phase - Test 4: Redis unavailable handling
	Context("Redis Unavailable", func() {
		It("should allow requests when Redis is unavailable (fail-open)", func() {
			// Arrange: Close Redis to simulate unavailability
			redisServer.Close()

			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 5, time.Minute)
			handler := rateLimiter(testHandler)

			// Act: Send request with Redis unavailable
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req.RemoteAddr = "192.168.1.100:12345"
			handler.ServeHTTP(recorder, req)

			// Assert: Request should succeed (fail-open for availability)
			Expect(recorder.Code).To(Equal(http.StatusOK), "Should fail-open when Redis unavailable")
		})
	})

	// Priority 1 Edge Case: IPv6 address support
	Context("IPv6 Address Support (CRITICAL)", func() {
		It("should rate limit IPv6 addresses correctly", func() {
			// Arrange: Create rate limiter with low limit
			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 2, time.Minute)
			handler := rateLimiter(testHandler)

			// IPv6 address with port
			ipv6Addr := "[2001:db8::1]:12345"

			// Act: Send 3 requests from same IPv6 address
			successCount := 0
			rejectedCount := 0

			for i := 0; i < 3; i++ {
				recorder = httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req.RemoteAddr = ipv6Addr
				handler.ServeHTTP(recorder, req)

				switch recorder.Code {
				case http.StatusOK:
					successCount++
				case http.StatusTooManyRequests:
					rejectedCount++
				}
			}

			// Assert: First 2 should succeed, third should be rejected
			Expect(successCount).To(Equal(2), "First 2 IPv6 requests should succeed")
			Expect(rejectedCount).To(Equal(1), "Third IPv6 request should be rejected")
		})

		It("should rate limit different IPv6 addresses independently", func() {
			// Arrange
			rateLimiter := middleware.NewRedisRateLimiter(redisClient, 2, time.Minute)
			handler := rateLimiter(testHandler)

			ipv6Addr1 := "[2001:db8::1]:12345"
			ipv6Addr2 := "[2001:db8::2]:12345"

			// Act: Send 2 requests from each IPv6 address
			for i := 0; i < 2; i++ {
				recorder = httptest.NewRecorder()
				req1 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req1.RemoteAddr = ipv6Addr1
				handler.ServeHTTP(recorder, req1)
				Expect(recorder.Code).To(Equal(http.StatusOK))

				recorder = httptest.NewRecorder()
				req2 := httptest.NewRequest("POST", "/webhook/prometheus", nil)
				req2.RemoteAddr = ipv6Addr2
				handler.ServeHTTP(recorder, req2)
				Expect(recorder.Code).To(Equal(http.StatusOK))
			}

			// Assert: Both IPv6 addresses should have independent limits
		})
	})
})
