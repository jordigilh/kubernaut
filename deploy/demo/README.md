# Kubernaut Demo Installation Guide

This guide walks through running the Kubernaut demo scenarios on a local Kind cluster. Each scenario showcases the full remediation lifecycle: from signal detection through AI analysis to automated workflow execution.

All platform images are pulled from `quay.io/kubernaut-ai/` at runtime -- no local building is required.

## Prerequisites

- **Kind** v0.30+
- **kubectl** v1.34+
- **Helm** v3.14+
- **Container runtime**: Podman or Docker (for Kind)
- **LLM Provider credentials**: One of Vertex AI (GCP), Anthropic, or OpenAI

Memory: ~9GB available for the Kind cluster.

## Quick Start

```bash
# 1. Configure LLM credentials (one-time setup)
cp deploy/demo/credentials/vertex-ai-example.yaml my-llm-credentials.yaml
# Edit my-llm-credentials.yaml with your provider credentials

# 2. Pick a scenario and run it (everything is automatic)
cd deploy/demo/scenarios/stuck-rollout/
cat README.md
./run.sh

# 3. Apply LLM credentials once the cluster is running
kubectl apply -f my-llm-credentials.yaml
kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system
```

`run.sh` handles everything: Kind cluster creation, monitoring stack (kube-prometheus-stack), platform deployment (Kubernaut Helm chart), scenario manifests, and fault injection. If LLM credentials are missing, it prints a warning with setup instructions.

## Step-by-Step Setup

### 1. Configure LLM Credentials

Copy the appropriate example credential file and fill in your values:

**Vertex AI (recommended):**
```bash
cp deploy/demo/credentials/vertex-ai-example.yaml my-llm-credentials.yaml
# Edit my-llm-credentials.yaml with your GCP project ID
```

The default HolmesGPT API ConfigMap is pre-configured for Vertex AI with `claude-sonnet-4`. If using a different provider, update the LLM config in `charts/kubernaut/templates/holmesgpt-api/holmesgpt-api.yaml` ConfigMap before running.

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

After the first `run.sh` creates the cluster and deploys the platform, apply and activate:
```bash
kubectl apply -f my-llm-credentials.yaml
kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system
```

### Optional: Slack Notifications

To receive remediation notifications in Slack:

