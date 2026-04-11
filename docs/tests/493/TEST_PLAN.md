# Test Plan: TLS for Inter-Pod HTTP (#493)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-493-v3
**Feature**: TLS encryption for inter-pod HTTP communication (DataStorage, Gateway, KubernautAgent)
**Version**: 3.0
**Created**: 2026-04-10
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.3_part4`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan validates the shared TLS helper package (`pkg/shared/tls/`) which provides
conditional HTTPS/HTTP server startup and client CA trust for inter-pod traffic. The helper
is currently wired into KubernautAgent's `cmd/` main; DataStorage and Gateway have the
`Start()` branch and config fields ready but `cmd/` wiring is deferred to Helm chart TLS
secret provisioning (see section 4.2).

### 1.2 Objectives

1. **Conditional TLS**: When cert files exist in `CertDir`, server starts HTTPS; otherwise plain HTTP — zero-downgrade path
2. **Client CA trust**: Clients can verify server certificates using a custom CA pool loaded from PEM file
3. **Config accessors**: `TLSConfig` struct exposes `Enabled()`, `CertPath()`, `KeyPath()` with correct path composition
4. **Graceful degradation**: Invalid or missing certs produce clear errors, not panics
5. **Real handshake validation**: Server with private CA cert rejects clients without the correct CA; clients with correct CA succeed

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-SECURITY-493: TLS for inter-pod HTTP
- Issue #493: TLS for inter-pod HTTP
- `pkg/shared/tls/tls.go` — Shared TLS helper package

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- `pkg/datastorage/server/config.go` — DS TLS config integration
- `pkg/gateway/config/config.go` — GW TLS config integration
- `internal/kubernautagent/config/config.go` — KA TLS config integration
- `cmd/kubernautagent/main.go` — KA `ConfigureConditionalTLS` wiring (only service currently wired)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `ConfigureConditionalTLS` reads wrong cert file paths | Server fails to start or loads wrong cert | Medium | UT-TLS-493-001, UT-TLS-493-002 | Exact `tls.crt`/`tls.key` file names validated in tests |
| R2 | Invalid cert content causes panic instead of error | Service crash on startup | High | UT-TLS-493-003 | Test verifies error return, not panic |
| R3 | `LoadCACert` accepts non-PEM content silently | Client trusts wrong CA; handshake succeeds when it shouldn't | Medium | IT-TLS-493-004 | Integration test with wrong CA PEM verifies handshake failure |
| R4 | Plain HTTP accepted when TLS is enabled | Unencrypted traffic between pods | High | IT-TLS-493-002 | Integration test sends HTTP to TLS server, verifies rejection |
| R5 | `TLSConfig.Enabled()` returns true when `CertDir` is empty | Service tries HTTPS without certs, crashes | Medium | UT-TLS-493-008 | Explicit test for empty CertDir |

### 3.1 Risk-to-Test Traceability

| Risk | Mitigating Tests |
|------|-----------------|
| R1 (wrong cert paths) | UT-TLS-493-001, UT-TLS-493-002 |
| R2 (invalid cert panic) | UT-TLS-493-003 |
| R3 (wrong CA accepted) | IT-TLS-493-004 |
| R4 (plaintext accepted) | IT-TLS-493-002 |
| R5 (empty CertDir) | UT-TLS-493-008 |

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **`ConfigureConditionalTLS`** (`pkg/shared/tls/tls.go`): Conditional HTTPS/HTTP server startup based on cert file existence
- **`LoadCACert`** (`pkg/shared/tls/tls.go`): PEM CA certificate loading into `x509.CertPool`
- **`NewTLSTransport`** (`pkg/shared/tls/tls.go`): HTTP transport with custom CA pool for client-side trust
- **`TLSConfig`** (`pkg/shared/tls/tls.go`): Config struct with `Enabled()`, `CertPath()`, `KeyPath()` accessors
- **Real TLS handshake** (integration): Server and client with private CA certs, correct and wrong CAs

### 4.2 Features Not to be Tested

- **DataStorage and Gateway `cmd/` TLS wiring**: DS (`cmd/datastorage/main.go`) and GW (`cmd/gateway/main.go`) do **not** yet call `ConfigureConditionalTLS`. The config fields (`Config.TLS`, `ServerSettings.TLS`) and `Start()` branches (`ListenAndServeTLS` when `TLSConfig != nil`) are in place, but `cmd/` mains do not import `sharedtls` or populate `TLSConfig`. This wiring is deferred until Helm chart TLS secret provisioning is implemented. Only `cmd/kubernautagent/main.go` is fully wired today.
- **Helm chart TLS templating**: Conditional volume mounts, secret references, URL scheme switching (validated in E2E/CI)
- **OCP service-serving certificates**: OpenShift-specific annotation-based cert provisioning (platform-specific)
- **Metrics/health port TLS**: Explicitly out of scope per design decision (ports 9090/8081 stay HTTP)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Shared `pkg/shared/tls/` package | All 3 services (DS, GW, KA) use identical TLS logic; DRY |
| Conditional HTTP/HTTPS via file existence | Zero-config: drop certs into CertDir and service auto-upgrades |
| Private CA certs in integration tests | No external CA dependency; tests run offline |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

- **Unit**: >=80% of `pkg/shared/tls/tls.go` (conditional logic, config parsing, error paths)
- **Integration**: >=80% of TLS handshake paths (server+client interaction)

### 5.2 Two-Tier Minimum

- **Unit tests**: Config parsing, file existence checks, error handling, graceful degradation
- **Integration tests**: Real TLS handshakes with `httptest` server, CA trust verification, plaintext rejection

### 5.3 Business Outcome Quality Bar

Tests validate **business outcomes**: "inter-pod traffic is encrypted when certs are provisioned," "services fall back to HTTP when no certs exist," and "clients reject servers presenting certs from untrusted CAs."

### 5.4 Pass/Fail Criteria

**PASS**: All 13 tests pass, per-tier coverage >=80%, `go build ./...` succeeds.

**FAIL**: Any test fails, coverage below 80%, or build breaks.

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/tls/tls.go` | `ConfigureConditionalTLS`, `LoadCACert`, `NewTLSTransport`, `TLSConfig.Enabled`, `TLSConfig.CertPath`, `TLSConfig.KeyPath` | ~120 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/tls/tls.go` | Full TLS handshake path (server + client via `httptest`) | same file, I/O path |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SECURITY-493 | Conditional TLS HTTPS start | P0 | Unit | UT-TLS-493-001 | Pass |
| BR-SECURITY-493 | Conditional TLS HTTP fallback | P0 | Unit | UT-TLS-493-002 | Pass |
| BR-SECURITY-493 | Invalid cert graceful error | P1 | Unit | UT-TLS-493-003 | Pass |
| BR-SECURITY-493 | CA cert loading | P0 | Unit | UT-TLS-493-004 | Pass |
| BR-SECURITY-493 | Missing CA file error | P1 | Unit | UT-TLS-493-005 | Pass |
| BR-SECURITY-493 | TLS transport with CA | P0 | Unit | UT-TLS-493-006 | Pass |
| BR-SECURITY-493 | TLSConfig Enabled and path accessors | P1 | Unit | UT-TLS-493-007 | Pass |
| BR-SECURITY-493 | Config empty CertDir = disabled | P1 | Unit | UT-TLS-493-008 | Pass |
| BR-SECURITY-493 | HTTPS handshake success | P0 | Integration | IT-TLS-493-001 | Pass |
| BR-SECURITY-493 | HTTP rejected on TLS | P0 | Integration | IT-TLS-493-002 | Pass |
| BR-SECURITY-493 | Client with correct CA | P0 | Integration | IT-TLS-493-003 | Pass |
| BR-SECURITY-493 | Client with wrong CA | P0 | Integration | IT-TLS-493-004 | Pass |
| BR-SECURITY-493 | Default trust rejects private CA cert | P1 | Integration | IT-TLS-493-005 | Pass |

---

## 8. Test Scenarios

### Tier 1: Unit Tests (8)

**Testable code scope**: `pkg/shared/tls/tls.go` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-TLS-493-001` | ConditionalTLS starts HTTPS when cert files exist | Pass |
| `UT-TLS-493-002` | ConditionalTLS starts HTTP when cert files don't exist; server.TLSConfig remains nil | Pass |
| `UT-TLS-493-003` | ConditionalTLS fails gracefully on invalid cert (error, not panic) | Pass |
| `UT-TLS-493-004` | LoadCACert loads valid PEM file into x509.CertPool | Pass |
| `UT-TLS-493-005` | LoadCACert returns error on missing file | Pass |
| `UT-TLS-493-006` | NewTLSTransport builds transport with custom CA pool | Pass |
| `UT-TLS-493-007` | TLSConfig.Enabled reports true when CertDir is set; CertPath/KeyPath return correct paths | Pass |
| `UT-TLS-493-008` | TLSConfig with empty CertDir means disabled | Pass |

