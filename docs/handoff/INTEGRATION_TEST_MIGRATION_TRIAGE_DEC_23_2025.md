# Integration Test Migration Triage - Complete Analysis

**Date**: December 23, 2025
**Status**: 🔍 **TRIAGE COMPLETE**
**Services Needing Migration**: 5/7 services

---

## 📊 **Service Migration Status**

| Service | Integration Tests | Infrastructure Pattern | Migration Status | Priority |
|---------|------------------|----------------------|------------------|----------|
| **Gateway** | ✅ Yes | ✅ **Shared DSBootstrapConfig** | ✅ **MIGRATED** | N/A |
| **AIAnalysis** | ✅ Yes | ⚠️ `StartAIAnalysisIntegrationInfrastructure` + HAPI | 🔴 **NEEDS MIGRATION** | **P1 - Complex** |
| **RemediationOrchestrator** | ✅ Yes | ⚠️ `StartROIntegrationInfrastructure` | 🔴 **NEEDS MIGRATION** | **P2 - Simple** |
| **SignalProcessing** | ✅ Yes | ⚠️ `StartSignalProcessingIntegrationInfrastructure` | 🔴 **NEEDS MIGRATION** | **P2 - Simple** |
| **WorkflowExecution** | ✅ Yes | ⚠️ `setup-infrastructure.sh` (shell script) | 🔴 **NEEDS MIGRATION** | **P2 - Simple** |
| **Notification** | ✅ Yes | ⚠️ `setup-infrastructure.sh` (shell script) | 🔴 **NEEDS MIGRATION** | **P2 - Simple** |
| **DataStorage** | ✅ Yes | ❓ N/A (IS the infrastructure) | ✅ **N/A** | N/A |

---

## 🎯 **Migration Priorities**

### **P1 - Complex Migration** (1 service - 90 minutes)
**AIAnalysis** - Requires custom HAPI container setup
- **Current**: `StartAIAnalysisIntegrationInfrastructure` (custom Go)
- **Target**: `DSBootstrapConfig` + `GenericContainerConfig` for HAPI
- **Complexity**: Medium-High (needs HAPI setup)
- **Files**: `test/integration/aianalysis/suite_test.go`, `test/infrastructure/aianalysis.go`
- **Estimated Time**: 60-90 minutes

### **P2 - Simple Migrations** (4 services - 150 minutes total)
**RemediationOrchestrator** - Sequential podman pattern
- **Current**: `StartROIntegrationInfrastructure` (custom Go)
- **Target**: `DSBootstrapConfig`
- **Complexity**: Low
- **Files**: `test/integration/remediationorchestrator/suite_test.go`, `test/infrastructure/remediationorchestrator.go`
- **Estimated Time**: 30-45 minutes

**SignalProcessing** - Sequential podman pattern
- **Current**: `StartSignalProcessingIntegrationInfrastructure` (custom Go)
- **Target**: `DSBootstrapConfig`
- **Complexity**: Low
- **Files**: `test/integration/signalprocessing/suite_test.go`, `test/infrastructure/signalprocessing.go`
- **Estimated Time**: 30-45 minutes

**WorkflowExecution** - Shell script pattern
- **Current**: `setup-infrastructure.sh` (shell script - DD-TEST-002 violation)
- **Target**: `DSBootstrapConfig`
- **Complexity**: Low
- **Files**: `test/integration/workflowexecution/suite_test.go`, `test/integration/workflowexecution/setup-infrastructure.sh`
- **Estimated Time**: 30-45 minutes

**Notification** - Shell script pattern
- **Current**: `setup-infrastructure.sh` (shell script - DD-TEST-002 violation)
- **Target**: `DSBootstrapConfig`
- **Complexity**: Low
- **Files**: `test/integration/notification/suite_test.go`, `test/integration/notification/setup-infrastructure.sh`
- **Estimated Time**: 30-45 minutes

---

## 📋 **Migration Statistics**

### **Code Reduction Potential**

