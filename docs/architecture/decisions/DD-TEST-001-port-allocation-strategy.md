# DD-TEST-001: Port Allocation Strategy for Integration & E2E Tests

**Status**: ✅ Approved
**Date**: 2025-11-26
**Last Updated**: 2026-02-09
**Version**: 2.8
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
| **Mock LLM Service** | N/A | 18140-18149 | ClusterIP only | 18140-18149 |
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
| **Effectiveness Monitor** | 9090 | 8081 | 30089 | 30189 | 8089 |

### **Kind NodePort Allocation for E2E Tests (AUTHORITATIVE)**

| Service | Host Port | NodePort | Metrics Host | Metrics NodePort | Health Host | Health NodePort | Kind Config Location |
|---------|-----------|----------|--------------|------------------|-------------|-----------------|---------------------|
| **Gateway** | 8080 | 30080 | 9090 | 30090 | — | — | `test/infrastructure/kind-gateway-config.yaml` |
| **Gateway → Data Storage** | 18091 | 30081 | — | — | — | — | `test/infrastructure/kind-gateway-config.yaml` (dependency) |
| **Data Storage** | 8081 | 30081 | 9181 | 30181 | — | — | `test/infrastructure/kind-datastorage-config.yaml` |
| **Signal Processing** | 8082 | 30082 | 9182 | 30182 | — | — | `test/infrastructure/kind-signalprocessing-config.yaml` |
| **Remediation Orchestrator** | 8083 | 30083 | 9183 | 30183 | — | — | `test/infrastructure/kind-remediationorchestrator-config.yaml` |
| **RO → Data Storage** | 8090 | 30081 | — | — | — | — | `test/infrastructure/kind-remediationorchestrator-config.yaml` (dependency) |
| **AIAnalysis** | 8084 | 30084 | 9184 | 30184 | 8184 | 30284 | `test/infrastructure/kind-aianalysis-config.yaml` |
| **WorkflowExecution** | 8085 | 30085 | 9185 | 30185 | — | — | `test/infrastructure/kind-workflowexecution-config.yaml` |
| **WE → Data Storage** | 8092 | 30081 | — | — | — | — | `test/infrastructure/kind-workflowexecution-config.yaml` (dependency) |
| **Notification** | 8086 | 30086 | 9186 | 30186 | — | — | `test/infrastructure/kind-notification-config.yaml` |
| **Toolset** | 8087 | 30087 | 9187 | 30187 | — | — | `test/infrastructure/kind-toolset-config.yaml` |
| **HolmesGPT API** | 8088 | 30088 | 9188 | 30188 | — | — | `holmesgpt-api/tests/infrastructure/kind-holmesgpt-config.yaml` |
| **Effectiveness Monitor** | 8089 | 30089 | 9189 | 30189 | — | — | `test/infrastructure/kind-effectivenessmonitor-config.yaml` |
| **Full Pipeline E2E** | — | — | — | — | — | — | `test/infrastructure/kind-fullpipeline-config.yaml` |
| &nbsp;&nbsp;→ Gateway | 30080 | 30080 | — | — | — | — | (Gateway ingress for event-exporter webhook) |
| &nbsp;&nbsp;→ Data Storage | 30081 | 30081 | — | — | — | — | (DataStorage for workflow catalog seeding) |
| &nbsp;&nbsp;→ Mock LLM | — | ClusterIP | — | — | — | — | (Internal only - accessed by HAPI) |
| &nbsp;&nbsp;→ Prometheus | 9190 | 30190 | — | — | — | — | (EM metric comparison - remote write receiver) |
| &nbsp;&nbsp;→ AlertManager | 9193 | 30193 | — | — | — | — | (EM alert resolution queries) |

**Note**:
- Health ports (8184/30284) are only needed for services with separate health probe endpoints. Most services expose health on their API port.
- Mock LLM Service uses ClusterIP only in E2E (no NodePort needed - accessed only by services inside cluster)

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
  Connection: 127.0.0.1:15439
  Purpose: Shared with Notification (separate test infrastructure)

Redis:
  Host Port: 16387
  Container Port: 6379
  Connection: 127.0.0.1:16387
  Purpose: Shared with Notification/WE (separate test infrastructure)

HAPI Service:
  Host Port: 18120
  Container Port: 8080
  Connection: http://127.0.0.1:18120
  Purpose: HolmesGPT API service (incident/recovery endpoints)

