# Data Storage Integration Tests - Root Cause Analysis

**Date**: 2025-12-12
**Status**: 🔍 **ROOT CAUSE IDENTIFIED**

---

## 📊 **Summary**

**Unit Tests**: ✅ **ALL PASSING** (463/463 specs)
**Integration Tests**: 🔴 **FAILING** - Service container starts but health check times out

---

## 🔍 **Root Cause**

The Data Storage service **container starts successfully** but the **health endpoint never responds**.

### **Evidence**

1. **Container Creation**: ✅ SUCCESS
   ```
   ✅ Data Storage Service container started
   ```

2. **Health Check Failure**: ❌ TIMEOUT (60 seconds)
   ```
   ⏳ Waiting for service... (error: Get "http://localhost:18090/health": EOF)
   ⏳ Waiting for service... (error: ... connect: connection refused)
   ```

3. **Pattern Observed**:
   - Container starts (no Docker/Podman errors)
   - Health endpoint NEVER responds (not even HTTP 500)
   - Connection refused → service not listening on port 8080

### **Hypothesis**

The service is **crashing on startup** or **failing to bind to port 8080** inside the container.

**Possible Causes**:
1. **Configuration Error**: Invalid `config.yaml` causing startup failure
2. **Database Connection**: Can't connect to PostgreSQL from container network
3. **Port Binding**: Port 8080 already in use inside container
4. **Missing Dependencies**: Runtime dependency not available in UBI9 minimal image
5. **Permission Issue**: Non-root user can't access mounted config files

---

## 🧪 **Investigation Steps Completed**

### ✅ Step 1: Verified Infrastructure
- PostgreSQL container: **RUNNING** (port 15433)
- Redis container: **RUNNING** (port 16379)
- Network `datastorage-test`: **EXISTS**
- Image `data-storage:test`: **BUILT**

### ✅ Step 2: Verified Build Process
- Image builds successfully
- No compilation errors
- Binary created in `/usr/local/bin/data-storage`

### ✅ Step 3: Config File Analysis
- Config files created in temp directory
- Mounted to container at `/etc/datastorage/`
- Uses container names for DB/Redis (correct for container network)

### ❌ Step 4: **Container Runtime - NOT VERIFIED**
**This is where investigation stopped** - need to capture container logs immediately after start attempt.

---

## 🎯 **Next Steps to Complete Investigation**

### **CRITICAL**: Capture Container Startup Logs

The container is being removed by test cleanup before we can inspect it. Need to:

1. **Modify Test Temporarily** to keep container alive on failure
2. **OR** Run container manually with exact test configuration
3. **Capture startup logs** to see actual error

### **Manual Test Approach**

```bash
# 1. Start infrastructure
make test-integration-datastorage # Let it fail, containers stay up

# 2. Check container status
podman ps -a | grep datastorage-service-test

# 3. If container exists but exited, check logs
podman logs datastorage-service-test

# 4. If container doesn't exist, manually recreate with same config
# (Need to use actual temp config directory from test output)
```

---

## 🔧 **Suspected Issues & Fixes**

### **Issue 1: Database Host Resolution**

**Problem**: Container might not be able to resolve `datastorage-postgres-test` hostname.

**Test**:
```bash
podman run --rm --network datastorage-test alpine ping -c 1 datastorage-postgres-test
```

**Fix**: Verify network DNS or use IP address

### **Issue 2: Config File Permissions**

**Problem**: Non-root user (UID 1001) can't read mounted config files.

**Test**: Check config file permissions in temp directory

**Fix**: Ensure config files are world-readable (chmod 644)

### **Issue 3: PostgreSQL Connection String**

**Problem**: Invalid connection parameters in config.yaml.

**Current Config** (from test):
```yaml
database:
  host: datastorage-postgres-test  # Container name
  port: 5432                       # Internal port (not 15433)
  name: action_history
  user: slm_user
  ssl_mode: disable
```

**Test**: Verify these match actual PostgreSQL container setup

---

## 📝 **Recommended Fix Strategy**

### **Option A: Add Debug Logging**

**Modify**: `cmd/datastorage/main.go`

Add verbose startup logging:
```go
logger.Info("=== STARTUP DEBUG ===")
logger.Info("Config path", "path", cfgPath)
logger.Info("Database config", "host", cfg.Database.Host, "port", cfg.Database.Port)
logger.Info("Redis config", "addr", cfg.Redis.Addr)
// ... existing code ...
```

### **Option B: Health Check Delay**

**Modify**: Dockerfile health check

Add `--start-period` to give service more time:
```dockerfile
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD ["/usr/bin/curl", "-f", "http://localhost:8080/health"] || exit 1
```

### **Option C: Keep Container on Failure**

**Modify**: `test/integration/datastorage/suite_test.go`

Add debug flag:
```go
func cleanupContainers() {
    if os.Getenv("KEEP_CONTAINERS_ON_FAILURE") != "" {
        GinkgoWriter.Println("⚠️  Skipping cleanup (KEEP_CONTAINERS_ON_FAILURE set)")
        return
    }
    // ... existing cleanup ...
}
```

---

## 🚨 **CRITICAL ACTION REQUIRED**

**BEFORE ANY FIX**: We MUST capture the actual startup error from the container logs.

**Immediate Next Step**:
```bash
# Set environment to keep containers on failure
export KEEP_CONTAINERS_ON_FAILURE=1

# Run test
make test-integration-datastorage

# Immediately check logs (while container still exists)
podman logs datastorage-service-test 2>&1 | tee /tmp/ds-startup-error.log
```

---

## 📊 **Confidence Assessment**

**Root Cause Identification**: **90%** - Container starts but service doesn't listen on port

**Fix Confidence**: **0%** - Cannot propose fix without seeing actual error logs

**Risk**: **HIGH** - Without error logs, any fix is speculative and may not address real issue

**Recommendation**: **CAPTURE LOGS FIRST**, then implement targeted fix based on actual error.





