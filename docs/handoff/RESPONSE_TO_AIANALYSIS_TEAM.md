# Response to AIAnalysis Team Questions

**From**: HolmesGPT-API Team
**To**: AIAnalysis Service Team
**Date**: December 2, 2025
**Re**: V1.0 integration requirements and workflow coordination

---

## Summary

This document contains HolmesGPT-API's responses to the AIAnalysis team's questions from [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](./AIANALYSIS_TO_HOLMESGPT_API_TEAM.md).

**Authoritative References**:
- `pkg/shared/types/enrichment.go` - DetectedLabels schema (AUTHORITATIVE SOURCE)
- `DD-HAPI-003-v1-confidence-scoring.md` - V1.0 confidence scoring methodology
- `DD-004-RFC7807-ERROR-RESPONSES.md` - Error response format

---

## Responses

### Q1: MCP Server Port and Endpoint

> **Correction**: HolmesGPT-API does NOT expose an MCP server. The MCP workflow catalog access was replaced with a **toolkit-based approach** where HolmesGPT-API uses internal tools to query the Data Storage service directly.

**Architecture**:
```
AIAnalysis → HTTP POST → HolmesGPT-API → Internal Toolkit → Data Storage
```

**Connection Details**:
- **HolmesGPT-API Endpoint**: `http://holmesgpt-api:8080/api/v1/investigate`
- **Protocol**: HTTP/REST (no MCP)
- **Authentication**: Service-to-service (K8s network policy, optional mTLS)

---

### Q2: DetectedLabels → Workflow Filtering

**Answer**: **Subset matching** - Workflow must match all non-empty fields provided.

**DetectedLabels Schema** (per `pkg/shared/types/enrichment.go`):
```go
type DetectedLabels struct {
    // GitOps Management
    GitOpsManaged bool   `json:"gitOpsManaged"`
    GitOpsTool    string `json:"gitOpsTool,omitempty"` // argocd, flux, ""

    // Workload Protection
    PDBProtected bool `json:"pdbProtected"`
    HPAEnabled   bool `json:"hpaEnabled"`

    // Workload Characteristics
    Stateful    bool `json:"stateful"`
    HelmManaged bool `json:"helmManaged"`

    // Security Posture
    NetworkIsolated  bool   `json:"networkIsolated"`
    PodSecurityLevel string `json:"podSecurityLevel,omitempty"` // privileged, baseline, restricted, ""
    ServiceMesh      string `json:"serviceMesh,omitempty"`      // istio, linkerd, ""
}
```

**⚠️ CORRECTION to Q2 in AIANALYSIS_TO_HOLMESGPT_API_TEAM.md**: Your assumed schema has incorrect fields (`Environment`, `Region`, `Platform`, etc.). Use the authoritative schema above.

**Filtering Logic**:
```sql
-- Generated WHERE clause for workflow search
WHERE
  (workflow.requires_gitops = false OR detected.gitOpsManaged = true)
  AND (workflow.requires_pdb = false OR detected.pdbProtected = true)
  AND (workflow.service_mesh IS NULL OR detected.serviceMesh = workflow.service_mesh)
  -- etc.
```

---

### Q3: OwnerChain Validation

**Answer**: ✅ **Implemented** (per DD-WORKFLOW-001 v1.7)

**Behavior when root cause resource is NOT in OwnerChain**:
- **Proceed with degraded filtering** - DetectedLabels are used but flagged as potentially stale
- **Response includes warning**: `"targetInOwnerChain": false` in response
- **Rationale**: Better to have a potentially applicable workflow than none

**Validation Flow**:
```
1. RCA identifies affected_resource (e.g., Deployment "nginx")
2. Check if affected_resource is in OwnerChain
3. If YES: targetInOwnerChain = true
4. If NO: targetInOwnerChain = false, add warning to response
5. Proceed with workflow search regardless
```

---

### Q4: Investigation Request Schema

**Answer**: Your assumed schema is **mostly correct** with minor adjustments.

**Corrected Request Schema**:
```json
{
  "signalContext": {
    "signalId": "alert-12345",
    "signalType": "PrometheusAlert",
    "severity": "critical",
    "timestamp": "2025-12-01T10:00:00Z",
    "businessPriority": "P0",
    "targetResource": {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "namespace": "default",
      "name": "nginx"
    },
    "enrichmentResults": {
      "kubernetesContext": { ... },
      "detectedLabels": {
        "gitOpsManaged": true,
        "gitOpsTool": "argocd",
        "pdbProtected": true,
        "hpaEnabled": false,
        "stateful": false,
        "helmManaged": true,
        "networkIsolated": true,
        "podSecurityLevel": "restricted",
        "serviceMesh": "istio"
      },
      "ownerChain": [
        {"namespace": "default", "kind": "ReplicaSet", "name": "nginx-7d8f9c6b5"},
        {"namespace": "default", "kind": "Deployment", "name": "nginx"}
      ],
      "customLabels": {
        "constraint": ["cost-constrained"],
        "team": ["name=payments"]
      },
      "enrichmentQuality": 0.95
    }
  },
  "recoveryContext": {
    "isRecovery": false,
    "previousExecutionId": null,
    "naturalLanguageSummary": null
  }
}
```

