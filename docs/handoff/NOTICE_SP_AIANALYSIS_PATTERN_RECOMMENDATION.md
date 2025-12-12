# NOTICE: SP Team - AIAnalysis Pattern Recommendation

**From**: RemediationOrchestrator Service Team
**To**: SignalProcessing Service Team
**Date**: 2025-12-12
**Priority**: 🟡 **MEDIUM** - Infrastructure improvement opportunity
**Type**: Cross-Service Coordination

---

## 📋 **Summary**

**Recommendation**: SignalProcessing team should adopt the **AIAnalysis Pattern** for integration test infrastructure

**Current SP Approach**: ❌ NOT parallel-safe
- Uses `BeforeSuite` (runs sequentially, not `SynchronizedBeforeSuite`)
- Direct `podman run` commands (manual container management)
- Dynamic port allocation (complex)

**Recommended Approach**: ✅ AIAnalysis Pattern (parallel-safe)
- Uses `SynchronizedBeforeSuite` (Ginkgo parallel execution support)
- Programmatic `podman-compose` management (simpler, more reliable)
- Fixed ports per service (per DD-TEST-001)

**Authority**: `docs/handoff/TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md`

---

## 🎯 **Why This Recommendation**

### **Benefits of AIAnalysis Pattern**:

1. ✅ **Parallel-Safe**: `SynchronizedBeforeSuite` enables `ginkgo -p --procs=4`
2. ✅ **Simpler**: Programmatic podman-compose vs manual `podman run` commands
3. ✅ **Health Checks**: HTTP endpoint validation ensures full stack readiness
4. ✅ **Consistency**: Same pattern across AI, RO, and eventually SP teams
5. ✅ **Proven**: AIAnalysis team has validated this approach successfully

### **Current SP Limitations**:

| Issue | Current Approach | AIAnalysis Pattern |
|-------|-----------------|-------------------|
| **Parallel Execution** | ❌ NOT supported (`BeforeSuite`) | ✅ Supported (`SynchronizedBeforeSuite`) |
| **Infrastructure Management** | ⚠️ Manual (`podman run` per service) | ✅ Declarative (`podman-compose`) |
| **Port Management** | ⚠️ Dynamic (FindAvailablePort) | ✅ Fixed (per DD-TEST-001) |
| **Health Validation** | ⚠️ Manual (waitForPostgresReady) | ✅ HTTP endpoints |
| **Code Complexity** | 🔴 High (custom container helpers) | 🟢 Low (reuse podman-compose) |

---

## 📊 **Pattern Comparison**

### **Current SP Pattern** (Direct Podman):

```go
// test/integration/signalprocessing/suite_test.go
var _ = BeforeSuite(func() {  // ❌ NOT parallel-safe
    By("Setting up infrastructure for BR-SP-090 audit testing")

    // Start PostgreSQL container for audit storage
    pgClient = SetupPostgresTestClient(ctx)  // Manual container creation
    Expect(pgClient).ToNot(BeNil())

    // Apply audit migrations
    err = ApplyAuditMigrations(pgClient)
    Expect(err).ToNot(HaveOccurred())

    // Start Redis container for DataStorage DLQ
    redisClient = SetupRedisTestClient(ctx)  // Manual container creation
    Expect(redisClient).ToNot(BeNil())

    // Start DataStorage service for audit API
    dataStorageServer = SetupDataStorageTestServer(ctx, pgClient, redisClient)
    Expect(dataStorageServer).ToNot(BeNil())
})
```

**Issues**:
- ❌ `BeforeSuite` runs sequentially (not parallel-safe)
- ⚠️ Manual container management (complex)
- ⚠️ Dynamic port allocation (potential conflicts)

---

### **Recommended Pattern** (AIAnalysis):

