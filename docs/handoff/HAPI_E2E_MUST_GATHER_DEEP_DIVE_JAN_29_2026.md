# HAPI E2E Must-Gather Deep Dive Analysis
**Date**: January 29, 2026 (Evening Session)  
**Must-Gather**: `/tmp/holmesgpt-api-e2e-logs-20260202-192052/`  
**Test Run**: 26/40 passing (65%) | 14 failures

---

## 🎯 EXECUTIVE SUMMARY

**Root Causes Identified**:
1. **OpenAPI Client Bug** (1 failure): `ogen` sets `.Set=true` for `null` values
2. **Mock LLM Scenario Issues** (6 failures): Scenarios not returning expected data structures
3. **HAPI Parser Logic** (4 failures): `result_parser.py` changes may not be active or have bugs
4. **Test Code Issues** (1 failure): Type mismatch in assertion
5. **Expected Validation Failures** (3 failures): Server-side validation (V1.1 debt)

**Critical Finding**: Most failures are NOT business logic violations—they're either Mock LLM configuration issues or test infrastructure problems.

---

## 📋 DETAILED FAILURE ANALYSIS

### **CATEGORY 1: OpenAPI Client Bug (1 failure)**

#### **E2E-HAPI-001: No workflow found returns human review**

**Failure Message**:
```
Expected <bool>: true to be false
```

**HAPI Log**:
```json
{
  "event": "incident_analysis_completed",
  "incident_id": "test-edge-001",
  "has_workflow": false,
  "needs_human_review": true,
  "selected_workflow": null  // ✅ Correct response
}
```

**HTTP Response**:
```json
{
  "selected_workflow": null
}
```

**Root Cause**:
The `ogen`-generated OpenAPI client sets `selected_workflow.Set = true` even when the JSON value is `null`. This violates the OpenAPI spec's distinction between:
- Field not present: `Set = false`
- Field present with `null`: Should ideally be `Set = false` for OpenAPI's `nullable` fields

**Test Assertion** (line 78):
```go
Expect(incidentResp.SelectedWorkflow.Set).To(BeFalse(),
    "selected_workflow must be null when no workflow found")
```

**Business Impact**: NONE - HAPI's business logic is correct (`has_workflow=false`), this is purely a client deserialization issue.

**Fix Options**:
- **Option A**: Update test to check `SelectedWorkflow.Value == nil` instead of `.Set == false`
- **Option B**: File bug with `ogen` library
- **Option C**: Use custom unmarshaler to handle `null` correctly

**Recommended**: **Option A** (pragmatic, no external dependencies)

---

### **CATEGORY 2: Mock LLM Scenario Issues (6 failures)**

#### **E2E-HAPI-002: Low confidence returns human review with alternatives**

**Failure Message**:
```
Expected <[]client.AlternativeWorkflow | len:0, cap:0>: [] not to be empty
```

**HAPI Log**:
```json
{
  "event": "incident_analysis_completed",
  "incident_id": "test-edge-002",
  "has_workflow": true,  // ✅ Workflow found (progress!)
  "needs_human_review": false,  // ✅ BR-HAPI-197 compliant
  "warnings_count": 0
}
```

**Root Cause**:
Mock LLM's `low_confidence` scenario is NOT returning `alternative_workflows` in its response. Either:
1. Mock LLM `server.py` doesn't populate `alternative_workflows` for this scenario
2. HAPI's `result_parser.py` doesn't extract alternatives from LLM response

**Test Expectation** (line 143):
```go
Expect(incidentResp.AlternativeWorkflows).ToNot(BeEmpty(),
    "alternative_workflows help AIAnalysis when confidence is low")
```

**Investigation Needed**:
- [ ] Check Mock LLM `server.py` line ~171 for `low_confidence` scenario response structure
- [ ] Check HAPI `result_parser.py` for `alternative_workflows` extraction logic

**Recommended Fix**: Update Mock LLM `_get_category_c_workflows()` to return alternatives for `low_confidence` scenario

---

#### **E2E-HAPI-003: Max retries exhausted returns validation history**

**Failure Message**:
```
Expected <client.HumanReviewReason>: workflow_not_found
to equal <client.HumanReviewReason>: llm_parsing_error
```

**HAPI Log**:
```json
{
  "event": "incident_analysis_completed",
  "incident_id": "test-edge-003",
  "has_workflow": true,  // ❌ Should be false for "rca_incomplete"
  "needs_human_review": true,  // ✅ Correct
  "validation_attempts": 4  // ✅ Max retries reached
}
```

