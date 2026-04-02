-- Migration 002: Add service_account_name column to remediation_workflow_catalog
-- DD-WE-005 v2.0: Per-Workflow ServiceAccount Reference (#481)
-- NULL = no per-workflow SA (K8s uses namespace default SA)

-- +goose Up
ALTER TABLE remediation_workflow_catalog ADD COLUMN IF NOT EXISTS service_account_name TEXT;

-- +goose Down
ALTER TABLE remediation_workflow_catalog DROP COLUMN IF EXISTS service_account_name;
