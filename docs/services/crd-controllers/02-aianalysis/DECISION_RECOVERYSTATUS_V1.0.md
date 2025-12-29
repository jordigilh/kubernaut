# DECISION: RecoveryStatus Field - V1.0 REQUIRED

**Date**: December 11, 2025 (Original), December 29, 2025 (Revised)
**Decision**: ✅ **V1.0 REQUIRED** (Decision Reversed)
**Authority**: crd-schema.md, DD-RECOVERY-002, HAPI OpenAPI spec

---

## 📋 Summary

**Decision**: `status.recoveryStatus` field is **V1.0 REQUIRED** for complete recovery observability.

**Rationale**: While recovery flow executes without this field, operator visibility requires RecoveryStatus to understand why recovery attempts succeed or fail. Low implementation cost (2-3 hours) justifies V1.0 inclusion.

**Decision Reversal**: Originally deferred to V1.1+ on December 11, but reversed on December 29 after recognizing critical operator experience gap.

---

## 🔍 Investigation

### **What is RecoveryStatus?**

```go
// From api/aianalysis/v1alpha1/aianalysis_types.go:528
type RecoveryStatus struct {
    // Assessment of why previous attempt failed
    PreviousAttemptAssessment *PreviousAttemptAssessment `json:"previousAttemptAssessment,omitempty"`
    // Whether the signal type changed due to the failed workflow
    StateChanged bool `json:"stateChanged"`
    // Current signal type (may differ from original after failed workflow)
    CurrentSignalType string `json:"currentSignalType,omitempty"`
}

type PreviousAttemptAssessment struct {
    // Whether the failure was understood
    FailureUnderstood bool `json:"failureUnderstood"`
    // Analysis of why the failure occurred
    FailureReasonAnalysis string `json:"failureReasonAnalysis"`
}
```

**Purpose**: Provides observability into HolmesGPT-API's assessment of the previous failed attempt.

---

### **Evidence Collected**

#### **1. CRD Schema** (crd-schema.md:427)

```go
// AIAnalysisStatus
RecoveryStatus *RecoveryStatus `json:"recoveryStatus,omitempty"`
```

**Finding**: Field is marked `omitempty` → **OPTIONAL**

---

#### **2. Recovery Flow** (DD-RECOVERY-002)

**Status**: ✅ APPROVED (Nov 29, 2025)

**Recovery Flow BRs** (BR_MAPPING.md):
| BR ID | Description | V1.0 | Status |
|-------|-------------|------|--------|
| BR-AI-080 | Support recovery attempts | ✅ | **COMPLETE** |
| BR-AI-081 | Accept previous execution context | ✅ | **COMPLETE** |
| BR-AI-082 | Call HolmesGPT-API recovery endpoint | ✅ | **COMPLETE** |
| BR-AI-083 | Reuse original enrichment | ✅ | **COMPLETE** |

**Finding**: Recovery flow BRs are V1.0, but they focus on **spec fields** (`isRecoveryAttempt`, `previousExecutions`), not **status fields** (`recoveryStatus`)

---

#### **3. HolmesGPT-API** (HAPI)

**Endpoint**: `POST /api/v1/recovery/analyze`

**Response** (from holmesgpt-api/src/extensions/recovery.py:603-609):
```json
{
  "recovery_analysis": {
    "previous_attempt_assessment": {
      "failure_understood": true,
      "failure_reason_analysis": "Explanation of why previous attempt failed",
      "state_changed": true,
      "current_signal_type": "Current signal type after failure"
    }
  },
  "selected_workflow": { ... }
}
```

**Finding**: HAPI **DOES** return recovery analysis data ✅

---

#### **4. AIAnalysis Controller** (pkg/aianalysis/)

**Search Result**: No references to `RecoveryStatus`, `recovery_analysis`, or `previousAttemptAssessment`

**Finding**: AIAnalysis controller **DOES NOT** populate this field ❌

---

## ✅ **Decision Matrix**

