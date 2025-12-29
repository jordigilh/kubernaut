# Notification Integration Infrastructure Reassessment - December 26, 2025

**Date**: December 26, 2025
**Status**: ✅ **ANALYSIS COMPLETE - Ready for Implementation**
**Related**: DD-TEST-002 (Parallel Test Execution Standard), SESSION_FINAL_SUMMARY_DEC_26_2025.md

---

## 🎯 **User Clarification**

**Key Insight**: "we don't use testcontainers-go, we use podman"

**Impact**: Need to use **Podman directly** via Go's `exec.Command`, not testcontainers-go library.

---

## 📊 **Current State Analysis**

### **Notification Integration Tests - Current Approach** ❌

**Pattern**: Shell Script Dependency

```go
// test/integration/notification/suite_test.go (lines 250-258)
Eventually(func() int {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        GinkgoWriter.Printf("  DataStorage health check failed: %v\n", err)
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK),
    "❌ REQUIRED: Data Storage not available at %s\n" +
    "To run these tests, start infrastructure:\n" +
    "  cd test/integration/notification\n" +
    "  ./setup-infrastructure.sh\n\n")  // ❌ Shell script dependency
```

**Issues**:
- Tests **depend** on external shell script
- Not portable (shell script must exist and be executable)
- Fails in CI/CD without manual setup
- No programmatic control over infrastructure lifecycle

### **Notification Constants - Already Defined** ✅

```go
// test/infrastructure/notification_integration.go
const (
    NTIntegrationPostgresPort         = 15439
    NTIntegrationRedisPort            = 16385
    NTIntegrationDataStoragePort      = 18096
    NTIntegrationMetricsPort          = 19096

    NTIntegrationPostgresContainer    = "notification_postgres_1"
    NTIntegrationRedisContainer       = "notification_redis_1"
    NTIntegrationDataStorageContainer = "notification_datastorage_1"
    NTIntegrationMigrationsContainer  = "notification_migrations"
    NTIntegrationNetwork              = "notification_test-network"

    NTIntegrationDBName               = "action_history"
    NTIntegrationDBUser               = "slm_user"
    NTIntegrationDBPassword           = "test_password"
)
```

**Status**: Constants are defined but no Start/Stop functions exist.

---

## 🔍 **Existing Service Patterns - Authoritative Analysis**

### **Pattern 1: podman-compose (Programmatic)** 📦

**Used By**: AIAnalysis Integration Tests

**Implementation**: `test/infrastructure/aianalysis.go:1585`

```go
func StartAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    composeFile := filepath.Join(projectRoot, AIAnalysisIntegrationComposeFile)

    // Start services via podman-compose
    cmd := exec.Command("podman-compose",
        "-f", composeFile,
        "-p", AIAnalysisIntegrationComposeProject,
        "up", "-d", "--build",
    )
    cmd.Dir = projectRoot
    cmd.Stdout = writer
    cmd.Stderr = writer

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start podman-compose stack: %w", err)
    }

    // Wait for services to be healthy
    if err := waitForHTTPHealth(
        fmt.Sprintf("http://localhost:%d/health", AIAnalysisIntegrationDataStoragePort),
        60*time.Second,
    ); err != nil {
        return fmt.Errorf("DataStorage failed to become healthy: %w", err)
    }

    return nil
}
```

**Pros**:
- ✅ Simple (one command starts all services)
- ✅ Declarative (compose file defines stack)
- ✅ Parallel startup (podman-compose handles ordering)

**Cons**:
- ❌ Requires podman-compose to be installed
- ❌ Less explicit about startup order
- ❌ Harder to debug individual service failures

### **Pattern 2: Sequential `podman run` (DD-TEST-002)** 🔄

**Used By**: Gateway, RemediationOrchestrator Integration Tests

**Implementation**: `test/infrastructure/gateway.go:47` and `test/infrastructure/remediationorchestrator.go:525`

