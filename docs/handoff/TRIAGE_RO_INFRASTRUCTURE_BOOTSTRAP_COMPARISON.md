# TRIAGE: RO Infrastructure Bootstrap - Cross-Service Comparison

**Date**: 2025-12-12
**Team**: RemediationOrchestrator
**Priority**: 🔴 **HIGH** - Infrastructure blocking integration tests
**Status**: 📊 **ANALYSIS COMPLETE**

---

## 📋 **Context**

**Problem**: RO's dedicated `podman-compose` infrastructure fails to start with "Command failed to spawn: Aborted"

**Goal**: Understand how other CRD controller services (AI, WE, SP, GW) bootstrap their integration test infrastructure

**Authority**:
- `docs/development/business-requirements/TESTING_GUIDELINES.md` (BeforeSuite automation)
- `DD-TEST-001-port-allocation-strategy.md` (Port allocation)
- `ADR-016` (Service-specific integration test infrastructure)

---

## 🔍 **Cross-Service Infrastructure Patterns**

### **Pattern 1: Programmatic podman-compose (AIAnalysis)**

**File**: `test/integration/aianalysis/suite_test.go`

**Approach**: `SynchronizedBeforeSuite` with full programmatic management

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Process 1 ONLY - creates shared infrastructure
    err := infrastructure.StartAIAnalysisIntegrationInfrastructure(GinkgoWriter)
    Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")

    // Starts: PostgreSQL, Redis, DataStorage, HolmesGPT-API
    // Per DD-TEST-001: Ports 15434, 16380, 18091, 18120

    return configBytes
}, func(data []byte) {
    // ALL processes - initialize per-process state
    suiteK8sClient = SetupK8sTestClient(suiteCtx)
})
```

**Infrastructure Function** (`test/infrastructure/aianalysis.go`):

```go
func StartAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    composeFile := filepath.Join(projectRoot, AIAnalysisIntegrationComposeFile)

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
    waitForHTTPHealth("http://localhost:18091/health", 60*time.Second)
    waitForHTTPHealth("http://localhost:18120/health", 60*time.Second)

    return nil
}
```

**Ports** (per DD-TEST-001):
- PostgreSQL: 15434
- Redis: 16380
- DataStorage API: 18091
- HolmesGPT API: 18120

**Cleanup**:
```go
var _ = SynchronizedAfterSuite(func() {
    // Process-specific cleanup
}, func() {
    // Process 1 ONLY - cleanup shared infrastructure
    infrastructure.StopAIAnalysisIntegrationInfrastructure(GinkgoWriter)
})
```

**Key Features**:
- ✅ `SynchronizedBeforeSuite` for parallel execution
- ✅ Programmatic `podman-compose up -d --build`
- ✅ Health checks via HTTP endpoints
- ✅ Complete teardown in `SynchronizedAfterSuite`

---

### **Pattern 2: Direct Podman Container Management (SignalProcessing)**

**File**: `test/integration/signalprocessing/suite_test.go`

**Approach**: `BeforeSuite` (NOT synchronized) with direct container creation

```go
var _ = BeforeSuite(func() {
    By("Setting up infrastructure for BR-SP-090 audit testing")

    // Start PostgreSQL container for audit storage
    pgClient = SetupPostgresTestClient(ctx)
    Expect(pgClient).ToNot(BeNil())

    // Apply audit migrations
    err = ApplyAuditMigrations(pgClient)
    Expect(err).ToNot(HaveOccurred())

    // Start Redis container for DataStorage DLQ
    redisClient = SetupRedisTestClient(ctx)
    Expect(redisClient).ToNot(BeNil())

    // Start DataStorage service for audit API
    dataStorageServer = SetupDataStorageTestServer(ctx, pgClient, redisClient)
    Expect(dataStorageServer).ToNot(BeNil())

    // Create audit store (BufferedStore pattern per ADR-038)
    dsClient := audit.NewHTTPDataStorageClient(dataStorageServer.BaseURL, nil)
    auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "signalprocessing", logger)
    Expect(err).ToNot(HaveOccurred())
})
```

**Infrastructure Functions** (`test/integration/signalprocessing/helpers_infrastructure.go`):

```go
func SetupPostgresTestClient(ctx context.Context) *PostgresTestClient {
    // Find available port
    port, err := FindAvailablePort(15400, 15500)

    // Create Podman container directly
    cmd := exec.Command("podman", "run", "-d",
        "--name", containerName,
        "-e", "POSTGRES_USER=slm_user",
        "-e", "POSTGRES_PASSWORD=test_password",
        "-e", "POSTGRES_DB=action_history",
        "-p", fmt.Sprintf("%d:5432", port),
        "postgres:16-alpine",
    )

    // Wait for health
    waitForPostgresReady(port, 60*time.Second)

    return &PostgresTestClient{Port: port, ContainerName: containerName}
}
```

**Key Features**:
- ✅ Direct `podman run` commands (no podman-compose)
- ✅ Dynamic port allocation (FindAvailablePort)
- ✅ Per-service health checks
- ✅ Test-scoped cleanup

**⚠️ Limitation**: NOT parallel-safe (uses `BeforeSuite`, not `SynchronizedBeforeSuite`)

---

### **Pattern 3: envtest Only (Gateway)**

**File**: `test/integration/gateway/suite_test.go`

**Approach**: `SynchronizedBeforeSuite` with envtest + minimal external services

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // 1. Start envtest (in-memory K8s API server)
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{"../../../config/crd/bases"},
    }
    k8sConfig, err = testEnv.Start()

    // 2. Start PostgreSQL container
    suitePgClient = SetupPostgresTestClient(ctx)

    // 3. Start Data Storage service
    suiteDataStorage = SetupDataStorageTestServer(ctx, suitePgClient)

    // DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free

    return configBytes
}, func(data []byte) {
    // ALL processes - initialize per-process K8s client
    k8sConfig, err = clientcmd.RESTConfigFromKubeConfig(sharedConfig.Kubeconfig)
    suiteK8sClient = SetupK8sTestClient(suiteCtx)
})
```

