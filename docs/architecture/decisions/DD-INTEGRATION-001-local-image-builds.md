# DD-INTEGRATION-001: Local Image Builds for Integration Tests

**Status**: 🔄 SUPERSEDED by DD-TEST-002 (Programmatic Podman Setup)
**Date**: December 16, 2025 (Original) | December 26, 2025 (v2.0 Update)
**Author**: Platform Team
**Category**: Testing Infrastructure
**Version**: 2.0
**Scope**: All Service Integration Tests

---

## ⚠️ **DEPRECATION NOTICE (December 26, 2025)**

**v1.0 Pattern (podman-compose) is DEPRECATED**. All services MUST migrate to:
- **DD-TEST-002**: Programmatic Podman setup using Go code
- **Composite image tags**: `{service}-{uuid}` for collision avoidance
- **Sequential startup**: PostgreSQL → Migrations → Redis → Services

**Migration Status**: 7/8 services migrated (Notification, Gateway, RO, WE, SP, AIAnalysis, HolmesGPT-API)

---

## 🎯 **Executive Summary (v2.0)**

**Current Decision**: All integration tests MUST build container images programmatically using Go code with `podman run` commands, NOT `podman-compose`.

**Image Tagging**: Use composite tags `{target-service}-{uuid}` to prevent collisions between parallel test runs.

**Rationale**:
- Eliminates `podman-compose` race conditions and timing issues
- Enables explicit health checks and sequential startup
- Provides better test isolation with unique image tags
- Matches E2E infrastructure pattern (programmatic Go setup)

**Affected Services**: All services with integration tests (AIAnalysis, WorkflowExecution, SignalProcessing, Gateway, DataStorage, Notification, RemediationOrchestrator, etc.)

---

## 📋 **Context**

### **Problem Statement**

Integration tests were failing due to attempts to pull non-existent images from Docker Hub:

```yaml
# ❌ PROBLEMATIC Pattern
services:
  datastorage:
    image: kubernaut/datastorage:latest  # Tries to pull from Docker Hub
    # build:                              # Build section commented out
    #   context: ../../../
    #   dockerfile: cmd/datastorage/Dockerfile
```

**Error**:
```
Error: unable to copy from source docker://kubernaut/datastorage:latest:
       requested access to the resource is denied
```

### **Why This Happened**

1. ❌ Build sections commented out due to temporary issues
2. ❌ Comments became permanent without resolution
3. ❌ Tests assumed images existed elsewhere (E2E builds, manual builds)
4. ❌ No clear documentation on integration test image strategy

### **Impact**

- ❌ Integration tests completely blocked (0/N tests running)
- ❌ Infrastructure timeouts (15 minutes waiting for non-existent images)
- ❌ No validation of service coordination and cross-component integration
- ❌ V1.0 readiness unclear due to missing integration test results

---

## ✅ **Decision (v2.0 - Current Standard)**

### **Mandatory Requirements**

#### **1. Programmatic Podman Setup via Go** (REQUIRED)

All integration tests MUST use programmatic `podman run` commands in Go code:

```go
// ✅ CORRECT Pattern (v2.0)
// File: test/infrastructure/{service}_integration.go

func Start{Service}IntegrationInfrastructure(writer io.Writer) error {
    // STEP 1: Start PostgreSQL using shared utility
    pgConfig := PostgreSQLConfig{
        ContainerName: "{service}_postgres_1",
        Port:          15XXX, // Per DD-TEST-001
        DBName:        "action_history",
        DBUser:        "slm_user",
        DBPassword:    "test_password",
        Network:       "{service}_test-network",
    }
    if err := StartPostgreSQL(pgConfig, writer); err != nil {
        return err
    }

    // STEP 2: Wait for PostgreSQL (explicit health check)
    if err := WaitForPostgreSQLReady(...); err != nil {
        return err
    }

    // STEP 3: Run migrations
    // STEP 4: Start Redis
    // STEP 5: Start DataStorage
    // etc.
}
```

##### **Option B: Python Services** (pytest fixtures pattern)

Python integration tests MUST use pytest fixtures in `conftest.py`:

```python
# File: tests/integration/conftest.py
# Pattern: DD-INTEGRATION-001 v2.0 - Python Pytest Fixtures

import os
import subprocess
from pathlib import Path
import pytest

def start_infrastructure() -> bool:
    """Start infrastructure using Python (no shell scripts)."""
    compose_cmd = "podman-compose"
    result = subprocess.run(
        [compose_cmd, "-f", "docker-compose.yml", "-p", "project", "up", "-d"],
        capture_output=True, timeout=180
    )
    return result.returncode == 0 and wait_for_services()

@pytest.fixture(scope="session")
def integration_infrastructure():
    """Session fixture managing infrastructure lifecycle."""
    if not is_infra_available():
        pytest.fail("REQUIRED: Infrastructure not running")
    yield
    # Cleanup via pytest_sessionfinish hook

def pytest_sessionfinish(session, exitstatus):
    """Automatic cleanup after test session."""
    for container in CONTAINERS:
        subprocess.run(["podman", "stop", container], check=False, capture_output=True)
        subprocess.run(["podman", "rm", "-f", container], check=False, capture_output=True)
    subprocess.run(["podman", "image", "prune", "-f"], check=False, capture_output=True)
```

**Reference**: HolmesGPT-API `holmesgpt-api/tests/integration/conftest.py` (complete implementation)

#### **2. Composite Image Tags** (REQUIRED)

Use composite tags to prevent collisions:

```go
// ✅ CORRECT: Composite tag with service and UUID
imageTag := fmt.Sprintf("{service}-%s", uuid.New().String())

buildCmd := exec.Command("podman", "build",
    "-t", imageTag,
    "-f", "build/{service}/Dockerfile",
    ".")
```

**Example tags**:
```
notification-a3b5c7d9-e1f2-4a5b-8c9d-0e1f2a3b4c5d
gateway-f7e8d9c0-b1a2-3c4d-5e6f-7a8b9c0d1e2f
```

#### **3. No External Registry Dependencies** (REQUIRED)

Integration tests MUST NOT depend on:
- ❌ Docker Hub images (public or private)
- ❌ E2E-built images (different test tier)
- ❌ Manually built images (not reproducible)
- ❌ CI/CD artifact registries (coupling)
- ❌ `podman-compose` (deprecated pattern)

#### **4. Sequential Startup with Health Checks** (REQUIRED)

Follow DD-TEST-002 pattern:

```go
// ✅ CORRECT: Explicit sequential startup
// 1. Start PostgreSQL
StartPostgreSQL(...)
WaitForPostgreSQLReady(...)  // BLOCK until ready

// 2. Run migrations
RunMigrations(...)

// 3. Start Redis
StartRedis(...)
WaitForRedisReady(...)  // BLOCK until ready

// 4. Start DataStorage
buildAndStartDataStorage(...)
WaitForHTTPHealth(...)  // BLOCK until ready
```

#### **5. Use Shared Infrastructure Utilities** (REQUIRED)

Reuse functions from `test/infrastructure/shared_integration_utils.go`:

```go
import shared "github.com/jordigilh/kubernaut/test/infrastructure"

// Available shared utilities:
// - StartPostgreSQL(cfg, writer)
// - WaitForPostgreSQLReady(container, user, db, writer)
// - StartRedis(cfg, writer)
// - WaitForRedisReady(container, writer)
// - RunMigrations(cfg, writer)
// - WaitForHTTPHealth(url, timeout, writer)
// - CleanupContainers(names, writer)
```

---

## 🏗️ **Implementation Pattern (v2.0)**

### **Standard Programmatic Go Structure**