Data Storage (Dependency):
  Host Port: 18098  # CHANGED from 18094 (SignalProcessing conflict)
  Container Port: 8080
  Connection: http://127.0.0.1:18098
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
  Connection: http://127.0.0.1:8088

Data Storage (Dependency):
  Host Port: 8089
  NodePort: 30089
  Connection: http://127.0.0.1:8089
```

---

### **Mock LLM Service** (Test Infrastructure)

#### **Integration Tests** (Service-Specific Ports)

**Created**: 2026-01-11 (Mock LLM Migration - MOCK_LLM_MIGRATION_PLAN.md v1.3.0)

```yaml
HAPI Integration Tests:
  Host Port: 18140
  Container Port: 8080
  Connection: http://127.0.0.1:18140
  Purpose: Mock OpenAI-compatible LLM for HAPI integration tests

AIAnalysis Integration Tests:
  Host Port: 18141
  Container Port: 8080
  Connection: http://127.0.0.1:18141
  Purpose: Mock OpenAI-compatible LLM for AIAnalysis integration tests
```

**Configuration Files**:
- `test/infrastructure/mock_llm.go` - Programmatic container lifecycle management
- `test/integration/holmesgptapi/suite_test.go` - HAPI integration suite (port 18140)
- `test/integration/aianalysis/suite_test.go` - AIAnalysis integration suite (port 18141)
- `test/services/mock-llm/` - Standalone Mock LLM service source

**Infrastructure**: Programmatic podman commands (no external dependencies)
**Pattern**: Ginkgo `SynchronizedBeforeSuite` with service-specific ports

**Port Allocation Rationale**:
- **18140 (HAPI)**: First port in Mock LLM range (18140-18149)
- **18141 (AIAnalysis)**: Second port in range - prevents collision during parallel execution
- **Per-Service Isolation**: Each integration test suite gets unique Mock LLM instance

**Service Purpose**: Provides deterministic OpenAI-compatible mock LLM responses for testing without real LLM calls. Replaces embedded mock logic in HolmesGPT-API business code (900 lines removed).

#### **E2E Tests** (`test/e2e/*/`)

**Status**: Infrastructure ready (Kind deployment)

```yaml
Mock LLM (in Kind):
  Namespace: kubernaut-system (shared with HAPI, DataStorage, etc.)
  Service Type: ClusterIP (internal only)
  Internal URL: http://mock-llm:8080 (simplified DNS - same namespace)
  Purpose: Shared Mock LLM for all E2E tests (HAPI, AIAnalysis, etc.)
  Access Pattern: HAPI/AIAnalysis pods → Mock LLM (ClusterIP, same namespace)

Note: No NodePort needed - Mock LLM accessed only by services inside Kind cluster
```

**Kind Config**: `deploy/mock-llm/` (deployment manifests)
**Infrastructure**: Kind cluster with ClusterIP service in `kubernaut-system` (no external access)
**Pattern**: Shared Mock LLM instance accessed via simplified Kubernetes DNS (same namespace)
**Access**: Test runner → HAPI (NodePort 30088) → Mock LLM (`http://mock-llm:8080`)
**DNS Benefit**: Kubernetes automatically resolves `mock-llm` to `mock-llm.kubernaut-system.svc.cluster.local` within namespace

---

### **Auth Webhook Service** (Kubernetes Admission Webhook)

#### **Integration Tests** (`test/integration/authwebhook/`)

**Updated**: 2026-01-06 (SOC2 CC8.1 Attribution - BR-WEBHOOK-001)

```yaml
Webhook Service:
  HTTPS Port: 9443 (DEFAULT)
  Protocol: HTTPS (TLS)
  Purpose: Kubernetes admission webhook endpoint (MutatingWebhookConfiguration)
  Note: Standard K8s webhook port - no configuration needed

PostgreSQL:
  Host Port: 15442
  Container Port: 5432
  Connection: 127.0.0.1:15442
  Purpose: Audit event storage for authenticated operator actions

Redis:
  Host Port: 16386
  Container Port: 6379
  Connection: 127.0.0.1:16386
  Purpose: Data Storage DLQ

Data Storage (Dependency):
  Host Port: 18099
  Container Port: 8080
  Connection: http://127.0.0.1:18099
  Purpose: Audit API for webhook authentication events
```

**Configuration Files**:
- `test/infrastructure/authwebhook.go` - Programmatic infrastructure setup
- `test/integration/authwebhook/suite_test.go` - Test suite with BeforeSuite setup
- `test/integration/authwebhook/datastorage-config.yaml` - Data Storage configuration

**Infrastructure**: Programmatic podman commands (follows AIAnalysis pattern - DD-INTEGRATION-001 v2.0)
**Pattern**: Ginkgo/Gomega with `BeforeSuite` infrastructure startup

**Port Allocation Rationale**:
- **PostgreSQL 15442**: Last available port in 15433-15442 range (no conflicts)
- **Redis 16386**: Available port between 16385 (Notification) and 16387 (HAPI)
- **Data Storage 18099**: Last available port in standard dependency range 18090-18099

**Service Type**: Kubernetes admission webhook (no HTTP API - only webhook endpoints)
**Purpose**: Extract authenticated user identity for SOC2 CC8.1 attribution
**CRDs Supported**: WorkflowExecution, RemediationApprovalRequest, NotificationRequest

#### **E2E Tests** (`test/e2e/authwebhook/`)

**Status**: Pending implementation (TDD Day 5-6)

```yaml
PostgreSQL:
  Host Port: 25442
  Container Port: 5432
  Connection: 127.0.0.1:25442
  Purpose: Audit event storage

Redis:
  Host Port: 26386
  Container Port: 6379
  Connection: 127.0.0.1:26386
  Purpose: Data Storage DLQ

Webhook (in Kind):
  NodePort: 30099
  Purpose: Webhook admission endpoint (no host port mapping needed)

Data Storage (Dependency):
  Host Port: 28099
  Container Port: 8080
  Connection: http://127.0.0.1:28099
  Purpose: Audit API
```

**Kind Config**: `test/infrastructure/kind-authwebhook-config.yaml`
**Infrastructure**: Kind cluster with webhook deployed as admission controller
**Pattern**: Ginkgo/Gomega with `SynchronizedBeforeSuite` for Kind cluster creation

---

### **Data Storage Service**

#### **Integration Tests** (`test/integration/datastorage/`)
```yaml
PostgreSQL:
  Host Port: 15433
  Container Port: 5432
  Connection: 127.0.0.1:15433
  Purpose: Workflow catalog storage (operational data)

Redis:
  Host Port: 16379
  Container Port: 6379
  Connection: 127.0.0.1:16379
  Purpose: DLQ for audit events

Data Storage API:
  Host Port: 18090
  Container Port: 8080
  Connection: http://127.0.0.1:18090

Embedding Service (Mock):
  Host Port: 18000
  Container Port: 8000
  Connection: http://127.0.0.1:18000
```

#### **E2E Tests** (`test/e2e/datastorage/`)
```yaml
PostgreSQL:
  Host Port: 25433
  Container Port: 5432
  Connection: 127.0.0.1:25433
  Purpose: Workflow catalog storage

Redis:
  Host Port: 26379
  Container Port: 6379
  Connection: 127.0.0.1:26379
  Purpose: DLQ for audit events

Data Storage API:
  Host Port: 28090
  Container Port: 8080
  Connection: http://127.0.0.1:28090

Embedding Service:
  Host Port: 28000
  Container Port: 8000
  Connection: http://127.0.0.1:28000
```

---

### **Gateway Service**

#### **Integration Tests** (`test/integration/gateway/`)
```yaml
PostgreSQL (Data Storage dependency):
  Host Port: 15437
  Container Port: 5432
  Connection: 127.0.0.1:15437
  Purpose: Workflow catalog for Data Storage

Redis:
  Host Port: 16380
  Container Port: 6379
  Connection: 127.0.0.1:16380
  Purpose: Gateway rate limiting + Data Storage DLQ

Gateway API:
  Host Port: 18080
  Container Port: 8080
  Connection: http://127.0.0.1:18080

Data Storage (Dependency):
  Host Port: 18091
  Container Port: 8080
  Connection: http://127.0.0.1:18091
```

#### **E2E Tests** (`test/e2e/gateway/`)
```yaml
Redis:
  Host Port: 26380
  Container Port: 6379
  Connection: 127.0.0.1:26380

Gateway API:
  Host Port: 28080
  Container Port: 8080
  Connection: http://127.0.0.1:28080

Data Storage (Dependency):
  Host Port: 28091
  Container Port: 8080
  Connection: http://127.0.0.1:28091
```

---

### **Effectiveness Monitor Controller** (CRD)

#### **Integration Tests** (`test/integration/effectivenessmonitor/`)
```yaml
PostgreSQL (Data Storage dependency):
  Host Port: 15434
  Container Port: 5432
  Connection: 127.0.0.1:15434
  Purpose: DataStorage audit trail storage

Redis (Data Storage dependency):
  Host Port: 16383
  Container Port: 6379
  Connection: 127.0.0.1:16383
  Purpose: DataStorage caching layer

Data Storage (Dependency):
  Host Port: 18092
  Container Port: 8080
  Connection: http://127.0.0.1:18092
  Purpose: Audit event persistence API

Prometheus Mock:
  Port: Ephemeral (httptest.NewServer)
  Purpose: Canned PromQL responses for metric comparison tests

AlertManager Mock:
  Port: Ephemeral (httptest.NewServer)
  Purpose: Canned alert responses for alert resolution tests
```

**Infrastructure**: envtest (K8s API) + programmatic Go (PostgreSQL, Redis, DataStorage via DS bootstrap) + in-process httptest mocks (Prometheus, AlertManager)
**Pattern**: SynchronizedBeforeSuite (Process 1 only) - supports parallel execution with up to 12 Ginkgo processes

**Prometheus/AlertManager Mocking**: Per TESTING_GUIDELINES.md v2.6.0 Section 4a, Tier 2 uses `httptest.NewServer` mock servers with canned API responses. Real Prometheus/AlertManager contract validation is deferred to E2E (Tier 3). Each Ginkgo process creates its own in-process mock on an ephemeral port -- no port allocation needed.

#### **E2E Tests** (`test/e2e/effectivenessmonitor/`)
```yaml
Kind Cluster:
  Config: test/infrastructure/kind-effectivenessmonitor-config.yaml

EM Controller (in Kind):
  Host Port: 8089
  NodePort: 30089
  Metrics Host: 9189
  Metrics NodePort: 30189

Data Storage (Dependency, in Kind):
  Host Port: 28092
  NodePort: 30081
  Connection: http://127.0.0.1:28092

Prometheus (Real, in Kind):
  Flags: --web.enable-remote-write-receiver --storage.tsdb.retention.time=1h
  Purpose: Real PromQL queries for metric comparison; data injected via remote write API

AlertManager (Real, in Kind):
  Purpose: Real alert resolution queries; alerts injected via POST /api/v2/alerts
```

**Note**: E2E uses real Prometheus and AlertManager deployed in Kind cluster to validate actual API contracts (PromQL query syntax, response formats, protobuf/snappy encoding). This avoids the contract mismatch surprises experienced with Mock LLM.

---

### **Workflow Engine Service**

#### **Integration Tests** (`test/integration/workflow-engine/`)
```yaml
Workflow Engine API:
  Host Port: 18110
  Container Port: 8080
  Connection: http://127.0.0.1:18110

Data Storage (Dependency):
  Host Port: 18093
  Container Port: 8080
  Connection: http://127.0.0.1:18093
```

#### **E2E Tests** (`test/e2e/workflow-engine/`)
```yaml
Workflow Engine API:
  Host Port: 28110
  Container Port: 8080
  Connection: http://127.0.0.1:28110

Data Storage (Dependency):
  Host Port: 28093
  Container Port: 8080
  Connection: http://127.0.0.1:28093
```

---

### **SignalProcessing Controller** (CRD)

#### **Integration Tests** (`test/integration/signalprocessing/`)
```yaml
PostgreSQL (Data Storage dependency):
  Host Port: 15436
  Container Port: 5432
  Connection: 127.0.0.1:15436
  Purpose: Workflow catalog for Data Storage

Redis (Data Storage dependency):
  Host Port: 16382
  Container Port: 6379
  Connection: 127.0.0.1:16382
  Purpose: DataStorage DLQ

Data Storage (Dependency):
  Host Port: 18094  # OFFICIAL ALLOCATION - SignalProcessing owns this port
  Container Port: 8080
  Connection: http://127.0.0.1:18094
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
    hostPort: {{HOST_PORT}}         # Port on host machine (127.0.0.1:{{HOST_PORT}})
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
  hostPort: 8082          # 127.0.0.1:8082
  protocol: TCP
- containerPort: 30182    # Metrics NodePort
  hostPort: 9182          # 127.0.0.1:9182 for metrics
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

**Test URL**: `http://127.0.0.1:8082` (for any HTTP endpoints), `http://127.0.0.1:9182/metrics`

---

### **Gateway Service** (E2E - Reference Implementation)

**Kind Config**: `test/infrastructure/kind-gateway-config.yaml`
```yaml
extraPortMappings:
# Gateway service ports
- containerPort: 30080    # Gateway NodePort
  hostPort: 8080          # 127.0.0.1:8080
  protocol: TCP
- containerPort: 30090    # Metrics NodePort
  hostPort: 9090          # 127.0.0.1:9090 for metrics
  protocol: TCP
# Data Storage dependency (for audit events - BR-GATEWAY-190)
- containerPort: 30081    # Data Storage NodePort
  hostPort: 18091         # 127.0.0.1:18091 (avoids conflict with Gateway metrics)
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

**Test URL**: `http://127.0.0.1:8080`, `http://127.0.0.1:9090/metrics`, `http://127.0.0.1:18091` (Data Storage)

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

**Test URL**: `http://127.0.0.1:9186/metrics` (metrics only - no HTTP API)

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
| **Effectiveness Monitor** | 8089 | 30089 | 9189 | 30189 |

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
        serviceURL = "http://127.0.0.1:8082"  // Signal Processing example
        metricsURL = "http://127.0.0.1:9182/metrics"

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
- [ ] Create `test/integration/effectivenessmonitor/` (ports: PostgreSQL 15434, Redis 16383, DataStorage 18092)
- [ ] Create `test/e2e/effectivenessmonitor/` (Kind NodePort: 30089/30189)
- [ ] Create `test/infrastructure/kind-effectivenessmonitor-config.yaml`
- [ ] Add Prometheus (NodePort 30190) and AlertManager (NodePort 30193) to Kind Full Pipeline config

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
| **Gateway** | 15437 | 16380 | 18080 | Data Storage: 18091 |
| **Effectiveness Monitor (CRD)** | 15434 | 16383 | N/A | Data Storage: 18092 |
| **Workflow Engine** | N/A | N/A | 18110 | Data Storage: 18093 |
| **SignalProcessing (CRD)** | 15436 | 16382 | N/A | Data Storage: 18094 |
| **RemediationOrchestrator (CRD)** | 15435 | 16381 | N/A | Data Storage: 18140 |
| **AIAnalysis (CRD)** | 15438 | 16384 | N/A | Data Storage: 18095 |
| **Notification (CRD)** | 15440 | 16385 | N/A | Data Storage: 18096 |
| **WorkflowExecution (CRD)** | 15441 | 16388 | N/A | Data Storage: 18097 |
| **HolmesGPT API (Python)** | 15439 | 16387 | 18120 | Data Storage: 18098 |
| **Auth Webhook (Admission)** | 15442 | 16386 | N/A | Data Storage: 18099 |

✅ **No Conflicts** - All services can run integration tests in parallel

### **E2E Tests** (Can run simultaneously)

| Service | PostgreSQL | Redis | API | Dependencies |
|---------|-----------|-------|-----|--------------|
| **Data Storage** | 25433 | 26379 | 28090 | Embedding: 28000 |
| **Gateway** | N/A | 26380 | 28080 | Data Storage: 28091 |
| **Effectiveness Monitor (CRD)** | 25434 | N/A | N/A | Data Storage: 28092 |
| **Workflow Engine** | N/A | N/A | 28110 | Data Storage: 28093 |
| **RemediationOrchestrator** | N/A | N/A | N/A | Data Storage: 8089 |
| **WorkflowExecution** | N/A | N/A | N/A | Data Storage: 8092 |
| **Auth Webhook** | 25442 | 26386 | N/A | Data Storage: 28099 |

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
curl http://127.0.0.1:18090/health
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
| 2.8 | 2026-02-09 | AI Assistant | **EFFECTIVENESS MONITOR**: Added EM to CRD Controllers NodePort table (30089/30189/8089); Added Redis 16383 for EM integration DS bootstrap (was N/A); Added Prometheus NodePort 30190 and AlertManager NodePort 30193 for Full Pipeline E2E; Updated EM detailed section to reflect CRD controller pattern (envtest + DS bootstrap + httptest mocks for Prom/AM); Updated Port Collision Matrix and NodePort Summary; Updated implementation checklist Phase 3 |
| 2.7 | 2026-02-05 | AI Assistant | **FULL PIPELINE E2E**: Added Full Pipeline E2E port allocations for end-to-end remediation lifecycle test (Issue #39); Gateway ingress NodePort 30080 (event-exporter webhook), DataStorage NodePort 30081 (workflow catalog seeding), Mock LLM ClusterIP (internal only, accessed by HAPI); Kind config: `test/infrastructure/kind-fullpipeline-config.yaml`; All ports verified against Port Collision Matrix - no conflicts |
| 2.6 | 2026-01-15 | AI Assistant | **IMMUDB REMOVAL**: Removed all Immudb port allocations (13322-13331 range) from integration tests; **USER MANDATE**: "Immudb is deprecated, we don't use this DB anymore by authoritative mandate"; **IMPACT**: Simpler infrastructure (one less container per service), faster startup, reduced port allocation requirements; **AFFECTED SERVICES**: Gateway (removed 13323), DataStorage (removed 13322), SignalProcessing (removed 13324), all other services; Port range 13322-13331 now available for future allocation; Updated Port Collision Matrix, service-specific sections, and example usage |
| 2.5 | 2026-01-11 | AI Assistant | **NAMESPACE CONSOLIDATION**: Mock LLM E2E moved to `kubernaut-system` namespace (from dedicated `mock-llm` namespace); **Simplified DNS**: `http://mock-llm:8080` (from `http://mock-llm.mock-llm.svc.cluster.local:8080`); **Rationale**: Matches established E2E pattern (AuthWebhook, DataStorage all use `kubernaut-system`); Kubernetes auto-resolves short DNS names within same namespace; Consistent with test dependency co-location pattern; Integration tests unchanged (still use podman ports 18140/18141) |
| 2.4 | 2026-01-11 | AI Assistant | **ARCHITECTURE FIX**: Mock LLM E2E service changed from NodePort to ClusterIP (internal only); E2E access pattern: Test runner → HAPI (NodePort 30088) → Mock LLM (ClusterIP); Removed NodePort 30091 allocation (not needed - Mock LLM never accessed directly from host); Matches DataStorage pattern (ClusterIP in E2E); Integration tests unchanged (still use podman ports 18140/18141); **Rationale**: Mock LLM accessed only by services inside Kind cluster, no external access required |
| 2.3 | 2026-01-11 | AI Assistant | **NEW SERVICE**: Added Mock LLM Service port allocations (MOCK_LLM_MIGRATION_PLAN.md v1.3.0); Integration tests: Per-service ports (HAPI: 18140, AIAnalysis: 18141) to prevent parallel test collisions; E2E tests: Shared Kind NodePort 30091 (host port 8091); Replaces embedded mock logic in HAPI business code (900 lines removed); Zero external dependencies (Python stdlib only); Range allocation: 18140-18149 (integration), 30091 (E2E); **CRITICAL IPv6 FIX**: All localhost references changed to 127.0.0.1 (GitHub Actions CI/CD IPv6 mapping issue) |
| 2.2 | 2026-01-06 | AI Assistant | **IMMUDB INTEGRATION** (DEPRECATED - See v2.6): Added Immudb port allocations for SOC2 Gap #9 (tamper-evidence); Integration tests: Immudb ports 13322-13331 (per-service allocation); E2E tests: Default port 3322 via Kubernetes Service; Updated all 11 services (DataStorage: 13322, Gateway: 13323, SP: 13324, RO: 13325, AIAnalysis: 13326, WE: 13327, NT: 13328, HAPI: 13329, AuthWebhook: 13330, EffMon: 13331); Enables parallel integration testing with immutable audit trails |
| 2.1 | 2026-01-06 | AI Assistant | **NEW SERVICE**: Added Auth Webhook (Kubernetes admission webhook) port allocations for SOC2 CC8.1 compliance; Integration tests (PostgreSQL: 15442, Redis: 16386, Data Storage: 18099); E2E tests (PostgreSQL: 25442, Redis: 26386, Data Storage: 28099); No port conflicts - parallel testing enabled |
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

