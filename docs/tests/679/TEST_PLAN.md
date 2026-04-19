# Test Plan: KA LabelDetector — Support Non-Workload Root Owner Kinds

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-679-v1.0
**Feature**: Extend KA LabelDetector to support ConfigMap, Secret, Service, and Node as root owner kinds, and fix incomplete Helm/GitOps label detection
**Version**: 1.0
**Created**: 2026-04-12
**Author**: AI Agent
**Status**: Draft
**Branch**: `fix/679-ka-label-detector-gvr`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for Issue #679 — the Kubernaut Agent (KA) `LabelDetector.fetchResource()` fails for non-workload root owner kinds (ConfigMap, Secret, Service, Node) because `knownWorkloadGVRs` only contains workload GVRs. This causes ALL 8 detection categories to be marked as `failedDetections`, preventing `helmManaged`, `gitOpsManaged`, and all other labels from being surfaced to the LLM prompt and DataStorage scoring.

This is the v1.3 (Go/KA) equivalent of the v1.2 (Python/HAPI) Issue #676, which was fixed and released in v1.2.1.

### 1.2 Objectives

1. **GVR coverage**: `fetchResource()` successfully fetches ConfigMap, Secret, Service, and Node root owners
2. **Helm detection parity**: `detectHelm()` detects Helm management via both `app.kubernetes.io/managed-by: Helm` AND `helm.sh/chart` labels (aligned with HAPI fix)
3. **GitOps detection parity**: `detectGitOps()` detects GitOps management via labels (ArgoCD `argocd.argoproj.io/instance`, Flux `fluxcd.io/sync-gc-mark`) in addition to existing annotation checks
4. **Zero regressions**: All existing label detection tests continue to pass unchanged
5. **Business continuity**: The `crashloop-helm` demo scenario (ConfigMap root owner with Helm labels) produces `helmManaged=true` in the KA investigation pipeline

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/enrichment/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `internal/kubernautagent/enrichment/label_detector.go` |
| Backward compatibility | 0 regressions | Existing `detected_labels_test.go` (UT-KA-433-DL-*) pass without modification |
| HAPI parity | F1+F2+F3 fixed | ConfigMap Helm detection, `helm.sh/chart`, GitOps label detection |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-KA-250**: Detected labels must include `helmManaged=true` when the root owner resource has Helm management labels
- **BR-KA-251**: Detected labels must include `gitOpsManaged=true` when the root owner resource has GitOps management labels/annotations
- **ADR-056 v1.7**: Label detection runs in EnrichmentService Phase 2
- **DD-HAPI-018 v1.4**: 8 detection characteristics (cross-language contract)
- **Issue #679**: KA LabelDetector.fetchResource() does not support ConfigMap and other core/v1 resource kinds
- **Issue #676**: HAPI does not surface helmManaged=true (v1.2 equivalent, fixed in v1.2.1)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [HAPI #676 Test Plan](../676/TEST_PLAN.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Missing GVR for a kind causes total label detection failure (8 failed categories) | Critical — wrong workflow selection, WFE failure | High (confirmed bug) | UT-KA-679-001..004 | Replace hardcoded GVR map with generic REST mapper resolution |
| R2 | `detectHelm` only checks `managed-by`, misses `helm.sh/chart`-only resources | Medium — Helm detection false negative | Medium | UT-KA-679-005 | Add `helm.sh/chart` label check |
| R3 | `detectGitOps` only checks annotations, misses label-only GitOps signals | Medium — GitOps detection false negative | Medium | UT-KA-679-006..007 | Add label-based checks for ArgoCD/Flux |
| R4 | Regression in existing Deployment-based label detection | High — breaks working functionality | Low | UT-KA-433-DL-001..006 | Run existing test suite as regression gate |
| R5 | `NewLabelDetector` signature change breaks callers | Low — only 2 call sites (main.go, tests) | Very Low | All | Update both call sites in same commit |

### 3.1 Risk-to-Test Traceability

- **R1** (Critical): Mitigated by UT-KA-679-001 (ConfigMap), UT-KA-679-002 (Secret), UT-KA-679-003 (Service), UT-KA-679-004 (Node)
- **R2** (Medium): Mitigated by UT-KA-679-005
- **R3** (Medium): Mitigated by UT-KA-679-006, UT-KA-679-007
- **R4** (High): Mitigated by regression checkpoint (existing UT-KA-433-DL-* suite)

