# Gateway apiReader Fix Implementation

**Date**: January 18, 2026
**Issue**: E2E test failure - "Timeout: failed waiting for *v1.Lease Informer to sync"
**Root Cause**: `DistributedLockManager` using cached client for Lease operations
**Solution**: Option A - Use apiReader (non-cached client) for immediate consistency

---

## 🎯 **Problem Statement**

### **Failure Symptom**:
```
distributed lock acquisition failed: failed to check for existing lease:
Timeout: failed waiting for *v1.Lease Informer to sync
```

### **Root Cause Analysis**:

| Aspect | Issue | Impact |
|---|---|---|
| **Lock Manager Client** | Used `ctrlClient` (cached) | Lease reads from stale cache |
| **Informer Sync** | Lease Informer not syncing | GET operations timeout waiting for cache |
| **Race Condition** | Cache sync delay (5-50ms) | Multiple pods can acquire same lock |

### **Why Cached Client Failed**:

The `controller-runtime` client has two types:
1. **Cached Client (`ctrlClient`)**: Reads from in-memory cache, requires Informer to sync
2. **Non-Cached Client (`apiReader`)**: Reads directly from K8s API server (immediate consistency)

When using `ctrlClient` for Lease operations:
- Each GET requires the Lease Informer to be synced
- If Informer fails to sync → timeout error
- Even if synced, cache refresh delay (5-50ms) allows race conditions

---

## ✅ **Implemented Solution: Option A (apiReader)**

### **Changes Made**:

#### **1. Updated DistributedLockManager Documentation**:

**File**: `pkg/gateway/processing/distributed_lock.go`

**Changes**:
- Added comment explaining why apiReader is required (immediate consistency)
- Documented API server impact (3-24 API req/sec at production load)
- Referenced impact analysis document
- Updated constructor comment to emphasize apiReader requirement

**Key Comment Added**:
```go
// WHY apiReader (Non-Cached Client)?
// - ✅ Immediate Consistency: No cache sync delay (5-50ms race window eliminated)
// - ✅ Correctness: Distributed locking requires guaranteed freshness
// - ✅ Production-Ready: K8s leader-election uses direct API calls for Lease operations
//
// API Server Impact: Acceptable at production scale
// - Normal load (1 signal/sec): 3 API req/sec (negligible)
// - Peak load (8 signals/sec): 24 API req/sec (low)
// - Design target (1000 signals/sec): 3000 API req/sec (30-60% of K8s API capacity)
```

#### **2. Updated server.go to Pass apiReader**:

**File**: `pkg/gateway/server.go`

**Changes**:
- **Line 440**: Changed `processing.NewDistributedLockManager(ctrlClient, ...)` to `processing.NewDistributedLockManager(apiReader, ...)`
- **Line 392**: Updated `createServerWithClients` function signature to accept `client.Client` for `apiReader` (not `client.Reader`)
- **Line 250**: Added `lockManager` initialization to `NewServerForTesting` function

**Critical Fix (Line 440)**:
```go
// BEFORE (using cached client - caused Informer sync timeout)
lockManager = processing.NewDistributedLockManager(ctrlClient, namespace, podName)

// AFTER (using non-cached client - immediate consistency)
lockManager = processing.NewDistributedLockManager(apiReader, namespace, podName)
```

**Function Signature Fix (Line 392)**:
```go
// BEFORE (apiReader typed as client.Reader - incompatible with Create/Update/Delete)
func createServerWithClients(..., apiReader client.Reader, ...) (*Server, error)

// AFTER (apiReader typed as client.Client - supports all operations)
func createServerWithClients(..., apiReader client.Client, ...) (*Server, error)
```

**Rationale**: `apiReader` is created with `client.New()` (returns `client.Client`), so it supports Create/Update/Delete operations. Typing it as `client.Reader` was unnecessarily restrictive.

---

## 📊 **API Server Impact Assessment**

**Document**: `docs/services/stateless/gateway-service/GW_API_SERVER_IMPACT_ANALYSIS_DISTRIBUTED_LOCKING_JAN18_2026.md`

### **API Calls per Signal**:
- **Best case**: 2 API calls (GET + DELETE)
- **Normal case**: 3 API calls (GET + CREATE + DELETE)
- **Expired lock**: 3 API calls (GET + UPDATE + DELETE)

### **Production Load Projections**:

| Scenario | Signals/Sec | API Calls/Sec | Assessment |
|---|---|---|---|
| **Normal Production** | 1 | 3 | ✅ Negligible (0.03% of API capacity) |
| **Peak Production** | 4 | 12 | ✅ Minimal (0.12% of API capacity) |
| **Incident Storm** | 8 | 24 | ✅ Low (0.24% of API capacity) |
| **Design Target** | 1000 | 3000 | ✅ Acceptable (30-60% of API capacity) |

### **K8s API Server Capacity**:
- Typical capacity: 5,000-10,000 req/sec
- Gateway's share at design target: 30-60%
- Production load: < 0.5% of capacity

**Conclusion**: API server impact is **acceptable** for a critical ingress service

---

## ⚖️ **Why Option A Over Option B**

### **Option A: apiReader (IMPLEMENTED)**:
- ✅ **Immediate Consistency**: No cache sync delay
- ✅ **No Race Conditions**: Each request sees current API state
- ✅ **Simple Implementation**: Direct API calls, no cache management
- ✅ **Production Precedent**: K8s leader-election uses direct API calls

