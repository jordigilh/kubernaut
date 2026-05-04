# Test Plan: Pattern B JWT Trust-Boundary for API Frontend Delegated Impersonation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1009-v1.0
**Feature**: JWT-based identity delegation from kubernaut-apifrontend to Kubernaut Agent
**Version**: 1.0
**Created**: 2026-05-03
**Author**: AI-assisted
**Status**: Active
**Branch**: `development/v1.5`

---

## 1. Introduction

### 1.1 Purpose

This test plan provides comprehensive quality assurance for Pattern B JWT trust-boundary
implementation (#1009). The feature enables `kubernaut-apifrontend` to delegate user identity
to KA via cryptographically signed JWTs, replacing the previously proposed SA-token +
Impersonate-header approach. Given the security-critical nature (authentication bypass,
privilege escalation, impersonation abuse), this plan mandates adversarial test scenarios
and checkpoints at each TDD boundary.

### 1.2 Objectives

1. **JWT authentication correctness**: JWTAuthenticator validates signatures via JWKS, extracts identity from verified claims, and rejects all invalid tokens
2. **Composite routing safety**: CompositeAuthenticator routes JWT vs opaque tokens deterministically with fail-closed semantics on known-issuer errors
3. **Pattern A regression**: Direct in-cluster clients remain completely unaffected by Pattern B introduction
4. **Identity propagation**: ProviderType metadata flows from authentication through MCP tools to audit logging
5. **Defense-in-depth**: Multi-layer security (JWKS verification, SAR authorization, ClusterRoleBinding, header stripping) prevents single-point failures
6. **Forward compatibility**: Multi-issuer architecture supports v1.6 SPIRE addition without middleware changes

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/shared/auth/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/shared/auth/jwt_auth.go`, `composite_auth.go`, `claims.go` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on middleware pipeline, main.go wiring |
| Pattern A regression | 0 regressions | Existing auth tests pass without modification |
| Security scenarios | 100% pass | All adversarial scenarios pass |
| Build integrity | 0 errors | `go build ./...` clean after every phase |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-INTERACTIVE-001**: Interactive investigation sessions
- **BR-INTERACTIVE-002**: MCP tool access with user-scoped RBAC
- **BR-INTERACTIVE-003**: Audit attribution for interactive actions
- **DD-AUTH-MCP-001 v2.0**: MCP endpoint security (JWT-based Pattern B)
- **Issue #1009**: Pattern B trust-boundary mechanism for AF delegated impersonation
- **Issue #896**: Strip Impersonate-* headers in auth middleware
- **Issue #895**: Authenticator returns user groups for impersonation

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Go Coding Standards](../../../.cursor/rules/02-go-coding-standards.mdc)
- [kubernaut-apifrontend#2](https://github.com/jordigilh/kubernaut-apifrontend/issues/2): KEP-3331 multi-provider OIDC
- [kubernaut-apifrontend#3](https://github.com/jordigilh/kubernaut-apifrontend/issues/3): MCP-to-MCP proxy
- [100 Go Mistakes](https://100go.co/) — REFACTOR phase validation checklist

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | JWT signature bypass (alg:none, key confusion) | Critical: impersonation of any user | Low | UT-KA-1009-008, UT-KA-1009-009 | Explicit alg allowlist (RS256 only), reject unsigned tokens |
| R2 | Cross-path authentication leak (JWT error falls through to K8s TokenReview) | Critical: untrusted token accepted | Medium | UT-KA-1009-016, UT-KA-1009-017 | ErrIssuerNotFound vs ErrTokenInvalid sentinel distinction; fail-closed on known issuer |
| R3 | Pattern A regression (direct clients broken) | High: production auth failure | Low | UT-KA-1009-018, IT-KA-1009-005 | CompositeAuthenticator passthrough; existing test suite unchanged |
| R4 | JWKS endpoint unavailable at startup | Medium: Pattern B non-functional | Medium | UT-KA-1009-010 | Pre-warm with 15s timeout; global disable (Pattern A unaffected) |
| R5 | Dot-notation claim extraction fails on Keycloak nested claims | Medium: user identity lost | Low | UT-KA-1009-003..005 | PoC validated with 11 scenarios; recursive map traversal |
| R6 | JWT replay within validity window | Medium: unauthorized action replay | Medium | UT-KA-1009-011 | Defense-in-depth (ClusterIP, NetworkPolicy, SAR, audit); v1.6: short-lived internal JWTs |
| R7 | ClusterRoleBinding group mismatch | Medium: JWT users denied | Low | UT-KA-1009-030, IT-KA-1009-009 | Helm template test validates group rendering |
| R8 | ProviderType not propagated to audit trail | Low: forensic gap | Low | UT-KA-1009-022, IT-KA-1009-006 | Middleware logs provider metadata |
| R9 | Concurrent JWKS cache refresh race condition | Low: intermittent auth failures | Low | UT-KA-1009-012 | lestrrat-go/jwx cache is goroutine-safe; integration test under concurrent load |

### 3.1 Risk-to-Test Traceability

- **R1 (Critical)**: Covered by UT-KA-1009-008 (alg:none rejection), UT-KA-1009-009 (wrong key rejection)
- **R2 (Critical)**: Covered by UT-KA-1009-016 (known issuer bad token → fail-closed), UT-KA-1009-017 (unknown issuer → fallback)
- **R3 (High)**: Covered by UT-KA-1009-018 (Pattern A passthrough), IT-KA-1009-005 (mixed traffic)
- **R4-R9**: Mapped in Affected Tests column above

---

## 4. Scope

### 4.1 Features to be Tested

- **JWTAuthenticator** (`pkg/shared/auth/jwt_auth.go`): JWKS-based JWT signature verification, multi-issuer routing, dot-notation claim extraction, error type distinction
- **CompositeAuthenticator** (`pkg/shared/auth/composite_auth.go`): Token shape routing (JWT vs opaque), fail-closed semantics, ErrIssuerNotFound fallback
- **JWTProviderConfig** (`internal/kubernautagent/config/config.go`): Configuration types, validation rules, Helm values mapping
- **ProviderType propagation** (`pkg/shared/auth/interfaces.go`, `internal/kubernautagent/mcp/interfaces.go`): Provider metadata in UserInfo, middleware audit logging
- **Main.go wiring** (`cmd/kubernautagent/main.go`): CompositeAuthenticator construction, JWKS pre-warm, soft-disable on failure
- **Claim extraction** (`pkg/shared/auth/claims.go`): Dot-notation path resolution for nested JWT claims
- **Helm chart** (`charts/kubernaut/templates/kubernaut-agent/`): jwtProviders ConfigMap rendering, conditional ClusterRoleBinding

### 4.2 Features Not to be Tested

- **AF-side JWT forwarding**: Owned by kubernaut-apifrontend repo (kubernaut-apifrontend#3)
- **Keycloak configuration**: External OIDC provider setup (documented in operator deployment guide)
- **SPIRE integration**: v1.6 scope (kubernaut-apifrontend#31)
- **DEX E2E provider**: Implemented — DEX v2.45.1 deployed in Kind cluster for E2E JWT testing

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Mock JWKS server for unit/integration tests | Real OIDC provider adds infrastructure complexity; MockJWKSServer (from PoC) provides deterministic behavior |
| Separate JWTAuthenticator from CompositeAuthenticator | Single responsibility; testable in isolation; CompositeAuthenticator is pure routing logic |
| ErrIssuerNotFound as distinct error type | Enables fallback without security leak; distinguishes "unknown provider" from "known provider, bad token" |
| ProviderType as string not enum | Forward-compatible with unknown future providers (SPIRE, AuthBridge); format: "jwt:<issuer>" or "k8s:tokenreview" |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`jwt_auth.go`, `composite_auth.go`, `claims.go`, config types, error types)
- **Integration**: >=80% of integration-testable code (middleware pipeline with mock JWKS, main.go wiring paths, mixed Pattern A + B traffic)
- **E2E**: >=80% of E2E-testable code — DEX OIDC provider in Kind cluster validates full JWT/OIDC flow

### 5.2 Two-Tier Minimum

Every acceptance criterion is covered by at least 2 test tiers:
- **Unit tests**: JWT validation logic, claim extraction, composite routing, error types
- **Integration tests**: Full middleware pipeline, main.go wiring, mixed traffic patterns

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes** — "can AF delegate identity to KA securely?" — not just
code path coverage. Each test scenario answers: "what does the operator/user/system get?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions approved by reviewer
3. Per-tier code coverage meets >=80% threshold
4. No regressions in existing test suites (auth middleware, header stripping, K8s auth)
5. `go build ./...` succeeds with 0 errors
6. `golangci-lint run --timeout=5m` produces no new errors

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage falls below 80% on any tier
3. Existing tests that were passing before the change now fail
4. Any adversarial security test fails
5. Build errors introduced

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- Build broken — code does not compile; unit tests cannot execute
- CHECKPOINT audit identifies blocking security issue
- lestrrat-go/jwx/v3 API incompatibility discovered (mitigate: PoC validated v3.0.13)

**Resume testing when**:

- Build fixed and green
- CHECKPOINT finding resolved and verified
- Dependency issue resolved with pinned version

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/auth/jwt_auth.go` | `NewJWTAuthenticator`, `ValidateToken`, `ValidateTokenFull`, `resolveIssuer`, `extractIdentity` | ~150 |
| `pkg/shared/auth/composite_auth.go` | `NewCompositeAuthenticator`, `ValidateToken`, `ValidateTokenFull`, `isJWT`, `routeToken` | ~80 |
| `pkg/shared/auth/claims.go` | `ExtractClaim`, `ExtractStringClaim`, `ExtractGroupsClaim` | ~73 |
| `internal/kubernautagent/config/config.go` | `JWTProviderConfig`, `ClaimMappings`, validation rules | ~60 |
| `pkg/shared/auth/interfaces.go` | `UserInfo` (ProviderType field), error sentinels | ~20 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/auth/middleware.go` | `Handler` (composite auth path), audit logging with ProviderType | ~30 (modified lines) |
| `cmd/kubernautagent/main.go` | `newAuthMiddleware` (CompositeAuthenticator wiring), JWKS pre-warm | ~50 |
| `internal/kubernautagent/mcp/interfaces.go` | `UserInfo` (ProviderType propagation) | ~10 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.5` HEAD | Post-PR #1018 merge |
| lestrrat-go/jwx/v3 | v3.0.13 | PoC validated |
| MockJWKSServer | From PoC | `test/shared/auth/mock_jwks_server.go` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTERACTIVE-002 | JWT-bearing requests authenticated via JWKS signature verification | P0 | Unit | UT-KA-1009-001 | Pending |
| BR-INTERACTIVE-002 | JWT-bearing requests authenticated via JWKS signature verification | P0 | Integration | IT-KA-1009-001 | Pending |
| BR-INTERACTIVE-002 | User identity extracted from verified JWT claims | P0 | Unit | UT-KA-1009-002..005 | Pending |
| BR-INTERACTIVE-002 | Unknown JWT issuers fall back to K8s TokenReview | P0 | Unit | UT-KA-1009-017 | Pending |
| BR-INTERACTIVE-002 | Known issuers with bad tokens fail-closed | P0 | Unit | UT-KA-1009-016 | Pending |
| BR-INTERACTIVE-001 | Pattern A clients unaffected (no regression on #896) | P0 | Unit | UT-KA-1009-018 | Pending |
| BR-INTERACTIVE-001 | Pattern A clients unaffected (no regression on #896) | P0 | Integration | IT-KA-1009-005 | Pending |
| BR-INTERACTIVE-002 | JWT users authorized via K8s SAR against ClusterRoleBinding | P0 | Integration | IT-KA-1009-003 | Deferred (v1.6) |
| BR-INTERACTIVE-003 | ProviderType propagated to audit logging | P1 | Unit | UT-KA-1009-022 | Pending |
| BR-INTERACTIVE-003 | ProviderType propagated to audit logging | P1 | Integration | IT-KA-1009-006 | Deferred (v1.6) |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-1009-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **KA**: Kubernaut Agent service
- **1009**: Issue number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/shared/auth/jwt_auth.go`, `composite_auth.go`, `claims.go`, config types — >=80% coverage target

#### Phase 1: Config Types (JWTProviderConfig + ClaimMappings)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-1009-025` | Valid JWTProviderConfig with all required fields passes validation | Pass |
| `UT-KA-1009-026` | JWTProviderConfig without issuer URL fails validation with clear error | Pass |
| `UT-KA-1009-027` | JWTProviderConfig without JWKS URL fails validation with clear error | Pass |
| `UT-KA-1009-028` | JWTProviderConfig without audience fails validation with clear error | Pass |
| `UT-KA-1009-029` | ClaimMappings defaults applied when not specified (preferred_username, groups) | Pass |
| `UT-KA-1009-030` | InteractiveConfig with jwtProviders validates all providers | Pass |
| `UT-KA-1009-031` | Duplicate issuer URLs across providers rejected | Pass |
| `UT-KA-1009-034` | JWKS URL exceeding max length (2048) rejected | Pass |
| `UT-KA-1009-035` | Syntactically invalid JWKS URL rejected via net/url.Parse | Pass |
| `UT-KA-1009-036` | JWKS URL with unsupported scheme (non-HTTP/HTTPS) rejected | Pass |
| `UT-KA-1009-037` | JWKS URL with HTTP scheme accepted (dev/test flexibility) | Pass |
| `UT-KA-1009-038` | Issuer URL exceeding max length (2048) rejected | Pass |

#### Phase 2: JWTAuthenticator

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-1009-001` | Valid JWT from known issuer accepted, correct username+groups extracted | Pending |
| `UT-KA-1009-002` | Username extracted from dot-notation claim path (e.g., preferred_username) | Pending |
| `UT-KA-1009-003` | Groups extracted from nested claim path (e.g., realm_access.roles) | Pending |
| `UT-KA-1009-004` | Groups extracted from double-nested path (resource_access.client.roles) | Pending |
| `UT-KA-1009-005` | Top-level groups claim extracted correctly | Pending |
| `UT-KA-1009-006` | Expired JWT rejected with ErrTokenInvalid | Pending |
| `UT-KA-1009-007` | JWT with wrong audience rejected with ErrTokenInvalid | Pending |
| `UT-KA-1009-008` | JWT with alg:none rejected (alg confusion attack) | Pending |
| `UT-KA-1009-009` | JWT signed by unknown key rejected with ErrTokenInvalid | Pending |
| `UT-KA-1009-010` | JWT from unknown issuer returns ErrIssuerNotFound (not ErrTokenInvalid) | Pending |
| `UT-KA-1009-011` | JWT with missing username claim rejected with descriptive error | Pending |
| `UT-KA-1009-012` | Multi-issuer: tokens routed to correct JWKS endpoint by iss claim | Pending |
| `UT-KA-1009-013` | Multi-issuer: cross-provider rejection (token from issuer-A rejected by issuer-B's JWKS) | Pending |
| `UT-KA-1009-014` | ProviderType set to "jwt:<issuer>" on successful validation | Pending |

#### Phase 3: CompositeAuthenticator

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-1009-015` | JWT-shaped token (3-dot structure) routed to JWTAuthenticator | Pending |
| `UT-KA-1009-016` | Known issuer with invalid signature → fail-closed (no fallback to K8s) | Pending |
| `UT-KA-1009-017` | Unknown issuer → ErrIssuerNotFound → fallback to K8sAuthenticator | Pending |
| `UT-KA-1009-018` | Opaque token (non-JWT) → direct K8sAuthenticator (Pattern A unchanged) | Pending |
| `UT-KA-1009-019` | Empty token → error (no fallback, no routing) | Pending |
| `UT-KA-1009-020` | K8sAuthenticator error after fallback → propagated correctly | Pending |
| `UT-KA-1009-021` | Malformed JWT (3 dots but not base64) → treated as opaque, fallback to K8s | Pending |

#### Phase 4: ProviderType Propagation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-1009-022` | UserInfo.ProviderType populated for JWT-authenticated users | Pending |
| `UT-KA-1009-023` | UserInfo.ProviderType populated for K8s-authenticated users ("k8s:tokenreview") | Pending |
| `UT-KA-1009-024` | MCP UserInfo bridges ProviderType from shared auth UserInfo | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Middleware pipeline, main.go wiring, mixed traffic — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-1009-001` | Full middleware pipeline accepts valid JWT, injects UserInfo into context | Pending |
| `IT-KA-1009-002` | Full middleware pipeline rejects expired JWT with 401 | Pending |
| `IT-KA-1009-003` | JWT-authenticated user passes SAR check against ClusterRoleBinding | Deferred (v1.6 — requires real K8s SAR in IT) |
| `IT-KA-1009-004` | JWT-authenticated user fails SAR check → 403 Forbidden | Pending |
| `IT-KA-1009-005` | Mixed traffic: JWT request + K8s SA request both succeed in same middleware instance | Pending |
| `IT-KA-1009-006` | ProviderType appears in middleware audit log entries | Deferred (v1.6 — audit integration) |
| `IT-KA-1009-007` | Impersonate-* headers stripped before JWT validation (defense-in-depth) | Pending |
| `IT-KA-1009-008` | JWKS pre-warm failure → Pattern B disabled globally, Pattern A works | Deferred (v1.6 — per-provider degradation) |
| `IT-KA-1009-009` | Helm-rendered ClusterRoleBinding resolves correct group from values | Deferred (v1.6 — Helm template testing) |

### Tier 3: E2E Tests

**Testable code scope**: Full stack with DEX v2.45.1 OIDC provider in Kind cluster

| ID | Business Outcome Under Test | Phase | Status |
|----|----------------------------|-------|--------|
| `E2E-KA-JWT-001` | Real JWT from DEX accepted by KA deployed in Kind → 200 OK | E2E | Implemented |
| `E2E-KA-JWT-002` | Forged JWT rejected with 401 (fail-closed in real cluster) | E2E | Implemented |
| `E2E-KA-JWT-003` | Pattern A (SA token) + Pattern B (JWT) coexist in deployed KA | E2E | Implemented |
| `E2E-KA-JWT-004` | JWT user invokes kubernaut_investigate → session with JWT identity | E2E | Implemented |
| `E2E-KA-JWT-005` | DEX-issued JWT structure validated end-to-end | E2E | Implemented |

**Infrastructure**: DEX deployed as Phase 5.8 in `SetupKubernautAgentInfrastructure()`, RBAC for DEX user, `jwtProviders` in KA ConfigMap.
**Location**: `test/e2e/kubernautagent/jwt_e2e_test.go`

---

## 9. Test Cases

### UT-KA-1009-001: Valid JWT accepted with correct identity extraction

**BR**: BR-INTERACTIVE-002
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/auth/jwt_auth_test.go`

**Preconditions**:
- MockJWKSServer running with known RSA key pair
- JWTAuthenticator configured with one provider pointing to mock server

**Test Steps**:
1. **Given**: A JWTAuthenticator configured with issuer "https://keycloak.example.com/realms/kubernaut" and audience "kubernaut-agent"
2. **When**: A valid JWT is issued by MockJWKSServer with sub="user-a@corp", preferred_username="user-a@corp", groups=["kubernaut-interactive-users"]
3. **Then**: ValidateTokenFull returns UserInfo{Username: "user-a@corp", Groups: ["kubernaut-interactive-users"], ProviderType: "jwt:https://keycloak.example.com/realms/kubernaut"}

**Expected Results**:
1. No error returned
2. Username matches preferred_username claim
3. Groups match groups claim
4. ProviderType contains "jwt:" prefix + issuer URL

**Acceptance Criteria**:
- **Behavior**: JWT signature verified against JWKS endpoint
- **Correctness**: Identity matches JWT claims exactly
- **Accuracy**: No data loss or transformation errors

### UT-KA-1009-008: alg:none JWT rejected (security-critical)

**BR**: BR-INTERACTIVE-002
**Priority**: P0 (Security)
**Type**: Unit
**File**: `test/unit/shared/auth/jwt_auth_test.go`

**Preconditions**:
- JWTAuthenticator configured with RS256-only provider

**Test Steps**:
1. **Given**: A JWTAuthenticator configured for RS256 algorithm
2. **When**: A JWT with alg:none header is presented (unsigned token)
3. **Then**: Token is rejected with ErrTokenInvalid

**Expected Results**:
1. Error returned wrapping ErrTokenInvalid
2. No identity extracted
3. No fallback to other auth path

**Acceptance Criteria**:
- **Behavior**: Algorithm confusion attack blocked
- **Correctness**: Error is ErrTokenInvalid (fail-closed, not ErrIssuerNotFound)

### UT-KA-1009-016: Known issuer, invalid signature → fail-closed

**BR**: BR-INTERACTIVE-002
**Priority**: P0 (Security)
**Type**: Unit
**File**: `test/unit/shared/auth/composite_auth_test.go`

**Preconditions**:
- CompositeAuthenticator with JWTAuthenticator + K8sAuthenticator
- JWT signed with wrong key but correct issuer claim

**Test Steps**:
1. **Given**: A CompositeAuthenticator with Keycloak issuer configured
2. **When**: A JWT with correct issuer but signed by a different RSA key is presented
3. **Then**: Authentication fails with ErrTokenInvalid — NO fallback to K8sAuthenticator

**Expected Results**:
1. Error wrapping ErrTokenInvalid returned
2. K8sAuthenticator.ValidateTokenFull is NOT called (zero call count)
3. Response is 401 Unauthorized

**Acceptance Criteria**:
- **Behavior**: Fail-closed prevents cross-path security leak
- **Correctness**: Known issuer errors are terminal, no fallback
- **Security**: Attacker cannot forge a JWT with known issuer to bypass into K8s TokenReview path

### IT-KA-1009-005: Mixed Pattern A + Pattern B traffic

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-002
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kubernautagent/jwt_middleware_test.go`

**Preconditions**:
- Full middleware pipeline with CompositeAuthenticator (MockJWKSServer + MockK8sAuthenticator)
- httptest.Server serving the middleware

**Test Steps**:
1. **Given**: A middleware pipeline with CompositeAuthenticator configured
2. **When**: Request 1 sends a valid JWT token (Pattern B), then Request 2 sends a valid K8s SA token (Pattern A)
3. **Then**: Both requests succeed with correct UserInfo in context

**Expected Results**:
1. JWT request: UserInfo.ProviderType = "jwt:<issuer>"
2. K8s request: UserInfo.ProviderType = "k8s:tokenreview"
3. Both requests pass SAR check
4. No cross-contamination between auth paths

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: MockJWKSServer (JWKS endpoint), MockAuthenticator (K8s fallback), MockAuthorizer (SAR)
- **Location**: `test/unit/shared/auth/`
- **External deps mocked**: JWKS HTTP endpoint (httptest), K8s TokenReview, K8s SAR

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: httptest for full middleware pipeline, MockJWKSServer for JWKS
- **Mocks**: MockAuthorizer for SAR (K8s auth not available without cluster)
- **Location**: `test/integration/kubernautagent/jwt_middleware_test.go`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster + DEX v2.45.1 OIDC provider (Podman)
- **OIDC Provider**: DEX with static password user (`e2e-user@kubernaut.test`) and static client (`kubernaut-agent`)
- **Token Flow**: Resource Owner Password Credentials grant → `id_token` (JWT)
- **KA Config**: `jwtProviders` pointing to `http://dex:5556/dex` (cluster-internal)
- **RBAC**: K8s Role/RoleBinding for DEX user (`e2e-user@kubernaut.test`) granting `services/kubernaut-agent` access
- **Location**: `test/e2e/kubernautagent/jwt_e2e_test.go`
- **Infrastructure helpers**: `test/infrastructure/dex_e2e.go`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25.6 | Build and test |
| Ginkgo CLI | v2.28+ | Test runner |
| lestrrat-go/jwx/v3 | v3.0.13 | JWT/JWKS operations |
| golangci-lint | latest | Lint validation |
| DEX | v2.45.1 | E2E OIDC provider |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| PR #1018 | Code | Merged | InteractiveReadiness, SSAR check | N/A (merged) |
| lestrrat-go/jwx/v3 | Library | Available (v3.0.13) | JWT operations unavailable | N/A (PoC validated) |
| MockJWKSServer | Test infra | Available (PoC) | Cannot test JWT validation | N/A (exists) |

### 11.2 Execution Order (TDD Phases with Checkpoints)

#### Phase 0.5: Documentation Updates
- Update DD-AUTH-MCP-001 to v2.0
- Update #1009 acceptance criteria

#### Phase 1-RED: Config Types (Failing Tests)
Write failing tests for `JWTProviderConfig`, `ClaimMappings`, validation rules.
Tests: UT-KA-1009-025..031

#### Phase 1-GREEN: Config Types (Minimal Implementation)
Implement `JWTProviderConfig`, `ClaimMappings` structs and validation rules to pass tests.

#### Phase 1-REFACTOR: Config Types (Code Quality)
**100-Go-Mistakes validation checklist**:
- [ ] #1: No variable shadowing in validation logic
- [ ] #2: No unnecessary nesting; happy path aligned left
- [ ] #5: No interface pollution — concrete types only for config
- [ ] #8: No `any` usage — all fields have specific types
- [ ] #11: Functional options pattern used where appropriate
- [ ] #45: No ignored errors in config validation
- [ ] #48: Error wrapping with %w for context
- [ ] #53: No panics in validation — return errors

**Build validation**: `go build ./...`

#### **CHECKPOINT 1: Config Types Audit**
- [ ] Adversarial: malformed YAML, empty strings, duplicate issuers, excessively long URLs
- [ ] Security: no secret values (API keys, credentials) in config struct
- [ ] Completeness: all required fields validated, optional fields have safe defaults
- [ ] Forward-compat: struct supports adding fields without breaking existing configs

---

#### Phase 2-RED: JWTAuthenticator (Failing Tests)
Write failing tests for JWKS validation, claim extraction, error types, multi-issuer routing.
Tests: UT-KA-1009-001..014

#### Phase 2-GREEN: JWTAuthenticator (Minimal Implementation)
Implement `JWTAuthenticator` with `ValidateToken`, `ValidateTokenFull`, issuer routing, JWKS cache.

#### Phase 2-REFACTOR: JWTAuthenticator (Code Quality)
**100-Go-Mistakes validation checklist**:
- [ ] #1: No variable shadowing in token parsing
- [ ] #2: Happy path left-aligned in ValidateTokenFull
- [ ] #6: Authenticator interface on consumer side (pkg/shared/auth)
- [ ] #7: Return concrete `UserInfo`, not interface
- [ ] #22: Nil vs empty slice distinction for Groups
- [ ] #27: No unnecessary map iteration — direct key lookup
- [ ] #45: All errors from jwx library checked
- [ ] #48: Errors wrapped with descriptive context (%w)
- [ ] #49: Sentinel errors (ErrTokenInvalid, ErrIssuerNotFound) used correctly
- [ ] #53: No panics — all error paths return errors
- [ ] #56: No unbounded goroutines from JWKS cache
- [ ] #61: No HTTP body leaks in JWKS client
- [ ] #73: No goroutine leak in JWKS pre-warm
- [ ] #77: Context propagation for cancellation
- [ ] #78: No context.Background() misuse — accept context from caller
- [ ] #83: sync.Once for initialization if needed
- [ ] #91: No unsafe type assertions — use ok pattern

**Build validation**: `go build ./...`

#### **CHECKPOINT 2: JWT Security Audit**
- [ ] **Token replay**: Can a valid JWT be replayed? (Accepted risk for v1.5, documented)
- [ ] **alg confusion**: Can alg:none or HS256 bypass RS256 verification? (Test UT-KA-1009-008)
- [ ] **Key confusion**: Can a token from issuer-A be verified by issuer-B's JWKS? (Test UT-KA-1009-013)
- [ ] **Claim injection**: Can a JWT with manipulated claims (extra groups) pass? (Only verified claims used)
- [ ] **JWKS poisoning**: Can a compromised JWKS endpoint inject keys? (HTTPS-only in prod; TLS CA validation)
- [ ] **Timing attacks**: Are token comparisons timing-safe? (Not applicable — cryptographic signature verification)
- [ ] **Empty claims**: What happens when username claim is empty? (Test UT-KA-1009-011)
- [ ] **Oversized JWT**: What happens with a 1MB JWT? (lestrrat-go/jwx handles; verify no OOM)
- [ ] **Concurrent access**: Is JWTAuthenticator goroutine-safe? (JWKS cache is; verify struct fields)

---

#### Phase 3-RED: CompositeAuthenticator (Failing Tests)
Write failing tests for token routing, fail-closed semantics, fallback behavior.
Tests: UT-KA-1009-015..021

#### Phase 3-GREEN: CompositeAuthenticator (Minimal Implementation)
Implement `CompositeAuthenticator` with `isJWT()` detection, routing, error handling.

#### Phase 3-REFACTOR: CompositeAuthenticator (Code Quality)
**100-Go-Mistakes validation checklist**:
- [ ] #1: No variable shadowing in routing logic
- [ ] #2: Early returns for non-JWT tokens
- [ ] #5: No interface pollution — minimal interface (Authenticator)
- [ ] #45: Error from JWTAuthenticator checked correctly (errors.Is for sentinel)
- [ ] #48: Error context preserved through routing
- [ ] #49: Correct use of errors.Is for ErrIssuerNotFound distinction
- [ ] #53: No panics in routing logic
- [ ] #77: Context forwarded to both authenticators
- [ ] #91: No unsafe type assertions

**Build validation**: `go build ./...`

#### **CHECKPOINT 3: Auth Pipeline Security Audit**
- [ ] **Cross-path leak**: Can a JWT error fall through to K8s TokenReview? (Only ErrIssuerNotFound allows fallback)
- [ ] **Pattern A regression**: Do existing K8s SA tokens still work? (Test UT-KA-1009-018)
- [ ] **Race condition**: Is CompositeAuthenticator safe under concurrent requests? (Stateless — yes)
- [ ] **Error type safety**: Are sentinel errors compared with errors.Is, not == ? (Verify in code)
- [ ] **Header stripping**: Are Impersonate-* headers still stripped before reaching CompositeAuthenticator? (Existing middleware)
- [ ] **Token shape detection**: Can `isJWT()` false-positive on K8s tokens? (K8s SA tokens are opaque/base64, not 3-dot JWT)
- [ ] **Nil safety**: What if JWTAuthenticator is nil? (CompositeAuthenticator requires non-nil at construction)

---

#### Phase 4-RED: ProviderType Propagation (Failing Tests)
Write failing tests for ProviderType in UserInfo, MCP bridge, middleware audit logging.
Tests: UT-KA-1009-022..024

#### Phase 4-GREEN: ProviderType Propagation (Minimal Implementation)
Add ProviderType field to UserInfo (shared + MCP), update K8sAuthenticator, update middleware logging.

#### Phase 4-REFACTOR: ProviderType Propagation (Code Quality)
**100-Go-Mistakes validation checklist**:
- [ ] #1: No variable shadowing in middleware handler
- [ ] #10: No unintended field exposure through type embedding
- [ ] #22: ProviderType empty string vs zero value handled consistently
- [ ] #45: All error paths still checked after middleware changes

**Build validation**: `go build ./...`

---

#### Phase 5-RED: Main.go Wiring (Failing Integration Tests)
Write failing integration tests for CompositeAuthenticator construction, JWKS pre-warm, mixed traffic.
Tests: IT-KA-1009-001..009

#### Phase 5-GREEN: Main.go Wiring (Minimal Implementation)
Wire CompositeAuthenticator in `newAuthMiddleware()`, add JWKS pre-warm at startup, handle soft-disable.

#### Phase 5-REFACTOR: Main.go Wiring (Code Quality)
**100-Go-Mistakes validation checklist**:
- [ ] #3: No init() misuse — startup logic in explicit functions
- [ ] #56: JWKS pre-warm goroutine bounded with timeout
- [ ] #61: HTTP client for JWKS uses response body close
- [ ] #73: No goroutine leak in pre-warm error path
- [ ] #77: Context with timeout for JWKS pre-warm
- [ ] #78: Startup context not leaked to request handlers

**Build validation**: `go build ./...`

#### **CHECKPOINT 4: Full Pipeline Audit**
- [ ] **Wiring verification**: Every new code path has an integration test from HTTP entry to context injection
- [ ] **Build**: `go build ./...` passes
- [ ] **Lint**: `golangci-lint run --timeout=5m` passes
- [ ] **Existing tests**: All existing auth tests still pass
- [ ] **Adversarial wiring**: What if config has jwtProviders but interactive.enabled=false? (CompositeAuthenticator not constructed)
- [ ] **Graceful degradation**: JWKS pre-warm fails → Pattern A works, Pattern B returns 401 with clear error
- [ ] **Startup timing**: JWKS pre-warm doesn't block server startup beyond 10s timeout

---

#### Phase 6-RED: Helm Chart (Failing Template Tests)
Write failing Helm template tests for jwtProviders ConfigMap rendering, conditional ClusterRoleBinding.
Tests: IT-KA-1009-009 (deferred to v1.6)

#### Phase 6-GREEN: Helm Chart (Minimal Implementation)
Add jwtProviders to ConfigMap template, conditional ClusterRoleBinding for JWT group.

#### Phase 6-REFACTOR: Helm Chart (Quality)
- [ ] Template renders valid YAML
- [ ] ClusterRoleBinding only created when jwtProviders configured
- [ ] Group name from values.yaml propagated correctly
- [ ] No hardcoded values — all configurable via values.yaml

#### **CHECKPOINT 5: Final Comprehensive Audit**
- [ ] **Build**: `go build ./...` clean
- [ ] **Tests**: All unit + integration tests pass
- [ ] **Coverage**: >=80% per tier on new code
- [ ] **Lint**: No new lint errors
- [ ] **Security review**: All 9 risks from Section 3 have mitigating tests passing
- [ ] **Pattern A regression**: Full existing auth test suite passes
- [ ] **Documentation**: DD-AUTH-MCP-001 v2.0 consistent with implementation
- [ ] **Helm**: Template renders correctly with various values permutations
- [ ] **Adversarial scenarios**: All CHECKPOINT findings resolved
- [ ] **100-Go-Mistakes**: All REFACTOR checklists completed

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1009/TEST_PLAN.md` | Strategy and test design |
| Unit test suite: JWT auth | `test/unit/shared/auth/jwt_auth_test.go` | JWT validation tests |
| Unit test suite: Composite auth | `test/unit/shared/auth/composite_auth_test.go` | Routing + fail-closed tests |
| Unit test suite: Config | `test/unit/kubernautagent/config/jwt_config_test.go` | Config validation tests |
| Integration test suite | `test/integration/kubernautagent/auth/` | Middleware pipeline tests |
| Mock JWKS server | `test/shared/auth/mock_jwks_server.go` | Test infrastructure (from PoC) |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests — JWT auth
go test ./test/unit/shared/auth/... -ginkgo.v -ginkgo.focus="UT-KA-1009"

# Unit tests — Config
go test ./test/unit/kubernautagent/config/... -ginkgo.v -ginkgo.focus="UT-KA-1009"

# Integration tests
go test ./test/integration/kubernautagent/auth/... -ginkgo.v -ginkgo.focus="IT-KA-1009"

# Coverage — unit tier
go test ./test/unit/shared/auth/... -coverprofile=coverage-unit.out \
  -coverpkg=github.com/jordigilh/kubernaut/pkg/shared/auth/...
go tool cover -func=coverage-unit.out

# Coverage — integration tier
go test ./test/integration/kubernautagent/auth/... -coverprofile=coverage-int.out \
  -coverpkg=github.com/jordigilh/kubernaut/pkg/shared/auth/...,github.com/jordigilh/kubernaut/cmd/kubernautagent/...
go tool cover -func=coverage-int.out

# Full build validation
go build ./...
golangci-lint run --timeout=5m
```

---

## 14. Wiring Verification (TDD Phase 5)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| JWT token → CompositeAuthenticator → JWTAuthenticator → UserInfo in context | HTTP POST /api/v1/mcp | UserInfo in request context, X-Auth-Request-User header | IT-KA-1009-001 | Pending |
| Opaque token → CompositeAuthenticator → K8sAuthenticator → UserInfo in context | HTTP POST /api/v1/investigate | UserInfo in request context | IT-KA-1009-005 | Pending |
| JWT expired → CompositeAuthenticator → 401 response | HTTP POST /api/v1/mcp | 401 JSON response | IT-KA-1009-002 | Pending |
| JWT valid → SAR denied → 403 response | HTTP POST /api/v1/mcp | 403 JSON response | IT-KA-1009-004 | Pending |
| JWKS pre-warm failure → global disable Pattern B | Server startup | Pattern A works, Pattern B disabled | IT-KA-1009-008 | Deferred (v1.6) |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/shared/auth/k8s_auth_test.go` | UserInfo has Username, Groups | No change required | ProviderType is additive (zero value "" is backward compatible) |
| `test/unit/shared/auth/header_stripping_test.go` | Impersonate-* headers stripped | No change required | Header stripping is unchanged |
| `pkg/shared/auth/mock_auth.go` | MockAuthenticator returns UserInfo | May need ProviderType in ValidUsersFull entries | Phase 4: ProviderType propagation |

---

## 16. REFACTOR Phase: 100 Go Mistakes Validation Checklist

Applicable mistakes to validate during every REFACTOR phase:

### Code Organization (#1-#16)
- [ ] #1: No variable shadowing
- [ ] #2: Unnecessary nesting removed; happy path left-aligned
- [ ] #5: No interface pollution; abstractions discovered not created
- [ ] #6: Interfaces on consumer side
- [ ] #7: Return concrete types, not interfaces
- [ ] #8: No `any` usage unless justified (JSON marshaling)
- [ ] #11: Functional options pattern where appropriate

### Data Types (#17-#28)
- [ ] #21: Slice initialization with known capacity
- [ ] #22: Nil vs empty slice distinction for Groups
- [ ] #23: Check slice length, not nil for emptiness

### Error Handling (#45-#53)
- [ ] #45: No ignored errors
- [ ] #48: Error wrapping with %w for context
- [ ] #49: Sentinel errors used correctly with errors.Is
- [ ] #52: No redundant error wrapping (wrap adds context, not noise)
- [ ] #53: No panics — all paths return errors

### Concurrency (#55-#73)
- [ ] #56: No unbounded goroutines
- [ ] #61: HTTP response bodies always closed
- [ ] #73: No goroutine leaks (context cancellation, cleanup)

### Standard Library (#74-#91)
- [ ] #77: Context propagated correctly
- [ ] #78: No context.Background() misuse in request handlers
- [ ] #83: sync.Once for one-time initialization
- [ ] #91: Type assertions use ok pattern

### Testing (#92-#100)
- [ ] #92: Tests organized by behavior, not by function
- [ ] #93: No time.Sleep — use Eventually
- [ ] #95: httptest used for HTTP testing
- [ ] #97: No hard-coded test ports
- [ ] #100: No flaky tests

---

## 17. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-03 | Initial test plan |