```go
// File: test/infrastructure/{service}_integration.go
// Pattern: DD-TEST-002 Sequential Startup with Shared Utilities

package infrastructure

import (
    "fmt"
    "io"
    "os/exec"
    "path/filepath"
    "time"

    shared "github.com/jordigilh/kubernaut/test/infrastructure"
)

// Port allocation per DD-TEST-001
const (
    {Service}IntegrationPostgresPort = 15XXX  // Unique per service
    {Service}IntegrationRedisPort    = 16XXX
    {Service}IntegrationDataStoragePort = 18XXX
)

// Container names
const (
    {Service}IntegrationPostgresContainer    = "{service}_postgres_1"
    {Service}IntegrationRedisContainer       = "{service}_redis_1"
    {Service}IntegrationDataStorageContainer = "{service}_datastorage_1"
    {Service}IntegrationNetwork              = "{service}_test-network"
)

// Start{Service}IntegrationInfrastructure starts all dependencies
func Start{Service}IntegrationInfrastructure(writer io.Writer) error {
    fmt.Fprintf(writer, "Starting {Service} Integration Infrastructure...\n")

    projectRoot := getProjectRoot()

    // STEP 1: Cleanup and create network
    shared.CleanupContainers([]string{...}, writer)
    exec.Command("podman", "network", "create", {Service}IntegrationNetwork).Run()

    // STEP 2: Start PostgreSQL (using shared utility)
    pgConfig := shared.PostgreSQLConfig{
        ContainerName:  {Service}IntegrationPostgresContainer,
        Port:           {Service}IntegrationPostgresPort,
        DBName:         "action_history",
        DBUser:         "slm_user",
        DBPassword:     "test_password",
        Network:        {Service}IntegrationNetwork,
        MaxConnections: 200,
    }
    if err := shared.StartPostgreSQL(pgConfig, writer); err != nil {
        return err
    }

    // CRITICAL: Wait for PostgreSQL
    if err := shared.WaitForPostgreSQLReady(
        {Service}IntegrationPostgresContainer, "slm_user", "action_history", writer,
    ); err != nil {
        return err
    }

    // STEP 3: Run migrations (using shared utility)
    migrationsConfig := shared.MigrationsConfig{
        ContainerName: "{service}_migrations",
        Network:       {Service}IntegrationNetwork,
        PostgresHost:  {Service}IntegrationPostgresContainer,
        PostgresPort:  5432, // Internal port
        DBName:        "action_history",
        DBUser:        "slm_user",
        DBPassword:    "test_password",
        MigrationsDir: "migrations/datastorage",
        ProjectRoot:   projectRoot,
    }
    if err := shared.RunMigrations(migrationsConfig, writer); err != nil {
        return err
    }

    // STEP 4: Start Redis (using shared utility)
    redisConfig := shared.RedisConfig{
        ContainerName: {Service}IntegrationRedisContainer,
        Port:          {Service}IntegrationRedisPort,
        Network:       {Service}IntegrationNetwork,
    }
    if err := shared.StartRedis(redisConfig, writer); err != nil {
        return err
    }

    // CRITICAL: Wait for Redis
    if err := shared.WaitForRedisReady(
        {Service}IntegrationRedisContainer, writer,
    ); err != nil {
        return err
    }

    // STEP 5: Build and start DataStorage
    // Use composite image tag for collision avoidance
    dsImage := fmt.Sprintf("datastorage-%s", uuid.New().String())
    buildCmd := exec.Command("podman", "build", "-t", dsImage,
        "-f", filepath.Join(projectRoot, "build/datastorage/Dockerfile"),
        projectRoot,
    )
    buildCmd.Stdout = writer
    buildCmd.Stderr = writer
    if err := buildCmd.Run(); err != nil {
        return fmt.Errorf("failed to build DataStorage: %w", err)
    }

    // Start DataStorage container
    dsCmd := exec.Command("podman", "run", "-d",
        "--name", {Service}IntegrationDataStorageContainer,
        "--network", {Service}IntegrationNetwork,
        "-p", fmt.Sprintf("%d:8080", {Service}IntegrationDataStoragePort),
        "-e", "POSTGRES_HOST="+{Service}IntegrationPostgresContainer,
        "-e", "POSTGRES_PORT=5432",
        "-e", "POSTGRES_USER=slm_user",
        "-e", "POSTGRES_PASSWORD=test_password",
        "-e", "POSTGRES_DB=action_history",
        "-e", "REDIS_HOST="+{Service}IntegrationRedisContainer,
        "-e", "REDIS_PORT=6379",
        dsImage,
    )
    dsCmd.Stdout = writer
    dsCmd.Stderr = writer
    if err := dsCmd.Run(); err != nil {
        return err
    }

    // CRITICAL: Wait for DataStorage HTTP health
    if err := shared.WaitForHTTPHealth(
        fmt.Sprintf("http://localhost:%d/health", {Service}IntegrationDataStoragePort),
        60*time.Second,
        writer,
    ); err != nil {
        return err
    }

    fmt.Fprintf(writer, "✅ {Service} Integration Infrastructure Ready\n")
    return nil
}

// Stop{Service}IntegrationInfrastructure cleans up all containers
func Stop{Service}IntegrationInfrastructure(writer io.Writer) error {
    fmt.Fprintf(writer, "Stopping {Service} Integration Infrastructure...\n")

    containers := []string{
        {Service}IntegrationDataStorageContainer,
        {Service}IntegrationRedisContainer,
        {Service}IntegrationPostgresContainer,
    }
    shared.CleanupContainers(containers, writer)

    exec.Command("podman", "network", "rm", {Service}IntegrationNetwork).Run()

    fmt.Fprintf(writer, "✅ Infrastructure cleaned up\n")
    return nil
}
```

