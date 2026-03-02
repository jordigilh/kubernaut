-- +goose Up
-- +goose StatementBegin
-- ========================================
-- ADR-034: Unified Audit Table Design - Core Schema
-- Migration: Create audit_events table with event sourcing pattern
-- BR-STORAGE-032: Unified audit trail for compliance and cross-service correlation
-- Date: November 18, 2025
-- Version: 5.7 (Phase 1 of Day 21)
-- ========================================
--
-- ARCHITECTURE PATTERNS:
-- 1. Event Sourcing: Immutable, append-only audit trail
-- 2. Monthly Range Partitioning: Partition key event_date (generated column)
-- 3. JSONB Hybrid Storage: 27 structured columns + flexible JSONB
-- 4. GIN Index: Fast JSONB path queries
-- 5. UUID Primary Keys: Distributed system compatibility
-- 6. Parent-Child Relationships: FK constraint with ON DELETE RESTRICT (requires parent_event_date)
--
-- COMPLIANCE:
-- - SOC 2: Immutable audit trail, 7-year retention support
-- - ISO 27001: Long-term audit storage infrastructure
-- - GDPR: Sensitive data tracking via is_sensitive flag
--
-- ========================================

-- Create partitioned audit_events table
-- AUTHORITATIVE SOURCE: ADR-034 (Unified Audit Table Design)
-- This migration implements the exact schema from ADR-034
CREATE TABLE IF NOT EXISTS audit_events (
    -- ========================================
    -- EVENT IDENTITY
    -- ========================================
    event_id UUID NOT NULL DEFAULT gen_random_uuid(),
    event_version VARCHAR(10) NOT NULL DEFAULT '1.0',

    -- ========================================
    -- TEMPORAL INFORMATION
    -- ========================================
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL, -- For partitioning (generated from event_timestamp)

    -- Primary key must include partition key for partitioned tables
    PRIMARY KEY (event_id, event_date),

    -- ========================================
    -- EVENT CLASSIFICATION (ADR-034)
    -- ========================================
    event_type VARCHAR(100) NOT NULL,        -- 'gateway.signal.received'
    event_category VARCHAR(50) NOT NULL,     -- 'signal', 'remediation', 'workflow'
    event_action VARCHAR(50) NOT NULL,       -- 'received', 'processed', 'executed'
    event_outcome VARCHAR(20) NOT NULL,      -- 'success', 'failure', 'pending'

    -- ========================================
    -- ACTOR INFORMATION (Who)
    -- ========================================
    actor_type VARCHAR(50) NOT NULL,         -- 'service', 'external', 'user'
    actor_id VARCHAR(255) NOT NULL,          -- 'gateway-service', 'aws-cloudwatch'
    actor_ip INET,                           -- IP address of actor (optional)

    -- ========================================
    -- RESOURCE INFORMATION (What)
    -- ========================================
    resource_type VARCHAR(100) NOT NULL,     -- 'Signal', 'RemediationRequest'
    resource_id VARCHAR(255) NOT NULL,       -- 'fp-abc123', 'rr-2025-001'
    resource_name VARCHAR(255),              -- Human-readable resource name (optional)

    -- ========================================
    -- CONTEXT INFORMATION (Where/Why)
    -- ========================================
    correlation_id VARCHAR(255) NOT NULL,    -- remediation_id (groups related events)
    parent_event_id UUID,                    -- Links to parent event (optional)
    parent_event_date DATE,                  -- Parent event date (required for FK on partitioned table)
    trace_id VARCHAR(255),                   -- OpenTelemetry trace ID (optional)
    span_id VARCHAR(255),                    -- OpenTelemetry span ID (optional)

    -- ========================================
    -- KUBERNETES CONTEXT
    -- ========================================
    namespace VARCHAR(253),                  -- Kubernetes namespace (optional)
    cluster_name VARCHAR(255),               -- Cluster identifier (optional)

    -- ========================================
    -- EVENT PAYLOAD (JSONB - flexible, queryable)
    -- ========================================
    event_data JSONB NOT NULL,               -- Service-specific payload (common envelope format)
    event_metadata JSONB,                    -- Additional metadata (optional)

    -- ========================================
    -- AUDIT METADATA
    -- ========================================
    severity VARCHAR(20),                    -- 'info', 'warning', 'error', 'critical'
    duration_ms INTEGER,                     -- Operation duration in milliseconds
    error_code VARCHAR(50),                  -- Error code if outcome is failure
    error_message TEXT,                      -- Detailed error message

    -- ========================================
    -- COMPLIANCE
    -- ========================================
    retention_days INTEGER DEFAULT 2555,     -- 7 years (SOC 2 / ISO 27001)
    is_sensitive BOOLEAN DEFAULT FALSE       -- Flag for sensitive data (GDPR, PII)

) PARTITION BY RANGE (event_date);

