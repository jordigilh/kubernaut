# Test Plan: Gateway Security Hardening

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-673-v1.4
**Feature**: Gateway service security hardening (14 audit findings + adversarial audit + L-3 timeout + config validation + L-1 trusted proxy, 34 test scenarios)
**Version**: 1.4
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active (all scenarios passing)
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the security hardening controls introduced by Issue #673. A security audit identified 14 findings across 4 severity levels. Eight findings were implemented as code/config changes. This plan covers the testable behavioral changes.

### 1.2 Objectives

1. **Body size limit**: Oversized request bodies are rejected with 413 at the earliest body-reading middleware layer.
2. **Generic error responses**: Auth/parse errors return generic messages; no internal details leak to clients.
3. **Identity header stripping**: Client-supplied `X-Auth-Request-User` is removed before authentication.
4. **Regression safety**: Existing auth middleware tests (UT-GW-036-*, UT-GW-037-*) continue to pass.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% (22/22 new) | `make test-unit-gateway` |
| Integration test pass rate | 100% (10/10 new) | `make test-integration-gateway` |

---

## 2. References

### 2.1 Authority

- Issue #673: Security audit: Gateway service hardening (14 findings)
- DD-AUTH-014: Middleware-Based SAR Authentication
- BR-GATEWAY-182: ServiceAccount Authentication (TokenReview)
- BR-GATEWAY-183: SubjectAccessReview Authorization
- BR-GATEWAY-017: Prometheus metrics observability
- BR-GATEWAY-053: Least-privilege RBAC

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Test Plan 291 (auth middleware)](../../testing/291/TEST_PLAN.md) — updated acceptance criteria for UT-GW-036-005, UT-GW-037-002, UT-GW-037-003

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | MaxBytesReader error not detected as distinct type | Oversized bodies return 400 instead of 413 | Low | IT-GW-673-001 | Explicit `errors.As(*http.MaxBytesError)` check in server.go |
| R2 | Generic error messages break client integrations expecting detail strings | Callers relying on error detail for logic break | Low | UT-GW-673-001..003 | Verified: no test or production code parses error detail strings |
| R3 | Header stripping removes legitimate downstream headers | Breaks inter-service identity propagation | Low | UT-GW-673-004/005 | Stripping occurs ONLY before auth; middleware re-sets header after successful auth |
| R4 | Integration test needs real K8s client for body limit test | Test env complexity | Medium | IT-GW-673-001/002 | nil auth + minimal Server config via StartTestGatewayWithOptions |

---

## 4. Scope

### 4.1 Features to be Tested

- **Request body size limit** (`pkg/gateway/server.go:readParseValidateSignal`): `http.MaxBytesReader` with 256KB cap. `*http.MaxBytesError` detection returns 413.
- **Generic error responses** (`pkg/shared/auth/middleware.go`): TokenReview errors return "Authentication service unavailable", SAR errors return "Authorization service unavailable", SAR denial returns "Insufficient permissions".
- **Identity header stripping** (`pkg/shared/auth/middleware.go`): `r.Header.Del("X-Auth-Request-User")` at handler entry.
- **Auth header format message** (`pkg/shared/auth/middleware.go`): Invalid format returns "Invalid Authorization header format" (no hint about expected format).
- **Per-handler K8s timeout** (`pkg/gateway/server.go:createAdapterHandler`): `context.WithTimeout` wraps `ProcessSignal` with configurable `K8sRequestTimeout` (default 15s). Timeout returns 504 with generic detail.

### 4.2 Features Not to be Tested

- **H-1 (metrics port)**: Chart/YAML change; validated by existing E2E `04_metrics_endpoint_test.go`
- **M-1 (RBAC secrets)**: YAML change; validated by RBAC audit
- **L-2 (ReadHeaderTimeout)**: Server config; validated by gosec G112 lint
- **L-4 (doc drift)**: Documentation only

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of auth middleware hardening code (error messages, header stripping)
- **Integration**: >=80% of body size limit code (MaxBytesReader + 413 response)

### 5.2 Option A (Retroactive GREEN)

Code changes are already implemented. Tests validate the NEW behavior and pass immediately. This is pragmatic; the implementation was verified via build + existing test suite before test plan creation.

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass.
**FAIL**: Any P0 test fails.

---

