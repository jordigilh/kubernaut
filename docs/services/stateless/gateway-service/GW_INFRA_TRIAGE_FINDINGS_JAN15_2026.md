# Gateway Infrastructure Triage Findings - Jan 15, 2026

## 🔍 **Triage Objective**

User Request: "Triage test/infrastructure with programmatic go to ensure DS service is setup properly like with the other services"

**Finding**: `test/infrastructure/` is for **E2E tests** (Kind clusters), NOT integration tests (Podman containers).

---

## 📊 **Infrastructure Patterns Discovered**

### **Pattern 1: E2E Tests** ❌ (NOT what we need)

**Location**: `test/infrastructure/*.go`

**Purpose**: E2E test infrastructure with **Kind clusters**

**Files**:
- `datastorage.go` - Creates Kind cluster + loads Docker images
- `gateway_e2e.go` - Gateway E2E infrastructure
- `holmesgpt_api.go` - HolmesGPT API infrastructure
- etc.

**Pattern**:
```go
// E2E Pattern (Kind cluster)
func CreateDataStorageCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
    // 1. Create Kind cluster
    createKindCluster(clusterName, kubeconfigPath, writer)
    
    // 2. Build Docker image
    buildDataStorageImage(writer)
    
    // 3. Load image into Kind
    loadDataStorageImage(clusterName, writer)
    
    return nil
}
```

**NOT applicable** to Gateway integration tests because:
- Integration tests use **Podman containers**, not Kind clusters
- Integration tests use **envtest** for K8s API, not real clusters
- E2E infrastructure is too heavyweight for integration tests

---

### **Pattern 2: Integration Tests** ✅ (CORRECT pattern)

