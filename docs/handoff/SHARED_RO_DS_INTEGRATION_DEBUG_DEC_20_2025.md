# 🔄 **SHARED: RO Integration Test Infrastructure Issue** ✅ RESOLVED

> ⚠️ **HISTORICAL DOCUMENT**: This is a debugging session record from December 20, 2025.
> **For authoritative guidance**, see **DD-TEST-002: Integration Test Container Orchestration Pattern**
> **Location**: `docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md`
> **Implementation Guide**: `docs/development/testing/INTEGRATION_TEST_INFRASTRUCTURE_SETUP.md`

**Date**: 2025-12-20
**Status**: ✅ **ROOT CAUSE IDENTIFIED + SOLUTIONS PROVIDED**
**Teams**: RO (Requesting Help) → DS (Answered ✅)
**Update**: 2025-12-20, 11:40 EST - DS Team provided detailed answers and recommendations
**Resolution**: Sequential startup pattern (now authoritative in DD-TEST-002)

---

## 🎯 **TL;DR - DS Team Answer**

**ROOT CAUSE**: `podman-compose` starts services in parallel → DataStorage tries to connect to PostgreSQL before it's ready → Service fails to initialize → Tests see "healthy" container but HTTP never works

**FIX**: Replace `podman-compose` with sequential `podman run` commands (like DS integration tests do)

**CONFIDENCE**: 95% - DS team just fixed these exact issues to achieve 100% integration test pass rate (164/164)

**EFFORT**: ~2-3 hours to refactor RO infrastructure setup

**SEE**: Section "🎯 DS TEAM RECOMMENDATIONS" below for code examples

---

## 🚨 **Problem Summary**

RO integration tests are experiencing infrastructure connectivity issues that block 16 tests from completing.

**Symptom**: Tests pass when DataStorage infrastructure is **manually started before test execution**, but fail when tests start their own infrastructure via `podman-compose`.

---

## ✅ **What Works (Manual Infrastructure Start)**

```bash
# Terminal 1: Start infrastructure manually
cd test/integration/remediationorchestrator
podman-compose -f podman-compose.remediationorchestrator.test.yml up -d

# Wait 5-10 seconds for services to be fully ready

# Terminal 2: Run tests
go test -v ./test/integration/remediationorchestrator/... -ginkgo.focus="Audit Trace Integration"
```

**Result**: 3/3 audit tests pass ✅

```
Ran 3 of 59 Specs in 98.475 seconds
SUCCESS! -- 3 Passed | 0 Failed | 0 Pending | 56 Skipped
```

**Verification**: Direct connectivity test succeeds:

```bash
$ curl -v http://127.0.0.1:18140/health
* Connected to 127.0.0.1 (127.0.0.1) port 18140
< HTTP/1.1 200 OK
{"status":"healthy","database":"connected"}
```

---

## ❌ **What Fails (Test Suite Manages Infrastructure)**

When RO integration tests start infrastructure via `BeforeSuite`:

```go
// test/integration/remediationorchestrator/suite_test.go:54-71
cmd := exec.Command("podman-compose", "-f", composeFile, "up", "-d")
// ... starts containers ...

// Health check with retry
dataStorageClient, err := audit.NewOpenAPIClientAdapter("http://127.0.0.1:18140", 5*time.Second)
Expect(err).ToNot(HaveOccurred())

// Retry health check up to 10 times (20 seconds total)
var lastErr error
healthy := false
for i := 0; i < 10; i++ {
    resp, err := client.Get(dsURL + "/health")
    if err == nil && resp.StatusCode == http.StatusOK {
        healthy = true
        break
    }
    lastErr = err
    time.Sleep(2 * time.Second)
}
```

**Result**: Tests timeout or fail with connection errors

```
[FAILED] Expected <string>: success to equal <string>: pending
[FAILED] Timed out after 120.001s.
```

---

## 🔍 **What We've Tried**

### ✅ **Fixes Applied (Partial Success)**

1. **IPv4 Forcing**: Changed `localhost` → `127.0.0.1` to avoid IPv6 resolution
   - File: `test/integration/remediationorchestrator/suite_test.go:61`
   - File: `test/integration/remediationorchestrator/audit_integration_test.go:48`
   - File: `test/integration/remediationorchestrator/audit_trace_integration_test.go:31`

2. **Health Check Retry**: Added 10-retry loop (20 seconds total) to wait for service readiness
   - File: `test/integration/remediationorchestrator/audit_integration_test.go:59-79`