### Tier 2: Integration Tests (5)

**Testable code scope**: `pkg/shared/tls/tls.go` TLS handshake paths — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-TLS-493-001` | HTTPS request to server with valid cert succeeds (200 OK) | Pass |
| `IT-TLS-493-002` | Plain HTTP request rejected when server runs TLS (error or 400) | Pass |
| `IT-TLS-493-003` | HTTPS request from client with valid CA succeeds (handshake verified) | Pass |
| `IT-TLS-493-004` | Client with wrong CA cert fails TLS handshake (x509 error) | Pass |
| `IT-TLS-493-005` | Client using default system trust rejects server cert signed by private CA | Pass |

### Tier Skip Rationale

- **E2E**: Deferred to CI/CD pipeline. Helm chart TLS templating and real cert-manager integration validated in Kind cluster E2E tests.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-TLS-493-001: ConditionalTLS starts HTTPS when cert files exist

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: Temp directory with valid `tls.crt` and `tls.key` files (generated via `generateSelfSignedCert`)
2. **When**: `ConfigureConditionalTLS(server, certDir)` is called
3. **Then**: Returns `(true, nil)` and `server.TLSConfig` is non-nil

### UT-TLS-493-002: ConditionalTLS starts HTTP when cert files absent

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: Empty temp directory (no cert files)
2. **When**: `ConfigureConditionalTLS(server, certDir)` is called
3. **Then**: Returns `(false, nil)` and `server.TLSConfig` is nil

### UT-TLS-493-003: ConditionalTLS fails gracefully on invalid cert

**BR**: BR-SECURITY-493
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: Temp directory with `tls.crt` and `tls.key` containing non-PEM garbage
2. **When**: `ConfigureConditionalTLS(server, certDir)` is called
3. **Then**: Returns `(false, err)` where `err` is non-nil; no panic

### UT-TLS-493-004: LoadCACert loads valid PEM file

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: PEM file containing a valid certificate
2. **When**: `LoadCACert(path)` is called
3. **Then**: Returns non-nil `*x509.CertPool` and nil error

### UT-TLS-493-005: LoadCACert returns error on missing file

**BR**: BR-SECURITY-493
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: Path that does not exist
2. **When**: `LoadCACert(path)` is called
3. **Then**: Returns nil pool and non-nil error

### UT-TLS-493-006: NewTLSTransport builds transport with custom CA pool

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: Valid CA PEM file
2. **When**: `NewTLSTransport(caFile)` is called
3. **Then**: Returns non-nil `*http.Transport` with `TLSClientConfig.RootCAs` populated

### UT-TLS-493-007: TLSConfig accessors

**BR**: BR-SECURITY-493
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: `TLSConfig{CertDir: "/certs"}`
2. **When**: `Enabled()`, `CertPath()`, `KeyPath()` are called
3. **Then**: `Enabled()` returns true; `CertPath()` returns `/certs/tls.crt`; `KeyPath()` returns `/certs/tls.key`

### UT-TLS-493-008: Empty CertDir means disabled

**BR**: BR-SECURITY-493
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/tls/tls_test.go`

