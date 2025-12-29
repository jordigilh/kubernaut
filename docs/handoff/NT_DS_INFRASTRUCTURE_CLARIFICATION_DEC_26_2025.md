# DataStorage Infrastructure Functions Clarification - December 26, 2025

**Date**: December 26, 2025
**Status**: ✅ **CLARIFICATION**
**Related**: NT_SHARED_INFRASTRUCTURE_UTILITIES_TRIAGE_DEC_26_2025.md

---

## 🎯 **User Question**

> "I was under the impression that there was a common function to build and deploy the DS service and dependencies. Was it for E2E only?"

**Answer**: **YES!** You're absolutely correct!

---

## 📊 **Two Different Infrastructure Patterns**

### **Pattern 1: E2E Tests** ✅ **HAS SHARED FUNCTIONS**

**Environment**: Kubernetes (Kind clusters)
**File**: `test/infrastructure/datastorage.go`

**Shared Functions**:
1. **`DeployDataStorageTestServices()`** (line 236)
   - Deploys PostgreSQL, Redis, DataStorage to Kubernetes namespace
   - Used by: AIAnalysis E2E, SignalProcessing E2E, WorkflowExecution E2E, Gateway E2E

2. **`deployPostgreSQLInNamespace()`** (line 345)
   - Deploys PostgreSQL StatefulSet to K8s

3. **`deployRedisInNamespace()`** (line 580)
   - Deploys Redis Deployment to K8s

4. **`deployDataStorageServiceInNamespace()`** (line 706)
   - Deploys DataStorage Deployment to K8s

**Usage Example** (from E2E tests):
```go
// test/infrastructure/gateway_e2e.go
err := infrastructure.DeployDataStorageTestServices(
    ctx,
    namespace,
    kubeconfigPath,
    dataStorageImage,
    writer,
)
```

**Who Uses This**: ✅ **All E2E tests**
- Gateway E2E
- AIAnalysis E2E
- SignalProcessing E2E
- WorkflowExecution E2E
- RemediationOrchestrator E2E

---

### **Pattern 2: Integration Tests** ❌ **NO SHARED FUNCTIONS**

**Environment**: Local Podman containers
**Files**: Gateway, RO, WE, SignalProcessing all have their own

**Current State**: Each service implements its own functions
1. **Gateway**: `StartGatewayIntegrationInfrastructure()`
2. **RO**: `StartROIntegrationInfrastructure()`
3. **WE**: `StartWEIntegrationInfrastructure()`
4. **SignalProcessing**: `StartSPIntegrationInfrastructure()`

**Usage Example** (from Integration tests):
```go
// test/integration/gateway/suite_test.go
err := infrastructure.StartGatewayIntegrationInfrastructure(GinkgoWriter)
```

**Who Uses This**: ❌ **Each service has its own implementation**
- Gateway Integration: `startGatewayPostgreSQL()`, `startGatewayRedis()`, `startGatewayDataStorage()`
- RO Integration: `startROPostgreSQL()`, `startRORedis()`, `startRODataStorage()`
- WE Integration: `startWEPostgreSQL()`, `startWERedis()`, `startWEDataStorage()`
- SP Integration: Similar pattern

**Result**: ~720 lines of duplicated code!

---

## 🔍 **Why the Confusion?**

### **DataStorage Has Its Own Function**

**File**: `test/infrastructure/datastorage.go:1271`

```go
// StartDataStorageInfrastructure starts all Data Storage Service infrastructure
// Returns an infrastructure handle that can be used to stop the services
func StartDataStorageInfrastructure(cfg *DataStorageConfig, writer io.Writer) (*DataStorageInfrastructure, error) {
    // Starts PostgreSQL, Redis, runs migrations, builds and starts DataStorage
    // ...
}
```

**BUT**: This is ONLY used by DataStorage's own integration tests!

**Who Uses**: ❌ **ONLY DataStorage integration tests**
- NOT used by Gateway integration tests
- NOT used by RO integration tests
- NOT used by WE integration tests
- NOT used by SP integration tests
- NOT used by Notification integration tests

---

## ✅ **The Gap: Integration Tests Need Shared Functions**

### **Current State**

| Test Type | Environment | Shared Functions? | Status |
|-----------|-------------|-------------------|--------|
| **E2E** | Kubernetes (Kind) | ✅ YES | `DeployDataStorageTestServices()` |
| **Integration** | Podman containers | ❌ NO | Each service rolls their own |

### **Problem**

**E2E Tests**: ✅ No duplication (use shared `DeployDataStorageTestServices()`)

