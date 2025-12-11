# NOTICE: Integration Test Infrastructure Ownership Clarification

**Date**: 2025-12-11
**Version**: 1.0
**From**: AIAnalysis Team (Triage)
**To**: All Service Teams
**Status**: 🟢 **CLARIFIED - EACH SERVICE OWNS INFRASTRUCTURE**
**Priority**: HIGH

---

## 📋 Summary

**Issue**: The shared `podman-compose.test.yml` is causing port collisions when multiple services run integration tests in parallel.

**Root Cause**: Multiple services are using the **same** `podman-compose.test.yml` with **fixed ports**, causing collisions when tests run in parallel.

**Clarification**: There is **NO shared DataStorage service** for integration or E2E tests. Each service must start its own complete infrastructure stack.

---

## 🎯 Architectural Clarification

### Service Infrastructure Requirements

| Service | Must Start | Port Allocation (DD-TEST-001) |
|---------|-----------|-------------------------------|
| **DataStorage** | PostgreSQL, Redis, DS API | PostgreSQL: 15433, Redis: 16379, API: 18090 |
| **AIAnalysis** | PostgreSQL, Redis, DS API, HAPI | PostgreSQL: 15434, Redis: 16380, DS: 18091, HAPI: 18120 |
| **Gateway** | PostgreSQL, Redis, DS API | PostgreSQL: 15435, Redis: 16381, DS: 18092 |
| **Notification** | PostgreSQL, Redis, DS API | PostgreSQL: 15436, Redis: 16382, DS: 18093 |
| **RO** | PostgreSQL, Redis, DS API | PostgreSQL: 15437, Redis: 16383, DS: 18094 |
| **WE** | PostgreSQL, Redis, DS API | PostgreSQL: 15438, Redis: 16384, DS: 18095 |
| **SP** | PostgreSQL, Redis, DS API | PostgreSQL: 15439, Redis: 16385, DS: 18096 |

### Key Insight

**Each service starts its own infrastructure stack:**
- PostgreSQL + Redis + DataStorage API + service-specific dependencies
- Uses **unique ports** per DD-TEST-001 to prevent collisions
- **No shared infrastructure** - each service is independent
- Enables **parallel test execution** without port conflicts

---

## 🏗️ Correct Architecture: Each Service Owns Its Infrastructure

```
┌─────────────────────────────────────────────────────────────┐
│ DataStorage Integration Tests                              │
│                                                             │
│   test/integration/datastorage/podman-compose.yml:          │
│     - PostgreSQL (:15433)                                   │
│     - Redis (:16379)                                        │
│     - DataStorage API (:18090)                              │
│     - Goose migrations                                      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ AIAnalysis Integration Tests                               │
│                                                             │
│   test/integration/aianalysis/podman-compose.yml:           │
│     - PostgreSQL (:15434)  ← AIAnalysis ports               │
│     - Redis (:16380)                                        │
│     - DataStorage API (:18091)                              │
│     - HolmesGPT API (:18120)                                │
│     - Goose migrations                                      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ Gateway Integration Tests                                   │
│                                                             │
│   test/integration/gateway/podman-compose.yml:              │
│     - PostgreSQL (:15435)  ← Gateway ports                  │
│     - Redis (:16381)                                        │
│     - DataStorage API (:18092)                              │
│     - Goose migrations                                      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ Notification/RO/WE/SP Integration Tests                     │
│                                                             │
│   Each service has its own podman-compose.yml with          │
│   unique ports per DD-TEST-001                              │
└─────────────────────────────────────────────────────────────┘

✅ NO PORT COLLISIONS - Each service uses unique ports
✅ PARALLEL EXECUTION - All services can test simultaneously
✅ ISOLATED INFRASTRUCTURE - No shared dependencies
```

---

## 📝 Required Changes: Each Service Creates Its Own Compose File

### 1. Create Per-Service Compose Files with Unique Ports

