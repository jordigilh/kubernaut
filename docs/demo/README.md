# Kubernaut Demo Guide

End-to-end demonstration of the Kubernaut autonomous remediation pipeline.
Two signal paths trigger the full pipeline: a **Kubernetes OOMKill event** and a
**Prometheus MemoryExceedsLimit alert**, each flowing through all 10 services
to diagnose, remediate, and assess effectiveness.

> **Issue**: [#94 -- Prepare Kubernaut demo](https://github.com/jordigilh/kubernaut/issues/94)

---

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Kubernetes cluster | 1.28+ | Kind works out of the box; any cluster supported |
| `kubectl` | 1.28+ | |
| `kustomize` | 5.0+ | Built into `kubectl` 1.28+ |
| Container runtime | podman 4+ or docker 24+ | For local image builds |
| LLM API key | -- | Anthropic, OpenAI, or Google Vertex AI |
| `kind` | 0.20+ | Only if using the Kind overlay |
| `make` | 3.81+ | |

---

## Quick Start (Kind)

The fastest way to get the demo running:

```bash
# 1. Build all service images locally
make demo-build-images

# 2. Full setup: create cluster, load images, deploy everything
make demo-setup
```

This builds images, creates a Kind cluster, loads images, deploys all
infrastructure and services, runs migrations, generates TLS certs, and seeds
the workflow catalog.

> **Pre-built images**: If you already have images in a registry, skip the build:
> ```bash
> make demo-setup DEMO_REGISTRY=quay.io/kubernaut-ai DEMO_TAG=demo-v1.0
> ```

---

## Step-by-Step Deployment

### 1. Create a Kind Cluster

```bash
make demo-create-cluster
```

This creates a cluster named `kubernaut-demo` with port mappings for:

| Service | Host Port | Description |
|---|---|---|
| Gateway | `localhost:30080` | Signal ingestion API |
| DataStorage | `localhost:30081` | Audit trail & workflow catalog API |
| Prometheus | `localhost:9190` | Metrics queries |
| AlertManager | `localhost:9193` | Alert management UI |
| Grafana | `localhost:3000` | Operational dashboard |

The kubeconfig is written to `~/.kube/kubernaut-demo-config`.

### 2. Build & Load Images

```bash
# Build all service images (single-arch, for Kind)
make demo-build-images

# Load into Kind cluster
make demo-load-images
```

For multi-architecture images (amd64 + arm64), use:

```bash
make image-build IMAGE_TAG=demo-v1.0
make image-push IMAGE_TAG=demo-v1.0
```

### 3. Configure LLM Credentials

Create a Secret with your LLM API credentials. Example templates are in
`deploy/demo/credentials/`:

**Google Vertex AI** (recommended):

```bash
# Copy and edit the example
cp deploy/demo/credentials/vertex-ai-example.yaml deploy/demo/credentials/my-llm-secret.yaml
# Edit: set your project ID and paste your service account JSON
vim deploy/demo/credentials/my-llm-secret.yaml

# Apply
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl apply -f deploy/demo/credentials/my-llm-secret.yaml
```

**Anthropic**:
```bash
cp deploy/demo/credentials/anthropic-example.yaml deploy/demo/credentials/my-llm-secret.yaml
# Edit: set your ANTHROPIC_API_KEY
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl apply -f deploy/demo/credentials/my-llm-secret.yaml
```

**OpenAI**:
```bash
cp deploy/demo/credentials/openai-example.yaml deploy/demo/credentials/my-llm-secret.yaml
# Edit: set your OPENAI_API_KEY
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl apply -f deploy/demo/credentials/my-llm-secret.yaml
```

If you already have a Vertex AI config at `~/.kubernaut/e2e/hapi-llm-config.yaml`,
the HolmesGPT API will pick it up automatically when mounted.

### 4. Deploy the Platform

```bash
make demo-deploy
```

This executes the following sequence:

1. Applies all 7 Kubernaut CRDs
2. Deploys infrastructure (PostgreSQL, Redis, Prometheus, AlertManager, Grafana)
3. Deploys all Kubernaut services with RBAC
4. Waits for PostgreSQL readiness
5. Runs database migrations
6. Generates AuthWebhook TLS certificates
7. Waits for all pods to become ready
8. Seeds the workflow catalog with remediation workflows

### 5. Verify Deployment

```bash
# Check all pods are Running
make demo-status

# Or manually:
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl get pods -n kubernaut-system
```

Expected output: all pods in `Running` state (12-15 pods depending on configuration).

### 6. Deploy Demo Workloads

The demo includes two memory-eater workloads that trigger the remediation pipeline:

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl apply -k deploy/demo/base/workloads/
```

| Workload | Trigger | Mechanism |
|---|---|---|
| `memory-eater-oomkill` | OOMKill event | Allocates 60Mi with 50Mi limit; K8s kills it |
| `memory-eater-high-usage` | Prometheus alert | Allocates 92Mi of 100Mi limit; triggers `MemoryExceedsLimit` |

---

## Observing the Pipeline

### Signal Flow

```
[OOMKill Event] ──> Event Exporter ──> Gateway ──┐
                                                  ├──> RemediationRequest CRD
[Prometheus Alert] ──> AlertManager ──> Gateway ──┘         │
                                                            v
                                                   SignalProcessing
                                                            │
                                                            v
                                                      AI Analysis
                                                     (LLM via HolmesGPT)
                                                            │
                                                            v
                                                   Remediation Orchestrator
                                                            │
                                                            v
                                                   Workflow Execution
                                                            │
                                                            v
                                                   Effectiveness Monitor
```

### Expected Timeline

| Time | Event |
|---|---|
| 0s | Deploy memory-eater workloads |
| ~10-30s | OOMKill occurs, K8s event generated |
| ~30-60s | Prometheus `MemoryExceedsLimit` alert fires |
| ~1-2min | Gateway creates RemediationRequest CRDs |
| ~2-3min | AI Analysis completes (LLM determines remediation action) |
| ~3-5min | Workflow Execution runs remediation Job |
| ~5-7min | Effectiveness Monitor assesses remediation outcome |

### Watching CRD State Transitions

```bash
export KUBECONFIG=~/.kube/kubernaut-demo-config

# Watch RemediationRequests (the main pipeline CRD)
kubectl get remediationrequests -n demo-workloads -w

# Watch all Kubernaut CRDs
kubectl get remediationrequests,aianalyses,workflowrequests,effectivenessassessments \
  -n demo-workloads -o wide
```

### Key Logs to Watch

```bash
export KUBECONFIG=~/.kube/kubernaut-demo-config

# Gateway: signal reception
kubectl logs -n kubernaut-system -l app=gateway -f --tail=50

# AI Analysis: LLM interaction
kubectl logs -n kubernaut-system -l app=aianalysis -f --tail=50

# Remediation Orchestrator: pipeline orchestration
kubectl logs -n kubernaut-system -l app=remediationorchestrator -f --tail=50

# Workflow Execution: Job creation and monitoring
kubectl logs -n kubernaut-system -l app=workflowexecution -f --tail=50
```

### Grafana Dashboard

Open [http://localhost:3000](http://localhost:3000) (credentials: `admin` / `kubernaut`).

The **Kubernaut Operations** dashboard is auto-provisioned and shows:

- **Signal Ingestion**: signals received, CRDs created, deduplication rate, latency
- **Signal Processing**: enrichment rate and duration
- **AI Analysis**: reconciliation rate, LLM confidence scores, failures
- **Remediation Orchestrator**: phase transitions, reconcile duration, blocked count
- **Data Storage**: audit trace ingestion rate, write latency
- **Effectiveness Monitor**: assessments completed, component scores
- **Audit Buffer Health**: per-service buffer utilization, events dropped vs written

### Verifying Remediation

Check that remediation completed successfully:

```bash
export KUBECONFIG=~/.kube/kubernaut-demo-config

# RemediationRequest should reach "Completed" phase
kubectl get remediationrequests -n demo-workloads -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\n"}{end}'

# Check the workflow Job ran successfully
kubectl get jobs -n kubernaut-workflows

# Check effectiveness assessment was created
kubectl get effectivenessassessments -n demo-workloads
```

---

## Troubleshooting

### Pods not starting

```bash
# Check events for a specific pod
kubectl describe pod <pod-name> -n kubernaut-system

# Common issue: images not loaded into Kind
make demo-load-images
```

### No signals reaching Gateway

```bash
# Check event-exporter is running and forwarding events
kubectl logs -n kubernaut-system -l app=event-exporter --tail=20

# Check AlertManager is routing to Gateway
curl -s http://localhost:9193/api/v2/alerts | jq .
```

### LLM errors in AI Analysis

```bash
# Check HolmesGPT API logs for credential issues
kubectl logs -n kubernaut-system -l app=holmesgpt-api --tail=50

# Verify the LLM secret was created
kubectl get secrets -n kubernaut-system | grep llm
```

### Prometheus not scraping services

```bash
# Check Prometheus targets
curl -s http://localhost:9190/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, instance: .labels.instance, health: .health}'

# Verify scrape annotations on pods
kubectl get pods -n kubernaut-system -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.prometheus\.io/scrape}{"\n"}{end}'
```

### Grafana shows "No data"

Wait 1-2 minutes after deployment for Prometheus to scrape initial metrics.
Verify the Prometheus datasource is reachable:

```bash
# From inside the cluster
kubectl exec -n kubernaut-system deploy/grafana -- wget -qO- http://prometheus-svc:9090/api/v1/status/config | head -5
```

---

## Cleanup

```bash
# Remove everything (cluster + kubeconfig)
make demo-teardown

