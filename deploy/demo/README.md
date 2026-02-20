# Kubernaut Demo Installation Guide

This guide walks through deploying the complete Kubernaut platform on a local Kind cluster for demonstration purposes. The demo showcases the full remediation lifecycle: from signal detection through AI analysis to automated workflow execution.

## Prerequisites

- **Go** 1.25+
- **Kind** v0.20+
- **kubectl** v1.29+
- **Container runtime**: Podman or Docker
- **openssl** (for AuthWebhook TLS cert generation)
- **curl** and **jq** (for workflow seeding and verification)
- **LLM Provider credentials**: One of Vertex AI (GCP), Anthropic, or OpenAI

Memory: ~6GB available for the Kind cluster.

## Quick Start

```bash
# Full automated setup (builds images, creates cluster, deploys everything)
make demo-setup

# Apply your LLM credentials (see "LLM Configuration" below)
kubectl --kubeconfig ~/.kube/kubernaut-demo-config apply -f my-llm-credentials.yaml

# Restart HolmesGPT API to pick up credentials
kubectl --kubeconfig ~/.kube/kubernaut-demo-config rollout restart deployment/holmesgpt-api -n kubernaut-system

# Deploy demo workloads to trigger remediation
kubectl --kubeconfig ~/.kube/kubernaut-demo-config apply -k deploy/demo/base/workloads/
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

Creates a Kind cluster named `kubernaut-demo` with NodePort mappings:

| Service | Host Port | NodePort |
|---------|-----------|----------|
| Gateway | 30080 | 30080 |
| DataStorage | 30081 | 30081 |
| Prometheus | 9190 | 30190 |
| AlertManager | 9193 | 30193 |

Kubeconfig: `~/.kube/kubernaut-demo-config`

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

The default HolmesGPT API ConfigMap is pre-configured for Vertex AI with `claude-sonnet-4`. If using a different provider, also update `deploy/demo/base/platform/holmesgpt-api.yaml` ConfigMap before deploying.

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

### 6. Deploy Demo Workloads

Two workload variants are available via Make targets:

**OOMKill scenario** (Kubernetes Event Exporter signal):
```bash
make demo-trigger-oomkill
```

**High memory usage scenario** (Prometheus AlertManager signal):
```bash
make demo-trigger-high-usage
```

**Reset workloads** (clean slate for re-triggering):
```bash
make demo-reset-workloads
```

You can also apply manifests directly:
```bash
kubectl --kubeconfig ~/.kube/kubernaut-demo-config apply -f deploy/demo/base/workloads/memory-eater-oomkill.yaml
```

## Expected Remediation Flow

### OOMKill Path
```
memory-eater OOMKill event
  -> Event Exporter detects OOMKilled event
  -> Gateway receives webhook, creates RemediationRequest CRD
  -> SignalProcessing classifies signal (environment, severity, priority)
  -> RemediationOrchestrator creates AIAnalysis CRD
  -> AIAnalysis controller calls HolmesGPT API
  -> HolmesGPT API investigates pod (reads logs, events, resource limits)
  -> HolmesGPT API discovers matching workflow from DataStorage catalog
  -> LLM recommends "oomkill-increase-memory-job" workflow
  -> RemediationOrchestrator creates WorkflowExecution CRD
  -> WorkflowExecution controller runs remediation Job
  -> Notification controller sends notification
  -> EffectivenessMonitor assesses remediation success
```

### Prometheus Alert Path
```
memory-eater high memory usage
  -> Prometheus scrapes cAdvisor metrics
  -> Alert rule "MemoryExceedsLimit" fires (>80% memory for 30s)
  -> AlertManager routes alert to Gateway webhook
  -> Gateway creates RemediationRequest CRD
  -> (same flow as OOMKill from here)
```

## Verification

### Check all pods are running
```bash
export KUBECONFIG=~/.kube/kubernaut-demo-config

kubectl get pods -n kubernaut-system
kubectl get pods -n demo-workloads
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
3. Verify demo-workloads namespace has `kubernaut.ai/managed: "true"` label

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
    workloads/                   # Memory-eater demo workloads
  overlays/
    kind/                        # Kind-specific patches (NodePort, node selectors)
  credentials/                   # LLM credential Secret examples
  scripts/                       # Migrations, TLS certs, workflow seeding
```
