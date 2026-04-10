-- +goose Up
-- Migration 006: Partition cleanup for Issue #235 / #620
-- BR-AUDIT-029: Automatic partition management
--
-- 1. Drop _default catch-all partitions (fail-loud strategy: inserts without
--    a matching partition will error immediately, preventing silent data silos).
-- 2. Fix set_audit_event_date trigger to use UTC (session TZ independence).
-- 3. Remove dead create_monthly_partitions() function (RAT-only, never called).

-- 1. Drop default partitions (fresh installs only — no data to drain)
DROP TABLE IF EXISTS audit_events_default;
DROP TABLE IF EXISTS resource_action_traces_default;

-- 2. Fix trigger to use UTC for event_date derivation
CREATE OR REPLACE FUNCTION set_audit_event_date()
RETURNS TRIGGER AS $$
BEGIN
    NEW.event_date := (NEW.event_timestamp AT TIME ZONE 'UTC')::DATE;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 3. Remove dead SQL function (replaced by Go EnsureMonthlyPartitions)
DROP FUNCTION IF EXISTS create_monthly_partitions();

-- +goose Down
-- Recreate default partitions
CREATE TABLE IF NOT EXISTS audit_events_default PARTITION OF audit_events DEFAULT;
CREATE TABLE IF NOT EXISTS resource_action_traces_default PARTITION OF resource_action_traces DEFAULT;

-- Restore original trigger (session TZ)
CREATE OR REPLACE FUNCTION set_audit_event_date()
RETURNS TRIGGER AS $$
BEGIN
    NEW.event_date := NEW.event_timestamp::DATE;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Restore dead function
CREATE OR REPLACE FUNCTION create_monthly_partitions()
RETURNS void AS $$
DECLARE
    start_date date;
    end_date date;
    table_name text;
BEGIN
    start_date := date_trunc('month', CURRENT_DATE + interval '1 month');
    end_date := start_date + interval '1 month';
    table_name := 'resource_action_traces_' || to_char(start_date, 'YYYY') || '_' || to_char(start_date, 'MM');
    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF resource_action_traces FOR VALUES FROM (%L) TO (%L)', table_name, start_date, end_date);
    RAISE NOTICE 'Created partition: %', table_name;
END;
$$ LANGUAGE plpgsql;
