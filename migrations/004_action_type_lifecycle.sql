-- Migration 004: ActionType lifecycle columns
-- BR-WORKFLOW-007: ActionType CRD lifecycle management
-- ADR-059: ActionType CRD lifecycle via Admission Webhook
--
-- Adds lifecycle management columns to action_type_taxonomy:
-- - status: 'active' (default) or 'disabled' for soft-delete
-- - disabled_at: timestamp when the action type was disabled
-- - disabled_by: identity of who disabled it (K8s SA or user)

ALTER TABLE action_type_taxonomy
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';

ALTER TABLE action_type_taxonomy
  ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE action_type_taxonomy
  ADD COLUMN IF NOT EXISTS disabled_by TEXT;

COMMENT ON COLUMN action_type_taxonomy.status IS 'Lifecycle status: active or disabled (BR-WORKFLOW-007)';
COMMENT ON COLUMN action_type_taxonomy.disabled_at IS 'Timestamp when action type was soft-disabled';
COMMENT ON COLUMN action_type_taxonomy.disabled_by IS 'Identity (K8s SA or user) who disabled the action type';