**Integration Tests**: ❌ Massive duplication
- Gateway: `startGatewayPostgreSQL()`, `startGatewayRedis()`, `startGatewayDataStorage()`
- RO: `startROPostgreSQL()`, `startRORedis()`, `startRODataStorage()`
- WE: `startWEPostgreSQL()`, `startWERedis()`, `startWEDataStorage()`
- SP: Similar pattern
- **Result**: ~720 lines duplicated

---

## 💡 **Solution: Create Shared Integration Functions**

### **Proposal**: `test/infrastructure/shared_integration_utils.go`

**Mirror E2E Pattern for Integration Tests**:

```go
// E2E Pattern (EXISTS - Kubernetes)
DeployDataStorageTestServices() // Shared across all E2E tests

// Integration Pattern (MISSING - Podman)
StartPostgreSQL()     // ❌ Should be shared, currently duplicated 4x
StartRedis()          // ❌ Should be shared, currently duplicated 4x
StartDataStorage()    // ❌ Should be shared, currently duplicated 4x
WaitForHTTPHealth()   // ❌ Should be shared, currently duplicated 4x
```

---

## 📋 **Why DataStorage Has Its Own Function But Others Don't**

### **DataStorage's `StartDataStorageInfrastructure()`**

**Purpose**: DataStorage's own integration tests
**Scope**: Comprehensive (PostgreSQL + Redis + Migrations + Build + Start DS service)
**Who Uses**: ONLY DataStorage integration tests

### **Why Other Services Don't Use It**

1. **Too Specific**: Includes DataStorage service build/start (other services just need PostgreSQL + Redis + existing DS)
2. **Different Ports**: DataStorage uses default ports, other services need unique ports (DD-TEST-001)
3. **Different Config**: Different DB names, users, passwords per service
4. **Not Designed for Reuse**: Hardcoded container names, not parameterized

---

## ✅ **Recommendation: Two-Tier Shared Functions**

### **Tier 1: Low-Level Utilities** (Shared across ALL integration tests)

```go
// test/infrastructure/shared_integration_utils.go
StartPostgreSQL(cfg PostgreSQLConfig, writer)
WaitForPostgreSQLReady(containerName, dbUser, dbName, writer)
StartRedis(cfg RedisConfig, writer)
WaitForRedisReady(containerName, writer)
WaitForHTTPHealth(healthURL, timeout, writer)
CleanupContainers(containerNames, writer)
```

**Usage**: Notification, Gateway, RO, WE, SP, DataStorage (all services)

### **Tier 2: Service-Specific Orchestration** (Per service)

```go
// test/infrastructure/notification_integration.go
StartNotificationIntegrationInfrastructure(writer) {
    // Uses Tier 1 utilities
    StartPostgreSQL(...)
    WaitForPostgreSQLReady(...)
    // ... service-specific orchestration
}
```

**Usage**: Each service has its own orchestration function that uses Tier 1 utilities

---

## 🎯 **Summary**

### **User's Intuition**: ✅ **CORRECT**

**E2E Tests**: YES, there ARE shared functions (`DeployDataStorageTestServices()`)

**Integration Tests**: NO, there are NOT shared functions (each service duplicates code)

### **What Exists**

| Function | Scope | Who Uses | Location |
|----------|-------|----------|----------|
| `DeployDataStorageTestServices()` | E2E (Kubernetes) | All E2E tests | `datastorage.go:236` |
| `StartDataStorageInfrastructure()` | Integration (Podman) | DataStorage only | `datastorage.go:1271` |

### **What's Missing**

**Shared integration utilities** for Podman-based integration tests:
- `StartPostgreSQL()`
- `StartRedis()`
- `WaitForHTTPHealth()`
- etc.

Currently: ~720 lines duplicated across 4+ services

---

## 📝 **Next Steps**

### **Option A**: Create Shared Integration Utilities (Recommended)

1. Create `test/infrastructure/shared_integration_utils.go`
2. Implement 7 shared functions (Tier 1)
3. Use in Notification (Tier 2)
4. Optionally migrate Gateway, RO, WE, SP later

**Benefits**:
- ✅ Follows E2E pattern (shared functions for common tasks)
- ✅ Reduces duplication from ~720 → ~400 lines (-44%)
- ✅ Consistent behavior across services

### **Option B**: Use DataStorage's Function (NOT Recommended)

**Why Not**:
- ❌ Too specific (builds/starts DS service)
- ❌ Hardcoded ports (conflicts with DD-TEST-001)
- ❌ Not parameterized for reuse
- ❌ Would require major refactoring

---

**Document Version**: 1.0.0
**Created**: December 26, 2025
**Status**: Clarification Complete
**Answer**: E2E has shared functions, Integration tests don't (yet)
**Recommendation**: Create shared integration utilities (Option A)




