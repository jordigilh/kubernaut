# HAPI OpenAPI Client Missing Recovery Fields: Action Plan

**Date**: December 30, 2025
**Priority**: P2 - E2E Test Blocker
**Business Requirement**: BR-HAPI-197 (Human review flags for uncertain AI decisions)
**Affected Components**: HAPI Python OpenAPI Client, HAPI E2E Tests

---

## 🚨 **Problem Summary**

The HAPI Python OpenAPI client (`holmesgpt_api_client`) is missing `needs_human_review` and `human_review_reason` fields from `RecoveryResponse`, causing E2E tests to fail with `KeyError`.

**Current State**:
- ✅ Python Pydantic model HAS the fields (`src/models/recovery_models.py`)
- ✅ Mock responses RETURN the fields (`src/mock_responses.py`)
- ❌ OpenAPI spec does NOT include the fields (or is out of date)
- ❌ Generated OpenAPI client CANNOT access the fields

**Impact**:
- E2E test failure: `test_signal_not_reproducible_returns_no_recovery`
- 4 recovery edge case tests cannot run to completion (pytest `-x` stops on first failure)
- Cannot validate BR-HAPI-197 compliance for recovery scenarios

---

## 📊 **Evidence**

### **Pydantic Model** (Source of Truth)
```python
# File: holmesgpt-api/src/models/recovery_models.py:213-257
class RecoveryResponse(BaseModel):
    incident_id: str
    can_recover: bool
    strategies: List[RecoveryStrategy]
    analysis_confidence: float
    warnings: List[str]
    selected_workflow: Optional[Dict[str, Any]]
    recovery_analysis: Optional[Dict[str, Any]]

    # ✅ THESE FIELDS EXIST (BR-HAPI-197)
    needs_human_review: bool = Field(default=False)
    human_review_reason: Optional[str] = Field(default=None)
```

### **Generated OpenAPI Client** (Out of Sync)
```python
# File: holmesgpt-api/tests/clients/holmesgpt_api_client/models/recovery_response.py:27-40
class RecoveryResponse(BaseModel):
    incident_id: StrictStr
    can_recover: StrictBool
    strategies: Optional[List[RecoveryStrategy]]
    analysis_confidence: float
    warnings: Optional[List[StrictStr]]
    selected_workflow: Optional[Dict[str, Any]]
    recovery_analysis: Optional[Dict[str, Any]]

    # ❌ MISSING FIELDS
    # needs_human_review: ???
    # human_review_reason: ???

    __properties: ClassVar[List[str]] = [
        "incident_id", "can_recover", "strategies",
        "primary_recommendation", "analysis_confidence",
        "warnings", "metadata", "selected_workflow",
        "recovery_analysis"
        # Missing: "needs_human_review", "human_review_reason"
    ]
```

### **Mock Responses** (Returns Fields)
```python
# File: holmesgpt-api/src/mock_responses.py:730-731
return {
    "can_recover": False,
    "needs_human_review": False,  # ✅ Field is returned
    "human_review_reason": None,  # ✅ Field is returned
    # ... other fields
}
```

### **E2E Test Failure**
```python
# File: holmesgpt-api/tests/e2e/test_mock_llm_edge_cases_e2e.py:255
data = response.model_dump()  # OpenAPI client model
assert data["needs_human_review"] is False  # ❌ KeyError!
```

---

## 🔧 **Root Cause**

The issue is in the **OpenAPI spec generation** process. FastAPI generates the OpenAPI spec from Pydantic models, but the generated spec is either:
1. Missing the fields (FastAPI not picking them up)
2. Out of date (client not regenerated after fields were added)

**Why Fields Might Be Missing from Spec**:
- Pydantic model uses `Field(default=False)` - FastAPI might not include default fields in spec
- OpenAPI spec might have been cached or not regenerated
- Client generation script might be using an old spec file

---

## ✅ **Solution**

### **Option 1: Regenerate HAPI OpenAPI Client (Recommended)**

**Step 1**: Verify OpenAPI Spec Includes Fields
```bash
cd holmesgpt-api
python3 -c "
from src.main import app
import json
spec = app.openapi()
recovery_response = spec['components']['schemas']['RecoveryResponse']
print('needs_human_review' in recovery_response['properties'])
print('human_review_reason' in recovery_response['properties'])
"
```

Expected output: `True` for both

If output is `False`, the Pydantic model fields are not being picked up by FastAPI. This requires:
- Adding `response_model_exclude_unset=False` to endpoint decorators
- Or explicitly including fields in OpenAPI schema

**Step 2**: Regenerate OpenAPI Client
```bash
cd holmesgpt-api/tests/integration
bash generate-client.sh
```

This script:
1. Starts HAPI service locally
2. Downloads `/openapi.json` from running service
3. Runs `openapi-generator-cli` to generate Python client
4. Installs generated client

