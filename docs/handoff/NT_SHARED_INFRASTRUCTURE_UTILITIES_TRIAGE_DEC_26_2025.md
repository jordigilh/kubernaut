# Shared Infrastructure Utilities Triage - December 26, 2025

**Date**: December 26, 2025
**Status**: ✅ **ANALYSIS COMPLETE - Recommendation Ready**
**Related**: NT_INFRASTRUCTURE_REASSESSMENT_DEC_26_2025.md, DD-TEST-002

---

## 🎯 **Problem Statement**

**User Request**: "reuse shared functions. Triage other services for insight"

**Finding**: Significant code duplication across 4+ integration infrastructure files (Gateway, RO, WE, SignalProcessing)

---

## 📊 **Duplication Analysis**

### **Current State**: Each Service Has Duplicated Code

| Function Pattern | Gateway | RO | WE | SignalProcessing | Lines Each | Total Waste |
|-----------------|---------|----|----|------------------|------------|-------------|
| **Start PostgreSQL** | ✅ | ✅ | ✅ | ✅ | ~15 | ~60 lines |
| **Wait PostgreSQL Ready** | ✅ | ✅ | ✅ | ✅ | ~20 | ~80 lines |
| **Start Redis** | ✅ | ✅ | ✅ | ✅ | ~15 | ~60 lines |
| **Wait Redis Ready** | ✅ | ✅ | ✅ | ✅ | ~20 | ~80 lines |
| **Run Migrations** | ✅ | ✅ | ✅ | ✅ | ~40 | ~160 lines |
| **Start DataStorage** | ✅ | ✅ | ✅ | ✅ | ~30 | ~120 lines |
| **HTTP Health Check** | ✅ | ✅ | ✅ | ✅ | ~25 | ~100 lines |
| **Cleanup Containers** | ✅ | ✅ | ✅ | ✅ | ~15 | ~60 lines |
| **TOTAL DUPLICATION** | | | | | | **~720 lines** |

### **Code Examples Showing Duplication**

#### **Example 1: Start PostgreSQL** (Almost Identical)

**Gateway** (`gateway.go:227-240`):
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
```

**WorkflowExecution** (`workflowexecution_integration_infra.go:235-248`):
```go
func startWEPostgreSQL(writer io.Writer) error {
    cmd := exec.Command("podman", "run",
        "-d",
        "--name", WEIntegrationPostgresContainer,
        "-p", fmt.Sprintf("%d:5432", WEIntegrationPostgresPort),
        "-e", "POSTGRES_DB="+WEIntegrationDBName,
        "-e", "POSTGRES_USER="+WEIntegrationDBUser,
        "-e", "POSTGRES_PASSWORD="+WEIntegrationDBPassword,
        "postgres:16-alpine",
    )
    cmd.Stdout = writer
    cmd.Stderr = writer
    return cmd.Run()
}
```

**Difference**: ONLY container name, port, and credentials (all parameters!)

#### **Example 2: Wait for PostgreSQL** (Identical Logic)

**Gateway** (`gateway.go:243-257`):
```go
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

**RemediationOrchestrator** (`remediationorchestrator.go:566-576`):
```go
for i := 1; i <= 30; i++ {
    cmd := exec.Command("podman", "exec", ROIntegrationPostgresContainer, "pg_isready", "-U", "slm_user")
    if err := cmd.Run(); err == nil {
        fmt.Fprintf(writer, "✅ PostgreSQL ready after %d seconds\n", i)
        break
    }
    if i == 30 {
        return fmt.Errorf("PostgreSQL failed to become ready after 30 seconds")
    }
    time.Sleep(1 * time.Second)
}
```

**Difference**: ONLY container name and user (both parameters!)

#### **Example 3: HTTP Health Check** (100% Identical Logic)

**Gateway** (`gateway.go:368-401`):
```go
func waitForGatewayHTTPHealth(healthURL string, timeout time.Duration, writer io.Writer) error {
    deadline := time.Now().Add(timeout)
    client := &http.Client{Timeout: 5 * time.Second}

    for time.Now().Before(deadline) {
        resp, err := client.Get(healthURL)
        if err == nil {
            resp.Body.Close()
            if resp.StatusCode == http.StatusOK {
                return nil
            }
        }
        time.Sleep(1 * time.Second)
    }
    return fmt.Errorf("health check failed for %s after %v", healthURL, timeout)
}
```

