# Test Plan: HAPI Helm/GitOps Detection for Non-Workload Root Owners

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-676-v1.0
**Feature**: Fix Helm and GitOps label detection when the remediation target root owner is a non-workload resource (ConfigMap, Secret, Service, etc.)
**Version**: 1.0
**Created**: 2026-04-12
**Author**: AI Agent
**Status**: Active
**Branch**: `fix/v1.2.1-helm-detection`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for Issue #676: HAPI does not surface `helmManaged=true` when the remediation target's root owner is a non-workload Kubernetes resource (e.g., ConfigMap). The fix spans three interconnected components: the K8s client metadata resolver, the label detection closure, and the detection methods themselves. Tests provide behavioral assurance that the LLM receives correct cluster context for workflow selection.

### 1.2 Objectives

1. **Helm detection for non-Deployment root owners**: ConfigMap/Secret/Service resources with `app.kubernetes.io/managed-by: Helm` labels produce `helmManaged=True` in detected labels
2. **GitOps detection for non-Deployment root owners**: Non-workload resources with ArgoCD/Flux labels produce `gitOpsManaged=True`
3. **K8s client extensibility**: `_get_resource_metadata_sync` supports ConfigMap, Secret, Service, Job resource kinds without returning `None`
4. **Zero regressions**: All 831 existing unit tests continue to pass without modification

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| New test pass rate | 100% (10/10) | `make test-unit-holmesgpt-api` |
| Existing test pass rate | 100% (831/831) | `make test-unit-holmesgpt-api` |
| Unit-testable code coverage | >=75.53% (baseline) | Coverage report from test run |
| Backward compatibility | 0 regressions | Existing tests pass without modification |
| Helm detection for ConfigMap | helmManaged=True | UT-HAPI-676-001, UT-HAPI-676-002 |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-250: DetectedLabels integration with Data Storage
- BR-SP-101: DetectedLabels Auto-Detection (reference implementation)
- BR-SP-103: FailedDetections Tracking
- DD-HAPI-018: DetectedLabels Detection Specification
- Issue #676: HAPI does not surface helmManaged=true from resource labels to LLM

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Existing test suite: `holmesgpt-api/tests/unit/test_label_detector.py` (19 tests, UT-HAPI-056 series)
- Existing test suite: `holmesgpt-api/tests/unit/test_k8s_client_label_queries.py` (15 tests)
- Existing test suite: `holmesgpt-api/tests/unit/test_enrichment_service.py` (9 tests)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `ResourceField` from dynamic client breaks `in` operator | Detection silently fails | Medium | UT-HAPI-676-001/002 | Mandatory `dict()` conversion in `_detect` closure; typed branches for common kinds |
| R2 | Secret `.data` exposed through metadata resolver | Security: credential leakage | Low | UT-HAPI-676-006 | Only `.metadata.labels`/`.annotations` accessed in `_detect` closure |
| R3 | Double API call for Deployment root owners | Performance degradation | Low | UT-HAPI-676-009 | Guard: `"deployment_details" not in k8s_context` |
| R4 | Existing tests break due to new `_batch_v1` in K8s client | Regression | Low | All UT-HAPI-056-* | Additive change; existing mocks unaffected |

### 3.1 Risk-to-Test Traceability

- R1 → UT-HAPI-676-001, UT-HAPI-676-002 (verify `helmManaged=True` with plain dict labels)
- R2 → UT-HAPI-676-006 (verify Secret metadata returned but only labels/annotations accessed)
- R3 → UT-HAPI-676-009 (verify EnrichmentService calls detector with correct root_owner for ConfigMap)
- R4 → Baseline: all 831 existing tests pass after changes

---

## 4. Scope

### 4.1 Features to be Tested

- **LabelDetector._detect_helm** (`holmesgpt-api/src/detection/labels.py`): Helm detection via `root_owner_details` k8s_context key
- **LabelDetector._detect_gitops** (`holmesgpt-api/src/detection/labels.py`): GitOps detection via `root_owner_details` k8s_context key
- **K8sResourceClient._get_resource_metadata_sync** (`holmesgpt-api/src/clients/k8s_client.py`): Support for ConfigMap, Secret, Service, Job kinds
- **_detect closure** (`holmesgpt-api/src/extensions/incident/llm_integration.py`): Population of `root_owner_details` for non-Deployment root owners

