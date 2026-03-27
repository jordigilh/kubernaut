# Test Plan: Custom K8s Credential Type — Kubeconfig Injection Fix (#552)

**Feature**: Fix AWX/AAP credential type injection to use kubeconfig-file injection instead of env vars, add robust built-in type resolution, and handle empty CA cert.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-WE-015: Ansible Execution Engine Backend
- BR-WE-017: Shared SA Execution Model (v1.1)
- DD-WE-007: Ansible Playbook RBAC Rules
- Issue #552: WE controller custom K8s credential type fails in AAP execution environment

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **`resolveK8sCredentialTypeID`** (`pkg/workflowexecution/executor/ansible.go:360-401`): Add `kind: kubernetes` + `managed: true` fallback search strategy when built-in type name lookup fails. Use versioned custom type name (`kubernaut-k8s-bearer-token-v2`) for kubeconfig-based fallback to avoid conflict with old env-var type.
- **Custom credential type injectors** (`ansible.go:382-392`): Replace env-var injection (`K8S_AUTH_HOST`, `K8S_AUTH_API_KEY`, `K8S_AUTH_SSL_CA_CERT`) with kubeconfig-file injection (`K8S_AUTH_KUBECONFIG` → template kubeconfig file). This ensures the credential takes precedence over in-cluster config in AAP execution environments.
- **`injectK8sCredential`** (`ansible.go:406-432`): Handle empty CA cert by rendering kubeconfig with `insecure-skip-tls-verify: true` and logging a warning. Omit `ssl_ca_cert` from built-in type inputs when empty.
- **AWX client interface**: Add `FindCredentialTypeByKind` method to support `kind: kubernetes` + `managed: true` search strategy.

### Out of Scope