```go
// Pattern: AIAnalysis (Programmatic podman-compose)
var _ = SynchronizedBeforeSuite(func() []byte {  // ✅ Parallel-safe
    // Process 1 ONLY - creates shared infrastructure

    By("Starting SP integration infrastructure (podman-compose)")
    err := infrastructure.StartSPIntegrationInfrastructure(GinkgoWriter)
    Expect(err).ToNot(HaveOccurred())

    // Starts: PostgreSQL, Redis, DataStorage
    // Per DD-TEST-001: SP-specific ports (e.g., 15436, 16382, 18142)
    // Health checks validate full stack

    By("Bootstrapping envtest")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    }
    cfg, err = testEnv.Start()

    // Serialize REST config for ALL processes
    configBytes, err := json.Marshal(struct {
        Host     string
        CAData   []byte
        CertData []byte
        KeyData  []byte
    }{...})

    return configBytes
}, func(data []byte) {
    // ALL processes - initialize per-process state

    // Deserialize REST config from Process 1
    var configData struct {...}
    err := json.Unmarshal(data, &configData)

    // Create per-process REST config
    cfg = &rest.Config{...}

    // Create per-process k8s client
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
})
```

**Benefits**:
- ✅ `SynchronizedBeforeSuite` enables parallel execution
- ✅ Programmatic podman-compose (simpler)
- ✅ Fixed SP-specific ports (per DD-TEST-001)
- ✅ HTTP health checks validate full stack

---

## 🔧 **Implementation Guide**

### **Step 1: Create `podman-compose.signalprocessing.test.yml`**

**File**: `test/integration/signalprocessing/podman-compose.signalprocessing.test.yml`

**Port Allocation** (per DD-TEST-001, sequential after RO):

| Service | Port | Range | Notes |
|---------|------|-------|-------|
| PostgreSQL | 15436 | 15433-15442 | SP-specific, after RO (15435) |
| Redis | 16382 | 16379-16388 | SP-specific, after RO (16381) |
| DataStorage API | 18142 | After stateless | SP-specific |
| DS Metrics | 18143 | - | SP-specific |

**Example**:

```yaml
version: '3.8'

# SignalProcessing Integration Test Infrastructure
# SP-specific ports per DD-TEST-001: Port Allocation Strategy
#
# Port Allocation (from DD-TEST-001 documented ranges):
#   PostgreSQL:       15436 (from range 15433-15442, after RO)
#   Redis:            16382 (from range 16379-16388, after RO)
#   Data Storage API: 18142 (after stateless services + RO)
#   DS Metrics:       18143

services:
  migrate:
    image: ghcr.io/pressly/goose:3.18.0
    volumes:
      - ../../../migrations:/migrations:ro
    environment:
      - GOOSE_DRIVER=postgres
      - GOOSE_DBSTRING=postgres://slm_user:test_password@postgres:5432/action_history?sslmode=disable
    command: ["-dir", "/migrations", "up"]
    networks:
      - sp-test-network
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    container_name: sp-postgres-integration
    environment:
      POSTGRES_DB: action_history
      POSTGRES_USER: slm_user
      POSTGRES_PASSWORD: test_password
    ports:
      - "15436:5432"
    networks:
      - sp-test-network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U slm_user -d action_history"]
      interval: 5s
      timeout: 5s
      retries: 10

  redis:
    image: quay.io/jordigilh/redis:7-alpine
    container_name: sp-redis-integration
    ports:
      - "16382:6379"
    networks:
      - sp-test-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 10

  datastorage:
    build:
      context: ../../..
      dockerfile: docker/data-storage.Dockerfile
    container_name: sp-datastorage-integration
    environment:
      - CONFIG_PATH=/etc/datastorage/config.yaml
    ports:
      - "18142:8080"  # Data Storage HTTP API
      - "18143:9090"  # Metrics
    volumes:
      - ./config:/etc/datastorage:ro
    networks:
      - sp-test-network
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

networks:
  sp-test-network:
    driver: bridge
```

---

### **Step 2: Create Infrastructure Functions**

**File**: `test/infrastructure/signalprocessing.go` (or add to existing)