**Required vs Optional Fields**:
| Field | Required | Notes |
|-------|----------|-------|
| `signalContext.signalId` | ✅ Required | |
| `signalContext.signalType` | ✅ Required | |
| `signalContext.targetResource` | ✅ Required | |
| `signalContext.severity` | Optional | Defaults to "medium" |
| `signalContext.businessPriority` | Optional | |
| `enrichmentResults.detectedLabels` | ✅ Required | Use empty struct if unknown |
| `enrichmentResults.ownerChain` | Optional | Empty = orphan resource |
| `enrichmentResults.customLabels` | Optional | |
| `recoveryContext` | Optional | Required for retry scenarios |

---

### Q5: Investigation Response Schema

**Answer**: Your assumed schema needs **significant corrections**.

**Corrected Response Schema**:
```json
{
  "investigationId": "inv-2025-12-01-abc123",
  "status": "completed",
  "rootCauseAnalysis": {
    "summary": "Deployment nginx has insufficient replicas due to resource exhaustion",
    "severity": "critical",
    "signalType": "OOMKilled",
    "confidence": 0.85,
    "affectedResource": {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "name": "nginx",
      "namespace": "default"
    },
    "evidenceChain": [
      "Pod nginx-abc123 OOMKilled 5 times in 10 minutes",
      "Memory usage at 98% of limit before kill",
      "No HPA configured for automatic scaling"
    ]
  },
  "selectedWorkflow": {
    "workflowId": "wf-memory-increase-001",
    "containerImage": "ghcr.io/kubernaut/workflows/memory-increase:v2.1.0",
    "containerDigest": "sha256:abc123def456...",
    "parameters": {
      "TARGET_NAMESPACE": "default",
      "TARGET_DEPLOYMENT": "nginx",
      "MEMORY_INCREASE_PERCENT": "50"
    },
    "confidence": 0.90,
    "rationale": "Selected based on 90% semantic similarity for OOMKilled signal with matching GitOps and service mesh labels"
  },
  "alternativeWorkflows": [
    {
      "workflowId": "wf-hpa-configure-001",
      "containerImage": "ghcr.io/kubernaut/workflows/hpa-configure:v1.0.0",
      "confidence": 0.75,
      "rationale": "Alternative: Configure HPA for automatic scaling"
    }
  ],
  "targetInOwnerChain": true,
  "warnings": []
}
```

**Key Corrections**:

1. **No `recommendations` array** - HolmesGPT-API returns `selectedWorkflow` (single) + `alternativeWorkflows` (list)

2. **No `requiresApproval` in response** - AIAnalysis determines approval via Rego policies (see Q6)

3. **No `version` field** - Per DD-CONTRACT-001 v1.5, `version` is human metadata only. Use `containerImage` + `containerDigest` for immutable reference.

4. **Parameters structure**: `map[string]string` with `UPPER_SNAKE_CASE` keys per DD-WORKFLOW-003

5. **Rationale format**: Confidence-based, no historical success rate reference (per DD-HAPI-003)

---

### Q6: Approval Flow Integration

**Answer**: **HolmesGPT-API does NOT determine approval**.

**Correction**: AIAnalysis determines `approvalRequired` via its own Rego policies, NOT based on HolmesGPT-API response.

**Flow**:
```
1. HolmesGPT-API returns selectedWorkflow with confidence
2. AIAnalysis evaluates its own Rego policies:
   - input.confidence >= 0.8 AND environment != "production" → AUTO_APPROVE
   - input.confidence < 0.8 → MANUAL_APPROVAL_REQUIRED
   - environment == "production" → ALWAYS MANUAL_APPROVAL_REQUIRED
3. AIAnalysis sets status.approvalRequired accordingly
4. RO reads AIAnalysis status and handles approval flow
```

**HolmesGPT-API provides**:
- `selectedWorkflow.confidence` - Used by AIAnalysis Rego policies
- `rootCauseAnalysis.severity` - May be used by AIAnalysis policies
- `targetInOwnerChain` - Warning flag if RCA target not found in OwnerChain (DetectedLabels may not apply)

