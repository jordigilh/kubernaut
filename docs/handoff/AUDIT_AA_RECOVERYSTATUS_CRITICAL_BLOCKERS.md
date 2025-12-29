# CRITICAL AUDIT: AIAnalysis RecoveryStatus Implementation Blockers

**To**: AIAnalysis Team
**From**: Senior Developer (Audit)
**Date**: December 11, 2025
**Subject**: 🚨 RecoveryStatus Implementation BLOCKED - API Contract Mismatch
**Priority**: 🔴 **CRITICAL V1.0 BLOCKER**
**Status**: 🔴 **CANNOT PROCEED** - Requires Architecture Decision

---

## 📊 **Executive Summary**

**Status**: 🔴 **BLOCKED** - RecoveryStatus implementation cannot proceed as planned

**Root Cause**: Fundamental API contract mismatch between:
1. AIAnalysis team's expectations (implementation plan assumptions)
2. HolmesGPT-API actual OpenAPI specification
3. Existing `investigating.go` implementation

**Impact**:
- The entire 18-gap fixed implementation plan is based on **incorrect assumptions**
- `InvestigateRecovery()` method signature is **wrong** in `investigating.go`
- The handoff document claims "RecoveryStatus is ready" but **fundamental infrastructure is missing**
- V1.0 cannot ship without resolving this architectural issue

**Required Action**: **IMMEDIATE** architecture decision meeting between AIAnalysis and HolmesGPT-API teams

---

## 🔍 **Critical Finding #1: InvestigateRecovery Return Type Mismatch**

### **Current Implementation** (`investigating.go:54-59`):

```go
type HolmesGPTClientInterface interface {
    Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)
    InvestigateRecovery(ctx context.Context, req *client.RecoveryRequest) (*client.IncidentResponse, error)  // ❌ WRONG TYPE
}
```

**Assumption**: Both methods return `*client.IncidentResponse`

### **Actual OpenAPI Specification**:

**Endpoint**: `POST /api/v1/recovery/analyze`
**Returns**: `RecoveryResponse` (NOT `IncidentResponse`)

**RecoveryResponse Schema**:
```json
{
  "incident_id": "string",
  "can_recover": true,
  "strategies": [
    {
      "action_type": "scale_down_gradual",
      "confidence": 0.9,
      "estimated_risk": "low",
      "rationale": "...",
      "prerequisites": []
    }
  ],
  "primary_recommendation": "scale_down_gradual",
  "analysis_confidence": 0.85,
  "warnings": [],
  "metadata": {}
}
```

### **Gap Analysis**:

| Field Needed for RecoveryStatus | In RecoveryResponse? | In IncidentResponse? | **Status** |
|----------------------------------|----------------------|----------------------|------------|
| `state_changed` | ❌ NO | ❌ NO | 🔴 **MISSING** |
| `current_signal_type` | ❌ NO | ❌ NO | 🔴 **MISSING** |
| `previous_attempt_assessment` | ❌ NO | ❌ NO | 🔴 **MISSING** |
| `.failureUnderstood` | ❌ NO | ❌ NO | 🔴 **MISSING** |
| `.failureReasonAnalysis` | ❌ NO | ❌ NO | 🔴 **MISSING** |

**Conclusion**: ❌ **NONE** of the required fields exist in **ANY** HAPI response type

---

## 🔍 **Critical Finding #2: Missing recovery_analysis Field**

### **Implementation Plan Assumption**:

The fixed implementation plan (all 18 gaps) assumes:
```go
// Expected (from handoff document)
resp *client.IncidentResponse
resp.RecoveryAnalysis.PreviousAttemptAssessment.StateChanged       // ❌ DOESN'T EXIST
resp.RecoveryAnalysis.PreviousAttemptAssessment.FailureUnderstood  // ❌ DOESN'T EXIST
```

### **Actual IncidentResponse Schema**:

