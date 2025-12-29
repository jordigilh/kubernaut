# DD-TEST-002: Integration Test Container Orchestration Pattern

**Status**: ❌ **FULLY DEPRECATED** - DO NOT USE
**Date**: 2025-12-21 (Original) | 2025-12-27 (Fully Deprecated)
**Deciders**: DataStorage Team, Infrastructure Team
**Supersedes**: N/A
**Superseded By**: DD-INTEGRATION-001 v2.0 (Programmatic Podman Setup)
**Related**: DD-TEST-001 (Integration Test Port Allocation), DD-INTEGRATION-001 v2.0

---

## 🚨 **FULL DEPRECATION NOTICE (December 27, 2025)**

**This document is FULLY DEPRECATED and should NOT be used for any purpose.**

### **Why Deprecated**

This document contains **conflicting and outdated guidance** that contradicts the authoritative standard:

1. ❌ **Wrong image tags**: Uses `datastorage:latest` instead of `{service}-{uuid}`
2. ❌ **Shell scripts**: Recommends `./setup-infrastructure.sh` (forbidden)
3. ❌ **Outdated patterns**: Recommends `podman-compose` for some scenarios
4. ❌ **Incomplete migration status**: Claims services "pending" that are already migrated

### **What to Use Instead**

**AUTHORITATIVE DOCUMENT**: [DD-INTEGRATION-001 v2.0](./DD-INTEGRATION-001-local-image-builds.md)

**For Go Services**:
- Use programmatic Podman setup via `test/infrastructure/{service}_integration.go`
- Use shared utilities from `test/infrastructure/shared_integration_utils.go`
- Use composite image tags: `{service}-{uuid}`
- NO shell scripts

**For Python Services** (HolmesGPT-API):
- Use pytest fixtures (documented in DD-INTEGRATION-001 v2.0)
- Framework manages infrastructure (no shell scripts)
- See: `holmesgpt-api/tests/integration/conftest.py`

### **Valid Content Consolidated**

The ONLY valid content from this document (Python pytest fixtures pattern) has been **consolidated into DD-INTEGRATION-001 v2.0**.

