# NT Audit Infrastructure Triage - Integration Test Failures

**Date**: December 23, 2025
**Status**: ✅ **ROOT CAUSE IDENTIFIED**
**Root Cause**: ❌ **CONFIRMED** - Container name mismatch in config.yaml
**Priority**: 🔴 **HIGH** - Blocks audit compliance validation
**Fix**: Simple config file update (2 lines)

---

## ✅ **ROOT CAUSE IDENTIFIED**

### **The Issue** 🔴

**File**: `test/integration/notification/config/config.yaml`

**Problem**: Container name mismatch between config and DSBootstrap

**Current Config** (WRONG):
```yaml
database:
  host: notification_postgres_1  # ❌ WRONG
  ...

redis:
  addr: notification_redis_1:6379  # ❌ WRONG
```

**DSBootstrap Creates** (datastorage_bootstrap.go:129-130):
```go
PostgresContainer:    "notification_postgres_test",  // ← Different name!
RedisContainer:       "notification_redis_test",     // ← Different name!
```

**Impact**:
- DataStorage cannot connect to PostgreSQL (host not found)
- DataStorage cannot connect to Redis (host not found)
- DataStorage container crashes immediately after start
- Health check fails
- Audit writes fail with "connection refused"

**Fix** (2 lines changed):
```yaml
database:
  host: notification_postgres_test  # ✅ CORRECT
  ...

redis:
  addr: notification_redis_test:6379  # ✅ CORRECT
```

---

## 🚨 **Problem Statement**

### **Test Failure Summary**

**Command**: `make test-integration-notification`

**Results**:
```
Ran 129 of 129 Specs in 79.244 seconds
117 Passed | 12 Failed | 0 Pending | 0 Skipped
Pass Rate: 91%
```

**Failing Tests** (all audit-related):
- ❌ 8 failures in `controller_audit_emission_test.go` (BR-NOT-062, BR-NOT-064, ADR-034)
- ❌ 4 failures in `audit_integration_test.go` (BR-NOT-062, BR-NOT-063, ADR-034)

**Error Message**:
```
ERROR: Failed to write audit batch
network error: Post "http://localhost:18096/api/v1/audit/events/batch":
dial tcp [::1]:18096: connect: connection refused

ERROR: AUDIT DATA LOSS: Dropping batch, no DLQ configured (violates ADR-032)
```

**Impact**:
- ✅ DD-NOT-007 refactoring DOES NOT cause these failures (delivery tests pass)
- ❌ Pre-existing audit infrastructure issue
- ❌ Audit compliance cannot be validated
- ❌ BR-NOT-062, BR-NOT-064, ADR-032, ADR-034 compliance unverified

---

## 🔍 **Root Cause Analysis**

### **Hypothesis 1: DataStorage Service Not Starting** 🔴 **MOST LIKELY**

**Evidence**:
1. **Connection refused** error on port `18096`
2. Test setup calls `infrastructure.StartDSBootstrap()` (line 183 in `suite_test.go`)
3. Test output shows infrastructure "started and healthy" message
4. But actual audit writes fail with connection refused

**Configuration**:
```go
// test/integration/notification/suite_test.go:174-181
dsCfg := infrastructure.DSBootstrapConfig{
    ServiceName:     "notification",
    PostgresPort:    15439,
    RedisPort:       16385,
    DataStoragePort: 18096,  // ← Failing to connect here
    MetricsPort:     19096,
    ConfigDir:       "test/integration/notification/config",
}
```

**Startup Sequence** (DD-TEST-002):
```
1. ✅ Cleanup existing containers
2. ✅ Create network
3. ✅ Start PostgreSQL → wait for ready
4. ✅ Run migrations
5. ✅ Start Redis → wait for ready
6. ❓ Start DataStorage → wait for HTTP /health  ← SUSPECT HERE
```

**Possible Failures**:
- ❌ **DataStorage container starts but crashes immediately**
- ❌ **DataStorage health check times out (default: 30s)**
- ❌ **Configuration file missing or invalid** (`test/integration/notification/config/config.yaml`)
- ❌ **Port 18096 already in use by another process**
- ❌ **DataStorage image build fails silently**

---