## 6. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-182 | TokenReview API error returns generic message | P0 | Unit | UT-GW-673-001 | Passed |
| BR-GATEWAY-183 | SAR API error returns generic message | P0 | Unit | UT-GW-673-002 | Passed |
| BR-GATEWAY-183 | SAR denial returns generic 403 | P0 | Unit | UT-GW-673-003 | Passed |
| BR-GATEWAY-183 | Client-supplied X-Auth-Request-User stripped | P0 | Unit | UT-GW-673-004 | Passed |
| BR-GATEWAY-183 | Spoofed header not propagated on auth failure | P0 | Unit | UT-GW-673-005 | Passed |
| BR-GATEWAY-182 | Auth header format hint removed | P1 | Unit | UT-GW-673-006 | Passed |
| BR-GATEWAY-182 | Multi-value X-Auth-Request-User headers all removed | P1 | Unit | UT-GW-673-015 | Passed |
| BR-GATEWAY-182 | Empty Authorization header returns 401 | P1 | Unit | UT-GW-673-016 | Passed |
| BR-GATEWAY-182 | Oversized body (>256KB) returns 413 | P0 | Integration | IT-GW-673-001 | Passed |
| BR-GATEWAY-182 | Normal body (<256KB) proceeds to parsing | P1 | Integration | IT-GW-673-002 | Passed |
| BR-GATEWAY-182 | 413 response carries correct RFC 7807 type/title | P0 | Integration | IT-GW-673-003 | Passed |
| BR-GATEWAY-182 | Boundary: exactly 256KB passes, 256KB+1 rejected | P0 | Integration | IT-GW-673-004 | Passed |
| BR-GATEWAY-182 | Oversized body via Prometheus freshness body-fallback | P0 | Integration | IT-GW-673-005 | Passed |
| BR-GATEWAY-182 | Oversized body via kubernetes-event endpoint | P0 | Integration | IT-GW-673-006 | Passed |
| BR-GATEWAY-182 | K8s API error returns generic detail | P0 | Integration | IT-GW-673-007 | Passed |
| BR-GATEWAY-102 | Slow K8s API returns 504 Gateway Timeout | P0 | Integration | IT-GW-673-008 | Passed |
| BR-GATEWAY-102 | Request within timeout succeeds normally | P1 | Integration | IT-GW-673-009 | Passed |
| BR-GATEWAY-102 | 504 detail is generic (no K8s/context leak) | P0 | Integration | IT-GW-673-010 | Passed |
| BR-GATEWAY-102 | Valid K8sRequestTimeout (< WriteTimeout) accepted | P1 | Unit | UT-GW-673-017 | Passed |
| BR-GATEWAY-102 | K8sRequestTimeout >= WriteTimeout rejected | P0 | Unit | UT-GW-673-018 | Passed |
| BR-GATEWAY-102 | K8sRequestTimeout < 1s rejected | P0 | Unit | UT-GW-673-019 | Passed |
| BR-GATEWAY-102 | K8sRequestTimeout of 0 (disabled) accepted | P1 | Unit | UT-GW-673-020 | Passed |
| BR-GATEWAY-102 | XFF honoured from trusted proxy | P0 | Unit | UT-GW-673-021 | Passed |
| BR-GATEWAY-102 | X-Real-IP preferred over XFF from trusted proxy | P1 | Unit | UT-GW-673-022 | Passed |
| BR-GATEWAY-102 | True-Client-IP honoured from trusted proxy | P1 | Unit | UT-GW-673-023 | Passed |
| BR-GATEWAY-102 | XFF ignored from untrusted source | P0 | Unit | UT-GW-673-024 | Passed |
| BR-GATEWAY-102 | X-Real-IP ignored from untrusted source | P0 | Unit | UT-GW-673-025 | Passed |
| BR-GATEWAY-102 | Fail-closed: empty CIDRs ignores all proxy headers | P0 | Unit | UT-GW-673-026 | Passed |
| BR-GATEWAY-102 | Malformed CIDR skipped, valid CIDR still works | P1 | Unit | UT-GW-673-027 | Passed |
| BR-GATEWAY-102 | Invalid IP in XFF gracefully rejected | P1 | Unit | UT-GW-673-028 | Passed |
| BR-GATEWAY-102 | RemoteAddr without port handled | P1 | Unit | UT-GW-673-029 | Passed |
| BR-GATEWAY-102 | IPv6 trusted proxy CIDR supported | P1 | Unit | UT-GW-673-030 | Passed |

---

## 7. Test Scenarios

### Unit Tier — `test/unit/gateway/middleware/auth_test.go`

---

### UT-GW-673-001: TokenReview API Error Returns Generic Detail

