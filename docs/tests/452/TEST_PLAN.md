# Test Plan: EM TLS CA + Bearer Token Support

**Feature**: Add TLS CA and bearer token support to EffectivenessMonitor HTTP clients for OCP HTTPS connectivity
**Version**: 1.0
**Created**: 2026-03-19
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.2`

**Authority**:
- BR-EM-002: AlertManager alert resolution scoring
- BR-EM-003: Prometheus metric comparison scoring
- Issue #452: EM must support OCP service-serving CA injection

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `ExternalConfig`: new `TLSCaFile` field, validation logic, default behavior
- TLS HTTP client builder: PEM loading, cert pool construction, transport wiring
- Prometheus/AlertManager client constructors: accept pre-built `*http.Client`
- `cmd/effectivenessmonitor` wiring: build TLS or plain client based on config
- Bearer token integration: SA token transport wrapping TLS transport

### Out of Scope

- Helm chart testing (validated manually on OCP, not in automated test suite)
- E2E tests (TLS requires OCP service-serving CA infrastructure; Kind uses HTTP)
- Performance benchmarking of TLS handshake overhead

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Separate TLS builder from bearer token wrapping | Composability -- callers choose whether to add auth |
| Constructor accepts `*http.Client` not `timeout` | Caller controls transport (TLS, auth, timeouts) |
| Validate file existence at config time, not client time | Fail fast with clear error on startup |
| System cert pool + custom CA (not replace) | Custom CA augments system trust, doesn't break other HTTPS |
| `NewHTTPClientWithCA` accepts `timeout` parameter | Guarantees connections never hang, matches existing behavior |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of unit-testable code (config validation, PEM parsing, client builder)
- **Integration**: >=80% of integration-testable code (HTTPS connectivity via httptest.NewTLSServer)

### 2-Tier Minimum

Every business requirement gap covered by UT + IT:
- Unit tests catch PEM parsing, validation, and error handling
- Integration tests catch real TLS handshake, cert verification, and bearer token injection

### Business Outcome Quality Bar

Tests validate business outcomes -- "can the EM reach Prometheus/AlertManager over HTTPS with a custom CA?" -- not just "does the code compile with TLS fields?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/config/effectivenessmonitor/config.go` | `ExternalConfig.TLSCaFile`, `Validate()` | ~15 new |
| `pkg/effectivenessmonitor/client/tls.go` (new) | `NewHTTPClientWithCA()` | ~30 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/effectivenessmonitor/client/prometheus_http.go` | `NewPrometheusHTTPClient` (new signature) | ~5 changed |
| `pkg/effectivenessmonitor/client/alertmanager_http.go` | `NewAlertManagerHTTPClient` (new signature) | ~5 changed |
| `cmd/effectivenessmonitor/main.go` | TLS client wiring block | ~15 new |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-002 | AlertManager works over HTTPS with custom CA | P0 | Unit | UT-EM-452-001 | Pending |
| BR-EM-002 | AlertManager HTTPS integration | P0 | Integration | IT-EM-452-002 | Pending |
| BR-EM-003 | Prometheus works over HTTPS with custom CA | P0 | Unit | UT-EM-452-001 | Pending |
| BR-EM-003 | Prometheus HTTPS integration | P0 | Integration | IT-EM-452-001 | Pending |
| #452 | Invalid CA file produces clear error | P1 | Unit | UT-EM-452-002 | Pending |
| #452 | Corrupt PEM produces clear error | P1 | Unit | UT-EM-452-003 | Pending |
| #452 | Empty tlsCaFile preserves Kind behavior | P0 | Unit | UT-EM-452-004 | Pending |
| #452 | Validate rejects non-existent tlsCaFile | P1 | Unit | UT-EM-452-005 | Pending |
| #452 | Validate accepts empty tlsCaFile | P0 | Unit | UT-EM-452-006 | Pending |
| #452 | Validate accepts valid tlsCaFile | P0 | Unit | UT-EM-452-007 | Pending |
| #452 | Bearer token injected on HTTPS requests | P0 | Integration | IT-EM-452-003 | Pending |
| #452 | Plain HTTP client when no CA configured | P0 | Integration | IT-EM-452-004 | Pending |

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-EM-452-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **EM**: EffectivenessMonitor service abbreviation
- **452**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `config.go` (Validate), `tls.go` (NewHTTPClientWithCA) -- target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-452-001` | Valid PEM CA file produces an HTTP client whose TLS config trusts the custom CA | RED |
| `UT-EM-452-002` | Non-existent CA file returns a clear, actionable error message | RED |
| `UT-EM-452-003` | Invalid/corrupt PEM content returns a clear error (not silent fallback to system CAs) | RED |
| `UT-EM-452-004` | Empty `tlsCaFile` produces a plain HTTP client (Kind/upstream path unchanged) | RED |
| `UT-EM-452-005` | `Validate()` rejects config where `tlsCaFile` points to non-existent path | RED |
| `UT-EM-452-006` | `Validate()` accepts config with empty `tlsCaFile` (Kind default) | RED |
| `UT-EM-452-007` | `Validate()` accepts config with valid `tlsCaFile` path pointing to real file | RED |

### Tier 2: Integration Tests

