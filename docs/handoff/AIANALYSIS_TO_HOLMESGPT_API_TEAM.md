# Questions from AIAnalysis Team

**From**: AIAnalysis Service Team
**To**: HolmesGPT-API Team
**Date**: December 1, 2025
**Context**: V1.0 integration requirements and MCP workflow coordination

---

## 🔔 NEW: LLM Self-Correction + Validation History (Dec 6, 2025)

**Status**: ✅ **IMPLEMENTATION COMPLETE** - Ready for AIAnalysis Integration

**API Contract Changes**:
1. `needs_human_review: bool` - indicates when human review is required
2. `human_review_reason: HumanReviewReason` - structured enum for reliable mapping
3. **NEW**: `validation_attempts_history: ValidationAttempt[]` - complete history of all validation attempts

**OpenAPI Spec**: **19 schemas** (regenerated Dec 6, 2025)

**LLM Self-Correction Loop** (DD-HAPI-002 v1.2):
- HAPI now retries up to 3 times with validation error feedback to LLM
- Each attempt is tracked in `validation_attempts_history`
- All LLM I/O is audited (BR-AUDIT-005)
- If all 3 attempts fail → `needs_human_review: true`

**When `needs_human_review` is `true`**:
- Workflow validation failed after 3 retry attempts
- LLM response parsing failed
- No suitable workflow found
- Low confidence selection (<70%)

**`human_review_reason` Enum Values**:
| Value | AIAnalysis SubReason |
|-------|---------------------|
| `workflow_not_found` | `WorkflowNotFound` |
| `image_mismatch` | `ImageMismatch` |
| `parameter_validation_failed` | `ParameterValidationFailed` |
| `no_matching_workflows` | `NoMatchingWorkflows` |
| `low_confidence` | `LowConfidence` |
| `llm_parsing_error` | `LLMParsingError` |

**NEW: `validation_attempts_history` Field**:

This field provides complete history of all validation attempts for:
- Operator notifications (natural language errors)
- Audit trail (compliance)
- Debugging (LLM behavior analysis)

```json
{
  "validation_attempts_history": [
    {
      "attempt": 1,
      "workflow_id": "bad-workflow-1",
      "is_valid": false,
      "errors": ["Workflow 'bad-workflow-1' not found in catalog"],
      "timestamp": "2025-12-06T10:00:00Z"
    },
    {
      "attempt": 2,
      "workflow_id": "restart-pod",
      "is_valid": false,
      "errors": ["Container image mismatch: expected 'ghcr.io/x:v1', got 'docker.io/y:v2'"],
      "timestamp": "2025-12-06T10:00:05Z"
    },
    {
      "attempt": 3,
      "workflow_id": "restart-pod",
      "is_valid": false,
      "errors": ["Missing required parameter: 'namespace'"],
      "timestamp": "2025-12-06T10:00:10Z"
    }
  ]
}
```

**AIAnalysis Action Required**:
```go
// Check this field BEFORE creating WorkflowExecution
if hapiResponse.NeedsHumanReview {
    status.Phase = "Failed"
    status.Reason = "WorkflowResolutionFailed"
    status.SubReason = mapToSubReason(hapiResponse.HumanReviewReason)  // Use enum directly

    // Use validation_attempts_history for detailed operator notification
    var attemptDetails []string
    for _, attempt := range hapiResponse.ValidationAttemptsHistory {
        attemptDetails = append(attemptDetails,
            fmt.Sprintf("Attempt %d: %s", attempt.Attempt, strings.Join(attempt.Errors, "; ")))
    }
    status.Message = strings.Join(attemptDetails, " | ")

    // Do NOT create WorkflowExecution - requires human intervention
    return
}
```

**Response Example (after 3 failed attempts)**:
```json
{
  "needs_human_review": true,
  "human_review_reason": "parameter_validation_failed",
  "warnings": [
    "Workflow validation failed after 3 attempts. Attempt 1: Workflow 'bad-1' not found | Attempt 2: Image mismatch | Attempt 3: Missing parameter"
  ],
  "validation_attempts_history": [
    {"attempt": 1, "workflow_id": "bad-1", "is_valid": false, "errors": ["Workflow 'bad-1' not found in catalog"], "timestamp": "..."},
    {"attempt": 2, "workflow_id": "restart-pod", "is_valid": false, "errors": ["Container image mismatch"], "timestamp": "..."},
    {"attempt": 3, "workflow_id": "restart-pod", "is_valid": false, "errors": ["Missing required parameter: 'namespace'"], "timestamp": "..."}
  ]
}
```

**Next Step**: Regenerate Go client with `ogen` to get new types:
```bash
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json

# New types available:
# - HumanReviewReason (enum)
# - ValidationAttempt (struct)
```

---

## 🔔 HolmesGPT-API Team Acknowledgment (Dec 5-6, 2025)

**Status**: ✅ **ALL CHANGES IMPLEMENTED**

The HolmesGPT-API team acknowledges receipt of all requests and confirms the following implementations:

