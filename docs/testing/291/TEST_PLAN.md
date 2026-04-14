# Test Plan: Gateway Authentication & Authorization Middleware

**Feature**: TokenReview-based authentication and SubjectAccessReview authorization for Gateway webhook endpoints
**Version**: 1.0
**Created**: 2026-03-02
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-bugfixes-demos`

**Authority**:
- [BR-GATEWAY-036]: Kubernetes TokenReviewer Authentication
- [BR-GATEWAY-037]: ServiceAccount RBAC Validation
- [DD-AUTH-014]: Middleware-Based SAR Authentication

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [DataStorage Auth Reference](../../../pkg/datastorage/server/middleware/auth.go)
- [Shared Auth Interfaces](../../../pkg/shared/auth/interfaces.go)

---

## 1. Scope

### In Scope

- **`pkg/gateway/middleware/auth.go`** (NEW): AuthMiddleware for Gateway — token extraction, TokenReview delegation, SAR delegation, RFC 7807 errors, context injection
- **`pkg/gateway/server.go`** (MODIFIED): Wire auth middleware into Server struct, constructors, and route setup for `/api/v1/signals/*` endpoints
- **`cmd/gateway/main.go`** (MODIFIED): Wire real K8sAuthenticator + K8sAuthorizer in production
- **Helm charts** (MODIFIED): ServiceAccount tokens and RBAC for AlertManager and Event Exporter
- **Existing GW tests** (MODIFIED): Wire real K8s auth (K8sAuthenticator + K8sAuthorizer) into integration test server constructors via envtest; add real ServiceAccount Bearer tokens to all webhook requests in INT, E2E, and FP tests

### Out of Scope

- **Auth config toggle**: Per user decision, auth is always enforced (DS pattern). No `auth.enabled` flag.
- **Rate limiting**: Implemented via chi Throttle middleware (ADR-048-ADDENDUM-001). Not related to auth.
- **Shared auth middleware refactoring**: Could extract a shared middleware from DS + GW, but that's a separate refactoring task.
- **mTLS or certificate-based auth**: Not in scope for v1.0.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Always-on auth (no disable flag) | DS pattern — more secure, real auth via DI in all tiers. Issue #291 AC-4 is superseded. |
| SAR resource: `services/gateway-service/create` | Consistent with DS pattern (`services/data-storage-service/create`). |
| Mirror DS auth middleware in `pkg/gateway/middleware/` | Keeps services independent; shared code is in `pkg/shared/auth/`. |
| Auth applied only to `/api/v1/signals/*` routes | Health, readiness, and metrics endpoints must remain unauthenticated for K8s probes and Prometheus scraping. |
| Helm changes in this PR | Feature is incomplete without SA tokens and RBAC for signal sources. |
| Real K8s auth in INT tests (no mocks) | No-mocks policy (INTEGRATION_E2E_NO_MOCKS_POLICY.md). Envtest supports TokenReview/SAR. GW already uses real auth for DS via `SecurityTestTokens` infrastructure. Mock auth only in unit tests. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (auth middleware: token extraction, error formatting, context injection, delegation logic)
- **Integration**: >=80% of **integration-testable** code (server wiring, middleware ordering, full HTTP flow with real K8s auth via envtest)
- **E2E**: >=80% of full service auth code (real K8s TokenReview + SAR in Kind)

### 2-Tier Minimum

Every BR is covered by all 3 tiers:
- **Unit tests**: Catch middleware logic errors (token parsing, error codes, context) — fast, isolated
- **Integration tests**: Catch wiring errors (middleware applied to correct routes, real K8s auth DI) — real HTTP, envtest
- **E2E tests**: Catch deployment/RBAC errors (real tokens, real SAR, Helm chart correctness) — Kind cluster

### Business Outcome Quality Bar

Tests validate business outcomes:
- "Unauthenticated callers cannot create RemediationRequests" (not "ValidateToken is called")
- "Authorized ServiceAccounts receive 201 Created" (not "Handler.ServeHTTP executes")
- "Health probes work without authentication" (not "auth middleware is skipped")

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/middleware/auth.go` (NEW) | `NewAuthMiddleware`, `Handler`, `GetUserFromContext`, `writeError` | ~120 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/server.go` | Constructor modifications, `setupRoutes` auth wiring | ~30 (delta) |
| `pkg/gateway/middleware/auth.go` | Full HTTP flow through middleware chain | ~120 |
| `test/integration/gateway/helpers_test.go` | Real K8s auth wiring in server creation helpers | ~20 (delta) |

### E2E-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/gateway/main.go` | Production wiring of K8sAuthenticator + K8sAuthorizer | ~15 (delta) |
| Helm charts | ServiceAccount, RBAC, token mounting for signal sources | ~60 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Unit | UT-GW-036-001 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Unit | UT-GW-036-002 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Unit | UT-GW-036-003 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Unit | UT-GW-036-004 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Unit | UT-GW-036-005 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Unit | UT-GW-036-006 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Unit | UT-GW-037-001 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Unit | UT-GW-037-002 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Unit | UT-GW-037-003 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Unit | UT-GW-037-004 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Unit | UT-GW-037-005 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Unit | UT-GW-037-006 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Integration | IT-GW-036-001 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Integration | IT-GW-036-002 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | Integration | IT-GW-036-003 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Integration | IT-GW-037-001 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | Integration | IT-GW-037-002 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | E2E | E2E-GW-036-001 | Pending |
| BR-GATEWAY-036 | TokenReview Authentication | P0 | E2E | E2E-GW-036-002 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | E2E | E2E-GW-037-001 | Pending |
| BR-GATEWAY-037 | SAR Authorization | P0 | E2E | E2E-GW-037-002 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-GW-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: GW (Gateway)
- **BR_NUMBER**: 036 (TokenReview) or 037 (SAR)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

---

### Tier 1: Unit Tests

**Testable code scope**: `pkg/gateway/middleware/auth.go` (~120 lines, target >=80%)

**File**: `test/unit/gateway/middleware/auth_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-036-001` | Callers without Authorization header receive 401 Unauthorized (RFC 7807) | Pending |
| `UT-GW-036-002` | Callers with non-Bearer Authorization scheme receive 401 Unauthorized | Pending |
| `UT-GW-036-003` | Callers with empty Bearer token receive 401 Unauthorized | Pending |
| `UT-GW-036-004` | Callers with invalid/expired token receive 401 Unauthorized | Pending |
| `UT-GW-036-005` | When TokenReview API fails, caller receives 500 Internal Server Error | Pending |
| `UT-GW-036-006` | Valid token is extracted and user identity is returned by authenticator | Pending |
| `UT-GW-037-001` | Authorized ServiceAccount passes through; next handler receives request with user in context | Pending |
| `UT-GW-037-002` | Authenticated but unauthorized ServiceAccount receives 403 Forbidden | Pending |
| `UT-GW-037-003` | When SAR API fails, caller receives 500 Internal Server Error | Pending |
| `UT-GW-037-004` | X-Auth-Request-User header is set for SOC2 user attribution | Pending |
| `UT-GW-037-005` | GetUserFromContext extracts authenticated user from context | Pending |
| `UT-GW-037-006` | GetUserFromContext returns empty string when no user in context | Pending |

---

### Tier 2: Integration Tests

**Testable code scope**: Server wiring + full HTTP flow (~150 lines delta, target >=80%)

**File**: `test/integration/gateway/auth_integration_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-GW-036-001` | Prometheus webhook endpoint rejects requests without authentication (401) | Pending |
| `IT-GW-036-002` | Prometheus webhook endpoint with valid auth creates RemediationRequest (201) | Pending |
| `IT-GW-036-003` | Health, readiness, and metrics endpoints work WITHOUT authentication | Pending |
| `IT-GW-037-001` | Webhook endpoint with valid token but unauthorized SA returns 403 | Pending |
| `IT-GW-037-002` | K8s Event webhook endpoint respects auth middleware identically to Prometheus | Pending |

**Infrastructure**: Real K8s auth via envtest (`K8sAuthenticator` + `K8sAuthorizer`). Real ServiceAccount tokens from `SecurityTestTokens` infrastructure. envtest for K8s API + TokenReview + SAR. No mocks (INTEGRATION_E2E_NO_MOCKS_POLICY).

---

### Tier 3: E2E Tests

**Testable code scope**: Full deployment with real K8s auth (~75 lines delta)

**File**: `test/e2e/gateway/38_auth_middleware_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-GW-036-001` | Real Gateway deployment rejects unauthenticated Prometheus webhook | Pending |
| `E2E-GW-036-002` | Real Gateway deployment accepts authenticated+authorized Prometheus webhook (201 Created) | Pending |
| `E2E-GW-037-001` | Real Gateway deployment rejects authenticated but unauthorized K8s event webhook (403) | Pending |
| `E2E-GW-037-002` | Authorized ServiceAccount's signal triggers full RR creation pipeline | Pending |

**Infrastructure**: Kind cluster, real K8s TokenReview + SAR, ServiceAccount tokens, RBAC.

---

### Existing Test Impact (NOT new scenarios — infrastructure updates)

These existing tests need auth-aware modifications:

**Integration tests** (`test/integration/gateway/`):
- `helpers_test.go`: Update `StartTestGateway`, `createGatewayServer`, and `SetupPriority1Test` to inject `K8sAuthenticator` + `K8sAuthorizer` (real K8s auth via envtest)
- `suite_test.go`: Create ServiceAccount + RBAC for Gateway auth in `SynchronizedBeforeSuite` Phase 1 (reuse `SecurityTestTokens` pattern)
- All webhook-sending helpers: Add real ServiceAccount Bearer token to HTTP requests
- `SendWebhookWithAuth` already supports Bearer tokens — existing tests will switch to using it

**E2E tests** (`test/e2e/gateway/`):
- All tests that POST to `/api/v1/signals/*` need ServiceAccount Bearer tokens
- `test/infrastructure/gateway_e2e.go`: Add RBAC for test ServiceAccounts to pass SAR
- E2E deduplication helpers: Add Bearer tokens to webhook requests

**Full Pipeline E2E** (`test/e2e/fullpipeline/`):
- AlertManager mock sends need ServiceAccount Bearer tokens
- Event Exporter sends need ServiceAccount Bearer tokens

---

## 6. Test Cases (Detail)

### UT-GW-036-001: Missing Authorization Header

**BR**: BR-GATEWAY-036
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: An HTTP request to a protected endpoint with no Authorization header
**When**: The auth middleware processes the request
**Then**: Returns 401 Unauthorized with RFC 7807 JSON body containing `"title": "Unauthorized"` and `"detail": "Missing Authorization header"`

**Acceptance Criteria**:
- HTTP status code is 401
- Content-Type is `application/problem+json`
- Response body is valid JSON matching RFC 7807 structure
- Next handler is NOT called

---

### UT-GW-036-002: Non-Bearer Authorization Scheme

**BR**: BR-GATEWAY-036
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: An HTTP request with `Authorization: Basic dXNlcjpwYXNz`
**When**: The auth middleware processes the request
**Then**: Returns 401 Unauthorized with detail about invalid format

**Acceptance Criteria**:
- HTTP status code is 401
- Detail mentions expected "Bearer" format
- Next handler is NOT called

---

### UT-GW-036-003: Empty Bearer Token

**BR**: BR-GATEWAY-036
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: An HTTP request with `Authorization: Bearer ` (empty token after prefix)
**When**: The auth middleware processes the request
**Then**: Returns 401 Unauthorized with detail about empty token

**Acceptance Criteria**:
- HTTP status code is 401
- Next handler is NOT called

---

### UT-GW-036-004: Invalid Token (Authentication Failure)

**BR**: BR-GATEWAY-036
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: An HTTP request with `Authorization: Bearer invalid-token-xyz` and a MockAuthenticator that rejects this token
**When**: The auth middleware processes the request
**Then**: Returns 401 Unauthorized with token validation failure detail

**Acceptance Criteria**:
- HTTP status code is 401
- MockAuthenticator.CallCount == 1
- Next handler is NOT called

---

### UT-GW-036-005: TokenReview API Error

**BR**: BR-GATEWAY-036
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: An HTTP request with a valid Bearer token, but MockAuthenticator.ErrorToReturn is set (simulating K8s API failure)
**When**: The auth middleware processes the request
**Then**: Returns 500 Internal Server Error (not 401, since the failure is infrastructure, not authentication)

**Acceptance Criteria**:
- HTTP status code is 500
- Detail equals "Authentication service unavailable" (Issue #673 C-2: generic error, no internals)
- Next handler is NOT called

---

### UT-GW-036-006: Valid Token Authenticated Successfully

**BR**: BR-GATEWAY-036
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: An HTTP request with `Authorization: Bearer valid-token` and MockAuthenticator maps "valid-token" to "system:serviceaccount:ns:sa"
**When**: The auth middleware processes the request
**Then**: The token is extracted correctly and the user identity is passed to the authorizer

**Acceptance Criteria**:
- MockAuthenticator.CallCount == 1
- MockAuthenticator received "valid-token" (without "Bearer " prefix)
- MockAuthorizer.CallCount == 1 (authorization check is invoked)

---

### UT-GW-037-001: Authorized User Passes Through

**BR**: BR-GATEWAY-037
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: A valid authenticated user and MockAuthorizer allows the user
**When**: The auth middleware processes the request
**Then**: Next handler is called with user identity in context

**Acceptance Criteria**:
- Next handler receives the request
- `GetUserFromContext(r.Context())` returns the authenticated user identity
- HTTP response status comes from the next handler (not middleware)

---

### UT-GW-037-002: Unauthorized User Gets 403

**BR**: BR-GATEWAY-037
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: A valid authenticated user but MockAuthorizer denies access
**When**: The auth middleware processes the request
**Then**: Returns 403 Forbidden with RBAC denial detail

**Acceptance Criteria**:
- HTTP status code is 403
- Content-Type is `application/problem+json`
- Detail equals "Insufficient permissions" (Issue #673 M-3: generic error, no RBAC details)
- Next handler is NOT called

---

### UT-GW-037-003: SAR API Error

**BR**: BR-GATEWAY-037
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: A valid authenticated user but MockAuthorizer.ErrorToReturn is set (simulating K8s API failure)
**When**: The auth middleware processes the request
**Then**: Returns 500 Internal Server Error

**Acceptance Criteria**:
- HTTP status code is 500
- Detail equals "Authorization service unavailable" (Issue #673 C-2: generic error, no internals)
- Next handler is NOT called

---

### UT-GW-037-004: X-Auth-Request-User Header Set

**BR**: BR-GATEWAY-037
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: A valid, authorized user "system:serviceaccount:kubernaut-system:alertmanager"
**When**: The auth middleware processes the request
**Then**: The `X-Auth-Request-User` header is set on the request passed to the next handler

**Acceptance Criteria**:
- Next handler receives request with `X-Auth-Request-User` header equal to the authenticated user identity
- This enables SOC2 CC8.1 user attribution in downstream handlers

---

### UT-GW-037-005: GetUserFromContext Returns User

**BR**: BR-GATEWAY-037
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: A context with UserContextKey set to "system:serviceaccount:ns:sa"
**When**: GetUserFromContext is called
**Then**: Returns "system:serviceaccount:ns:sa"

**Acceptance Criteria**:
- Return value matches the stored user identity exactly

---

### UT-GW-037-006: GetUserFromContext Returns Empty for Missing User

**BR**: BR-GATEWAY-037
**Type**: Unit
**File**: `test/unit/gateway/middleware/auth_test.go`

**Given**: A context without UserContextKey
**When**: GetUserFromContext is called
**Then**: Returns empty string

**Acceptance Criteria**:
- Return value is ""
- No panic or error

---

### IT-GW-036-001: Unauthenticated Webhook Request Rejected

**BR**: BR-GATEWAY-036
**Type**: Integration
**File**: `test/integration/gateway/auth_integration_test.go`

**Given**: A running Gateway server with real K8s auth (K8sAuthenticator + K8sAuthorizer via envtest) and a valid Prometheus alert payload
**When**: A POST request is sent to `/api/v1/signals/prometheus` without an Authorization header
**Then**: The server returns 401 Unauthorized and no RemediationRequest CRD is created

**Acceptance Criteria**:
- HTTP status code is 401
- Response body is RFC 7807 JSON
- No RR CRD exists in the test namespace

---

### IT-GW-036-002: Authenticated+Authorized Webhook Creates RR

**BR**: BR-GATEWAY-036
**Type**: Integration
**File**: `test/integration/gateway/auth_integration_test.go`

**Given**: A running Gateway server with real K8s auth via envtest. An authorized ServiceAccount (from `SecurityTestTokens`) with RBAC for `services/gateway-service/create` exists.
**When**: A POST request with `Authorization: Bearer <real-sa-token>` and valid Prometheus alert payload is sent to `/api/v1/signals/prometheus`
**Then**: The server returns 201 Created and a RemediationRequest CRD is created in Kubernetes

**Acceptance Criteria**:
- HTTP status code is 201
- RR CRD exists in the test namespace
- RR CRD has expected fields from the alert payload
- Real TokenReview + SAR validated the token (not mocked)

---

### IT-GW-036-003: Health/Readiness/Metrics Bypass Auth

**BR**: BR-GATEWAY-036
**Type**: Integration
**File**: `test/integration/gateway/auth_integration_test.go`

**Given**: A running Gateway server with real auth middleware enabled
**When**: GET requests are sent to `/health`, `/ready`, and `/metrics` without any Authorization header
**Then**: All return 200 OK

**Acceptance Criteria**:
- `/health` returns 200
- `/ready` returns 200
- `/metrics` returns 200
- No 401 or 403 responses for these operational endpoints

---

### IT-GW-037-001: Authenticated but Unauthorized Returns 403

**BR**: BR-GATEWAY-037
**Type**: Integration
**File**: `test/integration/gateway/auth_integration_test.go`

**Given**: A running Gateway server with real K8s auth via envtest. An unauthorized ServiceAccount (from `SecurityTestTokens`, no RBAC binding) exists.
**When**: A POST request with `Authorization: Bearer <unauthorized-sa-token>` and valid alert payload is sent
**Then**: The server returns 403 Forbidden and no RemediationRequest CRD is created

**Acceptance Criteria**:
- HTTP status code is 403
- Response body is RFC 7807 JSON mentioning insufficient permissions
- No RR CRD created
- Real SAR denied the request (not mocked)

---

### IT-GW-037-002: K8s Event Endpoint Equally Protected

**BR**: BR-GATEWAY-037
**Type**: Integration
**File**: `test/integration/gateway/auth_integration_test.go`

**Given**: A running Gateway server with real K8s auth via envtest
**When**: An unauthenticated POST request is sent to `/api/v1/signals/kubernetes-event`
**Then**: The server returns 401 Unauthorized

**Acceptance Criteria**:
- HTTP status code is 401
- Confirms auth middleware applies to ALL `/api/v1/signals/*` routes, not just Prometheus

---

### E2E-GW-036-001: Real Gateway Rejects Unauthenticated Request

**BR**: BR-GATEWAY-036
**Type**: E2E
**File**: `test/e2e/gateway/38_auth_middleware_test.go`

**Given**: Gateway deployed in Kind cluster with real K8s TokenReview + SAR
**When**: A Prometheus alert is POSTed to Gateway without a Bearer token
**Then**: Returns 401 Unauthorized

**Acceptance Criteria**:
- HTTP status code is 401
- Response is RFC 7807 JSON

---

### E2E-GW-036-002: Real Gateway Accepts Authorized Request

**BR**: BR-GATEWAY-036
**Type**: E2E
**File**: `test/e2e/gateway/38_auth_middleware_test.go`

**Given**: Gateway deployed in Kind cluster; a ServiceAccount with RBAC permissions for `services/gateway-service/create` exists
**When**: A Prometheus alert is POSTed with the ServiceAccount's Bearer token
**Then**: Returns 201 Created and a RemediationRequest CRD is created

**Acceptance Criteria**:
- HTTP status code is 201
- RR CRD exists in the expected namespace
- TokenReview and SAR were performed (confirmed by response, not mocked)

---

### E2E-GW-037-001: Real Gateway Rejects Unauthorized Request

**BR**: BR-GATEWAY-037
**Type**: E2E
**File**: `test/e2e/gateway/38_auth_middleware_test.go`

**Given**: Gateway deployed in Kind cluster; a ServiceAccount WITHOUT RBAC permissions exists
**When**: A K8s event is POSTed with the unauthorized ServiceAccount's Bearer token
**Then**: Returns 403 Forbidden

**Acceptance Criteria**:
- HTTP status code is 403
- Response mentions insufficient permissions

---

### E2E-GW-037-002: Full Pipeline with Auth

**BR**: BR-GATEWAY-037
**Type**: E2E
**File**: `test/e2e/gateway/38_auth_middleware_test.go`

**Given**: Gateway deployed with auth; authorized ServiceAccount token available
**When**: A valid Prometheus alert is POSTed with auth
**Then**: RemediationRequest CRD is created and enters the remediation pipeline

**Acceptance Criteria**:
- RR CRD created with correct fields
- RR reaches at least `Processing` phase (confirming full pipeline entry)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `auth.MockAuthenticator` + `auth.MockAuthorizer` from `pkg/shared/auth/mock_auth.go` (mocks permitted in unit tests ONLY — tests pure middleware logic in isolation)
- **HTTP**: `net/http/httptest.NewRecorder` + `http.NewRequest`
- **Location**: `test/unit/gateway/middleware/auth_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Auth**: Real `K8sAuthenticator` + `K8sAuthorizer` via envtest. Real ServiceAccount tokens from `SecurityTestTokens` infrastructure.
- **Infrastructure**: envtest (K8s API + TokenReview + SAR), DataStorage containers (PostgreSQL, Redis), httptest.NewServer
- **Auth helper**: Use `SendWebhookWithAuth` from `helpers_test.go` for Bearer token injection
- **Location**: `test/integration/gateway/auth_integration_test.go`
- **Existing test impact**: Update `StartTestGateway`, `createGatewayServer`, `SetupPriority1Test` to wire real K8s auth via envtest. Add real SA tokens to all existing webhook-sending helpers. Reuse `SecurityTestTokens` + `ServiceAccountHelper` patterns already in `security_suite_setup_test.go`.

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — real K8s TokenReview + SAR in Kind cluster
- **Infrastructure**: Kind cluster, ServiceAccount tokens via `helpers.ServiceAccountHelper`, RBAC via `setup-kind-cluster.sh`
- **Auth setup**: Reuse `SecurityTestTokens` pattern from `security_suite_setup_test.go`
- **Location**: `test/e2e/gateway/38_auth_middleware_test.go`
- **Existing test impact**: All existing E2E tests sending webhooks need Bearer tokens. Update `test/infrastructure/gateway_e2e.go` to create RBAC for test SA.

---

## 8. Execution

```bash
# Unit tests (auth middleware only)
go test ./test/unit/gateway/middleware/... --ginkgo.focus="UT-GW-036|UT-GW-037"

# All Gateway unit tests (verify no regressions)
make test

# Integration tests (auth only)
go test ./test/integration/gateway/... --ginkgo.focus="IT-GW-036|IT-GW-037"

# All Gateway integration tests (verify no regressions from helper changes)
make test-integration-gateway

# E2E tests (auth only)
go test ./test/e2e/gateway/... --ginkgo.focus="E2E-GW-036|E2E-GW-037"

# Full E2E suite
make test-e2e-gateway
```

---

## 9. TDD Execution Plan

### Phase 1: RED (Write Failing Tests)

**Step 1.1**: Write unit tests `UT-GW-036-001` through `UT-GW-037-006` in `test/unit/gateway/middleware/auth_test.go`. Tests reference `pkg/gateway/middleware/auth.go` which does not exist yet — tests will fail to compile.

**Step 1.2**: Write integration tests `IT-GW-036-001` through `IT-GW-037-002` in `test/integration/gateway/auth_integration_test.go`. Tests use real K8s auth via envtest (`K8sAuthenticator` + `K8sAuthorizer`) — will fail because server constructors don't accept auth yet and middleware doesn't exist.

**Step 1.3**: Write E2E tests `E2E-GW-036-001` through `E2E-GW-037-002` in `test/e2e/gateway/38_auth_middleware_test.go`. Tests send requests to real Gateway — will fail because Gateway has no auth.

**RED verification**: All new tests fail with clear reasons (compilation or assertion).

### Phase 2: GREEN (Minimal Implementation)

**Step 2.1**: Create `pkg/gateway/middleware/auth.go` — mirror DS AuthMiddleware pattern with Gateway-specific SAR config (`services/gateway-service/create`).

**Step 2.2**: Modify `pkg/gateway/server.go`:
- Add `authenticator` and `authorizer` fields to Server struct
- Update constructors (`NewServerForTesting`, `NewServerWithK8sClient`, `createServerWithClients`) to accept auth dependencies
- Apply auth middleware in `setupRoutes()` to `/api/v1/signals/*` route group only

**Step 2.3**: Modify `cmd/gateway/main.go`:
- Create K8s clientset for TokenReview/SAR
- Instantiate `K8sAuthenticator` + `K8sAuthorizer`
- Pass to `NewServer` (or equivalent)

**Step 2.4**: Update integration test helpers:
- Wire `K8sAuthenticator` + `K8sAuthorizer` into `createGatewayServer` and `StartTestGateway` (real K8s auth via envtest)
- Create Gateway-auth ServiceAccount + RBAC in `suite_test.go` SynchronizedBeforeSuite (reuse `SecurityTestTokens` pattern)
- Add real SA tokens to existing webhook helpers (use `SendWebhookWithAuth`)

**Step 2.5**: Update E2E infrastructure:
- Add ServiceAccount + RBAC to `test/infrastructure/gateway_e2e.go`
- Add Bearer tokens to E2E webhook requests

**Step 2.6**: Update Helm charts:
- ServiceAccount tokens for AlertManager and Event Exporter
- RBAC (ClusterRole/ClusterRoleBinding) for signal sources

**GREEN verification**: All 21 new tests pass. All existing tests pass (no regressions).

### Phase 3: REFACTOR (Clean Up)

**Step 3.1**: Review auth middleware for code quality — remove debug logging prefixes if not needed, ensure structured logging follows GW patterns.

**Step 3.2**: Review server.go changes — ensure constructor signatures are clean, document auth fields in Server struct.

**Step 3.3**: Ensure all existing test files that were modified compile and pass.

**Step 3.4**: Run full build + lint:
```bash
go build ./...
golangci-lint run --timeout=5m
```

**Step 3.5**: Fresh-eyes triage of all changes against BR-GATEWAY-036 and BR-GATEWAY-037 acceptance criteria.

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-02 | Initial test plan |
