# Triage: Podman-Compose Infrastructure Conflict

**Date**: 2025-12-11
**Issue**: `podman-compose up -d` fails with "Command failed to spawn: Aborted"
**Severity**: 🔴 **BLOCKING** - Cannot run integration tests
**Status**: 🟡 **DIAGNOSED** - Root cause identified

---

## 🚨 **Problem Statement**

When attempting to start integration test infrastructure:
```bash
$ podman-compose -f podman-compose.test.yml up -d
Error: Command failed to spawn: Aborted
```

**Impact**: Cannot validate RO integration tests per TRIAGE_RO_DAY1_TESTING_COMPLIANCE.md

---

## 🔍 **Root Cause Analysis**

### **Issue 1: Conflicting Containers Already Running** 🔴

**Evidence**:
```bash
$ podman ps -a | grep -E "postgres|redis|datastorage"

# ACTIVE (blocking ports):
datastorage-postgres-test    Up 3 minutes     0.0.0.0:15433->5432/tcp
datastorage-redis-test       Up 2 minutes     0.0.0.0:16379->6379/tcp
datastorage-service-test     Exited (0) 7s    0.0.0.0:18090->8080/tcp

# STOPPED (stale):
kubernaut_postgres_1         Exited (0) 7m    0.0.0.0:15433->5432/tcp
kubernaut_redis_1            Exited (0) 7m    0.0.0.0:16379->6379/tcp
```

**Analysis**:
1. **Ports 15433, 16379, 18090 already bound** by existing containers
2. Containers from different test runs exist (`datastorage-*` prefix vs `kubernaut_*` prefix)
3. Data Storage service exited recently (likely crashed or stopped)

**Root Cause**: Port conflicts prevent new containers from binding to required ports.

---

### **Issue 2: Multiple Test Infrastructure Patterns** 🟡

**Evidence from container names**:

| Container Name Pattern | Source | Status |
|------------------------|--------|--------|
| `kubernaut_postgres_1` | Old `podman-compose.test.yml` run | Stopped |
| `datastorage-postgres-test` | Data Storage team's test setup | Running |
| `gateway-datastorage-integration` | Gateway team's test setup | Exited |
| `signalprocessing-e2e-*` | SP E2E tests in Kind | Running |

**Analysis**: Multiple teams running different test infrastructure patterns:
1. **RO Team**: Trying to use `podman-compose.test.yml` (global test infrastructure)
2. **DS Team**: Using service-specific test containers (`datastorage-*` prefix)
3. **Gateway Team**: Using integration-specific containers
4. **SP Team**: Running E2E tests in Kind cluster

**Root Cause**: No coordination on shared test infrastructure usage.

---

### **Issue 3: Data Storage Service Failed** 🔴

**Evidence**:
```bash
datastorage-service-test     Exited (0) 7 seconds ago
```

**Analysis**:
- Service was running but exited recently
- Exit code 0 = clean shutdown (not a crash)
- But health check likely failing (service not responding)

**Root Cause**: Data Storage service stopped, but postgres/redis still running.

---

## 📊 **Current Infrastructure State**

### **Port Allocation** (Per DD-TEST-001)

| Port | Service | Expected Owner | Actual Status | Conflict? |
|------|---------|----------------|---------------|-----------|
| **15433** | PostgreSQL | `podman-compose.test.yml` | **BOUND** by `datastorage-postgres-test` | ✅ Can use |
| **16379** | Redis | `podman-compose.test.yml` | **BOUND** by `datastorage-redis-test` | ✅ Can use |
| **18090** | Data Storage | `podman-compose.test.yml` | **FREE** (container exited) | ⚠️ Need to restart |
| **8081** | HolmesGPT-API | `podman-compose.test.yml` | **FREE** | ✅ Available |

**Key Insight**: Postgres and Redis are already running! We just need to restart Data Storage.

---

### **Container Health Status**

```bash
# HEALTHY (can reuse):
✅ datastorage-postgres-test  → Running, healthy
✅ datastorage-redis-test     → Running, healthy

# NEEDS RESTART:
⚠️ datastorage-service-test   → Exited, need to start

# STALE (should clean up):
❌ kubernaut_postgres_1       → Old, stopped
❌ kubernaut_redis_1          → Old, stopped
❌ gateway-datastorage-integration → Old, stopped
```

---

## 🎯 **Resolution Strategy**

### **Option A: Use Existing Infrastructure** ✅ **RECOMMENDED**

**Rationale**: Postgres and Redis already running and healthy!