**Step 3**: Verify Fields in Generated Client
```bash
grep -A 5 "class RecoveryResponse" \
  holmesgpt-api/tests/clients/holmesgpt_api_client/models/recovery_response.py \
  | grep "needs_human_review"
```

Expected output: Line showing `needs_human_review` field definition

---

### **Option 2: Update E2E Tests to Handle Missing Fields (Temporary)**

If client regeneration is blocked or fails, update tests to gracefully handle missing fields:

```python
# File: holmesgpt-api/tests/e2e/test_mock_llm_edge_cases_e2e.py
def test_signal_not_reproducible_returns_no_recovery(self, hapi_recovery_api):
    # ... test setup ...

    data = response.model_dump()

    # Key assertion: can_recover=false means no action needed
    assert data["can_recover"] is False

    # Check needs_human_review if field exists (BR-HAPI-197)
    # Field might be missing if OpenAPI client is out of date
    if "needs_human_review" in data:
        assert data["needs_human_review"] is False, "No review needed when issue resolved"
    else:
        # Log warning but don't fail test
        import logging
        logging.warning("needs_human_review field missing from RecoveryResponse - client out of date")

    assert data["selected_workflow"] is None
    # ... rest of test ...
```

**⚠️ This is a temporary workaround** - the real fix is Option 1.

---

## 🎯 **Questions to Answer Before Fixing**

### **Q1: When were `needs_human_review` fields added to Pydantic model?**
**Answer needed from**: Git history of `holmesgpt-api/src/models/recovery_models.py`

```bash
git log --oneline --all -- holmesgpt-api/src/models/recovery_models.py | grep -i "needs_human_review\|BR-HAPI-197"
```

### **Q2: When was OpenAPI client last regenerated?**
**Answer needed from**: Git history of client files

```bash
git log -1 --oneline -- holmesgpt-api/tests/clients/holmesgpt_api_client/models/recovery_response.py
```

### **Q3: Does FastAPI OpenAPI spec include the fields?**
**Test**: Start HAPI locally and check `/openapi.json`

```bash
cd holmesgpt-api
python3 -m uvicorn src.main:app --port 18120 &
sleep 5
curl http://localhost:18120/openapi.json | jq '.components.schemas.RecoveryResponse.properties | keys'
```

Expected output: Array including `"needs_human_review"` and `"human_review_reason"`

---

## 📋 **Implementation Checklist**

### **Investigation Phase** (15 min)
- [ ] Check git history for when fields were added to Pydantic model
- [ ] Check git history for last client regeneration
- [ ] Verify OpenAPI spec includes fields (Q3 above)
- [ ] Document findings in this file

### **Fix Phase** (30 min - Option 1)
- [ ] Regenerate HAPI OpenAPI client: `cd holmesgpt-api/tests/integration && bash generate-client.sh`
- [ ] Verify fields in generated client (grep check above)
- [ ] Re-run E2E tests: `make test-e2e-holmesgpt-api`
- [ ] Verify all 8 recovery tests pass

### **Fix Phase** (15 min - Option 2, if Option 1 fails)
- [ ] Update test to handle missing fields gracefully
- [ ] Add logging/warning for missing fields
- [ ] Create follow-up ticket for Option 1
- [ ] Re-run E2E tests to verify workaround

### **Validation Phase** (10 min)
- [ ] Run full E2E suite: `make test-e2e-holmesgpt-api`
- [ ] Verify 62 tests pass (currently 7 pass, 1 fail, 54 not run)
- [ ] Document any remaining issues

---

## 🔗 **Related Issues**

### **Parallel Issue: Go OpenAPI Client**
The Go HAPI OpenAPI client (`pkg/holmesgpt/client/`) has the same issue - missing `needs_human_review` fields in `RecoveryResponse`.

**See**: `AA_RECOVERY_NEEDS_HUMAN_REVIEW_MISSING_DEC_30_2025.md`

**Impact**: AIAnalysis controller cannot check `needs_human_review` for recovery responses.

---

## 💡 **Recommendation**

**Proceed with Option 1** (Regenerate client):
1. Fastest path to fix (30 min vs 1.5 hr for implementing in HAPI business logic)
2. Unblocks E2E tests immediately
3. Validates that OpenAPI spec is correct
4. Both Go and Python clients need regeneration anyway

**If Option 1 fails** (OpenAPI spec doesn't include fields):
1. Fall back to Option 2 (temporary workaround)
2. Investigate why FastAPI isn't including fields in spec
3. May require FastAPI configuration changes or explicit schema definition

---

## 📞 **Next Steps**

1. **Answer Q1-Q3** above to understand root cause
2. **Choose** Option 1 or Option 2 based on investigation
3. **Execute** chosen fix
4. **Verify** all E2E tests pass
5. **Coordinate** with AA team for Go client regeneration (parallel issue)

---

**End of Document**


