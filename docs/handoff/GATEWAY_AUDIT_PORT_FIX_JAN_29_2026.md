# Gateway Audit Emission Fix - DataStorage Port Correction

**Date**: January 29, 2026  
**Status**: ✅ **COMPLETE - ALL 98 TESTS PASSING**  
**Session**: Gateway E2E Audit Failure Investigation

---

## 🎯 Executive Summary

Fixed Gateway E2E audit emission failures by correcting DataStorage service port in Gateway ConfigMap.

**Root Cause**: Gateway trying to connect to port 8080, but DataStorage listening on port 8081

**Fix**: Changed `data_storage_url` from port 8080 → 8081

**Result**: 
- ❌ Before: 89/98 passing (9 audit failures)
- ✅ After: **98/98 passing (0 failures)**

---

## 🔍 Investigation Process

### Symptom Analysis
**Test Failures**: 9 audit-related tests failing
- Test 15: Audit trace validation
- Test 23: Audit emission (signal.received, signal.deduplicated, crd.created)
- Test 24: Signal data for RR reconstruction (5 tests)

**Error Pattern**: Tests expect audit events, queries return 0 results

### Gateway Logs Analysis
```json
{"msg": "🔍 StoreAudit called", "event_type": "gateway.signal.received", ...}
{"msg": "Failed to write audit batch", "batch_size": 25, 
 "error": "Client.Timeout exceeded while awaiting headers"}
```

**Key Finding**: Gateway IS calling StoreAudit, but writes timeout

### DataStorage Logs Analysis
```json
{"msg": "HTTP server listening", "addr": "0.0.0.0:8081"}
{"user": "system:serviceaccount:kubernaut-system:gateway-e2e-audit-client", ...}  ✅
# NO Gateway ServiceAccount requests found ❌
```

**Key Finding**: 
- Test clients (e2e-audit-client) successfully reach DataStorage
- Gateway pod audit client NEVER reaches DataStorage
- DataStorage listening on port **8081**

### Configuration Analysis
**Gateway ConfigMap** (WRONG):
```yaml
data_storage_url: "http://data-storage-service...8080"  # ❌
```

**DataStorage Service** (CORRECT):
```go
// test/infrastructure/datastorage.go:1090
Port: 8081,  // Service port
TargetPort: intstr.FromInt(8081),  // Container port
```

**Result**: Port mismatch → Gateway can't connect → timeout after 5s

---

## ✅ Fix Applied

### File 1: test/e2e/gateway/gateway-deployment.yaml
```yaml
# BEFORE
data_storage_url: "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"

# AFTER  
data_storage_url: "http://data-storage-service.kubernaut-system.svc.cluster.local:8081"
```

### File 2: test/infrastructure/gateway_e2e.go
```yaml
# BEFORE (line 1031)
data_storage_url: "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"

# AFTER
data_storage_url: "http://data-storage-service.kubernaut-system.svc.cluster.local:8081"
```

**Comment Added**: "Port 8081: DataStorage container port (Service also exposes 8081)"

---

## 📊 Test Results

### Before Fix (Run 3)
```
Ran 98 of 98 Specs in 382.747 seconds
FAIL! -- 89 Passed | 9 Failed | 0 Pending | 0 Skipped
```

**Failures**:
- Test 15: Audit trace validation (1 failure)
- Test 23: Audit emission (3 failures)
- Test 24: Signal data capture (5 failures)

### After Fix (Run 4)
```
Ran 98 of 98 Specs in 196.828 seconds
SUCCESS! -- 98 Passed | 0 Failed | 0 Pending | 0 Skipped
```

**Performance Improvement**: 382s → 197s (48% faster!)
- Reason: No more retry timeouts (9 tests × 3 retries × 5s timeout = ~135s wasted)

---

## 🎓 Key Learnings

### 1. Port Configuration Consistency
**Rule**: Always verify Service port matches ConfigMap URLs

**Validation Command**:
```bash
# Check Service port
kubectl get service data-storage-service -n kubernaut-system -o jsonpath='{.spec.ports[?(@.name=="http")].port}'

# Verify matches Gateway ConfigMap
kubectl get configmap gateway-config -n kubernaut-system -o yaml | grep data_storage_url
```

### 2. Audit Client Silent Failure
**Issue**: ServiceAccountTransport returns empty string if token file missing (no error)

**Result**: Request sent without Authorization header → DataStorage middleware rejects

