# Final 3 Services - Batch Migration Guide

**Date**: December 23, 2025
**Status**: 🎯 **3/6 Services Migrated - Final Push**
**Progress**: 50% Complete

---

## ✅ **Completed Migrations** (3/6)

1. ✅ **Gateway** - Migrated + Tested (100 tests passing)
2. ✅ **RemediationOrchestrator** - Migrated (build successful)
3. ✅ **SignalProcessing** - Migrated (build successful)

**Code Eliminated**: ~1,210 lines

---

## 🚧 **Remaining Services** (3)

| Service | Type | Time | Status |
|---------|------|------|--------|
| **WorkflowExecution** | Shell script → Go | 30-45 min | 🔴 Pending |
| **Notification** | Shell script → Go | 30-45 min | 🔴 Pending |
| **AIAnalysis** | Complex (HAPI) | 60-90 min | 🔴 Pending |

---

## 3️⃣ **WorkflowExecution Migration**

### **Current State**
- Uses: `setup-infrastructure.sh` ❌ (DD-TEST-002 violation)
- Ports: 15441, 16387, 18097, 19097

### **Migration Steps**

#### **1. Update suite_test.go**

```go
// Add to package-level variables
var dsInfra *infrastructure.DSBootstrapInfra

// In BeforeSuite, replace shell script with:
By("Starting WorkflowExecution infrastructure using shared DS bootstrap")
dsCfg := infrastructure.DSBootstrapConfig{
    ServiceName:     "workflowexecution",
    PostgresPort:    infrastructure.WEIntegrationPostgresPort,    // 15441
    RedisPort:       infrastructure.WEIntegrationRedisPort,       // 16387
    DataStoragePort: infrastructure.WEIntegrationDataStoragePort, // 18097
    MetricsPort:     infrastructure.WEIntegrationMetricsPort,     // 19097
    ConfigDir:       "test/integration/workflowexecution/config",
}
var err error
dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
Expect(err).ToNot(HaveOccurred())

// In AfterSuite:
if dsInfra != nil {
    _ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
}
```

#### **2. Update config.yaml**

```yaml
database:
  host: workflowexecution_postgres_test  # Was: workflowexecution_postgres_1
  port: 5432
  name: action_history
  user: slm_user

redis:
  addr: workflowexecution_redis_test:6379  # Was: workflowexecution_redis_1:6379
```

#### **3. Delete shell scripts**

```bash
rm test/integration/workflowexecution/setup-infrastructure.sh
rm test/integration/workflowexecution/teardown-infrastructure.sh
```

#### **4. Verify**

```bash
go test -c ./test/integration/workflowexecution/...
```

---

## 4️⃣ **Notification Migration**

### **Current State**
- Uses: `setup-infrastructure.sh` ❌ (DD-TEST-002 violation)
- Ports: 15439, 16385, 18096, 19096

### **Migration Steps**

#### **1. Update suite_test.go**

```go
// Add to package-level variables
var dsInfra *infrastructure.DSBootstrapInfra

// In BeforeSuite, replace shell script with:
By("Starting Notification infrastructure using shared DS bootstrap")
dsCfg := infrastructure.DSBootstrapConfig{
    ServiceName:     "notification",
    PostgresPort:    infrastructure.NTIntegrationPostgresPort,    // 15439
    RedisPort:       infrastructure.NTIntegrationRedisPort,       // 16385
    DataStoragePort: infrastructure.NTIntegrationDataStoragePort, // 18096
    MetricsPort:     infrastructure.NTIntegrationMetricsPort,     // 19096
    ConfigDir:       "test/integration/notification/config",
}
var err error
dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
Expect(err).ToNot(HaveOccurred())

// In AfterSuite:
if dsInfra != nil {
    _ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
}
```

#### **2. Update config.yaml**