| Request | Status | Notes |
|---------|--------|-------|
| `failedDetections` field in DetectedLabels | ✅ **DONE** | With Pydantic validation |
| OpenAPI 3.1.0 native support | ✅ **DONE** | Use `ogen` for Go client |
| `target_in_owner_chain` field | ✅ **DONE** | In IncidentResponse |
| `warnings[]` field | ✅ **DONE** | In IncidentResponse |
| `alternative_workflows[]` field | ✅ **DONE** (v1.2) | For audit/context ONLY - NOT for execution |
| `needs_human_review` field | ✅ **DONE** (v1.3) | Top-level flag for AI reliability issues |
| `human_review_reason` enum | ✅ **DONE** (v1.3) | Structured reason for reliable AIAnalysis mapping |
| `validation_attempts_history[]` field | ✅ **DONE** (v1.4) | Complete history of all validation attempts |
| LLM Self-Correction Loop | ✅ **DONE** (v1.4) | Max 3 retries with error feedback to LLM |
| Full LLM I/O Audit | ✅ **DONE** (v1.4) | All LLM interactions audited per BR-AUDIT-005 |
| Workflow Response Validation | ✅ **DONE** | DD-HAPI-002 v1.2 - validates workflow_id, container_image, parameters |
| OpenAPI spec regenerated | ✅ **DONE** | **19 schemas**, OpenAPI 3.1.0 |

**AIAnalysis Next Step**: See "How to Proceed After These Changes" section below for detailed integration guide.

**Files Changed (v1.4)**:
- `holmesgpt-api/src/models/incident_models.py` - Added `ValidationAttempt` model, `validation_attempts_history` field
- `holmesgpt-api/src/extensions/incident.py` - LLM self-correction loop, full audit integration
- `holmesgpt-api/src/audit/events.py` - Added `create_validation_attempt_event`
- `holmesgpt-api/tests/unit/test_llm_self_correction.py` - 23 new tests
- `holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py` - E2E tests for audit pipeline
- `holmesgpt-api/api/openapi.json` - Regenerated (**19 schemas**, OpenAPI 3.1.0)

**Quick Start**:
```bash
# Regenerate Go client (19 schemas including ValidationAttempt)
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json

# New types:
# - ValidationAttempt (struct)
# - HumanReviewReason (enum)
```

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

---

## 🆕 New Questions (Dec 5, 2025)

### Q12: Alternative Workflow Recommendations

**From**: AIAnalysis Team
**Date**: December 5, 2025
**Status**: ✅ **RESPONDED** (Dec 5, 2025)

**Context**: During implementation planning, we discovered a potential gap between our implementation plan and the current OpenAPI spec.

**Current API Behavior** (per `openapi.json`):
- `IncidentResponse.selected_workflow` returns **ONE** workflow (singular)
- No alternative or backup workflow recommendations

**Our Implementation Plan (outdated?)** assumed:
```go
// Plan's Day 4 assumed multiple recommendations:
type RecommendingHandlerResponse struct {
    RecommendedWorkflows  []WorkflowRecommendation  // ❌ NOT in OpenAPI
    AlternativeWorkflows  []WorkflowRecommendation  // ❌ NOT in OpenAPI
}
```

**Question**: Can HolmesGPT-API provide alternative workflow recommendations?

**Options**:

| Option | Description | Impact |
|--------|-------------|--------|
| **A** | Single `selected_workflow` (current behavior) | AIAnalysis stores single workflow; if it fails, recovery creates new AIAnalysis |
| **B** | Add `alternative_workflows: List[SelectedWorkflow]` to `IncidentResponse` | AIAnalysis can store backup options; RO can try alternatives without new AI call |
| **C** | Add `alternative_workflow: Optional[SelectedWorkflow]` (singular) | Simpler: just one backup option |

**Use Case for Alternatives**:
1. Operator sees "Primary: increase-memory (90% confidence), Alternative: restart-pod (75% confidence)"
2. If primary fails, RO could try alternative before creating new AIAnalysis
3. Reduces LLM calls for common failure-recovery scenarios

**AIAnalysis Preference**: **Option B or C** would be beneficial, but we can work with **Option A** if adding alternatives is complex.

**CRD Impact**:
- Current CRD has `status.selectedWorkflow` (singular)
- Would need to add `status.alternativeWorkflow` or `status.alternativeWorkflows` if HAPI supports it

---

### Q13: Recovery Endpoint - Does It Return Workflow Selection?

**Status**: ✅ **RESPONDED** (Dec 5, 2025)

**Context**: The `/api/v1/recovery/analyze` endpoint returns `RecoveryResponse` with `strategies[]`, not `selected_workflow`.

**Current `RecoveryResponse`**:
```json
{
  "incident_id": "...",
  "can_recover": true,
  "strategies": [
    {
      "action_type": "scale_down_gradual",  // NOT a workflow_id
      "confidence": 0.9,
      "rationale": "...",
      "estimated_risk": "low"
    }
  ],
  "primary_recommendation": "scale_down_gradual"
}
```

**Question**: For recovery attempts (when `IsRecoveryAttempt=true`), does HolmesGPT-API:

| Option | Behavior |
|--------|----------|
| **A** | Use `/incident/analyze` with `previous_execution` context → Returns `selected_workflow` |
| **B** | Use `/recovery/analyze` → Returns `strategies[]` (not workflows) |
| **C** | `/recovery/analyze` should return `selected_workflow` too (OpenAPI update needed) |

**Why This Matters**:
- AIAnalysis needs `selected_workflow` (with `workflow_id`, `containerImage`, `parameters`) to populate CRD status
- `strategies[].action_type` is not the same as `workflow_id`

**Current Understanding**: Use `/incident/analyze` for BOTH initial AND recovery, with `previous_execution` context for recovery. Is this correct?

---

## HolmesGPT-API Team Response to Q12-Q13 (Dec 5, 2025)

**Status**: ✅ **RESPONDED**
**Respondent**: HolmesGPT-API Team

---

### A12: Alternative Workflow Recommendations