**Key Features**:
- ✅ `SynchronizedBeforeSuite` for parallel execution
- ✅ envtest (in-memory K8s) instead of Kind
- ✅ Minimal external services (Postgres + DataStorage only)
- ✅ NO Redis (DD-GATEWAY-012: Gateway moved to K8s-native deduplication)

**⚠️ Note**: Gateway migrated from Redis to K8s CRD status fields (DD-GATEWAY-011)

---

### **Pattern 4: envtest Only, No External Services (WorkflowExecution)**

**File**: `test/integration/workflowexecution/suite_test.go`

**Approach**: `BeforeSuite` with envtest only - NO external services

```go
var _ = BeforeSuite(func() {
    By("Registering CRD schemes")
    err := workflowexecutionv1alpha1.AddToScheme(scheme.Scheme)
    err = tektonv1.AddToScheme(scheme.Scheme)

    By("Bootstrapping test environment with WorkflowExecution AND Tekton CRDs")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
            filepath.Join("..", "..", "..", "config", "crd", "tekton"), // Tekton CRDs
        },
    }

    cfg, err = testEnv.Start()
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})

    By("Setting up the testable audit store")
    testAuditStore = newTestableAuditStore() // In-memory mock
})
```

**Key Features**:
- ✅ Pure envtest (no external services)
- ✅ In-memory audit store mock
- ✅ Tekton CRDs for PipelineRun integration
- ✅ Simplest approach (no containers)

**⚠️ Limitation**: NOT parallel-safe (uses `BeforeSuite`, not `SynchronizedBeforeSuite`)

---

## 📊 **Pattern Comparison Matrix**

| Pattern | Service | podman-compose | External Services | Parallel-Safe | Complexity |
|---------|---------|----------------|-------------------|---------------|------------|
| **Programmatic Compose** | AIAnalysis | ✅ Yes (programmatic) | Postgres, Redis, DS, HAPI | ✅ Yes (Synchronized) | 🔴 High |
| **Direct Podman** | SignalProcessing | ❌ No (direct `podman run`) | Postgres, Redis, DS | ❌ No (BeforeSuite) | 🟡 Medium |
| **envtest + Services** | Gateway | ❌ No | Postgres, DS | ✅ Yes (Synchronized) | 🟡 Medium |
| **envtest Only** | WorkflowExecution | ❌ No | None (mocks) | ❌ No (BeforeSuite) | 🟢 Low |
| **RO (Current)** | RemediationOrchestrator | ✅ Yes (attempting) | Postgres, Redis, DS | ❌ No (BeforeSuite) | 🔴 High |

---

## 🎯 **Recommended Approach for RO**

### **Option A: Follow AIAnalysis Pattern** (Recommended)

**Why AIAnalysis Pattern Works**:
1. ✅ Proven to work with podman-compose
2. ✅ Parallel-safe (`SynchronizedBeforeSuite`)
3. ✅ Health checks via HTTP endpoints
4. ✅ Clean separation: Process 1 creates infra, ALL processes create per-process clients
5. ✅ Automatic cleanup in `SynchronizedAfterSuite`

