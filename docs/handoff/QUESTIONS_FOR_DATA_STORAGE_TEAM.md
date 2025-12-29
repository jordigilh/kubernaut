# Questions from HolmesGPT-API Team

**From**: HolmesGPT-API Team
**To**: Data Storage Team
**Date**: December 1, 2025
**Re**: Workflow Catalog Integration - Follow-up Items

---

## 🔔 SCHEMA CHANGE: DetectedLabels `failedDetections` (Dec 2, 2025)

**Status**: ⚠️ **ACTION REQUIRED** - DetectedLabels schema updated per DD-WORKFLOW-001 v2.1

**Change**: Added `failedDetections []string` field to track detection failures. Avoids `*bool` anti-pattern.

**Impact on Workflow Filtering**: When matching incident `DetectedLabels` against workflow catalog `detected_labels`, you must **skip fields that are in `failedDetections`**.

**New Schema**:
```json
{
  "failedDetections": ["pdbProtected", "hpaEnabled"],
  "gitOpsManaged": true,
  "gitOpsTool": "argocd",
  "pdbProtected": false,   // SKIP - in failedDetections
  "hpaEnabled": false,     // SKIP - in failedDetections
  "stateful": false,
  "helmManaged": true,
  "networkIsolated": false
}
```

**SQL Filtering Adjustment**:
```sql
-- BEFORE: Match all provided labels
WHERE (workflow.detected_labels->>'gitOpsManaged' = 'true'
       OR workflow.detected_labels->>'gitOpsManaged' IS NULL)
  AND (workflow.detected_labels->>'pdbProtected' = 'true'
       OR workflow.detected_labels->>'pdbProtected' IS NULL)

-- AFTER: Skip fields in failedDetections
WHERE (workflow.detected_labels->>'gitOpsManaged' = 'true'
       OR workflow.detected_labels->>'gitOpsManaged' IS NULL)
  AND (
    'pdbProtected' = ANY($failed_detections)  -- Skip if failed
    OR workflow.detected_labels->>'pdbProtected' = signal.detected_labels->>'pdbProtected'
    OR workflow.detected_labels->>'pdbProtected' IS NULL
  )
```

**Validation**: `failedDetections` only accepts known field names:
- `gitOpsManaged`, `pdbProtected`, `hpaEnabled`, `stateful`, `helmManaged`, `networkIsolated`, `podSecurityLevel`, `serviceMesh`

**Key Distinction** (per SignalProcessing team):

| Scenario | `pdbProtected` | `failedDetections` | SQL Filtering |
|----------|----------------|-------------------|---------------|
| PDB exists | `true` | `[]` | ✅ Apply filter |
| No PDB | `false` | `[]` | ✅ Apply filter |
| RBAC denied | `false` | `["pdbProtected"]` | ⚠️ Skip filter |

**"Resource doesn't exist" ≠ detection failure** - it's a successful detection with result `false`. Only query errors (RBAC, timeout) go in `failedDetections`.

**Authoritative Source**: DD-WORKFLOW-001 v2.1

---

### ✅ Data Storage Team Response (Dec 2, 2025)

**Status**: ✅ **ACKNOWLEDGED** - Implementation plan documented below

#### Impact Assessment

| Component | Impact | Action Required |
|-----------|--------|-----------------|
| `pkg/shared/types/enrichment.go` | **HIGH** | Add `FailedDetections []string` to `DetectedLabels` struct |
| `pkg/datastorage/models/workflow.go` | **MEDIUM** | Add `FailedDetections` to search filter `DetectedLabels` |
| `pkg/datastorage/repository/workflow_repository.go` | **HIGH** | Update SQL filtering to skip `failedDetections` fields |
| `pkg/datastorage/audit/workflow_search_event.go` | **LOW** | Include `failedDetections` in audit events |

#### Current State Analysis

1. **Authoritative Schema** (`pkg/shared/types/enrichment.go`):
   - Current version: DD-WORKFLOW-001 v2.1 (comments reference this)
   - **Missing**: `FailedDetections` field not yet added to Go struct
   - **Note**: The schema comments mention "NO `*bool` anti-pattern" but the implementation approach differs from HolmesGPT-API

2. **Data Storage Filter Model** (`pkg/datastorage/models/workflow.go`):
   - Uses `*bool` for boolean fields (for optional filtering)
   - This is **correct for filtering** (null = no filter, true = filter for true, false = filter for false)
   - Different from incident `DetectedLabels` which uses plain `bool`

3. **SQL Filtering** (`pkg/datastorage/repository/workflow_repository.go`):
   - Currently does NOT handle `failedDetections` skip logic
   - Needs update to implement the SQL pattern shown above

