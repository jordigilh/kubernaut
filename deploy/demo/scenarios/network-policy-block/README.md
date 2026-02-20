# Scenario #138: NetworkPolicy Traffic Block

## Overview

Demonstrates Kubernaut detecting a Deployment with unavailable replicas caused by a deny-all NetworkPolicy blocking ingress traffic. Health checks fail, pods become NotReady and restart. Remediation removes the offending deny-all NetworkPolicy to restore traffic.

**Signal**: `KubernautDeploymentUnavailable` -- from `kube_deployment_status_replicas_unavailable` > 0
**Root cause**: Deny-all NetworkPolicy blocks all ingress; liveness/readiness probes fail
**Remediation**: `fix-network-policy-v1` workflow removes the offending deny-all policy

## Signal Flow

```
kube_deployment_status_replicas_unavailable > 0 for 3m
  → KubernautDeploymentUnavailable alert
  → Gateway → SP → AA (HAPI + LLM)
  → LLM detects networkIsolated=true, diagnoses NetworkPolicy block
  → Selects FixNetworkPolicy workflow
  → RO → WE (delete deny-all NetworkPolicy)
  → EM verifies all replicas ready
```

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/scenarios/kind-config-singlenode.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (or mock) via HAPI |
| Prometheus | With kube-state-metrics |
| Workflow catalog | `fix-network-policy-v1` registered in DataStorage |

## Detected Label

- **networkIsolated**: `true` -- indicates NetworkPolicy is blocking traffic; remediation removes the offending policy

## Automated Run

```bash
./deploy/demo/scenarios/network-policy-block/run.sh
```

## Manual Step-by-Step

### 1. Deploy scenario resources

```bash
kubectl apply -f deploy/demo/scenarios/network-policy-block/manifests/namespace.yaml
kubectl apply -f deploy/demo/scenarios/network-policy-block/manifests/deployment.yaml
kubectl apply -f deploy/demo/scenarios/network-policy-block/manifests/networkpolicy-allow.yaml
kubectl apply -f deploy/demo/scenarios/network-policy-block/manifests/prometheus-rule.yaml
```

### 2. Wait for deployment to be healthy

```bash
kubectl wait --for=condition=Available deployment/web-frontend -n demo-netpol --timeout=120s
kubectl get pods -n demo-netpol
```

### 3. Inject deny-all NetworkPolicy

```bash
bash deploy/demo/scenarios/network-policy-block/inject-deny-all-netpol.sh
```

The script applies a deny-all NetworkPolicy that blocks all ingress traffic. Liveness and readiness probes (HTTP on port 8080) will fail because kubelet cannot reach the pods.

### 4. Wait for alert and pipeline

```bash
# Alert fires after ~3 min of unavailable replicas
# Check: http://localhost:9190/alerts
kubectl get rr,sp,aa,we,ea -n demo-netpol -w
```

### 5. Verify remediation

```bash
kubectl get networkpolicies -n demo-netpol
kubectl get pods -n demo-netpol
# deny-all-ingress should be removed, all pods Running and Ready
```

## Cleanup

```bash
./deploy/demo/scenarios/network-policy-block/cleanup.sh
```

## BDD Specification

```gherkin
Feature: NetworkPolicy Traffic Block remediation

  Scenario: Deny-all NetworkPolicy blocks health checks
    Given a Deployment "web-frontend" in namespace "demo-netpol"
    And the deployment has 2 replicas with HTTP liveness/readiness probes
    And a baseline NetworkPolicy "allow-web-traffic" permits port 8080
    And all pods are Running and Ready
    When a deny-all NetworkPolicy "deny-all-ingress" is applied
    Then all ingress traffic to pods is blocked
    And liveness and readiness probes fail
    And pods become NotReady and may restart
    And the KubernautDeploymentUnavailable alert fires (unavailable > 0 for 3 min)

  Scenario: fix-network-policy-v1 remediates traffic block
    Given the Deployment has unavailable replicas due to blocked traffic
    And the pipeline detects networkIsolated=true
    When the fix-network-policy-v1 workflow executes
    Then the workflow removes the deny-all NetworkPolicy "deny-all-ingress"
    And health checks succeed again
    And all pods become Ready
    And the Deployment has desired replicas available
```

## Acceptance Criteria

- [ ] Deployment deploys with 2 replicas and HTTP probes
- [ ] Baseline allow policy permits port 8080
- [ ] All pods start Running and Ready
- [ ] Deny-all NetworkPolicy blocks ingress; probes fail
- [ ] Pods become NotReady; Deployment shows unavailable replicas
- [ ] Alert fires within 3-4 minutes of unavailable replicas
- [ ] LLM correctly detects networkIsolated=true and diagnoses NetworkPolicy block
- [ ] fix-network-policy-v1 workflow removes the deny-all NetworkPolicy
- [ ] All replicas become Ready after remediation
- [ ] EM confirms successful remediation
