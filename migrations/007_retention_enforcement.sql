-- +goose Up
-- Migration 007: Retention enforcement infrastructure for Issue #485
-- BR-AUDIT-009: Retention policies for audit data
-- BR-AUDIT-004: Immutability / integrity of audit records
--
-- 1. Add CHECK constraint on retention_days [1, 2555] with default 1.
-- 2. Redesign retention_operations for batch purge logging (no FK to action_histories).
-- 3. Add SOC2 CC6.1 trigger: legal_hold cannot be set to FALSE via UPDATE.

-- 1. retention_days constraint (fresh installs: column already has DEFAULT 2555,
--    override to 1 minimum with CHECK).
ALTER TABLE audit_events ALTER COLUMN retention_days SET DEFAULT 1;
ALTER TABLE audit_events ADD CONSTRAINT chk_retention_days_range
    CHECK (retention_days >= 1 AND retention_days <= 2555);

-- 2. Redesign retention_operations for batch purge operations.
-- Fresh installs: drop and recreate cleanly.
DROP TABLE IF EXISTS retention_operations;

CREATE TABLE retention_operations (
    id BIGSERIAL PRIMARY KEY,
    run_id UUID NOT NULL DEFAULT gen_random_uuid(),
    scope VARCHAR(50) NOT NULL DEFAULT 'audit_events',
    period_start DATE,
    period_end DATE,
    rows_scanned INTEGER NOT NULL DEFAULT 0,
    rows_deleted INTEGER NOT NULL DEFAULT 0,
    partitions_dropped TEXT[] DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    error_message TEXT,
    operation_start TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    operation_end TIMESTAMP WITH TIME ZONE,
    operation_duration_ms INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_retention_ops_run_id ON retention_operations (run_id);
CREATE INDEX idx_retention_ops_start ON retention_operations (operation_start DESC);
CREATE INDEX idx_retention_ops_status ON retention_operations (status);

-- 3. SOC2 CC6.1: prevent legal_hold removal via UPDATE
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prevent_legal_hold_removal()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.legal_hold = TRUE AND NEW.legal_hold = FALSE THEN
        RAISE EXCEPTION 'SOC2 CC6.1: legal_hold cannot be removed via UPDATE. Use the legal hold release API with audit trail.'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER enforce_legal_hold_immutability
    BEFORE UPDATE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_removal();

-- +goose Down
-- Remove SOC2 trigger
DROP TRIGGER IF EXISTS enforce_legal_hold_immutability ON audit_events;
DROP FUNCTION IF EXISTS prevent_legal_hold_removal();

-- Restore original retention_operations
DROP TABLE IF EXISTS retention_operations;
CREATE TABLE retention_operations (
    id BIGSERIAL PRIMARY KEY,
    action_history_id BIGINT NOT NULL REFERENCES action_histories(id) ON DELETE CASCADE,
    operation_type VARCHAR(30) NOT NULL,
    strategy_used VARCHAR(30) NOT NULL,
    records_before INTEGER NOT NULL,
    records_after INTEGER NOT NULL,
    records_deleted INTEGER NOT NULL,
    records_archived INTEGER,
    retention_criteria JSONB,
    preserved_criteria JSONB,
    operation_start TIMESTAMP WITH TIME ZONE NOT NULL,
    operation_end TIMESTAMP WITH TIME ZONE,
    operation_duration_ms INTEGER,
    operation_status VARCHAR(20) DEFAULT 'running',
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_ro_action_history_ops ON retention_operations (action_history_id);
CREATE INDEX idx_ro_operation_time ON retention_operations (operation_start);

-- Remove retention_days constraint
ALTER TABLE audit_events DROP CONSTRAINT IF EXISTS chk_retention_days_range;
ALTER TABLE audit_events ALTER COLUMN retention_days SET DEFAULT 2555;