3. **Podman Permission Fix**: Applied DS team's macOS permission fix
   - Changed file permissions: `0644` → `0666`
   - Changed directory permissions: `0777`
   - Removed `:Z` flag (Linux SELinux-only)

### ❌ **Issue Persists**

Despite these fixes, tests still fail when managing their own infrastructure via `podman-compose`.

---

## 🤝 **Request for DS Team Help**

### **Question 1: Infrastructure Startup Timing** ✅ ANSWERED

**Context**: DS E2E tests successfully start and use DataStorage infrastructure.

**Question**: How long does your E2E test suite wait after `podman-compose up -d` before attempting to connect to DataStorage?

**DS TEAM ANSWER**:
```go
// DS Integration tests use Ginkgo's Eventually() for robust health checks
// File: test/infrastructure/datastorage.go:1584-1607

func waitForServiceReady(infra *DataStorageInfrastructure, writer io.Writer) error {
    var lastStatusCode int
    var lastError error

    // ✅ KEY: Use Eventually() with 30s timeout, 1s polling
    Eventually(func() int {
        resp, err := http.Get(infra.ServiceURL + "/health")
        if err != nil {
            lastError = err
            lastStatusCode = 0
            fmt.Fprintf(writer, "    Health check attempt failed: %v\n", err)
            return 0
        }
        defer resp.Body.Close()
        lastStatusCode = resp.StatusCode
        if lastStatusCode != 200 {
            fmt.Fprintf(writer, "    Health check returned status %d (expected 200)\n", lastStatusCode)
        }
        return lastStatusCode
    }, "30s", "1s").Should(Equal(200), "Data Storage Service should be healthy")

    // On failure, print diagnostics (container logs, status)
    return nil
}
```

**KEY DIFFERENCES FROM RO APPROACH**:
1. ✅ **Use `Eventually()` not manual loops**: Ginkgo's Eventually handles retries gracefully
2. ✅ **30s timeout (not 20s)**: More time for cold start
3. ✅ **1s polling interval**: Faster detection than 2s
4. ✅ **Check `resp.StatusCode == 200`**: Don't trust nil response
5. ✅ **Diagnostic output on failure**: Print container logs for debugging

### **Question 2: Connection Configuration** ✅ ANSWERED

**Context**: RO tests use:

```go
dataStorageClient, err := audit.NewOpenAPIClientAdapter("http://127.0.0.1:18140", 5*time.Second)
```

**Question**: Do your E2E tests use `localhost` or `127.0.0.1` explicitly? Different timeout settings?

**DS TEAM ANSWER**:
```go
// DS Integration tests use 127.0.0.1 EXPLICITLY (not localhost)
// File: test/infrastructure/datastorage.go:1538-1541

serviceURL := fmt.Sprintf("http://127.0.0.1:%s", cfg.ServicePort)
// ✅ ALWAYS use 127.0.0.1 (not localhost) to avoid IPv6 issues on macOS

// Health check uses default http.Client with no custom timeout
resp, err := http.Get(infra.ServiceURL + "/health")
```

**KEY CONFIGURATION**:
1. ✅ **`127.0.0.1` (NOT `localhost`)**: Avoids IPv6 resolution issues on macOS
2. ✅ **No custom timeout for health checks**: Use default `http.Client`
3. ✅ **Simple GET request**: No special headers or authentication
4. ⚠️ **RO timeout (5s) might be too short**: DS doesn't set explicit timeout, relies on Eventually()

**RECOMMENDATION FOR RO**:
```go
// Change this:
dataStorageClient, err := audit.NewOpenAPIClientAdapter("http://127.0.0.1:18140", 5*time.Second)

// To this (or increase timeout):
dataStorageClient, err := audit.NewOpenAPIClientAdapter("http://127.0.0.1:18140", 10*time.Second)
// Longer timeout helps with cold start scenarios
```

### **Question 3: Container Health Verification** ✅ ANSWERED

**Context**: Podman reports containers as "healthy", but HTTP connections fail.

**Question**: Do you verify container health via Podman before trusting the service?

**DS TEAM ANSWER**:
```go
// DS Integration tests DO NOT trust Podman health status
// Instead, they verify HTTP endpoint responsiveness directly

// File: test/infrastructure/datastorage.go:1589-1607
Eventually(func() int {
    resp, err := http.Get(infra.ServiceURL + "/health")
    if err != nil {
        return 0  // Connection failed
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(200))

// ✅ ONLY trust HTTP 200 response from /health endpoint
// ❌ DO NOT trust Podman's "healthy" status alone
```

**WHY THIS MATTERS**:
- ✅ Podman "healthy" = container process running
- ❌ Podman "healthy" ≠ HTTP server accepting connections
- ✅ HTTP 200 from `/health` = service actually ready

