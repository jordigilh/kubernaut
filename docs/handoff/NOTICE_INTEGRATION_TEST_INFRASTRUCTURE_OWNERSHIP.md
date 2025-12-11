# NOTICE: Integration Test Infrastructure - Actual Architecture

**Date**: 2025-12-11
**Version**: 1.2 (Corrected - Actual Architecture)
**From**: RO Team (Triage)
**To**: All Service Teams
**Status**: 🟢 **CLARIFIED** - Each service manages own infrastructure
**Priority**: HIGH

---

## 📋 Summary

**Issue**: Confusion about integration test infrastructure ownership and who starts what.

**Root Cause**: Assumption that there's a "shared Data Storage service" for integration tests.

**Clarification**: **Each service starts its own infrastructure in `BeforeSuite`** or requires manual setup. There is NO shared automated infrastructure.

---

## 🎯 Actual Architecture (From Code Analysis)

### Service-by-Service Infrastructure Startup

| Service | Infrastructure Started in BeforeSuite | Manual Setup Required | E2E Infrastructure |
|---------|--------------------------------------|----------------------|-------------------|
| **DataStorage** | ✅ PostgreSQL + Redis + DS | ❌ None | Port 15433, 16379, 18090 |
| **Gateway** | ✅ PostgreSQL + Redis + DS | ❌ None | Dynamic (50001-60000) |
| **RO** | ✅ envtest only | ✅ Manual `podman-compose` for audit tests | E2E: DS in Kind |
| **WE** | ✅ envtest only | ✅ Manual `podman-compose` for audit tests | E2E: DS in Kind |
| **Notification** | ✅ envtest only | ✅ Manual `podman-compose` for audit tests | E2E: DS in Kind |
| **SP** | ✅ envtest only | ❌ None (ADR-038: Audit Non-Blocking) | ✅ **E2E: DS in Kind** (BR-SP-090) |

### Key Insights

1. **DataStorage** starts PostgreSQL, Redis, and DS in `SynchronizedBeforeSuite` (lines 335-427 of `suite_test.go`)
2. **Gateway** starts PostgreSQL, Redis, and DS in `SynchronizedBeforeSuite` (lines 56-178 of `suite_test.go`)
3. **RO/WE/Notification** use envtest only, but **REQUIRE manual `podman-compose up`** for audit tests to PASS
4. **SignalProcessing** uses envtest for integration tests (ADR-038: Audit Non-Blocking), deploys DS in Kind for E2E tests (BR-SP-090)
5. **Root `podman-compose.test.yml`** is a **manual developer convenience**, NOT automated infrastructure

---

## 🔍 Evidence from Codebase

### DataStorage Integration Tests

```go:366:400:test/integration/datastorage/suite_test.go
// 2. Start PostgreSQL with pgvector
GinkgoWriter.Println("📦 Starting PostgreSQL container...")
startPostgreSQL()

// 3. Start Redis for DLQ
GinkgoWriter.Println("📦 Starting Redis container...")
startRedis()

// 6. Setup Data Storage Service
GinkgoWriter.Println("🚀 Starting Data Storage Service container...")
startDataStorageService()
```

**Verdict**: DataStorage starts its own infrastructure automatically in BeforeSuite.

### Gateway Integration Tests

```go:140:160:test/integration/gateway/suite_test.go
// 2. Start Redis container
suiteLogger.Info("📦 Starting Redis container...")
redisPort, err := infrastructure.StartRedisContainer("gateway-redis-integration", 16380, GinkgoWriter)

// 3. Start PostgreSQL container
suiteLogger.Info("📦 Starting PostgreSQL container...")
suitePgClient = SetupPostgresTestClient(ctx)

// 4. Start Data Storage service
suiteLogger.Info("📦 Starting Data Storage service...")
suiteDataStorage = SetupDataStorageTestServer(ctx, suitePgClient)
```

**Verdict**: Gateway starts its own infrastructure automatically in SynchronizedBeforeSuite.

### RO Integration Tests

```go:87:202:test/integration/remediationorchestrator/suite_test.go
var _ = BeforeSuite(func() {
    // ... register CRD schemes ...

    By("Bootstrapping test environment with ALL CRDs")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    }

    // NO PostgreSQL, Redis, or DS started here
})
```

**Verdict**: RO starts envtest only (NO database containers).

### SignalProcessing Integration Tests

```go:87:190:test/integration/signalprocessing/suite_test.go
var _ = BeforeSuite(func() {
    // ... register CRD schemes ...

    By("Bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    }

    // NO PostgreSQL, Redis, or DS started here
    // Per ADR-038: Audit is non-blocking, processing continues without DS
})
```

**Verdict**: SP integration tests use envtest only (NO audit infrastructure per ADR-038).

### SignalProcessing E2E Tests

```go:106:111:test/e2e/signalprocessing/suite_test.go
// BR-SP-090: Deploy DataStorage infrastructure for audit testing
// This must be done BEFORE deploying the controller
By("Deploying DataStorage for BR-SP-090 audit testing")
err = infrastructure.DeployDataStorageForSignalProcessing(ctx, kubeconfigPath, GinkgoWriter)
Expect(err).ToNot(HaveOccurred())
```

