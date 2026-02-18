-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 028: Semantic split of container_image into schema_image + execution_bundle
-- ========================================
-- Authority: Issue #89 (Enforce digest-only references in execution.bundle)
-- Authority: DD-WORKFLOW-017 (OCI-based Workflow Registration)
-- ========================================
--
-- container_image/container_digest stored a single OCI reference with mixed semantics:
--   - Schema extraction: the image pulled at registration to extract /workflow-schema.yaml
--   - Execution: the Tekton bundle / Job image run at remediation time
--
-- This migration splits them into two distinct pairs:
--   schema_image / schema_digest       — OCI pullspec used for schema extraction
--   execution_bundle / execution_bundle_digest — OCI bundle used at execution time (digest-pinned)
--
-- Pre-release: existing data in container_image is treated as schema_image (the
-- registration pullspec). execution_bundle is populated as NULL until operators
-- re-register workflows with the new field.

-- Rename container_image → schema_image
ALTER TABLE remediation_workflow_catalog
RENAME COLUMN container_image TO schema_image;

-- Rename container_digest → schema_digest
ALTER TABLE remediation_workflow_catalog
RENAME COLUMN container_digest TO schema_digest;

-- Rename the digest index to match new column name
ALTER INDEX IF EXISTS idx_workflow_catalog_container_digest
RENAME TO idx_workflow_catalog_schema_digest;

-- Add execution_bundle column (digest-pinned OCI reference for runtime)
ALTER TABLE remediation_workflow_catalog
ADD COLUMN execution_bundle TEXT;

-- Add execution_bundle_digest column
ALTER TABLE remediation_workflow_catalog
ADD COLUMN execution_bundle_digest VARCHAR(71);

-- Create index for execution_bundle digest lookups
CREATE INDEX idx_workflow_catalog_execution_bundle_digest
    ON remediation_workflow_catalog(execution_bundle_digest)
    WHERE execution_bundle_digest IS NOT NULL;

COMMENT ON COLUMN remediation_workflow_catalog.schema_image IS 'Issue #89: OCI image pulled at registration to extract /workflow-schema.yaml (DD-WORKFLOW-017)';
COMMENT ON COLUMN remediation_workflow_catalog.schema_digest IS 'Issue #89: SHA256 digest of the schema image';
COMMENT ON COLUMN remediation_workflow_catalog.execution_bundle IS 'Issue #89: OCI execution bundle reference (digest-pinned) for Tekton/Job runtime';
COMMENT ON COLUMN remediation_workflow_catalog.execution_bundle_digest IS 'Issue #89: SHA256 digest portion of execution_bundle';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_workflow_catalog_execution_bundle_digest;

ALTER TABLE remediation_workflow_catalog DROP COLUMN IF EXISTS execution_bundle_digest;
ALTER TABLE remediation_workflow_catalog DROP COLUMN IF EXISTS execution_bundle;

ALTER INDEX IF EXISTS idx_workflow_catalog_schema_digest
RENAME TO idx_workflow_catalog_container_digest;

ALTER TABLE remediation_workflow_catalog
RENAME COLUMN schema_digest TO container_digest;

ALTER TABLE remediation_workflow_catalog
RENAME COLUMN schema_image TO container_image;

-- +goose StatementEnd