**RemediationOrchestrator** (`remediationorchestrator.go:804-827`):
```go
func waitForROHTTPHealth(healthURL string, timeout time.Duration, writer io.Writer) error {
    deadline := time.Now().Add(timeout)
    client := &http.Client{Timeout: 5 * time.Second}
    attempt := 0

    for time.Now().Before(deadline) {
        attempt++
        resp, err := client.Get(healthURL)
        if err == nil {
            resp.Body.Close()
            if resp.StatusCode == http.StatusOK {
                fmt.Fprintf(writer, "   ✅ Health check passed after %d attempts\n", attempt)
                return nil
            }
            fmt.Fprintf(writer, "   ⏳ Attempt %d: Status %d (waiting for 200 OK)...\n", attempt, resp.StatusCode)
        } else {
            if attempt%5 == 0 { // Log every 5th attempt
                fmt.Fprintf(writer, "   ⏳ Attempt %d: Connection failed (%v), retrying...\n", attempt, err)
            }
        }
        time.Sleep(1 * time.Second)
    }
    return fmt.Errorf("health check failed for %s after %v", healthURL, timeout)
}
```

**Difference**: Minor logging variations (can be unified)

---

## ✅ **Recommendation: Create Shared Utilities**

### **Decision**: **Create `test/infrastructure/shared_integration_utils.go`**

**Rationale**:
1. **DRY Principle**: Same logic in 4+ files
2. **Maintainability**: Fix once, benefit everywhere
3. **Consistency**: Identical behavior across services
4. **Testability**: Shared utilities can be unit tested
5. **Extensibility**: Easy to add new services (like Notification)

---

## 🛠️ **Proposed Shared Functions**

### **File**: `test/infrastructure/shared_integration_utils.go`

```go
package infrastructure

import (
    "fmt"
    "io"
    "net/http"
    "os/exec"
    "time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Shared Integration Test Infrastructure Utilities
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// These utilities are shared across all integration test infrastructure:
// - Gateway, RemediationOrchestrator, WorkflowExecution, SignalProcessing, Notification
//
// Per DD-TEST-002: Sequential Startup Pattern
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// PostgreSQLConfig holds configuration for starting PostgreSQL container
type PostgreSQLConfig struct {
    ContainerName string
    Port          int
    DBName        string
    DBUser        string
    DBPassword    string
}

// StartPostgreSQL starts a PostgreSQL container for integration tests
func StartPostgreSQL(cfg PostgreSQLConfig, writer io.Writer) error {
    cmd := exec.Command("podman", "run", "-d",
        "--name", cfg.ContainerName,
        "-p", fmt.Sprintf("%d:5432", cfg.Port),
        "-e", "POSTGRES_DB="+cfg.DBName,
        "-e", "POSTGRES_USER="+cfg.DBUser,
        "-e", "POSTGRES_PASSWORD="+cfg.DBPassword,
        "postgres:16-alpine",
    )
    cmd.Stdout = writer
    cmd.Stderr = writer
    return cmd.Run()
}

// WaitForPostgreSQLReady waits for PostgreSQL to be ready to accept connections
func WaitForPostgreSQLReady(containerName, dbUser, dbName string, writer io.Writer) error {
    maxAttempts := 30
    for i := 1; i <= maxAttempts; i++ {
        cmd := exec.Command("podman", "exec", containerName,
            "pg_isready", "-U", dbUser, "-d", dbName)
        if cmd.Run() == nil {
            fmt.Fprintf(writer, "   ✅ PostgreSQL ready (attempt %d/%d)\n", i, maxAttempts)
            return nil
        }
        if i < maxAttempts {
            time.Sleep(1 * time.Second)
        }
    }
    return fmt.Errorf("PostgreSQL failed to become ready after %d attempts", maxAttempts)
}

// RedisConfig holds configuration for starting Redis container
type RedisConfig struct {
    ContainerName string
    Port          int
}

// StartRedis starts a Redis container for integration tests
func StartRedis(cfg RedisConfig, writer io.Writer) error {
    cmd := exec.Command("podman", "run", "-d",
        "--name", cfg.ContainerName,
        "-p", fmt.Sprintf("%d:6379", cfg.Port),
        "redis:7-alpine",
    )
    cmd.Stdout = writer
    cmd.Stderr = writer
    return cmd.Run()
}

// WaitForRedisReady waits for Redis to be ready to accept connections
func WaitForRedisReady(containerName string, writer io.Writer) error {
    maxAttempts := 30
    for i := 1; i <= maxAttempts; i++ {
        cmd := exec.Command("podman", "exec", containerName, "redis-cli", "ping")
        if output, err := cmd.CombinedOutput(); err == nil && string(output) == "PONG\n" {
            fmt.Fprintf(writer, "   ✅ Redis ready (attempt %d/%d)\n", i, maxAttempts)
            return nil
        }
        if i < maxAttempts {
            time.Sleep(1 * time.Second)
        }
    }
    return fmt.Errorf("Redis failed to become ready after %d attempts", maxAttempts)
}

// WaitForHTTPHealth waits for an HTTP health endpoint to return 200 OK
func WaitForHTTPHealth(healthURL string, timeout time.Duration, writer io.Writer) error {
    deadline := time.Now().Add(timeout)
    client := &http.Client{Timeout: 5 * time.Second}
    attempt := 0

    for time.Now().Before(deadline) {
        attempt++
        resp, err := client.Get(healthURL)
        if err == nil {
            resp.Body.Close()
            if resp.StatusCode == http.StatusOK {
                fmt.Fprintf(writer, "   ✅ Health check passed (attempt %d)\n", attempt)
                return nil
            }
            // Log every 5th non-OK status
            if attempt%5 == 0 {
                fmt.Fprintf(writer, "   ⏳ Attempt %d: Status %d (waiting for 200 OK)...\n", attempt, resp.StatusCode)
            }
        } else {
            // Log every 5th connection error
            if attempt%5 == 0 {
                fmt.Fprintf(writer, "   ⏳ Attempt %d: Connection failed (%v), retrying...\n", attempt, err)
            }
        }
        time.Sleep(1 * time.Second)
    }
    return fmt.Errorf("health check failed for %s after %v (attempts: %d)", healthURL, timeout, attempt)
}

// CleanupContainers stops and removes containers
func CleanupContainers(containerNames []string, writer io.Writer) {
    for _, container := range containerNames {
        // Stop container
        stopCmd := exec.Command("podman", "stop", "-t", "5", container)
        stopCmd.Stdout = writer
        stopCmd.Stderr = writer
        _ = stopCmd.Run() // Ignore errors (container might not exist)

        // Remove container
        rmCmd := exec.Command("podman", "rm", "-f", container)
        rmCmd.Stdout = writer
        rmCmd.Stderr = writer
        _ = rmCmd.Run() // Ignore errors
    }
}

// MigrationsConfig holds configuration for running database migrations
type MigrationsConfig struct {
    ContainerName    string
    Network          string
    PostgresHost     string
    PostgresPort     int
    DBName           string
    DBUser           string
    DBPassword       string
    MigrationsImage  string
}

// RunMigrations runs database migrations in a temporary container
func RunMigrations(cfg MigrationsConfig, writer io.Writer) error {
    cmd := exec.Command("podman", "run", "--rm",
        "--network", cfg.Network,
        "--name", cfg.ContainerName,
        "-e", "DATABASE_URL=postgres://"+cfg.DBUser+":"+cfg.DBPassword+"@"+cfg.PostgresHost+":"+fmt.Sprintf("%d", cfg.PostgresPort)+"/"+cfg.DBName,
        cfg.MigrationsImage,
    )
    cmd.Stdout = writer
    cmd.Stderr = writer
    return cmd.Run()
}
```

