# HAPI Test Plan for ADR-056

**Version**: 1.0
**Created**: 2026-02-17
**Status**: Active
**Authority**: [ADR-056](../../architecture/decisions/ADR-056-post-rca-label-computation.md), [DD-HAPI-017](../../architecture/decisions/DD-HAPI-017-three-step-workflow-discovery-integration.md), [DD-HAPI-018](../../architecture/decisions/DD-HAPI-018-detected-labels-detection-specification.md)
**BRs**: BR-SP-101, BR-SP-103, BR-HAPI-194, BR-HAPI-250, BR-HAPI-252

---

## Overview

This test plan covers the HAPI (HolmesGPT API, Python) service implementation of ADR-056: relocating DetectedLabels computation from SignalProcessing (pipeline-time) to HAPI (post-RCA runtime).

### Scope

- **Cycle 1.1**: `LabelDetector` -- 8 cluster characteristics detection from K8s resources
- **Cycle 1.2**: K8s client extensions -- PDB, HPA, NetworkPolicy, Namespace metadata queries
- **Cycle 1.3**: `get_resource_context` tool -- session_state integration with LabelDetector
- **Cycle 2.1**: Flow enforcement -- prerequisite check in workflow discovery tools
- **Cycle 2.2**: Context params -- `_build_context_params` reads from session_state
- **Cycle 2.3**: Prompt removal -- detected_labels no longer injected into LLM prompts
- **Cycle 2.4**: Session state wiring -- registration code passes session_state to toolsets
- **Cycle 2.5**: Response model -- detected_labels in HAPI response for AIAnalysis PostRCAContext

### Out of Scope

- AIAnalysis Go-side PostRCAContext handling (see [aianalysis_test_plan_v1.0.md](aianalysis_test_plan_v1.0.md))
- SignalProcessing internal label computation (existing tests, unchanged)
- Integration and E2E tests (see INT/E2E sections below)

---

## Anti-Pattern Compliance

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

- No `time.Sleep()` -- async tests use `asyncio` event loop, not wall-clock waits
- No `Skip()` / `XIt` -- tests fail or pass, never skip
- No direct audit/metrics infrastructure testing
- No struct-shape tests -- all tests validate observable business outcomes
- No implementation-detail assertions -- tests assert `session_state["detected_labels"]` content, not internal `call_args` or `k8s_context` structure
- Mock ONLY external dependencies (K8s API, DataStorage) -- real business logic for LabelDetector, prompt builders, etc.

---

## Code Surface by Tier

| Tier | Files | LOC | Coverage | Target |
|------|-------|-----|----------|--------|
| Unit | `labels.py`, `k8s_client.py`, `resource_context.py`, `workflow_discovery.py`, `prompt_builder.py` (x2), `llm_config.py`, `incident_models.py`, `recovery_models.py` | ~450 | 80 tests | >= 80% MET |
| Integration | `test_detected_labels_integration.py` + `k8s_mock_fixtures.py` | ~200 | 7 tests | >= 80% Active |
| E2E | `detected_labels_e2e_test.go` | ~150 | 3 tests | >= 80% Active |

---

## Test Scenario Naming Convention

**Format**: `UT-HAPI-056-{SEQ}` (zero-padded 3-digit sequence)

**Usage in Python test functions**:
```python
def test_ut_hapi_056_001_argocd_pod_annotation(self):
    """UT-HAPI-056-001: ArgoCD pod annotation -> gitOpsManaged=true"""
```

---

## BR Coverage Matrix

| BR ID | Description | Test IDs | Count |
|-------|-------------|----------|-------|
| BR-SP-101 | DetectedLabels auto-detection (8 characteristics) | UT-HAPI-056-001 to 021, 068 to 080 | 34 |
| BR-SP-103 | FailedDetections tracking | UT-HAPI-056-018 to 021 | 4 |
| BR-HAPI-194 | Honor failedDetections in workflow filtering | UT-HAPI-056-043 to 055, 071, 072 | 15 |
| BR-HAPI-250/252 | DetectedLabels in HAPI response | UT-HAPI-056-063 to 067 | 5 |

---

## Cycle 1.1: LabelDetector (21 tests)