**Verdict**: SP E2E tests deploy PostgreSQL + Redis + DS in Kind cluster (BR-SP-090: Audit Trail Compliance).

### RO Audit Integration Test (FIXED)

```go:50:73:test/integration/remediationorchestrator/audit_integration_test.go
BeforeEach(func() {
    // REQUIRED: Data Storage must be running for audit integration tests
    // Per TESTING_GUIDELINES.md: Skip() is ABSOLUTELY FORBIDDEN - tests must FAIL
    dsURL := "http://localhost:18090"
    resp, err := client.Get(dsURL + "/health")
    if err != nil || resp.StatusCode != http.StatusOK {
        Fail(fmt.Sprintf(
            "❌ REQUIRED: Data Storage not available at %s\n"+
            "  Per DD-AUDIT-003: RemediationOrchestrator MUST have audit capability\n"+
            "  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n\n"+
            "  Start infrastructure first:\n"+
            "    podman-compose -f podman-compose.test.yml up -d",
            dsURL))
    }
})
```

**Verdict**: RO audit tests **FAIL** if Data Storage is not running at `:18090`. This requires MANUAL `podman-compose up`.

---

## 🏗️ Correct Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│ DataStorage Integration Tests                              │
│                                                             │
│   BeforeSuite automatically starts:                         │
│     - PostgreSQL (:15433)                                   │
│     - Redis (:16379)                                        │
│     - DataStorage (:18090)                                  │
│                                                             │
│   Tests run against THIS infrastructure                     │
│   AfterSuite tears down infrastructure                      │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ Gateway Integration Tests                                   │
│                                                             │
│   SynchronizedBeforeSuite automatically starts:             │
│     - PostgreSQL (dynamic: 50001-60000)                     │
│     - Redis (16380)                                         │
│     - DataStorage (dynamic: 50001-60000)                    │
│                                                             │
│   Tests run against THIS infrastructure                     │
│   SynchronizedAfterSuite tears down infrastructure          │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ RO/WE/Notification/SP Integration Tests                    │
│                                                             │
│   BeforeSuite starts: envtest ONLY (no containers)          │
│                                                             │
│   Audit tests REQUIRE manual setup:                         │
│     podman-compose -f podman-compose.test.yml up -d         │
│                                                             │
│   If DataStorage not at :18090 → audit tests FAIL           │
│   (Per TESTING_GUIDELINES.md: Skip() is FORBIDDEN)          │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ podman-compose.test.yml (Root Level)                       │
│                                                             │
│   MANUAL developer convenience only:                        │
│     - PostgreSQL (:15433)                                   │
│     - Redis (:16379)                                        │
│     - DataStorage (:18090)                                  │
│                                                             │
│   Used by: RO/WE/Notification audit tests (manual)         │
│   NOT used by: DataStorage or Gateway (start own)          │
│   NOT automated: Developer must run "podman-compose up"     │
└─────────────────────────────────────────────────────────────┘
```

---

## 📝 Testing Workflows

### DataStorage Integration Tests
```bash
# Infrastructure is AUTOMATED (BeforeSuite)
make test-integration-datastorage

# BeforeSuite starts PostgreSQL, Redis, DS automatically
# Tests run
# AfterSuite tears down infrastructure automatically
```

### Gateway Integration Tests
```bash
# Infrastructure is AUTOMATED (SynchronizedBeforeSuite)
make test-integration-gateway

# SynchronizedBeforeSuite starts PostgreSQL, Redis, DS with dynamic ports
# Tests run in parallel
# SynchronizedAfterSuite tears down infrastructure
```

### RO Integration Tests (Non-Audit)
```bash
# Infrastructure is AUTOMATED (envtest only)
make test-integration-remediationorchestrator

# BeforeSuite starts envtest (no containers)
# Blocking, phase, creator tests run (no audit needed)
# AfterSuite stops envtest
```

### SignalProcessing Integration Tests
```bash
# Infrastructure is AUTOMATED (envtest only)
make test-integration-signalprocessing

# BeforeSuite starts envtest (no containers)
# Per ADR-038: Audit is non-blocking, tests pass without DS
# AfterSuite stops envtest
```

### SignalProcessing E2E Tests
```bash
# Infrastructure is AUTOMATED (deploys DS in Kind)
make test-e2e-signalprocessing

# SynchronizedBeforeSuite:
#   - Creates Kind cluster
#   - Deploys PostgreSQL + Redis + DS (BR-SP-090)
#   - Deploys SP controller
# Tests validate audit trail compliance
# SynchronizedAfterSuite tears down Kind cluster
```

### RO Integration Tests (With Audit)
```bash
# Infrastructure REQUIRES MANUAL SETUP
podman-compose -f podman-compose.test.yml up -d  # MANUAL STEP

# Then run tests
make test-integration-remediationorchestrator

# BeforeSuite starts envtest
# Audit tests connect to manually-started DS at :18090
# If DS not running → audit tests FAIL (not skip)
# AfterSuite stops envtest (DS stays running for next test run)

