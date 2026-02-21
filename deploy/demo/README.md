# Kubernaut Demo Installation Guide

This guide walks through deploying the complete Kubernaut platform on a local Kind cluster for demonstration purposes. The demo showcases the full remediation lifecycle: from signal detection through AI analysis to automated workflow execution.

## Prerequisites

- **Go** 1.25+
- **Kind** v0.30+
- **kubectl** v1.34+
- **Container runtime**: Podman or Docker
- **openssl** (for AuthWebhook TLS cert generation)
- **curl** and **jq** (for workflow seeding and verification)
- **LLM Provider credentials**: One of Vertex AI (GCP), Anthropic, or OpenAI

Memory: ~6GB available for the Kind cluster.

## Quick Start

```bash
# Full automated setup (builds images, creates cluster, deploys everything)
make demo-setup

# Apply your LLM credentials (see "Configure LLM Credentials" below)
kubectl --kubeconfig ~/.kube/kubernaut-demo-config apply -f my-llm-credentials.yaml

# Restart HolmesGPT API to pick up credentials
kubectl --kubeconfig ~/.kube/kubernaut-demo-config rollout restart deployment/holmesgpt-api -n kubernaut-system

# Run a demo scenario (see "Scenario Catalog" below)
./deploy/demo/scenarios/stuck-rollout/run.sh
```

## Step-by-Step Setup

### 1. Build Service Images

Build all 10 Kubernaut service images locally:

```bash
make demo-build-images
```

This builds: datastorage, gateway, aianalysis, authwebhook, notification, remediationorchestrator, signalprocessing, workflowexecution, effectivenessmonitor, holmesgpt-api.

Images are tagged as `quay.io/kubernaut-ai/{service}:demo-v1.0` by default.

### 2. Create Kind Cluster

```bash
make demo-create-cluster
```

Creates a single-node Kind cluster named `kubernaut-demo` with NodePort mappings:

| Service | Host Port | NodePort |
|---------|-----------|----------|
| Gateway | 30080 | 30080 |
| DataStorage | 30081 | 30081 |
| Prometheus | 9190 | 30190 |
| AlertManager | 9193 | 30193 |

Kubeconfig: `~/.kube/kubernaut-demo-config`

**Multi-node scenarios**: The `autoscale`, `node-notready`, and `pending-taint` scenarios require a multi-node cluster. Their `run.sh` scripts validate the topology automatically and will prompt you to re-create the cluster with `run.sh --create-cluster` if needed.

### 3. Load Images into Kind

```bash
make demo-load-images
```

### 4. Deploy Platform

```bash
make demo-deploy
```

This performs:
1. Applies all Kustomize manifests (CRDs, RBAC, infrastructure, monitoring, platform services)
2. Waits for PostgreSQL readiness
3. Runs SQL migrations
4. Generates AuthWebhook TLS certificates and patches webhook configurations
5. Seeds the workflow catalog with remediation workflows

### 5. Configure LLM Credentials

Copy the appropriate example credential file and fill in your values:

**Vertex AI (recommended):**
```bash
cp deploy/demo/credentials/vertex-ai-example.yaml my-llm-credentials.yaml
# Edit my-llm-credentials.yaml with your GCP project ID
kubectl --kubeconfig ~/.kube/kubernaut-demo-config apply -f my-llm-credentials.yaml
```

The default HolmesGPT API ConfigMap is pre-configured for Vertex AI with `claude-sonnet-4`. If using a different provider, update `deploy/demo/base/platform/holmesgpt-api.yaml` ConfigMap **before** running `make demo-deploy` (step 4).

**Anthropic:**
```bash
cp deploy/demo/credentials/anthropic-example.yaml my-llm-credentials.yaml
# Edit with your Anthropic API key
```

**OpenAI:**
```bash
cp deploy/demo/credentials/openai-example.yaml my-llm-credentials.yaml
# Edit with your OpenAI API key
```

After applying credentials, restart the HolmesGPT API:
```bash
kubectl --kubeconfig ~/.kube/kubernaut-demo-config rollout restart deployment/holmesgpt-api -n kubernaut-system
```

### Optional: Slack Notifications

To receive remediation notifications in Slack:

