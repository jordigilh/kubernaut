# Integration Tests - Immudb Migration Plan
## January 6, 2026

## 📊 **CRITICAL DISCOVERY**

All 6 integration test infrastructures are **missing Immudb** and using custom infrastructure setup instead of the shared `StartDSBootstrap()` function.

### **Affected Services**:
- ❌ Gateway (`test/infrastructure/gateway.go`)
- ❌ SignalProcessing (`test/infrastructure/signalprocessing.go`)
- ❌ AIAnalysis (`test/infrastructure/aianalysis.go`)
- ❌ Notification (`test/infrastructure/notification.go`)
- ❌ WorkflowExecution (`test/infrastructure/workflowexecution.go`)
- ❌ RemediationOrchestrator (`test/infrastructure/remediationorchestrator.go`)

### **Root Cause**:
1. `StartDSBootstrap()` was created in `datastorage_bootstrap.go` to centralize Data Storage infrastructure (PostgreSQL + Redis + **Immudb** + DataStorage)
2. `AuthWebhook` was updated to use it (includes Immudb)
3. Other 6 services still use custom infrastructure setup (missing Immudb)

### **Impact**:
- Integration tests cannot validate SOC2 audit trails
- Data Storage cannot persist audit events (requires Immudb)
- Code duplication across 6 services
- Missing tamper-evident audit capabilities (SOC2 Gap #9)

---

## 🎯 **MIGRATION PLAN**

### **Pattern to Apply** (from `authwebhook.go`):

**BEFORE** (Custom Infrastructure):
```go
// Manual PostgreSQL startup
if err := startServicePostgreSQL(writer); err != nil {
    return err
}

// Manual Redis startup
if err := startServiceRedis(writer); err != nil {
    return err
}

// Manual DataStorage startup (NO IMMUDB!)
if err := startServiceDataStorage(writer); err != nil {
    return err
}
```

**AFTER** (Shared Bootstrap with Immudb):
```go
cfg := DSBootstrapConfig{
    ServiceName:     "gateway",  // Service-specific name
    PostgresPort:    15437,      // Per DD-TEST-001 v2.2
    RedisPort:       16383,
    ImmudbPort:      13323,      // ✅ IMMUDB INCLUDED!
    DataStoragePort: 18091,
    MetricsPort:     19091,
    ConfigDir:       "test/integration/gateway/config",
}

infra, err := StartDSBootstrap(cfg, writer)
if err != nil {
    return err
}

i.DSBootstrapInfra = infra  // Store for teardown
```

---

## 📋 **SERVICE-SPECIFIC CONFIGURATIONS**

### **1. Gateway** (`gateway.go`)
```go
cfg := DSBootstrapConfig{
    ServiceName:     "gateway",
    PostgresPort:    15437,
    RedisPort:       16383,
    ImmudbPort:      13323,  // DD-TEST-001 v2.2
    DataStoragePort: 18091,
    MetricsPort:     19091,
    ConfigDir:       "test/integration/gateway/config",
}
```

### **2. SignalProcessing** (`signalprocessing.go`)
```go
cfg := DSBootstrapConfig{
    ServiceName:     "signalprocessing",
    PostgresPort:    15436,
    RedisPort:       16382,
    ImmudbPort:      13324,  // DD-TEST-001 v2.2
    DataStoragePort: 18094,
    MetricsPort:     19094,
    ConfigDir:       "test/integration/signalprocessing/config",
}
```

### **3. RemediationOrchestrator** (`remediationorchestrator.go`)
```go
cfg := DSBootstrapConfig{
    ServiceName:     "remediationorchestrator",
    PostgresPort:    15435,
    RedisPort:       16381,
    ImmudbPort:      13325,  // DD-TEST-001 v2.2
    DataStoragePort: 18140,
    MetricsPort:     18141,
    ConfigDir:       "test/integration/remediationorchestrator/config",
}
```

### **4. AIAnalysis** (`aianalysis.go`)
```go
cfg := DSBootstrapConfig{
    ServiceName:     "aianalysis",
    PostgresPort:    15438,
    RedisPort:       16384,
    ImmudbPort:      13326,  // DD-TEST-001 v2.2
    DataStoragePort: 18095,
    MetricsPort:     19095,
    ConfigDir:       "test/integration/aianalysis/config",
}
```

### **5. Notification** (`notification.go`)
```go
cfg := DSBootstrapConfig{
    ServiceName:     "notification",
    PostgresPort:    15440,
    RedisPort:       16385,
    ImmudbPort:      13328,  // DD-TEST-001 v2.2
    DataStoragePort: 18096,
    MetricsPort:     19096,
    ConfigDir:       "test/integration/notification/config",
}
```

### **6. WorkflowExecution** (`workflowexecution.go`)
```go
cfg := DSBootstrapConfig{
    ServiceName:     "workflowexecution",
    PostgresPort:    15441,
    RedisPort:       16388,
    ImmudbPort:      13327,  // DD-TEST-001 v2.2
    DataStoragePort: 18097,
    MetricsPort:     19097,
    ConfigDir:       "test/integration/workflowexecution/config",
}
```

---

## 🔧 **IMPLEMENTATION STEPS**

### **For Each Service**:

1. **Add DSBootstrapInfra field** to infrastructure struct:
   ```go
   type GatewayInfrastructure struct {
       DSBootstrapInfra *DSBootstrapInfra  // ✅ ADD THIS
       // ... existing fields ...
   }
   ```

2. **Replace custom startup** with `StartDSBootstrap()`:
   - Remove `startServicePostgreSQL()`
   - Remove `startServiceRedis()`
   - Remove `startServiceDataStorage()`
   - Replace with single `StartDSBootstrap()` call

3. **Update Teardown** to use `StopDSBootstrap()`:
   ```go
   func (i *GatewayInfrastructure) Teardown(writer io.Writer) error {
       if i.DSBootstrapInfra == nil {
           return nil
       }
       return StopDSBootstrap(i.DSBootstrapInfra, writer)
   }
   ```

4. **Remove custom cleanup** functions:
   - Remove `stopServicePostgreSQL()`
   - Remove `stopServiceRedis()`
   - Remove `stopServiceDataStorage()`

---

## ✅ **BENEFITS**

### **Immediate**:
- ✅ **Immudb included** - SOC2-compliant immutable audit trails
- ✅ **Code deduplication** - 6 services use shared infrastructure
- ✅ **Consistent setup** - All services follow same pattern
- ✅ **Easier maintenance** - Single place to update infrastructure

### **SOC2 Compliance**:
- ✅ Tamper-evident audit logs (Immudb merkle trees)
- ✅ Time-travel queries for audit investigations
- ✅ Cryptographic proof of data integrity
- ✅ Integration tests can validate audit compliance

---

## 📊 **MIGRATION STATUS**

| Service | Status | Immudb Port | Notes |
|---------|--------|-------------|-------|
| AuthWebhook | ✅ **DONE** | 13330 | Already using `StartDSBootstrap()` |
| Gateway | ❌ **TODO** | 13323 | Custom infrastructure |
| SignalProcessing | ❌ **TODO** | 13324 | Custom infrastructure |
| RemediationOrchestrator | ❌ **TODO** | 13325 | Custom infrastructure |
| AIAnalysis | ❌ **TODO** | 13326 | Custom infrastructure |
| Notification | ❌ **TODO** | 13328 | Custom infrastructure |
| WorkflowExecution | ❌ **TODO** | 13327 | Custom infrastructure |

---

## ⚡ **PRIORITY**

**CRITICAL**: Without Immudb, integration tests cannot validate SOC2 audit compliance, which is a core business requirement.

**Estimated Effort**: 20-30 minutes per service = 2-3 hours total

---

**Document Status**: ✅ Active  
**Created**: 2026-01-06  
**Priority**: CRITICAL - Blocks SOC2 audit validation  
**Related**: DD-TEST-001 v2.2, SOC2 Gap #9, IMMUDB_INTEGRATION_STATUS_JAN06.md

