# Test Plan: Cluster-Scoped Enrichment + Resource Fingerprint Unification

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-762-v1
**Feature**: Scope-aware enrichment pipeline, resource fingerprint generalization, namespace validation, enrichment deduplication
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/762-cluster-scoped-enrichment`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the combined fix for Issues #762, #763, #764, and #765. The enrichment pipeline incorrectly assumes all Kubernetes resources are namespace-scoped, causing API failures for cluster-scoped resources (Node, ClusterRole) that result in `rca_incomplete`. Additionally, the hash algorithm is limited to `.spec` fields, the investigator lacks namespace validation for cluster-scoped kinds, and enrichment is called redundantly.

### 1.2 Objectives

1. **Scope awareness (#762)**: K8sAdapter and LabelDetector use `RESTMapping.Scope` to correctly dispatch cluster-scoped vs namespaced API calls.
2. **DS empty namespace (#762)**: DataStorage handler accepts empty `targetNamespace` for cluster-scoped resources and the adapter surfaces 400 errors.
3. **Resource fingerprint (#765)**: `CanonicalResourceFingerprint` hashes all functional state (not just `.spec`), enabling fingerprinting for ConfigMap, ClusterRole, etc.
4. **Namespace validation (#763)**: Investigator forces `namespace=""` for cluster-scoped kinds before enrichment.
5. **Enrichment dedup (#764)**: Identical pre-RCA and post-RCA targets reuse cached enrichment results.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/...` |
| Integration test pass rate | 100% | `go test ./test/integration/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Backward compatibility | 0 regressions | Existing tests pass (hash values change — clean cut) |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-261: Enrichment pipeline for incident investigation
- DD-EM-002: Canonical spec hash for pre/post remediation comparison
- Issue #762: Enrichment pipeline fails for cluster-scoped resources
- Issue #763: Namespace injection validation
- Issue #764: Enrichment deduplication
- Issue #765: Hash algorithm unification / resource fingerprint generalization

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | RESTMapper returns wrong scope for CRD kinds | Cluster-scoped CRD treated as namespaced | Low | UT-KA-762-001..004 | resettableMapper retry already included |
| R2 | Hash value change breaks DS history lookup | No historical context for existing remediations | Medium | UT-HASH-765-001..008 | Clean cut accepted by user; no backwards compat |
| R3 | InjectRemediationTarget namespace fallback wrong for cluster-scoped | Remediation target has wrong namespace in CRD | Medium | UT-KA-762-010 | Explicit cluster-scope namespace clearing |
| R4 | DS 400 error surfaced breaks existing callers | Enrichment logs errors where it previously swallowed them silently | Low | UT-KA-762-008 | Enricher already handles sub-call errors gracefully |
| R5 | Dedup cache returns stale data if enrichment is non-deterministic | Stale enrichment in workflow selection | Low | UT-KA-764-001..003 | Cache is per-call; investigation is single-request |

---

## 4. Scope

### 4.1 Features to be Tested

- **K8sAdapter** (`internal/kubernautagent/enrichment/k8s_adapter.go`): Scope-aware `GetOwnerChain`, `GetSpecHash`, `resolveMapping`
- **LabelDetector** (`internal/kubernautagent/enrichment/label_detector.go`): Scope-aware `fetchResource`, skip namespace-scoped lists for cluster roots
- **DS handler** (`pkg/datastorage/server/remediation_history_handler.go`): Accept empty namespace
- **DS adapter** (`internal/kubernautagent/enrichment/ds_adapter.go`): Surface 400 errors
- **InjectRemediationTarget** (`internal/kubernautagent/investigator/investigator.go`): Cluster-scope namespace fix
- **CanonicalResourceFingerprint** (`pkg/shared/hash/canonical.go`): Functional state hashing
- **CompositeResourceFingerprint** (`pkg/shared/hash/configmap.go`): Renamed composite hash
- **CapturePreRemediationHash** (`internal/controller/remediationorchestrator/reconciler.go`): Uses fingerprint
- **getTargetFunctionalState** (`internal/controller/effectivenessmonitor/target_resources.go`): Returns functional state + spec
- **ScopeResolver** (`internal/kubernautagent/investigator/investigator.go`): Namespace forcing
- **Enrichment cache** (`internal/kubernautagent/investigator/investigator.go`): Per-call dedup

### 4.2 Features Not to be Tested

- **Secret cascading**: Explicitly excluded per user directive (Vault-managed, rotational)
- **SignalProcessing owner chain**: Uses static `isClusterScoped` map; separate concern
- **E2E tests**: Deferred; unit + integration provide >=80% coverage

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Delete `CanonicalSpecHash` (no deprecation) | Clean cut; no backwards compatibility per user directive |
| Dedup cache as local variable, not struct field | Thread safety: `Investigate()` runs concurrently on shared `Investigator` |
| `ExtractConfigMapRefs` still operates on `.spec` | Needs pod template structure which lives under `.spec`; independent from fingerprint |
| No Secret cascading in fingerprint | Secrets are Vault-managed, rotational, not functional configuration state |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (hash functions, scope helpers, adapter logic, investigator enrichment paths)
- **Integration**: >=80% of integration-testable code (DS handler, enrichment wiring, RO/EM hash capture)
- **E2E**: Deferred (container contract validated by unit + integration)

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers (UT + IT).

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass, per-tier coverage >=80%, no regressions in existing test suites.
**FAIL**: Any P0 test fails, coverage below 80%, or existing tests regress.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/hash/canonical.go` | `CanonicalResourceFingerprint`, `normalizeValue` | ~50 |
| `pkg/shared/hash/configmap.go` | `CompositeResourceFingerprint`, `ExtractConfigMapRefs` | ~225 |
| `internal/kubernautagent/enrichment/k8s_adapter.go` | `resolveMapping`, `GetOwnerChain`, `GetSpecHash` | ~187 |
| `internal/kubernautagent/enrichment/label_detector.go` | `fetchResource`, `resolveMapping`, `detectHPA/PDB/NP/RQ` | ~322 |
| `internal/kubernautagent/enrichment/ds_adapter.go` | `GetRemediationHistory` | ~237 |
| `internal/kubernautagent/investigator/investigator.go` | `InjectRemediationTarget`, `ResolveEnrichmentTarget`, `Investigate` | ~959 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/remediation_history_handler.go` | `HandleGetRemediationHistoryContext` | ~259 |
| `internal/controller/remediationorchestrator/reconciler.go` | `CapturePreRemediationHash`, `resolveConfigMapHashes` | ~125 |
| `internal/controller/effectivenessmonitor/target_resources.go` | `getTargetFunctionalState`, `resolveConfigMapHashes` | ~257 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-261 | Enrichment for cluster-scoped resources | P0 | Unit | UT-KA-762-001..006 | Pending |
| BR-HAPI-261 | Enrichment for cluster-scoped resources | P0 | Integration | IT-KA-762-001..003 | Pending |
| BR-HAPI-261 | DS accepts empty namespace | P0 | Unit | UT-KA-762-007..009 | Pending |
| BR-HAPI-261 | InjectRemediationTarget cluster fix | P0 | Unit | UT-KA-762-010..011 | Pending |
| BR-EM-004 | Resource fingerprint unification | P0 | Unit | UT-HASH-765-001..008 | Pending |
| BR-EM-004 | RO/EM fingerprint consumers | P0 | Unit | UT-HASH-765-009..012 | Pending |
| BR-HAPI-261 | Namespace injection validation | P1 | Unit | UT-KA-763-001..004 | Pending |
| BR-HAPI-261 | Enrichment deduplication | P1 | Unit | UT-KA-764-001..003 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

#### Phase 1 — Core Scope Fix (#762)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-762-001` | K8sAdapter.GetOwnerChain uses cluster-scoped client for Node kind (no namespace) | Pending |
| `UT-KA-762-002` | K8sAdapter.GetOwnerChain uses namespaced client for Deployment kind | Pending |
| `UT-KA-762-003` | K8sAdapter.GetSpecHash uses cluster-scoped client for Node kind | Pending |
| `UT-KA-762-004` | K8sAdapter.GetSpecHash uses namespaced client for Deployment kind | Pending |
| `UT-KA-762-005` | LabelDetector.fetchResource uses scope-aware client dispatch | Pending |
| `UT-KA-762-006` | LabelDetector skips HPA/PDB/NP/RQ list when rootNS is empty (cluster-scoped root) | Pending |
| `UT-KA-762-007` | DS handler accepts empty targetNamespace and returns 200 | Pending |
| `UT-KA-762-008` | DS adapter returns error on 400 Bad Request (not empty result) | Pending |
| `UT-KA-762-009` | DS handler still rejects empty targetKind with 400 | Pending |
| `UT-KA-762-010` | InjectRemediationTarget sets namespace="" for cluster-scoped enrichment | Pending |
| `UT-KA-762-011` | InjectRemediationTarget preserves namespace for namespaced enrichment (no regression) | Pending |