**Test Steps**:
1. **Given**: `TLSConfig{CertDir: ""}`
2. **When**: `Enabled()` is called
3. **Then**: Returns false

### IT-TLS-493-001: HTTPS handshake success

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Integration
**File**: `test/integration/shared/tls/tls_integration_test.go`

**Test Steps**:
1. **Given**: Private CA, server cert signed by CA, HTTPS server started with cert, client with CA in trust pool via `NewTLSTransport`
2. **When**: Client makes GET request
3. **Then**: HTTP 200 response received

### IT-TLS-493-002: Plain HTTP rejected on TLS server

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Integration
**File**: `test/integration/shared/tls/tls_integration_test.go`

**Test Steps**:
1. **Given**: HTTPS server running with TLS
2. **When**: Plain HTTP client makes GET request
3. **Then**: Either request returns error containing "tls" or "http", or server responds with HTTP 400 (TLS handshake failed)

### IT-TLS-493-003: Client with correct CA succeeds

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Integration
**File**: `test/integration/shared/tls/tls_integration_test.go`

**Test Steps**:
1. **Given**: HTTPS server, client with correct CA loaded via `LoadCACert` + manual `http.Transport`
2. **When**: Client makes GET request
3. **Then**: HTTP 200 response received; confirms handshake through independent CA loading path