```go
func StartGatewayIntegrationInfrastructure(writer io.Writer) error {
    // STEP 1: Cleanup existing containers
    fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
    cleanupContainers(writer)

    // STEP 2: Network setup (using host network for localhost connectivity)

    // STEP 3: Start PostgreSQL FIRST
    fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
    if err := startGatewayPostgreSQL(writer); err != nil {
        return fmt.Errorf("failed to start PostgreSQL: %w", err)
    }

    // CRITICAL: Wait for PostgreSQL to be ready before proceeding
    fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
    if err := waitForGatewayPostgresReady(writer); err != nil {
        return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
    }

    // STEP 4: Run migrations
    fmt.Fprintf(writer, "🔄 Running database migrations...\n")
    if err := runGatewayMigrations(projectRoot, writer); err != nil {
        return fmt.Errorf("failed to run migrations: %w", err)
    }

    // STEP 5: Start Redis
    fmt.Fprintf(writer, "🔴 Starting Redis...\n")
    if err := startGatewayRedis(writer); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // STEP 6: Start DataStorage LAST
    fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
    if err := startGatewayDataStorage(projectRoot, writer); err != nil {
        return fmt.Errorf("failed to start DataStorage: %w", err)
    }

    // Wait for DataStorage HTTP endpoint to be ready
    if err := waitForGatewayHTTPHealth(
        fmt.Sprintf("http://localhost:%d/health", GatewayIntegrationDataStoragePort),
        30*time.Second,
        writer,
    ); err != nil {
        return fmt.Errorf("DataStorage failed to become healthy: %w", err)
    }

    return nil
}
```

**Helper Functions** (from `gateway.go:226-240`):

```go
func startGatewayPostgreSQL(writer io.Writer) error {
    cmd := exec.Command("podman", "run", "-d",
        "--name", GatewayIntegrationPostgresContainer,
        "-p", fmt.Sprintf("%d:5432", GatewayIntegrationPostgresPort),
        "-e", "POSTGRES_USER=kubernaut",
        "-e", "POSTGRES_PASSWORD=kubernaut-test-password",
        "-e", "POSTGRES_DB=kubernaut",
        "postgres:16-alpine",
    )
    cmd.Stdout = writer
    cmd.Stderr = writer
    return cmd.Run()
}

func waitForGatewayPostgresReady(writer io.Writer) error {
    for i := 1; i <= 30; i++ {
        cmd := exec.Command("podman", "exec", GatewayIntegrationPostgresContainer,
            "pg_isready", "-U", "kubernaut", "-d", "kubernaut")
        if cmd.Run() == nil {
            fmt.Fprintf(writer, "   PostgreSQL ready (attempt %d/30)\n", i)
            return nil
        }
        time.Sleep(1 * time.Second)
    }
    return fmt.Errorf("PostgreSQL failed to become ready after 30 seconds")
}
```

**Pros**:
- ✅ **Explicit startup order** (eliminates race conditions)
- ✅ **No podman-compose dependency** (only podman needed)
- ✅ **Granular health checks** (per service, easy to debug)
- ✅ **DD-TEST-002 compliant** (authoritative pattern)

**Cons**:
- ❌ More verbose code (separate functions per service)
- ❌ Slightly slower (sequential, not parallel)

---

## 📋 **Pattern Comparison**

| Aspect | podman-compose | Sequential `podman run` |
|--------|----------------|------------------------|
| **Services Using** | AIAnalysis | Gateway, RO |
| **Command** | `podman-compose up` | `podman run` (per service) |
| **Startup Order** | Implicit (compose file) | Explicit (code order) |
| **Health Checks** | Single (after all) | Per-service (detailed) |
| **Debugging** | Harder (all-or-nothing) | Easier (service-by-service) |
| **Dependencies** | podman + podman-compose | podman only |
| **DD-TEST-002 Compliance** | Partial | ✅ **Full** |
| **Code Verbosity** | Low | Medium |
| **Reliability** | Good | **Excellent** |
| **Maintainability** | Medium | **High** |

---

## ✅ **Recommendation for Notification**

### **Decision**: **Sequential `podman run` (DD-TEST-002 Pattern)**

**Rationale**:
1. **DD-TEST-002 Compliance**: Gateway and RO (newer services) use this pattern
2. **No Extra Dependencies**: Only podman needed (already required)
3. **Better Debugging**: Per-service health checks simplify troubleshooting
4. **Explicit Control**: Clear startup order prevents race conditions
5. **Consistency**: 2/3 services use this pattern (emerging standard)

---

## 🛠️ **Implementation Plan**

