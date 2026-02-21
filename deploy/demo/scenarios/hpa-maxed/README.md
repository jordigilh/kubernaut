# Scenario #123: HPA Maxed Out

## Overview

Demonstrates Kubernaut leveraging **detected labels** (`hpaEnabled`) to contextually
remediate an HPA that has hit its `maxReplicas` ceiling during a traffic spike. The LLM
knows an HPA exists and patches it to temporarily raise the ceiling.

**Detected label**: `hpaEnabled: "true"` -- LLM context includes HPA configuration
**Signal**: `KubeHpaMaxedOut` -- HPA at maxReplicas for >2 min
**Remediation**: Patch HPA to increase `maxReplicas` by 2

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/overlays/kind/kind-cluster-config.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With kube-state-metrics |
| metrics-server | Required for HPA CPU metric collection |
| Workflow catalog | `patch-hpa-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/hpa-maxed/run.sh
```

## Manual Step-by-Step

### 1. Deploy the workload with HPA

```bash
kubectl apply -f deploy/demo/scenarios/hpa-maxed/manifests/namespace.yaml
kubectl apply -f deploy/demo/scenarios/hpa-maxed/manifests/deployment.yaml
kubectl apply -f deploy/demo/scenarios/hpa-maxed/manifests/prometheus-rule.yaml
kubectl wait --for=condition=Available deployment/api-frontend -n demo-hpa --timeout=120s
```

### 2. Verify HPA

```bash
kubectl get hpa -n demo-hpa
# Should show: MINPODS=2, MAXPODS=3, REPLICAS=2
```

### 3. Inject CPU load

```bash
bash deploy/demo/scenarios/hpa-maxed/inject-load.sh
```

### 4. Watch HPA scale to ceiling

```bash
kubectl get hpa -n demo-hpa -w
# Replicas will climb to 3 (maxReplicas) and stay there
```

### 5. Wait for alert and pipeline

The `KubeHpaMaxedOut` alert fires when currentReplicas == maxReplicas for 2 minutes.

### 6. Verify remediation

```bash
kubectl get hpa -n demo-hpa
# maxReplicas should be 5 (raised from 3 by the workflow)
```

## Cleanup

```bash
kubectl delete namespace demo-hpa
```

## BDD Specification

```gherkin
Given a Kind cluster with Kubernaut services and a real LLM backend
  And metrics-server is installed for HPA CPU metrics
  And the "patch-hpa-v1" workflow is registered with detectedLabels: hpaEnabled: "true"
  And the "api-frontend" deployment has an HPA with maxReplicas=3

When CPU load drives the HPA to its maxReplicas ceiling
  And the HPA cannot scale further despite continued pressure
  And the KubeHpaMaxedOut alert fires

Then Kubernaut detects the hpaEnabled label on the namespace/deployment
  And the LLM receives HPA context in its analysis prompt
  And the LLM selects the PatchHPA workflow
  And WE patches the HPA maxReplicas to 5
  And the HPA scales up to meet demand
  And EM verifies the additional pods are healthy
```

## Acceptance Criteria

- [ ] HPA reaches maxReplicas under load
- [ ] Alert fires after 2 minutes at ceiling
- [ ] LLM leverages `hpaEnabled` detected label in diagnosis
- [ ] PatchHPA workflow is selected
- [ ] `maxReplicas` is raised (3 -> 5)
- [ ] HPA scales beyond the original ceiling
- [ ] EM confirms all new pods are healthy
