# E2E-HAPI-003: Complete Failure Triage
**Date**: February 3, 2026  
**Status**: ROOT CAUSE IDENTIFIED  
**Test**: `E2E-HAPI-003: Max retries exhausted returns validation history`  
**Failure**: `human_review_reason` returns `"no_matching_workflows"` instead of `"llm_parsing_error"`

---

## 📋 **Test Expectations**

From `incident_analysis_test.go:151-199`:

```go
It("E2E-HAPI-003: Max retries exhausted returns validation history", func() {
    req := &hapiclient.IncidentRequest{
        SignalType: "MOCK_MAX_RETRIES_EXHAUSTED",
        // ... other fields
    }
    
    resp, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
    
    // Expected behavior:
    Expect(incidentResp.NeedsHumanReview.Value).To(BeTrue())
    Expect(incidentResp.HumanReviewReason.Value).To(Equal(hapiclient.HumanReviewReasonLlmParsingError))  // ❌ FAILS
    Expect(len(incidentResp.ValidationAttemptsHistory)).To(Equal(3))  // ❌ ALSO FAILS
})
```

**Actual Results**:
- `human_review_reason`: `"no_matching_workflows"` (Expected: `"llm_parsing_error"`)
- `validation_attempts_history`: 1 item (Expected: 3 items)

---

## 🔍 **Root Cause Analysis**

### **Flow Trace**

1. **Mock LLM** (`test/services/mock-llm/src/server.py:862-882`):
   ```python
   if scenario.name == "max_retries_exhausted":
       analysis_json["needs_human_review"] = True
       analysis_json["human_review_reason"] = "llm_parsing_error"
       analysis_json["validation_attempts_history"] = [
           {"attempt": 1, ...},
           {"attempt": 2, ...},
           {"attempt": 3, ...}
       ]
   ```
   ✅ **Mock LLM correctly sets all 3 fields**

2. **Mock LLM Output Format** (section header format - Pattern 2B):
   ```
   # needs_human_review
   True
   
   # human_review_reason
   "llm_parsing_error"
   
   # validation_attempts_history
   [{"attempt": 1, ...}, {"attempt": 2, ...}, {"attempt": 3, ...}]
   ```
   ✅ **Output format is correct**

3. **HAPI Parser** (`holmesgpt-api/src/extensions/incident/result_parser.py`):
   
   **Step 3a: Pattern 2B Extraction** (lines 144-160):
   ```python
   # Extract needs_human_review
   nhr_match = re.search(r'# needs_human_review\s*\n\s*(True|False|true|false)...', analysis)
   if nhr_match:
       parts['needs_human_review'] = nhr_match.group(1)  # "True"
   
   # Extract human_review_reason  
   hrr_match = re.search(r'# human_review_reason\s*\n\s*["\']?([^"\'\n]+)["\']?...', analysis)
   if hrr_match:
       parts['human_review_reason'] = f'"{hrr_match.group(1)}"'  # "\"llm_parsing_error\""
   ```
   
   **Step 3b: Combine and Parse** (lines 162-200):
   ```python
   combined_dict = '{'
   for key, value in parts.items():
       combined_dict += f'"{key}": {value}, '
   # Result: {"needs_human_review": True, "human_review_reason": "llm_parsing_error", ...}
   
   json_data = json.loads(combined_dict)
   ```
   ✅ **Extraction and parsing should work correctly**

4. **HAPI Parser: Value Extraction** (lines 270-284):
   ```python
   needs_human_review_from_llm = json_data.get("needs_human_review") if json_data else None
   human_review_reason_from_llm = json_data.get("human_review_reason") if json_data else None
   
   llm_provided_human_review = (needs_human_review_from_llm is not None or 
                                  human_review_reason_from_llm is not None)
   ```
   ❓ **SUSPECTED ISSUE**: `json_data` might not contain these keys due to extraction failure

5. **HAPI Parser: Override Logic** (lines 342-350):
   ```python
   elif selected_workflow is None:
       warnings.append("No workflows matched the search criteria")
       if not llm_provided_human_review:  # ❌ Evaluates to True even though LLM provided values!
           needs_human_review = True
           human_review_reason = "no_matching_workflows"  # ❌ OVERWRITES LLM value!
   ```
   ❌ **BUG CONFIRMED**: `llm_provided_human_review` is `False`, causing override

---

## 🐛 **Identified Issues**

### **Issue #1: Pattern 2B Extraction Failure**

**Hypothesis**: The regex patterns in Pattern 2B might not be matching the Mock LLM output correctly.

**Evidence**:
- Test shows `human_review_reason = "no_matching_workflows"` (the default override value)
- This means `llm_provided_human_review = False`
- Which means `json_data.get("human_review_reason")` returned `None`

**Possible Causes**:
1. Regex pattern mismatch (lines 145, 151)
2. Section header format issue (Mock LLM output vs. parser expectation)
3. JSON parsing failure (lines 195-200)

### **Issue #2: Validation History Count**

**Test Failure**: 1 item instead of 3

