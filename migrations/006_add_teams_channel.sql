-- Migration 006: Correct notification_audit channel CHECK constraint to match implemented channels
-- Issues: #60, #593
-- BR-NOT-104: Multi-channel notification delivery support
--
-- The original constraint in 001_v1_schema.sql listed channels that were never implemented
-- (email, sms) and omitted channels that are (file, console, log, teams).
-- This migration corrects the constraint to the full set of implemented delivery adapters.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE notification_audit
    DROP CONSTRAINT IF EXISTS notification_audit_channel_check;

ALTER TABLE notification_audit
    ADD CONSTRAINT notification_audit_channel_check
    CHECK (channel IN ('slack', 'pagerduty', 'teams', 'console', 'file', 'log'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE notification_audit
    DROP CONSTRAINT IF EXISTS notification_audit_channel_check;

ALTER TABLE notification_audit
    ADD CONSTRAINT notification_audit_channel_check
    CHECK (channel IN ('email', 'slack', 'pagerduty', 'sms'));
-- +goose StatementEnd