**BR**: BR-GATEWAY-182
**Priority**: P0
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: HTTP request with valid Bearer token; MockAuthenticator.ErrorToReturn = "connection refused"
**When**: Auth middleware processes the request
**Then**: 500 Internal Server Error; `detail` is "Authentication service unavailable" (NOT "Token validation failed: connection refused")

**Acceptance Criteria**:
- HTTP status code is 500
- `problem["detail"]` equals "Authentication service unavailable"
- `problem["detail"]` does NOT contain "connection refused"
- Next handler is NOT called

---

### UT-GW-673-002: SAR API Error Returns Generic Detail

**BR**: BR-GATEWAY-183
**Priority**: P0
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: Valid authenticated user; MockAuthorizer.ErrorToReturn = "etcd timeout"
**When**: Auth middleware processes the request
**Then**: 500 Internal Server Error; `detail` is "Authorization service unavailable" (NOT "Authorization check failed: etcd timeout")

**Acceptance Criteria**:
- HTTP status code is 500
- `problem["detail"]` equals "Authorization service unavailable"
- `problem["detail"]` does NOT contain "etcd timeout"
- Next handler is NOT called

---

### UT-GW-673-003: SAR Denial Returns Generic 403

**BR**: BR-GATEWAY-183
**Priority**: P0
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: Valid authenticated user; MockAuthorizer denies access
**When**: Auth middleware processes the request
**Then**: 403 Forbidden; `detail` is "Insufficient permissions" (NOT "Insufficient RBAC permissions: user verb:create on remediationrequests/gateway-signals")

**Acceptance Criteria**:
- HTTP status code is 403
- `problem["detail"]` equals "Insufficient permissions"
- `problem["detail"]` does NOT contain any of: user identity, verb, resource name
- Next handler is NOT called

---

### UT-GW-673-004: Client-Supplied X-Auth-Request-User Stripped

**BR**: BR-GATEWAY-183
**Priority**: P0
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: HTTP request with `X-Auth-Request-User: attacker-spoofed` AND valid Bearer token for "system:serviceaccount:ns:sa"; MockAuthorizer allows
**When**: Auth middleware processes the request
**Then**: Next handler receives `X-Auth-Request-User: system:serviceaccount:ns:sa` (the authenticated identity, NOT "attacker-spoofed")

**Acceptance Criteria**:
- Next handler receives request
- `r.Header.Get("X-Auth-Request-User")` equals authenticated user (NOT "attacker-spoofed")
- `auth.GetUserFromContext(r.Context())` equals authenticated user

---

### UT-GW-673-005: Spoofed Header Not Propagated on Auth Failure

**BR**: BR-GATEWAY-183
**Priority**: P0
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: HTTP request with `X-Auth-Request-User: attacker-spoofed` AND no Authorization header
**When**: Auth middleware processes the request
**Then**: 401 returned; next handler NOT called; spoofed header not visible to any downstream

**Acceptance Criteria**:
- HTTP status code is 401
- Next handler is NOT called

---

### UT-GW-673-006: Auth Header Format Hint Removed

**BR**: BR-GATEWAY-182
**Priority**: P1
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: HTTP request with `Authorization: Basic dXNlcjpwYXNz` (wrong scheme)
**When**: Auth middleware processes the request
**Then**: 401 returned; `detail` is "Invalid Authorization header format" (no hint like "expected 'Bearer <token>'")

**Acceptance Criteria**:
- HTTP status code is 401
- `problem["detail"]` equals "Invalid Authorization header format"
- `problem["detail"]` does NOT contain "Bearer" or "expected"

---

### Integration Tier — `test/integration/gateway/body_limit_integration_test.go`

---

### IT-GW-673-001: Oversized Body Returns 413

**BR**: BR-GATEWAY-182
**Priority**: P0
**File**: `test/integration/gateway/body_limit_integration_test.go`

**Given**: POST to `/api/v1/signals/prometheus` with 300KB body (exceeds 256KB limit)
**When**: Gateway processes the request
**Then**: 413 Request Entity Too Large with RFC 7807 body

**Acceptance Criteria**:
- HTTP status code is 413
- Content-Type is `application/problem+json`
- Response body is valid JSON

---

### IT-GW-673-002: Normal Body Proceeds to Parsing

**BR**: BR-GATEWAY-182
**Priority**: P1
**File**: `test/integration/gateway/body_limit_integration_test.go`

**Given**: POST to `/api/v1/signals/prometheus` with 100KB valid-structure Prometheus payload
**When**: Gateway processes the request
**Then**: Request is NOT rejected due to size (may fail for other validation reasons, but NOT 413)

