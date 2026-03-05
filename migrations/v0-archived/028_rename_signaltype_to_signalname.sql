-- +goose Up
-- +goose StatementBegin
-- Issue #166: Rename signalType -> signalName in workflow catalog JSONB labels
-- This renames the JSONB key used for semantic signal name matching in the workflow catalog.
UPDATE remediation_workflow_catalog
SET labels = (labels - 'signalType') || jsonb_build_object('signalName', labels->>'signalType')
WHERE labels ? 'signalType';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Revert: Rename signalName -> signalType in workflow catalog JSONB labels
UPDATE remediation_workflow_catalog
SET labels = (labels - 'signalName') || jsonb_build_object('signalType', labels->>'signalName')
WHERE labels ? 'signalName';
-- +goose StatementEnd
