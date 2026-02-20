-- +goose Up
-- +goose StatementBegin
-- Migration: Add action types for demo remediation scenarios
-- Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
-- Purpose: Register new action types needed by demo scenario workflows (#114, #119-#130)

INSERT INTO action_type_taxonomy (action_type, description) VALUES
    ('GitRevertCommit', '{"what": "Revert a bad commit in a Git repository managed by GitOps (ArgoCD/Flux).", "when_to_use": "Root cause is a recent Git commit that introduced a regression in a GitOps-managed environment, and the ArgoCD/Flux controller will reconcile the reverted state.", "when_not_to_use": "When the environment is not GitOps-managed. Use RollbackDeployment instead for direct kubectl-managed workloads.", "preconditions": "GitOps tooling (ArgoCD or Flux) is active, the source Git repository is accessible, and a previous healthy commit exists."}'),
    ('ProvisionNode', '{"what": "Request provisioning of a new Kubernetes node to increase cluster capacity.", "when_to_use": "Pods are Pending due to insufficient cluster-wide resources (CPU, memory) and no existing node has capacity.", "when_not_to_use": "When scheduling failures are caused by taints, affinity rules, or PDB constraints rather than resource exhaustion.", "preconditions": "Confirmed that all schedulable nodes are at or near capacity and pending pods have resource requests that cannot be satisfied."}'),
    ('GracefulRestart', '{"what": "Perform a graceful rolling restart of a workload to reset its runtime state.", "when_to_use": "Predictive analysis indicates an impending failure (memory leak, resource exhaustion) that can be prevented by proactively restarting before the crash occurs.", "when_not_to_use": "When the issue is caused by a code bug or misconfiguration that will recur after restart.", "preconditions": "Evidence of gradual resource degradation (trending metrics) and the workload supports rolling restarts without data loss."}'),
    ('CleanupPVC', '{"what": "Remove old or unnecessary files from a PersistentVolumeClaim to reclaim disk space.", "when_to_use": "PVC usage has exceeded a critical threshold due to accumulated logs, temp files, or cache data.", "when_not_to_use": "When disk usage is from essential application data. Deleting it would cause data loss.", "preconditions": "PVC is mounted and accessible, and the files to be cleaned are identified as non-essential (logs, temp, cache)."}'),
    ('RemoveTaint', '{"what": "Remove a taint from a Kubernetes node to allow pod scheduling.", "when_to_use": "Pods are Pending because the target node has a taint that prevents scheduling, and the taint is no longer necessary.", "when_not_to_use": "When the taint was applied intentionally for maintenance or isolation purposes.", "preconditions": "The taint condition has been resolved and the node is healthy for general workload scheduling."}'),
    ('PatchHPA', '{"what": "Patch a HorizontalPodAutoscaler to increase maxReplicas or adjust scaling thresholds.", "when_to_use": "HPA has scaled to maxReplicas but the workload still cannot handle the load, indicating the max ceiling is too low.", "when_not_to_use": "When the scaling issue is caused by a code-level performance regression. Fix the code instead of adding more replicas.", "preconditions": "HPA is at maxReplicas, CPU/memory utilization remains above target, and the cluster has capacity for additional replicas."}'),
    ('RelaxPDB', '{"what": "Temporarily relax a PodDisruptionBudget to unblock a pending node drain, then restore it.", "when_to_use": "A node drain is blocked because a PDB prevents evicting the required number of pods.", "when_not_to_use": "When the PDB is correctly preventing unsafe evictions and the drain should be postponed.", "preconditions": "Confirmed that the drain is intentional and the PDB is the sole blocker. The workload can tolerate temporary reduced availability."}'),
    ('ProactiveRollback', '{"what": "Proactively roll back a deployment based on predictive SLO burn rate analysis.", "when_to_use": "Predictive analysis shows the current error rate will exhaust the SLO error budget before the next maintenance window.", "when_not_to_use": "When the error rate is within normal bounds or the SLO budget has sufficient headroom.", "preconditions": "A previous stable deployment revision exists, and the error spike correlates with a recent deployment change."}'),
    ('CordonDrainNode', '{"what": "Cordon a node to prevent new scheduling, then drain existing pods to other nodes.", "when_to_use": "A node is in NotReady state or showing signs of hardware/OS-level failure, and pods need to be relocated.", "when_not_to_use": "When only a single pod is affected. Use a pod-targeted action instead.", "preconditions": "Other healthy nodes have capacity to absorb the drained pods, and the node issue is confirmed as node-scoped."}')
ON CONFLICT (action_type) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM action_type_taxonomy WHERE action_type IN (
    'GitRevertCommit',
    'ProvisionNode',
    'GracefulRestart',
    'CleanupPVC',
    'RemoveTaint',
    'PatchHPA',
    'RelaxPDB',
    'ProactiveRollback',
    'CordonDrainNode'
);
-- +goose StatementEnd