### **Phase 1: Add Infrastructure Functions** (2 hours)

**File**: `test/infrastructure/notification_integration.go`

**Add Functions**:
1. `StartNotificationIntegrationInfrastructure(writer io.Writer) error`
2. `StopNotificationIntegrationInfrastructure(writer io.Writer) error`
3. `startNotificationPostgreSQL(writer io.Writer) error`
4. `waitForNotificationPostgresReady(writer io.Writer) error`
5. `runNotificationMigrations(projectRoot string, writer io.Writer) error`
6. `startNotificationRedis(writer io.Writer) error`
7. `waitForNotificationRedisReady(writer io.Writer) error`
8. `startNotificationDataStorage(projectRoot string, writer io.Writer) error`
9. `waitForNotificationHTTPHealth(healthURL string, timeout time.Duration, writer io.Writer) error`
10. `cleanupNotificationContainers(writer io.Writer)`

**Pattern**:
```go
// test/infrastructure/notification_integration.go

// StartNotificationIntegrationInfrastructure starts the Notification integration test infrastructure
// using sequential podman run commands per DD-TEST-002.
//
// Pattern: DD-TEST-002 Sequential Startup Pattern (Gateway/RO Implementation)
// - Sequential container startup (eliminates race conditions)
// - Explicit health checks after each service
// - No podman-compose (only podman needed)
// - Parallel-safe with unique ports (DD-TEST-001)
//
// Infrastructure Components:
// - PostgreSQL (port 15439): DataStorage backend
// - Redis (port 16385): DataStorage DLQ
// - DataStorage API (port 18096): Audit events
//
// Returns:
// - error: Any errors during infrastructure startup
func StartNotificationIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()

    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "Notification Integration Test Infrastructure Setup\n")
    fmt.Fprintf(writer, "Per DD-TEST-002: Sequential Startup Pattern\n")
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", NTIntegrationPostgresPort)
    fmt.Fprintf(writer, "  Redis:          localhost:%d\n", NTIntegrationRedisPort)
    fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", NTIntegrationDataStoragePort)
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

    // ============================================================================
    // STEP 1: Cleanup existing containers
    // ============================================================================
    fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
    cleanupNotificationContainers(writer)
    fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

    // ============================================================================
    // STEP 2: Start PostgreSQL FIRST
    // ============================================================================
    fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
    if err := startNotificationPostgreSQL(writer); err != nil {
        return fmt.Errorf("failed to start PostgreSQL: %w", err)
    }

    // CRITICAL: Wait for PostgreSQL to be ready before proceeding
    fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
    if err := waitForNotificationPostgresReady(writer); err != nil {
        return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
    }
    fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n\n")

    // ============================================================================
    // STEP 3: Run migrations
    // ============================================================================
    fmt.Fprintf(writer, "🔄 Running database migrations...\n")
    if err := runNotificationMigrations(projectRoot, writer); err != nil {
        return fmt.Errorf("failed to run migrations: %w", err)
    }
    fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

    // ============================================================================
    // STEP 4: Start Redis
    // ============================================================================
    fmt.Fprintf(writer, "🔴 Starting Redis...\n")
    if err := startNotificationRedis(writer); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // Wait for Redis to be ready
    fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
    if err := waitForNotificationRedisReady(writer); err != nil {
        return fmt.Errorf("Redis failed to become ready: %w", err)
    }
    fmt.Fprintf(writer, "   ✅ Redis ready\n\n")

    // ============================================================================
    // STEP 5: Start DataStorage LAST
    // ============================================================================
    fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
    if err := startNotificationDataStorage(projectRoot, writer); err != nil {
        return fmt.Errorf("failed to start DataStorage: %w", err)
    }

    // CRITICAL: Wait for DataStorage HTTP endpoint to be ready
    fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
    if err := waitForNotificationHTTPHealth(
        fmt.Sprintf("http://localhost:%d/health", NTIntegrationDataStoragePort),
        30*time.Second,
        writer,
    ); err != nil {
        // Print container logs for debugging
        fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
        logsCmd := exec.Command("podman", "logs", NTIntegrationDataStorageContainer)
        logsCmd.Stdout = writer
        logsCmd.Stderr = writer
        _ = logsCmd.Run()
        return fmt.Errorf("DataStorage failed to become healthy: %w", err)
    }
    fmt.Fprintf(writer, "   ✅ DataStorage ready\n\n")

    // ============================================================================
    // SUCCESS
    // ============================================================================
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "✅ Notification Integration Infrastructure Ready\n")
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "  PostgreSQL:        localhost:%d\n", NTIntegrationPostgresPort)
    fmt.Fprintf(writer, "  Redis:             localhost:%d\n", NTIntegrationRedisPort)
    fmt.Fprintf(writer, "  DataStorage HTTP:  http://localhost:%d\n", NTIntegrationDataStoragePort)
    fmt.Fprintf(writer, "  DataStorage Metrics: http://localhost:%d\n", NTIntegrationMetricsPort)
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

    return nil
}
```

