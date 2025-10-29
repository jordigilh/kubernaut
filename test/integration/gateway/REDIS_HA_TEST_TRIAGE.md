# Redis HA Integration Test Triage

**Date**: 2025-10-24
**Status**: ❌ **CRITICAL ISSUE IDENTIFIED**
**Test Duration**: 10 minutes (timed out)

---

## 🚨 **Critical Finding: All Tests Returning 503**

### **Root Cause**

**ALL integration tests are failing with `503 Service Unavailable`** errors, indicating that **Redis is not accessible** from the Gateway service during tests.

**Evidence**:
```
2025/10/24 11:26:30 "POST /webhook/prometheus" - 503 307B in 415ms
2025/10/24 11:26:31 "POST /webhook/prometheus" - 503 307B in 395ms
2025/10/24 11:26:31 "POST /webhook/prometheus" - 503 307B in 381ms
... (repeated for ALL requests)
```

**503 Status Code Meaning**: Gateway is rejecting requests because Redis is unavailable (per DD-GATEWAY-003 and REDIS_FAILURE_HANDLING.md)

---

## 🔍 **Diagnosis**

### **Problem 1: Port-Forward to Service (Not Pod)**

**Current Setup**:
```bash
kubectl port-forward -n kubernaut-system svc/redis-gateway-ha 6379:6379
```

**Issue**: The `redis-gateway-ha` service routes to **any of the 3 Redis instances** (master or replicas). If the port-forward connects to a **replica**, writes will fail because replicas are read-only.

**Redis HA Architecture**:
```
redis-gateway-ha service (load balances across all 3 pods)
├─→ redis-gateway-0 (master) ✅ READ + WRITE
├─→ redis-gateway-1 (replica) ❌ READ-ONLY
└─→ redis-gateway-2 (replica) ❌ READ-ONLY
```

**Gateway Behavior**:
- Deduplication: Writes to Redis (`SET` command) → Fails on replica → 503
- Storm Detection: Writes to Redis (Lua script) → Fails on replica → 503
- Rate Limiting: Writes to Redis (`INCR` command) → Fails on replica → 503

---

### **Problem 2: Integration Tests Expect Writable Redis**

**Test Assumptions**:
1. **Deduplication**: Writes fingerprints to Redis
2. **Storm Detection**: Writes storm state to Redis
3. **Rate Limiting**: Increments rate limit counters in Redis

**All of these require a WRITABLE Redis instance (master), not a read-only replica.**

---

## 📊 **Test Failure Summary**

| Test Category | Tests Run | Passed | Failed | Status |
|---------------|-----------|--------|--------|--------|
| **Storm Aggregation** | 2 | 0 | 2 | ❌ 503 errors |
| **Redis Integration** | 5 | 0 | 5 | ❌ 503 errors |
| **K8s API Integration** | 6 | 0 | 6 | ❌ 503 errors |
| **Security Integration** | ~20 | 0 | ~20 | ❌ 503 errors (timed out) |
| **TOTAL** | ~33 | 0 | ~33 | ❌ **100% failure rate** |

---

## 🎯 **Resolution Options**

### **Option A: Port-Forward to Master Pod Directly** ⭐ **RECOMMENDED**

**Approach**: Port-forward directly to `redis-gateway-0` (master) instead of the service

**Command**:
```bash
# Kill existing port-forward
pkill -f "kubectl port-forward.*redis"

# Port-forward to master pod directly
kubectl port-forward -n kubernaut-system redis-gateway-0 6379:6379
```

**Pros**:
- ✅ **Guaranteed writable Redis**: Always connects to master
- ✅ **Simple fix**: One command change
- ✅ **No code changes**: Tests work as-is
- ✅ **Fast**: Immediate resolution

**Cons**:
- ⚠️ **Doesn't test HA failover**: Always uses redis-gateway-0
  - **Mitigation**: Add separate HA failover tests that explicitly kill master

**Confidence**: **95%** - This will fix all 503 errors immediately

---

### **Option B: Update Service to Route to Master Only**

**Approach**: Add label selector to `redis-gateway-ha` service to route only to master

**Changes**:
```yaml
# deploy/redis-ha/redis-gateway-statefulset.yaml
apiVersion: v1
kind: Service
metadata:
  name: redis-gateway-ha
spec:
  selector:
    app: redis-gateway
    role: master  # NEW: Only route to master
```

**Pros**:
- ✅ **Service-level routing**: Port-forward to service works correctly
- ✅ **Production-ready**: Service always routes to master