| Service | Compose File | Ports (from DD-TEST-001) |
|---------|--------------|--------------------------|
| **DataStorage** | `test/integration/datastorage/podman-compose.yml` | PostgreSQL: 15433, Redis: 16379, DS: 18090 |
| **AIAnalysis** | `test/integration/aianalysis/podman-compose.yml` | PostgreSQL: 15434, Redis: 16380, DS: 18091, HAPI: 18120 |
| **Gateway** | `test/integration/gateway/podman-compose.yml` | PostgreSQL: 15435, Redis: 16381, DS: 18092 |
| **Notification** | `test/integration/notification/podman-compose.yml` | PostgreSQL: 15436, Redis: 16382, DS: 18093 |
| **RO** | `test/integration/remediationorchestrator/podman-compose.yml` | PostgreSQL: 15437, Redis: 16383, DS: 18094 |
| **WE** | `test/integration/workflowexecution/podman-compose.yml` | PostgreSQL: 15438, Redis: 16384, DS: 18095 |
| **SP** | `test/integration/signalprocessing/podman-compose.yml` | PostgreSQL: 15439, Redis: 16385, DS: 18096 |

### 2. Update Each Service's suite_test.go

Add infrastructure startup/teardown in `BeforeSuite`/`AfterSuite`:

```go
var _ = BeforeSuite(func() {
    // Start service-specific podman-compose stack
    err := infrastructure.StartServiceInfrastructure(GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
    // Stop service-specific podman-compose stack
    infrastructure.StopServiceInfrastructure(GinkgoWriter)
})
```

### 3. Remove Shared `podman-compose.test.yml` (Root Level)

| File | Action |
|------|--------|
| `podman-compose.test.yml` (root) | ❌ **DELETE** - No longer shared |

### 4. Update Documentation

| File | Change |
|------|--------|
| `TESTING_GUIDELINES.md` | Document per-service infrastructure ownership |
| `DD-TEST-001` | Already defines unique ports per service |
| Service READMEs | Add "Integration Test Setup" instructions |

---

## 🔧 Port Allocation Per Service (DD-TEST-001 Compliant)

### All Services Start Their Own Infrastructure

| Service | PostgreSQL | Redis | DataStorage API | Additional |
|---------|-----------|-------|----------------|------------|
| **DataStorage** | 15433 | 16379 | 18090 | — |
| **AIAnalysis** | 15434 | 16380 | 18091 | HAPI: 18120 |
| **Gateway** | 15435 | 16381 | 18092 | — |
| **Notification** | 15436 | 16382 | 18093 | — |
| **RO** | 15437 | 16383 | 18094 | — |
| **WE** | 15438 | 16384 | 18095 | — |
| **SP** | 15439 | 16385 | 18096 | — |

### Parallel Execution Enabled

With unique ports per service, all integration tests can run simultaneously:

```bash
# Run all integration tests in parallel - NO COLLISIONS!
make test-integration-datastorage &
make test-integration-aianalysis &
make test-integration-gateway &
make test-integration-notification &
make test-integration-ro &
make test-integration-we &
make test-integration-sp &
wait
```

---

## ✅ Benefits

1. **No port collisions** - Each service uses unique ports from DD-TEST-001
2. **Parallel execution** - All services can test simultaneously in CI/CD
3. **Isolation** - One service's test failures don't affect others
4. **Clear ownership** - Each service team owns their compose file
5. **Developer flexibility** - Developers can test any service without coordination

---

## 🗳️ Action Required Per Service

Each service team must create their own `podman-compose.yml`:

| Team | Status | Action Required |
|------|--------|----------------|
| **DataStorage** | ⏳ **TODO** | Move `podman-compose.test.yml` to `test/integration/datastorage/` |
| **AIAnalysis** | ✅ **IN PROGRESS** | Create `test/integration/aianalysis/podman-compose.yml` (ports: 15434, 16380, 18091, 18120) |
| **Gateway** | ⚠️  **REVIEW** | Uses dynamic ports - may need DD-TEST-001 compliance review |
| **Notification** | ⏳ **TODO** | Create `test/integration/notification/podman-compose.yml` (ports: 15436, 16382, 18093) |
| **RO** | ⏳ **TODO** | Create `test/integration/remediationorchestrator/podman-compose.yml` (ports: 15437, 16383, 18094) |
| **WE** | ⏳ **TODO** | Create `test/integration/workflowexecution/podman-compose.yml` (ports: 15438, 16384, 18095) |
| **SP** | ⏳ **TODO** | Create `test/integration/signalprocessing/podman-compose.yml` (ports: 15439, 16385, 18096) |

---

## 📚 References

- `docs/development/business-requirements/TESTING_GUIDELINES.md` - Authoritative testing policy
- `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` - Port allocation
- `podman-compose.test.yml` - Current shared file (to be moved)

---