#### Implementation Plan (BR-STORAGE-020)

**Phase 1: Schema Update** (Priority: HIGH)
```go
// pkg/shared/types/enrichment.go - ADD to DetectedLabels struct
type DetectedLabels struct {
    // ... existing fields ...

    // FailedDetections lists fields where detection failed (RBAC, timeout, etc.)
    // DD-WORKFLOW-001 v2.1: Consumers MUST skip these fields when filtering
    // Valid values: gitOpsManaged, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, podSecurityLevel, serviceMesh
    FailedDetections []string `json:"failedDetections,omitempty" validate:"omitempty,dive,oneof=gitOpsManaged pdbProtected hpaEnabled stateful helmManaged networkIsolated podSecurityLevel serviceMesh"`
}
```

**Phase 2: Filter Model Update** (Priority: MEDIUM)
```go
// pkg/datastorage/models/workflow.go - ADD to DetectedLabels filter struct
type DetectedLabels struct {
    // ... existing fields ...

    // FailedDetections from incident - fields to skip during filtering
    FailedDetections []string `json:"failed_detections,omitempty"`
}
```

**Phase 3: SQL Filtering Update** (Priority: HIGH)
- Update `SearchByEmbedding()` to accept `failedDetections` parameter
- Modify WHERE clause generation to skip fields in `failedDetections`
- Add unit tests for skip logic

#### Questions for AI Analysis Team

1. **Who passes `failedDetections` to Data Storage?**
   - Is it HolmesGPT-API (as part of the search request)?
   - Or should Data Storage parse it from the incident's `DetectedLabels`?

2. **Filtering Semantics Confirmation**:
   - If `pdbProtected` is in `failedDetections`, should we:
     - A) Skip the `pdbProtected` filter entirely (treat as "no preference")
     - B) Only skip if the workflow requires `pdbProtected=true`
   - Current understanding: Option A (skip entirely)

3. **Backwards Compatibility**:
   - If `failedDetections` is omitted from request, assume all detections succeeded?
   - Confirm: empty array = all detections succeeded

---

#### ✅ AIAnalysis Team Response (Dec 2, 2025)

**Status**: ✅ **ALL QUESTIONS ANSWERED**

**Authoritative Source**: DD-WORKFLOW-001 v2.1, Section "Detection Failure Handling"

---

##### A1: Who passes `failedDetections` to Data Storage?

✅ **HolmesGPT-API passes it** as part of the workflow search request.

**Data Flow**:
```
SignalProcessing → AIAnalysis → HolmesGPT-API → Data Storage
                                    │
                                    └── Search Request includes failedDetections
```

---

##### A2: Filtering Semantics

✅ **Option A is correct** - Skip the filter entirely (treat as "no preference")

**Per DD-WORKFLOW-001 v2.1**:
> "If a field name is in this array, its value should be ignored"

**SQL**:
```sql
AND (
    'pdbProtected' = ANY($failed_detections)  -- Skip if failed
    OR workflow.detected_labels->>'pdbProtected' IS NULL
    OR workflow.detected_labels->>'pdbProtected' = incident.detected_labels->>'pdbProtected'
)
```

---

##### A3: Backwards Compatibility

✅ **Not applicable** - V1.0 pre-release, no backwards compatibility required.

`failedDetections` is **mandatory** in V1.0. Empty array `[]` = all detections succeeded.

---

#### Timeline

| Task | Estimate | Dependency |
|------|----------|------------|
| Schema update (`enrichment.go`) | 1 hour | None |
| Filter model update | 1 hour | Schema update |
| SQL filtering implementation | 4 hours | Filter model |
| Unit tests | 2 hours | SQL implementation |
| Integration tests | 2 hours | Unit tests |
| **Total** | **10 hours** | - |

**Target Completion**: V1.0 ✅ **APPROVED - PROCEEDING WITH IMPLEMENTATION**

---

#### ✅ Data Storage Team Acknowledgement (Dec 2, 2025)

**Status**: ✅ **ALL ANSWERS RECEIVED - IMPLEMENTATION STARTING**

All clarifying questions have been answered by the AI Analysis team. Implementation will proceed with:

1. **Data Flow Confirmed**: HolmesGPT-API → Data Storage (search request includes `failedDetections`)
2. **Filtering Semantics Confirmed**: Option A - Skip filter entirely (treat as "no preference")
3. **Backwards Compatibility**: Not applicable - V1.0 pre-release, `failedDetections` is mandatory

**Implementation Approach**:
- Follow TDD methodology (RED → GREEN → REFACTOR)
- Unit tests first, then integration tests
- Per TESTING_GUIDELINES.md: Unit tests validate implementation correctness, integration tests validate behavior

