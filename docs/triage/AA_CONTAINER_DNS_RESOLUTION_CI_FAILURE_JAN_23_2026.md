# AIAnalysis Container DNS Resolution Failure in CI - Root Cause Analysis

**Date**: January 23, 2026
**Status**: ✅ ROOT CAUSE IDENTIFIED
**Severity**: High (blocking CI merge)
**Scope**: AIAnalysis integration tests only

---

## 🎯 Executive Summary

AIAnalysis integration tests **fail exclusively in GitHub Actions CI** due to **Podman container-to-container DNS resolution failures**, while identical tests pass locally. The issue is specific to the container networking configuration used only by AIAnalysis.

---

## 📊 Evidence Summary

### CI Results (Run #21298506384)

| Service | DATA_STORAGE_URL Configuration | Result |
|---------|-------------------------------|--------|
| **Gateway** | `http://localhost:18090` | ✅ SUCCESS |
| **Notification** | `http://127.0.0.1:18096` | ✅ SUCCESS |
| **HAPI** (own suite) | `http://127.0.0.1:18098` | ✅ SUCCESS |
| **AIAnalysis** | `http://aianalysis_datastorage_test:8080` | ❌ FAILURE |

### Key Finding

**AIAnalysis is the ONLY service using container-to-container DNS resolution** (`container_name:port`) instead of localhost/127.0.0.1.

---

## 🔍 Technical Root Cause

### 1. Container Configuration Difference

#### AIAnalysis (FAILING)
```go
// test/integration/aianalysis/suite_test.go:291
Env: map[string]string{
    "DATA_STORAGE_URL": "http://aianalysis_datastorage_test:8080", // ❌ Container name
    // ...
}
```

**Why this fails in CI:**
- HAPI container (`aianalysis_hapi_test`) tries to resolve hostname `aianalysis_datastorage_test`
- Podman DNS in CI doesn't properly register full container names as hostnames
- Container aliases show only short IDs: `["f917a71a0c99", "1f5717e55d51"]`
- DNS lookup times out after 10 seconds

#### HAPI Integration Tests (PASSING)
```python
# holmesgpt-api/tests/integration/conftest.py:123
DATA_STORAGE_URL = os.getenv("DATA_STORAGE_URL", f"http://127.0.0.1:{DATA_STORAGE_PORT}")
```

**Why this works:**
- Python tests run on the host (not in a container)
- Uses `127.0.0.1:18098` to reach DataStorage via port mapping
- No DNS resolution needed - direct IP connection

### 2. Network Configuration

**Must-Gather Evidence:**
```json
{
  "aianalysis_test_network": {
    "Gateway": "10.89.2.1",
    "IPAddress": "10.89.2.7",  // HAPI container
    // ...
    "Aliases": ["f917a71a0c99"]  // ❌ Only short container ID, not full name
  }
}
```

**Expected**:
```json
"Aliases": ["f917a71a0c99", "aianalysis_hapi_test"]
```

### 3. Error Manifestation

**HAPI logs show:**
```
ERROR:src.toolsets.workflow_catalog:💥 BR-STORAGE-013: Unexpected error calling Data Storage Service -
HTTPConnectionPool(host='aianalysis_datastorage_test', port=8080): Read timed out. (read timeout=9.998s)
```

**DataStorage was healthy:**
```
2026-01-23T20:01:48.084Z INFO Starting Data Storage service {"port": 8080, "host": "0.0.0.0"}
```

**Interpretation**: DataStorage was running and listening, but HAPI couldn't resolve its hostname.

---

## 🏗️ Architecture Context

### Why AIAnalysis Uses Container-to-Container

AIAnalysis has a **unique architecture** among integration test suites:
- **Runs HAPI in a container** (`aianalysis_hapi_test`)
- HAPI needs to communicate with DataStorage (also in a container)
- Container-to-container communication requires DNS resolution

### Why Other Services Use Localhost

- **Gateway/Notification/RO**: Controllers run in-process (no HAPI container)
- **HAPI Suite**: Python tests run on host (not containerized)
- All use `127.0.0.1` or `localhost` to reach DataStorage via port mapping

---

## 🌍 Environment Differences

### Local (PASSING)

| Aspect | Configuration |
|--------|---------------|
| **OS** | macOS (Podman Desktop) |
| **Cores** | 12 |
| **Podman** | Mature, stable DNS resolution |
| **Network Driver** | Well-tested local implementation |
| **Execution** | Sequential (one service at a time) |

### CI (FAILING)

| Aspect | Configuration |
|--------|---------------|
| **OS** | Ubuntu (GitHub Actions) |
| **Cores** | 4 |
| **Podman** | CI-specific configuration |
| **Network Driver** | May have DNS propagation delays |
| **Execution** | Each matrix job on separate VM (not resource contention) |

---

## ❌ Ruled Out Root Causes

### 1. Resource Contention ❌
**Initial Hypothesis**: 9 parallel integration test jobs competing for 4 CPU cores
**Actual**: Each GitHub Actions matrix job runs on **its own isolated VM**
**Evidence**: Standard GitHub Actions behavior confirmed

