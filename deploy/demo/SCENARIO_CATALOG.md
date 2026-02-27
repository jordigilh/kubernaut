# Kubernaut Demo Scenario Catalog

## Tier 1 — Core Value

| # | Scenario | Description |
|---|----------|-------------|
| P1 | **crashloop** | CrashLoopBackOff from bad ConfigMap → rolls back to previous working revision |
| P2 | **stuck-rollout** | Rollout stuck on non-existent image tag → rolls back to previous revision |
| P3 | **pending-taint** | Pods Pending due to node taint → removes the taint so scheduling resumes |
| P4 | **node-notready** | NotReady node detected → cordons and drains workloads to healthy nodes |

## Tier 2 — Differentiated

| # | Scenario | Description |
|---|----------|-------------|
| P5 | **memory-leak** | Predicts memory exhaustion before OOM → rolling restart avoids the crash |
| P6 | **slo-burn** | SLO error-budget burn detected → proactive rollback to preserve the SLO |
| P7 | **hpa-maxed** | HPA at maxReplicas during traffic spike → patches maxReplicas ceiling |
| P8 | **pdb-deadlock** | PDB blocks voluntary evictions during drain → relaxes PDB to unblock |
| P9 | **autoscale** | No schedulable nodes → provisions a new worker node via kubeadm join |

## Tier 3 — Advanced Integration

| # | Scenario | Description |
|---|----------|-------------|
| P10 | **gitops-drift** | Broken ConfigMap in GitOps-managed env → `git revert` instead of kubectl rollback |
| P11 | **crashloop-helm** | CrashLoopBackOff in a Helm-managed workload → `helm rollback` |
| P12 | **cert-failure** | cert-manager Certificate stuck NotReady (CA Secret deleted) → recreates CA Secret |
| P13 | **orphaned-pvc-no-action** | Orphaned PVCs trigger disk-pressure alert → LLM correctly dismisses as benign (NoActionRequired) |
| P14 | **statefulset-pvc-failure** | StatefulSet PVC disruption causes Pending pods → recreates PVC and deletes stuck pod |

## Tier 4 — Niche/Combo

| # | Scenario | Description |
|---|----------|-------------|
| P15 | **network-policy-block** | Deny-all NetworkPolicy blocks ingress → detects and removes the offending policy |
| P16 | **mesh-routing-failure** | Linkerd AuthorizationPolicy blocks traffic → removes/fixes the policy |
| P17 | **cert-failure-gitops** | Same cert-manager failure as P12 but remediated via `git revert` in GitOps |

## Additional Scenarios

| # | Scenario | Description |
|---|----------|-------------|
| — | **remediation-retry** | First remediation fails → LLM-driven escalation selects a different workflow |
| — | **memory-escalation** | Repeated OOM remediations without resolution → escalates to human review |
| — | **concurrent-cross-namespace** | Same alert in two namespaces with different risk tolerances → different workflows |
| — | **duplicate-alert-suppression** | Multiple pod alerts deduplicated via OwnerResolver into a single RR |
| — | **resource-quota-exhaustion** | Pods Pending due to ResourceQuota → escalates to human review (no auto-fix) |
