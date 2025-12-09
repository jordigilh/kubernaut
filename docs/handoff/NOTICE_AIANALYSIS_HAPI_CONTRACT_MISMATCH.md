# NOTICE: AIAnalysis ↔ HAPI API Contract Mismatch

**From**: HAPI Team
**To**: AIAnalysis Team
**Date**: December 9, 2025
**Priority**: 🔴 **HIGH** - Affects V1.0 integration
**Status**: ✅ **TRIAGED - Ready for Day 11 Implementation**

---

## 📋 Summary

During triage of `NOTICE_AIANALYSIS_V1_COMPLIANCE_GAPS.md`, the HAPI team identified a significant contract mismatch between AIAnalysis's `HolmesGPTClient` implementation and the authoritative HAPI OpenAPI specification.

**Impact**:
- AIAnalysis is sending incomplete requests to HAPI
- Critical fields are missing (e.g., `remediation_id` which is MANDATORY)
- Recovery flow is using wrong endpoint

---

## 🔴 Critical Gaps

### Gap 1: IncidentRequest Schema Mismatch

**Authoritative Source**: `holmesgpt-api/api/openapi.json`

| Field | HAPI OpenAPI (Authoritative) | AIAnalysis Code | Status |
|-------|------------------------------|-----------------|--------|
| `incident_id` | ✅ REQUIRED | ❌ MISSING | 🔴 **CRITICAL** |
| `remediation_id` | ✅ REQUIRED (DD-WORKFLOW-002) | ❌ MISSING | 🔴 **CRITICAL** |
| `signal_type` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `severity` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `signal_source` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `resource_namespace` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `resource_kind` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `resource_name` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `error_message` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `environment` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `priority` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `risk_tolerance` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `business_category` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `cluster_name` | ✅ REQUIRED | ❌ MISSING | 🔴 |
| `description` | Optional | ❌ MISSING | 🟡 |
| `is_duplicate` | Optional | ❌ MISSING | 🟡 |
| `occurrence_count` | Optional | ❌ MISSING | 🟡 |
| `firing_time` | Optional | ❌ MISSING | 🟡 |
| `signal_labels` | Optional | ❌ MISSING | 🟡 |
| `enrichment_results` | Optional (has DetectedLabels) | ❌ MISSING | 🟡 |
| `context` | ❌ NOT IN SPEC | ✅ Present | ⚠️ **EXTRA** |

**AIAnalysis Current Code** (`pkg/aianalysis/client/holmesgpt.go`):
```go
type IncidentRequest struct {
    Context        string                 `json:"context"`           // NOT IN HAPI SPEC
    DetectedLabels map[string]interface{} `json:"detected_labels"`   // Should be in enrichment_results
    CustomLabels   map[string][]string    `json:"custom_labels"`     // Should be in enrichment_results
    OwnerChain     []OwnerChainEntry      `json:"owner_chain"`       // Should be in enrichment_results
}
```

**HAPI OpenAPI Required Fields**:
```json
"required": [
  "incident_id",
  "remediation_id",
  "signal_type",
  "severity",
  "signal_source",
  "resource_namespace",
  "resource_kind",
  "resource_name",
  "error_message",
  "environment",
  "priority",
  "risk_tolerance",
  "business_category",
  "cluster_name"
]
```

---

### Gap 2: Missing RecoveryRequest Implementation

**AIAnalysis Code**: Only has `/api/v1/incident/analyze` endpoint

**Missing**: `/api/v1/recovery/analyze` with `RecoveryRequest` schema

**RecoveryRequest Schema** (from HAPI OpenAPI):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `incident_id` | string | ✅ YES | Unique incident identifier |
| `remediation_id` | string | ✅ YES | Audit correlation (MANDATORY) |
| `is_recovery_attempt` | boolean | No (default: false) | True if recovery |
| `recovery_attempt_number` | int | No | Which attempt (1, 2, 3...) |
| `previous_execution` | PreviousExecution | No | Context from failed attempt |
| `enrichment_results` | object | No | DetectedLabels, etc. |
| `signal_type` | string | No | Current signal type |
| `severity` | string | No | Current severity |
| `resource_namespace` | string | No | K8s namespace |
| `resource_kind` | string | No | K8s resource kind |
| `resource_name` | string | No | K8s resource name |
| `environment` | string | No (default: "unknown") | Environment |
| `priority` | string | No (default: "P2") | Priority level |
| `risk_tolerance` | string | No (default: "medium") | Risk tolerance |
| `business_category` | string | No (default: "standard") | Business category |
| `error_message` | string | No | Current error |
| `cluster_name` | string | No | Cluster name |
| `signal_source` | string | No | Signal source |

**PreviousExecution Schema** (nested):

