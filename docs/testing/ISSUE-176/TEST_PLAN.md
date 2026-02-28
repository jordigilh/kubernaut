# Test Plan: CRD Namespace Consolidation and Watch Restriction (Issue #176)

**Feature**: Consolidate all kubernaut CRDs into the controller namespace and restrict CRD type watches
**Version**: 1.0
**Created**: 2026-02-24
**Author**: AI Assistant
**Status**: Implemented

**Authority**:
- [Issue #176](https://github.com/jordigilh/kubernaut/issues/176): All CRD controllers: CRD namespace consolidation and watch restriction
- [ADR-057](../../architecture/decisions/ADR-057-crd-namespace-consolidation.md): CRD Namespace Consolidation
- [BR-SCOPE-001](../../requirements/BR-SCOPE-001-resource-scope-management.md): Resource Scope Management
- [BR-SCOPE-010](../../requirements/BR-SCOPE-010-ro-routing-validation.md): RO Routing Validation

**Cross-References**:
- Testing Strategy: `.cursor/rules/03-testing-strategy.mdc`
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [ADR-053](../../architecture/decisions/ADR-053-resource-scope-management.md): Resource Scope Management Architecture

---

## 1. Scope

### In Scope

- **Phase 2**: `GetControllerNamespace()` helper -- environment variable and service account file discovery
- **Phase 3**: Gateway CRD creation -- RR created in controller namespace instead of signal namespace
- **Phase 4**: RO scope bug fix -- `CheckUnmanagedResource` uses `rr.Spec.TargetResource.Namespace`
- **Phase 5**: CRD watch restriction -- `Cache.ByObject` scoping per controller
- **Phase 6**: Integration tests for namespace consolidation and watch restriction
- **Existing test updates**: ~45 assertions across ~15 files that assert CRD namespace == signal namespace

### Out of Scope

- E2E tests in a Kind cluster (existing E2E tests will be updated for namespace assertions only)
- RBAC Helm chart changes (documented but not tested at the code level)
- Gateway scope validation logic (unchanged -- already tested under BR-SCOPE-002)
- Non-CRD workload resource reads/watches (unchanged -- cluster-wide access preserved)

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit tier**: Covers namespace discovery helper, CRDCreator namespace usage, RO scope bug fix. Target: >=80% of new unit-testable code.
- **Integration tier**: Covers Cache.ByObject restrictions, cross-namespace CRD visibility, Gateway end-to-end CRD creation flow. Target: >=80% of new integration-testable code.

### Business Outcome Quality Bar

Tests validate **business outcomes**:

- **Security**: CRDs in workload namespaces are invisible to controllers (watch restriction)
- **Correctness**: CRDs are created in the controller namespace, not the signal namespace
- **Bug fix**: RO scope check evaluates the target resource namespace, not the CRD namespace
- **Discovery**: Controller namespace is resolved from env var or service account file

### Anti-Pattern Compliance

- **NO time.Sleep()**: Use Eventually/Consistently for async assertions
- **NO Skip()**: All tests run unconditionally
- **NO XIt/Pending**: All tests are implemented
- **NO `any`/`interface{}`**: Typed assertions throughout
- **NO null-checking / structure validation**: No `ToNot(BeNil())` existence checks as the sole assertion
- **NO constructor/framework testing**: Tests validate business behavior, not constructor guards or wiring
- **NO HTTP in integration tests**: Integration tests use direct business logic calls
- **NO audit/metrics infrastructure testing**: Verify business outcomes, not infrastructure

---

## 3. Unit Tests

### 3A. Namespace Discovery Helper

**File**: `test/unit/shared/scope/namespace_test.go`
**Source**: `pkg/shared/scope/namespace.go`

| Test ID | Scenario | Category | Expected Outcome |
|---------|----------|----------|------------------|
| UT-NS-057-001 | Env var `KUBERNAUT_CONTROLLER_NAMESPACE` is set | Happy Path | Returns env var value |
| UT-NS-057-002 | Env var value has leading/trailing whitespace | Edge Case | Returns trimmed value |
| UT-NS-057-003 | Env var is set to empty string | Error | Returns error containing "empty" |
| UT-NS-057-004 | Env var unset, SA file exists with valid content | Happy Path | Returns file content |
| UT-NS-057-005 | SA file content has whitespace/newlines | Edge Case | Returns trimmed content |
| UT-NS-057-006 | Env var unset, SA file does not exist | Error | Returns error containing "controller namespace" |
| UT-NS-057-007 | SA file exists but is empty | Error | Returns error containing "empty" |
| UT-NS-057-008 | Both env var and SA file available | Precedence | Returns env var (takes priority) |

**Status**: IMPLEMENTED (8/8 passing)

---

### 3B. Gateway CRD Creation Namespace

**File**: `test/unit/gateway/processing/crd_creation_business_test.go` (new tests added to existing file)
**Source**: `pkg/gateway/processing/crd_creator.go`

| Test ID | Scenario | Category | Expected Outcome |
|---------|----------|----------|------------------|
| UT-GW-057-001 | Signal from workload NS produces CRD in controller NS | Happy Path | `rr.Namespace == controllerNamespace` (security boundary enforced) |
| UT-GW-057-002 | Target resource namespace preserved for downstream controllers | Happy Path | `rr.Spec.TargetResource.Namespace == signal.Resource.Namespace` (accuracy for enrichment/health checks) |
| UT-GW-057-003 | Duplicate signal returns existing CRD from controller namespace | Edge Case | Existing RR returned with `rr.Namespace == controllerNamespace`, no error (dedup correctness) |

**Removed**: ~~UT-GW-057-004 (constructor panic on empty namespace)~~ -- Tests a developer safety guard (constructor validation), not a business outcome. Constructor panics are programming error guards, not runtime business scenarios per TESTING_GUIDELINES.md "Don't Use Unit Tests For" section.

---

### 3C. RO Scope Bug Fix

**File**: `test/unit/remediationorchestrator/routing/scope_blocking_test.go` (new tests added to existing file)
**Source**: `pkg/remediationorchestrator/routing/blocking.go`

| Test ID | Scenario | Category | Expected Outcome |
|---------|----------|----------|------------------|
| UT-RO-057-001 | RR in kubernaut-system, target in managed workload NS | Bug Fix | `IsManaged` called with target NS, returns managed=true |
| UT-RO-057-002 | RR in kubernaut-system, target in unmanaged workload NS | Bug Fix | `IsManaged` called with target NS, returns managed=false |
| UT-RO-057-003 | RR in kubernaut-system, cluster-scoped target (Node) | Bug Fix | `IsManaged` called with empty NS, checks resource label only |

---

### 3D. Existing Unit Test Updates (Assertion Changes Only)

These are not new tests -- they are existing tests whose namespace assertions change from `signal.Namespace` to `controllerNamespace`.

**File**: `test/unit/gateway/processing/crd_creation_business_test.go`

| Original Assertion | Updated Assertion | Count |
|--------------------|-------------------|-------|
| `Expect(rr.Namespace).To(Equal(testNamespace))` | `Expect(rr.Namespace).To(Equal(controllerNS))` | ~3 |
| `Expect(rr.Spec.TargetResource.Namespace).To(Equal(testNamespace))` | Unchanged (target NS stays as signal NS) | 0 changes |

**File**: `test/unit/gateway/crd_metadata_test.go`

| Original Assertion | Updated Assertion | Count |
|--------------------|-------------------|-------|
| `Expect(rr.Namespace).To(Equal("production"))` | `Expect(rr.Namespace).To(Equal(controllerNS))` | ~2 |

**File**: `test/unit/remediationorchestrator/*_creator_test.go`

| Original Assertion | Updated Assertion | Count |
|--------------------|-------------------|-------|
| Fixture RR `Namespace: "production"` used for child CRD NS | Fixture RR `Namespace: "kubernaut-system"` | ~20 |

---

## 4. Integration Tests

### 4A. CRD Namespace Consolidation

**File**: `test/integration/gateway/crd_namespace_consolidation_integration_test.go` (new)
**Source**: Gateway end-to-end CRD creation flow

| Test ID | Scenario | Category | Expected Outcome |
|---------|----------|----------|------------------|
| IT-GW-057-001 | Signal from "production" NS creates RR in controller NS | Happy Path | `rr.Namespace == controllerNS`, `rr.Spec.TargetResource.Namespace == "production"` (security + accuracy) |
| IT-GW-057-002 | Signals from different workload NSes all produce CRDs in same controller NS | Happy Path | CRD namespace is deterministic regardless of signal origin (correctness) |
| IT-GW-057-003 | Second signal with same fingerprint is deduplicated against CRD in controller NS | Happy Path | Dedup returns existing RR, occurrence count incremented (business behavior preserved after namespace change) |

### 4B. CRD Watch Restriction (Cache.ByObject)

**File**: `test/integration/controller/crd_watch_restriction_integration_test.go` (new)
**Source**: Controller cache configuration with envtest

| Test ID | Scenario | Category | Expected Outcome |
|---------|----------|----------|------------------|
| IT-CTRL-057-001 | CRD in controller NS triggers reconciliation | Happy Path | Reconciler invoked |
| IT-CTRL-057-002 | CRD in foreign NS is invisible to controller | Security | Reconciler NOT invoked (cache filtered) |
| IT-CTRL-057-003 | Workload resource in foreign NS still readable | Happy Path | Cross-namespace Get/List succeeds for Pods, Deployments |

### 4C. RO Scope Bug Fix Integration

**File**: `test/integration/remediationorchestrator/scope_namespace_fix_integration_test.go` (new)
**Source**: RO routing with cross-namespace RRs

| Test ID | Scenario | Category | Expected Outcome |
|---------|----------|----------|------------------|
| IT-RO-057-001 | RR in kubernaut-system for managed Pod in "production" | Bug Fix | RO routes RR (target NS is managed) |
| IT-RO-057-002 | RR in kubernaut-system for unmanaged Pod in "staging" | Bug Fix | RO blocks RR (target NS is unmanaged) |

### 4D. Existing Integration Test Updates (Assertion Changes Only)

| File | Changes | Count |
|------|---------|-------|
| `test/integration/gateway/05_multi_namespace_isolation_integration_test.go` | `crd.Namespace` assertions → controller NS | ~4 |
| `test/integration/gateway/06_concurrent_alerts_integration_test.go` | `crd.Namespace` assertion → controller NS | ~1 |
| `test/integration/gateway/10_crd_creation_lifecycle_integration_test.go` | `crd.Namespace` assertion → controller NS | ~1 |
| `test/integration/gateway/21_crd_lifecycle_integration_test.go` | Target NS assertions unchanged | ~0 |
| `test/integration/gateway/29_k8s_api_failure_integration_test.go` | `NewCRDCreator` call site updated | ~1 |

---

## 5. E2E Test Updates (Assertion Changes Only)

These are existing E2E tests whose CRD namespace assertions need updating. No new E2E tests are created for this issue.

| File | Changes | Count |
|------|---------|-------|
| `test/e2e/gateway/31_prometheus_adapter_test.go` | `crd.Namespace` assertions → controller NS | ~2 |
| `test/e2e/gateway/08_k8s_event_ingestion_test.go` | `crd.Namespace` assertion → controller NS | ~1 |
| `test/e2e/gateway/33_webhook_integration_test.go` | Target NS assertion unchanged | ~0 |

---

## 6. Coverage Targets

| Metric | Target | Tier |
|--------|--------|------|
| Namespace discovery helper (Phase 2) | 100% | Unit |
| Gateway CRD namespace change (Phase 3) | >=80% | Unit + Integration |
| RO scope bug fix (Phase 4) | >=80% | Unit + Integration |
| Cache.ByObject restriction (Phase 5) | >=80% | Integration |
| Overall new code coverage | >=80% across 2 tiers | Unit + Integration |

---

## 7. Test Execution Order (TDD)

Each phase follows strict RED-GREEN-REFACTOR:

1. **Phase 2** (DONE): UT-NS-057-001 through UT-NS-057-008 -- all 8 passing
2. **Phase 3** (DONE): UT-GW-057-001 through UT-GW-057-003 (RED) -> production code (GREEN) -> existing test updates (REFACTOR) -- all 3 passing
3. **Phase 4** (DONE): UT-RO-057-001 through UT-RO-057-003 (RED) -> bug fix (GREEN) -- all 3 passing
4. **Phase 5** (DONE): Cache.ByObject added to all 7 controller managers + Gateway cache
5. **Phase 6** (DEFERRED): Integration test env vars set. IT-GW-057 and IT-CTRL-057 integration tests deferred — require full envtest infrastructure. Gateway deduplication namespace bug discovered during triage and filed as [Issue #195](https://github.com/jordigilh/kubernaut/issues/195); IT-GW-057-003 will be addressed as part of that fix.

---

## 8. Risk Assessment

| Risk | Mitigation |
|------|------------|
| Large blast radius (~45 test assertions) | Mechanical changes; use search-replace patterns |
| envtest Cache.ByObject may behave differently than production | IT-CTRL-057-002 specifically validates filtering |
| RO creator tests coupled to `rr.Namespace` for child CRD placement | Verify RO uses `rr.Namespace` (now controller NS) consistently |
| Gateway dedup cache relies on RR namespace for lookups | IT-GW-057-003 validates dedup behavior preserved after namespace change |

---

## 9. Sign-off

| Role | Name | Date | Status |
|------|------|------|--------|
| Author | AI Assistant | 2026-02-24 | Draft |
| Reviewer | | | Pending |
| Approver | | | Pending |
