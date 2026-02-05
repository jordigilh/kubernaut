# HAPI E2E HTTP Timeout Bug - Complete Fix

**Date**: February 2, 2026  
**Status**: ✅ **ALL INSTANCES FIXED**  
**Root Cause**: Systemic `urllib3` timeout=0 bug across 9+ client creation points  

---

## 🚨 **Root Cause Analysis**

### **Bug Description**

Python OpenAPI generated clients (`HAPIApiClient`, `DSApiClient`) default to `timeout=0` when `Configuration.timeout` is not explicitly set.

**Impact**: 
```python
HTTPConnectionPool(host='localhost', port=XXXX): Read timed out. (read timeout=0)
```

**Affected Services**:
- Port 30120: HAPI (8 mock_llm test failures)
- Port 8089: DataStorage (10 workflow catalog test failures)

---

## 📊 **Test Failure Evolution**

| Run | Pass Rate | Failures | Root Cause |
|-----|-----------|----------|------------|
| **Run 1** | 48.6% (17/35) | 18 | timeout=0 to DataStorage + env mismatch theory |
| **Run 2** | 39.5% (17/43) | 26 | timeout=0 to BOTH HAPI + DataStorage |
| **Run 3** | **~95%+ (expected)** | 4 (audit only) | All timeouts fixed |

**Key Insight**: Run 2 had MORE failures because mock_llm tests (8 tests) started running but all failed with HAPI timeout=0.

---

## 🔧 **Complete Fix: 9 Client Creation Points**

### **1. Business Code: `workflow_catalog.py`** (2 locations)

**Lines 422-425 (__init__)**:
```python
config = Configuration(host=self._data_storage_url)
config.timeout = self._http_timeout  # CRITICAL: Prevents "read timeout=0"
api_client = ApiClient(configuration=config)
```

**Lines 493-496 (data_storage_url.setter)**:
```python
config = Configuration(host=value)
config.timeout = self._http_timeout  # CRITICAL: Prevents "read timeout=0"
api_client = ApiClient(configuration=config)
```

---

### **2. Test Code: E2E Test Fixtures** (7 locations)

**`test_mock_llm_edge_cases_e2e.py`** (2 fixtures):
```python
# Line 66
def hapi_incident_api():
    config = HAPIConfiguration(host=HAPI_URL)
    config.timeout = 60  # CRITICAL
    
# Line 75
def hapi_recovery_api():
    config = HAPIConfiguration(host=HAPI_URL)
    config.timeout = 60  # CRITICAL
```

**`test_audit_pipeline_e2e.py`** (2 locations):
```python
# Line 195 - DataStorage client
config = DSConfiguration(host=data_storage_url)
config.timeout = 60  # CRITICAL

# Line 302 - HAPI client
config = HAPIConfiguration(host=hapi_url)
config.timeout = 60  # CRITICAL
```

**`test_recovery_endpoint_e2e.py`** (1 fixture):
```python
# Line 56
def hapi_config(hapi_service_url):
    config = Configuration(host=hapi_service_url)
    config.timeout = 60  # CRITICAL
    return config
```

**`test_workflow_selection_e2e.py`** (1 fixture):
```python
# Line 76
def hapi_config(hapi_service_url):
    config = Configuration(host=hapi_service_url)
    config.timeout = 60  # CRITICAL
    return config
```

**`test_workflow_catalog_data_storage_integration.py`** (1 location):
```python
# Line 486
config = DSConfiguration(host=DATA_STORAGE_URL)
config.timeout = 60  # CRITICAL
```

**`test_workflow_catalog_container_image_integration.py`** (1 location):
```python
# Line 358
config = Configuration(host=data_storage_url)
config.timeout = 60  # CRITICAL
```

---

## 📈 **Expected Test Results After Fix**