**See**: [DD-INTEGRATION-001 v2.0 - Python Services](./DD-INTEGRATION-001-local-image-builds.md#-python-services-pytest-fixtures-pattern)

### **Triage Document**

**See**: `docs/handoff/DD_TEST_002_DEPRECATION_TRIAGE_DEC_27_2025.md` for detailed analysis of what was preserved vs. deprecated.

---

## 📋 **Historical Context (For Reference Only)**

**This section is kept for historical reference. DO NOT follow these patterns.**

---

## 📋 Context and Problem Statement

Integration tests for microservices require real infrastructure (PostgreSQL, Redis, DataStorage) to validate cross-service interactions. The standard approach using `podman-compose up -d` causes **race conditions** where services with startup dependencies crash repeatedly, leading to:

- **Exit 137 (SIGKILL)** - Containers killed after restart limit
- **DNS resolution failures** - "lookup postgres: no such host"
- **Health check failures** - Services show "healthy" but HTTP server never starts
- **BeforeSuite failures** - All tests skipped before execution

**Affected Services** (as of 2025-12-27):
- ✅ **DataStorage**: Fixed using sequential startup (Dec 20, 2025)
- ✅ **HolmesGPT-API**: Fixed using Python pytest fixtures (Dec 27, 2025)
- ⚠️ **RemediationOrchestrator**: Known issue, pending fix
- ⚠️ **Notification**: Known issue, pending fix
- ⚠️ **Other services**: At risk if using multi-service dependencies

---

## 🔍 Root Cause Analysis

### The Race Condition

`podman-compose up -d` starts **all services simultaneously**:

```bash
podman-compose up -d
  ├── PostgreSQL starts ⏱️ Takes 10-15 seconds to be ready
  ├── Redis starts ⏱️ Takes 2-3 seconds to be ready
  └── DataStorage starts ⚡ Tries to connect IMMEDIATELY
      ↓
      ❌ Connection fails (PostgreSQL not ready yet)
      ↓
      🔄 Container crashes and restarts repeatedly
      ↓
      💀 Podman kills after restart limit → SIGKILL (exit 137)
```

### Why `depends_on: service_healthy` Doesn't Work

```yaml
# ❌ THIS IS IGNORED BY PODMAN-COMPOSE:
datastorage:
  depends_on:
    postgres:
      condition: service_healthy  # Podman-compose doesn't respect this
```

**Reason**: `podman-compose` has limited Docker Compose v3 compatibility and ignores health check conditions in `depends_on`.

### Investigation Timeline

| Date | Event | Finding |
|------|-------|---------|
| Dec 20, 2025 | RO team reports infrastructure failures | DataStorage cannot connect to PostgreSQL |
| Dec 20, 2025 | DS team debugs root cause | Identified `podman-compose` race condition |
| Dec 20, 2025 | DS team implements sequential startup | 100% test pass rate achieved (818 tests) |
| Dec 21, 2025 | NT team reports identical issues | Confirmed root cause affects multiple services |
| Dec 21, 2025 | DD-TEST-002 created | Authoritative guidance established |

---

## 🎯 Decision

**We will use DIFFERENT orchestration strategies based on service dependencies:**

### Decision Matrix

| Scenario | Use | Rationale |
|----------|-----|-----------|
| **Multi-service with startup dependencies** | ✅ **Sequential `podman run`** | Eliminates race conditions |
| **Single-service testing** | ✅ **`podman-compose`** | Simpler, no race condition risk |
| **Developer local testing** | ✅ **`podman-compose`** | Convenience, can restart manually |
| **CI/CD integration tests** | ✅ **Sequential `podman run`** | Deterministic, reliable |
| **E2E tests (Kind clusters)** | ✅ **Kubernetes orchestration** | Native K8s startup ordering |

### Sequential Startup Pattern (Recommended for Integration Tests)

Services with startup dependencies **MUST** use sequential `podman run` commands with explicit health checks:

```bash
#!/bin/bash
# test/integration/{service}/setup-infrastructure.sh

set -e

# 1. Stop any existing containers
podman stop {service}_postgres_1 {service}_redis_1 {service}_datastorage_1 2>/dev/null || true
podman rm {service}_postgres_1 {service}_redis_1 {service}_datastorage_1 2>/dev/null || true

# 2. Create network
podman network create {service}_test-network 2>/dev/null || true

# 3. Start PostgreSQL FIRST
echo "🔵 Starting PostgreSQL..."
podman run -d \
  --name {service}_postgres_1 \
  --network {service}_test-network \
  -p {POSTGRES_PORT}:5432 \
  -e POSTGRES_USER=slm_user \
  -e POSTGRES_PASSWORD=test_password \
  -e POSTGRES_DB={DB_NAME} \
  postgres:16-alpine

# 4. WAIT for PostgreSQL to be ready (critical!)
echo "⏳ Waiting for PostgreSQL..."
for i in {1..30}; do
  podman exec {service}_postgres_1 pg_isready -U slm_user && break
  sleep 1
done

# 5. Run migrations (if applicable)
echo "🔄 Running migrations..."
podman run --rm \
  --network {service}_test-network \
  -e DB_HOST={service}_postgres_1 \
  -e DB_PORT=5432 \
  {service}_migrations:latest

# 6. Start Redis SECOND
echo "🔵 Starting Redis..."
podman run -d \
  --name {service}_redis_1 \
  --network {service}_test-network \
  -p {REDIS_PORT}:6379 \
  redis:7-alpine

# 7. WAIT for Redis to be ready
echo "⏳ Waiting for Redis..."
for i in {1..10}; do
  podman exec {service}_redis_1 redis-cli ping | grep -q PONG && break
  sleep 1
done

# 8. Start DataStorage LAST
echo "🔵 Starting DataStorage..."
# ⚠️ DEPRECATED PATTERN - Use DD-INTEGRATION-001 v2.0 instead
# Composite image tag format: {service}-{uuid} to prevent collisions
DS_IMAGE_TAG="datastorage-$(uuidgen | tr '[:upper:]' '[:lower:]')"
podman run -d \
  --name {service}_datastorage_1 \
  --network {service}_test-network \
  -p {DS_HTTP_PORT}:8080 \
  -p {DS_METRICS_PORT}:9090 \
  -e DB_HOST={service}_postgres_1 \
  -e DB_PORT=5432 \
  -e REDIS_HOST={service}_redis_1 \
  -e REDIS_PORT=6379 \
  $DS_IMAGE_TAG

# 9. WAIT for DataStorage health check
echo "⏳ Waiting for DataStorage..."
for i in {1..30}; do
  curl -s http://127.0.0.1:{DS_HTTP_PORT}/health | grep -q "ok" && break
  sleep 1
done

echo "✅ Infrastructure ready!"
```

### Test Suite Integration

#### Option A: Go Services (BeforeSuite Pattern)

**BeforeSuite Pattern** (use `Eventually()` with 30s timeout):

```go
// test/integration/{service}/suite_test.go

var _ = BeforeSuite(func() {
    // Start infrastructure sequentially
    cmd := exec.Command("./setup-infrastructure.sh")
    cmd.Dir = "test/integration/{service}"
    output, err := cmd.CombinedOutput()
    if err != nil {
        Fail(fmt.Sprintf("Failed to start infrastructure: %s\n%s", err, output))
    }

    // Use Eventually() for health checks (30s timeout)
    Eventually(func() int {
        resp, err := http.Get(dataStorageURL + "/health")
        if err != nil {
            GinkgoWriter.Printf("  Health check failed: %v\n", err)
            return 0
        }
        defer resp.Body.Close()
        return resp.StatusCode
    }, "30s", "1s").Should(Equal(http.StatusOK),
        "DataStorage should be healthy within 30 seconds")
})
```

**Why 30 seconds?**: Cold start on macOS Podman can take **15-20 seconds**.

#### Option B: Python Services (pytest Fixtures Pattern)

**Python Pattern** (recommended for consistency):

```python
# tests/integration/conftest.py

def start_infrastructure() -> bool:
    """
    Start integration infrastructure using Python (no shell scripts).

    This provides:
    - Consistency with Go service patterns (framework manages infrastructure)
    - Better error handling (Python exceptions propagate to pytest)
    - Simpler maintenance (single source of truth)
    """
    script_dir = os.path.dirname(os.path.abspath(__file__))
    compose_file = os.path.join(script_dir, "docker-compose.workflow-catalog.yml")

    # Determine compose command
    compose_cmd = "podman-compose" if shutil.which("podman-compose") else "docker-compose"

    # Start services sequentially via compose
    result = subprocess.run(
        [compose_cmd, "-f", "docker-compose.yml", "-p", "project-name", "up", "-d"],
        cwd=script_dir,
        capture_output=True,
        timeout=180
    )

    if result.returncode != 0:
        return False

    # Wait for services to be healthy (60s timeout)
    return wait_for_infrastructure(timeout=60.0)

@pytest.fixture(scope="session")
def integration_infrastructure():
    """Session-scoped fixture for infrastructure management."""
    if not is_integration_infra_available():
        pytest.fail("REQUIRED: Infrastructure not running")

    yield

    # Automatic cleanup handled by pytest_sessionfinish hook
```

**Benefits of Python Fixtures**:
- ✅ No external shell scripts needed
- ✅ Python debugging works natively
- ✅ Errors propagate cleanly to pytest
- ✅ Consistent with Go service pattern (framework manages infrastructure)

---

## ✅ Consequences

### Positive Consequences

1. ✅ **Eliminates race conditions** - Services start in dependency order
2. ✅ **Deterministic startup** - Explicit wait logic, no guessing
3. ✅ **Clear failure messages** - Know exactly which service failed to start
4. ✅ **CI/CD reliability** - Consistent behavior across environments
5. ✅ **Proven success** - DataStorage achieved 100% test pass rate (818 tests)

### Negative Consequences

1. ⚠️ **More verbose** - Sequential startup script vs one-line `podman-compose up`
2. ⚠️ **Per-service scripts** - Each service needs its own setup script
3. ⚠️ **Maintenance overhead** - Scripts need updating when dependencies change

**Mitigation**: Trade-off is acceptable given reliability benefits. Script templates reduce duplication.

---

## 📊 When to Use Each Approach

### ✅ Use Sequential Startup (`podman run`) For:

- **Integration tests** with multi-service dependencies
- **CI/CD pipelines** requiring deterministic startup
- **Services that connect to databases** at initialization
- **Any service** experiencing "exit 137" or DNS failures

**Examples**:
- DataStorage integration tests (PostgreSQL + Redis + DataStorage) - Bash scripts
- HolmesGPT-API integration tests (PostgreSQL + Redis + DataStorage) - Python fixtures
- Notification integration tests (PostgreSQL + Redis + DataStorage) - Bash scripts
- RemediationOrchestrator integration tests (PostgreSQL + Redis + DataStorage) - Bash scripts

### ✅ Use `podman-compose` For:

- **Single-service testing** (no startup dependencies)
- **Developer local testing** (can restart manually if issues occur)
- **E2E tests using Kind clusters** (different orchestration mechanism)

**Examples**:
- Gateway integration tests (if no DataStorage dependency)
- Single-service unit tests (no infrastructure needed)
- Local development convenience (non-CI)

### ❌ Never Use `podman-compose` For:

- **Multi-service integration tests** with startup dependencies
- **CI/CD pipelines** where reliability is critical
- **Automated test suites** where failures block deployments

---

## 🔧 Implementation Guide

**See**: `docs/development/testing/INTEGRATION_TEST_INFRASTRUCTURE_SETUP.md`

Provides:
- Step-by-step setup instructions
- Service-specific script templates
- Port allocation strategy
- Troubleshooting guide

---

## 📚 References

### Internal Documents
- **DD-TEST-001**: Integration Test Port Allocation Pattern
- **DD-AUDIT-003**: Audit Infrastructure Requirements
- **TESTING_GUIDELINES.md**: Testing standards and patterns
- **Implementation Guide**: `docs/development/testing/INTEGRATION_TEST_INFRASTRUCTURE_SETUP.md`

### Debugging Session Records (Historical)
- `docs/handoff/SHARED_RO_DS_INTEGRATION_DEBUG_DEC_20_2025.md` (RO team debugging)
- `docs/handoff/NT_INTEGRATION_TEST_INFRASTRUCTURE_ISSUES_DEC_21_2025.md` (NT team debugging)

**Note**: Handoff documents are historical debugging records. **This document (DD-TEST-002) is the authoritative reference.**

### Working Implementations
- **DataStorage (Go)**: `test/integration/datastorage/suite_test.go` (bash script sequential startup)
  - 100% test pass rate (818 tests) achieved Dec 20, 2025
- **HolmesGPT-API (Python)**: `holmesgpt-api/tests/integration/conftest.py` (Python pytest fixtures)
  - Pure Python infrastructure management (no shell scripts)
  - Achieved consistency with Go service patterns Dec 27, 2025

---

## 🎯 Success Metrics

### Acceptance Criteria

- ✅ Integration tests start infrastructure reliably (>99% success rate)
- ✅ No "exit 137" container failures in CI/CD
- ✅ BeforeSuite health checks pass within 30 seconds
- ✅ All affected services adopt sequential startup pattern

### Service Migration Status

| Service | Status | Date | Notes |
|---------|--------|------|-------|
| **DataStorage** | ✅ Migrated | 2025-12-20 | 100% tests passing, bash scripts, reference implementation |
| **HolmesGPT-API** | ✅ Migrated | 2025-12-27 | Python pytest fixtures, no shell scripts |
| **RemediationOrchestrator** | ⏳ Pending | - | Known issue documented |
| **Notification** | ⏳ Pending | - | Known issue documented |
| **Gateway** | N/A | - | Single-service, no issue |
| **SignalProcessing** | N/A | - | No DataStorage dependency in integration tests |
| **AIAnalysis** | N/A | - | Uses mocked dependencies |

---

## 🔄 Review and Updates

**Review Frequency**: Quarterly or when new integration test patterns emerge

**Last Reviewed**: 2025-12-27 (Added Python pytest fixtures pattern for HAPI)
**Next Review**: 2026-03-27

**Recent Updates**:
- 2025-12-27: Added Python pytest fixtures pattern (HolmesGPT-API)
- 2025-12-21: Initial document creation (bash script pattern)

---

## ✍️ Decision Rationale Summary

**Problem**: `podman-compose` race condition causes integration test failures
**Decision**: Use sequential `podman run` for multi-service dependencies
**Rationale**: Eliminates race conditions, proven reliable by DataStorage team
**Trade-off**: More verbose scripts, but significantly higher reliability
**Status**: Accepted and implemented by DataStorage, pending for RO and NT

---

**Document Status**: ✅ Authoritative
**Approved By**: DataStorage Team (validated), Infrastructure Team (approved)
**Implementation Required**: Yes (RO, NT services pending migration)

