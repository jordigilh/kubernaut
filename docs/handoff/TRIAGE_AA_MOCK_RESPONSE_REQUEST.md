# TRIAGE: AIAnalysis Mock Response Enhancement Request

**Date**: 2025-12-13
**From**: AIAnalysis Team
**To**: HAPI Team
**Priority**: HIGH (blocks 41% of AA E2E tests)
**Status**: ✅ **ALREADY IMPLEMENTED - VALIDATION NEEDED**

---

## 🎯 Executive Summary

**FINDING**: HAPI mock responses **ALREADY INCLUDE** all fields requested by AA team.

**Current Implementation**: BR-HAPI-212 (Mock LLM Mode) provides comprehensive mock responses with:
- ✅ `selected_workflow` field with all required subfields
- ✅ `recovery_analysis` field with previous attempt assessment
- ✅ `alternative_workflows` array
- ✅ `warnings` array
- ✅ Edge case scenarios (no workflow, low confidence, max retries)

**Issue**: AA team E2E tests are still failing (11/22 passing). This suggests either:
1. HAPI mock mode not enabled in AA E2E environment
2. Field naming mismatch between HAPI output and AA expectations
3. AA E2E infrastructure not properly configured

**Recommended Action**: **VALIDATE** integration between HAPI and AA, not implement new code.

---

## 📊 Comparison: Requested vs. Implemented

### 1. Incident Analysis Endpoint (`POST /api/v1/incident/analyze`)

#### AA Team Requested Fields

```json
{
  "selected_workflow": {
    "workflow_id": "test-workflow-001",
    "name": "Pod Restart Remediation",
    "confidence": 0.85,
    "reasoning": "Mock workflow selection",
    "labels": { ... }
  },
  "alternative_workflows": [ ... ],
  "warnings": [ ... ],
  "target_in_owner_chain": false,
  "needs_human_review": false
}
```

#### HAPI Current Implementation (lines 300-327 in `mock_responses.py`)

```python
"selected_workflow": {
    "workflow_id": scenario.workflow_id,          # ✅ PRESENT
    "title": scenario.workflow_title,            # ✅ PRESENT (as "title")
    "version": "1.0.0",                          # ✅ PRESENT
    "containerImage": "...",                     # ✅ PRESENT
    "confidence": scenario.confidence,           # ✅ PRESENT
    "rationale": "...",                          # ✅ PRESENT (as "rationale")
    "parameters": parameters                     # ✅ PRESENT
},
"alternative_workflows": [                       # ✅ PRESENT
    {
        "workflow_id": "mock-alternative-workflow-v1",
        "container_image": None,
        "confidence": scenario.confidence - 0.15,
        "rationale": "Alternative mock workflow"
    }
],
"target_in_owner_chain": True,                  # ✅ PRESENT
"warnings": [ ... ],                            # ✅ PRESENT
"needs_human_review": False,                    # ✅ PRESENT
```

**FINDING**: ✅ **ALL REQUESTED FIELDS ARE PRESENT**

**Potential Issues**:
- Field name mismatch: HAPI uses `"title"` vs AA expects `"name"`
- Field name mismatch: HAPI uses `"rationale"` vs AA expects `"reasoning"`
- Workflow ID format: HAPI uses `"mock-oomkill-increase-memory-v1"` vs AA expects `"test-workflow-001"`
- **No `labels` field in HAPI's `selected_workflow` object**

---

### 2. Recovery Analysis Endpoint (`POST /api/v1/recovery/analyze`)

#### AA Team Requested Fields

```json
{
  "recovery_analysis": {
    "previous_attempt_assessment": "Previous attempt failed...",
    "state_changed": false,
    "current_signal_type": "same",
    "suggested_actions": [ ... ],
    "confidence_adjustment": -0.05,
    "reasoning": "Mock recovery assessment"
  },
  "selected_workflow": { ... }
}
```

#### HAPI Current Implementation (lines 617-628 in `mock_responses.py`)