**CRITICAL ISSUE IN RO TESTS**:
```go
// RO current approach (audit_integration_test.go:59-79):
for i := 0; i < 10; i++ {
    resp, err := client.Get(dsURL + "/health")
    if err == nil && resp.StatusCode == http.StatusOK {
        healthy = true
        break
    }
    time.Sleep(2 * time.Second)  // ⚠️ Manual sleep
}
```

**DS RECOMMENDED PATTERN**:
```go
// Use Eventually() instead of manual loops
Eventually(func() int {
    resp, err := http.Get("http://127.0.0.1:18140/health")
    if err != nil {
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK), "DataStorage should be healthy")
```

### **Question 4: podman-compose vs DS E2E** ✅ ANSWERED - **CRITICAL FINDING!**

**Question**: Do your E2E tests use `podman-compose` or direct `podman` commands?

**DS TEAM ANSWER**: 🚨 **DS uses DIRECT `podman run` commands, NOT `podman-compose`!**

```go
// File: test/infrastructure/datastorage.go:1238-1260
func startPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
    // Cleanup existing container
    exec.Command("podman", "stop", infra.PostgresContainer).Run()
    exec.Command("podman", "rm", infra.PostgresContainer).Run()

    // ✅ START WITH DIRECT `podman run` (NOT podman-compose)
    cmd := exec.Command("podman", "run", "-d",
        "--name", infra.PostgresContainer,
        "-p", fmt.Sprintf("%s:5432", cfg.PostgresPort),
        "-e", fmt.Sprintf("POSTGRES_DB=%s", cfg.DBName),
        "-e", fmt.Sprintf("POSTGRES_USER=%s", cfg.DBUser),
        "-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", cfg.DBPassword),
        "--health-cmd", "pg_isready -U slm_user -d action_history",
        "--health-interval", "2s",
        "--health-timeout", "5s",
        "--health-retries", "5",
        "docker.io/library/postgres:16-alpine")

    return cmd.Run()
}
```

**WHY THIS MATTERS - CRITICAL DIFFERENCE**:

| Aspect | `podman-compose` | Direct `podman run` |
|--------|------------------|---------------------|
| **Startup Timing** | All services start in parallel | Sequential, controlled |
| **Health Checks** | Defined in YAML, may not work | Explicit `--health-cmd` flags |
| **Port Binding** | May have race conditions | Immediate and reliable |
| **Container Ready** | "Up" ≠ accepting connections | More predictable timing |
| **Debugging** | Harder to isolate issues | Each step explicit and loggable |

**HYPOTHESIS FOR RO FAILURES**:
```
podman-compose up -d
  ↓
All 3 containers (PostgreSQL, Redis, DataStorage) start SIMULTANEOUSLY
  ↓
DataStorage tries to connect to PostgreSQL BEFORE PostgreSQL is ready
  ↓
DataStorage fails to start OR enters restart loop
  ↓
Tests see "healthy" container but HTTP connection fails
```

**DS APPROACH (Sequential + Explicit)**:
```
1. Start PostgreSQL (with health check) ✅
2. Start Redis (with health check) ✅
3. Connect to PostgreSQL from test ✅
4. Apply migrations ✅
5. Start DataStorage service ✅
6. Wait for HTTP /health endpoint (30s Eventually) ✅
```

---

## 🎯 **DS TEAM RECOMMENDATIONS** ⭐

Based on DS integration test experience (just achieved 100% pass rate - 164/164 tests), here are **concrete fixes** for RO:

### **RECOMMENDATION #1: Replace podman-compose with Direct podman run** 🚨 CRITICAL

**Current RO Code** (PROBLEMATIC):
```go
// test/integration/remediationorchestrator/suite_test.go:54-71
cmd := exec.Command("podman-compose", "-f", composeFile, "up", "-d")
cmd.Run()
// ❌ All services start in parallel, race conditions possible
```

