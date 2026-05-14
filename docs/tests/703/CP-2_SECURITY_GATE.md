# CP-2: Security Gate — Test Case Specifications

**Checkpoint**: CP-2
**Gate Type**: Unit + Integration Tests (CRITICAL)
**Total Checks**: 28
**Merge Criteria**: All 28 tests pass, security-critical file coverage >=90%
**PR**: PR2 (MCP Server Transport + Auth)

---

## Overview

CP-2 is the most critical security gate. It validates that the MCP endpoint authentication, impersonation, and authorization infrastructure cannot be bypassed. All penetration scenarios must pass before any interactive tool code (PR3+) is merged.

---

## Test Environment

- **Unit Package**: `test/unit/kubernautagent/mcp/auth/`
- **Integration Package**: `test/integration/kubernautagent/mcp/`
- **Framework**: Ginkgo/Gomega BDD
- **Key Imports**:
  ```go
  "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/auth"
  "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/middleware"
  "github.com/jordigilh/kubernaut/pkg/shared/auth"
  authenticationv1 "k8s.io/api/authentication/v1"
  authorizationv1 "k8s.io/api/authorization/v1"
  "k8s.io/client-go/kubernetes/fake"
  "net/http"
  "net/http/httptest"
  ```
- **Mocks**:
  - `MockAuthenticator`: Returns configurable TokenReview responses
  - `MockAuthorizer`: Returns configurable SAR responses
  - `fake.NewSimpleClientset()`: K8s fake client for SAR/TokenReview
- **Helpers**:
  - `newMCPRequest(method, path, headers)`: Creates test HTTP requests with MCP content-type
  - `assertRFC7807Error(resp, expectedCode, expectedStatus)`: Validates Problem JSON responses

---

## PEN: Penetration Test Scenarios

### UT-KA-SEC-001: Header injection — Impersonate-User on direct call (Pattern A)

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-01

**Description**: A direct caller (Pattern A) includes a rogue `Impersonate-User` header attempting to elevate to a different identity. The middleware must strip this header before auth processing.

**Preconditions**:
- MCP auth middleware instantiated with header stripping enabled
- MockAuthenticator configured to validate the caller's own Bearer token as `"legitimate-user"`

**Steps**:
1. Create HTTP request with:
   - `Authorization: Bearer <valid-token-for-legitimate-user>`
   - `Impersonate-User: cluster-admin` (injected header)
2. Pass through header-stripping middleware
3. Assert `Impersonate-User` header is absent after middleware
4. Continue through auth handler (Pattern A: identity from TokenReview)
5. Assert resolved effective user is `"legitimate-user"` (from token), NOT `"cluster-admin"`

**Acceptance Criteria**:
- Injected `Impersonate-User` header stripped (not passed through)
- Effective identity is from TokenReview, not from injected header
- No error logged for the stripped header (silent defense)
- Response status 200 (valid request, just stripped)

---

### UT-KA-SEC-002: Header injection — Impersonate-Group on direct call (Pattern A)

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-02

**Description**: A direct caller includes rogue `Impersonate-Group` headers to gain extra group membership.

**Preconditions**:
- Same as UT-KA-SEC-001
- MockAuthenticator returns groups `["team-dev"]` for the token

**Steps**:
1. Create HTTP request with:
   - `Authorization: Bearer <valid-token>` (groups: `["team-dev"]`)
   - `Impersonate-Group: system:masters` (injected)
   - `Impersonate-Group: cluster-admins` (injected)
2. Pass through header-stripping middleware
3. Assert ALL `Impersonate-Group` headers stripped
4. Assert resolved effective groups are `["team-dev"]` (from TokenReview)
5. Assert `"system:masters"` is NOT in effective groups

**Acceptance Criteria**:
- All injected Group headers stripped
- Effective groups from TokenReview only
- Attacker cannot gain `system:masters` membership

---

### UT-KA-SEC-003: Header injection — Impersonate-Extra-* on direct call (Pattern A)

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-03

