# Test Plan: Fix Inter-Service TLS Client Trust Gaps

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-750-v1
**Feature**: Fix inter-service TLS client trust gaps in agentclient and authwebhook
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/749-em-reason-pascalcase-and-audit-flush`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that all inter-service HTTP clients in `agentclient` and
`authwebhook` correctly wire `sharedtls.DefaultBaseTransport()` so that the cluster CA
from `TLS_CA_FILE` is trusted on outbound HTTPS calls. It also validates that the
AuthWebhook Helm template mounts the `inter-service-ca` ConfigMap and sets `TLS_CA_FILE`
when `tls.interService.enabled=true`.

### 1.2 Objectives

1. **TLS trust wiring**: Both `NewKubernautAgentClient` and `NewDSClientAdapter` use `sharedtls.DefaultBaseTransport()` as base transport
2. **Error propagation**: Invalid `TLS_CA_FILE` values cause constructors to return errors instead of silently proceeding with a broken transport
3. **Regression safety**: With `TLS_CA_FILE` unset, constructors continue to work as before (plain HTTP)
4. **Helm parity**: AuthWebhook deployment template matches the TLS mount pattern used by all other controllers
5. **Dead code removal**: `NewInterServiceClient` (zero callers) is removed in REFACTOR phase

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on affected files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |
| Helm template correctness | Manual | `helm template --set tls.interService.enabled=true` |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #750: Fix inter-service TLS client trust gaps in agentclient and authwebhook
- Issue #493: TLS for inter-pod HTTP communication (introduced shared TLS infra)
- Issue #678: Wire Gateway/DataStorage inter-service TLS
- DD-AUTH-006: ServiceAccount authentication
- DD-HAPI-003: Mandatory OpenAPI Client Usage

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Existing TLS tests: `test/unit/shared/tls/tls_test.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `DefaultBaseTransport()` changes connection pool params in authwebhook | Increased idle connections (10→100) | Low | UT-AW-750-002 | Accepted: consistency with all other services; authwebhook is not a high-concurrency service |
| R2 | Existing agentclient tests break from new error path | Test failures | Low | UT-AC-750-003 | `TLS_CA_FILE` unset → `DefaultBaseTransport()` returns plain transport (identical behavior) |
| R3 | Helm template syntax error breaks deployment | Deployment failure | Low | Manual helm template | Copy exact pattern from 6 existing controllers |
| R4 | `NewDSClientAdapterFromClient` still uses `http.DefaultClient` | Test-only gap | Very Low | N/A | Test-only code; TLS responsibility is on caller who created ogen client |

### 3.1 Risk-to-Test Traceability

- **R1**: UT-AW-750-002 validates constructor succeeds with valid CA
- **R2**: UT-AC-750-003 validates constructor succeeds without `TLS_CA_FILE`
- **R3**: Manual `helm template` validation after Helm change

---

## 4. Scope

### 4.1 Features to be Tested

- **`pkg/agentclient/client.go`** (`NewKubernautAgentClient`): Validates TLS transport wiring for AI Analysis → KA outbound calls
- **`pkg/authwebhook/ds_client.go`** (`NewDSClientAdapter`): Validates TLS transport wiring for AuthWebhook → DataStorage outbound calls
- **`charts/kubernaut/templates/authwebhook/authwebhook.yaml`**: Validates Helm template includes CA mount when inter-service TLS enabled

### 4.2 Features Not to be Tested

- **`NewDSClientAdapterFromClient`**: Test-only wrapper; TLS responsibility belongs to caller who creates the ogen client
- **`audit.NewOpenAPIClientAdapter`**: Already correctly uses `DefaultBaseTransport()` (verified during due diligence)
- **Other controllers' TLS wiring**: Already correct (aianalysis, notification, workflowexecution, RO, SP, gateway)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Test via `TLS_CA_FILE` env var injection | Constructors are opaque (return wrapped clients); env var is the observable contract. Invalid path → error proves the constructor reads it. |
| Option A: use `DefaultBaseTransport()` as-is | Consistency with all other services; connection pool param difference is negligible |
| Remove `NewInterServiceClient` in REFACTOR | Zero callers confirmed; dead code per issue #750 |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in `NewKubernautAgentClient` and `NewDSClientAdapter`
- **Integration**: Not applicable — TLS wiring is a constructor concern, not an I/O integration boundary
- **E2E**: Deferred — requires full Kind cluster with `tls.interService.enabled=true`