1. Create a [Slack Incoming Webhook](https://api.slack.com/messaging/webhooks) for your workspace
2. Create the Secret in-cluster:

```bash
kubectl create secret generic slack-webhook \
  -n kubernaut-system \
  --from-literal=webhook-url="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"

kubectl rollout restart deployment/notification-controller -n kubernaut-system
```

### 2. Run a Demo Scenario

Each scenario lives in `deploy/demo/scenarios/<name>/` with its own `README.md`, `run.sh`, manifests, alerting rules, fault-injection scripts, and workflow.

```bash
cd deploy/demo/scenarios/stuck-rollout/
cat README.md      # understand the scenario
./run.sh           # everything happens
```

What `run.sh` does automatically:

```
1. Creates Kind cluster (if not present)
2. Installs kube-prometheus-stack (monitoring)
3. Deploys Kubernaut platform (Helm chart + CRDs + hooks)
4. Installs scenario-specific dependencies (cert-manager, Linkerd, etc.)
5. Deploys scenario manifests (namespace, deployment, PrometheusRule)
6. Injects fault (bad image, CPU load, taint, etc.)
```

The remediation pipeline then runs:

```
Prometheus alert or K8s Event fires
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

22 scenarios are available, organized by category. Each scenario deploys into its own namespace and can be run independently.

For the formal specification of scenario structure, deliverables, and authoring guidelines, see [BR-PLATFORM-002: Demo Scenario Specification](../../docs/requirements/BR-PLATFORM-002-demo-scenario-specification.md).

Some scenarios require additional components beyond the base platform:

| Dependency | Scenarios | Notes |
|------------|-----------|-------|
| **kube-prometheus-stack** | All scenarios | Installed by `ensure_monitoring_stack` in each `run.sh` |
| **metrics-server** | hpa-maxed, autoscale | Required for HPA CPU metrics |
| **cert-manager** | cert-failure, cert-failure-gitops | Certificate lifecycle management |
| **Linkerd** | mesh-routing-failure | Service mesh control plane |
| **blackbox-exporter** | slo-burn | HTTP probe metrics (probe_success) |
| **Helm CLI** | crashloop-helm | Helm-managed release rollback |
| **ArgoCD** | gitops-drift, cert-failure-gitops | GitOps delivery |

Each scenario's `README.md` lists its specific prerequisites. All dependencies are installed automatically by `run.sh`.

### Workload Health

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **crashloop** | `KubePodCrashLooping` | Bad config causes restarts >3 in 10m | Rollback to last working revision | `./deploy/demo/scenarios/crashloop/run.sh` |
| **crashloop-helm** | `KubePodCrashLooping` | CrashLoop on Helm-managed release | `helm rollback` to previous revision | `./deploy/demo/scenarios/crashloop-helm/run.sh` |
| **memory-leak** | `ContainerMemoryExhaustionPredicted` | Linear memory growth predicted to OOM | Graceful restart (rolling) | `./deploy/demo/scenarios/memory-leak/run.sh` |
| **stuck-rollout** | `KubeDeploymentRolloutStuck` | Non-existent image tag | `kubectl rollout undo` | `./deploy/demo/scenarios/stuck-rollout/run.sh` |
| **slo-burn** | `ErrorBudgetBurn` | Blackbox probe error rate >1.44% | Proactive rollback | `./deploy/demo/scenarios/slo-burn/run.sh` |
| **memory-escalation** | `ContainerMemoryHigh` | Memory usage exceeds threshold | Increase memory limits | `./deploy/demo/scenarios/memory-escalation/run.sh` |

### Autoscaling and Resources

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **hpa-maxed** | `KubeHpaMaxedOut` | CPU load drives HPA to ceiling | Patch `maxReplicas` +2 | `./deploy/demo/scenarios/hpa-maxed/run.sh` |
| **pdb-deadlock** | `KubePodDisruptionBudgetAtLimit` | PDB blocks all disruptions | Relax PDB `minAvailable` | `./deploy/demo/scenarios/pdb-deadlock/run.sh` |
| **autoscale** | `KubePodSchedulingFailed` | Pods Pending (resource exhaustion) | Provision additional node | `./deploy/demo/scenarios/autoscale/run.sh` |

### Infrastructure

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **pending-taint** | `KubePodNotScheduled` | NoSchedule taint on node | Remove taint | `./deploy/demo/scenarios/pending-taint/run.sh` |
| **node-notready** | `KubeNodeNotReady` | Node failure simulation | Cordon + drain node | `./deploy/demo/scenarios/node-notready/run.sh` |
| **disk-pressure** | `KubePersistentVolumeClaimOrphaned` | Orphaned PVCs accumulate | No action (no workflow seeded) | `./deploy/demo/scenarios/disk-pressure/run.sh` |
| **statefulset-pvc-failure** | `KubeStatefulSetReplicasMismatch` | PVC binding failure | Fix StatefulSet PVC | `./deploy/demo/scenarios/statefulset-pvc-failure/run.sh` |

### Network and Service Mesh

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **network-policy-block** | `KubePodCrashLooping` / `KubeDeploymentReplicasMismatch` | Deny-all NetworkPolicy | Fix NetworkPolicy rules | `./deploy/demo/scenarios/network-policy-block/run.sh` |
| **mesh-routing-failure** | `LinkerdHighErrorRate` / `LinkerdRequestsUnauthorized` | Restrictive AuthorizationPolicy | Fix AuthorizationPolicy | `./deploy/demo/scenarios/mesh-routing-failure/run.sh` |

### GitOps

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **gitops-drift** | `KubePodCrashLooping` | Bad commit via ArgoCD | `git revert` offending commit | `./deploy/demo/scenarios/gitops-drift/run.sh` |

### Certificates

| Scenario | Signal / Alert | Fault Injection | Remediation | Run |
|----------|---------------|-----------------|-------------|-----|
| **cert-failure** | `CertManagerCertNotReady` | cert-manager Certificate NotReady | Fix Certificate resource | `./deploy/demo/scenarios/cert-failure/run.sh` |
| **cert-failure-gitops** | `CertManagerCertNotReady` | Certificate NotReady (GitOps) | `git revert` cert config | `./deploy/demo/scenarios/cert-failure-gitops/run.sh` |

### Platform Behavior

| Scenario | Signal / Alert | Fault Injection | Behavior Tested | Run |
|----------|---------------|-----------------|-----------------|-----|
| **duplicate-alert-suppression** | `KubePodCrashLooping` | Bad config (same as crashloop) | Deduplication suppresses duplicate RRs | `./deploy/demo/scenarios/duplicate-alert-suppression/run.sh` |
| **resource-quota-exhaustion** | `KubeResourceQuotaExhausted` | Exhaust namespace ResourceQuota | Pipeline handles quota-blocked scenarios | `./deploy/demo/scenarios/resource-quota-exhaustion/run.sh` |
| **concurrent-cross-namespace** | `KubePodCrashLooping` (x2) | Bad config in two namespaces | Concurrent pipelines with cross-namespace rego policy | `./deploy/demo/scenarios/concurrent-cross-namespace/run.sh` |

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
kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090 &
curl -s http://localhost:9090/api/v1/alerts | jq '.'
```

### View AlertManager alerts
```bash
kubectl port-forward -n monitoring svc/kube-prometheus-stack-alertmanager 9093:9093 &
curl -s http://localhost:9093/api/v2/alerts | jq '.'
```

### Check audit events
```bash
curl -s http://localhost:30081/api/v1/audit-events | jq '.'
```

## Troubleshooting

### Pods stuck in ImagePullBackOff
Images are pulled from `quay.io/kubernaut-ai/`. Check that the Kind cluster has internet access and the image tag exists:
```bash
kubectl describe pod <pod-name> -n kubernaut-system
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
kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090 &
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {scrapeUrl, health}'
```

### AuthWebhook rejecting requests
Check webhook cert validity:
```bash
kubectl get secret authwebhook-tls -n kubernaut-system
kubectl logs -l app.kubernetes.io/name=authwebhook -n kubernaut-system
```

## Teardown

```bash
kind delete cluster --name kubernaut-demo
```

## Architecture

```
charts/kubernaut/                  # Kubernaut platform Helm chart (Issue #80)
  Chart.yaml                       # name: kubernaut, appVersion: demo-v1.0
  values.yaml                      # Configurable: images, resources, service types
  crds/                            # 8 Kubernaut CRDs (applied before chart install)
  templates/                       # 10 services + infrastructure + RBAC + hooks

deploy/demo/
  scenarios/                       # 22 demo scenarios (see Scenario Catalog above)
    <name>/
      run.sh                       # Single entry point (cluster + monitoring + platform + scenario)
      cleanup.sh                   # Teardown script (if applicable)
      README.md                    # BDD spec, acceptance criteria, manual steps
      manifests/                   # Namespace, Deployment, Service, PrometheusRule
      workflow/                    # workflow-schema.yaml + Dockerfile for OCI image
  helm/                            # Helm values: kube-prometheus-stack + Kubernaut Kind overrides
  credentials/                     # LLM credential Secret examples
  scripts/                         # Shared helpers: kind, monitoring, platform
  overlays/kind/                   # Kind cluster config (port mappings, node topology)
```