```json
{
  "original_rca": {
    "summary": "string",
    "severity": "string",
    "signal_type": "string",
    "contributing_factors": ["string"]
  },
  "selected_workflow": {
    "workflow_id": "string",
    "container_image": "string",
    "version": "string",
    "parameters": {},
    "confidence": 0.0
  },
  "failure": {
    "reason": "string",
    "message": "string",
    "exit_code": 0,
    "failed_step_name": "string",
    "failed_step_index": 0,
    "failed_at": "string",
    "execution_time": "string"
  },
  "natural_language_summary": "string"  // BR-HAPI-192
}
```

---

## 📋 Recommended Changes

### 1. Update IncidentRequest Struct

```go
// pkg/aianalysis/client/holmesgpt.go

// IncidentRequest represents request to /api/v1/incident/analyze
// Per HAPI OpenAPI spec (authoritative)
type IncidentRequest struct {
    // REQUIRED fields
    IncidentID        string `json:"incident_id"`
    RemediationID     string `json:"remediation_id"`      // MANDATORY per DD-WORKFLOW-002
    SignalType        string `json:"signal_type"`
    Severity          string `json:"severity"`
    SignalSource      string `json:"signal_source"`
    ResourceNamespace string `json:"resource_namespace"`
    ResourceKind      string `json:"resource_kind"`
    ResourceName      string `json:"resource_name"`
    ErrorMessage      string `json:"error_message"`
    Environment       string `json:"environment"`
    Priority          string `json:"priority"`
    RiskTolerance     string `json:"risk_tolerance"`
    BusinessCategory  string `json:"business_category"`
    ClusterName       string `json:"cluster_name"`

    // OPTIONAL fields
    Description                 *string            `json:"description,omitempty"`
    IsDuplicate                 *bool              `json:"is_duplicate,omitempty"`
    OccurrenceCount             *int               `json:"occurrence_count,omitempty"`
    DeduplicationWindowMinutes  *int               `json:"deduplication_window_minutes,omitempty"`
    IsStorm                     *bool              `json:"is_storm,omitempty"`
    StormSignalCount            *int               `json:"storm_signal_count,omitempty"`
    StormWindowMinutes          *int               `json:"storm_window_minutes,omitempty"`
    StormType                   *string            `json:"storm_type,omitempty"`
    AffectedResources           []string           `json:"affected_resources,omitempty"`
    FiringTime                  *string            `json:"firing_time,omitempty"`
    ReceivedTime                *string            `json:"received_time,omitempty"`
    FirstSeen                   *string            `json:"first_seen,omitempty"`
    LastSeen                    *string            `json:"last_seen,omitempty"`
    SignalLabels                map[string]string  `json:"signal_labels,omitempty"`
    EnrichmentResults           *EnrichmentResults `json:"enrichment_results,omitempty"`
}

// EnrichmentResults contains enriched context from SignalProcessing
type EnrichmentResults struct {
    DetectedLabels    map[string]interface{}      `json:"detectedLabels,omitempty"`
    CustomLabels      map[string][]string         `json:"customLabels,omitempty"`
    KubernetesContext map[string]interface{}      `json:"kubernetesContext,omitempty"`
    OwnerChain        []OwnerChainEntry           `json:"ownerChain,omitempty"`
}
```

### 2. Add RecoveryRequest and InvestigateRecovery Method

```go
// RecoveryRequest represents request to /api/v1/recovery/analyze
type RecoveryRequest struct {
    IncidentID            string             `json:"incident_id"`
    RemediationID         string             `json:"remediation_id"`
    IsRecoveryAttempt     bool               `json:"is_recovery_attempt"`
    RecoveryAttemptNumber *int               `json:"recovery_attempt_number,omitempty"`
    PreviousExecution     *PreviousExecution `json:"previous_execution,omitempty"`
    EnrichmentResults     map[string]interface{} `json:"enrichment_results,omitempty"`
    SignalType            *string            `json:"signal_type,omitempty"`
    Severity              *string            `json:"severity,omitempty"`
    ResourceNamespace     *string            `json:"resource_namespace,omitempty"`
    ResourceKind          *string            `json:"resource_kind,omitempty"`
    ResourceName          *string            `json:"resource_name,omitempty"`
    Environment           string             `json:"environment"`
    Priority              string             `json:"priority"`
    RiskTolerance         string             `json:"risk_tolerance"`
    BusinessCategory      string             `json:"business_category"`
    ErrorMessage          *string            `json:"error_message,omitempty"`
    ClusterName           *string            `json:"cluster_name,omitempty"`
    SignalSource          *string            `json:"signal_source,omitempty"`
}

type PreviousExecution struct {
    OriginalRCA            *OriginalRCA            `json:"original_rca,omitempty"`
    SelectedWorkflow       *SelectedWorkflowSummary `json:"selected_workflow,omitempty"`
    Failure                *ExecutionFailure       `json:"failure,omitempty"`
    NaturalLanguageSummary *string                 `json:"natural_language_summary,omitempty"`
}

// InvestigateRecovery calls /api/v1/recovery/analyze
func (c *HolmesGPTClient) InvestigateRecovery(ctx context.Context, req *RecoveryRequest) (*RecoveryResponse, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal recovery request: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
        c.baseURL+"/api/v1/recovery/analyze", bytes.NewReader(body))
    // ... rest of implementation
}
```

