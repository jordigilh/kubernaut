-- Migration 004: Add expression index on post_remediation_spec_hash for dual-hash query
-- Issue #616: QueryROEventsBySpecHash expanded to match both pre and post hashes
-- Mirrors idx_audit_events_pre_remediation_spec_hash pattern from 001_v1_schema.sql

-- +goose Up
CREATE INDEX IF NOT EXISTS idx_audit_events_post_remediation_spec_hash
  ON audit_events ((event_data->>'post_remediation_spec_hash'), event_timestamp DESC)
  WHERE event_data->>'post_remediation_spec_hash' IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_audit_events_post_remediation_spec_hash;
