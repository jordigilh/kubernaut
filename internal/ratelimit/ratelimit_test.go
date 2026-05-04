package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
)

func TestRateLimitSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RateLimit Suite")
}

var _ = Describe("Rate limiting", func() {
	Describe("per-IP", func() {
		It("UT-AF-009-001 allows requests under the burst limit", func() {
			cfg := ratelimit.PerIPConfig{
				RequestsPerSecond: 10,
				Burst:             20,
				CleanupInterval:   time.Minute,
				MaxAge:            5 * time.Minute,
			}
			limiter := ratelimit.NewIPLimiter(cfg)
			defer limiter.Stop()

			for i := 0; i < 15; i++ {
				Expect(limiter.Allow("192.168.1.1")).To(BeTrue(), "request %d should be allowed", i)
			}
		})

		It("UT-AF-009-002 returns 429 with Retry-After when over limit", func() {
			cfg := ratelimit.PerIPConfig{
				RequestsPerSecond: 1,
				Burst:             2,
				CleanupInterval:   time.Minute,
				MaxAge:            5 * time.Minute,
			}
			limiter := ratelimit.NewIPLimiter(cfg)
			defer limiter.Stop()

			handler := ratelimit.PreAuthMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for i := 0; i < 2; i++ {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "192.168.1.1:1234"
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusOK))
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "192.168.1.1:1234"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusTooManyRequests))
			Expect(rec.Header().Get("Retry-After")).NotTo(BeEmpty(), "Retry-After header must be present")
		})

		It("UT-AF-009-003 isolates rate limit buckets per IP", func() {
			cfg := ratelimit.PerIPConfig{
				RequestsPerSecond: 1,
				Burst:             2,
				CleanupInterval:   time.Minute,
				MaxAge:            5 * time.Minute,
			}
			limiter := ratelimit.NewIPLimiter(cfg)
			defer limiter.Stop()

			for i := 0; i < 3; i++ {
				limiter.Allow("10.0.0.1")
			}
			Expect(limiter.Allow("10.0.0.1")).To(BeFalse(), "IP1 should be rate limited")

			Expect(limiter.Allow("10.0.0.2")).To(BeTrue(), "IP2 should not be affected by IP1's limit")
		})
	})

	Describe("per-user", func() {
		It("UT-AF-009-004 allows requests under the per-user limit", func() {
			cfg := ratelimit.PerUserConfig{
				RequestsPerMinute:    60,
				MaxConcurrentSessions: 3,
				ToolCallsPerMinute:   60,
			}
			limiter := ratelimit.NewUserLimiter(cfg)

			for i := 0; i < 10; i++ {
				Expect(limiter.AllowRequest("alice")).To(BeTrue(), "request %d should be allowed", i)
			}
		})

		It("UT-AF-009-005 returns 429 when over per-user limit", func() {
			cfg := ratelimit.PerUserConfig{
				RequestsPerMinute:    5,
				MaxConcurrentSessions: 3,
				ToolCallsPerMinute:   60,
			}
			limiter := ratelimit.NewUserLimiter(cfg)

			handler := ratelimit.PostAuthMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for i := 0; i < 5; i++ {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"})
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusOK))
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"})
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusTooManyRequests))
		})

		It("UT-AF-009-006 enforces concurrent session limit", func() {
			cfg := ratelimit.PerUserConfig{
				RequestsPerMinute:    60,
				MaxConcurrentSessions: 3,
				ToolCallsPerMinute:   60,
			}
			limiter := ratelimit.NewUserLimiter(cfg)

			for i := 0; i < 3; i++ {
				Expect(limiter.AcquireSession("alice")).To(BeTrue(), "session %d should be granted", i)
			}

			Expect(limiter.AcquireSession("alice")).To(BeFalse(), "4th session should be denied")

			limiter.ReleaseSession("alice")
			Expect(limiter.AcquireSession("alice")).To(BeTrue(), "session should be granted after release")
		})

		It("UT-AF-009-007 enforces tool calls per minute", func() {
			cfg := ratelimit.PerUserConfig{
				RequestsPerMinute:    60,
				MaxConcurrentSessions: 3,
				ToolCallsPerMinute:   5,
			}
			limiter := ratelimit.NewUserLimiter(cfg)

			for i := 0; i < 5; i++ {
				Expect(limiter.AllowToolCall("alice")).To(BeTrue(), "tool call %d should be allowed", i)
			}

			Expect(limiter.AllowToolCall("alice")).To(BeFalse(), "6th tool call should be denied")
		})
	})

	Describe("per-provider", func() {
		It("UT-AF-009-008 rate-limits JWKS fetches per provider", func() {
			cfg := ratelimit.PerProviderConfig{
				FetchIntervalSeconds: 1,
			}
			limiter := ratelimit.NewProviderLimiter(cfg)

			Expect(limiter.AllowFetch("https://sso.example.com")).To(BeTrue())

			Expect(limiter.AllowFetch("https://sso.example.com")).To(BeFalse(), "rapid second fetch should be rate limited")

			Expect(limiter.AllowFetch("https://other-sso.example.com")).To(BeTrue())
		})
	})

	Describe("global LLM concurrency", func() {
		It("UT-AF-009-009 limits concurrent LLM work with a semaphore", func() {
			sem := ratelimit.NewLLMSemaphore(2)

			Expect(sem.Acquire()).To(BeTrue())
			Expect(sem.Acquire()).To(BeTrue())

			Expect(sem.Acquire()).To(BeFalse(), "should not exceed max concurrency")

			sem.Release()
			Expect(sem.Acquire()).To(BeTrue(), "should succeed after release")
		})
	})

	Describe("token budget", func() {
		It("UT-AF-009-010 is disabled when token budget is off but LLM semaphore still works", func() {
			cfg := ratelimit.Config{
				Global: ratelimit.GlobalConfig{
					MaxLLMConcurrency:  10,
					TokenBudgetEnabled: false,
				},
			}

			Expect(cfg.Global.TokenBudgetEnabled).To(BeFalse())

			sem := ratelimit.NewLLMSemaphore(cfg.Global.MaxLLMConcurrency)
			Expect(sem.Acquire()).To(BeTrue())
			sem.Release()
		})
	})

	Describe("configurable limits", func() {
		It("UT-AF-009-011 applies burst and limits from config", func() {
			cfg := ratelimit.Config{
				PerIP: ratelimit.PerIPConfig{
					RequestsPerSecond: 25,
					Burst:             50,
				},
				PerUser: ratelimit.PerUserConfig{
					RequestsPerMinute: 100,
				},
			}

			ipLimiter := ratelimit.NewIPLimiter(cfg.PerIP)
			defer ipLimiter.Stop()

			for i := 0; i < 50; i++ {
				Expect(ipLimiter.Allow("10.0.0.1")).To(BeTrue(), "request %d should pass with burst=50", i)
			}
			Expect(ipLimiter.Allow("10.0.0.1")).To(BeFalse(), "should be limited after burst exhausted")
		})
	})

	Describe("middleware", func() {
		It("UT-AF-009-012 uses the IP tier before authentication", func() {
			cfg := ratelimit.PerIPConfig{
				RequestsPerSecond: 1,
				Burst:             1,
				CleanupInterval:   time.Minute,
				MaxAge:            5 * time.Minute,
			}
			limiter := ratelimit.NewIPLimiter(cfg)
			defer limiter.Stop()

			var callCount atomic.Int32
			handler := ratelimit.PreAuthMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				w.WriteHeader(http.StatusOK)
			}))

			for range 3 {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = "10.0.0.1:5000"
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
			}

			Expect(callCount.Load()).To(Equal(int32(1)), "only 1 request should reach handler")
		})

		It("UT-AF-009-013 uses the user tier after authentication", func() {
			cfg := ratelimit.PerUserConfig{
				RequestsPerMinute:    2,
				MaxConcurrentSessions: 10,
				ToolCallsPerMinute:   60,
			}
			limiter := ratelimit.NewUserLimiter(cfg)

			var callCount atomic.Int32
			handler := ratelimit.PostAuthMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				w.WriteHeader(http.StatusOK)
			}))

			for range 4 {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "bob"})
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
			}

			Expect(callCount.Load()).To(Equal(int32(2)), "only 2 requests should reach handler for bob")
		})
	})
})
