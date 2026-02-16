-- +goose Up
-- +goose StatementBegin
-- Migration 026: BR-WORKFLOW-004 schema format alignment
-- Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
-- Authority: DD-WORKFLOW-017 (OCI-based Workflow Registration)
--
-- Changes:
-- 1. Convert description column from TEXT to JSONB (structured description)
-- 2. Update labels JSONB keys from snake_case to camelCase (signalType)
-- 3. Update action_type_taxonomy description keys to camelCase

-- 1. Convert description column from TEXT to JSONB
-- Step 1a: Add temporary JSONB column
ALTER TABLE remediation_workflow_catalog
    ADD COLUMN IF NOT EXISTS description_new JSONB;

-- Step 1b: Migrate existing TEXT description to structured JSONB format
-- Existing rows get their text description placed into the "what" field
UPDATE remediation_workflow_catalog
SET description_new = jsonb_build_object(
    'what', description,
    'whenToUse', ''
)
WHERE description_new IS NULL;

-- Step 1c: Drop old TEXT column and rename new column
ALTER TABLE remediation_workflow_catalog DROP COLUMN description;
ALTER TABLE remediation_workflow_catalog RENAME COLUMN description_new TO description;
ALTER TABLE remediation_workflow_catalog ALTER COLUMN description SET NOT NULL;
ALTER TABLE remediation_workflow_catalog ALTER COLUMN description SET DEFAULT '{}'::jsonb;

-- 2. Update labels JSONB: rename signal_type key to signalType
-- Pre-release: no backwards compatibility needed
UPDATE remediation_workflow_catalog
SET labels = (labels - 'signal_type') || jsonb_build_object('signalType', labels->>'signal_type')
WHERE labels ? 'signal_type';

-- 3. Update action_type_taxonomy description keys to camelCase
-- Convert when_to_use -> whenToUse, when_not_to_use -> whenNotToUse
UPDATE action_type_taxonomy
SET description = jsonb_build_object(
    'what', COALESCE(description->>'what', ''),
    'whenToUse', COALESCE(description->>'when_to_use', description->>'whenToUse', ''),
    'whenNotToUse', COALESCE(description->>'when_not_to_use', description->>'whenNotToUse', ''),
    'preconditions', COALESCE(description->>'preconditions', '')
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Reverse action_type_taxonomy description keys
UPDATE action_type_taxonomy
SET description = jsonb_build_object(
    'what', COALESCE(description->>'what', ''),
    'when_to_use', COALESCE(description->>'whenToUse', description->>'when_to_use', ''),
    'when_not_to_use', COALESCE(description->>'whenNotToUse', description->>'when_not_to_use', ''),
    'preconditions', COALESCE(description->>'preconditions', '')
);

-- Reverse labels JSONB: rename signalType back to signal_type
UPDATE remediation_workflow_catalog
SET labels = (labels - 'signalType') || jsonb_build_object('signal_type', labels->>'signalType')
WHERE labels ? 'signalType';

-- Convert description back from JSONB to TEXT
ALTER TABLE remediation_workflow_catalog
    ADD COLUMN description_old TEXT;

UPDATE remediation_workflow_catalog
SET description_old = COALESCE(description->>'what', '');

ALTER TABLE remediation_workflow_catalog DROP COLUMN description;
ALTER TABLE remediation_workflow_catalog RENAME COLUMN description_old TO description;
ALTER TABLE remediation_workflow_catalog ALTER COLUMN description SET NOT NULL;

-- +goose StatementEnd