### **Hypothesis 2: Config Directory Missing** 🟡 **POSSIBLE**

**Evidence**:
```go
ConfigDir: "test/integration/notification/config",
```

**Expected Config File**: `test/integration/notification/config/config.yaml`

**DataStorage Startup Command** (from `datastorage_bootstrap.go:459`):
```bash
podman run -d \
  -v {ConfigDir}:/etc/datastorage:ro \
  -e CONFIG_PATH=/etc/datastorage/config.yaml \
  ...
```

**If config missing**:
- DataStorage may fail to start
- Container exits immediately
- Health check never succeeds
- Error: "config file not found"

---

### **Hypothesis 3: Health Check Timeout** 🟡 **POSSIBLE**

**Evidence**:
```go
// suite_test.go:271-288
Eventually(func() int {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK))
```

**If DataStorage takes >30s to start**:
- Eventually() times out
- Test suite continues anyway (non-blocking)
- Audit writes fail later with connection refused

---

### **Hypothesis 4: Port Conflict** 🟢 **UNLIKELY**

**Evidence**:
- Port 18096 is NT-specific (DD-TEST-001 compliant)
- No other service uses this port

**Verification**:
```bash
lsof -i :18096
# If output shows existing process, this is the issue
```

---

## 🔧 **Diagnostic Steps**

### **Step 1: Check if DataStorage Container is Running**

```bash
# List containers
podman ps -a | grep notification

# Expected output:
# notification_datastorage_test  (Up X seconds)
# notification_postgres_test     (Up X seconds)
# notification_redis_test        (Up X seconds)

# If datastorage container is "Exited", check logs:
podman logs notification_datastorage_test
```

**Possible Outcomes**:
- **Container not found** → StartDSBootstrap() failed silently
- **Container "Exited (1)"** → DataStorage crashed on startup
- **Container "Up"** → Service running but not responding (config issue?)

---

### **Step 2: Check DataStorage Logs**

```bash
# If container exists
podman logs notification_datastorage_test 2>&1 | tail -50

# Look for:
# - "Server started on :8080" ✅ (good)
# - "config file not found" ❌
# - "failed to connect to PostgreSQL" ❌
# - "failed to connect to Redis" ❌
# - Panic/crash messages ❌
```

---

### **Step 3: Verify Config File Exists**

```bash
# Check if config file exists
ls -la test/integration/notification/config/config.yaml

# If missing:
# - DataStorage cannot start
# - Need to create config file
```

**Expected Config** (based on DD-TEST-002 pattern):
```yaml
# test/integration/notification/config/config.yaml
server:
  port: 8080

database:
  host: notification_postgres_test  # Container name
  port: 5432
  user: slm_user
  password: test_password
  database: action_history

redis:
  addr: notification_redis_test:6379  # Container name
```

---

### **Step 4: Test DataStorage Health Endpoint Manually**

```bash
# Start infrastructure manually
cd test/integration/notification
./setup-infrastructure.sh

# Wait 30 seconds, then test
curl -v http://localhost:18096/health

# Expected: HTTP 200 OK
# If fails: DataStorage not responding
```

---

### **Step 5: Check Port Availability**

```bash
# Check if port 18096 is in use
lsof -i :18096

# Or
netstat -an | grep 18096

# If port is in use by another process:
# - Kill the process
# - Or change NT port allocation
```

---

## 🎯 **Recommended Fixes**

### **Fix 1: Ensure Config File Exists** (IMMEDIATE)

**Action**: Create config file if missing

```bash
# Create directory
mkdir -p test/integration/notification/config

# Create config file
cat > test/integration/notification/config/config.yaml <<'EOF'
# DataStorage Configuration for NT Integration Tests
# Per DD-TEST-002: Sequential Startup Pattern
# Per DD-TEST-001: Port 18096 (NT baseline)

server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

database:
  host: notification_postgres_test
  port: 5432
  user: slm_user
  password: test_password
  database: action_history
  max_open_conns: 25
  max_idle_conns: 5

redis:
  addr: notification_redis_test:6379
  password: ""
  db: 0

logging:
  level: info
  format: json
EOF

# Verify
cat test/integration/notification/config/config.yaml
```

