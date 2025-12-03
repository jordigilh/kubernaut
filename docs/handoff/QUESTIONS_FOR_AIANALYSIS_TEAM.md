# Questions from HolmesGPT-API Team

**From**: HolmesGPT-API Team
**To**: AIAnalysis Controller Team
**Date**: December 1, 2025
**Re**: Analysis Results Handoff and WorkflowExecution Creation

---

## Context

The HolmesGPT-API team has implemented workflow selection and parameter generation (v3.2). We need to understand how AIAnalysis consumes our responses and creates WorkflowExecution CRDs.

---

## Questions

### Q1: HolmesGPT-API Response Consumption

**HolmesGPT-API Response Format**:
```json
{
  "incident_id": "uuid",
  "analysis": {
    "root_cause": "...",
    "confidence": 0.85,
    "evidence": [...]
  },
  "recommended_workflow": {
    "workflow_id": "uuid",
    "workflow_title": "Pod OOM Recovery",
    "parameters": {
      "target_namespace": "production",
      "target_pod": "payment-service-abc123",
      "memory_increase_percent": "50"
    },
    "confidence": 0.90
  },
  "audit_trail": [...]
}
```

**Questions**:
1. Is this response format compatible with AIAnalysis expectations?
2. Does AIAnalysis need additional fields?
3. How is the `recommended_workflow` mapped to WorkflowExecution spec?

---

### Q2: WorkflowExecution Creation

**Question**: When AIAnalysis creates a WorkflowExecution CRD:
1. How are `recommended_workflow.parameters` mapped to `WorkflowExecution.spec.parameters`?
2. Is there validation before CRD creation?
3. What status does the WorkflowExecution start with?

---

### Q3: Multi-Workflow Recommendations

**Future Feature**: HolmesGPT-API may recommend multiple workflows:
```json
{
  "recommended_workflows": [
    { "workflow_id": "...", "priority": 1, "parameters": {...} },
    { "workflow_id": "...", "priority": 2, "parameters": {...} }
  ]
}
```

**Questions**:
1. Is this planned for AIAnalysis?
2. Would multiple WorkflowExecution CRDs be created?
3. How would execution order be managed?

---

### Q4: Confidence Thresholds

**Question**: Does AIAnalysis have confidence thresholds?
- Below X% → No workflow recommended
- X-Y% → Workflow recommended with human approval
- Above Y% → Auto-execute workflow

**HolmesGPT-API Impact**: Should we include confidence levels in our response?

---

### Q5: Audit Trail Requirements

**HolmesGPT-API Provides**:
```json
{
  "audit_trail": [
    {
      "timestamp": "...",
      "action": "workflow_catalog_search",
      "query": "OOMKilled critical",
      "results_count": 3
    },
    {
      "timestamp": "...",
      "action": "workflow_selected",
      "workflow_id": "...",
      "reason": "Highest confidence match for OOMKilled signal"
    }
  ]
}
```

**Question**: Is this audit trail format useful? Should it be stored in AIAnalysis CRD status?

---

## Confirmed Working ✅

| Integration | Status | Notes |
|-------------|--------|-------|
| Parameter passthrough | ✅ Works | Workflow schema defines format |
| Confidence thresholds | ✅ Works | AIAnalysis Rego determines approval |
| Audit trail | ✅ Works | HAPI maintains internally (not in response) |
| Single workflow (V1.0) | ✅ Confirmed | Multi-workflow deferred to V1.1 |

---

## Action Items

| Item | Owner | Status | Notes |
|------|-------|--------|-------|
| Confirm response format | AA Team | ✅ **Done** (Dec 1) | See A1 - format clarified with corrections |
| Clarify WE creation mapping | AA Team | ✅ **Done** (Dec 1) | See A2 - RO creates WE, not AIAnalysis |
| Define confidence thresholds | AA Team | ✅ **Done** (Dec 1) | See A4 - Rego policies determine approval |
| ~~Add `version` to response~~ | HAPI | ❌ **Not Needed** | Per ADR-045: Use `containerImage` instead |
| ~~Add `auditTrail` to response~~ | HAPI | ❌ **Not Exposed** | HAPI maintains internally |

---

## Response

**Date**: December 1, 2025
**Responder**: AIAnalysis Team

---

### A1: HolmesGPT-API Response Consumption

