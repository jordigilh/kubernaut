# Scenario #121: Disk Pressure -- Orphaned PVC Cleanup

## Overview

Orphaned PVCs from completed batch jobs accumulate and consume storage. Kubernaut detects
the growing PVC count and cleans up PVCs not mounted by any running pod.

**Signal**: `KubernautOrphanedPVCs` -- >3 bound PVCs in namespace for >2 min
**Remediation**: Delete PVCs labeled `batch-run=completed` that are not mounted

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/overlays/kind/kind-cluster-config.yaml` |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With kube-state-metrics |
| StorageClass | `standard` (default in Kind) |
| Workflow catalog | `cleanup-pvc-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/disk-pressure/run.sh
```

## Cleanup

```bash
kubectl delete namespace demo-disk
```

## BDD Specification

```gherkin
Given a Kind cluster with Kubernaut services and a real LLM backend
  And the "cleanup-pvc-v1" workflow is registered in DataStorage
  And the "data-processor" deployment is running in namespace "demo-disk"

When 5 orphaned PVCs (from simulated completed batch jobs) are created
  And the KubernautOrphanedPVCs alert fires (>3 bound PVCs for 2 min)

Then the LLM identifies the PVCs as orphaned (not mounted by any running pod)
  And selects the CleanupPVC workflow
  And WE deletes PVCs matching label batch-run=completed that are not mounted
  And EM verifies PVC count is reduced
```

## Acceptance Criteria

- [ ] 5 orphaned PVCs are created successfully
- [ ] Alert fires after 2 minutes
- [ ] LLM identifies orphaned PVCs (not code bugs or data needs)
- [ ] CleanupPVC workflow is selected
- [ ] Only unmounted PVCs are deleted (safety check)
- [ ] PVC count drops after cleanup
- [ ] EM confirms successful remediation
