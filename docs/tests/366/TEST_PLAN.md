# Test Plan: Detect Namespace ResourceQuota and Surface to LLM

**Feature**: Detect Kubernetes ResourceQuota objects in a namespace and surface quota details (hard limits, usage, remaining) to the LLM via the HolmesGPT API context enrichment pipeline
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-SP-101: DetectedLabels Auto-Detection -- cluster characteristics
- BR-HAPI-250: DetectedLabels in workflow search
- DD-HAPI-018 v1.3 -> v1.4: Detection 8 -- ResourceQuota Constrained
- #366: Detect namespace ResourceQuota/LimitRange and surface to LLM

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [DD-HAPI-018: DetectedLabels Detection Specification](../../architecture/decisions/DD-HAPI-018-detected-labels-detection-specification.md)
- GitHub Issue: [#366](https://github.com/jordigilh/kubernaut/issues/366)

---

## 1. Scope

### In Scope

- **K8s client** (`kubernaut-agent/src/clients/k8s_client.py`): New `_list_resource_quotas_sync` and `list_resource_quotas` methods following the established `_list_pdbs_sync`/`list_pdbs` pattern
- **LabelDetector** (`kubernaut-agent/src/detection/labels.py`): New `_detect_resource_quota` and `_summarize_quotas` methods, signature change to return `Tuple[Optional[Dict], Optional[Dict]]`
- **Resource Context** (`kubernaut-agent/src/toolsets/resource_context.py`): Tuple destructuring in `_detect_labels_if_needed`, new `quota_details` top-level field in `result_data`
- **DetectedLabels Go struct** (`pkg/shared/types/enrichment.go`): New `ResourceQuotaConstrained bool` field
- **FailedDetections enum sync** (4 locations): Add `resourceQuotaConstrained` to kubebuilder annotation, generated CRD, DS OpenAPI spec, and Go validation tag
- **DD-HAPI-018 v1.4**: Specification for Detection 8 (ResourceQuota Constrained)

### Out of Scope

- **LimitRange detection**: Deferred to a follow-up issue; #366 focuses on ResourceQuota only
- **Quantity parsing**: Raw Kubernetes quantity strings (e.g., "8Gi", "2500m") are passed to the LLM as-is; no numeric conversion needed for data enrichment
- **E2E tests**: This is a data enrichment pass (query K8s, format, pass to LLM) with no new CRD types, no new API endpoints, and no multi-service coordination; UT + IT provide sufficient defense-in-depth
- **CRD schema changes beyond DetectedLabels**: No new CRDs introduced

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Option C: `detect_labels()` returns `Tuple[Optional[Dict], Optional[Dict]]` | Explicit contract; caller destructures `(labels, quota_summary)`. Avoids hidden state on `LabelDetector` instance and keeps labels dict flat (no nested dict pollution). Minimal impact since backwards compatibility is not required. |
| Aggregate across multiple ResourceQuotas in same namespace | Tightest hard limit per resource, sum used. Mirrors how K8s enforces quotas when multiple exist. |
| Raw quantity strings passed to LLM (no parsing, no `_remaining` keys) | LLMs can interpret "8Gi" and "2500m" natively and compute remaining themselves. Parsing adds complexity, bug surface, and risk of wrong data with no proportional value. Quota summary contains only `_hard` and `_used` per resource. |
| `resourceQuotaConstrained` as boolean + separate `quota_details` | Boolean in DetectedLabels for policy/filter use; structured quota_details at top level for LLM consumption. Keeps DetectedLabels flat. |
| Fix pre-existing `gitOpsTool` enum divergence in same PR | Avoids leaving a known inconsistency; isolated in a separate commit to keep the diff clean. |
| Integration tests placed in `test_resource_context_session_state.py` | Avoids dependency on Go-bootstrapped infrastructure in `tests/integration/conftest.py`. Uses mocked K8s client that already exists in this file. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (K8s client list methods, LabelDetector quota detection, quota summarization, resource context quota_details wiring, Go DetectedLabels struct)
- **Integration**: >=80% of integration-testable code (full pipeline: K8s client -> LabelDetector -> Resource Context -> session state + tool result)

### 2-Tier Minimum

Every business requirement gap is covered by Unit + Integration tiers:
- **Unit tests** validate detection logic correctness (single quota, multi-quota aggregation, RBAC errors, empty namespace), tuple return contract, and Go struct round-tripping
- **Integration tests** validate end-to-end pipeline: K8s has quotas -> `resourceQuotaConstrained=true` in session state AND `quota_details` in tool result

### Business Outcome Quality Bar

Tests validate business outcomes -- "LLM receives accurate quota context for remediation strategy decisions" and "operator policies can filter on `resourceQuotaConstrained` boolean" -- not just code path coverage. Each test asserts what the LLM or operator observes.

---

## 3. Testable Code Inventory

### Python Unit-Testable Code (mocked K8s API)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `kubernaut-agent/src/clients/k8s_client.py` | `_list_resource_quotas_sync`, `list_resource_quotas` | ~25 |
| `kubernaut-agent/src/detection/labels.py` | `_detect_resource_quota`, `_summarize_quotas`, tuple return from `detect_labels` | ~40 |
| `kubernaut-agent/src/toolsets/resource_context.py` | Tuple destructuring in `_detect_labels_if_needed`, `quota_details` in `result_data` | ~15 |

### Go Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/types/enrichment.go` | `ResourceQuotaConstrained bool` field, `FailedDetections` enum update | ~5 |

### Python Integration-Testable Code (wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `kubernaut-agent/src/toolsets/resource_context.py` | `_detect_labels_if_needed` full pipeline with real `LabelDetector` | ~15 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SP-101 | K8s client lists ResourceQuotas in namespace | P0 | Unit | UT-HAPI-366-001 | Pending |
| BR-SP-101 | K8s client returns empty list when no quotas | P0 | Unit | UT-HAPI-366-002 | Pending |
| BR-SP-103 | K8s client returns error string on API failure | P0 | Unit | UT-HAPI-366-003 | Pending |
| BR-SP-101 | resourceQuotaConstrained=true when quota exists | P0 | Unit | UT-HAPI-366-004 | Pending |
| BR-SP-101 | resourceQuotaConstrained=false when no quota | P0 | Unit | UT-HAPI-366-005 | Pending |
| BR-SP-103 | failedDetections includes resourceQuotaConstrained on RBAC error | P0 | Unit | UT-HAPI-366-006 | Pending |
| BR-HAPI-250 | Quota summary has cpu/memory/pods with hard, used, remaining | P0 | Unit | UT-HAPI-366-007 | Pending |
| BR-HAPI-250 | Quota summary aggregates across 2 quotas (tightest hard limit) | P0 | Unit | UT-HAPI-366-008 | Pending |
| BR-HAPI-250 | Quota summary is None when no quotas exist | P0 | Unit | UT-HAPI-366-009 | Pending |
| BR-SP-103 | Quota summary is None on detection error (graceful degradation) | P0 | Unit | UT-HAPI-366-010 | Pending |
| BR-HAPI-250 | quota_details top-level field in tool result_data when quotas exist | P0 | Unit | UT-HAPI-366-011 | Pending |
| BR-HAPI-250 | quota_details absent from tool result_data when no quotas | P0 | Unit | UT-HAPI-366-012 | Pending |
| BR-HAPI-250 | detected_infrastructure.labels contains only flat bool/string values | P0 | Unit | UT-HAPI-366-013 | Pending |
| BR-SP-101 / BR-SP-103 | ResourceQuotaConstrained + FailedDetections round-trip through JSON into PostRCAContext | P0 | Unit | UT-AA-366-001 | Pending |
| BR-HAPI-250 | Full pipeline: quotas present -> label=true + quota_details in result | P0 | Integration | IT-HAPI-366-001 | Pending |
| BR-HAPI-250 | Full pipeline: no quotas -> label=false + no quota_details in result | P0 | Integration | IT-HAPI-366-002 | Pending |

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

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `HAPI` (HolmesGPT API), `AA` (AIAnalysis)
- **BR_NUMBER**: 366 (issue number)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Python Unit Tests -- K8s Client (3 tests)

**Testable code scope**: `kubernaut-agent/src/clients/k8s_client.py` -- `_list_resource_quotas_sync`, `list_resource_quotas` (~25 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-366-001` | `list_resource_quotas` returns quota items when quotas exist in namespace | Pending |
| `UT-HAPI-366-002` | `list_resource_quotas` returns empty list when no quotas exist | Pending |
| `UT-HAPI-366-003` | `list_resource_quotas` returns error string on K8s API failure | Pending |

**File**: `kubernaut-agent/tests/unit/test_k8s_client_label_queries.py` (extend existing)
**Pattern**: `_make_client()` with mocked `_core_v1.list_namespaced_resource_quota`, assert `(items, error)` tuple. Same as existing `UT-HAPI-056-022` through `033`.

### Tier 1: Python Unit Tests -- LabelDetector (7 tests)

**Testable code scope**: `kubernaut-agent/src/detection/labels.py` -- `_detect_resource_quota`, `_summarize_quotas`, tuple return (~40 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-366-004` | `resourceQuotaConstrained=true` in labels when ResourceQuota exists | Pending |
| `UT-HAPI-366-005` | `resourceQuotaConstrained=false` in labels when no ResourceQuota (not in failedDetections) | Pending |
| `UT-HAPI-366-006` | `resourceQuotaConstrained=false` + `failedDetections` includes `"resourceQuotaConstrained"` on RBAC error | Pending |
| `UT-HAPI-366-007` | Quota summary (2nd tuple element) has cpu/memory/pods with hard and used keys | Pending |
| `UT-HAPI-366-008` | Quota summary aggregates across 2 quotas in same namespace (tightest hard per resource) | Pending |
| `UT-HAPI-366-009` | Quota summary is None when no quotas exist | Pending |
| `UT-HAPI-366-010` | Quota summary is None on detection error (graceful degradation) | Pending |

**File**: `kubernaut-agent/tests/unit/test_label_detector.py` (extend existing)
**Pattern**: Tests call `detect_labels()`, destructure `(labels, quota_summary)`, and assert both elements.

Mock fixture for UT-HAPI-366-007:
- Hard: cpu=4, memory=8Gi, pods=20; Used: cpu=2500m, memory=6Gi, pods=15
- Assert keys: `cpu_hard`, `cpu_used`, `memory_hard`, `memory_used`, `pods_hard`, `pods_used` (no `_remaining` keys -- LLM computes remaining from hard/used)

Mock fixture for UT-HAPI-366-008 (multi-quota aggregation):
- Quota A: cpu hard=4 used=2; Quota B: memory hard=8Gi used=6Gi
- Assert both resource types present in summary

### Tier 1: Python Unit Tests -- Resource Context (3 tests)

**Testable code scope**: `kubernaut-agent/src/toolsets/resource_context.py` -- `_detect_labels_if_needed` tuple destructuring, `quota_details` in `result_data` (~15 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-366-011` | `quota_details` top-level field in tool `result_data` when quotas exist, with hard/used per resource | Pending |
| `UT-HAPI-366-012` | `quota_details` absent from tool `result_data` when no quotas | Pending |
| `UT-HAPI-366-013` | `detected_infrastructure.labels` contains only flat bool/string values (no nested dict) | Pending |

**File**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py` (extend existing)
**Pattern**: Mock `LabelDetector.detect_labels` to return `(labels_dict, quota_summary)` tuple. Assert `result_data["quota_details"]` and `result_data["detected_infrastructure"]["labels"]` structure.

### Tier 1: Go Unit Tests -- DetectedLabels (2 tests)

**Testable code scope**: `pkg/shared/types/enrichment.go` -- `ResourceQuotaConstrained` field, `FailedDetections` enum (~5 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AA-366-001` | `ResourceQuotaConstrained=true` and `FailedDetections=["resourceQuotaConstrained"]` round-trip through JSON into `PostRCAContext.DetectedLabels` | Pending |

**File**: `test/unit/aianalysis/response_processor_post_rca_test.go` (extend existing, Ginkgo/Gomega BDD)
**Pattern**: Follow existing `UT-AA-056-*` test structure. Single JSON marshal/unmarshal round-trip covering both the new bool field and the new FailedDetections enum value.

### Tier 2: Python Integration Tests (2 tests)

**Testable code scope**: Full pipeline -- `K8sResourceClient` -> `LabelDetector` -> `GetResourceContextTool` -> session state + tool result (~15 lines wiring, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-HAPI-366-001` | Full pipeline: K8s has quotas -> `resourceQuotaConstrained=true` in session state AND `quota_details` in tool result | Pending |
| `IT-HAPI-366-002` | Full pipeline: K8s has no quotas -> `resourceQuotaConstrained=false` in session state AND no `quota_details` in tool result | Pending |

**File**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py` (extend existing)
**Rationale**: Uses mocked K8s client already available in this file. Avoids dependency on Go-bootstrapped infrastructure in `tests/integration/conftest.py` (Risk 5 mitigation).
**Pattern**: Uses real `LabelDetector` (not patched) with mocked K8s client extended with `list_resource_quotas` mock. Asserts both session_state and tool result.

### Tier Skip Rationale

- **E2E**: Data-enrichment pass only (query K8s, format, pass to LLM). No new CRD types, no new API endpoints, no multi-service coordination. UT + IT provide sufficient defense-in-depth per the 2-tier minimum policy.

---

## 6. Test Cases (Detail)

### UT-HAPI-366-001: list_resource_quotas returns items when quotas exist

**BR**: BR-SP-101
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_k8s_client_label_queries.py`

**Given**: K8s API has 1 ResourceQuota in namespace "default" with cpu hard=4, memory hard=8Gi
**When**: `list_resource_quotas("default")` is called
**Then**: Returns `([quota_item], None)` where `quota_item.status.hard` contains "cpu" and "memory"

**Acceptance Criteria**:
- Returned tuple first element is a non-empty list
- Returned tuple second element is None (no error)
- Each item has `.spec.hard` and `.status` attributes accessible

---

### UT-HAPI-366-002: list_resource_quotas returns empty list when no quotas

**BR**: BR-SP-101
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_k8s_client_label_queries.py`

**Given**: K8s API has no ResourceQuotas in namespace "default"
**When**: `list_resource_quotas("default")` is called
**Then**: Returns `([], None)`

**Acceptance Criteria**:
- Returned tuple first element is an empty list
- Returned tuple second element is None (no error)

---

### UT-HAPI-366-003: list_resource_quotas returns error on K8s API failure

**BR**: BR-SP-103
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_k8s_client_label_queries.py`

**Given**: K8s API raises `ApiException(status=403, reason="Forbidden")` on list
**When**: `list_resource_quotas("default")` is called
**Then**: Returns `([], "Forbidden")` or `([], error_string)` containing the reason

**Acceptance Criteria**:
- Returned tuple first element is an empty list
- Returned tuple second element is a non-empty string describing the error
- No exception propagated to caller

---

### UT-HAPI-366-004: resourceQuotaConstrained=true when quota exists

**BR**: BR-SP-101
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_label_detector.py`

**Given**: `list_resource_quotas` returns 1 quota with cpu hard=4
**When**: `detect_labels(k8s_context, owner_chain)` is called
**Then**: `labels["resourceQuotaConstrained"]` is `True`; `"resourceQuotaConstrained"` not in `labels["failedDetections"]`

**Acceptance Criteria**:
- `labels` (first tuple element) contains `resourceQuotaConstrained: True`
- `failedDetections` list does not contain `"resourceQuotaConstrained"`
- All other detection fields remain unchanged (existing detections unaffected)

---

### UT-HAPI-366-005: resourceQuotaConstrained=false when no quota

**BR**: BR-SP-101
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_label_detector.py`

**Given**: `list_resource_quotas` returns empty list `([], None)`
**When**: `detect_labels(k8s_context, owner_chain)` is called
**Then**: `labels["resourceQuotaConstrained"]` is `False`; `"resourceQuotaConstrained"` not in `labels["failedDetections"]`

**Acceptance Criteria**:
- `labels` (first tuple element) contains `resourceQuotaConstrained: False`
- `failedDetections` list does not contain `"resourceQuotaConstrained"` (absence is a valid result, not a failure)

---

### UT-HAPI-366-006: failedDetections includes resourceQuotaConstrained on RBAC error

**BR**: BR-SP-103
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_label_detector.py`

**Given**: `list_resource_quotas` returns `([], "Forbidden")`
**When**: `detect_labels(k8s_context, owner_chain)` is called
**Then**: `labels["resourceQuotaConstrained"]` is `False`; `labels["failedDetections"]` contains `"resourceQuotaConstrained"`

**Acceptance Criteria**:
- `labels` (first tuple element) contains `resourceQuotaConstrained: False`
- `failedDetections` list includes `"resourceQuotaConstrained"`
- `quota_summary` (second tuple element) is `None`

---

### UT-HAPI-366-007: Quota summary has cpu/memory/pods with hard and used

**BR**: BR-HAPI-250
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_label_detector.py`

**Given**: `list_resource_quotas` returns 1 quota: hard={cpu: "4", memory: "8Gi", pods: "20"}, used={cpu: "2500m", memory: "6Gi", pods: "15"}
**When**: `detect_labels(k8s_context, owner_chain)` is called
**Then**: Second tuple element is a dict with keys `cpu_hard`, `cpu_used`, `memory_hard`, `memory_used`, `pods_hard`, `pods_used`

**Acceptance Criteria**:
- `quota_summary` (second tuple element) is not None
- `quota_summary["cpu_hard"]` equals `"4"` (raw quantity string)
- `quota_summary["memory_hard"]` equals `"8Gi"` (raw quantity string)
- `quota_summary["pods_hard"]` equals `"20"`
- `quota_summary["cpu_used"]` equals `"2500m"`
- `quota_summary["memory_used"]` equals `"6Gi"`
- `quota_summary["pods_used"]` equals `"15"`
- No `_remaining` keys present (LLM computes remaining from hard/used)

---

### UT-HAPI-366-008: Quota summary aggregates across 2 quotas

**BR**: BR-HAPI-250
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_label_detector.py`

**Given**: `list_resource_quotas` returns 2 quotas: Quota A (cpu hard=4, cpu used=2), Quota B (memory hard=8Gi, memory used=6Gi)
**When**: `detect_labels(k8s_context, owner_chain)` is called
**Then**: Second tuple element contains both `cpu_hard` and `memory_hard` keys; tightest hard limit used when resource appears in multiple quotas

**Acceptance Criteria**:
- `quota_summary` contains `cpu_hard` from Quota A
- `quota_summary` contains `memory_hard` from Quota B
- Both resource types present in a single summary dict
- `resourceQuotaConstrained` is `True` in labels

---

### UT-HAPI-366-009: Quota summary is None when no quotas exist

**BR**: BR-HAPI-250
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_label_detector.py`

**Given**: `list_resource_quotas` returns `([], None)`
**When**: `detect_labels(k8s_context, owner_chain)` is called
**Then**: Second tuple element is `None`

**Acceptance Criteria**:
- `quota_summary` (second tuple element) is exactly `None` (not empty dict)
- `labels["resourceQuotaConstrained"]` is `False`

---

### UT-HAPI-366-010: Quota summary is None on detection error

**BR**: BR-SP-103
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_label_detector.py`

**Given**: `list_resource_quotas` raises `Exception("connection refused")`
**When**: `detect_labels(k8s_context, owner_chain)` is called
**Then**: Second tuple element is `None`; `labels["failedDetections"]` contains `"resourceQuotaConstrained"`

**Acceptance Criteria**:
- `quota_summary` (second tuple element) is `None`
- `labels["resourceQuotaConstrained"]` is `False`
- `labels["failedDetections"]` includes `"resourceQuotaConstrained"`
- Other detections (gitOps, pdb, hpa, etc.) are unaffected

---

### UT-HAPI-366-011: quota_details in tool result_data when quotas exist

**BR**: BR-HAPI-250
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py`

**Given**: `LabelDetector.detect_labels` returns `(labels_with_quota_true, quota_summary_dict)`
**When**: `_detect_labels_if_needed` is called via the tool
**Then**: `result_data["quota_details"]` is present and equals the quota_summary dict

**Acceptance Criteria**:
- `result_data` contains `"quota_details"` key
- `result_data["quota_details"]` matches the quota_summary from detect_labels
- `result_data["detected_infrastructure"]["labels"]` does not contain quota_details (flat)

---

### UT-HAPI-366-012: quota_details absent when no quotas

**BR**: BR-HAPI-250
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py`

**Given**: `LabelDetector.detect_labels` returns `(labels_with_quota_false, None)`
**When**: `_detect_labels_if_needed` is called via the tool
**Then**: `result_data` does not contain `"quota_details"` key

**Acceptance Criteria**:
- `"quota_details"` key not present in `result_data`
- `result_data["detected_infrastructure"]["labels"]["resourceQuotaConstrained"]` is `False`

---

### UT-HAPI-366-013: Labels dict contains only flat values

**BR**: BR-HAPI-250
**Type**: Unit (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py`

**Given**: `LabelDetector.detect_labels` returns `(labels_dict, quota_summary_dict)` where labels has all detection fields
**When**: `_detect_labels_if_needed` places labels into `detected_infrastructure`
**Then**: Every value in `detected_infrastructure["labels"]` is a `bool`, `str`, or `list` (no nested dict)

**Acceptance Criteria**:
- All values in the labels dict are primitive types (bool, str) or list of str (for failedDetections)
- No value is a dict -- confirms quota_summary is not leaking into labels

---

### UT-AA-366-001: ResourceQuotaConstrained + FailedDetections round-trip through JSON

**BR**: BR-SP-101 / BR-SP-103
**Type**: Unit (Go, Ginkgo/Gomega BDD)
**File**: `test/unit/aianalysis/response_processor_post_rca_test.go`

**Given**: JSON response from HAPI contains `"resourceQuotaConstrained": true` and `"failedDetections": ["resourceQuotaConstrained"]` in detected_labels
**When**: ResponseProcessor extracts DetectedLabels into PostRCAContext
**Then**: `PostRCAContext.DetectedLabels.ResourceQuotaConstrained` is `true` and `PostRCAContext.DetectedLabels.FailedDetections` contains `"resourceQuotaConstrained"`

**Acceptance Criteria**:
- `ResourceQuotaConstrained` field is `true` after JSON unmarshal
- `FailedDetections` contains `"resourceQuotaConstrained"` after JSON unmarshal
- No error during processing
- Existing DetectedLabels fields remain unaffected

---

### IT-HAPI-366-001: Full pipeline with quotas present

**BR**: BR-HAPI-250
**Type**: Integration (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py`

**Given**: Mocked K8s client with `list_resource_quotas` returning 1 quota (cpu hard=4 used=2500m, memory hard=8Gi used=6Gi, pods hard=20 used=15)
**When**: `GetResourceContextTool` executes full pipeline (real `LabelDetector`, mocked K8s)
**Then**: Session state has `detected_labels["resourceQuotaConstrained"] == True` AND tool result contains `quota_details` with hard/used per resource

**Acceptance Criteria**:
- `session_state["detected_labels"]["resourceQuotaConstrained"]` is `True`
- Tool `result_data` contains `"quota_details"` key with resource breakdown
- `result_data["detected_infrastructure"]["labels"]` is flat (no quota_details nested)
- Other detections (gitOps, pdb, hpa, etc.) still function correctly in the pipeline

---

### IT-HAPI-366-002: Full pipeline with no quotas

**BR**: BR-HAPI-250
**Type**: Integration (Python, pytest)
**File**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py`

**Given**: Mocked K8s client with `list_resource_quotas` returning `([], None)`
**When**: `GetResourceContextTool` executes full pipeline (real `LabelDetector`, mocked K8s)
**Then**: Session state has `detected_labels["resourceQuotaConstrained"] == False` AND tool result does not contain `quota_details`

**Acceptance Criteria**:
- `session_state["detected_labels"]["resourceQuotaConstrained"]` is `False`
- `"resourceQuotaConstrained"` not in `session_state["detected_labels"]["failedDetections"]`
- `"quota_details"` key not present in tool `result_data`
- Other detections still function correctly

---

## 7. Test Infrastructure

### Python Unit Tests

- **Framework**: pytest (HolmesGPT API standard)
- **Mocks**: K8s API only (`_core_v1.list_namespaced_resource_quota` via `MagicMock`)
- **Location**: `kubernaut-agent/tests/unit/`
- **Existing patterns**: Follow `_make_client()` and `_make_k8s_queries()` helpers in existing test files

### Python Integration Tests

- **Framework**: pytest
- **Mocks**: K8s API only (mocked client), real `LabelDetector` business logic
- **Location**: `kubernaut-agent/tests/unit/test_resource_context_session_state.py` (co-located per Risk 5 mitigation)
- **Infrastructure**: No external services needed; mocked K8s client provides all data

### Go Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None needed (pure type round-tripping)
- **Location**: `test/unit/aianalysis/`

---

## 8. Execution

```bash
# Python unit tests (K8s client + LabelDetector + Resource Context)
cd kubernaut-agent && python -m pytest tests/unit/test_k8s_client_label_queries.py -v -k "366"
cd kubernaut-agent && python -m pytest tests/unit/test_label_detector.py -v -k "366"
cd kubernaut-agent && python -m pytest tests/unit/test_resource_context_session_state.py -v -k "366"

# Python integration tests
cd kubernaut-agent && python -m pytest tests/unit/test_resource_context_session_state.py -v -k "IT_HAPI_366"

# Go unit tests
go test ./test/unit/aianalysis/... --ginkgo.focus="366"

# All #366 tests
cd kubernaut-agent && python -m pytest tests/unit/ -v -k "366"
go test ./test/unit/aianalysis/... --ginkgo.focus="366"
```

---

## 9. Risk Mitigations (from Triage)

| Risk | Mitigation | Validated By |
|------|-----------|--------------|
| R1: Multiple ResourceQuotas in same namespace | Aggregation logic in `_summarize_quotas`: tightest hard limit per resource, sum used | UT-HAPI-366-008 |
| R2: quota_summary leaking into labels dict | Option C: `detect_labels` returns `Tuple[Optional[Dict], Optional[Dict]]`; labels remain flat | UT-HAPI-366-013 |
| R3: RBAC for `resourcequotas` | RBAC already granted in `kubernaut-agent.yaml`; graceful degradation on 403 | UT-HAPI-366-003, UT-HAPI-366-006, UT-HAPI-366-010 |
| R4: FailedDetections enum divergence | Add `gitOpsTool` + `resourceQuotaConstrained` to all 4 authoritative sources + run `make manifests && make generate && make generate-datastorage-client`; `gitOpsTool` fix in separate commit | UT-AA-366-001 |
| R5: Integration test infra dependency | IT tests placed in `test_resource_context_session_state.py` using mocked K8s, not in `tests/integration/` | IT-HAPI-366-001, IT-HAPI-366-002 |
| R6: Quantity parsing complexity | Pass raw quantity strings to LLM; no `_remaining` keys; no parsing needed | UT-HAPI-366-007 (asserts raw strings) |
| R7: Enum sync surface is 10+ locations | 4 manual edits + 3 make targets for derived files; grep validation after | Section 10 checklist |
| R8: ~47 existing test call sites break on tuple return | Batch update all `detect_labels()` calls atomically; run existing suite to confirm no breakage | Existing test pass |
| R9: Python Pydantic model needs new field + enum | Add to `DETECTED_LABELS_FIELD_NAMES` + add bool field to `DetectedLabels` model | Section 10 checklist |
| R10: Go test asserts exact field count (8) | Update `validFields` and `HaveLen` in `workflow_search_failed_detections_test.go` | Section 10 checklist |

---

## 10. FailedDetections Enum Sync Checklist

### Authoritative sources (manual edits -- 4 files)

- [ ] `pkg/shared/types/enrichment.go` line 140: Add `gitOpsTool,resourceQuotaConstrained` to `+kubebuilder:validation:items:Enum`; add `ResourceQuotaConstrained bool` field to struct
- [ ] `api/openapi/data-storage-v1.yaml` line 2920: Add `resourceQuotaConstrained` to `failedDetections.items.enum`; add `resourceQuotaConstrained` boolean property
- [ ] `pkg/datastorage/models/workflow_labels.go` line 121: Add `resourceQuotaConstrained` to `oneof` validation tag; add to `ValidDetectedLabelFields` slice; add `ResourceQuotaConstrained bool` field
- [ ] `kubernaut-agent/src/models/incident_models.py` line 121: Add `"resourceQuotaConstrained"` to `DETECTED_LABELS_FIELD_NAMES` set; add `resourceQuotaConstrained: bool = Field(default=False, ...)` to Pydantic model; update `failedDetections` Field description

### Derived (regenerated -- run after manual edits)

- [ ] `make manifests` -> regenerates `config/crd/bases/kubernaut.ai_aianalyses.yaml` + `charts/kubernaut/crds/kubernaut.ai_aianalyses.yaml`
- [ ] `make generate` -> copies OpenAPI spec to `pkg/datastorage/server/middleware/openapi_spec_data.yaml` + `pkg/audit/openapi_spec_data.yaml`
- [ ] `make generate-datastorage-client` -> regenerates `pkg/datastorage/ogen-client/oas_*_gen.go` + `kubernaut-agent/src/clients/datastorage/.../detected_labels.py`

### Tests to update

- [ ] `test/unit/datastorage/workflow_search_failed_detections_test.go` line 132: Add `"resourceQuotaConstrained"` to `validFields` slice; line 140: change `HaveLen(8)` to `HaveLen(9)`

### Pre-existing divergence fix

`gitOpsTool` is in Go struct and DS OpenAPI/models but missing from CRD kubebuilder enum. Fixed in same PR, separate commit.

---

## 11. Existing Test Updates (Tuple Destructuring)

Existing tests that call `detect_labels()` must be updated to destructure the new tuple return:

- `kubernaut-agent/tests/unit/test_label_detector.py` (~35 call sites):
  - All test classes (`TestLabelDetectorHappyPath`, `TestLabelDetectorEdgeCases`, `TestLabelDetectorErrorHandling`, `TestLabelDetectorBranchGaps`)
  - Each `result = await detector.detect_labels(...)` becomes `result, _ = await detector.detect_labels(...)`
  - `_make_k8s_queries()` helper extended with `list_resource_quotas` mock (defaults to `([], None)` to preserve existing test behavior)
- `kubernaut-agent/tests/unit/test_resource_context_session_state.py` (~12 mock sites):
  - Each `mock_detector.detect_labels.return_value = LABELS_...` becomes `mock_detector.detect_labels.return_value = (LABELS_..., None)`
  - `mock_detector.detect_labels.return_value = None` becomes `mock_detector.detect_labels.return_value = (None, None)`

Additional test file to update:

- `test/unit/datastorage/workflow_search_failed_detections_test.go`: Add `"resourceQuotaConstrained"` to `validFields` slice (line 132), change `HaveLen(8)` to `HaveLen(9)` (line 140)

---

## 12. Anti-Pattern Compliance

| Anti-Pattern | Rule | Compliance |
|-------------|------|------------|
| `time.Sleep()` | FORBIDDEN in all test tiers | Not used; Python uses `await`, Go uses `Eventually()` |
| `Skip()` / `XIt` | FORBIDDEN in all test tiers | Not used; all 17 tests fully implemented |
| NULL-TESTING (`Expect().ToNot(BeNil())`) | FORBIDDEN per pre-commit hook | All assertions test concrete values (e.g., `assert labels["resourceQuotaConstrained"] is True`, `Expect(dl.ResourceQuotaConstrained).To(BeTrue())`) |
| Mock business logic | FORBIDDEN per testing strategy | Only K8s API is mocked; `LabelDetector` logic runs as real code in IT tests |
| Business outcome blind | Tests must answer "what does the user see?" | Every test validates LLM/operator observable output |

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan -- 16 scenarios (12 Python UT, 1 Go UT, 2 Python IT, 1 removed by folding UT-AA-366-002 into 001). Option C tuple return design. No `_remaining` keys (LLM computes from hard/used). Risk mitigations from triage incorporated. |
