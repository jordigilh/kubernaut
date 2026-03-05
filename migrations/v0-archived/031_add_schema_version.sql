-- +goose Up
-- +goose StatementBegin

-- ========================================
-- MIGRATION 031: Add schema_version to Workflow Catalog (#255)
-- ========================================
-- Authority: BR-WORKFLOW-004 v1.1 (Workflow Schema Format Specification)
-- Enables: DD-WE-005 (Workflow-Scoped RBAC in v1.1 via schemaVersion distinction)
-- ========================================
--
-- Purpose: Add schema_version column to track the structural generation
-- of the workflow-schema.yaml format. Allows the platform to distinguish
-- v1.0 schemas (current) from v1.1 schemas (with rbac stanza).
--
-- Default: '1.0' for all existing rows (pre-release, all workflows are v1.0)
-- ========================================

ALTER TABLE remediation_workflow_catalog
ADD COLUMN schema_version VARCHAR(10) NOT NULL DEFAULT '1.0';

COMMENT ON COLUMN remediation_workflow_catalog.schema_version IS 'Schema format version (e.g., "1.0", "1.1"). Determines which structural fields are valid. BR-WORKFLOW-004 v1.1, #255';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS schema_version;

-- +goose StatementEnd