#### Phase 2 — Resource Fingerprint (#765)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HASH-765-001` | CanonicalResourceFingerprint strips metadata/status/apiVersion/kind | Pending |
| `UT-HASH-765-002` | CanonicalResourceFingerprint produces stable hash for Deployment object | Pending |
| `UT-HASH-765-003` | CanonicalResourceFingerprint captures ConfigMap .data/.binaryData | Pending |
| `UT-HASH-765-004` | CanonicalResourceFingerprint captures ClusterRole .rules | Pending |
| `UT-HASH-765-005` | CanonicalResourceFingerprint is idempotent (1000 iterations) | Pending |
| `UT-HASH-765-006` | CanonicalResourceFingerprint map-order and slice-order independent | Pending |
| `UT-HASH-765-007` | CompositeResourceFingerprint identity when no ConfigMap hashes | Pending |
| `UT-HASH-765-008` | CompositeResourceFingerprint includes ConfigMap content in digest | Pending |
| `UT-HASH-765-009` | CapturePreRemediationHash uses CanonicalResourceFingerprint | Pending |
| `UT-HASH-765-010` | EM getTargetFunctionalState returns functional state + spec separately | Pending |
| `UT-HASH-765-011` | EM handleSpecDrift uses CanonicalResourceFingerprint | Pending |
| `UT-HASH-765-012` | K8sAdapter.GetSpecHash uses CanonicalResourceFingerprint | Pending |