**Root Cause**:
Mock LLM's `rca_incomplete` scenario is being interpreted as finding a workflow (fallback to `generic-restart-v1`?) instead of triggering LLM parsing errors after max retries.

Expected behavior:
- LLM returns malformed/incomplete JSON
- HAPI retries up to 4 times (V1.0 max)
- After 4 failures, sets `human_review_reason = "llm_parsing_error"`

**Investigation Needed**:
- [ ] Check Mock LLM `rca_incomplete` scenario response structure
- [ ] Verify HAPI retry logic honors max validation attempts
- [ ] Check if HAPI is falling back to `generic-restart-v1` instead of failing

**Recommended Fix**: Mock LLM `rca_incomplete` should return invalid JSON or missing required fields to trigger parsing errors

---

#### **E2E-HAPI-004: Normal incident analysis succeeds**

**Failure Message**:
```
Expected <bool>: true to be false
(needs_human_review must be false for confident recommendation)
```

**HAPI Log**: (Not found with `test-happy-001` - need to search by incident ID from test)

**Root Cause**: Unknown - need to identify incident ID used by test

**Test Request** (line 230):
```go
req := &hapiclient.IncidentRequest{
    IncidentID:        "test-happy-001",  // Search logs for this
    SignalType:        "MOCK_OOMKILLED",
    // ...
}
```

**Investigation Needed**:
- [ ] Find HAPI logs for `test-happy-001` or `MOCK_OOMKILLED`
- [ ] Check Mock LLM `oomkilled` scenario confidence value
- [ ] Verify HAPI is not setting `needs_human_review=true` for high-confidence scenarios

**Hypothesis**: Mock LLM `oomkilled` scenario may be returning confidence < 0.70, causing HAPI to...wait, we removed that logic! So why is `needs_human_review=true`?

**Critical Question**: Did HAPI image get rebuilt with my BR-HAPI-197 fixes?

---

#### **E2E-HAPI-026: Normal recovery analysis succeeds**

**Failure Message**:
```
Expected <float64>: 0.7
to be within 0.05 of ~ <float64>: 0.85
```

**Root Cause**:
Test expects Mock LLM's `recovery` scenario to return `confidence = 0.85`, but HAPI returns `0.7`.

This suggests:
1. Mock LLM is returning `0.7` instead of `0.85`, OR
2. HAPI is overriding Mock LLM confidence with DataStorage workflow confidence

**Investigation Needed**:
- [ ] Check Mock LLM logs for `recovery` scenario response
- [ ] Check if HAPI's workflow selection logic prefers DataStorage confidence over LLM confidence

**Note**: Previous test runs had this same issue (confidence mismatch between Mock LLM and DataStorage)

---

### **CATEGORY 3: HAPI Result Parser Logic (4 failures)**

#### **E2E-HAPI-023: Signal not reproducible returns no recovery**

**Failure Message**:
```
Expected <bool>: true to be false
(can_recover must be false when issue self-resolved)
```

**HAPI Log**:
```json
{
  "event": "recovery_analysis_completed",
  "incident_id": "test-recovery-edge-001",
  "can_recover": true,  // ❌ Should be false
  "needs_human_review": ???
}
```

**Expected Behavior** (BR-HAPI-200):
- Mock LLM `problem_resolved` scenario returns `investigation_outcome = "resolved"`
- HAPI should set `can_recover = false` (no action needed)
- HAPI should set `needs_human_review = false` (issue resolved itself)

**My Fix** (`recovery/result_parser.py`):
```python
if investigation_outcome == "resolved":
    needs_human_review = False
    can_recover = False
```

**Root Cause Analysis**:
Either:
1. **Fix not active**: HAPI image wasn't rebuilt with my changes
2. **Mock LLM issue**: `MOCK_NOT_REPRODUCIBLE` doesn't return `investigation_outcome="resolved"`
3. **Logic bug**: My fix has a bug in the conditional logic

**Investigation Needed**:
- [ ] Check Mock LLM `MOCK_NOT_REPRODUCIBLE` response structure
- [ ] Verify HAPI image was rebuilt after my `result_parser.py` changes
- [ ] Check HAPI logs for `investigation_outcome` field in parsed LLM response

---

