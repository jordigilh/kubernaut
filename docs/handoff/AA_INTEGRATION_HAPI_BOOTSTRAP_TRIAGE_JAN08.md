# AI Analysis Integration HAPI Bootstrap Triage - January 8, 2026

**Status**: 🔍 INVESTIGATING HTTP 500 Issue
**Date**: January 8, 2026
**Service**: AI Analysis Integration Tests
**Component**: HolmesGPT-API (HAPI) HTTP Service Bootstrap

---

## 🎯 Executive Summary

**Problem**: AI Analysis integration tests are experiencing HTTP 500 errors from HAPI, despite successful infrastructure bootstrap.

**Root Cause**: Under investigation - HAPI container starts successfully but returns HTTP 500 during analyze requests.

**Impact**: 1 of 13 integration tests failing (audit flow test timeout due to AIAnalysis never reaching "Completed" status).

---

## 🔍 Investigation Timeline

### Issue #1: Test Timeout (180s) - **RESOLVED ✅**

**Problem**:
- HAPI build takes ~100 seconds (Python wheels, dependencies)
- Test timeout was 180s
- Build process was terminated before completion

**Solution**:
- Increased `HealthCheck.Timeout` from 120s → 300s
- Maintained DD-TEST-001 v1.3 compliance (unique infrastructure tags)
- Format: `localhost/holmesgpt-api:aianalysis-{uuid}`

**Commit**: `b6f4a1bc7 - fix: AA Integration HAPI health check timeout (DD-TEST-001 compliant)`

**Result**: ✅ HAPI build completes successfully, health check passes

---

### Issue #2: HTTP 500 Errors During Test Execution - **INVESTIGATING 🔍**

**Symptoms**:
```
2026-01-08T12:55:41-05:00 INFO investigating-handler Transient error - retrying with backoff
{"error": "HolmesGPT-API error (HTTP 500): HolmesGPT-API returned HTTP 500: decode response: unexpected status code: 500"}
```

**Test Impact**:
- Test: "should generate complete audit trail from Pending to Completed"
- Timeout: 90 seconds (waiting for AIAnalysis to reach "Completed" status)
- Result: FAILED - AIAnalysis stuck in "Investigating" phase due to repeated HTTP 500 from HAPI

**Current Test Results**:
- ✅ 12 tests passed
- ❌ 1 test failed (audit flow integration)
- ⏸️  2 tests pending (intentionally marked as incomplete)
- ⏭️  44 tests skipped (parallel execution interrupted by failure)

---

## 🧪 Manual Verification Tests

### Test #1: Manual HAPI Container - **PASSED ✅**

**Configuration**:
```bash
podman run --rm -d --name test_hapi_manual \
  -p 18121:8080 \
  -e MOCK_LLM_MODE=true \
  -e DATA_STORAGE_URL=http://host.containers.internal:18095 \
  -e PORT=8080 \
  -e LOG_LEVEL=DEBUG \
  -v "$(pwd)/hapi-config:/etc/holmesgpt:ro" \
  localhost/holmesgpt-api:aianalysis-integration-latest
```

**Result**: ✅ HAPI starts successfully, responds to health checks and requests

**Logs**:
```
Starting HolmesGPT-API with config: /etc/holmesgpt/config.yaml
INFO:     Uvicorn running on http://0.0.0.0:8080
INFO:     Application startup complete.
```

**Test Request**: Returns HTTP 400 (expected validation error for test payload)

---

## 🔧 Configuration Verified

### HAPI Config (`test/integration/aianalysis/hapi-config/config.yaml`)
```yaml
llm:
  provider: "mock"
  model: "mock/test-model"
  endpoint: "http://localhost:11434"
  secrets_file: "/etc/holmesgpt/secrets/llm-credentials.yaml"  # ✅ Correct path

data_storage:
  url: "http://host.containers.internal:18095"  # ✅ Correct URL for container→host communication
```

### HAPI Secrets (`test/integration/aianalysis/hapi-config/secrets/llm-credentials.yaml`)
```yaml
api_key: "mock-api-key"  # ✅ File exists and is correctly formatted
```

### Volume Mount (suite_test.go)
```go
Volumes: map[string]string{
    hapiConfigDir: "/etc/holmesgpt:ro",  # ✅ Mounts hapi-config (including secrets subdirectory)
},
```

---

## 🤔 Hypothesis: Why HTTP 500 in Tests But Not Manual?

### Potential Causes Under Investigation:

1. **DATA_STORAGE_URL Reachability**:
   - HAPI may be trying to connect to DataStorage during analyze request
   - DataStorage is running on host port 18095
   - `host.containers.internal` should work but might have timing/connectivity issues

2. **Request Format Mismatch**:
   - Integration test sends AIAnalysis spec to HAPI
   - HAPI expects specific analyze request format
   - Mismatch could cause HTTP 500 (vs HTTP 400 validation error)

3. **Race Condition - HAPI Not Fully Ready**:
   - Health check passes (/health endpoint works)
   - BUT analyze endpoint might require additional initialization
   - First analyze request arrives before HAPI is fully ready

4. **Network Configuration**:
   - Test uses network: "aianalysis_test_network"
   - Manual test uses default network
   - Network isolation might affect `host.containers.internal` resolution

---

## 📋 Next Steps

### Immediate Actions:
1. Run single focused test with verbose logging to capture HAPI container output
2. Check HAPI container logs during test execution (before cleanup)
3. Verify DATA_STORAGE_URL is reachable from HAPI container during test
4. Compare request format sent by test vs what HAPI expects

### Debugging Commands:
```bash
# Run focused test
go test ./test/integration/aianalysis -ginkgo.focus="should generate complete audit trail" -v

# Check HAPI logs during test (before cleanup)
podman logs aianalysis_hapi_test

# Verify network connectivity
podman exec aianalysis_hapi_test curl -v http://host.containers.internal:18095/health
```

---

## 📊 Infrastructure Status

### ✅ Working Components:
- DD-TEST-001 v1.3 compliant unique infrastructure tags
- HAPI image build (100s, uses Podman layer caching)
- HAPI health check (300s timeout, passes successfully)
- DataStorage bootstrap (PostgreSQL, Redis, DS service on port 18095)
- Controller manager startup (envtest + real business logic)
- Volume mounts (hapi-config directory with secrets subdirectory)

### ❌ Not Working:
- HAPI /api/v1/incident/analyze endpoint returns HTTP 500 during tests
- AIAnalysis reconciliation never reaches "Completed" status
- Audit flow integration test times out after 90s

### 🔍 Under Investigation:
- Root cause of HTTP 500 errors from HAPI analyze endpoint
- Difference between manual HAPI container (works) and test HAPI container (HTTP 500)
- Network connectivity between HAPI and DataStorage during test

---

## 🔗 Related Documents

- `docs/handoff/AA_INTEGRATION_HTTP500_FIX_JAN08.md` - Previous HTTP 500 investigation (secrets file fix)
- `docs/architecture/decisions/DD-TEST-001-unique-container-image-tags.md` - Infrastructure tag standards
- `test/integration/aianalysis/suite_test.go` - HAPI bootstrap configuration

---

## 👥 Team Assignments

**Current Owner**: AI Assistant (triage in progress)
**Next Handoff**: After HTTP 500 root cause identified and fix applied

**Escalation Path**: If unable to resolve within 1 hour → escalate to user for guidance

---

**Last Updated**: January 8, 2026 1:05 PM ET
**Next Update**: After focused test completes and logs analyzed