---

## 🐍 **Python Services (pytest Fixtures Pattern)**

### **HolmesGPT-API Integration Tests**

Python services (like HolmesGPT-API) use pytest fixtures instead of programmatic Podman commands, but follow the same principles:

**Pattern**: Framework manages infrastructure (no shell scripts)

```python
# holmesgpt-api/tests/integration/conftest.py

import pytest
import subprocess
import shutil
import os

def start_infrastructure() -> bool:
    """
    Start integration infrastructure using Python (no shell scripts).

    Benefits:
    - Consistency with Go service patterns (framework manages infrastructure)
    - Better error handling (Python exceptions propagate to pytest)
    - Simpler maintenance (single source of truth)
    - Native Python debugging
    """
    script_dir = os.path.dirname(os.path.abspath(__file__))

    # Determine compose command
    compose_cmd = "podman-compose" if shutil.which("podman-compose") else "docker-compose"

    # Start services via compose (Python manages the process)
    result = subprocess.run(
        [compose_cmd, "-f", "docker-compose.workflow-catalog.yml", "-p", "hapi-integration", "up", "-d"],
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
    """
    Session-scoped fixture for infrastructure management.

    Equivalent to Go's SynchronizedBeforeSuite:
    - Runs once per test session
    - Ensures infrastructure is available
    - Fails tests if infrastructure not running (per TESTING_GUIDELINES.md)
    """
    if not is_integration_infra_available():
        pytest.fail(
            "REQUIRED: Infrastructure not running.\\n"
            "  Per TESTING_GUIDELINES.md: Tests MUST Fail, NEVER Skip\\n"
            "  Start it with: make test-integration-holmesgpt"
        )

    # Set environment variables for service clients
    os.environ["DATA_STORAGE_URL"] = DATA_STORAGE_URL
    os.environ["POSTGRES_HOST"] = "localhost"
    os.environ["POSTGRES_PORT"] = POSTGRES_PORT

    yield {
        "data_storage_url": DATA_STORAGE_URL,
        "postgres_host": "localhost",
        "postgres_port": POSTGRES_PORT,
    }

    # Cleanup handled by pytest_sessionfinish hook

@pytest.fixture(scope="session")
def data_storage_url(integration_infrastructure):
    """Fixture that provides Data Storage URL for integration tests."""
    return integration_infrastructure["data_storage_url"]
```

**Key Principles** (Same as Go Services):
1. ✅ **Framework manages infrastructure**: pytest fixtures (not shell scripts)
2. ✅ **Fail, don't skip**: Tests fail if infrastructure unavailable
3. ✅ **Session-scoped**: Infrastructure starts once per test session
4. ✅ **Explicit health checks**: Wait for services to be ready
5. ✅ **Environment variables**: Set for service clients

**Comparison with Go Services**:

| Aspect | Go (Ginkgo) | Python (pytest) |
|--------|-------------|-----------------|
| **Framework** | Ginkgo BeforeSuite | pytest fixtures |
| **Infrastructure** | `infrastructure.Start{Service}()` | `start_infrastructure()` |
| **Scope** | SynchronizedBeforeSuite | `@pytest.fixture(scope="session")` |
| **Failure Handling** | `Fail()` | `pytest.fail()` |
| **Health Checks** | `Eventually()` | `wait_for_infrastructure()` |

**Reference Implementation**: `holmesgpt-api/tests/integration/conftest.py`

---

## 🏗️ **DEPRECATED Pattern (v1.0 - Reference Only)**

<details>
<summary>Click to view deprecated podman-compose pattern (DO NOT USE)</summary>