### 4.2 Features Not to be Tested

- **_get_resource_spec_sync**: Same unsupported-kind gap exists but affects remediation history, not label detection. Deferred to separate issue.
- **Integration/E2E tests**: Fix is in Python HAPI code; integration tests require real K8s cluster. Covered by E2E test in CI.
- **CronJob support in _get_resource_metadata_sync**: Included in code fix but no dedicated test (low-priority kind for root owners)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Typed client branches over dynamic-only | Typed client returns plain `dict` labels; avoids `ResourceField` `in` operator bug |
| `root_owner_details` as new k8s_context key | Preserves existing `deployment_details` semantics; additive change |
| Guard on `"deployment_details" not in k8s_context` | Avoids double API call for 95% Deployment case |
| `dict()` conversion on labels/annotations | Handles both typed client (`dict`) and dynamic fallback (`ResourceField`) |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in changed files (`labels.py`, `k8s_client.py`)
- **Integration**: Deferred — fix is Python code tested via containerized pytest, not Go integration
- **E2E**: Deferred — covered by existing CI E2E pipeline

### 5.2 Two-Tier Minimum

This fix is covered by:
- **Unit tests** (this plan): 10 new tests validating detection logic and K8s client extensibility
- **E2E tests** (existing CI): Full pipeline validation with real K8s cluster

Integration tier is skipped because:
- HAPI integration tests use a hybrid Go+Python pattern with real infrastructure
- The fix is pure Python logic (label dict lookups) that does not require infrastructure testing
- E2E coverage in CI validates the full stack

### 5.3 Business Outcome Quality Bar

Each test validates a business outcome: "Given a ConfigMap root owner with Helm labels, the system correctly identifies it as Helm-managed and surfaces this to the LLM for workflow selection."

### 5.4 Pass/Fail Criteria

**PASS**:
1. All 10 new tests pass (0 failures)
2. All 831 existing tests pass (0 regressions)
3. Coverage does not decrease from 75.53% baseline
4. ConfigMap with `app.kubernetes.io/managed-by: Helm` produces `helmManaged=True`

**FAIL**:
1. Any new test fails
2. Any existing test regresses
3. Coverage drops below baseline

### 5.5 Suspension & Resumption Criteria