### 3. Update AIAnalysis Controller Logic

```go
// In investigating_handler.go or similar

func (h *Handler) callHAPI(ctx context.Context, aa *v1alpha1.AIAnalysis) (*client.IncidentResponse, error) {
    if aa.Spec.IsRecoveryAttempt {
        // Use recovery endpoint
        recoveryReq := buildRecoveryRequest(aa)
        return h.holmesClient.InvestigateRecovery(ctx, recoveryReq)
    }

    // Use incident endpoint
    incidentReq := buildIncidentRequest(aa)
    return h.holmesClient.Investigate(ctx, incidentReq)
}
```

---

## 📊 Field Mapping Reference

### AIAnalysis CRD Spec → HAPI IncidentRequest

| AIAnalysis Spec Field | HAPI IncidentRequest Field | Notes |
|----------------------|---------------------------|-------|
| `spec.SignalType` | `signal_type` | Direct map |
| `spec.Severity` | `severity` | Direct map |
| `spec.ResourceRef.Namespace` | `resource_namespace` | From ResourceRef |
| `spec.ResourceRef.Kind` | `resource_kind` | From ResourceRef |
| `spec.ResourceRef.Name` | `resource_name` | From ResourceRef |
| `spec.ErrorMessage` | `error_message` | Direct map |
| `spec.Environment` | `environment` | Direct map |
| `spec.Priority` | `priority` | Direct map |
| `spec.RiskTolerance` | `risk_tolerance` | Direct map |
| `spec.BusinessCategory` | `business_category` | Direct map |
| `spec.ClusterName` | `cluster_name` | Direct map |
| `metadata.name` | `incident_id` | Use CR name |
| `metadata.labels["remediation-id"]` | `remediation_id` | **MANDATORY** |
| `spec.DetectedLabels` | `enrichment_results.detectedLabels` | Nested |
| `spec.CustomLabels` | `enrichment_results.customLabels` | Nested |
| `spec.OwnerChain` | `enrichment_results.ownerChain` | Nested |

---

## 🎯 Action Items

| # | Action | Priority | Owner |
|---|--------|----------|-------|
| 1 | Update `IncidentRequest` struct with all required fields | P0 | AIAnalysis |
| 2 | Add `EnrichmentResults` struct | P0 | AIAnalysis |
| 3 | Add `RecoveryRequest` struct | P0 | AIAnalysis |
| 4 | Add `InvestigateRecovery()` method | P0 | AIAnalysis |
| 5 | Update controller to use correct endpoint based on `IsRecoveryAttempt` | P0 | AIAnalysis |
| 6 | Update tests to use new structs | P1 | AIAnalysis |
| 7 | Validate against OpenAPI spec | P1 | AIAnalysis |

---

## 📚 Authoritative References

| Document | Purpose |
|----------|---------|
| `holmesgpt-api/api/openapi.json` | **AUTHORITATIVE** - HAPI API contract |
| `DD-WORKFLOW-002` | `remediation_id` requirement |
| `BR-HAPI-192` | `natural_language_summary` in recovery |
| `DD-RECOVERY-002`, `DD-RECOVERY-003` | Recovery flow design |

---

## 📝 AIAnalysis Team Response

**Date**: December 9, 2025
**Responder**: AIAnalysis Team

**Acknowledgment**: [✅] Acknowledged

**Estimated Fix Timeline**: Day 11 (P0 items)

**Questions/Concerns**:
```
✅ ACKNOWLEDGED - Thank you for the detailed contract analysis.

This is a CRITICAL gap that explains why our integration tests have been using mocks
rather than the real HAPI service.

IMPLEMENTATION PLAN (Day 11):

1. P0 - IncidentRequest Schema Update
   - Add all required fields per OpenAPI spec
   - Add EnrichmentResults nested struct
   - Update buildRequest() in investigating_handler.go
   - Estimated: 2 hours

2. P0 - RecoveryRequest Implementation
   - Create RecoveryRequest struct with all fields
   - Add InvestigateRecovery() method to HolmesGPTClient
   - Update controller logic to route based on spec.IsRecoveryAttempt
   - Estimated: 2 hours

3. P1 - Test Updates
   - Update unit tests for new request structures
   - Add integration tests that validate field mappings
   - Estimated: 2 hours

CLARIFICATION QUESTIONS:
1. Is `metadata.name` correct for `incident_id` or should we use a UUID?
2. Is `metadata.labels["remediation-id"]` the correct source for `remediation_id`
   or should this come from `spec.RemediationID`?

NO BLOCKERS - This can proceed immediately.
```

