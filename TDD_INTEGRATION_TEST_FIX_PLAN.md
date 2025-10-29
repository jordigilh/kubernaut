# TDD Integration Test Fix Plan

**Date**: 2025-10-29
**Status**: 🔄 **IN PROGRESS**
**TDD Principle**: Tests define business requirements → Implementation must match

---

## 🎯 **TDD Methodology**

### **Core Principle**
> **Tests are the specification. Implementation is wrong if tests fail.**

**NOT**: "Fix tests to match implementation"  
**YES**: "Fix implementation to match test requirements"

---

## 📊 **Current Status**

### Test Results (After Health Fix)
- ✅ **13 Passed** (+1 from health fix)
- ❌ **42 Failed** (need TDD analysis)
- ⏸️ **14 Pending** (deferred to Day 10+)
- ⏭️ **1 Skipped**

### TDD Success Example: Health Endpoint
**Test Expected**: `{"status": "healthy", "timestamp": "..."}`  
**Implementation Returned**: `{"status": "ok"}` ❌  
**TDD Fix**: Changed implementation to return `"healthy"` ✅  
**Result**: Test now passes

---

## 🔍 **Failure Categories (TDD Analysis Required)**

### **Category 1: Deduplication TTL (4 failures)**
**Business Requirement**: BR-GATEWAY-003
**Test Expectations**:
- 5-minute TTL for deduplication window
- Expired fingerprints treated as new alerts
- TTL refreshes on duplicate detection
- Duplicate count preserved until TTL expiration

**TDD Question**: Does implementation correctly implement 5-minute TTL?

---

### **Category 2: K8s API Integration (7 failures)**
**Business Requirement**: DAY 8 PHASE 3
**Test Expectations**:
- Create RemediationRequest CRD successfully
- Populate CRD with correct metadata
- Handle CRD name collisions
- Validate CRD schema before creation
- Handle K8s API temporary failures with retry
- Handle K8s API quota exceeded gracefully
- Handle watch connection interruption

**TDD Question**: Does implementation correctly handle all K8s API scenarios?

---

### **Category 3: Redis Integration (6 failures)**
**Business Requirement**: DAY 8 PHASE 2
**Test Expectations**:
- Persist deduplication state in Redis
- Expire deduplication entries after TTL
- Store storm detection state in Redis
- Handle concurrent Redis writes without corruption
- Handle Redis cluster failover without data loss
- Handle Redis memory eviction (LRU) gracefully

**TDD Question**: Does implementation correctly use Redis for state management?

---

### **Category 4: Prometheus Alert Processing (25 failures)**
**Business Requirement**: BR-GATEWAY-001, BR-GATEWAY-002, BR-GATEWAY-005
**Test Expectations**:
- Create RemediationRequest CRD with correct business metadata
- Extract resource information for AI targeting
- Prevent duplicate CRDs using fingerprint
- Handle Kubernetes Warning events
- Storm aggregation logic

**TDD Question**: Does implementation correctly process Prometheus alerts?

---

## 🛠️ **TDD Fix Strategy**

### **Phase 1: Analyze Test Requirements**
For each failing test:
1. Read test code to understand business requirement
2. Identify what the test expects (assertions)
3. Document the business outcome being validated

### **Phase 2: Compare with Implementation**
1. Trace implementation code path
2. Identify where implementation diverges from test expectations
3. Determine if implementation is missing logic or has bugs

### **Phase 3: Fix Implementation (Not Tests)**
1. Update implementation to match test requirements
2. Preserve business logic integrity
3. Ensure no regressions in passing tests

### **Phase 4: Verify Fix**
1. Run focused test to verify fix
2. Run full suite to check for regressions
3. Document fix rationale

---

## 📋 **Systematic Fix Process**

### **Step 1: Focus on One Test**
```bash
go test ./test/integration/gateway -v \
  -ginkgo.focus "treats expired fingerprint as new alert after 5-minute TTL" \
  -timeout 30s
```

