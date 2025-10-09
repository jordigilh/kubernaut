# Kubernaut Command Entry Points

This directory contains entry points for all Kubernaut services following a microservices architecture.

---

## 🏗️ Service Entry Points

### **CRD Controller Services** (5 services)

Each CRD controller runs as a separate microservice with its own binary:

| Directory | Service | CRD | Controller | Documentation |
|-----------|---------|-----|------------|---------------|
| `remediationorchestrator/` | Remediation Orchestrator | RemediationRequest | RemediationRequestReconciler | [docs](../docs/services/crd-controllers/05-remediationorchestrator/) |
| `remediationprocessor/` | Remediation Processor | RemediationProcessing | RemediationProcessingReconciler | [docs](../docs/services/crd-controllers/01-remediationprocessor/) |
| `aianalysis/` | AI Analysis | AIAnalysis | AIAnalysisReconciler | [docs](../docs/services/crd-controllers/02-aianalysis/) |
| `workflowexecution/` | Workflow Execution | WorkflowExecution | WorkflowExecutionReconciler | [docs](../docs/services/crd-controllers/03-workflowexecution/) |
| `kubernetesexecutor/` | Kubernetes Executor | KubernetesExecution | KubernetesExecutionReconciler | [docs](../docs/services/crd-controllers/04-kubernetesexecutor/) |

**Port Configuration**:
- Health/Ready: `0.0.0.0:8080` (`/healthz`, `/readyz`)
- Metrics: `0.0.0.0:9090` (`/metrics`)

**Naming Convention**: 
- ✅ Directories use **no hyphens** (Go convention for `package main`)
- ✅ Binaries use **hyphens for readability** (via `-o` flag)

---

## 🚀 Building Services

### **Build Individual Service**
```bash
# Build remediation orchestrator
go build -o bin/remediation-orchestrator ./cmd/remediationorchestrator

# Build remediation processor
go build -o bin/remediation-processor ./cmd/remediationprocessor

# Build AI analysis
go build -o bin/ai-analysis ./cmd/aianalysis

# Build workflow execution
go build -o bin/workflow-execution ./cmd/workflowexecution

# Build kubernetes executor
go build -o bin/kubernetes-executor ./cmd/kubernetesexecutor
```

**Note**: Directory names have no hyphens (Go convention), but binary names use hyphens for readability.

### **Build All Services**
```bash
make build-all
```

---

## 🐳 Docker Images

Each service has its own Dockerfile in the `docker/` directory:

```bash
# Build Docker images
docker build -f docker/remediation-orchestrator.Dockerfile -t kubernaut/remediation-orchestrator:latest .
docker build -f docker/remediation-processor.Dockerfile -t kubernaut/remediation-processor:latest .
docker build -f docker/ai-analysis.Dockerfile -t kubernaut/ai-analysis:latest .
docker build -f docker/workflow-execution.Dockerfile -t kubernaut/workflow-execution:latest .
docker build -f docker/kubernetes-executor.Dockerfile -t kubernaut/kubernetes-executor:latest .
```

---

## 📦 Deployment

Each service is deployed as a separate Kubernetes Deployment:

```
deploy/
├── remediation-orchestrator-deployment.yaml
├── remediation-processor-deployment.yaml
├── ai-analysis-deployment.yaml
├── workflow-execution-deployment.yaml
└── kubernetes-executor-deployment.yaml
```

Deploy with:
```bash
kubectl apply -f deploy/
```

---

## 🧪 Development Tools

### **Test Utilities**
- `test-context-performance/` - Performance testing tool for context operations

### **Development Manager** (Optional)
- `main.go` - All-in-one manager for local development (runs all controllers in one process)
  - **Note**: This is for development/testing only. Production uses separate services.

---

## 🔍 Service Dependencies

```
Gateway Service (HTTP)
    ↓ creates
RemediationRequest CRD
    ↓ orchestrated by
Remediation Orchestrator
    ↓ creates child CRDs
    ├→ RemediationProcessing → Remediation Processor
    ├→ AIAnalysis → AI Analysis
    ├→ WorkflowExecution → Workflow Execution
    └→ KubernetesExecution → Kubernetes Executor
```

---

## 📚 Documentation

- **Architecture**: [docs/architecture/](../docs/architecture/)
- **CRD Schemas**: [docs/architecture/CRD_SCHEMAS.md](../docs/architecture/CRD_SCHEMAS.md)
- **Service Documentation**: [docs/services/](../docs/services/)
- **Implementation Guide**: [docs/development/](../docs/development/)

---

## 🎯 Quick Start

1. **Build all services**:
   ```bash
   make build-all
   ```

2. **Install CRDs**:
   ```bash
   make install
   ```

3. **Run service** (example):
   ```bash
   ./bin/remediation-orchestrator --leader-elect=false
   ```

4. **Check health**:
   ```bash
   curl http://localhost:8080/healthz
   curl http://localhost:9090/metrics
   ```

---

**Last Updated**: 2025-10-09
**Microservices Architecture**: 5 CRD controllers running as separate services