#### Phase 3 — Namespace Validation (#763)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-763-001` | ScopeResolver.IsClusterScoped returns true for Node | Pending |
| `UT-KA-763-002` | ScopeResolver.IsClusterScoped returns false for Deployment | Pending |
| `UT-KA-763-003` | Investigate forces namespace="" for cluster-scoped signal kind | Pending |
| `UT-KA-763-004` | Investigate preserves namespace for namespaced signal kind | Pending |

#### Phase 4 — Enrichment Deduplication (#764)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-764-001` | Enrichment called once when pre/post RCA targets identical | Pending |
| `UT-KA-764-002` | Enrichment called twice when post-RCA target differs from signal | Pending |
| `UT-KA-764-003` | Cache is per-call (not shared across concurrent investigations) | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-762-001` | Full enrichment pipeline succeeds for Node (cluster-scoped, real mapper) | Pending |
| `IT-KA-762-002` | Full enrichment pipeline succeeds for Deployment (namespaced, real mapper) | Pending |
| `IT-KA-762-003` | DS handler integration: empty namespace query returns valid response | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. The scope fix is in pure Go library code testable at UT+IT level. E2E would require Kind cluster with Node resources, which adds infrastructure cost without proportional coverage gain.

---

## 9. Test Cases

### UT-KA-762-001: K8sAdapter cluster-scoped GetOwnerChain

**BR**: BR-HAPI-261
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/enrichment/k8s_adapter_test.go`

**Test Steps**:
1. **Given**: A fake dynamic client with a Node resource at cluster scope; a fake RESTMapper that maps Node to `RESTScopeRoot`
2. **When**: `GetOwnerChain(ctx, "Node", "worker-1", "kube-system")` is called (namespace passed but should be ignored)
3. **Then**: The adapter uses the cluster-scoped client (no `.Namespace()` call), successfully retrieves the Node