**Implementation**:

```go
// test/integration/remediationorchestrator/suite_test.go
var _ = SynchronizedBeforeSuite(func() []byte {
    // Process 1 ONLY - creates shared infrastructure

    By("Starting RO integration infrastructure (podman-compose)")
    err := infrastructure.StartROIntegrationInfrastructure(GinkgoWriter)
    Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully")

    // Starts: PostgreSQL (15435), Redis (16381), DataStorage (18140)

    By("Bootstrapping envtest")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    }
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    // Serialize REST config to pass to all processes
    configBytes, err := json.Marshal(struct {
        Host     string
        CAData   []byte
        CertData []byte
        KeyData  []byte
    }{
        Host:     cfg.Host,
        CAData:   cfg.CAData,
        CertData: cfg.CertData,
        KeyData:  cfg.KeyData,
    })
    Expect(err).NotTo(HaveOccurred())

    return configBytes
}, func(data []byte) {
    // ALL processes - initialize per-process state

    // Deserialize REST config from process 1
    var configData struct {
        Host     string
        CAData   []byte
        CertData []byte
        KeyData  []byte
    }
    err := json.Unmarshal(data, &configData)
    Expect(err).NotTo(HaveOccurred())

    // Create per-process REST config
    cfg = &rest.Config{
        Host: configData.Host,
        TLSClientConfig: rest.TLSClientConfig{
            CAData:   configData.CAData,
            CertData: configData.CertData,
            KeyData:  configData.KeyData,
        },
    }

    // Create per-process k8s client
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())

    ctx, cancel = context.WithCancel(context.Background())
})

var _ = SynchronizedAfterSuite(func() {
    // ALL processes - per-process cleanup
    cancel()
}, func() {
    // Process 1 ONLY - cleanup shared infrastructure

    By("Stopping RO integration infrastructure")
    err := infrastructure.StopROIntegrationInfrastructure(GinkgoWriter)
    Expect(err).NotTo(HaveOccurred())

    By("Tearing down envtest")
    if testEnv != nil {
        err := testEnv.Stop()
        Expect(err).NotTo(HaveOccurred())
    }
})
```

**Infrastructure Function** (`test/infrastructure/remediationorchestrator.go`):

```go
// StartROIntegrationInfrastructure starts the podman-compose stack for RO integration tests
func StartROIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    composeFile := filepath.Join(projectRoot,
        "test/integration/remediationorchestrator/podman-compose.remediationorchestrator.test.yml")

    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "Starting RO Integration Test Infrastructure\n")
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "  PostgreSQL:     localhost:15435\n")
    fmt.Fprintf(writer, "  Redis:          localhost:16381\n")
    fmt.Fprintf(writer, "  DataStorage:    http://localhost:18140\n")
    fmt.Fprintf(writer, "  Compose File:   %s\n", composeFile)
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

    // Check if podman-compose is available
    if err := exec.Command("podman-compose", "--version").Run(); err != nil {
        return fmt.Errorf("podman-compose not found: %w", err)
    }

    // Start services
    cmd := exec.Command("podman-compose",
        "-f", composeFile,
        "-p", "remediationorchestrator-integration",
        "up", "-d", "--build",
    )
    cmd.Dir = projectRoot
    cmd.Stdout = writer
    cmd.Stderr = writer

    fmt.Fprintf(writer, "⏳ Starting containers...\n")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to start podman-compose stack: %w", err)
    }

    // Wait for services to be healthy
    fmt.Fprintf(writer, "⏳ Waiting for services to be healthy...\n")

    // Wait for DataStorage
    if err := waitForHTTPHealth(
        "http://localhost:18140/health",
        60*time.Second,
    ); err != nil {
        return fmt.Errorf("DataStorage failed to become healthy: %w", err)
    }
    fmt.Fprintf(writer, "✅ DataStorage is healthy\n")

    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Fprintf(writer, "✅ RO Integration Infrastructure Ready\n")
    fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

    return nil
}

// StopROIntegrationInfrastructure stops and cleans up the RO integration test infrastructure
func StopROIntegrationInfrastructure(writer io.Writer) error {
    projectRoot := getProjectRoot()
    composeFile := filepath.Join(projectRoot,
        "test/integration/remediationorchestrator/podman-compose.remediationorchestrator.test.yml")

    fmt.Fprintf(writer, "🛑 Stopping RO Integration Infrastructure...\n")

    cmd := exec.Command("podman-compose",
        "-f", composeFile,
        "-p", "remediationorchestrator-integration",
        "down", "-v",
    )
    cmd.Dir = projectRoot
    cmd.Stdout = writer
    cmd.Stderr = writer

    if err := cmd.Run(); err != nil {
        fmt.Fprintf(writer, "⚠️  Warning: Error stopping infrastructure: %v\n", err)
        return err
    }

    fmt.Fprintf(writer, "✅ RO Integration Infrastructure stopped and cleaned up\n")
    return nil
}

// waitForHTTPHealth waits for an HTTP health endpoint to respond with 200 OK
func waitForHTTPHealth(healthURL string, timeout time.Duration) error {
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
        time.Sleep(2 * time.Second)
    }

    return fmt.Errorf("timeout waiting for health endpoint: %s", healthURL)
}
```

