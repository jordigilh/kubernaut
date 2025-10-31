# Gateway Redis Integration Tests

**Location**: `test/integration/gateway/redis_resilience_test.go`
**Purpose**: Test Gateway deduplication service with real Redis infrastructure
**Redis Source**: OCP cluster (`kubernaut-system` namespace)

---

## 🎯 **Why Integration Tests?**

**Test Classification**:
- **Unit Tests** (miniredis): Business logic, validation, count tracking
- **Integration Tests** (real Redis): Infrastructure resilience, timeouts, connection failures

**Why Real Redis?**:
- ✅ miniredis executes too fast to trigger timeouts (<1ms)
- ✅ Real Redis has network latency (5-50ms) enabling timeout testing
- ✅ Tests production-like failure scenarios (connection loss, slow responses)

---

## 🚀 **Running Integration Tests**

### **Option 1: Automated Script** (Recommended)

```bash
# Automatically sets up port-forward and runs tests
./scripts/test-gateway-integration.sh
```

**What it does**:
1. Verifies Redis service exists in `kubernaut-system` namespace
2. Sets up port-forward: `localhost:6379` → `kubernaut-system/redis:6379`
3. Runs Gateway integration tests
4. Cleans up port-forward automatically

### **Option 2: Manual Port-Forward**

```bash
# Terminal 1: Setup port-forward
kubectl port-forward -n kubernaut-system svc/redis 6379:6379

# Terminal 2: Run tests
go test -v ./test/integration/gateway/... -timeout 2m
```

### **Option 3: Local Docker Redis** (Fallback)

If OCP Redis is unavailable, tests automatically fallback to local Docker Redis:

```bash
# Start local Redis (port 6380)
make bootstrap-dev

# Run tests (automatically uses localhost:6380 as fallback)
go test -v ./test/integration/gateway/... -timeout 2m
```

### **Option 4: Skip Integration Tests**

```bash
# Skip Redis integration tests (e.g., in CI without Redis)
SKIP_REDIS_INTEGRATION=true go test -v ./test/integration/gateway/...
```

---

## 📋 **Test Coverage**

### **Integration Test Suite** (2 tests)

| Test | BR | Purpose | Expected Result |
|------|----|---------|--------------|
| **Context timeout handling** | BR-GATEWAY-005 | Verify Gateway respects 1ms timeout | `context deadline exceeded` error |
| **Connection failure handling** | BR-GATEWAY-005 | Verify Gateway handles Redis crashes | `redis: client is closed` error |

### **Business Scenarios Tested**

1. **Slow Redis (P99 > 3s)**:
   - Scenario: Redis overloaded during high load
   - Behavior: Gateway times out after 1ms, returns 500
   - Value: Prevents webhook blocking, enables client retry

2. **Redis Crash**:
   - Scenario: Redis pod restarts during webhook processing
   - Behavior: Gateway returns connection error, remains operational
   - Value: Deduplication temporarily disabled, Gateway doesn't crash

---

## 🏗️ **Infrastructure Details**

### **OCP Redis Configuration**

**Location**: `kubernaut-system` namespace
**Service**: `redis`
**Port**: 6379
**Password**: None (internal cluster service)
**Database**: 1 (isolated from production DB 0)

**Deployment**:
- Manifest: `deploy/context-api/redis-deployment.yaml`
- Image: `redis:7-alpine`
- Resources: 256Mi-512Mi memory, 100m-500m CPU
- Persistence: EmptyDir (dev) / PVC (prod)

### **Connection Fallback Strategy**

```go
// 1. Try OCP Redis (port 6379, no password)
redisClient := goredis.NewClient(&goredis.Options{
    Addr:     "localhost:6379",
    Password: "",
    DB:       1,
})

// 2. If unavailable, fallback to local Docker Redis (port 6380)
redisClient := goredis.NewClient(&goredis.Options{
    Addr:     "localhost:6380",
    Password: "integration_redis_password",
    DB:       1,
})

// 3. If both unavailable, skip test
Skip("Redis not available - run port-forward or make bootstrap-dev")
```