**Acceptance Criteria**:
- HTTP status code is NOT 413
- Response indicates the request reached adapter parsing (400 for malformed alert, or 200/201 for valid)

---

### IT-GW-673-003: 413 Response Carries Correct RFC 7807 Type and Title

**BR**: BR-GATEWAY-182
**Priority**: P0
**File**: `test/integration/gateway/body_limit_integration_test.go`

**Given**: POST to `/api/v1/signals/prometheus` with 300KB body (exceeds 256KB limit)
**When**: Gateway processes the request
**Then**: 413 response body carries `type: "https://kubernaut.ai/problems/payload-too-large"` and `title: "Request Entity Too Large"`

**Acceptance Criteria**:
- HTTP status code is 413
- `problem["type"]` equals `ErrorTypePayloadTooLarge` URI
- `problem["title"]` equals `TitlePayloadTooLarge`
- `problem["status"]` equals 413

**Rationale**: IT-GW-673-001 validated status and Content-Type but not the RFC 7807 `type`/`title` fields. This test closes that gap and implicitly validates `getErrorTypeAndTitle(413)` through the full HTTP pipeline, which is preferable to a unit test since the function is package-private.

---

### IT-GW-673-004: Boundary — Exactly 256KB Passes, 256KB+1 Rejected

**BR**: BR-GATEWAY-182
**Priority**: P0
**File**: `test/integration/gateway/body_limit_integration_test.go`

**Given**: Two sequential requests to `/api/v1/signals/prometheus`
**When**: First request sends exactly 256KB (262144 bytes); second sends 256KB+1 (262145 bytes)
**Then**: First request is NOT rejected for size; second returns 413

**Acceptance Criteria**:
- 256KB body: HTTP status code is NOT 413
- 256KB+1 body: HTTP status code IS 413

**Rationale**: Validates the exact boundary of `maxRequestBodySize` to prevent off-by-one errors in `http.MaxBytesReader`.

---

### IT-GW-673-005: Oversized Body via Prometheus Freshness Body-Fallback

**BR**: BR-GATEWAY-182
**Priority**: P0
**File**: `test/integration/gateway/body_limit_integration_test.go`
**Finding**: C-ADV-1 (adversarial audit)

**Given**: POST to `/api/v1/signals/prometheus` with 300KB body and NO `X-Timestamp` header
**When**: AlertManagerFreshnessValidator body-fallback path reads the body
**Then**: 413 Request Entity Too Large with RFC 7807 `type: payload-too-large`

**Acceptance Criteria**:
- HTTP status code is 413
- Content-Type is `application/problem+json`
- `problem["type"]` equals `ErrorTypePayloadTooLarge` URI

**Rationale**: Without the C-ADV-1 fix, the freshness middleware performed unbounded `io.ReadAll` on the body-fallback path, allowing full memory allocation before the downstream `MaxBytesReader` in `readParseValidateSignal` could reject it.

---

### IT-GW-673-006: Oversized Body via Kubernetes-Event Endpoint

**BR**: BR-GATEWAY-182
**Priority**: P0
**File**: `test/integration/gateway/body_limit_integration_test.go`
**Finding**: C-ADV-1 + M-ADV-1 (adversarial audit)

**Given**: POST to `/api/v1/signals/kubernetes-event` with 300KB body
**When**: EventFreshnessValidator reads the body
**Then**: 413 Request Entity Too Large with RFC 7807 `type: payload-too-large`

**Acceptance Criteria**:
- HTTP status code is 413
- Content-Type is `application/problem+json`
- `problem["type"]` equals `ErrorTypePayloadTooLarge` URI

**Rationale**: The kubernetes-event adapter freshness middleware had the same unbounded `io.ReadAll` vulnerability as the Prometheus path.

---

### IT-GW-673-007: K8s API Error Returns Generic Detail

**BR**: BR-GATEWAY-182
**Priority**: P0
**File**: `test/integration/gateway/security_integration_test.go`
**Finding**: C-ADV-2 (adversarial audit)

**Given**: Valid Prometheus alert sent to gateway with broken K8s client (RemediationRequest CRD not in scheme)
**When**: `ShouldDeduplicate` List call fails → `handleProcessingError`
**Then**: 500 Internal Server Error; `detail` is "Internal server error" (no K8s API details)

**Acceptance Criteria**:
- HTTP status code is 500
- `problem["detail"]` equals "Internal server error"
- `problem["detail"]` does NOT contain "kubernetes", "deduplication", or "RemediationRequest"

