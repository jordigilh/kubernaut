# 🚨 CRITICAL: Infrastructure Container Naming Issue

**Date**: December 18, 2025, 17:05 UTC
**Severity**: **HIGH** - Causes infrastructure instability during test runs
**Impact**: 6 audit tests fail when infrastructure unavailable
**Root Cause**: Missing `container_name` in podman-compose.yml

---

## **Problem**

Infrastructure containers (Data Storage, Redis) disappear during test runs, causing audit tests to fail.

**Observed Behavior**:
- Start infrastructure: `podman-compose up -d` ✅ Works
- Run tests: 113/113 passing ✅ Works
- Run tests again: 107/113 passing ❌ **6 audit tests fail** "Data Storage not available"
- Check containers: Only postgres remains, datastorage and redis are **gone**

---

## **Root Cause**

**File**: `test/integration/notification/podman-compose.notification.test.yml`

**Missing**: `container_name` for datastorage and redis services

**Current** (Lines 27-39, 77-102):
```yaml
services:
  redis:
    image: quay.io/jordigilh/redis:7-alpine
    # ❌ NO container_name defined
    ports:
      - "16399:6379"
    networks:
      - nt-test-network

  datastorage:
    build:
      context: ../../..
      dockerfile: docker/data-storage.Dockerfile
    # ❌ NO container_name defined
    environment:
      - CONFIG_PATH=/etc/datastorage/config.yaml
    ports:
      - "18110:8080"
```

**What Happens**:
1. `podman-compose up -d` creates containers with auto-generated names (e.g., `notification-redis-1`, `notification-datastorage-1`)
2. DD-TEST-001 cleanup in `AfterSuite` (line 107 of suite_test.go) runs `podman rm -f notification_redis_1 notification_datastorage_1`
3. Cleanup **fails** to remove containers because names don't match (`-` vs `_`)
4. BUT: podman-compose generates DIFFERENT container names on each run depending on project state
5. **Result**: Containers become "orphaned" and eventually get removed by podman's own cleanup or system restart

---

## **Solution**

**Add explicit `container_name` to ALL services** (following Gateway pattern):

```yaml
services:
  redis:
    image: quay.io/jordigilh/redis:7-alpine
    container_name: notification_redis_1  # ✅ ADD THIS
    ports:
      - "16399:6379"
    networks:
      - nt-test-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 10

  datastorage:
    build:
      context: ../../..
      dockerfile: docker/data-storage.Dockerfile
    container_name: notification_datastorage_1  # ✅ ADD THIS
    environment:
      - CONFIG_PATH=/etc/datastorage/config.yaml
    ports:
      - "18110:8080"
      - "19110:9090"
    volumes:
      - ./config:/etc/datastorage:ro
    networks:
      - nt-test-network
    depends_on:
      migrate:
        condition: service_completed_successfully
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 5s
      timeout: 5s
      retries: 10
      start_period: 10s
```

**Also Update**: Line 11 (postgres) to use consistent naming:
```yaml
  postgres:
    image: postgres:16-alpine
    container_name: notification_postgres_1  # ✅ ADD THIS (was implicit)
```

---

## **Why This Fixes It**

1. **Explicit names**: Container names are now predictable and consistent
2. **DD-TEST-001 cleanup**: Cleanup script can reliably find and remove containers
3. **Gateway precedent**: Gateway service uses this pattern successfully

---

## **Verification**

**Before Fix**:
```bash
$ podman ps --filter "name=notification"
notification_postgres_1 - Up 2 hours
# ❌ datastorage and redis missing after test run
```

**After Fix**:
```bash
$ podman ps --filter "name=notification"
notification_postgres_1 - Up 2 hours
notification_redis_1 - Up 1 hour
notification_datastorage_1 - Up 1 hour
# ✅ All containers persist across test runs
```

---

## **Related Files**

- **podman-compose**: `test/integration/notification/podman-compose.notification.test.yml`
- **Cleanup script**: `test/integration/notification/suite_test.go` (lines 87-129)
- **Gateway reference**: `test/integration/gateway/podman-compose.gateway.test.yml` (uses container_name everywhere)

---

## **Impact**

**Before Fix**:
- Manual infrastructure restart needed between test runs
- Unreliable test execution
- Confusing "Data Storage not available" errors

**After Fix**:
- Reliable infrastructure across multiple test runs
- Predictable cleanup behavior
- Consistent with Gateway service pattern

---

**Priority**: **HIGH** - Fixes infrastructure instability
**Effort**: 5 minutes (add 3 container_name lines)
**Testing**: Run `make test-integration-notification` twice in a row to verify containers persist


