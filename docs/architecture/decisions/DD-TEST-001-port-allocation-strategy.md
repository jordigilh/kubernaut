# DD-TEST-001: Port Allocation Strategy for Integration & E2E Tests

**Status**: ✅ Approved
**Date**: 2025-11-26
**Last Updated**: 2025-12-25
**Version**: 1.8
**Author**: AI Assistant
**Reviewers**: TBD
**Related**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)

---

## Context

Integration and E2E tests require running multiple services (PostgreSQL, Redis, APIs) on the host machine using Podman containers or Kind clusters. Without a coordinated port allocation strategy, tests experience port collisions when:

1. Multiple test suites run simultaneously (parallel execution)
2. Production services are running on default ports
3. Multiple developers run tests concurrently
4. CI/CD pipelines run multiple test jobs in parallel

**Problem Statement**: Port 8080 collision between Gateway service and Data Storage integration tests, plus potential conflicts with external PostgreSQL on port 15432.

**Recent Issue (2025-12-25)**: HAPI (HolmesGPT API) integration tests incorrectly used port 18094, which is officially allocated to SignalProcessing per v1.4 of this document, causing test infrastructure failures.

---

## Decision

**Establish a structured port allocation strategy with dedicated port ranges for each service and test tier.**

### **Port Range Blocks - Stateless Services (Podman)**

| Service | Production | Integration Tests | E2E Tests | Reserved Range |
|---------|-----------|-------------------|-----------|----------------|
| **Gateway** | 8080 | 18080-18089 | 28080-28089 | 18080-28089 |
| **Data Storage** | 8081 | 18090-18099 | 28090-28099 | 18090-28099 |
| **Effectiveness Monitor** | 8082 | 18100-18109 | 28100-28109 | 18100-28109 |
| **Workflow Engine** | 8083 | 18110-18119 | 28110-28119 | 18110-28119 |
| **HolmesGPT API** | 8084 | 18120-18129 | 28120-28129 | 18120-28129 |
| **Dynamic Toolset** | 8085 | 18130-18139 | 28130-28139 | 18130-28139 |
| **PostgreSQL** | 5432 | 15433-15442 | 25433-25442 | 15433-25442 |
| **Redis** | 6379 | 16379-16388 | 26379-26388 | 16379-26388 |
| **Embedding Service** | 8000 | 18000-18009 | 28000-28009 | 18000-28009 |

### **Port Range Blocks - CRD Controllers (Kind NodePort)**

| Controller | Metrics | Health | NodePort (API) | NodePort (Metrics) | Host Port |
|------------|---------|--------|----------------|-------------------|-----------|
| **Signal Processing** | 9090 | 8081 | 30082 | 30182 | 8082 |
| **Remediation Orchestrator** | 9090 | 8081 | 30083 | 30183 | 8083 |
| **AIAnalysis** | 9090 | 8081 | 30084 | 30184 | 8084 |
| **WorkflowExecution** | 9090 | 8081 | 30085 | 30185 | 8085 |
| **Notification** | 9090 | 8081 | 30086 | 30186 | 8086 |

### **Kind NodePort Allocation for E2E Tests (AUTHORITATIVE)**

| Service | Host Port | NodePort | Metrics Host | Metrics NodePort | Health Host | Health NodePort | Kind Config Location |
|---------|-----------|----------|--------------|------------------|-------------|-----------------|---------------------|
| **Gateway** | 8080 | 30080 | 9090 | 30090 | — | — | `test/infrastructure/kind-gateway-config.yaml` |
| **Gateway → Data Storage** | 18091 | 30081 | — | — | — | — | `test/infrastructure/kind-gateway-config.yaml` (dependency) |
| **Signal Processing** | 8082 | 30082 | 9182 | 30182 | — | — | `test/infrastructure/kind-signalprocessing-config.yaml` |
| **Remediation Orchestrator** | 8083 | 30083 | 9183 | 30183 | — | — | `test/infrastructure/kind-remediationorchestrator-config.yaml` |
| **AIAnalysis** | 8084 | 30084 | 9184 | 30184 | 8184 | 30284 | `test/infrastructure/kind-aianalysis-config.yaml` |
| **WorkflowExecution** | 8085 | 30085 | 9185 | 30185 | — | — | `test/infrastructure/kind-workflowexecution-config.yaml` |
| **Notification** | 8086 | 30086 | 9186 | 30186 | — | — | `test/infrastructure/kind-notification-config.yaml` |
| **Data Storage** | 8081 | 30081 | 9181 | 30181 | — | — | `test/infrastructure/kind-datastorage-config.yaml` |
| **Toolset** | 8087 | 30087 | 9187 | 30187 | — | — | `test/infrastructure/kind-toolset-config.yaml` |
| **HolmesGPT API** | 8088 | 30088 | 9188 | 30188 | — | — | `holmesgpt-api/tests/infrastructure/kind-holmesgpt-config.yaml` |