```yaml
# ❌ DEPRECATED: This pattern is no longer supported
# Use programmatic Go setup instead (see above)

version: '3.8'
services:
  postgres:
    image: postgres:16-alpine
    # ... (omitted for brevity)

  datastorage:
    image: kubernaut/datastorage:latest
    build:
      context: ../../../
      dockerfile: cmd/datastorage/Dockerfile
    # ... (omitted for brevity)
```

**Why deprecated**: Race conditions, timing issues, lack of explicit health checks, no composite tags for collision avoidance.

</details>

---

## ⚡ **Build Optimization (v1.1 - Dec 2025)**

### **Parallel Image Builds (REQUIRED for Multi-Image Tests)**

When integration tests require **multiple custom images**, apply the same parallel build pattern as DD-E2E-001:

```yaml
# ❌ WRONG: Serial builds via podman-compose
# This blocks: datastorage builds → holmesgpt-api builds → controller builds
podman-compose up -d --build  # Each image builds sequentially

# ✅ CORRECT: Parallel builds before compose up
# Build images in parallel (shells or Go goroutines)
podman build -t img1:test -f Dockerfile1 . &
podman build -t img2:test -f Dockerfile2 . &
podman build -t img3:test -f Dockerfile3 . &
wait  # Wait for all builds

# Then start compose (images already built)
podman-compose up -d  # Fast startup - no builds needed
```

**Reference**: DD-E2E-001 §Implementation Pattern for goroutine-based parallel builds.

### **Layer Caching Optimization (RECOMMENDED)**

For faster iterative development, avoid `--no-cache`:

```yaml
# ❌ SLOW: Full rebuild every time (297+ seconds)
podman-compose up -d --build

# ✅ FASTER: Use layer cache for unchanged layers (~30-60 seconds)
podman-compose build  # Uses cached layers
podman-compose up -d
```

**When to use `--no-cache`**:
- CI/CD pipelines (ensure reproducibility)
- After changing base images or dependencies
- When debugging build issues

### **Pre-Built Image Strategy (OPTIONAL)**

For services that rarely change (e.g., DataStorage), consider pre-building:

```bash
# Build once and tag
podman build -t localhost/kubernaut-datastorage:integration \
    -f docker/data-storage.Dockerfile .

# Use pre-built image in compose (no build: section)
services:
  datastorage:
    image: localhost/kubernaut-datastorage:integration
    # No build: section - uses pre-built image
```

### **Timing Targets**

| Phase | Target | Optimization |
|-------|--------|--------------|
| **Image builds (single)** | <2 min | Layer caching |
| **Image builds (parallel)** | <4 min | Parallel builds (DD-E2E-001) |
| **Container startup** | <30 sec | Health check optimization |
| **Total SynchronizedBeforeSuite** | <5 min | Combined optimizations |

---

## 📊 **Comparison with E2E Tests**

### **E2E Tests** (DD-E2E-001)

**Pattern**: Build images in Go code (`test/infrastructure/*.go`)

```go
// E2E: Build via Go infrastructure code
func StartE2EInfrastructure() {
    // Build images using podman build
    buildImageOnly("Data Storage", "localhost/kubernaut-datastorage:latest", ...)
    buildImageOnly("Service", "localhost/kubernaut-service:latest", ...)

    // Load to Kind cluster
    loadImageToKind(...)

    // Deploy to Kubernetes
    deployManifests(...)
}
```

**Environment**: Kind Kubernetes cluster
**Build Tool**: `podman build` in Go code
**Image Prefix**: `localhost/`

### **Integration Tests** (DD-INTEGRATION-001)

**Pattern**: Build images via `podman-compose build`

```yaml
# Integration: Build via podman-compose.yml
services:
  datastorage:
    build:
      context: ../../../
      dockerfile: cmd/datastorage/Dockerfile
```

**Environment**: Podman containers (no Kubernetes)
**Build Tool**: `podman-compose build`
**Image Prefix**: `localhost/kubernaut/` (automatic)

### **Key Similarity** ✅

**Both E2E and Integration tests build images locally** - this is the established pattern.

---

## 🚀 **Migration Guide**

### **Step 1: Identify Broken podman-compose.yml Files**

```bash
# Find all podman-compose.yml files
find test/integration -name "podman-compose.yml"

# Check for commented build sections
grep -A 5 "# build:" test/integration/*/podman-compose.yml
```

