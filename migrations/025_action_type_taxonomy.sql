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

-- 2. Seed initial taxonomy values (from DD-WORKFLOW-016 examples)
INSERT INTO action_type_taxonomy (action_type, description) VALUES
    ('ScaleReplicas', '{"what": "Horizontally scale a workload by adjusting replica count", "when_to_use": "OOMKilled events, high memory/CPU pressure, pod evictions due to resource exhaustion", "when_not_to_use": "When the issue is a code bug or configuration error, not resource pressure", "preconditions": "HPA not managing the target workload, or HPA max not yet reached"}'),
    ('RestartPod', '{"what": "Delete and recreate a pod to recover from transient failures", "when_to_use": "CrashLoopBackOff with transient root cause, stuck init containers, stale connections", "when_not_to_use": "When the crash is caused by a persistent code bug or missing dependency", "preconditions": "Pod is managed by a controller (Deployment, StatefulSet, DaemonSet)"}'),
    ('RollbackDeployment', '{"what": "Roll back a deployment to a previous known-good revision", "when_to_use": "Post-deploy failures, new version introducing errors, configuration regression", "when_not_to_use": "When the previous version also had the same issue", "preconditions": "At least one previous revision exists in deployment history"}'),
    ('AdjustResources', '{"what": "Modify resource requests/limits for a workload", "when_to_use": "Consistent OOMKilled events, CPU throttling, resource quota violations", "when_not_to_use": "When the issue is a memory leak rather than undersized limits", "preconditions": "VPA not managing the target workload, or VPA recommendations available"}'),
    ('ReconfigureService', '{"what": "Update ConfigMap or Secret values to fix misconfiguration", "when_to_use": "Configuration-related failures (wrong endpoints, invalid credentials, missing keys)", "when_not_to_use": "When the configuration is correct but the downstream service is unavailable", "preconditions": "ConfigMap or Secret exists and is mounted by the target workload"}')
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

-- 6. Add index for action_type lookups
CREATE INDEX IF NOT EXISTS idx_workflow_action_type ON remediation_workflow_catalog(action_type);

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
DROP INDEX IF EXISTS idx_workflow_action_type;
ALTER TABLE remediation_workflow_catalog DROP COLUMN IF EXISTS action_type;
DROP TABLE IF EXISTS action_type_taxonomy;
-- +goose StatementEnd