### 5.2 Two-Tier Minimum

Unit tests provide the primary coverage. The Helm template change is validated manually
via `helm template`. E2E is deferred because it requires a Kind cluster with inter-service
TLS enabled, which is out of scope for this bug fix.

### 5.3 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. Existing agentclient and authwebhook tests pass without modification
3. `helm template` with `tls.interService.enabled=true` shows `TLS_CA_FILE`, `tls-ca` volume mount, and `inter-service-ca` ConfigMap in authwebhook deployment
4. `go build ./...` succeeds

**FAIL** — any of the following:

1. Any P0 test fails
2. Existing tests regress
3. Helm template missing required TLS artifacts

### 5.4 Suspension & Resumption Criteria

**Suspend**: Build broken or `DefaultBaseTransport()` API changes
**Resume**: Build fixed

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/agentclient/client.go` | `NewKubernautAgentClient` | ~15 |
| `pkg/authwebhook/ds_client.go` | `NewDSClientAdapter` | ~30 |
| `pkg/shared/tls/tls.go` | `NewInterServiceClient` (removal) | ~10 |

### 6.2 Helm Template (manual validation)

| File | Change | Lines (approx) |
|------|--------|-----------------|
| `charts/kubernaut/templates/authwebhook/authwebhook.yaml` | Add TLS env/volume/mount | ~15 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTEGRATION-750 | agentclient trusts cluster CA via TLS_CA_FILE | P0 | Unit | UT-AC-750-001 | Pending |
| BR-INTEGRATION-750 | agentclient TLS trust with valid CA | P0 | Unit | UT-AC-750-002 | Pending |
| BR-INTEGRATION-750 | agentclient regression guard (no TLS_CA_FILE) | P0 | Unit | UT-AC-750-003 | Pending |
| BR-INTEGRATION-750 | authwebhook DS client trusts cluster CA via TLS_CA_FILE | P0 | Unit | UT-AW-750-001 | Pending |
| BR-INTEGRATION-750 | authwebhook DS client TLS trust with valid CA | P0 | Unit | UT-AW-750-002 | Pending |
| BR-INTEGRATION-750 | authwebhook DS client regression guard (no TLS_CA_FILE) | P0 | Unit | UT-AW-750-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

- **UT-AC-750-NNN**: Unit tests for agentclient TLS (#750)
- **UT-AW-750-NNN**: Unit tests for authwebhook DS client TLS (#750)

### Tier 1: Unit Tests

**Testable code scope**: `NewKubernautAgentClient`, `NewDSClientAdapter` — >=80% coverage target

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AC-750-001` | agentclient constructor fails fast when TLS_CA_FILE points to invalid path | Pending |
| `UT-AC-750-002` | agentclient constructor succeeds with valid CA cert in TLS_CA_FILE | Pending |
| `UT-AC-750-003` | agentclient constructor succeeds when TLS_CA_FILE is unset (regression guard) | Pending |
| `UT-AW-750-001` | authwebhook DS client constructor fails fast when TLS_CA_FILE points to invalid path | Pending |
| `UT-AW-750-002` | authwebhook DS client constructor succeeds with valid CA cert in TLS_CA_FILE | Pending |
| `UT-AW-750-003` | authwebhook DS client constructor succeeds when TLS_CA_FILE is unset (regression guard) | Pending |

### Tier Skip Rationale

- **Integration**: TLS wiring is a constructor concern tested at unit level via env var injection. No cross-component I/O boundary to test.
- **E2E**: Requires Kind cluster with `tls.interService.enabled=true`. Deferred to a dedicated TLS E2E test effort.

---

## 9. Test Cases

### UT-AC-750-001: agentclient fails on invalid TLS_CA_FILE

**BR**: BR-INTEGRATION-750
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/agentclient_tls_test.go`

**Test Steps**:
1. **Given**: `TLS_CA_FILE` env var set to `/nonexistent/ca.crt`
2. **When**: `NewKubernautAgentClient(Config{BaseURL: "https://localhost:9999"})` is called
3. **Then**: Constructor returns a non-nil error containing "failed to read CA certificate"

**Acceptance Criteria**:
- **Behavior**: Constructor propagates `DefaultBaseTransport()` error
- **Correctness**: Error is returned, no nil-pointer client
- **Security**: Misconfigured TLS is a hard failure, not a silent fallback

### UT-AC-750-002: agentclient succeeds with valid TLS_CA_FILE

**BR**: BR-INTEGRATION-750
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/agentclient_tls_test.go`