**Clarification**: ✅ **Alternatives are for CONTEXT, not EXECUTION**

Per authoritative documentation (`APPROVAL_REJECTION_BEHAVIOR_DETAILED.md`):

> **"Alternatives are for CONTEXT, Not EXECUTION"** 📖
>
> The `ApprovalContext.AlternativesConsidered` field:
> - ✅ **Purpose**: Help operator make an informed decision
> - ✅ **Content**: Pros/cons of alternative approaches
> - ❌ **NOT**: A fallback queue for automatic execution

**Current State**:

| Aspect | Status | Details |
|--------|--------|---------|
| **ADR-045 Schema** | Defines `alternativeWorkflows[]` | In response schema YAML (lines 236-241) |
| **Actual Implementation** | Single `selected_workflow` only | Not yet implemented |
| **Purpose** | **Audit/Informational** | NOT for automatic execution |

**Why Alternatives Matter (for Operators)**:
1. **Informed Decision**: Operator sees "Primary: increase-memory (90%), Alternative: restart-pod (75%)"
2. **Audit Trail**: Post-incident analysis shows what options were considered
3. **Transparency**: Operator understands the AI's reasoning and trade-offs
4. **Manual Override**: If operator rejects primary, they know what alternatives existed

**What Alternatives are NOT for**:
- ❌ Automatic fallback execution
- ❌ RO trying alternatives without new AIAnalysis
- ❌ Bypassing operator approval

**Decision**: ✅ **IMPLEMENTED Option B** (`alternative_workflows[]`) for V1.0

**Implementation Complete** (Dec 5, 2025):
- Added `AlternativeWorkflow` model to `incident_models.py`
- LLM prompt requests up to 2-3 alternatives with rationale
- Parser extracts alternatives from LLM JSON response
- OpenAPI spec regenerated (17 schemas)
- ADR-045 updated to v1.2 with informational purpose documentation

**Benefits Delivered**:
- Richer approval context for operators
- Better audit trail of AI decision-making
- No execution complexity (only `selected_workflow` is executed)
- Transparency into AI reasoning for post-incident analysis

---

### A13: Recovery Endpoint - Does It Return Workflow Selection?

**Answer**: ✅ **Option A is correct** - Use `/incident/analyze` with `previous_execution` context.

**Important Discovery**: There's a schema gap we should address:

| Aspect | Formal Model (`RecoveryResponse`) | Actual Return |
|--------|-----------------------------------|---------------|
| `selected_workflow` | ❌ NOT in Pydantic model | ✅ Returned in response dict (line 1235) |
| `strategies[].action_type` | Contains workflow_id | Populated from `selected_workflow.workflow_id` |

**Current Behavior (recovery.py lines 1200-1235)**:
```python
# LLM returns selected_workflow in JSON
selected_workflow = structured.get("selected_workflow")

# We convert workflow_id → action_type for strategies[]
strategies.append(RecoveryStrategy(
    action_type=selected_workflow.get("workflow_id", "unknown_workflow"),  # ← workflow_id becomes action_type
    confidence=float(confidence),
    rationale=selected_workflow.get("rationale", "..."),
    estimated_risk="medium",
    prerequisites=[]
))

# BUT we also return selected_workflow as extra field (not in Pydantic model!)
result = {
    ...
    "strategies": [...],
    "selected_workflow": selected_workflow,  # ← Extra field
}
```

**Recommended Approach for AIAnalysis**:

| Option | Endpoint | When | Gets `selected_workflow`? |
|--------|----------|------|---------------------------|
| **A (Recommended)** | `/incident/analyze` | Both initial AND recovery | ✅ Yes - top-level field |
| B | `/recovery/analyze` | Recovery only | ⚠️ In response but not in formal model |

**Why Option A is Better**:
1. `IncidentResponse` formally includes `selected_workflow` in Pydantic model
2. Consistent schema across initial and recovery flows
3. Recovery context provided via `is_recovery_attempt` + `previous_execution` in request

**Action Item for HAPI Team**:
We should either:
1. **Add `selected_workflow` to `RecoveryResponse` model** (schema alignment), OR
2. **Deprecate `/recovery/analyze`** in favor of unified `/incident/analyze` with recovery context

**Recommendation**: Option 2 - Use `/incident/analyze` for everything, simplifies AIAnalysis integration.

---

### Summary of A12-A13

| Question | Answer | AIAnalysis Action |
|----------|--------|-------------------|
| Q12: Alternative workflows | **For audit/context only** (not execution) | Store in CRD for operator visibility |
| Q13: Recovery endpoint | **Use `/incident/analyze`** for both flows | Set `is_recovery_attempt=true` + `previous_execution` for recovery |

**Key Principle** (per `APPROVAL_REJECTION_BEHAVIOR_DETAILED.md`):
> Alternatives are for **CONTEXT**, not **EXECUTION**. Only `selected_workflow` is executed. Alternatives help operators make informed approval decisions and provide audit trail.

**Schema Gap to Address**: HAPI team will add `alternative_workflows[]` to `IncidentResponse` for operator context and audit purposes.

---

---

## 🆕 New Questions (Dec 5, 2025) - Catalog Validation Responsibility

### Q14: Workflow Catalog Validation (BR-AI-023)

**Status**: ✅ **RESPONDED** (Dec 5, 2025)
**From**: AIAnalysis Team
**Date**: December 5, 2025

**Context**: Per BR-AI-023 (catalog validation / hallucination detection) and `reconciliation-phases.md` Phase 3:
> Verify `workflowId` exists in catalog (hallucination detection)