```yaml
database:
  host: notification_postgres_test  # Was: notification_postgres_1
  port: 5432
  name: action_history
  user: slm_user

redis:
  addr: notification_redis_test:6379  # Was: notification_redis_1:6379
```

#### **3. Delete shell scripts**

```bash
rm test/integration/notification/setup-infrastructure.sh
```

#### **4. Verify**

```bash
go test -c ./test/integration/notification/...
```

---

## 5️⃣ **AIAnalysis Migration** (Complex)

### **Current State**
- Uses: `StartAIAnalysisIntegrationInfrastructure` (custom Go)
- **Special**: Needs HAPI (HolmesGPT API) container
- Ports: 15438, 16384, 18095, 19095, 18098 (HAPI)

### **Migration Steps**

#### **1. Update suite_test.go - DS Bootstrap**

```go
// Add to package-level variables
var (
    dsInfra    *infrastructure.DSBootstrapInfra
    hapiInstance *infrastructure.ContainerInstance
)

// In BeforeSuite, replace custom infrastructure with:
By("Starting AIAnalysis infrastructure using shared DS bootstrap")
dsCfg := infrastructure.DSBootstrapConfig{
    ServiceName:     "aianalysis",
    PostgresPort:    infrastructure.AIAnalysisIntegrationPostgresPort,    // 15438
    RedisPort:       infrastructure.AIAnalysisIntegrationRedisPort,       // 16384
    DataStoragePort: infrastructure.AIAnalysisIntegrationDataStoragePort, // 18095
    MetricsPort:     infrastructure.AIAnalysisIntegrationMetricsPort,     // 19095
    ConfigDir:       "test/integration/aianalysis/config",
}
var err error
dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
Expect(err).ToNot(HaveOccurred())
```

#### **2. Add HAPI Setup**

```go
// After DS bootstrap succeeds
By("Starting HAPI (HolmesGPT API) service")
hapiConfig := infrastructure.GenericContainerConfig{
    ContainerName: "aianalysis_hapi_test",
    Image:         infrastructure.GenerateInfraImageName("holmesgpt-api", "aianalysis"),
    BuildContext:  getProjectRoot(),
    Dockerfile:    "deployments/holmesgpt-api/Dockerfile",  // Adjust path as needed
    Network:       "aianalysis_test_network",
    Ports: map[int]int{
        8080: infrastructure.AIAnalysisIntegrationHAPIPort, // 18098
    },
    Env: map[string]string{
        "MOCK_LLM": "true",  // Use mock LLM for tests
        "LOG_LEVEL": "debug",
    },
    HealthCheck: &infrastructure.HealthCheckConfig{
        URL:     fmt.Sprintf("http://localhost:%d/health", infrastructure.AIAnalysisIntegrationHAPIPort),
        Timeout: 60 * time.Second,
    },
}
hapiInstance, err = infrastructure.StartGenericContainer(hapiConfig, GinkgoWriter)
Expect(err).ToNot(HaveOccurred(), "HAPI must start successfully")
GinkgoWriter.Println("✅ HAPI service started")
```

#### **3. Update AfterSuite**

```go
// Stop HAPI first
if hapiInstance != nil {
    By("Stopping HAPI service")
    _ = infrastructure.StopGenericContainer(hapiInstance, GinkgoWriter)
}

// Then stop DS infrastructure
if dsInfra != nil {
    By("Stopping AIAnalysis infrastructure")
    _ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
}
```

#### **4. Update config.yaml**

```yaml
database:
  host: aianalysis_postgres_test
  port: 5432
  name: action_history
  user: slm_user

redis:
  addr: aianalysis_redis_test:6379
```

#### **5. Delete podman-compose.yml**

```bash
rm test/integration/aianalysis/podman-compose.yml
```

#### **6. Verify**

```bash
go test -c ./test/integration/aianalysis/...
```

---

## 🧹 **Post-Migration Cleanup**

After all services are migrated and tested, clean up old infrastructure functions:

### **Files to Clean**

