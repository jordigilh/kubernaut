# Test Plan: AWX Credential Merging on Job Launch

**Feature**: Merge pre-configured job template credentials with ephemeral credentials on AWX job launch
**Version**: 1.0
**Created**: 2026-03-04
**Author**: Jordi Gil
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- [BR-WE-015]: Ansible Execution Engine
- [DD-WE-006]: Dependency injection for secrets and configmaps

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- GitHub Issue: [#365](https://github.com/jordigilh/kubernaut/issues/365)

---

## 1. Scope

### In Scope

- **`pkg/workflowexecution/executor/ansible.go`**: The `Create` method's credential handling — verifying that when ephemeral credentials are injected, the template's pre-configured credentials are fetched and merged into the launch payload so AWX receives the full union.
- **`pkg/workflowexecution/executor/awx_client.go`**: New `GetJobTemplateCredentials` HTTP method — verifying it correctly queries the AWX API and parses credential IDs.
- **Pure helper logic**: `mergeCredentialIDs` — verifying deduplication, ordering, and edge cases (empty inputs, overlapping IDs).

### Out of Scope

- AWX server-side credential validation (covered by AWX itself)
- Credential type creation/reuse (already covered by UT-WE-015-030..033)
- ConfigMap injection (already covered by UT-WE-015-040..042)
- WFE controller reconciliation loop (separate concern)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Merge is a deduplicated union (template-first, then ephemeral) | AWX `credentials` field is a full replacement; we must include all. Template creds come first to preserve ordering expectations. |
| Fetch failure is non-fatal (log + launch with ephemeral only) | Degraded launch is preferable to hard failure; the ephemeral-only case may still succeed if template has no mandatory creds. |
| E2E test creates template with a pre-configured credential | Reproduces the exact production failure: AWX rejects launch when a required credential is missing from the replacement list. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`mergeCredentialIDs` helper, mock-based `Create` credential path)
- **E2E**: >=80% of the credential merge path exercised against a real AWX instance

### 2-Tier Minimum

- **Unit tests**: Validate pure merge logic correctness and the executor's branching behavior via mock AWXClient
- **E2E tests**: Validate that a real AWX job template with pre-configured credentials launches successfully when ephemeral credentials are also injected

### Business Outcome Quality Bar

