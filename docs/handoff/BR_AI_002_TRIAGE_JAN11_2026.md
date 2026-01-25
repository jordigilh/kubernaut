# BR-AI-002 Triage: Support Multiple Analysis Types

**Date**: January 11, 2026
**Status**: ❌ NOT IMPLEMENTED → ⏸️ **DEFERRED TO v2.0**
**Priority**: ~~P1 (HIGH)~~ → Deferred to v2.0
**Confidence**: 100% - Comprehensive analysis completed
**Authority**: [DD-AIANALYSIS-005](../architecture/decisions/DD-AIANALYSIS-005-multiple-analysis-types-deferral.md) - Multiple Analysis Types Feature Deferral

---

## **IMPORTANT: This is a Historical Triage Document**

**Authoritative Decision**: See [DD-AIANALYSIS-005](../architecture/decisions/DD-AIANALYSIS-005-multiple-analysis-types-deferral.md)

This document contains the **comprehensive gap analysis** that led to the deferral decision. For current v1.x behavior and v2.0 planning, refer to DD-AIANALYSIS-005.

---

## Executive Summary

**BR-AI-002 is NOT implemented** in either the AIAnalysis controller or HAPI. The `AnalysisTypes` field exists in the CRD schema but is **completely unused** by the implementation.

**Current Test Failure**: Tests expect 2 HAPI calls when `AnalysisTypes: ["investigation", "workflow-selection"]` but controller only makes 1 call, ignoring the field entirely.

---

## Original Business Requirement (BR-AI-002)

### Documented Requirement

**From**: `docs/services/crd-controllers/02-aianalysis/BUSINESS_REQUIREMENTS.md:63-85`

```markdown
#### BR-AI-002: Support Multiple Analysis Types

**Description**: AIAnalysis MUST support multiple analysis types (diagnostic, predictive, prescriptive) through HolmesGPT-API.

**Priority**: P1 (HIGH)

**Rationale**: Different alert types require different investigation approaches. Diagnostic analysis identifies current issues, while predictive/prescriptive provide future guidance.

**Implementation**:
- HolmesGPT-API determines analysis type based on alert context
- `status.analysisType`: Captured from HolmesGPT response
- Investigation flow adapts to analysis type

**Acceptance Criteria**:
- ✅ HolmesGPT-API handles analysis type determination
- ✅ AIAnalysis passes through analysis results
```

### Original Design Intent

**HAPI determines analysis type automatically** based on incident context:
- Diagnostic: Current issue identification
- Predictive: Future risk assessment
- Prescriptive: Recommended actions

The controller was supposed to:
1. Send incident context to HAPI
2. HAPI determines appropriate analysis type
3. Controller captures `analysisType` in status
4. Flow adapts based on type

---

## Current Implementation Reality

### CRD Schema

**File**: `api/aianalysis/v1alpha1/aianalysis_types.go:107-109`

```go
// Analysis types to perform (e.g., "investigation", "root-cause", "workflow-selection")
// +kubebuilder:validation:MinItems=1
AnalysisTypes []string `json:"analysisTypes"`
```

**CRD Comment Examples**:
- `"investigation"`
- `"root-cause"`
- `"workflow-selection"`

**Note**: These do NOT match BR-AI-002's "diagnostic, predictive, prescriptive"

### Controller Implementation

**Search Results**: `pkg/aianalysis/` - **ZERO references** to `AnalysisTypes`

**File**: `pkg/aianalysis/handlers/investigating.go:157-167`

```go
// Makes SINGLE HAPI call regardless of AnalysisTypes
req := h.builder.BuildIncidentRequest(analysis) // Doesn't include AnalysisTypes
incidentResp, err := h.hgClient.Investigate(ctx, req)

// Records single HolmesGPT API call
h.auditClient.RecordHolmesGPTCall(ctx, analysis, "/api/v1/incident/analyze", statusCode, int(investigationTime))
```

**Behavior**: Controller makes **exactly 1 HAPI call** per reconciliation, completely ignoring the `AnalysisTypes` field.

### HAPI Implementation

**Search Results**: `holmesgpt-api/` - **ZERO references** to:
- `analysis_type`
- `analysisTypes`
- `diagnostic`
- `predictive`
- `prescriptive`

**Available Endpoints**:
| Endpoint | Purpose | Analysis Type |
|---|---|---|
| `/api/v1/incident/analyze` | Initial incident RCA + workflow selection | Fixed |
| `/api/v1/recovery/analyze` | Recovery after failed remediation | Fixed |
| `/api/v1/postexec/analyze` | Post-execution effectiveness | Fixed |

**HAPI Reality**: Each endpoint performs a **single, fixed analysis type**. There is NO support for:
- Multiple analysis types in a single request
- Dynamic analysis type selection
- diagnostic/predictive/prescriptive categorization