**AIAnalysis Rego Policy Example** (in `ai-approval-policies` ConfigMap):
```rego
package kubernaut.aianalysis.approval

default decision = "MANUAL_APPROVAL_REQUIRED"

decision = "AUTO_APPROVE" {
    input.workflow_confidence >= 0.8
    input.environment != "production"
}

decision = "MANUAL_APPROVAL_REQUIRED" {
    input.environment == "production"
}
```

---

### Q7: Retry and Timeout Behavior

**Recommended Timeout**: `30 seconds` for investigation requests

**Non-Retryable Errors** (4xx status codes):
| Error Code | Type | Description |
|------------|------|-------------|
| `400` | `VALIDATION_ERROR` | Invalid request schema |
| `404` | `SIGNAL_NOT_FOUND` | Signal ID not found |
| `422` | `UNPROCESSABLE_ENTITY` | Valid schema but cannot process |

**Retryable Errors** (5xx status codes):
| Error Code | Type | Description |
|------------|------|-------------|
| `500` | `INTERNAL_ERROR` | Unexpected server error |
| `502` | `LLM_UNAVAILABLE` | LLM service unavailable |
| `503` | `SERVICE_UNAVAILABLE` | HolmesGPT-API overloaded |
| `504` | `GATEWAY_TIMEOUT` | LLM request timeout |

**Health Endpoint**: `GET /health` - Returns:
```json
{
  "status": "healthy",
  "llm_connected": true,
  "data_storage_connected": true,
  "version": "v3.2.0"
}
```

**Error Response Format** (per DD-004: RFC 7807):
```json
{
  "type": "https://kubernaut.io/errors/validation-error",
  "title": "Validation Error",
  "status": 400,
  "detail": "signalContext.targetResource is required",
  "instance": "/api/v1/investigate/inv-2025-12-02-abc123"
}
```
**Content-Type**: `application/problem+json`

---

### Q8: CustomLabels in LLM Prompt

**Answer**: **Auto-appended to semantic search query ONLY** - NOT visible to LLM.

**Flow**:
```
1. AIAnalysis sends customLabels in request
2. HolmesGPT-API extracts customLabels
3. HolmesGPT-API appends to search query:
   - label.constraint=cost-constrained AND label.team=payments
4. Search results filtered by these labels
5. LLM sees workflow descriptions, NOT the customLabels
```

**Why customLabels are NOT in LLM prompt**:
- **a) LLM doesn't need them** - Labels are for filtering, not analysis
- **b) Prevents LLM forgetting** - If LLM sees labels, it might forget to use them
- **c) Smaller context** - Reduces prompt size

**Contrast with DetectedLabels**:
```
// customLabels (invisible to LLM)
customLabels: {
  "team": ["payments"],
  "region": ["us-west-2"]
}
→ Auto-appended to search: label.team=payments AND label.region=us-west-2

// detectedLabels (visible to LLM) - per pkg/shared/types/enrichment.go
detectedLabels: {
  "gitOpsManaged": true,
  "gitOpsTool": "argocd",
  "pdbProtected": true,
  "serviceMesh": "istio"
}
→ Included in prompt: "This workload is GitOps-managed via ArgoCD, has PDB protection, and uses Istio service mesh..."
→ Also used for search filtering
```

---

## Summary of Integration Contract

### AIAnalysis → HolmesGPT-API

**Endpoint**: `POST http://holmesgpt-api:8080/api/v1/investigate`

**Request**: See Q4 schema above

**Response**: See Q5 schema above

**Error Handling**: RFC 7807 format (see Q7)

### Key Contract Points

| Aspect | HolmesGPT-API Responsibility | AIAnalysis Responsibility |
|--------|------------------------------|---------------------------|
| Workflow Selection | Select best workflow based on RCA | Pass enrichment data |
| Confidence Scoring | Return `selectedWorkflow.confidence` | Evaluate against Rego policies |
| Approval Decision | **NOT RESPONSIBLE** | Determine `approvalRequired` via Rego |
| Parameter Formatting | `UPPER_SNAKE_CASE` keys | Passthrough to RO/WE |
| Retry Logic | Return appropriate error codes | Implement retry with backoff |
| Audit Trail | Maintain internal audit log | Capture response for CRD status |

---

## OpenAPI Specification & Go Client Generation

### OpenAPI Spec (Auto-Generated)

HolmesGPT-API uses FastAPI with Pydantic models, which **auto-generates OpenAPI 3.0**.

**Spec Location**: `holmesgpt-api/api/openapi.json`

**Live Endpoints** (dev mode):
- `GET /openapi.json` - Raw OpenAPI 3.0 spec
- `GET /docs` - Swagger UI
- `GET /redoc` - ReDoc UI

**Export Command**:
```bash
cd holmesgpt-api
python3 api/export_openapi.py
```

### Go Client Generation for AIAnalysis

AIAnalysis can generate a type-safe Go client from the OpenAPI spec:

```bash
# Install oapi-codegen if not present
go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest

# Generate Go client
oapi-codegen -package holmesgpt -generate types,client \
    holmesgpt-api/api/openapi.json > pkg/clients/holmesgpt/client.go
```

**Generated Code Includes**:
- Request/response types matching Pydantic models
- HTTP client with proper error handling
- RFC 7807 error type parsing

**Recommendation**: Add OpenAPI spec export to CI and regenerate Go client on spec changes.

---

## Action Items

| Item | Owner | Status | Notes |
|------|-------|--------|-------|
| Correct DetectedLabels schema in AIAnalysis docs | AIAnalysis | ✅ **Done** (Dec 2) | Using `pkg/shared/types/enrichment.go` as authoritative source |
| Implement RFC 7807 error responses | HAPI | ✅ Implemented | — |
| Remove `requiresApproval` from expected response | AIAnalysis | ✅ **Already Correct** | AIAnalysis CRD has `approvalRequired` in **status** (output), not expected from HAPI |
| Add `targetInOwnerChain` handling | AIAnalysis | ✅ **Ready** (Dec 2) | HAPI implemented field; AIAnalysis can now consume it |
| Update Q2 schema correction | AIAnalysis | ✅ **Done** (Dec 2) | DetectedLabels uses authoritative Go schema |
| Generate Go client from OpenAPI spec | AIAnalysis | ✅ **Unblocked** (Dec 2) | OpenAPI spec complete with response schemas |
| Export OpenAPI spec to `api/openapi.json` | HAPI | ✅ **Done** (Dec 2) | Complete spec: 19 schemas (requests + responses) |
| **Rename `labelsValidated` → `targetInOwnerChain`** | HAPI | ✅ **DONE** (Dec 2) | See implementation details below |

---

### ✅ **COMPLETED: Field Rename and Addition by HolmesGPT-API Team**

**Date**: December 2, 2025
**Status**: ✅ **ALL CHANGES IMPLEMENTED**

**Changes Made**:
| Change | Status |
|--------|--------|
| Add `target_in_owner_chain: bool` to `IncidentResponse` | ✅ Done |
| Add `warnings: List[str]` to `IncidentResponse` | ✅ Done |
| Regenerate OpenAPI spec | ✅ Done (16 schemas) |

**Implementation** (in `holmesgpt-api/src/models/incident_models.py`):
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

**AIAnalysis Next Steps**:
1. Regenerate Go client: `oapi-codegen -package holmesgpt -generate types,client holmesgpt-api/api/openapi.json > pkg/clients/holmesgpt/client.go`
2. Add `TargetInOwnerChain` field to `AIAnalysisStatus`
3. Use in Rego policy for approval decisions

---

> **Note (Dec 2, 2025)**: See **DD-WORKFLOW-001 v2.0** for authoritative DetectedLabels end-to-end architecture.
>
> **Update (Dec 2, 2025)**: OpenAPI spec now includes response schemas (`IncidentResponse`, `RecoveryResponse`, `PostExecResponse`). AIAnalysis can generate complete Go client.

---

## References

- **ADR-045**: AIAnalysis ↔ HolmesGPT-API Service Contract (AUTHORITATIVE)
- `holmesgpt-api/api/openapi.json` - OpenAPI 3.0 spec (auto-generated from Pydantic)
- `pkg/shared/types/enrichment.go` - **AUTHORITATIVE** DetectedLabels schema (Go)
- `holmesgpt-api/src/models/incident_models.py` - DetectedLabels schema (Python)
- `DD-CONTRACT-001-aianalysis-workflowexecution-alignment.md` - Contract alignment
- `DD-HAPI-003-v1-confidence-scoring.md` - V1.0 confidence methodology
- `DD-RECOVERY-002`: Direct AIAnalysis recovery flow
- `DD-RECOVERY-003`: Recovery prompt design
- `DD-004-RFC7807-ERROR-RESPONSES.md` - Error response format

---

**Version**: v1.6
**Last Updated**: December 2, 2025
**Changelog**:
- v1.6: **HAPI completed changes**: `target_in_owner_chain` and `warnings[]` added to `IncidentResponse`; OpenAPI spec regenerated
- v1.5: **Field rename**: `labelsValidated` → `targetInOwnerChain` (more descriptive, generic); Added action item for HAPI team
- v1.4: Updated action items - marked resolved items (DetectedLabels, requiresApproval, Go client generation unblocked)
- v1.3: Added OpenAPI spec section with Go client generation instructions for AIAnalysis
- v1.2: Added ADR-045 reference, OpenAPI spec location, DD-RECOVERY references
- v1.1: Corrected DetectedLabels schema, removed approvalRequired from response, added RFC 7807 reference
- v1.0: Initial response