### IT-TLS-493-004: Client with wrong CA fails

**BR**: BR-SECURITY-493
**Priority**: P0
**Type**: Integration
**File**: `test/integration/shared/tls/tls_integration_test.go`

**Test Steps**:
1. **Given**: HTTPS server with cert signed by CA-A, client with CA-B in trust pool
2. **When**: Client makes GET request
3. **Then**: TLS handshake error containing "certificate"

### IT-TLS-493-005: Default trust rejects private CA cert

**BR**: BR-SECURITY-493
**Priority**: P1
**Type**: Integration
**File**: `test/integration/shared/tls/tls_integration_test.go`

**Test Steps**:
1. **Given**: HTTPS server with cert signed by private test CA (not in system trust store)
2. **When**: Client using default system trust (no custom CA) makes GET request
3. **Then**: TLS handshake error (system trust store does not include private CA)

**Note**: The server cert is not self-signed — it is signed by a private test CA. The test verifies that the default system trust store rejects certs from unknown CAs.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None (uses temp directories for cert files)
- **Location**: `test/unit/shared/tls/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: `httptest.NewUnstartedServer()` with real TLS certs; `crypto/x509` for CA generation
- **Location**: `test/integration/shared/tls/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23+ | Build, test, and crypto/tls |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| None | N/A | N/A | N/A | N/A |

### 11.2 Execution Order

All tests implemented in a single pass. No inter-test dependencies.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/493/TEST_PLAN.md` | Strategy and test design |
| Shared TLS UTs | `test/unit/shared/tls/tls_test.go` | 8 Ginkgo BDD unit tests |
| Shared TLS ITs | `test/integration/shared/tls/tls_integration_test.go` | 5 Ginkgo BDD integration tests |

---

## 13. Execution

```bash
# Unit tests (shared packages including TLS)
make test-unit-shared-packages

# Integration tests (shared TLS)
go test ./test/integration/shared/... -ginkgo.v -timeout=120s --coverpkg=github.com/jordigilh/kubernaut/pkg/shared/...

# Focused run by test ID
go test ./test/unit/shared/tls/... -ginkgo.focus="493"
go test ./test/integration/shared/tls/... -ginkgo.focus="493"

# Full regression
make test-unit-shared-packages && go test ./test/integration/shared/... -ginkgo.v -timeout=120s

# Lint compliance
make lint-test-patterns
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | New `pkg/shared/tls/` package; no existing code modified |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-10 | Initial test plan with 8 UT + 5 IT |
| 2.0 | 2026-04-10 | Extended to full template. All tests marked Pass. Make target execution. |
| 3.0 | 2026-04-10 | Adversarial audit remediation: (C5) Documented DS/GW cmd/ TLS wiring as deferred in section 4.2 with explicit status. (M3) Corrected UT-007 description from "YAML parsing" to "TLSConfig accessors." (M4) Added CertPath/KeyPath assertions to UT-007 spec. (M5) Changed BR format from #493 to BR-SECURITY-493 throughout matrix. (M6) Corrected IT-005 from "self-signed" to "private CA cert" with explanatory note. (M7) Added missing section 5.3 (Business Outcome Quality Bar). (M8) Tightened IT-002 spec: error must contain "tls" or "http" substring. (M10) Added detailed section 9 case specs for all 13 tests. Updated section 1.1 to reflect actual wiring status. Replaced `make test-integration-shared` with direct go test command (H2: bogus coverpkg path). |