---

### **Option B: Follow SignalProcessing Pattern** (Alternative)

**Why SignalProcessing Pattern Might Work**:
1. ✅ No podman-compose dependency
2. ✅ Direct container control
3. ✅ Dynamic port allocation (no conflicts)

**⚠️ Limitations**:
- ❌ NOT parallel-safe (needs `Synchronized BeforeSuite`)
- ❌ More complex (manual container management)
- ❌ No health checks via podman-compose

**Skip**: Not recommended for RO (parallel execution needed)

---

### **Option C: Follow Gateway Pattern** (Alternative)

**Why Gateway Pattern Might Work**:
1. ✅ Parallel-safe (`SynchronizedBeforeSuite`)
2. ✅ Simpler than podman-compose (direct container control)
3. ✅ Minimal external services

**⚠️ Considerations**:
- ⚠️ Still uses direct `podman run` commands (not podman-compose)
- ⚠️ Requires manual container management
- ⚠️ More code duplication vs. podman-compose

**Verdict**: Less elegant than AIAnalysis pattern, but viable if podman-compose remains problematic

---

## 🚨 **Root Cause Analysis: Why RO's podman-compose Failed**

### **Investigation Findings**:

1. **Partial Container State**: Postgres and Redis were running, but DataStorage never started
2. **No Migrate Service**: The `migrate` service (required by DataStorage) was not found
3. **Aborted Command**: `podman-compose up -d` aborted mid-execution

### **Likely Causes**:

**Hypothesis A: DataStorage Image Not Available**
```bash
# Check if image exists
$ podman images | grep datastorage
# If missing → build fails → podman-compose aborts
```

**Hypothesis B: Volume Mount Issues**
```yaml
volumes:
  - ./config:/etc/datastorage:ro  # Path may not exist
```

**Hypothesis C: Migrate Service Definition Issues**
```yaml
migrate:
  image: ghcr.io/pressly/goose:3.18.0
  # If this image is unavailable → service fails → podman-compose aborts
```

---

## ✅ **Recommended Action Plan**

### **Immediate Steps** (Today):

1. **Implement AIAnalysis Pattern**:
   - Create `test/infrastructure/remediationorchestrator.go` with programmatic functions
   - Update `test/integration/remediationorchestrator/suite_test.go` to use `SynchronizedBeforeSuite`
   - Add health check functions (`waitForHTTPHealth`)

2. **Debug podman-compose** (if Option A still fails):
   - Run `podman-compose up` manually (not `-d`) to see full output
   - Check `podman images` for missing DataStorage image
   - Verify config file mount paths exist
   - Check if `goose` migration image is accessible

3. **Fallback to Gateway Pattern** (if podman-compose unusable):
   - Use direct `podman run` commands like Gateway/SP
   - Skip `migrate` service (apply migrations manually in test setup)
   - Simplify to just Postgres + Redis + DataStorage containers

---

## 📚 **Reference Documents**

| Document | Purpose | Status |
|----------|---------|--------|
| **TESTING_GUIDELINES.md** | BeforeSuite automation mandate | ✅ Authoritative |
| **DD-TEST-001** | Port allocation strategy | ✅ Authoritative |
| **ADR-016** | Service-specific integration infrastructure | ✅ Authoritative |
| **ADR-030** | Configuration management | ✅ Authoritative |

---

## 🎯 **Success Criteria**

This triage is successful when:
- ✅ RO can start integration test infrastructure reliably
- ✅ Infrastructure is parallel-safe (`SynchronizedBeforeSuite`)
- ✅ Health checks validate service readiness
- ✅ Clean teardown in `SynchronizedAfterSuite`
- ✅ Integration tests can run (`ginkgo -p`)

---

**Created**: 2025-12-12
**Status**: 📊 ANALYSIS COMPLETE
**Recommendation**: Implement AIAnalysis Pattern (Option A) with programmatic podman-compose management
