# CRITICAL FINDING: Mock LLM Never Received Requests
**Date**: January 29, 2026  
**Status**: 🚨 **CRITICAL** - Mock LLM service not being used

---

## 🎯 CRITICAL DISCOVERY

**Mock LLM received ZERO HTTP requests during the entire E2E test run**:

```bash
$ grep -c "POST /v1/chat/completions" /tmp/.../mock-llm/0.log
0
```

**This explains ALL 11 test failures**:
1. ✅ Python dict format (not JSON) - HolmesGPT SDK returning responses directly
2. ✅ Wrong workflows (`wait-for-heal-v1`) - SDK using internal logic, not test scenarios
3. ✅ Parser failures - SDK responses use Python dict format with `# section_headers`
4. ✅ Missing alternative workflows - SDK doesn't have test scenario configurations

---

## 📊 EVIDENCE

### **Evidence #1: Mock LLM Logs**
```bash
$ grep -E "Starting|Listening|Ready" /tmp/.../mock-llm/0.log
Starting Mock LLM server on 0.0.0.0:8080...

$ grep "POST /v1/chat/completions" /tmp/.../mock-llm/0.log
(no results)
```

**Result**: Mock LLM started successfully but received NO requests

---

### **Evidence #2: HAPI Configuration**
```
LLM configuration: provider=openai, model=mock-model -> openai/mock-model, endpoint=http://mock-llm:8080
```

**Result**: HAPI IS configured to use Mock LLM endpoint ✅

---

### **Evidence #3: HAPI Response Format**
```json
{
  "event": "sdk_analysis_structure",
  "has_json_codeblock": false,  // ❌ Not JSON codeblock
  "has_section_headers": false,
  "first_200_chars": "\n# root_cause_analysis\n{'summary': '...', ...}"  // ❌ Python dict
}
```

**Result**: HAPI receiving Python dict format, NOT JSON from Mock LLM

---

### **Evidence #4: Parser Fallback Logic**
HAPI parsers have explicit support for Python dict format:

**File**: `holmesgpt-api/src/extensions/incident/result_parser.py`
```python
# Pattern 2B: Legacy - Python dict format with section headers (HolmesGPT SDK format)
# Format: "# root_cause_analysis\n{'summary': '...', ...}\n\n# selected_workflow\n{'workflow_id': '...', ...}"
if not json_match and ('# selected_workflow' in analysis or '# root_cause_analysis' in analysis):
    import ast
    # ... extract sections ...
    json_data = ast.literal_eval(json_text)  # Parse Python dict
```

**Result**: HAPI was designed to handle responses from HolmesGPT SDK's internal logic (Python dicts), not external Mock LLM (JSON)

---

## 🔍 ROOT CAUSE ANALYSIS

### **Architecture Flow**

**Expected**:
```
HAPI → HTTP → Mock LLM Service (test/services/mock-llm/) → JSON response
```

**Actual**:
```
HAPI → HolmesGPT SDK (dependencies/holmesgpt/) → Internal LLM Logic → Python dict response
```

---

### **Why is Mock LLM Not Being Called?**

**Hypothesis 1: HolmesGPT SDK Has Embedded LLM Logic**
- SDK may have internal Mock/Test mode that bypasses HTTP calls
- SDK may be using fallback logic when endpoint unavailable
- SDK may be caching responses or using embedded scenarios

**Hypothesis 2: Network/DNS Issue**
- HAPI cannot resolve `http://mock-llm:8080`
- But: No connection errors in HAPI logs ❌ (would see "Connection refused")

**Hypothesis 3: SDK Override/Monkey-Patch**
- HAPI may be monkey-patching SDK to use internal logic
- Check for: `holmes.config.Config` overrides, `investigate_issues` patches

**Hypothesis 4: LiteLLM Fallback**
```
WARNING - Couldn't find model openai/mock-model in litellm's model list
```
- LiteLLM may be using fallback provider when model not recognized
- May be calling real OpenAI API or using embedded mock

---

## 🔬 INVESTIGATION PLAN

### **Step 1: Check HolmesGPT SDK for Embedded Mock Logic** (15 min)

**Search for**:
- Embedded Mock LLM scenarios
- Test mode / Mock mode flags
- Hardcoded `wait-for-heal` workflow references
- Python dict response generation

**Commands**:
```bash
cd dependencies/holmesgpt
grep -r "wait-for-heal\|MOCK_MODE\|test_mode\|# root_cause_analysis" --include="*.py"
grep -r "class.*MockLLM\|def.*mock_investigate" --include="*.py"
```

---

### **Step 2: Check HAPI for SDK Overrides** (10 min)

**Search for**:
- Monkey-patching of `investigate_issues()`
- Config overrides
- Test mode flags

**Commands**:
```bash
cd holmesgpt-api/src
grep -r "monkey.*patch\|unittest.*mock\|@patch\|mock.*holmes" --include="*.py"
grep -r "investigate_issues.*=\|Config.*mock" --include="*.py"
```

---

### **Step 3: Verify Mock LLM Service Accessibility** (5 min)

**From HAPI pod**:
```bash
kubectl exec -n holmesgpt-api-e2e <hapi-pod> -- curl -v http://mock-llm:8080/health
kubectl exec -n holmesgpt-api-e2e <hapi-pod> -- nslookup mock-llm
```