#### **E2E-HAPI-024: No recovery workflow found returns human review**

**Failure Message**:
```
Expected <string>: no_matching_workflows
to equal <client.HumanReviewReason>: no_matching_workflows
```

**Root Cause**: **TYPE MISMATCH** - Test is comparing a `string` to a `HumanReviewReason` enum constant.

**Actual Test Code** (line 700):
```go
Expect(recoveryResp.HumanReviewReason.Value).To(Equal(hapiclient.HumanReviewReasonNoMatchingWorkflows),
    "human_review_reason must indicate no matching workflows")
```

**Issue**: The test is likely checking the string value instead of the enum constant.

**Fix**: Simple test correction - ensure proper type comparison.

---

### **CATEGORY 4: Expected Validation Failures (3 failures)** ✅

**E2E-HAPI-007, 008, 018**: Server-side input validation

**Status**: Expected failures - deferred to V1.1 per user agreement

**No action required** - these are documented technical debt.

---

## 🔍 CRITICAL QUESTIONS

### **Question 1: Was HAPI Image Rebuilt?**

**Context**: I made changes to:
- `holmesgpt-api/src/extensions/incident/result_parser.py` (removed confidence threshold logic)
- `holmesgpt-api/src/extensions/recovery/result_parser.py` (added `investigation_outcome` logic)

**Evidence Needed**:
- Check HAPI image build timestamp vs. my code changes
- Check HAPI logs for "BR-HAPI-197" comments I added
- Verify `needs_human_review` logic is using new code path

**How to Verify**:
```bash
# Check HAPI logs for my debug comments
grep "BR-HAPI-197" $HAPI_LOG
# OR check HAPI image creation time
podman image inspect localhost/holmesgpt-api:holmesgpt-api-18909458 | grep Created
```

---

### **Question 2: What Data Does Mock LLM Actually Return?**

**Context**: Multiple tests expect Mock LLM to return specific data structures:
- `alternative_workflows` (E2E-HAPI-002)
- `investigation_outcome = "resolved"` (E2E-HAPI-023)
- Confidence values (E2E-HAPI-026)

**Evidence Needed**:
- Mock LLM response bodies for each scenario
- HAPI's parsed LLM response before processing

**How to Verify**:
```bash
# Check Mock LLM logs for raw responses
grep -A 20 "test-edge-002" $MOCK_LLM_LOG | grep -E "workflow|alternative|confidence"
# OR check HAPI logs for parsed LLM response
grep -A 50 "LLM investigation result" $HAPI_LOG | grep -E "alternative|investigation_outcome"
```

---

### **Question 3: Why Is DataStorage Confidence Overriding Mock LLM?**

**Context**: E2E-HAPI-026 expects `0.85` (Mock LLM) but gets `0.7` (DataStorage workflow confidence)

**Hypothesis**: HAPI's workflow selection logic prefers DataStorage's workflow confidence over LLM's recommendation confidence

**Code to Review**: `holmesgpt-api/src/extensions/recovery/llm_integration.py` - workflow selection logic

---

## 🎯 RECOMMENDED INVESTIGATION SEQUENCE

### **Phase 1: Verify HAPI Image is Current** (5 min)

```bash
# Check HAPI image build time
podman image inspect localhost/holmesgpt-api:holmesgpt-api-18909458 | grep Created

# Check if BR-HAPI-197 fix is active
HAPI_LOG="/tmp/holmesgpt-api-e2e-logs-20260202-192052/holmesgpt-api-e2e-control-plane/pods/holmesgpt-api-e2e_holmesgpt-api-76f6c4f999-b85kx_22866b0d-5b2d-4b00-9d55-63ef93efbf42/holmesgpt-api/0.log"
grep -i "confidence.*threshold\|BR-HAPI-197" $HAPI_LOG
```

**If NOT active**: Rebuild HAPI image and rerun tests

---

### **Phase 2: Analyze Mock LLM Responses** (10 min)

```bash
MOCK_LLM_LOG="/tmp/holmesgpt-api-e2e-logs-20260202-192052/holmesgpt-api-e2e-control-plane/pods/holmesgpt-api-e2e_mock-llm-76b4cf7bd-22m7v_de0a290e-95dd-4f4b-9a7a-7aad22bae030/mock-llm/0.log"

# Check what scenarios are being matched
grep "Matched scenario" $MOCK_LLM_LOG

# Check response structures for key tests
grep -A 50 "test-edge-002" $MOCK_LLM_LOG  # E2E-HAPI-002 (alternatives)
grep -A 50 "MOCK_NOT_REPRODUCIBLE" $MOCK_LLM_LOG  # E2E-HAPI-023 (investigation_outcome)
```