---

## 📋 **Migration Plan**

### **Phase 1: Create Shared Utilities** (1 hour)
1. Create `test/infrastructure/shared_integration_utils.go`
2. Implement 7 shared functions
3. Add comprehensive documentation
4. Add unit tests (optional but recommended)

### **Phase 2: Migrate Notification** (1 hour)
1. Use shared utilities in `notification_integration.go`
2. Implement service-specific `StartNotificationIntegrationInfrastructure()`
3. Test locally

### **Phase 3: Migrate Other Services** (Optional - 2 hours)
1. Refactor Gateway to use shared utilities
2. Refactor RO to use shared utilities
3. Refactor WE to use shared utilities
4. Refactor SignalProcessing to use shared utilities

---

## ✅ **Immediate Action: Notification Implementation**

### **Use Shared Utilities from Day 1**

**File**: `test/infrastructure/notification_integration.go`

```go
// StartNotificationIntegrationInfrastructure starts the Notification integration test infrastructure
// using sequential podman run commands per DD-TEST-002.
func StartNotificationIntegrationInfrastructure(writer io.Writer) error {
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "Notification Integration Test Infrastructure Setup\n")
    fmt.Fprintf(writer, "Per DD-TEST-002: Sequential Startup Pattern\n")
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

    // STEP 1: Cleanup existing containers
    fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
    CleanupContainers([]string{
        NTIntegrationPostgresContainer,
        NTIntegrationRedisContainer,
        NTIntegrationDataStorageContainer,
        NTIntegrationMigrationsContainer,
    }, writer)
    fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

    // STEP 2: Start PostgreSQL
    fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
    if err := StartPostgreSQL(PostgreSQLConfig{
        ContainerName: NTIntegrationPostgresContainer,
        Port:          NTIntegrationPostgresPort,
        DBName:        NTIntegrationDBName,
        DBUser:        NTIntegrationDBUser,
        DBPassword:    NTIntegrationDBPassword,
    }, writer); err != nil {
        return fmt.Errorf("failed to start PostgreSQL: %w", err)
    }

    // STEP 3: Wait for PostgreSQL ready
    fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
    if err := WaitForPostgreSQLReady(
        NTIntegrationPostgresContainer,
        NTIntegrationDBUser,
        NTIntegrationDBName,
        writer,
    ); err != nil {
        return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
    }
    fmt.Fprintf(writer, "\n")

    // STEP 4: Run migrations
    fmt.Fprintf(writer, "🔄 Running database migrations...\n")
    if err := runNotificationMigrations(writer); err != nil {
        return fmt.Errorf("failed to run migrations: %w", err)
    }
    fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

    // STEP 5: Start Redis
    fmt.Fprintf(writer, "🔴 Starting Redis...\n")
    if err := StartRedis(RedisConfig{
        ContainerName: NTIntegrationRedisContainer,
        Port:          NTIntegrationRedisPort,
    }, writer); err != nil {
        return fmt.Errorf("failed to start Redis: %w", err)
    }

    // STEP 6: Wait for Redis ready
    fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
    if err := WaitForRedisReady(NTIntegrationRedisContainer, writer); err != nil {
        return fmt.Errorf("Redis failed to become ready: %w", err)
    }
    fmt.Fprintf(writer, "\n")

    // STEP 7: Start DataStorage
    fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
    if err := startNotificationDataStorage(writer); err != nil {
        return fmt.Errorf("failed to start DataStorage: %w", err)
    }

    // STEP 8: Wait for DataStorage HTTP health
    fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
    if err := WaitForHTTPHealth(
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
    fmt.Fprintf(writer, "\n")

    // SUCCESS
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "✅ Notification Integration Infrastructure Ready\n")
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

    return nil
}

// StopNotificationIntegrationInfrastructure stops and cleans up the infrastructure
func StopNotificationIntegrationInfrastructure(writer io.Writer) error {
    fmt.Fprintf(writer, "🛑 Stopping Notification Integration Infrastructure...\n")

    CleanupContainers([]string{
        NTIntegrationDataStorageContainer,
        NTIntegrationRedisContainer,
        NTIntegrationPostgresContainer,
        NTIntegrationMigrationsContainer,
    }, writer)

    // Remove network (ignore errors)
    networkCmd := exec.Command("podman", "network", "rm", NTIntegrationNetwork)
    _ = networkCmd.Run()

    fmt.Fprintf(writer, "✅ Notification Integration Infrastructure stopped\n")
    return nil
}

// runNotificationMigrations is service-specific (migration image/command may vary)
func runNotificationMigrations(writer io.Writer) error {
    // Service-specific migration logic
    // (This would call RunMigrations() with service-specific config)
    return nil
}

// startNotificationDataStorage is service-specific (DataStorage version/config may vary)
func startNotificationDataStorage(writer io.Writer) error {
    // Service-specific DataStorage startup logic
    return nil
}
```