### **Step 2: Analyze Test Expectations**
- Read test code
- Identify assertions
- Document business requirement

### **Step 3: Trace Implementation**
- Find implementation code
- Identify bug or missing logic
- Plan fix

### **Step 4: Fix Implementation**
- Update implementation code
- Add missing logic or fix bug
- Preserve existing behavior

### **Step 5: Verify Fix**
- Run focused test (should pass)
- Run full suite (no regressions)
- Document fix

### **Step 6: Commit Progress**
```bash
git add -A
git commit -m "fix(gateway): TDD fix for [test name] - [brief description]"
```

---

## 🎯 **Success Criteria**

### **Per-Test Success**
- ✅ Test passes after implementation fix
- ✅ No regressions in other tests
- ✅ Business requirement validated
- ✅ Fix documented

### **Overall Success**
- ✅ All 42 failing tests pass
- ✅ All 13 passing tests still pass
- ✅ No new failures introduced
- ✅ Implementation matches all test requirements

---

## 📊 **Progress Tracking**

### **Deduplication TTL (0/4 fixed)**
- [ ] treats expired fingerprint as new alert after 5-minute TTL
- [ ] uses configurable 5-minute TTL for deduplication window
- [ ] refreshes TTL on each duplicate detection
- [ ] preserves duplicate count until TTL expiration

### **K8s API Integration (0/7 fixed)**
- [ ] should create RemediationRequest CRD successfully
- [ ] should populate CRD with correct metadata
- [ ] should handle CRD name collisions
- [ ] should validate CRD schema before creation
- [ ] should handle K8s API temporary failures with retry
- [ ] should handle K8s API quota exceeded gracefully
- [ ] should handle watch connection interruption

### **Redis Integration (0/6 fixed)**
- [ ] should persist deduplication state in Redis
- [ ] should expire deduplication entries after TTL
- [ ] should store storm detection state in Redis
- [ ] should handle concurrent Redis writes without corruption
- [ ] should handle Redis cluster failover without data loss
- [ ] should handle Redis memory eviction (LRU) gracefully

### **Prometheus Alert Processing (0/25 fixed)**
- [ ] creates RemediationRequest CRD with correct business metadata
- [ ] extracts resource information for AI targeting
- [ ] prevents duplicate CRDs using fingerprint
- [ ] (22 more tests...)

---

## 🔄 **Next Steps**

1. **Start with Category 1**: Deduplication TTL (smallest, most focused)
2. **Use Ginkgo Focus**: `-ginkgo.focus` for one test at a time
3. **Apply TDD Methodology**: Fix implementation, not tests
4. **Commit Frequently**: After each successful fix
5. **Track Progress**: Update this document after each fix

---

## 📚 **TDD Resources**

### **Key Files**
- **Tests**: `test/integration/gateway/*_test.go`
- **Implementation**: `pkg/gateway/processing/*.go`
- **Business Requirements**: `docs/requirements/*.md`

### **TDD Commands**
```bash
# Run one focused test
go test ./test/integration/gateway -v -ginkgo.focus "test name" -timeout 30s

# Run full suite
go test ./test/integration/gateway -v -timeout 5m

# Check test count
go test ./test/integration/gateway -v -timeout 5m 2>&1 | grep "Ran.*Specs"
```

---

## ✅ **Completed Fixes**

### **Fix 1: Health Endpoint**
**Test**: "should return 200 OK when all dependencies are healthy"  
**Issue**: Implementation returned `{"status": "ok"}` instead of `{"status": "healthy"}`  
**Fix**: Updated `pkg/gateway/server.go` healthHandler to return `"healthy"` + timestamp  
**Result**: ✅ Test passes  
**Commit**: `48291c84`

---

**Last Updated**: 2025-10-29
**Next Test to Fix**: "treats expired fingerprint as new alert after 5-minute TTL"