```json
{
  "incident_id": "string",
  "analysis": "string",
  "root_cause_analysis": {},
  "selected_workflow": {},
  "confidence": 0.8,
  "timestamp": "2025-12-11T...",
  "needs_human_review": false,
  "human_review_reason": null,
  "target_in_owner_chain": true,
  "warnings": [],
  "alternative_workflows": [],
  "validation_attempts_history": []
}
```

**Field**: `recovery_analysis`
**Status**: ❌ **DOES NOT EXIST**

### **Evidence**:

1. ✅ Verified IncidentResponse schema in `openapi.json` - NO `recovery_analysis` field
2. ✅ Verified ogen-generated Go client - NO `RecoveryAnalysis` field in struct
3. ✅ Searched entire codebase - `recovery_analysis` only exists in Python test files and docs

---

## 🔍 **Critical Finding #3: Existing Code Uses Wrong Pattern**

### **Current Implementation** (`investigating.go:88-101`):

```go
var resp *client.IncidentResponse  // ✅ Type declaration
var err error

// BR-AI-083: Route based on IsRecoveryAttempt
if analysis.Spec.IsRecoveryAttempt {
    h.log.Info("Using recovery endpoint",
        "attemptNumber", analysis.Spec.RecoveryAttemptNumber,
    )
    recoveryReq := h.buildRecoveryRequest(analysis)
    resp, err = h.hgClient.InvestigateRecovery(ctx, recoveryReq)  // ❌ WRONG: Returns RecoveryResponse, not IncidentResponse
} else {
    req := h.buildRequest(analysis)
    resp, err = h.hgClient.Investigate(ctx, req)  // ✅ CORRECT: Returns IncidentResponse
}
```

### **Problem**:

1. **Type Mismatch**: `InvestigateRecovery()` should return `*client.RecoveryResponse`, not `*client.IncidentResponse`
2. **Response Processing**: Lines 344-428 (`processResponse()`) expect `IncidentResponse` fields:
   - `resp.SelectedWorkflow` (exists in IncidentResponse, NOT in RecoveryResponse)
   - `resp.RootCauseAnalysis` (exists in IncidentResponse, NOT in RecoveryResponse)
   - `resp.NeedsHumanReview` (exists in IncidentResponse, NOT in RecoveryResponse)
3. **No Recovery Processing**: NO code exists to handle `RecoveryResponse` fields:
   - `resp.CanRecover`
   - `resp.Strategies`
   - `resp.PrimaryRecommendation`

---

## 🔍 **Critical Finding #4: RecoveryRequest is Correctly Built, But...**

### **buildRecoveryRequest() Analysis** (`investigating.go:187-221`):

```go
func (h *InvestigatingHandler) buildRecoveryRequest(analysis *aianalysisv1.AIAnalysis) *client.RecoveryRequest {
    // ✅ CORRECT: Builds proper RecoveryRequest with:
    // - IncidentID
    // - RemediationID
    // - IsRecoveryAttempt
    // - RecoveryAttemptNumber
    // - PreviousExecution context (lines 215-218)

    if len(analysis.Spec.PreviousExecutions) > 0 {
        prevExec := analysis.Spec.PreviousExecutions[len(analysis.Spec.PreviousExecutions)-1]
        req.PreviousExecution = h.buildPreviousExecution(prevExec)  // ✅ Maps to client.PreviousExecution
    }
}
```

**Status**: ✅ This part is CORRECT

**Problem**: The **response handling** is completely wrong

---

## 🧩 **Root Cause Analysis**

### **Why This Happened**:

1. **DD-RECOVERY-002 Misinterpretation**:
   - Decision document said "direct AIAnalysis recovery flow"
   - Team interpreted this as "call recovery endpoint and get IncidentResponse"
   - Reality: Recovery endpoint returns **different type** with **different fields**

2. **OpenAPI Spec Not Consulted**:
   - Implementation plan was written without checking actual OpenAPI spec
   - Handoff document claimed "HAPI returns recovery_analysis data (available)"
   - But OpenAPI spec shows NO such field exists