**File**: `holmesgpt-api/tests/unit/test_label_detector.py`
**Production code**: `holmesgpt-api/src/detection/labels.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-001 | ArgoCD pod annotation | P1 | GitOps workflows selected for ArgoCD workloads |
| UT-HAPI-056-002 | ArgoCD deployment label | P1 | GitOps detected even without pod-level markers |
| UT-HAPI-056-003 | Flux deployment label | P1 | Flux-managed workloads correctly identified |
| UT-HAPI-056-004 | Namespace ArgoCD label | P2 | Namespace-level GitOps propagates to workload |
| UT-HAPI-056-005 | Namespace Flux annotation | P2 | Namespace-level Flux detection works |
| UT-HAPI-056-006 | Namespace label precedence | P2 | Deterministic tool selection when multiple signals |
| UT-HAPI-056-007 | PDB protection | P1 | PDB-aware remediation avoids disruption budget violations |
| UT-HAPI-056-008 | HPA detection | P1 | HPA-aware workflows avoid conflicting scaling |
| UT-HAPI-056-009 | StatefulSet detection | P1 | Stateful workloads get ordered remediation |
| UT-HAPI-056-010 | Helm managed-by label | P1 | Helm-managed workloads use chart-aware remediation |
| UT-HAPI-056-011 | Helm chart label only | P2 | Helm detected via chart label fallback |
| UT-HAPI-056-012 | NetworkPolicy detection | P1 | Network-isolated workloads flagged for safe remediation |
| UT-HAPI-056-013 | Istio service mesh | P1 | Service mesh presence avoids sidecar disruption |
| UT-HAPI-056-014 | Linkerd service mesh | P1 | Linkerd proxy presence detected |
| UT-HAPI-056-015 | Plain deployment | P2 | No false positives on vanilla workloads |
| UT-HAPI-056-016 | None context | P2 | Graceful nil handling, no crash |
| UT-HAPI-056-017 | Multiple simultaneous | P1 | Combined detections (ArgoCD+PDB+HPA) all reported |
| UT-HAPI-056-018 | PDB RBAC forbidden | P1 | Partial results + failedDetections on RBAC error |
| UT-HAPI-056-019 | HPA timeout | P1 | Partial results + failedDetections on timeout |
| UT-HAPI-056-020 | All queries fail | P1 | All failures tracked, no crash |
| UT-HAPI-056-021 | Context cancellation | P1 | BaseException (CancelledError) handled gracefully |

---

### UT-HAPI-056-001: ArgoCD pod annotation detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Ensures ArgoCD-managed workloads are correctly identified so GitOps-safe remediation workflows are selected.

**Given**: A KubernetesContext with a Pod having annotation `argocd.argoproj.io/instance`
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `gitOpsManaged` is `true`
- `gitOpsTool` is `"argocd"`

**Acceptance Criteria**:
- Pod annotation alone is sufficient to detect ArgoCD
- Both boolean flag and tool identifier are set

---

### UT-HAPI-056-002: ArgoCD deployment label detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Catches ArgoCD management when pod has no annotations but deployment does.

**Given**: A KubernetesContext with a Deployment having label `argocd.argoproj.io/instance`, pod has no ArgoCD markers
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `gitOpsManaged` is `true`
- `gitOpsTool` is `"argocd"`

**Acceptance Criteria**:
- Deployment-level label detected as fallback when pod lacks annotations
- No false positive from pod

---

### UT-HAPI-056-003: Flux deployment label detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Flux-managed workloads are identified for Flux-compatible remediation.

**Given**: A KubernetesContext with a Deployment having label `fluxcd.io/sync-gc-mark`
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `gitOpsManaged` is `true`
- `gitOpsTool` is `"flux"`

**Acceptance Criteria**:
- Flux label on deployment triggers detection
- Tool identifier distinguishes Flux from ArgoCD

---

### UT-HAPI-056-004: Namespace ArgoCD label detection

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Namespace-level GitOps applies to all workloads in the namespace.

**Given**: A KubernetesContext where namespace has label `argocd.argoproj.io/instance`
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `gitOpsManaged` is `true`
- `gitOpsTool` is `"argocd"`

**Acceptance Criteria**:
- Namespace labels propagate to workload-level detection

---

### UT-HAPI-056-005: Namespace Flux annotation detection

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Flux annotations on namespaces propagate to workload detection.

**Given**: A KubernetesContext where namespace has annotation `fluxcd.io/sync-status`
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `gitOpsManaged` is `true`
- `gitOpsTool` is `"flux"`

**Acceptance Criteria**:
- Namespace annotations are checked for Flux markers

---

### UT-HAPI-056-006: Namespace label precedence over annotation

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Deterministic tool selection when multiple GitOps signals are present.

**Given**: A KubernetesContext where namespace has both a Flux label and an ArgoCD annotation
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `gitOpsManaged` is `true`
- `gitOpsTool` is `"flux"` (labels take precedence over annotations)

**Acceptance Criteria**:
- Labels have higher priority than annotations per DD-HAPI-018

---

### UT-HAPI-056-007: PDB protection detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: PDB-aware remediation avoids violating disruption budgets during pod restarts.

**Given**: A KubernetesContext with a PDB whose selector matches the pod labels
**When**: `LabelDetector.detect_labels()` is called
**Then**: `pdbProtected` is `true`

**Acceptance Criteria**:
- PDB selector-to-pod label matching is correct
- Only matching PDBs trigger the flag

---

### UT-HAPI-056-008: HPA detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: HPA-aware workflows avoid conflicting with autoscaler during remediation.

**Given**: A KubernetesContext with an HPA targeting the Deployment
**When**: `LabelDetector.detect_labels()` is called
**Then**: `hpaEnabled` is `true`

**Acceptance Criteria**:
- HPA scaleTargetRef matching works correctly

---

### UT-HAPI-056-009: StatefulSet detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Stateful workloads get ordered, data-aware remediation.

**Given**: A KubernetesContext whose owner chain includes a StatefulSet
**When**: `LabelDetector.detect_labels()` is called
**Then**: `stateful` is `true`

**Acceptance Criteria**:
- Owner chain traversal correctly identifies StatefulSet ancestry

---

### UT-HAPI-056-010: Helm managed detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Helm-managed workloads use chart-aware remediation (e.g., helm rollback).

**Given**: A KubernetesContext with a Deployment having label `app.kubernetes.io/managed-by: Helm`
**When**: `LabelDetector.detect_labels()` is called
**Then**: `helmManaged` is `true`

**Acceptance Criteria**:
- Standard Helm managed-by label is recognized

---

### UT-HAPI-056-011: Helm chart label only detection

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Catches Helm management when only `helm.sh/chart` label is present.

**Given**: A KubernetesContext with a Deployment having label `helm.sh/chart` but no `managed-by` label
**When**: `LabelDetector.detect_labels()` is called
**Then**: `helmManaged` is `true`

**Acceptance Criteria**:
- `helm.sh/chart` label is a valid fallback for Helm detection

---

### UT-HAPI-056-012: Network isolation detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Network-isolated workloads are flagged to avoid remediation that breaks policy.

**Given**: A KubernetesContext where a NetworkPolicy exists in the namespace
**When**: `LabelDetector.detect_labels()` is called
**Then**: `networkIsolated` is `true`

**Acceptance Criteria**:
- Presence of any NetworkPolicy in namespace triggers the flag

---

### UT-HAPI-056-013: Istio service mesh detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Service mesh presence avoids sidecar disruption during pod remediation.

**Given**: A KubernetesContext with a Pod having annotation `sidecar.istio.io/status`
**When**: `LabelDetector.detect_labels()` is called
**Then**: `serviceMesh` is `"istio"`

**Acceptance Criteria**:
- Istio sidecar annotation correctly identifies mesh type

---

### UT-HAPI-056-014: Linkerd service mesh detection

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Linkerd proxy presence is detected for mesh-aware remediation.

**Given**: A KubernetesContext with a Pod having annotation `linkerd.io/proxy-version`
**When**: `LabelDetector.detect_labels()` is called
**Then**: `serviceMesh` is `"linkerd"`

**Acceptance Criteria**:
- Linkerd annotation correctly identifies mesh type

---

### UT-HAPI-056-015: Plain deployment (no features)

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: No false positives on vanilla workloads without special infrastructure.

**Given**: A plain KubernetesContext with no special annotations, labels, PDBs, HPAs, or NetworkPolicies
**When**: `LabelDetector.detect_labels()` is called
**Then**: All boolean labels are `false`, string labels are empty, `failedDetections` is empty

**Acceptance Criteria**:
- Zero false positives on plain workloads

---

### UT-HAPI-056-016: None KubernetesContext

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Graceful nil handling prevents crashes when context is unavailable.

**Given**: `None` is passed as the KubernetesContext
**When**: `LabelDetector.detect_labels()` is called
**Then**: Returns `None`

**Acceptance Criteria**:
- No exception raised
- Return value is `None` (not empty dict)

---

### UT-HAPI-056-017: Multiple simultaneous detections

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Combined characteristics are all reported for accurate workflow selection.

**Given**: A KubernetesContext with ArgoCD annotation, matching PDB, and targeting HPA
**When**: `LabelDetector.detect_labels()` is called
**Then**: `gitOpsManaged`, `pdbProtected`, and `hpaEnabled` are all `true`

**Acceptance Criteria**:
- Multiple independent detections do not interfere with each other

---

### UT-HAPI-056-018: PDB RBAC forbidden

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: RBAC errors produce partial results instead of total failure.

**Given**: K8s client raises ApiException (403 Forbidden) for PDB list
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `pdbProtected` is `false`
- `failedDetections` includes `"pdbProtected"`
- Other detections still succeed

**Acceptance Criteria**:
- Partial results returned (not total failure)
- Failed detection is tracked for downstream visibility

---

### UT-HAPI-056-019: HPA query timeout

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: Timeouts produce partial results with failure tracking.

**Given**: K8s client raises ApiException (timeout) for HPA list
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `hpaEnabled` is `false`
- `failedDetections` includes `"hpaEnabled"`

**Acceptance Criteria**:
- Timeout handled identically to RBAC error (graceful degradation)

---

### UT-HAPI-056-020: All queries fail

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: Total K8s API failure still returns a valid (empty) result with all failures tracked.

**Given**: K8s client raises errors for PDB, HPA, and NetworkPolicy queries
**When**: `LabelDetector.detect_labels()` is called
**Then**: `failedDetections` includes `"pdbProtected"`, `"hpaEnabled"`, and `"networkIsolated"`

**Acceptance Criteria**:
- No crash on total query failure
- All individual failures tracked

---

### UT-HAPI-056-021: Context cancellation

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: Python asyncio CancelledError (BaseException) is handled without crashing.

**Given**: K8s client raises `CancelledError` (inherits from `BaseException`) for HPA query
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- Partial results returned (non-HPA detections succeed)
- `failedDetections` includes `"hpaEnabled"`

**Acceptance Criteria**:
- `BaseException` caught (not just `Exception`)
- Partial success preserved

---

## Cycle 1.2: K8s Client Label Queries (12 tests)

**File**: `holmesgpt-api/tests/unit/test_k8s_client_label_queries.py`
**Production code**: `holmesgpt-api/src/clients/k8s_client.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-022 | list_pdbs success | P1 | PDB data available for label detection |
| UT-HAPI-056-023 | list_pdbs ApiException | P1 | RBAC/quota errors return empty + error string |
| UT-HAPI-056-024 | list_pdbs unexpected error | P2 | Unknown errors handled gracefully |
| UT-HAPI-056-025 | list_hpas success | P1 | HPA data available for label detection |
| UT-HAPI-056-026 | list_hpas ApiException | P1 | Timeout/RBAC errors return empty + error |
| UT-HAPI-056-027 | list_hpas unexpected error | P2 | Unknown errors handled gracefully |
| UT-HAPI-056-028 | list_network_policies success | P1 | NetworkPolicy data available |
| UT-HAPI-056-029 | list_network_policies ApiException | P1 | ApiException returns empty + error |
| UT-HAPI-056-030 | list_network_policies unexpected error | P2 | Unknown errors handled gracefully |
| UT-HAPI-056-031 | get_namespace_metadata success | P1 | Namespace labels/annotations available |
| UT-HAPI-056-032 | get_namespace_metadata ApiException | P1 | Not found/RBAC returns None |
| UT-HAPI-056-033 | get_namespace_metadata unexpected error | P2 | Unknown errors return None |

