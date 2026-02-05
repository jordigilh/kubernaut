# HAPI E2E Test Failures: BR-HAPI-197 Compliance Analysis

**Date**: February 2, 2026  
**Status**: 🔍 Analysis Complete - Awaiting User Decision  
**Test Results**: 26/40 passing (14 failures)

---

## Executive Summary

After fixing Mock LLM UUID loading issues, HAPI E2E tests still have 14 failures. **Root cause**: Test expectations vs HAPI behavior misalignment regarding BR-HAPI-197 (`needs_human_review` field).

**Critical Finding**: **BR-HAPI-197 has internal contradictions** that must be resolved before fixing tests.

---

## BR-HAPI-197 Contradiction Analysis

### Contradiction 1: Who Sets `needs_human_review` for Low Confidence?

**BR-HAPI-197 Line 62** (Table):
```
| Low Confidence | Overall confidence is below threshold (threshold owned by AIAnalysis) |
```
✅ **Implies**: `low_confidence` is a valid trigger for `needs_human_review=true`

**BR-HAPI-197 Lines 220 & 232** (Notes):
```
Note: HAPI returns `confidence` but does NOT enforce thresholds. AIAnalysis owns the threshold logic.

Note: `needs_human_review` is only set by HAPI for **validation failures**, not confidence thresholds.
```
❌ **Contradicts**: HAPI should NOT set `needs_human_review` based on confidence thresholds

### User's Previous Decision (Jan 29, 2026)

When asked about this contradiction, user chose **Option B**:
> "HAPI only returns confidence, AIAnalysis makes the decision about `needs_human_review`"

**Implication**: BR-HAPI-197 lines 220 & 232 are authoritative. HAPI does NOT set `needs_human_review` for low confidence.

---

## Test Failures Analysis

### Category 1: `needs_human_review` Misalignment

#### **E2E-HAPI-004**: Normal incident analysis succeeds
```
Expected: needs_human_review = false (for confident recommendation)
Actual:   needs_human_review = true
```

**Root Cause**: HAPI is incorrectly setting `needs_human_review=true` even for confident workflows (confidence=0.88).

**BR Violation**: Violates BR-HAPI-197 lines 220 & 232 (HAPI should NOT set based on confidence).

**Fix Required**: HAPI Python code must be corrected to NOT set `needs_human_review=true` unless there's a **validation failure** (workflow not found, parsing error, etc.).

---

### Category 2: `alternative_workflows` Expectations

#### **E2E-HAPI-002**: Low confidence returns human review with alternatives
```
Expected: alternative_workflows != empty
Actual:   alternative_workflows = []
```

**Root Cause**: Mock LLM does NOT include `alternative_workflows` in its response for `low_confidence` scenario.

**Investigation**:
- `alternative_workflows` field exists in `IncidentResponse` schema (incident_models.py:316-320)
- Field is **optional** (`default_factory=list`)
- Purpose: "Informational purposes only - helps operators understand AI reasoning"
- HAPI extracts `alternative_workflows` from LLM response (result_parser.py:176-184)
- Mock LLM `low_confidence` scenario does NOT generate `alternative_workflows` in JSON response

**Question**: Is providing `alternative_workflows` **mandatory** for low confidence scenarios per any BR?

**Options**:
- **Option A**: Update Mock LLM to provide `alternative_workflows` for `low_confidence` scenario
- **Option B**: Update test to NOT require `alternative_workflows` (make it optional validation)

**Recommendation**: Option B - `alternative_workflows` is an optional field. Tests should validate it's present IF PROVIDED, but not require it.

---

### Category 3: `human_review_reason` Enum Mismatch

#### **E2E-HAPI-003**: Max retries exhausted returns validation history
```
Expected: human_review_reason = "llm_parsing_error"
Actual:   human_review_reason = "workflow_not_found"
```

**Root Cause**: HAPI's `determine_human_review_reason()` function is returning wrong enum value.

**BR Compliance**: BR-HAPI-197 defines these enum values (incident_models.py:45-56):
```python
class HumanReviewReason(str, Enum):
    WORKFLOW_NOT_FOUND = "workflow_not_found"
    IMAGE_MISMATCH = "image_mismatch"
    PARAMETER_VALIDATION_FAILED = "parameter_validation_failed"
    NO_MATCHING_WORKFLOWS = "no_matching_workflows"
    LOW_CONFIDENCE = "low_confidence"
    LLM_PARSING_ERROR = "llm_parsing_error"
    INVESTIGATION_INCONCLUSIVE = "investigation_inconclusive"
    RCA_INCOMPLETE = "rca_incomplete"
```