### Request Builder Implementation

**File**: `pkg/aianalysis/handlers/request_builder.go:64-90`

```go
func (b *RequestBuilder) BuildIncidentRequest(analysis *aianalysisv1.AIAnalysis) *client.IncidentRequest {
    spec := analysis.Spec.AnalysisRequest.SignalContext
    enrichment := spec.EnrichmentResults

    req := &client.IncidentRequest{
        IncidentID:        analysis.Name,
        RemediationID:     analysis.Spec.RemediationID,
        SignalType:        spec.SignalType,
        Severity:          spec.Severity,
        // ... other fields
    }
    // ❌ NO reference to analysis.Spec.AnalysisRequest.AnalysisTypes
    return req
}
```

**Missing**: No code path that reads or processes `AnalysisTypes`

---

## Test Expectations vs Reality

### Test Code

**File**: `test/integration/aianalysis/audit_flow_integration_test.go:354-358`

```go
// HolmesGPT calls: 2 calls (1 for investigation, 1 for workflow-selection)
// Test spec requests AnalysisTypes: ["investigation", "workflow-selection"]
// Each analysis type triggers a separate HAPI call
Expect(eventTypeCounts[aiaudit.EventTypeHolmesGPTCall]).To(Equal(2),
    "Expected exactly 2 HolmesGPT API calls (investigation + workflow-selection)")
```

**Test Assumption**: Multiple `AnalysisTypes` → multiple HAPI calls

**Reality**: Controller makes 1 call, test fails

### Test Creation Patterns

Tests use various `AnalysisTypes` values (90+ occurrences):
- `["investigation"]`
- `["investigation", "workflow-selection"]`
- `["incident-analysis", "workflow-selection"]`
- `["recovery-analysis", "workflow-selection"]`
- `["recommendation"]`

**All ignored by controller**.

---

## Gap Analysis

### Feature Gap Summary

| Component | Expected (BR-AI-002) | Current Reality | Gap |
|---|---|---|---|
| **HAPI** | Determines analysis type dynamically | Fixed analysis per endpoint | ❌ No dynamic type support |
| **Controller** | Sends context, receives type | Makes 1 HAPI call, ignores `AnalysisTypes` | ❌ Field unused |
| **CRD Status** | `status.analysisType` field | Field doesn't exist | ❌ No status field |
| **Tests** | Pass with multiple types | Fail expecting multiple calls | ❌ Incorrect expectations |

### What's Missing

#### 1. HAPI Side (Python)
- No support for `analysis_types` in request
- No logic to perform multiple analyses in one call
- No "diagnostic vs predictive vs prescriptive" categorization
- Endpoints are **fixed-function**, not dynamic

#### 2. Controller Side (Go)
- `AnalysisTypes` field never read
- No loop to process multiple types
- No logic to make multiple HAPI calls
- No status field to capture analysis type
- RequestBuilder doesn't include `AnalysisTypes`

#### 3. OpenAPI Contract
```bash
$ grep -r "analysis_type\|analysisTypes" api/openapi/holmesgpt-api-v1.yaml
# NO RESULTS
```

HAPI OpenAPI spec has **NO** `analysisTypes` field in:
- `IncidentRequest`
- `RecoveryRequest`
- `IncidentResponse`
- `RecoveryResponse`

---

## Design Conflict: Original Intent vs CRD Schema

### Original BR-AI-002 Design
**Model**: HAPI-driven analysis type determination
```
Controller → [Incident Context] → HAPI → [Determines: diagnostic/predictive/prescriptive] → Response with type
```

### CRD Schema Interpretation
**Model**: Controller-driven multiple analysis requests
```
Controller → [AnalysisTypes: ["investigation", "workflow-selection"]] → Multiple HAPI calls → Aggregate results
```

**These are fundamentally different designs!**

---

## Root Cause

BR-AI-002 was **never implemented**. The disconnect occurred because:

1. **Original BR**: HAPI determines analysis type (passive controller)
2. **CRD Schema**: Controller requests multiple types (active controller)
3. **Implementation**: Neither approach was actually built
4. **Tests**: Written assuming the CRD schema interpretation would work

The `AnalysisTypes` field was added to the CRD but:
- No controller code reads it
- No HAPI code supports it
- No OpenAPI contract defines it

---

## Impact Assessment

### Current Failures

**Test**: `audit_flow_integration_test.go:357`
```
Expected: 2 HolmesGPT API calls (investigation + workflow-selection)
Got: 1 HolmesGPT API call
```

**Reason**: Controller doesn't loop through `AnalysisTypes`

### Cascading Issues