### UT-HASH-765-001: CanonicalResourceFingerprint strips non-functional keys

**BR**: BR-EM-004
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/hash/canonical_test.go`

**Test Steps**:
1. **Given**: A full K8s object map with `apiVersion`, `kind`, `metadata`, `status`, and `spec` keys
2. **When**: `CanonicalResourceFingerprint(obj)` is called
3. **Then**: Hash is computed from `{spec: ...}` only (metadata/status/apiVersion/kind stripped). Adding/changing metadata does not change hash. Removing spec does change hash.

### UT-KA-763-003: Investigate forces namespace for cluster-scoped

**BR**: BR-HAPI-261
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/investigator_test.go`

**Test Steps**:
1. **Given**: A signal with `ResourceKind=Node`, `Namespace=kube-system`; ScopeResolver returns `IsClusterScoped=true` for Node
2. **When**: `Investigate()` is called
3. **Then**: The enricher receives `namespace=""` (not "kube-system")

### UT-KA-764-001: Enrichment dedup cache hit

**BR**: BR-HAPI-261
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/investigator_test.go`

**Test Steps**:
1. **Given**: A signal where pre-RCA and post-RCA resolve to the same (kind, name, namespace)
2. **When**: `Investigate()` completes
3. **Then**: `Enricher.Enrich()` is called exactly once

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: Fake dynamic client (`fake.NewSimpleDynamicClient`), fake RESTMapper, mock LLM client, mock DS client
- **Location**: `test/unit/kubernautagent/enrichment/`, `test/unit/shared/hash/`, `test/unit/kubernautagent/investigator/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO mocks. Real DS handler via httptest, real RESTMapper via envtest or discovery
- **Location**: `test/integration/kubernautagent/enrichment/`, `test/integration/datastorage/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1**: Core scope fix tests (UT-KA-762-*), then implementation
2. **Phase 2**: Fingerprint tests (UT-HASH-765-*), then implementation, then existing test updates
3. **Phase 3**: Namespace validation tests (UT-KA-763-*), then implementation
4. **Phase 4**: Dedup tests (UT-KA-764-*), then implementation

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/762/TEST_PLAN.md` | Strategy and test design |
| Unit test suite (Phase 1) | `test/unit/kubernautagent/enrichment/` | Scope-aware adapter/detector tests |
| Unit test suite (Phase 2) | `test/unit/shared/hash/` | Fingerprint tests |
| Unit test suite (Phase 3+4) | `test/unit/kubernautagent/investigator/` | ScopeResolver + dedup tests |
| Integration test suite | `test/integration/kubernautagent/enrichment/` | Full enrichment pipeline tests |

---

## 13. Execution

```bash
# Unit tests (all phases)
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.v
go test ./test/unit/shared/hash/... -ginkgo.v
go test ./test/unit/kubernautagent/investigator/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/enrichment/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.focus="UT-KA-762"

# Coverage
go test ./test/unit/kubernautagent/enrichment/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/shared/hash/canonical_test.go` | Tests `CanonicalSpecHash` with bare spec maps | Rewrite for `CanonicalResourceFingerprint` with full objects | Function deleted in #765 |
| `test/unit/shared/hash/configmap_test.go` | Tests `CompositeSpecHash` | Rename to `CompositeResourceFingerprint` | Function renamed in #765 |
| `test/unit/remediationorchestrator/pre_remediation_hash_test.go` | Asserts specific hash values from `CapturePreRemediationHash` | Update expected hash values | Algorithm change (functional state vs .spec only) |
| `test/unit/effectivenessmonitor/hash_test.go` | Tests `(*computer).Compute` with spec-only input | Update input shape and expected hashes | Algorithm change |
| `test/unit/effectivenessmonitor/reconciler_spec_drift_test.go` | Tests `handleSpecDrift` assertions | Update expected hashes | Algorithm change |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan covering all 4 issues |
