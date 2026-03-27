# Test Plan: RO/EM Cluster-Wide Read RBAC and Resilient Hash Capture (#545)

**Feature**: RO and EM bind to the Kubernetes `view` ClusterRole so both can capture pre/post-remediation hashes for effectiveness assessment; defense-in-depth soft-fail prevents pipeline failure when `view` does not cover a CRD
**Version**: 1.2
**Created**: 2026-03-04
**Author**: AI Agent
**Status**: Draft
**Branch**: `fix/v1.1.0-rc13`

**Authority**:
- DD-EM-002: Pre-remediation spec hash capture for effectiveness assessment
- DD-EM-003: Hash uses RemediationTarget (not incident resource)
- Issue #545: RO ClusterRole missing RBAC for cert-manager CRDs

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **Helm ClusterRoleBinding to `view` (PRIMARY FIX)**: Both RO and EM service accounts are bound to the Kubernetes built-in `view` ClusterRole via new `ClusterRoleBinding` resources. This is the primary fix: it gives both controllers the RBAC they need to read the remediationTarget spec and compute hashes. Without these hashes, the EffectivenessAssessment cannot compare pre- and post-remediation state, rendering the EA non-functional for that remediation. The `view` ClusterRole uses aggregation labels, so well-behaved CRDs (cert-manager, Istio, etc.) are automatically included.
- **`CapturePreRemediationHash` signature change + soft-fail (DEFENSE-IN-DEPTH)**: For CRDs not covered by `view` aggregation (poorly-configured operators that don't ship aggregation labels), the function returns `("", "reason", nil)` instead of a hard error. The new `degradedReason` return value enables callers to emit K8s Events alerting operators to the RBAC gap. This is a degraded mode — the EA will have no pre-hash to compare against — but it prevents the entire remediation pipeline from failing.

### Out of Scope

- **EM `getTargetSpec`**: Already gracefully degrades (returns empty map on any error). No code change needed; no new tests required.
- **EM unit hash tests (`hash_test.go`)**: The `hash.Computer` is a pure-logic component unaffected by this change.
- **Wildcard CRD read (Option B)**: Rejected for security reasons; not implemented.
- **Issue #544 (Dockerfile labels)**: Separate scope.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| `view` ClusterRole binding is the PRIMARY fix | Both RO and EM must be able to capture the hash for the EA to function. `view` grants broad read via aggregation labels — cert-manager, Istio, and other well-behaved operators automatically become readable. Without this, the EA cannot compare pre/post remediation state and the effectiveness assessment is non-functional. |
| Soft-fail on `reader.Get()` errors is DEFENSE-IN-DEPTH | For CRDs not covered by `view` (operators that don't ship aggregation labels), the pipeline should degrade gracefully rather than fail terminally. The soft-fail is a degraded mode, not the desired state. |
| Return `degradedReason` string (3-tuple signature) | `CapturePreRemediationHash` returns `(hash, degradedReason, err)`. `degradedReason` is non-empty when the function soft-fails on access errors (Forbidden, transient). The caller (which has `r.Recorder` and the RR) emits a K8s Event using `degradedReason`, keeping the function pure (no EventRecorder dependency). Blast radius: 2 production callers + 12 test call sites + 1 behavior test. |
| Log `Forbidden` at Warning level + K8s Event via caller | Unlike `NotFound` (logged at V(1)/debug), a `Forbidden` error indicates a configuration gap. Warning-level log in the function + K8s Event from the caller ensures operators notice the RBAC gap. |
| Keep existing explicit resource rules | Belt-and-suspenders: if `view` is unbound, explicit rules ensure common resource types still work. Explicit rules also serve as documentation of minimum required access. |
| Soft-fail scope: `reader.Get()` errors only | `NestedMap` and `CanonicalSpecHash` errors remain as hard errors — these indicate data problems that won't self-resolve. |
| UT-RO-214-010 behavior change | The existing test asserts Get() errors cause RR to transition to Failed. After this change, the function soft-fails and the RR proceeds with empty hash. The test must be updated to reflect the new expected behavior. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `CapturePreRemediationHash` code paths. The function has 7 distinct code paths; 6 have explicit tests (86% path coverage). The 7th (`CanonicalSpecHash` error) is an internal library failure that requires crafting data that passes `NestedMap` but fails JSON marshaling — excluded as low-value/fragile.
- **Integration**: >=80% of Helm template changes validated via `helm template` rendering.

### 2-Tier Minimum

- **Unit tests**: Validate `CapturePreRemediationHash` behavior for every error class.
- **Helm validation**: Validate chart renders the `ClusterRoleBinding` to `view` for both controllers via `helm template` output assertions.

### Tier Skip Rationale

- **E2E**: Deferred. Full E2E validation (deploy to Kind, trigger cert-manager scenario, verify RR does not fail) will be covered by the existing cert-failure E2E scenario in the release pipeline.

### Business Outcome Quality Bar

Every test validates a **business outcome**: "can the controller capture the hash so the EA functions?" for the happy path, and "does the pipeline survive when it cannot?" for the degraded path.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `CapturePreRemediationHash` | ~60 (lines 3363-3421) |

### Code Paths in `CapturePreRemediationHash`

| # | Path | Current Behavior | After Fix | Test Coverage |
|---|------|-----------------|-----------|---------------|
| 1 | GVK resolution failure | `("", nil)` soft-fail | No change | UT-RO-545-004 |
| 2 | `reader.Get()` returns `NotFound` | `("", nil)` soft-fail | No change | UT-RO-545-003 (pre-existing) |
| 3 | `reader.Get()` returns `Forbidden` | **Hard error** | `("", nil)` soft-fail + Warning log | UT-RO-545-001 (new) |
| 4 | `reader.Get()` returns other API error | **Hard error** | `("", nil)` soft-fail + Warning log | UT-RO-545-002 (new) |
| 5 | No `.spec` field | `("", nil)` soft-fail | No change | UT-RO-545-006 |
| 6 | Success (hash computed) | Returns `("sha256:...", nil)` | No change | UT-RO-545-005 (pre-existing) |
| 7 | `CanonicalSpecHash` error | Hard error | No change (hard error retained) | Excluded (see Section 2) |

### Integration-Testable Code (Helm)

| File | Change | Lines (approx) |
|------|--------|-----------------|
| `charts/kubernaut/templates/remediationorchestrator/remediationorchestrator.yaml` | New `ClusterRoleBinding` to `view` | ~10 |
| `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` | New `ClusterRoleBinding` to `view` | ~10 |

---

## 4. BR Coverage Matrix

| BR/Issue ID | Description | Priority | Tier | Test ID | Status |
|-------------|-------------|----------|------|---------|--------|
| Issue #545 | Forbidden error returns soft-fail (empty hash, no error) | P0 | Unit | UT-RO-545-001 | Pending |
| Issue #545 | Other API errors (K8s InternalError) return soft-fail | P0 | Unit | UT-RO-545-002 | Pending |
| DD-EM-002 | NotFound returns soft-fail (regression guard) | P1 | Unit | UT-RO-545-003 | Pass (pre-existing) |
| DD-EM-002 | Unknown Kind returns soft-fail | P1 | Unit | UT-RO-545-004 | Pending |
| DD-EM-002 | Successful hash capture unaffected (regression guard) | P1 | Unit | UT-RO-545-005 | Pass (pre-existing) |
| DD-EM-002 | No .spec field returns empty hash (regression guard) | P1 | Unit | UT-RO-545-006 | Pass (pre-existing) |
| Issue #545 | Helm renders `view` ClusterRoleBinding for RO SA | P0 | Helm | IT-HELM-545-001 | Pending |
| Issue #545 | Helm renders `view` ClusterRoleBinding for EM SA | P0 | Helm | IT-HELM-545-002 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass (pre-existing): Existing test that validates unchanged behavior; re-run as regression guard

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `RO` (RemediationOrchestrator), `HELM` (Helm chart)
- **BR_NUMBER**: 545

### Tier 1: Unit Tests

**Testable code scope**: `CapturePreRemediationHash` in `internal/controller/remediationorchestrator/reconciler.go` — 7 code paths, 6 tested (86% path coverage).

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-545-001` | Remediation proceeds (degraded) when RBAC denies access to target resource — Forbidden yields empty hash, no error, Warning log emitted | Pending |
| `UT-RO-545-002` | Remediation proceeds (degraded) when K8s API returns InternalError — transient error yields empty hash, no error, Warning log emitted | Pending |
| `UT-RO-545-003` | Remediation proceeds when target resource does not exist — NotFound returns empty hash (regression guard) | Pass (pre-existing) |
| `UT-RO-545-004` | Remediation proceeds when GVK cannot be resolved — unknown Kind returns empty hash | Pending |
| `UT-RO-545-005` | EA receives valid pre-remediation hash when target resource is accessible — hash is sha256-prefixed, 71 chars (regression guard) | Pass (pre-existing) |
| `UT-RO-545-006` | Remediation proceeds when target resource has no .spec — empty hash returned (regression guard) | Pass (pre-existing) |

### Tier 2: Helm Validation

**Testable code scope**: Helm chart templates for RO and EM — 2 new `ClusterRoleBinding` resources. Validated offline via `helm template` output assertions (no cluster required).

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HELM-545-001` | RO controller SA can read cluster resources via `view` ClusterRole (hash capture succeeds for cert-manager and other CRDs) | Pending |
| `IT-HELM-545-002` | EM controller SA can read cluster resources via `view` ClusterRole (post-remediation hash capture succeeds) | Pending |

### Tier Skip Rationale

- **E2E**: The cert-failure E2E scenario in the release pipeline covers the full end-to-end behavior. Adding a dedicated E2E test for this single function is disproportionate cost for incremental confidence.

---

## 6. Test Cases (Detail)

### UT-RO-545-001: Forbidden error yields empty hash (degraded soft-fail)

**BR**: Issue #545
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A REST mapper that resolves `Certificate` to `cert-manager.io/v1`, and a fake client built with `interceptor.Funcs` that returns `apierrors.NewForbidden(...)` on `Get`
**When**: `CapturePreRemediationHash` is called with kind=`Certificate`, name=`demo-app-cert`, namespace=`demo-cert-failure`
**Then**: Returns `("", "Forbidden: ...", nil)` — empty hash, non-empty degradedReason, no error

**Acceptance Criteria**:
- `err` is `nil` (not a hard error)
- `hash` is `""` (empty string)
- `degradedReason` is non-empty and contains `"Forbidden"` or describes the access denial
- The caller can use `degradedReason` to emit a K8s Event

---

### UT-RO-545-002: K8s InternalError yields empty hash (degraded soft-fail)

**BR**: Issue #545
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A REST mapper that resolves `Deployment` to `apps/v1`, and a fake client built with `interceptor.Funcs` that returns `apierrors.NewInternalError(fmt.Errorf("etcd timeout"))` on `Get`
**When**: `CapturePreRemediationHash` is called with kind=`Deployment`, name=`failing-deploy`, namespace=`default`
**Then**: Returns `("", "failed to fetch ...", nil)` — empty hash, non-empty degradedReason, no error

**Acceptance Criteria**:
- `err` is `nil`
- `hash` is `""` (empty string)
- `degradedReason` is non-empty and describes the fetch failure
- Transient API errors do not permanently fail the RR

---

### UT-RO-545-003: NotFound still returns empty hash (regression guard)

**BR**: DD-EM-002
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A REST mapper that resolves `Deployment` to `apps/v1`, and a fake client with no matching resource
**When**: `CapturePreRemediationHash` is called with kind=`Deployment`, name=`nonexistent`, namespace=`default`
**Then**: Returns `("", "", nil)` — empty hash, empty degradedReason, no error

**Acceptance Criteria**:
- `err` is `nil`
- `hash` is `""` (empty string)
- `degradedReason` is `""` (NotFound is a legitimate no-hash case, not degraded)
- Existing behavior for NotFound is preserved

**Note**: Pre-existing test. Updated for 3-tuple return. Re-validated as regression guard.

---

### UT-RO-545-004: Unknown Kind returns empty hash

**BR**: DD-EM-002
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A REST mapper that does NOT map `UnknownCRD`
**When**: `CapturePreRemediationHash` is called with kind=`UnknownCRD`, name=`something`, namespace=`default`
**Then**: Returns `("", "", nil)` — empty hash, empty degradedReason, no error

**Acceptance Criteria**:
- `err` is `nil`
- `hash` is `""` (empty string)
- `degradedReason` is `""` (unknown GVK is a legitimate no-hash case, not degraded)
- GVK resolution failure is non-fatal

---

### UT-RO-545-005: Successful hash capture (regression guard)

**BR**: DD-EM-002
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A REST mapper that resolves `Deployment` to `apps/v1`, and a fake client with an existing Deployment with a `.spec`
**When**: `CapturePreRemediationHash` is called
**Then**: Returns `("sha256:...", "", nil)` — valid hash, empty degradedReason, no error

**Acceptance Criteria**:
- `err` is `nil`
- `hash` starts with `sha256:` and has length 71 (sha256: prefix + 64 hex chars)
- `degradedReason` is `""` (success is not degraded)
- Existing behavior for successful hash capture is preserved

**Note**: Pre-existing test. Updated for 3-tuple return. Re-validated as regression guard.

---

### UT-RO-545-006: No .spec field returns empty hash (regression guard)

**BR**: DD-EM-002
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A REST mapper that resolves `ConfigMap` to `v1`, and a fake client with an existing ConfigMap (which has no `.spec`)
**When**: `CapturePreRemediationHash` is called with kind=`ConfigMap`, name=`test-config`, namespace=`default`
**Then**: Returns `("", "", nil)` — empty hash, empty degradedReason, no error

**Acceptance Criteria**:
- `err` is `nil`
- `hash` is `""` (empty string)
- `degradedReason` is `""` (no .spec is a legitimate no-hash case, not degraded)
- Resources without `.spec` are handled gracefully

**Note**: Pre-existing test. Updated for 3-tuple return. Re-validated as regression guard.

---

### IT-HELM-545-001: RO ClusterRoleBinding to `view`

**BR**: Issue #545
**Type**: Helm validation (offline)
**Validation**: `helm template` output parsed with grep/yq

**Given**: Default Helm values
**When**: `helm template kubernaut charts/kubernaut/` renders the RO template
**Then**: Output includes a `ClusterRoleBinding` named `remediationorchestrator-view` binding the `remediationorchestrator-controller` SA to the `view` ClusterRole

**Acceptance Criteria**:
- `kind: ClusterRoleBinding` with `name: remediationorchestrator-view` exists
- `roleRef.kind: ClusterRole`, `roleRef.name: view`
- `subjects[0].kind: ServiceAccount`, `subjects[0].name: remediationorchestrator-controller`
- `subjects[0].namespace` is set to `{{ .Release.Namespace }}`

---

### IT-HELM-545-002: EM ClusterRoleBinding to `view`

**BR**: Issue #545
**Type**: Helm validation (offline)
**Validation**: `helm template` output parsed with grep/yq

**Given**: Default Helm values
**When**: `helm template kubernaut charts/kubernaut/` renders the EM template
**Then**: Output includes a `ClusterRoleBinding` named `effectivenessmonitor-view` binding the `effectivenessmonitor-controller` SA to the `view` ClusterRole

**Acceptance Criteria**:
- `kind: ClusterRoleBinding` with `name: effectivenessmonitor-view` exists
- `roleRef.kind: ClusterRole`, `roleRef.name: view`
- `subjects[0].kind: ServiceAccount`, `subjects[0].name: effectivenessmonitor-controller`
- `subjects[0].namespace` is set to `{{ .Release.Namespace }}`

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **K8s Client**: `controller-runtime/pkg/client/fake` with `interceptor.Funcs` to simulate `Forbidden` and `InternalError` responses. This is an established project pattern (see `test/unit/signalprocessing/ownerchain_builder_test.go`, `test/unit/gateway/processing/crd_creator_retry_test.go`).
- **Error Construction**: `apierrors.NewForbidden(...)` for Forbidden, `apierrors.NewInternalError(...)` for InternalError.
- **Location**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

### Helm Validation

- **Method**: Offline `helm template` rendering with shell assertions
- **Tooling**: `helm template kubernaut charts/kubernaut/` piped to grep or yq
- **Location**: Validated during implementation and in CI Helm lint step
- **No cluster required**: `helm template` is purely offline

---

## 8. Execution

```bash
# Unit tests (all RO hash tests)
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="CapturePreRemediationHash"

# Specific test by ID
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-545-001"

# All new tests only
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-545-00[124]"

# Helm template validation (RO)
helm template kubernaut charts/kubernaut/ | grep -A 12 "remediationorchestrator-view"

# Helm template validation (EM)
helm template kubernaut charts/kubernaut/ | grep -A 12 "effectivenessmonitor-view"
```

---

## 9. Risk Mitigation Log

| Risk | Mitigation | Status |
|------|-----------|--------|
| Hash capture fails → EA non-functional | PRIMARY: `view` ClusterRole binding gives both RO and EM broad read access. CRDs with aggregation labels (cert-manager, Istio) are automatically covered. | Addressed in IT-HELM-545-001/002 |
| CRDs without `view` aggregation labels → hash still fails | DEFENSE-IN-DEPTH: Soft-fail on `reader.Get()` errors prevents pipeline failure. Warning log + K8s Event alerts operator to add explicit RBAC. | Addressed in UT-RO-545-001/002 |
| Forbidden logged at debug level → operator misses RBAC gap | Forbidden logged at Warning/Info level (not V(1)/debug). K8s Event emitted so it appears in `kubectl describe rr`. | Addressed in implementation spec |
| Soft-fail masks data corruption | Soft-fail scope limited to `reader.Get()` errors only. `NestedMap` and `CanonicalSpecHash` errors remain hard errors — these indicate actual data problems. | Documented in Design Decisions |
| Fake client cannot simulate Forbidden | Project already uses `interceptor.Funcs` pattern with `apierrors.NewForbidden()` in 5+ test files. Proven infrastructure. | Addressed in Test Infrastructure |
| Pre-existing tests break during refactor | UT-RO-545-003/005/006 are regression guards with "Pass (pre-existing)" status. They validate unchanged behavior and should pass throughout. | Tracked in BR Coverage Matrix |
| `CanonicalSpecHash` error path untested (path 7) | Excluded: requires crafting data that passes `NestedMap` but fails JSON marshal. Low-value, fragile test. 6/7 paths covered = 86% > 80% threshold. | Documented in Coverage Policy |

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
| 1.1 | 2026-03-04 | Risk mitigation update: (1) Reframed `view` ClusterRole as PRIMARY fix — hash capture is critical for EA functionality; (2) Soft-fail is defense-in-depth, not default path; (3) Added Warning-level logging for Forbidden errors; (4) Scoped soft-fail to `reader.Get()` errors only — `NestedMap`/`CanonicalSpecHash` remain hard errors; (5) Added UT-RO-545-006 for no-.spec regression guard; (6) Marked pre-existing tests as "Pass (pre-existing)"; (7) Fixed K8s API error terminology (InternalError, not HTTP 500); (8) Specified `interceptor.Funcs` as the established project pattern; (9) Clarified Helm validation is offline `helm template` rendering; (10) Added Risk Mitigation Log (Section 9) |
| 1.2 | 2026-03-04 | Signature change: (1) `CapturePreRemediationHash` returns 3-tuple `(hash, degradedReason, err)` so callers can emit K8s Events on degraded hash capture; (2) Updated all acceptance criteria to validate `degradedReason`; (3) Added UT-RO-214-010 behavior change to design decisions; (4) Blast radius: 2 production callers + 12 test call sites + 1 behavior test; (5) Created #546 for v1.2 notification enrichment when EA is degraded |
