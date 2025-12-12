# HolmesGPT-API Configuration Issues - From AIAnalysis E2E Testing

**Date**: 2025-12-12  
**Reporter**: AIAnalysis Team  
**Environment**: E2E Testing (Kind cluster)  
**Status**: 🟢 **1 Fixed** | 🔴 **1 Outstanding**  
**Impact**: Blocking 11/13 AIAnalysis E2E test failures

---

## 📋 **Executive Summary**

During AIAnalysis E2E testing, we discovered **two configuration issues** with HolmesGPT-API:

1. ✅ **FIXED**: Initial incident endpoint missing `LLM_MODEL` environment variable
2. 🔴 **OUTSTANDING**: Recovery endpoint returning 500 errors despite `LLM_MODEL` being set

**Impact**: The recovery endpoint issue is blocking **11 out of 13** AIAnalysis test failures (85% of remaining failures).

---

## 🔴 **Issue 1: Initial Endpoint - LLM_MODEL Required (FIXED)**

### **Endpoint**: `/api/v1/incident/analyze`

### **Problem**:
```python
ValueError: LLM_MODEL environment variable or config.llm.model is required
```

### **Error Details**:
```
{'event': 'incident_analysis_failed', 
 'incident_id': 'e2e-prod-incident-1765556676170980000', 
 'error': '500: LLM_MODEL environment variable or config.llm.model is required'}

Traceback:
  File "/opt/app-root/src/src/extensions/incident.py", line 810, in analyze_incident
    model_name, provider = get_model_config_for_sdk(app_config)
  File "/opt/app-root/src/src/extensions/llm_config.py", line 260, in get_model_config_for_sdk
    raise ValueError("LLM_MODEL environment variable or config.llm.model is required")
```

### **Fix Applied** ✅:
```yaml
env:
- name: LLM_PROVIDER
  value: mock
- name: LLM_MODEL          # ← Added
  value: mock://test-model  # ← Added
- name: MOCK_LLM_ENABLED
  value: "true"
```

### **Result**: ✅ Initial incident endpoint now working

### **Recommendation for HAPI Team**:
Consider making `LLM_MODEL` **optional with a sensible default** when `MOCK_LLM_ENABLED=true`, or document this requirement more prominently in deployment guides.

---

## 🔴 **Issue 2: Recovery Endpoint - Still Failing (OUTSTANDING)**

### **Endpoint**: `/api/v1/recovery/analyze`

### **Problem**:
Recovery endpoint returns 500 errors **even after** `LLM_MODEL` is configured.

### **Error Details**:
```
{'event': 'http_exception', 
 'request_id': '9fe6879e-2d42-4ba1-a344-a92b3c3bd59e', 
 'path': '/api/v1/recovery/analyze', 
 'status_code': 500, 
 'detail': 'LLM_MODEL environment variable or config.llm.model is required'}
```

### **Stack Trace**:
```python
File "/opt/app-root/src/src/extensions/recovery.py", line 1723, in recovery_analyze_endpoint
  result = await analyze_recovery(request_data)
           ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
File "/opt/app-root/src/src/extensions/recovery.py", line 1564, in analyze_recovery
  investigation_result = investigate_issues(
                         ^^^^^^^^^^^^^^^^^^^
File "/opt/app-root/lib64/python3.12/site-packages/holmes/core/investigation.py", line 44, in investigate_issues
  ai = config.create_issue_investigator(dal=dal, model=model)
       ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
```

### **Current Configuration** (Not Working):
```yaml
env:
- name: LLM_PROVIDER
  value: mock
- name: LLM_MODEL
  value: mock://test-model
- name: MOCK_LLM_ENABLED
  value: "true"
- name: DATASTORAGE_URL
  value: http://datastorage:8080
- name: LOG_LEVEL
  value: INFO
```

### **Analysis**:
The recovery endpoint has **different configuration requirements** than the initial incident endpoint:
- Initial endpoint (`incident.py`): ✅ Works with above config
- Recovery endpoint (`recovery.py`): ❌ Fails with same config

### **Hypothesis**:
1. Recovery endpoint may require **additional** environment variables
2. Recovery endpoint may need different LLM model format
3. Recovery endpoint may bypass mock configuration
4. `config.create_issue_investigator()` may have different requirements

---

## 🔍 **Test Scenarios Affected**

### **AIAnalysis E2E Tests Blocked** (11/13 failures):

