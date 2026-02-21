# Scenario #124: PDB Deadlock

## Overview

Demonstrates Kubernaut leveraging **detected labels** (`pdbProtected`) to resolve a
PodDisruptionBudget deadlock. The PDB has `minAvailable` equal to the replica count,
leaving zero allowed disruptions and blocking all rolling updates and voluntary evictions.

**Detected label**: `pdbProtected: "true"` -- LLM context includes PDB configuration
**Signal**: `KubePodDisruptionBudgetAtLimit` -- PDB at 0 allowed disruptions for >3 min
**Remediation**: Patch PDB `minAvailable` from 2 to 1, unblocking the rollout

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/overlays/kind/kind-cluster-config.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With kube-state-metrics |
| Workflow catalog | `relax-pdb-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/pdb-deadlock/run.sh
```

## Manual Step-by-Step

### 1. Deploy the workload with restrictive PDB

```bash
kubectl apply -f deploy/demo/scenarios/pdb-deadlock/manifests/namespace.yaml
kubectl apply -f deploy/demo/scenarios/pdb-deadlock/manifests/deployment.yaml
kubectl apply -f deploy/demo/scenarios/pdb-deadlock/manifests/prometheus-rule.yaml
kubectl wait --for=condition=Available deployment/payment-service -n demo-pdb --timeout=120s
```

### 2. Verify PDB state

```bash
kubectl get pdb -n demo-pdb
# ALLOWED DISRUPTIONS = 0 (minAvailable=2 with 2 replicas)
```

### 3. Trigger a rolling update (will be blocked)

```bash
bash deploy/demo/scenarios/pdb-deadlock/inject-rolling-update.sh
```

### 4. Observe the deadlock

```bash
kubectl rollout status deployment/payment-service -n demo-pdb
# Will hang -- new ReplicaSet created but old pods cannot be evicted
```

### 5. Wait for alert and pipeline

The `KubePodDisruptionBudgetAtLimit` alert fires after 3 minutes at 0 allowed disruptions.

### 6. Verify remediation

```bash
kubectl get pdb -n demo-pdb
# minAvailable should now be 1, ALLOWED DISRUPTIONS > 0
kubectl rollout status deployment/payment-service -n demo-pdb
# Rolling update should complete
```

## Cleanup

```bash
kubectl delete namespace demo-pdb
```

## BDD Specification

```gherkin
Given a Kind cluster with Kubernaut services and a real LLM backend
  And the "relax-pdb-v1" workflow is registered with detectedLabels: pdbProtected: "true"
  And the "payment-service" deployment has 2 replicas
  And a PDB with minAvailable=2 is applied (blocking all disruptions)

When a rolling update is triggered on payment-service
  And the rolling update stalls because the PDB blocks pod eviction
  And the KubePodDisruptionBudgetAtLimit alert fires (0 allowed disruptions for 3 min)

Then Kubernaut detects the pdbProtected label
  And the LLM receives PDB context in its analysis prompt
  And the LLM selects the RelaxPDB workflow
  And WE patches the PDB minAvailable from 2 to 1
  And the blocked rolling update resumes and completes
  And EM verifies all pods are healthy after the update
```

## Acceptance Criteria

- [ ] PDB blocks rolling update (ALLOWED DISRUPTIONS = 0)
- [ ] Alert fires after 3 minutes of deadlock
- [ ] LLM leverages `pdbProtected` detected label in diagnosis
- [ ] RelaxPDB workflow is selected
- [ ] PDB is patched to minAvailable=1
- [ ] Rolling update completes after PDB relaxation
- [ ] EM confirms all pods healthy