3. **Interface Definition Error**:
   - `HolmesGPTClientInterface` was defined with wrong return type
   - No compilation errors because method isn't called yet in tests
   - Would fail at runtime when RecoveryResponse is returned

---

## 🚧 **Why RecoveryStatus Implementation is BLOCKED**

### **Required for RecoveryStatus Population**:

Per `aianalysis_types.go:526-543`:
```go
type RecoveryStatus struct {
    PreviousAttemptAssessment *PreviousAttemptAssessment `json:"previousAttemptAssessment,omitempty"`
    StateChanged bool `json:"stateChanged"`
    CurrentSignalType string `json:"currentSignalType,omitempty"`
}

type PreviousAttemptAssessment struct {
    FailureUnderstood bool `json:"failureUnderstood"`
    FailureReasonAnalysis string `json:"failureReasonAnalysis"`
}
```

### **Available from HAPI**:

**Option A: IncidentResponse** (from `/incident/analyze`):
- ❌ No `state_changed`
- ❌ No `current_signal_type`
- ❌ No `previous_attempt_assessment`
- ❌ No `recovery_analysis`

**Option B: RecoveryResponse** (from `/recovery/analyze`):
- ❌ No `state_changed`
- ❌ No `current_signal_type`
- ❌ No `previous_attempt_assessment`
- ✅ Has `can_recover`, `strategies`, `primary_recommendation`

### **Conclusion**:

**NONE** of the required fields exist in **ANY** HAPI response type.

**RecoveryStatus CANNOT be populated without API changes.**

---

## 🎯 **Architecture Decision Required**

### **Option 1: Add recovery_analysis to IncidentResponse** (HAPI Change)

**Proposal**: Extend IncidentResponse with recovery_analysis field

**HAPI OpenAPI Change**:
```yaml
IncidentResponse:
  properties:
    # ... existing fields ...
    recovery_analysis:
      type: object
      nullable: true
      description: "Recovery-specific analysis (present only when isRecoveryAttempt=true)"
      properties:
        state_changed:
          type: boolean
          description: "Whether signal type changed due to failed workflow"
        current_signal_type:
          type: string
          description: "Current signal type (may differ from original)"
        previous_attempt_assessment:
          type: object
          properties:
            failure_understood:
              type: boolean
            failure_reason_analysis:
              type: string
```

**AIAnalysis Change**:
- Keep `InvestigateRecovery()` returning `*client.IncidentResponse`
- Add logic to populate `RecoveryStatus` from `resp.RecoveryAnalysis`

**Pros**:
- ✅ Keeps single response type for both endpoints
- ✅ Minimal changes to existing AIAnalysis code
- ✅ Follows handoff document assumptions

**Cons**:
- ⚠️ Requires HAPI API change
- ⚠️ Needs HAPI LLM prompt updates to generate recovery_analysis
- ⚠️ Client regeneration required

**Estimated Effort**:
- HAPI: 4-6 hours (OpenAPI, prompt, tests)
- AIAnalysis: 4-5 hours (per original plan)
- **Total**: 8-11 hours

---

### **Option 2: Use RecoveryResponse and Adapt** (AIAnalysis Change)

**Proposal**: Accept that recovery endpoint returns different type

**AIAnalysis Changes**:
```go
type HolmesGPTClientInterface interface {
    Investigate(ctx context.Context, req *client.IncidentRequest) (*client.IncidentResponse, error)
    InvestigateRecovery(ctx context.Context, req *client.RecoveryRequest) (*client.RecoveryResponse, error)  // ✅ CORRECT TYPE
}

// Add new method
func (h *InvestigatingHandler) processRecoveryResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
    // Map RecoveryResponse fields to RecoveryStatus
    // Problem: Required fields don't exist in RecoveryResponse
}
```