---

### UT-HAPI-056-022: list_pdbs success

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: PDB data is essential for pdbProtected detection.

**Given**: K8s PolicyV1Api returns PDB items for the namespace
**When**: `list_pdbs(namespace)` is called via `asyncio.to_thread()`
**Then**: Returns `(pdb_items_list, None)`

**Acceptance Criteria**:
- Tuple of (items, error) returned
- Error is `None` on success

---

### UT-HAPI-056-023: list_pdbs ApiException

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: RBAC errors are surfaced as error strings for failedDetections tracking.

**Given**: K8s PolicyV1Api raises `ApiException` (403 Forbidden)
**When**: `list_pdbs(namespace)` is called
**Then**: Returns `([], "403 Forbidden: ...")`

**Acceptance Criteria**:
- Empty list (not None) on error
- Error string includes status code and reason

---

### UT-HAPI-056-024: list_pdbs unexpected error

**Priority**: P2 | **BR**: BR-SP-103
**Business Value**: Unexpected exceptions do not crash the detection pipeline.

**Given**: K8s PolicyV1Api raises unexpected `Exception`
**When**: `list_pdbs(namespace)` is called
**Then**: Returns `([], "Unexpected error: ...")`

**Acceptance Criteria**:
- Same tuple contract as ApiException path

---

### UT-HAPI-056-025 through UT-HAPI-056-027: list_hpas (success / ApiException / unexpected)

Same contract as PDB queries (UT-HAPI-056-022 to 024) but for `AutoscalingV2Api.list_namespaced_horizontal_pod_autoscaler`.

---

### UT-HAPI-056-028 through UT-HAPI-056-030: list_network_policies (success / ApiException / unexpected)

Same contract as PDB queries but for `NetworkingV1Api.list_namespaced_network_policy`.

---

### UT-HAPI-056-031: get_namespace_metadata success

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Namespace labels/annotations enable namespace-level GitOps detection.

**Given**: K8s CoreV1Api returns a namespace with labels and annotations
**When**: `get_namespace_metadata(namespace)` is called
**Then**: Returns `{"labels": {...}, "annotations": {...}}`

**Acceptance Criteria**:
- Dict with both `labels` and `annotations` keys

---

### UT-HAPI-056-032: get_namespace_metadata ApiException

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: Missing namespace metadata does not crash detection.

**Given**: K8s CoreV1Api raises `ApiException` (404 or 403)
**When**: `get_namespace_metadata(namespace)` is called
**Then**: Returns `None`

**Acceptance Criteria**:
- `None` return (not exception) on API error

---

### UT-HAPI-056-033: get_namespace_metadata unexpected error

**Priority**: P2 | **BR**: BR-SP-103

Same as UT-HAPI-056-032 but for unexpected `Exception`. Returns `None`.

---

## Cycle 1.3: Resource Context + Session State (9 tests)