**Question**: Does HolmesGPT-API validate that the `selected_workflow.workflowId` exists in the workflow catalog before returning it in the `/api/v1/incident/analyze` response?

**Options**:
| Option | Description | AIAnalysis Action |
|--------|-------------|-------------------|
| **A** | **HAPI validates** - `workflowId` is guaranteed to exist in catalog | AIAnalysis only checks for `nil` (current implementation) |
| **B** | **AIAnalysis validates** - HAPI may return any `workflowId` | AIAnalysis must call Data Storage to verify workflow exists |
| **C** | **Both validate** - HAPI validates on selection, AIAnalysis double-checks | Defense-in-depth approach |

**Why This Matters**:
- If LLM hallucinates a `workflowId` that doesn't exist, WorkflowExecution will fail
- BR-AI-023 requires this validation in V1.0 scope
- Current AIAnalysis implementation only checks `if SelectedWorkflow == nil`

#### HolmesGPT-API Response (Dec 5, 2025):

✅ **Answer: Option A** (HAPI validates) - **CORRECTED after discussion**

**Key Principle from DD-HAPI-002 v1.1**:
> "If validation fails at HAPI → LLM can self-correct in same session (cheap, good UX)"
> "If validation fails at AIAnalysis → Late Stage, after LLM session - can't self-correct"

**Why HAPI (not AIAnalysis) Should Validate Workflow Existence**:

| Validation Location | LLM Context | Recovery Action | UX |
|---------------------|-------------|-----------------|-----|
| **HAPI** (in session) | ✅ Available | LLM picks different workflow | ✅ Good |
| **AIAnalysis** (post-session) | ❌ Lost | Restart entire RCA | ❌ Expensive |

**Documentation Inconsistency Identified**:
- `reconciliation-phases.md` (line 189) says AIAnalysis validates ← **OUTDATED**
- `DD-HAPI-002 v1.1` principle says HAPI should validate ← **AUTHORITATIVE**

**✅ HAPI Implementation Complete (V1.0)**:
- ~~Current: HAPI does **NOT** validate workflow existence before returning~~
- **IMPLEMENTED**: `_validate_workflow_exists()` in `src/validation/workflow_response_validator.py`
- If invalid: Return error to LLM, LLM self-corrects (picks different workflow)

**Proposed HAPI Enhancement**:
```python
# In incident.py - after parsing LLM response
if selected_workflow:
    workflow_id = selected_workflow.get("workflow_id")
    # Validate workflow exists in catalog (hallucination detection)
    exists = await data_storage_client.workflow_exists(workflow_id)
    if not exists:
        # Return error to LLM for self-correction
        return ToolResult(
            status="invalid",
            error=f"Workflow {workflow_id} not found in catalog. Please select a different workflow.",
            message="The selected workflow does not exist. Review search results and select an existing workflow."
        )
```

**AIAnalysis Responsibility (Defense-in-Depth)**:
- AIAnalysis can still validate as defense-in-depth (in case HAPI validation fails)
- But primary validation should be in HAPI where LLM can self-correct

**Action Items**:
| Owner | Action | Priority | Status |
|-------|--------|----------|--------|
| **HAPI Team** | Implement `validate_workflow_exists` check | 🔴 V1.0 | ✅ **DONE** (BR-AI-023) |
| **Docs Team** | Update `reconciliation-phases.md` to reflect HAPI validates | 🟡 V1.0 | ⏳ Pending |
| **AIAnalysis Team** | Keep defense-in-depth validation (optional) | 🟢 V1.1 | ⏳ Pending |

**✅ HAPI Implementation Confirmation (December 7, 2025)**:
- `_validate_workflow_exists()` implemented in `src/validation/workflow_response_validator.py`
- Calls Data Storage `GET /api/v1/workflows/{workflow_id}` to validate existence
- Returns error to LLM for self-correction if workflow not found (hallucination detection)

---

### Q15: OCI Container Image Format Validation

**Status**: ✅ **RESPONDED** (Dec 5, 2025)
**From**: AIAnalysis Team
**Date**: December 5, 2025

**Context**: Per `reconciliation-phases.md` Phase 3:
> Verify `containerImage` format is valid OCI reference

**Question**: Does HolmesGPT-API validate that `selected_workflow.containerImage` is a valid OCI reference (e.g., `registry.io/namespace/image:tag@sha256:...`) before returning it?

**Options**:
| Option | Description | AIAnalysis Action |
|--------|-------------|-------------------|
| **A** | **HAPI validates** - `containerImage` is always valid OCI format | AIAnalysis trusts the value |
| **B** | **AIAnalysis validates** - HAPI returns raw string | AIAnalysis must validate OCI format before storing in CRD |
| **C** | **Catalog enforces** - Workflow registration validates `containerImage` | Both services trust the catalog |

**Why This Matters**:
- Invalid OCI reference will cause WorkflowExecution to fail
- WorkflowExecution CRD may have its own validation, but early detection is preferred
- Need to know where validation boundary is

#### HolmesGPT-API Response (Dec 5, 2025):

✅ **Answer: Option C** (Catalog enforces) + **Option A** (HAPI validates as defense-in-depth)

**Applying DD-HAPI-002 Principle**:
> Validation should happen where LLM can self-correct

**However**, OCI format validation is different from workflow existence:
- `container_image` comes from **Data Storage catalog** (not LLM-generated)
- If catalog has invalid OCI format, it's a **data quality issue**, not LLM hallucination
- LLM cannot "self-correct" bad catalog data