# Manual teardown
podman-compose -f podman-compose.test.yml down
```

---

## 🚫 What Changed: Skip() Violation Fixed

### Before (WRONG - Violates TESTING_GUIDELINES.md)

```go
// ❌ FORBIDDEN per TESTING_GUIDELINES.md lines 420-536
if err != nil {
    Skip("Data Storage not available - run: podman-compose up")
}
```

### After (CORRECT - Per TESTING_GUIDELINES.md)

```go
// ✅ REQUIRED: Fail with clear error message
if err != nil || resp.StatusCode != http.StatusOK {
    Fail(fmt.Sprintf(
        "❌ REQUIRED: Data Storage not available at %s\n"+
        "  Per DD-AUDIT-003: RemediationOrchestrator MUST have audit capability\n"+
        "  Per TESTING_GUIDELINES.md: Skip() is ABSOLUTELY FORBIDDEN\n\n"+
        "  Start with: podman-compose -f podman-compose.test.yml up -d",
        dsURL))
}
```

**Rationale** (from TESTING_GUIDELINES.md lines 424-436):
- **False confidence**: Skipped tests show "green" but don't validate anything
- **Hidden dependencies**: Missing infrastructure goes undetected in CI
- **Compliance gaps**: Audit tests skipped = audit not validated (DD-AUDIT-003 violation)
- **Architectural enforcement**: If RO can run without DS, audit is effectively optional

---

## 🔧 Port Allocation Summary

| Service | PostgreSQL | Redis | DataStorage | Startup |
|---------|-----------|-------|-------------|---------|
| **DataStorage** | 15433 | 16379 | 18090 | Automated (BeforeSuite) |
| **Gateway** | Dynamic | 16380 | Dynamic | Automated (SynchronizedBeforeSuite) |
| **Manual (podman-compose)** | 15433 | 16379 | 18090 | Manual (`podman-compose up`) |

### Why No Port Collisions?

1. **DataStorage** uses ports 15433, 16379, 18090 (started in its own test suite)
2. **Gateway** uses dynamic ports for PostgreSQL/DS, fixed 16380 for Redis (started in its own test suite)
3. **Manual podman-compose** uses ports 15433, 16379, 18090 (only started when developer runs it)

**Key**: DataStorage integration tests and manual `podman-compose` use the SAME ports, but they're never running simultaneously:
- When DataStorage tests run → DataStorage starts its infrastructure
- When RO audit tests run → Developer manually starts `podman-compose`
- They don't interfere because they run at different times

---

## ✅ Action Items

### Completed
- [x] Fixed RO audit test to FAIL instead of Skip (per TESTING_GUIDELINES.md)
- [x] Documented actual architecture (each service starts own infrastructure)
- [x] Clarified that `podman-compose.test.yml` is manual, not automated

### Pending
- [ ] Consider if RO/WE/Notification should start their own DS in BeforeSuite (like Gateway does)
- [ ] Update CI/CD to handle manual infrastructure requirements
- [ ] Document Gateway's Dead Letter Queue (DLQ) implementation dependency on Redis

---

## 📚 References

- `test/integration/datastorage/suite_test.go` lines 335-427 - DataStorage BeforeSuite
- `test/integration/gateway/suite_test.go` lines 56-267 - Gateway SynchronizedBeforeSuite
- `test/integration/remediationorchestrator/suite_test.go` lines 87-202 - RO BeforeSuite
- `test/integration/remediationorchestrator/audit_integration_test.go` lines 50-73 - RO audit test
- `docs/development/business-requirements/TESTING_GUIDELINES.md` lines 420-536 - Skip() is FORBIDDEN

---

## 🎯 Summary

### The Truth About Integration Test Infrastructure

1. **NO shared automated infrastructure** - each service manages its own
2. **DataStorage and Gateway** start their own PostgreSQL + Redis + DS in BeforeSuite
3. **RO/WE/Notification** require MANUAL `podman-compose up` for audit tests
4. **SignalProcessing** integration tests use envtest only (ADR-038); E2E tests deploy DS in Kind (BR-SP-090)
5. **Skip() is ABSOLUTELY FORBIDDEN** - tests must FAIL if dependencies are missing
6. **Root `podman-compose.test.yml`** is a manual developer convenience, NOT automation

### Developer Workflows

**DataStorage/Gateway developers**: Just run `make test-integration-{service}` (infrastructure automated)

**RO/WE/Notification developers**:
```bash
# Start infrastructure MANUALLY first
podman-compose -f podman-compose.test.yml up -d

# Then run tests
make test-integration-remediationorchestrator

# Audit tests will pass because DS is at :18090
# If you forget this step → audit tests FAIL (as they should)
```

---

**Document Status**: ✅ Corrected (v1.2 - Actual Architecture)
**Created**: 2025-12-11
**Corrected**: 2025-12-11 (v1.2 - reflects actual code behavior)
**Skip() Violation**: ✅ Fixed in RO audit test
**TESTING_GUIDELINES.md**: ✅ Compliant