**But**: Gateway error was "timeout", not "401 Unauthorized"

**Explanation**: Gateway was hitting wrong port (8080 not listening) → connection timeout BEFORE auth check

### 3. Log Analysis Pattern
**Effective Debugging**:
1. ✅ Check if events reach buffer (StoreAudit called)
2. ✅ Check if batches are written (Failed to write)
3. ✅ Check if target service receives requests (DataStorage logs)
4. ✅ If target never receives → network/DNS/port issue

**Anti-Pattern**: Assuming authentication issue when seeing timeouts

---

## 🔗 Related Fixes (Previous Sessions)

### Infrastructure Fixes (January 29, 2026)
1. ✅ **Service Name**: `datastorage` → `data-storage-service` (DNS fix)
2. ✅ **KUBECONFIG**: Removed environment variable leak (pod crashes)
3. ✅ **Service Readiness**: Added `waitForDataStorageServicesReady()` (timing fix)
4. ✅ **Port Number**: 8080 → 8081 (THIS FIX)

**Authority**: 
- GATEWAY_E2E_COMPLETE_FIX_JAN_29_2026.md (Fixes 1-3)
- This document (Fix 4)

---

## ✅ Validation

### Test Results
- ✅ All 98 tests passing (100%)
- ✅ All audit tests passing (was 0/9, now 9/9)
- ✅ No authentication errors
- ✅ No DNS errors
- ✅ No timeout errors

### Audit Emission Verified
```
✅ signal.received events created
✅ signal.deduplicated events created
✅ crd.created events created
✅ Events queryable from DataStorage API
✅ ActorID captured (currently "external", will be ServiceAccount after DD-AUTH-014 Phase 4)
```

---

## 📋 Files Modified

| File | Change | Lines |
|------|--------|-------|
| `test/e2e/gateway/gateway-deployment.yaml` | Port 8080 → 8081 | Line 36 |
| `test/infrastructure/gateway_e2e.go` | Port 8080 → 8081 | Line 1031 |

**Total Changes**: 2 files, 2 lines

---

## 🚀 Next Steps

### Immediate: Integration Tests (Next Task)
1. Run integration tests for all 9 services
2. Fix any DD-AUTH-014 related issues (DataStorage/HAPI have SAR, others don't)
3. Document findings

### Short-Term: Gateway SAR Auth Implementation (Post Integration Tests)
1. Implement SAR middleware for Gateway (BR-GATEWAY-182, BR-GATEWAY-183)
2. Update audit emission to capture ServiceAccount (ActorID)
3. Follow DD-AUTH-014 V2.0 pattern (same as DataStorage/HAPI)

---

## 🎯 Confidence Assessment

**Fix Confidence**: 100% (empirically validated - all tests pass)

**Root Cause Confidence**: 100% (port mismatch confirmed via logs + Service definition)

**Regression Risk**: Low (only affects Gateway E2E test configuration)

---

## 📚 Reference Documents

### Investigation Documents
- [GATEWAY_AUDIT_INVESTIGATION_JAN_29_2026.md](./GATEWAY_AUDIT_INVESTIGATION_JAN_29_2026.md) - Initial DNS/KUBECONFIG fixes
- [GATEWAY_E2E_COMPLETE_FIX_JAN_29_2026.md](./GATEWAY_E2E_COMPLETE_FIX_JAN_29_2026.md) - Infrastructure complete summary
- [E2E_COMPLETE_TRIAGE_GW_NT_RO_JAN_29_2026.md](./E2E_COMPLETE_TRIAGE_GW_NT_RO_JAN_29_2026.md) - ServiceAccount triage
- This document - Port fix

### Decision Documents
- [DD-AUTH-014 V2.0](../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) - Gateway SAR auth
- [DD-AUDIT-003](../architecture/decisions/DD-AUDIT-003-gateway-datastorage-audit-integration.md) - Audit integration
- [ADR-032](../architecture/decisions/ADR-032-data-access-layer-isolation.md) - P0 audit requirement

---

**Status**: ✅ **GATEWAY E2E AUDIT COMPLETE**  
**Test Health**: 100% (98/98 passing)  
**Next**: Run integration tests for all services  
**Duration**: ~15 min investigation + 3.5 min test run

---

**Author**: AI Assistant  
**Date**: January 29, 2026  
**Fixes**: 4 cumulative fixes over 2 sessions (DNS, KUBECONFIG, Readiness, Port)