### **Phase 2: Update Test Suite** (1 hour)

**File**: `test/integration/notification/suite_test.go`

**Changes**:

```go
// BEFORE (lines 231-258):
// Check if Data Storage is available (MANDATORY for integration tests)
// DS TEAM PATTERN: Use Eventually() with 30s timeout instead of immediate check
Eventually(func() int {
    resp, err := http.Get(dataStorageURL + "/health")
    if err != nil {
        GinkgoWriter.Printf("  DataStorage health check failed: %v\n", err)
        return 0
    }
    defer resp.Body.Close()
    return resp.StatusCode
}, "30s", "1s").Should(Equal(http.StatusOK),
    "❌ REQUIRED: Data Storage not available at %s\n" +
    "To run these tests, start infrastructure:\n" +  // ❌ Shell script dependency
    "  cd test/integration/notification\n" +
    "  ./setup-infrastructure.sh\n\n")

// AFTER:
// Data Storage availability is guaranteed by SynchronizedBeforeSuite
// which starts infrastructure programmatically (DD-TEST-002)
```

**Add to `SynchronizedBeforeSuite`**:

```go
var _ = SynchronizedBeforeSuite(
    // Process 1: Setup shared infrastructure (NEW!)
    func() []byte {
        GinkgoWriter.Printf("🔧 [Process %d] Setting up shared Notification integration infrastructure (DD-TEST-002)\n", GinkgoParallelProcess())

        // Start infrastructure programmatically (DD-TEST-002 pattern)
        if err := infrastructure.StartNotificationIntegrationInfrastructure(GinkgoWriter); err != nil {
            Fail(fmt.Sprintf("❌ Failed to start infrastructure: %v", err))
        }

        return []byte("infrastructure-ready")
    },
    // All processes: Confirm infrastructure is available
    func(data []byte) {
        GinkgoWriter.Printf("🔧 [Process %d] Confirming infrastructure availability...\n", GinkgoParallelProcess())
        // Infrastructure is ready from Process 1
    },
)
```

**Add to `SynchronizedAfterSuite`**:

```go
var _ = SynchronizedAfterSuite(
    func() {
        // All processes: Cleanup test namespaces
    },
    func() {
        // Process 1 ONLY: Cleanup shared infrastructure
        GinkgoWriter.Println("🧹 Cleaning up shared Notification integration infrastructure...")
        if err := infrastructure.StopNotificationIntegrationInfrastructure(GinkgoWriter); err != nil {
            GinkgoWriter.Printf("⚠️  Failed to stop infrastructure: %v\n", err)
        }
    },
)
```

### **Phase 3: Remove Shell Script** (15 minutes)

**Delete**:
- `test/integration/notification/setup-infrastructure.sh`
- `test/integration/notification/podman-compose.notification.test.yml`

**Update Documentation**:
- Update `test/infrastructure/notification_integration.go` USAGE NOTES section

### **Phase 4: Test and Verify** (30 minutes)

```bash
# Clean environment
podman stop notification_postgres_1 notification_redis_1 notification_datastorage_1
podman rm notification_postgres_1 notification_redis_1 notification_datastorage_1

# Run tests (infrastructure starts automatically)
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-integration-notification

# Verify infrastructure cleanup
podman ps -a | grep notification  # Should show NO containers after tests
```

---

## 📊 **Estimated Effort**

