-- +goose Up
-- +goose StatementBegin
-- Migration: Add status_reason column to remediation_workflow_catalog
-- Purpose: Support workflow status updates with reason tracking
-- BR-STORAGE-016: Workflow status management
-- Issue: Integration test failure - column "status_reason" does not exist

ALTER TABLE remediation_workflow_catalog
ADD COLUMN IF NOT EXISTS status_reason TEXT;

COMMENT ON COLUMN remediation_workflow_catalog.status_reason IS 'Reason for status change (e.g., why workflow was disabled, activated, etc.)';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE remediation_workflow_catalog
DROP COLUMN IF EXISTS status_reason;
-- +goose StatementEnd

