# Context API Common Pitfalls & Lessons Learned

**Version**: 1.0  
**Last Updated**: 2025-11-02  
**Purpose**: Document anti-patterns and mistakes to avoid based on migration experience

---

## 🚨 **CRITICAL PITFALLS - MUST AVOID**

### ❌ **Pitfall #1: Wrapping RFC 7807 Errors with `fmt.Errorf`**

**What NOT to Do**:
```go
// ❌ WRONG: Wraps RFC7807Error, breaking type assertion
return nil, 0, fmt.Errorf("Data Storage unavailable: %w", rfc7807Err)
```

**Why It's Wrong**:
- `fmt.Errorf` creates a new error type, losing RFC 7807 structured fields
- Consumers can't access `type`, `title`, `detail`, `status`, `instance`
- Error handling logic breaks (e.g., retry vs. fail fast)

**Correct Approach**:
```go
// ✅ CORRECT: Return RFC7807Error directly to preserve type
return nil, 0, rfc7807Err
```

**Detection**:
```go
// Test that validates error type preservation
rfc7807Err, ok := contextapierrors.IsRFC7807Error(err)
Expect(ok).To(BeTrue(), "error should be RFC7807Error type")
```

**Impact**: P0 - Breaks structured error handling in production

---

### ❌ **Pitfall #2: Testing Only Behavior, Not Correctness**

**What NOT to Do**:
```go
// ❌ WRONG: Tests pagination WORKS but not that metadata is ACCURATE
It("should return paginated results", func() {
    incidents, total, err := executor.ListIncidents(ctx, params)
    Expect(err).ToNot(HaveOccurred())  // ✅ Behavior
    Expect(incidents).To(HaveLen(10))  // ✅ Behavior
    Expect(total).To(BeNumerically(">", 0))  // ❌ Weak assertion!
})
```

**Why It's Wrong**:
- Test passes even if `total = 10` (page size) instead of `10000` (database count)
- Pagination UI would show "Page 1 of 1" instead of "Page 1 of 1000"
- Real-world bug: Data Storage pagination bug (handler.go:178)

**Correct Approach**:
```go
// ✅ CORRECT: Test both behavior AND correctness
It("should return accurate pagination metadata", func() {
    // Known dataset: 25 records in database
    incidents, total, err := executor.ListIncidents(ctx, params)
    
    // BEHAVIOR: Pagination works
    Expect(err).ToNot(HaveOccurred())
    Expect(incidents).To(HaveLen(10), "page size should be 10")
    
    // CORRECTNESS: Pagination metadata is accurate
    Expect(total).To(Equal(25), 
        "total MUST equal database count (25), not page size (10)")
})
```

**Testing Principle**:
> **Always test both:**  
> 1. **Behavior**: Does the feature work?  
> 2. **Correctness**: Are the outputs accurate?

**Impact**: P0 - Missed critical pagination bug for 3 weeks