**Business Requirement**: BR-STORAGE-020 - failedDetections support in workflow search

---

## Context

The HolmesGPT-API team has successfully integrated with Data Storage for workflow catalog search. The `custom_labels` pass-through is now working end-to-end as confirmed in our integration tests (v3.2 release).

---

## Questions

### Q1: container_image and container_digest in Search Response

**Observation**: During integration testing, we noticed that `container_image` and `container_digest` fields are correctly stored via the CREATE endpoint but may not appear in SEARCH responses.

**Question**:
1. Are these fields intentionally excluded from search results?
2. If they should be included, is there a bug fix in progress?

**HolmesGPT-API Impact**: We need these fields for the Tekton PipelineRun to specify which container image runs the workflow.

---

### Q2: Pagination for Large Catalogs

**Current Behavior**: HolmesGPT-API requests `limit=10` for workflow searches.

**Questions**:
1. Is pagination fully supported in the current API?
2. What's the maximum recommended `limit` value?
3. Is there a cursor-based pagination option for deterministic results?

---

### Q3: detected_labels Auto-Population

**Per DD-WORKFLOW-001 v1.6**: Data Storage should auto-populate 9 `detected_labels` fields:
- `git_ops_managed`, `git_ops_tool`
- `has_pdb`, `pdb_min_available`
- `has_hpa`, `hpa_min_replicas`, `hpa_max_replicas`
- `pod_security_level`, `network_policy_exists`

**Questions**:
1. Is this auto-population implemented?
2. If so, when/how are these labels populated (at CREATE time or lazily)?
3. Can HolmesGPT-API rely on these being present in search results?

---

### Q4: Semantic Search Quality

**Observation**: Search results with `custom_labels` filtering work correctly.

**Questions**:
1. What embedding model is used for semantic similarity?
2. Is there a minimum similarity threshold we should expect?
3. Are there plans to tune the `label_boost` / `label_penalty` weights?

---

## Confirmed Working ✅

The following are confirmed working (no questions):
- `POST /api/v1/workflows/search` with `custom_labels[subdomain]=value` query params
- JSONB containment filtering: `custom_labels @> '{"constraint": ["cost-constrained"]}'::jsonb`
- `custom_labels` returned in search response
- Basic semantic search with signal_type filtering

---

## Action Items

| Item | Owner | Status |
|------|-------|--------|
| Clarify container_image in search | DS Team | ✅ **CONFIRMED WORKING** |
| Confirm pagination support | DS Team | ✅ **CONFIRMED** (top_k max 100) |
| Clarify detected_labels population | DS Team | ✅ **CLARIFIED** - Auto-population NOT DS responsibility (see A3) |

---

## Response

**Date**: December 1, 2025
**Responder**: Data Storage Team

**Answers**:

### A1: container_image and container_digest in Search Response ✅ CONFIRMED WORKING

**Status**: These fields ARE included in search responses.

**Implementation Details**:
- `container_image` and `container_digest` are correctly mapped in `WorkflowSearchResult` struct (`pkg/datastorage/models/workflow.go` lines 326-330)
- The `SearchByEmbedding` repository method extracts these fields from the database and includes them in the response (`pkg/datastorage/repository/workflow_repository.go` lines 717-735)
- Recent fix (commit in this session) ensured these fields are properly stored via the CREATE endpoint by adding them to the INSERT statement

**Code Reference**:
```go
// WorkflowSearchResult (pkg/datastorage/models/workflow.go)
ContainerImage  string `json:"container_image,omitempty"`
ContainerDigest string `json:"container_digest,omitempty"`
```

**Verification**: Integration test `Hybrid Scoring End-to-End` now validates `container_image` and `container_digest` are returned in search results.

---

### A2: Pagination for Large Catalogs ✅ FULLY SUPPORTED

**Status**: Pagination is fully implemented.

**Implementation Details**:

1. **Semantic Search (`POST /api/v1/workflows/search`)**:
   - Uses `top_k` parameter (not `limit`/`offset`)
   - Default: 10, Maximum: 100
   - Validation: `validate:"omitempty,min=1,max=100"` (`pkg/datastorage/models/workflow.go` line 163)

2. **List Workflows (`GET /api/v1/workflows`)**:
   - Uses `limit`/`offset` query parameters
   - Default limit: 50, Maximum: 100
   - Returns `total` count in response for calculating pages
   - Code: `pkg/datastorage/server/workflow_handlers.go` lines 301-313

3. **Cursor-based Pagination**:
   - **Not currently implemented** for workflow catalog
   - Offset-based pagination is used
   - For deterministic results, workflows are ordered by `created_at DESC`

