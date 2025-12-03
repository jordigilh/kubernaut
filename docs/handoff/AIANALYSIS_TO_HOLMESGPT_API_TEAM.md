# Questions from AIAnalysis Team

**From**: AIAnalysis Service Team
**To**: HolmesGPT-API Team
**Date**: December 1, 2025
**Context**: V1.0 integration requirements and MCP workflow coordination

---

## 🔔 HolmesGPT-API Team Acknowledgment (Dec 2, 2025)

**Status**: ✅ **ALL CHANGES IMPLEMENTED**

The HolmesGPT-API team acknowledges receipt of all requests and confirms the following implementations:

| Request | Status | Notes |
|---------|--------|-------|
| `failedDetections` field in DetectedLabels | ✅ **DONE** | With Pydantic validation |
| OpenAPI 3.1.0 native support | ✅ **DONE** | Use `ogen` for Go client |
| `target_in_owner_chain` field | ✅ **DONE** | In IncidentResponse |
| `warnings[]` field | ✅ **DONE** | In IncidentResponse |
| OpenAPI spec regenerated | ✅ **DONE** | 16 schemas, 3.1.0 |

**AIAnalysis Next Step**: Regenerate Go client to pick up all changes:
```bash
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json
```

**Files Changed**:
- `holmesgpt-api/src/models/incident_models.py` - Added `failedDetections`, `target_in_owner_chain`, `warnings`
- `holmesgpt-api/api/openapi.json` - Regenerated (OpenAPI 3.1.0)
- `holmesgpt-api/api/export_openapi.py` - Native 3.1.0 output

---

## ✅ IMPLEMENTED: DetectedLabels Schema Change (Dec 2, 2025)

**Status**: ✅ **IMPLEMENTED BY HAPI TEAM**

**Change**: Added `failedDetections` field to track detection failures (avoids `*bool` anti-pattern)

**New Schema**:
```json
{
  "failedDetections": ["pdbProtected", "hpaEnabled"],  // NEW: Fields where detection failed
  "gitOpsManaged": true,
  "gitOpsTool": "argocd",
  "pdbProtected": false,   // Ignore - in failedDetections
  "hpaEnabled": false,     // Ignore - in failedDetections
  "stateful": false,
  "helmManaged": true,
  "networkIsolated": false
}
```

**Validation**: ✅ `failedDetections` only accepts known field names (Pydantic validator added):
- `gitOpsManaged`, `gitOpsTool`, `pdbProtected`, `hpaEnabled`, `stateful`, `helmManaged`, `networkIsolated`, `podSecurityLevel`, `serviceMesh`

**Consumer Logic**:
- If a field is in `failedDetections`, ignore its value (detection failed)
- If `failedDetections` is empty/omitted, all detections succeeded

**Key Distinction** (per SignalProcessing team):

| Scenario | `pdbProtected` | `failedDetections` | Meaning |
|----------|----------------|-------------------|---------|
| PDB exists | `true` | `[]` | ✅ Has PDB - use for filtering |
| No PDB | `false` | `[]` | ✅ No PDB - use for filtering |
| RBAC denied | `false` | `["pdbProtected"]` | ⚠️ Unknown - skip filter |

**"Resource doesn't exist" ≠ detection failure** - it's a successful detection with result `false`.

**Implementation**: `holmesgpt-api/src/models/incident_models.py`
```python
failedDetections: List[str] = Field(
    default_factory=list,
    description="Field names where detection failed..."
)

@field_validator('failedDetections')
@classmethod
def validate_failed_detections(cls, v: List[str]) -> List[str]:
    """Validate that failedDetections only contains known field names."""
    invalid_fields = set(v) - DETECTED_LABELS_FIELD_NAMES
    if invalid_fields:
        raise ValueError(f"Invalid field names: {invalid_fields}")
    return v
```

**OpenAPI Spec**: ✅ Regenerated with new field

**Authoritative Source**: DD-WORKFLOW-001 v2.1

**AIAnalysis Action**: Regenerate Go client to get new `FailedDetections` field:
```bash
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json
```

---

## ✅ RESOLVED: OpenAPI 3.1.0 with `ogen` (Dec 2, 2025)

