package integration_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
)

var _ = Describe("Rate Limiting — Handler-Level (IT-RL)", func() {

	Describe("Pre-auth IP rate limiting", func() {

		It("IT-RL-001: burst exceeding IP limit returns HTTP 429", func() {
			limiter := ratelimit.NewIPLimiter(ratelimit.PerIPConfig{
				RequestsPerSecond: 2,
				Burst:             3,
			})
			defer limiter.Stop()

			mw := ratelimit.PreAuthMiddlewareWithConfig(ratelimit.PreAuthMiddlewareConfig{
				Limiter: limiter,
			})
			ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			h := mw(ok)

			var saw429 bool
			for i := 0; i < 20; i++ {
				req := httptest.NewRequest("POST", "/a2a/invoke", http.NoBody)
				req.RemoteAddr = "10.0.0.1:12345"
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code == http.StatusTooManyRequests {
					saw429 = true
					Expect(rec.Header().Get("Retry-After")).NotTo(BeEmpty())
					break
				}
			}
			Expect(saw429).To(BeTrue(), "expected HTTP 429 after exceeding per-IP burst")
		})
	})

	Describe("Post-auth user request rate limiting", func() {

		It("IT-RL-002: authenticated user burst returns HTTP 429", func() {
			limiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
				RequestsPerMinute: 3,
			})
			defer limiter.Stop()

			mw := ratelimit.PostAuthMiddlewareWithConfig(ratelimit.PostAuthMiddlewareConfig{
				Limiter: limiter,
			})
			ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			h := mw(ok)

			user := &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}}
			var saw429 bool
			for i := 0; i < 20; i++ {
				req := httptest.NewRequest("POST", "/mcp", http.NoBody)
				ctx := auth.WithUserIdentity(req.Context(), user)
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code == http.StatusTooManyRequests {
					saw429 = true
					break
				}
			}
			Expect(saw429).To(BeTrue(), "expected HTTP 429 after exceeding per-user request rate")
		})

		It("IT-RL-003: different users have independent request budgets", func() {
			limiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
				RequestsPerMinute: 3,
			})
			defer limiter.Stop()

			mw := ratelimit.PostAuthMiddlewareWithConfig(ratelimit.PostAuthMiddlewareConfig{
				Limiter: limiter,
			})
			ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			h := mw(ok)

			sre := &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}}
			cicd := &auth.UserIdentity{Username: "cicd@kubernaut.ai", Groups: []string{"cicd"}}

			// Exhaust SRE budget
			var sreBlocked bool
			for i := 0; i < 20; i++ {
				req := httptest.NewRequest("POST", "/mcp", http.NoBody)
				ctx := auth.WithUserIdentity(req.Context(), sre)
				req = req.WithContext(ctx)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code == http.StatusTooManyRequests {
					sreBlocked = true
					break
				}
			}
			Expect(sreBlocked).To(BeTrue(), "SRE budget should be exhausted")

			// CICD should still pass
			req := httptest.NewRequest("POST", "/mcp", http.NoBody)
			ctx := auth.WithUserIdentity(req.Context(), cicd)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK),
				"CICD user should not inherit SRE's exhausted budget")
		})
	})

	Describe("Per-user tool call rate limiting", func() {

		It("IT-RL-004: rapid tool calls hit per-user tool rate limit", func() {
			limiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
				RequestsPerMinute:  600,
				ToolCallsPerMinute: 3,
			})
			defer limiter.Stop()

			var limited bool
			for i := 0; i < 20; i++ {
				if !limiter.AllowToolCall("sre@kubernaut.ai") {
					limited = true
					break
				}
			}
			Expect(limited).To(BeTrue(), "expected tool call rate limit after rapid calls")
		})

		It("IT-RL-005: tool rate limit is per-user isolated", func() {
			limiter := ratelimit.NewUserLimiter(ratelimit.PerUserConfig{
				RequestsPerMinute:  600,
				ToolCallsPerMinute: 3,
			})
			defer limiter.Stop()

			// Exhaust SRE tool budget
			for i := 0; i < 20; i++ {
				if !limiter.AllowToolCall("sre@kubernaut.ai") {
					break
				}
			}

			// Observability user should still be allowed
			Expect(limiter.AllowToolCall("obs@kubernaut.ai")).To(BeTrue(),
				"other user's tool budget should be independent")
		})
	})
})