| Aspect | Evidence | V1.0 Requirement? |
|--------|----------|-------------------|
| **CRD Field Defined** | ✅ Yes (aianalysis_types.go:528) | Optional (`omitempty`) |
| **Recovery Flow BRs** | ✅ 4 BRs (BR-AI-080-083) | **Spec fields, not status** |
| **HAPI Returns Data** | ✅ Yes (recovery_analysis) | Available if needed |
| **Controller Populates** | ❌ No implementation | **Gap identified** |
| **Critical for Recovery** | ❌ No | Recovery works without it |
| **Observability Value** | ✅ Yes | Nice-to-have |

**Score**: 2/6 criteria suggest V1.0 requirement

---

## 🎯 **Final Decision: V1.0 REQUIRED (Decision Reversed)**

### **Original Decision (December 11, 2025)**: Defer to V1.1+

**Why Deferred**:
1. Recovery flow works without it
2. Observability-only field
3. V1.0 focus: core functionality

### **Revised Decision (December 29, 2025)**: V1.0 REQUIRED

**Why Reversed**:

1. **Operator Experience Gap**
   - Without RecoveryStatus, operators cannot easily see:
     - Why recovery succeeded/failed (`failureReasonAnalysis`)
     - If system state changed (`stateChanged`)
     - Current signal type after recovery (`currentSignalType`)
   - Debugging recovery requires checking audit trail (poor UX)

2. **Low Implementation Cost**
   - HAPI already returns the data ✅
   - Controller change: **30 lines** of code
   - Test coverage: **50 lines** of tests
   - Total effort: **2-3 hours**
   - Cost-benefit ratio strongly favors V1.0

3. **Recovery Flow Completeness**
   - `spec.isRecoveryAttempt` ✅ Triggers recovery
   - `spec.recoveryAttemptNumber` ✅ Tracks attempts
   - `spec.previousExecutions` ✅ Failure context
   - `status.recoveryStatus` ✅ **HAPI assessment** ← Missing piece

4. **Field Already Defined**
   - CRD schema has the field
   - HAPI returns the data
   - Just needs controller logic

**Conclusion**: The deferral was premature. Operator visibility is critical for V1.0 recovery success.

---

## 📊 **Impact Assessment**

### **WITHOUT RecoveryStatus (Original V1.0 Plan - REJECTED)**

**What Works**:
- ✅ Recovery attempts execute successfully
- ✅ HolmesGPT-API analyzes previous failures
- ✅ Alternative workflows are selected
- ✅ Operators see `isRecoveryAttempt = true` in spec
- ✅ Full failure context in `previousExecutions`

**What's Missing**:
- ❌ No status field showing HAPI's assessment of failure
- ❌ Operators must check audit trail to see "failure_understood"
- ❌ No status field showing if signal type changed
- ❌ Poor operator debugging experience

**Impact**: **MEDIUM** - Observability gap affects operator experience (decision reversed)

---

### **WITH RecoveryStatus (V1.0 - APPROVED)**

**What Improves**:
- ✅ Status shows HAPI's assessment: `failureUnderstood`, `failureReasonAnalysis`
- ✅ Status shows if system state changed: `stateChanged`
- ✅ Status shows current signal type vs original: `currentSignalType`
- ✅ Better operator visibility via `kubectl describe`
- ✅ Complete recovery observability for V1.0

**Example**:
```yaml
status:
  phase: Completed
  recoveryStatus:
    previousAttemptAssessment:
      failureUnderstood: true
      failureReasonAnalysis: "RBAC permissions insufficient for deployment patching"
    stateChanged: false
    currentSignalType: OOMKilled
  selectedWorkflow:
    workflowId: oomkill-restart-pods
```

---

## 📝 **Implementation Plan for V1.0**

### **Status**: ⏳ **PENDING HAPI TEAM CONFIRMATION**

**Implementation Request**: See `docs/shared/REQUEST_HAPI_RECOVERYSTATUS_V1_0.md`