1. Create a [Slack Incoming Webhook](https://api.slack.com/messaging/webhooks) for your workspace
2. Place the webhook URL in the secrets directory (this file is gitignored):

```bash
# Option A: Copy from ~/.kubernaut/ if you already have one
cp ~/.kubernaut/notification/slack-webhook.url deploy/demo/base/secrets/slack-webhook-url

# Option B: Create manually
echo "https://hooks.slack.com/services/YOUR/WEBHOOK/URL" > deploy/demo/base/secrets/slack-webhook-url
```

3. The `slack-webhook` Secret is generated automatically by Kustomize from `deploy/demo/base/secrets/kustomization.yaml` and referenced by the notification controller deployment via `secretKeyRef`.

If the demo is already running, you can create the Secret manually and restart:

```bash
kubectl --kubeconfig ~/.kube/kubernaut-demo-config create secret generic slack-webhook \
  -n kubernaut-system \
  --from-file=webhook-url=$HOME/.kubernaut/notification/slack-webhook.url

kubectl --kubeconfig ~/.kube/kubernaut-demo-config rollout restart deployment/notification-controller -n kubernaut-system
```

### 6. Run a Demo Scenario

Each scenario lives in `deploy/demo/scenarios/<name>/` and includes its own `run.sh`, Kubernetes manifests, Prometheus alerting rules, fault-injection scripts, and a remediation workflow. Run any scenario after the platform is deployed:

```bash
./deploy/demo/scenarios/stuck-rollout/run.sh
```

Every scenario follows the same pipeline:

```
Fault injection (bad image, CPU load, taint, etc.)
  -> Prometheus alert or K8s Event fires
  -> Gateway creates RemediationRequest CRD
  -> SignalProcessing enriches and classifies the signal
  -> AI Analysis investigates via HolmesGPT API + LLM
  -> LLM selects a matching remediation workflow from the catalog
  -> WorkflowExecution runs the remediation (K8s Job or Tekton Pipeline)
  -> Notification delivers status updates (Slack, etc.)
  -> EffectivenessMonitor verifies the fix actually worked
```

Each scenario's `README.md` contains its BDD specification, acceptance criteria, and manual step-by-step instructions.

## Scenario Catalog

17 scenarios are available, organized by category. Each scenario deploys into its own namespace and can be run independently.

Some scenarios require additional components beyond the base platform:

| Dependency | Scenarios | Notes |
|------------|-----------|-------|
| **kube-state-metrics** | Most scenarios | Included in base monitoring stack |
| **metrics-server** | hpa-maxed | Required for HPA CPU metrics |
| **cert-manager** | cert-failure, cert-failure-gitops | Certificate lifecycle management |
| **Linkerd** | mesh-routing-failure | Service mesh control plane |
| **Helm CLI** | crashloop-helm | Helm-managed release rollback |
| **ArgoCD** | gitops-drift, cert-failure-gitops | GitOps delivery |

Each scenario's `README.md` lists its specific prerequisites.

### Workload Health

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **crashloop** | `KubernautCrashLoopDetected` | Bad config causes restarts >3 in 10m | Rollback to last working revision | `./deploy/demo/scenarios/crashloop/run.sh` |
| **crashloop-helm** | `KubernautCrashLoopDetected` | CrashLoop on Helm-managed release | `helm rollback` to previous revision | `./deploy/demo/scenarios/crashloop-helm/run.sh` |
| **memory-leak** | `KubernautPredictiveMemoryExhaust` | Linear memory growth predicted to OOM | Graceful restart (rolling) | `./deploy/demo/scenarios/memory-leak/run.sh` |
| **stuck-rollout** | `KubernautStuckRollout` | Non-existent image tag | `kubectl rollout undo` | `./deploy/demo/scenarios/stuck-rollout/run.sh` |
| **slo-burn** | `KubernautSLOBudgetBurning` | Blackbox probe error rate >1.44% | Proactive rollback | `./deploy/demo/scenarios/slo-burn/run.sh` |

### Autoscaling and Resources

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **hpa-maxed** | `KubernautHPAMaxedOut` | CPU load drives HPA to ceiling | Patch `maxReplicas` +2 | `./deploy/demo/scenarios/hpa-maxed/run.sh` |
| **pdb-deadlock** | `KubernautPDBDeadlock` | PDB blocks all disruptions | Relax PDB `minAvailable` | `./deploy/demo/scenarios/pdb-deadlock/run.sh` |
| **autoscale** | `KubePodSchedulingFailed` | Pods Pending (resource exhaustion) | Provision additional node | `./deploy/demo/scenarios/autoscale/run.sh` |

### Infrastructure

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **pending-taint** | `KubernautPodsPendingTaint` | NoSchedule taint on node | Remove taint | `./deploy/demo/scenarios/pending-taint/run.sh` |
| **node-notready** | `KubernautNodeNotReady` | Node failure simulation | Cordon + drain node | `./deploy/demo/scenarios/node-notready/run.sh` |
| **disk-pressure** | `KubernautOrphanedPVCs` | Orphaned PVCs accumulate | Cleanup unused PVCs | `./deploy/demo/scenarios/disk-pressure/run.sh` |
| **statefulset-pvc-failure** | `KubernautStatefulSetReplicasMismatch` | PVC binding failure | Fix StatefulSet PVC | `./deploy/demo/scenarios/statefulset-pvc-failure/run.sh` |

### Network and Service Mesh

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **network-policy-block** | `KubernautNetworkConnectivityLost` | Deny-all NetworkPolicy | Fix NetworkPolicy rules | `./deploy/demo/scenarios/network-policy-block/run.sh` |
| **mesh-routing-failure** | `KubernautHighErrorRate` | Restrictive AuthorizationPolicy | Fix AuthorizationPolicy | `./deploy/demo/scenarios/mesh-routing-failure/run.sh` |

### GitOps

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **gitops-drift** | `KubePodCrashLooping` | Bad commit via ArgoCD | `git revert` offending commit | `./deploy/demo/scenarios/gitops-drift/run.sh` |

### Certificates

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **cert-failure** | `KubernautCertificateNotReady` | cert-manager Certificate NotReady | Fix Certificate resource | `./deploy/demo/scenarios/cert-failure/run.sh` |
| **cert-failure-gitops** | `KubernautCertificateNotReady` | Certificate NotReady (GitOps) | `git revert` cert config | `./deploy/demo/scenarios/cert-failure-gitops/run.sh` |

## Cleanup

Each scenario deploys into its own namespace. To clean up after running:

```bash
# Per-scenario cleanup (if a cleanup.sh exists)
bash deploy/demo/scenarios/stuck-rollout/cleanup.sh

# Or delete the namespace directly
kubectl delete namespace demo-rollout
```

## Building Workflow Images

Scenario workflow images are pre-built and hosted at `quay.io/kubernaut-cicd/test-workflows/`. To rebuild locally:

```bash
./deploy/demo/scripts/build-demo-workflows.sh --local
./deploy/demo/scripts/build-demo-workflows.sh --scenario stuck-rollout --local
```

## Verification

### Check all pods are running
```bash
export KUBECONFIG=~/.kube/kubernaut-demo-config

kubectl get pods -n kubernaut-system
kubectl get pods -A
```

### Check workflow catalog
```bash
curl -s http://localhost:30081/api/v1/workflows | jq '.'
```

### Check RemediationRequests
```bash
kubectl get remediationrequests -A
```

### Check AIAnalysis results
```bash
kubectl get aianalyses -A -o wide
```

### Check WorkflowExecutions
```bash
kubectl get workflowexecutions -A -o wide
```

### View Prometheus alerts
```bash
curl -s http://localhost:9190/api/v1/alerts | jq '.'
```

### View AlertManager alerts
```bash
curl -s http://localhost:9193/api/v2/alerts | jq '.'
```

### Check audit events
```bash
curl -s http://localhost:30081/api/v1/audit-events | jq '.'
```

## Troubleshooting

### Pods stuck in ImagePullBackOff
Images were not loaded into Kind. Run:
```bash
make demo-load-images
```

### PostgreSQL not starting
Check pod events:
```bash
kubectl describe pod -l app=postgresql -n kubernaut-system
```

### HolmesGPT API errors
Check logs for LLM credential issues:
```bash
kubectl logs -l app=holmesgpt-api -n kubernaut-system
```

### No RemediationRequests created
1. Check Gateway logs: `kubectl logs -l app=gateway -n kubernaut-system`
2. Check Event Exporter logs: `kubectl logs -l app=event-exporter -n kubernaut-system`
3. Verify the scenario namespace has the `kubernaut.ai/managed: "true"` label (each scenario's `namespace.yaml` sets this)

### Prometheus not scraping metrics
Check Prometheus targets:
```bash
curl -s http://localhost:9190/api/v1/targets | jq '.data.activeTargets[] | {scrapeUrl, health}'
```

### AuthWebhook rejecting requests
Check webhook cert validity:
```bash
kubectl get secret authwebhook-tls -n kubernaut-system
kubectl logs -l app.kubernetes.io/name=authwebhook -n kubernaut-system
```

## Teardown

```bash
make demo-teardown
```

This deletes the Kind cluster and removes the kubeconfig file.

## Architecture

```
deploy/demo/
  base/                          # Base manifests (ClusterIP services)
    infrastructure/              # PostgreSQL, Redis, namespace
    monitoring/                  # Prometheus, AlertManager
    platform/                    # All 10 Kubernaut services + CRDs + RBAC
    workloads/                   # Legacy memory-eater workloads (OOMKill / high-usage)
  scenarios/                     # 17 demo scenarios (see Scenario Catalog above)
    <name>/
      run.sh                     # Automated scenario runner
      cleanup.sh                 # Teardown script (if applicable)
      README.md                  # BDD spec, acceptance criteria, manual steps
      manifests/                 # Namespace, Deployment, Service, PrometheusRule
      workflow/                  # workflow-schema.yaml + Dockerfile for OCI image
  overlays/
    kind/                        # Kind-specific patches (NodePort, node selectors)
  credentials/                   # LLM credential Secret examples
  scripts/                       # Migrations, TLS certs, workflow seeding, image building
```
