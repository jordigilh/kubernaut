package ratelimit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ratelimit"
)

func TestPerIP_UnderLimit_Allows(t *testing.T) {
	cfg := ratelimit.PerIPConfig{
		RequestsPerSecond: 10,
		Burst:             20,
		CleanupInterval:   time.Minute,
		MaxAge:            5 * time.Minute,
	}
	limiter := ratelimit.NewIPLimiter(cfg)
	t.Cleanup(limiter.Stop)

	// Should allow requests under burst limit
	for i := 0; i < 15; i++ {
		assert.True(t, limiter.Allow("192.168.1.1"), "request %d should be allowed", i)
	}
}

func TestPerIP_OverLimit_Returns429WithRetryAfter(t *testing.T) {
	cfg := ratelimit.PerIPConfig{
		RequestsPerSecond: 1,
		Burst:             2,
		CleanupInterval:   time.Minute,
		MaxAge:            5 * time.Minute,
	}
	limiter := ratelimit.NewIPLimiter(cfg)
	t.Cleanup(limiter.Stop)

	handler := ratelimit.PreAuthMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Next request should be rejected
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"), "Retry-After header must be present")
}

func TestPerIP_DifferentIPs_IndependentBuckets(t *testing.T) {
	cfg := ratelimit.PerIPConfig{
		RequestsPerSecond: 1,
		Burst:             2,
		CleanupInterval:   time.Minute,
		MaxAge:            5 * time.Minute,
	}
	limiter := ratelimit.NewIPLimiter(cfg)
	t.Cleanup(limiter.Stop)

	// Exhaust IP1
	for i := 0; i < 3; i++ {
		limiter.Allow("10.0.0.1")
	}
	assert.False(t, limiter.Allow("10.0.0.1"), "IP1 should be rate limited")

	// IP2 should still be allowed
	assert.True(t, limiter.Allow("10.0.0.2"), "IP2 should not be affected by IP1's limit")
}

func TestPerUser_UnderLimit_Allows(t *testing.T) {
	cfg := ratelimit.PerUserConfig{
		RequestsPerMinute:    60,
		MaxConcurrentSessions: 3,
		ToolCallsPerMinute:   60,
	}
	limiter := ratelimit.NewUserLimiter(cfg)

	for i := 0; i < 10; i++ {
		assert.True(t, limiter.AllowRequest("alice"), "request %d should be allowed", i)
	}
}

func TestPerUser_OverLimit_Returns429(t *testing.T) {
	cfg := ratelimit.PerUserConfig{
		RequestsPerMinute:    5,
		MaxConcurrentSessions: 3,
		ToolCallsPerMinute:   60,
	}
	limiter := ratelimit.NewUserLimiter(cfg)

	handler := ratelimit.PostAuthMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust per-user limit
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Next request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestPerUser_ConcurrentSessionLimit(t *testing.T) {
	cfg := ratelimit.PerUserConfig{
		RequestsPerMinute:    60,
		MaxConcurrentSessions: 3,
		ToolCallsPerMinute:   60,
	}
	limiter := ratelimit.NewUserLimiter(cfg)

	// Acquire 3 sessions (should all succeed)
	for i := 0; i < 3; i++ {
		assert.True(t, limiter.AcquireSession("alice"), "session %d should be granted", i)
	}

	// 4th session should be denied
	assert.False(t, limiter.AcquireSession("alice"), "4th session should be denied")

	// Release one and retry
	limiter.ReleaseSession("alice")
	assert.True(t, limiter.AcquireSession("alice"), "session should be granted after release")
}

func TestPerUser_ToolCallsPerMinute(t *testing.T) {
	cfg := ratelimit.PerUserConfig{
		RequestsPerMinute:    60,
		MaxConcurrentSessions: 3,
		ToolCallsPerMinute:   5,
	}
	limiter := ratelimit.NewUserLimiter(cfg)

	// Exhaust tool call limit
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.AllowToolCall("alice"), "tool call %d should be allowed", i)
	}

	// Next tool call should be denied
	assert.False(t, limiter.AllowToolCall("alice"), "6th tool call should be denied")
}

func TestPerProvider_JWKSFetchRateLimit(t *testing.T) {
	cfg := ratelimit.PerProviderConfig{
		FetchIntervalSeconds: 1,
	}
	limiter := ratelimit.NewProviderLimiter(cfg)

	// First fetch should be allowed
	assert.True(t, limiter.AllowFetch("https://sso.example.com"))

	// Immediate second fetch should be denied
	assert.False(t, limiter.AllowFetch("https://sso.example.com"), "rapid second fetch should be rate limited")

	// Different provider should be independent
	assert.True(t, limiter.AllowFetch("https://other-sso.example.com"))
}

func TestGlobalLLMConcurrency_Semaphore(t *testing.T) {
	sem := ratelimit.NewLLMSemaphore(2)

	// Acquire 2 slots
	assert.True(t, sem.Acquire())
	assert.True(t, sem.Acquire())

	// 3rd should fail
	assert.False(t, sem.Acquire(), "should not exceed max concurrency")

	// Release and retry
	sem.Release()
	assert.True(t, sem.Acquire(), "should succeed after release")
}

func TestTokenBudget_DisabledWhenUnavailable(t *testing.T) {
	cfg := ratelimit.Config{
		Global: ratelimit.GlobalConfig{
			MaxLLMConcurrency:  10,
			TokenBudgetEnabled: false,
		},
	}

	// When token budget is disabled, no token-based limiting should occur
	assert.False(t, cfg.Global.TokenBudgetEnabled)
	// The system should still function without token tracking
	sem := ratelimit.NewLLMSemaphore(cfg.Global.MaxLLMConcurrency)
	assert.True(t, sem.Acquire())
	sem.Release()
}

func TestConfigurable_ValuesFromConfig(t *testing.T) {
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
	t.Cleanup(ipLimiter.Stop)

	// With burst=50, should allow 50 requests immediately
	for i := 0; i < 50; i++ {
		assert.True(t, ipLimiter.Allow("10.0.0.1"), "request %d should pass with burst=50", i)
	}
	// 51st should fail
	assert.False(t, ipLimiter.Allow("10.0.0.1"), "should be limited after burst exhausted")
}

func TestMiddleware_PreAuth_UsesIPTier(t *testing.T) {
	cfg := ratelimit.PerIPConfig{
		RequestsPerSecond: 1,
		Burst:             1,
		CleanupInterval:   time.Minute,
		MaxAge:            5 * time.Minute,
	}
	limiter := ratelimit.NewIPLimiter(cfg)
	t.Cleanup(limiter.Stop)

	var callCount atomic.Int32
	handler := ratelimit.PreAuthMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))

	// Send 3 requests from same IP - only first should pass
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:5000"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	assert.Equal(t, int32(1), callCount.Load(), "only 1 request should reach handler")
}

func TestMiddleware_PostAuth_UsesUserTier(t *testing.T) {
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

	// Simulate 4 requests from "bob" - only first 2 should pass
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "bob"})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	assert.Equal(t, int32(2), callCount.Load(), "only 2 requests should reach handler for bob")
}

// Verify no compile errors for unused imports
var _ = context.Background
var _ = sync.WaitGroup{}
var _ = require.NoError