**Description**: A direct caller includes `Impersonate-Extra-*` headers attempting to inject extra user info.

**Preconditions**:
- Same middleware setup

**Steps**:
1. Create HTTP request with:
   - `Authorization: Bearer <valid-token>`
   - `Impersonate-Extra-Scopes: admin,write`
   - `Impersonate-Extra-Tenant: privileged-tenant`
   - `impersonate-extra-mixed-case: value` (case variation)
2. Pass through header-stripping middleware
3. Assert ALL `Impersonate-Extra-*` headers stripped (case-insensitive prefix match)
4. Assert no extra info in resolved identity

**Acceptance Criteria**:
- Case-insensitive prefix match strips all `Impersonate-Extra-*` variants
- Mixed case (`Impersonate-Extra-`, `impersonate-extra-`, `IMPERSONATE-EXTRA-`) all stripped
- No extra identity info leaks through

---

### UT-KA-SEC-004: Unauthorized delegation — Pattern B without impersonate RBAC

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-04

**Description**: A service account presents valid token + Impersonate-User headers but does NOT have `impersonate` RBAC verb. SAR must reject.

**Preconditions**:
- MockAuthenticator validates SA token as `"system:serviceaccount:default:rogue-sa"`
- MockAuthorizer returns `Denied` for SAR(rogue-sa, impersonate, users)
- Request includes `Impersonate-User: target-user`

**Steps**:
1. Create request with SA Bearer token + `Impersonate-User: target-user`
2. Middleware preserves impersonation headers (Pattern B detection)
3. Auth handler calls `extractEffectiveUser` → detects Pattern B
4. SAR check: `rogue-sa` can `impersonate` `users/target-user` → **Denied**
5. Assert response is 403 with error code `rbac_denied`
6. Assert `target-user` is NOT used as effective identity

**Acceptance Criteria**:
- 403 Forbidden returned
- Error code is `rbac_denied` (not generic 403)
- No impersonation occurs
- Audit event emitted for denied access attempt

---

### UT-KA-SEC-005: Missing Impersonate-User header with Pattern B SA

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-05

**Description**: apifrontend SA token is valid and has impersonate RBAC, but sends `Impersonate-Group` WITHOUT `Impersonate-User`. System must reject (groups without user is undefined).

**Preconditions**:
- MockAuthenticator validates as `"system:serviceaccount:kubernaut:apifrontend"`
- MockAuthorizer allows impersonate for apifrontend SA
- Request has `Impersonate-Group` but NO `Impersonate-User`

**Steps**:
1. Create request with apifrontend SA Bearer + `Impersonate-Group: team-sre` (no User header)
2. Auth handler detects Pattern B attempt (Groups present)
3. Assert request is rejected (400 Bad Request or treated as Pattern A)
4. Assert effective user is apifrontend SA itself (Pattern A fallback), NOT empty string

**Acceptance Criteria**:
- Groups-only impersonation does not create empty-user identity
- Falls back to Pattern A (SA is the user) OR rejects explicitly
- No nil-pointer or empty-user in effective identity

---

### UT-KA-SEC-006: Empty Impersonate-User header value

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-06

**Description**: Request includes `Impersonate-User: ""` (empty string). Must not create a user with empty identity.

**Preconditions**:
- Valid SA token with impersonate RBAC

**Steps**:
1. Create request with `Impersonate-User: ""`
2. Auth handler processes Pattern B
3. Assert empty user is rejected (not used as identity)
4. Assert error response indicating invalid impersonation target

**Acceptance Criteria**:
- Empty string user rejected
- No session created with empty `ActingUser`
- Explicit error (not silent empty identity)

---

### UT-KA-SEC-007: Self-impersonation attempt

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-07

**Description**: SA impersonates itself. While not a privilege escalation, it's nonsensical and should be rejected or treated as Pattern A.

**Preconditions**:
- SA token validates as `"system:serviceaccount:kubernaut:apifrontend"`
- `Impersonate-User: system:serviceaccount:kubernaut:apifrontend` (self)

