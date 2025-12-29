# REQUEST: Complete Data Storage OpenAPI Spec with Workflow Endpoints

**From**: HAPI Team
**To**: Data Storage Team
**Date**: 2025-12-13
**Status**: ✅ **COMPLETE** (2025-12-13)
**Priority**: 🔴 **HIGH** (Blocks HAPI migration completion)
**Type**: Specification Completion Request

---

## ✅ **DS TEAM RESPONSE - COMPLETE**

**Status**: ✅ **COMPLETE**
**Completed By**: Data Storage Team (AI Assistant)
**Completion Date**: 2025-12-13
**Spec Location**: `api/openapi/data-storage-v1.yaml`
**Validation**: ✅ PASS (all endpoints and schemas added)

**Changes Made**:
1. ✅ Added 5 workflow endpoints (search, create, list, get, update, disable)
2. ✅ Added complete `WorkflowSearchFilters` schema with all 7 fields
3. ✅ Added `WorkflowSearchRequest`, `WorkflowSearchResponse`, `WorkflowSearchResult` schemas
4. ✅ Added complete `RemediationWorkflow` schema
5. ✅ Added `DetectedLabels`, `WorkflowUpdateRequest`, `WorkflowDisableRequest` schemas
6. ✅ **Corrected terminology**: "Label-based search" (not "semantic search" - pgvector removed in V1.0)

**Important Note**: The workflow search is **label-based** (not semantic) in V1.0 after pgvector removal. The spec now correctly reflects this.

**Next Steps for HAPI Team**:
1. Regenerate client: `./src/clients/generate-datastorage-client.sh`
2. Run tests: `pytest tests/unit/test_custom_labels_auto_append_dd_hapi_001.py`
3. Verify all 4 tests pass

---

---

## 📋 Request Summary

**Request**: Add workflow search and CRUD endpoints to `api/openapi/data-storage-v1.yaml`

**Reason**: The current authoritative spec is incomplete - it only has audit/incident endpoints, missing all workflow-related endpoints.

**Impact**: HAPI's OpenAPI client migration is blocked because the generated client is missing critical fields and endpoints.

---

## 🎯 What's Needed

### ✅ Endpoints Added

All requested endpoints have been added to `api/openapi/data-storage-v1.yaml`:

1. ✅ **POST `/api/v1/workflows/search`** - **Label-based workflow search** (corrected from "semantic search")
2. ✅ **POST `/api/v1/workflows`** - Create workflow
3. ✅ **GET `/api/v1/workflows/{workflow_id}`** - Get workflow by UUID
4. ✅ **GET `/api/v1/workflows`** - List workflows
5. ✅ **PATCH `/api/v1/workflows/{workflow_id}/disable`** - Disable workflow (corrected from PUT to PATCH)

**Note**: Endpoint #1 terminology corrected - V1.0 uses **label-based search** (not semantic search) after pgvector removal per CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md (92% confidence).

### ✅ Schemas Added

All requested schemas have been added to the `components/schemas` section:

#### 1. WorkflowSearchRequest
```yaml
WorkflowSearchRequest:
  type: object
  required:
    - filters
  properties:
    query:
      type: string
      description: Natural language search query (optional for label-only search)
    filters:
      $ref: '#/components/schemas/WorkflowSearchFilters'
    top_k:
      type: integer
      default: 5
      description: Maximum number of results to return
    min_similarity:
      type: number
      format: float
      default: 0.3
      description: Minimum similarity threshold (0.0-1.0)
    remediation_id:
      type: string
      description: Optional remediation ID for audit trail
```

#### 2. WorkflowSearchFilters (COMPLETE VERSION)
```yaml
WorkflowSearchFilters:
  type: object
  required:
    - signal_type
    - severity
    - component
    - environment
    - priority
  properties:
    signal_type:
      type: string
      description: "Signal type (mandatory: OOMKilled, CrashLoopBackOff, etc.)"
      example: "OOMKilled"
    severity:
      type: string
      description: "Severity level (mandatory: critical, high, medium, low)"
      example: "critical"
    component:
      type: string
      description: "Component type (mandatory: pod, node, deployment, etc.)"
      example: "pod"
    environment:
      type: string
      description: "Environment (mandatory: production, staging, development)"
      example: "production"
    priority:
      type: string
      description: "Priority level (mandatory: P0, P1, P2, P3, P4)"
      example: "P0"
    custom_labels:
      type: object
      additionalProperties:
        type: array
        items:
          type: string
      description: "Custom labels (subdomain -> values mapping)"
      example:
        team: ["name=payments"]
        constraint: ["cost-constrained", "stateful-safe"]
    detected_labels:
      type: object
      additionalProperties: true
      description: "Detected labels (boolean or string values)"
      example:
        gitOpsManaged: true
        gitOpsTool: "argocd"
```

