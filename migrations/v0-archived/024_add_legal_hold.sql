-- +goose Up
-- +goose StatementBegin
-- ========================================
-- GAP #8: Legal Hold & Retention Policies
-- Migration: Add legal hold capability to audit_events
-- SOC2/SOX Requirement: Prevent deletion of events during litigation/investigation
-- Date: January 6, 2026
-- Authority: AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md - Day 8
-- ========================================
--
-- ARCHITECTURE:
-- 1. Legal hold flag prevents event deletion (database-level enforcement)
-- 2. Retention policies define lifecycle per event_category (7 years for SOX)
-- 3. Database trigger enforces legal hold (cannot be bypassed)
-- 4. Legal hold metadata captured (who, when, why)
--
-- COMPLIANCE:
-- - Sarbanes-Oxley: 7-year retention requirement (2555 days)
-- - HIPAA: Legal hold capability for litigation
-- - SOC 2 Type II: Retention policy management
--
-- APPROVED DECISIONS (Q1-Q4):
-- - Q1: correlation_id-based legal holds (entire incident flow)
-- - Q2: legal_hold column in audit_events table (simple boolean)
-- - Q3: DataStorage service cron for retention (deferred to v1.1)
-- - Q4: X-User-ID header authorization
--
-- ========================================

-- Step 1: Add legal_hold flag to audit_events
ALTER TABLE audit_events ADD COLUMN legal_hold BOOLEAN DEFAULT FALSE;

-- Step 2: Add legal hold metadata columns
ALTER TABLE audit_events ADD COLUMN legal_hold_reason TEXT;
ALTER TABLE audit_events ADD COLUMN legal_hold_placed_by TEXT;
ALTER TABLE audit_events ADD COLUMN legal_hold_placed_at TIMESTAMP;

-- Step 3: Create partial index (only TRUE values for performance)
-- Rationale: Very few events have legal holds, so partial index is more efficient
CREATE INDEX idx_audit_events_legal_hold ON audit_events(legal_hold) 
  WHERE legal_hold = TRUE;

COMMENT ON COLUMN audit_events.legal_hold IS
'Gap #8: Legal hold flag prevents deletion during litigation. Enforced by database trigger.';

COMMENT ON COLUMN audit_events.legal_hold_reason IS
'Gap #8: Reason for legal hold (e.g., "Litigation: Case #2026-ABC-123")';

COMMENT ON COLUMN audit_events.legal_hold_placed_by IS
'Gap #8: User who placed legal hold (from X-User-ID header)';

COMMENT ON COLUMN audit_events.legal_hold_placed_at IS
'Gap #8: Timestamp when legal hold was placed';

-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- RETENTION POLICIES TABLE
-- ========================================

CREATE TABLE audit_retention_policies (
    policy_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_category TEXT NOT NULL UNIQUE,  -- e.g., 'gateway', 'workflow', 'remediation'
    retention_days INTEGER NOT NULL,      -- e.g., 2555 (7 years for SOX)
    legal_hold_override BOOLEAN DEFAULT FALSE,  -- If TRUE, never delete even after retention
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE audit_retention_policies IS
'Gap #8: Retention policies per event_category. SOX requires 7 years (2555 days).';

COMMENT ON COLUMN audit_retention_policies.retention_days IS
'Retention period in days. Default: 2555 days = 7 years (Sarbanes-Oxley requirement)';

COMMENT ON COLUMN audit_retention_policies.legal_hold_override IS
'If TRUE, events in this category are never deleted (permanent retention)';

-- Insert default SOX-compliant retention policies (7 years = 2555 days)
INSERT INTO audit_retention_policies (event_category, retention_days) VALUES
    ('gateway', 2555),        -- Gateway signal events
    ('workflow', 2555),       -- Workflow execution events
    ('remediation', 2555),    -- Remediation orchestration events
    ('analysis', 2555),       -- AI analysis events
    ('notification', 2555);   -- Notification events

-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- LEGAL HOLD ENFORCEMENT TRIGGER
-- ========================================

-- Function to prevent deletion of events with legal hold
CREATE OR REPLACE FUNCTION prevent_legal_hold_deletion()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.legal_hold = TRUE THEN
        -- Raise exception with detailed error message for compliance audits
        RAISE EXCEPTION 'Cannot delete audit event with legal hold: event_id=%, correlation_id=%', 
            OLD.event_id, OLD.correlation_id
            USING HINT = 'Release legal hold before deletion via DELETE /api/v1/audit/legal-hold/{correlation_id}',
                  ERRCODE = '23503';  -- foreign_key_violation (closest match for constraint)
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION prevent_legal_hold_deletion IS
'Gap #8: Database-level enforcement of legal hold. Cannot be bypassed by application.';

-- Create trigger to enforce legal hold
CREATE TRIGGER enforce_legal_hold
    BEFORE DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_deletion();

-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- MIGRATION VERIFICATION
-- ========================================

-- Verify legal_hold column exists
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'audit_events' AND column_name = 'legal_hold'
    ) THEN
        RAISE EXCEPTION 'Migration failed: legal_hold column not created';
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'audit_retention_policies'
    ) THEN
        RAISE EXCEPTION 'Migration failed: audit_retention_policies table not created';
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'enforce_legal_hold'
    ) THEN
        RAISE EXCEPTION 'Migration failed: enforce_legal_hold trigger not created';
    END IF;
    
    RAISE NOTICE 'Gap #8 migration successful: Legal hold capability added';
END $$;

-- ========================================
-- MIGRATION COMPLETE
-- ========================================

-- +goose Down
-- +goose StatementBegin
-- ========================================
-- ROLLBACK: Remove legal hold infrastructure
-- ========================================

-- Drop trigger first
DROP TRIGGER IF EXISTS enforce_legal_hold ON audit_events;

-- Drop function
DROP FUNCTION IF EXISTS prevent_legal_hold_deletion();

-- Drop retention policies table
DROP TABLE IF EXISTS audit_retention_policies;

-- Drop index
DROP INDEX IF EXISTS idx_audit_events_legal_hold;

-- Drop columns from audit_events
ALTER TABLE audit_events DROP COLUMN IF EXISTS legal_hold;
ALTER TABLE audit_events DROP COLUMN IF EXISTS legal_hold_reason;
ALTER TABLE audit_events DROP COLUMN IF EXISTS legal_hold_placed_by;
ALTER TABLE audit_events DROP COLUMN IF EXISTS legal_hold_placed_at;

RAISE NOTICE 'Gap #8 rollback complete: Legal hold removed';

-- +goose StatementEnd