**Validation Responsibility**:
| Layer | Validates OCI Format? | Why |
|-------|----------------------|-----|
| **Data Storage** (registration) | ✅ **PRIMARY** | Workflows shouldn't be registered with invalid images |
| **HolmesGPT-API** | ✅ **DEFENSE** | Catch corrupted data before returning to AIAnalysis |
| **AIAnalysis** | 🟡 Optional | Defense-in-depth (low priority) |

**HAPI Implementation (Defense-in-Depth)**:
```python
# In incident.py - validate OCI format before returning
import re

OCI_REFERENCE_PATTERN = re.compile(
    r'^(?P<registry>[a-z0-9.-]+(?::[0-9]+)?/)?'
    r'(?P<name>[a-z0-9._/-]+)'
    r'(?::(?P<tag>[a-zA-Z0-9._-]+))?'
    r'(?:@(?P<digest>sha256:[a-f0-9]{64}))?$'
)

def validate_oci_reference(container_image: str) -> bool:
    return bool(OCI_REFERENCE_PATTERN.match(container_image))

# If invalid, add warning (don't fail - it's catalog data issue)
if not validate_oci_reference(container_image):
    warnings.append(f"Invalid OCI reference format: {container_image}")
```

**Action Items**:
| Owner | Action | Priority |
|-------|--------|----------|
| **Data Storage Team** | Enforce OCI format on workflow registration | 🔴 V1.0 |
| **HAPI Team** | Add OCI format validation + warning | 🟡 V1.0 |
| **AIAnalysis Team** | Optional defense-in-depth | 🟢 V1.1 |

---

### Q16: Workflow Parameter Schema Validation

**Status**: ✅ **RESPONDED** (Dec 5, 2025)
**From**: AIAnalysis Team
**Date**: December 5, 2025

**Context**: Per `reconciliation-phases.md` Phase 3:
> Verify parameters conform to workflow schema

**Question**: Does HolmesGPT-API validate that `selected_workflow.parameters` conform to the workflow's expected parameter schema (types, required fields, value ranges)?

**Options**:
| Option | Description | AIAnalysis Action |
|--------|-------------|-------------------|
| **A** | **HAPI validates** - Parameters are schema-compliant | AIAnalysis trusts the parameters |
| **B** | **AIAnalysis validates** - HAPI returns unvalidated params | AIAnalysis must fetch workflow schema from catalog and validate |
| **C** | **LLM constrained** - LLM prompt includes schema, validation is implicit | Trust LLM + HAPI to generate valid params |

**Parameter Schema Example** (per DD-WORKFLOW-003):
```json
{
  "MEMORY_LIMIT": "2Gi",           // Must be valid K8s resource format
  "RESTART_DELAY_SECONDS": 30,     // Must be positive integer
  "TARGET_NAMESPACE": "production" // Must be non-empty string
}
```

**Why This Matters**:
- Invalid parameters will cause WorkflowExecution to fail or behave unexpectedly
- Schema validation may require fetching workflow definition from catalog
- Defense-in-depth vs. single source of truth

#### HolmesGPT-API Response (Dec 5, 2025):

✅ **Answer: Option A** (HAPI validates) - **GAP IDENTIFIED**

**Authoritative Source**: `DD-HAPI-002 v1.1` (Workflow Parameter Validation Architecture)

| Status | Behavior |
|--------|----------|
| **V1.0 Current** | ✅ `_validate_parameters()` **IMPLEMENTED** in `workflow_response_validator.py` |
| **V1.0 Required** | ✅ HAPI is **SOLE VALIDATOR** per DD-HAPI-002 v1.1 |

**DD-HAPI-002 v1.1 Architecture**:
```
HolmesGPT-API (SOLE VALIDATOR)
├── LLM performs RCA
├── LLM calls search_workflow_catalog → selects workflow
├── LLM suggests parameters based on RCA
├── LLM calls validate_workflow_parameters  ← NOT YET IMPLEMENTED
│   ├── Fetch schema from Data Storage
│   ├── Validate: required, types, enums, ranges
│   └── LLM self-corrects if invalid (up to 3 attempts)
└── Return validated workflow + parameters
```

**Why HAPI (not AIAnalysis) for Parameter Validation**:
1. **LLM Context Preservation**: If validation fails in LLM session, LLM can self-correct
2. **If validation fails at AIAnalysis**: Must restart entire RCA flow (expensive, poor UX)
3. **Workflow Immutability**: No schema drift between validation and execution

**✅ Current State (V1.0) - IMPLEMENTED**:
- ~~`validate_workflow_parameters` tool is **NOT IMPLEMENTED**~~
- **IMPLEMENTED**: `_validate_parameters()` in `src/validation/workflow_response_validator.py`
- LLM generates parameters based on prompt instructions + explicit schema validation
- **Schema validation** performs required/type/enum/range checks before returning

**AIAnalysis Options for V1.0**:
| Option | Approach | Risk |
|--------|----------|------|
| **A** | Trust LLM-generated parameters | WorkflowExecution may fail on invalid params |
| **B** | AIAnalysis validates (temporary) | Requires Data Storage call for schema |
| **C** | Defer to Tekton runtime validation | Late failure detection |

**Recommendation**: For V1.0, use **Option A** (trust LLM) with **Option C** (Tekton catches at runtime). Plan `validate_workflow_parameters` tool for V1.1.

---

### Q17: Data Storage API - Workflow Retrieval Endpoint