**Steps**:
1. Create request where SA impersonates its own identity
2. Process through auth handler
3. Assert: treated as Pattern A (self-impersonation is no-op) OR rejected with clear error

**Acceptance Criteria**:
- No crash or undefined behavior
- Either: falls through to Pattern A (same result), or explicit rejection
- Identity is correct regardless of path taken

---

### UT-KA-SEC-008: No authentication (missing Bearer token)

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-08

**Description**: Request arrives with no `Authorization` header at all. Must be rejected immediately.

**Preconditions**:
- MCP auth middleware active

**Steps**:
1. Create HTTP request to MCP endpoint with no `Authorization` header
2. Assert response is 401
3. Assert error code is `auth_required`
4. Assert response body is RFC 7807 Problem JSON
5. Assert no session created, no downstream processing

**Acceptance Criteria**:
- 401 Unauthorized
- RFC 7807 Problem JSON body with `type`, `title`, `status`, `detail`
- Error code: `auth_required`
- Human message: "Authentication required"

---

### UT-KA-SEC-009: Expired Bearer token

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-09

**Description**: Request has `Authorization: Bearer <expired-token>`. TokenReview must reject.

**Preconditions**:
- MockAuthenticator configured to return `Authenticated: false` for the expired token

**Steps**:
1. Create request with `Authorization: Bearer expired-token-12345`
2. MockAuthenticator returns `TokenReview{Status: {Authenticated: false}}`
3. Assert response is 401
4. Assert error code is `auth_failed`
5. Assert message does NOT reveal why token failed (no "token expired" leak)

**Acceptance Criteria**:
- 401 Unauthorized
- Error code: `auth_failed`
- Generic message: "Token validation failed" (no specific reason)
- No information disclosure about token state

---

### UT-KA-SEC-010: Forged Bearer token (valid format, unknown to API server)

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Penetration
**Check ID**: PEN-10

**Description**: Request has a well-formed but forged Bearer token that the K8s API server doesn't recognize.

**Preconditions**:
- MockAuthenticator returns error for unknown token