**Rationale**: Before C-ADV-2, `handleProcessingError` used `fmt.Sprintf("Kubernetes API error: %v", err)` which leaked internal K8s error messages to HTTP clients.

---

### UT-GW-673-015: Multi-Value X-Auth-Request-User Headers All Removed

**BR**: BR-GATEWAY-182
**Priority**: P1
**File**: `test/unit/gateway/middleware/auth_test.go`
**Finding**: L-ADV-2 (adversarial audit)

**Given**: HTTP request with two `X-Auth-Request-User` headers ("spoofed-val-1", "spoofed-val-2") AND valid Bearer token
**When**: Auth middleware processes the request
**Then**: Next handler receives exactly one `X-Auth-Request-User` value (the authenticated identity)

**Acceptance Criteria**:
- `r.Header.Values("X-Auth-Request-User")` has exactly 1 element
- That element equals the authenticated identity

**Rationale**: Go's `Header.Del` removes all values for a key, but this was untested. An attacker could inject multiple header values to try to survive partial cleanup.

---

### UT-GW-673-016: Empty Authorization Header Returns 401

**BR**: BR-GATEWAY-182
**Priority**: P1
**File**: `test/unit/gateway/middleware/auth_test.go`
**Finding**: L-ADV-3 (adversarial audit)

**Given**: HTTP request with `Authorization: ""` (empty value)
**When**: Auth middleware processes the request
**Then**: 401 Unauthorized; next handler NOT called

**Acceptance Criteria**:
- HTTP status code is 401
- RFC 7807 title is "Unauthorized"
- Next handler is NOT called

**Rationale**: Empty string is an edge case for header parsing. Validates the auth middleware treats empty authorization identically to missing authorization.

---

### Integration Tier — `test/integration/gateway/timeout_integration_test.go`

---

### IT-GW-673-008: Slow K8s API Exceeding Timeout Returns 504

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/integration/gateway/timeout_integration_test.go`
**Finding**: L-3 (original security audit)

**Given**: Gateway with `K8sRequestTimeout: 50ms`, K8s client with 200ms delay on List operations
**When**: Valid Prometheus alert is submitted
**Then**: 504 Gateway Timeout with RFC 7807 `type: gateway-timeout`, `title: Gateway Timeout`

**Acceptance Criteria**:
- HTTP status code is 504
- `problem["type"]` equals `ErrorTypeGatewayTimeout` URI
- `problem["title"]` equals `TitleGatewayTimeout`
- `problem["detail"]` does NOT contain "kubernetes" or "context deadline"

---

### IT-GW-673-009: Request Within Timeout Succeeds Normally

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/integration/gateway/timeout_integration_test.go`

**Given**: Gateway with `K8sRequestTimeout: 15s`, normal (fast) K8s client
**When**: Valid Prometheus alert is submitted
**Then**: Response is NOT 504

**Acceptance Criteria**:
- HTTP status code is NOT 504

**Rationale**: Regression safety -- confirms that the timeout mechanism does not interfere with normal fast requests.

---

### IT-GW-673-010: 504 Detail is Generic

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/integration/gateway/timeout_integration_test.go`

**Given**: Gateway with `K8sRequestTimeout: 50ms`, K8s client with 200ms delay
**When**: Valid Prometheus alert is submitted and times out
**Then**: `problem["detail"]` equals exactly "Request processing timed out"

**Acceptance Criteria**:
- `problem["detail"]` equals "Request processing timed out"

**Rationale**: Ensures the timeout error detail is generic and does not leak internal Go context or K8s API information.

---

### Unit Tier — `test/unit/gateway/config/config_test.go`

---

### UT-GW-673-017: Valid K8sRequestTimeout (< WriteTimeout) Accepted

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/config/config_test.go`
**Finding**: ADV2-1 (adversarial audit Phase 2)

**Given**: Config with `K8sRequestTimeout: 15s`, `WriteTimeout: 30s`
**When**: `Validate()` is called
**Then**: No error returned

---

### UT-GW-673-018: K8sRequestTimeout >= WriteTimeout Rejected

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/unit/gateway/config/config_test.go`
**Finding**: ADV2-1/ADV2-9 (adversarial audit Phase 2)

**Given**: Config with `K8sRequestTimeout: 30s`, `WriteTimeout: 30s`
**When**: `Validate()` is called
**Then**: Error containing "k8sRequestTimeout" and "less than writeTimeout"

**Rationale**: If K8sRequestTimeout >= WriteTimeout, the HTTP server kills the connection before the 504 JSON response can be written, causing the client to see a connection reset instead of a clean error.

---

### UT-GW-673-019: K8sRequestTimeout < 1s Rejected

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/unit/gateway/config/config_test.go`
**Finding**: ADV2-1 (adversarial audit Phase 2)