**Recommendations**:
- For HolmesGPT-API use case (`limit=10`), the current `top_k` approach is optimal
- Maximum recommended `top_k` value: **100** (enforced by validation)
- For large catalogs needing cursor pagination, please file a feature request (BR-STORAGE-XXX)

---

### A3: detected_labels Auto-Population ✅ CLARIFIED (NOT DS RESPONSIBILITY)

> ⚠️ **TERMINOLOGY CLARIFICATION (Dec 2, 2025)**: There was confusion between two different concepts. See **DD-WORKFLOW-001 v2.0** for authoritative end-to-end flow.

#### **Two Different Contexts for "detected_labels"**

| Context | Owner | When Populated | Purpose |
|---------|-------|----------------|---------|
| **Incident DetectedLabels** | SignalProcessing | At runtime (V1.0 ✅ IMPLEMENTED) | Auto-detected facts about the **incident's** K8s environment |
| **Workflow Catalog detected_labels** | Workflow Author | At workflow creation (manual) | Metadata describing what environments a workflow **supports** |

#### **Clarification**

**Original Question**: "Should Data Storage auto-populate the 9 `detected_labels` fields?"

**Answer**: **NO** - This was based on a misunderstanding.

1. **SignalProcessing** auto-detects `DetectedLabels` from the **live K8s cluster** at incident time ✅ IMPLEMENTED (V1.0)
   - Checks ArgoCD/Flux annotations → `gitOpsManaged`, `gitOpsTool`
   - Queries PDB, HPA → `pdbProtected`, `hpaEnabled`
   - Checks helm.sh/chart label → `helmManaged`

2. **Data Storage** stores `detected_labels` as **workflow metadata** (author-defined constraints)
   - Workflow authors specify which environments their workflow supports
   - Example: `{"gitOpsTool": "argocd"}` means "this workflow only supports ArgoCD-managed workloads"
   - This is NOT auto-populated - it's design-time metadata

3. **HolmesGPT-API** passes the incident's DetectedLabels as **search filters** to Data Storage
   - Data Storage matches incident labels against workflow metadata

#### **Current State (Correct)**

| Component | Status | Notes |
|-----------|--------|-------|
| Schema | ✅ Implemented | `detected_labels` JSONB column exists |
| Filtering | ✅ Implemented | Wildcard `*` support working |
| Auto-population | ❌ **NOT NEEDED** | Workflow authors define this manually |

#### **Why DS Auto-Population Was Never Needed**

The original question asked if DS should auto-populate workflow `detected_labels` at CREATE time. This was a misunderstanding:

- Workflow `detected_labels` describe what the workflow **SUPPORTS** (author intent)
- Incident `DetectedLabels` describe what the workload **IS** (runtime facts)
- These are matched during search, not populated by the same service

**Authoritative Reference**: DD-WORKFLOW-001 v2.0 "DetectedLabels End-to-End Architecture"

---

### A4: Semantic Search Quality ✅ DOCUMENTED

**Embedding Model**:
- **Model**: `sentence-transformers/all-mpnet-base-v2`
- **Dimensions**: 768 (per migration 016, enforced in code)
- **Reference**: `pkg/datastorage/models/workflow.go` line 95

**Similarity Threshold**:
- **Default**: 0.7 (70% cosine similarity)
- **Configurable**: Via `min_similarity` parameter in search request (0.0-1.0)
- **Reference**: `pkg/datastorage/models/workflow.go` lines 166-167

**Hybrid Scoring Weights** (DD-WORKFLOW-004 v1.1):
- **Label Boost**: +0.10 per matching optional label
- **Label Penalty**: -0.10 per conflicting optional label
- **Final Score**: `min(base_similarity + label_boost - label_penalty, 1.0)` (capped at 100%)
- **Tuning Plans**: Current weights are based on initial testing. We're collecting metrics to tune these values post-MVP. Please share any feedback on search quality.

**Future Enhancement (v1.1 Candidate)**: True score normalization (scaling all scores to 0-1 range based on max possible score) is being considered for v1.1 pending user feedback on whether capping at 1.0 is sufficient for ranking purposes.

**Index Optimization**:
- HNSW index on `embedding` column for fast approximate nearest neighbor search
- GIN index on `labels` JSONB for efficient label filtering

---

## Updated Action Items

| Item | Owner | Status |
|------|-------|--------|
| Clarify container_image in search | DS Team | ✅ **CONFIRMED WORKING** |
| Confirm pagination support | DS Team | ✅ **CONFIRMED** (top_k max 100) |
| Clarify detected_labels population | DS Team | ✅ **CLARIFIED** - Auto-population NOT DS responsibility (see A3) |

---

