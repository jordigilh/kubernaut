# Test Plan — Cycle B: Security Hardening

| Field | Value |
|-------|-------|
| **Identifier** | `TP-CYCLE-B-001` |
| **Version** | 1.0 |
| **Status** | Draft |
| **Created** | 2026-05-13 |
| **Standard** | IEEE 829-2008 |

## 1. References

| Ref | Document |
|-----|----------|
| R-01 | GA Readiness Audit — findings SEC-01 through SEC-07 |
| R-02 | `internal/auth/jwks_cache.go` — JWKS fetch and caching |
| R-03 | `internal/auth/config.go` — auth configuration and validation |
| R-04 | `internal/auth/jwt.go` — JWT validation and `nbf` enforcement |
| R-05 | `internal/auth/replay_cache.go` — JTI replay detection |
| R-06 | `internal/auth/middleware.go` — auth middleware and error classification |
| R-07 | `internal/auth/tokenreview.go` — Kubernetes TokenReview identity |
| R-08 | `internal/auth/types.go` — `UserIdentity` struct |
| R-09 | Cycle A Test Plan `TP-CYCLE-A-001` — cross-phase integration dependency |

## 2. Introduction

This test plan covers **security hardening** of the authentication subsystem. The audit
identified 5 findings where the auth package accepts inputs that should be rejected or
lacks defensive bounds on external data.

**Objective:** Prove that the auth subsystem rejects malformed, oversized, and adversarial
inputs with clear error sentinels, and that all security-relevant state changes are
observable via metrics and structured logs.

**Approach:** TDD at unit level. Each finding gets a failing test first (Red), then minimal
code to pass (Green), then code quality review against 100-go-mistakes (Refactor).

## 3. Test Items

| Item | Version | Source |
|------|---------|--------|
| `internal/auth/jwks_cache.go` | HEAD | R-02 |
| `internal/auth/config.go` | HEAD | R-03 |
| `internal/auth/jwt.go` | HEAD | R-04 |
| `internal/auth/replay_cache.go` | HEAD | R-05 |
| `internal/auth/middleware.go` | HEAD | R-06 |
| `internal/auth/tokenreview.go` | HEAD | R-07 |

## 4. Software Risk Issues

| Risk | Impact | Mitigation |
|------|--------|------------|
| JWKS body limit could reject legitimate large key sets | Medium | 1 MB limit accommodates ~100 RSA keys; document limit |
| Issuer URL scheme enforcement could break non-TLS dev setups | Medium | Allow `http` only when `config.allowInsecureIssuer: true` |
| Sanitizing TokenReview fields could truncate legitimate long usernames | Low | 256 char limit exceeds any realistic username; log truncation |
| `ErrTokenReplayed` sentinel change could break error handling upstream | Medium | Ensure `errors.Is` chains are preserved |

## 5. Features to be Tested

### 5.1 SEC-01 — JWKS Response Body Size Limit

**Current behavior:** `fetchJWKS` reads the full HTTP response body with no size limit.
An attacker controlling the JWKS endpoint could send a multi-GB response causing OOM.

**Required behavior:** Response body limited to 1 MB via `http.MaxBytesReader`. Responses
exceeding this limit return a clear error.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-B-01a | JWKS server returns 512 KB valid JWKS | Fetch keys | Success; keys parsed correctly | Unit |
| TC-B-01b | JWKS server returns 2 MB body | Fetch keys | Error containing "exceeds" or "too large" | Unit |
| TC-B-01c | JWKS server returns exactly 1 MB | Fetch keys | Success (boundary: limit is exclusive) | Unit |
| TC-B-01d | JWKS server returns 1 MB + 1 byte | Fetch keys | Error | Unit |
| TC-B-01e | JWKS server returns empty body | Fetch keys | Error: empty or malformed JWKS | Unit |
| TC-B-01f | JWKS server returns valid JSON but not JWKS (e.g. `{"foo":"bar"}`) | Fetch keys | Error: no keys found or malformed | Unit |

**Adversarial inputs:**

| TC ID | Input | Expected Result |
|-------|-------|-----------------|
| TC-B-01g | Response body is 100 MB of null bytes | Error; no OOM; memory usage stays bounded |
| TC-B-01h | Response body is slow-drip (1 byte/sec for 10s) | Times out via existing HTTP client timeout; body limit still applies |

### 5.2 SEC-02 — Issuer URL Scheme Validation

**Current behavior:** `Config.Validate()` checks that `Issuer.URL` is non-empty and
parseable, but does not validate the scheme. `ftp://`, `file://`, or `javascript:` URLs
pass validation. When `JWKSURL` is empty, JWKS URL is derived from `Issuer.URL + "/.well-known/..."`,
potentially allowing HTTP (non-TLS) JWKS fetches.

