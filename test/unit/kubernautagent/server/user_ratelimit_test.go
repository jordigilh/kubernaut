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
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("UserRateLimiter — SEC-02", func() {

	var (
		rl      *kaserver.UserRateLimiter
		handler http.Handler
	)

	BeforeEach(func() {
		cfg := kaserver.UserRateLimitConfig{
			RequestsPerSecond: 2,
			Burst:             2,
			CleanupInterval:   5 * 60 * 1e9,
			MaxAge:            10 * 60 * 1e9,
		}
		rl = kaserver.NewUserRateLimiter(cfg, nil)

		handler = rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	})

	AfterEach(func() {
		rl.Stop()
	})

	Describe("UT-KA-SEC02-001: rejects unauthenticated requests with 401", func() {
		It("should return 401 when no user in context", func() {
			req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("UT-KA-SEC02-002: allows requests within rate limit", func() {
		It("should return 200 for authenticated requests within burst", func() {
			req := authedRequest("alice@example.com")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("UT-KA-SEC02-003: rejects requests exceeding per-user limit", func() {
		It("should return 429 after burst is exhausted", func() {
			for i := 0; i < 2; i++ {
				req := authedRequest("bob@example.com")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusOK))
			}

			req := authedRequest("bob@example.com")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusTooManyRequests))
			Expect(rec.Header().Get("Retry-After")).To(Equal("1"))
		})
	})

	Describe("UT-KA-SEC02-004: rate limits are per-user (isolation)", func() {
		It("should not affect one user when another is rate limited", func() {
			for i := 0; i < 3; i++ {
				req := authedRequest("heavy-user@example.com")
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
			}

			req := authedRequest("fresh-user@example.com")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})
})

func authedRequest(username string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	ctx := context.WithValue(req.Context(), sharedauth.UserContextKey, username)
	return req.WithContext(ctx)
}