**1. Is this response format compatible with AIAnalysis expectations?**

⚠️ **Partially compatible - changes needed**. Your format is missing critical fields.

**Your format**:
```json
{
  "recommended_workflow": {
    "workflow_id": "uuid",
    "workflow_title": "Pod OOM Recovery",
    "parameters": {...},
    "confidence": 0.90
  }
}
```

**AIAnalysis expects** (per `SelectedWorkflow` in `aianalysis_types.go`):
```json
{
  "recommended_workflow": {
    "workflow_id": "uuid",
    "version": "v2.1.0",                    // ❌ MISSING - REQUIRED
    "container_image": "ghcr.io/...:v2.1.0", // ❌ MISSING - CRITICAL
    "container_digest": "sha256:abc...",     // Optional but recommended
    "parameters": {...},
    "confidence": 0.90,
    "rationale": "Selected because..."       // ❌ MISSING - REQUIRED
  }
}
```

**Critical missing fields**:
| Field | Status | Why Needed |
|-------|--------|------------|
| `version` | ❌ REQUIRED | RO uses for audit trail, WorkflowExecution CRD |
| `container_image` | ❌ **CRITICAL** | Without this, WorkflowExecution cannot run |
| `rationale` | ❌ REQUIRED | Displayed to operators in approval notifications |

**2. Does AIAnalysis need additional fields?**

**Root Cause Analysis mapping** (your `analysis` → our `RootCauseAnalysis`):
```json
// Your format
"analysis": {
  "root_cause": "...",        // → maps to summary
  "confidence": 0.85,         // → NOT mapped (different from workflow confidence)
  "evidence": [...]           // → maps to contributingFactors
}

// Add these fields:
"analysis": {
  "root_cause": "...",
  "confidence": 0.85,
  "evidence": [...],
  "severity": "critical",     // ❌ ADD - Enum: critical|high|medium|low
  "signal_type": "OOMKilled"  // ❌ ADD - May differ from input after RCA
}
```

**3. How is `recommended_workflow` mapped to WorkflowExecution spec?**

AIAnalysis does **NOT** create WorkflowExecution - that's RO's responsibility.

**Flow**:
```
HolmesGPT-API → AIAnalysis.status.selectedWorkflow → RO reads → RO creates WorkflowExecution
```

AIAnalysis maps your response to `status.selectedWorkflow`:
```go
// In AIAnalysis controller
status.SelectedWorkflow = &SelectedWorkflow{
    WorkflowID:      response.RecommendedWorkflow.WorkflowID,
    Version:         response.RecommendedWorkflow.Version,         // NEED THIS
    ContainerImage:  response.RecommendedWorkflow.ContainerImage,  // CRITICAL
    ContainerDigest: response.RecommendedWorkflow.ContainerDigest,
    Confidence:      response.RecommendedWorkflow.Confidence,
    Parameters:      response.RecommendedWorkflow.Parameters,
    Rationale:       response.RecommendedWorkflow.Rationale,       // NEED THIS
}
```

---

### A2: WorkflowExecution Creation

**1. How are parameters mapped?**

Direct passthrough:
```go
// AIAnalysis populates status
status.SelectedWorkflow.Parameters = map[string]string{
    "TARGET_NAMESPACE": "production",     // UPPER_SNAKE_CASE per DD-WORKFLOW-003
    "TARGET_POD": "payment-service-abc123",
    "MEMORY_INCREASE_PERCENT": "50",
}

// RO creates WorkflowExecution using these parameters verbatim
```

**Note**: Parameter keys should be `UPPER_SNAKE_CASE` per DD-WORKFLOW-003.

**2. Is there validation before CRD creation?**

Yes, AIAnalysis validates:
- `workflow_id` is non-empty
- `container_image` is valid OCI reference
- `confidence` is 0.0-1.0
- `parameters` keys match expected format

**3. What status does WorkflowExecution start with?**

RO creates WorkflowExecution with `phase: Pending`. This is RO's responsibility, not AIAnalysis.

---

### A3: Multi-Workflow Recommendations

**1. Is this planned for AIAnalysis?**

❌ **Not in V1.0**. Single workflow per AIAnalysis.

**2. Would multiple WorkflowExecution CRDs be created?**

Future consideration. If implemented:
- Option A: Single AIAnalysis → Multiple WorkflowExecutions (RO manages)
- Option B: Multiple AIAnalysis CRDs (one per workflow)