**Fix Required**: Correct HAPI's `determine_human_review_reason()` logic to return correct enum value based on validation errors.

---

### Category 4: Confidence Value Modification

#### **E2E-HAPI-005**: Incident response structure validation
```
Expected: confidence = 0.88 (from Mock LLM "crashloop" scenario)
Actual:   confidence = 0.7
```

**Root Cause**: HAPI is modifying confidence value received from Mock LLM/workflow search.

**BR Violation**: HAPI should pass through confidence values unchanged. AIAnalysis applies thresholds, not HAPI.

**Fix Required**: Identify where HAPI is modifying confidence and remove that logic.

---

## Summary of Required Fixes

### 🔴 **HAPI Business Logic Bugs** (Per BR-HAPI-197)

| Test ID | Issue | BR Violation | Fix Required |
|---------|-------|--------------|--------------|
| E2E-HAPI-004 | `needs_human_review=true` for confident workflow | BR-HAPI-197 lines 220 & 232 | Remove confidence-based `needs_human_review` logic |
| E2E-HAPI-003 | Wrong `human_review_reason` enum | BR-HAPI-197 enum definition | Fix `determine_human_review_reason()` |
| E2E-HAPI-005 | Confidence value modified (0.88→0.7) | BR-HAPI-197 (pass-through) | Remove confidence modification logic |

### 🟡 **Test Expectation Issues**

| Test ID | Issue | Fix Required |
|---------|-------|--------------|
| E2E-HAPI-002 | Requires `alternative_workflows` | Update test to make `alternative_workflows` optional OR enhance Mock LLM |

---

## Next Steps (Awaiting User Decision)

### Decision 1: `alternative_workflows` Requirement

**Question**: Should `alternative_workflows` be **mandatory** or **optional** for low confidence scenarios?

**Option A**: Mandatory (requires Mock LLM enhancement)
- ✅ Pro: Provides richer context for operators
- ❌ Con: Adds complexity to Mock LLM
- ❌ Con: Real LLMs may not always provide alternatives

**Option B**: Optional (update test expectations)
- ✅ Pro: Matches schema definition (`default_factory=list`)
- ✅ Pro: More flexible for real LLM integrations
- ✅ Pro: Less test maintenance
- ❌ Con: Less strict validation

**Recommendation**: **Option B** - Make `alternative_workflows` optional validation.

### Decision 2: Fix Approach

Once decisions are made, implement fixes in this order:
1. Fix HAPI `needs_human_review` logic (remove confidence-based setting)
2. Fix HAPI `human_review_reason` enum mapping
3. Fix HAPI confidence modification bug
4. Update test expectations per Decision 1

---

## Files to Modify

### HAPI Python Code
- `holmesgpt-api/src/extensions/incident/llm_integration.py` (needs_human_review logic)
- `holmesgpt-api/src/extensions/incident/result_parser.py` (determine_human_review_reason, confidence)

### Go E2E Tests
- `test/e2e/holmesgpt-api/incident_analysis_test.go` (E2E-HAPI-002, 003, 004, 005)
- `test/e2e/holmesgpt-api/recovery_analysis_test.go` (similar issues expected)
- `test/e2e/holmesgpt-api/workflow_catalog_test.go` (similar issues expected)

### Mock LLM (Optional, per Decision 1)
- `test/services/mock-llm/src/server.py` (add `alternative_workflows` generation)

---

## Confidence Assessment

**Analysis Confidence**: 95%
- ✅ BR-HAPI-197 contradiction confirmed by reading authoritative docs
- ✅ User's previous decision (Option B) clearly documented
- ✅ HAPI code inspection confirms bugs exist
- ✅ Test expectations analyzed against BRs
- ⚠️  Awaiting user decision on `alternative_workflows` requirement

**Implementation Risk**: Low
- Fixes are isolated to HAPI business logic
- TDD RED phase already complete (tests exist and are failing)
- GREEN phase will be straightforward once decisions made

---

## Related Documents

- [BR-HAPI-197: Human Review Required Flag](/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/requirements/BR-HAPI-197-needs-human-review-field.md)
- [HAPI_E2E_BREAKTHROUGH_JAN_29_2026.md](/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/handoff/HAPI_E2E_BREAKTHROUGH_JAN_29_2026.md)
- [HAPI_E2E_TRIAGE_RESULTS_JAN_29_2026.md](/Users/jgil/go/src/github.com/jordigilh/kubernaut/docs/handoff/HAPI_E2E_TRIAGE_RESULTS_JAN_29_2026.md)