**Suspend**: podman unavailable (containerized test runner), v1.2.0 tag cannot be checked out
**Resume**: Infrastructure restored, branch recreated from tag

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `src/detection/labels.py` | `_detect_helm`, `_detect_gitops` | ~30 changed |
| `src/clients/k8s_client.py` | `_get_resource_metadata_sync`, `_init_api_clients` | ~20 changed |
| `src/extensions/incident/llm_integration.py` | `_detect` closure | ~10 changed |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `v1.2.0` tag (`f7143827a`) | Branch: `fix/v1.2.1-helm-detection` |
| kubernetes SDK | >=29.0.0 | Typed client for ConfigMap/Secret/Service |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-250 | DetectedLabels integration with Data Storage | P0 | Unit | UT-HAPI-676-001 | Pending |
| BR-HAPI-250 | DetectedLabels integration with Data Storage | P0 | Unit | UT-HAPI-676-002 | Pending |
| BR-SP-101 | DetectedLabels Auto-Detection | P0 | Unit | UT-HAPI-676-003 | Pending |
| BR-SP-101 | DetectedLabels Auto-Detection | P0 | Unit | UT-HAPI-676-004 | Pending |
| BR-HAPI-250 | K8s client supports ConfigMap metadata | P0 | Unit | UT-HAPI-676-005 | Pending |
| BR-HAPI-250 | K8s client supports Secret metadata | P1 | Unit | UT-HAPI-676-006 | Pending |
| BR-HAPI-250 | K8s client supports Service metadata | P1 | Unit | UT-HAPI-676-007 | Pending |
| BR-HAPI-250 | K8s client supports Job metadata | P1 | Unit | UT-HAPI-676-008 | Pending |
| BR-HAPI-250 | EnrichmentService ConfigMap root owner | P0 | Unit | UT-HAPI-676-009 | Pending |
| BR-HAPI-250 | End-to-end Helm detection for ConfigMap | P0 | Unit | UT-HAPI-676-010 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-HAPI-676-{SEQUENCE}` (Unit Test, HAPI service, Issue #676)

### Tier 1: Unit Tests

**Testable code scope**: `src/detection/labels.py`, `src/clients/k8s_client.py`, `src/extensions/incident/llm_integration.py`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-HAPI-676-001 | ConfigMap with `managed-by: Helm` label produces `helmManaged=True` via `root_owner_details` | Pending |
| UT-HAPI-676-002 | ConfigMap with `helm.sh/chart` label (no managed-by) produces `helmManaged=True` via `root_owner_details` | Pending |
| UT-HAPI-676-003 | ConfigMap with ArgoCD `tracking-id` annotation produces `gitOpsManaged=True` via `root_owner_details` | Pending |
| UT-HAPI-676-004 | ConfigMap with Flux `sync-gc-mark` label produces `gitOpsManaged=True, gitOpsTool=flux` via `root_owner_details` | Pending |
| UT-HAPI-676-005 | `_get_resource_metadata_sync("ConfigMap", ...)` returns the ConfigMap object (not None) | Pending |
| UT-HAPI-676-006 | `_get_resource_metadata_sync("Secret", ...)` returns the Secret object (not None) | Pending |
| UT-HAPI-676-007 | `_get_resource_metadata_sync("Service", ...)` returns the Service object (not None) | Pending |
| UT-HAPI-676-008 | `_get_resource_metadata_sync("Job", ...)` returns the Job object (not None) | Pending |
| UT-HAPI-676-009 | EnrichmentService with ConfigMap root owner passes correct root_owner dict to label_detector | Pending |
| UT-HAPI-676-010 | LabelDetector.detect_labels with `root_owner_details` containing Helm labels returns `helmManaged=True` (end-to-end through detect_labels) | Pending |

### Tier Skip Rationale

- **Integration**: HAPI integration tests use hybrid Go+Python pattern with real infrastructure. The fix is pure Python logic (dict lookups). E2E tier provides integration-equivalent coverage.
- **E2E**: Covered by existing CI pipeline. Not added to this test plan.

---

## 9. Test Cases

### UT-HAPI-676-001: Helm detection via root_owner_details (managed-by label)

**BR**: BR-HAPI-250
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_label_detector.py`

**Preconditions**: LabelDetector instantiated with mock K8s queries

**Test Steps**:
1. **Given**: k8s_context with `root_owner_details` containing `{"labels": {"app.kubernetes.io/managed-by": "Helm"}}` and NO `deployment_details`
2. **When**: `detect_labels(k8s_context, [])` is called
3. **Then**: result contains `helmManaged=True`

### UT-HAPI-676-002: Helm detection via root_owner_details (chart label only)

**BR**: BR-HAPI-250
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_label_detector.py`

**Preconditions**: LabelDetector instantiated with mock K8s queries

**Test Steps**:
1. **Given**: k8s_context with `root_owner_details` containing `{"labels": {"helm.sh/chart": "api-1.0.0"}}` and NO `deployment_details`
2. **When**: `detect_labels(k8s_context, [])` is called
3. **Then**: result contains `helmManaged=True`

### UT-HAPI-676-003: GitOps detection via root_owner_details (ArgoCD tracking-id)

**BR**: BR-SP-101
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_label_detector.py`

**Test Steps**:
1. **Given**: k8s_context with `root_owner_details` containing `{"annotations": {"argocd.argoproj.io/tracking-id": "my-app:ConfigMap:default:worker-config"}}`
2. **When**: `detect_labels(k8s_context, [])` is called
3. **Then**: result contains `gitOpsManaged=True` and `gitOpsTool="argocd"`

### UT-HAPI-676-004: GitOps detection via root_owner_details (Flux label)