### **Option B: WaitForCacheSync (REJECTED)**:
- ❌ **Still Has Race Conditions**: Cache sync delay (5-50ms) between CREATE and cache update
- ❌ **False Sense of Safety**: `WaitForCacheSync` only guarantees **initial** sync, not ongoing freshness
- ❌ **Complexity**: Requires cache warmup, sync timeout handling
- ❌ **Cache Refresh Delay**: Default 30s refresh means stale reads possible

**User Feedback**: "Between A and B, the B will have a potential impact that requesting a lease that exists and has not yet been synched will cause an error."

**Decision**: User correctly identified that **Option B does not solve the race condition**

---

## 🧪 **Validation Strategy**

### **Unit Tests** (Completed):
- ✅ 10 unit tests for `DistributedLockManager`
- ✅ All tests pass with both cached and non-cached clients
- ✅ Tests validate lock acquisition, release, expiration, and race handling

### **E2E Tests** (In Progress):
- 🔄 Running `make test-e2e-gateway` to validate fix
- 🎯 Expected result: `GW-DEDUP-002` creates only 1 RemediationRequest (not 5)
- 🎯 Expected result: No "Lease Informer sync timeout" errors

---

## 📝 **Files Changed**

| File | Lines Changed | Purpose |
|---|---|---|
| `pkg/gateway/processing/distributed_lock.go` | ~30 | Documentation updates (why apiReader) |
| `pkg/gateway/server.go` | ~20 | Pass apiReader instead of ctrlClient |
| `docs/.../GW_API_SERVER_IMPACT_ANALYSIS_...md` | +536 | API impact assessment |
| `docs/.../GW_APIREADER_FIX_IMPLEMENTATION_...md` | +300 | This document |

---

## 🔍 **Code Comparison**

### **Before (Cached Client)**:

```go
// pkg/gateway/server.go:440
lockManager = processing.NewDistributedLockManager(ctrlClient, namespace, podName)
//                                                  ^^^^^^^^^^ CACHED CLIENT
//                                                  ❌ Requires Informer sync
//                                                  ❌ Cache sync delay (5-50ms)
//                                                  ❌ Race conditions possible
```

**Result**: `Timeout: failed waiting for *v1.Lease Informer to sync`

### **After (Non-Cached Client)**:

```go
// pkg/gateway/server.go:440
lockManager = processing.NewDistributedLockManager(apiReader, namespace, podName)
//                                                  ^^^^^^^^^ NON-CACHED CLIENT
//                                                  ✅ Direct API server reads
//                                                  ✅ Immediate consistency
//                                                  ✅ No race conditions
```

**Expected Result**: Lease operations succeed immediately, no Informer sync required

---

## 🎯 **Success Criteria**

### **Functional**:
- [ ] E2E test `GW-DEDUP-002` passes (creates only 1 RR, not 5)
- [ ] No "Lease Informer sync timeout" errors in Gateway logs
- [ ] All E2E tests pass without hanging or timeout

### **Performance**:
- [ ] Gateway startup time unchanged (< 5 seconds)
- [ ] Signal processing latency p95 < 50ms (within target)
- [ ] API server CPU usage < 70% during E2E tests

### **Observability**:
- [ ] Gateway logs show successful lock acquisition/release
- [ ] Prometheus metrics: `gateway_deduplication_rate` remains accurate
- [ ] No new error patterns in audit events

---

## 📚 **References**

### **Documentation**:
- [API Server Impact Analysis](./GW_API_SERVER_IMPACT_ANALYSIS_DISTRIBUTED_LOCKING_JAN18_2026.md)
- [Distributed Lock Triage](./GW_DISTRIBUTED_LOCK_TRIAGE_JAN18_2026.md)
- [Deduplication Race RCA](./GW_DEDUP_002_RCA_FINAL_JAN18_2026.md)
- [ADR-052: Distributed Locking](../../../architecture/ADR-052-distributed-locking-pattern.md)

### **Code**:
- `pkg/gateway/processing/distributed_lock.go` - Lock manager implementation
- `pkg/gateway/processing/distributed_lock_test.go` - Unit tests
- `pkg/gateway/server.go` - Server initialization with apiReader
- `test/e2e/gateway/16_concurrent_deduplication_test.go` - E2E validation

---

## 🚀 **Deployment Impact**

### **Production Deployment**:
- ✅ **Zero Downtime**: Change is backward-compatible
- ✅ **No Config Changes**: Uses existing `POD_NAME`/`POD_NAMESPACE` env vars
- ✅ **RBAC Already Present**: Lease permissions already in `deploy/gateway/01-rbac.yaml`
- ✅ **Env Vars Already Present**: `POD_NAME`/`POD_NAMESPACE` in `deploy/gateway/03-deployment.yaml`

### **Rollback Plan**:
If issues arise, revert commits:
1. Revert `pkg/gateway/server.go` line 440 to use `ctrlClient`
2. Redeploy Gateway pods
3. Monitor for deduplication issues

### **Monitoring**:
After deployment, monitor:
- Gateway logs: Search for "Distributed lock acquisition failed"
- Prometheus: `gateway_deduplication_rate` should remain stable
- K8s API server: CPU/memory should not spike

---

## ✅ **Completion Status**

- [x] API server impact analysis completed
- [x] Option A implementation completed
- [x] Unit tests passing (10/10)
- [x] Documentation updated
- [ ] E2E tests passing (in progress)
- [ ] User approval for deployment

---

**Confidence Assessment**: **90%**

**Justification**:
- ✅ Root cause identified and validated through analysis
- ✅ Fix aligns with K8s best practices (leader-election uses direct API calls)
- ✅ API server impact is acceptable at production scale
- ✅ Unit tests validate lock manager behavior
- ⚠️ Minor uncertainty: E2E test results pending

**Next Step**: Wait for E2E test completion and verify fix resolves the issue