---

## 🎯 **Test Requirements**

### **Prerequisites**

1. **OCP Cluster Access**:
   ```bash
   # Verify cluster access
   kubectl get svc redis -n kubernaut-system
   ```

2. **Redis Service Deployed**:
   ```bash
   # Deploy Redis if not present
   kubectl apply -f deploy/context-api/redis-deployment.yaml
   ```

3. **Port 6379 Available**:
   ```bash
   # Check if port is free
   lsof -i :6379
   ```

### **Environment Variables**

| Variable | Default | Purpose |
|----------|---------|---------|
| `SKIP_REDIS_INTEGRATION` | `false` | Skip integration tests if `true` |

---

## 📊 **Test Results**

### **Expected Output** (Success)

```bash
Running Suite: Gateway Integration Suite
=============================================
Random Seed: 1761157000

Will run 2 of 2 specs
••

Ran 2 of 2 Specs in 2.5 seconds
SUCCESS! -- 2 Passed | 0 Failed | 0 Skipped

Tests:
✅ respects context timeout when Redis is slow
✅ handles Redis connection failure gracefully
```

### **Expected Output** (Skipped)

```bash
Will run 0 of 2 specs
SS

Ran 0 of 2 Specs in 0.001 seconds
SUCCESS! -- 0 Passed | 0 Failed | 2 Skipped

Reason: Redis not available - run port-forward or make bootstrap-dev
```

---

## 🔧 **Troubleshooting**

### **Issue**: Port-forward fails

```bash
# Kill existing processes on port 6379
lsof -ti:6379 | xargs kill -9

# Verify Redis service exists
kubectl get svc redis -n kubernaut-system

# Check pod status
kubectl get pods -n kubernaut-system -l app=redis
```

### **Issue**: Test timeout

```bash
# Increase test timeout
go test -v ./test/integration/gateway/... -timeout 5m

# Check Redis connectivity
redis-cli -h localhost -p 6379 ping
```

### **Issue**: Context deadline not triggered

**Expected**: This is normal! Real Redis with network latency WILL trigger timeout.
**If test passes**: Redis is responding faster than 1ms (very rare, usually means local Redis).

---

## 📚 **Related Documentation**

- **Migration Assessment**: `REDIS_TIMEOUT_TEST_MIGRATION_ASSESSMENT.md`
- **Day 3 Status**: `docs/services/stateless/gateway-service/DAY3_FINAL_STATUS.md`
- **TDD Methodology**: `TDD_REFACTOR_CLARIFICATION.md`
- **OCP Redis Deployment**: `deploy/context-api/redis-deployment.yaml`
- **Port-Forward Setup**: `scripts/setup-port-forwarding.sh`

---

## 🎯 **Success Criteria**

Integration tests are successful when:

1. ✅ Tests connect to Redis (OCP or local)
2. ✅ Context timeout test triggers `context deadline exceeded`
3. ✅ Connection failure test returns Redis error
4. ✅ Tests complete in <5 seconds total
5. ✅ No test flakiness (deterministic results)

---

## 🚀 **CI/CD Integration**

### **GitHub Actions** (Future)

```yaml
- name: Setup Redis Port-Forward
  run: |
    kubectl port-forward -n kubernaut-system svc/redis 6379:6379 &
    sleep 2

- name: Run Gateway Integration Tests
  run: go test -v ./test/integration/gateway/... -timeout 2m

- name: Cleanup Port-Forward
  if: always()
  run: pkill -f "port-forward.*redis"
```

### **OpenShift CI** (Current)

Tests automatically connect to Redis in `kubernaut-system` namespace (no port-forward needed when running in-cluster).

---

**Confidence**: 95% ✅ Very High
**Status**: ✅ Ready for execution
**Next**: Run `./scripts/test-gateway-integration.sh` to verify



