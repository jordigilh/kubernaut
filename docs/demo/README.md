# Kubernaut Demo Guide

End-to-end demonstration of the Kubernaut autonomous remediation pipeline.
**17 standalone scenarios** cover the full range of Kubernetes failure modes --
from CrashLoopBackOff and OOMKill to cert-manager CRD failures, Linkerd mesh
routing issues, and GitOps drift -- each flowing through the complete
signal-to-remediation pipeline.

> **Issue**: [#94 -- Prepare Kubernaut demo](https://github.com/jordigilh/kubernaut/issues/94)

---

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Kubernetes cluster | 1.30+ | Kind works out of the box; any cluster supported |
| `kubectl` | 1.28+ | |
| `kustomize` | 5.0+ | Built into `kubectl` 1.28+ |
| Container runtime | podman 4+ or docker 24+ | For local image builds |
| LLM API key | -- | Anthropic, OpenAI, or Google Vertex AI |
| `kind` | 0.30+ | Only if using the Kind overlay |
| `make` | 3.81+ | |

---

## Running a Scenario

Each scenario is **self-contained** -- it manages its own Kind cluster
topology, deploys its workloads, injects a fault, and cleans up after itself.
There is no single "platform deploy" step; cluster setup varies by scenario.

### Quick Start

```bash
# 1. Pick a scenario (e.g., crashloop)
# 2. Create the cluster and run the scenario in one step
./deploy/demo/scenarios/crashloop/run.sh --create-cluster

# 3. Clean up when done
./deploy/demo/scenarios/crashloop/cleanup.sh
```

The `--create-cluster` flag tells the scenario to create (or recreate) a Kind
cluster with the correct topology before deploying. Without it, `run.sh`
expects an existing `kubernaut-demo` cluster and validates its topology.

### Cluster Topology

Scenarios declare their cluster requirements via a `kind-config.yaml` symlink
in their directory. Most scenarios need a single control-plane node; a few
require multiple nodes:

| Topology | Kind Config | Scenarios |
|---|---|---|
| **Single node** | `kind-config-singlenode.yaml` | crashloop, disk-pressure, stuck-rollout, memory-leak, slo-burn, hpa-maxed, pdb-deadlock, cert-failure, cert-failure-gitops, crashloop-helm, gitops-drift, mesh-routing-failure, network-policy-block, statefulset-pvc-failure |
| **Multi node** | `kind-config-multinode.yaml` | autoscale, node-notready, pending-taint |

Scenarios with the same topology can share a cluster -- run one with
`--create-cluster`, then run others without it.

### Scenario-Specific Prerequisites

Some scenarios install additional operators as part of their `run.sh`:

| Scenario | Extra Operator / Infra | Installed by `run.sh` |
|---|---|---|
| cert-failure, cert-failure-gitops | cert-manager | Yes |
| mesh-routing-failure | Linkerd | Yes |
| gitops-drift, cert-failure-gitops | ArgoCD + Gitea | Yes (via `deploy/demo/scenarios/gitops/`) |
| crashloop-helm | Helm | Yes (Helm chart deployed inline) |

### LLM Configuration

HolmesGPT API requires an LLM provider for AI-powered root cause analysis.
Configuration has two parts:

1. **Secret** (`llm-credentials`) -- API keys and project IDs, injected as
   environment variables into the HolmesGPT API pod.
2. **ConfigMap** (`holmesgpt-api-config`) -- provider name, model, endpoint,
   and tuning parameters. Defaults to Vertex AI in
   `deploy/demo/base/platform/holmesgpt-api.yaml`.

Example credential templates are in `deploy/demo/credentials/`. Copy one,
fill in your values, and apply it before deploying the platform.

#### Google Vertex AI (default)

Requires a GCP project with the Vertex AI API enabled and access to the
desired model (e.g., `claude-sonnet-4` via Model Garden).

```bash
cp deploy/demo/credentials/vertex-ai-example.yaml deploy/demo/credentials/my-llm-secret.yaml
```

Edit `my-llm-secret.yaml` and set:

| Field | Description |
|---|---|
| `VERTEXAI_PROJECT` | Your GCP project ID (e.g., `my-gcp-project`) |
| `VERTEXAI_LOCATION` | GCP region for the Vertex AI endpoint (e.g., `us-east5`) |
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to service account key JSON (leave empty if using Workload Identity or ADC) |

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl apply -f deploy/demo/credentials/my-llm-secret.yaml
```

The default ConfigMap already points to Vertex AI. If your project or region
differ from the defaults, also patch the ConfigMap:

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl edit configmap holmesgpt-api-config -n kubernaut-system
# Update: llm.gcp_project_id, llm.gcp_region, llm.endpoint
```

#### Anthropic

Requires an API key from [console.anthropic.com](https://console.anthropic.com/).

```bash
cp deploy/demo/credentials/anthropic-example.yaml deploy/demo/credentials/my-llm-secret.yaml
```

Edit `my-llm-secret.yaml` and set:

| Field | Description |
|---|---|
| `ANTHROPIC_API_KEY` | Your Anthropic API key |

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl apply -f deploy/demo/credentials/my-llm-secret.yaml
```

Then update the ConfigMap to use Anthropic as the provider:

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl edit configmap holmesgpt-api-config -n kubernaut-system
```

Change the `llm` section to:

```yaml
llm:
  provider: "anthropic"
  model: "claude-sonnet-4-20250514"
  max_retries: 3
  timeout_seconds: 120
  temperature: 0.7
```

#### OpenAI

Requires an API key from [platform.openai.com](https://platform.openai.com/).

```bash
cp deploy/demo/credentials/openai-example.yaml deploy/demo/credentials/my-llm-secret.yaml
```

Edit `my-llm-secret.yaml` and set:

| Field | Description |
|---|---|
| `OPENAI_API_KEY` | Your OpenAI API key |

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl apply -f deploy/demo/credentials/my-llm-secret.yaml
```

Then update the ConfigMap to use OpenAI as the provider:

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl edit configmap holmesgpt-api-config -n kubernaut-system
```

Change the `llm` section to:

```yaml
llm:
  provider: "openai"
  model: "gpt-4o"
  max_retries: 3
  timeout_seconds: 120
  temperature: 0.7
```

#### Applying changes

After modifying credentials or the ConfigMap, restart the HolmesGPT API pod
to pick up the new configuration:

```bash
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system
```

> **Tip**: If you already have a Vertex AI config at
> `~/.kubernaut/e2e/hapi-llm-config.yaml`, the HolmesGPT API will pick it up
> automatically when mounted.

### Verifying the Platform

After a scenario's `run.sh` completes setup, verify everything is running:

```bash
# Check all pods are Running
KUBECONFIG=~/.kube/kubernaut-demo-config kubectl get pods -n kubernaut-system
```

Expected output: all pods in `Running` state (12-15 pods depending on
configuration and which operators the scenario installed).

### Exposed Services (Kind)

The Kind cluster exposes these services via NodePort:

| Service | Host Port | Description |
|---|---|---|
| Gateway | `localhost:30080` | Signal ingestion API |
| DataStorage | `localhost:30081` | Audit trail & workflow catalog API |
| Prometheus | `localhost:9190` | Metrics queries |
| AlertManager | `localhost:9193` | Alert management UI |
| Grafana | `localhost:3000` | Operational dashboard (credentials: `admin` / `kubernaut`) |

---

## Demo Scenarios

17 scenarios organized by failure domain. Each scenario injects a realistic
fault, fires a Prometheus alert, and lets the pipeline diagnose and remediate
autonomously.

### Core Kubernetes Failures

| # | Scenario | Directory | Description | Workflow Image |
|---|----------|-----------|-------------|----------------|
| 1 | **CrashLoopBackOff** | `crashloop` | Bad ConfigMap causes CrashLoop; auto-rollback to previous revision | `crashloop-rollback-job` |
| 2 | **Disk Pressure** | `disk-pressure` | Orphaned PVCs from batch jobs; deletes unmounted PVCs | `cleanup-pvc-job` |
| 3 | **Pending Pods (Taint)** | `pending-taint` | Pods stuck Pending due to node taint; removes the taint | `remove-taint-job` |
| 4 | **Stuck Rollout** | `stuck-rollout` | ImagePullBackOff from non-existent tag; rolls back deployment | `rollback-deployment-job` |
| 5 | **Node NotReady** | `node-notready` | Worker node goes NotReady; cordons and drains to healthy nodes | `cordon-drain-job` |
| 6 | **Predictive Memory Leak** | `memory-leak` | `predict_linear()` detects memory growth before OOM; rolling restart | `graceful-restart-job` |
| 7 | **SLO Error Budget Burn** | `slo-burn` | Error budget burning too fast; proactive rollback to preserve SLO | `proactive-rollback-job` |
| 8 | **Cluster Autoscaling** | `autoscale` | Pods unschedulable from resource exhaustion; provisions new Kind node | `provision-node-job` |

### Detected-Label Scenarios

These scenarios exercise Kubernaut's label detection system, where the AI
pipeline recognizes environment-specific characteristics and selects
label-aware remediation workflows.

| # | Scenario | Directory | Detected Label | Description | Workflow Image |
|---|----------|-----------|----------------|-------------|----------------|
| 9 | **HPA Maxed Out** | `hpa-maxed` | `hpaEnabled` | HPA at `maxReplicas` ceiling; patches HPA to raise the limit | `patch-hpa-job` |
| 10 | **PDB Deadlock** | `pdb-deadlock` | `pdbProtected` | PDB `minAvailable` equals replicas, blocking rollout; relaxes PDB | `relax-pdb-job` |
| 11 | **StatefulSet PVC Failure** | `statefulset-pvc-failure` | `stateful` | PVC disruption causes StatefulSet pods stuck Pending; recreates PVC | `fix-statefulset-pvc-job` |
| 12 | **NetworkPolicy Block** | `network-policy-block` | `networkIsolated` | Deny-all NetworkPolicy blocks ingress; removes offending policy | `fix-network-policy-job` |
| 13 | **Mesh Routing Failure** | `mesh-routing-failure` | `serviceMesh=linkerd` | Linkerd AuthorizationPolicy causes high error rate; removes blocking policy | `fix-authz-policy-job` |

### GitOps & Packaging Scenarios

These scenarios demonstrate remediation in environments where resources are
managed by external tooling (ArgoCD, Helm) and direct `kubectl` mutations
would cause drift.

| # | Scenario | Directory | Detected Label | Description | Workflow Image |
|---|----------|-----------|----------------|-------------|----------------|
| 14 | **GitOps Drift** | `gitops-drift` | `gitOpsManaged` | Bad ConfigMap in ArgoCD-managed app; git revert via Gitea | `git-revert-job` |
| 15 | **Cert Failure (CRD)** | `cert-failure` | -- | cert-manager Certificate stuck NotReady; recreates CA Secret | `fix-certificate-job` |
| 16 | **Cert Failure (GitOps)** | `cert-failure-gitops` | `gitOpsManaged` | Same as #15 but ArgoCD-managed; git revert via Gitea | `fix-certificate-gitops-job` |
| 17 | **CrashLoop (Helm)** | `crashloop-helm` | `helmManaged` | Same fault as #1 but Helm-managed; `helm rollback` instead of kubectl | `helm-rollback-job` |

> **Shared GitOps infrastructure**: Scenarios #14, #16 depend on ArgoCD and
> Gitea. The shared setup scripts are in `deploy/demo/scenarios/gitops/`.

All workflow images are hosted at `quay.io/kubernaut-cicd/test-workflows/<name>:v1.0.0`.

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

Replace `<NAMESPACE>` with the scenario's namespace (e.g., `demo-crashloop`,
`demo-cert`, `demo-mesh`). Each scenario's README lists its namespace.

```bash
export KUBECONFIG=~/.kube/kubernaut-demo-config

# Watch RemediationRequests (the main pipeline CRD)
kubectl get remediationrequests -n <NAMESPACE> -w

# Watch all Kubernaut CRDs
kubectl get remediationrequests,aianalyses,workflowrequests,effectivenessassessments \
  -n <NAMESPACE> -o wide
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

Check that remediation completed successfully (replace `<NAMESPACE>` with the
scenario's namespace, e.g., `demo-crashloop`):

```bash
export KUBECONFIG=~/.kube/kubernaut-demo-config

# RemediationRequest should reach "Completed" phase
kubectl get remediationrequests -n <NAMESPACE> -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\n"}{end}'

# Check the workflow Job ran successfully
kubectl get jobs -n kubernaut-workflows

# Check effectiveness assessment was created
kubectl get effectivenessassessments -n <NAMESPACE>
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
# Clean up a specific scenario's resources
./deploy/demo/scenarios/crashloop/cleanup.sh

# Destroy the entire Kind cluster
kind delete cluster --name kubernaut-demo

# Or via make
make demo-teardown
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
├── scenarios/                       # 17 self-contained demo scenarios
│   ├── crashloop/                   # Each scenario contains:
│   │   ├── README.md                #   Scenario documentation
│   │   ├── run.sh                   #   Deploy workload + inject fault
│   │   ├── cleanup.sh               #   Remove scenario resources
│   │   ├── manifests/               #   K8s manifests (Deployment, Service, PrometheusRule)
│   │   └── workflow/                #   Remediation workflow (Dockerfile, remediate.sh, schema)
│   ├── disk-pressure/
│   ├── pending-taint/
│   ├── stuck-rollout/
│   ├── node-notready/
│   ├── memory-leak/
│   ├── slo-burn/
│   ├── autoscale/
│   ├── hpa-maxed/
│   ├── pdb-deadlock/
│   ├── statefulset-pvc-failure/
│   ├── network-policy-block/
│   ├── mesh-routing-failure/
│   ├── gitops-drift/
│   ├── cert-failure/
│   ├── cert-failure-gitops/
│   ├── crashloop-helm/
│   └── gitops/                      # Shared GitOps infra (Gitea + ArgoCD setup)
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
    ├── seed-workflows.sh
    └── build-demo-workflows.sh      # Build & push all 17 workflow OCI images
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

#### Workflow Image Targets

| Command | Description |
|---|---|
| `./deploy/demo/scripts/build-demo-workflows.sh --local` | Build all 17 workflow images locally |
| `./deploy/demo/scripts/build-demo-workflows.sh` | Build multi-arch + push to quay.io |
| `./deploy/demo/scripts/build-demo-workflows.sh --scenario crashloop` | Build a single scenario's workflow image |

### Credential Management

Database credentials are managed via Kustomize `secretGenerator` in
`deploy/demo/base/secrets/`. The password is defined once and shared between
PostgreSQL and DataStorage secrets. To change the demo password, edit
`postgresql.env` and `db-secrets.yaml` in that directory.

For production deployments, use a proper secrets management solution
(Vault, Sealed Secrets, External Secrets Operator).
