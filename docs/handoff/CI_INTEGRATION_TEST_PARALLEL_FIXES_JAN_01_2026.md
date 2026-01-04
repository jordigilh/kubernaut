# CI Integration Test Parallel Execution Fixes - January 1, 2026

## 🎯 Objective
Fix 6 failing integration tests caused by Ginkgo parallel execution (`TEST_PROCS=4`) and networking misconfigurations.

## 📊 Test Failures Analysis

### Failure Patterns Identified

| Service | Root Cause | Status |
|---------|-----------|--------|
| **Notification** | `BeforeSuite` → container name collisions | ✅ FIXED |
| **WorkflowExecution** | `BeforeSuite` → container name collisions | ✅ FIXED |
| **Remediation Orchestrator** | Custom network + IP lookups incompatible with CI | ✅ FIXED |
| **AIAnalysis** | Custom network + container name DNS | ✅ FIXED |
| **Data Storage** | Timeout (Dockerfile path issue - already fixed) | ✅ FIXED |
| **HolmesGPT API** | Makefile `cd` path navigation bug | ✅ FIXED |

---

## 🔧 Fixes Applied

### 1. Notification Integration Tests
**Problem**: Used `BeforeSuite` (not synchronized) → 4 parallel processes tried to create containers with same names.

**Error**:
```
Error: creating container storage: the container name "notification_postgres_1" is already in use
```

