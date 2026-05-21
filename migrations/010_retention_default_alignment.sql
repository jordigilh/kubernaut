-- +goose Up
-- Migration 010: Align retention_days column default with ADR-034 (SOC 2 / ISO 27001)
-- #1048 Phase 5 / AU-11: Migration 008 set DEFAULT 1 (minimum); restore to ADR-034 authority.
-- Operators can override via config `retention.defaultDays` at the application level.
ALTER TABLE audit_events ALTER COLUMN retention_days SET DEFAULT 2555;

-- +goose Down
-- Restore migration 008 default (minimum retention)
ALTER TABLE audit_events ALTER COLUMN retention_days SET DEFAULT 1;
