# Kubernaut Command Entry Points

This directory contains entry points for all Kubernaut services following a microservices architecture.

**Total**: 10 services (6 CRD controllers + 4 stateless) + 1 container image (Must-Gather).

---

## 🏗️ Service Inventory

### **CRD Controller Services** (Go, 6 services)

Each CRD controller runs as a separate microservice with its own binary:

| Directory | Service | CRD | Short Name |
|-----------|---------|-----|------------|
| `remediationorchestrator/` | Remediation Orchestrator | RemediationRequest | `rr` |
| `signalprocessing/` | Signal Processing | SignalProcessing | `sp` |
| `aianalysis/` | AI Analysis | AIAnalysis | `aia` |
| `workflowexecution/` | Workflow Execution | WorkflowExecution | `wfe` |
| `effectivenessmonitor/` | Effectiveness Monitor | EffectivenessAssessment | `ea` |
| `notification/` | Notification | NotificationRequest | `notif` |

### **Stateless Services** (Go, 3 services)

| Directory | Service | Description |
|-----------|---------|-------------|
| `gateway/` | Gateway | HTTP ingestion endpoint for alerts |
| `datastorage/` | DataStorage | Persistence and audit storage |
| `authwebhook/` | Auth Webhook | Kubernetes authentication webhook |

### **Stateless Service** (Python, 1 service)

| Directory | Service | Description |
|-----------|---------|-------------|
| `holmesgpt-api/` (repo root) | HolmesGPT-API (HAPI) | LLM-powered root cause analysis |

> **Note**: `holmesgpt-api/` lives at the repository root, not under `cmd/`.

### **Container Images / Tools**

| Directory | Tool | Description |
|-----------|------|-------------|
| `must-gather/` | Must-Gather | Container image for diagnostic triage and root cause analysis |

---

## ⚙️ Port Configuration

All Go services use the same standard ports:

- **Health/Ready**: `0.0.0.0:8080` (`/healthz`, `/readyz`)
- **Metrics**: `0.0.0.0:9090` (`/metrics`)

---

## 📐 Naming Convention

- **Directories**: No hyphens (Go convention for `package main`)
- **Binaries**: Hyphens for readability (via `-o` flag when building manually)

---

## 🔄 Service Dependency Flow

```
Gateway Service (HTTP)
    ↓ creates
RemediationRequest (rr)
    ↓ orchestrated by
Remediation Orchestrator
    ↓ creates child CRDs
    ├→ SignalProcessing (sp) → Signal Processing Service
    ├→ AIAnalysis (aia) → AI Analysis Service → calls HAPI (HolmesGPT-API)
    ├→ WorkflowExecution (wfe) → Workflow Execution Service
    ├→ NotificationRequest (notif) → Notification Service
    └→ EffectivenessAssessment (ea) → Effectiveness Monitor Service
```

---

## 🚀 Building Services

### **Build Individual Service**

```bash
# CRD controllers
go build -o bin/remediation-orchestrator ./cmd/remediationorchestrator
go build -o bin/signal-processing ./cmd/signalprocessing
go build -o bin/ai-analysis ./cmd/aianalysis
go build -o bin/workflow-execution ./cmd/workflowexecution
go build -o bin/effectiveness-monitor ./cmd/effectivenessmonitor
go build -o bin/notification-controller ./cmd/notification

# Stateless services
go build -o bin/gateway ./cmd/gateway
go build -o bin/data-storage ./cmd/datastorage
go build -o bin/auth-webhook ./cmd/authwebhook
```

Or use Make targets (binaries use directory name, e.g. `bin/remediationorchestrator`):

```bash
make build-remediationorchestrator
make build-gateway
# etc.
```

### **Build All Go Services**

```bash
make build-all
```

### **Build HolmesGPT-API** (Python, repo root)

```bash
# Local development
make build-holmesgpt-api

# Docker image (production)
make build-holmesgpt-api-image

# Docker image (E2E, minimal deps)
make build-holmesgpt-api-image-e2e
```

---

## 🐳 Docker Images

Go services use Dockerfiles in the `docker/` directory:

```bash
# Build all images (native arch)
make image-build-all

# Build individual service images
make image-build-gateway
make image-build-remediationorchestrator
make image-build-aianalysis
make image-build-signalprocessing
make image-build-workflowexecution
make image-build-effectivenessmonitor
make image-build-notification
make image-build-datastorage
make image-build-authwebhook

# HolmesGPT-API (separate target)
make image-build-holmesgpt-api
```

Dockerfile mappings (from `docker/`):

| Service | Dockerfile |
|---------|------------|
| gateway | `docker/gateway-ubi9.Dockerfile` |
| datastorage | `docker/data-storage.Dockerfile` |
| remediationorchestrator | `docker/remediationorchestrator-controller.Dockerfile` |
| signalprocessing | `docker/signalprocessing-controller.Dockerfile` |
| aianalysis | `docker/aianalysis.Dockerfile` |
| workflowexecution | `docker/workflowexecution-controller.Dockerfile` |
| effectivenessmonitor | `docker/effectivenessmonitor-controller.Dockerfile` |
| notification | `docker/notification-controller-ubi9.Dockerfile` |
| authwebhook | `docker/authwebhook.Dockerfile` |
| holmesgpt-api | `holmesgpt-api/Dockerfile` |

---

## 📦 Deployment

Services are deployed via the Helm chart:

```bash
# Install/upgrade via Helm
helm upgrade --install kubernaut ./charts/kubernaut -n kubernaut-system --create-namespace
```

Chart templates (in `charts/kubernaut/templates/`):

- `gateway/gateway.yaml`
- `datastorage/datastorage.yaml`
- `authwebhook/authwebhook.yaml`
- `remediationorchestrator/remediationorchestrator.yaml`
- `signalprocessing/signalprocessing.yaml`
- `aianalysis/aianalysis.yaml`
- `workflowexecution/workflowexecution.yaml`
- `effectivenessmonitor/effectivenessmonitor.yaml`
- `notification/notification.yaml`
- `holmesgpt-api/holmesgpt-api.yaml`

---

## 🧪 Development Tools

### **Must-Gather**

- `must-gather/` — Diagnostic container; build via `cmd/must-gather/Makefile`

---

## 🎯 Quick Start

1. **Build all Go services**:
   ```bash
   make build-all
   ```

2. **Install CRDs**:
   ```bash
   make install
   ```

3. **Run a service** (example):
   ```bash
   ./bin/remediationorchestrator --leader-elect=false
   ```

4. **Check health**:
   ```bash
   curl http://localhost:8080/healthz
   curl http://localhost:9090/metrics
   ```

---

## 📝 Deprecation Note

**KubernetesExecution / KubernetesExecutor** was eliminated by ADR-025 and replaced by Tekton TaskRun. The `kubernetesexecution/` controller and `remediationprocessor/` have been removed.

---

## 📚 Documentation

- **Architecture**: [docs/architecture/](../docs/architecture/)
- **CRD Schemas**: [docs/architecture/CRD_SCHEMAS.md](../docs/architecture/CRD_SCHEMAS.md)
- **Service Documentation**: [docs/services/](../docs/services/)
- **Implementation Guide**: [docs/development/](../docs/development/)

---

**Last Updated**: 2026-02-24  
**Microservices Architecture**: 6 CRD controllers + 4 stateless services + 1 container image