**Next Steps**:
1. **Each service team**: Create `test/integration/[service]/podman-compose.yml` with allocated ports
2. **Each service team**: Add infrastructure start/stop in `suite_test.go` 
3. **DataStorage team**: Move root `podman-compose.test.yml` to `test/integration/datastorage/`
4. **All teams**: Test parallel execution to verify no port collisions
5. **Cleanup**: Delete root-level `podman-compose.test.yml` after all services migrated

---

## 📝 Team Responses

### Notification Team Response

**Date**: 2025-12-11
**Status**: ⚠️  **NEEDS UPDATE** (was approved based on incorrect premise)
**Responded By**: Notification Team

#### Current State Analysis

Notification integration tests connect to DataStorage HTTP API:

```go
// From test/integration/notification/audit_integration_test.go:71-73
dataStorageURL = os.Getenv("DATA_STORAGE_URL")
if dataStorageURL == "" {
    dataStorageURL = "http://localhost:18090" // ⚠️  WRONG PORT - This is DataStorage's port!
}
```

#### ⚠️  Correction Needed

| Issue | Current | Required |
|-------|---------|----------|
| **Port collision** | Uses DataStorage's port (18090) | Must use Notification's port (18093) |
| **Missing infrastructure** | Expects shared DS | Must start own PostgreSQL, Redis, DS |
| **Compose file** | None | Create `test/integration/notification/podman-compose.yml` |

#### Required Changes

1. **Create** `test/integration/notification/podman-compose.yml` with ports:
   - PostgreSQL: 15436
   - Redis: 16382
   - DataStorage: **18093** (not 18090!)
   - Goose migrations

2. **Update** `suite_test.go` to start/stop infrastructure

3. **Update** tests to use `http://localhost:18093`

---

### WorkflowExecution (WE) Team Response

**Date**: 2025-12-11
**Status**: ⚠️  **NEEDS UPDATE** (was approved based on incorrect premise)
**Responded By**: WorkflowExecution Team

#### Current State Analysis

WE integration tests connect to DataStorage HTTP API:

```go
// From test/integration/workflowexecution/audit_datastorage_test.go:51-52
const dataStorageURL = "http://localhost:18090"  // ⚠️  WRONG PORT - This is DataStorage's port!
```

#### ⚠️  Correction Needed

| Issue | Current | Required |
|-------|---------|----------|
| **Port collision** | Uses DataStorage's port (18090) | Must use WE's port (18095) |
| **Missing infrastructure** | Expects shared DS | Must start own PostgreSQL, Redis, DS |
| **Compose file** | Uses root compose | Create `test/integration/workflowexecution/podman-compose.yml` |

#### Required Changes

1. **Create** `test/integration/workflowexecution/podman-compose.yml` with ports:
   - PostgreSQL: 15438
   - Redis: 16384
   - DataStorage: **18095** (not 18090!)
   - Goose migrations

2. **Update** `suite_test.go` to start/stop infrastructure

3. **Update** tests to use `http://localhost:18095`

---

### Gateway Team Response

**Date**: 2025-12-11
**Status**: ⚠️  **NEEDS REVIEW** (uses dynamic ports instead of DD-TEST-001 allocation)
**Responded By**: Gateway Team

#### Current State Analysis

Gateway integration tests use **dynamic port allocation**:

```go
// From test/integration/gateway/helpers_postgres.go
port := findAvailablePort(50001, 60000)  // Random ports, not DD-TEST-001
dataStorageURL := fmt.Sprintf("http://localhost:%d", dsPort)
```

#### ⚠️  DD-TEST-001 Compliance Review

| Aspect | Current | DD-TEST-001 Requirement |
|--------|---------|-------------------------|
| **Port strategy** | Random (50001-60000) | Fixed (15435, 16381, 18092) |
| **Infrastructure** | Starts own DS | ✅ Correct |
| **Parallel safety** | ✅ Random ports work | ✅ But not documented |

#### Decision Required

**Option A: Keep Random Ports (Current)**
- ✅ Proven to work
- ✅ Maximum flexibility
- ❌ Not DD-TEST-001 compliant
- ❌ Harder to debug (ports change each run)

**Option B: Switch to DD-TEST-001 Ports**
- ✅ Consistent with other services
- ✅ Easier to debug
- ✅ DD-TEST-001 compliant
- ❌ Requires code changes

**Recommendation**: Stay with random ports but document in DD-TEST-001 as "dynamic allocation"

---

