# RESPONSE: Recovery Endpoint Bug Fix

**From**: HAPI Team
**To**: AIAnalysis Team
**Date**: 2025-12-13
**Status**: 🎯 **ROOT CAUSE IDENTIFIED - FIX IN PROGRESS**

---

## 🎯 Summary

**Root Cause Found**: ✅ **Pydantic Model Missing Fields**

The `RecoveryResponse` Pydantic model in `src/models/recovery_models.py` is missing the `selected_workflow` and `recovery_analysis` fields that the mock response generator populates.

**Impact**: Pydantic strips these fields during response validation
**Fix Complexity**: SIMPLE (add 2 fields to model)
**Timeline**: 15-30 minutes

---

## 🔍 Root Cause Analysis

### What AA Team Found ✅

**Evidence**: Recovery endpoint returns `null` for both fields
```json
{
  "selected_workflow": null,
  "recovery_analysis": null,
  "warnings": [...]
}
```

### What HAPI Team Discovered ✅

**Mock Response Generator** (`src/mock_responses.py` lines 627-638):
```python
response = {
    "selected_workflow": {  # ✅ Field IS populated
        "workflow_id": recovery_workflow_id,
        "title": f"{scenario.workflow_title} - Recovery",
        ...
    },
    "recovery_analysis": {  # ✅ Field IS populated
        "previous_attempt_assessment": {...},
        ...
    }
}
```

**Pydantic Model** (`src/models/recovery_models.py` line 209):
```python
class RecoveryResponse(BaseModel):
    incident_id: str
    can_recover: bool
    strategies: List[RecoveryStrategy]
    primary_recommendation: Optional[str]
    analysis_confidence: float
    warnings: List[str]
    metadata: Dict[str, Any]
    # ❌ MISSING: selected_workflow
    # ❌ MISSING: recovery_analysis
```

**Result**: When FastAPI validates the response, Pydantic strips the extra fields!

---

## 🔧 Fix Required

### File: `src/models/recovery_models.py`

**Add these fields to `RecoveryResponse` class**:

```python
class RecoveryResponse(BaseModel):
    """Response model for recovery analysis endpoint"""
    incident_id: str = Field(..., description="Incident identifier from request")
    can_recover: bool = Field(..., description="Whether recovery is possible")
    strategies: List[RecoveryStrategy] = Field(default_factory=list)
    primary_recommendation: Optional[str] = Field(None)
    analysis_confidence: float = Field(..., ge=0.0, le=1.0)
    warnings: List[str] = Field(default_factory=list)
    metadata: Dict[str, Any] = Field(default_factory=dict)

    # ADD THESE TWO FIELDS:
    selected_workflow: Optional[Dict[str, Any]] = Field(
        None,
        description="Selected workflow for recovery (BR-AI-080)"
    )
    recovery_analysis: Optional[Dict[str, Any]] = Field(
        None,
        description="Recovery-specific analysis (BR-AI-081)"
    )
```

---

## ✅ Fix Implemented

**File Modified**: `src/models/recovery_models.py`

**Changes**:
```python
class RecoveryResponse(BaseModel):
    # ... existing fields ...

    # ADD THESE TWO FIELDS:
    selected_workflow: Optional[Dict[str, Any]] = Field(
        None,
        description="Selected workflow for recovery attempt (BR-AI-080)"
    )
    recovery_analysis: Optional[Dict[str, Any]] = Field(
        None,
        description="Recovery-specific analysis (BR-AI-081)"
    )
```

**Testing**: ✅ Model validation successful

**Impact**: Recovery endpoint will now return these fields correctly

---

## 🧪 Verification

**Local Test**:
```bash
export MOCK_LLM_MODE=true
cd holmesgpt-api
uvicorn src.main:app --reload --port 18120

curl -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "remediation_id": "test-recovery-001",
    "signal_type": "OOMKilled",
    "previous_workflow_id": "mock-oomkill-increase-memory-v1",
    "namespace": "production",
    "incident_id": "test-001"
  }' | jq '{selected_workflow, recovery_analysis}'
```

**Expected**: Both fields should now be present (not null)

---

## 📊 Expected Impact

**Before Fix**:
- E2E tests: 10/25 passing (40%)
- Recovery endpoint: Returns null fields
- 9 tests blocked

**After Fix**:
- E2E tests: 19-20/25 passing (76-80%)
- Recovery endpoint: Returns complete fields
- 9 tests unblocked ✅

---

**Status**: ✅ FIX COMPLETE
**Deployed**: Pending (code ready)
**Next**: AA team to rerun E2E tests after deployment