- **Per-workflow SA via TokenRequest** (#501, v1.2): Future enhancement, not blocking v1.1.
- **Credential type migration/cleanup**: Old `kubernaut-k8s-bearer-token` type left intact in AWX (manual cleanup if needed). New type uses versioned name.
- **Non-fatal credential failure surfacing**: WFE status condition for K8s credential injection failure (deferred — current non-fatal logging is acceptable for v1.1).

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Kubeconfig-file injection instead of env vars | `kubernetes.core` module resolution: `module params → kubeconfig → in-cluster config`. Env vars are processed at module param level but in-cluster config wins in K8s pods. Kubeconfig file takes precedence over in-cluster config. |
| Versioned custom type name (`kubernaut-k8s-bearer-token-v2`) | Avoids breaking existing AWX instances that have the old env-var type. Old type left intact, not used. |
| `FindCredentialTypeByKind` as secondary lookup | Built-in type name ("OpenShift or Kubernetes API Bearer Token") may differ across AAP versions. `kind: kubernetes` + `managed: true` is a stable AAP convention. |
| `insecure-skip-tls-verify` when CA cert is empty | Python `ssl` module crashes with `NO_CERTIFICATE_OR_CRL_FOUND` on empty CA file. Falling back to insecure with a warning is safer than crashing. |
| Keep `ssl_ca_cert` field optional in built-in type inputs | When CA is empty, AAP uses system trust store for built-in type. For custom type, kubeconfig renders with `insecure-skip-tls-verify: true`. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of modified functions in `ansible.go` (`resolveK8sCredentialTypeID`, `injectK8sCredential`, new custom type creation logic)
- **Integration**: N/A — AWX API interactions are external (mocked at unit tier). No real AWX instance available in test infrastructure.

### 2-Tier Minimum

- **Unit tests**: Validate credential type resolution strategies, kubeconfig injector structure, empty CA cert handling, and existing regression paths
- **Integration tests**: Skipped — see Tier Skip Rationale

### Business Outcome Quality Bar

Tests validate that:
1. **Playbooks authenticate with the correct SA** — kubeconfig injection ensures `kubernaut-workflow-runner` SA is used, not the AAP pod's own SA
2. **Built-in type resolution is resilient** — multiple search strategies prevent fragile name-based lookups
3. **Empty CA cert doesn't crash playbooks** — graceful fallback to insecure TLS with warning
4. **Existing credential flows are not regressed** — dependency secrets, K8s cred alongside secrets, cleanup

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, mocked AWX client)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/executor/ansible.go` | `resolveK8sCredentialTypeID` (modified: add kind-based fallback) | ~50 modified |
| `pkg/workflowexecution/executor/ansible.go` | `injectK8sCredential` (modified: empty CA handling) | ~30 modified |
| `pkg/workflowexecution/executor/ansible.go` | Custom type creation (new kubeconfig injectors) | ~40 new |
| `pkg/workflowexecution/executor/awx_client.go` | `FindCredentialTypeByKind` (new method) | ~40 new |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| N/A | AWX API is external — no envtest equivalent | N/A |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-015 | Kubeconfig-file injection creates correct kubeconfig structure | P0 | Unit | UT-WE-552-001 | Pending |
| BR-WE-015 | Kubeconfig contains host, token, and base64-encoded CA cert | P0 | Unit | UT-WE-552-002 | Pending |
| BR-WE-015 | K8S_AUTH_KUBECONFIG env var points to kubeconfig template file | P0 | Unit | UT-WE-552-003 | Pending |
| BR-WE-015 | Built-in type resolution by name (existing happy path) | P0 | Unit | UT-WE-552-004 | Pending |
| BR-WE-015 | Built-in type resolution by kind when name lookup fails | P0 | Unit | UT-WE-552-005 | Pending |
| BR-WE-015 | Versioned custom type created when both name and kind lookups fail | P0 | Unit | UT-WE-552-006 | Pending |
| BR-WE-015 | Existing v1 custom type reused (backward compatibility) | P1 | Unit | UT-WE-552-007 | Pending |
| BR-WE-015 | Empty CA cert → kubeconfig with insecure-skip-tls-verify | P0 | Unit | UT-WE-552-008 | Pending |
| BR-WE-015 | Empty CA cert → ssl_ca_cert omitted from built-in type inputs | P1 | Unit | UT-WE-552-009 | Pending |
| BR-WE-015 | Non-empty CA cert → ssl_ca_cert included in inputs | P0 | Unit | UT-WE-552-010 | Pending |
| BR-WE-015 | K8s cred injection proceeds without error on happy path | P0 | Unit | UT-WE-552-011 | Pending |
| BR-WE-015 | Existing UT-WE-500-004 updated for kubeconfig injectors (regression) | P0 | Unit | UT-WE-552-012 | Pending |
| BR-WE-015 | Existing UT-WE-500-001 still passes (built-in type, no change) | P0 | Unit | UT-WE-552-013 | Pending |
| BR-WE-015 | Existing UT-WE-500-003 still passes (in-cluster creds unavailable) | P1 | Unit | UT-WE-552-014 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-WE-552-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `WE` (WorkflowExecution)
- **ISSUE**: `552`

### Tier 1: Unit Tests

**Testable code scope**: `pkg/workflowexecution/executor/ansible.go` — modified `resolveK8sCredentialTypeID`, `injectK8sCredential`, and new kubeconfig-based custom type creation. Target: >=80% of new/modified code.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-WE-552-001` | Custom fallback type uses kubeconfig-file injector (not env vars) — playbooks override in-cluster config | Pending |
| `UT-WE-552-002` | Kubeconfig template contains correct host, token, and CA cert (base64-encoded) | Pending |
| `UT-WE-552-003` | `K8S_AUTH_KUBECONFIG` env var set in injectors pointing to kubeconfig template file | Pending |
| `UT-WE-552-004` | Built-in type resolved by name — existing happy path preserved | Pending |
| `UT-WE-552-005` | Built-in type resolved by kind when name lookup fails — resilient discovery | Pending |
| `UT-WE-552-006` | Versioned custom type (`kubernaut-k8s-bearer-token-v2`) created when all lookups fail | Pending |
| `UT-WE-552-007` | Existing v1 fallback type still reused if found (backward compatibility) | Pending |
| `UT-WE-552-008` | Empty CA cert produces kubeconfig with `insecure-skip-tls-verify: true` — no SSL crash | Pending |
| `UT-WE-552-009` | Empty CA cert omits `ssl_ca_cert` from built-in type credential inputs | Pending |
| `UT-WE-552-010` | Non-empty CA cert includes `ssl_ca_cert` in credential inputs and kubeconfig | Pending |
| `UT-WE-552-011` | Full happy path: creds read → type resolved → credential created → ID returned | Pending |
| `UT-WE-552-012` | Existing UT-WE-500-004 updated to validate kubeconfig injectors instead of env injectors | Pending |
| `UT-WE-552-013` | Existing UT-WE-500-001 (built-in type by name) still passes — no regression | Pending |
| `UT-WE-552-014` | Existing UT-WE-500-003 (in-cluster creds unavailable) still passes — no regression | Pending |

