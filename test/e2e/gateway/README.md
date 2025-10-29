# E2E Testing Tier - Gateway

**Purpose**: Test complete user workflows and production-like scenarios  
**Coverage Target**: <10% of total tests  
**Infrastructure**: Production-like environment, nightly test suite

---

## 🌐 **Tests to Implement** (5 tests)

### **Long-Running Tests** (1 test)

1. **Storm Window TTL Expiration** (`storm_ttl_expiration_test.go`)
   - Wait 2+ minutes for storm window TTL to expire
   - Send new alert after expiration
   - Verify new storm window created
   - Verify new CRD created (not aggregated)
   
   **Status**: ✅ Test is complete, just needs to be moved
   **Duration**: 2+ minutes
   **Recommendation**: Run in nightly E2E suite, not integration CI

### **Edge Case Tests** (3 tests)

2. **K8s API Rate Limiting** (`k8s_api_rate_limit_test.go`)
   - Simulate K8s API rate limiting (429 responses)
   - Verify Gateway backs off exponentially
   - Verify Gateway retries after backoff
   - Verify CRDs eventually created

3. **CRD Name Length Limit** (`crd_edge_cases_test.go`)
   - Send alert with very long namespace + alertname (>253 chars)
   - Verify CRD name truncated to 253 chars
   - Verify CRD created successfully
   - Verify fingerprint preserved in labels/annotations
   
   **Status**: ⚠️ **Borderline** - Could stay in integration if fast (<1s)
   **Recommendation**: Verify test speed, move if >1s

4. **Concurrent CRD Creates** (`concurrent_operations_test.go`)
   - Send multiple alerts simultaneously
   - Verify all CRDs created successfully
   - Verify no conflicts or race conditions
   - Verify all CRDs have unique names
   
   **Status**: ⚠️ **Depends on concurrency level**
   - If <10 concurrent: E2E
   - If 50+ concurrent: LOAD tier

### **CRD Lifecycle Tests** (1 test)

5. **Redis State Cleanup on CRD Deletion** (`crd_lifecycle_test.go`)
   - Create CRD with fingerprint in Redis
   - Delete CRD (kubectl delete or TTL expiration)
   - Verify fingerprint removed from Redis
   - Verify storm state removed from Redis
   - Verify no orphaned Redis keys
   
   **Status**: ❌ **DEFERRED** - Out of scope for Gateway v1.0
   **Requires**: CRD controller with finalizers
   **Estimated Effort**: 8-12 hours

---

## 🛠️ **Infrastructure Requirements**

### **E2E Environment**:
- Production-like Kubernetes cluster
- Production-like Redis setup
- Nightly test suite configuration
- Long-running test support (>5 minutes)

### **Edge Case Testing**:
- Rate limiting simulation (mock K8s API or proxy)
- Very long input generation
- Concurrent request framework

---

## 📊 **Estimated Effort**

- **E2E Infrastructure**: 5-6 hours
- **Edge Case Tests**: 5-8 hours
- **CRD Lifecycle** (deferred): 8-12 hours
- **Total**: 10-14 hours (excluding deferred)

---

## 🎯 **Success Criteria**

- All E2E tests passing in nightly suite
- Tests complete within 30 minutes
- Clear documentation for E2E scenarios
- Production-like environment setup

---

**Status**: 📋 **PENDING IMPLEMENTATION**  
**Priority**: **LOW-MEDIUM** - Important for edge case coverage  
**Next Step**: Build E2E infrastructure and nightly test suite