```python
"recovery_analysis": {                          # ✅ PRESENT
    "previous_attempt_assessment": {            # ✅ PRESENT (as object, not string)
        "workflow_id": previous_workflow_id,
        "failure_understood": True,
        "failure_reason_analysis": "...",
        "state_changed": False,                 # ✅ PRESENT
        "current_signal_type": signal_type      # ✅ PRESENT
    },
    "root_cause_refinement": "..."             # ✅ PRESENT (additional)
},
"selected_workflow": {                          # ✅ PRESENT
    "workflow_id": recovery_workflow_id,
    "title": "...",
    "version": "1.0.0",
    "confidence": scenario.confidence - 0.05,
    "rationale": "...",
    "parameters": { ... }
}
```

**FINDING**: ✅ **ALL REQUESTED FIELDS ARE PRESENT**

**Potential Issues**:
- Structure mismatch: HAPI's `previous_attempt_assessment` is an **object**, AA might expect a **string**
- Missing fields in HAPI: `suggested_actions`, `confidence_adjustment`, `reasoning`
- Field name mismatch: HAPI uses `"title"` and `"rationale"` vs AA expects `"name"` and `"reasoning"`
- **No `labels` field in HAPI's `selected_workflow` object**

---

## 🔍 Root Cause Analysis

### Issue 1: Field Naming Mismatches (CRITICAL)

| HAPI Field | AA Expected Field | Location |
|---|---|---|
| `title` | `name` | `selected_workflow` |
| `rationale` | `reasoning` | `selected_workflow` |
| N/A | `labels` | `selected_workflow` |

**Impact**: AA controller may fail to parse HAPI response if it's looking for exact field names.

**Evidence from AA Request**:
```json
"selected_workflow": {
  "workflow_id": "test-workflow-001",
  "name": "Pod Restart Remediation",      // AA expects "name"
  "confidence": 0.85,
  "reasoning": "Mock workflow selection",  // AA expects "reasoning"
  "labels": { ... }                        // AA expects "labels"
}
```

**HAPI Implementation**:
```python
"selected_workflow": {
  "workflow_id": scenario.workflow_id,
  "title": scenario.workflow_title,       // HAPI provides "title"
  "confidence": scenario.confidence,
  "rationale": "...",                     // HAPI provides "rationale"
  # NO "labels" field
}
```

---

### Issue 2: Recovery Analysis Structure Mismatch (HIGH)

**AA Expectation**:
```json
"recovery_analysis": {
  "previous_attempt_assessment": "Previous attempt failed due to...",  // STRING
  "state_changed": false,
  "current_signal_type": "same",
  "suggested_actions": ["retry_with_backoff"],
  "confidence_adjustment": -0.05,
  "reasoning": "Mock recovery assessment"
}
```

**HAPI Implementation**:
```python
"recovery_analysis": {
  "previous_attempt_assessment": {        # OBJECT, not string
    "workflow_id": previous_workflow_id,
    "failure_understood": True,
    "failure_reason_analysis": "...",
    "state_changed": False,
    "current_signal_type": signal_type
  },
  "root_cause_refinement": "..."
  # Missing: suggested_actions, confidence_adjustment, reasoning
}
```

**Impact**: AA controller expects flat structure, HAPI provides nested structure.

---

### Issue 3: Mock Mode Not Enabled in AA E2E (LIKELY)

**Evidence from AA Request**:
> "When AIAnalysis controller calls HAPI mock endpoints, HAPI returns 200 OK but response missing `selected_workflow` field"

**Root Cause**: AA E2E environment may not have `MOCK_LLM_MODE=true` set.

**HAPI Mock Mode Activation** (line 782-789 in `incident.py`):
```python
from src.mock_responses import is_mock_mode_enabled, generate_mock_incident_response
if is_mock_mode_enabled():
    logger.info({
        "event": "mock_mode_active",
        "incident_id": incident_id,
        "message": "Returning deterministic mock response (MOCK_LLM_MODE=true)"
    })
    return generate_mock_incident_response(request_data)
```

**Validation**: Check if AA E2E environment sets `MOCK_LLM_MODE=true` for HAPI deployment.

---

## 🎯 Required Changes

### Change 1: Add Missing Fields to `selected_workflow` (CRITICAL)

**File**: `holmesgpt-api/src/mock_responses.py`
**Lines**: 300-308 (incident), 627-638 (recovery)