1. **37 Skipped Tests**: `--fail-fast` flag stops execution after first failure
2. **False Test Assumptions**: 11+ tests use multiple `AnalysisTypes` expecting multiple calls
3. **Unused CRD Field**: `AnalysisTypes` is required (`MinItems=1`) but ignored
4. **Misleading Documentation**: BR-AI-002 marked "✅ Implemented" but it's not

---

## Options for Resolution

### Option A: Fix Tests (Quick - Recommended for Now)

**Approach**: Align tests with current implementation reality

**Changes**:
1. Use single `AnalysisTypes` in all tests: `["investigation"]`
2. Update audit event counts from 2 → 1 for HAPI calls
3. Add TODO comments referencing BR-AI-002 gap

**Pros**:
- ✅ Tests pass immediately
- ✅ No controller or HAPI changes
- ✅ Unblocks multi-controller migration

**Cons**:
- ❌ Defers BR-AI-002 implementation
- ❌ Field remains unused

**Estimated Time**: 30 minutes

---

### Option B: Implement BR-AI-002 (Feature Development)

**Approach**: Implement multiple analysis types as per CRD schema interpretation

**Changes Required**:

#### 1. HAPI (Python)
- Add `analysis_types: List[str]` to `IncidentRequest`
- Loop through types, call appropriate logic for each
- Return array of results or aggregated response
- Update OpenAPI spec

#### 2. Controller (Go)
- Read `analysis.Spec.AnalysisRequest.AnalysisTypes`
- Loop through each type:
  ```go
  for _, analysisType := range analysis.Spec.AnalysisRequest.AnalysisTypes {
      // Make HAPI call with type-specific request
      // Emit separate audit event for each
  }
  ```
- Aggregate responses
- Update status with all analysis types performed

#### 3. CRD Status
- Add `status.performedAnalyses: []PerformedAnalysis` with:
  - `type: string`
  - `confidence: float64`
  - `timestamp: metav1.Time`

**Pros**:
- ✅ Fulfills P1 business requirement
- ✅ Makes `AnalysisTypes` field functional
- ✅ Tests work as designed

**Cons**:
- ❌ HAPI architecture doesn't support this (endpoints are fixed-function)
- ❌ Requires HAPI refactoring (400+ tests to update)
- ❌ Unclear if "diagnostic/predictive/prescriptive" matches current use cases
- ❌ 2-4 days of work

**Estimated Time**: 2-4 days

---

### Option C: Remove AnalysisTypes Field (Breaking Change)

**Approach**: Remove unused field from CRD schema

**Changes**:
- Delete `AnalysisTypes` from CRD
- Update all 90+ test files
- Remove from OpenAPI generation

**Pros**:
- ✅ Eliminates confusion
- ✅ Aligns CRD with implementation

**Cons**:
- ❌ Breaking API change
- ❌ Requires CRD version bump
- ❌ BR-AI-002 still unimplemented

**Estimated Time**: 1 day

---

### Option D: Clarify BR-AI-002 Original Intent (Documentation)

**Approach**: Implement original BR-AI-002 design (HAPI determines type)

**Changes**:
- Add `analysis_type` to HAPI responses
- Add `status.analysisType: string` to CRD status
- Remove/deprecate `spec.analysisTypes` (or make it optional hints)
- HAPI categorizes analysis as diagnostic/predictive/prescriptive

**Pros**:
- ✅ Matches original BR-AI-002 intent
- ✅ Simpler than multiple calls
- ✅ HAPI controls complexity

**Cons**:
- ❌ Still requires HAPI changes
- ❌ Doesn't match test expectations
- ❌ 1-2 days of work

**Estimated Time**: 1-2 days

---

## Recommendation

**Adopt Option A** (Fix Tests) for immediate unblocking:

1. **Short-term**: Simplify tests to use single `AnalysisTypes`
2. **Document**: Create BR-AI-002 backlog item for future implementation
3. **Unblock**: Complete multi-controller migration for other services
4. **Revisit**: Determine if BR-AI-002 is still needed based on production usage

**Rationale**:
- Feature was never implemented or used
- No production code depends on it
- P1 priority may be outdated (feature gap report from Dec 2025 marked it "medium priority")
- Current focus: multi-controller migration (higher immediate value)

---

## Questions for Product/Business

1. Is BR-AI-002 still a P1 requirement in 2026?
2. Do we need multiple analysis types or is single incident/recovery analysis sufficient?
3. What real-world use case requires diagnostic vs predictive vs prescriptive?
4. Should we:
   - A) Implement as multiple HAPI calls (controller-driven)
   - B) Implement as HAPI categorization (HAPI-driven)
   - C) Remove the feature entirely

---

## Conclusion

**BR-AI-002 is NOT implemented** and the `AnalysisTypes` field is **completely unused** despite being required in the CRD schema.

**Current test failure** is due to tests expecting unimplemented functionality.

**Recommended Action**: Fix tests to match implementation (Option A), defer feature to backlog.