### **Effort Estimate**: 2-3 hours

**Files to Update**:
1. `pkg/aianalysis/handlers/investigating.go` (~30 lines)
   - Parse `recovery_analysis` from HAPI response
   - Map to `RecoveryStatus` struct
   - Populate `analysis.Status.RecoveryStatus`

2. `test/unit/aianalysis/investigating_handler_test.go` (~50 lines)
   - Add test for RecoveryStatus population
   - Verify field mapping from HAPI response

3. `test/integration/aianalysis/recovery_integration_test.go` (~30 lines)
   - Add assertion for RecoveryStatus in recovery tests

**Code Pattern** (for V1.1):
```go
// pkg/aianalysis/handlers/investigating.go
if analysis.Spec.IsRecoveryAttempt {
    // Parse recovery_analysis from HAPI response
    if resp.RecoveryAnalysis != nil {
        analysis.Status.RecoveryStatus = &aianalysisv1.RecoveryStatus{
            StateChanged:      resp.RecoveryAnalysis.PreviousAttemptAssessment.StateChanged,
            CurrentSignalType: resp.RecoveryAnalysis.PreviousAttemptAssessment.CurrentSignalType,
            PreviousAttemptAssessment: &aianalysisv1.PreviousAttemptAssessment{
                FailureUnderstood:     resp.RecoveryAnalysis.PreviousAttemptAssessment.FailureUnderstood,
                FailureReasonAnalysis: resp.RecoveryAnalysis.PreviousAttemptAssessment.FailureReasonAnalysis,
            },
        }
    }
}
```

---

## ✅ **V1.0 Approval Criteria Met**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| **Recovery Flow Works** | ✅ COMPLETE | 4/4 Recovery BRs implemented |
| **HAPI Integration** | ✅ COMPLETE | `/recovery/analyze` endpoint called |
| **Spec Fields** | ✅ COMPLETE | `isRecoveryAttempt`, `previousExecutions` |
| **Tests** | ✅ COMPLETE | 8/8 recovery tests passing |
| **RecoveryStatus** | ⏳ **DEFERRED** | Observability enhancement for V1.1+ |

**Result**: V1.0 can ship without RecoveryStatus field population

---

## 🎯 **Recommendations**

### **For V1.0**

1. ✅ **Ship without RecoveryStatus** - Recovery flow is complete
2. ✅ **Document deferral** - Update AIANALYSIS_TRIAGE.md (DONE Dec 11)
3. ✅ **Add to V1.1 roadmap** - 2-3 hour enhancement

### **For V1.1**

1. Implement RecoveryStatus population (2-3 hours)
2. Add E2E test verifying RecoveryStatus appears in `kubectl describe`
3. Update operator documentation with RecoveryStatus interpretation

---

## 📚 **References**

| Document | Section | Finding |
|----------|---------|---------|
| `api/aianalysis/v1alpha1/aianalysis_types.go:528` | RecoveryStatus type | **Defined** |
| `docs/services/crd-controllers/02-aianalysis/crd-schema.md:427` | Status fields | **Optional** (`omitempty`) |
| `docs/architecture/decisions/DD-RECOVERY-002` | Recovery flow | **Approved** (Nov 29, 2025) |
| `docs/services/crd-controllers/02-aianalysis/BR_MAPPING.md` | Recovery BRs | **4/4 BRs complete** |
| `holmesgpt-api/src/extensions/recovery.py:603-609` | HAPI response | **Data available** |
| `pkg/aianalysis/handlers/investigating.go` | Population logic | **NOT implemented** |
| `docs/audits/v1.0-implementation-triage/AIANALYSIS_TRIAGE.md:67` | Status | **Deferred** |

---

**Decision**: ✅ **APPROVED TO DEFER**
**Date**: 2025-12-11
**Updated**: AIANALYSIS_TRIAGE.md v1.2
**File**: `docs/services/crd-controllers/02-aianalysis/DECISION_RECOVERYSTATUS_V1.0.md`