**Steps**:
1. **Verify existing services are healthy**
2. **Restart Data Storage service** (if needed)
3. **Configure tests to use existing ports**
4. **Run integration tests**

**Advantages**:
- ✅ Fast (no rebuild/restart)
- ✅ Uses existing healthy infrastructure
- ✅ No port conflicts

**Disadvantages**:
- ⚠️ Might be using someone else's test infrastructure
- ⚠️ Need to coordinate with DS team

**Time Estimate**: 5 minutes

---

### **Option B: Clean All and Restart** ⚠️

**Rationale**: Start fresh with known state

**Steps**:
1. **Stop all test containers**
2. **Remove stopped containers**
3. **Start fresh infrastructure**

**Advantages**:
- ✅ Clean, known state
- ✅ No dependency on existing setup

**Disadvantages**:
- ⚠️ Might break other teams' tests (SP E2E running)
- ⚠️ Takes longer (5-10 minutes to rebuild/start)
- ⚠️ Risk of breaking currently running tests

**Time Estimate**: 10-15 minutes

---

### **Option C: Service-Specific Test Infrastructure** 🟢

**Rationale**: Each service manages its own test infrastructure

**Steps**:
1. **Create RO-specific podman-compose file** (`test/integration/remediationorchestrator/podman-compose.test.yml`)
2. **Use RO-specific port allocation**
3. **Don't conflict with shared infrastructure**

**Advantages**:
- ✅ No conflicts with other teams
- ✅ RO team has full control
- ✅ Follows service isolation principle

**Disadvantages**:
- ⚠️ More complex (new ports, new config)
- ⚠️ Duplicates infrastructure per service
- ⚠️ Longer term solution

**Time Estimate**: 30-60 minutes (implementation)

---

## 💡 **RECOMMENDED IMMEDIATE ACTION**

### **Use Option A: Leverage Existing Infrastructure** ✅

**Why**:
1. Postgres and Redis are **already running and healthy**
2. Just need to restart Data Storage service
3. Fastest path to validating integration tests
4. Can refactor to Option C later if needed

**Implementation**:

```bash
# Step 1: Verify existing infrastructure
podman ps --filter "name=datastorage-postgres-test"
podman ps --filter "name=datastorage-redis-test"

# Step 2: Check if Data Storage needs restart
podman ps -a --filter "name=datastorage-service-test"

# Step 3: Restart Data Storage if needed
podman start datastorage-service-test

# OR if it doesn't exist, start with podman-compose
# (will use existing postgres/redis, only start datastorage)
podman-compose -f podman-compose.test.yml up -d datastorage

# Step 4: Wait for health check
for i in {1..30}; do
    curl -f http://localhost:18090/health && break || sleep 1
done

# Step 5: Run RO integration tests
go test ./test/integration/remediationorchestrator/... -v -timeout 5m
```

---

## 🔧 **Detailed Resolution Steps**

### **Phase 1: Verify Existing Infrastructure** (2 min)

```bash
# Check Postgres health
podman exec datastorage-postgres-test pg_isready -U slm_user -d action_history
# Expected: "accepting connections"

# Check Redis health
podman exec datastorage-redis-test redis-cli ping
# Expected: "PONG"

# Check Data Storage status
podman ps -a --filter "name=datastorage-service-test" --format "{{.Status}}"
```

**Decision Point**:
- If Postgres/Redis healthy → Proceed to Phase 2
- If not healthy → Use Option B (clean restart)

---

### **Phase 2: Restart Data Storage** (2 min)

```bash
# Option 2A: Restart existing container
podman start datastorage-service-test

# Option 2B: Start via podman-compose (will reuse postgres/redis)
podman-compose -f podman-compose.test.yml up -d datastorage

# Wait for health
timeout 30 bash -c 'until curl -f http://localhost:18090/health; do sleep 1; done'
```

---

### **Phase 3: Validate Test Connectivity** (1 min)

```bash
# Test Data Storage API
curl -f http://localhost:18090/health
# Expected: {"status":"healthy",...}

# Test database connection
curl -f http://localhost:18090/api/v1/audit-events?limit=1
# Expected: {"events":[],...}

# Set environment variables for tests
export DATASTORAGE_URL=http://localhost:18090
export POSTGRES_HOST=localhost
export POSTGRES_PORT=15433
export REDIS_HOST=localhost
export REDIS_PORT=16379
```

---

### **Phase 4: Run Integration Tests** (3-5 min)