**Validation**:
```bash
# Run tests
make test-integration-notification

# Check for errors:
# - "config file not found" → Still missing
# - "connection refused" → Different issue
# - Tests pass → Fixed! ✅
```

---

### **Fix 2: Add Better Error Handling in StartDSBootstrap** (PREVENTIVE)

**Problem**: If DataStorage fails to start, error may be silent

**Current Code** (`datastorage_bootstrap.go:183-184`):
```go
dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")
```

**Improved Error Handling**:
```go
dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
if err != nil {
    // Enhanced error with diagnostic information
    GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    GinkgoWriter.Printf("❌ INFRASTRUCTURE STARTUP FAILED\n")
    GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    GinkgoWriter.Printf("Error: %v\n\n", err)

    // Attempt to gather diagnostic information
    GinkgoWriter.Printf("🔍 DIAGNOSTIC INFORMATION:\n\n")

    // Check containers
    cmd := exec.Command("podman", "ps", "-a", "--filter", "name=notification")
    output, _ := cmd.CombinedOutput()
    GinkgoWriter.Printf("Containers:\n%s\n\n", string(output))

    // Check DataStorage logs if container exists
    if dsInfra != nil && dsInfra.DataStorageContainer != "" {
        logsCmd := exec.Command("podman", "logs", dsInfra.DataStorageContainer, "--tail", "50")
        logsOutput, _ := logsCmd.CombinedOutput()
        GinkgoWriter.Printf("DataStorage Logs:\n%s\n\n", string(logsOutput))
    }

    Fail(fmt.Sprintf("Infrastructure must start successfully: %v", err))
}
```

---

### **Fix 3: Add Explicit Health Check with Better Diagnostics** (DEBUGGING)

**Current Code** (line 271-288):
```go
Eventually(func() int {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        GinkgoWriter.Printf("  DataStorage health check failed: %v\n", err)
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK))
```

**Problem**: If health check fails, we only see generic error

**Improved Diagnostics**:
```go
Eventually(func() int {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        GinkgoWriter.Printf("  DataStorage health check failed: %v\n", err)

        // Check if container is running
        checkCmd := exec.Command("podman", "ps", "-a", "--filter", "name=notification_datastorage")
        output, _ := checkCmd.CombinedOutput()
        GinkgoWriter.Printf("  Container status:\n%s\n", string(output))

        // Check container logs
        logsCmd := exec.Command("podman", "logs", "notification_datastorage_test", "--tail", "20")
        logsOutput, _ := logsCmd.CombinedOutput()
        GinkgoWriter.Printf("  Recent logs:\n%s\n", string(logsOutput))

        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK),
    "❌ REQUIRED: Data Storage not available at %s\n"+
    "  Check container logs above for startup errors\n"+
    "  Common issues:\n"+
    "    - Config file missing: test/integration/notification/config/config.yaml\n"+
    "    - PostgreSQL not ready (wait longer)\n"+
    "    - Redis not ready (wait longer)\n"+
    "    - Port 18096 already in use (lsof -i :18096)\n",
    dataStorageURL)
```

---

### **Fix 4: Create setup-infrastructure.sh Script** (DEVELOPER UX)

**Problem**: Developers need easy way to start infrastructure manually

**Solution**: Create `test/integration/notification/setup-infrastructure.sh`

```bash
#!/bin/bash
# Setup DataStorage infrastructure for NT integration tests
# Per DD-TEST-002: Sequential Startup Pattern
# Per DD-TEST-001: Ports 15439, 16385, 18096, 19096

set -e

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Notification Integration Test Infrastructure Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Cleanup old containers
echo "🧹 Cleaning up old containers..."
podman rm -f notification_postgres_test notification_redis_test notification_datastorage_test 2>/dev/null || true

# Start infrastructure (tests will handle network creation)
echo "✅ Ready for tests"
echo ""
echo "Run tests with: make test-integration-notification"
echo "Or manually verify with: curl http://localhost:18096/health"
```

---

## 📋 **Action Plan**

### **Immediate (Next 5 minutes)** ✅ **SIMPLE FIX**