**Location**: `test/integration/*/suite_test.go` (each service's own suite file)

**Purpose**: Integration test infrastructure with **Podman containers** + **envtest**

**Example**: DataStorage Integration Tests

**File**: `test/integration/datastorage/suite_test.go`

**Pattern**:
```go
var _ = SynchronizedBeforeSuite(
    // Phase 1: Start shared Podman infrastructure (Process 1 only)
    func() []byte {
        // 1. Preflight checks
        preflightCheck()
        
        // 2. Create Podman network
        createNetwork()
        
        // 3. Start PostgreSQL container
        startPostgreSQL()
        
        // 4. Start Redis container
        startRedis()
        
        // 5. Apply migrations
        tempDB := mustConnectPostgreSQL()
        applyMigrationsWithPropagationTo(tempDB.DB)
        
        return []byte("ready")
    },
    
    // Phase 2: Connect to shared infrastructure (ALL processes)
    func(data []byte) {
        processNum := GinkgoParallelProcess()
        
        // Connect to PostgreSQL
        connectPostgreSQL()
        
        // Create process-specific schema for isolation
        schemaName, err = createProcessSchema(db, processNum)
        
        // Connect to Redis
        connectRedis()
        
        // Create business components
        repo = repository.NewNotificationAuditRepository(db.DB, logger)
        dlqClient, err = dlq.NewClient(redisClient, logger, 10000)
    },
)
```

**Key Characteristics**:
1. ✅ **SynchronizedBeforeSuite** with 2 phases
2. ✅ **Phase 1** (Process 1 only): Start shared Podman containers
3. ✅ **Phase 2** (All processes): Connect to infrastructure + create per-process isolation
4. ✅ **Parallel-safe**: Schema-level or namespace-level isolation
5. ✅ **Real infrastructure**: PostgreSQL, Redis in Podman (not mocks)
6. ✅ **Real business components**: Repository, DLQ client, etc.

---

## 🎯 **Pattern for Gateway Integration Tests**

### **Current State** (Gateway integration)

**File**: `test/integration/gateway/suite_test.go`

```go
var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())
    
    // Only envtest (in-memory K8s API)
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{"../../../config/crd/bases"},
    }
    
    k8sConfig, err = testEnv.Start()
    k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme})
    
    // NO PostgreSQL
    // NO DataStorage
    // NO SynchronizedBeforeSuite
})
```

**Missing**:
- ❌ NO `SynchronizedBeforeSuite`
- ❌ NO PostgreSQL in Podman
- ❌ NO DataStorage service
- ❌ NO audit infrastructure

---

### **Target State** (Gateway integration with DataStorage)

**File**: `test/integration/gateway/suite_test.go` (needs upgrade)

```go
var (
    // Shared infrastructure (Phase 1)
    postgresContainer = "gateway-postgres-test"
    redisContainer    = "gateway-redis-test"  // If needed for Gateway
    
    // Per-process resources (Phase 2)
    ctx            context.Context
    cancel         context.CancelFunc
    k8sClient      client.Client
    testEnv        *envtest.Environment
    k8sConfig      *rest.Config
    dsClient       *api.Client       // ← NEW: Real DataStorage client
    logger         logr.Logger
    gatewayService *gateway.Service  // ← NEW: Real Gateway service
)

var _ = SynchronizedBeforeSuite(
    // Phase 1: Start shared Podman infrastructure (Process 1 only)
    func() []byte {
        // 1. Preflight checks
        preflightCheck()
        
        // 2. Create Podman network
        createNetwork()
        
        // 3. Start PostgreSQL container
        startPostgreSQL()
        
        // 4. Apply migrations to PUBLIC schema
        tempDB := mustConnectPostgreSQL()
        applyMigrationsWithPropagationTo(tempDB.DB)
        
        // 5. Start DataStorage service (optional: can be in-process)
        // startDataStorageService() // OR use direct DB connection
        
        return []byte("ready")
    },
    
    // Phase 2: Connect to shared infrastructure (ALL processes)
    func(data []byte) {
        processNum := GinkgoParallelProcess()
        
        ctx, cancel = context.WithCancel(context.Background())
        logger = kubelog.NewLogger(kubelog.DevelopmentOptions())
        
        // Connect to PostgreSQL (create per-process schema if needed)
        connectPostgreSQL()
        
        // Create DataStorage client
        dsClient, err = audit.NewOpenAPIClientAdapterWithTransport(
            "http://127.0.0.1:15433",  // DataStorage URL
            5*time.Second,
            authTransport,
        )
        Expect(err).ToNot(HaveOccurred())
        
        // Create envtest (per-process)
        testEnv = &envtest.Environment{
            CRDDirectoryPaths: []string{"../../../config/crd/bases"},
        }
        k8sConfig, err = testEnv.Start()
        k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme})
        
        // Create Gateway service with real dependencies
        gatewayService = gateway.NewService(dsClient, k8sClient, logger)
    },
)
```

---

## 📋 **Implementation Checklist**

### **Step 1: Add Infrastructure Functions**

Create new functions in `test/integration/gateway/suite_test.go`:

```go
// preflightCheck validates environment before running tests
func preflightCheck() error { ... }

// createNetwork creates Podman network for containers
func createNetwork() { ... }

// startPostgreSQL starts PostgreSQL in Podman
func startPostgreSQL() { ... }

// mustConnectPostgreSQL creates DB connection
func mustConnectPostgreSQL() *sqlx.DB { ... }

// applyMigrationsWithPropagationTo applies migrations
func applyMigrationsWithPropagationTo(targetDB *sql.DB) { ... }

// cleanupContainers removes Podman containers
func cleanupContainers() { ... }
```

**Source**: Copy patterns from `test/integration/datastorage/suite_test.go`

---

### **Step 2: Update BeforeSuite to SynchronizedBeforeSuite**

**Before**:
```go
var _ = BeforeSuite(func() {
    // Only envtest
})
```

**After**:
```go
var _ = SynchronizedBeforeSuite(
    func() []byte { /* Phase 1: Infrastructure */ },
    func(data []byte) { /* Phase 2: Per-process */ },
)
```

---

### **Step 3: Update AfterSuite to SynchronizedAfterSuite**

**Add Phase 1 (all processes) + Phase 2 (process 1 only):**
```go
var _ = SynchronizedAfterSuite(
    func() { /* Phase 1: Per-process cleanup */ },
    func() { /* Phase 2: Stop shared infrastructure */ },
)
```

---

### **Step 4: Create Helper Functions for Tests**

```go
// FindAuditEventByTypeAndCorrelationID queries DataStorage
func FindAuditEventByTypeAndCorrelationID(
    ctx context.Context,
    dsClient *api.Client,
    eventType api.GatewayAuditPayloadEventType,
    correlationID string,
    timeout time.Duration,
) (*api.AuditEvent, error) { ... }
```

---

## 🔗 **Reference Files**

### **Source Pattern** (Copy from)
- `test/integration/datastorage/suite_test.go` (lines 518-708)
- Infrastructure functions (lines 387-1020)
- Helper functions in test files

### **Target Pattern** (Apply to)
- `test/integration/gateway/suite_test.go` (needs upgrade)

### **Documentation**
- `GW_INTEGRATION_TEST_ARCHITECTURE_JAN15_2026.md`
- `GW_DS_INTEGRATION_TEST_COMPARISON_JAN15_2026.md`
- `GW_INTEGRATION_TEST_STANDARD_PATTERN.md`

---

## ⏱️ **Effort Estimate**

- **Infrastructure functions**: 1-2 hours (copy + adapt from DataStorage)
- **SynchronizedBeforeSuite**: 30 minutes (pattern is well-defined)
- **Helper functions**: 30 minutes (already documented)
- **Testing**: 30 minutes (smoke test with 1-2 tests)
- **Total**: 2.5-3.5 hours

---

## ✅ **Success Criteria**

1. ✅ Gateway integration tests use `SynchronizedBeforeSuite`
2. ✅ PostgreSQL starts in Podman (Phase 1)
3. ✅ DataStorage client connects (Phase 2)
4. ✅ Tests can query audit events from real DataStorage
5. ✅ Parallel execution works (4+ processes)
6. ✅ No mocks for audit infrastructure

---

**Document Status**: ✅ Active  
**Created**: 2026-01-15  
**Purpose**: Triage infrastructure patterns for Gateway integration tests  
**Next Step**: Apply DataStorage pattern to Gateway suite
