# Scenario #124: PDB Deadlock

## Overview

Demonstrates Kubernaut leveraging **detected labels** (`pdbProtected`) to resolve a
PodDisruptionBudget deadlock during a node drain. The PDB has `minAvailable` equal to
the replica count, leaving zero allowed disruptions and blocking all voluntary evictions.
When an SRE initiates `kubectl drain` for maintenance, the drain hangs indefinitely.
Kubernaut detects the deadlock, relaxes the PDB, and the drain completes.

**Detected label**: `pdbProtected: "true"` -- LLM context includes PDB configuration
**Signal**: `KubePodDisruptionBudgetAtLimit` -- PDB at 0 allowed disruptions for >3 min
**Remediation**: Patch PDB `minAvailable` from 2 to 1, unblocking the drain

## Why This Scenario Matters

This reproduces a well-known Kubernetes anti-pattern where an overly conservative PDB
silently blocks node maintenance operations:

1. **SRE team sets the PDB**: After a production incident where a node drain took down
   all pods simultaneously, the SRE team adds `minAvailable: 2` to guarantee the
   payment service is always available during voluntary disruptions. This is a sensible
   availability protection.

2. **Maintenance window arrives**: Weeks later, a kernel CVE requires patching all worker
   nodes. The SRE runs `kubectl drain <worker-node>` to move workloads off the node
   before rebooting. The drain command uses the Kubernetes eviction API, which respects
   PDB constraints.

3. **The deadlock**: The eviction API asks the PDB "can I evict a pod?" The PDB says no
   -- `minAvailable: 2` with exactly 2 running pods means zero disruptions are allowed.
   The drain hangs silently. The SRE sees the node stuck in `SchedulingDisabled` state
   with no progress. Meanwhile, the security patching window is ticking.

This is insidious because the PDB setting was correct when applied (2 replicas, want
both available), but it creates a hard constraint that blocks ALL voluntary disruptions
-- including routine maintenance. The failure mode is a silent hang rather than a
visible error, and it only surfaces during maintenance operations that may happen weeks
or months after the PDB was created.

### Future Expansion: Rolling Update Variant

An alternative single-node variant exists where `maxSurge: 0` combined with the same
PDB creates a rolling update deadlock. Kubernetes must evict an old pod before creating
a new one (`maxSurge: 0`), but the PDB blocks eviction. This variant is planned for
single-node validation as a future expansion.

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | Multi-node with worker (`kind-config-multinode.yaml`) |
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

### 2. Verify PDB state and pod placement

```bash
kubectl get pdb -n demo-pdb
# ALLOWED DISRUPTIONS = 0 (minAvailable=2 with 2 replicas)
kubectl get pods -n demo-pdb -o wide
# Both pods should be on the worker node
```

### 3. Drain the worker node (will be blocked by PDB)

```bash
bash deploy/demo/scenarios/pdb-deadlock/inject-drain.sh
```

### 4. Observe the deadlock

```bash
kubectl get nodes
# Worker node shows SchedulingDisabled, but drain is stuck
kubectl get pods -n demo-pdb
# All pods still Running on the worker (PDB blocks eviction)
```

### 5. Wait for alert and pipeline

The `KubePodDisruptionBudgetAtLimit` alert fires after 3 minutes at 0 allowed disruptions.

### 6. Verify remediation

```bash
kubectl get pdb -n demo-pdb
# minAvailable should now be 1, ALLOWED DISRUPTIONS > 0
kubectl get nodes
# Worker node drain should complete (SchedulingDisabled)
kubectl get pods -n demo-pdb -o wide
# Pods should be Running on the control-plane node
```

## Cleanup

```bash
bash deploy/demo/scenarios/pdb-deadlock/cleanup.sh
```

## BDD Specification

```gherkin
Given a Kind cluster with a worker node and Kubernaut services with a real LLM backend
  And the "relax-pdb-v1" workflow is registered with detectedLabels: pdbProtected: "true"
  And the "payment-service" deployment has 2 replicas on the worker node
  And a PDB with minAvailable=2 is applied (blocking all disruptions)

When an SRE drains the worker node for maintenance
  And the drain hangs because the PDB blocks all pod evictions
  And the KubePodDisruptionBudgetAtLimit alert fires (0 allowed disruptions for 3 min)

Then Kubernaut detects the pdbProtected label
  And the LLM receives PDB context in its analysis prompt
  And the LLM selects the RelaxPDB workflow
  And WE patches the PDB minAvailable from 2 to 1
  And the blocked drain resumes and completes
  And pods reschedule to the control-plane node
  And EM verifies all pods are healthy after the drain
```

## Acceptance Criteria

- [ ] Pods are scheduled on the worker node (nodeSelector)
- [ ] PDB blocks node drain (ALLOWED DISRUPTIONS = 0)
- [ ] Worker node stuck in SchedulingDisabled state
- [ ] Alert fires after 3 minutes of deadlock
- [ ] LLM leverages `pdbProtected` detected label in diagnosis
- [ ] RelaxPDB workflow is selected
- [ ] PDB is patched to minAvailable=1
- [ ] Node drain completes after PDB relaxation
- [ ] Pods reschedule to control-plane node
- [ ] EM confirms all pods healthy