**Required behavior:** `Issuer.URL` must have scheme `https` (or `http` only when
explicitly opted in via `AllowInsecureIssuer` config flag for dev/test).

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-B-02a | Default config | `issuerURL: "https://dex.example.com"` | Validation passes | Unit |
| TC-B-02b | Default config | `issuerURL: "http://dex.example.com"` | Validation error: "https required" | Unit |
| TC-B-02c | `allowInsecureIssuer: true` | `issuerURL: "http://dex.example.com"` | Validation passes | Unit |
| TC-B-02d | Default config | `issuerURL: "ftp://dex.example.com"` | Validation error: unsupported scheme | Unit |
| TC-B-02e | Default config | `issuerURL: "file:///etc/passwd"` | Validation error: unsupported scheme | Unit |
| TC-B-02f | Default config | `issuerURL: "javascript:alert(1)"` | Validation error: unsupported scheme | Unit |
| TC-B-02g | Default config | `issuerURL: ""` | Validation error: empty (existing test) | Unit |
| TC-B-02h | Default config | `issuerURL: "://missing-scheme"` | Validation error: malformed URL | Unit |

**Adversarial inputs:**

| TC ID | Input | Expected Result |
|-------|-------|-----------------|
| TC-B-02i | `issuerURL` with 4096+ characters | Validation error: URL too long |
| TC-B-02j | `issuerURL: "https://dex.example.com\x00evil"` | Validation error: contains null byte |
| TC-B-02k | `issuerURL: "https://dex.example.com/../../../etc/passwd"` | Validation error or sanitized path |

### 5.3 SEC-04 — Non-Numeric NBF Rejection

**Current behavior:** `validateNotBefore` attempts to parse `nbf` as `float64`. If `nbf`
is a non-numeric type (string, bool, object), the type assertion fails and the claim is
silently ignored — the token is accepted.

**Required behavior:** Non-numeric `nbf` claims return `ErrMalformedToken`.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-B-04a | Valid JWT | `nbf` is numeric, in the past | Token accepted | Unit |
| TC-B-04b | Valid JWT | `nbf` is numeric, in the future | `ErrNotYetValid` | Unit |
| TC-B-04c | Valid JWT | `nbf` is string `"1234567890"` | `ErrMalformedToken` | Unit |
| TC-B-04d | Valid JWT | `nbf` is boolean `true` | `ErrMalformedToken` | Unit |
| TC-B-04e | Valid JWT | `nbf` is JSON object `{"time": 123}` | `ErrMalformedToken` | Unit |
| TC-B-04f | Valid JWT | `nbf` is JSON array `[123]` | `ErrMalformedToken` | Unit |
| TC-B-04g | Valid JWT | `nbf` absent | Token accepted (nbf is optional per RFC 7519) | Unit |
| TC-B-04h | Valid JWT | `nbf` is `0` | Token accepted (epoch is valid) | Unit |
| TC-B-04i | Valid JWT | `nbf` is negative `-1` | Token accepted (before epoch is valid per spec) | Unit |
| TC-B-04j | Valid JWT | `nbf` is `NaN` (float) | `ErrMalformedToken` | Unit |
| TC-B-04k | Valid JWT | `nbf` is `Inf` (float) | `ErrMalformedToken` | Unit |

### 5.4 SEC-05 — ErrTokenReplayed Sentinel

**Current behavior:** When a replayed JTI is detected, the error wraps `ErrTokenExpired`
(reusing the expired-token sentinel). `classifyAuthError` tags replays as `"token_expired"`
in metrics, making it impossible for SREs to distinguish replay attacks from expired tokens.

**Required behavior:** Define `ErrTokenReplayed` as a distinct sentinel. `classifyAuthError`
returns `"token_replayed"`. Metric label distinguishes the two.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-B-05a | Replay cache enabled | Token with JTI seen first time | Accepted | Unit |
| TC-B-05b | Replay cache enabled | Same JTI presented again | Error; `errors.Is(err, ErrTokenReplayed)` is true | Unit |
| TC-B-05c | Replay cache enabled | Replayed token error | `errors.Is(err, ErrTokenExpired)` is **false** | Unit |
| TC-B-05d | Replay cache enabled | `classifyAuthError(ErrTokenReplayed)` | Returns `"token_replayed"` | Unit |
| TC-B-05e | Replay cache enabled | `classifyAuthError(ErrTokenExpired)` | Returns `"token_expired"` (unchanged) | Unit |
| TC-B-05f | Replay cache disabled (nil) | Token with any JTI | Accepted (no replay check) | Unit |
| TC-B-05g | Replay cache enabled | Token without JTI | Accepted (JTI is optional; or error if required — document decision) | Unit |

**Observability:**

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-B-05h | Replay detected | Auth middleware processes replayed token | `af_http_requests_total{status="401"}` incremented with replay reason in log | Unit |

### 5.5 SEC-07 — TokenReview Identity Sanitization

**Current behavior:** `tokenreview.go` passes username and groups from Kubernetes
TokenReview response directly into `UserIdentity` without sanitization. Malicious
values could contain null bytes, control characters, or path traversal sequences
that propagate into logs, audit events, and downstream headers.

