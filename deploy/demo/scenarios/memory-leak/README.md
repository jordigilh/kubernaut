# Scenario #129: Predictive Memory Exhaustion

## Overview

Demonstrates Kubernaut's **predictive remediation** capability. Instead of waiting for
an OOM kill, Prometheus `predict_linear()` detects that a container's memory is growing
linearly and will exceed its limit within 30 minutes. Kubernaut intervenes with a
graceful rolling restart that resets memory before the crash occurs.

**Key differentiator**: The pod never crashes. Kubernaut acts on a *prediction*, not a symptom.

## Signal Flow

```
predict_linear() → ContainerMemoryExhaustionPredicted alert
  → Gateway → SP → AA (HAPI + real LLM)
  → LLM diagnoses linear memory growth in "leaker" container
  → Selects GracefulRestart workflow
  → RO → WE (kubectl rollout restart)
  → EM verifies memory reset to baseline
```

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/overlays/kind/kind-cluster-config.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With cAdvisor scraping and kube-state-metrics |
| Workflow catalog | `graceful-restart-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/memory-leak/run.sh
```

## Manual Step-by-Step

### 1. Deploy the workload

```bash
kubectl apply -f deploy/demo/scenarios/memory-leak/manifests/namespace.yaml
kubectl apply -f deploy/demo/scenarios/memory-leak/manifests/deployment.yaml
kubectl apply -f deploy/demo/scenarios/memory-leak/manifests/prometheus-rule.yaml
kubectl wait --for=condition=Available deployment/leaky-app -n demo-memory-leak --timeout=120s
```

### 2. Observe memory growth

```bash
# Watch memory climb (~4MB/min in the leaker container)
watch kubectl top pods -n demo-memory-leak --containers
```

### 3. Wait for the predictive alert

The `ContainerMemoryExhaustionPredicted` alert fires once `predict_linear()` projects the
leaker container will exceed its 192Mi limit within 30 minutes. This typically takes
10-15 minutes of trend data.

Check: `kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090` then open `http://localhost:9090/alerts`

### 4. Monitor the Kubernaut pipeline

```bash
kubectl get rr,sp,aa,we,ea -n demo-memory-leak -w
```

Expected flow:
- **SP** enriches the alert with business labels
- **AA (HAPI)** diagnoses predictive memory exhaustion, selects GracefulRestart
- **RO** creates the WorkflowExecution
- **WE** runs `kubectl rollout restart deployment/leaky-app`
- **EM** confirms memory usage is back near baseline

### 5. Verify remediation

```bash
# Memory should be back near baseline
kubectl top pods -n demo-memory-leak --containers

# Deployment has a new revision
kubectl rollout history deployment/leaky-app -n demo-memory-leak
```

## Cleanup

```bash
kubectl delete namespace demo-memory-leak
```

## BDD Specification

```gherkin
Given a Kind cluster with Kubernaut services and a real LLM backend
  And Prometheus is scraping cAdvisor metrics and kube-state-metrics
  And the "graceful-restart-v1" workflow is registered in the DataStorage catalog
  And the "leaky-app" deployment is running in namespace "demo-memory-leak"
  And the "leaker" sidecar container has a memory limit of 192Mi

When the "leaker" container's memory usage grows linearly at ~4MB/min
  And Prometheus predict_linear() projects OOM within 30 minutes
  And the ContainerMemoryExhaustionPredicted alert fires

Then Kubernaut Gateway receives the alert via Alertmanager webhook
  And Signal Processing enriches the signal with business labels
  And AI Analysis (HAPI + LLM) diagnoses a predictive memory exhaustion
  And the LLM selects the "GracefulRestart" workflow (graceful-restart-v1)
  And Remediation Orchestrator creates a WorkflowExecution
  And Workflow Execution runs a rolling restart of the deployment
  And all pods restart with memory reset to baseline
  And Effectiveness Monitor confirms the deployment is healthy
  And the pod never experienced an OOM kill
```

## Acceptance Criteria

- [ ] `predict_linear()` alert fires before any OOM kill
- [ ] LLM correctly identifies the linear memory growth pattern
- [ ] `GracefulRestart` workflow is selected (not a scale-up or other action)
- [ ] Rolling restart completes without downtime (2 replicas, rolling update)
- [ ] Memory usage drops back near baseline after restart
- [ ] EM confirms successful remediation
- [ ] No pod in the namespace experienced an OOM kill during the demo
