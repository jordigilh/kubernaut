-- +goose Up
-- BR-WORKFLOW-006: Workflow Content Integrity
-- Enables multiple records with the same (workflow_name, version) for audit trail.
-- Active constraint ensures only ONE active workflow per name+version.
-- Adds 'superseded' status for workflows replaced by a new content hash.

-- Step 1: Drop the old UNIQUE constraint that prevented multiple records per name+version
ALTER TABLE remediation_workflow_catalog
    DROP CONSTRAINT uq_workflow_name_version;

-- Step 2: Create partial unique index — only one ACTIVE workflow per name+version
CREATE UNIQUE INDEX uq_workflow_name_version_active
    ON remediation_workflow_catalog (workflow_name, version)
    WHERE status = 'active';

-- Step 3: Replace the status CHECK constraint to include 'superseded'
ALTER TABLE remediation_workflow_catalog
    DROP CONSTRAINT IF EXISTS remediation_workflow_catalog_status_check;

ALTER TABLE remediation_workflow_catalog
    ADD CONSTRAINT remediation_workflow_catalog_status_check
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived', 'superseded'));


-- +goose Down
-- Revert: restore the old UNIQUE constraint and remove 'superseded' status

DROP INDEX IF EXISTS uq_workflow_name_version_active;

ALTER TABLE remediation_workflow_catalog
    ADD CONSTRAINT uq_workflow_name_version UNIQUE (workflow_name, version);

ALTER TABLE remediation_workflow_catalog
    DROP CONSTRAINT IF EXISTS remediation_workflow_catalog_status_check;

ALTER TABLE remediation_workflow_catalog
    ADD CONSTRAINT remediation_workflow_catalog_status_check
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived'));
