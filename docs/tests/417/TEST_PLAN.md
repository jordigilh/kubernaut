# Test Plan: Custom Authentication Headers for LLM Proxy Endpoints (#417)

> **Template**: IEEE 829-2008 + Kubernaut Hybrid v2.0

**Test Plan Identifier**: TP-417-v1.0
**Feature**: KAPI injects configurable custom HTTP headers (Authorization, API keys, sidecar-rotated JWTs) into all outbound LLM API requests via an `http.RoundTripper` wrapper, enabling enterprise proxy/gateway authentication without coupling KAPI to any specific identity provider or token lifecycle.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that KAPI (#433) correctly injects custom authentication headers into all outbound LLM requests. Enterprise deployments route LLM traffic through API gateways (Azure APIM, Kong, Apigee, AWS API Gateway) or SSO-fronted proxies that require authentication headers. KAPI must support arbitrary headers from three value sources (`secretKeyRef`, `filePath`, `value`) without hardcoding any auth scheme.

The test plan also validates that sensitive header values are never leaked into logs, metrics, or error messages (DD-HAPI-019-003, G4: Credential Scrubbing). Header values are injected at the `http.Transport` layer below prompt assembly, so they should never appear in LLM-bound content — but a defense-in-depth test validates this assumption.

### 1.2 Objectives

1. **Header injection correctness**: All configured headers are present in every outbound LLM request, with the correct values from all three sources
2. **Token rotation support**: `filePath`-sourced headers reflect the current file content on each request, supporting sidecar-rotated tokens without restart
3. **Backward compatibility**: KAPI without custom headers configured behaves identically to a default installation
4. **Credential safety**: Sensitive header values from `secretKeyRef` and `filePath` are redacted from all logs, metrics labels, and LLM-bound prompt content
5. **End-to-end verification**: Mock LLM (#570) receives and records the injected headers, verifiable via the verification API

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kapi/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kapi/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/kapi/llm/transport/`, `pkg/kapi/config/` |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on handler/client wiring |
| Existing test regressions | 0 | Existing KAPI test suites pass with auth headers feature merged |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-433: Go Language Migration (parent BR)
- DD-HAPI-019-001: Framework Selection — LangChainGo adapter
- DD-HAPI-019-003 (G4): Credential Scrubbing — Go reimplementation of DD-005 patterns for log/error redaction
- Issue #417: Support custom authentication headers for LLM proxy endpoints
- Issue #570: Mock LLM auth header passthrough and verification (companion)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Issue #433: KAPI Go rewrite (parent)
- Issue #531: Mock LLM Go rewrite (provides test verification infrastructure)
- Issue #493: TLS for all inter-pod HTTP communication (transport layer, orthogonal)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | **`filePath` race condition** — sidecar writes token while KAPI reads it | Partial read produces garbled token; LLM gateway rejects request | Medium | UT-KAPI-417-007, IT-KAPI-417-005 | Atomic file read (read into buffer, not streaming). UT-KAPI-417-007 tests concurrent read-during-write. |
| R2 | **Secret not mounted** — `secretKeyRef` references a Secret that doesn't exist in the Pod | KAPI fails to start or sends empty header value | High | UT-KAPI-417-004, UT-KAPI-417-010 | Fail-fast at startup: validate all `secretKeyRef` sources resolve to non-empty values. UT-KAPI-417-010 tests startup validation. |
| R3 | **Credential leakage via logs** — header value appears in HTTP debug logs or error messages | Security incident — API keys exposed in cluster logs | High | UT-KAPI-417-009 | RoundTripper redacts header values from all log output. UT-KAPI-417-009 validates no sensitive values in structured log fields. |
| R4 | **Mock LLM #570 not landed** — auth header verification API unavailable | IT-KAPI-417-006 (end-to-end verification) blocked | Medium | IT-KAPI-417-006 | Use `httptest.Server` with custom handler that captures headers as a fallback. Migrate to Mock LLM verification API when #570 lands. |
| R5 | **LangChainGo transport override** — LangChainGo replaces or wraps the custom `http.Transport` | Headers silently dropped; LLM gateway rejects unauthenticated requests | Medium | IT-KAPI-417-001, IT-KAPI-417-002 | Integration tests verify headers arrive at a real HTTP server, not just that the RoundTripper is configured. |
| R6 | **Header name collision** — custom header overwrites a standard header (Content-Type, Accept) | Malformed requests to LLM provider | Low | UT-KAPI-417-011 | Validate header names against a deny list at config parse time. UT-KAPI-417-011 tests rejection of reserved names. |
| R7 | **`filePath` missing at request time** — sidecar hasn't started or file deleted between requests | Request fails (blocking remediation) or proceeds without auth (security issue) | Medium | UT-KAPI-417-014 | Return error from `RoundTrip` with clear message identifying the missing file path. |
| R8 | **Request mutation violates RoundTripper contract** — implementation modifies original `*http.Request` instead of cloning | Data race: concurrent callers see each other's injected headers | Medium | UT-KAPI-417-016 | Clone request via `req.Clone(req.Context())` before adding headers. UT-KAPI-417-016 validates original is unchanged. |

### 3.1 Risk-to-Test Traceability

| Risk | Primary Tests | Secondary Tests |
|------|--------------|-----------------|
| R1 (filePath race) | UT-KAPI-417-007 | IT-KAPI-417-005 |
| R2 (secret not mounted) | UT-KAPI-417-010 | UT-KAPI-417-004 |
| R3 (credential leakage) | UT-KAPI-417-009 | IT-KAPI-417-004 |
| R5 (transport override) | IT-KAPI-417-001 | IT-KAPI-417-002 |
| R7 (filePath missing at runtime) | UT-KAPI-417-014 | UT-KAPI-417-015 |
| R8 (request mutation) | UT-KAPI-417-016 | — |

---

## 4. Scope

### 4.1 Features to be Tested

- **RoundTripper wrapper** (`pkg/kapi/llm/transport/auth_headers.go`): Custom `http.RoundTripper` that injects configured headers into every outbound request before delegating to the inner transport
- **Header value resolver** (`pkg/kapi/llm/transport/resolver.go`): Resolves header values from three sources — `value` (literal), `secretKeyRef` (env var or mounted file), `filePath` (re-read per request)
- **Config parser** (`pkg/kapi/config/headers.go`): Parses Helm-provided header configuration into typed header definitions with validation
- **Credential scrubbing** (`pkg/kapi/llm/transport/scrub.go`): Ensures sensitive header values are redacted from logs, error messages, and any LLM-bound content
- **Startup validation** (`cmd/kapi/main.go`): Fail-fast validation that all configured header sources are resolvable at startup

### 4.2 Features Not to be Tested

- **Token acquisition/refresh**: KAPI does NOT handle OAuth token lifecycle — that is the sidecar's responsibility. The `filePath` source simply reads whatever the sidecar wrote.
- **Helm chart templating**: Helm `values.yaml` → ConfigMap/Secret rendering is tested by Helm's own test framework, not by Go unit tests.
- **TLS transport** (#493): Orthogonal to auth headers. TLS encrypts the transport; auth headers authenticate the request. Tested separately.
- **LLM provider authentication logic**: KAPI doesn't validate tokens against the provider. It injects headers; the provider decides if they're valid.
- **Mock LLM header recording internals**: Tested by TP-531 (Mock LLM test plan). This plan only verifies that Mock LLM *receives* the headers.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| RoundTripper pattern (not middleware) | Headers must be injected at the `http.Transport` layer to work with any LangChainGo backend (OpenAI, Ollama, custom). Middleware would only work for KAPI's own HTTP server, not outbound LLM client calls. |
| `filePath` re-read on every request (no cache) | Sidecar-rotated tokens have unpredictable rotation schedules. Caching risks serving an expired token. File reads are <1ms for small token files. |
| Fail-fast on missing `secretKeyRef` | A misconfigured Secret is a deployment error, not a runtime error. Better to fail at startup than to silently send unauthenticated requests. |
| Reserved header deny list | Prevents accidental override of `Content-Type`, `Accept`, `User-Agent`, `Host` which would break the LLM request. |
| Sensitive sources (`secretKeyRef`, `filePath`) redacted; `value` source not redacted | `value` is for non-sensitive data (tenant IDs, correlation headers) explicitly placed in plaintext config. Only secret-sourced and file-sourced values need redaction. |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (RoundTripper logic, resolver, config parser, scrubbing, validation)
- **Integration**: >=80% of integration-testable code (full HTTP round trip, file I/O, startup wiring, Mock LLM verification)

### 5.2 Two-Tier Minimum

Every acceptance criterion from #417 is covered by at least 2 tiers:
- **Unit tests**: Validate resolver logic, config parsing, scrubbing, validation in isolation
- **Integration tests**: Validate headers arrive at a real HTTP server and are verifiable via the Mock LLM API

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**:
1. **Enterprise gateways authenticate KAPI requests** — correct headers arrive at the LLM endpoint
2. **Sidecar token rotation works without restart** — `filePath` headers reflect current file content
3. **API keys never leak** — logs and metrics contain redacted values, not plaintext secrets
4. **Operators can configure any header** — arbitrary names and values from any of the three sources

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. Per-tier coverage >=80%
4. No regressions in existing KAPI test suites
5. All three value sources (`secretKeyRef`, `filePath`, `value`) inject correct header values in integration tests
6. No sensitive header values appear in any log output (verified by UT-KAPI-417-009)

**FAIL** — any of the following:

1. Any P0 test fails
2. Per-tier coverage below 80%
3. Sensitive header value found in log output
4. `filePath` source does not reflect updated file content (stale token)
5. Original `*http.Request` mutated by `RoundTrip` (violates transport contract, risks data races)

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- **KAPI #433 base not buildable**: Auth headers depend on the KAPI LLM client infrastructure. If the base HTTP client isn't implemented, header injection tests are meaningless.
- **Mock LLM #570 not available**: IT-KAPI-417-006 (end-to-end verification) is blocked. Other integration tests can proceed using `httptest.Server`.
- **Build broken**: `go build ./cmd/kapi/...` fails.

**Resume testing when**:

- KAPI base LLM client compiles and can make outbound HTTP calls
- Mock LLM #570 merged → unblock end-to-end verification test
- Build fixed

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kapi/llm/transport/auth_headers.go` | `NewAuthHeadersTransport`, `RoundTrip` | ~50 |
| `pkg/kapi/llm/transport/resolver.go` | `ResolveValue`, `ResolveSecretKeyRef`, `ResolveFilePath`, `ResolveAll` | ~80 |
| `pkg/kapi/llm/transport/scrub.go` | `RedactHeaderValue`, `IsSensitiveSource` | ~30 |
| `pkg/kapi/config/headers.go` | `ParseCustomHeaders`, `ValidateHeaderName`, `ValidateSource` | ~60 |
| **Total unit-testable** | | **~220** |

### 6.2 Integration-Testable Code (I/O, wiring)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kapi/llm/transport/resolver.go` | `ResolveFilePath` (actual file read) | ~20 |
| `pkg/kapi/llm/client.go` | `NewLLMClient` (wires transport with auth headers) | ~30 |
| `cmd/kapi/main.go` | Startup validation of header sources | ~20 |
| **Total integration-testable** | | **~70** |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | KAPI auth headers implementation |
| Dependency: KAPI base client | #433 (In progress) | Must have working `http.Client` with configurable transport |
| Dependency: Mock LLM verification | #570 (Open) | For IT-KAPI-417-006. Fallback: `httptest.Server` |

---

## 7. BR Coverage Matrix

| BR / AC | Description | Priority | Tier | Test ID | Status |
|---------|-------------|----------|------|---------|--------|
| AC-417-01 | Arbitrary key-value headers configurable | P0 | Unit | UT-KAPI-417-001 | Pending |
| AC-417-01 | Arbitrary headers — over HTTP | P0 | Integration | IT-KAPI-417-001 | Pending |
| AC-417-02 | `secretKeyRef` source resolves from env/volume | P0 | Unit | UT-KAPI-417-002..003 | Pending |
| AC-417-02 | `secretKeyRef` — over HTTP | P0 | Integration | IT-KAPI-417-002 | Pending |
| AC-417-03 | `filePath` source re-reads on each request | P0 | Unit | UT-KAPI-417-005..006 | Pending |
| AC-417-03 | `filePath` re-read — file rotation | P0 | Integration | IT-KAPI-417-005 | Pending |
| AC-417-04 | `value` source inlines from config | P0 | Unit | UT-KAPI-417-004 | Pending |
| AC-417-04 | `value` — over HTTP | P0 | Integration | IT-KAPI-417-001 | Pending |
| AC-417-05 | Headers at transport layer (provider-agnostic) | P0 | Integration | IT-KAPI-417-001..002 | Pending |
| AC-417-06 | Sensitive values from Secrets/files only | P0 | Unit | UT-KAPI-417-009 | Pending |
| AC-417-07 | No token lifecycle management | P1 | Unit | UT-KAPI-417-005 | Pending |
| AC-417-08 | Unit tests for RoundTripper (all 3 sources) | P0 | Unit | UT-KAPI-417-001..008 | Pending |
| AC-417-09 | Integration test with Mock LLM | P0 | Integration | IT-KAPI-417-006 | Pending |
| — | Startup validation (fail-fast on missing source) | P1 | Unit | UT-KAPI-417-010 | Pending |
| — | Reserved header name rejection | P1 | Unit | UT-KAPI-417-011 | Pending |
| — | Backward compat (no headers configured) | P0 | Integration | IT-KAPI-417-003 | Pending |
| — | Config parse error handling | P1 | Unit | UT-KAPI-417-012 | Pending |
| — | Concurrent filePath reads thread-safe | P1 | Unit | UT-KAPI-417-007 | Pending |
| — | filePath missing at request time returns clear error | P1 | Unit | UT-KAPI-417-014 | Pending |
| — | filePath empty file content rejected | P1 | Unit | UT-KAPI-417-015 | Pending |
| — | RoundTripper request cloning contract | P0 | Unit | UT-KAPI-417-016 | Pending |
| — | Header values absent from request body (defense-in-depth) | P2 | Unit | UT-KAPI-417-017 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KAPI-417-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `KAPI`
- **ISSUE**: `417`

### Tier 1: Unit Tests

**Testable code scope**: `pkg/kapi/llm/transport/`, `pkg/kapi/config/headers.go`. Target: >=80% of ~220 lines.

#### 8.1.1 RoundTripper — Header Injection

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KAPI-417-001` | RoundTripper injects all configured headers into outbound request before delegating to inner transport | Pending |
| `UT-KAPI-417-002` | `secretKeyRef` source resolves header value from environment variable | Pending |
| `UT-KAPI-417-003` | `secretKeyRef` source resolves header value from mounted file (volume mount simulated as env) | Pending |
| `UT-KAPI-417-004` | `value` source inlines the literal string as header value | Pending |
| `UT-KAPI-417-005` | `filePath` source reads file content as header value (simulated via temp file) | Pending |
| `UT-KAPI-417-006` | `filePath` source returns updated content when file is overwritten between requests (token rotation) | Pending |
| `UT-KAPI-417-007` | Concurrent `filePath` reads from multiple goroutines do not panic or produce garbled values | Pending |
| `UT-KAPI-417-008` | Multiple headers from mixed sources (secretKeyRef + filePath + value) all injected in a single request | Pending |

#### 8.1.2 Credential Safety

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KAPI-417-009` | Sensitive header values (`secretKeyRef`, `filePath` sources) are redacted in log output; `value` source is not redacted | Pending |

#### 8.1.3 Config Validation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KAPI-417-010` | Startup validation fails fast with clear error when `secretKeyRef` env var is empty or unset | Pending |
| `UT-KAPI-417-011` | Config rejects reserved header names (`Content-Type`, `Accept`, `Host`, `User-Agent`) with descriptive error | Pending |
| `UT-KAPI-417-012` | Config rejects malformed header definitions (missing name, missing source, both `value` and `secretKeyRef` set, duplicate header names) | Pending |
| `UT-KAPI-417-013` | Config accepts zero custom headers (empty list) — no-op RoundTripper, no overhead | Pending |

#### 8.1.4 Runtime Safety

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KAPI-417-014` | `filePath` file missing at request time returns clear error identifying the missing path (not a panic or empty value) | Pending |
| `UT-KAPI-417-015` | `filePath` file exists but is empty returns error (empty auth token is worse than no auth) | Pending |
| `UT-KAPI-417-016` | Original `*http.Request` headers unchanged after `RoundTrip` returns — request cloned before mutation (RoundTripper contract) | Pending |
| `UT-KAPI-417-017` | Header values injected at transport layer do NOT appear in the request body (defense-in-depth: headers never leak into LLM prompt content) | Pending |

---

### Tier 2: Integration Tests

**Testable code scope**: Full HTTP round trip through `NewLLMClient` with configured auth headers to a real HTTP server. Target: >=80% of ~70 lines.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KAPI-417-001` | Full round trip: KAPI LLM client with 3 headers (one per source type) sends request to `httptest.Server` — all 3 headers present with correct values | Pending |
| `IT-KAPI-417-002` | Headers injected for both OpenAI and Ollama endpoints (provider-agnostic transport layer) | Pending |
| `IT-KAPI-417-003` | KAPI with zero custom headers configured sends request without any extra headers (backward compat) | Pending |
| `IT-KAPI-417-004` | Error log from a failed LLM request does NOT contain the `Authorization` header value (credential scrubbing in error path) | Pending |
| `IT-KAPI-417-005` | Token file updated between two consecutive requests — second request carries new token value (rotation without restart) | Pending |
| `IT-KAPI-417-006` | End-to-end: KAPI sends request with `Authorization: Bearer test-token` to Mock LLM → `GET /api/test/headers` returns recorded header | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. The full pipeline (signal → AA → KAPI → LLM) is tested by existing E2E suites. Adding auth headers to E2E would require provisioning Secrets and sidecars in Kind, which adds infrastructure cost for marginal coverage beyond unit + integration. Can be added when Helm chart tests (#239) are implemented.

---

## 9. Test Cases

### UT-KAPI-417-001: RoundTripper injects all configured headers

**BR**: AC-417-01, AC-417-05, AC-417-08
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kapi/transport/auth_headers_test.go`

**Preconditions**:
- Three header definitions: `x-api-key` (value: `"test-key"`), `x-tenant-id` (value: `"prod"`), `Authorization` (value: `"Bearer abc123"`)
- A mock inner `http.RoundTripper` that captures the outbound request

**Test Steps**:
1. **Given**: An `AuthHeadersTransport` wrapping a capturing inner transport, configured with 3 headers
2. **When**: `RoundTrip` is called with an `http.Request` to `https://llm.example.com/v1/chat/completions`
3. **Then**: The inner transport receives the request with all 3 headers present

**Expected Results**:
1. `request.Header.Get("x-api-key")` == `"test-key"`
2. `request.Header.Get("x-tenant-id")` == `"prod"`
3. `request.Header.Get("Authorization")` == `"Bearer abc123"`
4. Original request headers (Content-Type, etc.) are preserved

**Acceptance Criteria**:
- **Behavior**: All configured headers are injected before the inner transport sees the request
- **Correctness**: Header values match the configured sources exactly
- **Accuracy**: Pre-existing headers on the request are not removed or modified

**Dependencies**: None (foundational test)

---

### UT-KAPI-417-006: filePath re-read on token rotation

**BR**: AC-417-03
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kapi/transport/resolver_test.go`

**Preconditions**:
- Temporary file containing `"token-v1"`
- Header definition with `filePath` pointing to the temp file

**Test Steps**:
1. **Given**: A `filePath` resolver pointing to a temp file containing `"token-v1"`
2. **When**: `ResolveFilePath()` is called → returns `"token-v1"` → file is overwritten with `"token-v2"` → `ResolveFilePath()` is called again
3. **Then**: Second call returns `"token-v2"`

**Expected Results**:
1. First resolution: `"token-v1"`
2. After file overwrite: `"token-v2"`
3. No caching — file is re-read on every call

**Acceptance Criteria**:
- **Behavior**: Sidecar-rotated token is picked up on the next LLM request without KAPI restart
- **Correctness**: No stale token served after file update
- **Accuracy**: Trailing newlines/whitespace stripped from file content (sidecar may write `token\n`)

**Dependencies**: None

---

### UT-KAPI-417-009: Credential scrubbing in logs

**BR**: AC-417-06, DD-HAPI-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kapi/transport/scrub_test.go`

**Preconditions**:
- Header definitions: `Authorization` (secretKeyRef → `"Bearer secret-key"`), `x-tenant-id` (value → `"prod"`)
- Structured logger capturing output to buffer

**Test Steps**:
1. **Given**: An `AuthHeadersTransport` with one sensitive header (secretKeyRef) and one non-sensitive header (value), logger writing to a buffer
2. **When**: A request is made and the transport logs the header injection event
3. **Then**: Log output contains `Authorization: [REDACTED]` and `x-tenant-id: prod`

**Expected Results**:
1. `"Bearer secret-key"` does NOT appear anywhere in the log buffer
2. `"[REDACTED]"` appears in place of the secret value
3. `"prod"` appears as-is (non-sensitive `value` source)

**Acceptance Criteria**:
- **Behavior**: Operators see that auth headers were injected without seeing the secret values
- **Correctness**: Only `secretKeyRef` and `filePath` sourced values are redacted; `value` source is visible
- **Accuracy**: Redaction applies to all log levels (debug, info, error)

**Dependencies**: None

---

### UT-KAPI-417-010: Startup fail-fast on missing secret

**BR**: AC-417-06, R2
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kapi/config/headers_test.go`

**Preconditions**:
- Header definition with `secretKeyRef` pointing to env var `LLM_API_KEY`
- Env var is NOT set

**Test Steps**:
1. **Given**: A header config with `secretKeyRef: {name: llm-secret, key: api-key}` and the corresponding env var unset
2. **When**: `ValidateHeaderSources()` is called during startup
3. **Then**: Returns an error with a message identifying the missing secret

**Expected Results**:
1. Error returned (not nil)
2. Error message contains the secret name and key for operator debugging
3. Error message does NOT suggest a default value (fail-fast, not fail-soft)

**Acceptance Criteria**:
- **Behavior**: KAPI refuses to start with a misconfigured Secret rather than silently sending unauthenticated requests
- **Correctness**: Error is actionable — operator knows which Secret to fix
- **Accuracy**: Non-secret sources (`value`, existing `filePath`) do not trigger this validation

**Dependencies**: None

---

### IT-KAPI-417-001: Full round trip with all three source types

**BR**: AC-417-01..05, AC-417-08
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kapi/auth_headers_test.go`

**Preconditions**:
- `httptest.Server` that captures all received request headers
- Temp file containing `"jwt-token-xyz"` for `filePath` source
- Env var `TEST_API_KEY=secret-key-123` for `secretKeyRef` source

**Test Steps**:
1. **Given**: A KAPI LLM client configured with:
   - `x-api-key` from `secretKeyRef` (env: `TEST_API_KEY`)
   - `Authorization` from `filePath` (temp file: `"jwt-token-xyz"`)
   - `x-tenant-id` from `value` (`"kubernaut-prod"`)
2. **When**: Client sends a `POST /v1/chat/completions` to the `httptest.Server`
3. **Then**: Server receives all 3 headers with correct values

**Expected Results**:
1. `x-api-key: secret-key-123`
2. `Authorization: jwt-token-xyz`
3. `x-tenant-id: kubernaut-prod`
4. Standard headers (Content-Type: application/json) also present

**Acceptance Criteria**:
- **Behavior**: Enterprise gateway would authenticate this request
- **Correctness**: All three source types resolve correctly over real HTTP
- **Accuracy**: Headers are injected at the transport layer — the `httptest.Server` sees them, proving they're on the wire

**Dependencies**: None (uses `httptest.Server`, no external dependencies)

---

### IT-KAPI-417-005: Token rotation without restart

**BR**: AC-417-03
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kapi/auth_headers_test.go`

**Preconditions**:
- `httptest.Server` capturing headers
- Temp file initially containing `"token-v1"`
- KAPI LLM client with `Authorization` header from `filePath`

**Test Steps**:
1. **Given**: Client configured with `filePath` source pointing to temp file with `"token-v1"`
2. **When**: Client sends request → server captures `Authorization: token-v1` → file overwritten with `"token-v2"` → client sends another request
3. **Then**: Second request carries `Authorization: token-v2`

**Expected Results**:
1. First request: `Authorization: token-v1`
2. Second request (after file update): `Authorization: token-v2`
3. No KAPI restart between requests

**Acceptance Criteria**:
- **Behavior**: Vault Agent / cert-manager sidecar rotates token → KAPI picks it up on next LLM call
- **Correctness**: No stale token served
- **Accuracy**: File is re-read per request, not cached

**Dependencies**: None

---

### IT-KAPI-417-006: End-to-end with Mock LLM verification API

**BR**: AC-417-09, BR-MOCK-006, BR-MOCK-007
**Priority**: P0
**Type**: Integration
**File**: `test/integration/kapi/auth_headers_mockllm_test.go`

**Preconditions**:
- Mock LLM Go server running (httptest.Server or container)
- Mock LLM #570 verification API available
- KAPI LLM client configured with `Authorization: Bearer test-token-e2e`

**Test Steps**:
1. **Given**: Mock LLM server running with auth header recording enabled
2. **When**: KAPI sends `POST /v1/chat/completions` with `Authorization: Bearer test-token-e2e`
3. **Then**: `GET /api/test/headers` on Mock LLM returns the recorded `Authorization` header

**Expected Results**:
1. Mock LLM returns valid chat completion response (request not rejected)
2. `GET /api/test/headers` response includes `Authorization: Bearer test-token-e2e`
3. Request sequence and conversation ID are populated

**Acceptance Criteria**:
- **Behavior**: The acceptance criterion from #417 — "Integration test with mock LLM verifying headers are received" — is satisfied
- **Correctness**: Header value matches what KAPI was configured to send
- **Accuracy**: Mock LLM processes the request normally (auth headers don't interfere with scenario routing)

**Dependencies**: Mock LLM #570 (fallback: `httptest.Server` with custom handler)

---

### UT-KAPI-417-014: filePath missing at request time

**BR**: R7 (filePath missing at runtime)
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kapi/transport/resolver_test.go`

**Preconditions**:
- Header definition with `filePath` pointing to `/tmp/nonexistent-token.txt`
- File does NOT exist on disk

**Test Steps**:
1. **Given**: A `filePath` resolver pointing to a non-existent file path
2. **When**: `ResolveFilePath()` is called during `RoundTrip`
3. **Then**: Returns an error containing the file path and a clear description

**Expected Results**:
1. `RoundTrip` returns a non-nil error
2. Error message contains the file path (`/tmp/nonexistent-token.txt`)
3. Error message indicates the file is missing (not a generic I/O error)
4. No panic, no empty header value sent silently

**Acceptance Criteria**:
- **Behavior**: Missing file at runtime produces an actionable error, not a silent failure
- **Correctness**: Remediation request is blocked rather than sent without authentication

**Dependencies**: None

---

### UT-KAPI-417-015: filePath empty file content rejected

**BR**: R7 (filePath missing at runtime)
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kapi/transport/resolver_test.go`

**Preconditions**:
- Temporary file exists but is empty (0 bytes)
- Header definition with `filePath` pointing to the empty file

**Test Steps**:
1. **Given**: A `filePath` resolver pointing to an empty file
2. **When**: `ResolveFilePath()` is called during `RoundTrip`
3. **Then**: Returns an error indicating the file is empty

**Expected Results**:
1. `RoundTrip` returns a non-nil error
2. Error message indicates the file exists but contains no content
3. An empty string is NOT injected as the header value (empty auth token is worse than no auth)

**Acceptance Criteria**:
- **Behavior**: Empty token file is treated as an error, not a valid value
- **Correctness**: Prevents requests with empty `Authorization: ` headers

**Dependencies**: None

---

### UT-KAPI-417-016: RoundTripper request cloning contract

**BR**: R8 (request mutation)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kapi/transport/auth_headers_test.go`

**Preconditions**:
- An `AuthHeadersTransport` configured with `Authorization: Bearer test`
- An original `http.Request` with no `Authorization` header
- A capturing inner transport that records the forwarded request

**Test Steps**:
1. **Given**: An original request with `Content-Type: application/json` and NO `Authorization` header
2. **When**: `RoundTrip` is called
3. **Then**: The inner transport receives a request with `Authorization: Bearer test`, but the ORIGINAL request still has no `Authorization` header

**Expected Results**:
1. Inner transport sees `Authorization: Bearer test`
2. Original request's `Header` map does NOT contain `Authorization`
3. Original request's `Content-Type` is unchanged
4. Both requests share the same `Body` (clone preserves body reference)

**Acceptance Criteria**:
- **Behavior**: `RoundTrip` does not mutate the caller's `*http.Request` (per `http.RoundTripper` contract)
- **Correctness**: Concurrent callers are safe from header cross-contamination

**Dependencies**: None (foundational safety test)

---

### UT-KAPI-417-017: Header values absent from request body

**BR**: I3 (defense-in-depth)
**Priority**: P2
**Type**: Unit
**File**: `test/unit/kapi/transport/auth_headers_test.go`

**Preconditions**:
- An `AuthHeadersTransport` configured with a sensitive header `Authorization: Bearer secret-token-xyz`
- A JSON request body with a chat completion payload
- A capturing inner transport

**Test Steps**:
1. **Given**: A request body `{"model":"gpt-4","messages":[{"role":"user","content":"analyze pod crash"}]}`
2. **When**: `RoundTrip` is called with the `AuthHeadersTransport`
3. **Then**: The request body forwarded to the inner transport does NOT contain the header value

**Expected Results**:
1. Read the captured request body from the inner transport
2. Body does NOT contain `"secret-token-xyz"` anywhere
3. Body is unchanged from the original (transport layer does not modify body)

**Acceptance Criteria**:
- **Behavior**: Header values never leak into the LLM prompt/request body (defense-in-depth)
- **Correctness**: Transport-layer injection is isolated from content-layer data

**Dependencies**: None

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock `http.RoundTripper` (captures outbound request for header inspection)
- **Test data**: Temporary files for `filePath` tests, env vars for `secretKeyRef` tests
- **Location**: `test/unit/kapi/transport/`, `test/unit/kapi/config/`
- **Anti-patterns avoided**: No `time.Sleep()` — use `Eventually()` for async assertions. No `Skip()`. No direct audit store testing.

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — real HTTP via `httptest.NewServer`
- **Infrastructure**: `httptest.Server` (header capturing), temp files (token rotation), Mock LLM Go server (IT-KAPI-417-006)
- **Location**: `test/integration/kapi/`
- **Anti-patterns avoided**: No `time.Sleep()`. No mocks of business logic. No HTTP endpoint tests for pure logic.

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| KAPI base LLM client (#433) | Code | In progress | All tests blocked — no `http.Client` to wrap | Implement auth headers transport standalone; wire into KAPI client when available |
| Mock LLM #570 | Code | Open | IT-KAPI-417-006 blocked | Use `httptest.Server` with custom handler that records headers |

### 11.2 Execution Order

1. **Phase 1 — Config & resolver (Unit)**: UT-KAPI-417-010..013 (config validation), UT-KAPI-417-002..006 (resolver logic)
2. **Phase 2 — RoundTripper & scrubbing (Unit)**: UT-KAPI-417-001 (injection), UT-KAPI-417-008 (mixed sources), UT-KAPI-417-009 (scrubbing), UT-KAPI-417-007 (concurrency)
3. **Phase 3 — HTTP round trips (Integration)**: IT-KAPI-417-001..005 (httptest-based)
4. **Phase 4 — Mock LLM verification (Integration)**: IT-KAPI-417-006 (when #570 lands)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/417/TEST_PLAN.md` | Strategy, risk analysis, and test design |
| Unit test suite | `test/unit/kapi/transport/`, `test/unit/kapi/config/` | 13 Ginkgo BDD tests for resolver, RoundTripper, scrubbing, config |
| Integration test suite | `test/integration/kapi/` | 6 Ginkgo BDD tests for full HTTP round trips and Mock LLM verification |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kapi/... -ginkgo.v

# Integration tests
go test ./test/integration/kapi/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kapi/... -ginkgo.focus="UT-KAPI-417-006"

# Coverage — unit tier
go test ./test/unit/kapi/... -coverprofile=unit_coverage.out
go tool cover -func=unit_coverage.out
```

---

## 14. Test Count Summary

| Tier | Count | Coverage Target |
|------|-------|-----------------|
| Unit | 17 | >=80% of ~220 lines (RoundTripper, resolver, config, scrubbing, runtime safety) |
| Integration | 6 | >=80% of ~70 lines (HTTP round trip, file I/O, startup, Mock LLM) |
| **Total** | **23** | |

**AC Coverage**: All 9 acceptance criteria from #417 covered by >=2 tiers. Defensive tests beyond the AC list: startup validation (P1), reserved header rejection (P1), concurrent filePath safety (P1), filePath missing/empty at runtime (P1), RoundTripper request cloning contract (P0), and header value isolation from request body (P2).

---

## 14b. Audit Findings (v1.1)

### Gaps Closed

| # | Finding | Severity | Resolution |
|---|---------|----------|------------|
| G1 | FR-HAPI-433-10 referenced in v1.0 but no FR document exists | Medium | Removed reference; #417 acceptance criteria are the authoritative source |
| G2 | No test for `filePath` absent at request time (sidecar race) | High | Added UT-KAPI-417-014 |
| G3 | No test for empty file content from sidecar | Medium | Added UT-KAPI-417-015 |
| G4 | No test for `http.RoundTripper` contract (request cloning) | High | Added UT-KAPI-417-016 |
| G5 | Duplicate header names in config not tested | Medium | Extended UT-KAPI-417-012 to reject duplicate names |
| G6 | Scrubbing scope only covered logs/errors, not LLM-bound content | Low | Added UT-KAPI-417-017 (defense-in-depth: header values absent from request body) |

### Inconsistencies Resolved

| # | Finding | Severity | Resolution |
|---|---------|----------|------------|
| I1 | #417 issue body says "HAPI SHALL support..." but v1.3 work targets KAPI | Medium | Noted in test plan; issue title is provider-agnostic. A comment will be added to #417 clarifying KAPI is the consumer. |
| I2 | DD-HAPI-005 cited for credential scrubbing — but that DD covers LLM input sanitization, not HTTP header redaction | High | Fixed: now cites DD-HAPI-019-003 (G4: Credential Scrubbing) |
| I3 | #417 says "redacted from any LLM-bound content" but header values are injected at transport layer below prompt assembly | Low | Clarified in plan: real risk is logs/metrics/errors. UT-KAPI-417-017 provides defense-in-depth validation. |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan (IEEE 829 hybrid). 19 tests across 2 tiers covering 9 acceptance criteria + 4 defensive tests. 6 risks with traceability. Suspension criteria for #433 base client and #570 Mock LLM dependencies. |
| 1.1 | 2026-03-04 | Audit: added 4 unit tests (UT-KAPI-417-014..017) for runtime safety — filePath missing/empty at request time, RoundTripper request cloning contract, header value isolation from request body. Added risks R7 (filePath missing), R8 (request mutation). Fixed DD reference: DD-HAPI-019-003 G4 replaces DD-HAPI-005. Clarified scrubbing scope (logs/errors, not LLM prompts). Total: 23 tests (17 unit + 6 integration). |
