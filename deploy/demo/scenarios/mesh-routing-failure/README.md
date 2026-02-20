# Scenario #136: Linkerd Service Mesh Routing Failure

## Overview

Demonstrates Kubernaut detecting a Linkerd-meshed workload with high error rates caused by a restrictive AuthorizationPolicy blocking legitimate traffic. The Linkerd proxy returns 403 Forbidden for inbound requests, causing health check failures and service unavailability. Kubernaut automatically remediates by removing or fixing the blocking policy.

**Signal**: `KubernautHighErrorRate` / `KubernautMeshUnauthorized` -- from Linkerd proxy metrics (`response_total` with `classification="failure"` or `status_code="403"`)

**Root cause**: Restrictive Linkerd AuthorizationPolicy requiring MeshTLSAuthentication with a non-existent identity, denying all inbound traffic

**Remediation**: `fix-authz-policy-v1` workflow removes the blocking AuthorizationPolicy and restores traffic flow

## Signal Flow

```
Linkerd proxy metrics: response_total (failure/403) > 50% for 2m
  → KubernautHighErrorRate / KubernautMeshUnauthorized alert
  → Gateway → SP → AA (HAPI + LLM)
  → LLM detects serviceMesh=linkerd, diagnoses AuthorizationPolicy block
  → Selects FixAuthorizationPolicy workflow (fix-authz-policy-v1)
  → RO → WE (remove deny-all AuthorizationPolicy)
  → EM verifies traffic restored, pods Ready
```

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/scenarios/kind-config-singlenode.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | Scraping Linkerd proxy metrics |
| Linkerd | Installed (run.sh installs if missing) |
| Workflow catalog | `fix-authz-policy-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/mesh-routing-failure/run.sh
```

## Manual Step-by-Step

### 1. Install Linkerd (if not present)

```bash
linkerd install --crds | kubectl apply -f -
linkerd install | kubectl apply -f -
linkerd check --wait 120s
```

### 2. Deploy workload

```bash
kubectl apply -f deploy/demo/scenarios/mesh-routing-failure/manifests/namespace.yaml
kubectl apply -f deploy/demo/scenarios/mesh-routing-failure/manifests/deployment.yaml
kubectl apply -f deploy/demo/scenarios/mesh-routing-failure/manifests/prometheus-rule.yaml
kubectl wait --for=condition=Available deployment/api-server -n demo-mesh-failure --timeout=120s
```

### 3. Establish baseline

```bash
# Wait ~20s for healthy traffic
linkerd viz stat deploy -n demo-mesh-failure
```

### 4. Inject failure

```bash
bash deploy/demo/scenarios/mesh-routing-failure/inject-deny-policy.sh
```

The script applies a restrictive AuthorizationPolicy that requires MeshTLSAuthentication with a non-existent identity, causing the Linkerd proxy to deny all inbound traffic with 403 Forbidden.

### 5. Observe high error rate

```bash
kubectl get pods -n demo-mesh-failure -w
# Pods may become NotReady as health checks fail
linkerd viz stat deploy -n demo-mesh-failure
# High failure rate visible
```

### 6. Wait for alert and pipeline

```bash
# Alert fires after ~2 min of high error rate
# Check: http://localhost:9190/alerts
kubectl get rr,sp,aa,we,ea -n demo-mesh-failure -w
```

### 7. Verify remediation

```bash
kubectl get authorizationpolicies.policy.linkerd.io -n demo-mesh-failure
# deny-all-traffic should be removed
kubectl get pods -n demo-mesh-failure
# All pods should be Running and Ready
```

## Cleanup

```bash
./deploy/demo/scenarios/mesh-routing-failure/cleanup.sh
```

## BDD Specification

```gherkin
Feature: Linkerd Service Mesh Routing Failure remediation

  Given a Kind cluster with Kubernaut services and a real LLM backend
    And Prometheus is scraping Linkerd proxy metrics
    And the "fix-authz-policy-v1" workflow is registered in the DataStorage catalog
    And Linkerd is installed with mesh injection enabled
    And the "api-server" deployment is meshed in namespace "demo-mesh-failure"
    And the workload is healthy with traffic flowing through the Linkerd proxy

  When a restrictive AuthorizationPolicy is applied requiring MeshTLSAuthentication
    And the policy references a non-existent identity
    And the Linkerd proxy denies all inbound traffic with 403 Forbidden
    And health checks fail and pods may become NotReady
    And the KubernautHighErrorRate or KubernautMeshUnauthorized alert fires (2 min)

  Then Kubernaut Gateway receives the alert via Alertmanager webhook
    And Signal Processing enriches the signal with business labels
    And AI Analysis (HAPI + LLM) detects serviceMesh=linkerd
    And the LLM diagnoses AuthorizationPolicy as root cause
    And the LLM selects the "FixAuthorizationPolicy" workflow (fix-authz-policy-v1)
    And Remediation Orchestrator creates a WorkflowExecution
    And Workflow Execution removes the deny-all AuthorizationPolicy
    And traffic is restored through the Linkerd proxy
    And Effectiveness Monitor confirms pods are Ready and error rate drops
```

## Acceptance Criteria

- [ ] Namespace has `linkerd.io/inject: enabled` annotation; workload gets Linkerd sidecar
- [ ] Baseline: traffic flows, pods Ready, no high error rate
- [ ] deny-all AuthorizationPolicy causes Linkerd proxy to return 403 for inbound traffic
- [ ] PrometheusRule fires KubernautHighErrorRate or KubernautMeshUnauthorized within 2-3 min
- [ ] LLM correctly detects serviceMesh=linkerd and diagnoses AuthorizationPolicy block
- [ ] fix-authz-policy-v1 workflow removes the blocking AuthorizationPolicy
- [ ] After remediation, traffic flows and pods are Ready
- [ ] EM confirms successful remediation