---

## 📊 **Benefits**

### **Code Reduction**:
- **Before**: ~720 lines duplicated across 4 services
- **After**: ~200 lines shared utilities + ~50 lines per service = ~400 lines total
- **Savings**: **~320 lines (-44%)**

### **Maintainability**:
- ✅ Fix bugs once, benefit everywhere
- ✅ Consistent behavior across all services
- ✅ Easier to add new services (like Notification)
- ✅ Testable shared utilities

### **Developer Experience**:
- ✅ Clear, documented interfaces
- ✅ Reduced cognitive load (less code to understand)
- ✅ Faster implementation of new services

---

## 🎯 **Recommendation**

### **Immediate**: Create Shared Utilities + Use for Notification

1. **Create** `test/infrastructure/shared_integration_utils.go` (7 functions)
2. **Implement** Notification using shared utilities
3. **Test** locally to validate

### **Short-Term** (Next Sprint): Migrate Existing Services

4. **Refactor** Gateway, RO, WE, SignalProcessing to use shared utilities
5. **Remove** duplicated code
6. **Update** documentation

### **Long-Term**: Enforce Standard

7. **Add** to coding standards: "MUST use shared infrastructure utilities"
8. **Document** in DD-TEST-002
9. **Create** linter rule to detect duplication

---

**Document Version**: 1.0.0
**Created**: December 26, 2025
**Status**: Analysis Complete - Ready for Implementation
**Estimated Effort**: 2 hours (shared utils + Notification)
**Next Action**: Create `shared_integration_utils.go`