### Tier Skip Rationale

- **Integration**: Skipped — AWX/AAP is an external system not available in test infrastructure. The AWX REST API is mocked via `mockAWXClient` at the unit tier. The `AWXHTTPClient` is a thin HTTP wrapper tested by manual E2E against real AAP.
- **E2E**: Deferred — requires real OCP + AAP deployment. Will be validated during `v1.1.0-rc13` release testing on OCP.

---

## 6. Test Cases (Detail)

### UT-WE-552-001: Custom type uses kubeconfig-file injector

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Both built-in name and kind lookups return "not found"; no existing v1 or v2 custom type
**When**: `resolveK8sCredentialTypeID` is called
**Then**: `CreateCredentialType` is called with injectors containing `file.template.kubeconfig` (kubeconfig YAML) and `env.K8S_AUTH_KUBECONFIG` (template reference)

**Acceptance Criteria**:
- Behavior: Custom type uses kubeconfig file injection, not env var injection
- Correctness: `injectors["file"]` contains `template.kubeconfig` key
- Correctness: `injectors["env"]` contains `K8S_AUTH_KUBECONFIG` → `{{tower.filename.kubeconfig}}`
- Accuracy: No `K8S_AUTH_HOST`, `K8S_AUTH_API_KEY`, or `K8S_AUTH_SSL_CA_CERT` env vars present

---

### UT-WE-552-002: Kubeconfig template structure

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Built-in and kind lookups fail; custom type creation triggered
**When**: `CreateCredentialType` is called
**Then**: The kubeconfig template in `injectors["file"]["template.kubeconfig"]` is a valid YAML kubeconfig containing `{{host}}`, `{{bearer_token}}`, and `{{ssl_ca_cert | b64encode}}`

**Acceptance Criteria**:
- Behavior: Kubeconfig follows standard `~/.kube/config` format
- Correctness: Contains `clusters[0].cluster.server: {{host}}`
- Correctness: Contains `users[0].user.token: {{bearer_token}}`
- Accuracy: CA cert is base64-encoded via Jinja2 filter `| b64encode`

---

### UT-WE-552-003: K8S_AUTH_KUBECONFIG env var in injectors

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Custom type creation triggered
**When**: `CreateCredentialType` is called
**Then**: `injectors["env"]["K8S_AUTH_KUBECONFIG"]` equals `"{{tower.filename.kubeconfig}}"`

**Acceptance Criteria**:
- Behavior: AWX writes kubeconfig to a temp file and sets `K8S_AUTH_KUBECONFIG` to its path
- Correctness: Env key is exactly `K8S_AUTH_KUBECONFIG` (the `kubernetes.core` auth resolution key)

---

### UT-WE-552-004: Built-in type resolved by name (happy path)

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: `FindCredentialTypeByName("OpenShift or Kubernetes API Bearer Token")` returns ID 5
**When**: `resolveK8sCredentialTypeID` is called
**Then**: Returns ID 5 without calling `FindCredentialTypeByKind` or `CreateCredentialType`

**Acceptance Criteria**:
- Behavior: Built-in type preferred when available — no custom type created
- Correctness: ID matches the built-in type
- Accuracy: No secondary lookups or type creation calls