**Reference**: [Testing Strategy - Critical Principle](../../../../rules/03-testing-strategy.mdc#critical-principle-test-both-behavior-and-correctness)

---

### ❌ **Pitfall #3: Not Validating Cache Content After Serialization**

**What NOT to Do**:
```go
// ❌ WRONG: Tests cache hit but not data accuracy
It("should return cached data", func() {
    incidents, total, err := executor.ListIncidents(ctx, params)
    Expect(err).ToNot(HaveOccurred())
    Expect(incidents).ToNot(BeEmpty())  // ❌ Weak assertion!
})
```

**Why It's Wrong**:
- Cache could return corrupted data and test passes
- JSON serialization/deserialization bugs missed
- Field mapping issues undetected

**Correct Approach**:
```go
// ✅ CORRECT: Validate ALL fields are preserved
It("should return cached data with ALL fields accurate", func() {
    // ... force cache hit ...
    
    incident := incidents[0]
    Expect(incident.ID).To(Equal(int64(42)))
    Expect(incident.Name).To(Equal("HighMemoryUsage"))
    Expect(incident.Namespace).To(Equal("production"))
    // ... validate ALL 15+ fields ...
})
```

**Impact**: P0 - Could serve corrupt data to users

---

### ❌ **Pitfall #4: Skipping Circuit Breaker Recovery Tests**

**What NOT to Do**:
```go
// ❌ WRONG: Only tests circuit breaker opening
It("should open circuit breaker after 3 failures", func() {
    // ... trigger 3 failures ...
    Expect(circuitOpen).To(BeTrue())
})

// ⚠️ SKIPPED: Circuit breaker closing test
// PIt("should close circuit breaker after timeout", func() { ... })
```

**Why It's Wrong**:
- Production circuit breaker could stay permanently open
- No test validates auto-recovery mechanism
- System stays degraded forever after transient failure

**Correct Approach**:
```go
// ✅ CORRECT: Test full circuit breaker lifecycle
It("should close circuit breaker after timeout and allow requests", func() {
    // 1. Open circuit (3 failures)
    // ... trigger failures ...
    
    // 2. Wait for timeout
    time.Sleep(2100 * time.Millisecond)
    
    // 3. Verify circuit closes (half-open)
    _, _, err := executor.ListIncidents(ctx, params)
    Expect(err).ToNot(HaveOccurred(), 
        "circuit breaker should have closed after timeout")
})
```

**Impact**: P1 - Service stays degraded indefinitely in production

---

## 🟡 **IMPORTANT PITFALLS - HIGH RISK**

### ❌ **Pitfall #5: Using `NoOpCache` in Production**

**What NOT to Do**:
```go
// ❌ WRONG: NoOpCache silently fails in production
cache := cache.NoOpCache{}
executor := query.NewCachedExecutor(cache, ...)
```

**Why It's Wrong**:
- All cache operations silently fail
- 100% cache misses → high DB load
- No error indication

**Correct Approach**:
```go
// ✅ CORRECT: Use real cache with graceful degradation
cacheManager, err := cache.NewCacheManager(cfg, logger)
if err != nil {
    return nil, fmt.Errorf("cache initialization failed: %w", err)
}

executor := query.NewCachedExecutor(cacheManager, ...)
```

**Configuration**:
```yaml
cache:
  redisAddr: "redis-service:6379"
  lruSize: 10000
  defaultTTL: 5m
```

**Impact**: P1 - Performance degradation (10x slower queries)

---

### ❌ **Pitfall #6: Ignoring Field Mapping Completeness**

**What NOT to Do**:
```go
// ❌ WRONG: Partial field mapping
func convertIncident(inc *dsclient.Incident) *models.IncidentEvent {
    return &models.IncidentEvent{
        ID:   inc.Id,
        Name: inc.AlertName,
        // ... missing 15+ fields ...
    }
}
```

**Why It's Wrong**:
- Loses critical data (namespace, cluster, duration, error messages)
- Downstream services get incomplete context
- Debugging becomes impossible

**Correct Approach**:
```go
// ✅ CORRECT: Map ALL relevant fields
func convertIncident(inc *dsclient.Incident) *models.IncidentEvent {
    return &models.IncidentEvent{
        ID:                   inc.Id,
        Name:                 inc.AlertName,
        Namespace:            stringPtrToString(inc.Namespace),
        ClusterName:          stringPtrToString(inc.ClusterName),
        Environment:          stringPtrToString(inc.Environment),
        TargetResource:       stringPtrToString(inc.TargetResource),
        AlertFingerprint:     stringPtrToString(inc.AlertFingerprint),
        RemediationRequestID: stringPtrToString(inc.RemediationRequestId),
        // ... ALL 18 fields mapped ...
    }
}
```

**Validation Test**:
```go
// Test that validates ALL fields are mapped
It("should map ALL Data Storage API fields", func() {
    // ... mock with all fields ...
    incident := incidents[0]
    Expect(incident.ID).To(Equal(int64(42)))
    // ... assert ALL 18 fields ...
})
```

**Impact**: P1 - Data loss in production

---

### ❌ **Pitfall #7: Hardcoded Circuit Breaker Timeout in Tests**

**What NOT to Do**:
```go
// ❌ WRONG: Tests use production timeout (60s)
executor := query.NewCachedExecutor(...)
// Circuit breaker timeout: 60s (hardcoded in executor)

time.Sleep(61 * time.Second)  // ❌ Test takes 61s!
```

**Why It's Wrong**:
- Test suite takes minutes to run
- CI/CD becomes unbearably slow
- Developers skip tests locally

**Correct Approach**:
```go
// ✅ CORRECT: Make timeout configurable for testing
cfg := &query.DataStorageExecutorConfig{
    DSClient:                dsClient,
    CircuitBreakerThreshold: 3,
    CircuitBreakerTimeout:   2 * time.Second,  // ⭐ Test-friendly
}
executor, err := query.NewCachedExecutorWithDataStorage(cfg)

time.Sleep(2100 * time.Millisecond)  // ✅ Test takes 2s
```

**Impact**: P2 - Test suite becomes unusable

---

## 🟢 **MINOR PITFALLS - BEST PRACTICES**

### ❌ **Pitfall #8: Inconsistent Package Naming in Tests**

**What NOT to Do**:
```go
// ❌ WRONG: Black-box testing (package name_test)
package contextapi_test

import "github.com/jordigilh/kubernaut/pkg/contextapi"
```

**Why It's Wrong**:
- Inconsistent with project convention (white-box testing)
- Can't access internal fields for validation
- Forces inefficient test patterns

**Correct Approach**:
```go
// ✅ CORRECT: White-box testing (same package)
package contextapi

import "testing"
```

**Project Convention**: All tests use white-box testing (no `_test` suffix)

**Impact**: P3 - Test maintainability

---

### ❌ **Pitfall #9: Duplicate Test Code for Similar Scenarios**

**What NOT to Do**:
```go
// ❌ WRONG: Copy-paste tests for each filter
It("should pass namespace filters", func() {
    // 25 lines of test code
})

It("should pass severity filters", func() {
    // 25 lines of nearly identical test code
})

// Result: 50+ lines for 2 scenarios
```

**Correct Approach**:
```go
// ✅ CORRECT: Table-driven tests with DescribeTable
DescribeTable("Filter parameter passing",
    func(tc filterTestCase) {
        // 20 lines of shared logic
    },
    Entry("namespace filter", filterTestCase{...}),  // 5 lines
    Entry("severity filter", filterTestCase{...}),   // 5 lines
)

// Result: 30 lines for 2 scenarios (40% reduction)
```

**Benefits**:
- Less duplication (DRY principle)
- Easier to add new scenarios (1 Entry instead of 25 lines)
- Consistent test patterns

**Impact**: P3 - Code maintainability

---

## 📊 **Pitfall Detection Checklist**

Before merging code, verify:

### **RFC 7807 Error Handling**:
- [ ] No `fmt.Errorf` wrapping of RFC7807Error
- [ ] Error type preserved through all layers
- [ ] Tests validate structured error fields

### **Testing Quality**:
- [ ] Tests validate behavior AND correctness
- [ ] Pagination metadata accuracy tested
- [ ] Cache content accuracy validated (all fields)
- [ ] Circuit breaker recovery tested (not just opening)

### **Configuration**:
- [ ] Real cache configured (not NoOpCache)
- [ ] Circuit breaker timeout configurable for tests
- [ ] All environment-specific values in ConfigMap

### **Field Mapping**:
- [ ] ALL Data Storage API fields mapped
- [ ] Test validates completeness (18 fields)
- [ ] Null/optional fields handled correctly

### **Code Quality**:
- [ ] Package naming consistent (white-box testing)
- [ ] Repetitive tests refactored with DescribeTable
- [ ] No hardcoded timeouts in tests

---

## 📚 **Related Documents**

- [Testing Strategy - Behavior vs. Correctness](../../../../rules/03-testing-strategy.mdc)
- [Data Storage Common Pitfalls](../data-storage/implementation/IMPLEMENTATION_PLAN_V4.4.md#common-pitfalls)
- [Context API Test Gaps Fixed](implementation/CONTEXT-API-TEST-GAPS-FIXED.md)
- [Data Storage Integration Test Triage](../data-storage/implementation/DATA-STORAGE-INTEGRATION-TEST-TRIAGE.md)

---

## 🎯 **Key Lessons**

1. **Test Correctness, Not Just Behavior**: The pagination bug taught us that "works" ≠ "accurate"
2. **Preserve Error Types**: RFC 7807 errors are only useful if type-preserved
3. **Validate Complete Data Flow**: Cache serialization, field mapping, error propagation
4. **Make Tests Fast**: Configurable timeouts enable fast test suites
5. **Use Table-Driven Tests**: Reduces duplication, improves maintainability

---

**Document Status**: ✅ Production-Ready  
**Confidence**: 95% - Based on real migration experience  
**Last Updated**: 2025-11-02