-- ========================================
-- INDEXES (ADR-034 Standard Indexes)
-- ========================================

-- Index 1: Event timestamp (for time-range queries)
CREATE INDEX IF NOT EXISTS idx_audit_events_event_timestamp
    ON audit_events (event_timestamp DESC);

-- Index 2: Correlation ID (for cross-service correlation - most common query)
CREATE INDEX IF NOT EXISTS idx_audit_events_correlation_id
    ON audit_events (correlation_id, event_timestamp DESC);

-- Index 3: Event type (for filtering by event type)
CREATE INDEX IF NOT EXISTS idx_audit_events_event_type
    ON audit_events (event_type, event_timestamp DESC);

-- Index 4: Resource composite index (for resource-specific queries)
CREATE INDEX IF NOT EXISTS idx_audit_events_resource
    ON audit_events (resource_type, resource_id, event_timestamp DESC);

-- Index 5: Actor composite index (for actor-specific audit trails)
CREATE INDEX IF NOT EXISTS idx_audit_events_actor
    ON audit_events (actor_type, actor_id, event_timestamp DESC);

-- Index 6: Outcome (for success/failure analytics)
CREATE INDEX IF NOT EXISTS idx_audit_events_outcome
    ON audit_events (event_outcome, event_timestamp DESC);

-- Index 7: Event date (for partition pruning optimization)
CREATE INDEX IF NOT EXISTS idx_audit_events_event_date
    ON audit_events (event_date);

-- Index 8: GIN index for JSONB queries (1% of query volume, <500ms target)
CREATE INDEX IF NOT EXISTS idx_audit_events_event_data_gin
    ON audit_events USING GIN (event_data);

-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- TRIGGER: Auto-populate event_date from event_timestamp
-- ========================================

-- Trigger function to set event_date from event_timestamp
CREATE OR REPLACE FUNCTION set_audit_event_date()
RETURNS TRIGGER AS $$
BEGIN
    NEW.event_date := NEW.event_timestamp::DATE;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
-- Trigger to auto-populate event_date before INSERT
CREATE TRIGGER trg_set_audit_event_date
    BEFORE INSERT ON audit_events
    FOR EACH ROW
    EXECUTE FUNCTION set_audit_event_date();
-- +goose StatementEnd

-- ========================================
-- FOREIGN KEY CONSTRAINT (Parent-Child Relationships)
-- ========================================
-- +goose StatementBegin

-- Self-referencing FK with ON DELETE RESTRICT (enforces event sourcing immutability)
-- Rationale: Prevents accidental deletion of parent events with children
-- Requires: parent_event_date column (added 2025-11-18, see ADR-034 update)
-- PostgreSQL Requirement: FK constraints on partitioned tables must include partition key
ALTER TABLE audit_events
    ADD CONSTRAINT fk_audit_events_parent
    FOREIGN KEY (parent_event_id, parent_event_date)
    REFERENCES audit_events(event_id, event_date)
    ON DELETE RESTRICT;