| Service | Infrastructure Lines | Target Lines | Reduction |
|---------|---------------------|--------------|-----------|
| **Gateway** | ~460 lines | 10 lines | **98%** ✅ |
| **AIAnalysis** | ~800 lines | 30 lines (DS + HAPI) | **96%** |
| **RemediationOrchestrator** | ~350 lines | 10 lines | **97%** |
| **SignalProcessing** | ~400 lines | 10 lines | **98%** |
| **WorkflowExecution** | ~200 lines (shell) | 10 lines | **95%** |
| **Notification** | ~200 lines (shell) | 10 lines | **95%** |
| **TOTAL** | ~2,410 lines | ~80 lines | **97%** |

**Potential Code Reduction**: **2,330 lines** of duplicate infrastructure code

### **Effort Estimates**

| Category | Services | Total Time |
|----------|---------|------------|
| **Complex** | 1 (AIAnalysis) | 90 minutes |
| **Simple** | 4 (RO, SP, WE, NT) | 150 minutes |
| **Validation** | All | 60 minutes |
| **Documentation** | All | 30 minutes |
| **TOTAL** | 5 services | **5-6 hours** |

---

## 🔍 **Migration Pattern Analysis**

### **Pattern 1: Custom Go Infrastructure** (3 services)
- **AIAnalysis**: `StartAIAnalysisIntegrationInfrastructure` + HAPI
- **RemediationOrchestrator**: `StartROIntegrationInfrastructure`
- **SignalProcessing**: `StartSignalProcessingIntegrationInfrastructure`

**Migration**: Replace custom functions with `DSBootstrapConfig`

### **Pattern 2: Shell Script Infrastructure** (2 services - DD-TEST-002 violation)
- **WorkflowExecution**: `setup-infrastructure.sh`
- **Notification**: `setup-infrastructure.sh`

**Migration**: Replace shell scripts with `DSBootstrapConfig` in Go

### **Pattern 3: Shared Infrastructure** (1 service - target pattern)
- **Gateway**: `DSBootstrapConfig` ✅

**Status**: Migration complete, serving as reference pattern

---

## 📁 **File Inventory**

### **Files Needing Migration**

#### **AIAnalysis**
- `test/integration/aianalysis/suite_test.go` (update setup/teardown)
- `test/infrastructure/aianalysis.go` (cleanup ~800 lines)
- `test/integration/aianalysis/podman-compose.yml` (**DEPRECATE** - DD-TEST-002 violation)

#### **RemediationOrchestrator**
- `test/integration/remediationorchestrator/suite_test.go` (update setup/teardown)
- `test/infrastructure/remediationorchestrator.go` (cleanup ~350 lines)

#### **SignalProcessing**
- `test/integration/signalprocessing/suite_test.go` (update setup/teardown)
- `test/infrastructure/signalprocessing.go` (cleanup ~400 lines)

#### **WorkflowExecution**
- `test/integration/workflowexecution/suite_test.go` (update setup/teardown)
- `test/integration/workflowexecution/setup-infrastructure.sh` (**DELETE** - DD-TEST-002 violation)
- `test/infrastructure/workflowexecution.go` (may need creation/cleanup)

#### **Notification**
- `test/integration/notification/suite_test.go` (update setup/teardown)
- `test/integration/notification/setup-infrastructure.sh` (**DELETE** - DD-TEST-002 violation)
- `test/infrastructure/notification.go` (cleanup ~200 lines)

---

## 🚀 **Recommended Migration Order**

### **Phase 1: Simple Migrations** (150 minutes)
1. **RemediationOrchestrator** (30-45 min) - Validate pattern works for custom Go
2. **SignalProcessing** (30-45 min) - Confirm pattern scales
3. **WorkflowExecution** (30-45 min) - Validate shell script replacement
4. **Notification** (30-45 min) - Complete simple migrations

**Rationale**: Build confidence with simple migrations before tackling complex AIAnalysis