```bash
# RemediationOrchestrator
# Remove: StartROIntegrationInfrastructure, StopROIntegrationInfrastructure (~350 lines)
# Keep: E2E functions

# SignalProcessing
# Remove: StartSignalProcessingIntegrationInfrastructure, StopSignalProcessingIntegrationInfrastructure (~400 lines)
# Keep: E2E functions

# WorkflowExecution
# Delete: setup-infrastructure.sh, teardown-infrastructure.sh
# May need to keep: WEIntegration port constants

# Notification
# Delete: setup-infrastructure.sh
# Keep: Integration port constants, E2E functions

# AIAnalysis
# Remove: StartAIAnalysisIntegrationInfrastructure (~800 lines)
# Delete: podman-compose.yml
# Keep: E2E functions, HAPI constants
```

---

## ✅ **Final Validation Checklist**

For each migrated service:

```bash
# 1. Build test
go test -c ./test/integration/[service]/...

# 2. Run linter
golangci-lint run test/integration/[service]/...

# 3. Run integration tests (if infrastructure available)
go test -v ./test/integration/[service]/... -timeout 15m -ginkgo.v

# 4. Verify cleanup
podman ps  # Should show no [service]_*_test containers
podman images | grep [service]  # Should only show base images (postgres, redis)
```

---

## 📊 **Final Impact Summary**

### **Before Migration**
- 6 services with duplicate infrastructure code (~2,410 lines)
- 2 services violating DD-TEST-002 (shell scripts)
- 1 service violating DD-TEST-002 (podman-compose)
- Inconsistent patterns across services

### **After Migration**
- 6 services using shared infrastructure (~60 lines total)
- 100% DD-TEST-002 compliance
- 100% DD-TEST-001 v1.3 compliance
- **97% code reduction** (~2,350 lines eliminated)

---

## 🎯 **Estimated Time Remaining**

| Service | Time | Status |
|---------|------|--------|
| WorkflowExecution | 30 min | Pending |
| Notification | 30 min | Pending |
| AIAnalysis | 90 min | Pending |
| **Total** | **2.5 hours** | **50% Done** |

---

## 📚 **Reference Commands**

```bash
# Quick migration status check
for s in gateway remediationorchestrator signalprocessing workflowexecution notification aianalysis; do
  echo "=== $s ==="
  grep -q "DSBootstrapConfig" test/integration/$s/suite_test.go 2>/dev/null && echo "✅ Migrated" || echo "⏳ Pending"
done

# Find old infrastructure patterns
grep -r "setup-infrastructure.sh\|podman-compose\|Start.*IntegrationInfrastructure" test/integration/*/suite_test.go

# Verify all services build
go test -c ./test/integration/{gateway,remediationorchestrator,signalprocessing,workflowexecution,notification,aianalysis}/...
```

---

## 🔄 **Next: DataStorage E2E Infrastructure**

After all service migrations complete, update DataStorage E2E to use shared `BuildAndLoadImageToKind`:

```go
// In test/infrastructure/datastorage.go or test/e2e/datastorage/suite_test.go

// Current: Manual podman build + kind load
// Target: Shared helper

imageConfig := infrastructure.E2EImageConfig{
    ServiceName:      "datastorage",
    ImageName:        "kubernaut/datastorage",
    DockerfilePath:   "cmd/datastorage/Dockerfile",
    KindClusterName:  "datastorage-e2e",
    BuildContextPath: projectRoot,
}

dsImage, err := infrastructure.BuildAndLoadImageToKind(imageConfig, GinkgoWriter)
Expect(err).NotTo(HaveOccurred())

// Cleanup
defer infrastructure.CleanupE2EImage(dsImage, GinkgoWriter)
```

---

**Prepared by**: AI Assistant
**Review Status**: 🚀 Ready for final push
**Next**: Migrate WE → NT → AI → DS E2E update
**Total Remaining**: 2.5-3 hours









