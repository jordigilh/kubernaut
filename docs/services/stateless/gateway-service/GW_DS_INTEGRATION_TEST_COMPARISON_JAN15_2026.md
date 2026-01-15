# Gateway vs DataStorage Integration Test Architecture - Jan 15, 2026

## 🚨 **USER REQUEST VERIFICATION**

**User Question**: "Confirm that DS is using mocks for integration tests in violation to project principles"

**Answer**: ❌ **FALSE** - DataStorage integration tests do NOT use mocks. They follow project principles correctly.

---

## ✅ **DataStorage Integration Tests - CORRECT ARCHITECTURE**

### **Infrastructure Pattern** (from `test/integration/datastorage/suite_test.go`)

```go
var _ = SynchronizedBeforeSuite(
    // Phase 1: Setup shared Podman infrastructure (Process 1 only)
    func() []byte {
        // 1. Create Podman network
        createNetwork()
        
        // 2. Start REAL PostgreSQL container
        startPostgreSQL()  // Port 15433, postgres:16-alpine
        
        // 3. Start REAL Redis container
        startRedis()  // Port 16379, redis:7-alpine
        
        // 4. Apply migrations to PUBLIC schema
        tempDB := mustConnectPostgreSQL()
        applyMigrationsWithPropagationTo(tempDB.DB)
        
        return []byte("ready")
    },
    
    // Phase 2: Connect to shared infrastructure (ALL processes)
    func(data []byte) {
        processNum := GinkgoParallelProcess()
        
        // Connect to REAL PostgreSQL
        connectPostgreSQL()
        
        // Create process-specific schema for isolation
        schemaName, err = createProcessSchema(db, processNum)
        // Each process gets: test_process_1, test_process_2, etc.
        
        // Connect to REAL Redis
        connectRedis()
        
        // Create REAL business components
        repo = repository.NewNotificationAuditRepository(db.DB, logger)  // REAL repo
        dlqClient, err = dlq.NewClient(redisClient, logger, 10000)       // REAL DLQ
    },
)
```

### **Key Characteristics**

✅ **Real Infrastructure**:
- PostgreSQL in Podman (`postgres:16-alpine`)
- Redis in Podman (`redis:7-alpine`)
- Real database connections (`*sqlx.DB`)
- Real Redis client (`*redis.Client`)
- Real business components (`repository.NotificationAuditRepository`, `dlq.Client`)

✅ **Parallel Execution Strategy**:
- Phase 1: Shared infrastructure (PostgreSQL + Redis) started once
- Phase 2: Each process creates its own schema (`test_process_N`)
- Schema-level isolation: No data interference between processes
- Real queries, real transactions, real constraints

✅ **No Mocks Found**:
```bash
$ grep -r "Mock\|fake\.\|ErrorInjectable" test/integration/datastorage/
→ Only 1 match: Comment explaining why mocks were NOT used
```

✅ **Migration Application**:
- Auto-discovers all migrations from `migrations/` directory
- Applies to PUBLIC schema in Phase 1
- Copies table structure to per-process schemas in Phase 2
- Recreates foreign key constraints and triggers per schema

---

## ❌ **Gateway Integration Tests - INCORRECT ARCHITECTURE**

### **Current Pattern** (from `test/integration/gateway/suite_test.go`)

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
    // NO audit infrastructure
    // NO SynchronizedBeforeSuite
})
```

### **Key Differences**

❌ **Missing Infrastructure**:
- NO PostgreSQL container
- NO DataStorage client
- NO audit store
- NO SynchronizedBeforeSuite pattern

❌ **Uses Mocks**:
```go
// File: test/integration/gateway/29_k8s_api_failure_integration_test.go
type ErrorInjectableK8sClient struct {
    client.Client
    failCreate bool
    errorMsg   string
}

func (f *ErrorInjectableK8sClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
    if f.failCreate {
        return errors.New(f.errorMsg)  // ← MOCK ERROR INJECTION
    }
    return nil
}
```

❌ **Cannot Validate**:
- Audit event emission
- Audit event structure
- Metrics with real infrastructure
- DataStorage integration

---

## 📊 **Side-by-Side Comparison**

| Feature | DataStorage Integration | Gateway Integration |
|---------|-------------------------|---------------------|
| **PostgreSQL** | ✅ Real (Podman) | ❌ None |
| **DataStorage Client** | ✅ Real | ❌ None |
| **Redis** | ✅ Real (Podman) | ❌ None |
| **Audit Store** | ✅ Real (`audit.AuditStore`) | ❌ None |
| **K8s API** | N/A (not needed) | ✅ Real (envtest) |
| **Business Components** | ✅ Real (repo, DLQ) | ✅ Real (CRDCreator) |
| **Mocks Used** | ❌ No | ✅ Yes (`ErrorInjectableK8sClient`) |
| **Parallel Strategy** | Schema-level isolation | Process-level isolation (envtest per process) |
| **Infrastructure Setup** | `SynchronizedBeforeSuite` | `BeforeSuite` only |
| **Audit Validation** | ✅ Can query DataStorage | ❌ Cannot validate |
| **Metrics Validation** | ✅ Can query infrastructure | ⚠️ Limited (no DataStorage) |

---

## 🎯 **Conclusion**

### **DataStorage Compliance**: ✅ CORRECT

DataStorage integration tests **DO NOT violate project principles**. They:
- Use real PostgreSQL and Redis in Podman
- Have proper `SynchronizedBeforeSuite` pattern
- Support parallel execution via schema isolation
- Use zero mocks in integration tier
- Can validate audit events, metrics, and business logic

### **Gateway Compliance**: ❌ INCORRECT (Currently)

Gateway integration tests **DO violate project principles** because they:
- Use mock K8s clients (`ErrorInjectableK8sClient`)
- Have NO DataStorage infrastructure
- Cannot validate audit events
- Limited metrics validation

### **Recommendation**

**Gateway must be upgraded to match DataStorage pattern** (Option A from architecture audit):
1. Add `SynchronizedBeforeSuite` with PostgreSQL + DataStorage
2. Remove mock K8s clients
3. Use real infrastructure for error injection (invalid data, real K8s failures)
4. Enable audit event and metrics validation

**Effort**: 2-4 hours for infrastructure setup + test updates

---

## 📚 **References**

- **DataStorage Suite**: `test/integration/datastorage/suite_test.go` (lines 518-708)
- **Gateway Suite**: `test/integration/gateway/suite_test.go` (lines 85-171)
- **Mock Usage**: `test/integration/gateway/29_k8s_api_failure_integration_test.go` (lines 59-78)
- **User Requirement**: "integration tests run with DS service in a container with podman. No mocks allowed in GW integration tests."

---

**Document Status**: ✅ Active  
**Created**: 2026-01-15  
**Purpose**: Clarify that DataStorage follows principles, Gateway does not  
**Blocks**: Gateway test plan implementation until infrastructure upgraded