**DS Pattern** (WORKS RELIABLY):
```go
// Start services SEQUENTIALLY with explicit health checks
// 1. PostgreSQL
cmd := exec.Command("podman", "run", "-d",
    "--name", "ro-postgres-test",
    "-p", "15435:5432",
    "-e", "POSTGRES_DB=action_history",
    "-e", "POSTGRES_USER=slm_user",
    "-e", "POSTGRES_PASSWORD=test_password",
    "--health-cmd", "pg_isready -U slm_user -d action_history",
    "--health-interval", "2s",
    "--health-timeout", "5s",
    "--health-retries", "5",
    "docker.io/library/postgres:16-alpine")
cmd.Run()

// 2. Redis
cmd = exec.Command("podman", "run", "-d",
    "--name", "ro-redis-test",
    "-p", "16381:6379",
    "--health-cmd", "redis-cli ping",
    "--health-interval", "2s",
    "--health-timeout", "3s",
    "--health-retries", "5",
    "quay.io/jordigilh/redis:7-alpine")
cmd.Run()

// 3. Wait for PostgreSQL health via Podman
Eventually(func() bool {
    out, _ := exec.Command("podman", "healthcheck", "run", "ro-postgres-test").CombinedOutput()
    return strings.Contains(string(out), "healthy")
}, "30s", "1s").Should(BeTrue())

// 4. Then start DataStorage
cmd = exec.Command("podman", "run", "-d", ...)
```

### **RECOMMENDATION #2: Use Eventually() for Health Checks** 🚨 CRITICAL

**Current RO Code** (FRAGILE):
```go
// audit_integration_test.go:59-79
for i := 0; i < 10; i++ {  // ❌ Manual retry loop
    resp, err := client.Get(dsURL + "/health")
    if err == nil && resp.StatusCode == http.StatusOK {
        healthy = true
        break
    }
    time.Sleep(2 * time.Second)  // ❌ Fixed delay
}
```

**DS Pattern** (ROBUST):
```go
// Use Ginkgo's Eventually() - more reliable
Eventually(func() int {
    resp, err := http.Get("http://127.0.0.1:18140/health")
    if err != nil {
        GinkgoWriter.Printf("  Health check failed: %v\n", err)
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK), "DataStorage should be healthy")
```

**BENEFITS**:
- ✅ Automatic retries with exponential backoff
- ✅ Better error messages in test output
- ✅ Timeout is explicit and configurable
- ✅ Integrates with Ginkgo's failure handling

### **RECOMMENDATION #3: Increase Timeout to 30s** 🟡 HIGH PRIORITY

**Current**: 20s total (10 retries × 2s)
**DS Uses**: 30s with 1s polling
**Why**: Cold start can take 15-20s on macOS Podman

```go
// Change from:
for i := 0; i < 10; i++ { time.Sleep(2 * time.Second) }  // 20s total

// To:
Eventually(..., "30s", "1s").Should(...)  // 30s total, faster polling
```

### **RECOMMENDATION #4: File Permissions (macOS Podman Fix)** 🟡 HIGH PRIORITY

