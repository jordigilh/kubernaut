# Scenario #126: Cluster Autoscaling -- Add Node via kubeadm join

## Overview

This scenario demonstrates Kubernaut diagnosing pod scheduling failures caused by resource exhaustion across all cluster nodes, and remediating by provisioning a new Kind worker node via `kubeadm join`.

The architecture uses **split responsibility** (Option B), mirroring how cloud autoscalers work in production:

```
Production:
  WE Job → calls AWS/GCP/Azure API → cloud provisions VM → VM runs kubeadm join

Kind Demo (equivalent):
  WE Job → writes ScaleRequest ConfigMap → host-side provisioner detects it
                                            → podman run (create container)
                                            → podman exec (kubeadm join)
                                            → kubectl label (workload node)
  WE Job → waits for new Node Ready → verifies pods rescheduled
```

The WE Job runs **unprivileged inside Kubernetes**. It writes a scale request and waits. The host-side provisioner agent is the Kind-specific equivalent of Karpenter (EKS), NAP (GKE), or `cluster-autoscaler`.

## Prerequisites

- Kind cluster created with `deploy/demo/overlays/kind/kind-cluster-config.yaml` (multi-node: control-plane + 1 worker)
- Podman available on the host (used by the provisioner agent)
- Kubernaut services deployed with HAPI configured for a real LLM backend
- `ProvisionNode` action type registered in DataStorage (migration 026)
- `provision-node-v1` workflow registered in the workflow catalog

## BDD Specification

```gherkin
Feature: Cluster Autoscaling via Node Provisioning

  Scenario: Pods stuck Pending due to resource exhaustion trigger node provisioning
    Given a Kind cluster with 1 control-plane and 1 worker node
      And an nginx Deployment "web-cluster" with 2 replicas running on the worker node
      And each replica requests 512Mi memory
      And the Kubernaut pipeline is active with a real LLM
      And the "provision-node-v1" workflow is registered in the catalog
      And the host-side provisioner agent is running

    When the Deployment is scaled to 8 replicas
      And the worker node cannot satisfy 8 x 512Mi = 4GB memory requests
      And new pods enter Pending state with "Insufficient memory" events

    Then Prometheus fires KubePodSchedulingFailed alert after 2 minutes
      And Signal Processing enriches with node resource allocation data
      And the LLM identifies cluster-wide resource exhaustion as root cause
      And the LLM selects the ProvisionNode action type
      And Remediation Orchestrator creates a WorkflowExecution
      And the WE Job creates a ScaleRequest ConfigMap in kubernaut-system
      And the provisioner agent detects the request
      And the provisioner creates a new Kind node via podman + kubeadm join
      And the provisioner labels the new node kubernaut.ai/workload-node=true
      And the WE Job verifies the new node is Ready
      And previously-Pending pods schedule on the new node
      And EffectivenessAssessment confirms all pods are Running
```

## Automated Execution

```bash
./deploy/demo/scenarios/autoscale/run.sh
```

This script:
1. Deploys the namespace, workload, and Prometheus rules
2. Starts the provisioner agent in the background
3. Injects the failure (scales to 8 replicas)
4. Prints monitoring instructions

## Manual Step-by-Step

```bash
# 1. Verify cluster has control-plane + 1 worker
kubectl get nodes

# 2. Deploy namespace and workload (2 replicas)
kubectl apply -f deploy/demo/scenarios/autoscale/manifests/
kubectl wait --for=condition=Available deployment/web-cluster \
  -n demo-autoscale --timeout=60s

# 3. Deploy Prometheus alerting rules
kubectl apply -f deploy/demo/scenarios/autoscale/manifests/prometheus-rule.yaml

# 4. Start the provisioner agent in background
./deploy/demo/scenarios/autoscale/provisioner.sh &
PROVISIONER_PID=$!

# 5. Verify initial state (2 Running pods)
kubectl get pods -n demo-autoscale -o wide

# 6. Inject: scale beyond node capacity
kubectl scale deployment/web-cluster --replicas=8 -n demo-autoscale

# 7. Verify some pods are Pending
kubectl get pods -n demo-autoscale   # 2-3 Running, 5-6 Pending

# 8. Watch Kubernaut pipeline
kubectl get rr,sp,aa,we,ea -n demo-autoscale -w

# 9. Verify: new node appeared, all pods Running
kubectl get nodes                          # 3rd node visible
kubectl get pods -n demo-autoscale -o wide # distributed across nodes

# 10. Cleanup
kill $PROVISIONER_PID
./deploy/demo/scenarios/autoscale/cleanup.sh
```

## Acceptance Criteria

- [ ] nginx Deployment manifests in `deploy/demo/scenarios/autoscale/manifests/`
- [ ] WE Job workflow (remediate.sh) creates ScaleRequest and verifies fulfillment
- [ ] Host-side provisioner agent (provisioner.sh) watches and provisions nodes
- [ ] workflow-schema.yaml with actionType `ProvisionNode` registered in DataStorage
- [ ] Prometheus alerting rule for `FailedScheduling` / `Insufficient` resources
- [ ] Full pipeline with real LLM: Gateway -> RO -> SP -> AA -> WE -> EM
- [ ] LLM identifies resource exhaustion and selects node provisioning workflow
- [ ] New node joins via kubeadm, `kubectl get nodes` shows it as Ready
- [ ] Previously Pending pods schedule on the new node
- [ ] `run.sh` automates the entire flow (including starting provisioner agent)
- [ ] README documents both automated and manual step-by-step execution
- [ ] Cleanup instructions for removing the extra node and provisioner

## Memory Budget

| Component | Estimate |
|---|---|
| New Kind worker node | ~500MB |
| **Additional overhead** | **~500MB** |
| **Total cluster after scaling** | **~5.5-6.5GB** |
| **Headroom on 12GB** | **~5.5-6.5GB** |

## Cleanup

```bash
./deploy/demo/scenarios/autoscale/cleanup.sh
```

This removes the namespace, scale-request ConfigMap, kills the provisioner, and deletes dynamically provisioned node containers from Podman.

## Notes

- **Production analogy**: The provisioner agent is the Kind equivalent of Karpenter (EKS), NAP (GKE), or cluster-autoscaler. In production, the WE Job would call a cloud API directly instead of writing a ConfigMap.
- **Security**: The WE Job runs unprivileged inside K8s. Only the host-side agent (outside K8s) has Podman access.
- **EM target**: The `AffectedResource` should be the Deployment (pods now Running), not the cluster itself.
