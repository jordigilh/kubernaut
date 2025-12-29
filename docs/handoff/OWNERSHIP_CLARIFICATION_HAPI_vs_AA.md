# OWNERSHIP CLARIFICATION: HAPI vs AIAnalysis

**Date**: 2025-12-13
**Question**: "Is the HAPI team responsible or the AA team?"
**Answer**: 🔍 **NEEDS LOCAL REPRODUCTION TO CONFIRM**

---

## 🎯 **Responsibility Determination**

### **Rule of Thumb**
- **HAPI code bug** (`holmesgpt-api/` directory) → HAPI team/developer
- **AA code bug** (`pkg/aianalysis/`, `test/e2e/aianalysis/`) → AA team/developer

### **Current Project Structure**
Both services are in the same monorepo:
- `holmesgpt-api/` - Python FastAPI service
- `pkg/aianalysis/` - Go controller
- **Author**: Jordi Gil (both services)
- **Team**: Appears to be same team/person

---

## 🔍 **Current Evidence**

### **What We Know** ✅

1. ✅ **Mock mode IS active** - Warnings confirm it:
   ```json
   {
     "warnings": [
       "MOCK_MODE: This response is deterministic for integration testing (BR-HAPI-212)",
       "MOCK_MODE: No LLM was called - response based on signal_type matching"
     ]
   }
   ```

2. ✅ **Mock response generator populates fields** - Code confirmed (lines 617, 627 of mock_responses.py):
   ```python
   response = {
       ...
       "recovery_analysis": {...},   # Line 617-626
       "selected_workflow": {...}    # Line 627-638
   }
   ```

3. ❌ **But HTTP response has null fields**:
   ```json
   {
     "selected_workflow": null,
     "recovery_analysis": null
   }
   ```

---

## 🤔 **Possible Root Causes**

### **Option A: Request Format Issue** (Likely - AA team responsibility)

**Hypothesis**: The E2E tests are sending the wrong request format, causing `generate_mock_recovery_response()` to take a different code path.

**Evidence Needed**:
- What EXACT request format are the E2E tests sending?
- Does it match the expected format in `mock_responses.py`?

**Test**:
```bash
# Our diagnostic curl command:
curl -X POST http://holmesgpt-api:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "remediation_id": "recovery-diag-001",
    "signal_type": "OOMKilled",
    "previous_workflow_id": "mock-oomkill-increase-memory-v1",
    "namespace": "production",
    "incident_id": "test-001"
  }'
```

**Responsibility**: If request format is wrong → **AA E2E test code needs fixing**

---

### **Option B: Pydantic Model Serialization Bug** (Less Likely - HAPI team responsibility)

**Hypothesis**: FastAPI/Pydantic is stripping the fields during response serialization.

**Evidence Needed**:
- Does the mock response dict have the fields right before FastAPI serializes it?
- Is there a Pydantic validation error silently failing?

**Test**:
```python
# Add debug logging in recovery.py analyze_recovery():
result = generate_mock_recovery_response(request_data)
logger.info(f"DEBUG: Mock response has selected_workflow: {result.get('selected_workflow') is not None}")
logger.info(f"DEBUG: Mock response has recovery_analysis: {result.get('recovery_analysis') is not None}")
return result
```

**Responsibility**: If Pydantic is stripping fields → **HAPI service code needs fixing**

---

### **Option C: Edge Case Code Path** (Most Likely - Depends on cause)

**Hypothesis**: The request triggers an edge case handler that doesn't populate fields.

**Evidence From Code**:
```python
# generate_mock_recovery_response() has edge cases:
if signal_type and signal_type.upper() == EDGE_CASE_NOT_REPRODUCIBLE:
    return _generate_not_reproducible_recovery_response(...)  # Has fields

if signal_type and signal_type.upper() == EDGE_CASE_NO_WORKFLOW:
    return _generate_no_recovery_workflow_response(...)  # selected_workflow=None!

if signal_type and signal_type.upper() == EDGE_CASE_LOW_CONFIDENCE:
    return _generate_low_confidence_recovery_response(...)  # Has fields
```