---

### HAPI Team Response to Clarification Questions

**Date**: December 9, 2025
**Responder**: HAPI Team

**Authoritative Sources**:
- `holmesgpt-api/src/models/incident_models.py` (lines 173-182)
- `api/aianalysis/v1alpha1/aianalysis_types.go` (lines 44-47)

#### Q1: Is `metadata.name` correct for `incident_id` or should we use a UUID?

**Answer**: ✅ **`metadata.name` is correct**

Per `incident_models.py` line 173:
```python
incident_id: str = Field(..., description="Unique incident identifier")
```

**Rationale**:
- `incident_id` is simply a **unique identifier** for the incident
- The AIAnalysis CR's `metadata.name` is already unique within the namespace
- Using `metadata.name` provides natural correlation in logs/audit
- **DO NOT generate a UUID** - this would break traceability

**Mapping**: `incident_id = aa.ObjectMeta.Name`

---

#### Q2: Is `metadata.labels["remediation-id"]` or `spec.RemediationID` the source?

**Answer**: ✅ **Use `spec.RemediationID`**

Per `aianalysis_types.go` lines 44-47:
```go
// Remediation ID for audit correlation (DD-WORKFLOW-002 v2.2)
// +kubebuilder:validation:Required
// +kubebuilder:validation:MinLength=1
RemediationID string `json:"remediationId"`
```

Per `incident_models.py` lines 174-181:
```python
remediation_id: str = Field(
    ...,
    min_length=1,
    description=(
        "Remediation request ID for audit correlation (e.g., 'req-2025-11-27-abc123'). "
        "MANDATORY per DD-WORKFLOW-002 v2.2..."
    )
)
```

**Mapping**: `remediation_id = aa.Spec.RemediationID`

**Why NOT labels?**:
- `spec.RemediationID` is the **authoritative** source (per CRD definition)
- Labels are for K8s selectors, not data transfer
- Spec fields have validation (`MinLength=1`)

---

#### Summary: Field Mapping (AUTHORITATIVE)

| HAPI Field | AIAnalysis Source | Example |
|------------|-------------------|---------|
| `incident_id` | `aa.ObjectMeta.Name` | `"aianalysis-abc123"` |
| `remediation_id` | `aa.Spec.RemediationID` | `"req-2025-11-29-xyz789"` |

**Updated Code Pattern**:
```go
func buildIncidentRequest(aa *v1alpha1.AIAnalysis) *client.IncidentRequest {
    return &client.IncidentRequest{
        IncidentID:    aa.Name,                    // Q1: Use CR name
        RemediationID: aa.Spec.RemediationID,      // Q2: Use spec field
        SignalType:    aa.Spec.AnalysisRequest.SignalContext.SignalType,
        Severity:      aa.Spec.AnalysisRequest.SignalContext.Severity,
        // ... rest of mapping
    }
}
```

---

---

### AIAnalysis Team Acknowledgment of Q1/Q2 Answers

**Date**: December 9, 2025
**Status**: ✅ **CONFIRMED - All questions answered, ready for implementation**

**Acknowledgment**:
```
✅ Thank you for the clear answers with authoritative source references.

CONFIRMED FIELD MAPPINGS FOR DAY 11:

| HAPI Field      | AIAnalysis Source           | Status     |
|-----------------|----------------------------|------------|
| incident_id     | aa.Name                    | ✅ Confirmed |
| remediation_id  | aa.Spec.RemediationID      | ✅ Confirmed |

NO MORE BLOCKERS - Day 11 implementation can proceed immediately with:
1. Updated IncidentRequest struct (14 required fields)
2. RecoveryRequest struct + InvestigateRecovery() method
3. Field mapping per this document
```

---

## 📝 Document History

| Date | Author | Change |
|------|--------|--------|
| 2025-12-09 | HAPI Team | Initial contract mismatch analysis |
| 2025-12-09 | AIAnalysis Team | Acknowledged, asked clarification questions |
| 2025-12-09 | HAPI Team | Answered Q1 (use metadata.name) and Q2 (use spec.RemediationID) |
| 2025-12-09 | AIAnalysis Team | **TRIAGE COMPLETE** - All questions answered, ready for Day 11 |