**Required behavior:** `SanitizeClaimValue()` applied to username and each group.
Sanitization strips null bytes, control characters (except space), and truncates to 256
characters.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-B-07a | Normal username | `"sre-user@example.com"` | Passes through unchanged | Unit |
| TC-B-07b | Username with null byte | `"admin\x00../../etc/passwd"` | Null byte stripped: `"admin../../etc/passwd"` (or rejected) | Unit |
| TC-B-07c | Username with control chars | `"admin\x01\x02\x03"` | Control chars stripped: `"admin"` | Unit |
| TC-B-07d | Username exceeding 256 chars | 300-char string | Truncated to 256 chars | Unit |
| TC-B-07e | Empty username | `""` | Empty string accepted (identity may have empty user with groups) | Unit |
| TC-B-07f | Unicode RTL override | `"admin\u202Efdp.txt"` | RTLO character stripped | Unit |
| TC-B-07g | Group with path traversal | `"../../etc/shadow"` | Passes through (path traversal in group name is not dangerous in K8s RBAC context) OR stripped — document decision | Unit |
| TC-B-07h | Multiple groups, mixed clean/dirty | `["valid", "null\x00byte", "long..."]` | Each individually sanitized | Unit |

**Adversarial battery:**

| TC ID | Input | Expected Result |
|-------|-------|-----------------|
| TC-B-07i | Username: 1 MB string | Truncated to 256 |
| TC-B-07j | Username: only null bytes `"\x00\x00\x00"` | Empty string after sanitization |
| TC-B-07k | Username: valid UTF-8 with emoji `"user🚀admin"` | Passes through (emoji are valid) |
| TC-B-07l | Username: invalid UTF-8 sequence `"\xff\xfe"` | Invalid bytes stripped or replaced |

## 6. Features Not to be Tested

| Feature | Rationale |
|---------|-----------|
| Distributed replay cache (Redis) | Deferred to operator (issue #98); in-memory cache tested here |
| JWKS key rotation | Existing test coverage in `jwks_cache_test.go` |
| Full OIDC flow | Covered by E2E suite and adversarial JWT tests |
| Token delegation expiry | Existing coverage in `jwt_delegation_test.go` |

## 7. Approach

### 7.1 Test Infrastructure

- **JWKS server:** `httptest.NewServer` returning configurable response bodies (valid JWKS,
  oversized, malformed)
- **JWT generation:** Existing `testJWKS` helper with `rsa.GenerateKey` for signing test tokens
- **Replay cache:** Construct with short TTL (100ms) for lifecycle tests
- **TokenReview mock:** Fake `authenticationv1.TokenReview` responses with adversarial fields

### 7.2 Concurrency Tests

- **Replay cache:** 10 goroutines racing `Seen()` with same JTI under `-race`
- **JWKS cache:** 10 goroutines calling `GetKeys()` concurrently, one with oversized body
- **Sanitize function:** 10 goroutines with different adversarial strings (stateless, but verify no race)

### 7.3 Resource Bound Tests

- **Replay cache:** 50 insert/expire cycles; assert internal map size returns to 0 after TTL
- **JWKS cache:** 50 fetch cycles with refresh interval; assert no memory growth

## 8. Pass/Fail Criteria

### Item-level

A test item **passes** when all its TC-* cases pass with `-race`.

### Plan-level

The plan **passes** when:
1. All test items pass
2. `make test-unit` exits 0
3. `gosec ./...` introduces no new findings
4. `errors.Is` chains verified for all sentinel errors
5. 9-category checkpoint audit (Checkpoint B) satisfied

## 9. Suspension Criteria

| Condition | Action |
|-----------|--------|
| Error sentinel change breaks upstream middleware tests | Fix middleware tests before continuing |
| Sanitization logic too aggressive (rejects valid inputs) | Relax rules; document rationale |
| Concurrency test reveals data race | Fix before advancing to Refactor |

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/test/cycle-b-security-hardening/TEST_PLAN.md` |
| JWKS body limit tests | `internal/auth/jwks_cache_test.go` (extend) |
| Issuer URL scheme tests | `internal/auth/config_test.go` (extend) |
| NBF strict tests | `internal/auth/jwt_test.go` (extend) |
| Replay sentinel tests | `internal/auth/replay_cache_test.go` (extend) |
| TokenReview sanitization tests | `internal/auth/tokenreview_test.go` (new or extend) |
| Adversarial battery | `internal/auth/adversarial_security_test.go` (extend) |

## 11. Environmental Needs

| Environment | Purpose |
|-------------|---------|
| Local Go 1.26+ | Unit tests |
| `gosec` binary | Security lint gate |
| No external services required | All tests use mocks/fakes |

## 12. Cross-Phase Integration (Cycle A → B)

| Integration point | Verification |
|-------------------|-------------|
| `classifyAuthError` returns `"token_replayed"` → Cycle A metric label `af_http_requests_total{status="401"}` | Checkpoint B test: construct middleware with replay cache, present replayed token, assert metric label |
| JWKS CB metric (`af_circuit_breaker_state{dependency="jwks_*"}`) from Cycle A `WithCBMetrics` wiring | Checkpoint B test: trigger JWKS body limit error, verify CB state metric emitted |