**Mapping Strategy** (derive from available fields):
```go
analysis.Status.RecoveryStatus = &aianalysisv1.RecoveryStatus{
    StateChanged: false,  // ❌ NOT AVAILABLE - must guess or default
    CurrentSignalType: "", // ❌ NOT AVAILABLE
    PreviousAttemptAssessment: &aianalysisv1.PreviousAttemptAssessment{
        FailureUnderstood: resp.CanRecover,  // ⚠️ APPROXIMATION (not same semantics)
        FailureReasonAnalysis: resp.PrimaryRecommendation,  // ⚠️ WRONG FIELD (not failure analysis)
    },
}
```

**Pros**:
- ✅ No HAPI changes required
- ✅ Correct API contract usage

**Cons**:
- ❌ **Cannot populate RecoveryStatus correctly** (required fields missing)
- ❌ Field mappings are approximations, not semantically correct
- ❌ `state_changed` and `current_signal_type` cannot be populated
- ❌ Violates business requirement intent

**Estimated Effort**:
- AIAnalysis: 6-8 hours (interface refactor, response handling split, tests)
- **Quality**: 🔴 **LOW** - Incomplete RecoveryStatus population

---

### **Option 3: Deprecate RecoveryStatus Field** (Not Recommended)

**Proposal**: Remove RecoveryStatus from V1.0 scope

**Changes**:
- Remove `RecoveryStatus` from `aianalysis_types.go`
- Remove from `crd-schema.md`
- Update BR-AI-080-083 to not include observability

**Pros**:
- ✅ Immediate unblock for V1.0 release
- ✅ No API changes needed

**Cons**:
- ❌ Violates V1.0 requirements (per handoff doc: "V1.0 BLOCKING")
- ❌ Removes observability feature
- ❌ crd-schema.md example shows RecoveryStatus (would be incorrect)

**Estimated Effort**:
- AIAnalysis: 2-3 hours (removal, docs update)
- **Impact**: 🔴 **HIGH** - Removes planned V1.0 feature

---

## 📋 **Recommended Path Forward**

### **My Recommendation: Option 1 (Add recovery_analysis to IncidentResponse)**

**Rationale**:
1. ✅ Preserves V1.0 feature completeness
2. ✅ Enables correct RecoveryStatus population
3. ✅ Matches implementation plan assumptions
4. ✅ Clean API contract
5. ⚠️ Requires cross-team coordination (8-11 hours total)

### **Immediate Next Steps**:

1. **Architecture Decision Meeting** (30 minutes):
   - AIAnalysis Team Lead
   - HolmesGPT-API Team Lead
   - Present 3 options with pros/cons
   - **Decide**: Option 1, 2, or 3

2. **If Option 1 Selected** (RECOMMENDED):

   **HAPI Team** (4-6 hours):
   - Update OpenAPI spec with `recovery_analysis` field
   - Update LLM prompt to generate recovery_analysis
   - Regenerate ogen client
   - Update mock responses
   - Run tests

   **AIAnalysis Team** (4-5 hours):
   - Wait for HAPI changes
   - Follow fixed implementation plan (18-gap fixes apply)
   - Proceed with RecoveryStatus implementation

3. **If Option 2 Selected**:
   - Accept incomplete RecoveryStatus population
   - Refactor interface to use RecoveryResponse
   - Document field mapping limitations
   - Update V1.0 scope expectations

4. **If Option 3 Selected**:
   - Remove RecoveryStatus from V1.0
   - Update documentation
   - Defer to V1.1

---

## 🚨 **Critical Blockers Summary**

| Blocker ID | Description | Impact | Resolution Required |
|------------|-------------|--------|---------------------|
| **B1** | `InvestigateRecovery()` returns wrong type | 🔴 CRITICAL | Architecture decision |
| **B2** | `recovery_analysis` field doesn't exist in any response | 🔴 CRITICAL | HAPI API change OR acceptance |
| **B3** | No code to handle `RecoveryResponse` | 🔴 CRITICAL | Code refactor if Option 2 |
| **B4** | Implementation plan based on wrong assumptions | 🟡 HIGH | Plan update after decision |
| **B5** | Handoff document claims "ready" but isn't | 🟡 HIGH | Document correction |