---

### UT-WE-552-005: Built-in type resolved by kind fallback

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: `FindCredentialTypeByName` returns "not found" for both built-in and v1/v2 custom names; `FindCredentialTypeByKind("kubernetes", true)` returns ID 7
**When**: `resolveK8sCredentialTypeID` is called
**Then**: Returns ID 7 without calling `CreateCredentialType`

**Acceptance Criteria**:
- Behavior: Kind-based search is a resilient secondary strategy across AAP versions
- Correctness: `FindCredentialTypeByKind` called with `kind="kubernetes"` and `managed=true`

---

### UT-WE-552-006: Versioned custom type created as last resort

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: All name lookups and kind lookup fail
**When**: `resolveK8sCredentialTypeID` is called
**Then**: `CreateCredentialType` is called with name `"kubernaut-k8s-bearer-token-v2"` and kubeconfig injectors

**Acceptance Criteria**:
- Behavior: Versioned name avoids conflict with existing v1 env-var type
- Correctness: Type name is exactly `"kubernaut-k8s-bearer-token-v2"`

---

### UT-WE-552-007: Existing v1 custom type reused

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Built-in name lookup fails; `FindCredentialTypeByName("kubernaut-k8s-bearer-token")` returns ID 99
**When**: `resolveK8sCredentialTypeID` is called
**Then**: Returns ID 99 (v1 type reused for backward compatibility)

**Acceptance Criteria**:
- Behavior: Existing AWX instances with v1 type continue to work
- Correctness: No v2 type created

---

### UT-WE-552-008: Empty CA cert → insecure kubeconfig

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: `InClusterCredentialsFn` returns credentials with `CACert=""`
**When**: `injectK8sCredential` is called
**Then**: Credential inputs have `ssl_ca_cert` omitted or empty; if custom type creates kubeconfig, it includes `insecure-skip-tls-verify: true`

**Acceptance Criteria**:
- Behavior: No `NO_CERTIFICATE_OR_CRL_FOUND` SSL crash
- Correctness: Kubeconfig includes `insecure-skip-tls-verify: true` when CA is empty

---

### UT-WE-552-009: Empty CA cert → ssl_ca_cert omitted from inputs

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: `InClusterCredentialsFn` returns credentials with `CACert=""`
**When**: `injectK8sCredential` is called
**Then**: `CreateCredential` inputs map does NOT contain `ssl_ca_cert` key (or contains empty string)

**Acceptance Criteria**:
- Behavior: Built-in type falls back to system trust store when CA is empty
- Correctness: `inputs["ssl_ca_cert"]` is absent or empty

---

### UT-WE-552-010: Non-empty CA cert included in inputs

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: `InClusterCredentialsFn` returns credentials with valid CA cert PEM
**When**: `injectK8sCredential` is called
**Then**: `CreateCredential` inputs contain `ssl_ca_cert` with the PEM data

**Acceptance Criteria**:
- Behavior: CA cert is passed to AWX for TLS verification
- Correctness: `inputs["ssl_ca_cert"]` matches the PEM data from `InClusterCredentials.CACert`

---

### UT-WE-552-011: Full happy path — credential created and ID returned

**BR**: BR-WE-015, Issue #552
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Valid in-cluster creds; built-in type found by name
**When**: `injectK8sCredential` is called
**Then**: Returns credential ID without error

**Acceptance Criteria**:
- Behavior: End-to-end K8s credential injection succeeds
- Correctness: No error returned; credential ID is positive

---

### UT-WE-552-012: Existing UT-WE-500-004 updated for kubeconfig injectors

**BR**: BR-WE-015, Issue #552
**Type**: Unit (update existing test)
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Both built-in lookups fail; custom type created
**When**: `CreateCredentialType` is called
**Then**: Injectors contain kubeconfig structure (not env vars)

**Acceptance Criteria**:
- Behavior: UT-WE-500-004 assertions updated from checking `K8S_AUTH_HOST` env vars to checking kubeconfig file template
- Correctness: Test validates new kubeconfig injector structure