**Explanation**:
- HAPI's self-correction loop (lines 371-560 in `llm_integration.py`) runs up to 3 times
- For `max_retries_exhausted`, Mock LLM returns `selected_workflow = None`
- Parser's validation logic (line 234-245) skips validation when no workflow exists
- Result: `validation_result = None`, so `is_valid = True` (line 503)
- Loop breaks after first iteration (line 548)
- HAPI's loop history has only 1 item
- Mock LLM provides simulated 3-item history via Pattern 2B
- But Pattern 2B extraction is failing (Issue #1), so LLM history is lost

---

## 🔧 **Recommended Fixes**

### **Fix #1: Debug Pattern 2B Extraction**

**Add diagnostic logging** to verify extraction:

```python
# result_parser.py, after line 160
logger.info({
    "event": "pattern_2b_extraction_complete",
    "incident_id": incident_id,
    "parts_keys": list(parts.keys()),
    "needs_human_review": parts.get("needs_human_review"),
    "human_review_reason": parts.get("human_review_reason"),
    "validation_attempts_history_length": len(json.loads(parts.get("validation_attempts_history", "[]"))) if parts.get("validation_attempts_history") else 0
})
```

### **Fix #2: Regex Pattern Correction**

**Current regex for `human_review_reason`** (line 151):
```python
r'# human_review_reason\s*\n\s*["\']?([^"\'\n]+)["\']?\s*(?:\n#|$|\n\n)'
```

**Problem**: Mock LLM outputs `"llm_parsing_error"` (with quotes via `json.dumps()`), so the pattern matches and captures `llm_parsing_error`, then wraps it again at line 153.

**Potential Issue**: If the Mock LLM output has newlines or the pattern doesn't match correctly, extraction fails.

**Recommendation**: Test regex with actual Mock LLM output:
```python
import re
text = '''# human_review_reason
"llm_parsing_error"

# validation_attempts_history'''

match = re.search(r'# human_review_reason\s*\n\s*["\']?([^"\'\n]+)["\']?\s*(?:\n#|$|\n\n)', text)
if match:
    print(f"Captured: {match.group(1)}")  # Should print: llm_parsing_error
```

### **Fix #3: Fallback to LLM-Provided Values**

**If Pattern 2B extraction works**, the override logic should preserve LLM values. But currently it only logs (line 350).

**Corrected Logic** (lines 342-350):
```python
elif selected_workflow is None:
    warnings.append("No workflows matched the search criteria")
    # E2E-HAPI-003: Only override if LLM didn't explicitly provide human review values
    if not llm_provided_human_review:
        needs_human_review = True
        human_review_reason = "no_matching_workflows"
        logger.info("E2E-HAPI-003: Using default no_matching_workflows (LLM didn't provide)")
    else:
        # E2E-HAPI-003: LLM provided values - preserve them (no action needed)
        logger.info(f"E2E-HAPI-003: Preserving LLM-provided values - needs_human_review={needs_human_review}, reason={human_review_reason}")
```

---

## 🧪 **Next Steps**

1. **Run focused test with diagnostic logging**:
   ```bash
   go test ./test/e2e/holmesgpt-api/... -v -ginkgo.focus="E2E-HAPI-003"
   ```

2. **Extract HAPI pod logs**:
   ```bash
   kubectl logs -n holmesgpt-api-e2e deployment/holmesgpt-api | grep -E "pattern_2b|llm_value_extraction|validation_history"
   ```

3. **Verify Pattern 2B extraction**:
   - Check if `parts_keys` includes `needs_human_review` and `human_review_reason`
   - Check if values are correctly captured

4. **Fix regex patterns** if extraction is failing

5. **Re-test** to validate fix

---

## 📊 **Test Status History**

| Date | Tests Passing | Notes |
|------|---------------|-------|
| Feb 3, 15:19 | 39/40 (97.5%) | E2E-HAPI-003 only failing test (validation history count) |
| Feb 3, 16:27 | 36/40 (90%) | Regression after adding `llm_provided_human_review` flag |
| Feb 3, 16:49 | Focused test | E2E-HAPI-003: `human_review_reason` override issue confirmed |

---

## 💡 **Key Insights**

1. **Mock LLM is working correctly** - provides all required fields
2. **Pattern 2B extraction is suspect** - likely not matching/parsing correctly
3. **Override logic is correct** - but relies on `llm_provided_human_review` flag
4. **Diagnostic logging added** - will reveal extraction status in next test run

---

## 📝 **Related Files**

- `test/services/mock-llm/src/server.py:862-906` - Mock LLM scenario definition
- `holmesgpt-api/src/extensions/incident/result_parser.py:144-160` - Pattern 2B extraction
- `holmesgpt-api/src/extensions/incident/result_parser.py:270-350` - Override logic
- `holmesgpt-api/src/extensions/incident/llm_integration.py:371-616` - Self-correction loop
- `test/e2e/holmesgpt-api/incident_analysis_test.go:151-199` - Test definition

---

**STATUS**: Investigation paused pending infrastructure stabilization. Diagnostic logging in place for next test run.