| Phase | Task | Duration |
|-------|------|----------|
| **Phase 1** | Add infrastructure functions | 2 hours |
| **Phase 2** | Update test suite | 1 hour |
| **Phase 3** | Remove shell script | 15 min |
| **Phase 4** | Test and verify | 30 min |
| **Total** | **End-to-end implementation** | **4 hours** |

---

## ✅ **Success Criteria**

### **Functional**:
- [ ] Tests start infrastructure automatically (no manual script)
- [ ] All infrastructure components start successfully
- [ ] Health checks pass for all services
- [ ] Tests run in parallel (4 processes)
- [ ] Infrastructure cleans up after tests

### **Technical**:
- [ ] DD-TEST-002 pattern fully implemented
- [ ] Sequential startup with per-service health checks
- [ ] No podman-compose dependency
- [ ] Follows Gateway/RO implementation patterns

### **Quality**:
- [ ] Code is well-documented
- [ ] Error messages are helpful for debugging
- [ ] Cleanup is reliable (no container leaks)

---

## 🎯 **Benefits**

### **Developer Experience**:
- ✅ **One Command**: `make test-integration-notification` (no setup script)
- ✅ **Portable**: Works on any machine with podman installed
- ✅ **Reliable**: Sequential startup eliminates race conditions
- ✅ **Debuggable**: Per-service health checks simplify troubleshooting

### **CI/CD**:
- ✅ **No Manual Setup**: Infrastructure starts automatically
- ✅ **Repeatable**: Same behavior every run
- ✅ **Fast**: 30-60 seconds for infrastructure startup
- ✅ **Clean**: Automatic cleanup prevents resource leaks

### **Maintenance**:
- ✅ **Standard Pattern**: Follows Gateway/RO (DD-TEST-002)
- ✅ **Self-Contained**: No external scripts to maintain
- ✅ **Explicit**: Clear startup order and dependencies
- ✅ **Testable**: Infrastructure code can be unit tested

---

## 📚 **Related Documentation**

1. **DD-TEST-002**: Parallel Test Execution Standard (authoritative)
2. **test/infrastructure/gateway.go**: Reference implementation (lines 47-150)
3. **test/infrastructure/remediationorchestrator.go**: Reference implementation (lines 525-650)
4. **SESSION_FINAL_SUMMARY_DEC_26_2025.md**: Context for this work

---

## 🎓 **Key Learnings**

### **Pattern Selection Criteria**:

When choosing between podman-compose vs sequential `podman run`:

| Scenario | Recommendation |
|----------|---------------|
| **Simple stack** (1-2 services) | Either works |
| **Complex stack** (3+ services) | Sequential `podman run` |
| **Debugging required** | Sequential `podman run` |
| **Standard compliance** | Sequential `podman run` (DD-TEST-002) |
| **Minimal code** | podman-compose |

### **DD-TEST-002 Core Principles**:

1. **Sequential Startup**: Start dependencies first (PostgreSQL → Migrations → Redis → DataStorage)
2. **Health Checks**: Wait for each service to be ready before proceeding
3. **Explicit Errors**: Detailed error messages for each failure point
4. **Clean Cleanup**: Remove all containers and networks in AfterSuite
5. **Parallel-Safe**: Use unique ports per service (DD-TEST-001)

---

## 📝 **Implementation Checklist**

**Before Starting**:
- [ ] Read Gateway implementation: `test/infrastructure/gateway.go:47-250`
- [ ] Read RO implementation: `test/infrastructure/remediationorchestrator.go:525-650`
- [ ] Understand DD-TEST-002 pattern

**During Implementation**:
- [ ] Copy-paste Gateway pattern as template
- [ ] Update constants to use Notification values
- [ ] Test each step individually (PostgreSQL → Migrations → Redis → DataStorage)
- [ ] Add comprehensive error messages
- [ ] Implement cleanup functions

**After Implementation**:
- [ ] Run tests locally (verify 100% pass rate)
- [ ] Verify infrastructure cleanup (no leaked containers)
- [ ] Update documentation
- [ ] Commit with detailed message

---

**Document Version**: 1.0.0
**Created**: December 26, 2025
**Status**: Analysis Complete - Ready for Implementation
**Estimated Implementation**: 4 hours
**Next Action**: Implement Phase 1 (Add infrastructure functions)