### **Step 2: Uncomment and Verify Build Sections**

For each `podman-compose.yml`:

```bash
# 1. Edit file to uncomment build sections
vim test/integration/{service}/podman-compose.yml

# 2. Verify Dockerfile paths
ls -la cmd/{service}/Dockerfile

# 3. Test build
cd test/integration/{service}/
podman-compose build
```

### **Step 3: Update Makefile Target** (if needed)

```makefile
.PHONY: test-integration-{service}
test-integration-{service}: ## Run {service} integration tests
	@echo "Building images..."
	cd test/integration/{service} && podman-compose build
	@echo "Running tests..."
	ginkgo -v --procs=4 ./test/integration/{service}/...
	@echo "Cleaning up..."
	cd test/integration/{service} && podman-compose down
```

### **Step 4: Test and Validate**

```bash
# Run full integration test suite
make test-integration-{service}

# Verify images were built
podman images | grep kubernaut/{service}
```

---

## 📋 **Checklist for New Services**

When adding integration tests for a new service:

- [ ] Create `test/integration/{service}/podman-compose.yml`
- [ ] Include `build` sections for all custom images
- [ ] Verify Dockerfile paths are correct
- [ ] Use unique ports per DD-TEST-001
- [ ] Use unique network names per DD-TEST-004
- [ ] Test `podman-compose build` works
- [ ] Add Makefile target for test execution
- [ ] Document in service README

---

## 🔧 **Technical Details**

### **Image Naming Convention**

**Integration Tests (v2.0)** - Per DD-TEST-001 v1.3:

For **shared infrastructure images** (DataStorage, PostgreSQL, Redis):
```
localhost/{infrastructure}:{consumer}-{uuid}
```

**Examples**:
```
localhost/datastorage:workflowexecution-1884d074
localhost/datastorage:signalprocessing-a5f3c2e9
localhost/datastorage:gateway-7b8d9f12
```

**Note**: The format above is what gets stored by Podman. In code, use:
```go
dsImage := GenerateInfraImageName("datastorage", "workflowexecution")
// Returns: "localhost/datastorage:workflowexecution-{8-char-hex-uuid}"
```

**v1.0 Format (DEPRECATED)**:
```
❌ localhost/kubernaut/{service}:latest  (No longer used)
```

### **Build Context**

**Relative to podman-compose.yml location**:
```yaml
build:
  context: ../../../          # Go to repository root
  dockerfile: cmd/{service}/Dockerfile
```

**Why**: Dockerfiles need access to Go modules and shared code at repository root.

### **Build Performance**

| Image | Build Time | Size |
|---|---|---|
| DataStorage | ~30 seconds | 134 MB |
| HolmesGPT-API | ~2 minutes | 2.8 GB |
| Service Controller | ~20 seconds | ~100 MB |