# Or just remove workloads to re-trigger the pipeline
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl delete -k deploy/demo/base/workloads/
```

---

## Architecture

### Directory Structure

```
deploy/demo/
├── base/
│   ├── kustomization.yaml          # Root aggregator
│   ├── secrets/                    # Kustomize secretGenerator (PostgreSQL + DataStorage)
│   │   ├── kustomization.yaml
│   │   ├── postgresql.env          # POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
│   │   ├── db-secrets.yaml         # DataStorage DB credentials (matches postgresql.env)
│   │   └── redis-secrets.yaml      # Redis credentials (empty for demo)
│   ├── infrastructure/
│   │   ├── namespace.yaml          # kubernaut-system namespace
│   │   ├── postgresql.yaml         # PostgreSQL Deployment + Service
│   │   └── redis.yaml              # Redis Deployment + Service
│   ├── monitoring/
│   │   ├── prometheus.yaml         # Prometheus + scrape config + alert rules
│   │   ├── alertmanager.yaml       # AlertManager + route to Gateway webhook
│   │   └── grafana.yaml            # Grafana + datasource + operational dashboard
│   ├── platform/
│   │   ├── rbac/                   # DataStorage service + client RBAC
│   │   ├── datastorage.yaml        # DataStorage ConfigMap + Deployment + Service
│   │   ├── gateway.yaml            # Gateway ConfigMap + SA + RBAC + Deployment + Service
│   │   ├── authwebhook.yaml        # AuthWebhook ConfigMap + TLS + Deployment + Service
│   │   ├── signalprocessing.yaml   # SP ConfigMaps (Rego policies) + Deployment + Service
│   │   ├── aianalysis.yaml         # AA ConfigMaps (Rego) + Deployment + Service
│   │   ├── remediationorchestrator.yaml
│   │   ├── workflowexecution.yaml
│   │   ├── notification.yaml
│   │   ├── holmesgpt-api.yaml
│   │   ├── effectivenessmonitor.yaml
│   │   └── event-exporter.yaml
│   └── workloads/
│       ├── namespace.yaml          # demo-workloads namespace
│       ├── memory-eater-oomkill.yaml
│       └── memory-eater-high-usage.yaml
├── overlays/
│   └── kind/
│       ├── kustomization.yaml      # NodePort patches for Kind
│       └── kind-cluster-config.yaml
├── credentials/
│   ├── vertex-ai-example.yaml
│   ├── anthropic-example.yaml
│   └── openai-example.yaml
└── scripts/
    ├── apply-migrations.sh
    ├── generate-webhook-certs.sh
    └── seed-workflows.sh
```

### Make Targets

| Target | Description |
|---|---|
| `make demo-setup` | Full setup: build, cluster, load, deploy |
| `make demo-build-images` | Build all service images locally (single-arch) |
| `make demo-create-cluster` | Create Kind cluster with port mappings |
| `make demo-load-images` | Load images into Kind cluster |
| `make demo-deploy` | Deploy platform to existing cluster |
| `make demo-teardown` | Destroy Kind cluster |
| `make demo-status` | Show pod status |
| `make image-build IMAGE_TAG=v1.0` | Build multi-arch images (amd64 + arm64) |
| `make image-push IMAGE_TAG=v1.0` | Push multi-arch images to registry |

### Credential Management

Database credentials are managed via Kustomize `secretGenerator` in
`deploy/demo/base/secrets/`. The password is defined once and shared between
PostgreSQL and DataStorage secrets. To change the demo password, edit
`postgresql.env` and `db-secrets.yaml` in that directory.

For production deployments, use a proper secrets management solution
(Vault, Sealed Secrets, External Secrets Operator).