---

## 4. Scope

### 4.1 Features to be Tested

- **`fetchResource()` GVR map** (`internal/kubernautagent/enrichment/label_detector.go:31-39`): ConfigMap, Secret, Service, Node GVR resolution
- **`detectHelm()`** (`label_detector.go:133-141`): `helm.sh/chart` label detection
- **`detectGitOps()`** (`label_detector.go:112-131`): Label-based ArgoCD/Flux detection
- **End-to-end label flow**: ConfigMap root owner → `helmManaged=true` in `DetectedLabels`

### 4.2 Features Not to be Tested

- **Enricher wiring** (`enricher.go`): Already tested in existing enrichment_test.go; no changes needed
- **Investigator pipeline**: No changes to investigator.go; label propagation is tested at the enrichment boundary
- **HAPI Python code**: Separate codebase (v1.2), already fixed in v1.2.1
- **K8sAdapter owner chain resolution**: Uses REST mapper, already supports all kinds — no changes

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use REST mapper for generic GVR resolution (eliminate hardcoded map) | `K8sAdapter` already uses this pattern; eliminates the class of bug rather than patching one instance. Future-proof for any root owner kind (Ingress, PVC, etc.) |
| Pass `meta.RESTMapper` to `LabelDetector` constructor | Mapper is already available in `k8sInfra` at wiring time; minimal constructor change |
| Add `helm.sh/chart` AND label-based GitOps checks | Direct parity with the HAPI v1.2.1 fix; covers the same real-world scenarios |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `internal/kubernautagent/enrichment/label_detector.go` (pure logic: GVR lookup, label/annotation matching)
- **Integration**: Not applicable — `LabelDetector` is pure logic tested with `dynamicfake.NewSimpleDynamicClient`; no real K8s I/O
- **E2E**: Deferred — validated by existing KA E2E suite when deployed in Kind cluster

### 5.2 Two-Tier Minimum

- **Unit tests**: Cover all detection logic (GVR resolution, Helm detection, GitOps detection)
- **Integration tests**: The unit tests use `dynamicfake` which exercises the full `dynamic.Interface` contract, providing integration-equivalent assurance

### 5.3 Business Outcome Quality Bar

Each test validates a **business outcome**: "When the root owner is a ConfigMap with Helm labels, the LLM receives `helmManaged=true` in its prompt context."

### 5.4 Pass/Fail Criteria

**PASS** — all of:
1. All UT-KA-679-* tests pass (0 failures)
2. All existing UT-KA-433-DL-* tests pass (0 regressions)
3. Coverage on `label_detector.go` >=80%
4. `go build ./...` passes
5. `go vet ./...` passes

**FAIL** — any of:
1. Any UT-KA-679-* test fails
2. Any existing UT-KA-433-DL-* test regresses
3. Coverage on `label_detector.go` below 80%

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken, `dynamicfake` API incompatibility
**Resume**: Build fixed, dependency resolved

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/enrichment/label_detector.go` | `fetchResource`, `detectHelm`, `detectGitOps`, `DetectLabels` | ~296 |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `release/v1.3.0-rc2` HEAD | Branch for fix |
| HAPI reference fix | v1.2.1 (`release/v1.2.0`) | Pattern reference |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-KA-250 | Helm detection for ConfigMap root owner | P0 | Unit | UT-KA-679-001 | Pending |
| BR-KA-250 | Helm detection for Secret root owner | P0 | Unit | UT-KA-679-002 | Pending |
| BR-KA-250 | Label detection for Service root owner | P0 | Unit | UT-KA-679-003 | Pending |
| BR-KA-250 | Label detection for Node root owner | P1 | Unit | UT-KA-679-004 | Pending |
| BR-KA-250 | Helm detection via `helm.sh/chart` label only | P0 | Unit | UT-KA-679-005 | Pending |
| BR-KA-251 | GitOps detection via ArgoCD instance label | P0 | Unit | UT-KA-679-006 | Pending |
| BR-KA-251 | GitOps detection via Flux sync-gc-mark label | P0 | Unit | UT-KA-679-007 | Pending |
| BR-KA-250 | Full demo scenario: ConfigMap with all Helm labels | P0 | Unit | UT-KA-679-008 | Pending |
| BR-KA-250 | Regression: Deployment Helm detection unchanged | P0 | Unit | UT-KA-433-DL-004 | Pass (existing) |
| BR-KA-251 | Regression: Deployment GitOps detection unchanged | P0 | Unit | UT-KA-433-DL-001 | Pass (existing) |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-KA-679-{SEQUENCE}` (Unit Test, KA service, Issue 679)