-- +goose StatementEnd

-- ========================================
-- INITIAL PARTITIONS (Current month + 3 future months)
-- ========================================

-- Create partitions dynamically based on current date
-- Note: In production, use create_audit_events_partitions.sh for automation

DO $$
DECLARE
    current_month DATE := DATE_TRUNC('month', CURRENT_DATE);
    partition_start DATE;
    partition_end DATE;
    partition_name TEXT;
    i INT;
BEGIN
    -- Create 4 partitions: current month + 3 future months
    FOR i IN 0..3 LOOP
        partition_start := current_month + (i || ' months')::INTERVAL;
        partition_end := current_month + ((i + 1) || ' months')::INTERVAL;
        partition_name := 'audit_events_' || TO_CHAR(partition_start, 'YYYY_MM');

        -- Create partition if it doesn't exist
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF audit_events FOR VALUES FROM (%L) TO (%L)',
            partition_name,
            partition_start,
            partition_end
        );

        RAISE NOTICE 'Created partition: % for range [%, %)', partition_name, partition_start, partition_end;
    END LOOP;
END $$;

-- ========================================
-- COMMENTS FOR DOCUMENTATION
-- ========================================

COMMENT ON TABLE audit_events IS
'ADR-034: Unified audit trail for all Kubernaut services. Event sourcing pattern (immutable, append-only).';

COMMENT ON COLUMN audit_events.event_date IS
'Generated column from event_timestamp. Used for monthly range partitioning.';

COMMENT ON COLUMN audit_events.correlation_id IS
'Links events across services for complete remediation timeline (e.g., rr-2025-001).';

COMMENT ON COLUMN audit_events.parent_event_id IS
'Parent event for causality tracking. FK constraint enforces immutability with ON DELETE RESTRICT.';

COMMENT ON COLUMN audit_events.parent_event_date IS
'Parent event date (partition key). Required for FK constraint on partitioned tables (PostgreSQL requirement).';

COMMENT ON COLUMN audit_events.event_data IS
'JSONB payload with common envelope + service-specific data. GIN index for path queries.';

COMMENT ON COLUMN audit_events.is_sensitive IS
'GDPR compliance: Flag for sensitive data requiring special retention handling.';

-- +goose StatementBegin
-- ========================================
-- DEFAULT PARTITION
-- Catches any row outside defined monthly ranges (e.g., backdated test data).
-- Monthly partitions for Mar 2026–Dec 2028 are created by migrations 014/1000/030.
-- ========================================

CREATE TABLE IF NOT EXISTS audit_events_default
    PARTITION OF audit_events
    DEFAULT;
-- +goose StatementEnd

-- +goose StatementBegin
-- ========================================
-- GRANT PERMISSIONS (Event Sourcing: SELECT + INSERT only)
-- Rationale: No UPDATE or DELETE to enforce immutability
-- ========================================

-- Grant SELECT and INSERT to datastorage application user
-- Note: Assumes 'datastorage_app' role exists (created in earlier migrations)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'datastorage_app') THEN
        GRANT SELECT, INSERT ON audit_events TO datastorage_app;
        REVOKE UPDATE, DELETE ON audit_events FROM datastorage_app;
        RAISE NOTICE 'Granted SELECT/INSERT, revoked UPDATE/DELETE on audit_events for datastorage_app';
    ELSE
        RAISE NOTICE 'Role datastorage_app does not exist, skipping grants';
    END IF;
END $$;
-- +goose StatementEnd

-- ========================================
-- MIGRATION COMPLETE
-- ========================================

-- +goose Down
-- +goose StatementBegin
-- ========================================
-- ROLLBACK: Drop audit_events table and all partitions
-- ========================================

DROP TABLE IF EXISTS audit_events CASCADE;

-- Note: CASCADE will automatically drop all partitions
-- Note: This is a destructive operation - audit data will be lost

-- +goose StatementEnd

