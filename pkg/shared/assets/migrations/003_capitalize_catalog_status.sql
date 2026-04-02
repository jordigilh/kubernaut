-- Migration 003: Align catalog status values with PascalCase CRD convention (#483)
--
-- The CRD typed enums (sharedtypes.CatalogStatus) use PascalCase: Active, Disabled, etc.
-- This migration updates the database to match, eliminating the normalization layer
-- in the authwebhook DS adapter.
--
-- Also updates the CHECK constraint and DEFAULT on remediation_workflow_catalog,
-- and the DEFAULT on action_type_taxonomy.

-- +goose Up
-- +goose StatementBegin
UPDATE remediation_workflow_catalog
SET status = initcap(status)
WHERE status IN ('active', 'disabled', 'deprecated', 'archived', 'superseded');

UPDATE action_type_taxonomy
SET status = initcap(status)
WHERE status IN ('active', 'disabled', 'deprecated', 'archived');

ALTER TABLE remediation_workflow_catalog DROP CONSTRAINT IF EXISTS remediation_workflow_catalog_status_check;
ALTER TABLE remediation_workflow_catalog ADD CONSTRAINT remediation_workflow_catalog_status_check
    CHECK (status IN ('Active', 'Disabled', 'Deprecated', 'Archived', 'Superseded'));

ALTER TABLE remediation_workflow_catalog ALTER COLUMN status SET DEFAULT 'Active';
ALTER TABLE action_type_taxonomy ALTER COLUMN status SET DEFAULT 'Active';

DROP INDEX IF EXISTS uq_workflow_name_version_active;
CREATE UNIQUE INDEX uq_workflow_name_version_active ON remediation_workflow_catalog (workflow_name, version) WHERE status = 'Active';

DROP INDEX IF EXISTS idx_workflow_catalog_success_rate;
CREATE INDEX idx_workflow_catalog_success_rate ON remediation_workflow_catalog(actual_success_rate DESC) WHERE status = 'Active';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE remediation_workflow_catalog
SET status = lower(status)
WHERE status IN ('Active', 'Disabled', 'Deprecated', 'Archived', 'Superseded');

UPDATE action_type_taxonomy
SET status = lower(status)
WHERE status IN ('Active', 'Disabled', 'Deprecated', 'Archived');

ALTER TABLE remediation_workflow_catalog DROP CONSTRAINT IF EXISTS remediation_workflow_catalog_status_check;
ALTER TABLE remediation_workflow_catalog ADD CONSTRAINT remediation_workflow_catalog_status_check
    CHECK (status IN ('active', 'disabled', 'deprecated', 'archived', 'superseded'));

ALTER TABLE remediation_workflow_catalog ALTER COLUMN status SET DEFAULT 'active';
ALTER TABLE action_type_taxonomy ALTER COLUMN status SET DEFAULT 'active';

DROP INDEX IF EXISTS uq_workflow_name_version_active;
CREATE UNIQUE INDEX uq_workflow_name_version_active ON remediation_workflow_catalog (workflow_name, version) WHERE status = 'active';

DROP INDEX IF EXISTS idx_workflow_catalog_success_rate;
CREATE INDEX idx_workflow_catalog_success_rate ON remediation_workflow_catalog(actual_success_rate DESC) WHERE status = 'active';
-- +goose StatementEnd