### Tier 1: Unit Tests

**Testable code scope**: `internal/kubernautagent/enrichment/label_detector.go` (>=80% coverage target)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-679-001` | ConfigMap root owner with `managed-by: Helm` label → `helmManaged=true`, zero `failedDetections` | Pending |
| `UT-KA-679-002` | Secret root owner with `managed-by: Helm` label → `helmManaged=true`, zero `failedDetections` | Pending |
| `UT-KA-679-003` | Service root owner with no special labels → all detections run, zero `failedDetections` | Pending |
| `UT-KA-679-004` | Node (cluster-scoped) root owner with Istio annotation → `serviceMesh=istio`, zero `failedDetections` | Pending |
| `UT-KA-679-005` | Deployment with only `helm.sh/chart` label (no `managed-by`) → `helmManaged=true` | Pending |
| `UT-KA-679-006` | Deployment with ArgoCD `argocd.argoproj.io/instance` label → `gitOpsManaged=true`, `gitOpsTool=argocd` | Pending |
| `UT-KA-679-007` | Deployment with Flux `fluxcd.io/sync-gc-mark` label → `gitOpsManaged=true`, `gitOpsTool=flux` | Pending |
| `UT-KA-679-008` | ConfigMap with full demo-crashloop-helm labels → `helmManaged=true` (end-to-end scenario) | Pending |

### Tier Skip Rationale

- **Integration**: `LabelDetector` is pure logic; unit tests with `dynamicfake` provide equivalent assurance
- **E2E**: Deferred to KA E2E suite execution in Kind cluster (existing tests cover the full pipeline)

---

## 9. Test Cases

### UT-KA-679-001: ConfigMap root owner — Helm managed-by label

**BR**: BR-KA-250
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Preconditions**:
- `dynamicfake` client with a ConfigMap object in namespace `demo-crashloop-helm`
- ConfigMap has label `app.kubernetes.io/managed-by: Helm`

**Test Steps**:
1. **Given**: A ConfigMap `worker-config` in `demo-crashloop-helm` with `managed-by: Helm` label
2. **When**: `DetectLabels(ctx, "ConfigMap", "worker-config", "demo-crashloop-helm", nil)` is called
3. **Then**: `helmManaged=true`, `failedDetections` is empty

**Acceptance Criteria**:
- `result.HelmManaged == true`
- `len(result.FailedDetections) == 0`

### UT-KA-679-002: Secret root owner — Helm managed-by label

**BR**: BR-KA-250
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Test Steps**:
1. **Given**: A Secret `tls-cert` in `production` with `managed-by: Helm`
2. **When**: `DetectLabels(ctx, "Secret", "tls-cert", "production", nil)` is called
3. **Then**: `helmManaged=true`, `failedDetections` is empty

### UT-KA-679-003: Service root owner — no special labels

**BR**: BR-KA-250
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Test Steps**:
1. **Given**: A Service `api-gateway` in `default` with no special labels
2. **When**: `DetectLabels(ctx, "Service", "api-gateway", "default", nil)` is called
3. **Then**: All detection booleans are false, `failedDetections` is empty (proves fetch succeeded)

### UT-KA-679-004: Node root owner — cluster-scoped, Istio annotation

**BR**: BR-KA-250
**Priority**: P1
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Test Steps**:
1. **Given**: A Node `worker-1` with `sidecar.istio.io/inject: true` annotation
2. **When**: `DetectLabels(ctx, "Node", "worker-1", "", nil)` is called (empty namespace = cluster-scoped)
3. **Then**: `serviceMesh=istio`, `failedDetections` is empty

### UT-KA-679-005: Deployment with only helm.sh/chart label

**BR**: BR-KA-250
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Test Steps**:
1. **Given**: A Deployment `web-app` with only `helm.sh/chart: web-1.0.0` (no `managed-by`)
2. **When**: `DetectLabels(ctx, "Deployment", "web-app", "default", ...)` is called
3. **Then**: `helmManaged=true`

### UT-KA-679-006: Deployment with ArgoCD instance label (not annotation)

**BR**: BR-KA-251
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Test Steps**:
1. **Given**: A Deployment `web-deploy` with label `argocd.argoproj.io/instance: my-app`
2. **When**: `DetectLabels(ctx, "Deployment", "web-deploy", "default", ...)` is called
3. **Then**: `gitOpsManaged=true`, `gitOpsTool=argocd`

### UT-KA-679-007: Deployment with Flux sync-gc-mark label

**BR**: BR-KA-251
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Test Steps**:
1. **Given**: A Deployment `web-deploy` with label `fluxcd.io/sync-gc-mark: sha256:abc`
2. **When**: `DetectLabels(ctx, "Deployment", "web-deploy", "default", ...)` is called
3. **Then**: `gitOpsManaged=true`, `gitOpsTool=flux`

### UT-KA-679-008: Full demo-crashloop-helm scenario (ConfigMap + all Helm labels)

**BR**: BR-KA-250
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/detected_labels_679_test.go`

