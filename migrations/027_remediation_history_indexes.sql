-- +goose Up
-- +goose StatementBegin
-- ========================================
-- DD-HAPI-016: Expression Indexes for Remediation History Context
-- Migration: Add expression indexes on JSONB fields for efficient
-- remediation history lookups.
-- BR: BR-HAPI-016 (Remediation history context for LLM prompt enrichment)
-- Date: February 2026
-- ========================================
--
-- RATIONALE:
-- The remediation history endpoint (GET /api/v1/remediation-history/context)
-- queries audit events by:
-- 1. target_resource (stored in event_data JSONB) for Tier 1 chain
-- 2. pre_remediation_spec_hash (stored in event_data JSONB) for Tier 2 regression
--
-- While a GIN index exists on event_data (idx_audit_events_event_data_gin),
-- expression (B-tree) indexes are more efficient for exact-match lookups on
-- specific JSONB keys.
-- ========================================

-- Index 1: target_resource expression index
-- Supports Tier 1 queries: find all remediation events for a specific target
-- Format: "{namespace}/{kind}/{name}" (e.g. "prod/Deployment/my-app")
CREATE INDEX IF NOT EXISTS idx_audit_events_target_resource
    ON audit_events ((event_data->>'target_resource'), event_timestamp DESC)
    WHERE event_type IN ('remediation.workflow_created', 'effectiveness.assessment.completed');

-- Index 2: pre_remediation_spec_hash expression index
-- Supports Tier 2 queries: find historical events matching the current spec hash
-- indicating configuration regression
CREATE INDEX IF NOT EXISTS idx_audit_events_pre_remediation_spec_hash
    ON audit_events ((event_data->>'pre_remediation_spec_hash'), event_timestamp DESC)
    WHERE event_data->>'pre_remediation_spec_hash' IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_audit_events_target_resource;
DROP INDEX IF EXISTS idx_audit_events_pre_remediation_spec_hash;

-- +goose StatementEnd