**File**: `holmesgpt-api/tests/unit/test_resource_context_session_state.py`
**Production code**: `holmesgpt-api/src/toolsets/resource_context.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-034 | Writes detected_labels to session_state | P0 | Labels available for downstream tools |
| UT-HAPI-056-035 | Writes {} sentinel on None detection | P1 | Downstream tools know detection ran but found nothing |
| UT-HAPI-056-036 | Preserves return behavior | P1 | Tool result unchanged (root_owner + history) |
| UT-HAPI-056-037 | Pod->Deployment chain labels | P1 | Correct labels for common ownership pattern |
| UT-HAPI-056-038 | Deployment-only chain labels | P1 | Correct labels without Pod in chain |
| UT-HAPI-056-039 | StatefulSet chain labels | P1 | stateful=true for StatefulSet ownership |
| UT-HAPI-056-040 | Namespace metadata None | P2 | Graceful fallback to empty dicts |
| UT-HAPI-056-041 | LabelDetector exception | P1 | {} sentinel written, tool succeeds |
| UT-HAPI-056-042 | No session_state with detector | P2 | Detection runs without crash |

---

### UT-HAPI-056-034: Writes detected_labels to session_state

**Priority**: P0 | **BR**: BR-SP-101, BR-HAPI-194
**Business Value**: This is the core ADR-056 contract -- labels computed post-RCA are shared via session_state for downstream workflow discovery.

**Given**: A configured `GetResourceContextTool` with session_state dict and LabelDetector mock returning labels
**When**: Tool invocation completes with valid K8s context
**Then**: `session_state["detected_labels"]` contains the LabelDetector results

**Acceptance Criteria**:
- `detected_labels` key exists in session_state after invocation
- Values match LabelDetector output exactly

---

### UT-HAPI-056-035: Writes empty sentinel when detection returns None

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: The `{}` sentinel signals "detection ran but no context available" -- distinct from "detection not yet run" (key absent).

**Given**: LabelDetector returns `None` (nil context)
**When**: Tool invocation completes
**Then**: `session_state["detected_labels"]` is `{}`

**Acceptance Criteria**:
- Empty dict (not `None`) written to session_state
- Downstream tools can distinguish "ran, empty" from "not ran"

---

### UT-HAPI-056-036: Preserves existing return behavior

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Tool's public contract (return value) is unchanged by ADR-056 changes.

**Given**: A valid K8s context with owner chain
**When**: Tool invocation completes
**Then**: Tool result contains `root_owner` and `history`, but NOT `detected_labels`

**Acceptance Criteria**:
- detected_labels flow through session_state, not tool return value

---

### UT-HAPI-056-037: Pod->Deployment chain produces correct labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Most common K8s ownership pattern produces correct labels.

**Given**: K8s context with Pod owned by ReplicaSet owned by Deployment, with ArgoCD labels on Deployment
**When**: Tool invocation completes
**Then**: `session_state["detected_labels"]` has `gitOpsManaged=true` for the Deployment context

**Acceptance Criteria**:
- Owner chain traversal builds correct K8s context for LabelDetector
- ArgoCD detection works through the chain

---

### UT-HAPI-056-038: Deployment-only chain produces correct labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Deployment-only workloads (no Pod yet) still get labels.

**Given**: K8s context with Deployment only, with Helm labels
**When**: Tool invocation completes
**Then**: `session_state["detected_labels"]` has `helmManaged=true`

**Acceptance Criteria**:
- Labels computed correctly without Pod in owner chain

---

### UT-HAPI-056-039: StatefulSet chain produces correct labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: StatefulSet workloads are flagged for ordered remediation.

**Given**: K8s context with Pod owned by StatefulSet
**When**: Tool invocation completes
**Then**: `session_state["detected_labels"]` has `stateful=true`

**Acceptance Criteria**:
- StatefulSet in owner chain triggers stateful flag

---

### UT-HAPI-056-040: Namespace metadata None graceful fallback

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Missing namespace metadata does not crash label detection.

**Given**: K8s client returns `None` for namespace metadata
**When**: Tool invocation completes
**Then**: Label detection proceeds with empty namespace labels/annotations

**Acceptance Criteria**:
- No exception raised
- Detection completes with available data

---

### UT-HAPI-056-041: LabelDetector exception writes sentinel

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: LabelDetector failures do not crash the tool or block the LLM conversation.

**Given**: LabelDetector raises an unexpected exception
**When**: Tool invocation completes
**Then**: `session_state["detected_labels"]` is `{}` and tool returns normally

**Acceptance Criteria**:
- Exception caught, sentinel written
- Tool result still valid (root_owner + history)

---

### UT-HAPI-056-042: No session_state with detector present

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Edge case where session_state is None but detector exists -- no crash.

**Given**: Tool has a LabelDetector but session_state is `None`
**When**: Tool invocation completes
**Then**: Detection runs without crash, labels not written

**Acceptance Criteria**:
- No `AttributeError` or `TypeError`

---

## Cycle 2.1: Flow Enforcement (8 tests)

**File**: `holmesgpt-api/tests/unit/test_workflow_discovery_flow_enforcement.py`
**Production code**: `holmesgpt-api/src/toolsets/workflow_discovery.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-043 | list_available_actions flow error | P0 | Prevents DS query without label context |
| UT-HAPI-056-044 | list_available_actions proceeds | P1 | Normal flow with labels |
| UT-HAPI-056-045 | Proceeds with {} sentinel | P1 | Empty sentinel counts as "detection ran" |
| UT-HAPI-056-046 | list_workflows flow error | P0 | Same enforcement for list_workflows |
| UT-HAPI-056-047 | get_workflow flow error | P0 | Same enforcement for get_workflow |
| UT-HAPI-056-048 | list_workflows proceeds | P1 | Normal flow with labels |
| UT-HAPI-056-049 | get_workflow proceeds | P1 | Normal flow with labels |
| UT-HAPI-056-050 | Toolset propagates session_state | P1 | All tools share the same state |

---

### UT-HAPI-056-043: list_available_actions flow error when key absent

**Priority**: P0 | **BR**: BR-HAPI-194
**Business Value**: Prevents workflow discovery from querying DataStorage without label context, which would return incorrect/unfiltered results.

**Given**: session_state exists but `"detected_labels"` key is absent
**When**: `list_available_actions` is invoked
**Then**: Returns flow error `StructuredToolResult` indicating prerequisite not met

**Acceptance Criteria**:
- Error result (not exception) returned
- Error message mentions `get_resource_context` as the required prerequisite

---

### UT-HAPI-056-044: list_available_actions proceeds with non-empty labels

**Priority**: P1 | **BR**: BR-HAPI-194
**Business Value**: Normal operation when prerequisite is met.

**Given**: session_state contains `"detected_labels"` with label values
**When**: `list_available_actions` is invoked
**Then**: Proceeds to query DataStorage (no flow error)

**Acceptance Criteria**:
- No flow error returned
- DataStorage query is initiated

---

### UT-HAPI-056-045: Proceeds with empty sentinel

**Priority**: P1 | **BR**: BR-HAPI-194
**Business Value**: Empty sentinel `{}` means "detection ran, no labels" -- this is a valid state.

**Given**: session_state contains `"detected_labels": {}`
**When**: `list_available_actions` is invoked
**Then**: Proceeds (empty sentinel counts as prerequisite met)

**Acceptance Criteria**:
- `{}` is treated as "key present" (not as "key absent")

---

### UT-HAPI-056-046 / 047: list_workflows / get_workflow flow error

Same as UT-HAPI-056-043 but for `list_workflows` and `get_workflow` respectively. All three tools enforce the same prerequisite.

---

### UT-HAPI-056-048 / 049: list_workflows / get_workflow proceeds

Same as UT-HAPI-056-044 but for `list_workflows` and `get_workflow`.

---

### UT-HAPI-056-050: Toolset propagates session_state to all tools