### **Phase 2: Complex Migration** (90 minutes)
5. **AIAnalysis** (60-90 min) - HAPI + DS infrastructure

**Rationale**: Tackle most complex migration last with proven pattern

### **Phase 3: Validation** (60 minutes)
- Run all integration tests
- Verify no regressions
- Confirm DD-TEST-001 v1.3 compliance
- Validate image cleanup

---

## 📊 **Port Allocation Summary**

| Service | PostgreSQL | Redis | DataStorage | Metrics | Status |
|---------|-----------|-------|-------------|---------|--------|
| **Gateway** | 15437 | 16383 | 18091 | 19091 | ✅ Migrated |
| **AIAnalysis** | 15438 | 16384 | 18095 | 19095 | 🔴 Pending |
| **RemediationOrchestrator** | 15432 | 16378 | 18089 | 19089 | 🔴 Pending |
| **SignalProcessing** | 15440 | 16386 | 18098 | 19098 | 🔴 Pending |
| **WorkflowExecution** | 15441 | 16387 | 18097 | 19097 | 🔴 Pending |
| **Notification** | 15439 | 16385 | 18096 | 19096 | 🔴 Pending |

**Source**: DD-TEST-001 v1.7 Port Allocation Strategy

---

## 🎯 **Success Criteria**

For each migrated service:

✅ **Build Success**
- `go build ./test/infrastructure/...` passes
- No lint errors

✅ **Test Success**
- All integration tests pass (100%)
- No flakiness or timing issues
- Parallel execution works correctly

✅ **DD-TEST-001 v1.3 Compliance**
- Image tags use UUID format: `{infrastructure}-{consumer}-{uuid}`
- Automatic image cleanup works
- Base images (postgres, redis) properly cached

✅ **DD-TEST-002 Compliance**
- Sequential container startup (no race conditions)
- No shell scripts for multi-service dependencies
- Programmatic Go for all infrastructure

✅ **Code Quality**
- 95%+ code reduction in infrastructure setup
- Single source of truth (datastorage_bootstrap.go)
- Consistent patterns across all services

---

## 📚 **Reference Documentation**

- [Gateway Migration Complete](./GATEWAY_MIGRATION_TO_SHARED_INFRA_COMPLETE_DEC_23_2025.md) - Reference pattern
- [DD-TEST-001 v1.3](../architecture/decisions/DD-TEST-001-unique-container-image-tags.md) - Image tag compliance
- [DD-TEST-002](../architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md) - Sequential startup pattern
- [Shared Infrastructure API](./DD_TEST_001_V13_COMPLETE_DEC_22_2025.md) - DSBootstrapConfig usage

---

## 🔧 **Migration Template**

```go
// Before (Custom Infrastructure)
err = infrastructure.StartServiceIntegrationInfrastructure(GinkgoWriter)

// After (Shared Infrastructure)
dsCfg := infrastructure.DSBootstrapConfig{
    ServiceName:     "service",
    PostgresPort:    15XXX,
    RedisPort:       16XXX,
    DataStoragePort: 18XXX,
    MetricsPort:     19XXX,
    ConfigDir:       "test/integration/service/config",
}
dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)

// Teardown
infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
```

---

## 📈 **Impact Summary**

### **Before Migration**
- 5 services with duplicate infrastructure code (~2,410 lines)
- 2 services violating DD-TEST-002 (shell scripts)
- 1 service violating DD-TEST-002 (podman-compose)
- Inconsistent image tag formats
- Manual image cleanup

### **After Migration**
- 6 services using shared infrastructure (~80 lines total)
- 100% DD-TEST-002 compliance
- 100% DD-TEST-001 v1.3 compliance
- Automatic image cleanup
- **97% code reduction** (2,330 lines eliminated)

---

**Prepared by**: AI Assistant
**Review Status**: 🔍 Triage complete
**Next**: Begin Phase 1 simple migrations
**Total Effort**: 5-6 hours for all 5 services