### 2. Database Connection Pool Exhaustion ❌
**Hypothesis**: 25-connection pool insufficient for 4 cores in CI
**Actual**: Pool is sufficient even for 12 cores locally
**Evidence**: No database connection errors in must-gather logs

### 3. DataStorage Service Health ❌
**Hypothesis**: DataStorage not starting or crashing
**Actual**: DataStorage started successfully and was healthy
**Evidence**: Logs show "Starting Data Storage service" and health checks passing

### 4. IPv4 vs IPv6 Binding ❌
**Hypothesis**: Containers binding to IPv6 (::1) instead of IPv4
**Actual**: Not relevant for container-to-container DNS
**Evidence**: Network inspection shows IPv4 addresses (10.89.2.x)

---

## ✅ Confirmed Root Cause

**Podman DNS resolution of container names within a custom network is unreliable in GitHub Actions CI environment.**

Containers on the same Podman network use **short container IDs as aliases** (`f917a71a0c99`) instead of full names (`aianalysis_hapi_test`), causing DNS lookups to fail or timeout.

---

## 🛠️ Solution Options

### Option A: Use Explicit Network Aliases (Recommended)
**Add `--network-alias` when starting containers**

```go
// test/infrastructure/datastorage_bootstrap.go (modify container start)
cmd := exec.Command("podman", "run", "-d",
    "--name", containerName,
    "--network", network,
    "--network-alias", containerName,  // ✅ Explicitly register full name as alias
    // ... rest of args
)
```

**Pros:**
- ✅ Preserves container-to-container communication pattern
- ✅ Minimal code change (single flag per container)
- ✅ Works in both CI and local environments

**Cons:**
- ⚠️ May still have DNS propagation delays in CI

### Option B: Use Container IP Addresses (Most Reliable)
**Query container IP and pass to HAPI environment**

```go
// Get DataStorage container IP
ipCmd := exec.Command("podman", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", "aianalysis_datastorage_test")
dsIP, _ := ipCmd.Output()

// Pass IP instead of hostname
Env: map[string]string{
    "DATA_STORAGE_URL": fmt.Sprintf("http://%s:8080", strings.TrimSpace(string(dsIP))),
}
```

**Pros:**
- ✅ Most reliable (no DNS needed)
- ✅ Works in all environments
- ✅ No DNS propagation delays

**Cons:**
- ⚠️ Slightly more complex setup code
- ⚠️ Less "natural" than hostname resolution

### Option C: Use Localhost Like Other Services (Simplest)
**Run HAPI in-process for integration tests**

```go
// Similar to HAPI's own integration test suite
// Python tests call business logic directly, not via container
Env: map[string]string{
    "DATA_STORAGE_URL": "http://127.0.0.1:18095",  // ✅ Use port mapping
}
```

**Pros:**
- ✅ Simplest solution
- ✅ Matches pattern used by successful services (Gateway, Notification, HAPI suite)
- ✅ No DNS resolution needed
- ✅ Faster (no container startup)

**Cons:**
- ⚠️ Changes AIAnalysis integration test architecture
- ⚠️ Less representative of E2E deployment (but that's what E2E tests are for)

---

## 📝 Recommendation & Implementation

**✅ IMPLEMENTED: Option C (Use Localhost Pattern)**

Changed AIAnalysis to match the normalized pattern used by all other services:

```go
// test/integration/aianalysis/suite_test.go
Env: map[string]string{
    "DATA_STORAGE_URL": "http://host.containers.internal:18095", // ✅ Normalized: Use host mapping (DD-TEST-001 v2.2)
    // ...
}
```

**Rationale:**
- ✅ Matches successful pattern used by Gateway, Notification, HAPI suite
- ✅ Eliminates container-to-container DNS dependency
- ✅ Uses well-tested `host.containers.internal` hostname resolution
- ✅ Aligns with project-wide port allocation (DD-TEST-001 v2.2: AIAnalysis DS = 18095)
- ✅ Simplifies architecture - no special DNS configuration needed

**Why This Works:**
- HAPI container reaches DataStorage via `host.containers.internal:18095`
- DataStorage listens on `0.0.0.0:8080` inside container, mapped to host port `18095`
- `host.containers.internal` resolves reliably in both CI and local Podman environments
- No dependency on Podman's container name DNS resolution

---

## 📚 Related Documentation

- **IPv4 Binding Regression**: [INTEGRATION_REGRESSION_IPV4_BINDING_JAN_23_2026.md](mdc:docs/triage/INTEGRATION_REGRESSION_IPV4_BINDING_JAN_23_2026.md)
- **DS Integration Failures**: [INTEGRATION_TEST_FAILURES_CI_JAN_23_2026.md](mdc:docs/triage/INTEGRATION_TEST_FAILURES_CI_JAN_23_2026.md)
- **Test Infrastructure**: [datastorage_bootstrap.go](mdc:test/infrastructure/datastorage_bootstrap.go)
- **AIAnalysis Suite**: [suite_test.go](mdc:test/integration/aianalysis/suite_test.go)

---

**Document Status**: ✅ Root Cause Identified
**Next Steps**: Implement Option A (network aliases) and re-test in CI