Tests validate **business outcomes**:
- **Behavior**: AWX job launches succeed when templates have pre-configured credentials AND ephemeral credentials are injected.
- **Correctness**: The merged credential list contains every template credential AND every ephemeral credential, with no duplicates.
- **Accuracy**: AWX receives the exact credential IDs it expects — no dropped required credentials, no duplicate IDs.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/executor/ansible.go` | `mergeCredentialIDs` (new) | ~18 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/executor/ansible.go` | `AnsibleExecutor.Create` (credential merge path) | ~15 (new lines in existing method) |
| `pkg/workflowexecution/executor/awx_client.go` | `AWXHTTPClient.GetJobTemplateCredentials` (new) | ~35 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-015 | Ephemeral creds must not drop template's pre-configured creds | P0 | Unit | UT-WE-365-001 | REFACTORED |
| BR-WE-015 | Merge deduplicates overlapping credential IDs | P1 | Unit | UT-WE-365-002 | REFACTORED |
| BR-WE-015 | Merge handles empty template credentials (ephemeral-only) | P1 | Unit | UT-WE-365-003 | REFACTORED |
| BR-WE-015 | Merge handles empty ephemeral credentials (template-only) | P1 | Unit | UT-WE-015-032 | Pass (existing) |
| BR-WE-015 | GetJobTemplateCredentials fetch failure is non-fatal | P1 | Unit | UT-WE-365-005 | REFACTORED |
| BR-WE-015 | AWX job succeeds with merged template + ephemeral creds | P0 | E2E | E2E-WE-365-001 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `E2E` (End-to-End)
- **SERVICE**: WE (Workflow Execution)
- **BR_NUMBER**: 365 (GitHub issue)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `mergeCredentialIDs` (100%), `Create` credential merge branching (100%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-WE-365-001` | When a template has pre-configured creds and ephemeral creds are injected, the AWX launch receives the full union — no required creds are dropped | Pending |
| `UT-WE-365-002` | When template and ephemeral lists overlap, duplicates are removed — AWX receives each credential exactly once | Pending |
| `UT-WE-365-003` | When template has no pre-configured creds, only ephemeral creds are sent — existing behavior preserved | Pending |
| `UT-WE-365-004` | When there are no ephemeral creds, template creds are returned unchanged — no unnecessary merge | Pending |
| `UT-WE-365-005` | When fetching template creds fails, launch proceeds with ephemeral only — degraded but not blocked | Pending |

### Tier 2: E2E Tests

**Testable code scope**: Full credential merge path against real AWX

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-WE-365-001` | An AWX job template with a pre-configured K8s credential launches successfully when an ephemeral secret credential is also injected — the playbook completes and WFE reaches Completed | Pending |

### Tier Skip Rationale

- **Integration**: Skipped because the AWX HTTP client is a thin REST wrapper — unit tests cover the branching logic via mocks, and E2E tests cover the real AWX API behavior. An integration tier would only duplicate coverage.

---

## 6. Test Cases (Detail)

### UT-WE-365-001: Template credentials preserved when ephemeral credentials injected

**BR**: BR-WE-015
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: AWX job template has pre-configured credentials [100, 200] AND WFE has a Secret dependency producing ephemeral credential [42]
**When**: `AnsibleExecutor.Create()` is called
**Then**: `LaunchJobTemplateWithCreds` is called with credential IDs [100, 200, 42]

**Acceptance Criteria**:
- LaunchJobTemplateWithCreds receives exactly 3 credential IDs
- Template credential IDs 100 and 200 are present
- Ephemeral credential ID 42 is present
- No credential ID appears more than once

### UT-WE-365-002: Duplicate credential IDs are deduplicated

**BR**: BR-WE-015
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Template credentials = [100, 200], ephemeral credentials = [200, 300]
**When**: `mergeCredentialIDs` is called
**Then**: Result = [100, 200, 300] (200 appears only once)

**Acceptance Criteria**:
- Output length is 3 (not 4)
- All unique IDs are present
- Template IDs appear before ephemeral-only IDs (stable ordering)

### UT-WE-365-003: Empty template credentials preserves ephemeral-only behavior

**BR**: BR-WE-015
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Template has no pre-configured credentials (empty list), ephemeral = [42]
**When**: `mergeCredentialIDs` is called
**Then**: Result = [42]

**Acceptance Criteria**:
- Output contains only the ephemeral credential
- No nil or empty result

### UT-WE-365-004: Empty ephemeral credentials returns template-only

**BR**: BR-WE-015
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Template credentials = [100, 200], ephemeral = []
**When**: `mergeCredentialIDs` is called
**Then**: Result = [100, 200]

**Acceptance Criteria**:
- Output matches template credentials exactly

### UT-WE-365-005: GetJobTemplateCredentials failure is non-fatal

**BR**: BR-WE-015
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: AWX job template has pre-configured credentials BUT `GetJobTemplateCredentials` returns an error
**When**: `AnsibleExecutor.Create()` is called
**Then**: `LaunchJobTemplateWithCreds` is still called with ephemeral credentials only; the error is logged but does not fail the launch

**Acceptance Criteria**:
- `Create` returns no error
- AWX job reference is returned successfully
- LaunchJobTemplateWithCreds is called with only the ephemeral credential IDs
- Error is logged (verified via mock behavior, not log scraping)

### E2E-WE-365-001: AWX job template with pre-configured credential completes with injected secret

**BR**: BR-WE-015
**Type**: E2E
**File**: `test/e2e/workflowexecution/05_ansible_engine_test.go`

**Given**: An AWX job template `kubernaut-test-dep-secret-with-creds` exists with a pre-configured "Machine" credential AND a K8s Secret `dep-test-secret` exists in the controller namespace
**When**: A WFE is created with `engine=ansible`, referencing that template and declaring the secret as a dependency
**Then**: AWX launches the job successfully (no 400 error about missing credentials), the playbook validates the injected secret env var, and the WFE transitions to `Completed`

**Acceptance Criteria**:
- WFE reaches `Running` phase (AWX job was launched — no 400 rejection)
- WFE reaches `Completed` phase (playbook succeeded)
- Ephemeral credential annotation is set on the WFE
- No "Removing ... credential ... without replacement" error in WFE failure details

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `mockAWXClient` (external AWX API) + `fake.NewClientBuilder()` (K8s API)
- **Location**: `test/unit/workflowexecution/ansible_executor_test.go`

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (real AWX instance in Kind cluster)
- **Infrastructure**: AWX deployed in Kind, real K8s Secret, real job template with pre-configured credential
- **Location**: `test/e2e/workflowexecution/05_ansible_engine_test.go`
- **Setup**: `test/infrastructure/awx_e2e.go` — extended to create a test job template with a pre-configured Machine credential

---

## 8. Execution

```bash
# Unit tests (all WE unit tests)
go test ./test/unit/workflowexecution/... -ginkgo.v

# Specific unit tests for #365
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-365"

# E2E tests (all WE E2E tests)
go test ./test/e2e/workflowexecution/... -ginkgo.v -timeout=30m

# Specific E2E test for #365
go test ./test/e2e/workflowexecution/... -ginkgo.focus="E2E-WE-365"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