**Expected**: Service reachable, DNS resolves

---

### **Step 4: Add Debug Logging to SDK Call** (10 min)

**File**: `holmesgpt-api/src/extensions/incident/llm_integration.py`

**Add before `investigate_issues()` call**:
```python
logger.critical(f"🔍 DEBUG: About to call investigate_issues()")
logger.critical(f"🔍 DEBUG: Config endpoint: {config.llm_endpoint if hasattr(config, 'llm_endpoint') else 'not set'}")
logger.critical(f"🔍 DEBUG: Config provider: {config.llm_provider if hasattr(config, 'llm_provider') else 'not set'}")

result = investigate_issues(...)

logger.critical(f"🔍 DEBUG: Result type: {type(result)}")
logger.critical(f"🔍 DEBUG: Result.analysis preview: {result.analysis[:200] if result.analysis else 'none'}")
```

**Rerun test** and check if Mock LLM receives requests

---

## 🚀 LIKELY FIXES

### **Fix #1: HolmesGPT SDK Has Embedded Mock (Most Likely)**

**If SDK has internal Mock logic**:

**Option A**: Remove embedded Mock logic from SDK
- Ensures SDK always makes HTTP calls to configured endpoint
- Cleaner separation of concerns

**Option B**: Configure SDK to use external Mock LLM
- Find SDK Mock mode flag/config
- Set to use HTTP endpoint instead of embedded logic

**Option C**: Bypass SDK for tests
- HAPI calls Mock LLM HTTP API directly
- Skip SDK's `investigate_issues()` wrapper
- More invasive, but guarantees Mock LLM usage

---

### **Fix #2: LiteLLM Configuration Issue**

**If LiteLLM doesn't recognize `openai/mock-model`**:

Add to HAPI config:
```python
# Force LiteLLM to use custom endpoint
os.environ["OPENAI_API_BASE"] = "http://mock-llm:8080/v1"
os.environ["OPENAI_API_KEY"] = "mock-key"  # Mock LLM doesn't validate
```

Or in SDK Config:
```python
config = Config(
    llm_endpoint="http://mock-llm:8080/v1",
    llm_provider="openai",
    llm_model="gpt-4",  # Use recognized model name
    # ... other config
)
```

---

### **Fix #3: SDK Monkey-Patch Removed**

**If HAPI was monkey-patching SDK**:

Check `holmesgpt-api/src/extensions/llm_config.py`:
```python
2026-02-03 00:20:42,263 - src.extensions.llm_config - INFO - BR-HAPI-250: Monkey-patched list_server_toolsets() AND create_tool_executor() to inject workflow catalog at LLM-visible layer
```

**Verify**: Does this monkey-patch affect HTTP calls?
**Fix**: Ensure patch only affects toolsets, not LLM endpoint routing

---

## 📋 VALIDATION CHECKLIST

After applying fixes:

**Mock LLM**:
- [ ] Mock LLM receives HTTP POST requests
- [ ] Requests contain expected signal types (OOMKilled, CrashLoopBackOff, etc.)
- [ ] Mock LLM returns JSON codeblock format
- [ ] HAPI logs show `has_json_codeblock: true`

**HAPI Response Parsing**:
- [ ] Parser uses Pattern 1 (JSON codeblock), not Pattern 2B (Python dict)
- [ ] `workflow_id` matches test scenarios (e.g., `oomkill-increase-memory-v1`)
- [ ] `alternative_workflows` populated for `low_confidence` scenario
- [ ] `investigation_outcome` present for `problem_resolved` scenario

**Test Results**:
- [ ] E2E-HAPI-001: Still passes (OpenAPI fix)
- [ ] E2E-HAPI-002: `alternative_workflows` not empty
- [ ] E2E-HAPI-004: Correct workflow returned, `needs_human_review=false`
- [ ] E2E-HAPI-023, 024: Recovery logic works correctly
- [ ] All 10 Mock LLM-dependent tests pass

---

## 🔑 KEY INSIGHT

**The parsers having Python dict fallback logic is the smoking gun**:

```python
# Pattern 2B: Legacy - Python dict format with section headers (HolmesGPT SDK format)
```

This "legacy" format is what's currently being used, which means:
1. ✅ HAPI was originally designed with embedded Mock LLM (before extraction)
2. ✅ HolmesGPT SDK still has that embedded logic
3. ✅ SDK is not making HTTP calls to external Mock LLM service
4. ✅ All responses are coming from SDK's internal logic

**The external Mock LLM service** (`test/services/mock-llm/`) was created to replace this embedded logic, but **HAPI/SDK integration was never updated** to use it.

---

## 💡 RECOMMENDED NEXT ACTION

**Priority 1** (30 min): Check HolmesGPT SDK for embedded Mock logic

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut/dependencies/holmesgpt
grep -r "# root_cause_analysis\|# selected_workflow" --include="*.py" -n
grep -r "wait-for-heal\|test.*workflow\|mock.*workflow" --include="*.py" -n
```

Look for code that generates Python dict format responses with `# section_headers`.

**Expected Finding**: SDK has internal test/mock mode that returns hardcoded Python dict responses instead of calling configured LLM endpoint.

---

**Status**: Ready for SDK investigation - Expected to find embedded Mock LLM logic that needs removal or configuration
