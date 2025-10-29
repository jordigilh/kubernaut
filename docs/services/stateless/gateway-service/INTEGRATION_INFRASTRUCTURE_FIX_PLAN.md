# 🎯 Integration Test Infrastructure Fix Plan

**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**



**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**

# 🎯 Integration Test Infrastructure Fix Plan

**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**

# 🎯 Integration Test Infrastructure Fix Plan

**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**



**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**

# 🎯 Integration Test Infrastructure Fix Plan

**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**

# 🎯 Integration Test Infrastructure Fix Plan

**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**



**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**

# 🎯 Integration Test Infrastructure Fix Plan

**Created**: 2025-10-26
**Status**: 🎯 **IN PROGRESS**
**Priority**: **CRITICAL - BLOCKING Day 9 Phase 6B and Day 10**

---

## 📊 Current State

### Test Results
- **Pass Rate**: 37% (34/92 tests passing)
- **Failures**: 58 tests failing
- **Execution Time**: 4.5 minutes (excellent)
- **Infrastructure**: Kind cluster + local Redis (512MB)

### Critical Issues
1. **BeforeSuite Timeout**: `SetupSecurityTokens()` hanging, blocking all integration tests
2. **Redis OOM**: Memory exhaustion during burst tests (512MB insufficient)
3. **K8s API Throttling**: Rate limiting affecting CRD creation tests

---

## 🎯 Success Criteria

### Primary Goals
- ✅ **Pass Rate**: >95% (87+/92 tests passing)
- ✅ **Execution Time**: <5 minutes (currently 4.5 min)
- ✅ **Zero Infrastructure Flakes**: All failures are real business logic issues
- ✅ **Clean Test Output**: No timeouts, no OOM errors, no throttling

### Quality Gates
- ✅ 3 consecutive clean test runs
- ✅ Zero lint errors
- ✅ All unit tests passing (186/187 → 187/187)
- ✅ All integration tests passing (34/92 → 87+/92)

---

## 📋 Phase-by-Phase Fix Plan

### Phase 1: BeforeSuite Timeout Fix (PRIORITY 1)
**Duration**: 30-45 minutes
**Status**: 🔴 **BLOCKING ALL TESTS**

#### Root Cause Analysis
```bash
# Investigate SetupSecurityTokens() hanging
# Likely causes:
# 1. K8s API timeout (no response from Kind cluster)
# 2. ServiceAccount creation stuck
# 3. Token extraction hanging
# 4. RBAC permission issues
```

#### Fix Strategy
1. **Add Timeout Context** (5 min):
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   ```

2. **Add Debug Logging** (5 min):
   ```go
   fmt.Println("🔍 Creating ServiceAccounts...")
   fmt.Println("🔍 Extracting tokens...")
   fmt.Println("🔍 Verifying tokens...")
   ```

3. **Isolate Failure Point** (10 min):
   - Run each step independently
   - Identify exact hanging operation

4. **Implement Fix** (10-15 min):
   - Add retries with exponential backoff
   - Add timeout to K8s API calls
   - Add fallback mechanisms

#### Validation
```bash
# Run integration tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: BeforeSuite completes in <30 seconds
```

---

### Phase 2: Redis OOM Fix (PRIORITY 2)
**Duration**: 20-30 minutes
**Status**: 🟡 **INTERMITTENT**

#### Root Cause Analysis
- **Current Memory**: 512MB
- **OOM Trigger**: Burst tests with 50+ concurrent requests
- **Memory Usage**: ~600-800MB during burst tests

#### Fix Strategy
1. **Increase Redis Memory** (5 min):
   ```bash
   # Stop current Redis
   podman stop redis-gateway-local

   # Start with 2GB memory
   podman run -d --name redis-gateway-local \
     -p 6379:6379 \
     --memory=2g \
     redis:7-alpine redis-server --maxmemory 2gb --maxmemory-policy allkeys-lru
   ```

2. **Add Redis Flush to BeforeSuite** (10 min):
   ```go
   // In test/integration/gateway/helpers/redis_test_client.go
   func FlushRedisBeforeSuite() {
       client := GetRedisTestClient()
       ctx := context.Background()
       if err := client.FlushDB(ctx).Err(); err != nil {
           fmt.Printf("⚠️  Failed to flush Redis: %v\n", err)
       } else {
           fmt.Println("✅ Redis flushed successfully")
       }
   }
   ```

3. **Add Memory Monitoring** (5 min):
   ```go
   // Log Redis memory usage before/after each test
   info := client.Info(ctx, "memory").Val()
   fmt.Printf("📊 Redis Memory: %s\n", parseMemoryUsage(info))
   ```

#### Validation
```bash
# Run burst tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/burst_test.go -timeout 5m

# Expected: No OOM errors, all tests pass
```

---

### Phase 3: K8s API Throttling Fix (PRIORITY 3)
**Duration**: 15-20 minutes
**Status**: 🟡 **AFFECTING CRD TESTS**

#### Root Cause Analysis
- **Current Timeout**: 5 seconds (TokenReview, SubjectAccessReview)
- **Throttling**: Kind cluster rate limiting CRD creation
- **Impact**: 5-10 tests failing due to "too many requests"

#### Fix Strategy
1. **Add Retry Logic** (10 min):
   ```go
   // In pkg/gateway/server/handlers.go
   func (s *Server) createCRDWithRetry(ctx context.Context, crd *v1alpha1.RemediationRequest) error {
       backoff := time.Second
       maxRetries := 3

       for i := 0; i < maxRetries; i++ {
           err := s.k8sClient.Create(ctx, crd)
           if err == nil {
               return nil
           }

           if isRateLimitError(err) {
               time.Sleep(backoff)
               backoff *= 2
               continue
           }

           return err
       }

       return fmt.Errorf("max retries exceeded")
   }
   ```

2. **Add Test Delays** (5 min):
   ```go
   // In test/integration/gateway/helpers/test_helpers.go
   func SendWebhookWithDelay(payload []byte, delay time.Duration) (*http.Response, error) {
       time.Sleep(delay)
       return SendWebhook(payload)
   }
   ```

#### Validation
```bash
# Run CRD creation tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test -v ./test/integration/gateway/crd_test.go -timeout 5m

