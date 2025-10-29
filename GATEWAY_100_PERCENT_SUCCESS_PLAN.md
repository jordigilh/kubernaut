# Gateway 100% Test Success Plan

**Goal**: Achieve 100% test success for unit and integration tests + comprehensive edge case coverage  
**Priority**: CRITICAL - Gateway is the entry point, must be rock-solid and stable  
**Current Status**: 9/55 passing (16%), 17/17 unit tests passing (100%)

---

## 🎯 **MISSION: GATEWAY ROBUSTNESS**

**Why This Matters**:
- Gateway is the **critical entry point** for all signals
- Must handle **all edge cases** gracefully
- Must be **stable** under all conditions
- Must provide **clear error messages** for debugging

---

## 📊 **CURRENT STATE ANALYSIS**

### **Test Results**
```
Unit Tests:        17/17 PASSING (100%) ✅
Integration Tests:  9/55 PASSING (16%)  ⚠️
Total:            26/72 PASSING (36%)   ⚠️
```

### **Failure Categories** (46 failing tests)

#### **Category 1: Redis OOM (Out of Memory)** - 🔴 **HIGH PRIORITY**
**Count**: ~5-10 tests  
**Error**: `OOM command not allowed when used memory > 'maxmemory'`  
**Impact**: Tests fail randomly when Redis runs out of memory  
**Root Cause**: Redis container has 256MB limit, tests don't clean up

**Fix**:
1. Add `FLUSHDB` in `AfterEach` for all test suites
2. Increase Redis memory limit to 512MB
3. Add memory monitoring to detect leaks

---

#### **Category 2: HTTP 500 Errors** - 🔴 **HIGH PRIORITY**
**Count**: ~15-20 tests  
**Error**: Expected 201 Created, got 500 Internal Server Error  
**Impact**: Core business flow broken  
**Root Cause**: Unknown - needs investigation

**Examples**:
- `error_handling_test.go:103` - Malformed JSON handling
- `error_handling_test.go:159` - Large payload handling
- `webhook_integration_test.go` - CRD creation

**Fix**:
1. Run failing tests individually with verbose logging
2. Check Gateway logs for error details
3. Fix business logic errors
4. Add better error handling

---

#### **Category 3: Business Logic Mismatches** - 🟡 **MEDIUM PRIORITY**
**Count**: ~10-15 tests  
**Error**: Expected X, got Y (counts, values, etc.)  
**Impact**: Tests expect different behavior than implementation  
**Root Cause**: Test expectations vs actual business logic

**Examples**:
- Storm aggregation: Expected 2 resources, got 1
- Health endpoint: Expected timestamp field missing
- Concurrent requests: Expected all success, got failures

**Fix**:
1. Review each test expectation
2. Determine if test or implementation is wrong
3. Fix whichever is incorrect
4. Add edge case tests

---

#### **Category 4: Error Handling Edge Cases** - 🟡 **MEDIUM PRIORITY**
**Count**: ~5 tests  
**Error**: Tests for error handling not working correctly  
**Impact**: Error handling not robust  
**Root Cause**: Error handling code needs improvement

**Examples**:
- Malformed JSON should return 400, returns 500
- Large payloads should be rejected, accepted instead
- Missing fields should return clear error, returns generic error

**Fix**:
1. Implement proper input validation
2. Add size limits
3. Return appropriate HTTP status codes
4. Add clear error messages

---

## 🔧 **EXECUTION PLAN**

### **Phase 1: Infrastructure Stabilization** (30-45 minutes)

**Goal**: Fix Redis OOM and test cleanup

**Tasks**:
1. ✅ Add `AfterEach` cleanup to all test suites
2. ✅ Increase Redis memory limit
3. ✅ Verify tests run without OOM errors

**Success Criteria**: No more Redis OOM errors

---

### **Phase 2: HTTP 500 Error Investigation** (1-2 hours)

**Goal**: Fix all HTTP 500 errors

**Tasks**:
1. ✅ Run each failing test individually
2. ✅ Capture Gateway error logs
3. ✅ Fix business logic errors
4. ✅ Add proper error handling

**Success Criteria**: All tests expecting 201/202 get correct status

---

### **Phase 3: Business Logic Alignment** (1-2 hours)

**Goal**: Align test expectations with implementation

**Tasks**:
1. ✅ Review storm aggregation logic
2. ✅ Fix health endpoint response
3. ✅ Fix concurrent request handling
4. ✅ Update tests or implementation as needed

**Success Criteria**: All business logic tests passing

---

### **Phase 4: Edge Case Coverage** (2-3 hours)

**Goal**: Add comprehensive edge case tests

**Tasks**:
1. ✅ Add unit test edge cases (70% tier)
   - Invalid inputs
   - Boundary conditions
   - Error conditions
   - Concurrent access

2. ✅ Add integration test edge cases (>50% tier)
   - Redis failures
   - K8s API failures
   - Network timeouts
   - Resource limits

3. ✅ Add E2E test edge cases (10-15% tier)
   - Complete failure scenarios
   - Recovery scenarios
   - Load conditions

**Success Criteria**: Comprehensive edge case coverage at all tiers