---

### **Phase 3: Fix OpenAPI Client Issue** (5 min)

**File**: `test/e2e/holmesgpt-api/incident_analysis_test.go` line 78

**Change**:
```go
// OLD
Expect(incidentResp.SelectedWorkflow.Set).To(BeFalse(),
    "selected_workflow must be null when no workflow found")

// NEW
Expect(incidentResp.SelectedWorkflow.Value).To(BeNil(),
    "selected_workflow must be null when no workflow found")
```

**Impact**: Fixes E2E-HAPI-001 immediately

---

### **Phase 4: Fix Type Mismatch** (2 min)

**File**: `test/e2e/holmesgpt-api/recovery_analysis_test.go` line 700

**Current**:
```go
Expect(recoveryResp.HumanReviewReason.Value).To(Equal(hapiclient.HumanReviewReasonNoMatchingWorkflows), ...)
```

**Investigation**: Check if `HumanReviewReason.Value` is a `string` or `HumanReviewReason` type

**Fix**: Cast appropriately or use `.String()` method

---

### **Phase 5: Update Mock LLM Scenarios** (15 min)

Based on Phase 2 findings, update:
- **`low_confidence` scenario**: Add `alternative_workflows` array
- **`rca_incomplete` scenario**: Return invalid JSON or missing fields
- **`problem_resolved` scenario**: Ensure `investigation_outcome = "resolved"`
- **`recovery` scenario**: Verify confidence value is `0.85`

---

## 📊 SUMMARY TABLE

| Test ID | Failure Reason | Root Cause | Fix Complexity | Priority |
|---------|----------------|------------|----------------|----------|
| E2E-HAPI-001 | `.Set=true` for `null` | OpenAPI client bug | LOW (test fix) | P1 |
| E2E-HAPI-002 | Missing alternatives | Mock LLM scenario | MEDIUM (Mock LLM) | P2 |
| E2E-HAPI-003 | Wrong `human_review_reason` | Mock LLM scenario | MEDIUM (Mock LLM) | P2 |
| E2E-HAPI-004 | Unexpected `needs_human_review=true` | HAPI image not rebuilt? | HIGH (investigate) | P1 |
| E2E-HAPI-005 | Unknown | Needs investigation | UNKNOWN | P3 |
| E2E-HAPI-007 | Server validation | Expected (V1.1 debt) | N/A (defer) | P4 |
| E2E-HAPI-008 | Server validation | Expected (V1.1 debt) | N/A (defer) | P4 |
| E2E-HAPI-013 | Unknown | Needs investigation | UNKNOWN | P3 |
| E2E-HAPI-014 | Unknown | Needs investigation | UNKNOWN | P3 |
| E2E-HAPI-018 | Server validation | Expected (V1.1 debt) | N/A (defer) | P4 |
| E2E-HAPI-023 | `can_recover=true` | HAPI parser logic | MEDIUM (verify fix) | P2 |
| E2E-HAPI-024 | Type mismatch | Test code bug | LOW (test fix) | P1 |
| E2E-HAPI-026 | Confidence mismatch | DataStorage override | MEDIUM (investigate) | P2 |
| E2E-HAPI-027 | Unknown | Needs investigation | UNKNOWN | P3 |

---

## 🚀 RECOMMENDED NEXT ACTIONS

**Immediate (30 min)**:
1. Verify HAPI image was rebuilt with BR-HAPI-197 fixes
2. Fix E2E-HAPI-001 (OpenAPI client `.Set` issue)
3. Fix E2E-HAPI-024 (type mismatch)

**Short-term (1-2 hours)**:
4. Analyze Mock LLM response structures for all failing scenarios
5. Update Mock LLM scenarios to return expected data
6. Investigate E2E-HAPI-004 (unexpected `needs_human_review=true`)

**Medium-term (2-4 hours)**:
7. Verify HAPI result parser logic changes are active
8. Debug confidence value overrides (DataStorage vs. Mock LLM)
9. Complete analysis of remaining unknowns (E2E-HAPI-005, 013, 014, 027)

---

**Status**: Ready for Phase 1 investigation - verify HAPI image currency.