**Total integration setup**: ~3 minutes (much faster than E2E's ~12 minutes)

---

## 📊 **Consequences**

### **Positive** ✅

1. ✅ **Self-Contained**: No external dependencies
2. ✅ **Reproducible**: Fresh builds guarantee consistency
3. ✅ **Fast**: Local builds faster than registry pulls
4. ✅ **Consistent**: Matches E2E test pattern (DD-E2E-001)
5. ✅ **Debuggable**: Easy to test specific code changes

### **Negative** ⚠️

1. ⚠️ **Build Time**: Adds ~3 minutes to test execution
   - **Mitigation**: Faster than E2E, acceptable for integration tier

2. ⚠️ **Disk Space**: Builds consume ~3-4 GB per service
   - **Mitigation**: Cleanup after tests, same as E2E pattern

3. ⚠️ **Dockerfile Maintenance**: Must keep paths correct
   - **Mitigation**: CI validation, verification checklist

---

## 🔗 **Related Documents**

### **Architecture Decisions**
- **DD-E2E-001**: Parallel Image Builds for E2E Testing (similar pattern)
- **DD-TEST-001**: Unique Container Image Tags (tagging strategy)
- **DD-TEST-004**: Unique Resource Naming Strategy (naming conventions)

### **Service Documentation**
- `test/integration/{service}/README.md`: Service-specific integration test docs

---

## ✅ **Compliance Requirements (v2.0)**

### **Mandatory (MUST)**

1. ✅ All integration tests MUST use programmatic Go setup (NOT `podman-compose`)
2. ✅ All services MUST use `test/infrastructure/{service}_integration.go`
3. ✅ All images MUST use composite tags: `{service}-{uuid}`
4. ✅ All infrastructure MUST use shared utilities from `shared_integration_utils.go`
5. ✅ All services MUST follow DD-TEST-002 sequential startup pattern
6. ✅ All services MUST use unique ports per DD-TEST-001
7. ✅ All integration tests MUST build images locally (no registry pulls)

### **Recommended (SHOULD)**

1. ✅ Services SHOULD document programmatic infrastructure in README
2. ✅ Makefile targets SHOULD reference Go-managed infrastructure
3. ✅ CI/CD pipelines SHOULD verify programmatic setup works
4. ✅ Old `podman-compose.yml` files SHOULD be marked deprecated or removed

### **Deprecated (MUST NOT)**

1. ❌ Services MUST NOT use `podman-compose` for new infrastructure
2. ❌ Services MUST NOT use shell scripts for container management
3. ❌ Services MUST NOT use simple image tags without UUIDs

---

## 📞 **Support and Questions**

**Questions**: Contact Platform Team or open issue in GitHub

**Migration Support**: Platform team available for pairing on service migration

**Troubleshooting**: See service README or platform team documentation

---

## 📜 **Changelog**

### **v2.0 - December 26, 2025** (CURRENT)

**Major Architecture Change**: Migration from `podman-compose` to programmatic Go setup

**Breaking Changes**:
- ❌ **DEPRECATED**: `podman-compose` pattern no longer recommended
- ❌ **REMOVED**: Shell script infrastructure management
- ✅ **REQUIRED**: Programmatic Go setup via `test/infrastructure/{service}_integration.go`
- ✅ **REQUIRED**: Composite image tags `{service}-{uuid}` for collision avoidance
- ✅ **REQUIRED**: Shared utilities from `shared_integration_utils.go`

**New Features**:
- ✅ Composite image tagging prevents parallel test collisions
- ✅ Shared utilities reduce code duplication (~720 lines saved)
- ✅ Explicit sequential startup with health checks (DD-TEST-002)
- ✅ Custom network support for internal service DNS
- ✅ Programmatic cleanup guarantees no orphaned containers

**Migration Status**:
- ✅ Notification - Migrated (built with v2.0 from day 1, Go pattern)
- ✅ Gateway - Migrated (92 lines saved, Go pattern)
- ✅ RemediationOrchestrator - Migrated (67 lines saved, Go pattern)
- ✅ WorkflowExecution - Migrated (88 lines saved, Go pattern)
- ✅ SignalProcessing - Migrated (~80 lines saved, Go pattern)
- ✅ AIAnalysis - Migrated (~85 lines saved, Go pattern)
- ✅ HolmesGPT-API - Migrated (Dec 27, 2025, Python pytest fixtures pattern, 358 lines removed)
- ⏳ DataStorage - Migration pending

**Related Design Decisions**:
- DD-TEST-002: Sequential startup pattern and container orchestration
- DD-TEST-001: Unique port allocation for parallel tests

**Impact**:
- **Reliability**: ↑↑ Eliminated race conditions and timing issues
- **Maintainability**: ↑↑ Centralized utilities, reduced duplication
- **Test Isolation**: ↑↑ Composite tags prevent collisions
- **Developer Experience**: ↑ Explicit health checks, better debugging

**Deprecation Timeline**:
- **December 26, 2025**: v2.0 published, v1.0 marked deprecated
- **January 15, 2026**: All services must be migrated
- **February 1, 2026**: `podman-compose` support removed from CI/CD

---

### **v1.0 - December 16, 2025** (DEPRECATED)

**Initial Version**: Local image builds via `podman-compose`

**Pattern**: Build images using `podman-compose.yml` with active `build` sections

**Status**: ⚠️ **DEPRECATED** - Superseded by v2.0 programmatic Go setup

**Known Issues** (led to v2.0):
- Race conditions in `podman-compose up -d --build`
- No explicit health checks between service startups
- Image tag collisions during parallel test runs
- Duplicated infrastructure code across services
- Limited debugging capabilities for startup failures

---

**Document Version**: 2.0
**Last Updated**: December 26, 2025
**Next Review**: March 26, 2026 (3 months post-v2.0)