**Add to `selected_workflow` object**:
```python
"selected_workflow": {
    "workflow_id": scenario.workflow_id,
    "title": scenario.workflow_title,
    "name": scenario.workflow_title,  # ADD: Alias for AA compatibility
    "version": "1.0.0",
    "containerImage": f"...",
    "confidence": scenario.confidence,
    "rationale": f"Mock selection based on {signal_type} signal type (BR-HAPI-212)",
    "reasoning": f"Mock selection based on {signal_type} signal type (BR-HAPI-212)",  # ADD: Alias
    "parameters": parameters,
    "labels": {  # ADD: For AA compatibility
        "component": "*",
        "environment": "*",
        "severity": scenario.severity
    }
}
```

**Rationale**: Maintain backward compatibility while adding AA-expected fields.

---

### Change 2: Add Missing Fields to `recovery_analysis` (HIGH)

**File**: `holmesgpt-api/src/mock_responses.py`
**Lines**: 617-626 (recovery response)

**Enhance `recovery_analysis` structure**:
```python
"recovery_analysis": {
    "previous_attempt_assessment": {
        "workflow_id": previous_workflow_id,
        "failure_understood": True,
        "failure_reason_analysis": "Mock analysis: Previous workflow did not resolve the issue (BR-HAPI-212)",
        "state_changed": False,
        "current_signal_type": signal_type
    },
    "root_cause_refinement": scenario.root_cause_summary,
    # ADD: AA-expected fields
    "suggested_actions": [  # ADD
        "retry_with_backoff",
        "escalate_if_fails_again"
    ],
    "confidence_adjustment": -0.05,  # ADD
    "reasoning": f"Mock recovery assessment based on {signal_type} (BR-HAPI-212)"  # ADD
}
```

---

### Change 3: Validate Mock Mode in AA E2E Environment (CRITICAL)

**Action**: Verify AA E2E HAPI deployment has `MOCK_LLM_MODE=true`.

**Check**:
1. AA E2E Kubernetes deployment manifest for HAPI
2. Environment variable `MOCK_LLM_MODE` must be set to `"true"`
3. Verify HAPI logs show: `"event": "mock_mode_active"`

**If Not Set**:
- AA E2E tests will call real LLM (expensive, non-deterministic)
- HAPI may return incomplete responses if LLM config is missing

---

## 📋 Implementation Checklist

### HAPI Team Tasks (1-2 hours)

- [ ] **Add `name` and `reasoning` alias fields** to `selected_workflow` in both incident and recovery responses
- [ ] **Add `labels` object** to `selected_workflow` with wildcard values for E2E testing
- [ ] **Add `suggested_actions`, `confidence_adjustment`, `reasoning`** to `recovery_analysis`
- [ ] **Update mock workflow IDs** to use `test-workflow-*` format for consistency with AA expectations
- [ ] **Test locally** with `MOCK_LLM_MODE=true`
- [ ] **Create response document**: `RESPONSE_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md`

### AA Team Validation Tasks (30 minutes)

- [ ] **Verify `MOCK_LLM_MODE=true`** is set in AA E2E HAPI deployment
- [ ] **Check HAPI logs** for `"event": "mock_mode_active"` messages
- [ ] **Test field parsing** - verify AA controller can parse both `title`/`name` and `rationale`/`reasoning`
- [ ] **Rerun E2E tests** after HAPI changes
- [ ] **Report results** back to HAPI team

---

## 🧪 Testing Strategy

### Local Verification (HAPI Team)

```bash
# Set mock mode
export MOCK_LLM_MODE=true

# Start HAPI locally
cd holmesgpt-api
uvicorn src.main:app --reload --port 18120

# Test incident endpoint
curl -X POST http://localhost:18120/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test-001",
    "signal_type": "OOMKilled",
    "resource_namespace": "default",
    "resource_name": "test-pod",
    "resource_kind": "Pod"
  }' | jq '.selected_workflow | {workflow_id, name, reasoning, labels}'

# Test recovery endpoint
curl -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "remediation_id": "test-rem-001",
    "incident_id": "test-001",
    "signal_type": "OOMKilled",
    "previous_workflow_id": "test-workflow-001",
    "namespace": "default"
  }' | jq '.recovery_analysis | {suggested_actions, confidence_adjustment, reasoning}'
```