---

## 📋 **DETAILED TASK BREAKDOWN**

### **Task 1: Fix Redis OOM** ⏰ 30 minutes

**Subtasks**:
1. Add `AfterEach` to `suite_test.go`:
```go
AfterEach(func() {
    if redisClient != nil && redisClient.Client != nil {
        _ = redisClient.Client.FlushDB(ctx).Err()
    }
})
```

2. Increase Redis memory in docker command:
```bash
podman run -d --name redis-gateway \
  -p 6379:6379 \
  --memory=512m \
  redis:7-alpine redis-server --maxmemory 512mb --maxmemory-policy allkeys-lru
```

3. Verify fix:
```bash
go test ./test/integration/gateway -v -run "Redis" -timeout 5m
```

---

### **Task 2: Fix HTTP 500 Errors** ⏰ 1-2 hours

**Subtasks**:
1. Run first failing test with focus:
```bash
# Focus on one test
go test ./test/integration/gateway -v -run "malformed.*JSON" -timeout 5m
```

2. Capture Gateway logs:
```go
// Add to test
logger := zap.NewDevelopment()
server, _ := gateway.NewServer(cfg, logger)
```

3. Fix error handling in Gateway code

4. Repeat for each HTTP 500 error

---

### **Task 3: Add Edge Case Tests** ⏰ 2-3 hours

**Unit Test Edge Cases** (70% tier):
```go
Context("Edge Cases: Input Validation", func() {
    It("rejects empty payload", func() {})
    It("rejects payload > 1MB", func() {})
    It("handles special characters in labels", func() {})
    It("handles very long alert names (>253 chars)", func() {})
    It("handles Unicode in annotations", func() {})
})

Context("Edge Cases: Boundary Conditions", func() {
    It("handles exactly 63-char label values (K8s limit)", func() {})
    It("handles 64-char label values (should truncate)", func() {})
    It("handles empty annotations", func() {})
    It("handles nil labels map", func() {})
})

Context("Edge Cases: Concurrent Access", func() {
    It("handles concurrent Parse() calls safely", func() {})
    It("generates unique fingerprints under load", func() {})
})
```

**Integration Test Edge Cases** (>50% tier):
```go
Context("Edge Cases: Redis Failures", func() {
    It("handles Redis connection loss gracefully", func() {})
    It("handles Redis timeout gracefully", func() {})
    It("handles Redis OOM gracefully", func() {})
    It("recovers when Redis comes back", func() {})
})

Context("Edge Cases: K8s API Failures", func() {
    It("handles K8s API timeout", func() {})
    It("handles K8s API rate limiting", func() {})
    It("handles K8s API quota exceeded", func() {})
    It("handles CRD already exists", func() {})
})

Context("Edge Cases: Network Failures", func() {
    It("handles slow clients (timeout)", func() {})
    It("handles client disconnect mid-request", func() {})
    It("handles malformed HTTP headers", func() {})
})
```

---

## 🎯 **SUCCESS CRITERIA**

### **Phase 1 Complete**:
- ✅ No Redis OOM errors
- ✅ All tests run to completion
- ✅ Redis memory stable

### **Phase 2 Complete**:
- ✅ No HTTP 500 errors (unless expected)
- ✅ All CRD creation tests passing
- ✅ All webhook processing tests passing

### **Phase 3 Complete**:
- ✅ All business logic tests passing
- ✅ Storm aggregation working correctly
- ✅ Health endpoints working correctly

### **Phase 4 Complete**:
- ✅ 70%+ unit test coverage with edge cases
- ✅ >50% integration test coverage with edge cases
- ✅ 10-15% E2E test coverage with edge cases
- ✅ All edge cases documented

### **Final Success**:
- ✅ **100% unit tests passing**
- ✅ **100% integration tests passing**
- ✅ **Comprehensive edge case coverage**
- ✅ **Gateway is rock-solid and stable**

---

## 📊 **PROGRESS TRACKING**

| Phase | Tasks | Status | Time Estimate |
|-------|-------|--------|---------------|
| **Phase 1: Infrastructure** | Fix Redis OOM | 🔴 TODO | 30-45 min |
| **Phase 2: HTTP 500 Errors** | Fix business logic | 🔴 TODO | 1-2 hours |
| **Phase 3: Business Logic** | Align expectations | 🔴 TODO | 1-2 hours |
| **Phase 4: Edge Cases** | Add comprehensive tests | 🔴 TODO | 2-3 hours |
| **TOTAL** | | 🔴 TODO | **4-8 hours** |

---

## 🚀 **LET'S BEGIN**

**Next Action**: Start Phase 1 - Infrastructure Stabilization

**Command**:
```bash
# 1. Stop current Redis
podman stop redis-gateway && podman rm redis-gateway

# 2. Start Redis with more memory
podman run -d --name redis-gateway \
  -p 6379:6379 \
  --memory=512m \
  redis:7-alpine redis-server --maxmemory 512mb --maxmemory-policy allkeys-lru

# 3. Add AfterEach cleanup to suite_test.go
# 4. Run tests
go test ./test/integration/gateway -v -timeout 10m
```

**Ready to proceed?**