**Status**: ✅ **FIXED - Using native OpenAPI 3.1.0 with `ogen`**

**Solution**: Use [`ogen`](https://github.com/ogen-go/ogen) instead of `oapi-codegen` for Go client generation.

**Why `ogen`**:
- ✅ Native OpenAPI 3.1.0 support (no conversion needed)
- ✅ Handles `anyOf: [type, null]` nullable fields natively
- ✅ Built-in OpenTelemetry instrumentation
- ✅ Type-safe optional types (`OptNil...` for nullable fields)
- ✅ Compiles successfully - tested Dec 2, 2025

**OpenAPI Spec**: `holmesgpt-api/api/openapi.json` (OpenAPI 3.1.0, 16 schemas)

**AIAnalysis Go Client Generation**:
```bash
# Install ogen (one-time)
go install github.com/ogen-go/ogen/cmd/ogen@latest

# Generate Go client from OpenAPI 3.1.0 spec
mkdir -p pkg/clients/holmesgpt
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json
```

**Generated Files**:
```
pkg/clients/holmesgpt/
├── oas_client_gen.go      # Client implementation
├── oas_schemas_gen.go     # Type definitions (89KB)
├── oas_validators_gen.go  # Request/response validation
├── oas_json_gen.go        # JSON serialization
└── ... (18 files total)
```

**Impact**: ✅ **UNBLOCKED** - AIAnalysis can generate type-safe Go client with native 3.1.0 support

---

## 🔔 UPDATE: HolmesGPT-API Response to Q9-Q11 (Dec 2, 2025)

**Status**: ✅ **ALL REQUESTED CHANGES IMPLEMENTED**

The HolmesGPT-API team has implemented all changes requested by AIAnalysis:

| Request | Status | Details |
|---------|--------|---------|
| Add `target_in_owner_chain: bool` to `IncidentResponse` | ✅ **DONE** | Default: `true` |
| Add `warnings: List[str]` to `IncidentResponse` | ✅ **DONE** | Default: `[]` |
| Regenerate OpenAPI spec | ✅ **DONE** | 16 schemas, OpenAPI 3.1.0 |

### AIAnalysis Next Steps

1. ✅ **DONE: Go client generated** - Using `ogen` with native OpenAPI 3.1.0:
   ```bash
   # Client generated at pkg/clients/holmesgpt/ (18 files, ~12,600 lines)
   ogen -package holmesgpt -target pkg/clients/holmesgpt \
       holmesgpt-api/api/openapi.json
   ```

2. ✅ **New fields available in `IncidentResponse`**:
   - `TargetInOwnerChain OptBool` - For Rego policy input
   - `Warnings []string` - Non-fatal warnings

3. ✅ **Build verified** - Full project compiles with generated client

---

## Background

AIAnalysis is the **only consumer** of HolmesGPT-API. We're preparing our implementation plan and need to confirm integration details.

**AIAnalysis → HolmesGPT-API data flow**:
```
AIAnalysis spec → HolmesGPT-API request:
- EnrichmentResults.DetectedLabels → MCP workflow filtering
- EnrichmentResults.CustomLabels → LLM prompt context
- EnrichmentResults.OwnerChain → DetectedLabels validation
- EnrichmentResults.KubernetesContext → Investigation context
```

---

## Questions

### Q1: MCP Server Port and Endpoint

Per our documentation, HolmesGPT-API hosts an MCP server for workflow catalog access.

**Question**: What are the confirmed connection details?
- MCP Server Port: `____` (we have `8085` documented - is this correct?)
- Base endpoint: `/mcp/v1/...` or different?
- Authentication: API key header, mTLS, or none?

---

### Q2: DetectedLabels → MCP Workflow Filtering

AIAnalysis passes `DetectedLabels` to HolmesGPT-API for filtering the workflow catalog:

> ⚠️ **CORRECTION (HAPI Team, 2025-12-02)**: The schema below is incorrect. See authoritative schema in `pkg/shared/types/enrichment.go` and `RESPONSE_TO_AIANALYSIS_TEAM.md` Q2 for correct fields (gitOpsManaged, pdbProtected, serviceMesh, etc.).

```go
// ❌ INCORRECT SCHEMA (original question)
type DetectedLabels struct {
    Environment       string // e.g., "production"
    Region           string // e.g., "us-west-2"
    Platform         string // e.g., "kubernetes"
    ClusterType      string // e.g., "eks"
    ServiceMesh      string // e.g., "istio"
    IngressController string // e.g., "nginx"
    CNI              string // e.g., "calico"
}
```

**Question**: How does HolmesGPT-API use these for filtering?
- [ ] Exact match on all non-empty fields
- [ ] Subset matching (workflow must match all provided labels)
- [ ] Superset matching (workflow covers at least the provided labels)
- [ ] Other (describe):

---

### Q3: OwnerChain Validation

Per DD-WORKFLOW-001 v1.7, HolmesGPT-API uses `OwnerChain` for **100% safe DetectedLabels validation** when RCA identifies a different resource than the original signal source.

**Example scenario**:
1. Signal source: Pod `nginx-abc123`
2. DetectedLabels were derived from this Pod's namespace/node
3. RCA identifies root cause: Deployment `nginx`
4. HolmesGPT-API validates Deployment is in OwnerChain before applying DetectedLabels

**Question**: Is this validation logic implemented? What happens if the root cause resource is NOT in OwnerChain?
- [ ] Reject DetectedLabels entirely
- [ ] Use fallback labels
- [ ] Proceed without label filtering
- [ ] Other (describe):

---

### Q4: Investigation Request Schema

AIAnalysis calls HolmesGPT-API to perform investigation. What's the expected request schema?

**Our assumed schema**:
```json
{
  "signalContext": {
    "signalId": "alert-12345",
    "signalType": "PrometheusAlert",
    "severity": "critical",
    "timestamp": "2025-12-01T10:00:00Z",
    "environment": "production",
    "businessPriority": "P0",
    "targetResource": {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "namespace": "default",
      "name": "nginx"
    },
    "enrichmentResults": {
      "kubernetesContext": { ... },
      "detectedLabels": { ... },
      "ownerChain": [ ... ],
      "customLabels": { ... }
    }
  }
}
```

**Question**: Is this schema correct? What fields are required vs optional?

---

### Q5: Investigation Response Schema

**Our assumed response schema**:
```json
{
  "investigationId": "inv-67890",
  "status": "completed",
  "rootCauseAnalysis": {
    "summary": "Deployment nginx has insufficient replicas",
    "confidence": 0.85,
    "affectedResource": {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "name": "nginx",
      "namespace": "default"
    },
    "evidenceChain": [ ... ]
  },
  "recommendations": [
    {
      "id": "rec-001",
      "action": "scale_deployment",
      "parameters": { "replicas": 3 },
      "confidence": 0.9,
      "workflowId": "wf-scale-deployment-001",
      "requiresApproval": true
    }
  ],
  "selectedWorkflow": {
    "id": "wf-scale-deployment-001",
    "name": "Scale Deployment",
    "parameters": { ... }
  }
}
```

**Question**: Is this schema correct? Specifically:
- What's the structure of `recommendations[].parameters`?
- How is `requiresApproval` determined?
- What's the structure of `selectedWorkflow.parameters`?

---

### Q6: Approval Flow Integration

Per DD-RECOVERY-002/003, AIAnalysis signals `approvalRequired=true` in its status when HolmesGPT-API returns recommendations requiring approval.

**Question**: How does HolmesGPT-API determine if approval is required?
- [ ] Workflow metadata (approval field in workflow definition)
- [ ] Rego policy evaluation (separate approval policy)
- [ ] Risk scoring threshold
- [ ] Other (describe):

---

### Q7: Retry and Timeout Behavior

AIAnalysis implements retry logic for HolmesGPT-API calls (BR-AI-050: Max 3 retries with exponential backoff).

**Questions**:
- What's the recommended timeout for investigation requests?
- Are there specific error codes that should NOT be retried (e.g., validation errors)?
- Is there a `/health` endpoint for circuit breaker readiness checks?

---

### Q8: CustomLabels in LLM Prompt

`CustomLabels` are passed to HolmesGPT-API for inclusion in the LLM prompt context:

```json
{
  "customLabels": {
    "constraint": ["cost-constrained", "risk-tolerance=low"],
    "business": ["category=payments"],
    "team": ["name=platform"]
  }
}
```

**Question**: How are these incorporated into the LLM prompt?
- [ ] Verbatim inclusion in system prompt
- [ ] Semantic expansion (e.g., "This is a cost-constrained payments system...")
- [ ] Structured context injection
- [ ] Other (describe):

---

---

## HolmesGPT-API Team Response

**Date**: December 2, 2025
**Respondent**: HolmesGPT-API Team
**Status**: ✅ **RESPONDED**

---

### A1: MCP Connection

⚠️ **CORRECTION**: HolmesGPT-API does **NOT** expose an MCP server.

The MCP workflow catalog approach was replaced with a **toolkit-based architecture** where HolmesGPT-API uses internal tools to query the Data Storage service directly.

**Connection Details**:
- **Endpoint**: `http://holmesgpt-api:8080/api/v1/incident/analyze` (initial)
- **Endpoint**: `http://holmesgpt-api:8080/api/v1/recovery/analyze` (recovery)
- **Protocol**: HTTP/REST (no MCP)
- **Authentication**: Service-to-service (K8s network policy, optional mTLS)

**Reference**: ADR-045-aianalysis-holmesgpt-api-contract.md

---

### A2: DetectedLabels Filtering

✅ **Subset matching** - Workflow must match all provided (non-empty) labels.

**Correct Schema** (per `pkg/shared/types/enrichment.go`):
```go
type DetectedLabels struct {
    GitOpsManaged    bool   `json:"gitOpsManaged"`
    GitOpsTool       string `json:"gitOpsTool,omitempty"`    // "argocd", "flux", ""
    PDBProtected     bool   `json:"pdbProtected"`
    HPAEnabled       bool   `json:"hpaEnabled"`
    Stateful         bool   `json:"stateful"`
    HelmManaged      bool   `json:"helmManaged"`
    NetworkIsolated  bool   `json:"networkIsolated"`
    PodSecurityLevel string `json:"podSecurityLevel,omitempty"`  // "privileged", "baseline", "restricted"
    ServiceMesh      string `json:"serviceMesh,omitempty"`       // "istio", "linkerd"
}
```

**Reference**: DD-WORKFLOW-001 v1.6 "DetectedLabels (V1.0 - Auto-Detected from K8s)"

---

### A3: OwnerChain Validation

✅ **Implemented** per DD-WORKFLOW-001 v1.7

**Behavior when RCA resource is NOT in OwnerChain**:
- [x] **Proceed with degraded filtering** (not reject)
- DetectedLabels ARE included in prompt (LLM context)
- Response includes: `"targetInOwnerChain": false`
- Response includes warning in `warnings[]` array

**Reference**: DD-WORKFLOW-001 v1.7 "DetectedLabels Validation Architecture"

---

### A4: Request Schema

⚠️ **Corrections needed** - Use flat structure, correct DetectedLabels schema.

**OpenAPI Reference**: `holmesgpt-api/api/openapi.json` (auto-generated)
- Export: `cd holmesgpt-api && python3 api/export_openapi.py`
- Source models: `src/models/incident_models.py`, `src/models/recovery_models.py`

**Corrected Incident Request**:
```json
{
  "incident_id": "alert-12345",
  "remediation_id": "req-2025-12-02-abc123",
  "signal_type": "OOMKilled",
  "severity": "critical",
  "signal_source": "prometheus",
  "resource_namespace": "default",
  "resource_kind": "Deployment",
  "resource_name": "nginx",
  "error_message": "Container exceeded memory limit",
  "environment": "production",
  "priority": "P0",
  "risk_tolerance": "low",
  "business_category": "payment-service",
  "cluster_name": "prod-us-west-2",
  "enrichment_results": {
    "detectedLabels": {
      "gitOpsManaged": true,
      "gitOpsTool": "argocd",
      "pdbProtected": true,
      "serviceMesh": "istio"
    },
    "ownerChain": [
      {"namespace": "default", "kind": "ReplicaSet", "name": "nginx-7d8f9c6b5"},
      {"namespace": "default", "kind": "Deployment", "name": "nginx"}
    ],
    "customLabels": {
      "constraint": ["cost-constrained"],
      "team": ["name=payments"]
    }
  }
}
```

**Key Corrections**:
1. ❌ Your schema had `signalContext` wrapper - we use **flat structure**
2. ❌ Your `detectedLabels` schema was wrong - use authoritative Go struct
3. ✅ `remediation_id` is **MANDATORY** (audit correlation)

---

### A5: Response Schema

❌ **Significant corrections needed.**

**Corrected Response Schema**:
```json
{
  "incident_id": "alert-12345",
  "analysis": "Natural language analysis from LLM...",
  "root_cause_analysis": {
    "summary": "Container exceeded memory limit due to memory leak",
    "severity": "critical",
    "contributing_factors": ["Memory leak", "Insufficient limits"]
  },
  "selected_workflow": {
    "workflow_id": "wf-memory-increase-001",
    "containerImage": "ghcr.io/kubernaut/workflows/memory-increase:v2.1.0",
    "containerDigest": "sha256:abc123def456...",
    "confidence": 0.90,
    "rationale": "Selected based on 90% semantic similarity for OOMKilled signal",
    "parameters": {
      "TARGET_NAMESPACE": "default",
      "TARGET_DEPLOYMENT": "nginx",
      "MEMORY_INCREASE_PERCENT": "50"
    }
  },
  "confidence": 0.90,
  "timestamp": "2025-12-02T10:30:00Z"
}
```

**Key Corrections**:

| Field | Your Schema | Correct | Reason |
|-------|-------------|---------|--------|
| `recommendations[]` | Present | ❌ **REMOVED** | We return single `selected_workflow` |
| `requiresApproval` | Present | ❌ **REMOVED** | AIAnalysis determines via Rego |
| `version` | Present | ❌ **REMOVED** | Use `containerImage` + `containerDigest` |
| `containerImage` | Missing | ✅ **REQUIRED** | Immutable workflow reference |

**Reference**: ADR-045, DD-HAPI-003 (no historical success rate)

---

### A6: Approval Determination

❌ **HolmesGPT-API does NOT determine approval.**

AIAnalysis determines `approvalRequired` via its own **Rego policies**, NOT based on HolmesGPT-API response.

**HolmesGPT-API provides**:
- `selectedWorkflow.confidence` - Used by AIAnalysis Rego
- `rootCauseAnalysis.severity` - May be used by AIAnalysis Rego

**AIAnalysis Rego Policy** (example):
```rego
decision = "AUTO_APPROVE" {
    input.workflow_confidence >= 0.8
    input.environment != "production"
}

decision = "MANUAL_APPROVAL_REQUIRED" {
    input.environment == "production"
}
```

---

### A7: Retry/Timeout

| Aspect | Value |
|--------|-------|
| **Recommended Timeout** | 30 seconds |
| **Max Retries** | 3 |
| **Backoff Strategy** | Exponential (1s, 2s, 4s) |

**Non-Retryable Errors** (4xx): 400 (validation), 404 (not found), 422 (unprocessable)

**Retryable Errors** (5xx): 500, 502 (LLM unavailable), 503, 504

**Health Endpoint**: `GET /health`
```json
{"status": "healthy", "llm_connected": true, "data_storage_connected": true, "version": "v3.2.0"}
```

**Error Format**: RFC 7807 (`application/problem+json`)

---

### A8: CustomLabels in Prompt

✅ **Auto-appended to workflow search ONLY** - NOT visible to LLM.

| Label Type | Visible to LLM | Used for Search |
|------------|----------------|-----------------|
| `detectedLabels` | ✅ YES | ✅ YES |
| `customLabels` | ❌ NO | ✅ YES |

**Why CustomLabels are NOT in LLM prompt** (per DD-HAPI-001):
1. LLM doesn't need them - Labels are for filtering, not analysis
2. Prevents LLM forgetting - If LLM sees labels, it might forget to include them
3. Smaller context - Reduces prompt size

**Reference**: DD-HAPI-001 (Custom Labels Auto-Append Architecture)

---

## Action Items for AIAnalysis Team

| Item | Priority | Status | Notes |
|------|----------|--------|-------|
| Update DetectedLabels schema to match authoritative Go struct | 🔴 High | ✅ **Done** | Authoritative schema in `pkg/shared/types/enrichment.go` |
| Remove `requiresApproval` expectation from HAPI response | 🔴 High | ✅ **Done** | AIAnalysis uses Rego policies |
| Remove `recommendations[]` expectation - use `selected_workflow` | 🔴 High | ✅ **Done** | CRD types already use `SelectedWorkflow` |
| Implement Rego policy for approval determination | 🟡 Medium | ✅ **In Specs** | See `implementation-checklist.md` Phase 3 |
| Generate Go client from OpenAPI spec | 🔴 High | ✅ **READY** | Use `ogen` with OpenAPI 3.1.0 |
| Add `TargetInOwnerChain` to `AIAnalysisStatus` | 🔴 High | ✅ **Added** (Dec 2) | For Rego policy input and audit |
| Add `Warnings` handling from HAPI response | 🟡 Medium | ✅ **Added** (Dec 2) | For metrics and notifications |

### OpenAPI Client Generation (Dec 2, 2025)

✅ **Response schemas now available** - HolmesGPT-API added `response_model` definitions:

| Endpoint | Response Schema |
|----------|-----------------|
| `/api/v1/incident/analyze` | `IncidentResponse` |
| `/api/v1/recovery/analyze` | `RecoveryResponse` |
| `/api/v1/postexec/analyze` | `PostExecResponse` |

**Generate Go client with `ogen`** (supports OpenAPI 3.1.0):
```bash
# Install ogen (one-time)
go install github.com/ogen-go/ogen/cmd/ogen@latest

# Generate client
mkdir -p pkg/clients/holmesgpt
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json
```

**OpenAPI Spec**: `holmesgpt-api/api/openapi.json` (16 schemas, OpenAPI 3.1.0)

---

## Questions from HolmesGPT-API Team (Dec 2, 2025)

**Status**: ✅ **RESPONDED** (Dec 2, 2025)

### Q9: `targetInOwnerChain` Field - Does AIAnalysis Need This?

**Context**: A3 (line 261) states that the response includes `"targetInOwnerChain": false` for OwnerChain validation feedback.

**Problem**: This field does **NOT exist** in the current `IncidentResponse` model:

```python
# Actual IncidentResponse (holmesgpt-api/src/models/incident_models.py)
class IncidentResponse(BaseModel):
    incident_id: str
    analysis: str
    root_cause_analysis: Dict[str, Any]
    selected_workflow: Optional[Dict[str, Any]]
    confidence: float
    timestamp: str
    # ❌ NO targetInOwnerChain field
    # ❌ NO warnings field
```

**Question**: Does AIAnalysis need these fields for OwnerChain validation feedback?
- [x] **Yes** - Add `targetInOwnerChain: bool` and `warnings: List[str]` to `IncidentResponse`
- [ ] **No** - Remove this claim from A3 documentation (it's internal HAPI behavior, not exposed)

#### **AIAnalysis Response (Dec 2, 2025)**:

✅ **YES - Please add `targetInOwnerChain` to both `IncidentResponse` and `RecoveryResponse`**

**Why AIAnalysis needs this field**:

| Use Case | How AIAnalysis Uses `targetInOwnerChain` |
|----------|------------------------------------------|
| **Rego Policy Input** | Can include in approval policy: `input.targetInOwnerChain == false` → require manual approval |
| **Audit Trail** | Store in `AIAnalysis.status` for transparency on label accuracy |
| **Operator Notification** | Surface warning in approval notification: "⚠️ Labels may not apply to target resource" |
| **Metrics** | Track `aianalysis_target_not_in_owner_chain_total` for reliability monitoring |

**Proposed Field Definition**:
```python
class IncidentResponse(BaseModel):
    # ... existing fields ...
    target_in_owner_chain: bool = Field(
        default=True,
        description="Whether RCA-identified target resource was found in OwnerChain. "
                    "If false, DetectedLabels may be from different scope than affected resource."
    )
    warnings: List[str] = Field(
        default_factory=list,
        description="Non-fatal warnings (e.g., OwnerChain validation issues)"
    )
```

**Note**: Field was renamed from `labelsValidated` → `targetInOwnerChain` per earlier discussion (see `RESPONSE_TO_AIANALYSIS_TEAM.md` v1.5).

---

### Q10: `warnings[]` Field Scope

**Context**: A3 mentions `warnings[]` array in response for OwnerChain validation warnings.

**Current State** (Updated Dec 2, 2025):
| Response Model | Has `warnings[]`? |
|----------------|-------------------|
| `IncidentResponse` | ✅ YES (added Dec 2) |
| `RecoveryResponse` | ✅ YES |

**Question**: Should `warnings[]` be added to `IncidentResponse` for consistency?
- [x] **Yes** - Add `warnings: List[str]` to `IncidentResponse`
- [ ] **No** - Warnings only needed for recovery scenarios

#### **AIAnalysis Response (Dec 2, 2025)**:

✅ **YES - Please add `warnings[]` to `IncidentResponse` for consistency**

**Rationale**:
1. **Consistency**: Both incident and recovery flows can have OwnerChain validation issues
2. **Extensibility**: Future warnings (catalog validation, confidence degradation) apply to both
3. **Transparency**: Operators see same warning surface regardless of flow type

**Proposed Warning Types (for both response models)**:
| Warning | When Triggered |
|---------|----------------|
| `"Target resource not found in OwnerChain - DetectedLabels may not apply"` | `targetInOwnerChain == false` |
| `"No workflows matched DetectedLabels constraints"` | Workflow search returned 0 results |
| `"Low confidence selection (< 0.7)"` | Best match has low confidence |

---

### Q11: OpenAPI Version Typo (Line 466)

**Minor**: Line 466 says "OpenAPI 3.0 spec" but the actual spec is **OpenAPI 3.1.0**.

**Action**: Will fix this typo once questions are answered.

#### **AIAnalysis Response (Dec 2, 2025)**:

✅ **Acknowledged** - Fixed in line 453 below.

---

## Summary of HAPI Team Action Items

| Item | Priority | Response | Status |
|------|----------|----------|--------|
| Add `target_in_owner_chain: bool` to `IncidentResponse` | 🔴 High | ✅ **YES - Please add** | ✅ **DONE** (Dec 2) |
| Add `warnings: List[str]` to `IncidentResponse` | 🔴 High | ✅ **YES - Please add** | ✅ **DONE** (Dec 2) |
| Fix OpenAPI version typo | 🟢 Low | ✅ Acknowledged | ✅ **DONE** |
| Regenerate OpenAPI spec after changes | 🔴 High | Required after above changes | ✅ **DONE** (16 schemas) |
| ~~Downgrade OpenAPI spec to 3.0.x~~ | ~~🔴 High~~ | Use `ogen` instead (native 3.1.0) | ✅ **NOT NEEDED** |

### Implementation Details (Dec 2, 2025)

**Changes made to `holmesgpt-api/src/models/incident_models.py`**:
```python
class IncidentResponse(BaseModel):
    # ... existing fields ...
    target_in_owner_chain: bool = Field(
        default=True,
        description="Whether RCA-identified target resource was found in OwnerChain. "
                    "If false, DetectedLabels may be from different scope than affected resource."
    )
    warnings: List[str] = Field(
        default_factory=list,
        description="Non-fatal warnings (e.g., OwnerChain validation issues, low confidence)"
    )
```

**OpenAPI spec regenerated**: `holmesgpt-api/api/openapi.json` (16 schemas, OpenAPI 3.1.0)

---

## References

- **ADR-045**: AIAnalysis ↔ HolmesGPT-API Service Contract (AUTHORITATIVE)
- **DD-WORKFLOW-001 v1.8**: Mandatory label schema + DetectedLabels
- **DD-RECOVERY-002**: Direct AIAnalysis recovery flow
- **DD-RECOVERY-003**: Recovery prompt design
- **DD-HAPI-001**: Custom Labels Auto-Append Architecture
- **DD-HAPI-003**: V1.0 Confidence Scoring
- `pkg/shared/types/enrichment.go` - **AUTHORITATIVE** DetectedLabels schema
- `holmesgpt-api/api/openapi.json` - OpenAPI 3.1.0 spec (native, use `ogen` for Go client)

