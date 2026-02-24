# Scenario #122: No Action Required -- Disk Pressure Without Matching Workflow

## Overview

Orphaned PVCs from completed batch jobs trigger a disk-pressure alert, but **no matching
workflow is seeded** in the DataStorage catalog. The LLM evaluates the alert, determines
no automated remediation is available, and sets the AIAnalysis outcome to `WorkflowNotNeeded`.
The RO then marks the RR as `NoActionRequired`.

This scenario demonstrates Kubernaut's ability to gracefully handle situations where
automated remediation is not appropriate.

**Signal**: `KubePersistentVolumeClaimOrphaned` -- >3 bound PVCs in namespace for >2 min
**Outcome**: `NoActionRequired` (no workflow seeded, LLM concludes no action)

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | `deploy/demo/overlays/kind/kind-cluster-config.yaml` |
| Kubernaut services | Gateway, SP, AA, RO deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With kube-state-metrics |
| StorageClass | `standard` (default in Kind) |
| Workflow catalog | **Empty** -- no workflow seeded intentionally |

## Automated Run

```bash
./deploy/demo/scenarios/disk-pressure/run.sh
```

## Cleanup

```bash
./deploy/demo/scenarios/disk-pressure/cleanup.sh
```

## BDD Specification

```gherkin
Given a Kind cluster with Kubernaut services and a real LLM backend
  And NO cleanup workflow is registered in DataStorage
  And the "data-processor" deployment is running in namespace "demo-disk"

When 5 orphaned PVCs (from simulated completed batch jobs) are created
  And the KubePersistentVolumeClaimOrphaned alert fires (>3 bound PVCs for 2 min)

Then the alert flows through Gateway -> SP -> AA (HAPI)
  And the LLM finds no matching workflow in the catalog
  And AA sets outcome to WorkflowNotNeeded
  And RO marks the RR as Completed with outcome NoActionRequired
  And no WorkflowExecution CRD is created
```

## Acceptance Criteria

- [ ] 5 orphaned PVCs are created successfully
- [ ] Alert fires after 2 minutes
- [ ] AIAnalysis completes with outcome WorkflowNotNeeded
- [ ] RR reaches Completed phase with outcome NoActionRequired
- [ ] No WorkflowExecution CRD is created (0 WE resources)
- [ ] No notification is generated (pipeline completed without action)