**Status**: ✅ **RESPONDED** (Dec 5, 2025)
**From**: HolmesGPT-API Team
**Date**: December 5, 2025

**Question**: For `validate_workflow_exists`, does Data Storage have a `GET /api/v1/workflows/{workflow_id}` endpoint, or do we need to use search?

**Context**: HAPI needs to:
1. Validate workflow exists before returning to AIAnalysis
2. Retrieve full workflow spec to validate parameters
3. Retrieve container image pullspec for OCI format validation

#### Data Storage Team Response (Dec 5, 2025):

✅ **Answer: YES** - `GET /api/v1/workflows/{workflow_id}` exists and returns the **complete workflow object**.

**Business Requirement**: BR-STORAGE-039 (Workflow Catalog Retrieval API) - Added to `docs/services/stateless/data-storage/BUSINESS_REQUIREMENTS.md` v1.3

**Endpoint Details** (from `docs/services/stateless/data-storage/openapi/v3.yaml`):

```yaml
/api/v1/workflows/{workflow_id}:
  get:
    summary: Get workflow by UUID
    operationId: getWorkflow
    parameters:
      - name: workflow_id
        in: path
        required: true
        schema:
          type: string
          format: uuid
        description: DD-WORKFLOW-002 v3.0 - UUID primary key
    responses:
      '200':
        description: Workflow found
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/RemediationWorkflow'
      '404':
        description: Workflow not found
      '500':
        description: Internal server error
```

**Response Contains Everything HAPI Needs**:

| Field | Path | Use Case |
|-------|------|----------|
| **Workflow existence** | HTTP 200 vs 404 | `validate_workflow_exists` |
| **Container image** | `spec.container_image` | OCI format validation (Q15) |
| **Parameters schema** | `spec.parameters[]` | Parameter validation (Q16) |
| **Parameter types** | `spec.parameters[].type` | Type checking |
| **Required params** | `spec.parameters[].required` | Required field validation |
| **Default values** | `spec.parameters[].default` | Default substitution |
| **Full spec** | `spec.*` | Complete workflow definition |

**Example Response**:
```json
{
  "workflow_id": "550e8400-e29b-41d4-a716-446655440000",
  "workflow_name": "oom-memory-increase",
  "version": "1.0.0",
  "spec": {
    "container_image": "registry.example.com/remediation/oom-handler:v1.0.0",
    "parameters": [
      {"name": "namespace", "type": "string", "required": true},
      {"name": "memory_limit", "type": "string", "required": true, "default": "2Gi"},
      {"name": "timeout_seconds", "type": "integer", "required": false, "default": 300}
    ],
    "steps": [...]
  },
  "detected_labels": {
    "signal_type": "OOMKilled",
    "severity": "critical"
  },
  "is_enabled": true,
  "is_latest_version": true
}
```

**HAPI Implementation Pattern**:
```python
# In validate_workflow_exists tool
async def validate_workflow_exists(workflow_id: str) -> WorkflowValidationResult:
    response = await datastorage_client.get(f"/api/v1/workflows/{workflow_id}")

    if response.status_code == 404:
        return WorkflowValidationResult(exists=False, error="Workflow not found")

    if response.status_code == 200:
        workflow = response.json()
        return WorkflowValidationResult(
            exists=True,
            workflow_spec=workflow["spec"],
            container_image=workflow["spec"]["container_image"],
            parameters_schema=workflow["spec"]["parameters"]
        )
```

**Key Points**:
- ✅ **No search needed** - Direct UUID lookup
- ✅ **Single request** - Returns complete workflow object
- ✅ **Supports all validation needs** - Existence, image, parameters

---

## Summary: Validation Responsibility Matrix (Q14-Q17) - CORRECTED

| Validation | HAPI | AIAnalysis | Data Storage | Tekton |
|------------|------|------------|--------------|--------|
| **Q14: Workflow exists** | ✅ **PRIMARY** (LLM self-correct) | 🟡 Defense-in-depth | ✅ (source) | - |
| **Q15: Container image consistency** | ✅ **PRIMARY** (match catalog) | 🟡 Optional | ✅ (provides image) | ✅ (runtime pull) |
| **Q16: Parameter schema** | ✅ **SOLE VALIDATOR** (DD-HAPI-002) | ❌ (not recommended) | ✅ (provides schema) | ✅ (K8s state) |
| **Q17: Workflow retrieval** | ✅ Calls endpoint | - | ✅ **PROVIDES** `GET /workflows/{id}` | - |

**Key Principle** (per DD-HAPI-002 v1.2):
> If validation fails at HAPI → LLM can self-correct in same session (cheap, good UX)
> If validation fails at AIAnalysis → Late stage, after LLM session - can't self-correct (expensive, poor UX)

**Implementation Plan Created** ✅:
📄 See: [`IMPLEMENTATION_PLAN_WORKFLOW_RESPONSE_VALIDATION.md`](../services/stateless/holmesgpt-api/implementation/IMPLEMENTATION_PLAN_WORKFLOW_RESPONSE_VALIDATION.md)

| Gap | Implementation Phase | Timeline | Tests |
|-----|---------------------|----------|-------|
| **Q14**: Workflow existence | Phase 2: WorkflowResponseValidator | Day 2-3 | 2 unit tests |
| **Q15**: Container image consistency | Phase 2: WorkflowResponseValidator | Day 2-3 | 3 unit tests |
| **Q16**: Parameter schema | Phase 2: WorkflowResponseValidator | Day 2-3 | 11 unit tests |
| **Self-correction loop** | Phase 4 | Day 5 | 3 E2E tests |
| **Total** | | **5 days** | **24 new tests** |