```bash
# Run RO integration tests
go test ./test/integration/remediationorchestrator/... \
    -v \
    -timeout 5m \
    | tee /tmp/ro_integration_test_results.log

# Check results
grep -E "PASS|FAIL|--- PASS|--- FAIL" /tmp/ro_integration_test_results.log
```

---

## 🚫 **What NOT To Do**

### **❌ Don't Force Kill All Containers**

```bash
# ❌ WRONG: Will break SP E2E tests currently running
podman stop $(podman ps -aq)
```

**Why**: SignalProcessing E2E tests are running in Kind cluster.

---

### **❌ Don't Change Port Allocations Without Checking**

```bash
# ❌ WRONG: Will conflict with DD-TEST-001 port allocation standard
# Don't randomly change ports in podman-compose.test.yml
```

**Why**: Port allocations are standardized per DD-TEST-001.

---

### **❌ Don't Run Multiple podman-compose Instances**

```bash
# ❌ WRONG: Running multiple compose files simultaneously
podman-compose -f podman-compose.test.yml up -d &
podman-compose -f test/integration/datastorage/docker-compose.yml up -d &
```

**Why**: Port conflicts and resource contention.

---

## 📋 **Follow-Up Actions**

### **Immediate (Next 10 minutes)** 🔴

1. **Execute Option A** - Use existing infrastructure
2. **Validate integration tests pass**
3. **Document actual pass/fail results**

### **Short-Term (This Week)** 🟡

1. **Coordinate with DS team** - Are they using shared infrastructure?
2. **Document infrastructure ownership** - Who manages test postgres/redis?
3. **Consider BeforeSuite automation** - Auto-start infrastructure in tests

### **Long-Term (Next Sprint)** 🟢

1. **Implement Option C** - Service-specific test infrastructure
2. **Create RO-specific podman-compose** - Isolated from other teams
3. **Document infrastructure patterns** - Shared vs service-specific

---

## 🎓 **Lessons Learned**

### **1. Shared Test Infrastructure Needs Coordination**

**Issue**: Multiple teams trying to use same ports/services

**Solution**: Either:
- Coordinate shared infrastructure usage (who starts/stops)
- Use service-specific infrastructure (isolated per team)

### **2. Check Existing State Before Starting**

**Issue**: Assumed clean state, but infrastructure already running

**Solution**: Always check `podman ps` before `podman-compose up`

### **3. Port Conflicts are Common in Test Environments**

**Issue**: Multiple test runs leave stale containers on standard ports

**Solution**:
- Use unique ports per service (per DD-TEST-001)
- Clean up after tests
- Check port availability before starting

---

## 📊 **Infrastructure Ownership Recommendation**

### **Proposed Model**: **Service-Specific Test Infrastructure**

| Service | Infrastructure File | Ports | Ownership |
|---------|---------------------|-------|-----------|
| **Data Storage** | `test/integration/datastorage/podman-compose.yml` | 15433, 16379, 18090 | DS Team |
| **Gateway** | `test/integration/gateway/podman-compose.yml` | 15434, 16380, 18091 | Gateway Team |
| **RO** | `test/integration/remediationorchestrator/podman-compose.yml` | 15435, 16381, 18092 | RO Team |
| **Shared (optional)** | `podman-compose.test.yml` (root) | 15433, 16379, 18090 | Platform Team |

**Benefits**:
- ✅ No port conflicts
- ✅ Team autonomy
- ✅ Can run tests in parallel
- ✅ Clear ownership

---

## ✅ **Success Criteria**

Integration tests validation complete when:

- [ ] Infrastructure running and healthy
- [ ] Data Storage responding on http://localhost:18090/health
- [ ] Postgres accepting connections on localhost:15433
- [ ] Redis responding on localhost:16379
- [ ] RO integration tests executed
- [ ] Pass/fail results documented
- [ ] No port conflicts
- [ ] No interference with other teams' tests

---

**Triage Status**: ✅ **COMPLETE**
**Root Cause**: Port conflicts from existing test infrastructure
**Recommended Action**: Option A - Use existing infrastructure
**Estimated Time to Resolution**: 5-10 minutes
**Priority**: 🔴 **CRITICAL** - Blocking integration test validation

---

**Next Steps**: Execute Option A and validate RO integration tests

**Document Created**: 2025-12-11
**Team**: RemediationOrchestrator
**Blocks**: TRIAGE_RO_DAY1_TESTING_COMPLIANCE.md (GAP-1, GAP-2)






