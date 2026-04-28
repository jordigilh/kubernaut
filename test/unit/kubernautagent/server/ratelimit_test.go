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

package server_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
)

var _ = Describe("Rate Limiter — #823 Hardening", func() {

	Describe("UT-KA-823-RL01: Requests within burst are allowed", func() {
		It("allows burst-count requests through", func() {
			cfg := kaserver.RateLimitConfig{
				RequestsPerSecond: 100,
				Burst:             5,
				CleanupInterval:   time.Hour,
				MaxAge:            time.Hour,
			}
			rl := kaserver.NewRateLimiter(cfg, nil)
			defer rl.Stop()

			handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for i := 0; i < 5; i++ {
				req := httptest.NewRequest("GET", "/api/v1/incident/analyze", nil)
				req.RemoteAddr = "10.0.0.1:12345"
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusOK), "request %d should be allowed", i+1)
			}
		})
	})

	Describe("UT-KA-823-RL02: Requests exceeding burst are rejected with 429", func() {
		It("returns 429 after burst exhausted", func() {
			cfg := kaserver.RateLimitConfig{
				RequestsPerSecond: 1,
				Burst:             2,
				CleanupInterval:   time.Hour,
				MaxAge:            time.Hour,
			}
			rl := kaserver.NewRateLimiter(cfg, nil)
			defer rl.Stop()

			handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for i := 0; i < 2; i++ {
				req := httptest.NewRequest("GET", "/stream", nil)
				req.RemoteAddr = "10.0.0.1:12345"
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusOK))
			}

			req := httptest.NewRequest("GET", "/stream", nil)
			req.RemoteAddr = "10.0.0.1:12345"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusTooManyRequests))
		})
	})

	Describe("UT-KA-823-RL03: Different IPs have independent limits", func() {
		It("tracks limiters per IP", func() {
			cfg := kaserver.RateLimitConfig{
				RequestsPerSecond: 1,
				Burst:             1,
				CleanupInterval:   time.Hour,
				MaxAge:            time.Hour,
			}
			rl := kaserver.NewRateLimiter(cfg, nil)
			defer rl.Stop()

			handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req1 := httptest.NewRequest("GET", "/", nil)
			req1.RemoteAddr = "10.0.0.1:12345"
			rec1 := httptest.NewRecorder()
			handler.ServeHTTP(rec1, req1)
			Expect(rec1.Code).To(Equal(http.StatusOK))

			req2 := httptest.NewRequest("GET", "/", nil)
			req2.RemoteAddr = "10.0.0.2:12345"
			rec2 := httptest.NewRecorder()
			handler.ServeHTTP(rec2, req2)
			Expect(rec2.Code).To(Equal(http.StatusOK))

			req3 := httptest.NewRequest("GET", "/", nil)
			req3.RemoteAddr = "10.0.0.1:12345"
			rec3 := httptest.NewRecorder()
			handler.ServeHTTP(rec3, req3)
			Expect(rec3.Code).To(Equal(http.StatusTooManyRequests), "second request from same IP should be rate limited")
		})
	})

	Describe("UT-KA-823-RL04: X-Forwarded-For is used for IP extraction", func() {
		It("limits based on X-Forwarded-For when present", func() {
			cfg := kaserver.RateLimitConfig{
				RequestsPerSecond: 1,
				Burst:             1,
				CleanupInterval:   time.Hour,
				MaxAge:            time.Hour,
			}
			rl := kaserver.NewRateLimiter(cfg, nil)
			defer rl.Stop()

			handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req1 := httptest.NewRequest("GET", "/", nil)
			req1.RemoteAddr = "10.0.0.1:12345"
			req1.Header.Set("X-Forwarded-For", "192.168.1.1")
			rec1 := httptest.NewRecorder()
			handler.ServeHTTP(rec1, req1)
			Expect(rec1.Code).To(Equal(http.StatusOK))

			req2 := httptest.NewRequest("GET", "/", nil)
			req2.RemoteAddr = "10.0.0.1:12345"
			req2.Header.Set("X-Forwarded-For", "192.168.1.1")
			rec2 := httptest.NewRecorder()
			handler.ServeHTTP(rec2, req2)
			Expect(rec2.Code).To(Equal(http.StatusTooManyRequests))
		})
	})
})
