# Test Plan: OIDC/OAuth2 Authentication with Issuer-Based Routing

**Test Plan Identifier:** TP-AF-002
**Issue:** [#2](https://github.com/jordigilh/kubernaut-apifrontend/issues/2)
**Version:** 1.0
**Date:** 2026-05-03
**Status:** Draft

---

## 1. Introduction

This test plan validates the OIDC/OAuth2 authentication subsystem of the kubernaut API Frontend (AF). The subsystem implements multi-provider JWT validation using the KEP-3331 (Kubernetes Structured Authentication) pattern, including issuer-based deterministic routing, CEL-based claim validation, JWKS caching with circuit breaker, and K8s TokenReview fallback.

### 1.1 Scope

- JWT token validation (signature, expiry, audience, issuer)
- Multi-provider issuer-based routing (deterministic, no fallthrough)
- CEL-based claim validation rules
- JWKS cache with TTL and circuit breaker (sony/gobreaker)
- K8s TokenReview for ServiceAccount tokens
- HTTP middleware integration (context propagation)
- L1 input sanitization (auth header control chars, body size limit)

### 1.2 References

- Issue #2: OIDC/OAuth2 authentication with issuer-based routing (KEP-3331 pattern)
- ARCHITECTURE.md Section 6: Security Model (Identity Chain, Circuit Breakers)
- KEP-3331: Kubernetes Structured Authentication Config
- kubernaut DD-AUTH-MCP-001: Security architecture

---

## 2. Test Items

| Item | Package | Version |
|------|---------|---------|
| `JWTValidator` | `internal/auth` | PR2 |
| `JWKSCache` | `internal/auth` | PR2 |
| `AuthMiddleware` | `internal/auth` | PR2 |
| `TokenReviewer` | `internal/auth` | PR2 |
| `AuthConfig` | `internal/auth` | PR2 |
| `ValidateHeaderValue` | `internal/security` | PR2 |

---

## 3. Features to Be Tested

### 3.1 JWT Validation (Core)

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-2.1 | Valid token acceptance | Valid Keycloak JWT with correct issuer, audience, and signature returns `UserIdentity{Username, Groups}` | Issue #2 |
| F-2.2 | Expired token rejection | Tokens past `exp` claim are rejected with 401 | Issue #2 |
| F-2.3 | Wrong audience rejection | Tokens not matching configured `audiences` are rejected with 401 | Issue #2 |
| F-2.4 | Unknown issuer fail-closed | Tokens from unconfigured issuers return 401 (no fallthrough to other providers) | Issue #2, ARCH §6 |
| F-2.5 | Malformed token rejection | Corrupt/truncated JWTs return 401 | Issue #2 |

### 3.2 Multi-Provider Routing

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-2.6 | Issuer-based routing | Token's `iss` claim deterministically selects the correct provider config | Issue #2 |
| F-2.7 | Duplicate issuer detection | Config with duplicate issuer URLs is rejected at load time | Issue #2 |
| F-2.8 | Multiple providers | N providers can coexist; each validates only its own tokens | Issue #2 |

### 3.3 CEL Claim Validation

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-2.9 | CEL rule enforcement | `userValidationRules` expressions are evaluated post-verification; failures return 401 | Issue #2 |
| F-2.10 | System prefix rejection | Rule `!user.username.startsWith('system:')` blocks external users with system: prefix | Issue #2 |

### 3.4 JWKS Cache and Circuit Breaker

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-2.11 | Cold cache fetch | First request triggers JWKS fetch from provider's `jwks_uri` | Issue #2 |
| F-2.12 | Stale cache on failure | If JWKS fetch fails, cached keys are used (fail-open for existing sessions) | ARCH §6 |
| F-2.13 | CB opens after 3 failures | 3 consecutive JWKS fetch failures transition circuit to open state | ARCH §6 |
| F-2.14 | CB half-open after 30s | After 30s in open state, one probe request is allowed through | ARCH §6 |
| F-2.15 | CB closes on success | 1 successful fetch in half-open state transitions circuit to closed | ARCH §6 |
| F-2.16 | Fail-open existing / fail-closed new | Open circuit: existing sessions with cached JWKS work; new sessions without cache are rejected | ARCH §6 |

### 3.5 Middleware Integration

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-2.17 | Context propagation | Validated `UserIdentity` is set in `context.Context` for downstream handlers | ARCH §6 |
| F-2.18 | Missing auth header | Request without `Authorization` header returns 401 | Issue #2 |
| F-2.19 | Control char sanitization | Auth header containing control characters (0x00-0x1F except HT) returns 400 | ARCH §6 L1 |
| F-2.20 | Body size limit | Request body exceeding `MaxBodySize` returns 413 | ARCH §6 L1 |

### 3.6 K8s TokenReview

| ID | Feature | Acceptance Criteria | Source |
|----|---------|-------------------|--------|
| F-2.21 | SA token validation | Non-JWT tokens are validated via K8s TokenReview API; valid SA tokens return identity | Issue #2 |

---

## 4. Features Not Tested

- Hot-reloading of provider configuration (deferred to operational readiness)
- ClaimMappings CEL expressions beyond `preferred_username` and `groups` (covered by CEL engine correctness)
- TLS certificate validation for JWKS endpoints (relies on Go's `net/http` default behavior)

---

## 5. Approach

### 5.1 Test Methodology

Test-Driven Development (TDD):
1. **Red phase:** Write all test cases as failing tests (no production code)
2. **Green phase:** Implement minimal production code to pass all tests
3. **Refactor phase:** Clean up code against 100-go-mistakes checklist

### 5.2 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Unit | Individual functions (`Validate`, `FetchJWKS`, CEL eval) | `testing`, `go-jose` (mint JWTs) |
| Integration | Middleware chain with `httptest.Server` (fake JWKS endpoint) | `httptest`, `net/http` |
| Component | Full auth middleware with mock TokenReview client | `fake.NewSimpleClientset` |

### 5.3 Test Infrastructure

- **Fake JWKS server:** `httptest.NewServer` serving JWK Set with configurable RSA/EC keys
- **JWT minting:** `go-jose/v4` to create signed tokens with controlled claims
- **Mock TokenReview:** `k8s.io/client-go/kubernetes/fake` with reactor for TokenReview
- **Circuit breaker testability:** Short timeouts (1ms) for half-open state in tests; production uses 30s
- **Clock abstraction:** Inject `time.Now` function for expiry and CB timeout testing

---

## 6. Pass/Fail Criteria

### 6.1 Pass Criteria

- All 18 test cases pass
- Code coverage >= 80% of `internal/auth/*.go` (excluding test files)
- Zero `golangci-lint` errors
- Zero race conditions (`go test -race`)
- No `panic()` calls in production code

### 6.2 Fail Criteria

- Any test case fails
- Coverage drops below 80%
- Lint errors introduced
- Security: any path allows access without valid credentials (fail-open on new sessions)

---

## 7. Suspension and Resumption Criteria

### 7.1 Suspension

- Blocking dependency unavailable (e.g., `cel-go` incompatible with Go 1.25)
- Critical security vulnerability discovered in `go-jose/v4`

### 7.2 Resumption

- Dependency issue resolved or alternative identified
- Vulnerability patched and dependency updated

---

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Test source code | `internal/auth/jwt_test.go` |
| Coverage report | Generated by `go test -coverprofile=coverage.out` |
| Test execution log | CI artifacts |
| This test plan | `docs/tests/2/test_plan.md` |

---

## 9. Environmental Needs

| Requirement | Details |
|-------------|---------|
| Go version | 1.25.x |
| Dependencies | `go-jose/v4`, `cel-go`, `sony/gobreaker`, `k8s.io/client-go` |
| External services | None (all mocked) |
| CI environment | GitHub Actions (linux/amd64) |

---

## 10. Responsibilities

| Role | Responsibility |
|------|---------------|
| Developer | Write tests, implement code, run local verification |
| CI | Automated test execution, coverage reporting, lint |
| Reviewer | Verify test adequacy, security properties, 100-go-mistakes compliance |

---

## 11. Schedule

| Phase | Duration | Gate |
|-------|----------|------|
| Red (Phase 1) | Tests written, all fail | Compilation succeeds, all tests fail |
| Green (Phase 2) | All tests pass | Checkpoint A passes |
| Refactor (Phase 3) | Code cleaned up | Checkpoint B passes |

---

## 12. Test Case Matrix

| Test Case | Features Covered | Priority |
|-----------|-----------------|----------|
| `TestValidateJWT_ValidToken_ReturnsUserIdentity` | F-2.1 | P0 |
| `TestValidateJWT_ExpiredToken_Returns401` | F-2.2 | P0 |
| `TestValidateJWT_WrongAudience_Returns401` | F-2.3 | P0 |
| `TestValidateJWT_UnknownIssuer_FailsClosed` | F-2.4 | P0 |
| `TestValidateJWT_MultipleProviders_RoutesToCorrectIssuer` | F-2.6, F-2.8 | P0 |
| `TestValidateJWT_DuplicateIssuers_ConfigError` | F-2.7 | P1 |
| `TestValidateJWT_CELValidation_RejectsSystemPrefix` | F-2.9, F-2.10 | P0 |
| `TestValidateJWT_MalformedToken_Returns401` | F-2.5 | P0 |
| `TestJWKSCache_FetchesOnFirstRequest` | F-2.11 | P0 |
| `TestJWKSCache_UsesStaleOnFetchFailure` | F-2.12 | P0 |
| `TestJWKSCircuitBreaker_OpensAfter3Failures` | F-2.13 | P0 |
| `TestJWKSCircuitBreaker_HalfOpenAfter30s` | F-2.14 | P1 |
| `TestJWKSCircuitBreaker_ClosesOnSuccess` | F-2.15 | P1 |
| `TestJWKSCircuitBreaker_ExistingSessionsFailOpen` | F-2.16 | P0 |
| `TestMiddleware_SetsUserContextOnSuccess` | F-2.17 | P0 |
| `TestMiddleware_NoAuthHeader_Returns401` | F-2.18 | P0 |
| `TestMiddleware_AuthHeaderControlChars_Returns400` | F-2.19 | P1 |
| `TestMiddleware_OversizedBody_Returns413` | F-2.20 | P1 |
| `TestMiddleware_K8sTokenReview_ValidSA` | F-2.21 | P1 |