---

## 📊 **Verification Checklist**

Before proceeding with ANY RecoveryStatus implementation:

- [ ] Architecture decision made (Option 1/2/3)
- [ ] If Option 1: HAPI OpenAPI spec updated
- [ ] If Option 1: Ogen client regenerated
- [ ] If Option 2: Interface refactored to use RecoveryResponse
- [ ] Implementation plan updated based on decision
- [ ] Handoff document corrected
- [ ] All teams aligned on approach

---

## 🎯 **Questions for Architecture Meeting**

1. **API Design**: Should recovery endpoint return `IncidentResponse` or `RecoveryResponse`?
2. **Field Requirements**: Are `state_changed`, `current_signal_type`, `failure_understood` truly required for V1.0?
3. **Timeline**: Can HAPI team deliver API changes within V1.0 timeline? (4-6 hours estimated)
4. **Alternatives**: Is there existing HAPI data that could be repurposed for RecoveryStatus?

---

## 📞 **Contacts**

**AIAnalysis Team**: (waiting for architecture decision)
**HolmesGPT-API Team**: (needs to attend architecture meeting)
**This Audit**: Senior Developer performing V1.0 verification

---

**Prepared by**: AIAnalysis Team Member (Audit)
**Date**: December 11, 2025
**Version**: 1.0
**Status**: 🔴 **BLOCKING** - Requires immediate architecture decision meeting

**Next Action**: Schedule architecture decision meeting with both teams ASAP

---

## 📎 **Appendices**

### **Appendix A: Current investigating.go Audit**

**File**: `pkg/aianalysis/handlers/investigating.go`
**Status**: ✅ **MOSTLY CORRECT** except for recovery handling

**Correct Patterns**:
- ✅ Handler has `h.log` field (DD-005 compliant)
- ✅ Uses `*client.IncidentResponse` for Investigate()
- ✅ Builds RecoveryRequest correctly
- ✅ Error handling with retry logic
- ✅ Condition setting
- ✅ Integration with main controller

**Incorrect Patterns**:
- ❌ Interface declares `InvestigateRecovery()` returns `*client.IncidentResponse` (should be `*client.RecoveryResponse`)
- ❌ No code to handle `RecoveryResponse` type
- ❌ No RecoveryStatus population logic

**Confidence in Existing Code**: 95% (everything correct except recovery endpoint handling)

---

### **Appendix B: OpenAPI Spec Evidence**

**IncidentResponse Fields** (from `openapi.json`):
```
incident_id, analysis, root_cause_analysis, selected_workflow,
confidence, timestamp, needs_human_review, human_review_reason,
target_in_owner_chain, warnings, alternative_workflows,
validation_attempts_history
```

**RecoveryResponse Fields** (from `openapi.json`):
```
incident_id, can_recover, strategies, primary_recommendation,
analysis_confidence, warnings, metadata
```

**recovery_analysis**: ❌ NOT FOUND in either schema

---

### **Appendix C: Impact on V1.0 Timeline**

**Current V1.0 Status** (per handoff doc):
- 90-95% Complete
- RecoveryStatus is "V1.0 BLOCKING"
- Estimated 4-6 hours remaining

**With This Blocker**:
- **Option 1**: +4-6 hours (HAPI) + 4-5 hours (AIAnalysis) = **8-11 hours**
- **Option 2**: +6-8 hours (AIAnalysis refactor) = **6-8 hours** (but incomplete feature)
- **Option 3**: +2-3 hours (removal) = **2-3 hours** (but missing V1.0 feature)

**Revised V1.0 Timeline**:
- Best case: +8-11 hours (Option 1, assuming HAPI team available)
- Worst case: Feature removed from V1.0 (Option 3)

---

**END OF AUDIT REPORT**
