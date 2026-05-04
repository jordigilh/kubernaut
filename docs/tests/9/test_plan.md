# Test Plan: Rate Limiting for External Clients

**Test Plan Identifier:** TP-AF-009
**Issue:** [#9](https://github.com/jordigilh/kubernaut-apifrontend/issues/9)
**Version:** 1.0
**Date:** 2026-05-03
**Status:** Draft

---

## 1. Introduction

This test plan validates the 3-tier rate limiting subsystem of the kubernaut API Frontend (AF). Rate limiting is the first line of defense against abuse, protecting downstream systems (KA, LLM providers, Keycloak, K8s API) from resource exhaustion.

### 1.1 Scope

- Per-IP token bucket rate limiting (pre-authentication)
- Per-user rate limiting (post-authentication): request rate, concurrent sessions, tool calls/min
- Per-provider JWKS fetch rate limiting
- Global LLM concurrency semaphore (Tier 2)
- Per-user token budget (Tier 3, disabled when unavailable)
- HTTP 429 responses with `Retry-After` header (RFC 6585)
- Configurable limits via structured config

### 1.2 References

- Issue #9: Rate limiting for external clients (per-user, per-IP)
- ARCHITECTURE.md Section 6: Rate Limiting (3-tier table), Circuit Breakers
- KA reference: `internal/kubernautagent/server/ratelimit.go`
- RFC 6585: Additional HTTP Status Codes (429 Too Many Requests)

---

## 2. Test Items

| Item | Package | Version |
|------|---------|---------|
| `IPRateLimiter` | `internal/ratelimit` | PR2 |
| `UserRateLimiter` | `internal/ratelimit` | PR2 |
| `ProviderRateLimiter` | `internal/ratelimit` | PR2 |
| `LLMSemaphore` | `internal/ratelimit` | PR2 |
| `TokenBudget` | `internal/ratelimit` | PR2 |
| `RateLimitMiddleware` | `internal/ratelimit` | PR2 |
| `RateLimitConfig` | `internal/ratelimit` | PR2 |

---

## 3. Features to Be Tested

### 3.1 Per-IP Rate Limiting (Tier: Pre-Auth)

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-9.1 | Under-limit allows | Requests below `requestsPerSecond` + `burst` pass through | Issue #9 |
| F-9.2 | Over-limit rejects with 429 | Burst exceeded returns HTTP 429 | Issue #9 |
| F-9.3 | Retry-After header | 429 response includes `Retry-After` header (RFC 6585) | Issue #9 |
| F-9.4 | Independent IP buckets | Different source IPs have separate token buckets | Issue #9 |
| F-9.5 | XFF parsing | `X-Forwarded-For` header used for client IP extraction (leftmost) | KA ref |

### 3.2 Per-User Rate Limiting (Tier: Post-Auth)

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-9.6 | User request rate | Authenticated users limited to configured req/min | Issue #9, ARCH §6 (30 req/min) |
| F-9.7 | Concurrent session limit | Max concurrent MCP sessions per user enforced (e.g., 3) | Issue #9 |
| F-9.8 | Tool calls per minute | Per-user tool call rate limited (e.g., 60/min) | Issue #9 |
| F-9.9 | User identity from context | Limiter reads `UserIdentity` from `context.Context` (set by JWT middleware) | ARCH §6 |

### 3.3 Per-Provider Rate Limiting

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-9.10 | JWKS fetch rate | JWKS fetches limited per provider (e.g., max 1/5min) | Issue #9 |
| F-9.11 | Unknown issuer amplification | Rapid unknown-issuer tokens do not amplify JWKS fetches | Issue #9 |

### 3.4 Global LLM Concurrency (ARCHITECTURE.md Tier 2)

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-9.12 | Semaphore enforcement | Max 10 concurrent LLM calls enforced globally | ARCH §6 |
| F-9.13 | Queuing behavior | Requests beyond semaphore capacity wait or reject (configurable) | ARCH §6 |

### 3.5 Token Budget (ARCHITECTURE.md Tier 3)

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-9.14 | Budget enforcement | Per-user token budget enforced when LLM reports usage | ARCH §6 |
| F-9.15 | Disabled when unavailable | Tier 3 disabled when LLM provider does not report token usage | ARCH §6 |

### 3.6 Configuration

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-9.16 | Configurable values | All limits read from `RateLimitConfig` struct | Issue #9 |
| F-9.17 | Middleware position | Pre-auth middleware uses IP tier; post-auth uses user tier | Issue #9 |

---

## 4. Features Not Tested

- Distributed rate limiting (single-instance only in v1; clustered deferred)
- Prometheus metrics for rate limit events (tested in observability suite)
- Integration with specific LLM provider token reporting APIs

---

## 5. Approach

### 5.1 Test Methodology

Test-Driven Development (TDD):
1. **Red phase:** Write all test cases as failing tests
2. **Green phase:** Implement minimal code using `golang.org/x/time/rate`
3. **Refactor phase:** Clean up against 100-go-mistakes; add eviction; table-driven tests

### 5.2 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Unit | Individual limiters (IP, User, Provider, Semaphore) | `testing`, `time` manipulation |
| Integration | Middleware chain with `httptest` | `httptest`, `net/http` |
| Concurrency | Race condition and goroutine leak detection | `go test -race`, `goleak` |

### 5.3 Test Infrastructure

- **Clock control:** Inject `time.Now` or use short intervals for deterministic testing
- **Concurrent load:** `sync.WaitGroup` + goroutines to simulate burst traffic
- **Context injection:** Pre-set `UserIdentity` in context for post-auth tier tests
- **Reference:** KA's `ratelimit.go` pattern (per-IP with background eviction, `sync.Mutex`, `stopCh`)

---

## 6. Pass/Fail Criteria

### 6.1 Pass Criteria

- All 13 test cases pass
- Code coverage >= 80% of `internal/ratelimit/*.go`
- Zero `golangci-lint` errors
- Zero race conditions (`go test -race`)
- No goroutine leaks (background cleanup goroutines properly stopped)
- `Retry-After` header present on all 429 responses

### 6.2 Fail Criteria

- Any test case fails
- Coverage drops below 80%
- Goroutine leak detected (background eviction not stopped)
- Unbounded memory growth in per-IP/per-user maps (missing eviction)

---

## 7. Suspension and Resumption Criteria

### 7.1 Suspension

- `golang.org/x/time/rate` API breaking change
- Fundamental design conflict between pre-auth and post-auth middleware ordering

### 7.2 Resumption

- Dependency stabilized
- Design resolved with architecture team

---

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Test source code | `internal/ratelimit/ratelimit_test.go` |
| Coverage report | Generated by `go test -coverprofile=coverage.out` |
| Memory profile | `go test -memprofile` (Checkpoint D) |
| This test plan | `docs/tests/9/test_plan.md` |

---

## 9. Environmental Needs

| Requirement | Details |
|-------------|---------|
| Go version | 1.25.x |
| Dependencies | `golang.org/x/time/rate` |
| External services | None (all in-process) |
| CI environment | GitHub Actions (linux/amd64) |

---

## 10. Responsibilities

| Role | Responsibility |
|------|---------------|
| Developer | Write tests, implement code, verify eviction and cleanup |
| CI | Automated test execution, race detection, coverage |
| Reviewer | Verify concurrency safety, eviction correctness, config completeness |

---

## 11. Schedule

| Phase | Duration | Gate |
|-------|----------|------|
| Red (Phase 4) | Tests written, all fail | Compilation succeeds, all tests fail |
| Green (Phase 5) | All tests pass | Checkpoint C passes |
| Refactor (Phase 6) | Code cleaned up | Checkpoint D passes |

---

## 12. Test Case Matrix

| Test Case | Features Covered | Priority |
|-----------|-----------------|----------|
| `TestPerIP_UnderLimit_Allows` | F-9.1 | P0 |
| `TestPerIP_OverLimit_Returns429WithRetryAfter` | F-9.2, F-9.3 | P0 |
| `TestPerIP_DifferentIPs_IndependentBuckets` | F-9.4 | P0 |
| `TestPerUser_UnderLimit_Allows` | F-9.6 | P0 |
| `TestPerUser_OverLimit_Returns429` | F-9.6 | P0 |
| `TestPerUser_ConcurrentSessionLimit` | F-9.7 | P0 |
| `TestPerUser_ToolCallsPerMinute` | F-9.8 | P1 |
| `TestPerProvider_JWKSFetchRateLimit` | F-9.10, F-9.11 | P0 |
| `TestGlobalLLMConcurrency_Semaphore` | F-9.12, F-9.13 | P0 |
| `TestTokenBudget_DisabledWhenUnavailable` | F-9.14, F-9.15 | P1 |
| `TestConfigurable_ValuesFromConfig` | F-9.16 | P1 |
| `TestMiddleware_PreAuth_UsesIPTier` | F-9.17 | P0 |
| `TestMiddleware_PostAuth_UsesUserTier` | F-9.9, F-9.17 | P0 |