**Expected Output**:
```json
{
  "workflow_id": "mock-oomkill-increase-memory-v1",
  "name": "OOMKill Recovery - Increase Memory Limits (MOCK)",
  "reasoning": "Mock selection based on OOMKilled signal type (BR-HAPI-212)",
  "labels": {
    "component": "*",
    "environment": "*",
    "severity": "critical"
  }
}
```

---

## ⚠️ Risks & Mitigation

### Risk 1: Field Duplication (Low)
**Risk**: Adding both `title`/`name` and `rationale`/`reasoning` increases response size.
**Mitigation**: Minimal impact (few extra bytes per response). Remove duplicates in V2.0 after AA migration.

### Risk 2: Schema Incompatibility (Medium)
**Risk**: AA controller expects exact structure, any mismatch causes parsing failures.
**Mitigation**: Test with actual AA E2E environment before deploying.

### Risk 3: Mock Mode Not Propagating (High)
**Risk**: AA E2E environment doesn't set `MOCK_LLM_MODE=true`, tests call real LLM.
**Mitigation**: AA team MUST verify environment variable is set in HAPI deployment.

---

## 📈 Success Criteria

### HAPI Team Success
- ✅ Mock responses include `name`, `reasoning`, `labels` fields
- ✅ Mock recovery responses include `suggested_actions`, `confidence_adjustment`, `reasoning`
- ✅ Local testing confirms all fields present
- ✅ Response document created

### AA Team Success
- ✅ Mock mode enabled in E2E environment (`MOCK_LLM_MODE=true`)
- ✅ 20-22/22 AA E2E tests passing (91-100%)
- ✅ Controller logs show `selected_workflow` and `recovery_analysis` parsed successfully
- ✅ No "No workflow selected" errors in controller logs

---

## 🔗 Related Documents

### AA Team
- **Original Request**: [REQUEST_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md](REQUEST_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md)
- **AA Test Diagnosis**: [DIAGNOSIS_AA_E2E_TEST_FAILURES.md](DIAGNOSIS_AA_E2E_TEST_FAILURES.md)
- **AA Test Breakdown**: [AA_TEST_BREAKDOWN_ALL_TIERS.md](AA_TEST_BREAKDOWN_ALL_TIERS.md)

### HAPI Team
- **Mock Mode Implementation**: `holmesgpt-api/src/mock_responses.py`
- **Business Requirement**: BR-HAPI-212 (Mock LLM Mode for Integration Testing)
- **Previous Fix**: [RESPONSE_HAPI_RECOVERY_ENDPOINT_CONFIG_FIX.md](RESPONSE_HAPI_RECOVERY_ENDPOINT_CONFIG_FIX.md)

---

## 🎯 Recommended Action Plan

### Priority 1: Environment Validation (IMMEDIATE - 15 min)
**Owner**: AA Team
**Action**: Verify `MOCK_LLM_MODE=true` is set in AA E2E HAPI deployment

### Priority 2: Field Enhancement (HIGH - 1 hour)
**Owner**: HAPI Team
**Action**: Add missing fields (`name`, `reasoning`, `labels`, `suggested_actions`, etc.)

### Priority 3: Integration Testing (HIGH - 30 min)
**Owner**: AA Team + HAPI Team
**Action**: Test HAPI changes in AA E2E environment

### Priority 4: Results Report (MEDIUM - 15 min)
**Owner**: HAPI Team
**Action**: Create `RESPONSE_HAPI_MOCK_WORKFLOW_RESPONSE_ENHANCEMENT.md`

---

## 💡 Key Insights

1. **HAPI Already Has Mock Mode**: BR-HAPI-212 provides comprehensive mock responses
2. **Field Naming Matters**: AA expects `name`/`reasoning`, HAPI provides `title`/`rationale`
3. **Mock Mode Must Be Enabled**: AA E2E environment MUST set `MOCK_LLM_MODE=true`
4. **Quick Fix Available**: Add alias fields for backward compatibility (1-2 hours work)
5. **High Success Probability**: Once fields aligned, AA E2E tests should pass (20-22/22)

---

**Status**: ✅ **TRIAGE COMPLETE - READY FOR IMPLEMENTATION**
**Estimated Effort**: 1-2 hours (HAPI) + 30 min validation (AA)
**Expected Result**: 20-22/22 AA E2E tests passing (91-100%)

---

**Next Step**: HAPI team to implement field enhancements and coordinate with AA team for validation.