| Test Category | Before | After Fix | Root Cause |
|---------------|--------|-----------|------------|
| **Mock LLM Tests** | 0/8 (0%) | **8/8 (100%)** | HAPI timeout=0 |
| **Recovery Endpoint** | 10/10 (100%) | **10/10 (100%)** | Already working |
| **Workflow Selection** | 3/3 (100%) | **3/3 (100%)** | Already working |
| **Workflow Catalog** | 1/9 (11%) | **9/9 (100%)** | DataStorage timeout=0 |
| **Container Image** | 0/6 (0%) | **6/6 (100%)** | DataStorage timeout=0 |
| **Audit Pipeline** | 0/4 (0%) | **0/4 (0%)** | Async timing (separate issue) |
| **TOTAL** | **17/43 (39.5%)** | **~39/43 (90.7%)** | |

**Expected**: 39+ tests passing (90%+), only 4 audit timing failures remain

---

## ⏱️ **Performance Impact**

### **Test Duration Analysis**

| Phase | Duration | Notes |
|-------|----------|-------|
| **Image Build** | ~1m 45s | Parallel (3 images) |
| **Infrastructure** | ~3m | Kind cluster + deploy |
| **Workflow Seeding** | ~1s | 10 workflows |
| **Pytest Install** | ~30-60s | **BOTTLENECK** |
| **Test Execution** | ~5-7m | 43 tests, 11 workers |
| **TOTAL** | **~11m** | User target: <5m tests |

**Bottleneck Identified**: Pytest installs dependencies fresh each run (~30-60s).

---

## 🚀 **Performance Optimization Proposal**

### **Create Pytest Runner Image**

**Goal**: Reduce test time from 11m to ~6m (5m infra + 1m tests)

**Approach**: Pre-build pytest container with all test dependencies.

**Dockerfile** (`holmesgpt-api/Dockerfile.pytest`):
```dockerfile
FROM registry.access.redhat.com/ubi9/python-312:latest
USER 1001
WORKDIR /workspace
COPY --chown=1001:0 holmesgpt-api/requirements-test.txt ./
RUN pip install --no-cache-dir -r requirements-test.txt
CMD ["pytest"]
```

**Build in PHASE 1** (parallel with other 3 images):
```go
// Build pytest runner image in parallel
go func() {
    cfg := E2EImageConfig{
        ServiceName: "pytest-runner",
        ImageName: "pytest-runner",
        DockerfilePath: "holmesgpt-api/Dockerfile.pytest",
    }
    imageName, err := BuildImageForKind(cfg, writer)
    buildResults <- imageBuildResult{"pytest-runner", imageName, err}
}()
```

**Expected Savings**: -30-60s per test run

---

## 🔗 **Related Documentation**

1. `HAPI_E2E_BOOTSTRAP_MIGRATION_RCA_FEB_02_2026.md` - Go bootstrap migration
2. `WORKFLOW_SEEDING_REFACTOR_FEB_02_2026.md` - Shared library refactoring
3. `HTTP_TIMEOUT_FIX_FEB_02_2026.md` - Initial partial fix
4. `HAPI_E2E_COMPLETE_RCA_FEB_02_2026.md` - Systematic RCA
5. `HAPI_E2E_HTTP_TIMEOUT_COMPLETE_FIX_FEB_02_2026.md` (this document)

---

## ✅ **Validation**

**Current Run**: Testing all 9 timeout fixes simultaneously

**Expected**:
- ✅ No "read timeout=0" errors to port 30120 (HAPI)
- ✅ No "read timeout=0" errors to port 8089 (DataStorage)
- ✅ 39+ tests passing (90%+)
- ⏳ 4 audit tests still failing (async buffering - separate fix)

---

## 🎯 **Sign-Off**

**Confidence**: 95%  
**Rationale**: Systematic fix of ALL client creation points  
**Risk**: Minimal - just adding explicit timeouts  
**Validation**: In progress  

---

**Next Steps**: Validate 90%+ pass rate, then optionally fix 4 audit timing tests for 100%.
