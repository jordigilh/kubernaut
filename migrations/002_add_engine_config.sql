-- Add engine_config JSONB column for engine-specific workflow configuration (BR-WE-016)
--
-- +goose Up
-- +goose StatementBegin

ALTER TABLE remediation_workflow_catalog
    ADD COLUMN IF NOT EXISTS engine_config JSONB;

COMMENT ON COLUMN remediation_workflow_catalog.engine_config IS 'Engine-specific configuration as JSONB (BR-WE-016). For ansible: {"playbookPath": "...", "inventoryName": "...", "jobTemplateName": "..."}. For tekton/job: NULL.';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE remediation_workflow_catalog
    DROP COLUMN IF EXISTS engine_config;

-- +goose StatementEnd
