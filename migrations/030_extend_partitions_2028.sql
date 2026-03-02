-- Extend partitions for resource_action_traces and audit_events through December 2028
-- Migration: 030_extend_partitions_2028.sql
-- Issue: #234 — PostgreSQL partition expiry caused DS E2E failures on 2026-03-01
--
-- Previous partitions ended at 2026-03-01 (exclusive). This migration adds monthly
-- partitions from March 2026 through December 2028 for both partitioned tables.
-- Uses CREATE TABLE IF NOT EXISTS for idempotency (safe to re-run).

-- ============================================================================
-- DEFAULT partitions: catch rows outside defined monthly ranges
-- ============================================================================

CREATE TABLE IF NOT EXISTS resource_action_traces_default
    PARTITION OF resource_action_traces
    DEFAULT;

CREATE TABLE IF NOT EXISTS audit_events_default
    PARTITION OF audit_events
    DEFAULT;

-- ============================================================================
-- resource_action_traces: March 2026 – December 2028
-- Naming convention: resource_action_traces_{YYYY}_{MM}
-- ============================================================================

-- 2026 (March – December)
CREATE TABLE IF NOT EXISTS resource_action_traces_2026_03
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_04
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_05
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_06
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_07
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_08
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_09
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_10
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_11
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2026_12
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- 2027 (January – December)
CREATE TABLE IF NOT EXISTS resource_action_traces_2027_01
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_02
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_03
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_04
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_05
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_06
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_07
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-07-01') TO ('2027-08-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_08
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-08-01') TO ('2027-09-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_09
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-09-01') TO ('2027-10-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_10
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-10-01') TO ('2027-11-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_11
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-11-01') TO ('2027-12-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2027_12
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2027-12-01') TO ('2028-01-01');

-- 2028 (January – December)
CREATE TABLE IF NOT EXISTS resource_action_traces_2028_01
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_02
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_03
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_04
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-04-01') TO ('2028-05-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_05
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-05-01') TO ('2028-06-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_06
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-06-01') TO ('2028-07-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_07
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-07-01') TO ('2028-08-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_08
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-08-01') TO ('2028-09-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_09
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-09-01') TO ('2028-10-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_10
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-10-01') TO ('2028-11-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_11
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-11-01') TO ('2028-12-01');

CREATE TABLE IF NOT EXISTS resource_action_traces_2028_12
    PARTITION OF resource_action_traces
    FOR VALUES FROM ('2028-12-01') TO ('2029-01-01');

-- ============================================================================
-- audit_events: March 2026 – December 2028
-- Naming convention: audit_events_{YYYY}_{MM}
-- ============================================================================

-- 2026 (March – December)
CREATE TABLE IF NOT EXISTS audit_events_2026_03
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_04
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_05
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_06
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_07
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_08
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_09
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_10
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_11
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');

CREATE TABLE IF NOT EXISTS audit_events_2026_12
    PARTITION OF audit_events
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- 2027 (January – December)
CREATE TABLE IF NOT EXISTS audit_events_2027_01
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_02
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_03
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_04
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_05
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_06
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_07
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-07-01') TO ('2027-08-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_08
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-08-01') TO ('2027-09-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_09
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-09-01') TO ('2027-10-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_10
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-10-01') TO ('2027-11-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_11
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-11-01') TO ('2027-12-01');

CREATE TABLE IF NOT EXISTS audit_events_2027_12
    PARTITION OF audit_events
    FOR VALUES FROM ('2027-12-01') TO ('2028-01-01');

-- 2028 (January – December)
CREATE TABLE IF NOT EXISTS audit_events_2028_01
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-01-01') TO ('2028-02-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_02
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-02-01') TO ('2028-03-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_03
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-03-01') TO ('2028-04-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_04
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-04-01') TO ('2028-05-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_05
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-05-01') TO ('2028-06-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_06
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-06-01') TO ('2028-07-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_07
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-07-01') TO ('2028-08-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_08
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-08-01') TO ('2028-09-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_09
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-09-01') TO ('2028-10-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_10
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-10-01') TO ('2028-11-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_11
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-11-01') TO ('2028-12-01');

CREATE TABLE IF NOT EXISTS audit_events_2028_12
    PARTITION OF audit_events
    FOR VALUES FROM ('2028-12-01') TO ('2029-01-01');