**Test Steps**:
1. **Given**: A ConfigMap `worker-config` in `demo-crashloop-helm` with labels:
   - `app.kubernetes.io/managed-by: Helm`
   - `app.kubernetes.io/instance: demo-crashloop-helm`
   - `helm.sh/chart: demo-crashloop-helm-0.1.0`
   - `app.kubernetes.io/name: demo-crashloop-helm`
2. **When**: `DetectLabels(ctx, "ConfigMap", "worker-config", "demo-crashloop-helm", nil)` is called
3. **Then**: `helmManaged=true`, `failedDetections` is empty

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `dynamicfake.NewSimpleDynamicClient` (K8s client fake — external dependency)
- **Location**: `test/unit/kubernautagent/enrichment/`
- **Scheme registration**: Must register `corev1` (ConfigMap, Secret, Service, Node) in addition to existing `appsv1`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all dependencies are available on `release/v1.3.0-rc2`.

### 11.2 Execution Order (TDD Phases)

1. **Phase 1 — Branch Setup**: Create `fix/679-ka-label-detector-gvr` from `release/v1.3.0-rc2`, verify baseline
2. **Phase 2 — TDD RED**: Write all 8 failing test cases
3. **Checkpoint 1**: Verify all 8 tests fail for the correct reasons, 0 regressions
4. **Phase 3 — TDD GREEN**: Implement minimal fixes (F1: replace map with REST mapper, F2: detectHelm, F3: detectGitOps)
5. **Checkpoint 2**: All 8 new tests pass, 0 regressions, >=80% coverage, adversarial audit
6. **Phase 4 — TDD REFACTOR**: Remove dead `knownWorkloadGVRs` map, update docstrings, clean up
7. **Checkpoint 3**: All tests pass, build clean, security audit, final review
8. **Phase 5 — PR & Merge**: Create PR, CI green, merge

### Checkpoint Criteria

Each checkpoint performs:

**Comprehensive Audit**:
- All tests pass (new + existing)
- `go build ./...` succeeds
- `go vet ./...` clean
- No new lint warnings

**Adversarial Audit**:
- Attempt to bypass detection with edge-case labels (empty strings, mixed case, partial keys)
- Verify `failedDetections` is only populated when K8s API errors occur, not for unsupported kinds
- Confirm detection precedence (annotation > label where applicable)

**Security Audit**:
- No secrets or credentials in test fixtures
- No privilege escalation in K8s resource definitions
- Dynamic client uses only GET operations (read-only)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/679/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/enrichment/detected_labels_679_test.go` | 8 Ginkgo BDD test cases |
| Coverage report | CI artifact | Per-tier coverage on `label_detector.go` |

---

## 13. Execution

```bash
# Run new tests only
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.v -ginkgo.focus="679"

# Run all label detection tests (new + regression)
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.v

# Coverage
go test ./test/unit/kubernautagent/enrichment/... \
  -coverprofile=coverage-679.out \
  -coverpkg=github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment/...
go tool cover -func=coverage-679.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | None | All fixes are additive — new GVR entries, new label checks. Existing detection paths unchanged. |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-12 | Initial test plan |