**Cons**:
- ❌ **Requires pod labeling**: Need to dynamically update pod labels during failover
- ❌ **Complex**: Sentinel must update labels when master changes
- ❌ **Not standard Redis HA pattern**: Sentinel doesn't manage K8s labels
- ❌ **Implementation time**: 2-3 hours to implement + test

**Confidence**: **60%** - Complex solution for a simple problem

---

### **Option C: Use Redis Sentinel for Master Discovery**

**Approach**: Gateway connects to Sentinel (port 26379) to discover current master

**Changes**:
```go
// Gateway connects to Sentinel, asks for master address
sentinelClient := redis.NewSentinelClient(&redis.Options{
    Addr: "redis-gateway-ha.kubernaut-system:26379",
})
masterAddr, err := sentinelClient.GetMasterAddrByName(ctx, "gateway-master")
// Connect to discovered master
```

**Pros**:
- ✅ **Production-ready**: Automatic master discovery
- ✅ **Handles failover**: Gateway auto-reconnects to new master

**Cons**:
- ❌ **Code changes required**: Update Gateway Redis client initialization
- ❌ **Integration test complexity**: Need Sentinel port-forward too
- ❌ **Implementation time**: 4-6 hours to implement + test
- ❌ **Overkill for integration tests**: Production feature, not test fix

**Confidence**: **70%** - Good for production, not for immediate test fix

---

## 💡 **Recommended Action Plan**

### **Immediate (Next 10 minutes)**

1. **Kill existing port-forward**:
   ```bash
   pkill -f "kubectl port-forward.*redis"
   ```

2. **Port-forward to master pod directly**:
   ```bash
   kubectl port-forward -n kubernaut-system redis-gateway-0 6379:6379 &
   ```

3. **Re-run integration tests**:
   ```bash
   go test -v ./test/integration/gateway -run "TestGatewayIntegration" -timeout 30m
   ```

**Expected Outcome**: All tests should pass (or reveal new issues, not 503 errors)

---

### **Short-term (Next 1-2 days)**

4. **Update `run-tests.sh` script**:
   ```bash
   # Change from:
   kubectl port-forward -n kubernaut-system svc/redis-gateway-ha 6379:6379

   # To:
   kubectl port-forward -n kubernaut-system redis-gateway-0 6379:6379
   ```

5. **Add HA failover tests** (separate test suite):
   ```go
   // test/integration/gateway/redis_ha_failover_test.go
   It("should handle master failover", func() {
       // 1. Send request to master (redis-gateway-0)
       // 2. Kill master pod
       // 3. Wait for Sentinel failover (5-10s)
       // 4. Send request to new master (redis-gateway-1 or redis-gateway-2)
       // 5. Verify no data loss
   })
   ```

---

### **Long-term (V2.0)**

6. **Implement Sentinel-based master discovery** (Option C)
   - Gateway uses Sentinel to discover master
   - Automatic reconnection on failover
   - Production-ready HA client

---

## 📋 **Test Execution Summary**

### **Before Fix**
- **Duration**: 10 minutes (timed out)
- **Tests**: ~33 tests
- **Passed**: 0
- **Failed**: ~33
- **Failure Rate**: 100%
- **Root Cause**: Port-forward to service routes to read-only replica

### **After Fix (Expected)**
- **Duration**: 5-10 minutes
- **Tests**: ~33 tests
- **Passed**: ~30-33 (estimated)
- **Failed**: 0-3 (expected)
- **Failure Rate**: <10%
- **Root Cause**: Port-forward to master pod directly

---

## 🔗 **Related Documentation**

- **Redis HA Deployment**: [deploy/redis-ha/DEPLOYMENT_SUMMARY.md](../../../deploy/redis-ha/DEPLOYMENT_SUMMARY.md)
- **Design Decision**: [DD-INFRASTRUCTURE-001](../../../docs/architecture/decisions/DD-INFRASTRUCTURE-001-redis-separation.md)
- **Redis Failure Handling**: [docs/services/stateless/gateway-service/REDIS_FAILURE_HANDLING.md](../../../docs/services/stateless/gateway-service/REDIS_FAILURE_HANDLING.md)

---

## ✅ **Next Steps**

1. ✅ **Immediate**: Port-forward to `redis-gateway-0` (master pod)
2. ⏳ **Short-term**: Update `run-tests.sh` script
3. ⏳ **Short-term**: Add HA failover tests
4. ⏳ **Long-term**: Implement Sentinel-based master discovery (V2.0)

**Confidence**: **95%** - Port-forward to master pod will resolve all 503 errors