**Steps**:
1. Create request with `Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.forged-payload.signature`
2. MockAuthenticator returns error (API server doesn't recognize token)
3. Assert response is 401
4. Assert error code is `auth_failed`
5. Assert no partial identity created

**Acceptance Criteria**:
- 401 Unauthorized (not 500 Internal Server Error)
- Graceful handling of TokenReview error
- No partial session state

---

### UT-KA-SEC-011: Feature gate off returns 404 (not 403)

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-008
**Type**: Unit
**Category**: Security — Feature Gate
**Check ID**: PEN-11

**Description**: When `interactive.enabled: false`, the MCP endpoint handler is not registered. Requests get 404, revealing no information about the endpoint's existence.

**Preconditions**:
- KA config with `Interactive.Enabled = false`
- HTTP router configured

**Steps**:
1. Configure KA with `interactive.enabled: false`
2. Start HTTP server with routes
3. Send request to `/api/v1/mcp`
4. Assert response is 404 Not Found (not 403 Forbidden)
5. Assert no MCP-related headers in response
6. Assert body does not mention "interactive" or "MCP"

**Acceptance Criteria**:
- 404 (endpoint doesn't exist in router)
- No information leakage about disabled feature
- Not 403 (would reveal endpoint exists but is forbidden)

---

### UT-KA-SEC-012: Auth middleware nil guard (startup validation)

**BR**: BR-INTERACTIVE-008
**Type**: Unit
**Category**: Security — Defensive
**Check ID**: PEN-12

**Description**: If the MCP handler is started with nil authenticator/authorizer (misconfiguration), it must refuse to start rather than running unauthenticated.

**Preconditions**:
- MCP handler constructor available

**Steps**:
1. Attempt to create MCP handler with `authenticator = nil`
2. Assert error is returned (not nil handler)
3. Attempt to create MCP handler with `authorizer = nil`
4. Assert error is returned
5. Attempt to create with both nil
6. Assert error is returned
7. Verify the error message indicates configuration issue

**Acceptance Criteria**:
- Constructor returns error when auth deps are nil
- Never returns a handler that would accept unauthenticated requests
- Error message aids debugging: "authenticator must not be nil"

---

### UT-KA-SEC-013: ImpersonationConfig includes both UserName and Groups

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Property
**Check ID**: PEN-14

**Description**: When creating `rest.ImpersonationConfig` for K8s API calls, both UserName AND Groups must be set (not just UserName). Missing groups would grant different RBAC than intended.

**Preconditions**:
- Function `buildImpersonationConfig(userInfo)` exists
- `UserInfo` has Username and Groups

**Steps**:
1. Create `UserInfo{Username: "user-a@corp", Groups: ["team-sre", "system:authenticated"]}`
2. Call `buildImpersonationConfig(userInfo)`
3. Assert `config.UserName == "user-a@corp"`
4. Assert `config.Groups` contains both groups
5. Test with empty groups: `UserInfo{Username: "user-b", Groups: []}`
6. Assert config still has UserName set
7. Assert Groups is empty slice (not nil) — K8s API differentiates nil vs empty

**Acceptance Criteria**:
- Both UserName and Groups always set in ImpersonationConfig
- Groups never accidentally nil (would inherit caller's groups)
- Empty groups is explicit empty slice `[]string{}`

---

## PROP: Security Properties

### UT-KA-SEC-014: All auth failures emit audit event

**BR**: BR-INTERACTIVE-003
**Type**: Unit
**Category**: Security — Property
**Check ID**: PROP-01

**Description**: Every authentication/authorization failure emits an audit event for SOC2 compliance. No silent rejections.

**Preconditions**:
- Mock audit store capturing emitted events
- Auth middleware with audit integration

**Steps**:
1. Trigger 401 (no auth) → verify audit event emitted with `EventTypeAuthDenied`
2. Trigger 401 (bad token) → verify audit event with failure reason
3. Trigger 403 (no RBAC) → verify audit event with subject and resource
4. Trigger 429 (rate limit) → verify audit event with client identifier
5. Assert all events have: timestamp, source IP (if available), error code

**Acceptance Criteria**:
- Every rejection (401, 403, 429) produces exactly one audit event
- Event includes: error code, timestamp, client identity (if known)
- No silent rejections (audit store has N events for N rejections)

---

### UT-KA-SEC-015: Middleware ordering is stripping → auth → SAR → handler

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Property
**Check ID**: PROP-02

**Description**: Middleware chain must execute in correct order. Header stripping MUST precede auth to prevent injected headers from reaching auth logic.

**Preconditions**:
- Middleware chain configured
- Test middleware that records execution order

**Steps**:
1. Insert order-recording test middleware at each stage
2. Send a request through the full chain
3. Assert execution order: `[strip_headers, authenticate, authorize, handler]`
4. Verify no path allows skipping strip_headers

**Acceptance Criteria**:
- Strip headers always first
- No code path bypasses stripping
- Order is deterministic (not random middleware map)

---

### UT-KA-SEC-016: Pattern B preserved headers only read after SAR success

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Property
**Check ID**: PROP-03

**Description**: In Pattern B, the preserved copy of impersonation headers is only accessed AFTER SAR confirms the caller has impersonate RBAC. If SAR fails, preserved headers are never read.

**Preconditions**:
- `extractEffectiveUser` function with SAR check
- Mock that tracks whether preserved headers were accessed

**Steps**:
1. Create request with Pattern B setup (SA + Impersonate headers)
2. Configure SAR to deny
3. Call `extractEffectiveUser`
4. Assert: SAR check executed
5. Assert: preserved headers NOT accessed (SAR failed first)
6. Configure SAR to allow
7. Call `extractEffectiveUser`
8. Assert: preserved headers accessed after SAR success

**Acceptance Criteria**:
- SAR failure → preserved headers never read (fail-closed)
- SAR success → preserved headers read for identity extraction
- No TOCTOU between SAR check and header read

---

### UT-KA-SEC-017: Constant-time comparison for sensitive operations

**BR**: BR-INTERACTIVE-002
**Type**: Unit
**Category**: Security — Property
**Check ID**: PROP-04

**Description**: Token comparison and session ID validation use constant-time operations to prevent timing attacks.

**Preconditions**:
- Session lookup and token validation code available

**Steps**:
1. Verify session ID comparison uses `subtle.ConstantTimeCompare` or equivalent
2. Verify no early-return on partial match in token validation
3. Run timing test: compare valid vs invalid session IDs, assert no statistically significant timing difference (10k iterations, p<0.01)

**Acceptance Criteria**:
- `crypto/subtle` used for sensitive comparisons
- No early-return optimization on security paths
- Timing differential < 1μs between valid/invalid inputs

---

### UT-KA-SEC-018: Session ID is crypto-random (not sequential)

**BR**: BR-INTERACTIVE-004
**Type**: Unit
**Category**: Security — Property
**Check ID**: PROP-05

**Description**: Session IDs are generated using `crypto/rand`, not predictable sequences. An attacker cannot guess session IDs.

**Preconditions**:
- Session ID generation function available

**Steps**:
1. Generate 1000 session IDs
2. Assert all are unique (no collisions in 1000)
3. Assert length >= 16 bytes of entropy (base64 encoded ~22+ chars)
4. Assert no sequential pattern (ID[n+1] is not ID[n]+1)
5. Verify source code uses `crypto/rand` (not `math/rand`)

**Acceptance Criteria**:
- Uniqueness: 0 collisions in 1000 generations
- Entropy: >=128 bits (16 bytes)
- Source: `crypto/rand.Read` (not `math/rand`)
- Format: URL-safe (base64url or hex)

---

## QE-INT: Integration Security Checks

### IT-KA-SEC-001: Rate limit bypass via MCP rapid requests

**BR**: BR-INTERACTIVE-002
**Type**: Integration
**Category**: Security — Rate Limiting
**Check ID**: PEN-13

**Description**: Rapid MCP tool calls cannot bypass rate limiting. The rate limiter applies to the MCP endpoint consistently.

**Preconditions**:
- Full MCP server running via `httptest.NewServer`
- Rate limiter configured at 10 req/s
- Valid auth token available

**Steps**:
1. Start MCP httptest server with rate limiter (10 req/s)
2. Send 20 requests in rapid succession (within 1 second)
3. Assert first 10 requests succeed (200)
4. Assert remaining requests get 429
5. Assert 429 response includes `Retry-After` header
6. Wait for retry-after duration
7. Send another request, assert it succeeds (200)

**Acceptance Criteria**:
- Rate limit enforced at configured threshold
- 429 with `Retry-After` header present
- Recovery after wait period
- Per-user/per-IP rate limiting (not global)

---

### IT-KA-SEC-002: Full Pattern A flow with real httptest server

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-002
**Type**: Integration
**Category**: Security — Integration
**Check ID**: QE-INT-01

**Description**: Complete Pattern A authentication flow through real HTTP stack (not mocked middleware).

**Preconditions**:
- httptest server running full MCP handler chain
- Fake K8s clientset configured with TokenReview behavior
- Valid SA token that passes TokenReview

**Steps**:
1. Start httptest server with full middleware chain
2. Send MCP initialize request with `Authorization: Bearer <valid-sa-token>`
3. Assert: TokenReview called, returns authenticated
4. Assert: response is 200 with MCP session ID
5. Assert: effective user in session matches TokenReview result
6. Verify audit event emitted for successful auth

**Acceptance Criteria**:
- Full HTTP round-trip succeeds
- Session created with correct identity
- Audit event emitted
- No middleware skipped

---

### IT-KA-SEC-003: Full Pattern B flow with SAR verification

**BR**: BR-INTERACTIVE-001, BR-INTERACTIVE-002
**Type**: Integration
**Category**: Security — Integration
**Check ID**: QE-INT-02

**Description**: Complete Pattern B delegation flow: apifrontend SA impersonates user, SAR verified, effective user is impersonated identity.

**Preconditions**:
- httptest server with full chain
- Fake K8s clientset: TokenReview accepts apifrontend SA, SAR allows impersonate
- Valid apifrontend SA token

**Steps**:
1. Create request with:
   - `Authorization: Bearer <apifrontend-sa-token>`
   - `Impersonate-User: user-a@corp`
   - `Impersonate-Group: team-sre`
2. Send to httptest server
3. Assert: TokenReview called for SA token → authenticated as apifrontend SA
4. Assert: SAR called → apifrontend-sa can impersonate users
5. Assert: effective user is `"user-a@corp"` with groups `["team-sre"]`
6. Assert: session `ActingUser` is `"user-a@corp"` (not apifrontend-sa)

**Acceptance Criteria**:
- Both TokenReview and SAR called
- Effective user is impersonated identity
- Session attributed to human user
- Audit trail shows delegation

---

### IT-KA-SEC-004: Pattern B rejected when SAR denies

**BR**: BR-INTERACTIVE-002
**Type**: Integration
**Category**: Security — Integration
**Check ID**: QE-INT-03

**Description**: When SAR denies impersonation RBAC, the request is fully rejected even though the SA token is valid.

**Preconditions**:
- httptest server
- Fake K8s: TokenReview accepts SA, SAR DENIES impersonate

**Steps**:
1. Create Pattern B request (SA token + Impersonate-User)
2. Configure SAR to deny: `{Allowed: false, Reason: "no impersonate clusterrole"}`
3. Send request
4. Assert: 403 Forbidden
5. Assert: error code `rbac_denied`
6. Assert: no session created
7. Assert: audit event emitted for denied impersonation

**Acceptance Criteria**:
- 403 (not 200 or 401)
- Session NOT created
- Audit event captures attempted impersonation target
- SAR denial reason not leaked to client

---

### IT-KA-SEC-005: MCP session isolation between concurrent users

**BR**: BR-INTERACTIVE-002
**Type**: Integration
**Category**: Security — Integration
**Check ID**: QE-INT-04

**Description**: Two authenticated users connecting simultaneously get isolated sessions. User A cannot access User B's session data.

**Preconditions**:
- httptest server
- Two valid tokens (user-a, user-b)
- Both authenticated via Pattern A

**Steps**:
1. User A connects → gets session-A
2. User B connects → gets session-B
3. Assert session-A.ActingUser == "user-a"
4. Assert session-B.ActingUser == "user-b"
5. User A attempts to access session-B's endpoint → rejected
6. User B attempts to access session-A's endpoint → rejected

**Acceptance Criteria**:
- Sessions fully isolated
- Cross-session access rejected (not just hidden)
- Session IDs not sequential (can't guess other session)

---

### IT-KA-SEC-006: MCP disconnect cleans up auth state

**BR**: BR-INTERACTIVE-002, BR-INTERACTIVE-005
**Type**: Integration
**Category**: Security — Integration
**Check ID**: QE-INT-05

**Description**: When an MCP client disconnects, all auth state (session, Lease, impersonation config) is cleaned up. Reconnection requires fresh authentication.

**Preconditions**:
- httptest server with active session
- K8s Lease created for session

**Steps**:
1. Establish authenticated MCP session (user-a)
2. Assert: session active, Lease held
3. Simulate client disconnect (close TCP connection)
4. Wait for cleanup (Lease expiry or explicit release)
5. Assert: session removed from store
6. Assert: Lease released (or expired)
7. Send new request with same token
8. Assert: requires fresh session initialization (not reusing old state)

**Acceptance Criteria**:
- Session cleaned up on disconnect
- Lease released (not orphaned)
- No stale auth state persists
- Reconnection starts fresh (no session resumption without re-auth)