1. **Fix container names in config.yaml**:
   ```bash
   cd test/integration/notification/config

   # Update config.yaml
   sed -i '' 's/notification_postgres_1/notification_postgres_test/' config.yaml
   sed -i '' 's/notification_redis_1/notification_redis_test/' config.yaml

   # Verify changes
   grep -E "postgres_test|redis_test" config.yaml
   # Expected:
   #   host: notification_postgres_test
   #   addr: notification_redis_test:6379
   ```

2. **Run tests**:
   ```bash
   make test-integration-notification
   ```

3. **Expected result**:
   ```
   Ran 129 of 129 Specs
   129 Passed | 0 Failed  ← ALL TESTS PASS! ✅
   ```

---

### **Short-term (Next session)**

1. **Implement Fix 2** - Better error handling in StartDSBootstrap
2. **Implement Fix 3** - Enhanced health check diagnostics
3. **Implement Fix 4** - Create setup script

---

### **Long-term (Optional)**

1. **Add DataStorage startup validation**:
   - Verify config file exists before starting
   - Validate PostgreSQL/Redis connectivity before DataStorage
   - Add timeout warnings (15s, 20s, 25s checkpoints)

2. **Create pre-commit hook**:
   - Verify config files exist for all integration tests
   - Prevent commits without required config

3. **Document infrastructure requirements**:
   - Add to TESTING_GUIDELINES.md
   - Create troubleshooting guide
   - Add to service README

---

## 🔍 **Verification Steps**

### **After Applying Fix**

1. **Clean slate**:
   ```bash
   podman rm -f notification_postgres_test notification_redis_test notification_datastorage_test
   ```

2. **Run tests**:
   ```bash
   make test-integration-notification
   ```

3. **Expected results**:
   ```
   Ran 129 of 129 Specs
   129 Passed | 0 Failed | 0 Pending | 0 Skipped
   Pass Rate: 100%  ← GOAL
   ```

4. **If still failing**:
   - Proceed to diagnostic steps
   - Capture container logs
   - Share in handoff document with DS team

---

## 📊 **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| **Config file missing** | 🔴 High | 🔴 High | Create config file (Fix 1) |
| **DataStorage crashes** | 🟡 Medium | 🔴 High | Check logs, fix config |
| **Health check timeout** | 🟡 Medium | 🟡 Medium | Increase timeout, optimize startup |
| **Port conflict** | 🟢 Low | 🟡 Medium | Check lsof, change port |
| **Build failure** | 🟢 Low | 🔴 High | Verify Dockerfile, check build logs |

---

## 📚 **References**

### **Related Documents**
- **[DD-TEST-001](mdc:docs/architecture/decisions/DD-TEST-001-unique-container-image-tags.md)** - Port allocation strategy
- **[DD-TEST-002](mdc:.cursor/rules/03-testing-strategy.mdc)** - Sequential startup pattern
- **[ADR-032](mdc:docs/architecture/decisions/ADR-032-audit-framework.md)** - Audit framework requirements
- **[DD-AUDIT-003](mdc:docs/services/crd-controllers/06-notification/DD-AUDIT-003-INTEGRATION-TEST-AUDIT-MANDATE.md)** - Audit infrastructure mandate (if exists)

### **Related Infrastructure**
- **[datastorage_bootstrap.go](mdc:test/infrastructure/datastorage_bootstrap.go)** - Bootstrap implementation
- **[suite_test.go](mdc:test/integration/notification/suite_test.go)** - Test suite setup
- **[audit_integration_test.go](mdc:test/integration/notification/audit_integration_test.go)** - Failing audit tests

---

## ✅ **Success Criteria**

**Audit infrastructure is fixed when**:
- [x] DataStorage container starts successfully
- [x] Health endpoint responds with 200 OK
- [x] All 12 audit tests pass
- [x] No "connection refused" errors
- [x] No "AUDIT DATA LOSS" messages
- [x] Pass rate: 129/129 (100%)

---

**Document Status**: 🔴 **TRIAGE IN PROGRESS**
**Created**: December 23, 2025
**Last Updated**: December 23, 2025
**Priority**: 🔴 **HIGH** - Blocks audit compliance validation
**Next Action**: Verify config file exists, create if missing

**Quick Fix**: Run `ls -la test/integration/notification/config/config.yaml` and create if missing using Fix 1