**Documentation Updated** ✅:
- ✅ DD-HAPI-002 updated to v1.2 with comprehensive validation design
- ✅ `reconciliation-phases.md` updated to reflect HAPI validates (not AIAnalysis)
- ✅ `BR_MAPPING.md` updated with validation responsibility matrix

---

## 🆕 New Questions (Dec 6, 2025) - Threshold & Retry Clarification

### Q18: Confidence Threshold Configuration

**Status**: ✅ **RESOLVED**
**From**: AIAnalysis Team
**Date**: December 6, 2025
**Resolved**: December 6, 2025

**Context**: We found inconsistency in documentation regarding the confidence threshold for `needs_human_review=true`:

| Document | Threshold |
|----------|-----------|
| **BR-HAPI-197** (authoritative BR) | **70%** ("confidence is below 70% threshold") |
| Some downstream docs | 60% |
| AIANALYSIS_TO_HOLMESGPT_API_TEAM.md | 70% |

**Questions**:

1. **What is the authoritative threshold?** Is it 70% (per BR-HAPI-197) or 60%?

2. **Is this threshold configurable?** Can it be set via environment variable or config file?

   ```yaml
   # Example config question
   confidence_thresholds:
     auto_execute: 0.80      # ≥80% → auto-execute
     approval_required: ???  # What's the low end?
     manual_review: ???      # Below what triggers needs_human_review?
   ```

3. **How does the threshold relate to ApprovalRequired vs NeedsHumanReview?**

   Our understanding:
   - ≥80% → Auto-execute (`needs_human_review=false`, AIAnalysis sets `ApprovalRequired=false`)
   - 60-79% → Approval required (`needs_human_review=false`, AIAnalysis sets `ApprovalRequired=true`)
   - <60% → Manual review (`needs_human_review=true`, `reason=low_confidence`)

   Is this correct? If so, shouldn't BR-HAPI-197 say "60%" not "70%"?

---

#### ✅ HAPI Team Response (December 6, 2025)

**Key Clarification**: The confidence threshold is **AIAnalysis's responsibility**, not HAPI's.

**HAPI's Role** (stateless, threshold-agnostic):
- Returns `confidence: 0.XX` in the response
- Does NOT enforce any threshold
- Does NOT set `needs_human_review` based on confidence alone
- Only sets `needs_human_review=true` for validation failures (workflow not found, parsing errors, etc.)

**AIAnalysis's Role** (owns threshold logic):
- Reads `confidence` from HAPI response
- Applies threshold rules to determine `ApprovalRequired` status
- Owns the business logic for confidence-based decisions

**V1.0 Recommendation**:
```yaml
# AIAnalysis ConfigMap (global setting)
confidence_thresholds:
  manual_review: 0.70  # Below 70% → operator review recommended
```

**V1.1 Enhancement** (new BR to be created):
Operator-tunable thresholds based on context:
```yaml
# Future: Operator-defined rules
confidence_rules:
  - match:
      severity: critical
      environment: production
      resource_kind: StatefulSet
    threshold: 0.90  # Higher bar for critical prod stateful workloads

  - match:
      severity: low
      environment: dev
    threshold: 0.60  # Lower bar for dev environments

  - default:
      threshold: 0.70
```

