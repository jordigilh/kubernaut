# Scenario #127: Node NotReady -- Cordon + Drain

## Overview

A worker node becomes NotReady (simulated by pausing the Kind container with Podman).
Kubernaut detects the node failure, cordons it to prevent new scheduling, and drains
existing workloads to healthy nodes.

**Signal**: `KubernautNodeNotReady` -- node in NotReady state for >1 min
**Fault injection**: `podman pause <worker-node>` (stops kubelet heartbeat)
**Remediation**: `kubectl cordon` + `kubectl drain`

## Prerequisites

| Component | Requirement |
|-----------|-------------|
| Kind cluster | Multi-node with `kubernaut.ai/workload-node=true` label |
| Kubernaut services | Gateway, SP, AA, RO, WE, EM deployed |
| LLM backend | Real LLM (not mock) via HAPI |
| Prometheus | With kube-state-metrics |
| Podman | Required to pause/unpause Kind node container |
| Workflow catalog | `cordon-drain-v1` registered in DataStorage |

## Automated Run

```bash
./deploy/demo/scenarios/node-notready/run.sh
```

## Cleanup

```bash
WORKER=$(kubectl get nodes -l kubernaut.ai/workload-node=true -o name | head -1 | sed 's|node/||')
podman unpause $WORKER
kubectl uncordon $WORKER
kubectl delete namespace demo-node
```

## Acceptance Criteria

- [ ] Worker node transitions to NotReady after `podman pause`
- [ ] Alert fires within 1-2 minutes
- [ ] LLM identifies node failure (not a network or pod issue)
- [ ] CordonDrainNode workflow is selected
- [ ] Node is cordoned (unschedulable)
- [ ] Node is drained (non-system pods evicted)
- [ ] Pods rescheduled to healthy nodes
- [ ] EM confirms all pods healthy on new nodes
