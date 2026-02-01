# Integration Test Failure RCA: Host Network Port Mismatch (Feb 01, 2026)

**Date**: 2026-02-01  
**CI Run**: [21554327749](https://github.com/jordigilh/kubernaut/actions/runs/21554327749)  
**Status**: ❌ 8/9 Integration Tests Failed  
**Severity**: Critical (blocks PR merge)

---

## Executive Summary

**ROOT CAUSE**: Port mismatch in host network mode - DataStorage listening on port 8080 while tests expect service-specific ports (15440, 18096, etc.).

**IMPACT**: 100% failure rate for 8 services (Notification, RO, WE, AIAnalysis, Gateway, SP, AuthWebhook, HolmesGPT-API). Only DataStorage INT passed (uses default port 8080).

**RESOLUTION**: Set `PORT` environment variable to `cfg.DataStoragePort` when using `--network=host`.

---

## ✅ What Worked

1. **Healthcheck Fix** (Commit `a67110871`): All containers reached healthy status
2. **Platform Detection** (Commit `76aecc5f7`): `--network=host` enabled on Linux CI
3. **Authentication**: No "connection refused" errors - DataStorage can now reach envtest K8s API

---

## ❌ What Broke

### Technical Root Cause

**With `--network=host` mode:**
```go
// Current (BROKEN):
args = append(args, "-e", "PORT=8080")  // Always 8080

// Container listens on: localhost:8080
// Test expects: localhost:18096 (cfg.DataStoragePort)
// Result: Connection timeout after 30s
```

**Why This Happens:**
1. Host network mode: No port mapping occurs
2. Service listens on whatever `PORT` env var specifies
3. We hardcoded `PORT=8080` for all services
4. Each service has different configured ports:
   - Notification: 18096
   - RO: 18140
   - Gateway: 18091
   - etc.

---

## 📊 Evidence

### Container Configuration
```json
{
  "HostConfig": {
    "NetworkMode": "host"  // ✅ Correct
  },
  "Config": {
    "Env": [
      "PORT=8080",          // ❌ Wrong! Should be 18096
      "POSTGRES_PORT=15440" // ✅ Correct (external port)
    ]
  }
}
```

### Test Failure Pattern
```
Error: timeout waiting for http://localhost:18096/health to become healthy after 30s

DataStorage logs show:
  "HTTP server listening" {"addr": ":8080"}  // Listening on 8080
  
Test trying to connect:
  GET http://localhost:18096/health  // Connecting to 18096
  
Result: ECONNREFUSED (nothing listening on 18096)
```

---

## 🎯 Root Cause Analysis by Group

### Group 1: All 8 Failed Services (Same Root Cause)

**Services**: Notification, RO, WE, AIAnalysis, Gateway, SP, AuthWebhook, HolmesGPT-API

**RCA**: Port mismatch in host network mode

**Failure Pattern**:
```
[FAILED] DataStorage failed to become healthy: 
  timeout waiting for http://localhost:{service_port}/health to become healthy after 30s
```

**Service-Specific Details**:

| Service | Expected Port | Actual Port | Gap |
|---------|--------------|-------------|-----|
| Notification | 18096 | 8080 | 10016 |
| RO | 18140 | 8080 | 10060 |
| Gateway | 18091 | 8080 | 10011 |
| AIAnalysis | 18095 | 8080 | 10015 |
| WE | 18097 | 8080 | 10017 |
| SP | 18094 | 8080 | 10014 |
| AuthWebhook | 18099 | 8080 | 10019 |
| HolmesGPT-API | 18098 | 8080 | 10018 |

**Evidence**:
- All containers: `NetworkMode: host` ✅
- All containers: `PORT=8080` ❌
- All containers: Healthy status ✅
- All containers: PostgreSQL/Redis connection successful ✅
- All tests: HTTP health check timeout ❌

---

### Group 2: DataStorage INT (Passed - Used Default Port)

**Service**: DataStorage  
**Status**: ✅ PASSED  
**Why**: DataStorage INT uses port 8080 by default (matches hardcoded PORT)

**Configuration**:
```
DataStoragePort: 8080  // Matches hardcoded PORT=8080
Result: SUCCESS
```

---

## 🔧 Fix Required

### Code Location
`test/infrastructure/datastorage_bootstrap.go:642`

### Current Code (Broken)
```go
args = append(args,
    "-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
    "-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
    "-e", fmt.Sprintf("POSTGRES_HOST=%s", postgresHost),
    "-e", fmt.Sprintf("POSTGRES_PORT=%d", postgresPort),
    // ... other vars ...
    "-e", "PORT=8080",  // ❌ WRONG for host network mode
)
```

### Fixed Code
```go
// Determine listen port based on network mode
var listenPort int
if useHostNetwork {
    // Host network: Listen on external port (no port mapping)
    listenPort = cfg.DataStoragePort  // e.g., 18096 for Notification
} else {
    // Bridge network: Always listen on 8080 (port mapping handles external)
    listenPort = 8080
}

args = append(args,
    "-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
    "-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
    "-e", fmt.Sprintf("POSTGRES_HOST=%s", postgresHost),
    "-e", fmt.Sprintf("POSTGRES_PORT=%d", postgresPort),
    // ... other vars ...
    "-e", fmt.Sprintf("PORT=%d", listenPort),  // ✅ Correct for both modes
)
```

---

## 🧪 Validation Plan

### Phase 1: Fix Implementation
- [x] RCA completed
- [ ] Apply port fix to `datastorage_bootstrap.go`
- [ ] Commit with detailed explanation

### Phase 2: Local Verification (if macOS with IPv6 disabled)
```bash
# Run single service locally
cd test/integration/notification
go test -v -timeout=30m

# Verify:
# 1. Container uses host network
# 2. PORT env var matches cfg.DataStoragePort
# 3. Health check succeeds
```

### Phase 3: CI Verification
- [ ] Push fix to branch
- [ ] Monitor CI run for all 8 services
- [ ] Expected: 9/9 integration tests pass

---

## 📈 Expected Outcomes

**Before Fix**:
- Integration Tests: 1/9 pass (11%)
- Healthcheck: 9/9 healthy ✅
- Authentication: 9/9 working ✅
- HTTP Health: 1/9 reachable ❌

**After Fix**:
- Integration Tests: 9/9 pass (100%) ✅
- Healthcheck: 9/9 healthy ✅
- Authentication: 9/9 working ✅
- HTTP Health: 9/9 reachable ✅

---

## 🔗 Related

- **Previous Fix**: Healthcheck syntax (`a67110871`)
- **Previous Fix**: Platform detection (`76aecc5f7`)
- **Authority**: DD-AUTH-014, DD_AUTH_014_MACOS_PODMAN_LIMITATION.md
- **Related RCA**: INT_TEST_FAILURE_RCA_JAN_31_2026.md

---

## 💡 Lessons Learned

1. **Host network != Bridge network**: Port mapping doesn't exist in host mode
2. **Service must bind to configured port**: When using host network, service must listen on the external port
3. **Test each platform mode**: Host vs bridge have different port requirements
4. **Port configuration is network-dependent**: Can't use same PORT value for both modes

---

## 🎯 Summary

| Aspect | Status | Details |
|--------|--------|---------|
| **Healthcheck** | ✅ Fixed | All containers healthy |
| **Authentication** | ✅ Fixed | TokenReview API working |
| **Network Mode** | ✅ Fixed | Host network on Linux |
| **Port Configuration** | ❌ **NEEDS FIX** | PORT mismatch in host mode |

**Next Action**: Apply port fix and retest on CI.