**Given**: Config with `K8sRequestTimeout: 500ms`
**When**: `Validate()` is called
**Then**: Error containing "k8sRequestTimeout" and "too low"

**Rationale**: Sub-second timeouts would cause all K8s operations to fail immediately, effectively DoS-ing the gateway.

---

### UT-GW-673-020: K8sRequestTimeout of 0 (Disabled) Accepted

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/config/config_test.go`

**Given**: Config with `K8sRequestTimeout: 0`
**When**: `Validate()` is called
**Then**: No error returned

**Rationale**: Zero means "disabled" (`createAdapterHandler` skips the timeout). This is intentional for environments where WriteTimeout alone provides adequate protection.

---

### Unit Tier — `test/unit/gateway/middleware/trusted_realip_test.go`

---

### UT-GW-673-021: XFF Honoured from Trusted Proxy

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`
**Finding**: L-1 (original security audit)

**Given**: Middleware with trusted CIDR `127.0.0.1/32`; request from `127.0.0.1:12345` with `X-Forwarded-For: 203.0.113.50, 10.0.0.1`
**When**: Middleware processes the request
**Then**: `RemoteAddr` is set to `203.0.113.50` (leftmost XFF IP)

---

### UT-GW-673-022: X-Real-IP Preferred Over XFF

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`

**Given**: Trusted proxy; both `X-Real-IP` and `X-Forwarded-For` set
**When**: Middleware processes the request
**Then**: `RemoteAddr` is set to `X-Real-IP` value (higher priority)

---

### UT-GW-673-023: True-Client-IP Honoured

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`

**Given**: Trusted proxy in `10.0.0.0/8`; `True-Client-IP: 192.0.2.1`
**When**: Middleware processes the request
**Then**: `RemoteAddr` is set to `192.0.2.1`

---

### UT-GW-673-024: XFF Ignored from Untrusted Source

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`
**Finding**: L-1 (original security audit)

**Given**: Trusted CIDRs = `10.0.0.0/8`; request from `192.168.1.1:45678` with `X-Forwarded-For: 203.0.113.50`
**When**: Middleware processes the request
**Then**: `RemoteAddr` unchanged (`192.168.1.1:45678`)

---

### UT-GW-673-025: X-Real-IP Ignored from Untrusted Source

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`

**Given**: Untrusted source with `X-Real-IP` set
**When**: Middleware processes the request
**Then**: `RemoteAddr` unchanged

---

### UT-GW-673-026: Fail-Closed with Empty CIDRs

**BR**: BR-GATEWAY-102
**Priority**: P0
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`
**Finding**: L-1 (original security audit)

**Given**: No trusted CIDRs configured (nil); all three proxy headers set
**When**: Middleware processes the request
**Then**: `RemoteAddr` unchanged (proxy headers never trusted)

**Rationale**: Fail-closed is the critical security property. Without explicit CIDR configuration, the middleware behaves as if no reverse proxy exists.

---

### UT-GW-673-027: Malformed CIDR Skipped

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`

**Given**: CIDRs = `["not-a-cidr", "127.0.0.1/32"]`; request from `127.0.0.1`
**When**: Middleware processes the request
**Then**: Valid CIDR works; malformed CIDR silently skipped

---

### UT-GW-673-028: Invalid IP in XFF Rejected

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`

**Given**: Trusted proxy; `X-Forwarded-For: not-an-ip`
**When**: Middleware processes the request
**Then**: `RemoteAddr` unchanged (invalid IP rejected)

---

### UT-GW-673-029: RemoteAddr Without Port

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`

**Given**: `RemoteAddr = "127.0.0.1"` (no port); trusted CIDR matches
**When**: Middleware processes the request
**Then**: XFF IP used (handles missing port gracefully)

---

### UT-GW-673-030: IPv6 Trusted Proxy CIDR

**BR**: BR-GATEWAY-102
**Priority**: P1
**File**: `test/unit/gateway/middleware/trusted_realip_test.go`

**Given**: Trusted CIDR `::1/128`; request from `[::1]:12345`
**When**: Middleware processes the request
**Then**: XFF `2001:db8::1` is used as `RemoteAddr`