**BR**: BR-SP-101
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_label_detector.py`

**Test Steps**:
1. **Given**: k8s_context with `root_owner_details` containing `{"labels": {"fluxcd.io/sync-gc-mark": "sha256:abc123"}}`
2. **When**: `detect_labels(k8s_context, [])` is called
3. **Then**: result contains `gitOpsManaged=True` and `gitOpsTool="flux"`

### UT-HAPI-676-005 through UT-HAPI-676-008: K8s client metadata support

**BR**: BR-HAPI-250
**Priority**: P0 (005), P1 (006-008)
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_k8s_client_label_queries.py`

**Test Steps** (template for each kind):
1. **Given**: K8sResourceClient with mocked `_core_v1` (or `_batch_v1`) API client
2. **When**: `_get_resource_metadata_sync(kind, "test-resource", "default")` is called
3. **Then**: Returns the mock resource object (not None), confirming the kind is supported

### UT-HAPI-676-009: EnrichmentService ConfigMap root owner

**BR**: BR-HAPI-250
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_enrichment_service.py`

**Test Steps**:
1. **Given**: EnrichmentService with mock K8s client returning empty owner chain for ConfigMap
2. **When**: `enrich({"kind": "ConfigMap", "name": "worker-config", "namespace": "default"})` is called
3. **Then**: `label_detector` is called with root_owner `{"kind": "ConfigMap", "name": "worker-config", "namespace": "default"}`

### UT-HAPI-676-010: End-to-end Helm detection for ConfigMap root owner

**BR**: BR-HAPI-250
**Priority**: P0
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_label_detector.py`

**Test Steps**:
1. **Given**: k8s_context with `root_owner_details` containing `{"kind": "ConfigMap", "name": "worker-config", "labels": {"app.kubernetes.io/managed-by": "Helm", "helm.sh/chart": "demo-crashloop-helm-0.1.0"}}` — mimicking the exact labels from the #676 scenario
2. **When**: `detect_labels(k8s_context, [])` is called
3. **Then**: result contains `helmManaged=True`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: pytest + pytest-asyncio (HAPI is Python)
- **Mocks**: `unittest.mock.MagicMock`, `unittest.mock.AsyncMock` for K8s API clients
- **Location**: `holmesgpt-api/tests/unit/`
- **Runner**: `make test-unit-holmesgpt-api` (containerized via podman + UBI10 Python 3.12)

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Python | 3.12 | Runtime (UBI10 container) |
| pytest | 7.4.3 | Test runner |
| kubernetes SDK | >=29.0.0 | K8s API client |
| podman | 5.x | Container runtime for test execution |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| v1.2.0 tag | Code | Available | Cannot create branch | N/A |
| podman | Infra | Available (5.8.1) | Cannot run containerized tests | Local pytest (degraded) |

### 11.2 Execution Order

1. **Phase 3 (TDD RED)**: Write all 10 failing tests
2. **Phase 4 (TDD GREEN)**: Fix k8s_client, _detect closure, _detect_helm, _detect_gitops
3. **Phase 5 (TDD REFACTOR)**: Docstrings, naming, consistency

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/676/TEST_PLAN.md` | Strategy and test design |
| Label detector tests | `holmesgpt-api/tests/unit/test_label_detector.py` | 5 new tests (UT-HAPI-676-001/002/003/004/010) |
| K8s client tests | `holmesgpt-api/tests/unit/test_k8s_client_label_queries.py` | 4 new tests (UT-HAPI-676-005/006/007/008) |
| Enrichment service test | `holmesgpt-api/tests/unit/test_enrichment_service.py` | 1 new test (UT-HAPI-676-009) |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Full unit test suite (containerized)
make test-unit-holmesgpt-api

# Lint
make lint-holmesgpt-api

# Anti-pattern check (Go tests only, but verify no regressions)
make lint-test-patterns
```

---

## 14. Existing Tests Requiring Updates

No existing tests require modification. All changes are additive (new `root_owner_details` k8s_context key, new typed branches in `_get_resource_metadata_sync`). Existing tests do not supply `root_owner_details` and will not trigger new code paths.

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-12 | Initial test plan |