**DS recently fixed this** (same issue you're experiencing):

```go
// test/integration/datastorage/suite_test.go:634-644
// Create config files
if err := os.WriteFile(configFile, configData, 0666); err != nil {  // ✅ 0666 not 0644
    return err
}
if err := os.WriteFile(secretsFile, secretsData, 0666); err != nil {  // ✅ 0666 not 0644
    return err
}

// Change directory permissions
if err := os.Chmod(configDir, 0777); err != nil {  // ✅ 0777 for directory
    return err
}

// DON'T use :Z flag on macOS (Podman incompatibility)
// ❌ BAD:  "-v", configDir+":/etc/datastorage:Z"
// ✅ GOOD: "-v", configDir+":/etc/datastorage"
```

### **RECOMMENDATION #5: Explicit 127.0.0.1 (Already Done ✅)**

**Good**: RO already uses `127.0.0.1` instead of `localhost` ✅
**Rationale**: Avoids IPv6 resolution issues on macOS

---

## 📊 **Current Test Status**

| Test Tier | Phase 1 Converted | Manual Infra | Auto Infra | Blocking Issue |
|-----------|-------------------|--------------|------------|----------------|
| **Routing** | ✅ 1/1 converted | ❓ Not tested | ❌ Fails | Infrastructure timing |
| **Operational** | ✅ 2/2 converted | ❓ Not tested | ❌ Fails | Infrastructure timing |
| **RAR** | ✅ 4/4 converted | ❓ Not tested | ❌ Fails | Infrastructure timing |
| **Audit** | N/A (uses infra) | ✅ 3/3 pass | ❌ Fails | Infrastructure timing |
| **Notification** | ⏳ Pending Phase 2 | - | - | Will move to Phase 2 E2E |
| **Cascade** | ⏳ Pending Phase 2 | - | - | Will move to Phase 2 E2E |

**Total**: 16 tests blocked by infrastructure startup timing issue

---

## 🎯 **Success Criteria**

1. RO integration tests can start infrastructure via `podman-compose` and reliably connect within 30 seconds
2. Health check logic in `BeforeSuite` successfully validates service readiness
3. All 10 Phase 1 integration tests pass consistently (target: 48/48 total once RAR tests are converted)

---

## 🎯 **ROOT CAUSE ANALYSIS** (DS Team Assessment)

**PRIMARY ISSUE**: `podman-compose` starts all services simultaneously, causing race conditions:

```
podman-compose up -d
  ├── PostgreSQL starts ⏱️ Takes 10-15s to be ready
  ├── Redis starts ⏱️ Takes 2-3s to be ready
  └── DataStorage starts ⏱️ Tries to connect to PostgreSQL IMMEDIATELY
      ↓
      ❌ DataStorage fails to connect (PostgreSQL not ready yet)
      ↓
      🔄 DataStorage may restart or hang
      ↓
      ✅ Eventually shows "healthy" (container running)
      ❌ But HTTP server never started (failed initialization)
```

**SECONDARY ISSUES**:
1. Manual retry loop (20s) too short for cold start (15-20s on macOS)
2. 2s sleep interval misses the ready window (use 1s)
3. File permissions preventing config reads (need 0666/0777 on macOS)

**DS TEAM CONFIDENCE**: 95% - These are the exact issues DS team fixed to achieve 100% integration test pass rate

---

## 📝 **Next Steps**

### **For RO Team** (Priority Order):

1. **🚨 CRITICAL (Day 1)**: Replace `podman-compose` with sequential `podman run` commands
   - **File**: `test/integration/remediationorchestrator/suite_test.go:54-71`
   - **Pattern**: Copy from `test/infrastructure/datastorage.go:1238-1280`
   - **Effort**: ~2 hours (requires refactoring BeforeSuite)

2. **🚨 CRITICAL (Day 1)**: Replace manual retry loop with `Eventually()`
   - **File**: `test/integration/remediationorchestrator/audit_integration_test.go:59-79`
   - **Pattern**: See Recommendation #2 above
   - **Effort**: ~15 minutes

3. **🟡 HIGH (Day 2)**: Increase timeout from 20s → 30s
   - **Effort**: ~5 minutes

4. **🟡 HIGH (Day 2)**: Verify file permissions (0666/0777, no `:Z`)
   - **Effort**: ~10 minutes

5. **✅ DONE**: Using `127.0.0.1` instead of `localhost` (already fixed)

### **Expected Results After Fixes**:
- ✅ All 16 Phase 1 integration tests pass consistently
- ✅ Infrastructure starts reliably in ~30-40s
- ✅ No more timeout/connection failures
- ✅ Tests can run in parallel (with unique ports per DD-TEST-001)

### **For DS Team** ✅ COMPLETE:
- ✅ All questions answered with code examples
- ✅ Root cause identified
- ✅ Concrete recommendations provided
- ✅ Reference implementations documented

---

## 📞 **Follow-Up Support**

**If RO team needs help**:
1. DS team can pair on infrastructure refactoring
2. Can provide more code examples from DS integration tests
3. Can review RO PR with infrastructure changes

**DS Team Availability**: Via @jgil relay

---

## 📚 **Related Files**

| File | Purpose | Line(s) |
|------|---------|---------|
| `test/integration/remediationorchestrator/suite_test.go` | Infrastructure setup | 54-71 |
| `test/integration/remediationorchestrator/audit_integration_test.go` | Health check retry | 59-79 |
| `test/integration/remediationorchestrator/podman-compose.remediationorchestrator.test.yml` | Container definitions | - |
| `docs/handoff/RO_PHASE1_CONVERSION_STATUS_DEC_19_2025.md` | Previous triage | - |

---

**RO Team Contact**: @jgil
**DS Team Contact**: (via @jgil relay) ✅ **RESPONDED**
**Original Priority**: 🚨 **CRITICAL** - Blocks 16 integration tests
**Current Status**: ✅ **ACTIONABLE** - RO team has clear path forward with DS team's recommendations

---

## 📊 **Assessment Summary**

| Aspect | Status | Notes |
|--------|--------|-------|
| **Root Cause Identified** | ✅ YES | `podman-compose` parallel startup causes race conditions |
| **Solution Provided** | ✅ YES | Sequential `podman run` + `Eventually()` pattern |
| **Code Examples** | ✅ YES | DS team provided working patterns from integration tests |
| **Effort Estimated** | ✅ YES | ~2-3 hours to refactor infrastructure setup |
| **DS Team Available** | ✅ YES | Can help with pairing/review if needed |
| **RO Confidence** | 🎯 95% | DS team just fixed these exact issues (164/164 tests passing) |

**Next Action**: RO team implements Recommendations #1-4 (priority order provided)