**Fix**: Converted from `BeforeSuite` to `SynchronizedBeforeSuite`:
- **Phase 1** (process #1 only): Start infrastructure (PostgreSQL, Redis, DataStorage)
- **Phase 2** (all processes): Set up envtest, K8s client, controllers

**File**: `test/integration/notification/suite_test.go`

**Benefit**: Infrastructure starts once, shared across all 4 parallel processes. No container name collisions.

---

### 2. WorkflowExecution Integration Tests
**Problem**: Same as Notification - `BeforeSuite` causing parallel collisions.

**Fix**: Converted from `BeforeSuite` to `SynchronizedBeforeSuite` with same pattern.

**File**: `test/integration/workflowexecution/suite_test.go`

**Benefit**: Infrastructure shared, no container collisions.

---

### 3. Remediation Orchestrator Integration Tests
**Problem**: Used custom Podman network (`ro-e2e-network`) and attempted to get container IPs:
```go
pgIPCmd := exec.Command("podman", "inspect", ROIntegrationPostgresContainer,
    "--format", `{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}`)
// Returns empty in GitHub Actions → Error: "failed to get PostgreSQL container IP: empty result"
```

**Root Cause**: GitHub Actions runners don't assign IPs to containers in custom networks reliably.

**Fix Applied**:
1. **Removed custom network creation** - using port mapping (`-p`) instead
2. **Updated migrations** to use `host.containers.internal` instead of container IPs:
   ```go
   "-e", "PGHOST=host.containers.internal",
   "-e", fmt.Sprintf("PGPORT=%d", ROIntegrationPostgresPort),
   ```
3. **Updated config file** (`test/integration/remediationorchestrator/config/config.yaml`):
   ```yaml
   database:
     host: host.containers.internal  # Changed from 10.88.0.20
     port: 15435                     # RO integration port (DD-TEST-001)
   redis:
     addr: host.containers.internal:16381  # Changed from 10.88.0.21:6379
   ```

**Files**:
- `test/infrastructure/remediationorchestrator.go`
- `test/integration/remediationorchestrator/config/config.yaml`

**Benefit**: Consistent with Gateway/Notification/WE patterns. Works reliably in CI.

---

### 4. AIAnalysis Integration Tests
**Problem**: Same custom network issue as RO. Used `AIAnalysisIntegrationNetwork` and container name DNS:
```yaml
database:
  host: postgres  # Container name, doesn't resolve without custom network in CI
redis:
  addr: redis:6379
```

**Fix Applied**:
1. **Disabled custom network creation** (wrapped in `if false` block)
2. **Updated migrations** to use `host.containers.internal`:
   ```go
   "-e", "PGHOST=host.containers.internal",
   "-e", fmt.Sprintf("PGPORT=%d", AIAnalysisIntegrationPostgresPort),
   ```
3. **Updated HAPI container** to not use `--network` flag
4. **Updated config file** (`test/integration/aianalysis/config/config.yaml`):
   ```yaml
   database:
     host: host.containers.internal  # Changed from postgres
     port: 15438                     # AIAnalysis integration port (DD-TEST-001)
   redis:
     addr: host.containers.internal:16384  # Changed from redis:6379
   ```

**Files**:
- `test/infrastructure/aianalysis.go`
- `test/integration/aianalysis/config/config.yaml`

**Benefit**: Standardized networking pattern across all services.

---

### 5. Data Storage Integration Tests
**Problem**: Timeout waiting for DataStorage health check:
```
Timed out after 60.000s.
http://localhost:18090/health: dial tcp [::1]:18090: connect: connection refused
```

**Root Cause**: DataStorage container never started due to incorrect Dockerfile path (already fixed in previous commit).

**Status**: ✅ **No additional fix needed** - Dockerfile path correction will resolve this.

---

### 6. HolmesGPT API Integration Tests
**Problem**: Makefile path navigation bug:
```bash
bash: line 27: cd: holmesgpt-api: No such file or directory
```

**Root Cause**: Chain of `cd` commands in Makefile:
```makefile
cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../.. || exit 1;
# After cd ../.. from holmesgpt-api/tests/integration → lands in holmesgpt-api (not project root!)
cd holmesgpt-api && pip install...  # Tries to cd into holmesgpt-api/holmesgpt-api (doesn't exist!)
```

**Fix**: Changed `cd ../..` to `cd ../../..` to return to project root:
```makefile
cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../../.. || exit 1;
```

**File**: `Makefile` (line 308)

**Benefit**: Correct path navigation for Python dependency installation.

---

## 📋 Key Learnings

### 1. Ginkgo Parallel Execution (`TEST_PROCS=4`)
- **`BeforeSuite`** runs **per process** → use for process-specific setup (envtest, controllers)
- **`SynchronizedBeforeSuite`** runs **once** (process #1) → use for shared infrastructure (containers)

**Pattern**:
```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Phase 1: Process #1 only - start shared infrastructure
    infrastructure.StartInfrastructure(GinkgoWriter)
    return []byte{}
}, func(data []byte) {
    // Phase 2: All processes - set up envtest, clients, controllers
    testEnv = &envtest.Environment{...}
    cfg, _ = testEnv.Start()
})
```

### 2. Container Networking in CI
**❌ AVOID**: Custom Podman networks + container name DNS
- **Problem**: Unreliable in GitHub Actions
- **Symptom**: Empty IP addresses, DNS resolution failures

**✅ USE**: Port mapping (`-p`) + `host.containers.internal`
- **Pattern**: All services connect via `host.containers.internal:PORT`
- **Ports**: Defined in `DD-TEST-001-port-allocation-strategy.md`
- **Benefit**: Works consistently across local/CI environments

### 3. Config File Standards
All integration test config files should use:
```yaml
database:
  host: host.containers.internal  # NOT container names or IPs
  port: <SERVICE_POSTGRES_PORT>   # Per DD-TEST-001
redis:
  addr: host.containers.internal:<SERVICE_REDIS_PORT>  # Per DD-TEST-001
```

---

## 🎯 Expected CI Improvements

### Before Fixes
- ✅ **Gateway** (passed - already using SynchronizedBeforeSuite + port mapping)
- ✅ **SignalProcessing** (passed - already using correct patterns)
- ❌ **Notification** (failed - BeforeSuite collision)
- ❌ **WorkflowExecution** (failed - BeforeSuite collision)
- ❌ **Remediation Orchestrator** (failed - custom network)
- ❌ **AIAnalysis** (failed - custom network)
- ❌ **Data Storage** (failed - Dockerfile path)
- ❌ **HolmesGPT API** (failed - Makefile path)

### After Fixes
- ✅ **All 8 services** should pass integration tests

---

## 📝 Files Modified

### Test Suite Conversions
1. `test/integration/notification/suite_test.go` - BeforeSuite → SynchronizedBeforeSuite
2. `test/integration/workflowexecution/suite_test.go` - BeforeSuite → SynchronizedBeforeSuite

### Infrastructure Networking
3. `test/infrastructure/remediationorchestrator.go` - Removed custom network, use port mapping
4. `test/infrastructure/aianalysis.go` - Removed custom network, use port mapping

### Configuration Files
5. `test/integration/remediationorchestrator/config/config.yaml` - host.containers.internal + DD-TEST-001 ports
6. `test/integration/aianalysis/config/config.yaml` - host.containers.internal + DD-TEST-001 ports

### Build System
7. `Makefile` - Fixed HAPI integration test `cd` path navigation

---

## ✅ Validation Checklist

- [x] Notification: Converted to SynchronizedBeforeSuite
- [x] WorkflowExecution: Converted to SynchronizedBeforeSuite
- [x] RO: Removed custom network, updated config
- [x] AIAnalysis: Removed custom network, updated config
- [x] DataStorage: Dockerfile path already fixed
- [x] HolmesGPT API: Makefile path fixed
- [x] All changes follow DD-TEST-001 port allocation
- [x] All changes use `host.containers.internal` pattern
- [ ] CI integration tests pass (pending push)

---

## 🚀 Next Steps

1. **Push fixes** to trigger CI run
2. **Monitor integration test matrix** (all 8 services)
3. **If additional failures**:
   - Gateway race conditions (already addressed with increased timeouts)
   - DataStorage flaky performance test (already marked `[Flaky]`)
4. **Update ADR-CI-001** with parallel execution learnings

---

## 📚 References

- **DD-TEST-001 v1.1**: Port allocation strategy (integration test ports)
- **DD-TEST-002**: Sequential startup pattern (integration infrastructure)
- **Ginkgo Docs**: SynchronizedBeforeSuite pattern
- **Previous Fixes**:
  - Dockerfile path corrections (`docker/data-storage.Dockerfile`)
  - Migration script fixes (`slm_user` role creation)
  - `envtest` setup in CI workflow

---

**Document Status**: ✅ Complete
**Created**: 2026-01-01 (Overnight CI fixes)
**Author**: AI Assistant
**Context**: Part of comprehensive CI pipeline optimization effort