```go
const (
    SPIntegrationPostgresPort           = 15436
    SPIntegrationRedisPort              = 16382
    SPIntegrationDataStoragePort        = 18142
    SPIntegrationDataStorageMetricsPort = 18143

    SPIntegrationComposeProject = "signalprocessing-integration"
    SPIntegrationComposeFile    = "test/integration/signalprocessing/podman-compose.signalprocessing.test.yml"
)

func StartSPIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    composeFile := filepath.Join(projectRoot, SPIntegrationComposeFile)

    cmd := exec.Command("podman-compose",
        "-f", composeFile,
        "-p", SPIntegrationComposeProject,
        "up", "-d", "--build",
    )
    cmd.Dir = projectRoot
    cmd.Stdout = writer
    cmd.Stderr = writer

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start podman-compose stack: %w", err)
    }

    // Wait for DataStorage health
    if err := waitForHTTPHealth(
        fmt.Sprintf("http://localhost:%d/health", SPIntegrationDataStoragePort),
        90*time.Second,
    ); err != nil {
        return fmt.Errorf("DataStorage failed to become healthy: %w", err)
    }

    return nil
}

func StopSPIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    composeFile := filepath.Join(projectRoot, SPIntegrationComposeFile)

    cmd := exec.Command("podman-compose",
        "-f", composeFile,
        "-p", SPIntegrationComposeProject,
        "down", "-v",
    )
    cmd.Dir = projectRoot
    cmd.Stdout = writer
    cmd.Stderr = writer

    return cmd.Run()
}
```

---

### **Step 3: Update `suite_test.go`**

**Changes Required**:

1. Change `BeforeSuite` → `SynchronizedBeforeSuite`
2. Add infrastructure startup call
3. Add REST config serialization for parallel processes
4. Add `SynchronizedAfterSuite` for cleanup
5. Add `encoding/json` import

**See**: `test/integration/remediationorchestrator/suite_test.go` for full example

---

## ✅ **Expected Benefits**

### **Performance**:
- ✅ Parallel test execution: `ginkgo -p --procs=4`
- ✅ Faster test runs (4x potential speedup)

### **Reliability**:
- ✅ Declarative infrastructure (podman-compose YAML)
- ✅ Health checks validate full stack readiness
- ✅ No port conflicts (fixed SP-specific ports)

### **Maintainability**:
- ✅ Less code (podman-compose vs manual container helpers)
- ✅ Consistent pattern across AI, RO, SP teams
- ✅ Easier onboarding (standard Ginkgo pattern)

---

## 📚 **Reference Documents**

| Document | Purpose | Location |
|----------|---------|----------|
| **AIAnalysis Pattern** | Original implementation | `test/integration/aianalysis/suite_test.go` |
| **RO Implementation** | Recent adoption (2025-12-12) | `test/integration/remediationorchestrator/suite_test.go` |
| **Infrastructure Comparison** | Pattern analysis | `docs/handoff/TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md` |
| **DD-TEST-001** | Port allocation strategy | `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` |
| **TESTING_GUIDELINES.md** | BeforeSuite automation mandate | `docs/development/business-requirements/TESTING_GUIDELINES.md` |

---

## 📞 **Support & Coordination**

### **AIAnalysis Team** (Original Pattern Authors):
- ✅ Pattern proven to work with podman-compose
- ✅ Available for questions on implementation
- ✅ `test/integration/aianalysis/suite_test.go` is reference implementation

### **RO Team** (Recently Adopted):
- ✅ Successfully migrated from manual approach to AIAnalysis pattern (2025-12-12)
- ✅ Available for migration assistance
- ✅ `test/integration/remediationorchestrator/suite_test.go` is recent example

### **Contact**:
- **AIAnalysis Team**: `#aianalysis-service` (Slack)
- **RO Team**: `#remediation-orchestrator` (Slack)

---

## 🎯 **Recommendation**

**Action**: SP team should triage and plan migration to AIAnalysis pattern

**Timeline**: At SP team's convenience (not blocking)

**Priority**: 🟡 MEDIUM
- Not urgent (current approach works)
- High value (parallel execution + maintainability)
- Future-proof (consistent with other teams)

**Decision**: SP team's choice, but RO team recommends adoption for consistency and parallel execution support

---

## ✅ **Approval & Sign-Off**

### **RO Team** (Sender):
- [x] **Pattern Validated**: RO successfully adopted AIAnalysis pattern (2025-12-12)
- [x] **Notification Created**: This document
- [x] **Available for Support**: Yes

### **SP Team** (Recipient):
- [ ] **Notification Received**: _Pending SP team acknowledgment_
- [ ] **Triage Scheduled**: _Pending SP team review_
- [ ] **Decision**: _Pending SP team decision (adopt/defer/alternative)_

---

**Document Status**: ✅ Active
**Created**: 2025-12-12
**Next Action**: SP team triage and decision