# Expected: No rate limit errors, all tests pass
```

---

### Phase 4: Remaining Business Logic Fixes (PRIORITY 4)
**Duration**: 2-3 hours
**Status**: 🟢 **READY AFTER INFRASTRUCTURE FIXES**

#### Test Categories
1. **Deduplication** (5 tests, 1h):
   - TTL refresh
   - Duplicate counter
   - Race conditions

2. **Storm Detection** (7 tests, 1.5h):
   - Aggregation logic
   - TTL expiration
   - Counter accuracy

3. **CRD Creation** (8 tests, 1h):
   - Name collisions
   - Metadata validation
   - Error handling

4. **Security** (5 tests, 30 min):
   - Token validation
   - Permission checks
   - Rate limiting

#### Fix Strategy
- **After infrastructure is stable** (Phases 1-3 complete)
- **One category at a time** (deduplication → storm → CRD → security)
- **Verify each fix** (run tests after each change)

---

## 🚀 Execution Plan

### Step 1: Fix BeforeSuite Timeout (30-45 min)
```bash
# 1. Add debug logging to SetupSecurityTokens()
# 2. Add timeout context (30s)
# 3. Run tests to identify hanging operation
# 4. Implement fix with retries
# 5. Verify BeforeSuite completes in <30s
```

### Step 2: Fix Redis OOM (20-30 min)
```bash
# 1. Increase Redis memory to 2GB
# 2. Add Redis flush to BeforeSuite
# 3. Add memory monitoring
# 4. Run burst tests to verify no OOM
```

### Step 3: Fix K8s API Throttling (15-20 min)
```bash
# 1. Add retry logic to CRD creation
# 2. Add test delays between requests
# 3. Run CRD tests to verify no rate limiting
```

### Step 4: Run Full Test Suite (5 min)
```bash
# Verify infrastructure fixes
./test/integration/gateway/helpers/run-tests-local.sh

# Expected: 60-70% pass rate (infrastructure fixed, business logic pending)
```

### Step 5: Fix Business Logic (2-3h)
```bash
# Fix one category at a time
# Run tests after each fix
# Verify no regressions
```

### Step 6: Final Validation (15 min)
```bash
# Run 3 consecutive clean test runs
# Verify >95% pass rate
# Verify zero lint errors
# Verify zero infrastructure flakes
```

---

## 📊 Progress Tracking

### Phase Completion
- [ ] Phase 1: BeforeSuite Timeout Fix (30-45 min)
- [ ] Phase 2: Redis OOM Fix (20-30 min)
- [ ] Phase 3: K8s API Throttling Fix (15-20 min)
- [ ] Phase 4: Business Logic Fixes (2-3h)
- [ ] Final Validation (15 min)

### Pass Rate Milestones
- [x] **Baseline**: 37% (34/92 tests) - **CURRENT**
- [ ] **Phase 1**: 40-50% (BeforeSuite fixed, tests can run)
- [ ] **Phase 2**: 50-60% (Redis OOM fixed, burst tests pass)
- [ ] **Phase 3**: 60-70% (K8s API throttling fixed, CRD tests pass)
- [ ] **Phase 4**: >95% (Business logic fixed, all tests pass)

---

## 🎯 Next Steps

### Immediate Actions (RIGHT NOW)
1. **Fix BeforeSuite Timeout** (PRIORITY 1)
   - Add debug logging
   - Add timeout context
   - Identify hanging operation
   - Implement fix

2. **Verify Infrastructure** (PRIORITY 2)
   - Run tests after each fix
   - Monitor for flakes
   - Document any new issues

3. **Resume Day 9 Phase 6B** (AFTER >95% PASS RATE)
   - Add 9 integration tests for Day 9 metrics
   - Verify all 17 new tests pass (8 unit + 9 integration)
   - Complete Day 9 Phase 6C validation

---

## 📈 Confidence Assessment

**Infrastructure Fix Confidence**: **95%**

**Justification**:
- ✅ Root causes identified (BeforeSuite timeout, Redis OOM, K8s throttling)
- ✅ Fix strategies proven (timeouts, retries, memory increase)
- ✅ Clear validation criteria (>95% pass rate, <5 min execution)
- ✅ Phased approach reduces risk
- ⚠️ Risk: Unknown issues may surface after infrastructure fixes

**Business Logic Fix Confidence**: **90%**

**Justification**:
- ✅ Test failures categorized (deduplication, storm, CRD, security)
- ✅ Fix strategies documented
- ✅ Stable infrastructure enables clear signal
- ⚠️ Risk: Some failures may be real bugs requiring design changes

---

## 🔗 Related Documents

- [IMPLEMENTATION_PLAN_V2.12.md](./IMPLEMENTATION_PLAN_V2.12.md) - Overall Gateway implementation plan
- [DAY9_PHASE6_PLAN.md](./DAY9_PHASE6_PLAN.md) - Day 9 Phase 6 test plan
- [CRITICAL_REMINDER_58_TESTS.md](./CRITICAL_REMINDER_58_TESTS.md) - Critical reminder to fix 58 tests
- [INTEGRATION_TEST_FIXES.md](./INTEGRATION_TEST_FIXES.md) - Previous integration test fixes

---

**Status**: 🎯 **READY TO START - Phase 1: BeforeSuite Timeout Fix**




