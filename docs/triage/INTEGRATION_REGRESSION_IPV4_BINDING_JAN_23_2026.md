# Integration Test Regression: IPv4 Explicit Binding Issue

**Date**: January 23, 2026  
**CI Run**: 21302193075  
**Branch**: `feature/soc2-compliance`  
**Commit**: `73788539`

---

## Summary

**REGRESSION**: 7 integration test suites now failing (previously 4)

**Failing Services**:
1. ❌ DataStorage
2. ❌ SignalProcessing  
3. ❌ Notification
4. ❌ RemediationOrchestrator
5. ❌ WorkflowExecution
6. ❌ AuthWebhook
7. ❌ Gateway

**Passing Services**:
- ✅ AIAnalysis (still running)
- ✅ HolmesGPTAPI (still running)

---

## Root Cause

### Change That Caused Regression

**File**: `test/infrastructure/datastorage_bootstrap.go`  
**Lines**: 358 (PostgreSQL), 412 (Redis)

```go
// ❌ REGRESSION: This breaks host.containers.internal access
"-p", fmt.Sprintf("127.0.0.1:%d:5432", cfg.PostgresPort),  // PostgreSQL
"-p", fmt.Sprintf("127.0.0.1:%d:6379", cfg.RedisPort),     // Redis
```

### Why This Breaks

**Port Binding Semantics**:
- `-p 127.0.0.1:host_port:container_port` → Port accessible ONLY from host's loopback (127.0.0.1)
- `-p host_port:container_port` → Port accessible from all interfaces (0.0.0.0 and ::1)

**Container Network Access**:
- Services use `host.containers.internal` to access PostgreSQL/Redis from within container network
- `host.containers.internal` resolves to the **host's IP address** (not 127.0.0.1)
- If port is bound ONLY to 127.0.0.1, it's **not accessible** via `host.containers.internal`

### Evidence

1. **Local Tests**: Passed because Go test code connects via `127.0.0.1:port` directly
2. **CI Tests**: Fail because DataStorage container uses `host.containers.internal:port`

**Example from `test/integration/aianalysis/config/config.yaml`**:
```yaml
database:
  host: host.containers.internal  # ❌ Cannot reach 127.0.0.1-bound port
  port: 15438
```

---

## Proposed Solutions

### Option A: Revert and Use Different Approach (RECOMMENDED)

**Revert port binding to original format**:
```go
"-p", fmt.Sprintf("%d:5432", cfg.PostgresPort),  // Allow all interfaces
"-p", fmt.Sprintf("%d:6379", cfg.RedisPort),
```

**Instead, configure PostgreSQL to listen ONLY on IPv4**:
```go
cmd := exec.Command("podman", "run", "-d",
    "--name", infra.PostgresContainer,
    "--network", infra.Network,
    "-p", fmt.Sprintf("%d:5432", cfg.PostgresPort),
    "-e", fmt.Sprintf("POSTGRES_USER=%s", defaultPostgresUser),
    "-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", defaultPostgresPassword),
    "-e", fmt.Sprintf("POSTGRES_DB=%s", defaultPostgresDB),
    "-c", "listen_addresses='0.0.0.0'",  // PostgreSQL config: IPv4 only
    "postgres:16-alpine",
)
```

**Pro**: Preserves container network functionality  
**Con**: PostgreSQL still listens on IPv6 inside container (but not exposed to host)

### Option B: Change Client Configuration (NOT RECOMMENDED)

Change all service configs to use `127.0.0.1` instead of `host.containers.internal`.

**Pro**: Works with explicit loopback binding  
**Con**: 
- Breaks container-to-container communication pattern
- Requires changes to 9 service configurations
- Doesn't match production patterns

### Option C: Use Podman --network=host (NOT RECOMMENDED)

Run containers with `--network=host` instead of custom network.

**Pro**: No port binding issues  
**Con**:
- Breaks container isolation
- Requires all services to use unique ports
- Not suitable for parallel test execution

---

## Recommended Fix

**Revert the IPv4 explicit binding change**:

```bash
git revert 73788539
```

**Then investigate the original AIAnalysis failure** with a different approach:
1. Check if AIAnalysis failure was actually IPv6-related or a different issue
2. Consider PostgreSQL configuration changes instead of port binding changes
3. Verify GitHub Actions network stack behavior

---

## Impact Assessment

**Severity**: **CRITICAL** - 7/9 integration test suites failing (77% failure rate)

**Blast Radius**: All services using `datastorage_bootstrap.go`:
- Gateway
- SignalProcessing
- WorkflowExecution
- RemediationOrchestrator
- Notification
- AuthWebhook
- AIAnalysis
- DataStorage
- HolmesGPTAPI

**Timeline**: Immediate revert required to unblock SOC2 compliance PR

---

## Next Steps

1. ✅ **IMMEDIATE**: Revert commit `73788539`
2. ✅ **SHORT-TERM**: Re-investigate original AIAnalysis failure
3. ✅ **LONG-TERM**: Consider network architecture improvements

---

## Lessons Learned

1. **Test Locally AND in CI**: Local tests passed but CI failed due to network differences
2. **Understand Port Binding Semantics**: `127.0.0.1` binding breaks container network access
3. **Incremental Changes**: Test each service individually before mass changes
4. **Network Patterns**: `host.containers.internal` requires ports accessible from host's network interface
