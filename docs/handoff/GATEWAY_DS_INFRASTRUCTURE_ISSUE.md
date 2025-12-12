# Gateway Data Storage Infrastructure Issue

**Date**: 2025-12-12  
**Team**: Gateway Service  
**Priority**: 🟡 **MEDIUM** - Blocks integration tests but workaround available  
**Status**: ⏸️ **BLOCKED** - Requires Redis or shared infrastructure refactor

---

## 🎯 **PROGRESS SUMMARY**

### ✅ **Completed** Work:
1. **Mock Fallback Removed** - No more silent failures (defense-in-depth)
2. **Config File Creation** - `config.yaml`, `db-secrets.yaml`, `redis-secrets.yaml`
3. **Secrets Mounting** - All secrets files correctly mounted
4. **Config Structure Fixed** - Separate `server:` section with `port` and `host`
5. **Network Mode Fixed** - Using `--network=host` for container communication
6. **PostgreSQL Connection** - ✅ **WORKING!** DS successfully connects to PostgreSQL

### ❌ **Current Blocker**:
**Redis Dependency**  
```
ERROR: failed to connect to Redis: dial tcp [::1]:6379: connect: connection refused
```

**Root Cause**: Data Storage service requires Redis connection even though Gateway doesn't use Redis (DD-GATEWAY-012). The DS service validates all config sections at startup per ADR-030.

---

## 📊 **COMMITS MADE** (7 total):

1. `1a8293bd` - Remove mock fallback, add config mount
2. `21b7d6a0` - Add db-secrets.yaml
3. `124d8c1a` - Add usernameKey/passwordKey fields
4. `a176cbd9` - Add Redis config and secrets
5. `46b702dc` - Fix config structure (server section)
6. `f6fa49bf` - Use host network mode
7. `[current HEAD]` - Latest changes

---

## 🔀 **OPTIONS**

### **Option A: Start Redis Container** (Quick Fix)
**Pros**:
- Gateway tests work immediately
- Minimal code changes

**Cons**:
- Duplicates infrastructure logic
- Gateway doesn't actually use Redis (DD-GATEWAY-012)
- Maintenance burden

**Implementation**:
```go
// Add to helpers_postgres.go before starting DS
redisCmd := exec.Command("podman", "run", "-d",
    "--name", "gateway-redis-integration",
    "--network=host",
    "docker.io/redis:7-alpine",
)
redisCmd.Run()
```

### **Option B: Use Shared Infrastructure** (Recommended)
**Pros**:
- Reuses proven `test/infrastructure/datastorage.go` code
- Handles PostgreSQL + Redis + DS containerization
- Consistent with other services (AIAnalysis, SignalProcessing)
- Robust health checks and error handling

**Cons**:
- Requires refactoring `helpers_postgres.go`
- More complex integration

**Implementation**:
```go
// In suite_test.go SynchronizedBeforeSuite
import "github.com/jordigilh/kubernaut/test/infrastructure"

dsInfra, err := infrastructure.StartDataStorageInfrastructure(&infrastructure.DataStorageConfig{
    ServicePort: "8080",
    DBName: "kubernaut_audit",
    DBUser: "kubernaut",
    DBPassword: "test_password",
}, GinkgoWriter)
```

### **Option C: Make DS Optional** (Risky)
**Pros**:
- Gateway tests don't strictly need audit storage to validate core logic

**Cons**:
- Breaks audit integration tests (BR-GATEWAY-186)
- Violates defense-in-depth testing (03-testing-strategy.mdc)
- Masks real DS integration issues

---

## 🎯 **RECOMMENDATION**

**⭐ Option B: Use Shared Infrastructure**

**Rationale**:
1. **Proven Pattern**: Already working in AIAnalysis, SignalProcessing services
2. **Maintainability**: Centralized infrastructure management
3. **Completeness**: Handles all dependencies (PostgreSQL, Redis, DS, migrations)
4. **Test Quality**: Uses REAL services for defense-in-depth testing

**Estimated Effort**: 2-3 hours
- 1 hour: Refactor `helpers_postgres.go` to use shared infra
- 1 hour: Update `suite_test.go` to initialize shared infra
- 1 hour: Run tests and validate

---

## 📝 **HANDOFF NOTES**

### **For Next Developer**:
1. **Current State**: DS container crashes due to missing Redis
2. **Files Modified**: `test/integration/gateway/helpers_postgres.go` (extensive changes)
3. **Commits**: 7 commits fixing DS config, networking, secrets
4. **Test Command**: `make test-gateway`
5. **Log Location**: `/tmp/gateway-test-with-host-network.log`

### **To Continue**:
```bash
# See all DS infrastructure commits
git log --oneline --grep="Data Storage" -n 10

# Review shared infrastructure pattern
cat test/infrastructure/datastorage.go

# Check current test failure
grep -A 20 "❌ Container logs:" /tmp/gateway-test-with-host-network.log
```

---

## 🔗 **RELATED DOCUMENTS**

- [RESPONSE_DS_CONFIG_FILE_MOUNT_FIX.md](./RESPONSE_DS_CONFIG_FILE_MOUNT_FIX.md) - Authoritative config pattern
- [TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md](./TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md) - Shared infra comparison
- [GATEWAY_REMAINING_8_FAILURES_RCA.md](./GATEWAY_REMAINING_8_FAILURES_RCA.md) - Test failure analysis
- [DD-GATEWAY-012](../architecture/decisions/DD-GATEWAY-012-redis-deprecation.md) - Redis removal decision

---

## ⏰ **TIME INVESTED**

- **Config Fixes**: ~2 hours (7 iterations)
- **Network Debugging**: ~30 minutes
- **Mock Removal**: ~30 minutes
- **Total**: ~3 hours

**Status**: Recommend switching to Option B (shared infrastructure) for faster resolution.

