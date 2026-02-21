# Scenario #130: Stuck Rollout

## Overview

A deployment update gets stuck because the new image tag doesn't exist. After exceeding
`progressDeadlineSeconds`, Kubernetes marks the rollout as not progressing. Kubernaut
detects this and rolls back to the previous working revision.

**Signal**: `KubeDeploymentRolloutStuck` -- Progressing condition is False for >1 min
**Fault injection**: `kubectl set image` with non-existent tag
**Remediation**: `kubectl rollout undo` to restore previous image

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/overlays/kind/kind-cluster-config.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With kube-state-metrics |
| Workflow catalog | `rollback-deployment-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/stuck-rollout/run.sh
```

## Cleanup

```bash
kubectl delete namespace demo-rollout
```

## BDD Specification

```gherkin
Given a Kind cluster with Kubernaut services and a real LLM backend
  And the "rollback-deployment-v1" workflow is registered in DataStorage
  And the "checkout-api" deployment is running with 3 healthy replicas

When the deployment image is updated to a non-existent tag
  And new pods enter ImagePullBackOff
  And the rollout exceeds progressDeadlineSeconds (120s)
  And the KubeDeploymentRolloutStuck alert fires

Then the LLM diagnoses the stuck rollout from a bad image reference
  And selects the GracefulRestart (rollback) workflow
  And WE rolls back to the previous working revision
  And the original nginx:1.27-alpine image is restored
  And all 3 replicas become Running/Ready
  And EM confirms the deployment is healthy
```

## Acceptance Criteria

- [ ] Deployment starts healthy with 3 replicas
- [ ] Bad image causes ImagePullBackOff on new pods
- [ ] Rollout exceeds progressDeadlineSeconds
- [ ] Alert fires within ~3 minutes of injection
- [ ] LLM correctly identifies bad image as root cause
- [ ] Rollback restores the original working image
- [ ] All replicas healthy after rollback
- [ ] EM confirms successful remediation