**Test Steps**:
1. **Given**: Self-signed CA cert written to temp file, `TLS_CA_FILE` set to that path
2. **When**: `NewKubernautAgentClient(Config{BaseURL: "https://localhost:9999"})` is called
3. **Then**: Constructor returns a non-nil client and nil error

### UT-AC-750-003: agentclient succeeds without TLS_CA_FILE (regression)

**BR**: BR-INTEGRATION-750
**Priority**: P0
**Type**: Unit
**File**: `test/unit/aianalysis/agentclient_tls_test.go`

**Test Steps**:
1. **Given**: `TLS_CA_FILE` env var is unset
2. **When**: `NewKubernautAgentClient(Config{BaseURL: "http://localhost:9999"})` is called
3. **Then**: Constructor returns a non-nil client and nil error

### UT-AW-750-001: authwebhook DS client fails on invalid TLS_CA_FILE

**BR**: BR-INTEGRATION-750
**Priority**: P0
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_tls_test.go`

**Test Steps**:
1. **Given**: `TLS_CA_FILE` env var set to `/nonexistent/ca.crt`
2. **When**: `NewDSClientAdapter("https://localhost:9999", 5s, logger)` is called
3. **Then**: Constructor returns a non-nil error containing "failed to read CA certificate"

### UT-AW-750-002: authwebhook DS client succeeds with valid TLS_CA_FILE

**BR**: BR-INTEGRATION-750
**Priority**: P0
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_tls_test.go`

**Test Steps**:
1. **Given**: Self-signed CA cert written to temp file, `TLS_CA_FILE` set to that path
2. **When**: `NewDSClientAdapter("https://localhost:9999", 5s, logger)` is called
3. **Then**: Constructor returns a non-nil adapter and nil error

### UT-AW-750-003: authwebhook DS client succeeds without TLS_CA_FILE (regression)

**BR**: BR-INTEGRATION-750
**Priority**: P0
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_tls_test.go`

**Test Steps**:
1. **Given**: `TLS_CA_FILE` env var is unset
2. **When**: `NewDSClientAdapter("http://localhost:9999", 5s, logger)` is called
3. **Then**: Constructor returns a non-nil adapter and nil error

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None — tests use env var injection and temp cert files
- **Location**: `test/unit/aianalysis/`, `test/unit/authwebhook/`
- **Resources**: Minimal (temp file I/O only)

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| Helm | 3.x | Template validation (manual) |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All required infrastructure (`pkg/shared/tls`, Helm helpers) already exists.

### 11.2 Execution Order

1. **Phase 1 — TDD RED**: Write failing tests (UT-AC-750-001 through UT-AW-750-003)
2. **Phase 2 — TDD GREEN**: Fix `agentclient/client.go`, `authwebhook/ds_client.go`, and Helm template
3. **Phase 3 — TDD REFACTOR**: Remove `NewInterServiceClient` dead code, final audit
4. **Checkpoint**: Build, lint, full test pass, confidence assessment

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/750/TEST_PLAN.md` | Strategy and test design |
| agentclient TLS tests | `test/unit/aianalysis/agentclient_tls_test.go` | 3 Ginkgo BDD tests |
| authwebhook DS client TLS tests | `test/unit/authwebhook/ds_client_tls_test.go` | 3 Ginkgo BDD tests |

---

## 13. Execution

```bash
# Unit tests — agentclient TLS
go test ./test/unit/aianalysis/... -ginkgo.v -ginkgo.focus="750"

# Unit tests — authwebhook DS client TLS
go test ./test/unit/authwebhook/... -ginkgo.v -ginkgo.focus="750"

# All unit tests (regression check)
go test ./test/unit/... -ginkgo.v

# Helm template validation (manual)
helm template kubernaut charts/kubernaut \
  --set tls.interService.enabled=true \
  | grep -A5 "TLS_CA_FILE"
```

---

## 14. Existing Tests Requiring Updates

None. Existing tests do not set `TLS_CA_FILE` and will continue to work with
`DefaultBaseTransport()` returning a plain transport (identical to `http.DefaultTransport`).

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