**Priority**: P1 | **BR**: BR-HAPI-194
**Business Value**: All three tools share the same session_state reference for consistent behavior.

**Given**: `WorkflowDiscoveryToolset` constructed with a session_state dict
**When**: Toolset creates its 3 tools
**Then**: All 3 tools receive the same session_state reference (`is` check)

**Acceptance Criteria**:
- Identity check (not equality) confirms same dict instance

---

## Cycle 2.2: Context Params from session_state (5 tests)

**File**: `holmesgpt-api/tests/unit/test_workflow_discovery_context_params.py`
**Production code**: `holmesgpt-api/src/toolsets/workflow_discovery.py` (`_build_context_params`)

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-051 | Labels read from session_state | P1 | Runtime labels used for DS queries |
| UT-HAPI-056-052 | session_state takes precedence | P1 | Runtime labels override stale constructor values |
| UT-HAPI-056-053 | strip_failed_detections | P1 | failedDetections not sent to DS |
| UT-HAPI-056-054 | Empty sentinel -> no param | P2 | No empty labels sent to DS |
| UT-HAPI-056-055 | Key missing -> no param | P2 | Graceful absence handling |

---

### UT-HAPI-056-051: detected_labels read from session_state

**Priority**: P1 | **BR**: BR-HAPI-194
**Business Value**: Runtime-computed labels (from LabelDetector) are used for DataStorage workflow queries.

**Given**: session_state has `"detected_labels"` with label values `{"gitOpsManaged": true, "gitOpsTool": "argocd"}`
**When**: `_build_context_params()` is called
**Then**: Returned params dict includes `detected_labels` with those values

**Acceptance Criteria**:
- Labels from session_state appear in params dict

---

### UT-HAPI-056-052: session_state takes precedence over constructor

**Priority**: P1 | **BR**: BR-HAPI-194
**Business Value**: Runtime labels (post-RCA) override any stale constructor-time labels.

**Given**: Both `session_state["detected_labels"]` and `self._detected_labels` have different values
**When**: `_build_context_params()` is called
**Then**: session_state values are used

**Acceptance Criteria**:
- Constructor values ignored when session_state has labels

---

### UT-HAPI-056-053: strip_failed_detections applied

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: `failedDetections` is internal metadata, not a workflow filter parameter.

**Given**: session_state labels include a `failedDetections` key
**When**: `_build_context_params()` is called
**Then**: `failedDetections` is stripped from the labels in params

**Acceptance Criteria**:
- `failedDetections` key absent from params dict

---

### UT-HAPI-056-054: Empty sentinel produces no detected_labels param

**Priority**: P2 | **BR**: BR-HAPI-194
**Business Value**: Empty labels are not sent to DataStorage (avoids empty filter).

**Given**: session_state has `"detected_labels": {}`
**When**: `_build_context_params()` is called
**Then**: No `detected_labels` key in returned params

**Acceptance Criteria**:
- Empty dict treated as "no labels to send"

---

### UT-HAPI-056-055: Key missing -> no param

**Priority**: P2 | **BR**: BR-HAPI-194
**Business Value**: Graceful handling when key is absent (should not happen after flow enforcement, but defensive).

**Given**: session_state exists but has no `"detected_labels"` key
**When**: `_build_context_params()` is called
**Then**: No `detected_labels` key in returned params

**Acceptance Criteria**:
- No `KeyError` raised

---

## Cycle 2.3: Prompt Removal (3 tests)

**File**: `holmesgpt-api/tests/unit/test_prompt_no_detected_labels.py`
**Production code**: `holmesgpt-api/src/extensions/incident/prompt_builder.py`, `holmesgpt-api/src/extensions/recovery/prompt_builder.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-056 | Incident prompt omits cluster context | P1 | LLM does not see stale labels |
| UT-HAPI-056-057 | Recovery prompt omits cluster context | P1 | Same for recovery flow |
| UT-HAPI-056-058 | Prompts still valid | P1 | No regression in prompt quality |

---

### UT-HAPI-056-056: Incident prompt omits cluster context section

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: ADR-056 moves labels to HAPI runtime tools. LLM prompt must NOT include stale SP-time "Cluster Environment Characteristics" section.

**Given**: An incident analysis request with enrichment results
**When**: Incident prompt is built
**Then**: Prompt does NOT contain `"Cluster Environment Characteristics"` section

**Acceptance Criteria**:
- String `"Cluster Environment Characteristics"` absent from prompt output

---

### UT-HAPI-056-057: Recovery prompt omits cluster context section

Same as UT-HAPI-056-056 but for recovery prompt builder.

---

### UT-HAPI-056-058: Prompts produce valid output without section

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: No regression -- prompts are still valid and non-empty after section removal.

**Given**: Both incident and recovery requests
**When**: Both prompts are built
**Then**: Both produce valid, non-empty prompt strings

**Acceptance Criteria**:
- Prompt length > 0
- Contains expected structural sections (incident description, etc.)

---

## Cycle 2.4: Session State Wiring (4 tests)

**File**: `holmesgpt-api/tests/unit/test_session_state_wiring.py`
**Production code**: `holmesgpt-api/src/extensions/llm_config.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-059 | Discovery toolset receives session_state | P1 | Toolset can read runtime labels |
| UT-HAPI-056-060 | Resource context toolset receives session_state | P1 | Toolset can write runtime labels |
| UT-HAPI-056-061 | Both share same dict instance | P0 | Write-then-read contract works |
| UT-HAPI-056-062 | session_state flows to individual tools | P1 | Tools within toolset have access |

---

### UT-HAPI-056-061: Both registrations share same dict instance

**Priority**: P0 | **BR**: BR-SP-101, BR-HAPI-194
**Business Value**: This is the core wiring contract -- `get_resource_context` writes to session_state, `list_workflows` reads from it. They MUST share the same dict instance.

**Given**: A single session_state dict passed to both `register_resource_context_toolset` and `register_workflow_discovery_toolset`
**When**: Both toolsets are constructed
**Then**: Both toolsets hold a reference to the SAME dict instance

**Acceptance Criteria**:
- `toolset_a.session_state is toolset_b.session_state` is `True`

---

### UT-HAPI-056-059, 060, 062

Same pattern: verify session_state reaches the toolset (059, 060) and flows to individual tools within WorkflowDiscoveryToolset (062).

---

## Cycle 2.5: Response Model (5 tests)