**Testable code scope**: Prometheus/AlertManager clients over real HTTPS via `httptest.NewTLSServer` -- target >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-452-001` | Prometheus client successfully queries a TLS server signed by a custom CA | RED |
| `IT-EM-452-002` | AlertManager client successfully retrieves alerts from a TLS server signed by a custom CA | RED |
| `IT-EM-452-003` | Bearer token header is present on HTTPS requests when SA transport wraps TLS transport | RED |
| `IT-EM-452-004` | When no `tlsCaFile` is configured, clients connect via plain HTTP (backward-compatible) | RED |

### Tier Skip Rationale

- **E2E**: Skipped -- TLS CA injection requires OCP service-serving CA infrastructure. Kind E2E tests use HTTP endpoints. OCP validation is manual during rc testing.

---

## 6. Test Cases (Detail)

### UT-EM-452-001: Valid PEM produces TLS-enabled HTTP client

**BR**: BR-EM-002, BR-EM-003
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/tls_client_test.go`

**Given**: A temp file containing a valid PEM-encoded CA certificate
**When**: `NewHTTPClientWithCA(caFile, 10s)` is called
**Then**: The returned `*http.Client` has a non-nil Transport with TLS config whose RootCAs pool contains the custom CA

**Acceptance Criteria**:
- Client is non-nil and error is nil
- Client Transport is `*http.Transport` (not default)
- TLS config RootCAs is non-nil
- Client Timeout equals the provided duration

### UT-EM-452-002: Non-existent CA file returns clear error

**BR**: #452
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/tls_client_test.go`

**Given**: A path that does not exist on the filesystem
**When**: `NewHTTPClientWithCA("/nonexistent/ca.crt", 10s)` is called
**Then**: Error is returned containing the file path and "no such file" context

**Acceptance Criteria**:
- Error is non-nil
- Error message contains the file path
- Client is nil

### UT-EM-452-003: Invalid PEM returns clear error

**BR**: #452
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/tls_client_test.go`

**Given**: A temp file containing "not-a-valid-pem-file"
**When**: `NewHTTPClientWithCA(corruptFile, 10s)` is called
**Then**: Error is returned indicating invalid PEM content

**Acceptance Criteria**:
- Error is non-nil
- Error message indicates PEM parsing failure

### UT-EM-452-004: Empty tlsCaFile yields plain HTTP client

**BR**: BR-EM-002, BR-EM-003
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/tls_client_test.go`

**Given**: Empty string for CA file path
**When**: Wiring logic selects plain client path
**Then**: Returned client uses default transport with no custom TLS config

**Acceptance Criteria**:
- Client timeout matches provided value
- Client transport is nil (uses http.DefaultTransport)

### UT-EM-452-005: Validate rejects non-existent tlsCaFile

**BR**: #452
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/service_config_test.go`

**Given**: Config with `TLSCaFile: "/does/not/exist"`
**When**: `cfg.Validate()` is called
**Then**: Validation error referencing the file path

**Acceptance Criteria**:
- Error contains "tlsCaFile"
- Error contains the path

### UT-EM-452-006: Validate accepts empty tlsCaFile

**BR**: #452
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/service_config_test.go`

**Given**: DefaultConfig with `TLSCaFile: ""`
**When**: `cfg.Validate()` is called
**Then**: Validation passes

### UT-EM-452-007: Validate accepts valid tlsCaFile

**BR**: #452
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/service_config_test.go`

**Given**: Config with `TLSCaFile` pointing to a temp file that exists
**When**: `cfg.Validate()` is called
**Then**: Validation passes

### IT-EM-452-001: Prometheus queries TLS server with custom CA

**BR**: BR-EM-003
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/tls_integration_test.go`

**Given**: An `httptest.NewTLSServer` serving Prometheus-format responses, its CA cert written to a temp PEM file, and a Prometheus client built with `NewHTTPClientWithCA`
**When**: `promClient.Query(ctx, "up", time.Now())` is called
**Then**: Response is successfully parsed with no TLS errors

**Acceptance Criteria**:
- Query returns non-nil result
- No `x509: certificate signed by unknown authority` error

### IT-EM-452-002: AlertManager queries TLS server with custom CA

**BR**: BR-EM-002
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/tls_integration_test.go`

**Given**: An `httptest.NewTLSServer` serving AlertManager-format responses, its CA cert written to a temp PEM file, and an AlertManager client built with `NewHTTPClientWithCA`
**When**: `amClient.GetAlerts(ctx, filters)` is called
**Then**: Response is successfully parsed with no TLS errors

### IT-EM-452-003: Bearer token injected on HTTPS requests

**BR**: #452
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/tls_integration_test.go`

**Given**: A TLS server that captures `Authorization` headers, a temp SA token file, and TLS transport wrapped with `NewServiceAccountTransportWithBase`
**When**: Prometheus client makes a query
**Then**: Request contains `Authorization: Bearer <token>` header

**Acceptance Criteria**:
- Header is present and non-empty
- Token matches temp file content

### IT-EM-452-004: Plain HTTP when no CA configured

**BR**: BR-EM-002, BR-EM-003
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/tls_integration_test.go`

**Given**: An `httptest.NewServer` (plain HTTP) and a Prometheus client built with `&http.Client{Timeout: 10s}` (no TLS)
**When**: `promClient.Query(ctx, "up", time.Now())` is called
**Then**: Query succeeds over plain HTTP (backward-compatible Kind path)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `os.CreateTemp` for PEM files; no external dependencies
- **Location**: `test/unit/effectivenessmonitor/tls_client_test.go` and existing `service_config_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks -- `httptest.NewTLSServer` with real TLS, temp PEM files with real x509 certs
- **Infrastructure**: `httptest.NewTLSServer` (Go stdlib, no external services)
- **Location**: `test/integration/effectivenessmonitor/tls_integration_test.go`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="UT-EM-452"

# Integration tests
go test ./test/integration/effectivenessmonitor/... -ginkgo.focus="IT-EM-452"

# All EM tests (regression)
make test-unit-effectivenessmonitor
make test-integration-effectivenessmonitor
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-19 | Initial test plan |
