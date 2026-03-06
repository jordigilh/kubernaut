-- +goose Up
-- +goose StatementBegin
-- ========================================
-- GAP #9: Event Hashing (Tamper-Evidence)
-- Migration: Add blockchain-style hash chain to audit_events
-- SOC2 Requirement: Tamper-evident audit logs (SOC 2 Type II, NIST 800-53, Sarbanes-Oxley)
-- Date: January 6, 2026
-- Authority: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md - Day 7
-- ========================================
--
-- ARCHITECTURE:
-- 1. Blockchain-style hash chain: event_hash = SHA256(previous_event_hash + event_json)
-- 2. Each event links to previous event in same correlation_id
-- 3. First event in chain has previous_event_hash = '' (empty string)
-- 4. Tampering with ANY event breaks the chain (detectable via verification API)
--
-- COMPLIANCE:
-- - SOC 2 Type II: Tamper-evident audit logs (Trust Services Criteria CC8.1)
-- - NIST 800-53: AU-9 (Protection of Audit Information)
-- - Sarbanes-Oxley: Section 404 (Internal Controls)
--
-- BACKWARDS COMPATIBILITY:
-- - Existing events: event_hash = NULL, previous_event_hash = NULL
-- - New events: Hash calculated on INSERT
-- - Chain starts from implementation date (2026-01-06)
-- - No backfill required (pragmatic approach for pre-release product)
--
-- ========================================

-- Step 0: Enable pgcrypto extension for digest() function
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Step 1: Add event_hash column (stores SHA256 hash of this event)
ALTER TABLE audit_events ADD COLUMN event_hash TEXT;

-- Step 2: Add previous_event_hash column (links to previous event in chain)
ALTER TABLE audit_events ADD COLUMN previous_event_hash TEXT;

-- Step 3: Add comment for documentation
COMMENT ON COLUMN audit_events.event_hash IS
'SHA256 hash of (previous_event_hash + event_json). Blockchain-style tamper detection per Gap #9.';

COMMENT ON COLUMN audit_events.previous_event_hash IS
'Hash of previous event in same correlation_id. Empty string for first event. Enables chain verification.';

-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- INDEXES FOR HASH CHAIN VERIFICATION
-- ========================================

-- Index 1: event_hash lookup (for chain verification)
-- Used by: Verification API to detect tampered events
-- Note: CONCURRENTLY removed for E2E test compatibility (transaction-based migrations)
--       E2E tests apply migrations in transactions for atomicity
--       CONCURRENTLY cannot run inside transaction blocks (PostgreSQL restriction)
--       E2E tests have empty databases, so no locking/downtime concerns
CREATE INDEX IF NOT EXISTS idx_audit_events_hash
    ON audit_events(event_hash)
    WHERE event_hash IS NOT NULL;

-- Index 2: previous_event_hash lookup (for chain traversal)
-- Used by: getPreviousEventHash() to find last event in chain
-- Note: Composite index (correlation_id, event_timestamp) already exists
--       so we don't need a separate index on previous_event_hash alone

-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- ADVISORY LOCK HELPER FUNCTION
-- ========================================

-- Function to convert correlation_id to consistent lock ID
-- Ensures same correlation_id always gets same lock ID
-- Range: PostgreSQL advisory locks use bigint (8 bytes)
CREATE OR REPLACE FUNCTION audit_event_lock_id(correlation_id_param TEXT)
RETURNS BIGINT AS $$
DECLARE
    hash_bytes BYTEA;
    lock_id BIGINT;
BEGIN
    -- Hash the correlation_id to get consistent lock ID
    hash_bytes := digest(correlation_id_param, 'sha256');

    -- Convert first 8 bytes to BIGINT
    -- Note: PostgreSQL advisory locks use BIGINT (signed 64-bit)
    lock_id := ('x' || encode(substring(hash_bytes, 1, 8), 'hex'))::bit(64)::bigint;

    RETURN lock_id;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION audit_event_lock_id IS
'Gap #9: Converts correlation_id to consistent advisory lock ID. Prevents race conditions during hash chain inserts.';

-- +goose StatementEnd

-- ========================================
-- MIGRATION VERIFICATION
-- ========================================

-- Verify columns exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'audit_events' AND column_name = 'event_hash'
    ) THEN
        RAISE EXCEPTION 'Migration failed: event_hash column not created';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'audit_events' AND column_name = 'previous_event_hash'
    ) THEN
        RAISE EXCEPTION 'Migration failed: previous_event_hash column not created';
    END IF;

    RAISE NOTICE 'Gap #9 migration successful: Hash chain columns added';
END $$;

-- ========================================
-- MIGRATION COMPLETE
-- ========================================

-- +goose Down
-- +goose StatementBegin
-- ========================================
-- ROLLBACK: Remove hash chain infrastructure
-- ========================================

-- Drop indexes
DROP INDEX IF EXISTS idx_audit_events_hash;

-- Drop helper function
DROP FUNCTION IF EXISTS audit_event_lock_id(TEXT);

-- Drop columns
ALTER TABLE audit_events DROP COLUMN IF EXISTS event_hash;
ALTER TABLE audit_events DROP COLUMN IF EXISTS previous_event_hash;

-- Note: We do NOT drop pgcrypto extension as other migrations may use it
-- If needed, drop manually: DROP EXTENSION IF EXISTS pgcrypto CASCADE;

RAISE NOTICE 'Gap #9 rollback complete: Hash chain removed';

-- +goose StatementEnd