**Action Items**:
1. ✅ BR-HAPI-197 will be updated to remove "70%" mention (threshold is AIAnalysis's responsibility)
2. ✅ AIAnalysis should implement global 70% threshold for V1.0
3. 📋 V1.1 BR to be created for operator-tunable threshold rules

---

### Q19: In-Session LLM Retry Status When `needs_human_review=true`

**Status**: ✅ **RESOLVED**
**From**: AIAnalysis Team
**Date**: December 6, 2025
**Resolved**: December 6, 2025

**Context**: DD-HAPI-002 describes an in-session LLM self-correction loop (max 3 retries). The HAPI README v3.6 mentions "LLM self-correction loop (max 3 retries)".

**Question**: When HAPI returns `needs_human_review=true` with `human_review_reason=llm_parsing_error`, does this mean:

| Option | Description |
|--------|-------------|
| **A** | HAPI already attempted in-session LLM self-correction (up to 3 attempts) and **all failed** |
| **B** | HAPI did NOT attempt retry, and external retry might help |

**Why This Matters**:

RemediationOrchestrator (RO) is considering whether to retry `LLMParsingError` cases by creating a new AIAnalysis CRD. If HAPI already exhausted in-session retries (Option A), external retry will likely fail again.

**RO's Current Assumption** (based on DD-HAPI-002):
```
┌─────────────────────────────────────────────────────────────┐
│ HAPI receives LLM response                                  │
├─────────────────────────────────────────────────────────────┤
│ Parse fails → Return error to LLM → LLM self-corrects       │
│ Parse fails → Return error to LLM → LLM self-corrects       │
│ Parse fails → Return error to LLM → LLM self-corrects       │
│ Parse fails (3rd attempt) → needs_human_review=true         │
└─────────────────────────────────────────────────────────────┘
```

**Clarification Needed**: Does the above flow match HAPI's implementation? If yes, then RO should NOT retry `LLMParsingError` since HAPI already tried 3 times.

---

#### ✅ HAPI Team Response (December 6, 2025)

**Answer: Option A is correct.**

Per DD-HAPI-002 v1.2, HAPI's implementation:

1. **In-Session Self-Correction**: HAPI retries up to 3 times **within the same LLM conversation context**
2. **Error Feedback**: Each failed attempt feeds validation errors back to the LLM for self-correction
3. **Exhaustion**: `needs_human_review=true` is only set after **all 3 attempts fail**
4. **Audit Trail**: `validation_attempts_history` in the response shows all attempts and their errors

**Flow Confirmation**:
```
┌─────────────────────────────────────────────────────────────┐
│ HAPI receives LLM response                                  │
├─────────────────────────────────────────────────────────────┤
│ Attempt 1: Parse/validate fails → Feed errors to LLM        │
│ Attempt 2: Parse/validate fails → Feed errors to LLM        │
│ Attempt 3: Parse/validate fails → EXHAUSTED                 │
│                                                             │
│ Result: needs_human_review=true                             │
│         human_review_reason=llm_parsing_error               │
│         validation_attempts_history=[attempt1, 2, 3]        │
└─────────────────────────────────────────────────────────────┘
```

**Implication for RO**:
- ❌ **Do NOT retry** `llm_parsing_error` by creating a new AIAnalysis CRD
- HAPI already exhausted 3 in-session attempts with the same context
- External retry will likely fail again (same LLM, same prompt structure)
- Correct action: Surface to operator for manual intervention

**When External Retry MIGHT Help**:
- `workflow_not_found` - if workflow catalog was updated after failure
- `container_image_mismatch` - if catalog was corrected
- NOT for parsing/validation errors (LLM behavior issue)

---

## 🔔 AIAnalysis Team: How to Proceed After These Changes (Dec 5, 2025)

**Status**: ✅ **ALL HAPI CHANGES COMPLETE - READY FOR AIANALYSIS INTEGRATION**

### Step 1: Regenerate Go Client

```bash
# Install ogen if not already installed
go install github.com/ogen-go/ogen/cmd/ogen@latest

# Regenerate Go client from updated OpenAPI spec (17 schemas)
mkdir -p pkg/clients/holmesgpt
ogen -package holmesgpt -target pkg/clients/holmesgpt \
    holmesgpt-api/api/openapi.json

# Verify new types are generated
grep -l "AlternativeWorkflow" pkg/clients/holmesgpt/*.go
```

### Step 2: New Types Available

After regeneration, you'll have access to:

| Type | Purpose | Usage |
|------|---------|-------|
| `AlternativeWorkflow` | Alternative workflow recommendation | Audit trail, operator context |
| `IncidentResponse.AlternativeWorkflows` | List of alternatives | Store in CRD status for transparency |

### Step 3: Update AIAnalysis CRD Status (Optional)

Consider adding to `AIAnalysisStatus` for audit/operator visibility:

```go
// In api/aianalysis/v1alpha1/aianalysis_types.go
type AIAnalysisStatus struct {
    // ... existing fields ...

    // AlternativeWorkflows stores other workflows considered but not selected.
    // INFORMATIONAL ONLY - NOT for automatic execution.
    // Helps operators understand AI reasoning during approval.
    // +optional
    AlternativeWorkflows []AlternativeWorkflow `json:"alternativeWorkflows,omitempty"`
}

type AlternativeWorkflow struct {
    WorkflowID     string  `json:"workflowId"`
    ContainerImage string  `json:"containerImage,omitempty"`
    Confidence     float64 `json:"confidence"`
    Rationale      string  `json:"rationale"`
}
```

### Step 4: Use `/incident/analyze` for Both Flows

**Recommended**: Use single endpoint for both initial and recovery:

```go
// Initial investigation
resp, err := hapiClient.AnalyzeIncident(ctx, &IncidentRequest{
    IncidentID:        spec.SignalRef.Name,
    RemediationID:     req.Name,
    IsRecoveryAttempt: false,
    // ... other fields
})

// Recovery investigation (after workflow failure)
resp, err := hapiClient.AnalyzeIncident(ctx, &IncidentRequest{
    IncidentID:          spec.SignalRef.Name,
    RemediationID:       req.Name,
    IsRecoveryAttempt:   true,
    RecoveryAttemptNum:  attemptNumber,
    PreviousExecution:   &PreviousExecution{...},
    // ... other fields
})

// Both return IncidentResponse with:
// - SelectedWorkflow (for execution)
// - AlternativeWorkflows (for audit/context only)
// - TargetInOwnerChain (for Rego policy input)
// - Warnings (for operator notification)
```

### Step 5: Handle Alternatives in Recommending Handler

```go
func (h *RecommendingHandler) Handle(ctx context.Context, req *AIAnalysis) error {
    // Call HAPI
    resp, err := h.hapiClient.AnalyzeIncident(ctx, request)
    if err != nil {
        return err
    }

    // Store primary workflow for execution
    req.Status.SelectedWorkflow = resp.SelectedWorkflow

    // Store alternatives for audit/operator context (NOT for execution)
    req.Status.AlternativeWorkflows = resp.AlternativeWorkflows

    // Store validation flags for Rego policy
    req.Status.TargetInOwnerChain = resp.TargetInOwnerChain

    // Store warnings for operator notification
    req.Status.Warnings = resp.Warnings

    return nil
}
```

### Key Principle to Remember

> **Alternatives are for CONTEXT, not EXECUTION.**
>
> Only `SelectedWorkflow` is executed by RemediationOrchestrator.
> `AlternativeWorkflows` help operators make informed approval decisions and provide audit trail.

### Questions?

If you have questions about the new fields or integration patterns, add them to this document under "New Questions" and we'll respond.

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