**3. How would execution order be managed?**

Likely via `priority` field in response. RO would sequence WorkflowExecutions.

**Recommendation**: Table this for V1.1. V1.0 scope is single workflow.

---

### A4: Confidence Thresholds

**Yes, AIAnalysis has confidence thresholds:**

| Threshold | Behavior |
|-----------|----------|
| `confidence >= 0.8` | `approvalRequired = false` (auto-execute in non-prod) |
| `confidence < 0.8` | `approvalRequired = true` (requires approval) |
| `environment == "production"` | **Always** `approvalRequired = true` (Rego policy) |

**Rego policy can override** (see `ai-approval-policies` ConfigMap):
```rego
decision = "AUTO_APPROVE" {
    input.confidence >= 0.8
    input.environment != "production"
}

decision = "MANUAL_APPROVAL_REQUIRED" {
    input.confidence < 0.8
}

decision = "MANUAL_APPROVAL_REQUIRED" {
    input.environment == "production"
}
```

**HolmesGPT-API Impact**:
✅ Yes, please include confidence levels. We use:
- `recommended_workflow.confidence` for workflow selection confidence
- `analysis.confidence` (if added) for RCA confidence

---

### A5: Audit Trail Requirements

**Is this audit trail format useful?**

✅ **Yes, very useful for operator transparency.**

**Should it be stored in AIAnalysis CRD status?**

✅ **Recommended**. We should add an `auditTrail` field to status:

```go
// Proposed addition to AIAnalysisStatus
type AIAnalysisStatus struct {
    // ... existing fields ...

    // Audit trail from HolmesGPT-API investigation
    AuditTrail []AuditEntry `json:"auditTrail,omitempty"`
}

type AuditEntry struct {
    Timestamp string `json:"timestamp"`
    Action    string `json:"action"`
    Details   string `json:"details,omitempty"`
}
```

**ACTION ITEM**: AIAnalysis team will add `auditTrail` field to CRD status.

---

## Updated Action Items (Dec 2, 2025)

> ⚠️ **SUPERSEDED**: This section's original action items are outdated. See **ADR-045** and **AIANALYSIS_TO_HOLMESGPT_API_TEAM.md** for authoritative contract.

| Item | Owner | Status | Notes |
|------|-------|--------|-------|
| ~~Add `version` to response~~ | HAPI | ❌ **NOT NEEDED** | Per ADR-045: Use `containerImage` + `containerDigest` instead |
| ~~Add `container_image`, `rationale`~~ | HAPI | ✅ **Already included** | In `selected_workflow` per ADR-045 |
| ~~Add `severity`, `signal_type` to analysis~~ | HAPI | ✅ **Already included** | In `root_cause_analysis` |
| ~~Use `UPPER_SNAKE_CASE` for parameters~~ | HAPI | ✅ **N/A** | Workflow schema defines casing, no hardcoded format |
| ~~Add `auditTrail` to response~~ | HAPI | ❌ **NOT EXPOSED** | HAPI maintains internally, not in API response |
| Document confidence threshold behavior | AA Team | ✅ Done | AIAnalysis Rego policies determine approval |

---

## Confirmed Working ✅

| Integration | Status |
|-------------|--------|
| Parameter passthrough | ✅ Works (workflow schema defines format) |
| Confidence thresholds | ✅ AIAnalysis Rego determines approval |
| Audit trail | ✅ HAPI maintains internally (not in response) |
| Single workflow (V1.0) | ✅ Confirmed |

---

## Authoritative Response Format

> ⚠️ **SUPERSEDED**: The format below was the original assumption. See **ADR-045-aianalysis-holmesgpt-api-contract.md** for the authoritative schema.

**Authoritative Reference**: `ADR-045`, `holmesgpt-api/api/openapi.json`

**Key Corrections** (Dec 2, 2025):
- ❌ `version` NOT included (use `containerImage` for immutable reference)
- ❌ `audit_trail` NOT in response (HAPI internal only)
- ❌ `rationale` should NOT reference "historical success rate" (V1.0 uses semantic similarity only per DD-HAPI-003)
- ✅ `containerImage`, `containerDigest`, `confidence`, `rationale` ARE included
- ✅ `root_cause_analysis` includes `summary`, `severity`, `contributing_factors`

---

