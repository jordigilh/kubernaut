# Scenario #137: StatefulSet PVC Failure

## Overview

Demonstrates Kubernaut detecting a StatefulSet-based workload with PVC disruption causing pods stuck in Pending, and performing automatic remediation by recreating the missing PVC and deleting the stuck pod to allow rescheduling.

**Signal**: `KubernautStatefulSetReplicasMismatch` -- from `kube_statefulset_status_replicas_ready` < `kube_statefulset_status_replicas`
**Root cause**: PVC (and backing PV) deleted; pod cannot bind and remains Pending
**Remediation**: `fix-statefulset-pvc-v1` workflow recreates PVC, deletes stuck pod

## Signal Flow

```
kube_statefulset_status_replicas_ready < kube_statefulset_status_replicas for 3m
  → KubernautStatefulSetReplicasMismatch alert
  → Gateway → SP → AA (HAPI + LLM)
  → LLM detects stateful=true, diagnoses PVC failure
  → Selects FixStatefulSetPVC workflow
  → RO → WE (recreate PVC, delete stuck pod)
  → EM verifies all replicas ready
```

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/scenarios/kind-config-singlenode.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (or mock) via HAPI |
| Prometheus | With kube-state-metrics |
| Workflow catalog | `fix-statefulset-pvc-v1` registered in DataStorage |

## Detected Label

- **stateful**: `true` -- indicates StatefulSet workload; remediation uses StatefulSet-aware logic (recreate PVC, delete pod)

## Automated Run

```bash
./deploy/demo/scenarios/statefulset-pvc-failure/run.sh
```

## Manual Step-by-Step

### 1. Deploy scenario resources

```bash
kubectl apply -f deploy/demo/scenarios/statefulset-pvc-failure/manifests/namespace.yaml
kubectl apply -f deploy/demo/scenarios/statefulset-pvc-failure/manifests/statefulset.yaml
kubectl apply -f deploy/demo/scenarios/statefulset-pvc-failure/manifests/prometheus-rule.yaml
```

### 2. Wait for StatefulSet to be ready

```bash
kubectl rollout status statefulset/kv-store -n demo-statefulset --timeout=180s
kubectl get pods -n demo-statefulset
```

### 3. Inject PVC failure

```bash
bash deploy/demo/scenarios/statefulset-pvc-failure/inject-pvc-issue.sh
```

The script deletes the pod `kv-store-2`, its PVC `data-kv-store-2`, and the backing PV. The pod will be recreated by the StatefulSet controller but will remain Pending because the PVC no longer exists.

### 4. Wait for alert and pipeline

```bash
# Alert fires after ~3 min of replicas mismatch
# Check: http://localhost:9190/alerts
kubectl get rr,sp,aa,we,ea -n demo-statefulset -w
```

### 5. Verify remediation

```bash
kubectl get pods -n demo-statefulset
kubectl get pvc -n demo-statefulset
# All 3 pods should be Running, all PVCs Bound
```

## Cleanup

```bash
./deploy/demo/scenarios/statefulset-pvc-failure/cleanup.sh
```

## BDD Specification

```gherkin
Feature: StatefulSet PVC Failure remediation

  Scenario: PVC deleted causes pod stuck in Pending
    Given a StatefulSet "kv-store" in namespace "demo-statefulset"
    And the StatefulSet has 3 replicas with volumeClaimTemplate "data"
    And all pods are Running and Ready
    When the PVC "data-kv-store-2" and its backing PV are deleted
    And the pod "kv-store-2" is deleted to trigger recreation
    Then the pod "kv-store-2" is recreated but remains Pending
    And the KubernautStatefulSetReplicasMismatch alert fires (2/3 ready for 3 min)

  Scenario: fix-statefulset-pvc-v1 remediates PVC failure
    Given the StatefulSet has fewer ready replicas than desired
    And the pipeline detects stateful=true
    When the fix-statefulset-pvc-v1 workflow executes
    Then the workflow recreates the missing PVC "data-kv-store-2"
    And the workflow deletes the stuck pod "kv-store-2"
    And the StatefulSet controller recreates the pod
    And the pod binds to the new PVC and becomes Running
    And all 3 replicas are Ready
```

## Acceptance Criteria

- [ ] StatefulSet deploys with 3 replicas and volumeClaimTemplate
- [ ] All pods start Running with PVCs Bound
- [ ] PVC/PV deletion causes pod kv-store-2 to remain Pending
- [ ] Alert fires within 3-4 minutes of replicas mismatch
- [ ] LLM correctly detects stateful=true and diagnoses PVC failure
- [ ] fix-statefulset-pvc-v1 workflow recreates the missing PVC
- [ ] Workflow deletes stuck pod to trigger reschedule
- [ ] All 3 StatefulSet replicas become Ready after remediation
- [ ] EM confirms successful remediation
