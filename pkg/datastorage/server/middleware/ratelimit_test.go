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

package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// BR-STORAGE-1505 (GAP-09, Issue #1505): per-IP rate limiting for the Data
// Storage HTTP API (SC-5).
var _ = Describe("IPLimiter and IPRateLimitMiddleware", func() {
	var logger = kubelog.NewLogger(kubelog.DefaultOptions())

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	Describe("IPLimiter", func() {
		It("UT-DS-1505-RL-001: allows requests within the burst, then denies", func() {
			l := middleware.NewIPLimiter(middleware.IPLimiterConfig{RequestsPerSecond: 1, Burst: 2})
			defer l.Stop()

			Expect(l.Allow("1.2.3.4")).To(BeTrue())
			Expect(l.Allow("1.2.3.4")).To(BeTrue())
			Expect(l.Allow("1.2.3.4")).To(BeFalse())
		})

		It("UT-DS-1505-RL-002: tracks separate buckets per IP", func() {
			l := middleware.NewIPLimiter(middleware.IPLimiterConfig{RequestsPerSecond: 1, Burst: 1})
			defer l.Stop()

			Expect(l.Allow("1.1.1.1")).To(BeTrue())
			Expect(l.Allow("1.1.1.1")).To(BeFalse())
			// A different IP has its own independent bucket.
			Expect(l.Allow("2.2.2.2")).To(BeTrue())
		})

		It("UT-DS-1505-RL-003: Stop is safe to call multiple times", func() {
			l := middleware.NewIPLimiter(middleware.IPLimiterConfig{RequestsPerSecond: 1, Burst: 1})
			Expect(func() {
				l.Stop()
				l.Stop()
			}).NotTo(Panic())
		})
	})

	Describe("IPRateLimitMiddleware", func() {
		It("UT-DS-1505-RL-004: passes requests through while under the limit", func() {
			limiter := middleware.NewIPLimiter(middleware.IPLimiterConfig{RequestsPerSecond: 10, Burst: 10})
			defer limiter.Stop()

			mw := middleware.IPRateLimitMiddleware(middleware.IPRateLimitMiddlewareConfig{
				Limiter: limiter,
				Logger:  logger,
			})
			handler := mw(okHandler)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
			req.RemoteAddr = "203.0.113.1:54321"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
		})

		It("UT-DS-1505-RL-005: returns RFC 7807 429 with Retry-After when the limit is exceeded", func() {
			limiter := middleware.NewIPLimiter(middleware.IPLimiterConfig{RequestsPerSecond: 1, Burst: 1})
			defer limiter.Stop()

			mw := middleware.IPRateLimitMiddleware(middleware.IPRateLimitMiddlewareConfig{
				Limiter: limiter,
				Logger:  logger,
			})
			handler := mw(okHandler)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
			req.RemoteAddr = "203.0.113.2:54321"

			rec1 := httptest.NewRecorder()
			handler.ServeHTTP(rec1, req)
			Expect(rec1.Code).To(Equal(http.StatusOK))

			rec2 := httptest.NewRecorder()
			handler.ServeHTTP(rec2, req)
			Expect(rec2.Code).To(Equal(http.StatusTooManyRequests))
			Expect(rec2.Header().Get("Retry-After")).NotTo(BeEmpty())
			Expect(rec2.Header().Get("Content-Type")).To(Equal("application/problem+json"))

			var problem map[string]interface{}
			Expect(json.Unmarshal(rec2.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["status"]).To(BeNumerically("==", http.StatusTooManyRequests))
			Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/rate-limit-exceeded"))
		})

		It("UT-DS-1505-RL-006: invokes AuditFunc exactly once on denial, without blocking the response", func() {
			limiter := middleware.NewIPLimiter(middleware.IPLimiterConfig{RequestsPerSecond: 1, Burst: 1})
			defer limiter.Stop()

			var mu sync.Mutex
			var calls []string
			mw := middleware.IPRateLimitMiddleware(middleware.IPRateLimitMiddlewareConfig{
				Limiter: limiter,
				Logger:  logger,
				AuditFunc: func(_ context.Context, sourceIP, path, method string) {
					mu.Lock()
					defer mu.Unlock()
					calls = append(calls, sourceIP+" "+method+" "+path)
				},
			})
			handler := mw(okHandler)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", nil)
			req.RemoteAddr = "203.0.113.3:1111"

			handler.ServeHTTP(httptest.NewRecorder(), req) // consumes the single token
			handler.ServeHTTP(httptest.NewRecorder(), req) // denied -> audits

			mu.Lock()
			defer mu.Unlock()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0]).To(Equal("203.0.113.3 POST /api/v1/audit/events"))
		})

		It("UT-DS-1505-RL-007: does not invoke AuditFunc when nil", func() {
			limiter := middleware.NewIPLimiter(middleware.IPLimiterConfig{RequestsPerSecond: 1, Burst: 1})
			defer limiter.Stop()

			mw := middleware.IPRateLimitMiddleware(middleware.IPRateLimitMiddlewareConfig{
				Limiter: limiter,
				Logger:  logger,
			})
			handler := mw(okHandler)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
			req.RemoteAddr = "203.0.113.4:2222"

			handler.ServeHTTP(httptest.NewRecorder(), req)
			rec := httptest.NewRecorder()
			Expect(func() { handler.ServeHTTP(rec, req) }).NotTo(Panic())
			Expect(rec.Code).To(Equal(http.StatusTooManyRequests))
		})
	})
})