#### **Recovery Flow Tests** (6 tests):
1. BR-AI-080: Recovery attempt support
2. BR-AI-081: Previous execution context handling
3. BR-AI-082: Recovery endpoint routing verification
4. BR-AI-083: Multi-attempt recovery escalation
5. Conditions population during recovery flow
6. Recovery endpoint routing verification

#### **Full Flow Tests** (5 tests - depend on recovery):
1. Production incident - full 4-phase reconciliation cycle
2. Production incident - approval required
3. Staging incident - auto-approve
4. Data quality warnings
5. Recovery attempt escalation

### **Test Coverage Impact**:
```
Current: 9/22 tests passing (41%)
If Fixed: 20/22 tests passing (91%)
Impact: +50% test coverage
```

---

## 🎯 **Requested Actions for HAPI Team**

### **Priority 1: Investigation** (30 minutes)
1. **Compare configuration requirements**:
   - `/api/v1/incident/analyze` (working)
   - `/api/v1/recovery/analyze` (failing)

2. **Check `recovery.py` line 1564**:
   ```python
   investigation_result = investigate_issues(...)
   ```
   What parameters does this require that differ from initial endpoint?

3. **Review `config.create_issue_investigator()`**:
   - Does it need different config for recovery mode?
   - Does it bypass `MOCK_LLM_ENABLED`?

### **Priority 2: Documentation** (15 minutes)
1. Document **all** required environment variables for recovery endpoint
2. Create comparison table: Initial vs Recovery endpoint requirements
3. Add to deployment guide: "Recovery Endpoint Configuration"

### **Priority 3: Fix or Workaround** (1-2 hours)
**Option A**: Make recovery endpoint use same config as initial endpoint
**Option B**: Document additional required environment variables
**Option C**: Add better error messages indicating missing config

---

## 📝 **Sample Request for Testing**

### **Working Request** (Initial Endpoint):
```bash
curl -X POST http://holmesgpt-api:8080/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test-incident-001",
    "description": "Pod is crashing",
    "context": {
      "namespace": "default",
      "pod": "test-pod"
    }
  }'

# Result: 200 OK ✅
```

### **Failing Request** (Recovery Endpoint):
```bash
curl -X POST http://holmesgpt-api:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test-recovery-001",
    "is_recovery_attempt": true,
    "attempt_number": 1,
    "previous_executions": [{
      "workflow_id": "restart-pod-v1",
      "failed": true
    }]
  }'

# Result: 500 Internal Server Error ❌
# Error: "LLM_MODEL environment variable or config.llm.model is required"
```

---

## 🔧 **Potential Solutions**

### **Solution 1: Add Missing Config** (Most Likely)
If recovery endpoint needs additional config:
```yaml
env:
- name: LLM_MODEL
  value: mock://test-model
- name: RECOVERY_LLM_MODEL      # ← New?
  value: mock://recovery-model
- name: HOLMES_CONFIG_PATH      # ← New?
  value: /etc/holmesgpt/config.yaml
# ... other missing vars?
```

### **Solution 2: Fix Config Lookup** (If Bug)
If recovery endpoint should use same config as initial:
```python
# In recovery.py, line 1564
# Ensure it uses the same config source as incident.py
investigation_result = investigate_issues(
    ...,
    config=get_config_from_env(),  # Use same method as initial endpoint
    ...
)
```

### **Solution 3: Default Model for Mock Mode**
```python
# In llm_config.py or recovery.py
def get_model_config_for_sdk(app_config):
    # If MOCK_LLM_ENABLED, provide default
    if os.getenv("MOCK_LLM_ENABLED") == "true":
        return "mock://default-model", "mock"
    
    # Otherwise require explicit config
    if not app_config.llm.model:
        raise ValueError("LLM_MODEL environment variable or config.llm.model is required")
    ...
```

---

## 📊 **Impact Analysis**

### **On AIAnalysis Service**:
| Metric | Current | After Fix | Impact |
|--------|---------|-----------|--------|
| E2E Tests Passing | 9/22 (41%) | 20/22 (91%) | +50% |
| Recovery Tests | 0/6 (0%) | 6/6 (100%) | +100% |
| Full Flow Tests | 0/5 (0%) | 5/5 (100%) | +100% |
| Blocked Features | 2 major | 0 | Unblocked |