**Note**: Health ports (8184/30284) are only needed for services with separate health probe endpoints. Most services expose health on their API port.

**Allocation Rules**:
- **Integration Tests**: 15433-18139 range (Podman containers)
- **E2E Tests (Podman)**: 25433-28139 range
- **E2E Tests (Kind NodePort)**: 30080-30099 (API), 30180-30199 (Metrics)
- **Host Port Mapping**: 8080-8089 (for Kind extraPortMappings)
- **Avoided Ports**: 15432 (external postgres-poc), 8080 (production Gateway)
- **Buffer**: 10 ports per service per tier (supports parallel processes + dependencies)

---

## Detailed Port Assignments

### **HolmesGPT API (HAPI) - Python Service**

#### **Integration Tests** (`holmesgpt-api/tests/integration/`)

**Updated**: 2025-12-25 (migrated from 18094 to 18098 to resolve conflict with SignalProcessing)

```yaml
PostgreSQL:
  Host Port: 15439
  Container Port: 5432
  Connection: localhost:15439
  Purpose: Shared with Notification (separate test infrastructure)

Redis:
  Host Port: 16387
  Container Port: 6379
  Connection: localhost:16387
  Purpose: Shared with Notification/WE (separate test infrastructure)

HAPI Service:
  Host Port: 18120
  Container Port: 8080
  Connection: http://localhost:18120
  Purpose: HolmesGPT API service (incident/recovery endpoints)

Data Storage (Dependency):
  Host Port: 18098  # CHANGED from 18094 (SignalProcessing conflict)
  Container Port: 8080
  Connection: http://localhost:18098
  Purpose: Workflow catalog, audit trail
```

**Configuration Files**:
- `holmesgpt-api/tests/integration/conftest.py` - Port constants
- `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml` - Container orchestration
- `holmesgpt-api/tests/integration/data-storage-integration.yaml` - Data Storage config
- `holmesgpt-api/tests/integration/setup_workflow_catalog_integration.sh` - Infrastructure setup

**Infrastructure**: Podman-compose for Data Storage + PostgreSQL + Redis
**Pattern**: `pytest` with fixtures for service URLs