**Key Finding**: `_generate_no_recovery_workflow_response()` DOES set `selected_workflow=None`!

**Check**: What is `EDGE_CASE_NO_WORKFLOW` defined as?

**Responsibility**:
- If E2E tests accidentally trigger edge case → **AA test needs fixing**
- If edge case should populate fields → **HAPI mock response needs fixing**

---

## 🔧 **REQUIRED ACTION: Local Reproduction**

**You need to**:

1. **Start HAPI locally with mock mode**:
   ```bash
   cd holmesgpt-api
   export MOCK_LLM_MODE=true
   export DATASTORAGE_URL=http://localhost:8080  # Or mock it
   export LOG_LEVEL=DEBUG
   uvicorn src.main:app --reload --port 18120
   ```

2. **Test with EXACT E2E request format**:
   ```bash
   # Find the exact request the E2E tests send
   grep -A20 "POST.*recovery/analyze" test/e2e/aianalysis/*.go

   # Send that exact request
   curl -X POST http://localhost:18120/api/v1/recovery/analyze \
     -H "Content-Type: application/json" \
     -d '{...exact request from E2E test...}' | jq '.'
   ```

3. **Check HAPI logs**:
   ```bash
   # Look for:
   - "event": "mock_mode_active"  # Should appear
   - "event": "mock_recovery_response_generated"  # Should appear
   - Any edge case handling messages
   - DEBUG logs showing field presence
   ```

4. **Verify response**:
   ```bash
   # Does selected_workflow appear?
   # Does recovery_analysis appear?
   # If not, which code path was taken?
   ```

---

## 📊 **Decision Tree**

```
Are mock mode warnings present in response?
├─ YES → Mock mode IS working
│   │
│   └─ Are selected_workflow/recovery_analysis null?
│       ├─ YES → One of three issues:
│       │   ├─ Wrong request format (AA team fixes E2E test)
│       │   ├─ Edge case triggered (Check if intentional)
│       │   └─ Pydantic serialization bug (HAPI team fixes)
│       │
│       └─ NO → Problem solved! ✅
│
└─ NO → Mock mode NOT working
    └─ Check MOCK_LLM_MODE environment variable (Infrastructure issue)
```

---

## 🎯 **Current Status**

**Blocker**: We diagnosed the issue FROM the E2E cluster but haven't done LOCAL REPRODUCTION.

**Next Step**: Run HAPI locally and test with the EXACT request format that E2E tests send.

**Expected Outcome**:
- **If local test works** → E2E cluster configuration issue (infrastructure)
- **If local test fails** → Reproducible bug to fix (HAPI or AA, depends on root cause)

---

## 💡 **Recommendation**

**DO THIS FIRST** (5 minutes):

```bash
cd holmesgpt-api

# Check what edge cases are defined
grep "EDGE_CASE" src/mock_responses.py

# Check if any edge case sets fields to None
grep -A10 "_generate.*recovery.*response" src/mock_responses.py | grep "selected_workflow.*None"
```

**If you find an edge case that sets fields to None**, then check:
- What trigger value activates it?
- Does the E2E test accidentally send that value?

---

## 📝 **Answer to Your Question**

**"Is the HAPI team responsible or the AA team?"**

**Answer**: **BOTH need to check their code**, but in this order:

1. **AA team** (15 min): Check if E2E tests send correct request format
2. **HAPI team** (15 min): Reproduce locally with E2E's exact request
3. **Whoever finds the bug** (30 min): Fix it

**Most Likely**: AA E2E tests are either:
- Sending wrong request format
- Accidentally triggering an edge case
- Missing a required field that causes mock generator to skip field population

**Recommendation**: Start with AA E2E test code review, then reproduce locally.

---

**Created**: 2025-12-13
**By**: AIAnalysis Team (with critical input from user)
**Status**: 🔄 NEEDS LOCAL REPRODUCTION
**Priority**: 🚨 CRITICAL (Blocks 9 E2E tests)

---

**TL;DR**: Mock mode works, but fields come back null. Need to check if E2E tests are sending the right request format, or if there's a Pydantic serialization bug. Local reproduction required to determine responsibility.