### **Features Currently Blocked**:
1. **Recovery Attempt Analysis** (BR-AI-080, BR-AI-081, BR-AI-082, BR-AI-083)
   - Cannot analyze why previous remediation attempts failed
   - Cannot provide intelligent retry strategies
   - Cannot track state changes between attempts

2. **Multi-Attempt Recovery Escalation** (BR-AI-013)
   - Cannot escalate after multiple failures
   - Cannot require approval for risky retries

### **Business Impact**:
- AIAnalysis can handle **initial incidents** ✅
- AIAnalysis **cannot handle recovery attempts** ❌
- **50% of AIAnalysis value** is blocked by this issue

---

## 🤝 **Cross-Team Collaboration**

### **What AIAnalysis Team Can Provide**:
1. ✅ Detailed error logs from E2E tests
2. ✅ Test cluster access for debugging (Kind cluster available)
3. ✅ Sample requests that trigger the issue
4. ✅ Comparison with working initial endpoint
5. ✅ Test infrastructure for validation

### **What We Need from HAPI Team**:
1. 🔍 Investigation of configuration differences (30 min)
2. 📝 Documentation of recovery endpoint requirements (15 min)
3. 🔧 Fix or workaround implementation (1-2 hours)
4. ✅ Validation with AIAnalysis E2E tests

### **Estimated Timeline**:
```
Investigation:   30 minutes
Documentation:   15 minutes
Implementation:  1-2 hours
Testing:         30 minutes
─────────────────────────────
Total:           2-3 hours
```

---

## 📚 **Reference Documentation**

### **AIAnalysis E2E Test Infrastructure**:
- Location: `test/infrastructure/aianalysis.go`
- HolmesGPT Deployment: Lines 599-645
- Current Config: Lines 622-630

### **Related Issues**:
- BR-AI-080: Recovery attempt support
- BR-AI-081: Previous execution context handling
- BR-AI-082: Recovery endpoint routing
- BR-AI-083: Multi-attempt escalation

### **Test Results**:
- Full test output: `docs/handoff/AA_E2E_FINAL_STATUS_WHEN_YOU_RETURN.md`
- Infrastructure fixes: `docs/handoff/COMPLETE_AIANALYSIS_E2E_INFRASTRUCTURE_FIXES.md`

---

## 🎯 **Success Criteria**

### **Definition of Done**:
1. ✅ Recovery endpoint returns 200 (not 500)
2. ✅ Mock responses include `recovery_analysis` structure
3. ✅ AIAnalysis E2E recovery tests pass (6 tests)
4. ✅ AIAnalysis E2E full flow tests pass (5 tests)
5. ✅ Configuration documented in HAPI deployment guide

### **Validation**:
```bash
# Test 1: Recovery endpoint health
curl -X POST http://holmesgpt-api:8080/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{"incident_id":"test","is_recovery_attempt":true,"attempt_number":1}'

# Expected: 200 OK (not 500)

# Test 2: Run AIAnalysis E2E tests
make test-e2e-aianalysis

# Expected: 20/22 tests passing (currently 9/22)
```

---

## 💬 **Contact Information**

### **For Questions**:
- Team: AIAnalysis
- Primary Contact: [Your Team Contact]
- Slack Channel: #aianalysis-e2e
- Documentation: `docs/services/crd-controllers/02-aianalysis/`

### **For Testing Support**:
- E2E Infrastructure: `test/infrastructure/aianalysis.go`
- Test Cluster: Kind cluster `aianalysis-e2e`
- Test Execution: `make test-e2e-aianalysis`

---

## 🎉 **Positive Notes**

### **What's Working Well**:
1. ✅ Initial incident endpoint works perfectly after `LLM_MODEL` fix
2. ✅ Mock LLM responses are comprehensive
3. ✅ HAPI integration with AIAnalysis is well-designed
4. ✅ Error messages are clear and helpful

### **Collaboration Highlights**:
- Quick identification of Issue 1 led to immediate fix
- Clear error messages made debugging straightforward
- Mock mode works excellently for E2E testing

**This is a great opportunity to make the recovery endpoint as robust as the initial endpoint!** 🚀

---

**Status**: 🟡 **Awaiting HAPI Team Investigation**  
**Priority**: **HIGH** (blocking 50% of AIAnalysis functionality)  
**Estimated Fix Time**: 2-3 hours  
**Date**: 2025-12-12  
**Next Steps**: HAPI team investigation of recovery endpoint config requirements