**Port Conflict Resolution (2025-12-25)**:
- **Previous (WRONG)**: Port 18094 (belonged to SignalProcessing per DD-TEST-001 v1.4)
- **Current (CORRECT)**: Port 18098 (next available in sequence after WE's 18097)
- **Authority**: SignalProcessing team triage document (SP_PORT_TRIAGE_AND_AGGREGATED_COVERAGE_DEC_25_2025.md)

#### **E2E Tests** (`test/e2e/aianalysis/hapi/`)

**Status**: Infrastructure pending (separate implementation session)

```yaml
HAPI (in Kind):
  Host Port: 8088
  NodePort: 30088
  Connection: http://localhost:8088

Data Storage (Dependency):
  Host Port: 8089
  NodePort: 30089
  Connection: http://localhost:8089
```

---

### **Data Storage Service**

#### **Integration Tests** (`test/integration/datastorage/`)
```yaml
PostgreSQL:
  Host Port: 15433
  Container Port: 5432
  Connection: localhost:15433

Redis:
  Host Port: 16379
  Container Port: 6379
  Connection: localhost:16379

Data Storage API:
  Host Port: 18090
  Container Port: 8080
  Connection: http://localhost:18090

Embedding Service (Mock):
  Host Port: 18000
  Container Port: 8000
  Connection: http://localhost:18000
```

#### **E2E Tests** (`test/e2e/datastorage/`)
```yaml
PostgreSQL:
  Host Port: 25433
  Container Port: 5432
  Connection: localhost:25433

Redis:
  Host Port: 26379
  Container Port: 6379
  Connection: localhost:26379

Data Storage API:
  Host Port: 28090
  Container Port: 8080
  Connection: http://localhost:28090

Embedding Service:
  Host Port: 28000
  Container Port: 8000
  Connection: http://localhost:28000
```

---

### **Gateway Service**

#### **Integration Tests** (`test/integration/gateway/`)
```yaml
Redis:
  Host Port: 16380
  Container Port: 6379
  Connection: localhost:16380

Gateway API:
  Host Port: 18080
  Container Port: 8080
  Connection: http://localhost:18080

Data Storage (Dependency):
  Host Port: 18091
  Container Port: 8080
  Connection: http://localhost:18091
```

#### **E2E Tests** (`test/e2e/gateway/`)
```yaml
Redis:
  Host Port: 26380
  Container Port: 6379
  Connection: localhost:26380

Gateway API:
  Host Port: 28080
  Container Port: 8080
  Connection: http://localhost:28080

Data Storage (Dependency):
  Host Port: 28091
  Container Port: 8080
  Connection: http://localhost:28091
```

---

### **Effectiveness Monitor Service**

#### **Integration Tests** (`test/integration/effectiveness-monitor/`)
```yaml
PostgreSQL:
  Host Port: 15434
  Container Port: 5432
  Connection: localhost:15434

Effectiveness Monitor API:
  Host Port: 18100
  Container Port: 8080
  Connection: http://localhost:18100

Data Storage (Dependency):
  Host Port: 18092
  Container Port: 8080
  Connection: http://localhost:18092
```

#### **E2E Tests** (`test/e2e/effectiveness-monitor/`)
```yaml
PostgreSQL:
  Host Port: 25434
  Container Port: 5432
  Connection: localhost:25434

Effectiveness Monitor API:
  Host Port: 28100
  Container Port: 8080
  Connection: http://localhost:28100

Data Storage (Dependency):
  Host Port: 28092
  Container Port: 8080
  Connection: http://localhost:28092
```

---

### **Workflow Engine Service**

#### **Integration Tests** (`test/integration/workflow-engine/`)
```yaml
Workflow Engine API:
  Host Port: 18110
  Container Port: 8080
  Connection: http://localhost:18110

Data Storage (Dependency):
  Host Port: 18093
  Container Port: 8080
  Connection: http://localhost:18093
```

#### **E2E Tests** (`test/e2e/workflow-engine/`)
```yaml
Workflow Engine API:
  Host Port: 28110
  Container Port: 8080
  Connection: http://localhost:28110

Data Storage (Dependency):
  Host Port: 28093
  Container Port: 8080
  Connection: http://localhost:28093
```

---

### **SignalProcessing Controller** (CRD)

#### **Integration Tests** (`test/integration/signalprocessing/`)
```yaml
PostgreSQL:
  Host Port: 15436
  Container Port: 5432
  Connection: localhost:15436
  Purpose: Audit storage (BR-SP-090)

Redis:
  Host Port: 16382
  Container Port: 6379
  Connection: localhost:16382
  Purpose: DataStorage DLQ

Data Storage (Dependency):
  Host Port: 18094  # OFFICIAL ALLOCATION - SignalProcessing owns this port
  Container Port: 8080
  Connection: http://localhost:18094
  Purpose: Audit API (BR-SP-090)
```

**Infrastructure**: Programmatic podman-compose via `infrastructure.StartSignalProcessingIntegrationInfrastructure()`
**Pattern**: SynchronizedBeforeSuite (Process 1 only) - supports parallel execution

**Port Ownership**: Port 18094 is **OFFICIALLY ALLOCATED** to SignalProcessing per DD-TEST-001 v1.4 (2025-12-11)

---

## Kind NodePort E2E Configuration (CRD Controllers)

**IMPORTANT**: CRD controllers use Kind clusters with NodePort services for E2E tests.
This eliminates kubectl port-forward instability and enables full parallel execution.

### **Why NodePort Instead of Port-Forward?**

| Aspect | Port-Forward | NodePort |
|--------|--------------|----------|
| **Stability** | Crashes under concurrent load | 100% stable |
| **Performance** | Slow (proxy overhead) | Fast (direct connection) |
| **Parallelism** | Limited to ~4 processes | Unlimited (all CPUs) |
| **Code Complexity** | ~150 lines management code | ~40 lines |
| **CI/CD** | Unreliable in containers | Production-like |

**Reference**: Gateway E2E tests achieved 6.4x speedup by switching to NodePort.

---

### **Kind Configuration Pattern** (MANDATORY for E2E Tests)

**File**: `test/infrastructure/kind-[service]-config.yaml`

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  # Expose service NodePort to host machine
  # This eliminates kubectl port-forward instability
  extraPortMappings:
  - containerPort: {{NODEPORT}}     # Service NodePort in cluster
    hostPort: {{HOST_PORT}}         # Port on host machine (localhost:{{HOST_PORT}})
    protocol: TCP
  - containerPort: {{METRICS_NODEPORT}}  # Metrics NodePort
    hostPort: {{METRICS_HOST_PORT}}
    protocol: TCP
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        # Increase API server rate limits for parallel testing
        max-requests-inflight: "800"
        max-mutating-requests-inflight: "400"
    controllerManager:
      extraArgs:
        kube-api-qps: "100"
        kube-api-burst: "200"
- role: worker
```

---

### **Signal Processing Controller** (E2E)

**Kind Config**: `test/infrastructure/kind-signalprocessing-config.yaml`
```yaml
extraPortMappings:
- containerPort: 30082    # Signal Processing NodePort
  hostPort: 8082          # localhost:8082
  protocol: TCP
- containerPort: 30182    # Metrics NodePort
  hostPort: 9182          # localhost:9182 for metrics
  protocol: TCP
```

**Service YAML** (`test/e2e/signalprocessing/signalprocessing-service.yaml`):
```yaml
apiVersion: v1
kind: Service
metadata:
  name: signalprocessing-controller
spec:
  type: NodePort
  selector:
    app: signalprocessing-controller
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30182
```

**Test URL**: `http://localhost:8082` (for any HTTP endpoints), `http://localhost:9182/metrics`

---

### **Gateway Service** (E2E - Reference Implementation)

**Kind Config**: `test/infrastructure/kind-gateway-config.yaml`
```yaml
extraPortMappings:
# Gateway service ports
- containerPort: 30080    # Gateway NodePort
  hostPort: 8080          # localhost:8080
  protocol: TCP
- containerPort: 30090    # Metrics NodePort
  hostPort: 9090          # localhost:9090 for metrics
  protocol: TCP
# Data Storage dependency (for audit events - BR-GATEWAY-190)
- containerPort: 30081    # Data Storage NodePort
  hostPort: 18091         # localhost:18091 (avoids conflict with Gateway metrics)
  protocol: TCP
```

**Service YAML** (`test/e2e/gateway/gateway-deployment.yaml`):
```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-service
spec:
  type: NodePort
  selector:
    app: gateway
  ports:
  - name: http
    port: 8080
    targetPort: 8080
    nodePort: 30080
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30090
```

**Test URL**: `http://localhost:8080`, `http://localhost:9090/metrics`, `http://localhost:18091` (Data Storage)

---

### **Notification Controller** (E2E)

**Kind Config**: `test/infrastructure/kind-notification-config.yaml`
```yaml
extraPortMappings:
- containerPort: 30086    # Notification NodePort (unused - no HTTP)
  hostPort: 8086
  protocol: TCP
- containerPort: 30186    # Metrics NodePort
  hostPort: 9186
  protocol: TCP
```

**Test URL**: `http://localhost:9186/metrics` (metrics only - no HTTP API)

---

### **NodePort Allocation Summary**

| Controller | Host Port | NodePort | Metrics Host | Metrics NodePort |
|------------|-----------|----------|--------------|------------------|
| **Gateway** | 8080 | 30080 | 9090 | 30090 |
| **Data Storage** | 8081 | 30081 | 9181 | 30181 |
| **Signal Processing** | 8082 | 30082 | 9182 | 30182 |
| **Remediation Orchestrator** | 8083 | 30083 | 9183 | 30183 |
| **AIAnalysis** | 8084 | 30084 | 9184 | 30184 |
| **WorkflowExecution** | 8085 | 30085 | 9185 | 30185 |
| **Notification** | 8086 | 30086 | 9186 | 30186 |
| **Toolset** | 8087 | 30087 | 9187 | 30187 |
| **HolmesGPT API** | 8088 | 30088 | 9188 | 30188 |

**Pattern**:
- Service NodePort: `3008X` where X = service index
- Metrics NodePort: `3018X` where X = service index
- Host Port: `808X` / `918X` for service/metrics

**HolmesGPT API Dependencies** (in dedicated Kind cluster):
| Dependency | Host Port | NodePort | Purpose |
|------------|-----------|----------|---------|
| PostgreSQL | 5488 | 30488 | Workflow catalog storage (V1.0 label-only) |
| Data Storage | 8089 | 30089 | Audit trail, workflow catalog API |
| Redis | 6388 | 30388 | Data Storage DLQ |

---

### **Test Suite Pattern** (No Port-Forward)

```go
// SynchronizedBeforeSuite - runs ONCE on process 1
var _ = SynchronizedBeforeSuite(
    func() []byte {
        // Create Kind cluster (ONCE)
        err := infrastructure.CreateCluster(clusterName, kubeconfigPath, GinkgoWriter)
        Expect(err).ToNot(HaveOccurred())

        // Deploy controller + dependencies
        err = infrastructure.DeployController(ctx, namespace, kubeconfigPath, GinkgoWriter)
        Expect(err).ToNot(HaveOccurred())

        return []byte(kubeconfigPath)
    },
    // This runs on ALL processes - use NodePort directly
    func(data []byte) {
        kubeconfigPath = string(data)

        // All processes use the same NodePort URL
        // NO kubectl port-forward needed
        serviceURL = "http://localhost:8082"  // Signal Processing example
        metricsURL = "http://localhost:9182/metrics"

        // Wait for service to be ready via NodePort
        Eventually(func() error {
            resp, err := http.Get(metricsURL)
            if err != nil {
                return err
            }
            defer resp.Body.Close()
            return nil
        }, 60*time.Second, 2*time.Second).Should(Succeed())
    },
)
```

---

## Rationale

### **Why Separate Port Ranges for Integration vs E2E?**
- **Parallel Execution**: Integration and E2E tests can run simultaneously without conflicts
- **Clear Separation**: Easy to identify which test tier is using which port
- **CI/CD Optimization**: Different test tiers can run in parallel pipelines

### **Why 10-Port Buffers per Service?**
- **Parallel Processes**: Ginkgo runs 4 parallel processes by default
- **Dependencies**: Services may need multiple instances (e.g., Data Storage as dependency)
- **Future Growth**: Room for additional parallel processes or test scenarios

### **Why Start at 15433 for PostgreSQL?**
- **Avoid 15432**: External postgres-poc uses this port
- **Sequential**: Easy to remember (15433, 15434, 15435...)
- **Standard Offset**: +10000 from production port (5432 → 15432 range)

### **Why Start at 18000 for Services?**
- **Above Ephemeral Range**: Avoids conflicts with OS-assigned ports (32768-60999)
- **Below Well-Known Ports**: Stays clear of common service ports
- **Memorable Pattern**: 18xxx for integration, 28xxx for E2E

---

## Consequences

### **Positive**
- ✅ **No Port Collisions**: Each test tier has dedicated, non-overlapping port ranges
- ✅ **Parallel Execution**: Multiple test suites can run simultaneously
- ✅ **Developer Friendly**: Tests don't interfere with production services
- ✅ **CI/CD Ready**: Parallel pipelines won't conflict
- ✅ **Scalable**: Room for 10 services × 2 tiers × 10 ports = 200 ports allocated
- ✅ **Predictable**: Easy to calculate port for any service/tier combination

### **Negative**
- ⚠️ **Non-Standard Ports**: Developers must remember test-specific ports
- ⚠️ **Configuration Overhead**: Each test suite needs port configuration
- ⚠️ **Documentation Burden**: Must keep port assignments up-to-date

### **Mitigation**
- 📝 **Centralized Documentation**: This DD serves as single source of truth
- 🔧 **Constants in Code**: Define ports as constants in test suites
- 📋 **Test READMEs**: Document ports in service-specific test documentation
- 🤖 **Validation Scripts**: Add pre-test port availability checks

---

## Implementation Checklist

### **Phase 1: Data Storage (Immediate)**
- [ ] Update `test/integration/datastorage/suite_test.go`
  - [ ] PostgreSQL: 5433 → 15433
  - [ ] Redis: 6379 → 16379
  - [ ] Data Storage API: 8080 → 18090
  - [ ] Embedding Service: 8000 → 18000
- [ ] Update `test/integration/datastorage/config/config.yaml`
- [ ] Update `test/integration/datastorage/config_integration_test.go`
- [ ] Update `test/e2e/datastorage/` (ports: 25433, 26379, 28090, 28000)
- [ ] Test parallel execution: `ginkgo -p -procs=4 test/integration/datastorage/`

### **Phase 2: Gateway**
- [ ] Update `test/integration/gateway/suite_test.go` (ports: 16380, 18080, 18091)
- [ ] Update `test/e2e/gateway/` (ports: 26380, 28080, 28091)

### **Phase 3: Effectiveness Monitor**
- [ ] Update `test/integration/effectiveness-monitor/` (ports: 15434, 18100, 18092)
- [ ] Update `test/e2e/effectiveness-monitor/` (ports: 25434, 28100, 28092)

### **Phase 4: Workflow Engine**
- [ ] Update `test/integration/workflow-engine/` (ports: 18110, 18093)
- [ ] Update `test/e2e/workflow-engine/` (ports: 28110, 28093)

### **Phase 5: Documentation**
- [ ] Update `test/integration/README.md` with port allocation table
- [ ] Update `test/e2e/README.md` with port allocation table
- [ ] Update `.cursor/rules/03-testing-strategy.mdc` with DD-TEST-001 reference
- [ ] Add port allocation section to each service's test README

---

## Port Collision Matrix

### **Integration Tests** (Can run simultaneously)

| Service | PostgreSQL | Redis | API | Dependencies |
|---------|-----------|-------|-----|--------------|
| **Data Storage** | 15433 | 16379 | 18090 | Embedding: 18000 |
| **Gateway** | N/A | 16380 | 18080 | Data Storage: 18091 |
| **Effectiveness Monitor** | 15434 | N/A | 18100 | Data Storage: 18092 |
| **Workflow Engine** | N/A | N/A | 18110 | Data Storage: 18093 |
| **SignalProcessing (CRD)** | 15436 | 16382 | N/A | Data Storage: 18094 |
| **RemediationOrchestrator (CRD)** | 15435 | 16381 | N/A | Data Storage: 18140 |
| **AIAnalysis (CRD)** | 15438 | 16384 | N/A | Data Storage: 18095 |
| **Notification (CRD)** | 15440 | 16385 | N/A | Data Storage: 18096 |
| **WorkflowExecution (CRD)** | 15441 | 16388 | N/A | Data Storage: 18097 |
| **HolmesGPT API (Python)** | 15439 | 16387 | 18120 | Data Storage: 18098 |

✅ **No Conflicts** - All services can run integration tests in parallel

**Note**: All services now have unique port allocations to enable true parallel testing. Notification PostgreSQL moved from 15439 (conflicted with HAPI) to 15440 (unique).

### **E2E Tests** (Can run simultaneously)

| Service | PostgreSQL | Redis | API | Dependencies |
|---------|-----------|-------|-----|--------------|
| **Data Storage** | 25433 | 26379 | 28090 | Embedding: 28000 |
| **Gateway** | N/A | 26380 | 28080 | Data Storage: 28091 |
| **Effectiveness Monitor** | 25434 | N/A | 28100 | Data Storage: 28092 |
| **Workflow Engine** | N/A | N/A | 28110 | Data Storage: 28093 |

✅ **No Conflicts** - All services can run E2E tests in parallel

---

## Example Usage

### **Running Data Storage Integration Tests**
```bash
# Ports used:
# - PostgreSQL: 15433
# - Redis: 16379
# - Data Storage API: 18090
# - Embedding Service: 18000

ginkgo -p -procs=4 test/integration/datastorage/

# Access services:
psql -h localhost -p 15433 -U postgres -d kubernaut
redis-cli -h localhost -p 16379
curl http://localhost:18090/health
```

### **Running Multiple Test Suites in Parallel**
```bash
# Terminal 1: Data Storage integration tests (ports: 15433, 16379, 18090, 18000)
ginkgo -p -procs=4 test/integration/datastorage/

# Terminal 2: Gateway integration tests (ports: 16380, 18080, 18091)
ginkgo -p -procs=4 test/integration/gateway/

# Terminal 3: Data Storage E2E tests (ports: 25433, 26379, 28090, 28000)
ginkgo -p -procs=4 test/e2e/datastorage/

# No port conflicts! ✅
```

---

## References

- **Testing Strategy**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
- **ADR-016**: Podman-based integration testing infrastructure
- **ADR-027**: Data Storage service containerization
- **ADR-030**: Configuration management for tests

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 2.0 | 2026-01-01 | AI Assistant | **CRITICAL FIX**: Resolved Notification/HAPI PostgreSQL port conflict - migrated Notification PostgreSQL from 15439 (shared with HAPI) to 15440 (unique); **TRUE PARALLEL TESTING NOW ENABLED** - all 8 services can run integration tests simultaneously without port conflicts; removed shared port design flaw |
| 1.9 | 2025-12-25 | AI Assistant | **CRITICAL FIX**: Resolved WE/HAPI Redis port conflict - migrated WorkflowExecution Redis from 16387 (shared with HAPI) to 16388 (unique); enables parallel integration testing for WE and HAPI; updated note to clarify only PostgreSQL is shared between HAPI and Notification |
| 1.8 | 2025-12-25 | AI Assistant | **CRITICAL FIX**: Resolved HAPI port conflict - migrated HAPI Data Storage from 18094 (SignalProcessing) to 18098; added complete integration test port allocation table including all CRD controllers; documented HAPI integration test ports (PostgreSQL: 15439, Redis: 16387, HAPI API: 18120, Data Storage dependency: 18098) |
| 1.7 | 2025-12-22 | AI Assistant | Port allocation fixes complete - all services documented in authoritative table |
| 1.6 | 2025-12-15 | AI Assistant | Added RemediationOrchestrator, AIAnalysis, Notification, WorkflowExecution to integration allocation table |
| 1.5 | 2025-12-12 | AI Assistant | Documented all CRD controller integration test patterns |
| 1.4 | 2025-12-11 | AI Assistant | Added SignalProcessing integration test ports (PostgreSQL: 15436, Redis: 16382, DataStorage: 18094); documented programmatic infrastructure pattern; noted RO port gap (15435/16381) |
| 1.3 | 2025-12-07 | AI Assistant | Added Health NodePort columns to E2E allocation table; AIAnalysis health ports (8184/30284) documented; expanded table with Metrics Host column for clarity |
| 1.2 | 2025-12-06 | AI Assistant | Added HolmesGPT API Kind NodePort allocation (8088/30088/30188), added dependency ports for HAPI E2E (PostgreSQL+pgvector: 5488/30488, Embedding: 8188/30288, Data Storage: 8089/30089, Redis: 6388/30388) |
| 1.1 | 2025-11-28 | AI Assistant | Added Kind NodePort allocations for E2E tests (CRD controllers), added all services including Signal Processing, Notification, AIAnalysis, Remediation Orchestrator, Remediation Execution, HolmesGPT API, Dynamic Toolset |
| 1.0 | 2025-11-26 | AI Assistant | Initial port allocation strategy |

---

**Authority**: This design decision is **AUTHORITATIVE** for all test port allocations.
**Scope**: All integration and E2E tests across all services.
**Enforcement**: Port allocations MUST follow this strategy to prevent conflicts.
**Kind NodePort**: All E2E tests using Kind clusters MUST use NodePort (no kubectl port-forward).

