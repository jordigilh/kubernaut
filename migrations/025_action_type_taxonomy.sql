-- +goose Up
-- +goose StatementBegin
-- Migration: Create action_type_taxonomy table and add action_type FK to workflow catalog
-- Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
-- Purpose: Enable three-step discovery protocol (list actions -> list workflows -> get workflow)
-- BR-HAPI-017-001: Three-step tool implementation

-- 1. Create the action_type_taxonomy table
CREATE TABLE IF NOT EXISTS action_type_taxonomy (
    action_type TEXT PRIMARY KEY,
    description JSONB NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE action_type_taxonomy IS 'Curated taxonomy of remediation action types (DD-WORKFLOW-016)';
COMMENT ON COLUMN action_type_taxonomy.action_type IS 'Action type identifier (e.g., ScaleReplicas, RestartPod)';
COMMENT ON COLUMN action_type_taxonomy.description IS 'JSONB with fields: what, when_to_use, when_not_to_use, preconditions';

-- 2. Seed initial taxonomy values (DD-WORKFLOW-016 V1.0 - 10 action types)
INSERT INTO action_type_taxonomy (action_type, description) VALUES
    ('ScaleReplicas', '{"what": "Horizontally scale a workload by adjusting the replica count.", "when_to_use": "Root cause is insufficient capacity to handle current load and the workload supports horizontal scaling.", "preconditions": "Evidence of increased incoming traffic or load correlating with the resource exhaustion."}'),
    ('RestartPod', '{"what": "Kill and recreate one or more pods.", "when_to_use": "Root cause is a transient runtime state issue (corrupted cache, leaked connections, stuck threads) that a fresh process would resolve.", "preconditions": "Evidence that the issue is transient (e.g., pod was healthy before, no recent code deployment)."}'),
    ('IncreaseCPULimits', '{"what": "Increase CPU resource limits on containers.", "when_to_use": "CPU throttling is caused by resource limits being too low relative to the workload actual requirements, not by a code-level issue.", "preconditions": "Container is actively CPU-throttled (not just using high CPU), and CPU usage pattern is consistent with legitimate workload."}'),
    ('IncreaseMemoryLimits', '{"what": "Increase memory resource limits on containers.", "when_to_use": "OOM kills are caused by memory limits being too low relative to the workload actual requirements.", "preconditions": "Memory usage shows a stable pattern consistent with legitimate workload, not unbounded growth over time."}'),
    ('RollbackDeployment', '{"what": "Revert a deployment to its previous stable revision.", "when_to_use": "Root cause is a recent deployment that introduced a regression, and the previous revision was healthy.", "preconditions": "A previous healthy revision exists (verify via rollout history) and the issue started after the most recent deployment."}'),
    ('DrainNode', '{"what": "Drain and cordon a Kubernetes node, evicting all pods and preventing new scheduling.", "when_to_use": "Root cause is a node-level issue (hardware degradation, kernel problems, disk pressure) affecting multiple workloads on the node, and pods must be moved to healthy nodes.", "when_not_to_use": "Only a single pod is affected on the node. This indicates a pod-level issue, not node-level -- use a pod-targeted action instead. If pods don''t need to be evicted yet, use CordonNode instead.", "preconditions": "Confirmed that multiple workloads on the same node are affected, indicating node-scoped impact."}'),
    ('CordonNode', '{"what": "Cordon a Kubernetes node to prevent new pod scheduling without evicting existing pods.", "when_to_use": "Root cause is an emerging node-level issue that warrants preventing new pods from being scheduled, but existing pods are still running and do not need immediate eviction.", "when_not_to_use": "If existing pods on the node are already failing or need to be moved to healthy nodes, use DrainNode instead.", "preconditions": "Evidence of degrading node health (intermittent errors, rising resource pressure) but existing workloads still functional."}'),
    ('RestartDeployment', '{"what": "Perform a rolling restart of all pods in a workload (Deployment or StatefulSet).", "when_to_use": "Root cause is a workload-wide state issue affecting all or most pods, such as stale configuration, expired certificates, or corrupted shared state that requires all pods to be refreshed.", "preconditions": "Evidence that the issue affects multiple pods in the same workload (not just a single pod), and a fresh set of pods would resolve the issue."}'),
    ('CleanupNode', '{"what": "Reclaim disk space on a node by purging temporary files, old logs, and unused container images.", "when_to_use": "Node disk pressure is caused by accumulated ephemeral data (temp files, old container logs, unused images), not by legitimate workload storage growth.", "when_not_to_use": "If disk usage is from legitimate workload data (persistent volumes, application databases). Cleanup would not help and could cause data loss. Use DrainNode instead if the node needs to be decommissioned.", "preconditions": "Evidence that disk usage is dominated by ephemeral/reclaimable data (container image cache, log files, tmp directories), not persistent workload data."}'),
    ('DeletePod', '{"what": "Delete one or more specific pods without waiting for graceful termination.", "when_to_use": "Pods are stuck in a terminal state (Terminating, Unknown) and cannot be restarted through normal means.", "when_not_to_use": "Do not use as a general restart mechanism. Use RestartPod instead for transient runtime issues.", "preconditions": "Pod is genuinely stuck and not responding to graceful termination (verify via pod events and state duration)."}')
ON CONFLICT (action_type) DO NOTHING;

-- 3. Add action_type column to remediation_workflow_catalog
-- Use 'ScaleReplicas' as default for existing rows (pre-release data only)
ALTER TABLE remediation_workflow_catalog
    ADD COLUMN IF NOT EXISTS action_type TEXT;

-- 4. Backfill existing rows with a default action type
UPDATE remediation_workflow_catalog
SET action_type = 'ScaleReplicas'
WHERE action_type IS NULL;

-- 5. Add NOT NULL constraint and FK after backfill
ALTER TABLE remediation_workflow_catalog
    ALTER COLUMN action_type SET NOT NULL;

ALTER TABLE remediation_workflow_catalog
    ADD CONSTRAINT fk_workflow_action_type
    FOREIGN KEY (action_type)
    REFERENCES action_type_taxonomy(action_type);

-- 6. Add composite index for discovery queries (ListActions, ListWorkflowsByActionType)
-- GAP-2: Replaces single-column idx_workflow_action_type with composite
-- covering the three-column filter pattern: action_type + status + is_latest_version
CREATE INDEX IF NOT EXISTS idx_workflow_action_type_status_version
    ON remediation_workflow_catalog(action_type, status, is_latest_version);

-- 7. Add trigger for updated_at on taxonomy table
DROP TRIGGER IF EXISTS trigger_action_type_taxonomy_updated_at ON action_type_taxonomy;
CREATE TRIGGER trigger_action_type_taxonomy_updated_at
    BEFORE UPDATE ON action_type_taxonomy
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trigger_action_type_taxonomy_updated_at ON action_type_taxonomy;
ALTER TABLE remediation_workflow_catalog DROP CONSTRAINT IF EXISTS fk_workflow_action_type;
DROP INDEX IF EXISTS idx_workflow_action_type_status_version;
ALTER TABLE remediation_workflow_catalog DROP COLUMN IF EXISTS action_type;
DROP TABLE IF EXISTS action_type_taxonomy;
-- +goose StatementEnd