#### 3. WorkflowSearchResponse
```yaml
WorkflowSearchResponse:
  type: object
  properties:
    workflows:
      type: array
      items:
        $ref: '#/components/schemas/RemediationWorkflow'
    total_results:
      type: integer
      description: Total number of matching workflows
    search_metadata:
      type: object
      properties:
        query_time_ms:
          type: integer
        search_strategy:
          type: string
          enum: ["label_only", "semantic", "hybrid"]
```

#### 4. RemediationWorkflow
```yaml
RemediationWorkflow:
  type: object
  required:
    - workflow_id
    - workflow_name
    - version
    - name
    - description
    - content
    - labels
  properties:
    workflow_id:
      type: string
      format: uuid
      description: "Unique workflow identifier (UUID)"
    workflow_name:
      type: string
      description: "Workflow name (identifier for versions)"
    version:
      type: string
      description: "Semantic version (e.g., 1.0.0)"
    name:
      type: string
      description: "Human-readable workflow title"
    description:
      type: string
      description: "Workflow description"
    content:
      type: string
      description: "YAML workflow definition"
    labels:
      type: object
      additionalProperties:
        type: string
      description: "Workflow labels"
    custom_labels:
      type: object
      additionalProperties:
        type: array
        items:
          type: string
      description: "Custom labels"
    detected_labels:
      type: object
      additionalProperties: true
      description: "Detected labels"
    container_image:
      type: string
      description: "OCI image reference"
      example: "ghcr.io/kubernaut/workflows/oomkill:v1.0.0@sha256:abc123..."
    container_digest:
      type: string
      description: "OCI image digest"
      example: "sha256:abc123..."
    status:
      type: string
      enum: ["active", "disabled", "deprecated", "archived"]
    created_at:
      type: string
      format: date-time
    updated_at:
      type: string
      format: date-time
```

---

## 📖 Reference Implementation

**Source**: `docs/services/stateless/data-storage/openapi/v3.yaml` (deprecated but has workflow endpoints)

You can use this as a reference for the workflow endpoint definitions. The schemas above are based on the actual Go implementation in Data Storage.

---

## 🔍 Current Problem

### Generated Client is Incomplete

**Current Generated Model** (`WorkflowSearchFilters`):
```python
class WorkflowSearchFilters(BaseModel):
    signal_type: Optional[StrictStr] = None    # ✅ Present
    severity: Optional[StrictStr] = None       # ✅ Present
    environment: Optional[StrictStr] = None    # ✅ Present
    # ❌ MISSING: component
    # ❌ MISSING: priority
    # ❌ MISSING: custom_labels
    # ❌ MISSING: detected_labels
```

**Expected Model** (from Go code):
```python
class WorkflowSearchFilters(BaseModel):
    signal_type: Optional[StrictStr] = None    # ✅ Present
    severity: Optional[StrictStr] = None       # ✅ Present
    component: Optional[StrictStr] = None      # ❌ MISSING
    environment: Optional[StrictStr] = None    # ✅ Present
    priority: Optional[StrictStr] = None       # ❌ MISSING
    custom_labels: Optional[Dict[str, List[str]]] = None  # ❌ MISSING
    detected_labels: Optional[Dict[str, Any]] = None      # ❌ MISSING
```

### Test Failures

```
FAILED test_auto_append_custom_labels_to_filters
  AssertionError: assert False
   where False = hasattr(WorkflowSearchFilters(...), 'custom_labels')

FAILED test_custom_labels_structure_preserved
  AttributeError: 'WorkflowSearchFilters' object has no attribute 'custom_labels'
```

---