---

### UT-WE-552-013: Regression — UT-WE-500-001 still passes

**BR**: BR-WE-015
**Type**: Unit (existing test, no changes expected)
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Built-in type found by name; valid in-cluster creds
**When**: Full `Create` flow with K8s credential injection
**Then**: K8s credential created with built-in type ID, included in launch

**Acceptance Criteria**:
- Behavior: Built-in type happy path is not regressed by #552 changes

---

### UT-WE-552-014: Regression — UT-WE-500-003 still passes

**BR**: BR-WE-015
**Type**: Unit (existing test, no changes expected)
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: `InClusterCredentialsFn` returns error (not in-cluster)
**When**: `Create` is called
**Then**: Job launches without K8s credential (degraded mode)

**Acceptance Criteria**:
- Behavior: In-cluster detection failure is non-fatal (existing behavior preserved)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `mockAWXClient` (existing in test file) with new `findCredentialTypeByKindFn` field; `InClusterCredentialsFn` overridden for test scenarios
- **Location**: `test/unit/workflowexecution/ansible_executor_test.go`

### Integration Tests

- **Skipped**: AWX/AAP is external; no envtest equivalent. Validated by unit tests with mock AWX client.

---

## 8. Execution

```bash
# Unit tests (all WE unit tests)
make test

# Specific test by ID
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-552"

# Regression tests
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-500"
```

---

## 9. Risk Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| **R1: Existing UT-WE-500-004 breaks** | Test asserts env-var injector structure (`K8S_AUTH_HOST`, etc.) | Certain | Update to assert kubeconfig injector structure (UT-WE-552-012) |
| **R2: Built-in type name varies across AAP versions** | Exact string match fails | High | Added kind-based fallback search (UT-WE-552-005) |
| **R3: Old custom type conflicts with new v2 type** | Two custom types in AWX | Medium | Versioned name (`-v2`) avoids conflict. Old type left intact. |
| **R4: Empty CA cert crashes Python ssl module** | `NO_CERTIFICATE_OR_CRL_FOUND` in AAP pod | High (observed in evidence) | Omit CA from inputs; render kubeconfig with `insecure-skip-tls-verify` (UT-WE-552-008/009) |
| **R5: AWXClient interface change** | `FindCredentialTypeByKind` is a new method | Certain | Add method to interface, implement in `AWXHTTPClient`, add to mock |
| **R6: Kubeconfig template Jinja2 syntax** | AWX template engine may not support `b64encode` filter | Low | AWX uses Jinja2 natively; `b64encode` is a standard filter. Validated in Job 31 evidence. |

---

## 10. Existing Tests Requiring Updates

### Unit Tests

| Test ID | File | Current Assertion | Required Change | Reason |
|---------|------|-------------------|-----------------|--------|
| **UT-WE-500-004** (line 1362) | `ansible_executor_test.go` | Asserts `injectors["env"]["K8S_AUTH_HOST"]`, `K8S_AUTH_API_KEY`, `K8S_AUTH_SSL_CA_CERT`, `K8S_AUTH_VERIFY_SSL` | Change to assert `injectors["file"]["template.kubeconfig"]` and `injectors["env"]["K8S_AUTH_KUBECONFIG"]` | Kubeconfig replaces env-var injection |

### Tests NOT Requiring Updates (Verified Safe)

| Test ID | File | Why Safe |
|---------|------|----------|
| UT-WE-500-001 | `ansible_executor_test.go` | Uses built-in type by name — no injector change |
| UT-WE-500-002 | `ansible_executor_test.go` | K8s cred alongside dependency secrets — uses built-in type |
| UT-WE-500-003 | `ansible_executor_test.go` | In-cluster creds unavailable — K8s injection skipped entirely |
| UT-WE-500-005 | `ansible_executor_test.go` | Reuses existing v1 custom type — no creation, no injector check |
| UT-WE-500-006 | `ansible_executor_test.go` | Cleanup flow — deletes credentials, no injector logic |

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