**File**: `holmesgpt-api/tests/unit/test_response_detected_labels.py`
**Production code**: `holmesgpt-api/src/models/incident_models.py`, `holmesgpt-api/src/models/recovery_models.py`, `holmesgpt-api/src/extensions/llm_config.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-063 | IncidentResponse has field | P1 | Labels available in API response |
| UT-HAPI-056-064 | RecoveryResponse has field | P1 | Same for recovery |
| UT-HAPI-056-065 | inject_detected_labels incident | P1 | Labels injected into response dict |
| UT-HAPI-056-066 | inject_detected_labels recovery | P1 | Same for recovery |
| UT-HAPI-056-067 | No labels -> not added | P2 | Clean response when no labels |

---

### UT-HAPI-056-065: inject_detected_labels adds labels to incident result

**Priority**: P1 | **BR**: BR-HAPI-250
**Business Value**: AIAnalysis Go service receives detected_labels in HAPI response for PostRCAContext population.

**Given**: session_state with `"detected_labels"` containing label values, and an incident result dict
**When**: `inject_detected_labels(result, session_state)` is called
**Then**: `result["detected_labels"]` contains the labels from session_state

**Acceptance Criteria**:
- Labels appear in response dict under `detected_labels` key
- Original result fields preserved

---

### UT-HAPI-056-067: detected_labels not added when absent from session

**Priority**: P2 | **BR**: BR-HAPI-250
**Business Value**: Clean response when no labels available (no null or empty field).

**Given**: session_state without `"detected_labels"` key
**When**: `inject_detected_labels(result, session_state)` is called
**Then**: `result` does NOT contain `detected_labels` key

**Acceptance Criteria**:
- No `detected_labels` key in result (omitted, not null)

---

## Cycle 3.1: should_include_detected_labels Safety Gate (9 tests)

**File**: `holmesgpt-api/tests/unit/test_should_include_detected_labels.py`
**Production code**: `holmesgpt-api/src/toolsets/workflow_discovery.py` (`should_include_detected_labels`)

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-068 | Source resource missing | P1 | Labels excluded when signal source unknown -- prevents mismatch |
| UT-HAPI-056-069 | RCA resource missing | P1 | Labels excluded when LLM provides no resource -- safe default |
| UT-HAPI-056-070 | RCA resource kind missing | P1 | Labels excluded when kind is empty -- cannot validate relationship |
| UT-HAPI-056-071 | Exact resource match | P0 | Labels included when LLM identifies same resource as signal source |
| UT-HAPI-056-072 | Owner chain match | P0 | Labels included when RCA resource is an owner of the signal source |
| UT-HAPI-056-073 | Same namespace+kind fallback | P1 | Labels included for sibling resources in same namespace/kind |
| UT-HAPI-056-074 | Cluster-scoped same kind | P2 | Labels included for cluster-scoped resources with matching kind |
| UT-HAPI-056-075 | Cross-scope mismatch | P1 | Labels excluded when namespaced vs cluster-scoped (different scope) |
| UT-HAPI-056-076 | Default exclude (no relationship) | P0 | Safe default: labels excluded when relationship cannot be proven |

---

### UT-HAPI-056-068: Source resource missing excludes labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: When the signal source resource identity is unknown, labels MUST NOT be included to prevent applying wrong infrastructure characteristics to an unrelated resource.

**Given**: `source_resource` is `None`, `rca_resource` has valid kind/namespace/name
**When**: `should_include_detected_labels(None, rca_resource)` is called
**Then**: Returns `False`

**Acceptance Criteria**:
- Function returns False without exception
- No log message references a crash or unexpected state

---

### UT-HAPI-056-069: RCA resource missing excludes labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: When the LLM does not identify a specific RCA resource, labels cannot be safely attributed and must be excluded.

**Given**: `source_resource` has valid kind/namespace/name, `rca_resource` is `None`
**When**: `should_include_detected_labels(source_resource, None)` is called
**Then**: Returns `False`

**Acceptance Criteria**:
- Safe default applied when LLM provides no resource

---

### UT-HAPI-056-070: RCA resource kind missing excludes labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: A resource without a `kind` field cannot be validated against the source, so labels must be excluded.

**Given**: `rca_resource` is `{"namespace": "prod", "name": "api"}` (no `kind` key)
**When**: `should_include_detected_labels(source_resource, rca_resource)` is called
**Then**: Returns `False`

**Acceptance Criteria**:
- Missing `kind` triggers safe exclusion

---

### UT-HAPI-056-071: Exact resource match includes labels

**Priority**: P0 | **BR**: BR-SP-101, BR-HAPI-194
**Business Value**: When the LLM identifies the exact same resource as the signal source (same kind, namespace, name), labels are proven relevant and must be included for accurate workflow selection.

**Given**: `source_resource = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}`, `rca_resource` has identical values
**When**: `should_include_detected_labels(source_resource, rca_resource)` is called
**Then**: Returns `True`

**Acceptance Criteria**:
- Exact match on all three fields (kind, namespace, name) → True
- No dependence on owner_chain

---

### UT-HAPI-056-072: Owner chain match includes labels

**Priority**: P0 | **BR**: BR-SP-101, BR-HAPI-194
**Business Value**: When the RCA resource is a proven owner of the signal source (e.g., Pod → Deployment via ownerReferences), labels are relevant because they describe the same workload context.

**Given**: `source_resource = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}`, `rca_resource = {"kind": "Deployment", "namespace": "prod", "name": "api"}`, `owner_chain = [{"kind": "ReplicaSet", ...}, {"kind": "Deployment", "namespace": "prod", "name": "api"}]`
**When**: `should_include_detected_labels(source_resource, rca_resource, owner_chain)` is called
**Then**: Returns `True`

**Acceptance Criteria**:
- RCA resource matched in owner_chain → proven relationship → True

---

### UT-HAPI-056-073: Same namespace+kind fallback includes labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: When owner_chain is provided but empty (no owners found), sibling resources in the same namespace and kind are conservatively included as likely related.

**Given**: `source_resource = {"kind": "Pod", "namespace": "prod", "name": "api-abc"}`, `rca_resource = {"kind": "Pod", "namespace": "prod", "name": "api-def"}`, `owner_chain = []` (empty list, explicitly provided)
**When**: `should_include_detected_labels(source_resource, rca_resource, owner_chain)` is called
**Then**: Returns `True`

**Acceptance Criteria**:
- Empty owner_chain (not None) with matching namespace+kind → conservative include

---

### UT-HAPI-056-074: Cluster-scoped same kind includes labels

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: For cluster-scoped resources (Node, PV, etc.), namespace is irrelevant -- same kind is sufficient to establish relationship.

**Given**: `source_resource = {"kind": "Node", "name": "worker-1"}`, `rca_resource = {"kind": "Node", "name": "worker-2"}`, `owner_chain = []`
**When**: `should_include_detected_labels(source_resource, rca_resource, owner_chain)` is called
**Then**: Returns `True`

**Acceptance Criteria**:
- Cluster-scoped resources matched by kind only (namespace ignored)

---

### UT-HAPI-056-075: Cross-scope mismatch excludes labels

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Prevents applying namespaced resource labels to a cluster-scoped resource (or vice versa), which would cause incorrect workflow selection.

**Given**: `source_resource = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}`, `rca_resource = {"kind": "Node", "name": "worker-3"}`, `owner_chain = []`
**When**: `should_include_detected_labels(source_resource, rca_resource, owner_chain)` is called
**Then**: Returns `False`

**Acceptance Criteria**:
- Namespaced (Pod) vs cluster-scoped (Node) → excluded

---

### UT-HAPI-056-076: Default exclude when no relationship provable

**Priority**: P0 | **BR**: BR-SP-101
**Business Value**: The safety principle: when no relationship can be proven between source and RCA resource, labels are excluded to prevent wrong workflow selection. This is the most critical safety path.

**Given**: `source_resource = {"kind": "Pod", "namespace": "prod", "name": "api-xyz"}`, `rca_resource = {"kind": "Deployment", "namespace": "prod", "name": "api"}`, `owner_chain = None` (not provided)
**When**: `should_include_detected_labels(source_resource, rca_resource, owner_chain)` is called
**Then**: Returns `False`

**Acceptance Criteria**:
- owner_chain is None (not available) → GATE 4 skipped → default exclude
- Different kind+name without owner proof → False

---

## Cycle 3.2: LabelDetector Branch Gaps (4 tests)

**File**: `holmesgpt-api/tests/unit/test_label_detector.py` (extend existing)
**Production code**: `holmesgpt-api/src/detection/labels.py`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-HAPI-056-077 | ArgoCD namespace annotation detection | P2 | Namespace-level ArgoCD annotation propagates to workload |
| UT-HAPI-056-078 | PDB exists but selector doesn't match | P1 | No false positive when PDB protects different workload |
| UT-HAPI-056-079 | PDB with selector = None | P2 | Graceful handling of PDB without selector |
| UT-HAPI-056-080 | HPA targets different deployment | P1 | No false positive when HPA scales different workload |

---

### UT-HAPI-056-077: ArgoCD namespace annotation detection

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Namespace-level ArgoCD annotation (`argocd.argoproj.io/managed`) is the lowest-precedence GitOps signal. It must still be detected when no pod/deployment/namespace-label markers are present.

**Given**: A KubernetesContext where namespace has annotation `argocd.argoproj.io/managed: "true"`, no other GitOps markers at pod, deployment, or namespace label level
**When**: `LabelDetector.detect_labels()` is called
**Then**:
- `gitOpsManaged` is `true`
- `gitOpsTool` is `"argocd"`

**Acceptance Criteria**:
- The lowest-precedence GitOps path is reachable and functional
- No higher-precedence markers intercept detection

---

### UT-HAPI-056-078: PDB exists but selector doesn't match pod

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Prevents false positive pdbProtected flag when a PDB exists in the namespace but protects a different workload. Wrong pdbProtected=true could cause overly cautious remediation for unprotected workloads.

**Given**: Pod with labels `{"app": "api"}`, PDB in namespace with selector `{"app": "frontend"}`
**When**: `LabelDetector.detect_labels()` is called
**Then**: `pdbProtected` is `false`

**Acceptance Criteria**:
- PDB selector mismatch → no false positive
- failedDetections does NOT include pdbProtected

---

### UT-HAPI-056-079: PDB with selector = None

**Priority**: P2 | **BR**: BR-SP-101
**Business Value**: Handles PDBs created without a selector (edge case in some K8s API versions). Must not crash or produce false positive.

**Given**: PDB where `spec.selector` is `None`
**When**: `LabelDetector.detect_labels()` is called
**Then**: `pdbProtected` is `false`, no exception

**Acceptance Criteria**:
- None selector gracefully skipped
- No AttributeError

---

### UT-HAPI-056-080: HPA targets different deployment

**Priority**: P1 | **BR**: BR-SP-101
**Business Value**: Prevents false positive hpaEnabled flag when an HPA exists but targets a completely different deployment. Wrong hpaEnabled=true could cause remediation to avoid scaling operations unnecessarily.

**Given**: Deployment named `api`, HPA targeting `frontend-deployment`
**When**: `LabelDetector.detect_labels()` is called
**Then**: `hpaEnabled` is `false`

**Acceptance Criteria**:
- HPA target mismatch → no false positive
- failedDetections does NOT include hpaEnabled

---

## ID Cross-Reference Table

| New ID | Old ID | File | Cycle |
|--------|--------|------|-------|
| UT-HAPI-056-001 | DL-HP-01 | test_label_detector.py | 1.1 |
| UT-HAPI-056-002 | DL-HP-01b | test_label_detector.py | 1.1 |
| UT-HAPI-056-003 | DL-HP-02 | test_label_detector.py | 1.1 |
| UT-HAPI-056-004 | DL-HP-02a | test_label_detector.py | 1.1 |
| UT-HAPI-056-005 | DL-HP-02b | test_label_detector.py | 1.1 |
| UT-HAPI-056-006 | DL-HP-02c | test_label_detector.py | 1.1 |
| UT-HAPI-056-007 | DL-HP-03 | test_label_detector.py | 1.1 |
| UT-HAPI-056-008 | DL-HP-04 | test_label_detector.py | 1.1 |
| UT-HAPI-056-009 | DL-HP-05 | test_label_detector.py | 1.1 |
| UT-HAPI-056-010 | DL-HP-06 | test_label_detector.py | 1.1 |
| UT-HAPI-056-011 | DL-HP-06b | test_label_detector.py | 1.1 |
| UT-HAPI-056-012 | DL-HP-07 | test_label_detector.py | 1.1 |
| UT-HAPI-056-013 | DL-HP-08 | test_label_detector.py | 1.1 |
| UT-HAPI-056-014 | DL-HP-09 | test_label_detector.py | 1.1 |
| UT-HAPI-056-015 | DL-EC-01 | test_label_detector.py | 1.1 |
| UT-HAPI-056-016 | DL-EC-02 | test_label_detector.py | 1.1 |
| UT-HAPI-056-017 | DL-EC-03 | test_label_detector.py | 1.1 |
| UT-HAPI-056-018 | DL-ER-01 | test_label_detector.py | 1.1 |
| UT-HAPI-056-019 | DL-ER-02 | test_label_detector.py | 1.1 |
| UT-HAPI-056-020 | DL-ER-03 | test_label_detector.py | 1.1 |
| UT-HAPI-056-021 | DL-ER-04 | test_label_detector.py | 1.1 |
| UT-HAPI-056-022 | UT-K8S-LQ-001 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-023 | UT-K8S-LQ-002 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-024 | UT-K8S-LQ-003 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-025 | UT-K8S-LQ-004 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-026 | UT-K8S-LQ-005 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-027 | UT-K8S-LQ-006 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-028 | UT-K8S-LQ-007 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-029 | UT-K8S-LQ-008 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-030 | UT-K8S-LQ-009 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-031 | UT-K8S-LQ-010 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-032 | UT-K8S-LQ-011 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-033 | UT-K8S-LQ-012 | test_k8s_client_label_queries.py | 1.2 |
| UT-HAPI-056-034 | UT-RC-SS-001 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-035 | UT-RC-SS-002 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-036 | UT-RC-SS-003 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-037 | UT-RC-SS-005 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-038 | UT-RC-SS-006 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-039 | UT-RC-SS-007 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-040 | UT-RC-SS-008 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-041 | UT-RC-SS-009 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-042 | UT-RC-SS-010 | test_resource_context_session_state.py | 1.3 |
| UT-HAPI-056-043 | UT-FE-001 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-044 | UT-FE-002 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-045 | UT-FE-003 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-046 | UT-FE-005 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-047 | UT-FE-006 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-048 | UT-FE-007 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-049 | UT-FE-008 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-050 | UT-FE-009 | test_workflow_discovery_flow_enforcement.py | 2.1 |
| UT-HAPI-056-051 | UT-CP-001 | test_workflow_discovery_context_params.py | 2.2 |
| UT-HAPI-056-052 | UT-CP-003 | test_workflow_discovery_context_params.py | 2.2 |
| UT-HAPI-056-053 | UT-CP-004 | test_workflow_discovery_context_params.py | 2.2 |
| UT-HAPI-056-054 | UT-CP-005 | test_workflow_discovery_context_params.py | 2.2 |
| UT-HAPI-056-055 | UT-CP-006 | test_workflow_discovery_context_params.py | 2.2 |
| UT-HAPI-056-056 | UT-PP-001 | test_prompt_no_detected_labels.py | 2.3 |
| UT-HAPI-056-057 | UT-PP-002 | test_prompt_no_detected_labels.py | 2.3 |
| UT-HAPI-056-058 | UT-PP-003 | test_prompt_no_detected_labels.py | 2.3 |
| UT-HAPI-056-059 | UT-SW-001 | test_session_state_wiring.py | 2.4 |
| UT-HAPI-056-060 | UT-SW-002 | test_session_state_wiring.py | 2.4 |
| UT-HAPI-056-061 | UT-SW-003 | test_session_state_wiring.py | 2.4 |
| UT-HAPI-056-062 | UT-SW-005 | test_session_state_wiring.py | 2.4 |
| UT-HAPI-056-063 | UT-RL-001 | test_response_detected_labels.py | 2.5 |
| UT-HAPI-056-064 | UT-RL-002 | test_response_detected_labels.py | 2.5 |
| UT-HAPI-056-065 | UT-RL-003 | test_response_detected_labels.py | 2.5 |
| UT-HAPI-056-066 | UT-RL-004 | test_response_detected_labels.py | 2.5 |
| UT-HAPI-056-067 | UT-RL-005 | test_response_detected_labels.py | 2.5 |

| UT-HAPI-056-068 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-069 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-070 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-071 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-072 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-073 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-074 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-075 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-076 | NEW | test_should_include_detected_labels.py | 3.1 |
| UT-HAPI-056-077 | NEW | test_label_detector.py | 3.2 |
| UT-HAPI-056-078 | NEW | test_label_detector.py | 3.2 |
| UT-HAPI-056-079 | NEW | test_label_detector.py | 3.2 |
| UT-HAPI-056-080 | NEW | test_label_detector.py | 3.2 |

---

## Integration Tests: DetectedLabels End-to-End Flow (7 tests)

**File**: `holmesgpt-api/tests/integration/test_detected_labels_integration.py`
**Fixtures**: `holmesgpt-api/tests/integration/fixtures/k8s_mock_fixtures.py`
**Production code**: `src/extensions/incident/llm_integration.py`, `src/extensions/recovery/llm_integration.py`, `src/toolsets/workflow_discovery.py`, `src/detection/labels.py`

### Infrastructure

- Python `AsyncMock` K8s client (mock fixtures for PDB, HPA, NetworkPolicy, Namespace)
- Direct calls to `analyze_incident` / `analyze_recovery` (no HTTP, no Mock LLM)
- Real `LabelDetector`, `WorkflowDiscoveryToolset`, session_state wiring

### Summary

| ID | Scenario (BDD) | BR | Priority |
|----|----------------|-------|----------|
| IT-HAPI-056-001 | Given CrashLoopBackOff + K8s resources (Deployment + PDB), When analyze_incident completes, Then `detected_labels.pdbProtected=true` | BR-SP-101, ADR-056 | P0 |
| IT-HAPI-056-002 | Given recovery + K8s resources (Deployment + HPA), When analyze_recovery completes, Then `detected_labels.hpaEnabled=true` | BR-SP-101, ADR-056 | P0 |
| IT-HAPI-056-003 | Given detected_labels, When workflow discovery queries DataStorage, Then query params include `detected_labels` (stripped of failedDetections) | BR-HAPI-194 | P1 |
| IT-HAPI-056-004 | Given Helm-managed + ArgoCD-managed Deployment, When analyze_incident completes, Then `helmManaged=true`, `gitOpsManaged=true`, `gitOpsTool=argocd` | BR-SP-101 | P1 |
| IT-HAPI-056-005 | Given K8s API returns RBAC 403 for PDB list, When analyze_incident completes, Then `failedDetections` includes `pdbProtected` | BR-SP-103 | P1 |
| IT-HAPI-056-006 | Given no K8s resources found, When analyze_incident completes, Then `detected_labels` contains all-false booleans | BR-SP-101 | P2 |
| IT-HAPI-056-007 | Given labels computed during list_available_actions, When list_workflows/get_workflow called, Then cached labels reused (no recomputation) | ADR-056 | P2 |

---

## E2E Tests: DetectedLabels in Kind Cluster (3 tests)

**File**: `test/e2e/holmesgpt-api/detected_labels_e2e_test.go`
**Infrastructure**: Kind cluster with HAPI, Mock LLM (3-step), DataStorage, real K8s resources

### Summary

| ID | Scenario (BDD) | BR | Priority |
|----|----------------|-------|----------|
| E2E-HAPI-056-001 | Given HAPI deployed in Kind with K8s resources, When incident analysis completes via session client, Then response includes `detected_labels` | ADR-056, BR-SP-101 | P0 |
| E2E-HAPI-056-002 | Given a recovery analysis request, When session completes, Then `detected_labels` present in recovery response | ADR-056 | P0 |
| E2E-HAPI-056-003 | Given Deployment with PDB and HPA in Kind cluster, When incident analysis runs, Then `detected_labels` correctly detects both | BR-SP-101 | P1 |

---

## Updated Code Surface by Tier

| Tier | Files | Tests | Coverage | Target |
|------|-------|-------|----------|--------|
| Unit | `labels.py`, `k8s_client.py`, `resource_context.py`, `workflow_discovery.py`, `prompt_builder.py` (x2), `llm_config.py`, `incident_models.py`, `recovery_models.py` | 80 | >= 80% | MET |
| Integration | `test_detected_labels_integration.py` + fixtures | 7 | >= 80% | Active |
| E2E | `detected_labels_e2e_test.go` | 3 | >= 80% | Active |