## 🎯 Success Criteria

**Spec is Complete When**:
- ✅ `/api/v1/workflows/search` endpoint is defined
- ✅ `WorkflowSearchFilters` has all 7 fields (signal_type, severity, component, environment, priority, custom_labels, detected_labels)
- ✅ `WorkflowSearchRequest` schema is complete
- ✅ `RemediationWorkflow` schema is complete
- ✅ Spec validates: `openapi-generator-cli validate -i api/openapi/data-storage-v1.yaml`

**HAPI Can Proceed When**:
- ✅ Client regenerated from complete spec
- ✅ All 4 unit tests pass
- ✅ No manual patches needed

---

## ⏱️ Timeline Estimate

**DS Team Effort**: 2-4 hours
- Copy endpoint definitions from old spec
- Update schemas to match Go implementation
- Validate spec
- Notify HAPI team

**HAPI Team Effort**: 30 minutes (after DS completion)
- Regenerate client
- Run tests
- Verify migration complete

---

## 🤝 Coordination

### DS Team Actions

1. **Review** old spec: `docs/services/stateless/data-storage/openapi/v3.yaml`
2. **Add** workflow endpoints to `api/openapi/data-storage-v1.yaml`
3. **Ensure** all 7 fields in `WorkflowSearchFilters`
4. **Validate** spec with `openapi-generator-cli validate`
5. **Notify** HAPI team in this document (update status below)

### HAPI Team Actions (After DS Completion)

1. **Regenerate** client: `./src/clients/generate-datastorage-client.sh`
2. **Run** unit tests: `pytest tests/unit/test_custom_labels_auto_append_dd_hapi_001.py`
3. **Verify** all tests pass
4. **Update** migration status documents

---

## 📊 Status Tracking

**Request Status**: ✅ **COMPLETE - READY FOR HAPI TEAM**

| Task | Owner | Status | Notes |
|---|---|---|---|
| Add workflow endpoints to spec | DS Team | ✅ COMPLETE | All 5 endpoints added (2025-12-13) |
| Validate spec | DS Team | ✅ COMPLETE | All schemas match Go implementation |
| Regenerate HAPI client | HAPI Team | ⏳ READY | Can proceed now |
| Verify tests pass | HAPI Team | ⏳ READY | Can proceed now |

---

## 💬 DS Team Response

**✅ SPEC COMPLETION CONFIRMED**

```
Status: COMPLETE
Completed By: Data Storage Team (AI Assistant)
Completion Date: 2025-12-13
Spec Location: api/openapi/data-storage-v1.yaml
Validation: PASS

Notes:
- All 5 workflow endpoints added with complete documentation
- All 9 schemas added (WorkflowSearchRequest, WorkflowSearchFilters, DetectedLabels,
  WorkflowSearchResponse, WorkflowSearchResult, RemediationWorkflow, WorkflowListResponse,
  WorkflowUpdateRequest, WorkflowDisableRequest)
- WorkflowSearchFilters now has all 7 required fields:
  * signal_type ✅
  * severity ✅
  * component ✅
  * environment ✅
  * priority ✅
  * custom_labels ✅
  * detected_labels ✅
- IMPORTANT: Corrected terminology from "semantic search" to "label-based search"
  (V1.0 removed pgvector/embeddings per CONFIDENCE_ASSESSMENT_REMOVE_EMBEDDINGS.md)
- HTTP method corrected: PATCH (not PUT) for disable endpoint per REST conventions
```

---

## 🔗 Related Documents

- **Triage**: `TRIAGE_OPENAPI_SPEC_INCOMPLETE.md` - Detailed problem analysis
- **Migration Status**: `HAPI_OPENAPI_MIGRATION_COMPLETE.md` - Current migration state
- **Old Spec**: `docs/services/stateless/data-storage/openapi/v3.yaml` - Reference implementation
- **Go Implementation**: `pkg/datastorage/models/workflow.go` - Source of truth for schemas

---

**Thank you for your help completing the OpenAPI spec!** 🙏

This will unblock HAPI's migration and provide a complete API contract for all teams.

---

**Created**: 2025-12-13
**By**: HAPI Team
**Priority**: 🔴 HIGH
**Waiting On**: Data Storage Team

